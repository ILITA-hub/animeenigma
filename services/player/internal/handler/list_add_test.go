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

// TestAddToList_HonorsCallerSuppliedStatus is the regression case: posting
// status "watching" for a brand-new entry must persist "watching", not
// silently downgrade to "plan_to_watch".
func TestAddToList_HonorsCallerSuppliedStatus(t *testing.T) {
	router, db := setupListAddHandlerTest(t)

	body, _ := json.Marshal(map[string]string{"anime_id": "anime-1", "status": "watching"})
	req := httptest.NewRequest(http.MethodPost, "/api/users/watchlist/", bytes.NewReader(body))
	req = withListClaims(req, "user-1", "alice")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "response: %s", w.Body.String())

	var status string
	require.NoError(t, db.Table("anime_list").
		Where("user_id = ? AND anime_id = ?", "user-1", "anime-1").
		Select("status").Scan(&status).Error)
	assert.Equal(t, "watching", status, "must persist the caller-supplied status, not fall back to plan_to_watch")
}

// TestAddToList_DefaultsToPlanToWatch preserves the original default for
// callers that omit status entirely.
func TestAddToList_DefaultsToPlanToWatch(t *testing.T) {
	router, db := setupListAddHandlerTest(t)

	body, _ := json.Marshal(map[string]string{"anime_id": "anime-2"})
	req := httptest.NewRequest(http.MethodPost, "/api/users/watchlist/", bytes.NewReader(body))
	req = withListClaims(req, "user-1", "alice")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "response: %s", w.Body.String())

	var status string
	require.NoError(t, db.Table("anime_list").
		Where("user_id = ? AND anime_id = ?", "user-1", "anime-2").
		Select("status").Scan(&status).Error)
	assert.Equal(t, "plan_to_watch", status)
}

// TestAddToList_RejectsInvalidStatus guards against an unrecognized status
// value silently being persisted (falls back to the plan_to_watch default).
func TestAddToList_RejectsInvalidStatus(t *testing.T) {
	router, db := setupListAddHandlerTest(t)

	body, _ := json.Marshal(map[string]string{"anime_id": "anime-3", "status": "bogus"})
	req := httptest.NewRequest(http.MethodPost, "/api/users/watchlist/", bytes.NewReader(body))
	req = withListClaims(req, "user-1", "alice")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "response: %s", w.Body.String())

	var status string
	require.NoError(t, db.Table("anime_list").
		Where("user_id = ? AND anime_id = ?", "user-1", "anime-3").
		Select("status").Scan(&status).Error)
	assert.Equal(t, "plan_to_watch", status)
}
