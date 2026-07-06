package repo

import (
	"context"
	"errors"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"gorm.io/gorm"
)

// Repository is the owner-scoped GORM-backed store for domain.Fanfic rows.
// Its method set is consumed as service.fanficStore (Task 5) and
// handler.libraryStore (Task 6).
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Create inserts a new fanfic row. BeforeCreate (domain/fanfic.go) fills ID
// when empty, so f.ID is populated on return.
func (r *Repository) Create(ctx context.Context, f *domain.Fanfic) error {
	if err := r.db.WithContext(ctx).Create(f).Error; err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "create fanfic")
	}
	return nil
}

// UpdateResult stores the generated title/content/usage and flips the row to
// StatusComplete.
func (r *Repository) UpdateResult(ctx context.Context, id, title, content string, usage int) error {
	if err := r.db.WithContext(ctx).Model(&domain.Fanfic{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"title":       title,
			"content":     content,
			"token_usage": usage,
			"status":      domain.StatusComplete,
		}).Error; err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "update fanfic result")
	}
	return nil
}

// MarkFailed flips the row to StatusFailed and records the error message.
func (r *Repository) MarkFailed(ctx context.Context, id, msg string) error {
	if err := r.db.WithContext(ctx).Model(&domain.Fanfic{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":    domain.StatusFailed,
			"error_msg": msg,
		}).Error; err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "mark fanfic failed")
	}
	return nil
}

// List returns the user's fanfics newest-first, paginated, plus the total
// count for that user (ignoring limit/offset).
func (r *Repository) List(ctx context.Context, userID string, limit, offset int) ([]domain.Fanfic, int64, error) {
	var items []domain.Fanfic
	var total int64
	q := r.db.WithContext(ctx).Model(&domain.Fanfic{}).Where("user_id = ?", userID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, liberrors.Wrap(err, liberrors.CodeInternal, "count fanfics")
	}
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, liberrors.Wrap(err, liberrors.CodeInternal, "list fanfics")
	}
	return items, total, nil
}

// Get fetches a single fanfic scoped to its owner; a non-owner or missing row
// both return a NotFound error.
func (r *Repository) Get(ctx context.Context, userID, id string) (*domain.Fanfic, error) {
	var f domain.Fanfic
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("fanfic")
		}
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "get fanfic")
	}
	return &f, nil
}

// SoftDelete soft-deletes a fanfic scoped to its owner (GORM DeletedAt). A
// non-owner delete affects zero rows and returns NotFound.
func (r *Repository) SoftDelete(ctx context.Context, userID, id string) error {
	res := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&domain.Fanfic{})
	if res.Error != nil {
		return liberrors.Wrap(res.Error, liberrors.CodeInternal, "soft delete fanfic")
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("fanfic")
	}
	return nil
}
