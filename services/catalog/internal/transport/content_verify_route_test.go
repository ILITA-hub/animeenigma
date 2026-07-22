package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/go-chi/chi/v5"
)

// fakeVerifySrc records the visitor key content_verify's Hint was called
// with, so the route test can assert scraperUserKey's authed/anon branch
// without reaching into the unexported handler package internals.
type fakeVerifySrc struct {
	hintVisitor string
}

func (f *fakeVerifySrc) RawVerdicts(_ context.Context, _ string) (json.RawMessage, error) {
	return json.RawMessage(`{"anime_id":"a1","providers":[]}`), nil
}
func (f *fakeVerifySrc) Hint(_, visitor, _ string) { f.hintVisitor = visitor }

// buildContentVerifyOnlyRouter mirrors transport.NewRouter's
// /api/anime/{animeId}/content-verify registration (OptionalAuthMiddleware
// wrap included) without needing the rest of the catalog router's real
// GORM-backed dependencies.
func buildContentVerifyOnlyRouter(h *handler.ContentVerifyHandler, jwtCfg authz.JWTConfig) chi.Router {
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Route("/anime", func(r chi.Router) {
			r.With(OptionalAuthMiddleware(jwtCfg)).Get("/{animeId}/content-verify", h.Get)
		})
	})
	return r
}

// TestContentVerifyRoute_AnonResolvesIPHashVisitor asserts an unauthenticated
// request still 200s (OptionalAuthMiddleware never 401s) and the visitor
// forwarded to Hint falls through to the anonymous "ip:"+hash branch of
// scraperUserKey.
func TestContentVerifyRoute_AnonResolvesIPHashVisitor(t *testing.T) {
	src := &fakeVerifySrc{}
	h := handler.NewContentVerifyHandler(src, nil, nil, nil)
	r := buildContentVerifyOnlyRouter(h, testJWTConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/anime/anime-1/content-verify", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	if src.hintVisitor == "" {
		t.Fatal("expected a non-empty visitor")
	}
	if len(src.hintVisitor) < 3 || src.hintVisitor[:3] != "ip:" {
		t.Fatalf("anon request should resolve the ip-hash branch, got visitor=%q", src.hintVisitor)
	}
}

// TestContentVerifyRoute_AuthedResolvesUIDVisitor is the regression test for
// the Task-11 fix-round-1 bug: without OptionalAuthMiddleware on this route,
// an authenticated caller's content-verify visit hint and the player's
// watching-hint (Task 12, which sends "u:"+uid directly) would dedupe as two
// different visitors for the same user, double-counting the +1 visit signal.
// Mints a real JWT via the same authz.JWTManager the middleware validates
// against (mirrors optional_auth_test.go's TestOptionalAuth_ValidJWT_
// AttachesClaims) so this exercises the actual "u:"+uid resolution, not just
// a 200-with-garbage-header smoke check.
func TestContentVerifyRoute_AuthedResolvesUIDVisitor(t *testing.T) {
	cfg := testJWTConfig()
	jm := authz.NewJWTManager(cfg)
	pair, err := jm.GenerateTokenPair("user-42", "tester", authz.RoleUser, "")
	if err != nil {
		t.Fatal(err)
	}

	src := &fakeVerifySrc{}
	h := handler.NewContentVerifyHandler(src, nil, nil, nil)
	r := buildContentVerifyOnlyRouter(h, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/anime/anime-1/content-verify", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	if src.hintVisitor != "u:user-42" {
		t.Fatalf("authed request should resolve scraperUserKey's u:+uid branch, got visitor=%q", src.hintVisitor)
	}
}

// TestContentVerifyRoute_MalformedJWT_StillResolvesAnon asserts the
// middleware's never-401 contract holds on this route too: a garbage bearer
// token doesn't reject the request, it just falls through to the anon key.
func TestContentVerifyRoute_MalformedJWT_StillResolvesAnon(t *testing.T) {
	src := &fakeVerifySrc{}
	h := handler.NewContentVerifyHandler(src, nil, nil, nil)
	r := buildContentVerifyOnlyRouter(h, testJWTConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/anime/anime-1/content-verify", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	if len(src.hintVisitor) < 3 || src.hintVisitor[:3] != "ip:" {
		t.Fatalf("malformed JWT must not reject or produce a uid visitor, got visitor=%q", src.hintVisitor)
	}
}
