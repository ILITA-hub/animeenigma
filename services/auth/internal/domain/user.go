package domain

import (
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
)

// User represents a user in the system
type User struct {
	ID           string      `db:"id" json:"id"`
	Username     string      `db:"username" json:"username"`
	PasswordHash string      `db:"password_hash" json:"-"`
	Role         authz.Role  `db:"role" json:"role"`
	CreatedAt    time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time   `db:"updated_at" json:"updated_at"`
	DeletedAt    *time.Time  `db:"deleted_at" json:"-"`
}

// Session represents a user session
type Session struct {
	ID           string    `db:"id" json:"id"`
	UserID       string    `db:"user_id" json:"user_id"`
	RefreshToken string    `db:"refresh_token" json:"-"`
	UserAgent    string    `db:"user_agent" json:"user_agent"`
	IP           string    `db:"ip" json:"ip"`
	ExpiresAt    time.Time `db:"expires_at" json:"expires_at"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
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

// AuthResponse represents an authentication response
type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         *User     `json:"user"`
}

// UpdateUserRequest represents a user update request
type UpdateUserRequest struct {
	Username        *string `json:"username,omitempty"`
	CurrentPassword *string `json:"current_password,omitempty"`
	NewPassword     *string `json:"new_password,omitempty"`
}

// PublicUser represents a user visible to other users
type PublicUser struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// ToPublic converts a User to PublicUser
func (u *User) ToPublic() *PublicUser {
	return &PublicUser{
		ID:        u.ID,
		Username:  u.Username,
		CreatedAt: u.CreatedAt,
	}
}
