package domain

import "time"

// UpscaleEnrollToken is a durable, server-side, single-use worker-enrollment
// token (CD-14). A token is consumed by setting ConsumedAt in an atomic
// conditional UPDATE; once consumed it can never be replayed, and because the
// store is Postgres (not Redis) the single-use state survives process/host
// restarts (this host has had OOM restarts that would lose Redis state).
type UpscaleEnrollToken struct {
	Token      string     `gorm:"type:text;primaryKey;column:token" json:"token"`
	ConsumedAt *time.Time `gorm:"column:consumed_at" json:"consumed_at,omitempty"`
	CreatedAt  time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (UpscaleEnrollToken) TableName() string {
	return "upscale_enroll_tokens"
}
