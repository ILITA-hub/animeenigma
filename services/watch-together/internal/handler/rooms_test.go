package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/config"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/service"
)

// testFixture bundles the moving parts an end-to-end handler test needs:
// a real chi router with the /rooms subtree mounted, a miniredis-backed
// service, and a small helper to mint claims-bearing requests.
type testFixture struct {
	router *chi.Mux
	svc    *service.RoomService
	cfg    *config.Config
	mr     *miniredis.Miniredis
}

// newFixture spins up a fresh fixture per test. The JWT secret is fixed so
// authenticated calls can mint a matching token via the authz package.
//
// We bypass AuthMiddleware in the unit tests by mounting an in-test
// middleware that injects a *Claims directly into the request context —
// equivalent to what the real AuthMiddleware would do for a valid token,
// but without the JWT round-trip. Tests that exercise the no-auth path
// mount a different middleware that calls httputil.Unauthorized.
func newFixture(t *testing.T) *testFixture {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cfg := &config.Config{
		PublicBaseURL: "https://animeenigma.ru",
	}
	r := repo.NewRoomRepo(client, 900*time.Second, logger.Default())
	svc := service.NewRoomService(r, logger.Default())
	// Pin uuid + now for deterministic assertions. Each test that needs a
	// stable room_id overrides via SetIDProviderForTest; the default
	// suffices for tests that only care about the response *shape*.
	svc.SetIDProviderForTest(func() string { return "room-fixed-id" })
	svc.SetClockForTest(func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) })

	h := NewRoomHandler(svc, cfg, logger.Default())

	router := chi.NewRouter()
	// Auth-bypass middleware: every request gets a synthesized claim for
	// "test-user". Tests that need to assert 401 behavior use newRouterNoAuth.
	router.Use(injectClaims("test-user", "test-username"))
	router.Route("/api/watch-together/rooms", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Delete("/{id}", h.Delete)
	})

	return &testFixture{router: router, svc: svc, cfg: cfg, mr: mr}
}

// newFixtureNoAuth builds a router with the REAL AuthMiddleware-equivalent
// rejection path (no claims in context → 401). We exercise it with a single
// canary request to prove unauthenticated callers cannot reach the handler.
//
// We do NOT mount the real transport.AuthMiddleware here because doing so
// would force the test to mint a real JWT against the package's secret —
// extra mechanism for a test that only needs to assert "no claims → 401".
// The handler's defensive userID == "" check is what we're testing.
func newFixtureNoAuth(t *testing.T) *chi.Mux {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cfg := &config.Config{PublicBaseURL: "http://localhost:8000"}
	r := repo.NewRoomRepo(client, 900*time.Second, logger.Default())
	svc := service.NewRoomService(r, logger.Default())
	h := NewRoomHandler(svc, cfg, logger.Default())

	router := chi.NewRouter()
	// No injectClaims — handler's defensive userID-empty check fires.
	router.Route("/api/watch-together/rooms", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Delete("/{id}", h.Delete)
	})
	return router
}

