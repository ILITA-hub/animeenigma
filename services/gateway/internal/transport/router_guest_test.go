package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
)

// TestBlockGuestRoleMiddleware verifies the gateway-side guest containment:
// a role=guest token is 403'd, every other role (and the anonymous / no-claims
// case, which JWTValidationMiddleware would already have rejected upstream)
// passes through to the next handler.
//
// 418 (I'm a teapot) is the pass-through sentinel — distinct from any status
// the middleware itself writes (403) so the two outcomes can't be confused.
func TestBlockGuestRoleMiddleware(t *testing.T) {
	const passThrough = http.StatusTeapot
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(passThrough)
	})
	h := BlockGuestRoleMiddleware(next)

	cases := []struct {
		name     string
		setClaim bool
		role     authz.Role
		wantCode int
	}{
		{"guest is blocked", true, authz.RoleGuest, http.StatusForbidden},
		{"user passes through", true, authz.RoleUser, passThrough},
		{"admin passes through", true, authz.RoleAdmin, passThrough},
		{"empty role passes through", true, authz.Role(""), passThrough},
		{"no claims passes through", false, "", passThrough},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
			if tc.setClaim {
				ctx := authz.ContextWithClaims(req.Context(), &authz.Claims{Role: tc.role})
				req = req.WithContext(ctx)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Errorf("role=%q claims=%v: got %d, want %d", tc.role, tc.setClaim, rec.Code, tc.wantCode)
			}
		})
	}
}
