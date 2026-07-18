package domain

import "time"

// UserFollow stores a directional subscription between two users.
type UserFollow struct {
	FollowerID string    `gorm:"type:uuid;primaryKey" json:"follower_id"`
	FollowedID string    `gorm:"type:uuid;primaryKey;index" json:"followed_id"`
	CreatedAt  time.Time `gorm:"not null;index" json:"created_at"`
}

func (UserFollow) TableName() string { return "user_follows" }

// FollowedUser is the public profile projection used by the following page.
type FollowedUser struct {
	ID         string    `json:"id"`
	Username   string    `json:"username"`
	PublicID   string    `json:"public_id,omitempty"`
	Avatar     string    `json:"avatar,omitempty"`
	FollowedAt time.Time `json:"followed_at"`
}
