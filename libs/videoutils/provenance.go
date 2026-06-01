// provenance.go — HMAC provenance tokens for the HLS proxy.
//
// Problem: some upstream players (megaplay.buzz / mewstream, 2026-06) serve
// their master + variant playlists from a STABLE, allowlistable origin
// (cdn.mewstream.buzz) but place the actual .ts segments on an UNBOUNDED,
// continuously-rotating pool of throwaway .click/.buzz/.club domains. A
// static host allowlist cannot keep up — every new episode draws a fresh
// segment domain.
//
// Solution: when the proxy rewrites a playlist it fetched from an
// already-allowlisted origin, it signs each rewritten child/segment URL with
// a short-TTL HMAC (the "provenance token"). A later segment request bearing
// a valid token bypasses the static host allowlist — its provenance is the
// trusted playlist, not its own domain. This is purely ADDITIVE: tokens only
// ever GRANT access, and they can only be minted for URLs that appeared
// inside a playlist served from an allowlisted host, so the blast radius is
// exactly "hosts a trusted CDN's playlist points at". Non-token requests are
// unaffected and still go through the static allowlist.
//
// The token is a provenance marker, NOT an auth boundary: a weak/absent
// secret degrades gracefully (segments simply fall back to failing the
// static allowlist), so this never blocks a legitimately-allowlisted host.
package videoutils

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// provenanceTTL bounds how long a minted token stays valid. It MUST exceed a
// full VOD watch session: hls.js fetches a VOD child playlist once and then
// streams its segments for the whole episode without re-fetching, so the
// segment tokens minted at child-fetch time must outlive the entire watch.
// 12h covers a long episode plus generous pauses.
const provenanceTTL = 12 * time.Hour

var (
	provenanceSecretOnce sync.Once
	provenanceSecret     []byte
)

// loadProvenanceSecret resolves the signing key once from the environment.
// STREAM_TOKEN_SECRET (already set for the streaming service) is preferred;
// JWT_SECRET is the fallback; a build-time default is the last resort.
func loadProvenanceSecret() []byte {
	provenanceSecretOnce.Do(func() {
		for _, env := range []string{"STREAM_TOKEN_SECRET", "JWT_SECRET"} {
			if v := strings.TrimSpace(os.Getenv(env)); v != "" {
				provenanceSecret = []byte(v)
				return
			}
		}
		provenanceSecret = []byte("animeenigma-hls-provenance-default")
	})
	return provenanceSecret
}

// provenanceMAC computes the 128-bit (32 hex char) HMAC-SHA256 over
// rawURL + "\n" + expStr. Binding the exact URL prevents a token minted for
// one segment from being replayed against a different host/path.
func provenanceMAC(rawURL, expStr string) string {
	m := hmac.New(sha256.New, loadProvenanceSecret())
	m.Write([]byte(rawURL))
	m.Write([]byte("\n"))
	m.Write([]byte(expStr))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// signProvenance returns the (exp, sig) pair to append as &exp=&sig= on a
// rewritten proxy URL. exp is a unix-seconds expiry; sig authenticates
// (rawURL, exp).
func signProvenance(rawURL string, now time.Time) (exp, sig string) {
	exp = strconv.FormatInt(now.Add(provenanceTTL).Unix(), 10)
	return exp, provenanceMAC(rawURL, exp)
}

// validProvenanceToken reports whether (expStr, sig) authenticate rawURL and
// the token is unexpired. Constant-time over the signature. Missing/garbled
// tokens return false (caller then falls back to the static allowlist).
func validProvenanceToken(rawURL, expStr, sig string, now time.Time) bool {
	if expStr == "" || sig == "" {
		return false
	}
	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || now.Unix() > expUnix {
		return false
	}
	want := provenanceMAC(rawURL, expStr)
	return subtle.ConstantTimeCompare([]byte(want), []byte(sig)) == 1
}
