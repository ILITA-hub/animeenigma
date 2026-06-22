package service

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// Login brute-force throttle: after loginMaxFails failures within the window a
// username is locked; a successful login clears the counter. Keyed
// case-insensitively. (audit medium #6)
func TestLoginThrottle(t *testing.T) {
	c := newFakeCache()
	s := NewAuthService(nil, nil, c, authz.JWTConfig{}, "", time.Hour, logger.Default())
	s.loginMaxFails = 3
	s.loginFailWindow = time.Minute
	ctx := context.Background()
	const u = "victim"

	if s.loginLocked(ctx, u) {
		t.Fatal("should not be locked initially")
	}
	s.recordLoginFailure(ctx, u)
	s.recordLoginFailure(ctx, u)
	if s.loginLocked(ctx, u) {
		t.Fatal("should not be locked at 2/3 failures")
	}
	s.recordLoginFailure(ctx, u) // 3rd failure -> at threshold
	if !s.loginLocked(ctx, u) {
		t.Fatal("should be locked at 3/3 failures")
	}
	if !s.loginLocked(ctx, "VICTIM") {
		t.Fatal("lockout key must be case-insensitive")
	}

	s.clearLoginFailures(ctx, u)
	if s.loginLocked(ctx, u) {
		t.Fatal("a successful login (clear) must reset the lockout")
	}
}

// A zero/negative threshold disables the throttle (never locks).
func TestLoginThrottle_Disabled(t *testing.T) {
	c := newFakeCache()
	s := NewAuthService(nil, nil, c, authz.JWTConfig{}, "", time.Hour, logger.Default())
	s.loginMaxFails = 0
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		s.recordLoginFailure(ctx, "u")
	}
	if s.loginLocked(ctx, "u") {
		t.Fatal("threshold 0 must disable the lockout")
	}
}
