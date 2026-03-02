package domain

import "time"

type SyncJob struct {
	ID             string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         string     `gorm:"type:uuid;index" json:"user_id"`
	Source         string     `gorm:"size:20;index" json:"source"`
	SourceUsername string     `gorm:"size:100" json:"source_username"`
	Status         string     `gorm:"size:20;index;default:'processing'" json:"status"`
	Total          int        `json:"total"`
	Imported       int        `json:"imported"`
	Skipped        int        `json:"skipped"`
	ErrorMessage   string     `gorm:"type:text" json:"error_message,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (SyncJob) TableName() string { return "sync_jobs" }
