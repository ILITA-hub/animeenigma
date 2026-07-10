package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// isNotFound reports whether err is a libs/errors NotFound AppError (there is no
// IsNotFound helper — assert on the code).
func isNotFound(err error) bool {
	var ae *liberrors.AppError
	return errors.As(err, &ae) && ae.Code == liberrors.CodeNotFound
}

func newMaintSvc(t *testing.T) *MaintenanceService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&domain.MaintenanceRoutine{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewMaintenanceService(repo.NewMaintenanceRepository(db), logger.Default())
}

func TestMaintenanceService_SeedListSet(t *testing.T) {
	svc := newMaintSvc(t)
	ctx := context.Background()
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rows, err := svc.List(ctx)
	if err != nil || len(rows) != 9 {
		t.Fatalf("list len=%d err=%v", len(rows), err)
	}
	// list is sorted by id
	if rows[0].ID > rows[1].ID {
		t.Errorf("list not sorted by id")
	}
	if err := svc.SetRoutine(ctx, "maintenance_bot", false, domain.SettingsJSON(`{"auto_apply_max_risk":"low"}`)); err != nil {
		t.Fatalf("set: %v", err)
	}
	g, err := svc.Gate(ctx, "maintenance_bot")
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if g.Enabled {
		t.Errorf("gate enabled=true; want false")
	}
}

func TestMaintenanceService_UnknownID_NotFound(t *testing.T) {
	svc := newMaintSvc(t)
	ctx := context.Background()
	_ = svc.SeedDefaults(ctx)
	if _, err := svc.Gate(ctx, "nope"); !isNotFound(err) {
		t.Errorf("gate unknown err = %v; want NotFound", err)
	}
	if err := svc.SetRoutine(ctx, "nope", true, domain.SettingsJSON("{}")); !isNotFound(err) {
		t.Errorf("set unknown err = %v; want NotFound", err)
	}
	if err := svc.SetStatus(ctx, "nope", true, "x", nil); !isNotFound(err) {
		t.Errorf("setStatus unknown err = %v; want NotFound", err)
	}
}

func TestMaintenanceService_SetStatus_stampsLastRun(t *testing.T) {
	svc := newMaintSvc(t)
	ctx := context.Background()
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.SetStatus(ctx, "git_autosync", true, "synced 3 repos", nil); err != nil {
		t.Fatalf("setStatus: %v", err)
	}
	g, err := svc.Gate(ctx, "git_autosync")
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if g.LastRunAt == nil {
		t.Errorf("LastRunAt not stamped")
	}
	if g.LastOK == nil || !*g.LastOK {
		t.Errorf("LastOK = %v; want true", g.LastOK)
	}
	if g.LastSummary != "synced 3 repos" {
		t.Errorf("LastSummary = %q; want %q", g.LastSummary, "synced 3 repos")
	}
}

func TestMaintenanceService_SetStatus_truncatesOnRuneBoundary(t *testing.T) {
	svc := newMaintSvc(t)
	ctx := context.Background()
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// 600 two-byte Cyrillic runes: a byte slice at 512 would split a rune.
	summary := strings.Repeat("я", 600)
	if err := svc.SetStatus(ctx, "git_autosync", false, summary, nil); err != nil {
		t.Fatalf("setStatus: %v", err)
	}
	g, err := svc.Gate(ctx, "git_autosync")
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if !utf8.ValidString(g.LastSummary) {
		t.Errorf("stored summary is not valid UTF-8")
	}
	if n := utf8.RuneCountInString(g.LastSummary); n != 512 {
		t.Errorf("stored summary rune count = %d; want 512", n)
	}
}

func TestMaintenanceService_RejectsInvalidSettings(t *testing.T) {
	svc := newMaintSvc(t)
	ctx := context.Background()
	_ = svc.SeedDefaults(ctx)
	if err := svc.SetRoutine(ctx, "git_autosync", true, domain.SettingsJSON(`not json`)); err == nil {
		t.Errorf("expected invalid-settings error, got nil")
	}
}

// TestMaintenanceService_RejectsNonObjectSettings locks the "settings must be a JSON
// object" refinement: json.Valid alone accepts null/arrays/scalars, so SetRoutine must
// additionally reject anything that doesn't unmarshal into a map[string]any. Empty and
// "{}" must still be accepted (covered by TestMaintenanceService_UnknownID_NotFound's
// "{}" case and SeedDefaults' own empty-settings routines).
func TestMaintenanceService_RejectsNonObjectSettings(t *testing.T) {
	svc := newMaintSvc(t)
	ctx := context.Background()
	_ = svc.SeedDefaults(ctx)
	if err := svc.SetRoutine(ctx, "git_autosync", true, domain.SettingsJSON(`null`)); err == nil {
		t.Errorf("expected invalid-settings error for null, got nil")
	}
	if err := svc.SetRoutine(ctx, "git_autosync", true, domain.SettingsJSON(`[1,2,3]`)); err == nil {
		t.Errorf("expected invalid-settings error for array, got nil")
	}
	if err := svc.SetRoutine(ctx, "git_autosync", true, domain.SettingsJSON(`"a string"`)); err == nil {
		t.Errorf("expected invalid-settings error for scalar, got nil")
	}
	if err := svc.SetRoutine(ctx, "git_autosync", true, domain.SettingsJSON(`{}`)); err != nil {
		t.Errorf("expected {} to be accepted, got %v", err)
	}
	if err := svc.SetRoutine(ctx, "git_autosync", true, domain.SettingsJSON(``)); err != nil {
		t.Errorf("expected empty settings to be accepted, got %v", err)
	}
}
