// Package repo is the persistence layer for the library service.
//
// JobRepository is the only piece of state the service holds for any
// length of time: search is stateless, the in-flight torrent map is
// in-memory. Every queued / downloading / encoding job is durable.
//
// The Claim() method is the worker checkout. It runs a
// SELECT ... FOR UPDATE SKIP LOCKED LIMIT 1 inside a transaction so
// that N concurrent workers each grab a distinct row without one
// stomping another. GORM's clause.Locking is the canonical way to
// express this — the existing scheduler service uses raw SQL for the
// same effect, but the Phase 3 CONTEXT decision prefers the typed
// clause variant for readability.
package repo

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// JobFilter narrows the List() query. Statuses == nil means "any
// status"; Limit defaults to 100 and is clamped to 500.
type JobFilter struct {
	Statuses []domain.JobStatus
	Limit    int
	Offset   int
}

// JobRepository handles persistence for library_jobs rows. All methods
// take a context so cancellation propagates from the HTTP / worker layer.
type JobRepository struct {
	db *gorm.DB
}

// NewJobRepository constructs a JobRepository over the provided *gorm.DB.
func NewJobRepository(db *gorm.DB) *JobRepository {
	return &JobRepository{db: db}
}

// Create inserts a new job. The migration provides server-side defaults
// for id (uuid), status, progress_pct, size_bytes, created_at,
// updated_at — so callers only need to populate the user-supplied
// fields (source, magnet, title, optional uploader/quality/etc.).
// GORM writes the generated id + timestamps back onto the struct so the
// handler can return the freshly-created row.
func (r *JobRepository) Create(ctx context.Context, job *domain.Job) error {
	if job == nil {
		return liberrors.InvalidInput("job is nil")
	}
	return r.db.WithContext(ctx).Create(job).Error
}

// GetByID returns the row with the given id, or liberrors.NotFound when
// no such row exists. Any other DB error is wrapped Internal.
func (r *JobRepository) GetByID(ctx context.Context, id string) (*domain.Job, error) {
	var job domain.Job
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&job).Error
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, liberrors.NotFound("job")
	}
	if err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "fetch job")
	}
	return &job, nil
}

// List returns jobs matching the filter, ordered by created_at DESC so
// the admin UI sees the freshest job first. Limit defaults to 100 and
// caps at 500 — large pages put pressure on the connection pool.
func (r *JobRepository) List(ctx context.Context, f JobFilter) ([]domain.Job, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	q := r.db.WithContext(ctx).Model(&domain.Job{}).Order("created_at DESC").Limit(limit)
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("status IN ?", f.Statuses)
	}
	var jobs []domain.Job
	if err := q.Find(&jobs).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list jobs")
	}
	return jobs, nil
}

// HasActiveForEpisode is the TRIG-04 single-flight gate. It reports whether a
// NON-TERMINAL library_jobs row already targets the given (shikimori_id,
// episode) — i.e. a job for that episode is queued / downloading / encoding /
// uploading and has not yet reached done|failed|cancelled. The Phase-09 Planner
// calls this before enqueueing so a concurrent demand for the same episode
// collapses to one job: the second drain sees the in-flight row and skips.
//
// Terminal rows are EXCLUDED so a previously-failed job never permanently blocks
// a re-enqueue (a failed job goes terminal → the next drain re-attempts). The
// episode argument keys on the intended-episode column added by migration 009.
// Returns (false, nil) when no active row exists; non-not-found DB errors are
// wrapped CodeInternal.
func (r *JobRepository) HasActiveForEpisode(ctx context.Context, shikimoriID string, episode int) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.Job{}).
		Where("shikimori_id = ? AND episode = ? AND status NOT IN ?", shikimoriID, episode, []domain.JobStatus{
			domain.JobStatusDone,
			domain.JobStatusFailed,
			domain.JobStatusCancelled,
		}).
		Limit(1).
		Count(&count).Error
	if err != nil {
		return false, liberrors.Wrap(err, liberrors.CodeInternal, "check active job for episode")
	}
	return count > 0, nil
}

