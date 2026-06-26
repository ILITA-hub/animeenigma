package domain

import "time"

type UpscaleWorker struct {
	WorkerID         string     `gorm:"type:text;primaryKey;column:worker_id" json:"worker_id"`
	GPUInfo          string     `gorm:"type:text;column:gpu_info" json:"gpu_info,omitempty"`
	ImageVersion     string     `gorm:"type:text;column:image_version" json:"image_version,omitempty"`
	ModelsAvailable  string     `gorm:"type:text;column:models_available" json:"models_available,omitempty"` // csv
	Status           string     `gorm:"type:text;not null;default:'idle';column:status" json:"status"`           // idle|busy|draining|gone
	CurrentJobID     string     `gorm:"type:text;column:current_job_id" json:"current_job_id,omitempty"`
	CurrentSegment   int        `gorm:"type:int;column:current_segment" json:"current_segment"`
	SessionExpiresAt *time.Time `gorm:"column:session_expires_at" json:"session_expires_at,omitempty"`
	LastHeartbeatAt  *time.Time `gorm:"column:last_heartbeat_at" json:"last_heartbeat_at,omitempty"`
	CreatedAt        time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (UpscaleWorker) TableName() string {
	return "upscale_workers"
}
