package config

import (
	"testing"
)

func TestLoad_ProbeDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.CatalogURL != "http://catalog:8081" {
		t.Errorf("CatalogURL = %q, want http://catalog:8081", cfg.CatalogURL)
	}
	if cfg.StreamingURL != "http://streaming:8082" {
		t.Errorf("StreamingURL = %q, want http://streaming:8082", cfg.StreamingURL)
	}
	if cfg.ProbeAnchorUUID != "f0b40660-6627-4a59-8dcf-7ec8596b3623" {
		t.Errorf("ProbeAnchorUUID = %q, want f0b40660-...", cfg.ProbeAnchorUUID)
	}
	if cfg.ProbeAllanimeOkruAnchorUUID != "6f2bc143-71d1-47a2-902f-ead849c82d63" {
		t.Errorf("ProbeAllanimeOkruAnchorUUID = %q, want 6f2bc143-...", cfg.ProbeAllanimeOkruAnchorUUID)
	}
	if cfg.ProbeAllanimeOkruAnchorName != "Кот и дракон" {
		t.Errorf("ProbeAllanimeOkruAnchorName = %q, want Кот и дракон", cfg.ProbeAllanimeOkruAnchorName)
	}
	if cfg.FFprobePath != "ffprobe" {
		t.Errorf("FFprobePath = %q, want ffprobe", cfg.FFprobePath)
	}
	// AUTO-608: PROBE_PROVIDERS is now an optional filter over the DB roster's
	// wirable rows; the default is empty (= no filter, every wirable row probed).
	if cfg.ProbeProviders != "" {
		t.Errorf("ProbeProviders = %q, want \"\" (optional filter, default = all wirable roster rows)", cfg.ProbeProviders)
	}
}
