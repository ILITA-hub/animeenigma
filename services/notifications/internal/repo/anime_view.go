package repo

import (
	"context"
	stderrors "errors"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"gorm.io/gorm"
)

// AnimeViewRepository serves catalog-side anime metadata to the Phase 2
// detector via the read-only AnimeView projection in views.go. Used to
// build the new_episode payload (anime_title, anime_poster_url,
// shikimori_id) without crossing service boundaries.
//
// Read-only — never writes to catalog's animes table.
type AnimeViewRepository struct {
	db *gorm.DB
}

// NewAnimeViewRepository constructs the repo.
func NewAnimeViewRepository(db *gorm.DB) *AnimeViewRepository {
	return &AnimeViewRepository{db: db}
}

// GetByID returns the AnimeView projection for a given catalog anime UUID.
// Returns apperrors.NotFound if no row exists — the detector memoizes
// per-animeID lookups across combos sharing an animeID, so this is at
// most one query per unique anime per run.
func (r *AnimeViewRepository) GetByID(ctx context.Context, animeID string) (*AnimeView, error) {
	var v AnimeView
	err := r.db.WithContext(ctx).
		Table("animes").
		Select("id, shikimori_id, status, name, name_ru, poster_url").
		Where("id = ?", animeID).
		Take(&v).Error
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("anime")
	}
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "get anime view")
	}
	return &v, nil
}
