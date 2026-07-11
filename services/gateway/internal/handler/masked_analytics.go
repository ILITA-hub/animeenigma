package handler

// Track B5 (spec 2026-07-10 §4): rotating HMAC-masked ingestion paths for the
// analytics endpoints. Static filter lists (EasyPrivacy-class) carry
// /api/analytics/* verbatim, silently zeroing telemetry for adblock users —
// exactly the population whose playback failures we most need telemetry from.
// A path segment that is HMAC(secret, hour-bucket) rotates hourly and cannot
// be pinned by a static rule. The gateway hands the SPA the current masked
// base via the X-AE-Cfg response header (MaskedPathHintMiddleware) and
// validates inbound masked posts against the current AND previous bucket
// (clock skew / session straddle) before rewriting to the real analytics
// path. Normal users keep hitting /api/analytics/* directly, keeping the
// masked path low-profile.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const maskedBucketSeconds = 3600

// maskedAnalyticsEndpoints maps the short masked leaf to the real analytics
// ingestion path. Single letters — no "collect"/"analytics" bait words.
var maskedAnalyticsEndpoints = map[string]string{
	"c": "/api/analytics/collect",
	"e": "/api/analytics/client-errors",
	"p": "/api/analytics/player-events",
}

// maskedAnalyticsSegment derives the rotating 24-hex path segment for a bucket.
func maskedAnalyticsSegment(secret []byte, bucket int64) string {
	m := hmac.New(sha256.New, secret)
	fmt.Fprintf(m, "ae-analytics-mask\n%d", bucket)
	return hex.EncodeToString(m.Sum(nil))[:24]
}

// CurrentMaskedAnalyticsBase returns "/api/<segment>" for now's bucket.
func CurrentMaskedAnalyticsBase(secret []byte, now time.Time) string {
	return "/api/" + maskedAnalyticsSegment(secret, now.Unix()/maskedBucketSeconds)
}

// validMaskedSegment reports whether seg matches the current or previous
// bucket. Constant-time compares.
func validMaskedSegment(secret []byte, seg string, now time.Time) bool {
	bucket := now.Unix() / maskedBucketSeconds
	for _, b := range []int64{bucket, bucket - 1} {
		want := maskedAnalyticsSegment(secret, b)
		if subtle.ConstantTimeCompare([]byte(want), []byte(seg)) == 1 {
			return true
		}
	}
	return false
}

// MaskedAnalyticsHandler validates and forwards masked ingestion posts.
type MaskedAnalyticsHandler struct {
	proxy  *ProxyHandler
	secret []byte
}

func NewMaskedAnalyticsHandler(proxy *ProxyHandler, secret []byte) *MaskedAnalyticsHandler {
	return &MaskedAnalyticsHandler{proxy: proxy, secret: secret}
}

// Handle validates {maskedSeg}/{maskedEp} and forwards to the analytics
// service under the real path. Rejections rewrite r.URL.Path to a stable
// value: libs/metrics normalizePath labels the RAW first two path segments
// after the handler runs, so an attacker-chosen path would otherwise mint
// unbounded label values.
func (h *MaskedAnalyticsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	seg := chi.URLParam(r, "maskedSeg")
	target, ok := maskedAnalyticsEndpoints[chi.URLParam(r, "maskedEp")]
	if !ok || !validMaskedSegment(h.secret, seg, time.Now()) {
		r.URL.Path = "/api/masked-rejected"
		http.NotFound(w, r)
		return
	}
	// Forward() reads r.URL.Path verbatim for the analytics service (no
	// rewrite case in service/proxy.go) — hand it the real path.
	r.URL.Path = target
	h.proxy.proxy(w, r, "analytics")
}

// MaskedPathHintMiddleware stamps the current masked analytics base onto
// every /api response, so the SPA learns it from any bootstrap call it
// already makes (e.g. /api/policy/features/mine fires unconditionally at app
// start). Recomputed once per bucket, cached under a mutex.
func MaskedPathHintMiddleware(secret []byte) func(http.Handler) http.Handler {
	var mu sync.Mutex
	var cachedBucket int64 = -1
	var cachedBase string
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()
			bucket := now.Unix() / maskedBucketSeconds
			mu.Lock()
			if bucket != cachedBucket {
				cachedBucket = bucket
				cachedBase = CurrentMaskedAnalyticsBase(secret, now)
			}
			base := cachedBase
			mu.Unlock()
			w.Header().Set("X-AE-Cfg", base)
			next.ServeHTTP(w, r)
		})
	}
}