// SumInflightJobBytes returns Σ size_bytes over the NON-TERMINAL autocache jobs —
// i.e. rows the Planner/admin path admitted that have NOT yet materialized into a
// pool episode row (status queued|downloading|encoding|uploading). It is the
// in-flight reservation that closes the WR-01 admit→materialize gap: a job is
// admitted (jobs.Create) but does not become a SumPoolBytes-counted aeProvider row
// until the encoder runs episodeRepo.Create minutes-to-hours later, so without this
// the budget is enforced only against already-materialized rows and N concurrent /
// sequential admits against a stale snapshot can overshoot by ΣestBytes.
//
// Scoped to source='autocache': admin uploads materialize via the Link handler and
// their in-flight window is the operator's own synchronous action (and admin
// size_bytes is the operator's declared estimate, not a torrent's), so counting only
// autocache in-flight bytes matches the planner-driven over-admission this guards.
// The reservation is self-releasing and CANNOT leak: a job leaving the non-terminal
// set (done → its row now counts in SumPoolBytes; failed|cancelled → it never will)
// drops out of this SUM automatically — there is no counter to decrement, so success
// AND failure both release it.
//
// SUM over zero rows is SQL NULL, COALESCE'd to 0, so the budget math never special-
// cases "no jobs in flight".
func (r *JobRepository) SumInflightJobBytes(ctx context.Context) (int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&domain.Job{}).
		Where("source = ?", domain.JobSourceAutocache).
		Where("status NOT IN ?", []domain.JobStatus{
			domain.JobStatusDone,
			domain.JobStatusFailed,
			domain.JobStatusCancelled,
		}).
		Select("COALESCE(SUM(size_bytes), 0)").
		Scan(&total).Error; err != nil {
		return 0, liberrors.Wrap(err, liberrors.CodeInternal, "sum inflight job bytes")
	}
	return total, nil
}

// Claim atomically picks the oldest row whose status is in `statuses`
// and flips it to 'downloading'. The whole thing runs in a single
// transaction with FOR UPDATE SKIP LOCKED so two parallel workers will
// each receive a different row and a third concurrent caller (with no
// remaining rows) receives (nil, nil).
//
// Returns (nil, nil) when the queue is empty — callers MUST handle
// that as "back off and retry later".
func (r *JobRepository) Claim(ctx context.Context, statuses ...domain.JobStatus) (*domain.Job, error) {
	if len(statuses) == 0 {
		statuses = []domain.JobStatus{domain.JobStatusQueued}
	}

	var claimed *domain.Job
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row domain.Job
		res := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ?", statuses).
			Order("created_at ASC").
			Limit(1).
			Take(&row)
		if stderrors.Is(res.Error, gorm.ErrRecordNotFound) {
			return nil // claimed stays nil
		}
		if res.Error != nil {
			return res.Error
		}
		now := time.Now()
		upd := tx.Model(&row).Updates(map[string]any{
			"status":     domain.JobStatusDownloading,
			"updated_at": now,
		})
		if upd.Error != nil {
			return upd.Error
		}
		// Reflect the new status on the returned struct so the caller
		// doesn't have to re-fetch.
		row.Status = domain.JobStatusDownloading
		row.UpdatedAt = now
		claimed = &row
		return nil
	})
	if err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "claim job")
	}
	return claimed, nil
}

