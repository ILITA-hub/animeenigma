// Package repo persists policy-service maintenance routine intent + status.
package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MaintenanceRepository struct{ db *gorm.DB }

func NewMaintenanceRepository(db *gorm.DB) *MaintenanceRepository {
	return &MaintenanceRepository{db: db}
}

// GetAll returns every maintenance routine row.
func (r *MaintenanceRepository) GetAll(ctx context.Context) ([]domain.MaintenanceRoutine, error) {
	var rows []domain.MaintenanceRoutine
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetByID returns gorm.ErrRecordNotFound when the id has no row.
func (r *MaintenanceRepository) GetByID(ctx context.Context, id string) (*domain.MaintenanceRoutine, error) {
	var row domain.MaintenanceRoutine
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// SeedIfAbsent inserts a default only when the id has no row (idempotent boot seed).
func (r *MaintenanceRepository) SeedIfAbsent(ctx context.Context, m domain.MaintenanceRoutine) error {
	m.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&m).Error
}

// SetIntent writes enabled+settings via a column-scoped Updates map so the
// zero-value `false` is always persisted (a struct Save would skip it) and the
// status columns are left untouched.
func (r *MaintenanceRepository) SetIntent(ctx context.Context, id string, enabled bool, settings domain.SettingsJSON) error {
	return r.db.WithContext(ctx).Model(&domain.MaintenanceRoutine{}).
		Where("id = ?", id).
		Updates(map[string]any{"enabled": enabled, "settings": settings, "updated_at": time.Now()}).Error
}

// SetStatus stamps last-run fields only; never touches enabled/settings.
func (r *MaintenanceRepository) SetStatus(ctx context.Context, id string, ok bool, summary string, next *time.Time) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&domain.MaintenanceRoutine{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_run_at": now, "last_ok": ok, "last_summary": summary,
			"next_run_at": next, "updated_at": now,
		}).Error
}
