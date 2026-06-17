package repo

import (
	"context"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/gorm"
)

// AutocacheConfigRepository is the typed Get/Patch accessor over the
// singleton autocache_config row (id=1, seeded by migration 006). The
// future downloader/evictor (Phases 8-10) reads the master `enabled`
// switch + freshness/budget windows through Get; the admin handler
// writes individual fields through Patch with no redeploy.
type AutocacheConfigRepository struct {
	db *gorm.DB
}

// NewAutocacheConfigRepository constructs an AutocacheConfigRepository
// over the provided *gorm.DB.
func NewAutocacheConfigRepository(db *gorm.DB) *AutocacheConfigRepository {
	return &AutocacheConfigRepository{db: db}
}

// Get loads the singleton row (id=1). A missing row is treated as an
// internal error (not NotFound): the row is seeded by migration 006 at
// startup, so its absence indicates a broken migration, not a normal
// not-found condition.
func (r *AutocacheConfigRepository) Get(ctx context.Context) (*domain.AutocacheConfig, error) {
	var c domain.AutocacheConfig
	if err := r.db.WithContext(ctx).Where("id = ?", 1).First(&c).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "fetch autocache config")
	}
	return &c, nil
}

// Patch applies a partial update to the singleton row (id=1). The fields
// map is keyed by DB column name → new value; an empty map is rejected
// with InvalidInput rather than issuing an empty UPDATE. `updated_at` is
// always bumped to now() in the same Updates() call. The full updated row
// is re-read and returned so the caller (and the API response) reflect
// every field, not just the patched ones.
func (r *AutocacheConfigRepository) Patch(ctx context.Context, fields map[string]any) (*domain.AutocacheConfig, error) {
	if len(fields) == 0 {
		return nil, liberrors.InvalidInput("no fields to update")
	}

	// Copy so we don't mutate the caller's map, then force the updated_at
	// bump in the same partial write.
	updates := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		updates[k] = v
	}
	updates["updated_at"] = gorm.Expr("now()")

	res := r.db.WithContext(ctx).
		Model(&domain.AutocacheConfig{}).
		Where("id = ?", 1).
		Updates(updates)
	if res.Error != nil {
		return nil, liberrors.Wrap(res.Error, liberrors.CodeInternal, "update autocache config")
	}
	// Assert the singleton invariant at the write, independent of Get's
	// missing-row handling: if the id=1 seed row is absent (truncated table,
	// or migration 006's seed failed while the table create succeeded) the
	// UPDATE matches zero rows and would otherwise be a silent write-to-nowhere.
	if res.RowsAffected == 0 {
		return nil, liberrors.Internal("autocache config singleton row missing (broken migration 006)")
	}

	return r.Get(ctx)
}
