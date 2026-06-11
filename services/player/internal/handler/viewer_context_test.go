package handler

// Tests for the aggregate anime-page viewer-context endpoint (page-fetch
// optimization 2026-06-11). The endpoint collapses rating / watchers-count /
// progress / watchlist entry / my-review / saved-combo into one optional-auth
// round-trip; these tests assert both the anonymous (public subset) and the
// authenticated (full) payloads, plus the null semantics for absent rows.

import (
	"context"
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

func setupViewerContextTestDB(t *testing.T) (*ViewerContextHandler, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	stmts := []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			name TEXT,
			name_ru TEXT,
			name_jp TEXT,
			poster_url TEXT,
			mal_id TEXT,
			episodes_count INTEGER DEFAULT 0,
			episodes_aired INTEGER DEFAULT 0
		)`,
		`CREATE TABLE genres (id TEXT PRIMARY KEY, name TEXT, name_ru TEXT)`,
		`CREATE TABLE anime_genres (anime_id TEXT, genre_id TEXT)`,
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			status TEXT DEFAULT 'watching',
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
		`CREATE TABLE review_reactions (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			review_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			emoji TEXT NOT NULL,
			created_at DATETIME,
			UNIQUE (review_id, user_id, emoji)
		)`,
		`CREATE TABLE watch_progress (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			episode_number INTEGER NOT NULL,
			progress INTEGER DEFAULT 0,
			duration INTEGER DEFAULT 0,
			completed INTEGER DEFAULT 0,
			watch_count INTEGER DEFAULT 1,
			dropped_off_at INTEGER,
			last_watched_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE (user_id, anime_id, episode_number)
		)`,
		`CREATE TABLE user_anime_preferences (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			player TEXT NOT NULL,
			language TEXT NOT NULL,
			watch_type TEXT NOT NULL,
			translation_id TEXT,
			translation_title TEXT,
			updated_at DATETIME,
			UNIQUE (user_id, anime_id)
		)`,
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error)
	}

	log, err := logger.New(logger.Config{Level: "error", Development: false, Encoding: "json"})
	require.NoError(t, err)

	listRepo := repo.NewListRepository(db)
	activityRepo := repo.NewActivityRepository(db)
	prefRepo := repo.NewPreferenceRepository(db)
	progressRepo := repo.NewProgressRepository(db)

	prefService := service.NewPreferenceService(prefRepo, log)
	progressService := service.NewProgressService(progressRepo, prefService, log)
	listService := service.NewListService(listRepo, activityRepo, prefRepo, progressRepo, nil, nil, log)
	reviewService := service.NewReviewService(listRepo, activityRepo, log)

	return NewViewerContextHandler(progressService, listService, reviewService, prefService, log), db
}

func viewerContextRequest(t *testing.T, animeID string, claims *authz.Claims) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/anime/"+animeID+"/viewer-context", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("animeId", animeID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	if claims != nil {
		ctx = authz.ContextWithClaims(ctx, claims)
	}
	return req.WithContext(ctx)
}

func decodeViewerContext(t *testing.T, w *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var resp struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Data, &obj))
	return obj
}

// Anonymous caller gets the public subset (rating, watchers_count) and null
// for every user-scoped field.
func TestViewerContext_Anonymous_PublicSubsetOnly(t *testing.T) {
	h, db := setupViewerContextTestDB(t)

	// Two watchers, one with a scored review.
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score, review_text, username)
		VALUES ('r1', 'user-A', 'anime-1', 'watching', 8, 'great', 'alice'),
		       ('r2', 'user-B', 'anime-1', 'watching', 0, '', 'bob')`).Error)

	w := httptest.NewRecorder()
	h.GetViewerContext(w, viewerContextRequest(t, "anime-1", nil))
	require.Equal(t, http.StatusOK, w.Code)

	obj := decodeViewerContext(t, w)
	assert.Equal(t, "2", string(obj["watchers_count"]))
	assert.NotEqual(t, "null", string(obj["rating"]), "rating must be populated for anonymous callers")
	for _, k := range []string{"progress", "watchlist_entry", "my_review", "combo"} {
		assert.Equalf(t, "null", string(obj[k]), "user-scoped field %q must be null for anonymous callers", k)
	}
}

