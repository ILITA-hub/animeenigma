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
	if cfg.Interval != 10*time.Second || cfg.UnitBudget != 240*time.Second {
		t.Fatalf("throttle defaults wrong: %s / %s", cfg.Interval, cfg.UnitBudget)
	}
	if cfg.Workers != 2 {
		t.Fatalf("Workers default = %d, want 2", cfg.Workers)
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

func TestLoadIntervalTooSmallErrors(t *testing.T) {
	t.Setenv("CV_INTERVAL", "5s")
	if _, err := Load(); err == nil {
		t.Fatal("CV_INTERVAL below 10s must error")
	}
}

func TestLoadIntervalAtFloorAccepted(t *testing.T) {
	t.Setenv("CV_INTERVAL", "10s")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("CV_INTERVAL at the 10s floor must be accepted: %v", err)
	}
	if cfg.Interval != 10*time.Second {
		t.Fatalf("Interval = %s, want 10s", cfg.Interval)
	}
}

func TestLoadWorkersClamped(t *testing.T) {
	tests := []struct {
		env  string
		want int
	}{
		{"0", 1},
		{"-3", 1},
		{"1", 1},
		{"3", 3},
		{"4", 4},
		{"5", 5},
		{"99", 6},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv("CV_WORKERS", tt.env)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Workers != tt.want {
				t.Fatalf("CV_WORKERS=%s => Workers = %d, want %d", tt.env, cfg.Workers, tt.want)
			}
		})
	}
}

// Graduated-degradation Phase 0: the worker ceiling is 6 (was 4) so the
// score curve's top band is reachable. Floor stays 1.
func TestClampWorkersBounds(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 1}, {-3, 1}, {1, 1}, {4, 4}, {6, 6}, {7, 6}, {100, 6},
	}
	for _, c := range cases {
		if got := clampWorkers(c.in); got != c.want {
			t.Errorf("clampWorkers(%d) = %d; want %d", c.in, got, c.want)
		}
	}
}

func TestParsePins(t *testing.T) {
	tests := []struct {
		env  string
		want map[string]string
	}{
		{"", map[string]string{}},
		{"uuid-1", map[string]string{"uuid-1": ""}},
		{"uuid-1:animejoy-allvideo", map[string]string{"uuid-1": "animejoy-allvideo"}},
		{" uuid-1 : kodik , uuid-2 ", map[string]string{"uuid-1": "kodik", "uuid-2": ""}},
		{",,:", map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv("CV_PIN_ANIME", tt.env)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.Pins) != len(tt.want) {
				t.Fatalf("Pins = %+v, want %+v", cfg.Pins, tt.want)
			}
			for id, provider := range tt.want {
				if got, ok := cfg.Pins[id]; !ok || got != provider {
					t.Fatalf("Pins[%q] = %q (present=%v), want %q", id, got, ok, provider)
				}
			}
		})
	}
}
