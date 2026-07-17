package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
)

// newUpcomingTestDB builds the minimal shared-schema slice the upcoming
// endpoint touches: animes (+franchise/status), anime_list, anime_genres,
// anime_tags/tags (S5/S2/S7 attr loaders), watch_history (S5), and the
// service-owned dismissals table.
func newUpcomingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, ddl := range []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY, name TEXT DEFAULT '', name_ru TEXT DEFAULT '',
			name_jp TEXT DEFAULT '', poster_url TEXT DEFAULT '',
			score REAL DEFAULT 0, episodes_count INTEGER DEFAULT 0,
			status TEXT DEFAULT 'released', year INTEGER DEFAULT 0,
			season TEXT DEFAULT '', kind TEXT DEFAULT '',
			franchise TEXT DEFAULT '', hidden INTEGER DEFAULT 0,
			deleted_at DATETIME
		)`,
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY, user_id TEXT, anime_id TEXT,
			status TEXT, score INTEGER
		)`,
		`CREATE TABLE anime_genres (anime_id TEXT, genre_id TEXT)`,
		`CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE anime_tags (anime_id TEXT, tag_id TEXT, rank INTEGER DEFAULT 0)`,
		`CREATE TABLE anime_studios (anime_id TEXT, studio_id TEXT)`, // S5 loadM2M touches it
		`CREATE TABLE watch_history (
			id TEXT PRIMARY KEY, user_id TEXT, anime_id TEXT,
			episode_number INTEGER, watched_at DATETIME
		)`,
		// S5.Score reads rec_user_signals via repo.GetUserSignals (nil row =
		// OK, missing TABLE = SQL error). Raw DDL (not AutoMigrate) — mirrors
		// setupRecsTestDB / setupS5TestDB: the real domain.RecUserSignals tag
		// `default:now()` is Postgres-only syntax and fails on SQLite
		// (`near "(": syntax error`), so every other test in this package
		// hand-rolls the table instead of AutoMigrate-ing it.
		`CREATE TABLE rec_user_signals (
			user_id TEXT PRIMARY KEY,
			s1_vector TEXT NOT NULL DEFAULT '{}',
			s5_affinity TEXT NOT NULL DEFAULT '{}',
			s6_seed_anime_id TEXT,
			s6_seed_completed_at DATETIME,
			s6_seed_score INTEGER,
			last_computed DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}
	// rec_announcement_dismissals AutoMigrates cleanly on SQLite (its domain
	// struct carries no Postgres-only default expressions) — Task 3's own
	// repo test already relies on this.
	require.NoError(t, db.AutoMigrate(&domain.RecAnnouncementDismissal{}))
	return db
}

func upcomingRequest(t *testing.T, h *UpcomingHandler, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/users/recs/upcoming", nil)
	if userID != "" {
		ctx := authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: userID})
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	h.GetUpcoming(rec, req)
	return rec
}

func seedUpcoming(t *testing.T, db *gorm.DB) {
	t.Helper()
	// User u1 loved franchise "frieren" (score 9 on seed-1).
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name, name_ru, franchise, status) VALUES
		('seed-1', 'Frieren', 'Фрирен', 'frieren', 'released'),
		('ann-franchise', 'Frieren S2', 'Фрирен 2', 'frieren', 'announced'),
		('ann-unrelated', 'Blob', 'Блоб', '', 'announced')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score)
		VALUES ('l1', 'u1', 'seed-1', 'completed', 9)`).Error)
	// Genre overlap for S2, deliberately asymmetric: seed-1 and ann-franchise
	// share all 4 genres (Jaccard=1.0); ann-unrelated shares only 1 of them
	// (Jaccard=0.25, under MinS2=0.3 — stays gated out). This ALSO matters for
	// S8: RankWithBreakdown's per-pool MinMaxNormalize treats a signal as
	// degenerate (all-zero) whenever only ONE candidate in the pool carries a
	// present raw value for it — S8 has exactly that shape here (only
	// ann-franchise has a non-empty franchise). Without a genuine two-point S2
	// spread, EVERY signal in the {ann-franchise, ann-unrelated} pool would
	// normalize to 0 and Final would be 0 for the matched item even though the
	// raw-score gate correctly passed it — the gate and the ordering score are
	// deliberately independent (see package doc), but ordering still needs a
	// non-degenerate pool to produce a positive Final for the assertion below.
	require.NoError(t, db.Exec(`INSERT INTO anime_genres (anime_id, genre_id) VALUES
		('seed-1', 'g1'), ('seed-1', 'g2'), ('seed-1', 'g3'), ('seed-1', 'g4'),
		('ann-franchise', 'g1'), ('ann-franchise', 'g2'), ('ann-franchise', 'g3'), ('ann-franchise', 'g4'),
		('ann-unrelated', 'g1')`).Error)
}

