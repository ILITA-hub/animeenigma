// Package repo persists content verifications. Single-writer by design (one
// worker goroutine), so read-modify-write on the Units JSON needs no locking.
package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) Get(ctx context.Context, animeID, provider string) (*domain.ContentVerification, error) {
	var row domain.ContentVerification
	err := s.db.WithContext(ctx).
		Where("anime_id = ? AND provider = ?", animeID, provider).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) ByAnime(ctx context.Context, animeID string) ([]domain.ContentVerification, error) {
	var rows []domain.ContentVerification
	err := s.db.WithContext(ctx).Where("anime_id = ?", animeID).Order("provider").Find(&rows).Error
	return rows, err
}

// UpsertUnit merges one unit verdict into the (anime, provider) row,
// replacing an existing unit with the same canonical key.
func (s *Store) UpsertUnit(ctx context.Context, animeID, provider string, v domain.UnitVerdict) error {
	row, err := s.Get(ctx, animeID, provider)
	if err != nil {
		return err
	}
	if row == nil {
		row = &domain.ContentVerification{AnimeID: animeID, Provider: provider}
	}
	key := v.Key.String()
	replaced := false
	for i := range row.Units {
		if row.Units[i].Key.String() == key {
			row.Units[i] = v
			replaced = true
			break
		}
	}
	if !replaced {
		row.Units = append(row.Units, v)
	}
	row.UpdatedAt = time.Now().UTC()
	return s.db.WithContext(ctx).Save(row).Error
}
