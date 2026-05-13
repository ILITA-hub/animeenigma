package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB builds an in-memory sqlite DB with the canary's required schema:
// `animes` (mal_id, russian, updated_at, deleted_at) and `watch_history`
// (anime_id, watched_at). The schemas mirror the production tables loosely —
// only the columns the canary's composeAnimeList query touches.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// animes table (subset of services/catalog domain.Anime).
	require.NoError(t, db.Exec(`
		CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			mal_id INTEGER,
			russian TEXT,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`).Error)
	// watch_history (subset of services/player domain.WatchHistory).
	require.NoError(t, db.Exec(`
		CREATE TABLE watch_history (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			anime_id TEXT,
			episode_number INTEGER,
			watched_at DATETIME
		)
	`).Error)
	return db
}

// seedAnime inserts an anime row.
func seedAnime(t *testing.T, db *gorm.DB, id string, malID int, russian string, updatedAt time.Time) {
	t.Helper()
	require.NoError(t, db.Exec(`
		INSERT INTO animes (id, mal_id, russian, updated_at) VALUES (?, ?, ?, ?)
	`, id, malID, russian, updatedAt).Error)
}

// seedWatchHistory inserts a watch_history row.
func seedWatchHistory(t *testing.T, db *gorm.DB, animeID string, watchedAt time.Time) {
	t.Helper()
	require.NoError(t, db.Exec(`
		INSERT INTO watch_history (id, user_id, anime_id, episode_number, watched_at)
		VALUES (?, ?, ?, ?, ?)
	`, fmt.Sprintf("wh-%d-%s", watchedAt.UnixNano(), animeID), "user-1", animeID, 1, watchedAt).Error)
}

// fakeScraper builds an httptest server whose /scraper/servers returns the
// given list of servers and whose /scraper/stream returns a URL plus optional
// extra response headers. status overrides the 200 default for /scraper/servers
// when non-zero.
type fakeScraperConfig struct {
	servers     []map[string]any
	streamURL   string
	extraHdrs   map[string]string
	serversCode int // 0 → 200
	streamCode  int
}

func newFakeScraper(t *testing.T, cfg fakeScraperConfig) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/scraper/servers", func(w http.ResponseWriter, r *http.Request) {
		if cfg.serversCode != 0 {
			w.WriteHeader(cfg.serversCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"servers": cfg.servers,
				"meta":    map[string]any{"tried": []string{"gogoanime"}},
			},
		})
	})
	mux.HandleFunc("/scraper/stream", func(w http.ResponseWriter, r *http.Request) {
		if cfg.streamCode != 0 {
			w.WriteHeader(cfg.streamCode)
			return
		}
		for k, v := range cfg.extraHdrs {
			w.Header().Set(k, v)
		}
		// Per-server URL: include the requested server param so probeOne can
		// distinguish probe outcomes by server.
		server := r.URL.Query().Get("server")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"stream": map[string]any{
					"url":     cfg.streamURL + "?server=" + server,
					"headers": map[string]string{"Referer": "https://example.test/"},
				},
				"meta": map[string]any{"tried": []string{"gogoanime"}, "gated": true},
			},
		})
	})
	return httptest.NewServer(mux)
}

// stubProbe builds a streamprobe.Probe stand-in that classifies the URL into
// a Result by inspecting the `?server=` query param: streamhg → playable,
// vibeplayer → ad_decoy, anything else → zero_match.
func stubProbe(ctx context.Context, masterURL string, hdrs http.Header) streamprobe.Result {
	if strings.Contains(masterURL, "server=streamhg") {
		return streamprobe.Result{Playable: true, Reason: streamprobe.ReasonPlayable, Sampled: []string{"cdn.streamhg.test"}}
	}
	if strings.Contains(masterURL, "server=vibeplayer") {
		return streamprobe.Result{Playable: false, Reason: streamprobe.ReasonAdDecoy, Sampled: []string{"adcdn.test"}}
	}
	return streamprobe.Result{Playable: false, Reason: streamprobe.ReasonZeroMatch, Sampled: nil}
}

// newTestJob builds a ScraperPlayabilityCanaryJob wired against a fake scraper
// + stub Probe + temp report dir + fixed clock.
func newTestJob(t *testing.T, db *gorm.DB, fakeScraperURL, reportDir string) *ScraperPlayabilityCanaryJob {
	t.Helper()
	log := logger.Default()
	cfg := &config.JobsConfig{
		ScraperBaseURL:  fakeScraperURL,
		CanaryReportDir: reportDir,
	}
	j := NewScraperPlayabilityCanaryJob(db, cfg, log)
	j.probe = stubProbe
	j.rng = rand.New(rand.NewSource(42))
	now := time.Date(2026, 5, 13, 3, 0, 0, 0, time.UTC)
	j.now = func() time.Time { return now }
	// Tests want jitter to be effectively zero (no sleep).
	j.skipJitter = true
	return j
}

