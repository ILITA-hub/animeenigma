package domain

import (
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// ActivityVisibility values — what of the user's activity other users can
// see (global activity feed + public watchlist/stats). The value itself is
// NEVER exposed on PublicUser: revealing "non_hentai" would reveal that the
// user watches hentai, defeating the setting.
const (
	ActivityVisibilityAll       = "all"        // default — current behaviour
	ActivityVisibilityNonHentai = "non_hentai" // 18+ titles excluded from public activity
	ActivityVisibilityNone      = "none"       // no publicly visible activity at all
)

// ValidActivityVisibility reports whether v is one of the allowed values.
func ValidActivityVisibility(v string) bool {
	return v == ActivityVisibilityAll || v == ActivityVisibilityNonHentai || v == ActivityVisibilityNone
}

// ShowcaseState values — the cheap visibility signal carried on PublicUser so
// the FE knows whether to show the profile showcase tab WITHOUT fetching the
// (player-owned) showcase blocks. Derived by a co-located in-DB read of the
// player-owned profile_showcases table (see UserRepository.GetShowcaseState).
const (
	ShowcaseStateNone    = "none"    // no showcase row, or blocks empty
	ShowcaseStateHidden  = "hidden"  // has content but enabled = false
	ShowcaseStateVisible = "visible" // enabled = true (player coerces enabled ⟹ non-empty)
)

// User represents a user in the system
type User struct {
	ID           string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username     string `gorm:"size:32;uniqueIndex" json:"username"`
	PasswordHash string `gorm:"size:255" json:"-"`
	TelegramID   *int64 `gorm:"uniqueIndex" json:"telegram_id,omitempty"`
	// Telegram display identity, refreshed on every Telegram login (spec
	// 2026-07-24 admin users page). Distinct from Username, which is the
	// 32-char unique login handle (derived + de-duplicated). Nullable — blank
	// for existing users until their next Telegram login.
	TelegramUsername  *string        `gorm:"size:64" json:"telegram_username,omitempty"`
	TelegramFirstName *string        `gorm:"size:128" json:"telegram_first_name,omitempty"`
	PublicID          string         `gorm:"size:32;uniqueIndex" json:"public_id"`
	PublicStatuses    pq.StringArray `gorm:"type:text[]" json:"public_statuses"`
	// ActivityVisibility is enforced server-side by the player service
	// (activity feed + public watchlist reads) — see
	// docs/superpowers/specs/2026-06-12-activity-visibility-design.md.
	ActivityVisibility string  `gorm:"size:20;default:'all'" json:"activity_visibility"`
	Avatar             string  `gorm:"type:text" json:"avatar,omitempty"`
	Timezone           string  `gorm:"size:64" json:"timezone,omitempty"`
	ApiKeyHash         *string `gorm:"size:64;uniqueIndex" json:"-"`
	// CertAutoLogin: when true, a valid client-cert handshake on the mTLS
	// vhost silently logs this user in (spec 2026-07-24). Server-side SSoT.
	// Default ON (owner decision 2026-07-24): issuing a cert means the user
	// wants silent login; the toggle exists to opt OUT. GORM omits zero-value
	// fields that carry a default tag on insert, so new rows get the DB
	// default (true). Existing rows were backfilled to true via SQL.
	CertAutoLogin bool           `gorm:"default:true" json:"cert_auto_login"`
	Role          authz.Role     `gorm:"size:20;default:'user'" json:"role"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=32,alphanum"`
	Password string `json:"password" validate:"required,min=8,max=72"`
	// Browser-detected IANA timezone, captured once at sign-up; afterwards
	// changeable only via settings (PUT /auth/profile/timezone). Invalid
	// values are silently dropped — never fail a registration over tz.
	Timezone string `json:"timezone,omitempty" validate:"omitempty,max=64"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// RefreshRequest represents a token refresh request
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// TelegramWebhookUser represents user data received from Telegram bot webhook
type TelegramWebhookUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// DeepLinkResponse is returned when creating a new deep link auth token
type DeepLinkResponse struct {
	Token       string `json:"token"`
	DeepLinkURL string `json:"deeplink_url"`
	ExpiresIn   int    `json:"expires_in"`
}

// DeepLinkCheckResponse is returned when polling deep link auth status
type DeepLinkCheckResponse struct {
	Status      string     `json:"status"`
	AccessToken string     `json:"access_token,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	User        *User      `json:"user,omitempty"`
}

// TelegramAuthSession is stored in Redis during the deep link auth flow
type TelegramAuthSession struct {
	Status     string `json:"status"`
	TelegramID int64  `json:"telegram_id,omitempty"`
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	Username   string `json:"username,omitempty"`

	// NonceHash binds the pending session to the browser that minted the token
	// (vector A: token theft). It is the sha256-hex of a one-time nonce handed
	// to that browser as an HttpOnly cookie; CheckDeepLinkToken refuses to hand
	// out credentials unless the polling request presents the matching cookie.
	// Empty on sessions minted before browser-binding existed (treated as
	// unbound so an in-flight login survives a rolling deploy).
	NonceHash string `json:"nonce_hash,omitempty"`

	// RequestIP / RequestUA record the client that requested the login at mint
	// time. They are surfaced in the bot's Confirm-login prompt (vector B:
	// attacker-initiated flow) so a victim asked to confirm a login they did
	// not start sees an unfamiliar device/IP and can decline.
	RequestIP string `json:"request_ip,omitempty"`
	RequestUA string `json:"request_ua,omitempty"`
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"-"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         *User     `json:"user"`
}

// PublicAuthResponse is the response sent to the client (without refresh token)
type PublicAuthResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	User        *User     `json:"user"`
}

