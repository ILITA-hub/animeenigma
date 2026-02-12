package domain

import (
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID             string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username       string         `gorm:"size:32;uniqueIndex" json:"username"`
	PasswordHash   string         `gorm:"size:255" json:"-"`
	TelegramID     *int64         `gorm:"uniqueIndex" json:"telegram_id,omitempty"`
	PublicID       string         `gorm:"size:32;uniqueIndex" json:"public_id"`
	PublicStatuses pq.StringArray `gorm:"type:text[]" json:"public_statuses"`
	Avatar         string         `gorm:"type:text" json:"avatar,omitempty"`
	Role           authz.Role     `gorm:"size:20;default:'user'" json:"role"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// Session represents a user session
type Session struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID       string    `gorm:"type:uuid;index" json:"user_id"`
	RefreshToken string    `json:"-"`
	UserAgent    string    `json:"user_agent"`
	IP           string    `json:"ip"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=32,alphanum"`
	Password string `json:"password" validate:"required,min=8,max=128"`
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

// TelegramLoginRequest represents Telegram Login Widget data
type TelegramLoginRequest struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
	PhotoURL  string `json:"photo_url,omitempty"`
	AuthDate  int64  `json:"auth_date"`
	Hash      string `json:"hash"`
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

// UpdateAvatarRequest represents a request to change the user's avatar
type UpdateAvatarRequest struct {
	Avatar string `json:"avatar" validate:"required"`
}

// PublicUser represents a user visible to other users
type PublicUser struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	PublicID       string    `json:"public_id"`
	PublicStatuses []string  `json:"public_statuses"`
	Avatar         string    `json:"avatar,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// ToPublic converts a User to PublicUser
func (u *User) ToPublic() *PublicUser {
	return &PublicUser{
		ID:             u.ID,
		Username:       u.Username,
		PublicID:       u.PublicID,
		PublicStatuses: u.PublicStatuses,
		Avatar:         u.Avatar,
		CreatedAt:      u.CreatedAt,
	}
}
