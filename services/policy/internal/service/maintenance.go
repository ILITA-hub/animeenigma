package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"
	"unicode/utf8"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"gorm.io/gorm"
)

type MaintenanceService struct {
	repo *repo.MaintenanceRepository
	log  *logger.Logger
}

func NewMaintenanceService(r *repo.MaintenanceRepository, log *logger.Logger) *MaintenanceService {
	return &MaintenanceService{repo: r, log: log}
}

// SeedDefaults inserts the parity defaults (insert-if-absent, idempotent).
func (s *MaintenanceService) SeedDefaults(ctx context.Context) error {
	for _, m := range domain.SeedRoutines() {
		if err := s.repo.SeedIfAbsent(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// List returns all routines sorted by id (stable admin-list order).
func (s *MaintenanceService) List(ctx context.Context) ([]domain.MaintenanceRoutine, error) {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

// Gate returns the enforcement view a routine reads each run. Unknown id ⇒ NotFound
// (P3 callers treat any non-200 as fail-open enabled=true).
func (s *MaintenanceService) Gate(ctx context.Context, id string) (*domain.MaintenanceRoutine, error) {
	row, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, liberrors.NotFound("routine not found")
	}
	return row, err
}

// SetRoutine replaces enabled+settings for an existing routine. settings must be a
// JSON object ({...}) — null, arrays, and scalars are rejected even though they are
// valid JSON, per spec ("settings must be a valid JSON object").
func (s *MaintenanceService) SetRoutine(ctx context.Context, id string, enabled bool, settings domain.SettingsJSON) error {
	if err := s.mustExist(ctx, id); err != nil {
		return err
	}
	if !isJSONObject(nonEmpty(settings)) {
		return liberrors.InvalidInput("settings must be a JSON object")
	}
	return s.repo.SetIntent(ctx, id, enabled, settings)
}

// SetStatus stamps last-run fields (consumed by P3 routines).
func (s *MaintenanceService) SetStatus(ctx context.Context, id string, ok bool, summary string, next *time.Time) error {
	if err := s.mustExist(ctx, id); err != nil {
		return err
	}
	// LastSummary is varchar(512) — counts CHARACTERS, not bytes. This is a
	// bilingual EN/RU codebase; a byte slice would split a multi-byte Cyrillic
	// rune and store invalid UTF-8 (Postgres text rejects it). Truncate on rune
	// boundaries to 512 runes.
	if utf8.RuneCountInString(summary) > 512 {
		summary = string([]rune(summary)[:512])
	}
	return s.repo.SetStatus(ctx, id, ok, summary, next)
}

func (s *MaintenanceService) mustExist(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); errors.Is(err, gorm.ErrRecordNotFound) {
		return liberrors.NotFound("routine not found")
	} else if err != nil {
		return err
	}
	return nil
}

func nonEmpty(s domain.SettingsJSON) []byte {
	if len(s) == 0 {
		return []byte("{}")
	}
	return []byte(s)
}

// isJSONObject reports whether b is valid JSON AND its top-level value is an object
// ({...}). json.Valid alone would also accept null/arrays/scalars, which the spec's
// "must be a valid JSON object" wording excludes. Note: unmarshaling straight into a
// map[string]any is NOT sufficient — encoding/json treats a top-level `null` as a
// no-op for map targets (err==nil, map left nil), so decode into `any` first and
// type-assert the result.
func isJSONObject(b []byte) bool {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return false
	}
	_, ok := v.(map[string]any)
	return ok
}
