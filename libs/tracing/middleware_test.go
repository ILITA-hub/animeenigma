package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// With tracing enabled, an inbound W3C traceparent must be extracted so the
// handler sees a span whose TraceID equals the header's trace-id (the FE→BE
// continuation guarantee).
func TestHTTPMiddleware_ContinuesInboundTrace(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "200")
	tr, err := New(context.Background(), Config{ServiceName: "test", Enabled: true, SampleRate: 1.0, OTLPEndpoint: "127.0.0.1:4317"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Shutdown(context.Background()) }()

	const traceID = "0af7651916cd43dd8448eb211c80319c"
	var seen string
	h := HTTPMiddleware("test")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = trace.SpanContextFromContext(r.Context()).TraceID().String()
	}))

	req := httptest.NewRequest(http.MethodGet, "/anime/123", nil)
	req.Header.Set("traceparent", "00-"+traceID+"-b7ad6b7169203331-01")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if seen != traceID {
		t.Fatalf("expected inbound trace continued (%s), got %q", traceID, seen)
	}
}

// normalizeSpanPath must collapse dynamic path segments (UUIDs, numeric IDs)
// so the otelhttp span name has coarse cardinality — otherwise every distinct
// anime UUID becomes a distinct SpanName value in the CH otel_traces column.
func TestNormalizeSpanPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"uuid segment", "/api/anime/3fa85f64-5717-4562-b3fc-2c963f66afa6", "/api/anime/:id"},
		{"numeric id with trailing segment", "/api/anime/42/episodes", "/api/anime/:id/episodes"},
		{"static path unchanged", "/api/genres", "/api/genres"},
		{"empty path", "", "/"},
		{"root", "/", "/"},
		{"trailing slash", "/api/anime/42/", "/api/anime/:id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeSpanPath(tc.in); got != tc.want {
				t.Fatalf("normalizeSpanPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The otelhttp span-name formatter must produce "METHOD <normalized-path>" so
// the trace span name is low-cardinality regardless of concrete IDs in the URL.
func TestSpanNameFormatter(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/anime/3fa85f64-5717-4562-b3fc-2c963f66afa6", nil)
	if got, want := spanNameFromRequest(r), "GET /api/anime/:id"; got != want {
		t.Fatalf("spanNameFromRequest = %q, want %q", got, want)
	}
}

// /health, /healthz and /metrics must bypass the span machinery so health
// checks and Prometheus scrapes never create trace spam.
func TestHTTPMiddleware_BypassesOpsEndpoints(t *testing.T) {
	called := false
	h := HTTPMiddleware("test")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))
	for _, p := range []string{"/health", "/healthz", "/metrics"} {
		called = false
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
		if !called {
			t.Fatalf("bypass path %s did not call next handler", p)
		}
	}
}

// WebSocket upgrades bypass otelhttp.NewHandler: not because Hijacker is lost
// (httpsnoop preserves it) but to avoid a span that mis-times against the
// long-lived WS connection. We assert the next handler runs unwrapped.
func TestHTTPMiddleware_BypassesWebSocketUpgrade(t *testing.T) {
	called := false
	h := HTTPMiddleware("test")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodGet, "/api/watch-together/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !called {
		t.Fatal("websocket upgrade was not passed through unwrapped")
	}
}
