package repo

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SegmentRepository provides access to the upscale_segments table.
type SegmentRepository struct {
	db *gorm.DB
}

// NewSegmentRepository constructs a SegmentRepository backed by db.
func NewSegmentRepository(db *gorm.DB) *SegmentRepository {
	return &SegmentRepository{db: db}
}

// BulkInsertPending inserts n segments with idx 0..n-1 in status 'pending' for
// the given job. Existing rows (same job_id, idx) are ignored so BulkInsertPending
// is safe to retry.
func (r *SegmentRepository) BulkInsertPending(ctx context.Context, jobID string, n int) error {
	if n == 0 {
		return nil
	}
	segs := make([]domain.UpscaleSegment, n)
	for i := 0; i < n; i++ {
		segs[i] = domain.UpscaleSegment{
			JobID:  jobID,
			Idx:    i,
			Status: domain.SegPending,
		}
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&segs).Error
}

// LeaseNext atomically claims the lowest-idx segment for jobID that is either
// pending or has an expired lease. Returns (nil, nil) when no segment is available.
//
// WR-03 dialect gap: SKIP LOCKED is a Postgres-only hint. SQLite ignores it
// silently — single-threaded unit tests don't need it; real concurrency
// exclusion is verified by the Postgres integration test in
// segment_integration_test.go.
func (r *SegmentRepository) LeaseNext(ctx context.Context, jobID, workerID string, ttl time.Duration) (*domain.UpscaleSegment, error) {
	now := time.Now()
	expires := now.Add(ttl)

	var seg domain.UpscaleSegment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(
				"job_id = ? AND (status = ? OR (status = ? AND lease_expires_at < ?))",
				jobID, domain.SegPending, domain.SegLeased, now,
			).
			Order("idx ASC").
			First(&seg)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound // sentinel: no rows
			}
			return result.Error
		}

		// Atomically update: set leased state; COALESCE started_at to preserve
		// a prior start time on re-lease after expiry.
		// Use explicit WHERE on composite PK — GORM Model(&composite-pk-struct).Updates()
		// can silently mis-build the WHERE for composite keys on some drivers.
		return tx.Model(&domain.UpscaleSegment{}).
			Where("job_id = ? AND idx = ?", seg.JobID, seg.Idx).
			Updates(map[string]interface{}{
				"status":           domain.SegLeased,
				"worker_id":        workerID,
				"lease_expires_at": expires,
				"started_at":       gorm.Expr("CASE WHEN started_at IS NULL THEN ? ELSE started_at END", now),
			}).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	// Re-read to return the post-update state.
	if err := r.db.WithContext(ctx).
		Where("job_id = ? AND idx = ?", jobID, seg.Idx).
		First(&seg).Error; err != nil {
		return nil, err
	}
	return &seg, nil
}

// MarkDone records successful completion of a segment.
func (r *SegmentRepository) MarkDone(ctx context.Context, jobID string, idx int, outBytes int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.UpscaleSegment{}).
		Where("job_id = ? AND idx = ?", jobID, idx).
		Updates(map[string]interface{}{
			"status":       domain.SegDone,
			"out_bytes":    outBytes,
			"completed_at": now,
		}).Error
}

// ExpireStale flips all leased segments whose lease_expires_at is before now
// back to 'pending', clearing the worker_id. Returns the number of rows flipped.
func (r *SegmentRepository) ExpireStale(ctx context.Context, now time.Time) (int, error) {
	result := r.db.WithContext(ctx).
		Model(&domain.UpscaleSegment{}).
		Where("status = ? AND lease_expires_at < ?", domain.SegLeased, now).
		Updates(map[string]interface{}{
			"status": domain.SegPending,
			// worker_id cleared to "" (sentinel) — WorkerID is non-pointer string by design; no query relies on NULL
			"worker_id": "",
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

// segmentCount is a scratch type for the GROUP BY status aggregation query.
type segmentCount struct {
	Status string
	Count  int
}

// Counts returns the number of segments in each status for the given job.
func (r *SegmentRepository) Counts(ctx context.Context, jobID string) (pending, leased, done int, err error) {
	var rows []segmentCount
	if err = r.db.WithContext(ctx).
		Model(&domain.UpscaleSegment{}).
		Select("status, COUNT(*) as count").
		Where("job_id = ?", jobID).
		Group("status").
		Scan(&rows).Error; err != nil {
		return
	}
	for _, row := range rows {
		switch domain.SegmentStatus(row.Status) {
		case domain.SegPending:
			pending = row.Count
		case domain.SegLeased:
			leased = row.Count
		case domain.SegDone:
			done = row.Count
		}
	}
	return
}

// ListByJob returns all segments for a job ordered by idx ASC.
func (r *SegmentRepository) ListByJob(ctx context.Context, jobID string) ([]domain.UpscaleSegment, error) {
	var segs []domain.UpscaleSegment
	if err := r.db.WithContext(ctx).
		Where("job_id = ?", jobID).
		Order("idx ASC").
		Find(&segs).Error; err != nil {
		return nil, err
	}
	return segs, nil
}
