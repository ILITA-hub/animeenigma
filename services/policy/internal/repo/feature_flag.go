// Package repo persists policy-service feature flags.
package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FeatureFlagRepository struct{ db *gorm.DB }

func NewFeatureFlagRepository(db *gorm.DB) *FeatureFlagRepository {
	return &FeatureFlagRepository{db: db}
}

// GetAll returns every flag row (including the reserved __roulette__ master).
func (r *FeatureFlagRepository) GetAll(ctx context.Context) ([]domain.FeatureFlag, error) {
	var flags []domain.FeatureFlag
	if err := r.db.WithContext(ctx).Find(&flags).Error; err != nil {
		return nil, err
	}
	return flags, nil
}

// Upsert writes a flag by key (create or full replace of all columns).
func (r *FeatureFlagRepository) Upsert(ctx context.Context, f domain.FeatureFlag) error {
	f.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		UpdateAll: true,
	}).Create(&f).Error
}

// SeedIfAbsent inserts a default only when the key has no row (idempotent boot seed).
func (r *FeatureFlagRepository) SeedIfAbsent(ctx context.Context, f domain.FeatureFlag) error {
	f.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&f).Error
}
