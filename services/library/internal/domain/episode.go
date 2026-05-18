package domain

import "time"

// Episode is the Go-side mirror of the library_episodes row defined in
// migrations/002_library_episodes.sql. One row per successfully-encoded
// episode; pointer fields are nullable in the SQL schema (job_id,
// duration_sec, size_bytes) so nil → NULL in the DB and is omitted
// from JSON output.
//
// MinioPath stores the bucket-relative PREFIX (always ends with `/`):
// e.g. "12345/3/" or "pending/abcd-uuid/1/". The HTTP handler appends
// "playlist.m3u8" when building the public URL.
type Episode struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid();column:id" json:"id"`
	ShikimoriID   string    `gorm:"type:text;not null;column:shikimori_id" json:"shikimori_id"`
	EpisodeNumber int       `gorm:"type:int;not null;column:episode_number" json:"episode_number"`
	JobID         *string   `gorm:"type:uuid;column:job_id" json:"job_id,omitempty"`
	MinioPath     string    `gorm:"type:text;not null;column:minio_path" json:"minio_path"`
	DurationSec   *int      `gorm:"type:int;column:duration_sec" json:"duration_sec,omitempty"`
	SizeBytes     *int64    `gorm:"type:bigint;column:size_bytes" json:"size_bytes,omitempty"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName pins the table name (GORM would otherwise pluralize to
// "episodes"). The migration uses "library_episodes".
func (Episode) TableName() string { return "library_episodes" }
