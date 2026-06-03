package tracing

import (
	"context"
	"os"
	"strconv"
)

// FromEnv builds a Config from standard env vars. Defaults are chosen so that
// the only var a service needs to set to participate is TRACING_ENABLED=true:
//
//	TRACING_ENABLED      bool    (default false — off until explicitly enabled)
//	OTLP_ENDPOINT        string  (default "otel-collector:4317", gRPC)
//	TRACING_SAMPLE_RATE  float64 (default 1.0 — head-sample everything; the
//	                              OTel Collector does tail sampling centrally)
//	ENV                  string  (default "development")
func FromEnv(service string) Config {
	enabled, _ := strconv.ParseBool(os.Getenv("TRACING_ENABLED"))

	endpoint := os.Getenv("OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "otel-collector:4317"
	}

	rate := 1.0
	if v := os.Getenv("TRACING_SAMPLE_RATE"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			rate = parsed
		}
	}

	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
	}

	return Config{
		ServiceName:    service,
		ServiceVersion: os.Getenv("SERVICE_VERSION"),
		Environment:    env,
		OTLPEndpoint:   endpoint,
		SampleRate:     rate,
		Enabled:        enabled,
	}
}

// InitFromEnv is the one-call convenience used in service main.go:
//
//	tr, err := tracing.InitFromEnv(context.Background(), "catalog")
//	if err != nil { log.Fatalw("tracing init", "error", err) }
//	defer func() { _ = tr.Shutdown(context.Background()) }()
func InitFromEnv(ctx context.Context, service string) (*Tracer, error) {
	return New(ctx, FromEnv(service))
}
