package tracing

import (
	"testing"
)

func TestFromEnv_Defaults(t *testing.T) {
	t.Setenv("TRACING_ENABLED", "")
	t.Setenv("OTLP_ENDPOINT", "")
	t.Setenv("TRACING_SAMPLE_RATE", "")
	cfg := FromEnv("catalog")
	if cfg.Enabled {
		t.Error("expected disabled by default")
	}
	if cfg.OTLPEndpoint != "otel-collector:4317" {
		t.Errorf("default endpoint = %q", cfg.OTLPEndpoint)
	}
	if cfg.SampleRate != 1.0 {
		t.Errorf("default sample rate = %v, want 1.0 (collector tail-samples)", cfg.SampleRate)
	}
	if cfg.ServiceName != "catalog" {
		t.Errorf("service name = %q", cfg.ServiceName)
	}
}

func TestFromEnv_Enabled(t *testing.T) {
	t.Setenv("TRACING_ENABLED", "true")
	t.Setenv("OTLP_ENDPOINT", "host:1234")
	t.Setenv("TRACING_SAMPLE_RATE", "0.5")
	cfg := FromEnv("auth")
	if !cfg.Enabled || cfg.OTLPEndpoint != "host:1234" || cfg.SampleRate != 0.5 {
		t.Errorf("unexpected cfg: %+v", cfg)
	}
}
