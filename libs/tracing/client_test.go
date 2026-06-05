package tracing

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/baggage"
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

// TestNoUserIDOnOutboundWire is the T-02-PII security proof: user_id rides ONLY
// the private non-propagated ctx value, so it must never appear in the W3C
// `baggage:` header on an outbound 3rd-party-bound request — while the recorder's
// in-process Effect still has UserID populated (read from the private ctx). We
// additionally inject a forged user_id baggage member to prove the defense-in-depth
// strip removes it before the request leaves the process.
func TestNoUserIDOnOutboundWire(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "200")
	tr, err := New(context.Background(), Config{ServiceName: "client-test", Enabled: true, SampleRate: 1.0, OTLPEndpoint: "127.0.0.1:4317"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Shutdown(context.Background()) }()

	var gotBaggage string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotBaggage = r.Header.Get("baggage")
	}))
	defer srv.Close()

	// Seed the legitimate attribution: origin/operation on wire baggage, user_id
	// on the PRIVATE ctx value.
	ctx, span := tr.Start(context.Background(), "outbound")
	defer span.End()
	ctx = SeedBaggage(ctx, "api", "catalog GET /api/anime/{id}")
	ctx = WithUserID(ctx, "user-42")

	// Defense-in-depth: forge a user_id baggage member the way a future careless
	// caller might. The recordingTransport MUST strip it before it rides the wire.
	if m, mErr := baggage.NewMemberRaw("user_id", "user-42"); mErr == nil {
		bg := baggage.FromContext(ctx)
		if next, sErr := bg.SetMember(m); sErr == nil {
			ctx = baggage.ContextWithBaggage(ctx, next)
		}
	}

	sink := &captureSink{}
	client := &http.Client{Transport: WrapRecording(WrapTransport(nil), sink)}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, doErr := client.Do(req)
	if doErr != nil {
		t.Fatalf("Do: %v", doErr)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// The wire baggage header must carry origin/operation but NOT user_id.
	if !strings.Contains(gotBaggage, "origin=api") {
		t.Fatalf("expected origin=api in wire baggage, got %q", gotBaggage)
	}
	if strings.Contains(gotBaggage, "user_id") {
		t.Fatalf("user_id leaked onto the wire baggage header: %q", gotBaggage)
	}

	// The recorder's in-process Effect still carries UserID from the private ctx.
	if sink.count() != 1 {
		t.Fatalf("expected exactly 1 recorded effect, got %d", sink.count())
	}
	if got := sink.at(0).UserID; got != "user-42" {
		t.Fatalf("recorded Effect.UserID = %q, want user-42 (from private ctx)", got)
	}
}

// TestBaggageE2E proves origin/operation seeded on an inbound request (via
// SeedMiddleware) carry end-to-end into the egress Effect emitted by an outbound
// call the handler makes on the recording transport (AR-EGRESS-02).
func TestBaggageE2E(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "200")
	tr, err := New(context.Background(), Config{ServiceName: "e2e-test", Enabled: true, SampleRate: 1.0, OTLPEndpoint: "127.0.0.1:4317"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Shutdown(context.Background()) }()

	// The 3rd-party host the handler calls outbound.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	sink := &captureSink{}
	outboundClient := &http.Client{Transport: WrapRecording(WrapTransport(nil), sink)}

	// Inbound handler wrapped in SeedMiddleware: makes an outbound call.
	handler := SeedMiddleware("catalog")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In a real chi service the route pattern is resolved before the handler
		// runs; the test asserts origin (eagerly seeded) carries through to the
		// outbound Effect.
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream.URL, nil)
		resp, callErr := outboundClient.Do(req)
		if callErr != nil {
			http.Error(w, callErr.Error(), http.StatusBadGateway)
			return
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))

	inbound := httptest.NewServer(handler)
	defer inbound.Close()

	resp, err := http.Get(inbound.URL + "/api/anime/42")
	if err != nil {
		t.Fatalf("inbound Get: %v", err)
	}
	_ = resp.Body.Close()

	if sink.count() != 1 {
		t.Fatalf("expected 1 outbound effect, got %d", sink.count())
	}
	eff := sink.at(0)
	if eff.Origin != "api" {
		t.Fatalf("outbound Effect.Origin = %q, want api (seeded inbound)", eff.Origin)
	}
}
