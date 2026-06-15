package transport

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func testJWTConfig() authz.JWTConfig {
	return authz.JWTConfig{
		Secret:         "test-secret-admin-refresh",
		Issuer:         "animeenigma",
		AccessTokenTTL: 15 * time.Minute,
	}
}

// stubAuthServer returns an httptest server that imitates POST /api/auth/refresh.
// status controls the response code; on 200 it returns the given access token
// and a Set-Cookie header. calls counts how many times it was hit.
func stubAuthServer(t *testing.T, status int, accessToken string, calls *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "access_token", Value: accessToken, Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "rt_rotated", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		// Mirror the REAL auth service response, which wraps the payload in the
		// httputil.OK envelope ({success, data:{...}}) — NOT a flat
		// {access_token}. The flat stub previously here masked a decode bug in
		// doRefresh that 401'd every browser-driven admin session.
		_, _ = w.Write([]byte(`{"success":true,"data":{"access_token":"` + accessToken + `"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// captureNext is a terminal handler that records whether it ran and what
// Authorization header it saw.
type captureNext struct {
	called   bool
	authSeen string
}

func (c *captureNext) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	c.called = true
	c.authSeen = r.Header.Get("Authorization")
}

func TestAdminSessionRefresh_ValidAccessToken_NoAuthCall(t *testing.T) {
	var calls int32
	srv := stubAuthServer(t, http.StatusOK, "NEWTOKEN", &calls)

	cfg := testJWTConfig()
	pair, err := authz.NewJWTManager(cfg).GenerateTokenPair("u1", "admin", authz.RoleAdmin, "sid1")
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	next := &captureNext{}
	mw := AdminSessionRefreshMiddleware(cfg, srv.URL, logger.Default())(next)

	req := httptest.NewRequest(http.MethodGet, "/admin/grafana/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.AccessToken})
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if !next.called {
		t.Fatal("next was not called")
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected 0 auth refresh calls on valid-token fast path, got %d", got)
	}
}

func TestAdminSessionRefresh_ExpiredAccess_ValidRefresh_TopsUp(t *testing.T) {
	var calls int32
	srv := stubAuthServer(t, http.StatusOK, "NEWTOKEN", &calls)

	next := &captureNext{}
	mw := AdminSessionRefreshMiddleware(testJWTConfig(), srv.URL, logger.Default())(next)

	// No access token at all (cookie absent) + a refresh_token cookie present.
	req := httptest.NewRequest(http.MethodGet, "/admin/grafana/api/dashboards", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "rt_valid"})
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if !next.called {
		t.Fatal("next was not called")
	}
	if next.authSeen != "Bearer NEWTOKEN" {
		t.Fatalf("expected downstream Authorization 'Bearer NEWTOKEN', got %q", next.authSeen)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly 1 auth refresh call, got %d", got)
	}
	// The auth Set-Cookie headers must be relayed to the browser.
	gotCookies := rec.Result().Header.Values("Set-Cookie")
	if len(gotCookies) == 0 {
		t.Fatal("expected Set-Cookie headers relayed to the client, got none")
	}
	var sawAccess bool
	for _, c := range gotCookies {
		if len(c) >= len("access_token=") && c[:len("access_token=")] == "access_token=" {
			sawAccess = true
		}
	}
	if !sawAccess {
		t.Fatalf("expected a relayed access_token Set-Cookie, got %v", gotCookies)
	}
}

func TestAdminSessionRefresh_RefreshRejected_FallsThrough(t *testing.T) {
	var calls int32
	srv := stubAuthServer(t, http.StatusUnauthorized, "", &calls)

	next := &captureNext{}
	mw := AdminSessionRefreshMiddleware(testJWTConfig(), srv.URL, logger.Default())(next)

	req := httptest.NewRequest(http.MethodGet, "/admin/grafana/", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "rt_expired"})
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if !next.called {
		t.Fatal("next must still be called so JWTValidationMiddleware can 401")
	}
	if next.authSeen != "" {
		t.Fatalf("expected no Authorization rewrite on failed refresh, got %q", next.authSeen)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 auth refresh attempt, got %d", got)
	}
}

func TestAdminSessionRefresh_NoCredentials_NoAuthCall(t *testing.T) {
	var calls int32
	srv := stubAuthServer(t, http.StatusOK, "NEWTOKEN", &calls)

	next := &captureNext{}
	mw := AdminSessionRefreshMiddleware(testJWTConfig(), srv.URL, logger.Default())(next)

	// Neither an access token nor a refresh_token cookie.
	req := httptest.NewRequest(http.MethodGet, "/admin/grafana/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if !next.called {
		t.Fatal("next must be called so JWTValidationMiddleware can 401")
	}
	if next.authSeen != "" {
		t.Fatalf("expected no Authorization header, got %q", next.authSeen)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected 0 auth calls with no refresh cookie, got %d", got)
	}
}
