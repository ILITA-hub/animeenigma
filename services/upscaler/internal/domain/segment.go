package domain

import "time"

type SegmentStatus string

const (
	SegPending SegmentStatus = "pending"
	SegLeased  SegmentStatus = "leased"
	SegDone    SegmentStatus = "done"
)

type UpscaleSegment struct {
	JobID          string        `gorm:"type:uuid;primaryKey;column:job_id" json:"job_id"`
	Idx            int           `gorm:"type:int;primaryKey;column:idx" json:"idx"`
	Status         SegmentStatus `gorm:"type:text;not null;default:'pending';index;column:status" json:"status"`
	LeaseExpiresAt *time.Time    `gorm:"column:lease_expires_at" json:"lease_expires_at,omitempty"`
	WorkerID       string        `gorm:"type:text;column:worker_id" json:"worker_id,omitempty"`
	InBytes        int64         `gorm:"type:bigint;not null;default:0;column:in_bytes" json:"in_bytes"`
	OutBytes       int64         `gorm:"type:bigint;not null;default:0;column:out_bytes" json:"out_bytes"`
	StartedAt      *time.Time    `gorm:"column:started_at" json:"started_at,omitempty"`
	CompletedAt    *time.Time    `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (UpscaleSegment) TableName() string {
	return "upscale_segments"
}
