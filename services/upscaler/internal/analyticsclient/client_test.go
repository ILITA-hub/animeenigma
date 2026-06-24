package analyticsclient

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestClient_SendEnqueues verifies that Send enqueues rows and they are
// delivered to the analytics stub via a batched POST.
func TestClient_SendEnqueues(t *testing.T) {
	var received []UpscaleTelemetryRow
	var mu sync.Mutex
	posted := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/upscale-telemetry" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var rows []UpscaleTelemetryRow
		if err := json.Unmarshal(body, &rows); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		mu.Lock()
		received = append(received, rows...)
		mu.Unlock()
		select {
		case posted <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := New(srv.URL, Config{
		BufferSize:    16,
		BatchSize:     2,
		FlushInterval: 50 * time.Millisecond,
	})
	c.Start()
	defer c.Stop()

	now := time.Now().UTC()
	c.Send(UpscaleTelemetryRow{
		TS: now, WorkerID: "w-1", GPUModel: "RTX 4090", ImageVersion: "v1.0",
		JobID: "job-1", SegmentIdx: 0, GPUUtil: 85.5,
		VRAMUsedB: 8e9, VRAMTotalB: 24e9,
		GPUTempC: 72.3, GPUPowerW: 350.0,
		DecodeFPS: 120.5, InferenceFPS: 30.2, EncodeFPS: 118.0,
	})
	c.Send(UpscaleTelemetryRow{
		TS: now.Add(time.Second), WorkerID: "w-1", GPUModel: "RTX 4090", ImageVersion: "v1.0",
		JobID: "job-1", SegmentIdx: 1, GPUUtil: 88.0,
		VRAMUsedB: 9e9, VRAMTotalB: 24e9,
		GPUTempC: 75.0, GPUPowerW: 360.0,
		DecodeFPS: 122.0, InferenceFPS: 31.0, EncodeFPS: 120.0,
	})

	// Wait for the batch to be POSTed (batch size=2 triggers immediately).
	select {
	case <-posted:
	case <-time.After(2 * time.Second):
		t.Fatal("analytics stub never received a POST within 2s")
	}

	mu.Lock()
	n := len(received)
	mu.Unlock()
	if n < 2 {
		t.Fatalf("expected at least 2 rows delivered, got %d", n)
	}
}

// TestClient_BufferFullDrop verifies that when the buffer is full, Send drops
// the row without blocking. This is the fire-and-forget / drop-on-full contract.
func TestClient_BufferFullDrop(t *testing.T) {
	// Create a client with NO Start() — the drain goroutine is not running,
	// so the channel is never consumed and fills immediately.
	c := &Client{
		url:     "http://127.0.0.1:1/internal/upscale-telemetry", // unreachable
		cfg:     Config{BufferSize: 2, BatchSize: 100, FlushInterval: time.Hour},
		ch:      make(chan UpscaleTelemetryRow, 2),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
	// Do NOT call c.Start() — the drain goroutine is intentionally absent so
	// the channel is never consumed, proving the drop-on-full path.

	now := time.Now().UTC()
	row := UpscaleTelemetryRow{TS: now, WorkerID: "w-drop", GPUModel: "GPU"}

	// Fill the buffer (BufferSize=2) and then send more — all must return
	// immediately without blocking (drop-on-full).
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			c.Send(row) // must never block, even on full buffer
		}
		close(done)
	}()

	select {
	case <-done:
		// All 20 Send calls returned — no blocking.
	case <-time.After(2 * time.Second):
		t.Fatal("Send blocked on full buffer (should drop and return)")
	}
}

// TestClient_AnalyticsOutageSwallowed verifies that a 5xx / network error
// from the analytics endpoint is logged and swallowed — never propagated to
// the caller or the read pump goroutine.
func TestClient_AnalyticsOutageSwallowed(t *testing.T) {
	var postCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		postCount.Add(1)
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, Config{
		BufferSize:    16,
		BatchSize:     1,
		FlushInterval: 50 * time.Millisecond,
	})
	c.Start()
	defer c.Stop()

	// This must not panic or block even though the analytics server returns 5xx.
	c.Send(UpscaleTelemetryRow{TS: time.Now(), WorkerID: "w-err", GPUModel: "GPU"})

	// Give drain goroutine time to POST and swallow the error.
	time.Sleep(200 * time.Millisecond)

	// The POST reached the stub (proving the drain ran) but nothing panicked.
	if postCount.Load() == 0 {
		t.Fatal("expected at least one POST to analytics stub")
	}
}

// TestClient_Unreachable verifies that an unreachable analytics endpoint is
// swallowed — the drain goroutine continues without crashing.
func TestClient_Unreachable(t *testing.T) {
	// Port 1 is always refused.
	c := New("http://127.0.0.1:1", Config{
		BufferSize:    8,
		BatchSize:     1,
		FlushInterval: 50 * time.Millisecond,
	})
	c.Start()
	defer c.Stop()

	// Must not block or panic even though no server is listening.
	done := make(chan struct{})
	go func() {
		c.Send(UpscaleTelemetryRow{TS: time.Now(), WorkerID: "w-unreach", GPUModel: "GPU"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Send blocked on unreachable analytics endpoint")
	}

	// Let the drain attempt + swallow the error.
	time.Sleep(300 * time.Millisecond)
	// No panic = pass.
}

// TestClient_NilSend verifies that a nil client's Send is a safe no-op.
func TestClient_NilSend(t *testing.T) {
	var c *Client
	// Must not panic.
	c.Send(UpscaleTelemetryRow{TS: time.Now(), WorkerID: "w", GPUModel: "GPU"})
}
