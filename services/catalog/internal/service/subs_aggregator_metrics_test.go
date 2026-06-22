package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/jimaku"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeSubsAnimeRepo is a handwritten fake that satisfies animeRepoForSubs.
type fakeSubsAnimeRepo struct {
	anime *domain.Anime
}

func (f *fakeSubsAnimeRepo) GetByID(_ context.Context, _ string) (*domain.Anime, error) {
	return f.anime, nil
}

func (f *fakeSubsAnimeRepo) UpdateExternalIDs(_ context.Context, _ string, _, _ *string) error {
	return nil
}

func (f *fakeSubsAnimeRepo) UpdateAniListID(_ context.Context, _ string, _ string) error {
	return nil
}

// metricsTestRedis returns a Redis cache on DB 13, flushed before and after.
// If Redis is unreachable the test is skipped.
func metricsTestRedis(t *testing.T) *cache.RedisCache {
	t.Helper()
	host := "127.0.0.1"
	c, err := cache.New(cache.Config{Host: host, Port: 6379, DB: 13})
	if err != nil {
		t.Skipf("redis unreachable (%v); skipping metrics test", err)
	}
	_ = c.Client().FlushDB(context.Background()).Err()
	t.Cleanup(func() { _ = c.Client().FlushDB(context.Background()).Err(); _ = c.Close() })
	return c
}

const testAnimeIDMetrics = "anime-metrics-test-uuid"

// TestFetchAll_EmitsMetrics verifies that a non-cached FetchAll where Jimaku
// returns 2 tracks and OpenSubtitles is unconfigured emits:
//   - SubtitleProviderUp["jimaku"] == 1
//   - SubtitleResolveTotal["jimaku","ok"] == 1
//   - SubtitleResolveTotal["opensubtitles","unconfigured"] == 1
func TestFetchAll_EmitsMetrics(t *testing.T) {
	metrics.SubtitleProviderUp.Reset()
	metrics.SubtitleResolveTotal.Reset()

	// Jimaku httptest server: returns one entry and two subtitle files.
	// SearchByAnilistID calls GET /entries/search?anilist_id=...
	// GetFiles calls GET /entries/{id}/files
	anilistID := 123
	jimakuSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/entries/search" {
			entries := []jimaku.Entry{{ID: 1, AnilistID: &anilistID, Name: "Test Anime"}}
			_ = json.NewEncoder(w).Encode(entries)
			return
		}
		if r.URL.Path == fmt.Sprintf("/entries/%d/files", 1) {
			files := []jimaku.SubtitleFile{
				{Name: "ep08.ja.ass", URL: "https://example.com/sub1.ass"},
				{Name: "ep08.ja.srt", URL: "https://example.com/sub2.srt"},
			}
			_ = json.NewEncoder(w).Encode(files)
			return
		}
		http.NotFound(w, r)
	}))
	defer jimakuSrv.Close()

	jimakuClient := jimaku.NewClientWithURL("test-key", jimakuSrv.URL)

	// OpenSubtitles: unconfigured (empty API key → IsConfigured() == false).
	oscClient := opensubtitles.NewClient(opensubtitles.Config{APIKey: ""})

	// Anime: AniList ID set so Jimaku can search; no external IDs needed for opensubs.
	anime := &domain.Anime{
		ID:        testAnimeIDMetrics,
		AniListID: "123",
		Name:      "Test Anime",
	}

	agg := &SubsAggregator{
		jimaku:    jimakuClient,
		opensubs:  oscClient,
		idmap:     nil,
		animeRepo: &fakeSubsAnimeRepo{anime: anime},
		cache:     metricsTestRedis(t),
		log:       logger.Default(),
	}

	_, err := agg.FetchAll(context.Background(), testAnimeIDMetrics, 8, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := testutil.ToFloat64(metrics.SubtitleProviderUp.WithLabelValues("jimaku")); got != 1 {
		t.Fatalf("jimaku up = %v want 1", got)
	}
	if got := testutil.ToFloat64(metrics.SubtitleResolveTotal.WithLabelValues("jimaku", "ok")); got != 1 {
		t.Fatalf("jimaku ok = %v want 1", got)
	}
	// unconfigured opensubtitles → no up gauge series + unconfigured counter
	if got := testutil.ToFloat64(metrics.SubtitleResolveTotal.WithLabelValues("opensubtitles", "unconfigured")); got != 1 {
		t.Fatalf("opensubtitles unconfigured = %v want 1", got)
	}
}
