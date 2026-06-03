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

// WebSocket upgrades must bypass otelhttp.NewHandler — its ResponseWriter
// wrapper is not guaranteed to implement http.Hijacker, which gorilla/websocket
// (watch-together) and the gateway WS proxy require. Wrapping a WS request
// would 500 the handshake. We assert the next handler runs unwrapped.
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
