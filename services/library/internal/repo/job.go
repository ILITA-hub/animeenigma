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

// UpdateProgress writes a new progress_pct (and bumps updated_at).
// pct = clamp(downloadedBytes * 100 / totalBytes, 0, 100). When
// totalBytes <= 0 (anacrolix hasn't received metadata yet) we leave
// the column unchanged — the previous value is the best we have.
//
// peers is accepted for symmetry with the SPEC signature but not
// currently persisted; the metric collector consumes it directly.
func (r *JobRepository) UpdateProgress(ctx context.Context, id string, downloadedBytes, totalBytes int64, peers int) error {
	_ = peers
	if totalBytes <= 0 {
		// Just bump updated_at so stall detection can read fresh tx.
		return r.db.WithContext(ctx).
			Model(&domain.Job{}).
			Where("id = ?", id).
			Update("updated_at", time.Now()).Error
	}
	pct := int(downloadedBytes * 100 / totalBytes)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return r.db.WithContext(ctx).
		Model(&domain.Job{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"progress_pct": pct,
			"updated_at":   time.Now(),
		}).Error
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
