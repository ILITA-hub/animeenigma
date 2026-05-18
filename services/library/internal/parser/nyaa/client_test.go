package nyaa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/errors"
)

// mustWrite writes the body to w and fails the test on any error.
func mustWrite(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write body: %v", err)
	}
}

// rssFixture is a two-item Nyaa RSS feed shaped like the live site's
// `?page=rss` output. Namespace prefixes for nyaa: and dc: are declared
// on the rss root so encoding/xml's namespace-aware decoder can match
// the `xml:"namespace localname"` tags on rssItem.
const rssFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:nyaa="https://nyaa.si/xmlns/nyaa" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Nyaa - Search Results</title>
    <link>https://nyaa.si/</link>
    <description>RSS</description>
    <item>
      <title>[SubsPlease] Frieren - 01 (1080p) [ABCDEF12].mkv</title>
      <link>https://nyaa.si/download/1234.torrent</link>
      <pubDate>Sat, 18 May 2024 10:00:00 +0000</pubDate>
      <dc:creator>SubsPlease</dc:creator>
      <nyaa:infoHash>AABBCCDDEEFF00112233445566778899AABBCCDD</nyaa:infoHash>
      <nyaa:size>1.4 GiB</nyaa:size>
    </item>
    <item>
      <title>[Erai-raws] Frieren - 01 [720p][HEVC].mkv</title>
      <link>https://nyaa.si/download/5678.torrent</link>
      <pubDate>Sat, 18 May 2024 09:30:00 +0000</pubDate>
      <dc:creator>Erai-raws</dc:creator>
      <nyaa:infoHash>11223344556677889900aabbccddeeff11223344</nyaa:infoHash>
      <nyaa:size>700 MiB</nyaa:size>
    </item>
  </channel>
</rss>`

func TestSearch_RSSParsesFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		mustWrite(t, w, rssFixture)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	releases, err := c.Search(context.Background(), "frieren", 10)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}

	r0 := releases[0]
	if r0.Source != "nyaa" {
		t.Errorf("Source: want nyaa, got %q", r0.Source)
	}
	if r0.Uploader != "SubsPlease" {
		t.Errorf("Uploader: want SubsPlease, got %q", r0.Uploader)
	}
	if r0.Quality != "1080p" {
		t.Errorf("Quality: want 1080p, got %q", r0.Quality)
	}
	if r0.InfoHash != "aabbccddeeff00112233445566778899aabbccdd" {
		t.Errorf("InfoHash: want lowercase, got %q", r0.InfoHash)
	}
	if !strings.HasPrefix(r0.Magnet, "magnet:?xt=urn:btih:") {
		t.Errorf("Magnet: want magnet:?xt=urn:btih: prefix, got %q", r0.Magnet)
	}
	if !strings.Contains(r0.Magnet, "aabbccddeeff00112233445566778899aabbccdd") {
		t.Errorf("Magnet: want infohash embedded, got %q", r0.Magnet)
	}
	if r0.SizeBytes <= 0 {
		t.Errorf("SizeBytes: should be > 0, got %d", r0.SizeBytes)
	}
	// 1.4 GiB ≈ 1.5e9 bytes
	if r0.SizeBytes < 1_000_000_000 || r0.SizeBytes > 2_000_000_000 {
		t.Errorf("SizeBytes: want ~1.5GB, got %d", r0.SizeBytes)
	}
	if r0.FoundAt.IsZero() {
		t.Error("FoundAt: should be set from pubDate")
	}
	if r0.MALID != 0 {
		t.Errorf("MALID: Nyaa does not expose MAL IDs, want 0, got %d", r0.MALID)
	}

	if releases[1].Quality != "720p" {
		t.Errorf("releases[1].Quality: want 720p, got %q", releases[1].Quality)
	}
	if releases[1].Uploader != "Erai-raws" {
		t.Errorf("releases[1].Uploader: want Erai-raws, got %q", releases[1].Uploader)
	}
}

func TestSearch_QueryParameters(t *testing.T) {
	type captured struct {
		q, c, f, page string
	}
	var cap captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.q = r.URL.Query().Get("q")
		cap.c = r.URL.Query().Get("c")
		cap.f = r.URL.Query().Get("f")
		cap.page = r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/rss+xml")
		mustWrite(t, w, `<?xml version="1.0"?><rss><channel></channel></rss>`)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	if _, err := c.Search(context.Background(), "frieren", 10); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if cap.q != "frieren" {
		t.Errorf("q: want frieren, got %q", cap.q)
	}
	if cap.c != "1_2" {
		t.Errorf("c: want 1_2, got %q", cap.c)
	}
	if cap.f != "0" {
		t.Errorf("f: want 0, got %q", cap.f)
	}
	if cap.page != "rss" {
		t.Errorf("page: want rss, got %q", cap.page)
	}
}

func TestSearch_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		mustWrite(t, w, "down for maintenance")
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	_, err := c.Search(context.Background(), "frieren", 10)
	if err == nil {
		t.Fatal("expected error on 503, got nil")
	}
	if appErr, ok := errors.IsAppError(err); !ok || appErr.Code != errors.CodeExternalAPI {
		t.Errorf("expected ExternalAPI wrapped error, got %v", err)
	}
}

func TestSearch_LimitClamp(t *testing.T) {
	// Build a 10-item RSS body.
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0"?><rss xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:nyaa="https://nyaa.si/xmlns/nyaa"><channel>`)
	for i := 0; i < 10; i++ {
		sb.WriteString(`<item><title>X - 01 (1080p).mkv</title><link>x</link><pubDate>Sat, 18 May 2024 10:00:00 +0000</pubDate><dc:creator>g</dc:creator><nyaa:infoHash>`)
		sb.WriteString(strings.Repeat("a", 39))
		sb.WriteByte(byte('0' + i))
		sb.WriteString(`</nyaa:infoHash><nyaa:size>100 MiB</nyaa:size></item>`)
	}
	sb.WriteString(`</channel></rss>`)
	body := sb.String()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		mustWrite(t, w, body)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	releases, err := c.Search(context.Background(), "x", 3)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(releases) != 3 {
		t.Fatalf("limit=3 should clamp to 3, got %d", len(releases))
	}
}

func TestParseSize(t *testing.T) {
	// Constant arithmetic with a non-integer leading float is not
	// implicitly truncatable to int64 — break the calculation into a
	// runtime expression so the compiler doesn't reject the truncation.
	gib := func() float64 { return 1024 * 1024 * 1024 }()
	want14GiB := int64(1.4 * gib)
	cases := []struct {
		in   string
		want int64
	}{
		{"1.4 GiB", want14GiB},
		{"700 MiB", 700 * 1024 * 1024},
		{"512 MB", 512 * 1000 * 1000},
		{"2 TiB", 2 * 1024 * 1024 * 1024 * 1024},
		{"1024", 1024},   // bare number → bytes
		{"1024 B", 1024}, // explicit bytes
		{"", 0},
		{"bogus", 0},
		{"1.5 ZB", 0}, // unknown unit → 0
		{"  ", 0},
	}
	for _, tc := range cases {
		got := parseSize(tc.in)
		if got != tc.want {
			t.Errorf("parseSize(%q): want %d, got %d", tc.in, tc.want, got)
		}
	}
}
