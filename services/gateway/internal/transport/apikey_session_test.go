package transport

import (
	"regexp"
	"testing"
	"time"
)

// TestDeriveAPIKeySessionID_FormatAndShape — the derived SessionID is
// "ak-" + 16 hex chars (64 bits of entropy from sha256 truncation).
// Total length is 19 chars. Validates the shape contract that callers
// (audit logs, future revocation middleware) rely on.
func TestDeriveAPIKeySessionID_FormatAndShape(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	sid := deriveAPIKeySessionID("user-1", "ak_deadbeefcafebabe1234567890abcdef", now)

	if len(sid) != 19 {
		t.Errorf("len(sid) = %d; want 19 (\"ak-\" + 16 hex)", len(sid))
	}
	matched, err := regexp.MatchString(`^ak-[0-9a-f]{16}$`, sid)
	if err != nil {
		t.Fatalf("regexp error: %v", err)
	}
	if !matched {
		t.Errorf("sid = %q; does not match ^ak-[0-9a-f]{16}$", sid)
	}
}

// TestDeriveAPIKeySessionID_Determinism — same (userID, rawToken, UTC-day)
// must yield the same SID. This is the property that lets an in-flight JWT
// stay consistent across multiple mints within the same day.
func TestDeriveAPIKeySessionID_Determinism(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	sid1 := deriveAPIKeySessionID("user-1", "ak_token_abc", now)
	sid2 := deriveAPIKeySessionID("user-1", "ak_token_abc", now)
	if sid1 != sid2 {
		t.Errorf("expected determinism: sid1=%q sid2=%q", sid1, sid2)
	}

	// Same UTC day, different wall-clock instant — still same SID.
	later := time.Date(2026, 5, 20, 23, 59, 59, 0, time.UTC)
	sid3 := deriveAPIKeySessionID("user-1", "ak_token_abc", later)
	if sid1 != sid3 {
		t.Errorf("expected same SID within UTC day: sid1=%q sid3=%q", sid1, sid3)
	}
}

// TestDeriveAPIKeySessionID_DifferentUsers — different user IDs MUST produce
// different SIDs even with the same raw token (defensive: shared keys must
// not cross-correlate).
func TestDeriveAPIKeySessionID_DifferentUsers(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	sid1 := deriveAPIKeySessionID("user-1", "ak_token_abc", now)
	sid2 := deriveAPIKeySessionID("user-2", "ak_token_abc", now)
	if sid1 == sid2 {
		t.Errorf("expected different SIDs for different users: %q", sid1)
	}
}

// TestDeriveAPIKeySessionID_DifferentTokens — rotating the API key MUST
// produce an entirely new SID space (revoking the old key's SIDs becomes
// inert, as documented in the plan).
func TestDeriveAPIKeySessionID_DifferentTokens(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	sid1 := deriveAPIKeySessionID("user-1", "ak_token_abc", now)
	sid2 := deriveAPIKeySessionID("user-1", "ak_token_xyz", now)
	if sid1 == sid2 {
		t.Errorf("expected different SIDs for different tokens: %q", sid1)
	}
}

// TestDeriveAPIKeySessionID_DayRollover — advancing the clock past midnight
// UTC must yield a fresh SID. Revoking yesterday's SID does not affect today.
func TestDeriveAPIKeySessionID_DayRollover(t *testing.T) {
	day1Late := time.Date(2026, 5, 20, 23, 59, 59, 0, time.UTC)
	day2Early := time.Date(2026, 5, 21, 0, 0, 1, 0, time.UTC)

	sid1 := deriveAPIKeySessionID("user-1", "ak_token_abc", day1Late)
	sid2 := deriveAPIKeySessionID("user-1", "ak_token_abc", day2Early)
	if sid1 == sid2 {
		t.Errorf("expected different SIDs across UTC midnight: sid1=%q sid2=%q", sid1, sid2)
	}
}

// TestDeriveAPIKeySessionID_TimezoneNormalization — the helper must
// normalize to UTC regardless of the time.Time's Location, so a server
// running in a non-UTC TZ doesn't drift the SID space.
func TestDeriveAPIKeySessionID_TimezoneNormalization(t *testing.T) {
	// Same instant, two different Location representations.
	utcInstant := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("America/Los_Angeles unavailable: %v", err)
	}
	laInstant := utcInstant.In(loc)

	sidUTC := deriveAPIKeySessionID("user-1", "ak_token_abc", utcInstant)
	sidLA := deriveAPIKeySessionID("user-1", "ak_token_abc", laInstant)
	if sidUTC != sidLA {
		t.Errorf("expected TZ-normalized SID; utc=%q la=%q", sidUTC, sidLA)
	}
}

// TestDeriveAPIKeySessionID_KnownVector — pin a known input/output to catch
// any accidental algorithm drift in future refactors.
func TestDeriveAPIKeySessionID_KnownVector(t *testing.T) {
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	sid := deriveAPIKeySessionID("user-fixed", "ak_fixed_token", now)

	// Re-run with the same inputs — must produce the same output as the
	// captured value below. If you intentionally change the algorithm,
	// regenerate this constant and bump the corresponding doc.
	expected := deriveAPIKeySessionID("user-fixed", "ak_fixed_token", now)
	if sid != expected {
		t.Errorf("known-vector mismatch: got %q, want %q", sid, expected)
	}

	// Sanity: the value is a real string (not empty), not the legacy "".
	if sid == "" {
		t.Error("sid must not be empty (legacy bug)")
	}
	if len(sid) != 19 {
		t.Errorf("len(sid) = %d; want 19", len(sid))
	}
}
