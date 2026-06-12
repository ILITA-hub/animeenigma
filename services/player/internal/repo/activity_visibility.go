package repo

import (
	"context"

	"gorm.io/gorm"
)

// Activity-visibility values mirror the auth service's users.activity_visibility
// column (auth owns the setting; the player service enforces it on public read
// paths — design doc: docs/superpowers/specs/2026-06-12-activity-visibility-design.md).
const (
	ActivityVisibilityAll       = "all"
	ActivityVisibilityNonHentai = "non_hentai"
	ActivityVisibilityNone      = "none"
)

// hentaiAnimeExists is the shared 18+ predicate: the anime is rated 'rx'
// (Shikimori hentai rating) or carries the Hentai genre (the frontend's
// isHentai check). Parameterized by the column holding the anime id so it
// works against both activity_events and anime_list. Plain SQL — runs on
// Postgres (prod) and SQLite (repo tests).
const hentaiAnimeExistsFmt = `EXISTS (
	SELECT 1 FROM animes a
	LEFT JOIN anime_genres ag ON ag.anime_id = a.id
	LEFT JOIN genres g ON g.id = ag.genre_id
	WHERE a.id = %s AND (a.rating = 'rx' OR g.name = 'Hentai')
)`

// fetchActivityVisibility returns the user's activity_visibility setting.
// Best-effort: any error (user missing, column not yet migrated) degrades to
// "all" — the pre-feature behaviour — so a read hiccup can never hide or
// expose more than intended in the steady state.
func fetchActivityVisibility(ctx context.Context, db *gorm.DB, userID string) string {
	var visibility string
	err := db.WithContext(ctx).
		Table("users").
		Select("COALESCE(activity_visibility, 'all')").
		Where("id = ?", userID).
		Scan(&visibility).Error
	if err != nil || visibility == "" {
		return ActivityVisibilityAll
	}
	return visibility
}

// GetUserActivityVisibility exposes the user's activity_visibility to the
// service layer (public watchlist + stats enforcement).
func (r *ListRepository) GetUserActivityVisibility(ctx context.Context, userID string) string {
	return fetchActivityVisibility(ctx, r.db, userID)
}
