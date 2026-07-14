// provenance.go — HMAC provenance tokens for the HLS proxy.
//
// Problem: some upstream players (megaplay.buzz / mewstream, 2026-06) serve
// their master + variant playlists from a STABLE origin (cdn.mewstream.buzz)
// but place the actual .ts segments on an UNBOUNDED, continuously-rotating
// pool of throwaway .click/.buzz/.club domains. No static host list can keep
// up — every new episode draws a fresh segment domain.
//
// Solution: when the proxy rewrites a playlist it fetched from a trusted
// origin (provenance-signed by catalog, or a first-party internal host), it
// signs each rewritten child/segment URL with a short-TTL HMAC (the
// "provenance token"). A later segment request bearing a valid token passes
// the proxy's trust gate — its provenance is the trusted playlist, not its
// own domain. Tokens only ever GRANT access, and they can only be minted for
// URLs that appeared inside a playlist served through the gate, so the blast
// radius is exactly "hosts a trusted CDN's playlist points at".
//
// Since 2026-07-14 signing IS the trust model (the static external-domain
// allowlist was retired — docs/plans/2026-07-14-retire-allowlist-blocklist.md):
// the HLS gate is `preauth (sealed token) OR first-party internal host OR
// provenance-signed`. Catalog signs every externally-hosted stream/subtitle
// URL at the source (streamsign.Stamp), and this file's minting covers the
// playlist children.
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

	"github.com/ILITA-hub/animeenigma/libs/videoutils/netguard"
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
	provenanceConfigured bool
)

// loadProvenanceSecret resolves the signing key once from the environment.
// STREAM_TOKEN_SECRET (already set for the streaming service) is preferred;
// JWT_SECRET is the fallback.
//
// FAIL CLOSED when neither is set: previously this fell back to a public,
// hardcoded default ("animeenigma-hls-provenance-default"). Because that value
// lives in the source tree, anyone could compute a valid provenance MAC for an
// ARBITRARY url and have the HLS proxy fetch it (SSRF / open-proxy — the
// provenance token is the trust gate's "OR signed" arm). With no real secret
// we disable the token mechanism instead: signing is a no-op and validation
// always fails, so the gate admits only preauth (sealed) requests and
// first-party internal hosts. Every external stream stops working until
// STREAM_TOKEN_SECRET (or JWT_SECRET) is set — which production always sets.
func loadProvenanceSecret() []byte {
	provenanceSecretOnce.Do(func() {
		for _, env := range []string{"STREAM_TOKEN_SECRET", "JWT_SECRET"} {
			if v := strings.TrimSpace(os.Getenv(env)); v != "" {
				provenanceSecret = []byte(v)
				provenanceConfigured = true
				return
			}
		}
		provenanceSecret = nil
		provenanceConfigured = false
	})
	return provenanceSecret
}

// provenanceEnabled reports whether a real signing secret is configured. When
// false the token mechanism is disabled (fail closed) — see loadProvenanceSecret.
func provenanceEnabled() bool {
	loadProvenanceSecret()
	return provenanceConfigured
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
	if !provenanceEnabled() {
		// No secret configured → mint nothing. Callers append &exp=&sig= with
		// empty values, which validProvenanceToken rejects, so the segment
		// is refused by the trust gate (fail closed).
		return "", ""
	}
	// SSRF guard (finding #65): never mint a token for a URL whose scheme is
	// not http/https or whose IP-literal host is private/loopback/link-local.
	// A token IS the trust gate's authorization, so a compromised trusted CDN
	// must not be able to self-mint authorization for http://169.254.169.254
	// or http://10.x. Hostnames pass this cheap check; the dial-time guard in
	// newIPv4Transport blocks any that resolve to a private address.
	if !allowLoopbackForTest && netguard.ValidatePublicURL(rawURL) != nil {
		return "", ""
	}
	exp = strconv.FormatInt(now.Add(provenanceTTL).Unix(), 10)
	return exp, provenanceMAC(rawURL, exp)
}

// SignStreamURL signs an entry-point stream/subtitle URL that the backend
// resolved, returning the (exp, sig) pair the frontend appends as &exp=&sig= so
// the HLS proxy's trust gate admits it. It is the public
// counterpart of the internal segment-rewrite minting and verifies against the
// same validProvenanceToken the proxy uses.
//
// INVARIANT: the caller must sign the EXACT byte string that ends up in the
// proxy's `url` query parameter. The proxy validates over
// `r.URL.Query().Get("url")` (URL-decoded), so as long as the frontend places
// this same string into `url` with standard query encoding (encode→decode is
// identity), the MAC matches. See TestSignStreamURL_SurvivesQueryRoundTrip.
func SignStreamURL(rawURL string) (exp, sig string) {
	return signProvenance(rawURL, time.Now())
}

// validProvenanceToken reports whether (expStr, sig) authenticate rawURL and
// the token is unexpired. Constant-time over the signature. Missing/garbled
// tokens return false (the trust gate then rejects the request unless the
// host is first-party or the call is preauth).
func validProvenanceToken(rawURL, expStr, sig string, now time.Time) bool {
	if !provenanceEnabled() {
		// Fail closed: with no configured secret, accept no tokens (a forged
		// token computed from the old hardcoded default must not grant access).
		return false
	}
	// Mirror the mint-side SSRF guard (finding #65): refuse to honor a token for
	// a private/loopback/link-local IP-literal host or a non-http(s) scheme, even
	// if it carries a valid MAC. Closes the case of a token minted before this
	// guard (or otherwise forged) for an internal target.
	if !allowLoopbackForTest && netguard.ValidatePublicURL(rawURL) != nil {
		return false
	}
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
