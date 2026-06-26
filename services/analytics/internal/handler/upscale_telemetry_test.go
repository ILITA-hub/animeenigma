package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

// fakeUpscaleStore captures InsertUpscaleTelemetry calls for assertions.
type fakeUpscaleStore struct {
	mu   sync.Mutex
	rows []repo.UpscaleTelemetryRow
	err  error
}

func (f *fakeUpscaleStore) InsertUpscaleTelemetry(_ context.Context, rows []repo.UpscaleTelemetryRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, rows...)
	return f.err
}

func (f *fakeUpscaleStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}

func (f *fakeUpscaleStore) at(i int) repo.UpscaleTelemetryRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rows[i]
}

// TestUpscaleTelemetryHandler_Ingest: a JSON batch → 202, rows captured by store.
func TestUpscaleTelemetryHandler_Ingest(t *testing.T) {
	store := &fakeUpscaleStore{}
	h := NewUpscaleTelemetryHandler(store)

	now := time.Now().UTC().Truncate(time.Millisecond)
	batch := []repo.UpscaleTelemetryRow{
		{
			TS:           now,
			WorkerID:     "worker-1",
			GPUModel:     "RTX 4090",
			ImageVersion: "v1.0",
			JobID:        "job-abc",
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
			JobID:        "",
			SegmentIdx:   -1,
			GPUUtil:      70.0,
			VRAMUsedB:    6_000_000_000,
			VRAMTotalB:   10_000_000_000,
			GPUTempC:     65.0,
			GPUPowerW:    280.0,
			DecodeFPS:    90.0,
			InferenceFPS: 22.0,
			EncodeFPS:    88.0,
		},
	}
	body, _ := json.Marshal(batch)

	req := httptest.NewRequest(http.MethodPost, "/internal/upscale-telemetry", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if store.count() != 2 {
		t.Fatalf("expected 2 rows enqueued, got %d", store.count())
	}
	r0 := store.at(0)
	if r0.WorkerID != "worker-1" {
		t.Errorf("worker_id: got %q, want worker-1", r0.WorkerID)
	}
	if r0.GPUModel != "RTX 4090" {
		t.Errorf("gpu_model: got %q, want RTX 4090", r0.GPUModel)
	}
	if r0.JobID != "job-abc" {
		t.Errorf("job_id: got %q, want job-abc", r0.JobID)
	}
}

// TestUpscaleTelemetryHandler_MalformedBody: bad JSON → 400.
func TestUpscaleTelemetryHandler_MalformedBody(t *testing.T) {
	h := NewUpscaleTelemetryHandler(&fakeUpscaleStore{})
	req := httptest.NewRequest(http.MethodPost, "/internal/upscale-telemetry", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on malformed body, got %d", rec.Code)
	}
}

// TestUpscaleTelemetryHandler_EmptyBatch: empty array → 202, zero rows.
func TestUpscaleTelemetryHandler_EmptyBatch(t *testing.T) {
	store := &fakeUpscaleStore{}
	h := NewUpscaleTelemetryHandler(store)
	req := httptest.NewRequest(http.MethodPost, "/internal/upscale-telemetry", strings.NewReader("[]"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 on empty batch, got %d", rec.Code)
	}
	if store.count() != 0 {
		t.Fatalf("expected 0 rows for empty batch, got %d", store.count())
	}
}

