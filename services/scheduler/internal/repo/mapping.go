package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MappingRepository handles database operations for MAL-Shikimori mappings
type MappingRepository struct {
	db *gorm.DB
}

// NewMappingRepository creates a new mapping repository
func NewMappingRepository(db *gorm.DB) *MappingRepository {
	return &MappingRepository{db: db}
}

// Create creates a new mapping
func (r *MappingRepository) Create(ctx context.Context, mapping *domain.MALShikimoriMapping) error {
	mapping.CreatedAt = time.Now()
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "mal_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"shikimori_id", "anime_id", "confidence", "source"}),
		}).
		Create(mapping).Error
}

// GetByMALID retrieves a mapping by MAL ID
func (r *MappingRepository) GetByMALID(ctx context.Context, malID int) (*domain.MALShikimoriMapping, error) {
	var mapping domain.MALShikimoriMapping
	if err := r.db.WithContext(ctx).Where("mal_id = ?", malID).First(&mapping).Error; err != nil {
		return nil, err
	}
	return &mapping, nil
}

// GetByShikimoriID retrieves a mapping by Shikimori ID
func (r *MappingRepository) GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.MALShikimoriMapping, error) {
	var mapping domain.MALShikimoriMapping
	if err := r.db.WithContext(ctx).Where("shikimori_id = ?", shikimoriID).First(&mapping).Error; err != nil {
		return nil, err
	}
	return &mapping, nil
}

// GetByMALIDs retrieves mappings for multiple MAL IDs
func (r *MappingRepository) GetByMALIDs(ctx context.Context, malIDs []int) ([]*domain.MALShikimoriMapping, error) {
	var mappings []*domain.MALShikimoriMapping
	if err := r.db.WithContext(ctx).Where("mal_id IN ?", malIDs).Find(&mappings).Error; err != nil {
		return nil, err
	}
	return mappings, nil
}

// Update updates a mapping
func (r *MappingRepository) Update(ctx context.Context, mapping *domain.MALShikimoriMapping) error {
	return r.db.WithContext(ctx).Save(mapping).Error
}

// UpdateAnimeID updates the anime_id for a mapping
func (r *MappingRepository) UpdateAnimeID(ctx context.Context, malID int, animeID string) error {
	return r.db.WithContext(ctx).
		Model(&domain.MALShikimoriMapping{}).
		Where("mal_id = ?", malID).
		Update("anime_id", animeID).Error
}

// Delete deletes a mapping
func (r *MappingRepository) Delete(ctx context.Context, malID int) error {
	return r.db.WithContext(ctx).Delete(&domain.MALShikimoriMapping{}, "mal_id = ?", malID).Error
}

// Exists checks if a mapping exists for a MAL ID
func (r *MappingRepository) Exists(ctx context.Context, malID int) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.MALShikimoriMapping{}).
		Where("mal_id = ?", malID).
		Count(&count).Error
	return count > 0, err
}

// CreateBatch creates multiple mappings in a batch
func (r *MappingRepository) CreateBatch(ctx context.Context, mappings []*domain.MALShikimoriMapping) error {
	if len(mappings) == 0 {
		return nil
	}

	now := time.Now()
	for _, m := range mappings {
		m.CreatedAt = now
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "mal_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"shikimori_id", "anime_id", "confidence", "source"}),
		}).
		CreateInBatches(mappings, 100).Error
}

// Count returns the total number of mappings
func (r *MappingRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.MALShikimoriMapping{}).Count(&count).Error
	return count, err
}
