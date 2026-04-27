// Wave 0 RED test — references behavior introduced by Wave 1 plan 01-03.
// This file SHOULD fail (assertion failure, not compile failure) until Wave 1
// modifies handler.PreferenceHandler.ResolvePreference to accept anonymous
// callers. Going green is the Wave 1 acceptance gate (per phase 01 VALIDATION.md).
//
// Behavioral contract — the assertion below FREEZES the Wave 1 contract:
//   - POST to ResolvePreference with NO claims and NO X-Anon-ID returns 200
//     (currently 401). Wave 1 plan 01-03 replaces the early httputil.Unauthorized
//     return with a `var userID string` fallthrough so anon callers work.
//   - The repo path GetAnimePreference(ctx, "", animeID) returns nothing
//     (Tier 1 + Tier 2 skipped), and the resolver falls through to default
//     tier — the response body should still encode resolved != nil because
//     `available` contains at least one combo.

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

// setupAnonResolveTestDB stands up a minimal SQLite schema for the
// PreferenceRepository methods Resolve calls. All tables empty — ensures the
// resolver hits the default tier path with no Tier 1 hit for an anon user.
func setupAnonResolveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE user_anime_preferences (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		player TEXT NOT NULL,
		language TEXT NOT NULL,
		watch_type TEXT NOT NULL,
		translation_id TEXT,
		translation_title TEXT,
		updated_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE watch_histories (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		anime_id TEXT,
		player TEXT,
		language TEXT,
		watch_type TEXT,
		translation_title TEXT,
		duration_watched INTEGER DEFAULT 0,
		started_at DATETIME,
		created_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE pinned_translations (
		anime_id TEXT,
		translation_id INTEGER,
		translation_title TEXT,
		translation_type TEXT
	)`).Error)

	return db
}

func TestResolve_AcceptsAnon(t *testing.T) {
	db := setupAnonResolveTestDB(t)
	prefRepo := repo.NewPreferenceRepository(db)
	log := logger.Default()
	svc := service.NewPreferenceService(prefRepo, log)
	h := NewPreferenceHandler(svc, log)

	r := chi.NewRouter()
	r.Post("/preferences/resolve", h.ResolvePreference)

	body := domain.ResolveRequest{
		AnimeID: "anime-anon",
		Available: []domain.WatchCombo{{
			Player:           "kodik",
			Language:         "ru",
			WatchType:        "sub",
			TranslationID:    "963",
			TranslationTitle: "Crunchyroll",
		}},
	}
	buf := &bytes.Buffer{}
	require.NoError(t, json.NewEncoder(buf).Encode(body))

	// No claims, no X-Anon-ID — pure anonymous caller.
	req := httptest.NewRequest(http.MethodPost, "/preferences/resolve", buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"Wave 1 plan 01-03 makes ResolvePreference anon-friendly: no claims must yield 200, not 401")

	var resp struct {
		Data domain.ResolveResponse `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp), "response must decode as ResolveResponse")
	assert.NotNil(t, resp.Data.Resolved,
		"with one available combo the resolver must produce a resolved combo even for anon callers")
}
