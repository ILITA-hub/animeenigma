package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PreferenceRepository struct {
	db *gorm.DB
}

func NewPreferenceRepository(db *gorm.DB) *PreferenceRepository {
	return &PreferenceRepository{db: db}
}

// UpsertAnimePreference creates or updates the user's per-anime preference,
// then bumps prefs_version so the frontend cache invalidates. Best-effort:
// upsert errors fail the call, but a failed prefs_version bump is logged
// (caller's responsibility) and not surfaced — the version will catch up on
// the next save.
func (r *PreferenceRepository) UpsertAnimePreference(ctx context.Context, pref *domain.UserAnimePreference) error {
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"player", "language", "watch_type", "translation_id", "translation_title", "updated_at"}),
		}).
		Create(pref).Error; err != nil {
		return err
	}
	// Best-effort bump — preference write must not fail because the version
	// counter could not increment. The next successful save will catch up.
	_, _ = r.BumpPrefsVersion(ctx, pref.UserID)
	return nil
}

// GetAnimePreference returns the user's saved preference for a specific anime
func (r *PreferenceRepository) GetAnimePreference(ctx context.Context, userID, animeID string) (*domain.UserAnimePreference, error) {
	var pref domain.UserAnimePreference
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		First(&pref).Error
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

// GetUserTopCombos returns the user's top combos ranked by watch count
func (r *PreferenceRepository) GetUserTopCombos(ctx context.Context, userID string, limit int) ([]domain.ComboCount, error) {
	var results []domain.ComboCount
	err := r.db.WithContext(ctx).
		Model(&domain.WatchHistory{}).
		Select("player, language, watch_type, translation_title, COUNT(*) as count").
		Where("user_id = ?", userID).
		Group("player, language, watch_type, translation_title").
		Order("count DESC").
		Limit(limit).
		Scan(&results).Error
	return results, err
}

// communityPopularityLimit bounds the combos returned — the resolver only
// consults the most popular handful, so an unbounded result set was wasteful
// (audit #14).
const communityPopularityLimit = 50

// GetCommunityPopularity returns the most popular combos for a specific anime
func (r *PreferenceRepository) GetCommunityPopularity(ctx context.Context, animeID string) ([]domain.CommunityCombo, error) {
	var results []domain.CommunityCombo
	err := r.db.WithContext(ctx).
		Model(&domain.WatchHistory{}).
		Select("player, language, watch_type, translation_id, translation_title, COUNT(DISTINCT user_id) as viewers").
		Where("anime_id = ?", animeID).
		Group("player, language, watch_type, translation_id, translation_title").
		Order("viewers DESC").
		Limit(communityPopularityLimit).
		Scan(&results).Error
	return results, err
}

// GetPinnedTranslations queries catalog's pinned_translations table (shared DB)
func (r *PreferenceRepository) GetPinnedTranslations(ctx context.Context, animeID string) ([]domain.PinnedTranslation, error) {
	var results []domain.PinnedTranslation
	err := r.db.WithContext(ctx).
		Where("anime_id = ?", animeID).
		Find(&results).Error
	return results, err
}

// CreateWatchHistory inserts a watch_history row with full combo context
func (r *PreferenceRepository) CreateWatchHistory(ctx context.Context, history *domain.WatchHistory) error {
	return r.db.WithContext(ctx).Create(history).Error
}

// GetUserHistoryForTier2 returns the user's recent watch_history rows ordered
// by watched_at DESC so the service-layer aggregation (preference.AggregateTier2)
// can compute the duration-weighted, exponentially-decayed coarse + fine signals.
// maxRows is a safety cap to bound resolver latency for users with very long
// history. Phase 6 (Tier 2 inference rewrite).
func (r *PreferenceRepository) GetUserHistoryForTier2(ctx context.Context, userID string, maxRows int) ([]domain.WatchHistory, error) {
	var rows []domain.WatchHistory
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("watched_at DESC").
		Limit(maxRows).
		Find(&rows).Error
	return rows, err
}

// BumpPrefsVersion atomically increments the user's prefs_version generation
// counter, creating the row at version 1 on first call. Returns the new
// version. Phase 7 D-03 — the frontend uses this to invalidate its 24h
// composable cache cross-device.
func (r *PreferenceRepository) BumpPrefsVersion(ctx context.Context, userID string) (int64, error) {
	// Postgres-native upsert: insert a row at version 1, or atomically
	// increment the existing row's version. SQLite tests register an
	// "increment_prefs_version" path that follows the same shape via UDF.
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO user_prefs_version (user_id, version, updated_at)
		VALUES (?, 1, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			version = user_prefs_version.version + 1,
			updated_at = NOW()
	`, userID).Error
	if err != nil {
		return 0, err
	}

	var row domain.UserPrefsVersion
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&row).Error; err != nil {
		return 0, err
	}
	return row.Version, nil
}

// GetPrefsVersion returns the user's current prefs_version generation, or 0
// if the row hasn't been created yet (i.e., the user has never saved a
// preference). Phase 7 D-03 — read on every preference response so the
// frontend's X-Prefs-Version-aware cache stays consistent.
func (r *PreferenceRepository) GetPrefsVersion(ctx context.Context, userID string) (int64, error) {
	var row domain.UserPrefsVersion
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&row).Error
	if err != nil {
		// Row not found means user has never saved a preference. The frontend
		// treats version=0 as "no learned preferences yet" and skips
		// invalidation. Don't surface the error.
		return 0, nil
	}
	return row.Version, nil
}

// ResetLearnedPreferences deletes all per-anime preference rows for a user.
// Watch history is NOT touched — community popularity (Tier 3) is a public
// good and Tier 2 weights would resurface from history alone. Phase 7 B-05.
// Bumps prefs_version so the frontend cache busts immediately.
func (r *PreferenceRepository) ResetLearnedPreferences(ctx context.Context, userID string) (int64, error) {
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&domain.UserAnimePreference{}).Error; err != nil {
		return 0, err
	}
	return r.BumpPrefsVersion(ctx, userID)
}
