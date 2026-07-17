package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

// RefreshAnimeFromShikimori refreshes anime data from Shikimori
func (s *CatalogService) RefreshAnimeFromShikimori(ctx context.Context, animeID string) (*domain.Anime, error) {
	// Get anime from database
	existing, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	if existing.ShikimoriID == "" {
		return nil, errors.InvalidInput("anime does not have a shikimori_id")
	}

	s.log.Infow("refreshing anime from Shikimori",
		"anime_id", animeID,
		"shikimori_id", existing.ShikimoriID)

	// Fetch fresh data from Shikimori
	shikimoriAnime, err := s.shikimoriClient.GetAnimeByID(ctx, existing.ShikimoriID)
	if err != nil {
		return nil, fmt.Errorf("fetch from shikimori: %w", err)
	}

	// Preserve local ID and flags
	shikimoriAnime.ID = existing.ID
	shikimoriAnime.HasVideo = existing.HasVideo
	shikimoriAnime.CreatedAt = existing.CreatedAt

	// Update in database
	if err := s.animeRepo.Update(ctx, shikimoriAnime); err != nil {
		return nil, fmt.Errorf("update anime: %w", err)
	}

	// Update genres
	s.persistAnimeGenres(ctx, shikimoriAnime)

	// Invalidate cache
	_ = s.cache.Delete(ctx, cache.KeyAnime(animeID))

	s.enrichAnime(ctx, shikimoriAnime)
	return shikimoriAnime, nil
}

