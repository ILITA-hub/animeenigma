package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// MaxUploadBytes is the PUT body cap (~4× a 2 min / 45 s segment at ~8 Mbps after upscale).
// The original segment is ≤ segment_seconds × ~4 MB/s ≈ 180 MB; 4× = 720 MB.
// We use 750 MB as a round cap to guard against DoS on the shared upscale_staging volume.
const MaxUploadBytes = 750 * 1024 * 1024 // 750 MiB

// jobGetter is the minimal JobRepository surface needed by SegmentHandler.
type jobGetter interface {
	Get(ctx context.Context, id string) (*domain.UpscaleJob, error)
}

// segmentGetter is the minimal SegmentRepository surface needed by SegmentHandler.
type segmentGetter interface {
	Get(ctx context.Context, jobID string, idx int) (*domain.UpscaleSegment, error)
	MarkDone(ctx context.Context, jobID string, idx int, outBytes int64) error
}

// SegmentHandler implements GET and PUT /worker/segments/{job}/{idx}.
type SegmentHandler struct {
	stagingRoot string
	jobs        jobGetter
	segs        segmentGetter
	log         *logger.Logger
}

// NewSegmentHandler constructs a SegmentHandler.
// stagingRoot is config.Upscaler.StagingDir.
func NewSegmentHandler(stagingRoot string, jobs jobGetter, segs segmentGetter, log *logger.Logger) *SegmentHandler {
	if log == nil {
		log = logger.Default()
	}
	return &SegmentHandler{
		stagingRoot: stagingRoot,
		jobs:        jobs,
		segs:        segs,
		log:         log,
	}
}

// writeUnauthorized writes a generic 401 with no internal detail (security req #7).
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"}) //nolint:errcheck
}

// writeBadRequest writes a generic 400 with no internal detail.
func writeBadRequest(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": "bad request"}) //nolint:errcheck
}

// writeConflict writes a generic 409 with no internal detail.
func writeConflict(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	json.NewEncoder(w).Encode(map[string]string{"error": "conflict"}) //nolint:errcheck
}

// writeInternalError writes a generic 500 with no internal detail.
func writeInternalError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"error": "internal error"}) //nolint:errcheck
}

// verifyAndResolve is the shared preamble for GET and PUT:
//  1. Verify capability (security req #1).
//  2. Parse and bound-check idx (security req #2).
//  3. Build and validate paths (security req #3).
//  4. Confirm lease ownership (security req #4).
//
// Returns (idx, inputPath, false, nil) on success.
// On rejection, it writes the error response and returns (0, "", true, nil).
func (h *SegmentHandler) verifyAndResolve(
	w http.ResponseWriter,
	r *http.Request,
	operation string,
) (idx int, inputPath string, job *domain.UpscaleJob, rejected bool) {
	jobID := chi.URLParam(r, "job")
	idxStr := chi.URLParam(r, "idx")

	// ── Security req #2: parse idx ─────────────────────────────────────────
	idxVal, err := strconv.Atoi(idxStr)
	if err != nil || idxVal < 0 {
		writeBadRequest(w)
		return 0, "", nil, true
	}

	// ── Security req #1: verify capability ────────────────────────────────
	exp := r.URL.Query().Get("exp")
	sig := r.URL.Query().Get("sig")
	if !capability.VerifyJobHandle(jobID, operation, idxVal, exp, sig, time.Now()) {
		writeUnauthorized(w)
		return 0, "", nil, true
	}

	// ── Security req #2: bound-check against job.SegmentCount ─────────────
	job, err = h.jobs.Get(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeBadRequest(w)
		} else {
			h.log.Errorw("segments: job lookup failed", "job_id", jobID, "error", err)
			writeInternalError(w)
		}
		return 0, "", nil, true
	}
	if idxVal >= job.SegmentCount { // idxVal < 0 already rejected at parse above
		writeBadRequest(w)
		return 0, "", nil, true
	}

	// ── Security req #3: path traversal defense ───────────────────────────
	// jobID comes from the verified capability (HMAC-bound), so it cannot be
	// an attacker-supplied string unless they break HMAC. We still apply
	// filepath.Clean + prefix assertions as defense-in-depth. Two checks:
	//   (a) the per-job dir stays inside stagingRoot (catches a forged jobID
	//       containing "../" that would escape the staging tree), and
	//   (b) the final file path stays inside the per-job dir.
	segName := fmt.Sprintf("seg_%05d.mkv", idxVal) // idxVal>=0 here → clean filename
	rootClean := filepath.Clean(h.stagingRoot)
	jobDir := filepath.Clean(filepath.Join(rootClean, jobID))
	if jobDir != filepath.Join(rootClean, jobID) || !strings.HasPrefix(jobDir, rootClean+string(filepath.Separator)) {
		h.log.Warnw("segments: jobID escapes staging root", "job_id", jobID, "idx", idxVal)
		writeBadRequest(w)
		return 0, "", nil, true
	}
	p := filepath.Clean(filepath.Join(jobDir, segName))
	if !strings.HasPrefix(p, jobDir+string(filepath.Separator)) {
		h.log.Warnw("segments: path traversal detected", "job_id", jobID, "idx", idxVal)
		writeBadRequest(w)
		return 0, "", nil, true
	}

	return idxVal, p, job, false
}

