package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
)

func TestRateLimitMiddleware_AllowsNormalTraffic(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         20,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitMiddleware(cfg)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("single request should pass, got status %d", w.Code)
	}
}

func TestRateLimitMiddleware_BlocksExcessiveRequests(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 1,
		BurstSize:         3,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitMiddleware(cfg)(inner)

	blocked := false
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}

	if !blocked {
		t.Error("rate limiter should block excessive requests with 429")
	}
}

func TestRateLimitMiddleware_DifferentIPsIndependent(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 1,
		BurstSize:         2,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitMiddleware(cfg)(inner)

	// Exhaust IP1's burst
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// IP2 should still pass
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("different IP should not be rate limited, got status %d", w.Code)
	}
}

func TestAdminRoleMiddleware_AdminAllowed(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("admin content"))
	})

	handler := AdminRoleMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	claims := &authz.Claims{
		UserID:   "admin-1",
		Username: "admin",
		Role:     authz.RoleAdmin,
	}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("admin should be allowed, got status %d", w.Code)
	}
}

func TestAdminRoleMiddleware_UserBlocked(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := AdminRoleMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	claims := &authz.Claims{
		UserID:   "user-1",
		Username: "regular_user",
		Role:     authz.RoleUser,
	}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("regular user should be blocked with 403, got status %d", w.Code)
	}
}

func TestAdminRoleMiddleware_NoClaims(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := AdminRoleMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	// No claims in context
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("request without claims should be blocked with 403, got status %d", w.Code)
	}
}
