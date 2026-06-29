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
	if cfg.FFprobePath != "ffprobe" {
		t.Errorf("FFprobePath = %q, want ffprobe", cfg.FFprobePath)
	}
	if cfg.ProbeProviders != "gogoanime,miruro,allanime,okru,nineanime,animepahe,animefever,ae,kodik-noads" {
		t.Errorf("ProbeProviders = %q, want gogoanime,...,nineanime,animepahe,animefever,ae,kodik-noads", cfg.ProbeProviders)
	}
}
