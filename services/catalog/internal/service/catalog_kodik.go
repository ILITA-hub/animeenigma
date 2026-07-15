package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/kodikextract"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/streamsign"
)

// GetKodikTranslations gets available translations from Kodik for an anime
func (s *CatalogService) GetKodikTranslations(ctx context.Context, animeID string) (_ []domain.KodikTranslation, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("kodik", "get_translations", start, &retErr)
	if s.kodikClient == nil {
		s.log.Warnw("kodik client not initialized, returning empty translations",
			"anime_id", animeID)
		return []domain.KodikTranslation{}, nil
	}

	// Get anime to get Shikimori ID
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	if anime.ShikimoriID == "" {
		s.log.Warnw("anime does not have shikimori_id, cannot fetch kodik translations",
			"anime_id", animeID)
		return []domain.KodikTranslation{}, nil
	}

	// Check cache first
	cacheKey := fmt.Sprintf("kodik:translations:%s", animeID)
	var cached []domain.KodikTranslation
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Get translations from Kodik (with retry on failure)
	translations, err := s.kodikClient.GetTranslations(ctx, anime.ShikimoriID)
	if err != nil {
		s.log.Warnw("failed to get kodik translations, returning empty list",
			"anime_id", animeID,
			"shikimori_id", anime.ShikimoriID,
			"error", err)
		// Return empty list instead of error - Kodik is not critical
		return []domain.KodikTranslation{}, nil
	}

	result := make([]domain.KodikTranslation, len(translations))
	for i, t := range translations {
		result[i] = domain.KodikTranslation{
			ID:            t.ID,
			Title:         t.Title,
			Type:          t.Type,
			EpisodesCount: t.EpisodesCount,
		}
	}

	// Phase 9 (UX-18): lazily backfill animes.has_dub whenever the catalog
	// touches Kodik for this anime. Best-effort — failures are logged but
	// non-fatal (the dub badge is decorative, never blocks playback). Only
	// write when the computed value differs from the stored row to avoid
	// noisy UPDATE traffic.
	hasDub := kodik.TranslationsHaveDub(translations)
	if hasDub != anime.HasDub {
		if updateErr := s.animeRepo.SetHasDub(ctx, anime.ID, hasDub); updateErr != nil {
			s.log.Warnw("failed to persist anime.has_dub from kodik translations",
				"anime_id", animeID,
				"has_dub", hasDub,
				"error", updateErr)
		}
	}

	// Phase 15 (UX-31): lazily backfill animes.has_kodik. Reaching this
	// point means Kodik returned >=1 translation, so the anime IS available
	// on Kodik. Best-effort; idempotent guard against noisy UPDATEs.
	if len(translations) > 0 && !anime.HasKodik {
		if updateErr := s.animeRepo.SetHasKodik(ctx, anime.ID, true); updateErr != nil {
			s.log.Warnw("failed to persist anime.has_kodik",
				"anime_id", anime.ID,
				"error", updateErr)
		}
	}

	// Cache for 1 hour
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}

