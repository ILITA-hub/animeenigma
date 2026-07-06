package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// AuthMiddleware rejects missing/invalid JWTs with 401 and attaches claims.
func AuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	m := authz.NewJWTManager(jwtConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token == "" {
				httputil.Unauthorized(w)
				return
			}
			claims, err := m.ValidateAccessToken(token)
			if err != nil {
				httputil.Unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(authz.ContextWithClaims(r.Context(), claims)))
		})
	}
}

// OptionalAuthMiddleware attaches claims IF a valid token is present; never rejects.
func OptionalAuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	m := authz.NewJWTManager(jwtConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token := httputil.BearerToken(r); token != "" {
				if claims, err := m.ValidateAccessToken(token); err == nil {
					r = r.WithContext(authz.ContextWithClaims(r.Context(), claims))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AdminRoleMiddleware requires Role==admin (mount AFTER AuthMiddleware).
func AdminRoleMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authz.IsAdmin(r.Context()) {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