// BatchRefreshAnime refreshes all stale anime of a given status using batch Shikimori queries.
// Returns counts of refreshed and failed anime.
func (s *CatalogService) BatchRefreshAnime(ctx context.Context, status domain.AnimeStatus, staleBefore time.Time) (refreshed, failed int, err error) {
	staleAnime, err := s.animeRepo.GetStaleAnime(ctx, status, staleBefore)
	if err != nil {
		return 0, 0, err
	}

	if len(staleAnime) == 0 {
		s.log.Infow("no stale anime to refresh", "status", status)
		return 0, 0, nil
	}

	s.log.Infow("batch refreshing stale anime",
		"status", status,
		"count", len(staleAnime),
	)

	// Build shikimoriID -> existing anime map
	existingMap := make(map[string]*domain.Anime, len(staleAnime))
	var shikimoriIDs []string
	for _, a := range staleAnime {
		existingMap[a.ShikimoriID] = a
		shikimoriIDs = append(shikimoriIDs, a.ShikimoriID)
	}

	// Genres are a small global lookup (~80 rows) shared across every anime.
	// Track which we've already upserted so the whole run upserts each distinct
	// genre once instead of once per anime (~15k redundant writes → ~80).
	seenGenres := make(map[string]struct{})

	// Snapshot the stored genres once so we can also skip upserting a genre whose
	// name/name_ru already match — the steady state, since the taxonomy is
	// static, so without this every run still rewrites all ~80 rows. Best-effort:
	// on error we fall back to upserting every distinct genre as before.
	storedGenres := make(map[string]domain.Genre)
	if all, gerr := s.genreRepo.GetAll(ctx); gerr != nil {
		s.log.Warnw("failed to preload genres for change detection; will upsert all", "error", gerr)
	} else {
		for _, g := range all {
			storedGenres[g.ID] = g
		}
	}

	// Run-wide counters for observability: how many stale anime were actually
	// rewritten vs verified-unchanged (updated_at bulk-touched, no metadata write).
	var changedCount, unchangedCount int

	// Process in chunks of 50
	const batchSize = 50
	for i := 0; i < len(shikimoriIDs); i += batchSize {
		select {
		case <-ctx.Done():
			return refreshed, failed, ctx.Err()
		default:
		}

		end := i + batchSize
		if end > len(shikimoriIDs) {
			end = len(shikimoriIDs)
		}
		batch := shikimoriIDs[i:end]

		s.log.Infow("fetching batch from Shikimori",
			"batch", fmt.Sprintf("%d-%d/%d", i+1, end, len(shikimoriIDs)),
			"status", status,
		)

		freshAnime, err := s.shikimoriClient.GetAnimeByIDs(ctx, batch)
		if err != nil {
			s.log.Warnw("batch fetch failed", "error", err, "batch_start", i)
			failed += len(batch)
			continue
		}

		// anime ID -> genre IDs for the rows refreshed in this chunk; written to
		// the join table in one bulk DELETE+INSERT below instead of four
		// statements per anime.
		chunkLinks := make(map[string][]string, len(freshAnime))
		// IDs of anime whose metadata was unchanged this chunk — updated_at is
		// bulk-advanced for them after the loop (no per-row rewrite).
		var unchangedIDs []string

		for _, fresh := range freshAnime {
			existing, ok := existingMap[fresh.ShikimoriID]
			if !ok {
				continue
			}

			// Upsert genres we haven't seen yet this run, and only when the stored
			// row actually differs (best-effort; a failed upsert is left unmarked
			// so a later anime retries it).
			for _, g := range fresh.Genres {
				if _, done := seenGenres[g.ID]; done {
					continue
				}
				if cur, ok := storedGenres[g.ID]; ok && cur.Name == g.Name && cur.NameRU == g.NameRU {
					seenGenres[g.ID] = struct{}{} // already current — no write
					continue
				}
				if err := s.genreRepo.Upsert(ctx, &g); err != nil {
					s.log.Warnw("failed to upsert genre", "genre_id", g.ID, "error", err)
					continue
				}
				seenGenres[g.ID] = struct{}{}
				storedGenres[g.ID] = g // remember for the rest of the run
			}

			changed, err := s.refreshStaleAnime(ctx, fresh, existing)
			if err != nil {
				failed++
				continue
			}
			if changed {
				changedCount++
			} else {
				// Metadata unchanged: no row write. Collect for the bulk
				// updated_at touch below so it still leaves the stale window.
				unchangedCount++
				unchangedIDs = append(unchangedIDs, existing.ID)
			}

			// Record links only when Shikimori returned genres — an empty fetch
			// must NOT wipe the existing join rows (matches the prior per-anime
			// SetAnimeGenres guard). fresh.ID was set to existing.ID by
			// refreshStaleAnime.
			if len(fresh.Genres) > 0 {
				genreIDs := make([]string, len(fresh.Genres))
				for j, g := range fresh.Genres {
					genreIDs[j] = g.ID
				}
				chunkLinks[fresh.ID] = genreIDs
			}

			refreshed++
		}

		// Drop anime whose stored genre set already equals the freshly fetched
		// one, so the bulk DELETE+INSERT only runs for genuinely changed links
		// (the steady state changes nothing). Best-effort: on a read error we keep
		// the full set and rewrite as before.
		if len(chunkLinks) > 0 {
			ids := make([]string, 0, len(chunkLinks))
			for id := range chunkLinks {
				ids = append(ids, id)
			}
			if existingSets, gerr := s.genreRepo.GetForAnimes(ctx, ids); gerr != nil {
				s.log.Warnw("failed to load existing genre links; rewriting all", "error", gerr, "batch_start", i)
			} else {
				for id, want := range chunkLinks {
					have := make([]string, 0, len(existingSets[id]))
					for _, g := range existingSets[id] {
						have = append(have, g.ID)
					}
					if sameStringSet(have, want) {
						delete(chunkLinks, id)
					}
				}
			}
		}

		// Bulk-rewrite only the changed genre join rows (best-effort: the anime
		// metadata rows are already updated).
		if len(chunkLinks) > 0 {
			if err := s.genreRepo.ReplaceAnimeGenresBatch(ctx, chunkLinks); err != nil {
				s.log.Warnw("failed to bulk relink anime genres", "error", err, "batch_start", i)
			}
		}

		// Advance updated_at for the unchanged rows in one statement so they leave
		// the stale-refresh window without a full-row rewrite + index churn.
		if len(unchangedIDs) > 0 {
			if err := s.animeRepo.TouchUpdatedAt(ctx, unchangedIDs); err != nil {
				s.log.Warnw("failed to touch updated_at for unchanged anime", "error", err, "count", len(unchangedIDs), "batch_start", i)
			}
		}

		// Rate limit safety between batches
		if end < len(shikimoriIDs) {
			time.Sleep(200 * time.Millisecond)
		}
	}

	s.log.Infow("batch refresh completed",
		"status", status,
		"refreshed", refreshed,
		"changed", changedCount,
		"unchanged", unchangedCount,
		"failed", failed,
	)
	return refreshed, failed, nil
}

// sameStringSet reports whether a and b contain the same set of strings,
// ignoring order and duplicates. Used to detect an unchanged genre-ID set so the
// batch refresh can skip rewriting a join row that would be identical.
func sameStringSet(a, b []string) bool {
	sa := make(map[string]struct{}, len(a))
	for _, s := range a {
		sa[s] = struct{}{}
	}
	sb := make(map[string]struct{}, len(b))
	for _, s := range b {
		sb[s] = struct{}{}
	}
	if len(sa) != len(sb) {
		return false
	}
	for s := range sa {
		if _, ok := sb[s]; !ok {
			return false
		}
	}
	return true
}

