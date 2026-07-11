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

// MaskedURL returns the Track A opaque path-token proxy URL
// (/api/streaming/m/<token>/<leaf>) for an external stream/subtitle URL, or
// "" for same-origin URLs or when the token mechanism is unconfigured.
// streamType is "mp4" for progressive MP4 (selects the proxy's
// range-passthrough path), "" for HLS/sniffed content.
func MaskedURL(u, referer, streamType string) string {
	if !IsExternal(u) {
		return ""
	}
	return videoutils.MaskedStreamURL(u, referer, streamType)
}

// SignScraperStreamBody rewrites a scraper stream JSON envelope in place,
// adding "exp"/"sig" (legacy dual-accept) plus the Track A "masked_url"
// opaque path-token form to data.stream.sources[].url and external
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

	// The upstream Referer applies to every source fetch; subtitle tracks are
	// fetched WITHOUT a referer today (buildSubtitleProxyUrl passes none), so
	// their masked tokens keep referer "" for behavior parity.
	referer := ""
	if h, ok := stream["headers"].(map[string]any); ok {
		if v, ok := h["Referer"].(string); ok {
			referer = v
		} else if v, ok := h["referer"].(string); ok {
			referer = v
		}
	}

	changed := signArrayField(stream["sources"], "url", referer, true)
	changed = signArrayField(stream["tracks"], "file", "", false) || changed

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
// stamping "exp"/"sig" siblings (legacy dual-accept) plus the Track A
// "masked_url" opaque path form. withType propagates the item's own
// "type" ("mp4"/"webm") into the token so the proxy picks its
// range-passthrough path. Returns whether anything was signed.
func signArrayField(raw any, urlKey, referer string, withType bool) bool {
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
		streamType := ""
		if withType {
			if tv, ok := m["type"].(string); ok && (tv == "mp4" || tv == "webm") {
				streamType = tv
			}
		}
		if masked := videoutils.MaskedStreamURL(u, referer, streamType); masked != "" {
			m["masked_url"] = masked
		}
		changed = true
	}
	return changed
}
