package anime365

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchSeriesByMAL_InvalidIDReturnsZeroNoError(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://127.0.0.1:0", Enabled: true})
	for _, bad := range []string{"", "0", "-5", "abc"} {
		id, err := c.SearchSeriesByMAL(context.Background(), bad, "x")
		if err != nil || id != 0 {
			t.Fatalf("mal %q: got (%d, %v), want (0, nil)", bad, id, err)
		}
	}
}

func TestSearchSeriesByMAL_MatchesMyAnimeListID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/series" {
			_, _ = w.Write([]byte(`{"data":[{"id":111,"myAnimeListId":999},{"id":28440,"myAnimeListId":51553}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	id, err := c.SearchSeriesByMAL(context.Background(), "51553", "Tongari Boushi no Atelier")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if id != 28440 {
		t.Fatalf("series id = %d, want 28440", id)
	}
}

func TestSearchSeriesByMAL_NoMatchReturnsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":111,"myAnimeListId":999}]}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	id, err := c.SearchSeriesByMAL(context.Background(), "51553", "x")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if id != 0 {
		t.Fatalf("series id = %d, want 0 (no match)", id)
	}
}

func TestListEpisodes_DecodesFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":349360,"episodeInt":"1","episodeType":"tv","isActive":true},
			{"id":371073,"episodeInt":"1","episodeType":"preview","isActive":true},
			{"id":380283,"episodeInt":"12","episodeType":"tv","isActive":true}
		]}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	eps, err := c.ListEpisodes(context.Background(), 28440)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(eps) != 3 || eps[2].ID != 380283 || eps[2].EpisodeInt != "12" {
		t.Fatalf("episodes = %+v", eps)
	}
	if eps[1].EpisodeType != "preview" {
		t.Fatalf("expected preview type, got %q", eps[1].EpisodeType)
	}
}

func TestListTranslations_DecodesFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":380283,"translations":[
			{"id":5825652,"typeKind":"sub","typeLang":"ru","authorsSummary":"Crunchyroll"},
			{"id":5819457,"typeKind":"sub","typeLang":"ru","authorsSummary":"Sa4ko aka Kiyoso"},
			{"id":111,"typeKind":"voice","typeLang":"ru","authorsSummary":"AniJoy"}
		]}}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	trs, err := c.ListTranslations(context.Background(), 380283)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(trs) != 3 || trs[0].ID != 5825652 || trs[0].TypeKind != "sub" || trs[0].TypeLang != "ru" {
		t.Fatalf("translations = %+v", trs)
	}
}
