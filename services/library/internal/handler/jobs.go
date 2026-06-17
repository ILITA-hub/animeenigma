package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/autocache"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/go-chi/chi/v5"
)

// JobStoreAPI is the slice of repo.JobRepository the handler needs.
// Defined here so jobs_test.go can inject a stub without spinning
// up a real Postgres.
//
// Phase 5 adds UpdateShikimoriID + Retry (re-enqueue).
type JobStoreAPI interface {
	Create(ctx context.Context, job *domain.Job) error
	GetByID(ctx context.Context, id string) (*domain.Job, error)
	List(ctx context.Context, f repo.JobFilter) ([]domain.Job, error)
	UpdateShikimoriID(ctx context.Context, id, shikimoriID string) error
	Retry(ctx context.Context, oldID string) (*domain.Job, error)
}

// MinioMover is the slice of *minio.Writer the Phase-5 Link handler
// needs. ListObjectsByPrefix is used to detect the existing episode
// number from `pending/{job_id}/{ep}/`; Move does the server-side
// CopyObject + RemoveObject sequence.
type MinioMover interface {
	ListObjectsByPrefix(ctx context.Context, prefix string) ([]string, error)
	Move(ctx context.Context, srcPrefix, dstPrefix string) error
}

// EpisodeStore is the slice of *repo.EpisodeRepository the Link
// handler needs.
type EpisodeStore interface {
	Create(ctx context.Context, ep *domain.Episode) error
}

// DiskGuardAPI is the slice of service.DiskGuard the handler needs.
type DiskGuardAPI interface {
	Allow(minFreePct int) (allowed bool, freePct int, err error)
}

// JobCanceller is the slice of service.WorkerPool the handler needs.
// CancelJob flips status FIRST then signals the in-memory handle —
// callers don't have to know that.
type JobCanceller interface {
	CancelJob(ctx context.Context, jobID string) error
}

// JobsHandler implements the admin-gated CRUD on /api/library/jobs.
// Auth is enforced at the gateway (gateway routes /api/library/*
// behind JWT + AdminMiddleware); the handler trusts what the gateway
// forwards.
//
// Phase 5: mover + episodeStore are nil in the legacy constructor
// `NewJobsHandler` (no Link/Retry handlers wired). Use
// `NewJobsHandlerWithLink` to opt into the Phase-5 Link + Retry paths.
type JobsHandler struct {
	jobRepo      JobStoreAPI
	diskGuard    DiskGuardAPI
	canceller    JobCanceller
	mover        MinioMover
	episodeStore EpisodeStore
	metrics      *metrics.LibraryMetrics
	minFreePct   int
	log          *logger.Logger
}

// NewJobsHandler constructs a JobsHandler.
func NewJobsHandler(
	jobRepo JobStoreAPI,
	diskGuard DiskGuardAPI,
	canceller JobCanceller,
	libMetrics *metrics.LibraryMetrics,
	minFreePct int,
	log *logger.Logger,
) *JobsHandler {
	return &JobsHandler{
		jobRepo:    jobRepo,
		diskGuard:  diskGuard,
		canceller:  canceller,
		metrics:    libMetrics,
		minFreePct: minFreePct,
		log:        log,
	}
}

// NewJobsHandlerWithLink is the Phase-5 constructor that adds the
// MinIO mover + EpisodeStore needed for the Link + Retry endpoints.
// Existing callers keep using NewJobsHandler if they only want the
// Phase-3 CRUD.
func NewJobsHandlerWithLink(
	jobRepo JobStoreAPI,
	diskGuard DiskGuardAPI,
	canceller JobCanceller,
	mover MinioMover,
	episodeStore EpisodeStore,
	libMetrics *metrics.LibraryMetrics,
	minFreePct int,
	log *logger.Logger,
) *JobsHandler {
	return &JobsHandler{
		jobRepo:      jobRepo,
		diskGuard:    diskGuard,
		canceller:    canceller,
		mover:        mover,
		episodeStore: episodeStore,
		metrics:      libMetrics,
		minFreePct:   minFreePct,
		log:          log,
	}
}