// injectClaims puts a synthesized authz.Claims into request context so
// downstream handlers see authenticated user_id == hardcoded value. Mirrors
// what transport.AuthMiddleware does after JWT validation succeeds.
func injectClaims(userID, username string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := &authz.Claims{
				UserID:   userID,
				Username: username,
				Role:     authz.RoleUser,
			}
			ctx := authz.ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// doJSON is a one-line helper for POSTing JSON bodies in tests.
func doJSON(t *testing.T, router http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// validBody returns a request body that passes Create validation.
func validBody() CreateRoomBody {
	return CreateRoomBody{
		AnimeID:       "anime-uuid-1",
		EpisodeID:     "ep-1",
		Player:        domain.PlayerAnimeLib,
		TranslationID: "translation-1",
	}
}

func TestCreate_NoAuth_Returns401(t *testing.T) {
	router := newFixtureNoAuth(t)
	rec := doJSON(t, router, http.MethodPost, "/api/watch-together/rooms", validBody())
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCreate_Success_Returns200WithExpectedShape(t *testing.T) {
	fx := newFixture(t)
	rec := doJSON(t, fx.router, http.MethodPost, "/api/watch-together/rooms", validBody())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	// libs/httputil.OK wraps the payload in {success, data}.
	var env struct {
		Success bool               `json:"success"`
		Data    CreateRoomResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body: %v\nbody=%s", err, rec.Body.String())
	}
	if !env.Success {
		t.Errorf("success = false, want true")
	}
	if env.Data.RoomID != "room-fixed-id" {
		t.Errorf("room_id = %q, want %q", env.Data.RoomID, "room-fixed-id")
	}
	wantInvite := "https://animeenigma.ru/watch/room/room-fixed-id"
	if env.Data.InviteURL != wantInvite {
		t.Errorf("invite_url = %q, want %q", env.Data.InviteURL, wantInvite)
	}
	wantWS := "wss://animeenigma.ru/api/watch-together/ws?room=room-fixed-id"
	if env.Data.WSUrl != wantWS {
		t.Errorf("ws_url = %q, want %q", env.Data.WSUrl, wantWS)
	}
}

func TestCreate_MalformedJSON_Returns400(t *testing.T) {
	fx := newFixture(t)
	req := httptest.NewRequest(http.MethodPost, "/api/watch-together/rooms",
		strings.NewReader(`{"anime_id": not-json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MissingFields_Returns400(t *testing.T) {
	cases := map[string]CreateRoomBody{
		"missing anime_id":       {EpisodeID: "ep", Player: domain.PlayerAnimeLib, TranslationID: "t"},
		"missing episode_id":     {AnimeID: "a", Player: domain.PlayerAnimeLib, TranslationID: "t"},
		"missing player":         {AnimeID: "a", EpisodeID: "ep", TranslationID: "t"},
		"missing translation_id": {AnimeID: "a", EpisodeID: "ep", Player: domain.PlayerAnimeLib},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t)
			rec := doJSON(t, fx.router, http.MethodPost, "/api/watch-together/rooms", body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", name, rec.Code)
			}
		})
	}
}

func TestCreate_UnknownPlayer_Returns400(t *testing.T) {
	fx := newFixture(t)
	body := validBody()
	body.Player = "vlc"
	rec := doJSON(t, fx.router, http.MethodPost, "/api/watch-together/rooms", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGet_LiveRoom_Returns200WithSnapshot(t *testing.T) {
	fx := newFixture(t)
	// Create first so the room exists in Redis.
	if _, err := fx.svc.Create(context.Background(), "test-user", "test-username", service.CreateRoomInput{
		AnimeID:       "a-1",
		EpisodeID:     "e-1",
		Player:        domain.PlayerAnimeLib,
		TranslationID: "t-1",
	}); err != nil {
		t.Fatalf("seed Create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/watch-together/rooms/room-fixed-id", nil)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Success bool                `json:"success"`
		Data    domain.RoomSnapshot `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if env.Data.Room.ID != "room-fixed-id" {
		t.Errorf("room.id = %q, want %q", env.Data.Room.ID, "room-fixed-id")
	}
	if env.Data.ProtocolVersion != domain.ProtocolVersion {
		t.Errorf("protocol_version = %q, want %q", env.Data.ProtocolVersion, domain.ProtocolVersion)
	}
}

func TestGet_MissingRoom_Returns410(t *testing.T) {
	fx := newFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/watch-together/rooms/no-such-room", nil)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "room expired") {
		t.Errorf("body does not contain 'room expired': %s", rec.Body.String())
	}
}

func TestDelete_Host_Returns204(t *testing.T) {
	fx := newFixture(t)
	if _, err := fx.svc.Create(context.Background(), "test-user", "test-username", service.CreateRoomInput{
		AnimeID:       "a-1",
		EpisodeID:     "e-1",
		Player:        domain.PlayerAnimeLib,
		TranslationID: "t-1",
	}); err != nil {
		t.Fatalf("seed Create: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/watch-together/rooms/room-fixed-id", nil)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	// Subsequent GET should report Gone.
	req2 := httptest.NewRequest(http.MethodGet, "/api/watch-together/rooms/room-fixed-id", nil)
	rec2 := httptest.NewRecorder()
	fx.router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusGone {
		t.Errorf("post-delete GET status = %d, want 410", rec2.Code)
	}
}

func TestDelete_NonHost_Returns403(t *testing.T) {
	// Build a fixture seeded by user-A, then issue the DELETE as user-B.
	// Two routers + two services — but they share the same miniredis so
	// the room user-A creates is visible to user-B's handler.
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cfg := &config.Config{PublicBaseURL: "https://animeenigma.ru"}
	r := repo.NewRoomRepo(client, 900*time.Second, logger.Default())

	// user-A's service creates the room.
	svcA := service.NewRoomService(r, logger.Default())
	svcA.SetIDProviderForTest(func() string { return "room-shared-id" })
	svcA.SetClockForTest(func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) })
	if _, err := svcA.Create(context.Background(), "user-A", "name-A", service.CreateRoomInput{
		AnimeID:       "a-1",
		EpisodeID:     "e-1",
		Player:        domain.PlayerAnimeLib,
		TranslationID: "t-1",
	}); err != nil {
		t.Fatalf("seed Create as user-A: %v", err)
	}

	// user-B's handler attempts the delete.
	svcB := service.NewRoomService(r, logger.Default())
	hB := NewRoomHandler(svcB, cfg, logger.Default())
	routerB := chi.NewRouter()
	routerB.Use(injectClaims("user-B", "name-B"))
	routerB.Route("/api/watch-together/rooms", func(r chi.Router) {
		r.Delete("/{id}", hB.Delete)
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/watch-together/rooms/room-shared-id", nil)
	rec := httptest.NewRecorder()
	routerB.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
	// Room must remain in Redis.
	if !mr.Exists("wt:room:room-shared-id") {
		t.Errorf("room was deleted despite 403 response — non-host should NOT delete")
	}
}

func TestDelete_Missing_Returns410(t *testing.T) {
	fx := newFixture(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/watch-together/rooms/no-such-room", nil)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410, body=%s", rec.Code, rec.Body.String())
	}
}

// TestWSURLFromBase_SchemeSwap is a direct unit test for the URL helper.
// The Create-handler tests above also exercise it indirectly via the
// PublicBaseURL=https://animeenigma.ru fixture, but the explicit table
// here documents the http→ws / https→wss swap for the next implementer.
func TestWSURLFromBase_SchemeSwap(t *testing.T) {
	cases := []struct {
		base   string
		roomID string
		want   string
	}{
		{"https://animeenigma.ru", "r1", "wss://animeenigma.ru/api/watch-together/ws?room=r1"},
		{"http://localhost:8000", "r2", "ws://localhost:8000/api/watch-together/ws?room=r2"},
		{"animeenigma.ru", "r3", "ws://animeenigma.ru/api/watch-together/ws?room=r3"},
	}
	for _, tc := range cases {
		got := wsURLFromBase(tc.base, tc.roomID)
		if got != tc.want {
			t.Errorf("wsURLFromBase(%q, %q) = %q, want %q", tc.base, tc.roomID, got, tc.want)
		}
	}
}
