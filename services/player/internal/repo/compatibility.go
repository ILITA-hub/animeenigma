package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
)

// maxCompatEntries defensively bounds the unbounded Preload Find in ListEntries
// so a pathologically large watchlist can't blow up the Anime + Genres preload
// (audit L606). Mirrors the LIMIT 200 defensive cap on the spotlight aggregator
// query (repo/list.go GetByUserAndStatusesWithProgress); 1000 is generous for a
// small self-hosted group's list while still bounding the genre preload.
const maxCompatEntries = 1000

// CompatibilityRepository loads a user's anime list with genres for the
// compatibility blend computation.
type CompatibilityRepository struct{ db *gorm.DB }

func NewCompatibilityRepository(db *gorm.DB) *CompatibilityRepository {
	return &CompatibilityRepository{db: db}
}

// ListEntries returns the user's list as compatibility projections.
// Each row's genres are eagerly loaded via Preload so the blend can compute
// genre cosine similarity without additional queries. Capped at maxCompatEntries
// rows so the preload can't run unbounded.
func (r *CompatibilityRepository) ListEntries(ctx context.Context, userID string) ([]domain.UserListEntry, error) {
	var rows []domain.AnimeListEntry
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("user_id = ?", userID).
		Limit(maxCompatEntries).
		Find(&rows).Error
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to load list for compatibility")
	}
	out := make([]domain.UserListEntry, 0, len(rows))
	for _, row := range rows {
		if row.Anime == nil {
			// soft-deleted or missing anime — include entry with no genres
			out = append(out, domain.UserListEntry{AnimeID: row.AnimeID, Score: row.Score})
			continue
		}
		genreIDs := make([]string, 0, len(row.Anime.Genres))
		for _, g := range row.Anime.Genres {
			genreIDs = append(genreIDs, g.ID)
		}
		out = append(out, domain.UserListEntry{AnimeID: row.AnimeID, Score: row.Score, GenreIDs: genreIDs})
	}
	return out, nil
}