func TestUpcoming_Unauthorized(t *testing.T) {
	db := newUpcomingTestDB(t)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeRecsCache(), logger.Default(), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})
	rec := upcomingRequest(t, h, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUpcoming_FranchiseMatchReturnsItemWithReason(t *testing.T) {
	db := newUpcomingTestDB(t)
	seedUpcoming(t, db)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeRecsCache(), logger.Default(), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})

	rec := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data struct {
			Items []UpcomingItem `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data.Items, 1, "only the franchise-matched announcement passes the gate")
	it := env.Data.Items[0]
	assert.Equal(t, "ann-franchise", it.Anime.ID)
	assert.Equal(t, "franchise", it.Reason.Kind)
	assert.Equal(t, "seed-1", it.Reason.SeedAnimeID)
	assert.Equal(t, "Frieren", it.Reason.SeedAnimeName)
	assert.Equal(t, 9, it.Reason.UserScore)
	assert.Greater(t, it.MatchScore, 0.0)
}

func TestUpcoming_DismissedAndListedExcluded(t *testing.T) {
	db := newUpcomingTestDB(t)
	seedUpcoming(t, db)
	dismissals := repo.NewAnnouncementDismissalsRepository(db)
	h := NewUpcomingHandler(db, dismissals, newFakeRecsCache(), logger.Default(), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})

	// Dismiss the only eligible announcement → empty items.
	require.NoError(t, dismissals.Insert(context.Background(), "u1", "ann-franchise"))
	rec := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"items":[]`)
}

func TestUpcoming_ListedAnnouncementExcluded(t *testing.T) {
	db := newUpcomingTestDB(t)
	seedUpcoming(t, db)
	// u1 already planned the announced title → excluded from the pool.
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score)
		VALUES ('l2', 'u1', 'ann-franchise', 'plan_to_watch', NULL)`).Error)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeRecsCache(), logger.Default(), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})
	rec := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"items":[]`)
}

func TestUpcoming_WeakMatchesGatedOut(t *testing.T) {
	db := newUpcomingTestDB(t)
	// Announced title exists but user has NO affinity signals at all.
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name, franchise, status) VALUES
		('ann-1', 'Nobody Cares', '', 'announced')`).Error)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeRecsCache(), logger.Default(), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})
	rec := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"items":[]`)
}

func TestUpcomingDismiss_PersistsAndBustsCache(t *testing.T) {
	db := newUpcomingTestDB(t)
	seedUpcoming(t, db)
	dismissals := repo.NewAnnouncementDismissalsRepository(db)
	cache := newFakeRecsCache()
	h := NewUpcomingHandler(db, dismissals, cache, logger.Default(), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})

	// Warm the cache.
	rec := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec.Code)

	// Dismiss via handler.
	req := httptest.NewRequest(http.MethodPost, "/api/users/recs/upcoming/dismiss",
		strings.NewReader(`{"anime_id":"ann-franchise"}`))
	req = req.WithContext(authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: "u1"}))
	drec := httptest.NewRecorder()
	h.PostDismiss(drec, req)
	require.Equal(t, http.StatusOK, drec.Code)

	// Persisted + next GET recomputes without the dismissed title.
	ids, err := dismissals.ListAnimeIDs(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, []string{"ann-franchise"}, ids)

	rec2 := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), `"items":[]`)
}

func TestUpcomingDismiss_BadBody(t *testing.T) {
	db := newUpcomingTestDB(t)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeRecsCache(), logger.Default(), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})
	req := httptest.NewRequest(http.MethodPost, "/api/users/recs/upcoming/dismiss",
		strings.NewReader(`{}`))
	req = req.WithContext(authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: "u1"}))
	rec := httptest.NewRecorder()
	h.PostDismiss(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