// createJobRequest is the POST body shape locked in 03-SPEC.md.
// Server fills id / status / timestamps via DB defaults.
type createJobRequest struct {
	Magnet      string `json:"magnet"`
	Title       string `json:"title"`
	Source      string `json:"source"`
	Uploader    string `json:"uploader,omitempty"`
	Quality     string `json:"quality,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	ShikimoriID string `json:"shikimori_id,omitempty"`
}

var allowedSources = map[domain.JobSource]bool{
	domain.JobSourceNyaa:       true,
	domain.JobSourceAnimeTosho: true,
	domain.JobSourceManual:     true,
	domain.JobSourceJackett:    true,
	domain.JobSourceAutocache:  true,
}

var allowedStatuses = map[domain.JobStatus]bool{
	domain.JobStatusQueued:      true,
	domain.JobStatusDownloading: true,
	domain.JobStatusEncoding:    true,
	domain.JobStatusUploading:   true,
	domain.JobStatusDone:        true,
	domain.JobStatusFailed:      true,
	domain.JobStatusCancelled:   true,
}

// Create handles POST /api/library/jobs.
//
// Validation order matters: cheap checks (body parse + required
// fields + source enum) before the expensive magnet parse, and the
// disk guard last so a 400 doesn't burn a Statfs syscall.
func (h *JobsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createJobRequest
	if err := httputil.Bind(r, &body); err != nil {
		httputil.Error(w, err)
		return
	}
	body.Magnet = strings.TrimSpace(body.Magnet)
	body.Title = strings.TrimSpace(body.Title)
	body.Source = strings.TrimSpace(body.Source)

	if body.Title == "" {
		httputil.BadRequest(w, "title is required")
		return
	}
	if body.Magnet == "" {
		httputil.BadRequest(w, "magnet is required")
		return
	}
	src := domain.JobSource(body.Source)
	if !allowedSources[src] {
		httputil.BadRequest(w, "source must be one of: nyaa, animetosho, manual")
		return
	}
	if _, err := metainfo.ParseMagnetUri(body.Magnet); err != nil {
		if h.metrics != nil {
			h.metrics.IncEnqueueRejected("invalid_magnet")
		}
		httputil.BadRequest(w, "invalid magnet")
		return
	}

	if h.diskGuard != nil {
		allowed, freePct, err := h.diskGuard.Allow(h.minFreePct)
		if err != nil {
			if h.log != nil {
				h.log.Warnw("disk guard check failed", "error", err)
			}
			// Don't 500 — log and let the worker handle it on dequeue.
		} else if !allowed {
			if h.metrics != nil {
				h.metrics.IncEnqueueRejected("disk_full")
			}
			if h.log != nil {
				h.log.Warnw("enqueue rejected: disk full",
					"free_pct", freePct, "min_required_pct", h.minFreePct,
				)
			}
			writeInsufficientStorage(w)
			return
		}
	}

	job := &domain.Job{
		Source:      src,
		Magnet:      body.Magnet,
		Title:       body.Title,
		Uploader:    body.Uploader,
		Quality:     body.Quality,
		SizeBytes:   body.SizeBytes,
		ShikimoriID: body.ShikimoriID,
		Status:      domain.JobStatusQueued,
	}
	if err := h.jobRepo.Create(r.Context(), job); err != nil {
		httputil.Error(w, err)
		return
	}
	if h.metrics != nil {
		h.metrics.IncJobsTotal(string(domain.JobStatusQueued))
	}
	httputil.Created(w, job)
}

// List handles GET /api/library/jobs?status=&limit=.
//
// status is comma-separated (e.g. "queued,downloading"). Unknown
// values → 400. limit defaults to 100, clamps 1..500.
func (h *JobsHandler) List(w http.ResponseWriter, r *http.Request) {
	statuses, err := parseStatusList(r.URL.Query().Get("status"))
	if err != nil {
		httputil.BadRequest(w, err.Error())
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"))

	jobs, err := h.jobRepo.List(r.Context(), repo.JobFilter{
		Statuses: statuses,
		Limit:    limit,
	})
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if jobs == nil {
		jobs = []domain.Job{}
	}
	httputil.OK(w, map[string]any{"jobs": jobs})
}

// Get handles GET /api/library/jobs/{id}.
func (h *JobsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	job, err := h.jobRepo.GetByID(r.Context(), id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, job)
}

// Delete handles DELETE /api/library/jobs/{id}.
//
// canceller.CancelJob flips status='cancelled' first, then signals
// the in-memory handle.Cancel() — the CONTEXT-locked order that keeps
// status consistent even if the in-memory cancel is lost.
func (h *JobsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	if h.canceller == nil {
		httputil.Error(w, liberrors.Internal("canceller not configured"))
		return
	}
	if err := h.canceller.CancelJob(r.Context(), id); err != nil {
		httputil.Error(w, err)
		return
	}
	if h.metrics != nil {
		h.metrics.IncJobsTotal(string(domain.JobStatusCancelled))
	}
	httputil.NoContent(w)
}

// parseStatusList — comma-separated, validated against the SQL enum.
func parseStatusList(raw string) ([]domain.JobStatus, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]domain.JobStatus, 0, len(parts))
	for _, p := range parts {
		s := domain.JobStatus(strings.TrimSpace(p))
		if !allowedStatuses[s] {
			return nil, &badStatusError{value: string(s)}
		}
		out = append(out, s)
	}
	return out, nil
}

type badStatusError struct{ value string }

func (e *badStatusError) Error() string { return "unknown status: " + e.value }

func parseLimit(raw string) int {
	limit := 100
	if raw == "" {
		return limit
	}
	if n, err := strconv.Atoi(raw); err == nil {
		limit = n
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	return limit
}

// writeInsufficientStorage emits HTTP 507 with the SPEC-locked body.
// httputil has no helper for 507 so we drive httputil.JSON directly.
func writeInsufficientStorage(w http.ResponseWriter) {
	httputil.JSON(w, http.StatusInsufficientStorage, map[string]string{"error": "disk_full"})
}

// linkJobRequest is the PATCH body shape. Only shikimori_id is
// supported in v0.2 — the resource model stays narrow on purpose.
type linkJobRequest struct {
	ShikimoriID string `json:"shikimori_id"`
}

// Link handles PATCH /api/library/jobs/{id}. Retroactively links a
// `done` job that lacks a shikimori_id to a catalog anime: parses the
// existing episode number out of the MinIO `pending/{job_id}/{ep}/`
// path, server-side-copies the HLS files to the canonical
// `{shikimori_id}/{ep}/` prefix, inserts the library_episodes row,
// and flips the job's shikimori_id column.
//
// Errors:
//   - 404: job not found
//   - 400: job not in `done` state OR already has shikimori_id OR no shikimori_id in body
//   - 500: minio objects missing OR move failure
//   - 409: episode already exists for that (shikimori_id, episode)
func (h *JobsHandler) Link(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	if h.mover == nil || h.episodeStore == nil {
		httputil.Error(w, liberrors.Internal("link handler not configured"))
		return
	}
	var body linkJobRequest
	if err := httputil.Bind(r, &body); err != nil {
		httputil.Error(w, err)
		return
	}
	body.ShikimoriID = strings.TrimSpace(body.ShikimoriID)
	if body.ShikimoriID == "" {
		httputil.BadRequest(w, "shikimori_id is required")
		return
	}

	job, err := h.jobRepo.GetByID(r.Context(), id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if job.Status != domain.JobStatusDone {
		httputil.BadRequest(w, "job must be done to link")
		return
	}
	if job.ShikimoriID != "" {
		httputil.BadRequest(w, "job already linked")
		return
	}

	srcPrefix := "pending/" + id + "/"
	keys, err := h.mover.ListObjectsByPrefix(r.Context(), srcPrefix)
	if err != nil {
		if h.log != nil {
			h.log.Warnw("list pending objects", "job_id", id, "error", err)
		}
		httputil.Error(w, liberrors.Internal("list pending objects"))
		return
	}
	if len(keys) == 0 {
		httputil.Error(w, liberrors.Internal("orphan job has no minio objects"))
		return
	}

	episodeNum, err := parseEpisodeFromPendingKey(keys[0], srcPrefix)
	if err != nil {
		if h.log != nil {
			h.log.Warnw("parse episode from minio path", "key", keys[0], "error", err)
		}
		httputil.Error(w, liberrors.Internal("could not parse episode number from minio path"))
		return
	}

	// Link a resolved episode into the unified autocache pool layout
	// (aeProvider/<mal>/RAW/<ep>/) so new admin content needs no migration.
	dstPrefix := autocache.RawPrefix(body.ShikimoriID, episodeNum)
	srcEpPrefix := srcPrefix + strconv.Itoa(episodeNum) + "/"
	if err := h.mover.Move(r.Context(), srcEpPrefix, dstPrefix); err != nil {
		if h.log != nil {
			h.log.Warnw("minio move", "src", srcEpPrefix, "dst", dstPrefix, "error", err)
		}
		httputil.Error(w, liberrors.Internal("minio move failed"))
		return
	}

	jobID := id
	ep := &domain.Episode{
		ShikimoriID:   body.ShikimoriID,
		EpisodeNumber: episodeNum,
		JobID:         &jobID,
		MinioPath:     dstPrefix,
	}
	if err := h.episodeStore.Create(r.Context(), ep); err != nil {
		httputil.Error(w, err)
		return
	}

	if err := h.jobRepo.UpdateShikimoriID(r.Context(), id, body.ShikimoriID); err != nil {
		httputil.Error(w, err)
		return
	}

	// Return the fresh row so the UI sees the updated shikimori_id.
	updated, err := h.jobRepo.GetByID(r.Context(), id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, updated)
}

// Retry handles POST /api/library/jobs/{id}/retry. Creates a NEW
// queued row that inherits the failed row's magnet + title + uploader
// + quality + size + shikimori_id. The old row stays in `failed` for
// audit; the new row's error_text is "retry of {oldID}".
//
// Errors:
//   - 404: job not found
//   - 400: job not in `failed` state
func (h *JobsHandler) Retry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	// Pre-flight status check so we emit a clean 400 (the repo's Retry
	// returns InvalidInput → 400 too, but the pre-flight gives a
	// SPEC-locked phrase).
	old, err := h.jobRepo.GetByID(r.Context(), id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if old.Status != domain.JobStatusFailed {
		httputil.BadRequest(w, "only failed jobs can be retried")
		return
	}
	fresh, err := h.jobRepo.Retry(r.Context(), id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if h.metrics != nil {
		h.metrics.IncJobsTotal(string(domain.JobStatusQueued))
	}
	httputil.Created(w, fresh)
}

// parseEpisodeFromPendingKey extracts the {ep} integer directory from
// a MinIO object key under the `pending/{job_id}/{ep}/...` path
// shape. Returns an error if the key doesn't start with srcPrefix or
// the first path segment isn't a positive integer.
func parseEpisodeFromPendingKey(key, srcPrefix string) (int, error) {
	tail := strings.TrimPrefix(key, srcPrefix)
	if tail == key {
		// TrimPrefix didn't match → key not under srcPrefix.
		return 0, &badStatusError{value: "key does not match prefix"}
	}
	// First path segment is the episode number.
	idx := strings.Index(tail, "/")
	if idx <= 0 {
		return 0, &badStatusError{value: "missing episode segment"}
	}
	n, err := strconv.Atoi(tail[:idx])
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, &badStatusError{value: "non-positive episode"}
	}
	return n, nil
}
