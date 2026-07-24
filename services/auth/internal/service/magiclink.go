package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// ErrMagicSessionConflict is returned by ConsumeMagicToken when the browser
// already carries a still-alive session that belongs to a DIFFERENT user than
// the one the magic token is bound to. Refusing to overwrite is the login-CSRF
// / session-fixation defense (CWE-352): nothing binds the URL token to the
// browser that consumes it, so an attacker could 302 an already-logged-in
// victim to /magic-link-login carrying a token minted for the ATTACKER's own
// account; honoring it would silently evict the victim into the attacker's
// session. First-time visitors (no session — the normal .ru->.org handoff) and
// same-user re-handoffs are unaffected: the users match and consume proceeds.
var ErrMagicSessionConflict = errors.New("magic link: active session belongs to a different user")

// SanitizeOldURL constrains a caller-supplied return path to a safe SAME-ORIGIN
// relative path, defeating open-redirect. A valid value is a single rooted
// relative reference: it must begin with a single '/' followed by a non-'/',
// non-'\' character, carry no scheme or host, contain no backslash or ASCII
// control character, and still resolve on-origin after path normalization.
// Anything else collapses to "/".
//
// The backslash rule is load-bearing: r.URL.Query().Get percent-decodes the
// oldurl parameter before it reaches here, so "oldurl=/./%5Cevil.com" arrives
// as "/./\evil.com" with a literal backslash. A prior version only rejected a
// literal "/\" prefix, but http.Redirect runs path.Clean on the value before
// emitting Location, normalizing "/./\evil.com" (and "/a/../\evil.com") down to
// the protocol-relative "/\evil.com" — which browsers resolve off-origin. We
// therefore reject '\' anywhere and re-run path.Clean ourselves to confirm the
// normalized path stays same-origin (CWE-601).
func SanitizeOldURL(raw string) string {
	// Reject empty input and any backslash anywhere in the value.
	if raw == "" || strings.Contains(raw, "\\") {
		return "/"
	}
	// Must begin with a single '/' followed by a non-'/', non-'\' character.
	// Rejects "" / bare "/", "//evil.com" (protocol-relative) and scheme- or
	// host-prefixed values ("https://…", "relative/…").
	if len(raw) < 2 || raw[0] != '/' || raw[1] == '/' || raw[1] == '\\' {
		return "/"
	}
	// Reject ASCII control characters anywhere in the value.
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "/"
		}
	}
	// Parse and require a pure relative reference: no scheme, no host, not
	// absolute. Rejects "https://evil.com", "javascript:alert(1)", "///evil.com"
	// (parses host-less but with a "//" path prefix — caught below) and any
	// value url.Parse cannot handle.
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Scheme != "" || u.Host != "" {
		return "/"
	}
	// Normalize the path exactly as http.Redirect will, and confirm the result
	// still starts with a single '/' (not "//" or "/\").
	cleaned := path.Clean(u.Path)
	if cleaned == "" || cleaned[0] != '/' || strings.HasPrefix(cleaned, "//") || strings.HasPrefix(cleaned, "/\\") {
		return "/"
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
//
// currentRefreshToken is the refresh_token cookie the consuming browser already
// carries (empty when it has none). If that cookie resolves to a still-alive
// session for a DIFFERENT user than the token's bound user, consume is refused
// with ErrMagicSessionConflict and the token is left intact — this is the
// session-fixation guard (CWE-352). A first-time visitor (no cookie) and a
// same-user re-handoff both proceed normally.
func (s *AuthService) ConsumeMagicToken(ctx context.Context, token, currentRefreshToken string, sc SessionContext) (*domain.AuthResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("empty magic token")
	}
	var val xdomainMagicSession
	if err := s.cache.Get(ctx, cache.KeyXDomainMagic(token), &val); err != nil {
		return nil, fmt.Errorf("magic token not found or expired")
	}

	// Session-fixation guard: if this browser is already signed in as a
	// different user, do NOT overwrite that session with the token's user. The
	// token is unbound to the browser, so an attacker could hand a victim a
	// token minted for the attacker's own account; adopting it would silently
	// replace the victim's session with the attacker's. We check BEFORE the
	// single-use delete so a refused (or racing) request does not burn a token
	// that may still be legitimately consumed by the matching user.
	if currentRefreshToken = strings.TrimSpace(currentRefreshToken); currentRefreshToken != "" {
		if existing, err := s.magicSessionFinder.FindAliveByHash(ctx, hashRefreshToken(currentRefreshToken)); err == nil && existing != nil && existing.UserID != val.UserID {
			return nil, ErrMagicSessionConflict
		}
	}

	// Single-use: delete immediately so a replay finds nothing.
	_ = s.cache.Delete(ctx, cache.KeyXDomainMagic(token))

	user, err := s.magicUserGetter.GetByID(ctx, val.UserID)
	if err != nil {
		return nil, fmt.Errorf("magic token user: %w", err)
	}
	return s.createSessionAndAuthResponse(ctx, user, sc)
}
