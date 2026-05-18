package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
)

// DiskCheckProbe is the slice of service.DiskGuard the extended
// health endpoint consumes. Returns (freeBytes, totalBytes, freePct, err).
// uint64 mirrors statfs(2) bavail / blocks; the JSON payload narrows
// to int64 (sufficient up to 8 EiB).
type DiskCheckProbe interface {
	Check() (free uint64, total uint64, freePct int, err error)
}

// TorrentCounter is the slice of service.WorkerPool.ActiveCount.
type TorrentCounter interface {
	ActiveCount() int
}

// JobLister is the slice of repo.JobRepository.List used by the
// extended health endpoint. Same shape as JobStoreAPI.List but kept
// distinct so callers can wire just the lister without exposing
// Create / GetByID.
type JobLister interface {
	List(ctx context.Context, f repo.JobFilter) ([]domain.Job, error)
}

// HealthHandler responds to GET /health with a flat {status: "ok"} payload
// wrapped in the standard httputil envelope ({success, data}). Phase 5
// adds the /health/extended endpoint (LIB-09) that returns
// {disk_free_bytes, disk_total_bytes, active_torrents, active_jobs_by_status}.
// All Phase-5 deps are optional — when nil HealthExtended degrades to a
// minimal payload rather than 500.
type HealthHandler struct {
	disk    DiskCheckProbe
	counter TorrentCounter
	lister  JobLister
}

// NewHealthHandler constructs a HealthHandler with no dependencies
// (legacy /health path).
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// NewHealthHandlerExtended constructs a HealthHandler wired for the
// /health/extended endpoint. Dependencies may be nil — the handler
// degrades to a minimal payload rather than 500.
func NewHealthHandlerExtended(disk DiskCheckProbe, counter TorrentCounter, lister JobLister) *HealthHandler {
	return &HealthHandler{disk: disk, counter: counter, lister: lister}
}

// Health responds 200 with {"status":"ok"} (wrapped in httputil's envelope).
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	httputil.OK(w, map[string]string{"status": "ok"})
}

// healthExtendedResponse is the locked Phase-5 SPEC body shape.
type healthExtendedResponse struct {
	DiskFreeBytes      int64          `json:"disk_free_bytes"`
	DiskTotalBytes     int64          `json:"disk_total_bytes"`
	ActiveTorrents     int            `json:"active_torrents"`
	ActiveJobsByStatus map[string]int `json:"active_jobs_by_status"`
}

// HealthExtended responds 200 with the extended health payload — disk
// free + total, active in-memory torrents, and a per-status count of
// active (non-terminal) jobs. Used by the Phase-5 admin UI stats strip
// on a 30s poll cycle.
//
// Disk-probe error → 500. Lister error → log and continue with zeroed
// counts (less critical than disk).
func (h *HealthHandler) HealthExtended(w http.ResponseWriter, r *http.Request) {
	resp := healthExtendedResponse{
		ActiveJobsByStatus: map[string]int{
			"queued":      0,
			"downloading": 0,
			"encoding":    0,
			"uploading":   0,
		},
	}

	if h.disk != nil {
		free, total, _, err := h.disk.Check()
		if err != nil {
			http.Error(w, "disk check failed", http.StatusInternalServerError)
			return
		}
		// Narrow uint64 → int64 for JSON output (sufficient up to 8 EiB).
		resp.DiskFreeBytes = int64(free)
		resp.DiskTotalBytes = int64(total)
	}

	if h.counter != nil {
		resp.ActiveTorrents = h.counter.ActiveCount()
	}

	if h.lister != nil {
		jobs, err := h.lister.List(r.Context(), repo.JobFilter{
			Statuses: []domain.JobStatus{
				domain.JobStatusQueued,
				domain.JobStatusDownloading,
				domain.JobStatusEncoding,
				domain.JobStatusUploading,
			},
			Limit: 500,
		})
		if err == nil {
			for _, j := range jobs {
				resp.ActiveJobsByStatus[string(j.Status)]++
			}
		}
	}

	httputil.OK(w, resp)
}
