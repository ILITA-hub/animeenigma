package jobs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newLogicATestDB builds an in-memory sqlite DB with the three tables the Logic A
// DISTINCT join touches: watch_history (player/language/user_id/anime_id),
// anime_list (status/updated_at/user_id/anime_id), and animes
// (id/shikimori_id/status/episodes_aired). The column names mirror production so
// the same query string runs in both sqlite tests and Postgres prod.
func newLogicATestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE watch_history (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			anime_id TEXT,
			episode_number INTEGER,
			player TEXT,
			language TEXT,
			watch_type TEXT,
			watched_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE anime_list (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			anime_id TEXT,
			status TEXT,
			updated_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			shikimori_id TEXT,
			status TEXT,
			episodes_aired INTEGER
		)
	`).Error)
	return db
}

func seedAnimeRow(t *testing.T, db *gorm.DB, id, shikimoriID, status string, episodesAired int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, shikimori_id, status, episodes_aired) VALUES (?, ?, ?, ?)`,
		id, shikimoriID, status, episodesAired).Error)
}

func seedListRow(t *testing.T, db *gorm.DB, userID, animeID, status string, updatedAt time.Time) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO anime_list (id, user_id, anime_id, status, updated_at) VALUES (?, ?, ?, ?, ?)`,
		userID+":"+animeID, userID, animeID, status, updatedAt).Error)
}

func seedWatchRow(t *testing.T, db *gorm.DB, userID, animeID, player, language string) {
	t.Helper()
	seedWatchRowWT(t, db, userID, animeID, player, language, "sub")
}

func seedWatchRowWT(t *testing.T, db *gorm.DB, userID, animeID, player, language, watchType string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO watch_history (id, user_id, anime_id, episode_number, player, language, watch_type, watched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID+":"+animeID+":"+player+":"+watchType, userID, animeID, 1, player, language, watchType, time.Now()).Error)
}

// capturingLibrary returns an httptest server that records every demand POST body
// to the demand endpoint, plus a function returning the captured bodies.
type capturedDemand struct {
	MalID   string `json:"mal_id"`
	Episode int    `json:"episode"`
	Reason  string `json:"reason"`
}

func capturingLibrary(t *testing.T, status int) (*httptest.Server, func() []capturedDemand) {
	t.Helper()
	var mu sync.Mutex
	var got []capturedDemand
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/library/autocache/demand" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var d capturedDemand
		_ = json.Unmarshal(body, &d)
		mu.Lock()
		got = append(got, d)
		mu.Unlock()
		w.WriteHeader(status)
	}))
	return srv, func() []capturedDemand {
		mu.Lock()
		defer mu.Unlock()
		out := make([]capturedDemand, len(got))
		copy(out, got)
		return out
	}
}

// TestLogicA_FiresOngoingDemandForJPAudioWatcher verifies the happy path: an
// ongoing anime with a watching + JP-audio + recent watcher fires exactly one
// ongoing demand carrying mal_id/episode=episodes_aired/reason="ongoing".
func TestLogicA_FiresOngoingDemandForJPAudioWatcher(t *testing.T) {
	db := newLogicATestDB(t)
	now := time.Now()
	seedAnimeRow(t, db, "a1", "111", "ongoing", 7)
	seedListRow(t, db, "u1", "a1", "watching", now.Add(-1*time.Hour))
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")

	srv, captured := capturingLibrary(t, http.StatusOK)
	defer srv.Close()

	j := NewAutocacheLogicAJob(db, srv.URL, 30, logger.Default())
	if err := j.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := captured()
	require.Len(t, got, 1)
	require.Equal(t, "111", got[0].MalID)
	require.Equal(t, 7, got[0].Episode)
	require.Equal(t, "ongoing", got[0].Reason)
}

// TestLogicA_RawPlayerAlsoQualifies verifies player='raw' qualifies as JP-audio
// even when language is not ja.
func TestLogicA_RawPlayerAlsoQualifies(t *testing.T) {
	db := newLogicATestDB(t)
	seedAnimeRow(t, db, "a1", "111", "ongoing", 3)
	seedListRow(t, db, "u1", "a1", "watching", time.Now())
	seedWatchRow(t, db, "u1", "a1", "raw", "")

	srv, captured := capturingLibrary(t, http.StatusOK)
	defer srv.Close()

	j := NewAutocacheLogicAJob(db, srv.URL, 30, logger.Default())
	require.NoError(t, j.Run(context.Background()))
	require.Len(t, captured(), 1)
}

// TestLogicA_SubCombosQualify verifies that ANY sub combo (kodik/ru/sub,
// english/en/sub) carries original Japanese audio and therefore fires an ongoing
// demand — two distinct ongoings → two demands.
func TestLogicA_SubCombosQualify(t *testing.T) {
	db := newLogicATestDB(t)
	seedAnimeRow(t, db, "a1", "111", "ongoing", 5)
	seedListRow(t, db, "u1", "a1", "watching", time.Now())
	seedWatchRowWT(t, db, "u1", "a1", "kodik", "ru", "sub")
	seedAnimeRow(t, db, "a2", "222", "ongoing", 5)
	seedListRow(t, db, "u1", "a2", "watching", time.Now())
	seedWatchRowWT(t, db, "u1", "a2", "english", "en", "sub")

	srv, captured := capturingLibrary(t, http.StatusOK)
	defer srv.Close()

	j := NewAutocacheLogicAJob(db, srv.URL, 30, logger.Default())
	require.NoError(t, j.Run(context.Background()))
	require.Len(t, captured(), 2)
}

// TestLogicA_DubFiresNothing verifies dub combos (replaced audio) produce no
// demand, regardless of player.
func TestLogicA_DubFiresNothing(t *testing.T) {
	db := newLogicATestDB(t)
	seedAnimeRow(t, db, "a1", "111", "ongoing", 5)
	seedListRow(t, db, "u1", "a1", "watching", time.Now())
	seedWatchRowWT(t, db, "u1", "a1", "kodik", "ru", "dub")
	seedAnimeRow(t, db, "a2", "222", "ongoing", 5)
	seedListRow(t, db, "u1", "a2", "watching", time.Now())
	seedWatchRowWT(t, db, "u1", "a2", "english", "en", "dub")

	srv, captured := capturingLibrary(t, http.StatusOK)
	defer srv.Close()

	j := NewAutocacheLogicAJob(db, srv.URL, 30, logger.Default())
	require.NoError(t, j.Run(context.Background()))
	require.Empty(t, captured())
}

// TestLogicA_StaleWatcherExcluded verifies the D8 recency predicate: a JP-audio
// watching row whose anime_list.updated_at is older than active_watcher_days
// produces no demand.
func TestLogicA_StaleWatcherExcluded(t *testing.T) {
	db := newLogicATestDB(t)
	seedAnimeRow(t, db, "a1", "111", "ongoing", 5)
	seedListRow(t, db, "u1", "a1", "watching", time.Now().AddDate(0, 0, -45)) // > 30d
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")

	srv, captured := capturingLibrary(t, http.StatusOK)
	defer srv.Close()

	j := NewAutocacheLogicAJob(db, srv.URL, 30, logger.Default())
	require.NoError(t, j.Run(context.Background()))
	require.Empty(t, captured())
}

// TestLogicA_NonOngoingExcluded verifies released/announced animes never fire.
func TestLogicA_NonOngoingExcluded(t *testing.T) {
	db := newLogicATestDB(t)
	seedAnimeRow(t, db, "a1", "111", "released", 12)
	seedListRow(t, db, "u1", "a1", "watching", time.Now())
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")

	srv, captured := capturingLibrary(t, http.StatusOK)
	defer srv.Close()

	j := NewAutocacheLogicAJob(db, srv.URL, 30, logger.Default())
	require.NoError(t, j.Run(context.Background()))
	require.Empty(t, captured())
}

// TestLogicA_NotWatchingExcluded verifies a JP-audio row whose list status is not
// 'watching' (e.g. planned) fires nothing.
func TestLogicA_NotWatchingExcluded(t *testing.T) {
	db := newLogicATestDB(t)
	seedAnimeRow(t, db, "a1", "111", "ongoing", 5)
	seedListRow(t, db, "u1", "a1", "planned", time.Now())
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")

	srv, captured := capturingLibrary(t, http.StatusOK)
	defer srv.Close()

	j := NewAutocacheLogicAJob(db, srv.URL, 30, logger.Default())
	require.NoError(t, j.Run(context.Background()))
	require.Empty(t, captured())
}

// TestLogicA_ZeroAiredOrEmptyMalSkipped verifies rows with episodes_aired<=0 or an
// empty shikimori_id are skipped (no valid target).
func TestLogicA_ZeroAiredOrEmptyMalSkipped(t *testing.T) {
	db := newLogicATestDB(t)
	seedAnimeRow(t, db, "a1", "111", "ongoing", 0) // zero aired → skip
	seedListRow(t, db, "u1", "a1", "watching", time.Now())
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")
	seedAnimeRow(t, db, "a2", "", "ongoing", 4) // empty shikimori_id → skip
	seedListRow(t, db, "u1", "a2", "watching", time.Now())
	seedWatchRow(t, db, "u1", "a2", "raw", "ja")

	srv, captured := capturingLibrary(t, http.StatusOK)
	defer srv.Close()

	j := NewAutocacheLogicAJob(db, srv.URL, 30, logger.Default())
	require.NoError(t, j.Run(context.Background()))
	require.Empty(t, captured())
}

// TestLogicA_DistinctPerAnime verifies two JP-audio watchers of the same ongoing
// collapse to a single demand (DISTINCT), and two distinct ongoings fire two.
func TestLogicA_DistinctPerAnime(t *testing.T) {
	db := newLogicATestDB(t)
	seedAnimeRow(t, db, "a1", "111", "ongoing", 5)
	seedListRow(t, db, "u1", "a1", "watching", time.Now())
	seedListRow(t, db, "u2", "a1", "watching", time.Now())
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")
	seedWatchRow(t, db, "u2", "a1", "raw", "ja")
	seedAnimeRow(t, db, "a2", "222", "ongoing", 8)
	seedListRow(t, db, "u1", "a2", "watching", time.Now())
	seedWatchRow(t, db, "u1", "a2", "ae", "ja")

	srv, captured := capturingLibrary(t, http.StatusOK)
	defer srv.Close()

	j := NewAutocacheLogicAJob(db, srv.URL, 30, logger.Default())
	require.NoError(t, j.Run(context.Background()))
	require.Len(t, captured(), 2)
}

// TestLogicA_SingleDemandFailureDoesNotAbortSweep verifies a non-2xx demand POST
// is counted but does not abort the loop, and Run returns nil (the JOIN
// succeeded).
func TestLogicA_SingleDemandFailureDoesNotAbortSweep(t *testing.T) {
	db := newLogicATestDB(t)
	seedAnimeRow(t, db, "a1", "111", "ongoing", 5)
	seedListRow(t, db, "u1", "a1", "watching", time.Now())
	seedWatchRow(t, db, "u1", "a1", "ae", "ja")
	seedAnimeRow(t, db, "a2", "222", "ongoing", 8)
	seedListRow(t, db, "u1", "a2", "watching", time.Now())
	seedWatchRow(t, db, "u1", "a2", "raw", "ja")

	// Library returns 500 for every POST — both demands fail but the sweep
	// completes and Run returns nil (JOIN itself succeeded).
	srv, captured := capturingLibrary(t, http.StatusInternalServerError)
	defer srv.Close()

	j := NewAutocacheLogicAJob(db, srv.URL, 30, logger.Default())
	require.NoError(t, j.Run(context.Background()))
	// Both demands were attempted despite each failing.
	require.Len(t, captured(), 2)
}

// TestLogicA_JoinFailureReturnsError verifies a broken schema (missing table)
// surfaces as a Run error so the JobService metrics wrap records a real failure.
func TestLogicA_JoinFailureReturnsError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// No tables created → the join fails.
	j := NewAutocacheLogicAJob(db, "http://library:8089", 30, logger.Default())
	if err := j.Run(context.Background()); err == nil {
		t.Fatal("want error on join failure, got nil")
	}
}