// Authenticated caller with full state gets every field populated.
func TestViewerContext_Authenticated_FullPayload(t *testing.T) {
	h, db := setupViewerContextTestDB(t)

	require.NoError(t, db.Exec(`INSERT INTO animes (id, name, episodes_count) VALUES ('anime-1', 'Test Anime', 12)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score, episodes, review_text, username)
		VALUES ('r1', 'user-A', 'anime-1', 'watching', 9, 3, 'my review text', 'alice')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO watch_progress (id, user_id, anime_id, episode_number, progress, duration, completed)
		VALUES ('p1', 'user-A', 'anime-1', 1, 1200, 1440, 1),
		       ('p2', 'user-A', 'anime-1', 2, 300, 1440, 0)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO user_anime_preferences (id, user_id, anime_id, player, language, watch_type, translation_title)
		VALUES ('pref1', 'user-A', 'anime-1', 'kodik', 'ru', 'dub', 'AniLibria')`).Error)

	claims := &authz.Claims{UserID: "user-A", Username: "alice"}
	w := httptest.NewRecorder()
	h.GetViewerContext(w, viewerContextRequest(t, "anime-1", claims))
	require.Equal(t, http.StatusOK, w.Code)

	obj := decodeViewerContext(t, w)

	var progress []map[string]any
	require.NoError(t, json.Unmarshal(obj["progress"], &progress))
	assert.Len(t, progress, 2, "both progress rows must come back")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(obj["watchlist_entry"], &entry))
	assert.Equal(t, "watching", entry["status"])

	var review map[string]any
	require.NoError(t, json.Unmarshal(obj["my_review"], &review))
	assert.Equal(t, "my review text", review["review_text"])
	// my_review must use the canonical projection — wider AnimeListEntry
	// fields must not leak (SOCIAL-NF-01).
	for _, forbidden := range []string{"notes", "tags", "mal_id", "priority"} {
		_, present := review[forbidden]
		assert.Falsef(t, present, "forbidden key %q leaked into my_review", forbidden)
	}

	var combo map[string]any
	require.NoError(t, json.Unmarshal(obj["combo"], &combo))
	assert.Equal(t, "kodik", combo["player"])
	assert.Equal(t, "ru", combo["language"])
	assert.Equal(t, "dub", combo["watch_type"])

	var rating map[string]any
	require.NoError(t, json.Unmarshal(obj["rating"], &rating))
	assert.InDelta(t, 9.0, rating["average_score"], 0.001)
}

// Authenticated caller with NO state for this anime gets nulls (not errors)
// for the user-scoped fields — mirrors GetUserAnimeEntry/GetUserReview/
// GetAnimePreference null-on-missing semantics.
func TestViewerContext_Authenticated_NoState_NullsNotErrors(t *testing.T) {
	h, _ := setupViewerContextTestDB(t)

	claims := &authz.Claims{UserID: "user-Z", Username: "zoe"}
	w := httptest.NewRecorder()
	h.GetViewerContext(w, viewerContextRequest(t, "anime-unknown", claims))
	require.Equal(t, http.StatusOK, w.Code)

	obj := decodeViewerContext(t, w)
	assert.Equal(t, "0", string(obj["watchers_count"]))
	for _, k := range []string{"watchlist_entry", "my_review", "combo"} {
		assert.Equalf(t, "null", string(obj[k]), "field %q must be null when the user has no state", k)
	}
	// progress is an empty array (repo Find returns empty slice), encoded as [].
	assert.Contains(t, []string{"null", "[]"}, string(obj["progress"]))
}

// Legacy MAL imports park entries under anime_id="mal_{malId}" until first
// visit migrates them. The ?mal_id= fallback must surface such an entry.
func TestViewerContext_MalIDFallback_SurfacesLegacyEntry(t *testing.T) {
	h, db := setupViewerContextTestDB(t)

	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score, review_text, username)
		VALUES ('r1', 'user-A', 'mal_5114', 'completed', 0, '', 'alice')`).Error)

	claims := &authz.Claims{UserID: "user-A", Username: "alice"}
	req := viewerContextRequest(t, "anime-uuid-1", claims)
	q := req.URL.Query()
	q.Set("mal_id", "5114")
	req.URL.RawQuery = q.Encode()

	w := httptest.NewRecorder()
	h.GetViewerContext(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	obj := decodeViewerContext(t, w)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(obj["watchlist_entry"], &entry))
	assert.Equal(t, "mal_5114", entry["anime_id"], "legacy mal_ entry must surface so the frontend can migrate it")
	assert.Equal(t, "completed", entry["status"])
}

// Same legacy-entry scenario WITHOUT the ?mal_id= override: the handler must
// resolve the MAL id from the catalog-owned animes row itself. This is what
// lets the frontend prefetch viewer-context from a route guard before the
// anime metadata response (the old mal_id source) has arrived.
func TestViewerContext_MalIDFallback_ResolvedFromAnimesRow(t *testing.T) {
	h, db := setupViewerContextTestDB(t)

	require.NoError(t, db.Exec(`INSERT INTO animes (id, name, mal_id)
		VALUES ('anime-uuid-1', 'FMA: Brotherhood', '5114')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score, review_text, username)
		VALUES ('r1', 'user-A', 'mal_5114', 'completed', 0, '', 'alice')`).Error)

	claims := &authz.Claims{UserID: "user-A", Username: "alice"}
	w := httptest.NewRecorder()
	h.GetViewerContext(w, viewerContextRequest(t, "anime-uuid-1", claims))
	require.Equal(t, http.StatusOK, w.Code)

	obj := decodeViewerContext(t, w)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(obj["watchlist_entry"], &entry))
	assert.Equal(t, "mal_5114", entry["anime_id"], "legacy mal_ entry must surface without the query param")
	assert.Equal(t, "completed", entry["status"])
}

func TestViewerContext_MissingAnimeID_BadRequest(t *testing.T) {
	h, _ := setupViewerContextTestDB(t)

	w := httptest.NewRecorder()
	h.GetViewerContext(w, viewerContextRequest(t, "", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
