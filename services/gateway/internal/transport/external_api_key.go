package transport

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

// ExternalAPIKeyMiddleware gates /worker/* routes with a static shared secret
// (X-API-Key header). It is a COARSE defense-in-depth filter — NOT the auth
// boundary. Real per-worker identity is established by the enroll→session→
// idx-bound-capability chain (Tasks 5/10). A single shared key across untrusted
// operators is intentionally weak at the per-operator level; rotate via env
// change + restart, and prefer per-operator keys or CF mTLS (CD-9) in Phase 2.
//
// Security properties:
//   - Constant-time comparison (subtle.ConstantTimeCompare) to prevent timing attacks.
//   - Fail-closed: an empty configured key rejects ALL requests (a missing env var
//     must not accidentally admit everything).
//   - Generic 401 body — no internal host/path/bucket/stack detail in the response.
func ExternalAPIKeyMiddleware(configuredKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fail-closed: if no key is configured, reject everything.
			// An empty configured key means "not set up yet" — never admit all.
			if configuredKey == "" {
				writeUnauthorized(w)
				return
			}

			provided := r.Header.Get("X-API-Key")

			// subtle.ConstantTimeCompare requires equal-length slices; it
			// returns 0 on any mismatch including length mismatch. An empty
			// provided value will never match a non-empty configured key.
			if subtle.ConstantTimeCompare([]byte(provided), []byte(configuredKey)) != 1 {
				writeUnauthorized(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// writeUnauthorized writes a generic 401 with a minimal JSON body. The body
// intentionally carries no internal detail (no stack, no host, no path) so the
// error is safe to return to untrusted GPU workers.
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
