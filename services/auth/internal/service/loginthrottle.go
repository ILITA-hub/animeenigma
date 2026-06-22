package service

import (
	"context"
	"strings"
)

// Login brute-force throttle (audit medium #6). A per-account sliding window:
// after loginMaxFails failures within loginFailWindow the account is locked
// (the handler maps the RateLimited error to HTTP 429); a successful login
// clears the counter. The gateway per-IP limiter is the complementary
// (fail-open) layer — this is the per-account backstop bcrypt's cost alone
// cannot provide. All cache ops are best-effort: a Redis error never blocks
// the authentication decision (fail-open, like the gateway limiter).

func loginFailKey(username string) string {
	return "auth:loginfail:" + strings.ToLower(strings.TrimSpace(username))
}

// loginLocked reports whether the username has reached the failure threshold.
func (s *AuthService) loginLocked(ctx context.Context, username string) bool {
	if s.loginMaxFails <= 0 {
		return false
	}
	var fails int
	_ = s.cache.Get(ctx, loginFailKey(username), &fails)
	return fails >= s.loginMaxFails
}

// recordLoginFailure increments the username's failure counter, (re)arming the
// sliding window TTL.
func (s *AuthService) recordLoginFailure(ctx context.Context, username string) {
	if s.loginMaxFails <= 0 {
		return
	}
	key := loginFailKey(username)
	var fails int
	_ = s.cache.Get(ctx, key, &fails)
	fails++
	_ = s.cache.Set(ctx, key, fails, s.loginFailWindow)
}

// clearLoginFailures resets the counter after a successful authentication.
func (s *AuthService) clearLoginFailures(ctx context.Context, username string) {
	_ = s.cache.Delete(ctx, loginFailKey(username))
}
