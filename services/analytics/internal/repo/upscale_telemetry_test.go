//go:build integration

package repo

import (
	"context"
	"testing"
	"time"
)

// TestUpscaleTelemetry_EnsureSchema verifies that EnsureSchema idempotently
// creates the upscale_worker_telemetry table (can be called multiple times
// without error).
func TestUpscaleTelemetry_EnsureSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker (ClickHouse testcontainer)")
	}
	conn := newCHContainerConn(t)
	ctx := context.Background()

	// EnsureSchema was already called in newCHContainerConn. Call it again to
	// prove idempotency — IF NOT EXISTS makes re-runs a no-op.
	if err := EnsureSchema(ctx, conn); err != nil {
		t.Fatalf("EnsureSchema idempotent re-run failed: %v", err)
	}

	// The table must exist: query it (empty result is fine).
	row := conn.QueryRow(ctx, "SELECT count() FROM upscale_worker_telemetry")
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatalf("upscale_worker_telemetry table not found after EnsureSchema: %v", err)
	}
}

// TestInsertUpscaleTelemetry verifies batch insert and round-trip read.
func TestInsertUpscaleTelemetry(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker (ClickHouse testcontainer)")
	}
	conn := newCHContainerConn(t)
	ctx := context.Background()
	store := NewClickHouseStore(conn)

	now := time.Now().UTC().Truncate(time.Millisecond)
	rows := []UpscaleTelemetryRow{
		{
			TS:           now,
			WorkerID:     "worker-1",
			GPUModel:     "RTX 4090",
			ImageVersion: "v1.0",
			JobID:        "job-aaa",
			SegmentIdx:   0,
			GPUUtil:      85.5,
			VRAMUsedB:    8_000_000_000,
			VRAMTotalB:   24_000_000_000,
			GPUTempC:     72.3,
			GPUPowerW:    350.0,
			DecodeFPS:    120.5,
			InferenceFPS: 30.2,
			EncodeFPS:    118.0,
		},
		{
			TS:           now.Add(time.Second),
			WorkerID:     "worker-2",
			GPUModel:     "RTX 3080",
			ImageVersion: "v1.0",
			JobID:        "job-bbb",
			SegmentIdx:   1,
			GPUUtil:      70.0,
			VRAMUsedB:    6_000_000_000,
			VRAMTotalB:   10_000_000_000,
			GPUTempC:     65.0,
			GPUPowerW:    280.0,
			DecodeFPS:    90.0,
			InferenceFPS: 22.0,
			EncodeFPS:    88.0,
		},
		{
			TS:           now.Add(2 * time.Second),
			WorkerID:     "worker-1",
			GPUModel:     "RTX 4090",
			ImageVersion: "v1.1",
			JobID:        "job-aaa",
			SegmentIdx:   2,
			GPUUtil:      90.0,
			VRAMUsedB:    9_000_000_000,
			VRAMTotalB:   24_000_000_000,
			GPUTempC:     78.0,
			GPUPowerW:    370.0,
			DecodeFPS:    125.0,
			InferenceFPS: 31.0,
			EncodeFPS:    122.0,
		},
	}

	if err := store.InsertUpscaleTelemetry(ctx, rows); err != nil {
		t.Fatalf("InsertUpscaleTelemetry: %v", err)
	}

	// Verify count == 3.
	var count uint64
	row := conn.QueryRow(ctx, "SELECT count() FROM upscale_worker_telemetry")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 rows, got %d", count)
	}

	// Round-trip: SELECT worker_id, gpu_util for the first row and verify values.
	qrows, err := conn.Query(ctx,
		"SELECT worker_id, gpu_util FROM upscale_worker_telemetry WHERE worker_id = ? ORDER BY ts LIMIT 1",
		"worker-1")
	if err != nil {
		t.Fatalf("select worker_id/gpu_util: %v", err)
	}
	defer qrows.Close()

	if !qrows.Next() {
		t.Fatal("expected at least one row for worker-1")
	}
	var workerID string
	var gpuUtil float32
	if err := qrows.Scan(&workerID, &gpuUtil); err != nil {
		t.Fatalf("scan worker_id/gpu_util: %v", err)
	}
	if workerID != "worker-1" {
		t.Errorf("worker_id: got %q, want %q", workerID, "worker-1")
	}
	// Float32 precision: allow small delta.
	if gpuUtil < 85.0 || gpuUtil > 86.0 {
		t.Errorf("gpu_util: got %v, want ~85.5", gpuUtil)
	}
	if err := qrows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
}

// TestInsertUpscaleTelemetry_Empty verifies empty batch is a no-op.
func TestInsertUpscaleTelemetry_Empty(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker (ClickHouse testcontainer)")
	}
	conn := newCHContainerConn(t)
	ctx := context.Background()
	store := NewClickHouseStore(conn)

	if err := store.InsertUpscaleTelemetry(ctx, nil); err != nil {
		t.Fatalf("empty batch must be a no-op, got %v", err)
	}
	var count uint64
	row := conn.QueryRow(ctx, "SELECT count() FROM upscale_worker_telemetry")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after empty batch, got %d", count)
	}
}
