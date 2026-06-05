package domain

import "time"

// UserSession is one persistent login. Created on login/register, refreshed
// (activity-stamped) on every /auth/refresh, revoked on logout or via the
// settings UI. The refresh token is NON-ROTATING: it stays stable for the
// session's life, so there is no rotation race to absorb.
//
// `RefreshTokenHash` is the sha256-hex of the opaque `rt_<64-hex>` refresh
// token. `PreviousRefreshTokenHash`/`GraceUntil`/`GraceOpenedAt` are DORMANT
// columns kept from the old rotating design (never read/written).
type UserSession struct {
	ID                       string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID                   string     `gorm:"type:uuid;not null;index:idx_user_sessions_user_id" json:"user_id"`
	RefreshTokenHash         string     `gorm:"type:char(64);not null;uniqueIndex:idx_user_sessions_rt_hash" json:"-"`
	PreviousRefreshTokenHash *string    `gorm:"type:char(64);index:idx_user_sessions_prev_rt_hash" json:"-"` // DORMANT (non-rotating sessions): column kept, never read/written
	GraceUntil               *time.Time `json:"-"` // DORMANT (non-rotating sessions): column kept, never read/written
	GraceOpenedAt            *time.Time `json:"-"` // DORMANT (non-rotating sessions): column kept, never read/written
	UserAgent                string     `gorm:"type:text;not null;default:''" json:"user_agent"`
	IP                       string     `gorm:"type:text;not null;default:''" json:"ip"` // text not inet — keeps GORM portable; valid IPv4/IPv6 strings only
	CreatedAt                time.Time  `json:"created_at"`
	LastSeenAt               time.Time  `gorm:"not null;default:now()" json:"last_seen_at"`
	ExpiresAt                time.Time  `gorm:"not null;index:idx_user_sessions_expires_at" json:"expires_at"`
	RevokedAt                *time.Time `json:"revoked_at,omitempty"`
}

func (UserSession) TableName() string { return "user_sessions" }

// IsAlive reports whether the session can be used for refresh. With
// non-rotating sessions there is no time wall: a session lives until it is
// explicitly revoked. ExpiresAt is kept as a far-future sentinel and is not
// consulted here (the `now` arg is retained for signature stability).
func (s *UserSession) IsAlive(now time.Time) bool {
	_ = now
	return s.RevokedAt == nil
}
