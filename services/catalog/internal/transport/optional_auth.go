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
// Plan 04 wraps this around the catalog `/home/spotlight` route ONLY — the
// rest of the catalog routes use the strict AuthMiddleware for admin paths and
// anonymous-friendly bare GETs for the catalog browse endpoints. The spotlight
// handler reads claims via authz.ClaimsFromContext to decide whether
// login-only cards (personal_pick login path, not_time_yet,
// continue_watching_new) are eligible for this caller.
//
// Behavioral contract — FROZEN by ports of the player-service tests:
//  1. missing Authorization header → next handler called WITHOUT claims attached
//  2. valid Bearer JWT → next handler called WITH claims attached
//  3. malformed/expired Bearer JWT → next handler called WITHOUT claims attached
//     (i.e. NEVER 401s — that's the inversion vs AuthMiddleware)
//
// Verbatim port of services/player/internal/transport/optional_auth.go. The
// behavioral contract is identical across both services.
//
// See services/player/internal/transport/optional_auth.go (the source) and
// .planning/workstreams/hero-spotlight/phases/03-dynamic-cards-migration/03-CONTEXT.md
// `<decisions>` Optional-auth middleware section.
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
