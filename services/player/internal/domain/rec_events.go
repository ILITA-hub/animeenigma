package domain

import (
	"time"

	"gorm.io/gorm"
)

// RecEvent is one row in the eval pipeline's append-only audit log.
// Phase 14 (REC-EVAL-01). Persisted on every POST /api/events/rec call;
// queried by Postgres for ad-hoc analysis (e.g., per-anime CTR breakdown
// deferred to v2.1). Indexed on (user_id, created_at desc) for per-user
// history and (signal_id, event_type, created_at) for the rate panels.
//
// Anonymous events (anonymous trending row CTR) are valid per CONTEXT.md
// §C4 — UserID is nullable.
type RecEvent struct {
	ID             string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	EventType      string         `gorm:"size:32;not null;index:idx_rec_events_signal_event_time,priority:2" json:"event_type"`
	UserID         *string        `gorm:"type:uuid;index:idx_rec_events_user_time,priority:1" json:"user_id,omitempty"`
	AnimeID        string         `gorm:"type:uuid;not null" json:"anime_id"`
	SignalID       string         `gorm:"size:32;not null;index:idx_rec_events_signal_event_time,priority:1" json:"signal_id"`
	Pinned         bool           `gorm:"not null;default:false" json:"pinned"`
	PinSource      *string        `gorm:"size:32" json:"pin_source,omitempty"`
	PinSeedAnimeID *string        `gorm:"type:uuid" json:"pin_seed_anime_id,omitempty"`
	SourceRoute    *string        `gorm:"size:128" json:"source_route,omitempty"`
	Rank           *int           `json:"rank,omitempty"`
	CreatedAt      time.Time      `gorm:"index:idx_rec_events_user_time,priority:2,sort:desc;index:idx_rec_events_signal_event_time,priority:3" json:"created_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName fixes the GORM-derived table name to "rec_events" (otherwise
// gorm pluralizes RecEvent → "rec_events" anyway, but pinning it explicitly
// avoids surprises if the struct is renamed).
func (RecEvent) TableName() string { return "rec_events" }
