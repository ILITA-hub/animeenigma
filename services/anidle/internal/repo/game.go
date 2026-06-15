package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("anidle: not found")

type GameRepo struct{ db *gorm.DB }

func NewGameRepo(db *gorm.DB) *GameRepo { return &GameRepo{db: db} }

func (r *GameRepo) GetDailyPuzzle(ctx context.Context, date string) (*domain.DailyPuzzle, error) {
	var p domain.DailyPuzzle
	err := r.db.WithContext(ctx).First(&p, "date = ?", date).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get daily puzzle: %w", err)
	}
	return &p, nil
}

func (r *GameRepo) CreateDailyPuzzle(ctx context.Context, p *domain.DailyPuzzle) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// RecentAnswerIDs returns the anime_ids of the most recent `days` puzzles.
func (r *GameRepo) RecentAnswerIDs(ctx context.Context, days int) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&domain.DailyPuzzle{}).
		Order("date DESC").Limit(days).Pluck("anime_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("recent answer ids: %w", err)
	}
	return ids, nil
}

func (r *GameRepo) GetUserResult(ctx context.Context, userID, date, mode string) (*domain.UserGameResult, error) {
	var res domain.UserGameResult
	err := r.db.WithContext(ctx).First(&res, "user_id = ? AND puzzle_date = ? AND mode = ?", userID, date, mode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil // absence is not an error for resume
	}
	if err != nil {
		return nil, fmt.Errorf("get user result: %w", err)
	}
	return &res, nil
}

// SaveUserResult upserts on (user_id, puzzle_date, mode).
func (r *GameRepo) SaveUserResult(ctx context.Context, res *domain.UserGameResult) error {
	existing, err := r.GetUserResult(ctx, res.UserID, res.PuzzleDate, res.Mode)
	if err != nil {
		return err
	}
	if existing != nil {
		res.ID = existing.ID
		res.CreatedAt = existing.CreatedAt
		return r.db.WithContext(ctx).Save(res).Error
	}
	// Ensure ID is set for databases that don't auto-generate UUIDs (e.g. sqlite in tests).
	if res.ID == "" {
		res.ID = uuid.New().String()
	}
	return r.db.WithContext(ctx).Create(res).Error
}

func (r *GameRepo) GetUserStats(ctx context.Context, userID string) (*domain.UserStats, error) {
	var st domain.UserStats
	err := r.db.WithContext(ctx).First(&st, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user stats: %w", err)
	}
	return &st, nil
}

func (r *GameRepo) SaveUserStats(ctx context.Context, st *domain.UserStats) error {
	return r.db.WithContext(ctx).Save(st).Error
}
