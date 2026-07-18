package handler

// Regression test for the AUTO-report bug (2026-07-18, NANDIorg): POST
// /users/watchlist (first-time add) silently discarded the caller-supplied
// status and always inserted "plan_to_watch", even when the frontend had
// picked e.g. "watching" or "completed" for an anime not yet on the list.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupListAddHandlerTest(t *testing.T) (*chi.Mux, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "open in-memory sqlite")

	stmts := []string{
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			status TEXT DEFAULT 'plan_to_watch',
			score INTEGER DEFAULT 0,
			episodes INTEGER NOT NULL DEFAULT 0,
			notes TEXT,
			tags TEXT,
			review_text TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			is_rewatching INTEGER DEFAULT 0,
			rewatch_count INTEGER DEFAULT 0,
			priority TEXT,
			mal_id INTEGER,
			started_at DATETIME,
			completed_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE (user_id, anime_id)
		)`,
		`CREATE TABLE activity_events (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT,
			username TEXT,
			anime_id TEXT,
			type TEXT,
			old_value TEXT,
			new_value TEXT,
			content TEXT,
			created_at DATETIME,
			deleted_at DATETIME
		)`,
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error)
	}

	log, err := logger.New(logger.Config{Level: "error", Development: false, Encoding: "json"})
	require.NoError(t, err)

	listRepo := repo.NewListRepository(db)
	activityRepo := repo.NewActivityRepository(db)
	svc := service.NewListService(listRepo, activityRepo, nil, nil, nil, nil, nil, log)
	h := NewListHandler(svc, log)

	r := chi.NewRouter()
	r.Route("/api/users/watchlist", func(r chi.Router) {
		r.Post("/", h.AddToList)
	})
	return r, db
}

func withListClaims(r *http.Request, userID, username string) *http.Request {
	claims := &authz.Claims{UserID: userID, Username: username, Role: authz.RoleUser}
	return r.WithContext(authz.ContextWithClaims(r.Context(), claims))
}

// TestAddToList covers the three status-handling paths on first watchlist
// add: honoring a caller-supplied status (the regression case), defaulting
// when status is omitted, and falling back to the default for an
// unrecognized status rather than persisting it verbatim.
func TestAddToList(t *testing.T) {
	tests := []struct {
		name       string
		animeID    string
		reqBody    map[string]string
		wantStatus string
	}{
		{
			name:       "honors caller-supplied status",
			animeID:    "anime-1",
			reqBody:    map[string]string{"anime_id": "anime-1", "status": "watching"},
			wantStatus: "watching",
		},
		{
			name:       "defaults to plan_to_watch when status is omitted",
			animeID:    "anime-2",
			reqBody:    map[string]string{"anime_id": "anime-2"},
			wantStatus: "plan_to_watch",
		},
		{
			name:       "falls back to plan_to_watch for an unrecognized status",
			animeID:    "anime-3",
			reqBody:    map[string]string{"anime_id": "anime-3", "status": "bogus"},
			wantStatus: "plan_to_watch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, db := setupListAddHandlerTest(t)

			body, _ := json.Marshal(tt.reqBody)
			req := httptest.NewRequest(http.MethodPost, "/api/users/watchlist/", bytes.NewReader(body))
			req = withListClaims(req, "user-1", "alice")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusCreated, w.Code, "response: %s", w.Body.String())

			var status string
			require.NoError(t, db.Table("anime_list").
				Where("user_id = ? AND anime_id = ?", "user-1", tt.animeID).
				Select("status").Scan(&status).Error)
			assert.Equal(t, tt.wantStatus, status)
		})
	}
}
