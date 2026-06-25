package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ModelRepository provides CRUD access to the upscale_models table.
type ModelRepository struct {
	db *gorm.DB
}

// NewModelRepository constructs a ModelRepository backed by db.
func NewModelRepository(db *gorm.DB) *ModelRepository {
	return &ModelRepository{db: db}
}

// Upsert inserts or updates a model record. On conflict on (name, version),
// checksum, object_path, and builtin are refreshed.
func (r *ModelRepository) Upsert(ctx context.Context, m *domain.UpscaleModel) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}, {Name: "version"}},
		DoUpdates: clause.AssignmentColumns([]string{"checksum", "object_path", "builtin"}),
	}).Create(m).Error
}

// Get returns the model identified by (name, version), or gorm.ErrRecordNotFound
// when absent.
func (r *ModelRepository) Get(ctx context.Context, name, version string) (*domain.UpscaleModel, error) {
	var m domain.UpscaleModel
	if err := r.db.WithContext(ctx).
		Where("name = ? AND version = ?", name, version).
		First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// List returns all registered models ordered by name, version.
func (r *ModelRepository) List(ctx context.Context) ([]domain.UpscaleModel, error) {
	var models []domain.UpscaleModel
	if err := r.db.WithContext(ctx).
		Order("name ASC, version ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

// GetLatest returns the most recently created model row for the given name, or
// gorm.ErrRecordNotFound when no rows exist for that name. When multiple
// versions are registered the row with the latest created_at is returned; if
// created_at is identical, the lexicographically greatest version string wins.
func (r *ModelRepository) GetLatest(ctx context.Context, name string) (*domain.UpscaleModel, error) {
	var m domain.UpscaleModel
	if err := r.db.WithContext(ctx).
		Where("name = ?", name).
		Order("created_at DESC, version DESC").
		First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}
