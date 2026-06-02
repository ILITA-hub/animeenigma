// Package repo holds the Postgres-backed EventStore and the GORM models.
package repo

import (
	"time"

	"gorm.io/gorm"
)

// Event is the GORM model for the analytics_events table. JSON-shaped
// columns (el_attrs, properties) are stored as text holding JSON for
// portability (sqlite tests + postgres prod); they can be ALTERed to
// jsonb later if JSON operators are needed.
type Event struct {
	EventID     string    `gorm:"column:event_id;primaryKey"`
	EventType   string    `gorm:"column:event_type;index:idx_evt_type_ts,priority:1"`
	EventName   string    `gorm:"column:event_name;default:''"`
	AnonymousID string    `gorm:"column:anonymous_id;index:idx_anon_ts,priority:1"`
	UserID      *string   `gorm:"column:user_id"`
	SessionID   string    `gorm:"column:session_id;index"`
	Timestamp   time.Time `gorm:"column:timestamp;index:idx_anon_ts,priority:2;index:idx_ts"`
	ReceivedAt  time.Time `gorm:"column:received_at"`

	URL      string `gorm:"column:url"`
	Path     string `gorm:"column:path"`
	Referrer string `gorm:"column:referrer;default:''"`
	Title    string `gorm:"column:title;default:''"`

	ElSelector string `gorm:"column:el_selector"`
	ElText     string `gorm:"column:el_text"`
	ElTag      string `gorm:"column:el_tag"`
	ElAttrs    string `gorm:"column:el_attrs;type:text;default:'{}'"`

	ActiveMS int `gorm:"column:active_ms"`

	UserAgent  string `gorm:"column:user_agent;default:''"`
	DeviceType string `gorm:"column:device_type;default:''"`
	ScreenW    int    `gorm:"column:screen_w;default:0"`
	ScreenH    int    `gorm:"column:screen_h;default:0"`
	IPHash     string `gorm:"column:ip_hash;default:''"`

	TraceID    string `gorm:"column:trace_id"`
	Properties string `gorm:"column:properties;type:text;default:'{}'"`
}

func (Event) TableName() string { return "analytics_events" }

// Identity maps an anonymous_id to a user_id at a point in time. Append
// only; the latest row per anonymous_id wins (see EnsureView).
type Identity struct {
	ID          uint      `gorm:"primaryKey"`
	AnonymousID string    `gorm:"column:anonymous_id;index:idx_ident_anon_ts,priority:1"`
	UserID      string    `gorm:"column:user_id"`
	Timestamp   time.Time `gorm:"column:timestamp;index:idx_ident_anon_ts,priority:2"`
}

func (Identity) TableName() string { return "analytics_identities" }

// AutoMigrateAll creates the service-owned tables.
func AutoMigrateAll(db *gorm.DB) error {
	return db.AutoMigrate(&Event{}, &Identity{})
}

// EnsureView creates analytics_events_resolved, which adds resolved_user_id
// and person_id (canonical identity: user if known, else anonymous). Uses a
// correlated subquery so it runs identically on sqlite and postgres.
func EnsureView(db *gorm.DB) error {
	return db.Exec(`CREATE VIEW IF NOT EXISTS analytics_events_resolved AS
SELECT e.*,
  COALESCE(e.user_id,
    (SELECT i.user_id FROM analytics_identities i
      WHERE i.anonymous_id = e.anonymous_id ORDER BY i.timestamp DESC LIMIT 1)
  ) AS resolved_user_id,
  COALESCE(e.user_id,
    (SELECT i.user_id FROM analytics_identities i
      WHERE i.anonymous_id = e.anonymous_id ORDER BY i.timestamp DESC LIMIT 1),
    e.anonymous_id
  ) AS person_id
FROM analytics_events e`).Error
}
