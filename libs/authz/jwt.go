package authz

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Role represents a user role
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Claims represents JWT claims
type Claims struct {
	jwt.RegisteredClaims
	UserID   string `json:"uid"`
	Username string `json:"username"`
	Role     Role   `json:"role"`
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret           string        `json:"secret" yaml:"secret"`
	Issuer           string        `json:"issuer" yaml:"issuer"`
	AccessTokenTTL   time.Duration `json:"access_token_ttl" yaml:"access_token_ttl"`
	RefreshTokenTTL  time.Duration `json:"refresh_token_ttl" yaml:"refresh_token_ttl"`
}

// DefaultJWTConfig returns sensible defaults
func DefaultJWTConfig() JWTConfig {
	return JWTConfig{
		Issuer:          "animeenigma",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
}

// JWTManager handles JWT operations
type JWTManager struct {
	config JWTConfig
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(cfg JWTConfig) *JWTManager {
	return &JWTManager{config: cfg}
}

// TokenPair contains access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// GenerateTokenPair generates a new access and refresh token pair
func (m *JWTManager) GenerateTokenPair(userID, username string, role Role) (*TokenPair, error) {
	now := time.Now()

	// Access token
	accessClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.config.Issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.config.AccessTokenTTL)),
		},
		UserID:   userID,
		Username: username,
		Role:     role,
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(m.config.Secret))
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	// Refresh token
	refreshClaims := jwt.RegisteredClaims{
		Issuer:    m.config.Issuer,
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(m.config.RefreshTokenTTL)),
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(m.config.Secret))
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresAt:    now.Add(m.config.AccessTokenTTL),
	}, nil
}

// ValidateAccessToken validates an access token and returns claims
func (m *JWTManager) ValidateAccessToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(m.config.Secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token and returns the user ID
func (m *JWTManager) ValidateRefreshToken(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(m.config.Secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrTokenExpired
		}
		return "", fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return "", ErrInvalidToken
	}

	return claims.Subject, nil
}

var (
	ErrTokenExpired = errors.New("token expired")
	ErrInvalidToken = errors.New("invalid token")
)

// Context key for user claims
type claimsContextKey struct{}

// ContextWithClaims adds claims to context
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsContextKey{}, claims)
}

// ClaimsFromContext gets claims from context
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(*Claims)
	return claims, ok
}

// UserIDFromContext gets user ID from context
func UserIDFromContext(ctx context.Context) string {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return ""
	}
	return claims.UserID
}

// RoleFromContext gets role from context
func RoleFromContext(ctx context.Context) Role {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return ""
	}
	return claims.Role
}

// IsAdmin checks if the user in context is an admin
func IsAdmin(ctx context.Context) bool {
	return RoleFromContext(ctx) == RoleAdmin
}
