package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/redis/go-redis/v9"
)

// Login brute-force throttle (audit medium #6). A per-account sliding window:
// after loginMaxFails failures within loginFailWindow the account is locked
// (the handler maps the RateLimited error to HTTP 429); a successful login
// clears the counter. The gateway per-IP limiter is the complementary
// (fail-open) layer — this is the per-account backstop bcrypt's cost alone
// cannot provide. All cache ops are best-effort: a Redis error never blocks
// the authentication decision (fail-open, like the gateway limiter).

// maxLoginUsernameLen bounds the username Login will act on. Registration caps
// usernames at 32 chars (RegisterRequest: max=32,alphanum), so no real account
// can exceed this — anything longer is treated as an unknown user. Callers
// reject it before touching the DB or the throttle store.
const maxLoginUsernameLen = 32

// loginFailKey derives a FIXED-WIDTH throttle key from the normalized username.
// The username is hashed (sha256 → 64 hex chars) so no caller-controlled length
// reaches Redis: a pathological multi-megabyte username can never become a
// multi-megabyte Redis key (audit F13). Normalization (lower + trim) keeps the
// key case-insensitive, matching the pre-hash behaviour.
func loginFailKey(username string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(username))))
	return "auth:loginfail:" + hex.EncodeToString(sum[:])
}

// loginFailIncrScript atomically increments the per-account failure counter and
// (re)arms its sliding-window TTL in ONE round trip. INCR alone creates a key
// with no TTL; a separate follow-up EXPIRE can fail (transient Redis error or a
// cancelled context) and leave the counter at/over the threshold forever — and
// since the counter is only cleared after a *successful* login, which the lock
// blocks before the password is even checked, the account would be locked
// permanently. Doing INCR + PEXPIRE as one atomic unit guarantees the key can
// never exist without a TTL, so a lockout always auto-expires (audit F17).
// Returns the new counter value.
var loginFailIncrScript = redis.NewScript(`
local n = redis.call("INCR", KEYS[1])
redis.call("PEXPIRE", KEYS[1], ARGV[1])
return n
`)

// loginLocked reports whether the username has reached the failure threshold.
func (s *AuthService) loginLocked(ctx context.Context, username string) bool {
	if s.loginMaxFails <= 0 {
		return false
	}
	key := loginFailKey(username)

	// Production path: read the counter with the raw client, in the SAME
	// integer format recordLoginFailure's INCR writes it. A missing key
	// (redis.Nil) or ANY Redis error is fail-open — never block auth.
	if rc, ok := s.cache.(*cache.RedisCache); ok {
		n, err := rc.Client().Get(ctx, key).Int()
		if err != nil {
			return false
		}
		return n >= s.loginMaxFails
	}

	// In-memory fallback (test cache): JSON-encoded int round-trips identically.
	var fails int
	_ = s.cache.Get(ctx, key, &fails)
	return fails >= s.loginMaxFails
}

// recordLoginFailure increments the username's failure counter, (re)arming the
// sliding window TTL.
func (s *AuthService) recordLoginFailure(ctx context.Context, username string) {
	if s.loginMaxFails <= 0 {
		return
	}
	key := loginFailKey(username)

	// Production path: atomic INCR + PEXPIRE via EVAL, so the counter and its
	// TTL are written together and a lockout can never become permanent from a
	// missed EXPIRE. The error is discarded (fail-open).
	if rc, ok := s.cache.(*cache.RedisCache); ok {
		ms := s.loginFailWindow.Milliseconds()
		if ms <= 0 {
			ms = 1 // never persist a TTL-less key
		}
		_ = loginFailIncrScript.Run(ctx, rc.Client(), []string{key}, ms).Err()
		return
	}

	// In-memory fallback (test cache): Set writes value + TTL atomically, so the
	// non-atomic Get/Set is only a benign lost-update under concurrency here.
	var fails int
	_ = s.cache.Get(ctx, key, &fails)
	fails++
	_ = s.cache.Set(ctx, key, fails, s.loginFailWindow)
}

// clearLoginFailures resets the counter after a successful authentication.
func (s *AuthService) clearLoginFailures(ctx context.Context, username string) {
	_ = s.cache.Delete(ctx, loginFailKey(username))
}
