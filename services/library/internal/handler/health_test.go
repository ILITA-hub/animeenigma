package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
)

// fakeDiskProbe stubs the disk-free probe for HealthExtended.
type fakeDiskProbe struct {
	free, total uint64
	freePct     int
	err         error
}

func (f *fakeDiskProbe) Check() (uint64, uint64, int, error) {
	return f.free, f.total, f.freePct, f.err
}

// fakeTorrentCounter stubs ActiveCount.
type fakeTorrentCounter struct{ n int }

func (f *fakeTorrentCounter) ActiveCount() int { return f.n }

// fakeJobLister stubs List() for HealthExtended.
type fakeJobLister struct {
	jobs []domain.Job
	err  error
}

func (f *fakeJobLister) List(_ context.Context, _ repo.JobFilter) ([]domain.Job, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.jobs, nil
}

// Test the legacy /health endpoint still returns ok.
func TestHealthHandler_Health_StatusOK(t *testing.T) {
	h := NewHealthHandler()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// HealthExtended happy path: returns disk + active torrents + per-status counts.
func TestHealthHandler_HealthExtended_Happy(t *testing.T) {
	disk := &fakeDiskProbe{free: 500, total: 1000, freePct: 50}
	tc := &fakeTorrentCounter{n: 3}
	lister := &fakeJobLister{jobs: []domain.Job{
		{ID: "1", Status: domain.JobStatusQueued},
		{ID: "2", Status: domain.JobStatusQueued},
		{ID: "3", Status: domain.JobStatusDownloading},
		{ID: "4", Status: domain.JobStatusEncoding},
		{ID: "5", Status: domain.JobStatusUploading},
	}}
	h := NewHealthHandlerExtended(disk, tc, lister)

	r := httptest.NewRequest(http.MethodGet, "/health/extended", nil)
	w := httptest.NewRecorder()
	h.HealthExtended(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			DiskFreeBytes      int64          `json:"disk_free_bytes"`
			DiskTotalBytes     int64          `json:"disk_total_bytes"`
			ActiveTorrents     int            `json:"active_torrents"`
			ActiveJobsByStatus map[string]int `json:"active_jobs_by_status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, w.Body.String())
	}
	if resp.Data.DiskFreeBytes != 500 {
		t.Errorf("disk_free_bytes = %d, want 500", resp.Data.DiskFreeBytes)
	}
	if resp.Data.DiskTotalBytes != 1000 {
		t.Errorf("disk_total_bytes = %d, want 1000", resp.Data.DiskTotalBytes)
	}
	if resp.Data.ActiveTorrents != 3 {
		t.Errorf("active_torrents = %d, want 3", resp.Data.ActiveTorrents)
	}
	expected := map[string]int{
		"queued": 2, "downloading": 1, "encoding": 1, "uploading": 1,
	}
	for k, v := range expected {
		if resp.Data.ActiveJobsByStatus[k] != v {
			t.Errorf("active_jobs_by_status[%q] = %d, want %d", k, resp.Data.ActiveJobsByStatus[k], v)
		}
	}
}

// HealthExtended with a disk probe error returns 500.
func TestHealthHandler_HealthExtended_DiskError(t *testing.T) {
	disk := &fakeDiskProbe{err: errors.New("statfs failed")}
	tc := &fakeTorrentCounter{n: 0}
	lister := &fakeJobLister{}
	h := NewHealthHandlerExtended(disk, tc, lister)

	r := httptest.NewRequest(http.MethodGet, "/health/extended", nil)
	w := httptest.NewRecorder()
	h.HealthExtended(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// HealthExtended with empty job list returns zeroes for all statuses.
func TestHealthHandler_HealthExtended_EmptyJobs(t *testing.T) {
	disk := &fakeDiskProbe{free: 100, total: 200, freePct: 50}
	tc := &fakeTorrentCounter{n: 0}
	lister := &fakeJobLister{jobs: []domain.Job{}}
	h := NewHealthHandlerExtended(disk, tc, lister)

	r := httptest.NewRequest(http.MethodGet, "/health/extended", nil)
	w := httptest.NewRecorder()
	h.HealthExtended(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			ActiveJobsByStatus map[string]int `json:"active_jobs_by_status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, k := range []string{"queued", "downloading", "encoding", "uploading"} {
		if resp.Data.ActiveJobsByStatus[k] != 0 {
			t.Errorf("active_jobs_by_status[%q] = %d, want 0", k, resp.Data.ActiveJobsByStatus[k])
		}
	}
}
