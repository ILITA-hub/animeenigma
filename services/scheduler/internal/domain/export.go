package domain

import (
	"time"
)

// ExportJobStatus represents the status of an export job
type ExportJobStatus string

const (
	ExportStatusPending    ExportJobStatus = "pending"
	ExportStatusProcessing ExportJobStatus = "processing"
	ExportStatusCompleted  ExportJobStatus = "completed"
	ExportStatusFailed     ExportJobStatus = "failed"
	ExportStatusCancelled  ExportJobStatus = "cancelled"
)

// TaskStatus represents the status of an anime load task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusSkipped    TaskStatus = "skipped"
	TaskStatusManual     TaskStatus = "manual" // Requires user intervention
)

// ResolutionMethod indicates how MAL ID was resolved to Shikimori ID
type ResolutionMethod string

const (
	ResolutionExactJapanese  ResolutionMethod = "exact_japanese"
	ResolutionExactRomanized ResolutionMethod = "exact_romanized"
	ResolutionUserSelected   ResolutionMethod = "user_selected"
	ResolutionNotFound       ResolutionMethod = "not_found"
	ResolutionCached         ResolutionMethod = "cached"
)

// MappingSource indicates where the MAL-Shikimori mapping came from
type MappingSource string

const (
	MappingSourceShikimoriAPI MappingSource = "shikimori_api"
	MappingSourceTitleSearch  MappingSource = "title_search"
	MappingSourceManual       MappingSource = "manual"
)

// ExportJob tracks the overall progress of a MAL export
type ExportJob struct {
	ID             string          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         string          `gorm:"type:uuid;not null" json:"user_id"`
	MALUsername    string          `gorm:"column:mal_username;size:255;not null" json:"mal_username"`
	Status         ExportJobStatus `gorm:"size:20;default:pending" json:"status"`
	TotalAnime     int             `gorm:"default:0" json:"total_anime"`
	ProcessedAnime int             `gorm:"default:0" json:"processed_anime"`
	LoadedAnime    int             `gorm:"default:0" json:"loaded_anime"`
	SkippedAnime   int             `gorm:"default:0" json:"skipped_anime"`
	FailedAnime    int             `gorm:"default:0" json:"failed_anime"`
	ErrorMessage   string          `gorm:"type:text" json:"error_message,omitempty"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// TableName returns the table name for ExportJob
func (ExportJob) TableName() string {
	return "mal_export_jobs"
}

// IsActive returns true if the export is still running
func (e *ExportJob) IsActive() bool {
	return e.Status == ExportStatusPending || e.Status == ExportStatusProcessing
}

// ProgressPercent returns the completion percentage
func (e *ExportJob) ProgressPercent() float64 {
	if e.TotalAnime == 0 {
		return 0
	}
	return float64(e.ProcessedAnime) / float64(e.TotalAnime) * 100
}

// AnimeLoadTask represents a single anime to be loaded from Shikimori
type AnimeLoadTask struct {
	ID                  string           `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ExportJobID         string           `gorm:"type:uuid" json:"export_job_id"`
	UserID              string           `gorm:"type:uuid;not null" json:"user_id"`
	MALID               int              `gorm:"column:mal_id;not null" json:"mal_id"`
	MALTitle            string           `gorm:"column:mal_title;size:500;not null" json:"mal_title"`
	MALTitleJapanese    string           `gorm:"column:mal_title_japanese;size:500" json:"mal_title_japanese,omitempty"`
	MALTitleEnglish     string           `gorm:"column:mal_title_english;size:500" json:"mal_title_english,omitempty"`
	Status              TaskStatus       `gorm:"size:20;default:pending" json:"status"`
	Priority            int              `gorm:"default:0" json:"priority"`
	AttemptCount        int              `gorm:"default:0" json:"attempt_count"`
	MaxAttempts         int              `gorm:"default:3" json:"max_attempts"`
	LastError           string           `gorm:"type:text" json:"last_error,omitempty"`
	NextRetryAt         *time.Time       `json:"next_retry_at,omitempty"`
	ResolvedShikimoriID string           `gorm:"size:50" json:"resolved_shikimori_id,omitempty"`
	ResolvedAnimeID     string           `gorm:"type:uuid" json:"resolved_anime_id,omitempty"`
	ResolutionMethod    ResolutionMethod `gorm:"size:20" json:"resolution_method,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`

	// Associations
	ExportJob *ExportJob `gorm:"foreignKey:ExportJobID" json:"-"`
}

