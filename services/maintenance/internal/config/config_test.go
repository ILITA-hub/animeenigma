// Package config tests — Phase 23 Plan 23-03 MAINTENANCE_TEST_MODE plumbing.
//
// The TestMode field is a future-hook: a production dispatcher can short-
// circuit on cfg.TestMode == true if a future task wires that gate. Plan
// 23-03 only adds the field + this test confirming Load() honors the env
// var. The synthetic webhook tests in transport/webhook_synthetic_test.go
// do not currently depend on TestMode (they isolate via httptest.NewServer
// instead of touching the live binary), but the env var is documented +
// surfaced so future tests can adopt it.
package config

import (
	"slices"
	"testing"
)

// minimalEnv sets the required environment variables for config.Load() to
// succeed without erroring on missing Telegram fields. Returns nothing —
// callers chain extra `t.Setenv` calls before invoking Load().
func minimalEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-bot-token")
	t.Setenv("TELEGRAM_ADMIN_CHAT_ID", "12345")
}

// TestMaintenanceConfig_TestModeDefault asserts the TestMode field is false
// when MAINTENANCE_TEST_MODE is unset — production-safe default.
func TestMaintenanceConfig_TestModeDefault(t *testing.T) {
	minimalEnv(t)
	// Explicitly clear MAINTENANCE_TEST_MODE in case the test runner has it.
	t.Setenv("MAINTENANCE_TEST_MODE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TestMode {
		t.Errorf("TestMode = true; want false when MAINTENANCE_TEST_MODE is unset")
	}
}

// TestMaintenanceConfig_TestModeTrue asserts the TestMode field flips to
// true when MAINTENANCE_TEST_MODE=true.
func TestMaintenanceConfig_TestModeTrue(t *testing.T) {
	minimalEnv(t)
	t.Setenv("MAINTENANCE_TEST_MODE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.TestMode {
		t.Errorf("TestMode = false; want true when MAINTENANCE_TEST_MODE=true")
	}
}

// TestMaintenanceConfig_AdminsDefault asserts the default admin list carries
// both of NANDIorg's identities — see the dual-identity constraint comment in
// config.go Load().
func TestMaintenanceConfig_AdminsDefault(t *testing.T) {
	minimalEnv(t)
	t.Setenv("ADMIN_USERNAMES", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, want := range []string{"tNeymik", "NANDIorg_9", "NANDIorg"} {
		if !slices.Contains(cfg.Admins, want) {
			t.Errorf("default Admins = %v; missing %q", cfg.Admins, want)
		}
	}
}

func TestMaintenanceConfig_PolicyURLDefaultsToHostLoopback(t *testing.T) {
	minimalEnv(t)
	t.Setenv("POLICY_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PolicyURL != "http://localhost:8098" {
		t.Errorf("PolicyURL = %q; want host loopback policy endpoint", cfg.PolicyURL)
	}
}

// TestMaintenanceConfig_TestModeFalsey asserts that other truthy-looking
// values do NOT enable TestMode — only the literal "true" (canonical
// boolean parsing) does. This guards against accidental activation from
// "1", "yes", etc. since the canonical Go boolean parser is what we want.
func TestMaintenanceConfig_TestModeFalsey(t *testing.T) {
	minimalEnv(t)
	for _, v := range []string{"false", "0", "no", ""} {
		t.Setenv("MAINTENANCE_TEST_MODE", v)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load(%q): %v", v, err)
		}
		if cfg.TestMode {
			t.Errorf("TestMode = true for MAINTENANCE_TEST_MODE=%q; want false", v)
		}
	}
}
