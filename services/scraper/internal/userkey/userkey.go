// Package userkey carries an opaque per-user quota key (an authenticated user
// id, or a salted client-IP hash for anonymous callers) from the inbound
// catalog→scraper request (the X-AE-User header) onto the request context, so
// the deep sidecar.Client.ResolveEmbed call can stamp it onto the stealth-
// scraper request body for per-user session-quota accounting. The key is opaque
// and is never logged in clear or persisted.
package userkey

import (
	"context"
	"net/http"
)

// HeaderName is the request header the catalog sets and the scraper reads.
const HeaderName = "X-AE-User"

type ctxKey struct{}

// WithUserKey returns ctx carrying key (no-op stored value when empty).
func WithUserKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, ctxKey{}, key)
}

// FromContext returns the user key, or "" when unset.
func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}

// Middleware seeds the X-AE-User header value into the request context. A
// missing/empty header leaves the context unchanged (FromContext → "").
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if k := r.Header.Get(HeaderName); k != "" {
			r = r.WithContext(WithUserKey(r.Context(), k))
		}
		next.ServeHTTP(w, r)
	})
}
