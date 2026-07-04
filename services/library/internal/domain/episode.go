package domain

import "time"

// EpisodeSource distinguishes admin-uploaded content from autocache-downloaded
// content (POOL-03 / D6). The path is uniform (aeProvider/.../RAW/...) — THIS
// column is the discriminator. Values match the `episode_source` Postgres enum
// in migrations/005_autocache_pool.sql.
type EpisodeSource string

const (
	EpisodeSourceAdmin     EpisodeSource = "admin"
	EpisodeSourceAutocache EpisodeSource = "autocache"
)

// EpisodeTrack is the audio/subtitle track of a stored episode. v1 writes only
// `raw` (D2); `sub` / `dub` are reserved in the `episode_track` Postgres enum
// (migrations/005_autocache_pool.sql) so a future milestone needs no schema
// change, but they are NEVER written in this milestone.
type EpisodeTrack string

const (
	EpisodeTrackRaw EpisodeTrack = "raw"
	EpisodeTrackSub EpisodeTrack = "sub" // reserved, never written v1
	EpisodeTrackDub EpisodeTrack = "dub" // reserved, never written v1
)

// Episode is the Go-side mirror of the library_episodes row defined in
// migrations/002_library_episodes.sql (extended by 005_autocache_pool.sql).
// One row per successfully-encoded episode; pointer fields are nullable in the
// SQL schema (job_id, duration_sec, size_bytes, downloaded_at, last_fetch_at) so
// nil → NULL in the DB and is omitted from JSON output.
//
// DownloadedAt is a *time.Time on purpose: migration 005 declares the column
// nullable and the §3.3 backfill heals NULL → created_at. Modeling it as a
// non-pointer time.Time would make GORM insert the Go zero value
// (0001-01-01) instead of NULL on every Create that omits it, poisoning the
// Phase 8+ budget/freshness ledger. Leaving it nil → NULL keeps the backfill
// (and the future serve path that sets it) authoritative.
//
// MinioPath stores the bucket-relative PREFIX (always ends with `/`):
// e.g. "aeProvider/12345/RAW/3/" or "pending/abcd-uuid/1/". The HTTP handler
// appends "playlist.m3u8" when building the public URL.
//
// The five Phase-7 ledger fields (Source/Track/DownloadedAt/LastFetchAt/
// FetchCount) mirror migration 005 1:1 and feed the (future) Accountant and
// evictor; size_bytes already existed (POOL-03).
type Episode struct {
	ID            string        `gorm:"type:uuid;primaryKey;default:gen_random_uuid();column:id" json:"id"`
	ShikimoriID   string        `gorm:"type:text;not null;column:shikimori_id" json:"shikimori_id"`
	EpisodeNumber int           `gorm:"type:int;not null;column:episode_number" json:"episode_number"`
	JobID         *string       `gorm:"type:uuid;column:job_id" json:"job_id,omitempty"`
	MinioPath     string        `gorm:"type:text;not null;column:minio_path" json:"minio_path"`
	DurationSec   *int          `gorm:"type:int;column:duration_sec" json:"duration_sec,omitempty"`
	SizeBytes     *int64        `gorm:"type:bigint;column:size_bytes" json:"size_bytes,omitempty"`
	Source        EpisodeSource `gorm:"type:episode_source;not null;default:admin;column:source" json:"source"`
	Track         EpisodeTrack  `gorm:"type:episode_track;not null;default:raw;column:track" json:"track"`
	DownloadedAt  *time.Time    `gorm:"column:downloaded_at" json:"downloaded_at,omitempty"`
	LastFetchAt   *time.Time    `gorm:"column:last_fetch_at" json:"last_fetch_at,omitempty"`
	FetchCount    int64         `gorm:"type:bigint;not null;default:0;column:fetch_count" json:"fetch_count"`
	CreatedAt     time.Time     `gorm:"column:created_at" json:"created_at"`
	// HasStoryboard marks that storyboard_NNN.jpg + storyboard.vtt exist
	// under MinioPath (scrub-preview sprite track).
	HasStoryboard bool `gorm:"not null;default:false;column:has_storyboard" json:"has_storyboard"`
}

// TableName pins the table name (GORM would otherwise pluralize to
// "episodes"). The migration uses "library_episodes".
func (Episode) TableName() string { return "library_episodes" }
