package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/google/uuid"
)

type ListRepository struct {
	db *database.DB
}

func NewListRepository(db *database.DB) *ListRepository {
	return &ListRepository{
		db: db,
	}
}

// Upsert creates or updates an anime list entry
func (r *ListRepository) Upsert(ctx context.Context, entry *domain.AnimeListEntry) error {
	query := `
		INSERT INTO anime_list (id, user_id, anime_id, anime_title, anime_cover, status, score, episodes, notes, started_at, completed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (user_id, anime_id)
		DO UPDATE SET
			anime_title = COALESCE(NULLIF(EXCLUDED.anime_title, ''), anime_list.anime_title),
			anime_cover = COALESCE(NULLIF(EXCLUDED.anime_cover, ''), anime_list.anime_cover),
			status = EXCLUDED.status,
			score = EXCLUDED.score,
			episodes = EXCLUDED.episodes,
			notes = EXCLUDED.notes,
			started_at = EXCLUDED.started_at,
			completed_at = EXCLUDED.completed_at,
			updated_at = EXCLUDED.updated_at
	`

	now := time.Now()
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	entry.CreatedAt = now
	entry.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		entry.ID,
		entry.UserID,
		entry.AnimeID,
		entry.AnimeTitle,
		entry.AnimeCover,
		entry.Status,
		entry.Score,
		entry.Episodes,
		entry.Notes,
		entry.StartedAt,
		entry.CompletedAt,
		entry.CreatedAt,
		entry.UpdatedAt,
	)
	return err
}

// GetByUser returns all anime list entries for a user
func (r *ListRepository) GetByUser(ctx context.Context, userID string) ([]*domain.AnimeListEntry, error) {
	query := `
		SELECT id, user_id, anime_id, anime_title, anime_cover, status, score, episodes, notes, started_at, completed_at, created_at, updated_at
		FROM anime_list
		WHERE user_id = $1
		ORDER BY updated_at DESC
	`

	var entries []*domain.AnimeListEntry
	err := r.db.SelectContext(ctx, &entries, query, userID)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// GetByUserAndStatus returns anime list entries for a user filtered by status
func (r *ListRepository) GetByUserAndStatus(ctx context.Context, userID, status string) ([]*domain.AnimeListEntry, error) {
	query := `
		SELECT id, user_id, anime_id, anime_title, anime_cover, status, score, episodes, notes, started_at, completed_at, created_at, updated_at
		FROM anime_list
		WHERE user_id = $1 AND status = $2
		ORDER BY updated_at DESC
	`

	var entries []*domain.AnimeListEntry
	err := r.db.SelectContext(ctx, &entries, query, userID, status)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// Delete removes an anime from user's list
func (r *ListRepository) Delete(ctx context.Context, userID, animeID string) error {
	query := `DELETE FROM anime_list WHERE user_id = $1 AND anime_id = $2`
	_, err := r.db.ExecContext(ctx, query, userID, animeID)
	return err
}
