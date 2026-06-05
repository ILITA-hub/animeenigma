package tracing

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureSink records every Effect for assertions.
type captureSink struct {
	mu      sync.Mutex
	effects []Effect
}

func (c *captureSink) Record(e Effect) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.effects = append(c.effects, e)
}
func (c *captureSink) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.effects)
}
func (c *captureSink) at(i int) Effect {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.effects[i]
}

// TestRecordingTransport: exactly one Effect per outbound request, with Host,
// Status, BytesIn (counted on body read/close) and DurationMS > 0.
func TestRecordingTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "hello world payload")
	}))
	defer srv.Close()

	sink := &captureSink{}
	client := &http.Client{Transport: WrapRecording(http.DefaultTransport, sink)}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if sink.count() != 1 {
		t.Fatalf("expected exactly 1 effect, got %d", sink.count())
	}
	eff := sink.at(0)
	if eff.Host == "" {
		t.Fatalf("Host not recorded: %+v", eff)
	}
	if eff.Status != http.StatusOK {
		t.Fatalf("Status = %d, want 200", eff.Status)
	}
	if eff.BytesIn != len(body) {
		t.Fatalf("BytesIn = %d, want %d (full body)", eff.BytesIn, len(body))
	}
	if eff.DurationMS < 0 {
		t.Fatalf("DurationMS must be >= 0, got %d", eff.DurationMS)
	}
	if eff.EffectKind != "egress" {
		t.Fatalf("EffectKind = %q, want egress", eff.EffectKind)
	}
	if eff.Requests != 1 {
		t.Fatalf("Requests = %d, want 1", eff.Requests)
	}
}

// TestRecordingTransportNoReadAll: the RoundTripper does not buffer the whole
// body — a large streamed body returns promptly and the effect emits on Close.
func TestRecordingTransportNoReadAll(t *testing.T) {
	const n = 1 << 20 // 1 MiB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", n)))
	}))
	defer srv.Close()

	sink := &captureSink{}
	client := &http.Client{Transport: WrapRecording(http.DefaultTransport, sink)}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// RoundTrip returned before any body read — no effect should be emitted yet
	// (it emits on Close), proving the transport did not ReadAll the body.
	if sink.count() != 0 {
		t.Fatalf("effect emitted before body Close — transport buffered the body")
	}

	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if len(body) != n {
		t.Fatalf("body length = %d, want %d", len(body), n)
	}
	if sink.count() != 1 {
		t.Fatalf("expected 1 effect after Close, got %d", sink.count())
	}
	if got := sink.at(0).BytesIn; got != n {
		t.Fatalf("BytesIn = %d, want %d", got, n)
	}
}

// TestRecordingTransportEmitsOnEOFWithoutClose proves CR-01(a): a caller that
// reads the body to EOF but NEVER calls Close still emits exactly one effect
// (and the connection is recoverable). Before the fix the effect fired only on
// Close, so a missing Close silently dropped the egress row.
func TestRecordingTransportEmitsOnEOFWithoutClose(t *testing.T) {
	const payload = "hello world payload"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, payload)
	}))
	defer srv.Close()

	sink := &captureSink{}
	client := &http.Client{Transport: WrapRecording(http.DefaultTransport, sink)}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Read to EOF but deliberately DO NOT Close.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if sink.count() != 1 {
		t.Fatalf("expected exactly 1 effect after read-to-EOF (no Close), got %d", sink.count())
	}
	if got := sink.at(0).BytesIn; got != len(body) {
		t.Fatalf("BytesIn = %d, want %d (full body)", got, len(body))
	}
	if sink.at(0).Status != http.StatusOK {
		t.Fatalf("Status = %d, want 200", sink.at(0).Status)
	}
}

// TestRecordingTransportEmitsOnCloseWithoutFullRead proves CR-01(b): a caller
// that closes the body without reading it to EOF still emits exactly one
// effect.
func TestRecordingTransportEmitsOnCloseWithoutFullRead(t *testing.T) {
	const n = 1 << 20 // 1 MiB — large enough that we won't read it all.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", n)))
	}))
	defer srv.Close()

	sink := &captureSink{}
	client := &http.Client{Transport: WrapRecording(http.DefaultTransport, sink)}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Read only a few bytes, then Close without reaching EOF.
	buf := make([]byte, 8)
	_, _ = io.ReadFull(resp.Body, buf)
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if sink.count() != 1 {
		t.Fatalf("expected exactly 1 effect after Close (partial read), got %d", sink.count())
	}
}

// TestRecordingTransportReadToEOFThenCloseEmitsOnce proves CR-01(c): the common
// well-behaved path (read to EOF, THEN Close) emits exactly ONE effect, not two
// — the EOF emit and the Close emit must be guarded so they don't double-count.
func TestRecordingTransportReadToEOFThenCloseEmitsOnce(t *testing.T) {
	const payload = "hello world payload"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, payload)
	}))
	defer srv.Close()

	sink := &captureSink{}
	client := &http.Client{Transport: WrapRecording(http.DefaultTransport, sink)}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if sink.count() != 1 {
		t.Fatalf("expected exactly 1 effect after read-to-EOF-then-Close, got %d (double-emit)", sink.count())
	}
	if got := sink.at(0).BytesIn; got != len(body) {
		t.Fatalf("BytesIn = %d, want %d (full body)", got, len(body))
	}
}

// TestProducerDropOnFull: Record never blocks; overflow increments a dropped
// counter; a flush POSTs a JSON batch to a stub /internal/effects.
func TestProducerDropOnFull(t *testing.T) {
	var (
		mu       sync.Mutex
		gotPosts int
		gotBody  string
	)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotPosts++
		gotBody = string(b)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer stub.Close()

	p := NewProducer(ProducerConfig{
		AnalyticsURL:  stub.URL,
		BufferSize:    4,
		MaxBatch:      8,
		FlushInterval: 20 * time.Millisecond,
	})
	p.Start()

	// Flood far past the buffer; Record must never block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			p.Record(Effect{EffectKind: "egress", Host: "h", Status: 200})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Record blocked — producer is not non-blocking")
	}

	if p.Dropped() == 0 {
		t.Fatalf("expected some drops with a size-4 buffer and 1000 records")
	}

	p.Stop() // graceful drain + final flush

	mu.Lock()
	posts, body := gotPosts, gotBody
	mu.Unlock()
	if posts == 0 {
		t.Fatalf("producer never POSTed a batch to /internal/effects")
	}
	if !strings.Contains(body, `"effects"`) || !strings.Contains(body, `"egress"`) {
		t.Fatalf("POST body not the expected effect batch JSON: %q", body)
	}
}