func TestCanary_AnimeListComposition_BothEmpty(t *testing.T) {
	db := newTestDB(t)
	scraper := newFakeScraper(t, fakeScraperConfig{servers: []map[string]any{}, streamURL: "http://cdn.test/master.m3u8"})
	defer scraper.Close()

	j := newTestJob(t, db, scraper.URL, t.TempDir())
	list, err := j.composeAnimeList(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 2, "expected only the two anchors when both watch_history + animes empty")
	assert.Equal(t, "anchor_frieren", list[0].Slot)
	assert.Equal(t, AnchorFrierenMAL, list[0].MALID)
	assert.Equal(t, "anchor_one_piece", list[1].Slot)
	assert.Equal(t, AnchorOnePieceMAL, list[1].MALID)
}

func TestCanary_AnimeListComposition_Anchors(t *testing.T) {
	db := newTestDB(t)
	// Seed the animes table with a row for Frieren so title resolution works.
	seedAnime(t, db, "uuid-frieren", AnchorFrierenMAL, "Фрирен", time.Now())
	scraper := newFakeScraper(t, fakeScraperConfig{servers: []map[string]any{}, streamURL: "http://cdn.test/master.m3u8"})
	defer scraper.Close()
	j := newTestJob(t, db, scraper.URL, t.TempDir())

	list, err := j.composeAnimeList(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(list), 2)
	assert.Equal(t, "anchor_frieren", list[0].Slot)
	assert.Equal(t, AnchorFrierenMAL, list[0].MALID)
	// Title resolution should pick up "Фрирен" from the animes row.
	assert.Equal(t, "Фрирен", list[0].Title)
	assert.Equal(t, "anchor_one_piece", list[1].Slot)
}

func TestCanary_AnimeListComposition_RecentFromWatchHistory(t *testing.T) {
	db := newTestDB(t)
	// 5 distinct anime, all watched within last hour. Anime "a3" is the most
	// recent, then a2, then a1. The canary should pick those 3 in that order
	// for recent_1, recent_2, recent_3.
	base := time.Now().Add(-30 * time.Minute)
	seedAnime(t, db, "uuid-a1", 1001, "anime-1", base)
	seedAnime(t, db, "uuid-a2", 1002, "anime-2", base)
	seedAnime(t, db, "uuid-a3", 1003, "anime-3", base)
	seedAnime(t, db, "uuid-a4", 1004, "anime-4", base)
	seedAnime(t, db, "uuid-a5", 1005, "anime-5", base)
	seedWatchHistory(t, db, "uuid-a1", base.Add(-5*time.Minute))  // 35min ago
	seedWatchHistory(t, db, "uuid-a2", base.Add(-3*time.Minute))  // 33min ago
	seedWatchHistory(t, db, "uuid-a3", base.Add(-1*time.Minute))  // 31min ago — most recent
	seedWatchHistory(t, db, "uuid-a4", base.Add(-10*time.Minute)) // 40min ago
	seedWatchHistory(t, db, "uuid-a5", base.Add(-15*time.Minute)) // 45min ago — oldest, won't make top 3

	scraper := newFakeScraper(t, fakeScraperConfig{servers: []map[string]any{}, streamURL: "http://cdn.test/master.m3u8"})
	defer scraper.Close()
	j := newTestJob(t, db, scraper.URL, t.TempDir())

	list, err := j.composeAnimeList(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 5)
	assert.Equal(t, "anchor_frieren", list[0].Slot)
	assert.Equal(t, "anchor_one_piece", list[1].Slot)
	assert.Equal(t, "recent_1", list[2].Slot)
	assert.Equal(t, 1003, list[2].MALID, "recent_1 should be the most recently watched anime")
	assert.Equal(t, "recent_2", list[3].Slot)
	assert.Equal(t, 1002, list[3].MALID)
	assert.Equal(t, "recent_3", list[4].Slot)
	assert.Equal(t, 1001, list[4].MALID)
}