// GetKodikVideoSource gets video embed link from Kodik for a specific episode
func (s *CatalogService) GetKodikVideoSource(ctx context.Context, animeID string, episode int, translationID int) (_ *domain.KodikVideoSource, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("kodik", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("kodik").Inc()
	if s.kodikClient == nil {
		return nil, errors.NotFound("kodik not available")
	}

	// Get anime to get Shikimori ID
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	if anime.ShikimoriID == "" {
		return nil, errors.NotFound("anime does not have shikimori_id")
	}

	// Check cache first
	cacheKey := fmt.Sprintf("kodik:video:%s:%d:%d", animeID, episode, translationID)
	var cached domain.KodikVideoSource
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	// Get episode link from Kodik
	embedLink, err := s.kodikClient.GetEpisodeLink(ctx, anime.ShikimoriID, episode, translationID)
	if err != nil {
		s.log.Warnw("failed to get kodik video source",
			"anime_id", animeID,
			"shikimori_id", anime.ShikimoriID,
			"episode", episode,
			"translation_id", translationID,
			"error", err)
		return nil, errors.NotFound("video not found on kodik")
	}

	// Get translation name
	translationName := ""
	translations, _ := s.kodikClient.GetTranslations(ctx, anime.ShikimoriID)
	for _, t := range translations {
		if t.ID == translationID {
			translationName = t.Title
			break
		}
	}

	result := &domain.KodikVideoSource{
		EmbedLink:     embedLink,
		Episode:       episode,
		TranslationID: translationID,
		Translation:   translationName,
	}

	// Cache for 1 hour (embed links are relatively stable)
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}

// GetKodikStreamSource resolves the ad-free HLS stream for a Kodik episode.
// quality<=0 means "use the provider default / highest".
func (s *CatalogService) GetKodikStreamSource(ctx context.Context, animeID string, episode, translationID, quality int) (_ *domain.KodikStreamSource, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("kodik", "get_adfree_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("kodik-adfree").Inc()
	if s.kodikClient == nil {
		return nil, errors.NotFound("kodik not available")
	}

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime.ShikimoriID == "" {
		return nil, errors.NotFound("anime does not have shikimori_id")
	}

	cacheKey := fmt.Sprintf("kodik:stream:%s:%d:%d:%d", animeID, episode, translationID, quality)
	var cached domain.KodikStreamSource
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		signKodikStreamSource(&cached)
		return &cached, nil
	}

	embedLink, err := s.kodikClient.GetEpisodeLink(ctx, anime.ShikimoriID, episode, translationID)
	if err != nil {
		s.log.Warnw("kodik adfree: embed link failed", "anime_id", animeID, "episode", episode, "translation_id", translationID, "error", err)
		return nil, errors.NotFound("video not found on kodik")
	}

	// AR-EGRESS-03 (D-08): route the Kodik extractor's outbound requests through
	// the recording transport when a wrap func is configured, preserving the
	// per-call cookie jar + IPv4 dialer. Falls back to the package default.
	var resolved *kodikextract.Result
	if s.kodikExtractWrap != nil {
		resolved, err = kodikextract.ResolveWithClient(ctx, embedLink,
			kodikextract.NewRecordingClient(s.kodikExtractWrap))
	} else {
		resolved, err = kodikextract.Resolve(ctx, embedLink)
	}
	if err != nil {
		s.log.Warnw("kodik adfree: extraction failed", "anime_id", animeID, "embed", embedLink, "error", err)
		return nil, errors.NotFound("could not extract kodik stream")
	}
	chosen := resolved.PickQuality(quality)

	qualities := make([]int, 0, len(resolved.Streams))
	for _, st := range resolved.Streams {
		qualities = append(qualities, st.Quality)
	}

	translationName := ""
	if translations, terr := s.kodikClient.GetTranslations(ctx, anime.ShikimoriID); terr == nil {
		for _, t := range translations {
			if t.ID == translationID {
				translationName = t.Title
				break
			}
		}
	}

	source := &domain.KodikStreamSource{
		StreamURL:     chosen.M3U8URL,
		Referer:       resolved.Referer,
		Quality:       chosen.Quality,
		Qualities:     qualities,
		Episode:       episode,
		TranslationID: translationID,
		Translation:   translationName,
	}

	// Cache <1h — the CDN URL carries an expiry token. Cached BEFORE signing so
	// the Redis body never persists a signature; signatures are minted at
	// response time on both this path and the cache-hit path above.
	_ = s.cache.Set(ctx, cacheKey, source, 30*time.Minute)
	signKodikStreamSource(source)
	return source, nil
}

// signKodikStreamSource mints the provenance signature + Track A masked form
// for the Kodik CDN (.m3u8 on solodcdn.com) stream URL, authorizing it through
// the HLS proxy without a static allowlist entry. Called at RESPONSE time on
// both the fresh and cache-hit paths — never persisted in the cache — so a
// cached entry can never outlive (or lack) its signature. Copies the animejoy
// pattern (GetAnimejoyStream).
func signKodikStreamSource(src *domain.KodikStreamSource) {
	src.Exp, src.Sig, src.MaskedURL = streamsign.Stamp(src.StreamURL, src.Referer, "")
}

// SearchKodik searches for anime on Kodik by title
func (s *CatalogService) SearchKodik(ctx context.Context, title string) ([]domain.KodikSearchResult, error) {
	if s.kodikClient == nil {
		return nil, errors.Internal("kodik client not initialized")
	}

	results, err := s.kodikClient.SearchByTitle(ctx, title)
	if err != nil {
		return nil, err
	}

	searchResults := make([]domain.KodikSearchResult, 0, len(results))
	seen := make(map[string]bool)

	for _, r := range results {
		// Deduplicate by title (different translations appear as separate results)
		if seen[r.Title] {
			continue
		}
		seen[r.Title] = true

		var translation *domain.KodikTranslation
		if r.Translation != nil {
			translation = &domain.KodikTranslation{
				ID:    r.Translation.ID,
				Title: r.Translation.Title,
				Type:  r.Translation.Type,
			}
		}

		searchResults = append(searchResults, domain.KodikSearchResult{
			ID:            r.ID,
			Type:          r.Type,
			Link:          r.Link,
			Title:         r.Title,
			TitleOrig:     r.TitleOrig,
			Year:          r.Year,
			EpisodesCount: r.EpisodesCount,
			ShikimoriID:   r.ShikimoriID,
			Translation:   translation,
			Quality:       r.Quality,
		})
	}

	return searchResults, nil
}
