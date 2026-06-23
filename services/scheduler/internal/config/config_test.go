package config

import (
	"testing"
)

// TestLoad_PlaybackProbeDefaults asserts the PlaybackProbeCron field defaults
// to `0 */6 * * *` (every 6h — the policy-machine base tick). The env var is
// PLAYBACK_PROBE_CRON (replaced SCRAPER_PLAYABILITY_CANARY_CRON in Phase A).
func TestLoad_PlaybackProbeDefaults(t *testing.T) {
	t.Setenv("PLAYBACK_PROBE_CRON", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err = %v; want nil", err)
	}
	if got, want := cfg.Jobs.PlaybackProbeCron, "0 */6 * * *"; got != want {
		t.Errorf("PlaybackProbeCron = %q; want %q", got, want)
	}
}

// TestLoad_PlaybackProbeOverride asserts the env-var override is honored.
func TestLoad_PlaybackProbeOverride(t *testing.T) {
	t.Setenv("PLAYBACK_PROBE_CRON", "*/5 * * * *")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err = %v; want nil", err)
	}
	if got, want := cfg.Jobs.PlaybackProbeCron, "*/5 * * * *"; got != want {
		t.Errorf("PlaybackProbeCron = %q; want %q", got, want)
	}
}
