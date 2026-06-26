package config

import (
	"os"
	"testing"
)

func TestLoad_DefaultsAndOverrides(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("SERVER_PORT", "8096")
	t.Setenv("SEGMENT_SECONDS", "") // unset → default 45
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 8096 {
		t.Fatalf("port = %d, want 8096", cfg.Server.Port)
	}
	if cfg.Upscaler.SegmentSeconds != 45 {
		t.Fatalf("SegmentSeconds = %d, want 45 default", cfg.Upscaler.SegmentSeconds)
	}
	if cfg.Upscaler.DefaultScale != 2 {
		t.Fatalf("DefaultScale = %d, want 2 default", cfg.Upscaler.DefaultScale)
	}
	if !cfg.Upscaler.RemoteShellEnabled {
		t.Fatal("RemoteShellEnabled should default true")
	}
	_ = os.Unsetenv
}