func TestCanary_AnimeListComposition_FallbackToAnimeList(t *testing.T) {
	db := newTestDB(t)
	// No watch_history rows. animes table has 5 rows w/ varying updated_at.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedAnime(t, db, "uuid-fa1", 2001, "fa-1", base.Add(1*time.Hour))
	seedAnime(t, db, "uuid-fa2", 2002, "fa-2", base.Add(5*time.Hour)) // 2nd newest
	seedAnime(t, db, "uuid-fa3", 2003, "fa-3", base.Add(10*time.Hour)) // newest
	seedAnime(t, db, "uuid-fa4", 2004, "fa-4", base.Add(2*time.Hour))
	seedAnime(t, db, "uuid-fa5", 2005, "fa-5", base.Add(3*time.Hour)) // 3rd newest

	scraper := newFakeScraper(t, fakeScraperConfig{servers: []map[string]any{}, streamURL: "http://cdn.test/master.m3u8"})
	defer scraper.Close()
	j := newTestJob(t, db, scraper.URL, t.TempDir())

	list, err := j.composeAnimeList(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 5)
	assert.Equal(t, "anchor_frieren", list[0].Slot)
	assert.Equal(t, "anchor_one_piece", list[1].Slot)
	assert.Equal(t, "recent_1", list[2].Slot)
	assert.Equal(t, 2003, list[2].MALID, "recent_1 = newest by updated_at")
	assert.Equal(t, "recent_2", list[3].Slot)
	assert.Equal(t, 2002, list[3].MALID)
	assert.Equal(t, "recent_3", list[4].Slot)
	assert.Equal(t, 2005, list[4].MALID)
}

func TestCanary_EmitsMetric_PerTuple(t *testing.T) {
	db := newTestDB(t)
	// Seed 3 distinct anime in watch_history so the run hits all 5 anime_slots.
	base := time.Now()
	seedAnime(t, db, "uuid-x1", 3001, "x-1", base)
	seedAnime(t, db, "uuid-x2", 3002, "x-2", base)
	seedAnime(t, db, "uuid-x3", 3003, "x-3", base)
	seedWatchHistory(t, db, "uuid-x1", base.Add(-5*time.Minute))
	seedWatchHistory(t, db, "uuid-x2", base.Add(-3*time.Minute))
	seedWatchHistory(t, db, "uuid-x3", base.Add(-1*time.Minute))

	// Fake scraper exposes two servers per anime: streamhg (playable) and
	// vibeplayer (ad_decoy).
	scraper := newFakeScraper(t, fakeScraperConfig{
		servers: []map[string]any{
			{"id": "streamhg", "name": "StreamHG", "url": "https://ext.test/streamhg"},
			{"id": "vibeplayer", "name": "VibePlayer", "url": "https://ext.test/vibeplayer"},
		},
		streamURL: "http://cdn.test/master.m3u8",
	})
	defer scraper.Close()

	// Reset the counter so we can count exactly this run's emissions.
	metrics.PlayabilityCanaryRunsTotal.Reset()
	j := newTestJob(t, db, scraper.URL, t.TempDir())
	require.NoError(t, j.Run(context.Background()))

	// 5 anime × 2 servers = 10 unique (slot, server) tuples each incremented once.
	gotPass := testutil.ToFloat64(metrics.PlayabilityCanaryRunsTotal.WithLabelValues(
		"gogoanime", "streamhg", "pass", "playable", "anchor_frieren"))
	assert.Equal(t, 1.0, gotPass, "anchor_frieren+streamhg should emit one pass+playable")
	gotFail := testutil.ToFloat64(metrics.PlayabilityCanaryRunsTotal.WithLabelValues(
		"gogoanime", "vibeplayer", "fail", "ad_decoy", "anchor_frieren"))
	assert.Equal(t, 1.0, gotFail, "anchor_frieren+vibeplayer should emit one fail+ad_decoy")
}