// ClaimForEncoding atomically picks the oldest row in status='encoding'
// (the download→encode handoff state) and flips it to 'transcoding' (the
// in-progress state) inside one FOR UPDATE SKIP LOCKED transaction. The
// returned row carries Status='transcoding'.
//
// This is the encoder's counterpart to Claim() and exists specifically to
// close the double-encode race. The old path called Claim(ctx, 'encoding')
// — which (mis)flipped to 'downloading' — and the encoder then re-flipped
// the row BACK to 'encoding' for the whole ffmpeg run. Because nobody held
// a row lock during that minutes-long encode and 'encoding' is the
// claimable state, a second idle encoder worker re-claimed the SAME row and
// spawned a second ffmpeg. Flipping to the dedicated, non-claimable
// 'transcoding' state instead means the row drops out of the claimable set
// the instant it is claimed, so the second worker sees nothing.
//
// Returns (nil, nil) when no 'encoding' row is available — callers MUST
// handle that as "back off and retry later".
func (r *JobRepository) ClaimForEncoding(ctx context.Context) (*domain.Job, error) {
	var claimed *domain.Job
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row domain.Job
		res := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ?", domain.JobStatusEncoding).
			Order("created_at ASC").
			Limit(1).
			Take(&row)
		if stderrors.Is(res.Error, gorm.ErrRecordNotFound) {
			return nil // claimed stays nil
		}
		if res.Error != nil {
			return res.Error
		}
		now := time.Now()
		upd := tx.Model(&row).Updates(map[string]any{
			"status":     domain.JobStatusTranscoding,
			"updated_at": now,
		})
		if upd.Error != nil {
			return upd.Error
		}
		row.Status = domain.JobStatusTranscoding
		row.UpdatedAt = now
		claimed = &row
		return nil
	})
	if err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "claim job for encoding")
	}
	return claimed, nil
}

// SetProgressAndStatus transitions a job to newStatus AND persists a final
// progress_pct in the same UPDATE. In-flight progress now lives only in the
// live-progress cache (service.ProgressStore), so the DB row is written only at
// a job's terminal transitions: the download worker calls this on →encoding
// (pct=100) and on →failed (the last observed pct, deliberately non-100 so the
// stored row shows where a failed download died). pct is clamped to [0,100].
// Terminal statuses also stamp completed_at, matching UpdateStatus.
func (r *JobRepository) SetProgressAndStatus(ctx context.Context, id string, newStatus domain.JobStatus, pct int, errorText string) error {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	now := time.Now()
	updates := map[string]any{
		"status":       newStatus,
		"progress_pct": pct,
		"error_text":   errorText,
		"updated_at":   now,
	}
	if newStatus.IsTerminal() {
		updates["completed_at"] = now
	}
	res := r.db.WithContext(ctx).
		Model(&domain.Job{}).
		Where("id = ?", id).
		Updates(updates)
	if res.Error != nil {
		return liberrors.Wrap(res.Error, liberrors.CodeInternal, "set job progress+status")
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("job")
	}
	return nil
}

// UpdateStatus transitions a job to newStatus. errorText is written
// to error_text (empty string clears it). For terminal statuses
// (done / failed / cancelled) we also set completed_at = now().
func (r *JobRepository) UpdateStatus(ctx context.Context, id string, newStatus domain.JobStatus, errorText string) error {
	now := time.Now()
	updates := map[string]any{
		"status":     newStatus,
		"error_text": errorText,
		"updated_at": now,
	}
	if newStatus.IsTerminal() {
		updates["completed_at"] = now
	}
	res := r.db.WithContext(ctx).
		Model(&domain.Job{}).
		Where("id = ?", id).
		Updates(updates)
	if res.Error != nil {
		return liberrors.Wrap(res.Error, liberrors.CodeInternal, "update job status")
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("job")
	}
	return nil
}

