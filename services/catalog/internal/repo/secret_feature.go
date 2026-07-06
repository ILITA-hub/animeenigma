package repo

import (
	"context"
	"fmt"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SecretFeatureRepository persists admin on/off overrides for the secret-feature
// roulette. Rows are sparse — absence means "enabled (default)".
type SecretFeatureRepository struct {
	db *gorm.DB
}

func NewSecretFeatureRepository(db *gorm.DB) *SecretFeatureRepository {
	return &SecretFeatureRepository{db: db}
}

// GetAll returns every explicit override as a key→enabled map (empty when no
// admin has toggled anything yet).
func (r *SecretFeatureRepository) GetAll(ctx context.Context) (map[string]bool, error) {
	var flags []domain.SecretFeatureFlag
	if err := r.db.WithContext(ctx).Find(&flags).Error; err != nil {
		return nil, fmt.Errorf("list secret feature flags: %w", err)
	}
	out := make(map[string]bool, len(flags))
	for _, f := range flags {
		out[f.Key] = f.Enabled
	}
	return out, nil
}

// Set upserts a single flag (master switch or a feature key).
func (r *SecretFeatureRepository) Set(ctx context.Context, key string, enabled bool) error {
	flag := domain.SecretFeatureFlag{Key: key, Enabled: enabled}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
		}).
		Create(&flag).Error
	if err != nil {
		return fmt.Errorf("set secret feature flag %q: %w", key, err)
	}
	return nil
}
