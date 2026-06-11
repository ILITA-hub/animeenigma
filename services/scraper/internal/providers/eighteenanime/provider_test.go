package eighteenanime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// newFixtureServer serves the captured fixtures: GET /?s= search, the episode
// page, and an mp4upload-style embed page (for the GetStream failover path).
func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	search, _ := os.ReadFile("testdata/search_results.html")
	episode, _ := os.ReadFile("testdata/episode_page.html")
	mp4embed, _ := os.ReadFile("testdata/embed_mp4upload.html")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" && r.URL.Query().Get("s") != "":
			_, _ = w.Write(search)
		case strings.Contains(r.URL.Path, "/embed-"):
			_, _ = w.Write(mp4embed)
		case strings.HasPrefix(r.URL.Path, "/hentai/"):
			_, _ = w.Write(episode)
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTestProvider(base string) *Provider {
	return New(Deps{BaseURL: base, SearchBase: base})
}

func TestProvider_Name(t *testing.T) {
	if New(Deps{}).Name() != "18anime" {
		t.Fatal("Name must be 18anime")
	}
}

func TestProvider_FindID(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p := newTestProvider(srv.URL)

	// "JK to Inkou Kyoushi 4" matches two real series (…-4 and …-4-feat-…);
	// bestMatch breaks the tie by order. Either base is a valid resolution.
	id, err := p.FindID(context.Background(), domain.AnimeRef{Title: "JK to Inkou Kyoushi 4"})
	if err != nil {
		t.Fatalf("FindID: %v", err)
	}
	if !strings.HasPrefix(id, "jk-to-inkou-kyoushi-4") {
		t.Fatalf("FindID base slug = %q", id)
	}

	if _, err := p.FindID(context.Background(), domain.AnimeRef{Title: "totally unrelated xyzzy"}); err == nil {
		t.Fatal("expected error for no match")
	}
}

func TestProvider_ListEpisodes(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p := newTestProvider(srv.URL)

	// Exact-base grouping: the shorter series must not absorb the "-feat-..." one.
	eps, err := p.ListEpisodes(context.Background(), "jk-to-inkou-kyoushi-4")
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(eps) != 2 || eps[0].Number != 1 || eps[1].Number != 2 {
		t.Fatalf("episodes wrong: %+v", eps)
	}
	if !strings.Contains(eps[0].ID, "jk-to-inkou-kyoushi-4-episode-1") {
		t.Fatalf("episode ID wrong: %q", eps[0].ID)
	}
}

func TestProvider_ListServers(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p := newTestProvider(srv.URL)

	servers, err := p.ListServers(context.Background(), "base", "472-akiba-girls-episode-1")
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	var hasMP4, hasTurbo bool
	for _, s := range servers {
		if s.ID == "mp4upload" {
			hasMP4 = true
		}
		if s.ID == "turbovid" {
			hasTurbo = true
		}
		if s.Type != domain.CategorySub {
			t.Fatalf("server %s type = %q, want sub", s.ID, s.Type)
		}
	}
	if !hasMP4 || !hasTurbo {
		t.Fatalf("expected both mp4upload + turbovid servers, got %+v", servers)
	}
	// mp4upload must come first (failover order).
	if servers[0].ID != "mp4upload" {
		t.Fatalf("expected mp4upload first, got %q", servers[0].ID)
	}
}

func TestProvider_resolveStream_Failover(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p := newTestProvider(srv.URL)

	// Mirror link must contain the "mp4upload" host token (supportedMirrors
	// filters on it) AND be fetchable from the fixture server (/embed- path).
	mirrors := []Mirror{{Link: srv.URL + "/embed-mp4upload-x.html", Quality: "FullHD"}}
	src, err := p.resolveStream(context.Background(), mirrors, "")
	if err != nil {
		t.Fatalf("resolveStream: %v", err)
	}
	if src.URL == "" || src.IsHLS {
		t.Fatalf("expected mp4 source, got %+v", src)
	}
	if src.Referer != "https://www.mp4upload.com/" {
		t.Fatalf("expected mp4upload referer, got %q", src.Referer)
	}
}

func TestProvider_resolveStream_ServerPin(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	p := newTestProvider(srv.URL)

	// Pinning a server not present in the mirror list yields no supported mirror.
	mirrors := []Mirror{{Link: srv.URL + "/embed-mp4upload-x.html"}} // mp4upload-only
	if _, err := p.resolveStream(context.Background(), mirrors, "turbovid"); err == nil {
		t.Fatal("expected error pinning absent server")
	}
}
