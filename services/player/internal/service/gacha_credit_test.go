package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// captureServer is a tiny httptest.Server that collects all POSTed bodies.
type captureServer struct {
	mu      sync.Mutex
	bodies  []creditMsg
	handler http.Handler
}

func newCaptureServer() *captureServer {
	s := &captureServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/gacha/credit", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var m creditMsg
		_ = json.Unmarshal(raw, &m)
		s.mu.Lock()
		s.bodies = append(s.bodies, m)
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"applied":true}`))
	})
	s.handler = mux
	return s
}

func (s *captureServer) Captured() []creditMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]creditMsg, len(s.bodies))
	copy(out, s.bodies)
	return out
}

func testLog(t *testing.T) *logger.Logger {
	t.Helper()
	l, err := logger.New(logger.Config{Level: "error", Development: false})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return l
}

// TestGachaCreditProducer_PayloadShape verifies that EpisodeWatched and
// TitleCompleted post the exact JSON payload expected by the gacha service.
func TestGachaCreditProducer_PayloadShape(t *testing.T) {
	cap := newCaptureServer()
	srv := httptest.NewServer(cap.handler)
	defer srv.Close()

	p := NewGachaCreditProducer(srv.URL, 22, 80, true, testLog(t))
	p.Start()

	p.EpisodeWatched("user-1", "anime-abc", 5)
	p.TitleCompleted("user-1", "anime-abc")

	// Drain by stopping (waits for worker to process all items).
	p.Stop()

	msgs := cap.Captured()
	if len(msgs) != 2 {
		t.Fatalf("want 2 captured messages, got %d", len(msgs))
	}

	// Episode message.
	ep := msgs[0]
	if ep.UserID != "user-1" {
		t.Errorf("episode UserID = %q; want %q", ep.UserID, "user-1")
	}
	if ep.Amount != 22 {
		t.Errorf("episode Amount = %d; want 22", ep.Amount)
	}
	if ep.Reason != "episode_watched" {
		t.Errorf("episode Reason = %q; want %q", ep.Reason, "episode_watched")
	}
	if ep.Ref != "anime-abc:5" {
		t.Errorf("episode Ref = %q; want %q", ep.Ref, "anime-abc:5")
	}

	// Title message.
	tc := msgs[1]
	if tc.UserID != "user-1" {
		t.Errorf("title UserID = %q; want %q", tc.UserID, "user-1")
	}
	if tc.Amount != 80 {
		t.Errorf("title Amount = %d; want 80", tc.Amount)
	}
	if tc.Reason != "title_completed" {
		t.Errorf("title Reason = %q; want %q", tc.Reason, "title_completed")
	}
	if tc.Ref != "anime-abc" {
		t.Errorf("title Ref = %q; want %q", tc.Ref, "anime-abc")
	}
}

// TestGachaCreditProducer_FloodDropDoesNotBlock verifies that sending 1000
// messages to a channel-full producer completes without blocking, even when
// the server is artificially slow. The send loop must finish well under the
// channel-cap + some buffer — the key invariant is "caller never blocks".
func TestGachaCreditProducer_FloodDropDoesNotBlock(t *testing.T) {
	// Server delays every response by 200ms to keep the channel backed up.
	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowSrv.Close()

	p := NewGachaCreditProducer(slowSrv.URL, 22, 80, true, testLog(t))
	p.Start()

	start := time.Now()
	for i := 0; i < 1000; i++ {
		p.EpisodeWatched("user-x", "anime-y", i)
	}
	elapsed := time.Since(start)

	// The 1000 non-blocking sends must complete in much less than 1s total.
	// With a 256-cap channel and drop-on-full, all 1000 calls take O(µs) each.
	if elapsed > 500*time.Millisecond {
		t.Errorf("flood send took %v; want < 500ms (caller must not block)", elapsed)
	}

	// Stop will drain remaining buffered items (≤256) with the slow server.
	// We set a generous timeout to avoid test flakiness on CI.
	done := make(chan struct{})
	go func() {
		p.Stop()
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(60 * time.Second):
		t.Error("Stop() did not return within 60s — possible goroutine leak")
	}
}

// TestGachaCreditProducer_StopDrainsPending confirms that Stop() waits for
// all buffered items to be processed before returning.
func TestGachaCreditProducer_StopDrainsPending(t *testing.T) {
	var mu sync.Mutex
	received := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	const n = 50
	p := NewGachaCreditProducer(srv.URL, 22, 80, true, testLog(t))
	p.Start()

	for i := 0; i < n; i++ {
		p.EpisodeWatched("user-drain", "anime-drain", i)
	}
	p.Stop()

	mu.Lock()
	got := received
	mu.Unlock()

	if got != n {
		t.Errorf("after Stop, received = %d; want %d (all buffered items drained)", got, n)
	}
}

// TestGachaCreditProducer_NilReceiverSafe verifies that calling methods on a
// nil *GachaCreditProducer does not panic.
func TestGachaCreditProducer_NilReceiverSafe(t *testing.T) {
	var p *GachaCreditProducer
	// None of these must panic.
	p.EpisodeWatched("u", "a", 1)
	p.TitleCompleted("u", "a")
	p.Start()
	p.Stop()
}

// TestGachaCreditProducer_DisabledIsNoop verifies that a producer constructed
// with enabled=false discards all messages without hitting the network.
func TestGachaCreditProducer_DisabledIsNoop(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewGachaCreditProducer(srv.URL, 22, 80, false, testLog(t))
	p.Start()
	p.EpisodeWatched("u", "a", 1)
	p.TitleCompleted("u", "a")
	p.Stop()

	if called {
		t.Error("disabled producer must not send HTTP requests")
	}
}