func TestCanary_WritesPerRunLog(t *testing.T) {
	db := newTestDB(t)
	base := time.Now()
	seedAnime(t, db, "uuid-y1", 4001, "y-1", base)
	seedWatchHistory(t, db, "uuid-y1", base.Add(-2*time.Minute))

	// Fake scraper includes a SECRET token in its /scraper/stream response
	// headers (Authorization). The canary MUST redact it before serializing.
	scraper := newFakeScraper(t, fakeScraperConfig{
		servers: []map[string]any{
			{"id": "streamhg", "name": "StreamHG", "url": "https://ext.test/streamhg"},
		},
		streamURL: "http://cdn.test/master.m3u8",
		extraHdrs: map[string]string{
			"Authorization": "Bearer secret-token-xyz",
			"Set-Cookie":    "session=secret-cookie-abc",
		},
	})
	defer scraper.Close()

	reportDir := t.TempDir()
	metrics.PlayabilityCanaryRunsTotal.Reset()
	j := newTestJob(t, db, scraper.URL, reportDir)
	require.NoError(t, j.Run(context.Background()))

	// Exactly one .json file appears.
	entries, err := os.ReadDir(reportDir)
	require.NoError(t, err)
	var jsonFiles []os.DirEntry
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			jsonFiles = append(jsonFiles, e)
		}
	}
	require.Len(t, jsonFiles, 1, "exactly one JSON log file expected")

	// Filename matches YYYY-MM-DD-HHMMSS.json.
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-\d{6}\.json$`)
	assert.True(t, re.MatchString(jsonFiles[0].Name()),
		"filename %q does not match YYYY-MM-DD-HHMMSS.json", jsonFiles[0].Name())

	// Parse JSON and assert structure.
	content, err := os.ReadFile(filepath.Join(reportDir, jsonFiles[0].Name()))
	require.NoError(t, err)
	var run RunSummary
	require.NoError(t, json.Unmarshal(content, &run))
	require.NotZero(t, run.RunStartedAt)
	require.NotZero(t, run.RunEndedAt)
	require.NotEmpty(t, run.Results)

	// Defense-in-depth: secrets MUST NOT appear in the serialized log body.
	bodyStr := string(content)
	assert.NotContains(t, bodyStr, "secret-token-xyz", "Authorization secret leaked into per-run log")
	assert.NotContains(t, bodyStr, "secret-cookie-abc", "Set-Cookie secret leaked into per-run log")
}

func TestCanary_AllFiveAnimeSlots(t *testing.T) {
	db := newTestDB(t)
	base := time.Now()
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("uuid-z%d", i)
		seedAnime(t, db, id, 5000+i, fmt.Sprintf("z-%d", i), base)
		seedWatchHistory(t, db, id, base.Add(-time.Duration(i+1)*time.Minute))
	}
	scraper := newFakeScraper(t, fakeScraperConfig{
		servers: []map[string]any{
			{"id": "streamhg", "name": "StreamHG", "url": "https://ext.test/streamhg"},
		},
		streamURL: "http://cdn.test/master.m3u8",
	})
	defer scraper.Close()

	metrics.PlayabilityCanaryRunsTotal.Reset()
	j := newTestJob(t, db, scraper.URL, t.TempDir())
	require.NoError(t, j.Run(context.Background()))

	// Every literal slot should have at least one increment.
	for _, slot := range metrics.AnimeSlots() {
		v := testutil.ToFloat64(metrics.PlayabilityCanaryRunsTotal.WithLabelValues(
			"gogoanime", "streamhg", "pass", "playable", slot))
		assert.Equal(t, 1.0, v, "slot %q should have one pass+playable tuple", slot)
	}
}

func TestCanary_JitterIsBounded(t *testing.T) {
	j := &ScraperPlayabilityCanaryJob{rng: rand.New(rand.NewSource(42))}
	var minD, maxD time.Duration
	for i := 0; i < 1000; i++ {
		d := j.computeJitter()
		if i == 0 || d < minD {
			minD = d
		}
		if i == 0 || d > maxD {
			maxD = d
		}
	}
	assert.GreaterOrEqual(t, minD, -5*time.Minute, "jitter min < -5min")
	assert.LessOrEqual(t, maxD, 5*time.Minute, "jitter max > +5min")
	// Sanity: span should be reasonably wide given 1000 samples.
	assert.Greater(t, maxD-minD, 5*time.Minute, "jitter span too narrow — RNG may be broken")
}

func TestCanary_ScraperUnreachable_DoesNotPanic(t *testing.T) {
	db := newTestDB(t)
	// /scraper/servers returns 503.
	scraper := newFakeScraper(t, fakeScraperConfig{
		serversCode: http.StatusServiceUnavailable,
	})
	defer scraper.Close()

	metrics.PlayabilityCanaryRunsTotal.Reset()
	j := newTestJob(t, db, scraper.URL, t.TempDir())

	// Run does not panic and returns nil — failures are recorded as metric
	// tuples, not propagated as job errors.
	err := j.Run(context.Background())
	assert.NoError(t, err)

	// At least one anime_slot should have a fail+cdn_unreachable+_unreachable tuple.
	v := testutil.ToFloat64(metrics.PlayabilityCanaryRunsTotal.WithLabelValues(
		"gogoanime", "_unreachable", "fail", "cdn_unreachable", "anchor_frieren"))
	assert.Equal(t, 1.0, v, "expected anchor_frieren+_unreachable+fail+cdn_unreachable when scraper down")
}
