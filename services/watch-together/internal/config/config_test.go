package config

import (
	"testing"
	"time"
)

// setJWTSecret sets a JWT_SECRET for tests that need Load() to succeed past
// the required-env-var gate. Returned cleanup is wired via t.Setenv (auto).
func setJWTSecret(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret-32chars-minimum-yes-i-am")
}

func TestLoad_RequiresJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected Load() to fail without JWT_SECRET, got nil error")
	}
}

func TestLoad_DefaultServerPortIs8091(t *testing.T) {
	setJWTSecret(t)
	t.Setenv("SERVER_PORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 8091 {
		t.Errorf("expected default Server.Port=8091, got %d", cfg.Server.Port)
	}
}

func TestLoad_ServerPortOverride(t *testing.T) {
	setJWTSecret(t)
	t.Setenv("SERVER_PORT", "9999")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("expected Server.Port=9999, got %d", cfg.Server.Port)
	}
}

func TestLoad_MaxMembersDefaultAndOverride(t *testing.T) {
	setJWTSecret(t)

	// Default.
	t.Setenv("WATCH_TOGETHER_MAX_MEMBERS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MaxMembers != 10 {
		t.Errorf("expected default MaxMembers=10, got %d", cfg.MaxMembers)
	}

	// Override.
	t.Setenv("WATCH_TOGETHER_MAX_MEMBERS", "5")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MaxMembers != 5 {
		t.Errorf("expected MaxMembers=5, got %d", cfg.MaxMembers)
	}
}

func TestLoad_RoomTTLDefaultAndOverride(t *testing.T) {
	setJWTSecret(t)

	// Default.
	t.Setenv("WATCH_TOGETHER_ROOM_TTL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RoomTTL != 900*time.Second {
		t.Errorf("expected default RoomTTL=900s, got %s", cfg.RoomTTL)
	}

	// Override.
	t.Setenv("WATCH_TOGETHER_ROOM_TTL", "600s")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RoomTTL != 600*time.Second {
		t.Errorf("expected RoomTTL=600s, got %s", cfg.RoomTTL)
	}
}

func TestLoad_GracePeriodDefault(t *testing.T) {
	setJWTSecret(t)
	t.Setenv("WATCH_TOGETHER_GRACE_PERIOD", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GracePeriod != 5*time.Minute {
		t.Errorf("expected default GracePeriod=5m, got %s", cfg.GracePeriod)
	}
}

// TestLoad_CatalogURLDefault — WT-STATE-02 (Plan 04.2). With CATALOG_URL unset,
// the catalog service base URL should fall back to the in-cluster Docker
// Compose DNS entry the catalog service registers under.
func TestLoad_CatalogURLDefault(t *testing.T) {
	setJWTSecret(t)
	t.Setenv("CATALOG_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.CatalogURL != "http://catalog:8081" {
		t.Errorf("expected default CatalogURL=http://catalog:8081, got %q", cfg.CatalogURL)
	}
}

// TestLoad_CatalogURLOverride — WT-STATE-02. CATALOG_URL with a trailing
// slash MUST be trimmed (mirrors PublicBaseURL handling) so downstream URL
// construction doesn't produce "http://x//internal/..." double-slashes.
func TestLoad_CatalogURLOverride(t *testing.T) {
	setJWTSecret(t)
	t.Setenv("CATALOG_URL", "http://catalog.test:9000/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.CatalogURL != "http://catalog.test:9000" {
		t.Errorf("expected trimmed CatalogURL=http://catalog.test:9000, got %q", cfg.CatalogURL)
	}
}
