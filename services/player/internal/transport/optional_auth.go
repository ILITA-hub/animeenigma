package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// OptionalAuthMiddleware decodes a JWT from the Authorization header IF one is present
// and attaches the resulting Claims to the request context. It does NOT reject requests
// that lack a token or whose token is invalid — those simply continue without claims.
//
// Pair this middleware with handlers that:
//  1. Read claims via authz.ClaimsFromContext(r.Context()) — returns (nil, false) on miss.
//  2. Fall back to an X-Anon-ID header for anonymous callers.
//  3. Reject when BOTH JWT claims AND X-Anon-ID are missing (T-01-01: prevents
//     headerless flood as a DDoS amplification vector on anon-friendly endpoints).
//
// See .planning/phases/01-instrumentation-baseline/01-RESEARCH.md §Pattern 6.
func OptionalAuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	jwtManager := authz.NewJWTManager(jwtConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token != "" {
				if claims, err := jwtManager.ValidateAccessToken(token); err == nil {
					r = r.WithContext(authz.ContextWithClaims(r.Context(), claims))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