// TableName returns the table name for AnimeLoadTask
func (AnimeLoadTask) TableName() string {
	return "anime_load_tasks"
}

// CanRetry returns true if the task can be retried
func (t *AnimeLoadTask) CanRetry() bool {
	return t.AttemptCount < t.MaxAttempts && t.Status != TaskStatusCompleted
}

// ShouldProcess returns true if the task is ready to be processed
func (t *AnimeLoadTask) ShouldProcess() bool {
	if t.Status != TaskStatusPending {
		return false
	}
	if t.NextRetryAt != nil && time.Now().Before(*t.NextRetryAt) {
		return false
	}
	return true
}

// MALShikimoriMapping caches the relationship between MAL and Shikimori IDs
type MALShikimoriMapping struct {
	MALID       int           `gorm:"column:mal_id;primaryKey" json:"mal_id"`
	ShikimoriID string        `gorm:"column:shikimori_id;size:50;not null" json:"shikimori_id"`
	AnimeID     string        `gorm:"type:uuid" json:"anime_id,omitempty"`
	Confidence  float64       `gorm:"type:decimal(3,2);default:1.0" json:"confidence"`
	Source      MappingSource `gorm:"size:20;not null" json:"source"`
	CreatedAt   time.Time     `json:"created_at"`
}

// TableName returns the table name for MALShikimoriMapping
func (MALShikimoriMapping) TableName() string {
	return "mal_shikimori_mapping"
}

// CreateExportJobRequest is the request to create a new export job
type CreateExportJobRequest struct {
	UserID      string `json:"user_id"`
	MALUsername string `json:"mal_username"`
}

// CreateTasksRequest is the request to create anime load tasks
type CreateTasksRequest struct {
	ExportJobID string           `json:"export_job_id"`
	UserID      string           `json:"user_id"`
	Tasks       []AnimeTaskInput `json:"tasks"`
	Priority    int              `json:"priority,omitempty"`
}

// AnimeTaskInput represents a single anime to be loaded
type AnimeTaskInput struct {
	MALID         int    `json:"mal_id"`
	Title         string `json:"title"`
	TitleJapanese string `json:"title_japanese,omitempty"`
	TitleEnglish  string `json:"title_english,omitempty"`
}

// ExportJobResponse is the API response for export job status
type ExportJobResponse struct {
	ID              string          `json:"id"`
	MALUsername     string          `json:"mal_username"`
	Status          ExportJobStatus `json:"status"`
	TotalAnime      int             `json:"total_anime"`
	ProcessedAnime  int             `json:"processed_anime"`
	LoadedAnime     int             `json:"loaded_anime"`
	SkippedAnime    int             `json:"skipped_anime"`
	FailedAnime     int             `json:"failed_anime"`
	ProgressPercent float64         `json:"progress_percent"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

// ToResponse converts ExportJob to ExportJobResponse
func (e *ExportJob) ToResponse() *ExportJobResponse {
	return &ExportJobResponse{
		ID:              e.ID,
		MALUsername:     e.MALUsername,
		Status:          e.Status,
		TotalAnime:      e.TotalAnime,
		ProcessedAnime:  e.ProcessedAnime,
		LoadedAnime:     e.LoadedAnime,
		SkippedAnime:    e.SkippedAnime,
		FailedAnime:     e.FailedAnime,
		ProgressPercent: e.ProgressPercent(),
		ErrorMessage:    e.ErrorMessage,
		StartedAt:       e.StartedAt,
		CompletedAt:     e.CompletedAt,
		CreatedAt:       e.CreatedAt,
	}
}

// TaskStats represents statistics for tasks
type TaskStats struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	Skipped    int `json:"skipped"`
}
