package repo

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
)

// AnnouncementDismissalsRepository persists upcoming_for_you dismiss actions
// (spec 2026-07-17).
type AnnouncementDismissalsRepository struct {
	db *gorm.DB
}

// NewAnnouncementDismissalsRepository wires the repository.
func NewAnnouncementDismissalsRepository(db *gorm.DB) *AnnouncementDismissalsRepository {
	return &AnnouncementDismissalsRepository{db: db}
}

// Insert records a dismissal. Idempotent: a duplicate (user, anime) pair is
// a silent no-op via ON CONFLICT DO NOTHING on the unique index. The UUID is
// generated in Go so the same code runs on Postgres and the SQLite test DB.
func (r *AnnouncementDismissalsRepository) Insert(ctx context.Context, userID, animeID string) error {
	row := domain.RecAnnouncementDismissal{
		ID:      uuid.NewString(),
		UserID:  userID,
		AnimeID: animeID,
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
			DoNothing: true,
		}).
		Create(&row).Error
}

// ListAnimeIDs returns every anime the user has dismissed, for candidate-pool
// exclusion. Ordered by anime_id for deterministic tests.
func (r *AnnouncementDismissalsRepository) ListAnimeIDs(ctx context.Context, userID string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).
		Model(&domain.RecAnnouncementDismissal{}).
		Where("user_id = ?", userID).
		Order("anime_id ASC").
		Pluck("anime_id", &ids).Error
	return ids, err
}
