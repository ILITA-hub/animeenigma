package domain

import "time"

// JobSource identifies which provider seeded the magnet for a
// library_jobs row. Values are exactly the strings persisted in the
// `job_source` Postgres enum (see migrations/001_library_jobs.sql).
type JobSource string

const (
	JobSourceNyaa       JobSource = "nyaa"
	JobSourceAnimeTosho JobSource = "animetosho"
	JobSourceManual     JobSource = "manual"
	// JobSourceJackett tags rows whose magnet came via the Jackett
	// multi-indexer aggregator (the primary search tier). Added to the
	// `job_source` enum by migrations/004_jackett_source.sql.
	JobSourceJackett JobSource = "jackett"
)

// JobStatus is the locked state machine for a library_jobs row:
//
//	queued → downloading → encoding → uploading → done|failed|cancelled
//
// Phase 3 implements queued → downloading and stops at the 'encoding'
// boundary; Phase 4 picks up at 'encoding'. Values match the
// `job_status` Postgres enum.
type JobStatus string

const (
	JobStatusQueued      JobStatus = "queued"
	JobStatusDownloading JobStatus = "downloading"
	JobStatusEncoding    JobStatus = "encoding"
	JobStatusUploading   JobStatus = "uploading"
	JobStatusDone        JobStatus = "done"
	JobStatusFailed      JobStatus = "failed"
	JobStatusCancelled   JobStatus = "cancelled"
)

// IsTerminal reports whether the status is one of the end states
// (done / failed / cancelled). Used by Cancel(): once a row is
// terminal we never transition it again.
func (s JobStatus) IsTerminal() bool {
	switch s {
	case JobStatusDone, JobStatusFailed, JobStatusCancelled:
		return true
	default:
		return false
	}
}

// Job is the Go-side mirror of the library_jobs row defined in
// migrations/001_library_jobs.sql. Field tags use snake_case to match
// the columns. UUID PKs default-server-fill via gen_random_uuid(); we
// keep the GORM `default:gen_random_uuid()` tag in sync so AutoMigrate
// stays a safe no-op alongside the SQL migration.
type Job struct {
	ID           string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid();column:id" json:"id"`
	Source       JobSource  `gorm:"type:job_source;not null;column:source" json:"source"`
	Magnet       string     `gorm:"type:text;not null;column:magnet" json:"magnet"`
	Title        string     `gorm:"type:text;not null;column:title" json:"title"`
	Uploader     string     `gorm:"type:text;column:uploader" json:"uploader,omitempty"`
	Quality      string     `gorm:"type:text;column:quality" json:"quality,omitempty"`
	SizeBytes    int64      `gorm:"type:bigint;not null;default:0;column:size_bytes" json:"size_bytes"`
	ShikimoriID  string     `gorm:"type:text;column:shikimori_id" json:"shikimori_id,omitempty"`
	// Episode is the INTENDED episode persisted at enqueue (migration 009,
	// Phase 09). Nullable pointer: absent (NULL) for admin/manual rows whose
	// episode is only known after detector.DetectEpisode runs post-download;
	// set by the Planner for autocache rows so single-flight dedup on
	// (shikimori_id, episode) + the per-trigger download metric work before
	// filename detection.
	Episode      *int       `gorm:"type:int;column:episode" json:"episode,omitempty"`
	Status       JobStatus  `gorm:"type:job_status;not null;default:queued;column:status" json:"status"`
	ProgressPct  int        `gorm:"type:int;not null;default:0;column:progress_pct" json:"progress_pct"`
	ErrorText    string     `gorm:"type:text;column:error_text" json:"error_text,omitempty"`
	CreatedAt    time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at" json:"updated_at"`
	CompletedAt  *time.Time `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

// TableName pins the table name (GORM would otherwise pluralize to
// "jobs"). The migration uses "library_jobs".
func (Job) TableName() string { return "library_jobs" }
