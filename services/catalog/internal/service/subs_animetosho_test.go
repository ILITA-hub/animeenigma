package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animetosho"
)

// toshoAggTestServer mimics the AnimeTosho feed for AniDB 18884 (TenSura S4):
// an Erai-raws ep5 release plus an ASW ep5 re-encode, where only the
// Erai-raws one carries subtitle attachments (eng + rus + untagged + font).
func toshoAggTestServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	detailFetches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/json" && q.Get("aid") == "18884":
			_, _ = w.Write([]byte(`[
			  {"id": 900, "title": "[ASW] Tensei Shitara Slime Datta Ken S4 - 05 [1080p HEVC]", "num_files": 1},
			  {"id": 901, "title": "[Erai-raws] Tensei Shitara Slime Datta Ken 4th Season - 05 [1080p CR WEBRip][MultiSub]", "num_files": 1},
			  {"id": 902, "title": "[Erai-raws] Tensei Shitara Slime Datta Ken 4th Season - 04 [1080p CR WEBRip][MultiSub]", "num_files": 1}
			]`))
		case r.URL.Path == "/json" && q.Get("show") == "torrent" && q.Get("id") == "901":
			detailFetches++
			_, _ = w.Write([]byte(`{"files": [{
			  "filename": "[Erai-raws] Tensei Shitara Slime Datta Ken 4th Season - 05 [1080p CR WEBRip][MultiSub].mkv",
			  "attachments": [
			    {"id": 1, "type": "font", "info": {"name": "arial.ttf"}},
			    {"id": 2905415, "type": "subtitle", "info": {"codec": "ASS", "lang": "eng", "name": "CR"}},
			    {"id": 2905425, "type": "subtitle", "info": {"codec": "ASS", "lang": "rus", "name": "CR"}},
			    {"id": 2905754, "type": "subtitle", "info": {"codec": "ASS", "lang": "", "name": ""}}
			  ]}]}`))
		case r.URL.Path == "/json" && q.Get("show") == "torrent":
			detailFetches++
			_, _ = w.Write([]byte(`{"files": [{"filename": "x.mkv", "attachments": []}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, &detailFetches
}

func toshoTestAggregator(t *testing.T, srvURL string) *SubsAggregator {
	t.Helper()
	tc := animetosho.NewClient(animetosho.Config{FeedBaseURL: srvURL, StorageBaseURL: srvURL, Enabled: true})
	return NewSubsAggregator(SubsAggregatorDeps{Tosho: tc, Cache: resolveTestRedis(t), Log: logger.Default()})
}

func seedAniDB(t *testing.T, agg *SubsAggregator, animeID string, aniDBID int) {
	t.Helper()
	if err := agg.cache.Set(context.Background(), "subs:animetosho:anidb:"+animeID, aniDBID, aniDBHitTTL); err != nil {
		t.Fatalf("seed anidb cache: %v", err)
	}
}

func TestFetchAnimeTosho_ReturnsOfficialTracks(t *testing.T) {
	srv, fetches := toshoAggTestServer(t)
	defer srv.Close()

	agg := toshoTestAggregator(t, srv.URL)
	anime := &domain.Anime{ID: "uuid-tosho-1", Name: "Tensei shitara Slime Datta Ken 4th Season", Kind: "tv"}
	seedAniDB(t, agg, anime.ID, 18884)

	tracks, err := agg.fetchAnimeTosho(context.Background(), anime, 5)
	if err != nil {
		t.Fatalf("fetchAnimeTosho: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("want 2 tracks (eng+rus; untagged and font skipped), got %d: %+v", len(tracks), tracks)
	}
	byLang := map[string]SubtitleTrack{}
	for _, tr := range tracks {
		byLang[tr.Lang] = tr
	}
	ru, ok := byLang["ru"]
	if !ok {
		t.Fatalf("no ru track: %+v", tracks)
	}
	if ru.Provider != "animetosho" || ru.Format != "ass" {
		t.Fatalf("unexpected ru track: %+v", ru)
	}
	if ru.URL != "/api/anime/uuid-tosho-1/subtitles/animetosho/file/2905425" {
		t.Fatalf("unexpected ru URL: %s", ru.URL)
	}
	if !strings.Contains(ru.Label, "CR") || !strings.Contains(ru.Label, "Erai-raws") {
		t.Fatalf("unexpected ru label: %q", ru.Label)
	}
	// The Erai-raws release is preferred, so exactly one detail fetch: the
	// ASW re-encode (listed first, no subs) must not be consulted.
	if *fetches != 1 {
		t.Fatalf("want 1 detail fetch (preferred group first), got %d", *fetches)
	}
}

func TestFetchAnimeTosho_NoAniDBMappingIsSilentlyEmpty(t *testing.T) {
	srv, fetches := toshoAggTestServer(t)
	defer srv.Close()

	agg := toshoTestAggregator(t, srv.URL)
	anime := &domain.Anime{ID: "uuid-tosho-2", Name: "Unmapped Show", Kind: "tv"}
	seedAniDB(t, agg, anime.ID, 0) // cached miss: ARM had no AniDB id

	tracks, err := agg.fetchAnimeTosho(context.Background(), anime, 5)
	if err != nil {
		t.Fatalf("fetchAnimeTosho: %v", err)
	}
	if tracks != nil || *fetches != 0 {
		t.Fatalf("want no tracks and no fetches, got %+v / %d", tracks, *fetches)
	}
}

func TestFetchAnimeTosho_UnconfiguredSentinel(t *testing.T) {
	agg := NewSubsAggregator(SubsAggregatorDeps{Cache: resolveTestRedis(t), Log: logger.Default()})
	_, err := agg.fetchAnimeTosho(context.Background(), &domain.Anime{ID: "x"}, 1)
	if err != errProviderUnconfigured {
		t.Fatalf("want errProviderUnconfigured, got %v", err)
	}
}
