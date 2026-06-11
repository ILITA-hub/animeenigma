package repo

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// allowedSortFields is a whitelist of column names that can appear in ORDER BY.
var allowedSortFields = map[string]bool{
	"updated_at": true,
	"created_at": true,
	"score":      true,
	"status":     true,
	"episodes":   true,
	"title":      true, // handled specially via JOIN
	"genre":      true, // handled specially via representative-genre subquery
}

// genreSortJoin derives one representative (alphabetically-first) genre per anime
// so the my-list query can ORDER BY a real column. Genres are many-to-many
// (anime_genres → genres) with no denormalized column. We deliberately use a
// joined derived-table column rather than a correlated subquery in ORDER BY:
// GORM applies a raw subquery string passed to .Order() unreliably (it gets
// silently dropped, leaving the default order), whereas a qualified joined column
// — like the existing animes.name title sort — is applied correctly. The LEFT
// JOIN is 1:1 (GROUP BY anime_id) so it never changes the row Count. Anime with
// no genres yield NULL min_genre (Postgres sorts NULLs last ASC, first DESC).
const genreSortJoin = "LEFT JOIN (SELECT ag.anime_id, MIN(g.name) AS min_genre FROM anime_genres ag JOIN genres g ON g.id = ag.genre_id GROUP BY ag.anime_id) genre_sort ON genre_sort.anime_id = anime_list.anime_id"

// genreSortOrderColumn is the ORDER BY target the join above exposes.
const genreSortOrderColumn = "genre_sort.min_genre"

// sanitizedOrderClause returns a safe ORDER BY clause built from validated
// sort field and direction. If either value is invalid, it falls back to
// "updated_at DESC".
func sanitizedOrderClause(sort, order string) string {
	if !allowedSortFields[sort] {
		return "updated_at DESC"
	}
	dir := strings.ToUpper(order)
	if dir != "ASC" && dir != "DESC" {
		dir = "DESC"
	}
	switch sort {
	case "title":
		return "animes.name " + dir
	case "genre":
		return genreSortOrderColumn + " " + dir
	}
	return "anime_list." + sort + " " + dir
}

// isTitleSort returns true when the requested sort requires a JOIN with animes.
func isTitleSort(sort string) bool {
	return sort == "title"
}

// isGenreSort returns true when the requested sort requires the derived genre join.
func isGenreSort(sort string) bool {
	return sort == "genre"
}

type ListRepository struct {
	db *gorm.DB
}

func NewListRepository(db *gorm.DB) *ListRepository {
	return &ListRepository{db: db}
}

func (r *ListRepository) Upsert(ctx context.Context, entry *domain.AnimeListEntry) error {
	now := time.Now()
	entry.UpdatedAt = now
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"status":        entry.Status,
			"score":         entry.Score,
			"episodes":      entry.Episodes,
			"notes":         gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.notes)", entry.Notes),
			"tags":          gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.tags)", entry.Tags),
			"is_rewatching": entry.IsRewatching,
			"rewatch_count": entry.RewatchCount,
			"priority":      gorm.Expr("COALESCE(NULLIF(?, ''), anime_list.priority)", entry.Priority),
			"mal_id":        gorm.Expr("COALESCE(?, anime_list.mal_id)", entry.MalID),
			"started_at":    gorm.Expr("COALESCE(?, anime_list.started_at)", entry.StartedAt),
			"completed_at":  gorm.Expr("COALESCE(?, anime_list.completed_at)", entry.CompletedAt),
			"updated_at":    entry.UpdatedAt,
		}),
	}).Create(entry).Error
}

func (r *ListRepository) GetByUser(ctx context.Context, userID string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Find(&entries).Error
	return entries, err
}

func (r *ListRepository) GetByUserAndStatus(ctx context.Context, userID, status string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("user_id = ? AND status = ?", userID, status).
		Order("updated_at DESC").
		Find(&entries).Error
	return entries, err
}

func (r *ListRepository) GetByUserAndAnime(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	var entry domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &entry, err
}

