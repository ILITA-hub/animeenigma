package domain

import "time"

type JobStatus string

const (
	JobQueued     JobStatus = "queued"
	JobSegmenting JobStatus = "segmenting"
	JobUpscaling  JobStatus = "upscaling"
	JobFinalizing JobStatus = "finalizing"
	JobDone       JobStatus = "done"
	JobFailed     JobStatus = "failed"
	JobCancelled  JobStatus = "cancelled"
)

func (s JobStatus) IsTerminal() bool {
	switch s {
	case JobDone, JobFailed, JobCancelled:
		return true
	}
	return false
}

type UpscaleJob struct {
	ID            string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid();column:id" json:"id"`
	ShikimoriID   string     `gorm:"type:text;not null;index;column:shikimori_id" json:"shikimori_id"`
	Episode       int        `gorm:"type:int;not null;column:episode" json:"episode"`
	// LibraryInfohash is the anacrolix torrent infohash for the source file
	// on the library_torrents volume. Set by the admin trigger (CD-6).
	// Used by source.Resolver to locate {TorrentsDir}/{LibraryInfohash}/...
	LibraryInfohash string    `gorm:"type:text;column:library_infohash" json:"library_infohash,omitempty"`
	Model         string     `gorm:"type:text;not null;column:model" json:"model"`
	Scale         int        `gorm:"type:int;not null;default:2;column:scale" json:"scale"`
	Status        JobStatus  `gorm:"type:text;not null;default:'queued';index;column:status" json:"status"`
	ProgressPct   int        `gorm:"type:int;not null;default:0;column:progress_pct" json:"progress_pct"`
	SourceCodec   string     `gorm:"type:text;column:source_codec" json:"source_codec,omitempty"`
	SourcePixFmt  string     `gorm:"type:text;column:source_pixfmt" json:"source_pixfmt,omitempty"`
	SourceFPS     string     `gorm:"type:text;column:source_fps" json:"source_fps,omitempty"`
	SegmentCount  int        `gorm:"type:int;not null;default:0;column:segment_count" json:"segment_count"`
	OutputPrefix  string     `gorm:"type:text;column:output_prefix" json:"output_prefix,omitempty"`
	ErrorText     string     `gorm:"type:text;column:error_text" json:"error_text,omitempty"`
	CreatedAt     time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at" json:"updated_at"`
	CompletedAt   *time.Time `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (UpscaleJob) TableName() string {
	return "upscale_jobs"
}
