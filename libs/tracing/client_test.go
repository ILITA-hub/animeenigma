package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// An outbound request made through the wrapped transport, while a valid span
// is active in the context, must carry a W3C traceparent so the downstream
// service can continue the trace. This is the gateway propagation guarantee.
func TestWrapTransport_InjectsTraceparent(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "200")
	tr, err := New(context.Background(), Config{ServiceName: "client-test", Enabled: true, SampleRate: 1.0, OTLPEndpoint: "127.0.0.1:4317"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Shutdown(context.Background()) }()

	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("traceparent")
	}))
	defer srv.Close()

	// Establish an active span so there is a context to propagate.
	ctx, span := tr.Start(context.Background(), "outbound")
	defer span.End()

	client := NewHTTPClient(nil, 5*time.Second)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()

	if got == "" {
		t.Fatal("expected traceparent header on the downstream request, got none")
	}
}
