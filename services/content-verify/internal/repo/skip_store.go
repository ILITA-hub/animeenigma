package repo

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

// SkipByAnime returns all skip-timing rows for an anime, ordered
// provider, team, episode.
func (s *Store) SkipByAnime(ctx context.Context, animeID string) ([]domain.SkipTiming, error) {
	var rows []domain.SkipTiming
	err := s.db.WithContext(ctx).Where("anime_id = ?", animeID).
		Order("provider").Order("team").Order("episode").Find(&rows).Error
	return rows, err
}

// UpsertSkip inserts or updates the skip-timing row for (anime, provider,
// team, episode), preserving the row's ID across updates.
func (s *Store) UpsertSkip(ctx context.Context, t domain.SkipTiming) error {
	var existing domain.SkipTiming
	err := s.db.WithContext(ctx).
		Where("anime_id = ? AND provider = ? AND team = ? AND episode = ?", t.AnimeID, t.Provider, t.Team, t.Episode).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.WithContext(ctx).Create(&t).Error
	}
	if err != nil {
		return err
	}
	t.ID = existing.ID
	return s.db.WithContext(ctx).Save(&t).Error
}

// Fingerprints returns both kinds of season fingerprint for an anime,
// oldest first.
func (s *Store) Fingerprints(ctx context.Context, animeID string) ([]domain.SkipFingerprint, error) {
	var rows []domain.SkipFingerprint
	err := s.db.WithContext(ctx).Where("anime_id = ?", animeID).Order("created_at").Find(&rows).Error
	return rows, err
}

// AddFingerprint appends a new season fingerprint row.
func (s *Store) AddFingerprint(ctx context.Context, fp domain.SkipFingerprint) error {
	return s.db.WithContext(ctx).Create(&fp).Error
}