// Cancel transitions a queued|downloading job to cancelled. Already-
// terminal rows are a no-op (returns nil; the row still exists). A
// missing row returns liberrors.NotFound.
//
// The DELETE handler calls this FIRST and then signals the in-memory
// torrent handle — flipping the status first guarantees that the
// worker's next progress tick observes the cancelled status and
// exits cleanly even if the in-memory handle.Cancel() is lost in a
// crash.
func (r *JobRepository) Cancel(ctx context.Context, id string) error {
	// First confirm the row exists at all.
	job, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if job.Status.IsTerminal() {
		return nil // no-op, already terminal
	}
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&domain.Job{}).
		Where("id = ? AND status IN ?", id, []domain.JobStatus{
			domain.JobStatusQueued,
			domain.JobStatusDownloading,
		}).
		Updates(map[string]any{
			"status":       domain.JobStatusCancelled,
			"updated_at":   now,
			"completed_at": now,
		})
	if res.Error != nil {
		return liberrors.Wrap(res.Error, liberrors.CodeInternal, "cancel job")
	}
	return nil
}

// ResumeInterruptedDownloads is the startup hook documented in the
// CONTEXT decisions: any row left in 'downloading' from a previous
// process is rewritten to 'queued' so a worker re-claims it. Returns
// the number of rows touched so main.go can log it. Run once at boot.
func (r *JobRepository) ResumeInterruptedDownloads(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).Exec(
		"UPDATE library_jobs SET status = 'queued', updated_at = now() WHERE status = 'downloading'",
	)
	if res.Error != nil {
		return 0, fmt.Errorf("resume interrupted downloads: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// retryRowFrom builds the fresh re-enqueue row from a failed job. Every
// user-facing field is inherited verbatim; status resets to queued, progress to
// 0, and error_text carries the SPEC-locked "retry of {oldID}" marker.
//
// Episode MUST carry over (CR-01): an autocache-sourced row stores a non-null
// intended-episode precisely so the Phase-09 Planner's single-flight gate
// HasActiveForEpisode(shikimori_id, episode) collapses concurrent demand to one
// job (TRIG-04). Dropping it gave a retried autocache row episode=NULL, so the
// next Planner sweep's in-flight check missed the retry and enqueued a SECOND
// download for the same episode. The *int copies cleanly: nil stays nil for
// manual/admin rows (their episode is only known post-detection), so manual
// retries are unaffected.
//
// Extracted from Retry() as a pure function so the field-copy contract is
// unit-testable without a Postgres container (the enum-typed Job table cannot
// AutoMigrate under SQLite).
func retryRowFrom(old *domain.Job, oldID string) *domain.Job {
	return &domain.Job{
		Source:      old.Source,
		Magnet:      old.Magnet,
		Title:       old.Title,
		Uploader:    old.Uploader,
		Quality:     old.Quality,
		Storage:     old.Storage,
		SizeBytes:   old.SizeBytes,
		ShikimoriID: old.ShikimoriID,
		Episode:     old.Episode,
		Status:      domain.JobStatusQueued,
		ProgressPct: 0,
		ErrorText:   formatRetryErrorText(oldID),
	}
}

// formatRetryErrorText returns the SPEC-locked audit string written
// into a retried job row's error_text column. Centralized here so the
// Phase-5 unit test pins the format.
func formatRetryErrorText(oldID string) string {
	return "retry of " + oldID
}

// UpdateShikimoriID flips the shikimori_id column on a row by id and
// bumps updated_at. The Phase-5 Link handler calls this AFTER the
// MinIO Move + library_episodes insert have both succeeded; on a
// missing row we return liberrors.NotFound.
//
// Empty id / shikimoriID are rejected so a typo upstream doesn't run
// an unbounded UPDATE.
func (r *JobRepository) UpdateShikimoriID(ctx context.Context, id, shikimoriID string) error {
	if id == "" {
		return liberrors.InvalidInput("job id required")
	}
	if shikimoriID == "" {
		return liberrors.InvalidInput("shikimori id required")
	}
	res := r.db.WithContext(ctx).
		Model(&domain.Job{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"shikimori_id": shikimoriID,
			"updated_at":   time.Now(),
		})
	if res.Error != nil {
		return liberrors.Wrap(res.Error, liberrors.CodeInternal, "update shikimori_id")
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("job")
	}
	return nil
}

// UpdateStorage writes the RESOLVED storage backend id (e.g. "minio" / "s3")
// onto a job row by id and bumps updated_at. The encoder worker calls this after
// a successful upload so a later Link of an unresolved (pending) job knows which
// backend to list/move its objects from. Best-effort by its caller — a missing
// row returns NotFound but the encoder only logs it (the episode row is the
// authoritative serving pointer). Empty id is rejected so a typo upstream can't
// run an unbounded UPDATE; an empty storage is allowed (the empty-string
// class-default sentinel, though the encoder always passes a concrete backend).
func (r *JobRepository) UpdateStorage(ctx context.Context, id, storage string) error {
	if id == "" {
		return liberrors.InvalidInput("job id required")
	}
	res := r.db.WithContext(ctx).
		Model(&domain.Job{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"storage":    storage,
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return liberrors.Wrap(res.Error, liberrors.CodeInternal, "update job storage")
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("job")
	}
	return nil
}

// Retry re-enqueues a failed job. The old row is preserved (still
// in 'failed') for the audit trail; the returned new row inherits
// every user-facing field plus the SPEC-locked error_text marker
// "retry of {old_id}". The whole thing runs in a single transaction so
// a partial failure can't leave a half-created row.
//
// Old row must be in 'failed' (otherwise InvalidInput). Missing old
// row → NotFound.
func (r *JobRepository) Retry(ctx context.Context, oldID string) (*domain.Job, error) {
	if oldID == "" {
		return nil, liberrors.InvalidInput("job id required")
	}
	var newJob *domain.Job
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old domain.Job
		if err := tx.Where("id = ?", oldID).First(&old).Error; err != nil {
			if stderrors.Is(err, gorm.ErrRecordNotFound) {
				return liberrors.NotFound("job")
			}
			return liberrors.Wrap(err, liberrors.CodeInternal, "fetch job for retry")
		}
		if old.Status != domain.JobStatusFailed {
			return liberrors.InvalidInput("only failed jobs can be retried")
		}
		fresh := retryRowFrom(&old, oldID)
		if err := tx.Create(fresh).Error; err != nil {
			return liberrors.Wrap(err, liberrors.CodeInternal, "create retry row")
		}
		newJob = fresh
		return nil
	})
	if err != nil {
		return nil, err
	}
	return newJob, nil
}

// ResumeInterruptedEncodes is the Phase-04 analogue of
// ResumeInterruptedDownloads. Any row left in an IN-PROGRESS encode state
// ('transcoding' = mid-ffmpeg, 'uploading' = mid-MinIO-push) from a
// previous process is rewritten to 'queued' so a worker re-claims it from
// the top of the pipeline. Run once at boot.
//
// No staleness guard (the old version only rewrote rows stale > 1h): this
// hook runs ONCE at boot, BEFORE this process's encoder pool starts, and
// the previous process is already dead (the deploy recreates the single
// library container stop-then-start). So every 'transcoding'/'uploading'
// row at boot is definitively an orphan — a recent updated_at just means it
// was killed mid-encode, which is exactly the case the 1h guard used to
// strand. This now mirrors ResumeInterruptedDownloads, which likewise
// requeues ALL 'downloading' rows unconditionally at boot.
//
// 'encoding' (the download→encode handoff / ready state) is deliberately
// NOT requeued: it is not in-progress, and the encoder will claim it
// normally via ClaimForEncoding, so it is self-healing.
//
// Returns the number of rows touched.
func (r *JobRepository) ResumeInterruptedEncodes(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).Exec(
		"UPDATE library_jobs SET status = 'queued', updated_at = now() " +
			"WHERE status IN ('transcoding', 'uploading')",
	)
	if res.Error != nil {
		return 0, fmt.Errorf("resume interrupted encodes: %w", res.Error)
	}
	return res.RowsAffected, nil
}