// GetAnimeMALID returns the catalog-owned animes.mal_id for an anime UUID,
// or "" when the row is missing / has no MAL id. Lets the viewer-context
// aggregate resolve legacy "mal_{id}" anime_list entries server-side, so the
// frontend can fire the request from a route guard before the anime metadata
// response (which used to carry mal_id) has arrived.
func (r *ListRepository) GetAnimeMALID(ctx context.Context, animeID string) (string, error) {
	var malID string
	err := r.db.WithContext(ctx).
		Raw(`SELECT COALESCE(mal_id, '') FROM animes WHERE id = ?`, animeID).
		Scan(&malID).Error
	return malID, err
}

func (r *ListRepository) Delete(ctx context.Context, userID, animeID string) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		Delete(&domain.AnimeListEntry{}).Error
}

func (r *ListRepository) IncrementEpisodes(ctx context.Context, userID, animeID string, episodeNumber int) (bool, error) {
	// The rewatch_count / is_rewatching CASE branches fire only on the
	// watching→completed transition (finale reached) AND only when a rewatch is
	// in progress: completing a rewatch bumps the tally once and clears the flag.
	// A normal first completion (is_rewatching=false) leaves both untouched.
	// Design 2026-06-05.
	result := r.db.WithContext(ctx).Exec(`
		UPDATE anime_list SET
			episodes = ?,
			status = CASE
				WHEN a.episodes_count > 0 AND ? >= a.episodes_count THEN 'completed'
				WHEN anime_list.status = 'plan_to_watch' THEN 'watching'
				ELSE anime_list.status END,
			rewatch_count = CASE
				WHEN a.episodes_count > 0 AND ? >= a.episodes_count AND anime_list.is_rewatching
					THEN anime_list.rewatch_count + 1
				ELSE anime_list.rewatch_count END,
			is_rewatching = CASE
				WHEN a.episodes_count > 0 AND ? >= a.episodes_count AND anime_list.is_rewatching
					THEN false
				ELSE anime_list.is_rewatching END,
			started_at = COALESCE(anime_list.started_at, NOW()),
			completed_at = CASE
				WHEN a.episodes_count > 0 AND ? >= a.episodes_count THEN NOW()
				ELSE anime_list.completed_at END,
			updated_at = NOW()
		FROM animes a
		WHERE anime_list.anime_id = a.id
		  AND anime_list.user_id = ? AND anime_list.anime_id = ?
		  AND anime_list.episodes < ?`,
		episodeNumber, episodeNumber, episodeNumber, episodeNumber, episodeNumber, userID, animeID, episodeNumber)
	return result.RowsAffected > 0, result.Error
}

