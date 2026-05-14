package domain

import "time"

// UserSession is one persistent login. Created on login/register, rotated on
// every /auth/refresh, revoked on logout or via the settings UI.
//
// `RefreshTokenHash` and `PreviousRefreshTokenHash` are sha256-hex of the
// opaque `rt_<64-hex>` refresh token. The previous hash is accepted during
// the grace window to absorb cross-tab refresh races.
type UserSession struct {
	ID                       string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID                   string     `gorm:"type:uuid;not null;index:idx_user_sessions_user_id" json:"user_id"`
	RefreshTokenHash         string     `gorm:"type:char(64);not null;uniqueIndex:idx_user_sessions_rt_hash" json:"-"`
	PreviousRefreshTokenHash *string    `gorm:"type:char(64);index:idx_user_sessions_prev_rt_hash" json:"-"`
	GraceUntil               *time.Time `json:"-"`
	UserAgent                string     `gorm:"type:text;not null;default:''" json:"user_agent"`
	IP                       string     `gorm:"type:text;default:''" json:"ip"` // text not inet — keeps GORM portable; valid IPv4/IPv6 strings only
	CreatedAt                time.Time  `json:"created_at"`
	LastSeenAt               time.Time  `json:"last_seen_at"`
	ExpiresAt                time.Time  `gorm:"not null;index:idx_user_sessions_expires_at" json:"expires_at"`
	RevokedAt                *time.Time `json:"revoked_at,omitempty"`
}

func (UserSession) TableName() string { return "user_sessions" }

// IsAlive reports whether the session can be used for refresh.
func (s *UserSession) IsAlive(now time.Time) bool {
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}
