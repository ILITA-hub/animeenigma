// Package capability provides isolated HMAC job-capability handles.
//
// # Purpose
//
// Workers that process upscaler segments need proof that they are authorised
// to fetch/upload a specific segment of a specific job. A capability handle
// is a short-lived, idx-bound HMAC token that the leaser (Task 11) mints per
// segment and the data-plane handler (Task 11b) verifies. A worker that holds
// a lease for segment 0 cannot forge a handle for segment 1, and an expired
// handle from a completed lease cannot be replayed.
//
// # Algorithm
//
//	handle  = jobID + ":" + operation + ":" + strconv.Itoa(idx)
//	sig     = hex(HMAC-SHA256(secret, handle + "\n" + exp))[:32]   // 32-hex truncation
//	exp     = strconv.FormatInt(now.Add(ttl).Unix(), 10)            // Unix seconds
//
// Verification: recompute handle from (jobID, operation, idx); parse exp;
// reject if now.Unix() > expUnix (one second grace at boundary); constant-time
// compare recomputed sig vs supplied sig.
//
// # Fail-closed
//
// When the secret is empty / not configured, Init is a no-op, Enabled returns
// false, MintJobHandle returns ("","",""), and VerifyJobHandle always returns
// false. This is intentional: an unconfigured deployment must not grant access.
//
// # sync.Once strategy
//
// Public Init is sync.Once-gated — safe for concurrent service startup.
// The unexported initWith bypasses the once for white-box testing (same-package
// tests call initWith to switch secrets between test cases). Production code
// only ever calls Init.
package capability

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"sync"
	"time"
)

var (
	secretOnce  sync.Once
	secret      string
	configured  bool
	secretMu    sync.RWMutex // guards secret + configured for initWith resets
)

// Init loads the capability secret once. Subsequent calls are no-ops (sync.Once).
// Pass cfg.Upscaler.JobCapabilitySecret. An empty string disables the mechanism
// (fail-closed). Log a WARN at the call site when Enabled() is false.
func Init(s string) {
	secretOnce.Do(func() {
		initWith(s)
	})
}

// initWith unconditionally reinitialises the secret. Used by white-box tests to
// switch secrets between test cases without process isolation. Production code
// MUST only call Init (once-gated). Not exported — same-package tests only.
func initWith(s string) {
	secretMu.Lock()
	defer secretMu.Unlock()
	secret = s
	configured = s != ""
}

// Enabled reports whether a non-empty secret was Init'd.
func Enabled() bool {
	secretMu.RLock()
	defer secretMu.RUnlock()
	return configured
}

// jobHandle assembles the deterministic handle string from its components.
// This is the message that is HMAC'd together with exp.
func jobHandle(jobID, operation string, idx int) string {
	return jobID + ":" + operation + ":" + strconv.Itoa(idx)
}

// capabilityMAC computes the 128-bit (32 hex char) HMAC-SHA256 over
// (handle + "\n" + expStr). Truncation to 32 hex chars matches the
// provenance.go idiom used elsewhere in this repo.
func capabilityMAC(secretKey, handle, expStr string) string {
	m := hmac.New(sha256.New, []byte(secretKey))
	m.Write([]byte(handle))
	m.Write([]byte("\n"))
	m.Write([]byte(expStr))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// mintAt is the internal implementation used by MintJobHandle and directly by
// tests when a deterministic "now" is required. Returns (handle, exp, sig).
func mintAt(jobID, operation string, idx int, ttl time.Duration, now time.Time) (handle, exp, sig string) {
	secretMu.RLock()
	s := secret
	ok := configured
	secretMu.RUnlock()

	if !ok {
		return "", "", ""
	}

	handle = jobHandle(jobID, operation, idx)
	exp = strconv.FormatInt(now.Add(ttl).Unix(), 10)
	sig = capabilityMAC(s, handle, exp)
	return handle, exp, sig
}

// MintJobHandle mints an HMAC capability handle that binds jobID, operation,
// and idx to a specific expiry. Returns ("","","") when the secret is not
// configured (fail-closed). TTL should be the lease TTL + ~10 min grace.
func MintJobHandle(jobID, operation string, idx int, ttl time.Duration) (handle, exp, sig string) {
	return mintAt(jobID, operation, idx, ttl, time.Now())
}

// VerifyJobHandle verifies a capability handle. Returns false when:
//   - the secret is not configured (fail-closed)
//   - exp is missing, unparseable, or expired (now.Unix() > expUnix)
//   - the sig does not match the recomputed MAC (constant-time comparison)
//   - jobID, operation, or idx do not match what was minted
//
// Pass the caller's wall-clock time as now (enables deterministic tests).
func VerifyJobHandle(jobID, operation string, idx int, exp, sig string, now time.Time) bool {
	secretMu.RLock()
	s := secret
	ok := configured
	secretMu.RUnlock()

	if !ok {
		return false
	}
	if exp == "" || sig == "" {
		return false
	}

	expUnix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || now.Unix() > expUnix {
		return false
	}

	handle := jobHandle(jobID, operation, idx)
	want := capabilityMAC(s, handle, exp)

	// Constant-time comparison — both strings are always 32 hex chars.
	return subtle.ConstantTimeCompare([]byte(want), []byte(sig)) == 1
}