// StartRewatch resets a COMPLETED entry to a fresh rewatch cycle: status back
// to 'watching', episodes=0, is_rewatching=true. rewatch_count is left alone —
// it bumps when the rewatch reaches the finale (see IncrementEpisodes). Returns
// true when a completed entry was actually reset. Design 2026-06-05.
func (r *ListRepository) StartRewatch(ctx context.Context, userID, animeID string) (bool, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE anime_list SET
			status = 'watching',
			episodes = 0,
			is_rewatching = true,
			updated_at = NOW()
		WHERE user_id = ? AND anime_id = ? AND status = 'completed'`,
		userID, animeID)
	return result.RowsAffected > 0, result.Error
}

// CountWatchers returns the number of anime_list rows where the given anime is
// being actively watched (status='watching'). Used by Phase 14 / UX-28 to
// render a soft social-proof badge on the anime detail view. Soft-deleted
// rows are excluded automatically by GORM's gorm.DeletedAt handling on the
// model.
func (r *ListRepository) CountWatchers(ctx context.Context, animeID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.AnimeListEntry{}).
		Where("anime_id = ? AND status = ?", animeID, "watching").
		Count(&count).Error
	return count, err
}

func (r *ListRepository) GetByUserAndStatuses(ctx context.Context, userID string, statuses []string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("anime_list.user_id = ? AND anime_list.status IN ?", userID, statuses).
		Order("anime_list.updated_at DESC").
		Find(&entries).Error
	return entries, err
}

func (r *ListRepository) GetByUserPaginated(ctx context.Context, userID, status, search string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate() // defense in depth

	var entries []*domain.AnimeListEntry
	var total int64

	base := r.db.WithContext(ctx).Where("anime_list.user_id = ?", userID)
	if status != "" {
		base = base.Where("anime_list.status = ?", status)
	}

	needsAnimesJoin := isTitleSort(params.Sort) || search != ""
	if needsAnimesJoin {
		base = base.Joins("LEFT JOIN animes ON animes.id = anime_list.anime_id")
	}
	if isGenreSort(params.Sort) {
		base = base.Joins(genreSortJoin)
	}
	if search != "" {
		like := "%" + search + "%"
		base = base.Where(
			"animes.name ILIKE ? OR animes.name_ru ILIKE ? OR animes.name_jp ILIKE ?",
			like, like, like,
		)
	}

	if err := base.Session(&gorm.Session{}).Model(&domain.AnimeListEntry{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base.Session(&gorm.Session{}).
		Preload("Anime").Preload("Anime.Genres").
		Order(sanitizedOrderClause(params.Sort, params.Order)).
		Offset(params.Offset()).
		Limit(params.PerPage).
		Find(&entries).Error

	return entries, total, err
}

func (r *ListRepository) GetByUserStatuses(ctx context.Context, userID string) ([]domain.AnimeStatusEntry, error) {
	var entries []domain.AnimeStatusEntry
	err := r.db.WithContext(ctx).
		Model(&domain.AnimeListEntry{}).
		Select("anime_id, status, score, episodes").
		Where("user_id = ?", userID).
		Scan(&entries).Error
	return entries, err
}

// GetByUserAndStatusesWithProgress returns one InternalListItem per
// (user_id, anime_id) where anime_list.status ∈ statuses, joined with the
// animes projection (name / name_ru / poster_url / episodes_aired /
// episodes_count) and a LEFT JOIN against watch_progress so the user's
// furthest COMPLETED episode_number for that anime rides along on the same
// row. Missing watch_progress yields last_watched_episode=0 via COALESCE.
//
// 2026-06-11: counts only completed=true rows — the same semantics the
// anime page's resume state machine uses (/users/progress/{animeId} →
// max completed). Counting any-touched rows made spotlight and the anime
// page disagree about "continue from ep N" whenever a user sampled an
// episode without finishing it. Duration-aware auto-complete (90% rule)
// keeps `completed` trustworthy for short episodes.
//
// Used by the workstream hero-spotlight v1.0 Phase 3 catalog aggregator
// (`not_time_yet`, `continue_watching_new` resolvers) via the
// /internal/users/{user_id}/list endpoint. LIMIT 200 defensively bounds
// the join even if the user has a pathologically large list.
//
// ORDER BY anime_list.updated_at DESC so the caller gets recency for free
// (resolvers that pick "most recently aired" benefit from the locality).
func (r *ListRepository) GetByUserAndStatusesWithProgress(
	ctx context.Context,
	userID string,
	statuses []string,
) ([]domain.InternalListItem, error) {
	const q = `
SELECT
  al.anime_id                                                 AS anime_id,
  a.name                                                      AS name,
  a.name_ru                                                   AS name_ru,
  a.poster_url                                                AS poster_url,
  COALESCE(a.episodes_aired, 0)                               AS episodes_aired,
  COALESCE(a.episodes_count, 0)                               AS episodes_count,
  al.status                                                   AS status,
  COALESCE(MAX(wp.episode_number) FILTER (WHERE wp.completed), 0) AS last_watched_episode,
  to_char(al.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')        AS updated_at
FROM anime_list al
JOIN animes a ON a.id = al.anime_id
LEFT JOIN watch_progress wp
  ON wp.user_id = al.user_id AND wp.anime_id = al.anime_id
WHERE al.user_id = ? AND al.status IN ?
GROUP BY al.anime_id, a.name, a.name_ru, a.poster_url, a.episodes_aired, a.episodes_count, al.status, al.updated_at
ORDER BY al.updated_at DESC
LIMIT 200
`
	var out []domain.InternalListItem
	if err := r.db.WithContext(ctx).Raw(q, userID, statuses).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ListRepository) GetUserWatchlistStats(ctx context.Context, userID string, statuses []string) (*domain.WatchlistStats, error) {
	var stats domain.WatchlistStats

	base := r.db.WithContext(ctx).Model(&domain.AnimeListEntry{}).Where("anime_list.user_id = ?", userID)
	if len(statuses) > 0 {
		base = base.Where("anime_list.status IN ?", statuses)
	}

	// Lifetime episodes counts rewatches: a completed entry's `episodes` equals
	// its total, so episodes * (1 + rewatch_count) credits each completed
	// rewatch once more. Design 2026-06-05.
	err := base.Select(
		"COALESCE(AVG(NULLIF(anime_list.score, 0)), 0) as avg_score, "+
			"COALESCE(SUM(anime_list.episodes * (1 + anime_list.rewatch_count)), 0) as total_episodes, "+
			"COUNT(*) as total_entries, "+
			"COUNT(CASE WHEN anime_list.status = 'completed' THEN 1 END) as completed",
	).Scan(&stats).Error

	return &stats, err
}

// --- Phase 1 (workstream: social) plan 02 — review-shaped queries -------
// These methods power the six review endpoints by operating on the same
// anime_list table that powers the watchlist, filtered by
// `(score > 0 OR review_text != '')` so MAL-imported score=8 rows and
// written-review rows both qualify as "reviews".

// GetReviewsByAnime returns every anime_list row for `animeID` that has
// either a non-zero score OR a non-empty review_text. Preloads Anime so the
// handler can include the existing JSON `anime` field unchanged.
func (r *ListRepository) GetReviewsByAnime(ctx context.Context, animeID string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").
		Where("anime_id = ? AND (score > 0 OR review_text <> '')", animeID).
		Order("created_at DESC").
		Find(&entries).Error
	if err != nil {
		return nil, err
	}
	r.attachUserAvatars(ctx, entries)
	return entries, nil
}

// GetReviewsByUser returns every anime_list row for `userID` that qualifies
// as a review (score>0 OR review_text!=''). Newest-first, preloads Anime.
func (r *ListRepository) GetReviewsByUser(ctx context.Context, userID string) ([]*domain.AnimeListEntry, error) {
	var entries []*domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").
		Where("user_id = ? AND (score > 0 OR review_text <> '')", userID).
		Order("created_at DESC").
		Find(&entries).Error
	if err != nil {
		return nil, err
	}
	r.attachUserAvatars(ctx, entries)
	return entries, nil
}

// attachUserAvatars populates the transient UserAvatar on each review row
// from the users table (current avatar, not snapshotted — same pattern as
// the activity feed). Best-effort: on lookup failure rows keep "" and the
// frontend falls back to username initials.
func (r *ListRepository) attachUserAvatars(ctx context.Context, entries []*domain.AnimeListEntry) {
	if len(entries) == 0 {
		return
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.UserID)
	}
	avatars := fetchUserAvatars(ctx, r.db, ids)
	for _, e := range entries {
		e.UserAvatar = avatars[e.UserID]
	}
}

// GetUserReview returns the single anime_list row for the given (user, anime)
// pair if it carries a review (score>0 OR review_text!=''). Returns
// errors.NotFound when the row is absent OR exists with empty review.
func (r *ListRepository) GetUserReview(ctx context.Context, userID, animeID string) (*domain.AnimeListEntry, error) {
	var entry domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").
		Where("user_id = ? AND anime_id = ? AND (score > 0 OR review_text <> '')", userID, animeID).
		First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("review")
	}
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// ApplyEffectiveEpisodes mutates each entry's Episodes in place to
// max(anime_list.episodes, distinct completed episodes in watch_progress) for
// that (user, anime). This fixes the false ⚠️ "0 episodes" review flag for
// passive watchers who never updated their list (repo-todo 19:00:02). One
// batched query over all distinct (user_id, anime_id) pairs — no N+1.
func (r *ListRepository) ApplyEffectiveEpisodes(ctx context.Context, entries []*domain.AnimeListEntry) error {
	if len(entries) == 0 {
		return nil
	}

	type pair struct{ u, a string }
	seen := make(map[pair]bool, len(entries))
	conds := make([]string, 0, len(entries))
	args := make([]interface{}, 0, len(entries)*2)
	for _, e := range entries {
		p := pair{e.UserID, e.AnimeID}
		if seen[p] {
			continue
		}
		seen[p] = true
		// OR-of-pairs rather than a row-value tuple IN so the query is portable
		// across Postgres (prod) and SQLite (tests).
		conds = append(conds, "(user_id = ? AND anime_id = ?)")
		args = append(args, e.UserID, e.AnimeID)
	}

	sql := "SELECT user_id, anime_id, COUNT(DISTINCT episode_number) AS cnt " +
		"FROM watch_progress WHERE completed = true AND (" +
		strings.Join(conds, " OR ") + ") GROUP BY user_id, anime_id"

	var rows []struct {
		UserID  string
		AnimeID string
		Cnt     int
	}
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return err
	}

	counts := make(map[pair]int, len(rows))
	for _, row := range rows {
		counts[pair{row.UserID, row.AnimeID}] = row.Cnt
	}
	for _, e := range entries {
		if c := counts[pair{e.UserID, e.AnimeID}]; c > e.Episodes {
			e.Episodes = c
		}
	}
	return nil
}

// UpsertReview writes the (score, review_text, username) triple onto an
// existing anime_list row OR creates a fresh row with status='completed' when
// none exists. On the update path it ONLY assigns score, review_text,
// username, updated_at — status / episodes / notes / tags / etc. on the
// pre-existing watchlist row are preserved. Returns the resulting entry.
func (r *ListRepository) UpsertReview(ctx context.Context, userID, animeID, username string, score int, reviewText string) (*domain.AnimeListEntry, error) {
	now := time.Now()
	entry := &domain.AnimeListEntry{
		UserID:     userID,
		AnimeID:    animeID,
		Status:     "completed", // only takes effect on INSERT path
		Score:      score,
		ReviewText: reviewText,
		Username:   username,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"score":       score,
			"review_text": reviewText,
			"username":    username,
			"updated_at":  now,
		}),
	}).Create(entry).Error
	if err != nil {
		return nil, err
	}
	// Reload to capture any preserved fields (status, episodes, etc.).
	// REVIEW.md CR-03: propagate the reload error rather than returning
	// the locally-constructed `entry` — that entry has no ID (Postgres
	// gen_random_uuid() default fires server-side), no canonical
	// CreatedAt, and no preserved pre-existing fields on the conflict
	// path. Returning it would let clients PATCH/DELETE with an empty ID.
	var fresh domain.AnimeListEntry
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		First(&fresh).Error; err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "failed to reload review after upsert")
	}
	return &fresh, nil
}

// ClearReview sets score=0 + review_text='' on the matching row (the row
// stays in anime_list — it just drops out of the reviews filter). Idempotent
// — no error when no row matches.
func (r *ListRepository) ClearReview(ctx context.Context, userID, animeID string) error {
	return r.db.WithContext(ctx).
		Model(&domain.AnimeListEntry{}).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		Updates(map[string]interface{}{
			"score":       0,
			"review_text": "",
			"updated_at":  time.Now(),
		}).Error
}

// GetAnimeRating returns the average score and total scoring-row count for an
// anime, only considering anime_list rows where score>0.
func (r *ListRepository) GetAnimeRating(ctx context.Context, animeID string) (*domain.AnimeRating, error) {
	var result struct {
		AverageScore float64 `gorm:"column:average_score"`
		TotalReviews int64   `gorm:"column:total_reviews"`
	}
	err := r.db.WithContext(ctx).
		Raw(`SELECT COALESCE(AVG(score), 0) AS average_score, COUNT(*) AS total_reviews
		     FROM anime_list WHERE anime_id = ? AND score > 0`, animeID).
		Scan(&result).Error
	if err != nil {
		// Propagate the error so callers can distinguish "no ratings yet"
		// (legitimate AverageScore=0, TotalReviews=0) from "DB hiccup
		// during lookup" — REVIEW.md CR-02. Previously the error was
		// swallowed and the handler served a fake zero rating that was
		// indistinguishable from a real one.
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "failed to get anime rating")
	}
	return &domain.AnimeRating{
		AnimeID:      animeID,
		AverageScore: result.AverageScore,
		TotalReviews: int(result.TotalReviews),
	}, nil
}

// GetBatchAnimeRatings returns a map keyed by anime_id, only for anime that
// have at least one anime_list row with score>0.
func (r *ListRepository) GetBatchAnimeRatings(ctx context.Context, animeIDs []string) (map[string]*domain.AnimeRating, error) {
	var rows []struct {
		AnimeID      string  `gorm:"column:anime_id"`
		AverageScore float64 `gorm:"column:average_score"`
		TotalReviews int64   `gorm:"column:total_reviews"`
	}
	err := r.db.WithContext(ctx).
		Raw(`SELECT anime_id,
		            COALESCE(AVG(score), 0) AS average_score,
		            COUNT(*) AS total_reviews
		     FROM anime_list
		     WHERE anime_id IN ? AND score > 0
		     GROUP BY anime_id`, animeIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]*domain.AnimeRating, len(rows))
	for _, row := range rows {
		out[row.AnimeID] = &domain.AnimeRating{
			AnimeID:      row.AnimeID,
			AverageScore: row.AverageScore,
			TotalReviews: int(row.TotalReviews),
		}
	}
	return out, nil
}

// ToggleReaction toggles a user's emoji reaction on a review.
//
// multi=false (regular users) enforces ONE reaction per (review, user):
// clicking the SAME emoji you already have removes it (added=false); clicking
// a DIFFERENT emoji replaces whatever you had (added=true); with none set it
// inserts (added=true). It clears ALL of the user's existing rows on the
// review first, so a legacy multi-reaction collapses to the single new choice
// on first interaction.
//
// multi=true (admins) toggles each emoji INDEPENDENTLY: clicking an emoji you
// already have removes just that one; clicking a new emoji adds it alongside
// your existing reactions. (The DB's (review, user, emoji) unique index
// already permits this.)
//
// username is denormalized for the who-reacted popover. Runs in a transaction
// so the replace is atomic. AUTO-408.
func (r *ListRepository) ToggleReaction(ctx context.Context, reviewID, userID, username, emoji string, multi bool) (added bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []domain.ReviewReaction
		if e := tx.Where("review_id = ? AND user_id = ?", reviewID, userID).Find(&existing).Error; e != nil {
			return e
		}
		if multi {
			// Per-emoji independent toggle: remove if present, add otherwise.
			for _, ex := range existing {
				if ex.Emoji == emoji {
					added = false
					return tx.Where("review_id = ? AND user_id = ? AND emoji = ?", reviewID, userID, emoji).
						Delete(&domain.ReviewReaction{}).Error
				}
			}
			added = true
			return tx.Create(&domain.ReviewReaction{
				ReviewID: reviewID, UserID: userID, Emoji: emoji, Username: username,
			}).Error
		}
		// Exactly the same single reaction → toggle it off.
		if len(existing) == 1 && existing[0].Emoji == emoji {
			added = false
			return tx.Where("review_id = ? AND user_id = ?", reviewID, userID).
				Delete(&domain.ReviewReaction{}).Error
		}
		// Otherwise replace: drop everything this user had, set the new one.
		if len(existing) > 0 {
			if e := tx.Where("review_id = ? AND user_id = ?", reviewID, userID).
				Delete(&domain.ReviewReaction{}).Error; e != nil {
				return e
			}
		}
		added = true
		return tx.Create(&domain.ReviewReaction{
			ReviewID: reviewID, UserID: userID, Emoji: emoji, Username: username,
		}).Error
	})
	return added, err
}

// DeleteUserReaction removes one user's reaction rows on a review — the whole
// user (all emojis) when emoji is "", or just the given emoji otherwise.
// Returns the number of rows removed (0 when nothing matched). Admin
// moderation path. AUTO-408.
func (r *ListRepository) DeleteUserReaction(ctx context.Context, reviewID, targetUserID, emoji string) (int64, error) {
	q := r.db.WithContext(ctx).Where("review_id = ? AND user_id = ?", reviewID, targetUserID)
	if emoji != "" {
		q = q.Where("emoji = ?", emoji)
	}
	res := q.Delete(&domain.ReviewReaction{})
	return res.RowsAffected, res.Error
}

// SeedSystemReaction idempotently adds the System «AnimeEnigma» 👍 to a review
// (used for admin-authored reviews). No-op if it already exists. AUTO-408.
func (r *ListRepository) SeedSystemReaction(ctx context.Context, reviewID string) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&domain.ReviewReaction{
			ReviewID: reviewID,
			UserID:   domain.SystemReactionUserID,
			Emoji:    domain.SystemReactionEmoji,
			Username: domain.SystemReactionUsername,
		}).Error
}

// GetReviewAuthorID returns the user_id that authored a review (anime_list row),
// or "" if the review doesn't exist. Used to block self-reactions. AUTO-408.
func (r *ListRepository) GetReviewAuthorID(ctx context.Context, reviewID string) (string, error) {
	var ids []string
	if err := r.db.WithContext(ctx).Model(&domain.AnimeListEntry{}).
		Where("id = ?", reviewID).Limit(1).Pluck("user_id", &ids).Error; err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", nil
	}
	return ids[0], nil
}

// GetReactionCounts returns per-review aggregated reaction counts for the given
// review IDs, keyed by review_id. Each ReactionCount carries the ordered list
// of reactor display names (Users) for the who-reacted popover. When
// viewerUserID is non-nil, ReactedByMe is set per emoji for that viewer.
// Reviews with no reactions are absent from the map. AUTO-408.
func (r *ListRepository) GetReactionCounts(ctx context.Context, reviewIDs []string, viewerUserID *string) (map[string][]domain.ReactionCount, error) {
	out := make(map[string][]domain.ReactionCount)
	if len(reviewIDs) == 0 {
		return out, nil
	}

	// Fetch the raw rows and aggregate in Go — portable across Postgres/SQLite
	// (no string_agg) and lets us build counts + reactor names + reacted_by_me
	// from a single query. Reactions per review are few, so this is cheap.
	var rows []domain.ReviewReaction
	if err := r.db.WithContext(ctx).
		Where("review_id IN ?", reviewIDs).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	type agg struct {
		count       int
		users       []string
		reactors    []domain.ReactionUser
		reactedByMe bool
	}
	perReview := make(map[string]map[string]*agg) // reviewID -> emoji -> agg
	emojiOrder := make(map[string][]string)        // reviewID -> emoji first-seen order
	for _, rr := range rows {
		byEmoji := perReview[rr.ReviewID]
		if byEmoji == nil {
			byEmoji = make(map[string]*agg)
			perReview[rr.ReviewID] = byEmoji
		}
		a := byEmoji[rr.Emoji]
		if a == nil {
			a = &agg{}
			byEmoji[rr.Emoji] = a
			emojiOrder[rr.ReviewID] = append(emojiOrder[rr.ReviewID], rr.Emoji)
		}
		a.count++
		if rr.Username != "" {
			a.users = append(a.users, rr.Username)
		}
		a.reactors = append(a.reactors, domain.ReactionUser{UserID: rr.UserID, Username: rr.Username})
		if viewerUserID != nil && rr.UserID == *viewerUserID {
			a.reactedByMe = true
		}
	}

	for reviewID, byEmoji := range perReview {
		list := make([]domain.ReactionCount, 0, len(byEmoji))
		for _, emoji := range emojiOrder[reviewID] {
			a := byEmoji[emoji]
			list = append(list, domain.ReactionCount{
				Emoji: emoji, Count: a.count, ReactedByMe: a.reactedByMe, Users: a.users, Reactors: a.reactors,
			})
		}
		// Stable display order: most-reacted first, then emoji for ties.
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Count != list[j].Count {
				return list[i].Count > list[j].Count
			}
			return list[i].Emoji < list[j].Emoji
		})
		out[reviewID] = list
	}
	return out, nil
}

func (r *ListRepository) GetByUserAndStatusesPaginated(ctx context.Context, userID string, statuses []string, search string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate() // defense in depth

	var entries []*domain.AnimeListEntry
	var total int64

	base := r.db.WithContext(ctx).Where("anime_list.user_id = ? AND anime_list.status IN ?", userID, statuses)

	needsAnimesJoin := isTitleSort(params.Sort) || search != ""
	if needsAnimesJoin {
		base = base.Joins("LEFT JOIN animes ON animes.id = anime_list.anime_id")
	}
	if isGenreSort(params.Sort) {
		base = base.Joins(genreSortJoin)
	}
	if search != "" {
		like := "%" + search + "%"
		base = base.Where(
			"animes.name ILIKE ? OR animes.name_ru ILIKE ? OR animes.name_jp ILIKE ?",
			like, like, like,
		)
	}

	if err := base.Session(&gorm.Session{}).Model(&domain.AnimeListEntry{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base.Session(&gorm.Session{}).
		Preload("Anime").Preload("Anime.Genres").
		Order(sanitizedOrderClause(params.Sort, params.Order)).
		Offset(params.Offset()).
		Limit(params.PerPage).
		Find(&entries).Error

	return entries, total, err
}
