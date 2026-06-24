// Package analyticsclient is a non-blocking, drop-on-full telemetry producer
// that forwards per-worker GPU telemetry rows to the analytics service
// (POST /internal/upscale-telemetry). It mirrors the drop-on-full discipline
// of libs/tracing.Producer while being a small dedicated client rather than
// reusing the effects-row pipeline (CD-15).
//
// An analytics outage (5xx, network error, unreachable) is logged and swallowed;
// it MUST NEVER block the WS read pump or propagate back to a caller.
package analyticsclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// UpscaleTelemetryRow is one per-worker GPU telemetry sample. JSON tags are
// BYTE-IDENTICAL to the analytics-side repo.UpscaleTelemetryRow, in DDL
// column order (CD-15 parity contract).
type UpscaleTelemetryRow struct {
	TS           time.Time `json:"ts"`
	WorkerID     string    `json:"worker_id"`
	GPUModel     string    `json:"gpu_model"`
	ImageVersion string    `json:"image_version"`
	JobID        string    `json:"job_id"`
	SegmentIdx   int32     `json:"segment_idx"`
	GPUUtil      float32   `json:"gpu_util"`
	VRAMUsedB    uint64    `json:"vram_used_b"`
	VRAMTotalB   uint64    `json:"vram_total_b"`
	GPUTempC     float32   `json:"gpu_temp_c"`
	GPUPowerW    float32   `json:"gpu_power_w"`
	DecodeFPS    float32   `json:"decode_fps"`
	InferenceFPS float32   `json:"inference_fps"`
	EncodeFPS    float32   `json:"encode_fps"`
}

// Config holds tuning knobs for the Client. The zero value is invalid; use
// DefaultConfig() or pass explicit values.
type Config struct {
	// BufferSize is the capacity of the in-process channel. When full, Send
	// drops the row (drop-on-full, non-blocking).
	BufferSize int

	// BatchSize is the number of rows that trigger an immediate POST. The
	// drain goroutine also flushes on FlushInterval to bound latency.
	BatchSize int

	// FlushInterval is the maximum time between flushes even when BatchSize
	// is not reached.
	FlushInterval time.Duration
}

// DefaultConfig returns the production-suitable defaults.
func DefaultConfig() Config {
	return Config{
		BufferSize:    256,
		BatchSize:     20,
		FlushInterval: 5 * time.Second,
	}
}

// Client is the analyticsclient entry point. Construct with New, call Start
// before first use, and Stop on shutdown. A nil *Client is safe to call Send
// on (no-op).
type Client struct {
	url     string
	cfg     Config
	ch      chan UpscaleTelemetryRow
	stop    chan struct{}
	stopped chan struct{}
	http    *http.Client

	// dropCount tracks how many rows were dropped due to a full buffer. It is
	// accessible only via internal log-path; no external accessor needed.
	dropCount uint64
}

// New constructs a Client that will POST to analyticsURL. Call Start() before
// sending rows. The URL should be the ANALYTICS_INTERNAL_URL base
// (e.g. "http://analytics:8092").
func New(analyticsURL string, cfg Config) *Client {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = DefaultConfig().BufferSize
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultConfig().BatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = DefaultConfig().FlushInterval
	}
	return &Client{
		url:     analyticsURL + "/internal/upscale-telemetry",
		cfg:     cfg,
		ch:      make(chan UpscaleTelemetryRow, cfg.BufferSize),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Start launches the background drain goroutine. Must be called once before
// any Send calls.
func (c *Client) Start() {
	go c.drain()
}

// Stop signals the drain goroutine to flush any pending rows and exit. It
// blocks until the goroutine has stopped (or 10s elapses).
func (c *Client) Stop() {
	close(c.stop)
	select {
	case <-c.stopped:
	case <-time.After(10 * time.Second):
	}
}

// Send enqueues row for async delivery. If the internal buffer is full the
// row is dropped (drop-on-full) and a warning is noted internally — the call
// returns immediately in all cases. Send is safe to call concurrently.
//
// A nil *Client is a safe no-op.
func (c *Client) Send(row UpscaleTelemetryRow) {
	if c == nil {
		return
	}
	select {
	case c.ch <- row:
	default:
		// Buffer full — drop the row. An analytics outage or slow consumer
		// must never block the WS read pump.
		c.dropCount++
	}
}

// drain is the single background goroutine that batches rows and POSTs them.
// It exits when Stop is called, flushing any remaining rows first.
func (c *Client) drain() {
	defer close(c.stopped)

	ticker := time.NewTicker(c.cfg.FlushInterval)
	defer ticker.Stop()

	buf := make([]UpscaleTelemetryRow, 0, c.cfg.BatchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		c.post(buf)
		buf = buf[:0]
	}

	for {
		select {
		case row := <-c.ch:
			buf = append(buf, row)
			if len(buf) >= c.cfg.BatchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-c.stop:
			// Drain any remaining rows in the channel before exiting.
			for {
				select {
				case row := <-c.ch:
					buf = append(buf, row)
				default:
					flush()
					return
				}
			}
		}
	}
}

// post marshals rows as a JSON array and POSTs them to the analytics endpoint.
// Any error (network failure, 5xx, etc.) is logged to stderr and swallowed —
// it must never be propagated to the caller.
func (c *Client) post(rows []UpscaleTelemetryRow) {
	body, err := json.Marshal(rows)
	if err != nil {
		// Marshalling our own struct should never fail; log defensively.
		fmt.Printf("[analyticsclient] WARN marshal error: %v\n", err)
		return
	}

	resp, err := c.http.Post(c.url, "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		// Network failure — swallow. The analytics service may be temporarily
		// unavailable; the upscaler's job pipeline must not be affected.
		fmt.Printf("[analyticsclient] WARN POST failed (rows dropped): %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		fmt.Printf("[analyticsclient] WARN analytics returned %d (rows dropped)\n", resp.StatusCode)
	}
}
