package controlplane

import (
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
)

// SessionTTL — worker sessions are effectively permanent (owner decision 2026-06-24):
// a long-lived dial-home worker must never need to re-burn its single-use enroll
// token. ~100 years; the HMAC sig still gates auth, only time-expiry is removed in
// practice. SessionExpiresAt in the DB becomes a far-future timestamp (harmless).
// The shared capability package's VerifyJobHandle has no max-TTL/sanity cap (it
// only rejects exp <= now), so a ~100-year exp verifies cleanly. Segment/job
// handles keep their own short TTLs — only the session lifetime is extended here.
const SessionTTL = 100 * 365 * 24 * time.Hour

// MintSession mints a session capability handle bound to workerID using the
// shared capability package.  It delegates to capability.MintJobHandle with
// operation="session" and idx=0.
//
// Returns ("","","") when the capability secret is not configured (fail-closed).
func MintSession(workerID string, ttl time.Duration) (handle, exp, sig string) {
	return capability.MintJobHandle(workerID, "session", 0, ttl)
}

// VerifySession verifies a session capability token for the given workerID.
// Returns false when the secret is not configured, the signature is invalid, or
// the handle has expired.
func VerifySession(workerID, exp, sig string, now time.Time) bool {
	return capability.VerifyJobHandle(workerID, "session", 0, exp, sig, now)
}
