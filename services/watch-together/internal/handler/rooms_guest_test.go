package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/config"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/service"
)

// injectGuestClaims mirrors injectClaims but stamps role=guest — the identity
// a Watch Together invite-link guest carries. The gateway intentionally lets
// guest tokens reach the WT routes (so GET /rooms/{id} + the WS work), so the
// service's POST /rooms handler is the layer that must reject room creation.
func injectGuestClaims(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := &authz.Claims{
			UserID:   "guest_abc",
			Username: "Guest-1234",
			Role:     authz.RoleGuest,
		}
		next.ServeHTTP(w, r.WithContext(authz.ContextWithClaims(r.Context(), claims)))
	})
}

// TestCreate_Guest_Returns403 proves a guest (role=guest) cannot create a room
// even though its token is otherwise valid and reaches the handler.
func TestCreate_Guest_Returns403(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cfg := &config.Config{PublicBaseURL: "https://animeenigma.ru"}
	r := repo.NewRoomRepo(client, 900*time.Second, logger.Default())
	svc := service.NewRoomService(r, logger.Default())
	h := NewRoomHandler(svc, newFakeDeleteHub(), newFakeGrace(), cfg, logger.Default())

	router := chi.NewRouter()
	router.Use(injectGuestClaims)
	router.Route("/api/watch-together/rooms", func(r chi.Router) {
		r.Post("/", h.Create)
	})

	rec := doJSON(t, router, http.MethodPost, "/api/watch-together/rooms", validBody())
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (guests must not create rooms)", rec.Code)
	}
}
