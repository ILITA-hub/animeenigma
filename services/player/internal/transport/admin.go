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
// Phase 14 (REC-ADMIN-01) — mirrors services/themes/internal/transport
// AdminRoleMiddleware. Used by the /api/admin/recs/* group on the player
// service for the admin debug page + force-recompute endpoints.
func AdminRoleMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authz.IsAdmin(r.Context()) {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
