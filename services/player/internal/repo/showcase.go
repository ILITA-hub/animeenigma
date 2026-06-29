package repo

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ShowcaseRepository is the data-access layer for profile_showcases.
type ShowcaseRepository struct {
	db *gorm.DB
}

func NewShowcaseRepository(db *gorm.DB) *ShowcaseRepository {
	return &ShowcaseRepository{db: db}
}

// Get returns the user's showcase, or an empty (Blocks="[]") showcase when
// no row exists yet — never a NotFound error.
func (r *ShowcaseRepository) Get(ctx context.Context, userID string) (*domain.ProfileShowcase, error) {
	var s domain.ProfileShowcase
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&s).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return &domain.ProfileShowcase{UserID: userID, Blocks: "[]"}, nil
		}
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to load showcase")
	}
	return &s, nil
}

// Upsert writes the full blocks JSON + enabled flag for a user (insert or
// replace). On conflict we use explicit clause.Assignments (not
// AssignmentColumns): a false `enabled` is GORM's zero value, which the
// column-name form would omit from the UPDATE — explicit values are robust and
// portable across Postgres/SQLite.
func (r *ShowcaseRepository) Upsert(ctx context.Context, userID, blocksJSON string, enabled bool) error {
	row := domain.ProfileShowcase{UserID: userID, Blocks: blocksJSON, Enabled: enabled}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"blocks":     blocksJSON,
			"enabled":    enabled,
			"updated_at": time.Now(),
		}),
	}).Create(&row).Error
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "failed to upsert showcase")
	}
	return nil
}
