package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// SanitizeOldURL constrains a caller-supplied return path to a safe SAME-ORIGIN
// relative path, defeating open-redirect. It must start with a single '/', must
// not start with '//' or '/\' (protocol-relative), must contain no scheme and no
// ASCII control chars. Anything else collapses to "/".
func SanitizeOldURL(raw string) string {
	if raw == "" || raw[0] != '/' {
		return "/"
	}
	if strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/\\") {
		return "/"
	}
	if strings.Contains(raw, "://") {
		return "/"
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "/"
		}
	}
	return raw
}

// xdomainMagicSession is the Redis value behind a magic token.
type xdomainMagicSession struct {
	UserID string `json:"user_id"`
}

const magicTokenPrefix = "ml_"

func generateMagicToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return magicTokenPrefix + hex.EncodeToString(b), nil
}

// MintMagicToken resolves a refresh-token cookie value to its user and stores a
// one-time cross-domain handoff token. Returns ("", nil) when the refresh token
// is missing/invalid/revoked (caller is anonymous — no token minted).
func (s *AuthService) MintMagicToken(ctx context.Context, refreshToken string) (string, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return "", nil
	}
	session, err := s.magicSessionFinder.FindAliveByHash(ctx, hashRefreshToken(refreshToken))
	if err != nil {
		return "", nil // anonymous / revoked — not an error
	}
	token, err := generateMagicToken()
	if err != nil {
		return "", err
	}
	val := &xdomainMagicSession{UserID: session.UserID}
	if err := s.cache.Set(ctx, cache.KeyXDomainMagic(token), val, cache.TTLXDomainMagic); err != nil {
		return "", fmt.Errorf("store magic token: %w", err)
	}
	return token, nil
}

// ConsumeMagicToken validates a one-time magic token, deletes it (single-use),
// and issues a fresh session for the bound user — exactly like a login. Returns
// an error for unknown/expired/already-used tokens.
func (s *AuthService) ConsumeMagicToken(ctx context.Context, token string, sc SessionContext) (*domain.AuthResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("empty magic token")
	}
	var val xdomainMagicSession
	if err := s.cache.Get(ctx, cache.KeyXDomainMagic(token), &val); err != nil {
		return nil, fmt.Errorf("magic token not found or expired")
	}
	// Single-use: delete immediately so a replay finds nothing.
	_ = s.cache.Delete(ctx, cache.KeyXDomainMagic(token))

	user, err := s.magicUserGetter.GetByID(ctx, val.UserID)
	if err != nil {
		return nil, fmt.Errorf("magic token user: %w", err)
	}
	return s.createSessionAndAuthResponse(ctx, user, sc)
}
