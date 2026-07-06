package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// canAccess mirrors services/policy internal/domain.FeatureFlag.CanAccess
// EXACTLY (kept in sync by hand; policy is the source of truth). Order:
// guest→deny; deny-list→deny; allow-list→allow; everyone→allow; role→allow.
func canAccess(a audience, userID, role string) bool {
	if role == "guest" {
		return false
	}
	if userID != "" && audienceContains(a.DenyUsers, userID) {
		return false
	}
	if userID != "" && audienceContains(a.AllowUsers, userID) {
		return true
	}
	if audienceContains(a.Roles, "everyone") {
		return true
	}
	if role != "" && audienceContains(a.Roles, role) {
		return true
	}
	return false
}

func audienceContains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// featureAllowed resolves whether (userID, role) may reach a flag-gated route.
// Loaded snapshot with the key present → evaluate its audience. Cold start or
// an unknown key → the flag's failSafe: "everyone" opens, anything else
// (incl. blank/unknown) fails closed to admin-only.
func featureAllowed(cache *rulesetCache, key, userID, role string) bool {
	snap, loaded := cache.snapshot()
	if loaded {
		if a, ok := snap.Flags[key]; ok {
			return canAccess(a, userID, role)
		}
	}
	failSafe := ""
	if loaded {
		failSafe = snap.FailSafe[key]
	}
	if failSafe == "everyone" {
		return canAccess(audience{Roles: []string{"everyone"}}, userID, role)
	}
	return canAccess(audience{Roles: []string{"admin"}}, userID, role)
}

// FeatureGate returns middleware that 403s callers not in the flag's audience.
// Mount AFTER a JWT middleware (required or optional) so claims are in context.
func FeatureGate(key string, cache *rulesetCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := authz.UserIDFromContext(r.Context())
			role := string(authz.RoleFromContext(r.Context()))
			if featureAllowed(cache, key, userID, role) {
				next.ServeHTTP(w, r)
				return
			}
			httputil.Forbidden(w)
		})
	}
}