// ToPublicResponse converts AuthResponse to PublicAuthResponse
func (r *AuthResponse) ToPublicResponse() *PublicAuthResponse {
	return &PublicAuthResponse{
		AccessToken: r.AccessToken,
		ExpiresAt:   r.ExpiresAt,
		User:        r.User,
	}
}

// UpdateUserRequest represents a user update request
type UpdateUserRequest struct {
	Username        *string `json:"username,omitempty"`
	CurrentPassword *string `json:"current_password,omitempty"`
	NewPassword     *string `json:"new_password,omitempty"`
}

// UpdatePublicIDRequest represents a request to change public_id
type UpdatePublicIDRequest struct {
	PublicID string `json:"public_id" validate:"required,min=3,max=32,alphanum"`
}

// UpdatePrivacyRequest represents a request to change public_statuses
type UpdatePrivacyRequest struct {
	PublicStatuses []string `json:"public_statuses" validate:"required"`
}

// UpdateActivityVisibilityRequest represents a request to change activity_visibility
type UpdateActivityVisibilityRequest struct {
	ActivityVisibility string `json:"activity_visibility" validate:"required"`
}

// UpdateAvatarRequest represents a request to change the user's avatar
type UpdateAvatarRequest struct {
	Avatar string `json:"avatar" validate:"required"`
}

// UpdateTimezoneRequest represents a request to change the user's timezone
type UpdateTimezoneRequest struct {
	Timezone string `json:"timezone" validate:"required,max=64"`
}

// ApiKeyResponse is returned when generating an API key
type ApiKeyResponse struct {
	ApiKey string `json:"api_key"`
}

// ResolveApiKeyRequest is the request body for the internal resolve endpoint
type ResolveApiKeyRequest struct {
	ApiKey string `json:"api_key" validate:"required"`
}

// PublicUser represents a user visible to other users
type PublicUser struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	PublicID       string    `json:"public_id"`
	PublicStatuses []string  `json:"public_statuses"`
	Avatar         string    `json:"avatar,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	// ShowcaseState is a co-located denormalization of the player-owned
	// profile_showcases table, set by the service layer (ToPublic has no DB
	// access, so it leaves this as the zero value "" — the service overwrites
	// it). One of ShowcaseStateNone/Hidden/Visible.
	ShowcaseState string `json:"showcase_state"`
}

// ToPublic converts a User to PublicUser. When the user hid all activity,
// public_statuses comes back empty so the profile page renders its existing
// "no public lists" state — without exposing the setting value itself.
func (u *User) ToPublic() *PublicUser {
	statuses := []string(u.PublicStatuses)
	if u.ActivityVisibility == ActivityVisibilityNone {
		statuses = []string{}
	}
	return &PublicUser{
		ID:             u.ID,
		Username:       u.Username,
		PublicID:       u.PublicID,
		PublicStatuses: statuses,
		Avatar:         u.Avatar,
		CreatedAt:      u.CreatedAt,
	}
}
