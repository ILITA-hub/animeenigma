package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// OptionalAuthMiddleware validates a Bearer token if present and stashes Claims
// in the context; missing/invalid tokens pass through as anonymous.
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
