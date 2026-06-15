package jackett

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/errors"
)

func mustWrite(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write body: %v", err)
	}
}

// fixture is a four-entry Jackett results payload covering every code
// path: an out-of-order seeder ranking, magnet-only (no info_hash),
// info_hash-only (synthesize magnet), and a Link-only entry that must be
// dropped (no magnet, no info hash).
const fixture = `{
  "Results": [
    {
      "Title": "[Erai-raws] Frieren - 01 [720p].mkv",
      "MagnetUri": "magnet:?xt=urn:btih:1111111111111111111111111111111111111111&dn=low",
      "InfoHash": "1111111111111111111111111111111111111111",
      "Size": 700000000,
      "Seeders": 5,
      "PublishDate": "2023-09-29T00:00:00"
    },
    {
      "Title": "[SubsPlease] Frieren - 01 (1080p) [ABCDEF12].mkv",
      "MagnetUri": "magnet:?xt=urn:btih:2222222222222222222222222222222222222222&dn=hi",
      "InfoHash": "",
      "Size": 1500000000,
      "Seeders": 99,
      "PublishDate": "2023-09-29T12:00:00Z"
    },
    {
      "Title": "Frieren 2160p HDR",
      "MagnetUri": "",
      "InfoHash": "3333333333333333333333333333333333333333",
      "Size": 9000000000,
      "Seeders": 42,
      "PublishDate": "2023-09-30T00:00:00"
    },
    {
      "Title": "Frieren Link-only no hash",
      "MagnetUri": "",
      "InfoHash": "",
      "Size": 123,
      "Seeders": 1000,
      "PublishDate": "2023-09-30T00:00:00"
    }
  ]
}`

func TestSearch_RanksSeedersAndDropsLinkOnly(t *testing.T) {
	var seenPath, seenRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		mustWrite(t, w, fixture)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, APIKey: "secret", Categories: []string{"5070"}})
	releases, err := c.Search(context.Background(), "frieren", 10)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	// URL shape: aggregated results path + apikey + Query + Category[].
	if seenPath != "/api/v2.0/indexers/all/results" {
		t.Errorf("path: want /api/v2.0/indexers/all/results, got %q", seenPath)
	}
	if !strings.Contains(seenRawQuery, "apikey=secret") {
		t.Errorf("query missing apikey: %q", seenRawQuery)
	}
	if !strings.Contains(seenRawQuery, "Query=frieren") {
		t.Errorf("query missing Query: %q", seenRawQuery)
	}
	if !strings.Contains(seenRawQuery, "Category%5B%5D=5070") {
		t.Errorf("query missing Category[]: %q", seenRawQuery)
	}

	// 4 entries in, 1 dropped (Link-only), 3 out.
	if len(releases) != 3 {
		t.Fatalf("expected 3 releases (1 dropped), got %d", len(releases))
	}

	// Ranked Seeders DESC: 99, 42, 5.
	if releases[0].Seeders != 99 || releases[1].Seeders != 42 || releases[2].Seeders != 5 {
		t.Fatalf("seeder ranking wrong: %d, %d, %d",
			releases[0].Seeders, releases[1].Seeders, releases[2].Seeders)
	}

	// Top entry: magnet-only path — InfoHash derived from MagnetUri.
	top := releases[0]
	if top.Source != "jackett" {
		t.Errorf("Source: want jackett, got %q", top.Source)
	}
	if top.InfoHash != "2222222222222222222222222222222222222222" {
		t.Errorf("InfoHash derive-from-magnet failed: %q", top.InfoHash)
	}
	if top.Quality != "1080p" {
		t.Errorf("Quality: want 1080p, got %q", top.Quality)
	}
	if top.Uploader != "SubsPlease" {
		t.Errorf("Uploader: want SubsPlease, got %q", top.Uploader)
	}
	if top.FoundAt.IsZero() {
		t.Error("FoundAt: should parse RFC3339")
	}

	// Middle entry: info_hash-only path — magnet synthesized from hash.
	mid := releases[1]
	if !strings.Contains(mid.Magnet, "urn:btih:3333333333333333333333333333333333333333") {
		t.Errorf("synthesized magnet missing hash: %q", mid.Magnet)
	}
	if mid.Quality != "2160p" {
		t.Errorf("Quality: want 2160p, got %q", mid.Quality)
	}
}

func TestSearch_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		mustWrite(t, w, `boom`)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, APIKey: "k"})
	_, err := c.Search(context.Background(), "x", 10)
	if err == nil {
		t.Fatal("expected error on 502 response, got nil")
	}
	if appErr, ok := errors.IsAppError(err); !ok || appErr.Code != errors.CodeExternalAPI {
		t.Errorf("expected ExternalAPI wrapped error, got %v", err)
	}
}

func TestSearch_LimitClampAndEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mustWrite(t, w, fixture)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, APIKey: "k"})
	releases, err := c.Search(context.Background(), "frieren", 2)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("limit=2 should clamp to 2, got %d", len(releases))
	}
	// Highest-seeded survive the cap.
	if releases[0].Seeders != 99 || releases[1].Seeders != 42 {
		t.Errorf("cap kept wrong entries: %d, %d", releases[0].Seeders, releases[1].Seeders)
	}

	// Empty Results array → 0 releases, no error.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mustWrite(t, w, `{"Results":[]}`)
	}))
	defer srv2.Close()
	c2 := NewClient(Config{BaseURL: srv2.URL, APIKey: "k"})
	rs, err := c2.Search(context.Background(), "nothing", 10)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(rs) != 0 {
		t.Fatalf("empty Results should yield 0 releases, got %d", len(rs))
	}
}