// refreshStaleAnime reconciles a single stale anime against fresh Shikimori data,
// preserving local-only fields (ID, HasVideo, CreatedAt) and the existing MAL
// poster. It writes (and busts the per-anime cache) ONLY when a metadata column
// actually changed; when the fetch was identical it writes nothing and reports
// changed=false, leaving the caller to bulk-advance updated_at instead of doing a
// full-row rewrite + secondary-index churn per unchanged anime. Genre upserts and
// join-table relinking are done in bulk by the BatchRefreshAnime caller. Returns
// an error only when the primary anime row update fails.
func (s *CatalogService) refreshStaleAnime(ctx context.Context, fresh, existing *domain.Anime) (changed bool, err error) {
	// Preserve local fields
	fresh.ID = existing.ID
	fresh.HasVideo = existing.HasVideo
	fresh.CreatedAt = existing.CreatedAt

	// Defend an AniList-corroborated next-episode date against this Shikimori
	// batch refresh, which would otherwise clobber the correction.
	defendAniListNextEpisode(fresh, existing)

	// Preserve or fetch MAL poster if Shikimori has none
	if fresh.PosterURL == "" {
		if existing.PosterURL != "" {
			fresh.PosterURL = existing.PosterURL
		} else {
			s.fetchMALPosterIfMissing(ctx, fresh)
		}
	}

	// Skip the write entirely when Shikimori returned unchanged metadata — the
	// common case for released/announced titles. Comparison runs AFTER the
	// preserve/defend/poster fixups above so it reflects exactly what Update
	// would persist. The caller advances updated_at in bulk for these rows.
	if repo.AnimeMetadataEqual(fresh, existing) {
		return false, nil
	}

	if err := s.animeRepo.Update(ctx, fresh); err != nil {
		s.log.Warnw("failed to update anime", "id", existing.ID, "error", err)
		return false, err
	}

	// Genre upserts and join relinking are handled in bulk by the
	// BatchRefreshAnime caller (one upsert per distinct genre across the whole
	// run, one bulk join rewrite per chunk) — not per anime here.

	// Invalidate cache (only when something actually changed)
	_ = s.cache.Delete(ctx, cache.KeyAnime(existing.ID))
	return true, nil
}

// SyncCalendar fetches the Shikimori calendar and imports/updates anime schedule data.
// For anime not in the local DB, fetches full details from Shikimori and creates them.
// For anime already in the DB, updates next_episode_at.
// Returns counts of imported, updated, and failed anime.
func (s *CatalogService) SyncCalendar(ctx context.Context) (imported, updated, failed int, err error) {
	s.log.Info("starting calendar sync from Shikimori")

	calendar, err := s.shikimoriClient.GetCalendar(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("fetch calendar: %w", err)
	}

	s.log.Infow("fetched calendar entries", "count", len(calendar))

	seen := dedupeCalendarEntries(calendar)

	// Corroborate Shikimori's naive next-episode dates against AniList's
	// broadcaster schedule (later-wins). Best-effort: failures keep Shikimori.
	s.reconcileCalendarWithAniList(ctx, seen)

	// Check which anime already exist locally.
	missingIDs, existingByShikimoriID, partFailed, err := s.partitionCalendarAnime(ctx, seen)
	failed += partFailed
	if err != nil {
		return imported, updated, failed, err
	}

	s.log.Infow("calendar sync status",
		"total_unique", len(seen),
		"already_in_db", len(existingByShikimoriID),
		"missing", len(missingIDs),
	)

	// Batch-fetch + import missing anime from Shikimori.
	imp, impFailed, err := s.importMissingCalendarAnime(ctx, missingIDs, seen)
	imported += imp
	failed += impFailed
	if err != nil {
		return imported, updated, failed, err
	}

	// Update next_episode_at for anime already in DB.
	upd, updFailed := s.updateExistingCalendarEpisodes(ctx, existingByShikimoriID, seen)
	updated += upd
	failed += updFailed

	s.log.Infow("calendar sync completed",
		"imported", imported,
		"updated", updated,
		"failed", failed,
	)

	return imported, updated, failed, nil
}

// calendarInfo holds the deduplicated next-episode air time for a calendar anime.
type calendarInfo struct {
	shikimoriID   string
	nextEpisodeAt *time.Time
	source        string // sourceShikimori (default) or sourceAniList after reconciliation
	status        string // Shikimori anime status ("ongoing", "anons", …); gates AniList reconciliation to ongoing only
}

