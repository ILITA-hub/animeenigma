package handler

// Plan-03 tests for SOCIAL-04a (happy path), SOCIAL-04b (empty body),
// SOCIAL-04c (non-owner PATCH). Mapped to `01-VALIDATION.md` rows
// 01-Comment-01 / 01-Comment-02 / 01-Comment-03.
//
// SQLite caveat (same as repo/service tests): AutoMigrate(&domain.Comment{})
// fails on SQLite because the production tags carry Postgres-only defaults
// (`default:gen_random_uuid()` / `default:now()`). The schema is therefore
// created via raw SQL, mirroring production but with
// `lower(hex(randomblob(16)))` as the id default.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// withCommentClaims attaches a *authz.Claims to the request context.
// Mirrors the claims-into-context idiom used by mal_import_test.go.
func withCommentClaims(r *http.Request, userID, username string, role authz.Role) *http.Request {
	claims := &authz.Claims{UserID: userID, Username: username, Role: role}
	return r.WithContext(authz.ContextWithClaims(r.Context(), claims))
}

// setupCommentHandlerTest builds the SQLite schema for comments +
// activity_events, wires repo → service → handler, and registers chi
// routes that mirror plan-04's intended router shape so chi.URLParam
// resolves `animeId` / `commentId` correctly.
func setupCommentHandlerTest(t *testing.T) (*chi.Mux, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "open in-memory sqlite")

	stmts := []string{
		`CREATE TABLE comments (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT NOT NULL,
			anime_id TEXT NOT NULL,
			username TEXT,
			body TEXT NOT NULL,
			parent_id TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at DATETIME
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

	commentRepo := repo.NewCommentRepository(db)
	activityRepo := repo.NewActivityRepository(db)
	svc := service.NewCommentService(commentRepo, activityRepo, log)
	h := NewCommentHandler(svc, log)

	r := chi.NewRouter()
	r.Route("/api/anime/{animeId}/comments", func(r chi.Router) {
		r.Get("/", h.ListComments)
		r.Post("/", h.CreateComment)
		r.Patch("/{commentId}", h.UpdateComment)
		r.Delete("/{commentId}", h.DeleteComment)
	})
	return r, db
}

// decodeData unwraps `{ "success": ..., "data": <body> }` and returns the
// inner JSON-encoded body as a map.
func decodeData(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var env struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &env), "decode response: %s", string(raw))
	return env.Data
}

// decodeError returns the `error.message` field of an envelope-wrapped
// error response.
func decodeError(t *testing.T, raw []byte) (code, message string) {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &env), "decode error response: %s", string(raw))
	return env.Error.Code, env.Error.Message
}

// TestCommentHandler_CreateComment_HappyPath validates SOCIAL-04a: a
// logged-in POST with a valid body returns 201 + the created comment.
func TestCommentHandler_CreateComment_HappyPath(t *testing.T) {
	router, _ := setupCommentHandlerTest(t)

	body, _ := json.Marshal(domain.CreateCommentRequest{Body: "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/anime/a1/comments/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withCommentClaims(req, "user-1", "alice", authz.RoleUser)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "expected 201; body=%s", w.Body.String())

	data := decodeData(t, w.Body.Bytes())
	assert.Equal(t, "hello", data["body"])
	assert.Equal(t, "user-1", data["user_id"])
	assert.Equal(t, "a1", data["anime_id"])
	assert.Equal(t, "alice", data["username"])
	id, _ := data["id"].(string)
	assert.NotEmpty(t, id, "id should be populated by the SQLite default")
}

// TestCommentHandler_CreateComment_EmptyBody validates SOCIAL-04b: a
// whitespace-only body returns 400 + an "empty" error message.
func TestCommentHandler_CreateComment_EmptyBody(t *testing.T) {
	router, _ := setupCommentHandlerTest(t)

	body, _ := json.Marshal(domain.CreateCommentRequest{Body: "   "})
	req := httptest.NewRequest(http.MethodPost, "/api/anime/a1/comments/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withCommentClaims(req, "user-1", "alice", authz.RoleUser)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "expected 400; body=%s", w.Body.String())

	code, msg := decodeError(t, w.Body.Bytes())
	assert.Equal(t, "INVALID_INPUT", code)
	assert.True(t,
		strings.Contains(strings.ToLower(msg), "cannot be empty"),
		"error message should mention 'cannot be empty'; got %q", msg,
	)
}

// TestCommentHandler_UpdateComment_NotOwner validates SOCIAL-04c: a PATCH
// from a non-owner non-admin user returns 403 and the comment body is
// unchanged. Admin override → 200 and the body IS changed.
func TestCommentHandler_UpdateComment_NotOwner(t *testing.T) {
	router, db := setupCommentHandlerTest(t)

	// Seed: alice owns the comment.
	createBody, _ := json.Marshal(domain.CreateCommentRequest{Body: "original"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/anime/a1/comments/", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = withCommentClaims(createReq, "alice-id", "alice", authz.RoleUser)
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)
	require.Equal(t, http.StatusCreated, createW.Code, "seed: %s", createW.Body.String())

	created := decodeData(t, createW.Body.Bytes())
	commentID, _ := created["id"].(string)
	require.NotEmpty(t, commentID)

	// PATCH as bob (non-admin) → 403.
	patchBody, _ := json.Marshal(domain.UpdateCommentRequest{Body: "hacked"})
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/anime/a1/comments/"+commentID, bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = withCommentClaims(patchReq, "bob-id", "bob", authz.RoleUser)
	patchW := httptest.NewRecorder()
	router.ServeHTTP(patchW, patchReq)

	require.Equal(t, http.StatusForbidden, patchW.Code, "non-owner non-admin must get 403; body=%s", patchW.Body.String())
	code, _ := decodeError(t, patchW.Body.Bytes())
	assert.Equal(t, "FORBIDDEN", code)

	// Comment body in DB is unchanged.
	var bodyInDB string
	require.NoError(t, db.Raw(
		`SELECT body FROM comments WHERE id = ?`, commentID,
	).Scan(&bodyInDB).Error)
	assert.Equal(t, "original", bodyInDB, "PATCH must not have mutated the row")

	// PATCH as bob (admin) → 200, body IS changed (admin override at the
	// service layer per CONTEXT.md).
	patchBody2, _ := json.Marshal(domain.UpdateCommentRequest{Body: "admin-edit"})
	patchReq2 := httptest.NewRequest(http.MethodPatch, "/api/anime/a1/comments/"+commentID, bytes.NewReader(patchBody2))
	patchReq2.Header.Set("Content-Type", "application/json")
	patchReq2 = withCommentClaims(patchReq2, "bob-id", "bob", authz.RoleAdmin)
	patchW2 := httptest.NewRecorder()
	router.ServeHTTP(patchW2, patchReq2)

	require.Equal(t, http.StatusOK, patchW2.Code, "admin override must return 200; body=%s", patchW2.Body.String())
	updated := decodeData(t, patchW2.Body.Bytes())
	assert.Equal(t, "admin-edit", updated["body"], "admin override must persist the edit")

	// Confirm DB.
	require.NoError(t, db.Raw(
		`SELECT body FROM comments WHERE id = ?`, commentID,
	).Scan(&bodyInDB).Error)
	assert.Equal(t, "admin-edit", bodyInDB)
}
