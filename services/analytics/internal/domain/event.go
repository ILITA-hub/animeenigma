// Package domain holds the analytics clickstream event model and the
// EventStore port. Events are produced by the browser snippet (Plan 2),
// validated here, and persisted via an EventStore implementation.
package domain

import (
	"fmt"
	"time"
)

// EventType enumerates the clickstream event kinds the snippet emits.
type EventType string

const (
	EventTypePageview  EventType = "pageview"
	EventTypeClick     EventType = "click"
	EventTypeHeartbeat EventType = "heartbeat"
	EventTypeIdentify  EventType = "identify"
	EventTypeCustom    EventType = "custom"
	// EventTypePlayer is used by the /api/analytics/player-events endpoint to
	// record player telemetry (resolve outcomes + stalls). These rows are NOT
	// clickstream events — they carry effect dimensions (EffectKind, Target,
	// DurationMS) and bypass Validate() in the handler, but the type is declared
	// here so it can be used as a typed constant.
	EventTypePlayer EventType = "player"
)

func (t EventType) valid() bool {
	switch t {
	case EventTypePageview, EventTypeClick, EventTypeHeartbeat, EventTypeIdentify, EventTypeCustom,
		EventTypePlayer:
		return true
	default:
		return false
	}
}

// Event is one clickstream event. anonymous_id and session_id are always
// present; user_id is set only once the visitor is identified.
type Event struct {
	EventID     string
	EventType   EventType
	EventName   string
	AnonymousID string
	UserID      string // empty when unknown
	SessionID   string
	Timestamp   time.Time
	ReceivedAt  time.Time

	URL      string
	Path     string
	Referrer string
	Title    string

	ElSelector string
	ElText     string
	ElTag      string
	ElAttrs    string // JSON string, default "{}"

	ActiveMS int // heartbeat foreground time

	UserAgent  string
	DeviceType string
	ScreenW    int
	ScreenH    int
	IPHash     string

	TraceID    string
	Properties string // JSON string, default "{}"

	// --- Activity-register effect dimensions (v4.0 Phase 2) ---
	// These describe an effect row (one row per outbound effect: egress / db /
	// cache). Clickstream rows leave them empty/zero and InsertBatch falls back
	// to the historical defaults (origin="api", source="be", accuracy="exact").
	Origin     string // who caused it: "api" (FE-attributed) / "be" / "cron" / ...
	Operation  string // coarse op label, e.g. "catalog GET /api/anime/{id}"
	EffectKind string // "egress" | "db" | "cache" | "" (clickstream)
	TargetKind string // "host" | "table" | "key" | ...
	Target     string // the concrete target: host, table name, cache key family
	Source     string // attribution accuracy origin: "be" | "fe" | ...
	Accuracy   string // "exact" | "approximate"
	AnimeID    string // optional correlation; empty → NULL

	// --- Activity-register effect measures (v4.0 Phase 2) ---
	Requests   int // count of underlying requests this row aggregates (default 1)
	BytesIn    int // response/read bytes
	BytesOut   int // request/write bytes
	DurationMS int // wall-clock duration of the effect
	RowCount   int // DB rows touched (effect_kind="db")
}

// Validate enforces the always-present fields and a known event_type.
func (e Event) Validate() error {
	if e.AnonymousID == "" {
		return fmt.Errorf("anonymous_id is required")
	}
	if e.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if !e.EventType.valid() {
		return fmt.Errorf("unknown event_type: %q", e.EventType)
	}
	return nil
}
