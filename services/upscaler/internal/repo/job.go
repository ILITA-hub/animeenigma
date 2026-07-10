package repo

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// JobFilter controls which jobs are returned by List.
// Zero values mean "no filter" for that field.
type JobFilter struct {
	Status      domain.JobStatus
	ShikimoriID string
	Limit       int
	Offset      int
}

// JobRepository provides access to the upscale_jobs table.
type JobRepository struct {
	db *gorm.DB
}

// NewJobRepository constructs a JobRepository backed by db.
func NewJobRepository(db *gorm.DB) *JobRepository {
	return &JobRepository{db: db}
}

// Create inserts a new job. On Postgres the database populates ID via
// gen_random_uuid(); on SQLite (used by unit tests) the UUID is generated in
// Go so the NOT NULL constraint is satisfied without a SQL-level default.
func (r *JobRepository) Create(ctx context.Context, job *domain.UpscaleJob) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	return r.db.WithContext(ctx).Create(job).Error
}

// Get returns the job with the given UUID, or gorm.ErrRecordNotFound when absent.
func (r *JobRepository) Get(ctx context.Context, id string) (*domain.UpscaleJob, error) {
	var job domain.UpscaleJob
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// List returns jobs matching the filter, ordered by created_at ASC.
func (r *JobRepository) List(ctx context.Context, f JobFilter) ([]domain.UpscaleJob, error) {
	q := r.db.WithContext(ctx).Order("created_at ASC")
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.ShikimoriID != "" {
		q = q.Where("shikimori_id = ?", f.ShikimoriID)
	}
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}

	var jobs []domain.UpscaleJob
	if err := q.Find(&jobs).Error; err != nil {
		return nil, err
	}
	return jobs, nil
}

// UpdateStatus sets the job status and error_text. When the new status is a
// terminal state, completed_at is also recorded.
func (r *JobRepository) UpdateStatus(ctx context.Context, id string, status domain.JobStatus, errText string) error {
	updates := map[string]interface{}{
		"status":     status,
		"error_text": errText,
	}
	if status.IsTerminal() {
		now := time.Now()
		updates["completed_at"] = now
	}
	return r.db.WithContext(ctx).
		Model(&domain.UpscaleJob{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// SetProgress updates progress_pct for the given job.
func (r *JobRepository) SetProgress(ctx context.Context, id string, pct int) error {
	return r.db.WithContext(ctx).
		Model(&domain.UpscaleJob{}).
		Where("id = ?", id).
		Update("progress_pct", pct).Error
}

// SetSourceMeta records video source metadata discovered during segmentation.
// height is the source video height in pixels (from ffprobe); it is persisted so
// the finalizer can derive the UPSCALED-{height}p prefix deterministically.
func (r *JobRepository) SetSourceMeta(ctx context.Context, id, codec, pixfmt, fps string, height, segCount int) error {
	return r.db.WithContext(ctx).
		Model(&domain.UpscaleJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"source_codec":  codec,
			"source_pixfmt": pixfmt,
			"source_fps":    fps,
			"source_height": height,
			"segment_count": segCount,
		}).Error
}

// SetOutput records where the finalized HLS output landed: the object prefix
// plus the storage-service backend id it resolved to (class "upscaled").
func (r *JobRepository) SetOutput(ctx context.Context, id, prefix, storage string) error {
	return r.db.WithContext(ctx).
		Model(&domain.UpscaleJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"output_prefix": prefix,
			"storage":       storage,
		}).Error
}

// NextEligible returns the oldest non-terminal job that has at least one
// segment in 'pending' status OR one segment in 'leased' status with an
// expired lease. Returns (nil, nil) when no eligible job exists.
func (r *JobRepository) NextEligible(ctx context.Context) (*domain.UpscaleJob, error) {
	now := time.Now()

	// Terminal statuses to exclude.
	terminalStatuses := []domain.JobStatus{
		domain.JobDone,
		domain.JobFailed,
		domain.JobCancelled,
	}

	var job domain.UpscaleJob
	err := r.db.WithContext(ctx).
		Where("status NOT IN ?", terminalStatuses).
		Where(`id IN (
			SELECT DISTINCT job_id FROM upscale_segments
			WHERE status = ? OR (status = ? AND lease_expires_at < ?)
		)`, domain.SegPending, domain.SegLeased, now).
		Order("created_at ASC").
		First(&job).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}
