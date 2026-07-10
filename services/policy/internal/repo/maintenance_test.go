package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMaintDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.MaintenanceRoutine{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestMaintenanceRepo_SeedGetSetIntent(t *testing.T) {
	db := newMaintDB(t)
	r := NewMaintenanceRepository(db)
	ctx := context.Background()

	for _, m := range domain.SeedRoutines() {
		if err := r.SeedIfAbsent(ctx, m); err != nil {
			t.Fatalf("seed %s: %v", m.ID, err)
		}
	}
	// Idempotent: second seed must not overwrite an admin edit.
	if err := r.SetIntent(ctx, "provider_recovery", false, domain.SettingsJSON(`{"model":"opus"}`)); err != nil {
		t.Fatalf("set intent: %v", err)
	}
	for _, m := range domain.SeedRoutines() {
		_ = r.SeedIfAbsent(ctx, m) // no-op on conflict
	}
	got, err := r.GetByID(ctx, "provider_recovery")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Enabled {
		t.Errorf("enabled = true; want false (admin paused, survived re-seed)")
	}
	if string(got.Settings) != `{"model":"opus"}` {
		t.Errorf("settings = %s; want {\"model\":\"opus\"}", string(got.Settings))
	}

	all, err := r.GetAll(ctx)
	if err != nil || len(all) != 9 {
		t.Fatalf("getall len = %d err = %v; want 9", len(all), err)
	}
}

func TestMaintenanceRepo_SetStatus(t *testing.T) {
	db := newMaintDB(t)
	r := NewMaintenanceRepository(db)
	ctx := context.Background()
	_ = r.SeedIfAbsent(ctx, domain.MaintenanceRoutine{ID: "git_autosync", Enabled: true, Settings: domain.SettingsJSON("{}")})

	if err := r.SetStatus(ctx, "git_autosync", true, "in-sync · HEAD abc123", nil); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, _ := r.GetByID(ctx, "git_autosync")
	if got.LastOK == nil || !*got.LastOK {
		t.Errorf("lastOk = %v; want true", got.LastOK)
	}
	if got.LastRunAt == nil {
		t.Errorf("lastRunAt not stamped")
	}
	if got.LastSummary != "in-sync · HEAD abc123" {
		t.Errorf("summary = %q", got.LastSummary)
	}
	if !got.Enabled { // status write must not touch intent
		t.Errorf("enabled clobbered by status write")
	}
}
