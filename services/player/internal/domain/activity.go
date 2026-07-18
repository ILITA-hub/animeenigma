package domain

import (
	"time"

	"gorm.io/gorm"
)

type ActivityEvent struct {
	ID       string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID   string `gorm:"type:uuid;index" json:"user_id"`
	Username string `gorm:"size:32" json:"username"`
	PublicID string `gorm:"-" json:"public_id,omitempty"`
	// UserAvatar is not a column — it is populated at read time from the
	// users table (see ActivityRepository.GetFeed) so the feed shows the
	// commenter's current avatar without denormalizing (which would go stale).
	UserAvatar string         `gorm:"-" json:"user_avatar,omitempty"`
	AnimeID    string         `gorm:"type:uuid;index" json:"anime_id"`
	Anime      *AnimeInfo     `gorm:"foreignKey:AnimeID" json:"anime,omitempty"`
	Type       string         `gorm:"size:20;index" json:"type"`
	OldValue   string         `gorm:"size:50" json:"old_value"`
	NewValue   string         `gorm:"size:50" json:"new_value"`
	Content    string         `gorm:"type:text" json:"content,omitempty"`
	CreatedAt  time.Time      `gorm:"index" json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ActivityEvent) TableName() string { return "activity_events" }
