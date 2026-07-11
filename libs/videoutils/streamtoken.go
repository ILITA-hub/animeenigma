// streamtoken.go — opaque AES-GCM stream tokens for the HLS proxy (Track A of
// docs/superpowers/specs/2026-07-10-playback-resilience-constrained-browsers-design.md §3).
//
// Problem: the legacy proxy URL (hls-proxy?url=https%3A%2F%2Fp12.solodcdn…&exp=&sig=)
// embeds the upstream CDN hostname and a `proxy?url=` query shape — a bullseye
// for uBlock-style static network filters, which silently break playback for
// users with hardened browsers (the @gerahertz class of report).
//
// Solution: seal {upstream URL, referer, exp, type} into a single opaque
// AES-256-GCM token carried in the URL PATH (/api/streaming/m/<token>/<leaf>).
// The hostname is unreadable by the client/DOM/filter list; the token is
// unforgeable and tamper-evident (GCM tag); expiry rides inside the sealed
// payload, superseding the separate exp+sig query pair on this path. The key
// derives from the same secret as the provenance HMAC (STREAM_TOKEN_SECRET,
// JWT_SECRET fallback), so catalog-minted tokens open on the streaming service
// exactly like provenance signatures verify today. Fail-closed like
// provenance: no secret → no tokens → callers keep the legacy signed form.
//
// Kill-switch: AE_MASKED_STREAM_DISABLED=1 disables minting (decode keeps
// working so in-flight tokens survive a rollback toggle).
package videoutils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/videoutils/netguard"
)

// StreamTokenPayload is the sealed content of an opaque /m/<token> proxy URL.
type StreamTokenPayload struct {
	URL     string `json:"u"`           // upstream absolute URL (authoritative)
	Referer string `json:"r,omitempty"` // Referer header for the upstream fetch
	Exp     int64  `json:"e"`           // unix-seconds expiry
	Type    string `json:"t,omitempty"` // "mp4"/"webm" content-type override ("" = sniff)
}

var (
	streamTokenAEADOnce sync.Once
	streamTokenAEAD     cipher.AEAD
	maskedMintDisabled  = os.Getenv("AE_MASKED_STREAM_DISABLED") == "1"
)

// streamTokenCipher returns the package AEAD, or nil when no secret is
// configured. provenanceEnabled() is consulted on every call (not inside the
// Once) so tests can toggle provenanceConfigured the same way provenance
// tests do.
func streamTokenCipher() cipher.AEAD {
	if !provenanceEnabled() {
		return nil
	}
	streamTokenAEADOnce.Do(func() {
		key := sha256.Sum256(append([]byte("ae-stream-token-v1\n"), loadProvenanceSecret()...))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return
		}
		if aead, err := cipher.NewGCM(block); err == nil {
			streamTokenAEAD = aead
		}
	})
	return streamTokenAEAD
}

// EncodeStreamToken seals (rawURL, referer, streamType) into an opaque
// URL-safe token valid for provenanceTTL (12h — must outlive a full VOD watch,
// same rationale as the provenance token). Returns "" when minting is disabled
// (no secret / kill-switch) or the URL fails the SSRF guard — callers then
// fall back to the legacy signed query form.
func EncodeStreamToken(rawURL, referer, streamType string, now time.Time) string {
	if maskedMintDisabled {
		return ""
	}
	aead := streamTokenCipher()
	if aead == nil {
		return ""
	}
	// Mirror signProvenance's SSRF guard: a token authorizes a proxy fetch,
	// so never mint one for a private/loopback/non-http(s) target.
	if !allowLoopbackForTest && netguard.ValidatePublicURL(rawURL) != nil {
		return ""
	}
	payload, err := json.Marshal(StreamTokenPayload{
		URL:     rawURL,
		Referer: referer,
		Exp:     now.Add(provenanceTTL).Unix(),
		Type:    streamType,
	})
	if err != nil {
		return ""
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, payload, nil))
}

// DecodeStreamToken opens a token, validating the GCM tag, expiry, and the
// SSRF guard (defense in depth — mirrors validProvenanceToken).
func DecodeStreamToken(token string, now time.Time) (*StreamTokenPayload, error) {
	aead := streamTokenCipher()
	if aead == nil {
		return nil, errors.New("stream tokens disabled: no secret configured")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(sealed) < aead.NonceSize() {
		return nil, errors.New("malformed stream token")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], nil)
	if err != nil {
		return nil, errors.New("invalid stream token")
	}
	var p StreamTokenPayload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, errors.New("invalid stream token payload")
	}
	if now.Unix() > p.Exp {
		return nil, errors.New("stream token expired")
	}
	if !allowLoopbackForTest && netguard.ValidatePublicURL(p.URL) != nil {
		return nil, errors.New("stream token target not allowed")
	}
	return &p, nil
}

// maskedLeaf returns the last path segment of the upstream URL, kept on the
// masked URL purely so extension-based player heuristics (.m3u8/.ts/.vtt/.key)
// keep working. Cosmetic only — the token is authoritative.
func maskedLeaf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "media"
	}
	p := u.Path
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	if p == "" {
		return "media"
	}
	return url.PathEscape(p)
}

// MaskedStreamURL returns the opaque path-token proxy URL for an upstream
// stream/subtitle URL: /api/streaming/m/<token>/<leaf>. Returns "" when the
// token mechanism is disabled — callers keep the legacy signed query form.
func MaskedStreamURL(rawURL, referer, streamType string) string {
	tok := EncodeStreamToken(rawURL, referer, streamType, time.Now())
	if tok == "" {
		return ""
	}
	return "/api/streaming/m/" + tok + "/" + maskedLeaf(rawURL)
}
