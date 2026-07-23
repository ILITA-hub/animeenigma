package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm/clause"
)

// UpsertAiringOccurrences stores confirmed episode timestamps and lets a later
// provider sync correct the date/source for the same anime episode.
func (r *AnimeRepository) UpsertAiringOccurrences(ctx context.Context, occurrences []domain.AnimeAiringOccurrence) error {
	if len(occurrences) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "anime_id"}, {Name: "episode"}},
		DoUpdates: clause.AssignmentColumns([]string{"aired_at", "source", "updated_at"}),
	}).Create(&occurrences).Error; err != nil {
		return fmt.Errorf("upsert airing occurrences: %w", err)
	}
	return nil
}

// GetAiringOccurrences returns confirmed history in [from, to), including the
// anime metadata needed by the schedule UI. Hidden/deleted anime stay hidden.
func (r *AnimeRepository) GetAiringOccurrences(ctx context.Context, from, to time.Time) ([]domain.AnimeAiringOccurrence, error) {
	var occurrences []domain.AnimeAiringOccurrence
	err := r.db.WithContext(ctx).
		Model(&domain.AnimeAiringOccurrence{}).
		Joins("JOIN animes ON animes.id = anime_airing_occurrences.anime_id").
		Where("anime_airing_occurrences.aired_at >= ? AND anime_airing_occurrences.aired_at < ?", from, to).
		Where("(animes.hidden = ? OR animes.hidden IS NULL) AND animes.deleted_at IS NULL", false).
		Preload("Anime").
		Preload("Anime.Genres").
		Order("anime_airing_occurrences.aired_at ASC, anime_airing_occurrences.episode ASC").
		Find(&occurrences).Error
	if err != nil {
		return nil, fmt.Errorf("get airing occurrences: %w", err)
	}
	return occurrences, nil
}
