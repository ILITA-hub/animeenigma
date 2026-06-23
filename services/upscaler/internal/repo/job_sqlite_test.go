package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
)

func TestJobRepository_CreateAndGet(t *testing.T) {
	db := openTestDB(t)
	r := NewJobRepository(db)
	ctx := context.Background()

	job := &domain.UpscaleJob{
		ShikimoriID: "99999",
		Episode:     3,
		Model:       "realesrgan-x4plus-anime",
		Scale:       4,
		Status:      domain.JobQueued,
	}
	if err := r.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if job.ID == "" {
		t.Fatal("Create did not populate ID")
	}

	got, err := r.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ShikimoriID != "99999" {
		t.Fatalf("ShikimoriID = %q, want %q", got.ShikimoriID, "99999")
	}
	if got.Episode != 3 {
		t.Fatalf("Episode = %d, want 3", got.Episode)
	}
	if got.Status != domain.JobQueued {
		t.Fatalf("Status = %q, want %q", got.Status, domain.JobQueued)
	}
}

func TestJobRepository_List_ByStatus(t *testing.T) {
	db := openTestDB(t)
	r := NewJobRepository(db)
	ctx := context.Background()

	jobs := []*domain.UpscaleJob{
		{ShikimoriID: "1", Episode: 1, Model: "m", Scale: 2, Status: domain.JobQueued},
		{ShikimoriID: "2", Episode: 1, Model: "m", Scale: 2, Status: domain.JobDone},
		{ShikimoriID: "3", Episode: 1, Model: "m", Scale: 2, Status: domain.JobQueued},
	}
	for _, j := range jobs {
		if err := r.Create(ctx, j); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	queued, err := r.List(ctx, JobFilter{Status: domain.JobQueued})
	if err != nil {
		t.Fatalf("List queued: %v", err)
	}
	if len(queued) != 2 {
		t.Fatalf("List(queued) len=%d, want 2", len(queued))
	}

	done, err := r.List(ctx, JobFilter{Status: domain.JobDone})
	if err != nil {
		t.Fatalf("List done: %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("List(done) len=%d, want 1", len(done))
	}
}

func TestJobRepository_List_ByShikimoriID(t *testing.T) {
	db := openTestDB(t)
	r := NewJobRepository(db)
	ctx := context.Background()

	jobs := []*domain.UpscaleJob{
		{ShikimoriID: "AAA", Episode: 1, Model: "m", Scale: 2, Status: domain.JobQueued},
		{ShikimoriID: "BBB", Episode: 1, Model: "m", Scale: 2, Status: domain.JobQueued},
	}
	for _, j := range jobs {
		if err := r.Create(ctx, j); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	list, err := r.List(ctx, JobFilter{ShikimoriID: "AAA"})
	if err != nil {
		t.Fatalf("List by ShikimoriID: %v", err)
	}
	if len(list) != 1 || list[0].ShikimoriID != "AAA" {
		t.Fatalf("List by ShikimoriID = %v, want [AAA]", list)
	}
}

func TestJobRepository_UpdateStatus(t *testing.T) {
	db := openTestDB(t)
	r := NewJobRepository(db)
	ctx := context.Background()

	job := &domain.UpscaleJob{ShikimoriID: "X", Episode: 1, Model: "m", Scale: 2, Status: domain.JobQueued}
	if err := r.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.UpdateStatus(ctx, job.ID, domain.JobFailed, "something broke"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := r.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.JobFailed {
		t.Fatalf("Status = %q, want %q", got.Status, domain.JobFailed)
	}
	if got.ErrorText != "something broke" {
		t.Fatalf("ErrorText = %q, want %q", got.ErrorText, "something broke")
	}
}

func TestJobRepository_SetProgress(t *testing.T) {
	db := openTestDB(t)
	r := NewJobRepository(db)
	ctx := context.Background()

	job := &domain.UpscaleJob{ShikimoriID: "X", Episode: 1, Model: "m", Scale: 2, Status: domain.JobUpscaling}
	if err := r.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.SetProgress(ctx, job.ID, 42); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}

	got, err := r.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ProgressPct != 42 {
		t.Fatalf("ProgressPct = %d, want 42", got.ProgressPct)
	}
}

func TestJobRepository_SetSourceMeta(t *testing.T) {
	db := openTestDB(t)
	r := NewJobRepository(db)
	ctx := context.Background()

	job := &domain.UpscaleJob{ShikimoriID: "X", Episode: 1, Model: "m", Scale: 2, Status: domain.JobSegmenting}
	if err := r.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.SetSourceMeta(ctx, job.ID, "h264", "yuv420p", "23.976", 120); err != nil {
		t.Fatalf("SetSourceMeta: %v", err)
	}

	got, err := r.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SourceCodec != "h264" {
		t.Fatalf("SourceCodec = %q, want h264", got.SourceCodec)
	}
	if got.SourcePixFmt != "yuv420p" {
		t.Fatalf("SourcePixFmt = %q, want yuv420p", got.SourcePixFmt)
	}
	if got.SourceFPS != "23.976" {
		t.Fatalf("SourceFPS = %q, want 23.976", got.SourceFPS)
	}
	if got.SegmentCount != 120 {
		t.Fatalf("SegmentCount = %d, want 120", got.SegmentCount)
	}
}

func TestJobRepository_SetOutputPrefix(t *testing.T) {
	db := openTestDB(t)
	r := NewJobRepository(db)
	ctx := context.Background()

	job := &domain.UpscaleJob{ShikimoriID: "X", Episode: 1, Model: "m", Scale: 2, Status: domain.JobFinalizing}
	if err := r.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.SetOutputPrefix(ctx, job.ID, "uploads/upscaler/X/1/"); err != nil {
		t.Fatalf("SetOutputPrefix: %v", err)
	}

	got, err := r.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.OutputPrefix != "uploads/upscaler/X/1/" {
		t.Fatalf("OutputPrefix = %q, want uploads/upscaler/X/1/", got.OutputPrefix)
	}
}

func TestJobRepository_NextEligible(t *testing.T) {
	db := openTestDB(t)
	jr := NewJobRepository(db)
	sr := NewSegmentRepository(db)
	ctx := context.Background()

	// Create two non-terminal jobs. The older one has a pending segment and
	// should be returned by NextEligible.
	older := &domain.UpscaleJob{ShikimoriID: "OLDER", Episode: 1, Model: "m", Scale: 2, Status: domain.JobQueued}
	if err := jr.Create(ctx, older); err != nil {
		t.Fatalf("Create older: %v", err)
	}
	// Ensure created_at ordering is different (SQLite second-resolution).
	time.Sleep(10 * time.Millisecond)
	newer := &domain.UpscaleJob{ShikimoriID: "NEWER", Episode: 2, Model: "m", Scale: 2, Status: domain.JobQueued}
	if err := jr.Create(ctx, newer); err != nil {
		t.Fatalf("Create newer: %v", err)
	}

	// Give the older job a pending segment.
	if err := sr.BulkInsertPending(ctx, older.ID, 1); err != nil {
		t.Fatalf("BulkInsertPending: %v", err)
	}

	got, err := jr.NextEligible(ctx)
	if err != nil {
		t.Fatalf("NextEligible: %v", err)
	}
	if got == nil {
		t.Fatal("NextEligible returned nil, want the older job")
	}
	if got.ID != older.ID {
		t.Fatalf("NextEligible returned job %s, want older job %s", got.ID, older.ID)
	}

	// Terminal jobs should never be returned.
	terminal := &domain.UpscaleJob{ShikimoriID: "TERM", Episode: 1, Model: "m", Scale: 2, Status: domain.JobDone}
	if err := jr.Create(ctx, terminal); err != nil {
		t.Fatalf("Create terminal: %v", err)
	}
	if err := sr.BulkInsertPending(ctx, terminal.ID, 1); err != nil {
		t.Fatalf("BulkInsertPending terminal: %v", err)
	}
	// Next should still return the older job, not terminal.
	got2, err := jr.NextEligible(ctx)
	if err != nil {
		t.Fatalf("NextEligible (2nd): %v", err)
	}
	if got2 == nil || got2.ID != older.ID {
		t.Fatalf("NextEligible (2nd) = %v, want older job %s", got2, older.ID)
	}
}
