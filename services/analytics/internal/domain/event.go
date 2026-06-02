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
)

func (t EventType) valid() bool {
	switch t {
	case EventTypePageview, EventTypeClick, EventTypeHeartbeat, EventTypeIdentify, EventTypeCustom:
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
