package domain

import "time"

// RecAnnouncementDismissal records a user's permanent "don't show this
// announced title again" action from the upcoming_for_you spotlight card
// (spec 2026-07-17). One row per (user, anime); inserts are idempotent.
//
// Owned by the recs service (AutoMigrate in cmd/recs-api/main.go). Also
// reusable later as a mild negative signal and by the future announcement
// notification producer.
type RecAnnouncementDismissal struct {
	ID        string    `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    string    `gorm:"type:uuid;not null;uniqueIndex:uk_rec_ann_dismiss_user_anime,priority:1" json:"user_id"`
	AnimeID   string    `gorm:"type:uuid;not null;uniqueIndex:uk_rec_ann_dismiss_user_anime,priority:2" json:"anime_id"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName pins the table name explicitly.
func (RecAnnouncementDismissal) TableName() string { return "rec_announcement_dismissals" }
