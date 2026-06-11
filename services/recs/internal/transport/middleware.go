package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// AdminRoleMiddleware ensures the request carries claims with Role == admin.
// Mount AFTER AuthMiddleware in the chain so unauthenticated callers get
// 401 (from AuthMiddleware) rather than 403 (from this middleware).
//
// Copied from services/player/internal/transport/admin.go — player still
// owns its copy; this is an independent duplicate for the recs service.
func AdminRoleMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authz.IsAdmin(r.Context()) {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// OptionalAuthMiddleware decodes a JWT from the Authorization header IF one is present
// and attaches the resulting Claims to the request context. It does NOT reject requests
// that lack a token or whose token is invalid — those simply continue without claims.
//
// Copied from services/player/internal/transport/optional_auth.go — player still
// owns its copy; this is an independent duplicate for the recs service.
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