// GetSegment handles GET /worker/segments/{job}/{idx}.
// Downloads the leased input segment from {stagingRoot}/{jobID}/seg_%05d.mkv.
func (h *SegmentHandler) GetSegment(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job")

	idx, inputPath, _, rejected := h.verifyAndResolve(w, r, "segment-get")
	if rejected {
		return
	}

	// ── Security req #4: lease ownership ──────────────────────────────────
	seg, err := h.segs.Get(r.Context(), jobID, idx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeBadRequest(w)
		} else {
			h.log.Errorw("segments: segment lookup failed", "job_id", jobID, "idx", idx, "error", err)
			writeInternalError(w)
		}
		return
	}
	// The segment must be leased (not pending, not done) to be fetched.
	// The HMAC handle already proves the caller was the leased worker at mint time;
	// requiring status=leased adds a server-side state check as defense-in-depth.
	if seg.Status != domain.SegLeased {
		writeUnauthorized(w)
		return
	}

	// Serve the file.
	f, err := os.Open(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			// The segment file not existing is an internal setup error, not a
			// caller error — but we must not leak the path. Log and return 500.
			h.log.Errorw("segments: input segment file not found on disk",
				"job_id", jobID, "idx", idx)
			writeInternalError(w)
		} else {
			h.log.Errorw("segments: open input segment failed",
				"job_id", jobID, "idx", idx, "error", err)
			writeInternalError(w)
		}
		return
	}
	defer f.Close()

	// Large segment body (tens of MiB) over a slow worker link can exceed the
	// global absolute WriteTimeout — clear the deadline before streaming.
	clearWriteDeadline(w, h.log, "segments: clear write deadline failed", "job_id", jobID, "idx", idx)

	w.Header().Set("Content-Type", "video/x-matroska")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, f); err != nil {
		// Client likely disconnected mid-stream; log at debug to avoid noise.
		h.log.Warnw("segments: copy to response body failed",
			"job_id", jobID, "idx", idx, "error", err)
	}
}

// PutSegment handles PUT /worker/segments/{job}/{idx}.
// Accepts the upscaled output and writes it to {stagingRoot}/{jobID}/upscaled/seg_%05d.mkv,
// then calls SegmentRepository.MarkDone.
func (h *SegmentHandler) PutSegment(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job")

	idx, _, job, rejected := h.verifyAndResolve(w, r, "segment-put")
	if rejected {
		return
	}

	// ── Security req #4: lease ownership + anti-overwrite check ───────────
	seg, err := h.segs.Get(r.Context(), jobID, idx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeBadRequest(w)
		} else {
			h.log.Errorw("segments: segment lookup failed (put)", "job_id", jobID, "idx", idx, "error", err)
			writeInternalError(w)
		}
		return
	}

	// ── Security req #6: anti-overwrite — reject if already done ──────────
	if seg.Status == domain.SegDone {
		writeConflict(w)
		return
	}
	// Segment must be leased to accept a PUT.
	if seg.Status != domain.SegLeased {
		writeUnauthorized(w)
		return
	}

	// ── Security req #6: reject if job is terminal or finalizing ──────────
	// job was already loaded + bound-checked by verifyAndResolve above, so we
	// reuse it here instead of re-fetching the same row.
	if job.Status.IsTerminal() || job.Status == domain.JobFinalizing {
		writeConflict(w)
		return
	}

	// ── Security req #5: body cap ──────────────────────────────────────────
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)

	// ── Build output path (defense-in-depth path check for upscaled dir) ──
	// jobID + idx already passed verifyAndResolve's traversal checks above, so
	// jobID is known to stay within stagingRoot. We re-assert the output file
	// stays within {stagingRoot}/{jobID}/upscaled as belt-and-braces.
	segName := fmt.Sprintf("seg_%05d.mkv", idx)
	outDir := filepath.Clean(filepath.Join(h.stagingRoot, jobID, "upscaled"))
	outPath := filepath.Clean(filepath.Join(outDir, segName))
	if !strings.HasPrefix(outPath, outDir+string(filepath.Separator)) {
		h.log.Warnw("segments: PUT path traversal detected", "job_id", jobID, "idx", idx)
		writeBadRequest(w)
		return
	}

	// Ensure the output directory exists.
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		h.log.Errorw("segments: mkdir upscaled dir failed", "job_id", jobID, "idx", idx, "error", err)
		writeInternalError(w)
		return
	}

	// Write via temp file + atomic rename (security req #6).
	tmp, err := os.CreateTemp(outDir, ".seg_tmp_")
	if err != nil {
		h.log.Errorw("segments: create temp file failed", "job_id", jobID, "idx", idx, "error", err)
		writeInternalError(w)
		return
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup of the temp file if we didn't rename it.
		os.Remove(tmpName)
	}()

	written, err := io.Copy(tmp, r.Body)
	if err != nil {
		tmp.Close()
		// Check if this was a body-too-large error from MaxBytesReader.
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			json.NewEncoder(w).Encode(map[string]string{"error": "request entity too large"}) //nolint:errcheck
			return
		}
		h.log.Errorw("segments: read PUT body failed", "job_id", jobID, "idx", idx, "error", err)
		writeInternalError(w)
		return
	}
	if err := tmp.Close(); err != nil {
		h.log.Errorw("segments: close temp file failed", "job_id", jobID, "idx", idx, "error", err)
		writeInternalError(w)
		return
	}

	// Atomic rename to final path.
	if err := os.Rename(tmpName, outPath); err != nil {
		h.log.Errorw("segments: rename temp→final failed", "job_id", jobID, "idx", idx, "error", err)
		writeInternalError(w)
		return
	}
	// Temp file consumed — prevent the deferred Remove from running on the now-renamed path.
	// We shadow tmpName so the defer Remove is a benign no-op.
	tmpName = ""

	// Mark the segment done.
	if err := h.segs.MarkDone(r.Context(), jobID, idx, written); err != nil {
		h.log.Errorw("segments: MarkDone failed", "job_id", jobID, "idx", idx, "error", err)
		writeInternalError(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}
