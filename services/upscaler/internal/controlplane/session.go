package controlplane

import (
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
)

// SessionTTL is the lifetime of a freshly minted worker session capability.
const SessionTTL = 12 * time.Hour

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