// dedupeCalendarEntries collapses calendar entries (one per upcoming episode) to
// one record per anime, keeping the first (earliest) entry seen.
func dedupeCalendarEntries(calendar []shikimori.CalendarEntry) map[string]*calendarInfo {
	seen := make(map[string]*calendarInfo)
	for _, entry := range calendar {
		id := strconv.Itoa(entry.Anime.ID)
		if _, exists := seen[id]; exists {
			continue // keep the first (earliest) entry
		}
		info := &calendarInfo{shikimoriID: id, source: sourceShikimori, status: entry.Anime.Status}
		if entry.NextEpisodeAt != "" {
			if t, err := time.Parse(time.RFC3339, entry.NextEpisodeAt); err == nil {
				info.nextEpisodeAt = &t
			}
		}
		seen[id] = info
	}
	return seen
}

// partitionCalendarAnime splits the deduplicated calendar anime into those already
// in the local DB (returned as a map) and those still missing (returned as IDs).
// Returns the number of lookups that failed, and ctx.Err() if the context is cancelled.
func (s *CatalogService) partitionCalendarAnime(ctx context.Context, seen map[string]*calendarInfo) (missingIDs []string, existingByShikimoriID map[string]*domain.Anime, failed int, err error) {
	existingByShikimoriID = make(map[string]*domain.Anime)

	for id := range seen {
		select {
		case <-ctx.Done():
			return missingIDs, existingByShikimoriID, failed, ctx.Err()
		default:
		}

		existing, err := s.animeRepo.GetByShikimoriID(ctx, id)
		if err != nil {
			s.log.Warnw("failed to check anime existence", "shikimori_id", id, "error", err)
			failed++
			continue
		}
		if existing != nil {
			existingByShikimoriID[id] = existing
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	return missingIDs, existingByShikimoriID, failed, nil
}

// importMissingCalendarAnime batch-fetches missing anime from Shikimori (50 per
// batch, conservative rate limiting between batches) and upserts them, overriding
// next_episode_at with the more accurate calendar value when available. Returns the
// imported and failed counts, and ctx.Err() if the context is cancelled.
func (s *CatalogService) importMissingCalendarAnime(ctx context.Context, missingIDs []string, seen map[string]*calendarInfo) (imported, failed int, err error) {
	const batchSize = 50
	for i := 0; i < len(missingIDs); i += batchSize {
		select {
		case <-ctx.Done():
			return imported, failed, ctx.Err()
		default:
		}

		end := i + batchSize
		if end > len(missingIDs) {
			end = len(missingIDs)
		}
		batch := missingIDs[i:end]

		s.log.Infow("fetching missing anime batch from Shikimori",
			"batch", fmt.Sprintf("%d-%d/%d", i+1, end, len(missingIDs)),
		)

		freshAnime, err := s.shikimoriClient.GetAnimeByIDs(ctx, batch)
		if err != nil {
			s.log.Warnw("batch fetch for calendar sync failed", "error", err)
			failed += len(batch)
			continue
		}

		for _, anime := range freshAnime {
			// Override next_episode_at + source from reconciled calendar data.
			// Source is written whenever the calendar knows this anime, even when
			// it has no air date, so provenance is never left blank.
			if info, ok := seen[anime.ShikimoriID]; ok {
				anime.NextEpisodeSource = info.source
				if info.nextEpisodeAt != nil {
					anime.NextEpisodeAt = info.nextEpisodeAt
				}
			}

			if err := s.upsertAnimeFromExternal(ctx, anime); err != nil {
				s.log.Warnw("failed to import calendar anime",
					"shikimori_id", anime.ShikimoriID, "error", err)
				failed++
				continue
			}
			imported++
		}

		// Conservative rate limiting between batches
		if end < len(missingIDs) {
			time.Sleep(2 * time.Second)
		}
	}

	return imported, failed, nil
}

// updateExistingCalendarEpisodes refreshes next_episode_at for anime already in the
// DB whose calendar value differs, invalidating the per-anime cache on each update.
// Returns the updated and failed counts.
func (s *CatalogService) updateExistingCalendarEpisodes(ctx context.Context, existingByShikimoriID map[string]*domain.Anime, seen map[string]*calendarInfo) (updated, failed int) {
	for id, existing := range existingByShikimoriID {
		info := seen[id]
		if info.nextEpisodeAt == nil {
			continue
		}

		// Only update if the calendar has a different/newer value OR the source
		// provenance changed (so a source-only correction still persists).
		if existing.NextEpisodeAt != nil && existing.NextEpisodeAt.Equal(*info.nextEpisodeAt) &&
			existing.NextEpisodeSource == info.source {
			continue
		}

		existing.NextEpisodeAt = info.nextEpisodeAt
		existing.NextEpisodeSource = info.source
		if err := s.animeRepo.Update(ctx, existing); err != nil {
			s.log.Warnw("failed to update next_episode_at",
				"id", existing.ID, "shikimori_id", id, "error", err)
			failed++
			continue
		}

		// Invalidate cache
		_ = s.cache.Delete(ctx, cache.KeyAnime(existing.ID))
		updated++
	}

	return updated, failed
}

// SyncAnnouncements discovers featured announced (anons) titles from
// Shikimori and prepares them for S8/announcement matching (spec 2026-07-17):
//
//  1. Fetch top-`limit` anons titles by community popularity (the implicit
//     "featured" gate) and upsert them — new titles are imported with
//     genres; existing rows get status/metadata refreshed (this also
//     persists announced→ongoing transitions between batch-refresh runs).
//  2. Franchise-enrich the announced titles (S8 candidate side).
//  3. Franchise-enrich up to `seedBackfillLimit` list-referenced anime that
//     were never franchise-checked (S8 seed side) — converges the sparse
//     franchise coverage (425/4942 rows as of 2026-07-17) where it matters.
//
// Per-title failures are logged and counted, never fatal. Mirrors
// SyncCalendar's structure; called by the scheduler daily.
func (s *CatalogService) SyncAnnouncements(ctx context.Context, limit, seedBackfillLimit int) (imported, refreshed, enriched, failed int, err error) {
	s.log.Infow("starting announcements sync from Shikimori", "limit", limit, "seed_backfill", seedBackfillLimit)

	announced, err := s.shikimoriClient.GetAnnouncedAnime(ctx, 1, limit)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("fetch announced: %w", err)
	}

	// 1. Upsert (import missing / refresh existing).
	for _, anime := range announced {
		select {
		case <-ctx.Done():
			return imported, refreshed, enriched, failed, ctx.Err()
		default:
		}
		existing, gerr := s.animeRepo.GetByShikimoriID(ctx, anime.ShikimoriID)
		if gerr != nil {
			s.log.Warnw("announcements sync: existence check failed", "shikimori_id", anime.ShikimoriID, "error", gerr)
			failed++
			continue
		}
		if uerr := s.upsertAnimeFromExternal(ctx, anime); uerr != nil {
			s.log.Warnw("announcements sync: upsert failed", "shikimori_id", anime.ShikimoriID, "error", uerr)
			failed++
			continue
		}
		if existing == nil {
			imported++
		} else {
			refreshed++
		}
	}

	// 2+3. Franchise enrichment: announced candidates + list-referenced seeds.
	enrichPool := make([]*domain.Anime, 0, limit+seedBackfillLimit)
	for _, anime := range announced {
		row, gerr := s.animeRepo.GetByShikimoriID(ctx, anime.ShikimoriID)
		if gerr != nil || row == nil {
			continue
		}
		enrichPool = append(enrichPool, row)
	}
	if seedBackfillLimit > 0 {
		seeds, serr := s.animeRepo.ListFranchiseUncheckedListed(ctx, seedBackfillLimit)
		if serr != nil {
			s.log.Warnw("announcements sync: seed backfill pool query failed", "error", serr)
		} else {
			enrichPool = append(enrichPool, seeds...)
		}
	}

	// Dedupe enrichPool by anime ID: announced titles may also appear in list-referenced seeds.
	seen := make(map[string]bool)
	deduped := make([]*domain.Anime, 0, len(enrichPool))
	for _, a := range enrichPool {
		if !seen[a.ID] {
			seen[a.ID] = true
			deduped = append(deduped, a)
		}
	}
	enrichPool = deduped

	for _, a := range enrichPool {
		select {
		case <-ctx.Done():
			return imported, refreshed, enriched, failed, ctx.Err()
		default:
		}
		if a.FranchiseChecked || a.Franchise != "" || a.ShikimoriID == "" {
			continue
		}
		fr, ferr := s.shikimoriClient.GetAnimeFranchise(ctx, a.ShikimoriID)
		if ferr != nil {
			// Not marked checked — retried on the next daily run.
			s.log.Debugw("announcements sync: franchise fetch failed", "anime_id", a.ID, "shikimori_id", a.ShikimoriID, "error", ferr)
			failed++
			continue
		}
		if serr := s.animeRepo.SetFranchise(ctx, a.ID, fr); serr != nil {
			s.log.Warnw("announcements sync: persist franchise failed", "anime_id", a.ID, "error", serr)
			failed++
			continue
		}
		enriched++
	}

	s.log.Infow("announcements sync completed",
		"imported", imported, "refreshed", refreshed, "enriched", enriched, "failed", failed)
	return imported, refreshed, enriched, failed, nil
}
