package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// deriveAPIKeySessionID derives a deterministic SessionID for a JWT minted
// from an "ak_*" API-key request. Audit finding S4 (WV3-T1) fix.
//
// Algorithm:
//
//	day       = now.UTC().Format("2006-01-02")
//	keyHash   = sha256(rawAPIKey)
//	sidInput  = userID + "|" + hex(keyHash) + "|" + day
//	sidHash   = sha256(sidInput)
//	SessionID = "ak-" + hex(sidHash[:8])     // 19 chars, 64 bits entropy
//
// Properties:
//   - Same (userID, rawAPIKey, UTC-day) → same SID. In-flight JWTs minted
//     for the same identity within a day share a stable correlation ID
//     suitable for audit-log grouping.
//   - Different UTC day → fresh SID space; revoking yesterday does not
//     affect today.
//   - Rotating the raw key → entirely new SID space; old revocations
//     become inert.
//   - Independent of host TZ — UTC normalization is explicit.
//
// Scope guardrail: this helper only populates the SessionID claim. It does
// NOT enforce access-token revocation (no middleware currently consults
// user_sessions.revoked_at for access tokens; a future task wires that up).
//
// The function is pure (no IO, no global state) — tests pass an explicit
// time.Time; the production callers pass time.Now().
func deriveAPIKeySessionID(userID, rawAPIKey string, now time.Time) string {
	day := now.UTC().Format("2006-01-02")
	keyHash := sha256.Sum256([]byte(rawAPIKey))
	sidInput := userID + "|" + hex.EncodeToString(keyHash[:]) + "|" + day
	sidHash := sha256.Sum256([]byte(sidInput))
	return "ak-" + hex.EncodeToString(sidHash[:8])
}
