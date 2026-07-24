// Package domain holds the GORM models and DTOs for the notifications service.
//
// v1.0 Notifications Engine — workstream notifications, Phase 1.
// Reference design doc: docs/superpowers/specs/2026-05-11-notifications-engine-design.md
package domain

import (
	"time"

	"gorm.io/datatypes"
)

// NotificationType enumerates the kinds of notifications the engine emits.
// v1.0 ships only "new_episode"; later phases add more types (e.g.
// "ongoing_resumed", "system_message") without altering the schema.
type NotificationType string

const (
	// TypeNewEpisode signals that one or more watched provider/team combos
	// detected new episodes for an anime. Detector output is grouped per
	// user + anime; the payload retains one representative combo for source
	// display while its episode range covers the grouped result.
	TypeNewEpisode NotificationType = "new_episode"

	// Feedback triage loop (AUTO-417): the player service emits one
	// notification per triage stage of a user-submitted feedback report.
	// Each stage invalidates the previous stage's unread notification for
	// the same report (via UpsertRequest.InvalidateDedupeKeys). Payload
	// shape for all three: FeedbackStatusPayload.

	// TypeFeedbackCreated — "we received your feedback and opened a task".
	TypeFeedbackCreated NotificationType = "feedback_created"
	// TypeFeedbackInProgress — "the robot started working on your task".
	TypeFeedbackInProgress NotificationType = "feedback_in_progress"
	// TypeFeedbackAIDone — "the robot finished, thanks for the feedback".
	TypeFeedbackAIDone NotificationType = "feedback_ai_done"
)

// UserNotification is the per-user notification row.
//
// Schema notes (design doc §Data Model):
//   - payload is JSONB so the same table can carry every NotificationType.
//   - read_at / dismissed_at / deleted_at / clicked_at are nullable timestamps
//     acting as state flags AND telemetry. dismissed_at ("cleared from the
//     bell, still visible in history") is distinct from deleted_at ("removed
//     from history too — the user hit the bin in the All-notifications modal").
//   - The two partial indexes GORM cannot express are created by
//     repo.EnsureIndexes after AutoMigrate:
//   - uk_user_dedupe UNIQUE (user_id, dedupe_key) WHERE dismissed_at IS NULL
//     AND deleted_at IS NULL (lets a fresh new_episode re-fire after the user
//     dismisses OR deletes the previous)
//   - idx_user_unread (user_id, created_at DESC) WHERE dismissed_at IS NULL
//     AND invalidated_at IS NULL AND deleted_at IS NULL (bell/dropdown path)
type UserNotification struct {
	ID            string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        string         `gorm:"type:uuid;not null;index" json:"user_id"`
	Type          string         `gorm:"size:32;not null;index" json:"type"`
	DedupeKey     string         `gorm:"size:255;not null" json:"dedupe_key"`
	Payload       datatypes.JSON `gorm:"type:jsonb;not null" json:"payload"`
	ReadAt        *time.Time     `json:"read_at"`
	DismissedAt   *time.Time     `gorm:"index" json:"dismissed_at"`
	InvalidatedAt *time.Time     `gorm:"index" json:"invalidated_at"`
	// DeletedAt is a plain lifecycle stamp (NOT gorm.DeletedAt — this service
	// manages every visibility filter explicitly, like dismissed_at, and must
	// never let GORM auto-scope Upsert/Get/cleanup). Set when the user deletes
	// a notification from the history modal; excluded from every list surface.
	DeletedAt *time.Time `gorm:"index" json:"deleted_at"`
	ClickedAt *time.Time `json:"clicked_at"`
	CreatedAt time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// TableName pins the table name so it does not depend on GORM's pluralization
// heuristic. (GORM would derive "user_notifications" either way; explicit
// override insulates the schema from future GORM changes.)
func (UserNotification) TableName() string { return "user_notifications" }

// FeedbackStatusPayload is the JSON shape stored in UserNotification.Payload
// for the three feedback_* types. ReportID is the on-disk report id
// (`{ts}_{user}_{type}` — see services/player report.go); Description is a
// truncated snippet of the user's original feedback text so the card can
// remind them which report this is about.
type FeedbackStatusPayload struct {
	ReportID    string `json:"report_id"`
	Category    string `json:"category,omitempty"` // bug | issue | feature
	Description string `json:"description,omitempty"`
	Status      string `json:"status"` // created | in_progress | ai_done
}

// NewEpisodePayload is the JSON shape stored in UserNotification.Payload
// when Type == TypeNewEpisode. Mirrors the design-doc payload spec.
// All fields lowercase_snake_case per the project's JSON convention.
type NewEpisodePayload struct {
	AnimeID                string `json:"anime_id"`
	ShikimoriID            string `json:"shikimori_id,omitempty"`
	AnimeTitle             string `json:"anime_title"`
	AnimePosterURL         string `json:"anime_poster_url,omitempty"`
	FirstUnwatchedEpisode  int    `json:"first_unwatched_episode"`
	LatestAvailableEpisode int    `json:"latest_available_episode"`
	Player                 string `json:"player"`
	Language               string `json:"language"`
	WatchType              string `json:"watch_type"`
	TranslationID          string `json:"translation_id"`
	TranslationTitle       string `json:"translation_title,omitempty"`
	WatchURL               string `json:"watch_url"`
}
