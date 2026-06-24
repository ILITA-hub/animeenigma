package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anime365"
)

// anime365TestServer serves the three resolution endpoints for MAL 51553 /
// episode 12 (anime365 ep id 380283) with two RU subtitle translations.
func anime365TestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/series":
			_, _ = w.Write([]byte(`{"data":[{"id":28440,"myAnimeListId":51553}]}`))
		case r.URL.Path == "/api/episodes":
			_, _ = w.Write([]byte(`{"data":[
				{"id":371073,"episodeInt":"1","episodeType":"preview","isActive":true},
				{"id":380283,"episodeInt":"12","episodeType":"tv","isActive":true}
			]}`))
		case r.URL.Path == "/api/episodes/380283":
			_, _ = w.Write([]byte(`{"data":{"id":380283,"translations":[
				{"id":5825652,"typeKind":"sub","typeLang":"ru","authorsSummary":"Crunchyroll"},
				{"id":5819457,"typeKind":"sub","typeLang":"ru","authorsSummary":"Sa4ko"},
				{"id":222,"typeKind":"voice","typeLang":"ru","authorsSummary":"AniJoy"},
				{"id":333,"typeKind":"sub","typeLang":"en","authorsSummary":"SubsPlease"}
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestFetchAnime365_ReturnsRussianSubTracks(t *testing.T) {
	srv := anime365TestServer(t)
	defer srv.Close()

	a365 := anime365.NewClient(anime365.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, a365, nil, nil, resolveTestRedis(t), nil, logger.Default())

	anime := &domain.Anime{ID: "uuid-1", MALID: "51553", Name: "Tongari Boushi no Atelier", Kind: "tv"}
	tracks, err := agg.fetchAnime365(context.Background(), anime, 12)
	if err != nil {
		t.Fatalf("fetchAnime365: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2 (RU subs only): %+v", len(tracks), tracks)
	}
	for _, tr := range tracks {
		if tr.Lang != "ru" || tr.Provider != "anime365" || tr.Format != "ass" {
			t.Fatalf("bad track: %+v", tr)
		}
	}
	if tracks[0].URL != "/api/anime/uuid-1/subtitles/anime365/file/5825652" {
		t.Fatalf("url = %q", tracks[0].URL)
	}
}

func TestFetchAnime365_UnknownAnimeReturnsEmptyNoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`)) // no series matches
	}))
	defer srv.Close()

	a365 := anime365.NewClient(anime365.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, a365, nil, nil, resolveTestRedis(t), nil, logger.Default())

	anime := &domain.Anime{ID: "uuid-2", MALID: "999999", Name: "Nope", Kind: "tv"}
	tracks, err := agg.fetchAnime365(context.Background(), anime, 1)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(tracks) != 0 {
		t.Fatalf("tracks = %+v, want empty", tracks)
	}
}

func TestFetchAnime365_DisabledIsUnconfigured(t *testing.T) {
	a365 := anime365.NewClient(anime365.Config{Enabled: false})
	agg := NewSubsAggregator(nil, nil, a365, nil, nil, resolveTestRedis(t), nil, logger.Default())
	_, err := agg.fetchAnime365(context.Background(), &domain.Anime{ID: "x", MALID: "1"}, 1)
	if err != errProviderUnconfigured {
		t.Fatalf("err = %v, want errProviderUnconfigured", err)
	}
}
