// Package streamsign signs externally-hosted stream/subtitle URLs with the HLS
// proxy's provenance HMAC, so the streaming proxy can trust them WITHOUT a
// static host allowlist. The frontend appends the returned (exp, sig) to its
// /api/streaming/hls-proxy?url=... request; the proxy verifies the token.
//
// Only absolute http(s) URLs are signed. Same-origin (/api/...) URLs are
// fetched directly by the frontend (never through the proxy) and MUST stay
// unsigned.
package streamsign

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/videoutils"
)

// IsExternal reports whether a URL is an absolute http(s) URL (the kind the
// frontend routes through the HLS proxy and therefore must be signed).
func IsExternal(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// Sign returns the provenance (exp, sig) for an external URL, or empty strings
// for a non-external (same-origin) one.
func Sign(u string) (exp, sig string) {
	if !IsExternal(u) {
		return "", ""
	}
	return videoutils.SignStreamURL(u)
}

// SignScraperStreamBody rewrites a scraper stream JSON envelope in place,
// adding "exp"/"sig" to data.stream.sources[].url and external
// data.stream.tracks[].file. It operates on map[string]any (NOT a typed
// struct) so meta/intro/outro/headers and any future fields are preserved
// byte-for-byte, and only rewrites successful (200 + success:true) bodies —
// error envelopes pass through untouched.
func SignScraperStreamBody(status int, body []byte) []byte {
	if status != http.StatusOK {
		return body
	}
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		return body
	}
	if ok, _ := env["success"].(bool); !ok {
		return body
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		return body
	}
	stream, ok := data["stream"].(map[string]any)
	if !ok {
		return body
	}

	changed := signArrayField(stream["sources"], "url")
	changed = signArrayField(stream["tracks"], "file") || changed

	if !changed {
		return body
	}
	out, err := json.Marshal(env)
	if err != nil {
		return body // never corrupt the body on a marshal error
	}
	return out
}

// signArrayField signs the `urlKey` field of each object in a JSON array,
// stamping "exp"/"sig" siblings. Returns whether anything was signed.
func signArrayField(raw any, urlKey string) bool {
	arr, ok := raw.([]any)
	if !ok {
		return false
	}
	changed := false
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		u, ok := m[urlKey].(string)
		if !ok || !IsExternal(u) {
			continue
		}
		exp, sig := videoutils.SignStreamURL(u)
		m["exp"] = exp
		m["sig"] = sig
		changed = true
	}
	return changed
}
