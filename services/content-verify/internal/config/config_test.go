package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8101 {
		t.Fatalf("port = %d, want 8101", cfg.Server.Port)
	}
	if cfg.Interval != time.Minute || cfg.UnitBudget != 240*time.Second {
		t.Fatalf("throttle defaults wrong: %s / %s", cfg.Interval, cfg.UnitBudget)
	}
}

// Budget MAY exceed the interval: the inter-probe pause runs after each
// probe completes, so a long budget stretches cadence instead of stacking.
func TestLoadAllowsBudgetOverInterval(t *testing.T) {
	t.Setenv("CV_UNIT_BUDGET", "10m")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("budget > interval must be accepted: %v", err)
	}
	if cfg.UnitBudget != 10*time.Minute {
		t.Fatalf("UnitBudget = %s, want 10m", cfg.UnitBudget)
	}
}
