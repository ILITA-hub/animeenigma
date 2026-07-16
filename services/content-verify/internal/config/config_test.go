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
	if cfg.Interval != time.Minute || cfg.UnitBudget != 50*time.Second {
		t.Fatalf("throttle defaults wrong: %s / %s", cfg.Interval, cfg.UnitBudget)
	}
}

func TestLoadRejectsBudgetOverInterval(t *testing.T) {
	t.Setenv("CV_UNIT_BUDGET", "2m")
	if _, err := Load(); err == nil {
		t.Fatal("want error when budget >= interval")
	}
}
