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
	if !cfg.SkipEnabled {
		t.Fatal("SkipEnabled default = false, want true")
	}
	if cfg.SkipBudget != 480*time.Second {
		t.Fatalf("SkipBudget = %s, want 480s", cfg.SkipBudget)
	}
	if cfg.SkipHeadWindow != 480*time.Second || cfg.SkipTailWindow != 480*time.Second {
		t.Fatalf("skip window defaults wrong: head=%s tail=%s", cfg.SkipHeadWindow, cfg.SkipTailWindow)
	}
	if cfg.SkipMinMatch != 50*time.Second || cfg.SkipMaxMatch != 150*time.Second {
		t.Fatalf("skip match defaults wrong: min=%s max=%s", cfg.SkipMinMatch, cfg.SkipMaxMatch)
	}
	if cfg.SkipSimThreshold != 0.75 {
		t.Fatalf("SkipSimThreshold = %v, want 0.75", cfg.SkipSimThreshold)
	}
}

func TestLoadSkipEnabledFalse(t *testing.T) {
	t.Setenv("CV_SKIP_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SkipEnabled {
		t.Fatal("CV_SKIP_ENABLED=false must disable the skip lane")
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
