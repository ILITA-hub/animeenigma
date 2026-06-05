package handler

// Tests for Phase 1 (workstream: social) plan 02 — SOCIAL-NF-01 contract:
// the review endpoints' JSON wire shape must be EXACTLY the 9 canonical
// scalars + optional `anime` preload, even though the underlying
// `AnimeListEntry` row has many more fields (notes, tags, mal_id,
// is_rewatching, priority, started_at, completed_at, updated_at). The
// handler-local `reviewResponse` projection struct is what enforces this;
// these tests assert the projection is in place.
//
// 2026-05-21: `status` + `episodes` promoted from forbidden to allowed as
// part of the Steam-style review-context feature. See
// docs/superpowers/specs/2026-05-21-steam-style-review-context-design.md.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupReviewHandlerTestDB wires the schema + repos + service + handler the
// shape tests need. Uses the same SQLite fixture pattern as
// repo/list_review_test.go / service/review_test.go.
func setupReviewHandlerTestDB(t *testing.T) (*ReviewHandler, *gorm.DB) {
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
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error)
	}

	log, err := logger.New(logger.Config{Level: "error", Development: false, Encoding: "json"})
	require.NoError(t, err)
	listRepo := repo.NewListRepository(db)
	activityRepo := repo.NewActivityRepository(db)
	svc := service.NewReviewService(listRepo, activityRepo, log)
	return NewReviewHandler(svc, log), db
}

// seedHandlerListRow inserts an anime_list row with all the wider fields
// populated (status, episodes, notes, tags, etc.) so the test can prove the
// handler projection strips them.
func seedHandlerListRow(t *testing.T, db *gorm.DB, e domain.AnimeListEntry) {
	t.Helper()
	require.NoError(t, db.Exec(`
		INSERT INTO anime_list (
			id, user_id, anime_id, status, score, episodes, notes, tags,
			review_text, username, is_rewatching, priority, mal_id,
			started_at, completed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.UserID, e.AnimeID, e.Status, e.Score, e.Episodes,
		e.Notes, e.Tags, e.ReviewText, e.Username, e.IsRewatching,
		e.Priority, e.MalID, e.StartedAt, e.CompletedAt,
		e.CreatedAt, e.UpdatedAt,
	).Error)
}

// allowedReviewKeys is the canonical set of JSON keys the reviews endpoint
// is allowed to expose. `anime` is the optional Preload field.
var allowedReviewKeys = map[string]bool{
	"id":          true,
	"user_id":     true,
	"anime_id":    true,
	"username":    true,
	"score":       true,
	"review_text": true,
	"created_at":  true,
	"status":      true, // Steam-style review-context (2026-05-21)
	"episodes":    true, // Steam-style review-context (2026-05-21)
	"anime":       true,
}

// forbiddenLeakKeys are the AnimeListEntry-only keys that MUST NOT appear
// in the wire response. If the handler accidentally writes
// `*domain.AnimeListEntry` directly, every one of these leaks.
var forbiddenLeakKeys = []string{
	"notes", "tags", "mal_id",
	"is_rewatching", "priority", "started_at", "completed_at", "updated_at",
}

// decodeResponseData reads the httputil.Response wrapper and returns the
// `data` field as a json.RawMessage.
func decodeResponseData(t *testing.T, body *bytes.Buffer) json.RawMessage {
	t.Helper()
	var resp struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body.Bytes(), &resp))
	require.True(t, resp.Success, "response must be success=true")
	return resp.Data
}

// assertReviewShape asserts a single JSON object has only the canonical
// review keys (any subset of allowedReviewKeys), none of the forbidden
// leak keys, AND the two Steam-style context fields (status, episodes)
// are present in the body — proves the projection actually populates them.
func assertReviewShape(t *testing.T, obj map[string]json.RawMessage) {
	t.Helper()
	for k := range obj {
		assert.Truef(t, allowedReviewKeys[k],
			"unexpected key %q in review response — projection must strip AnimeListEntry-only fields", k)
	}
	for _, k := range forbiddenLeakKeys {
		_, present := obj[k]
		assert.Falsef(t, present, "forbidden key %q leaked into review response", k)
	}
	_, hasStatus := obj["status"]
	assert.Truef(t, hasStatus, "review response must include `status` for Steam-style context")
	_, hasEpisodes := obj["episodes"]
	assert.Truef(t, hasEpisodes, "review response must include `episodes` for Steam-style context")
}

// TestReviewHandler_GetAnimeReviews_ShapeIsCanonical — seed a row with
// ALL the wider AnimeListEntry fields populated; assert the JSON body
// elements expose only the 9 canonical scalars (+ optional `anime`).
func TestReviewHandler_GetAnimeReviews_ShapeIsCanonical(t *testing.T) {
	h, db := setupReviewHandlerTestDB(t)

	seedHandlerListRow(t, db, domain.AnimeListEntry{
		ID: "row-1", UserID: "user-A", AnimeID: "anime-1",
		Status: "watching", Score: 8, Episodes: 12,
		Notes: "abc", Tags: "action,drama",
		ReviewText: "great show", Username: "alice",
		IsRewatching: false, Priority: "high",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/anime/anime-1/reviews", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("animeId", "anime-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetAnimeReviews(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	data := decodeResponseData(t, w.Body)
	var list []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &list))
	require.Len(t, list, 1, "one seeded row")
	assertReviewShape(t, list[0])
}

// TestReviewHandler_CreateOrUpdateReview_ShapeIsCanonical — POST a new
// review; assert the response body has the canonical shape and no leaks.
func TestReviewHandler_CreateOrUpdateReview_ShapeIsCanonical(t *testing.T) {
	h, _ := setupReviewHandlerTestDB(t)

	body := bytes.NewBufferString(`{"anime_id":"anime-1","score":9,"review_text":"loved it"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/anime/anime-1/reviews", body)
	req.Header.Set("Content-Type", "application/json")
	// Inject auth claims.
	claims := &authz.Claims{UserID: "user-A", Username: "alice", Role: authz.RoleUser}
	req = req.WithContext(authz.ContextWithClaims(req.Context(), claims))

	w := httptest.NewRecorder()
	h.CreateOrUpdateReview(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	data := decodeResponseData(t, w.Body)
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &obj))
	assertReviewShape(t, obj)
}

// TestReviewHandler_GetUserReview_ShapeIsCanonical — GET /reviews/me
// after a review exists; assert the same shape contract.
func TestReviewHandler_GetUserReview_ShapeIsCanonical(t *testing.T) {
	h, db := setupReviewHandlerTestDB(t)
	seedHandlerListRow(t, db, domain.AnimeListEntry{
		ID: "row-1", UserID: "user-A", AnimeID: "anime-1",
		Status: "completed", Score: 9, Episodes: 0,
		Notes: "x", Tags: "y", ReviewText: "great", Username: "alice",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/anime/anime-1/reviews/me", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("animeId", "anime-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	claims := &authz.Claims{UserID: "user-A", Username: "alice", Role: authz.RoleUser}
	req = req.WithContext(authz.ContextWithClaims(req.Context(), claims))

	w := httptest.NewRecorder()
	h.GetUserReview(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	data := decodeResponseData(t, w.Body)
	// GetUserReview may return null for "no review"; here we seeded a row,
	// so expect a non-null object.
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &obj))
	assertReviewShape(t, obj)
}
