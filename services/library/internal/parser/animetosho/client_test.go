package animetosho

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

// malFixture is a two-entry response shaped like AnimeTosho's
// `?show=mal&id=` payload. Both entries carry an explicit info_hash so
// the parser doesn't need to fall back to ParseMagnetUri.
const malFixture = `[
  {
    "title": "[SubsPlease] Frieren - 01 (1080p) [ABCDEF12].mkv",
    "link": "magnet:?xt=urn:btih:aabbccddeeff00112233445566778899aabbccdd&dn=Frieren-01",
    "info_hash": "aabbccddeeff00112233445566778899aabbccdd",
    "total_size": 1500000000,
    "timestamp": 1700000000
  },
  {
    "title": "[Erai-raws] Frieren - 01 [720p][HEVC].mkv",
    "link": "magnet:?xt=urn:btih:1122334455667788990011223344556677889900&dn=Frieren-01-720",
    "info_hash": "1122334455667788990011223344556677889900",
    "total_size": 700000000,
    "timestamp": 1700000100
  }
]`

func TestSearch_MALPath(t *testing.T) {
	var seenRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		mustWrite(t, w, malFixture)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	releases, err := c.Search(context.Background(), SearchParams{MALID: 52991, Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if !strings.Contains(seenRawQuery, "show=mal") || !strings.Contains(seenRawQuery, "id=52991") {
		t.Fatalf("expected MAL path query, got %q", seenRawQuery)
	}
	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}

	r0 := releases[0]
	if r0.Source != "animetosho" {
		t.Errorf("Source: want animetosho, got %q", r0.Source)
	}
	if r0.MALID != 52991 {
		t.Errorf("MALID: want 52991, got %d", r0.MALID)
	}
	if r0.Quality != "1080p" {
		t.Errorf("Quality: want 1080p, got %q", r0.Quality)
	}
	if r0.Uploader != "SubsPlease" {
		t.Errorf("Uploader: want SubsPlease, got %q", r0.Uploader)
	}
	if r0.InfoHash != "aabbccddeeff00112233445566778899aabbccdd" {
		t.Errorf("InfoHash: want lowercase hex match, got %q", r0.InfoHash)
	}
	if r0.SizeBytes != 1500000000 {
		t.Errorf("SizeBytes: want 1.5e9, got %d", r0.SizeBytes)
	}
	if r0.FoundAt.IsZero() {
		t.Error("FoundAt: should be set from timestamp")
	}

	if releases[1].Quality != "720p" {
		t.Errorf("releases[1].Quality: want 720p, got %q", releases[1].Quality)
	}
	if releases[1].Uploader != "Erai-raws" {
		t.Errorf("releases[1].Uploader: want Erai-raws, got %q", releases[1].Uploader)
	}
}

func TestSearch_QueryPath(t *testing.T) {
	var seenQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenQ = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		mustWrite(t, w, `[]`)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	releases, err := c.Search(context.Background(), SearchParams{Query: "frieren"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if seenQ != "frieren" {
		t.Errorf("query q: want frieren, got %q", seenQ)
	}
	if len(releases) != 0 {
		t.Errorf("empty payload should produce 0 releases, got %d", len(releases))
	}
}

func TestSearch_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		mustWrite(t, w, `boom`)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	_, err := c.Search(context.Background(), SearchParams{Query: "x"})
	if err == nil {
		t.Fatal("expected error on 500 response, got nil")
	}
	if appErr, ok := errors.IsAppError(err); !ok || appErr.Code != errors.CodeExternalAPI {
		t.Errorf("expected ExternalAPI wrapped error, got %v", err)
	}
}

func TestSearch_LimitClamp(t *testing.T) {
	// Build a 10-entry response.
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 10; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		// Distinct lowercase-hex info hashes so dedupe (in later tests) doesn't trip.
		sb.WriteString(`{"title":"[Grp] X - 01 (1080p).mkv","link":"magnet:?xt=urn:btih:`)
		// 40 chars total: pad with the loop index
		sb.WriteString(strings.Repeat("a", 39))
		sb.WriteByte(byte('0' + i))
		sb.WriteString(`","info_hash":"`)
		sb.WriteString(strings.Repeat("a", 39))
		sb.WriteByte(byte('0' + i))
		sb.WriteString(`","total_size":1,"timestamp":1700000000}`)
	}
	sb.WriteString("]")
	body := sb.String()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mustWrite(t, w, body)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	releases, err := c.Search(context.Background(), SearchParams{Query: "x", Limit: 3})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(releases) != 3 {
		t.Fatalf("limit=3 should clamp to 3, got %d", len(releases))
	}

	releases, err = c.Search(context.Background(), SearchParams{Query: "x", Limit: 0})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	// 10 < default 50, all returned.
	if len(releases) != 10 {
		t.Fatalf("limit=0 (default 50) with 10 entries should return 10, got %d", len(releases))
	}
}

func TestSearch_InfoHashFromMagnet(t *testing.T) {
	// Magnet xt token defines the hash. ParseMagnetUri accepts both
	// uppercase and lowercase btih values; we normalize to lowercase.
	const hash = "0123456789abcdef0123456789abcdef01234567"
	body := `[
      {
        "title": "[NoHashGroup] Foo - 01 (1080p).mkv",
        "link": "magnet:?xt=urn:btih:` + hash + `&dn=Foo",
        "info_hash": "",
        "total_size": 1,
        "timestamp": 1700000000
      }
    ]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mustWrite(t, w, body)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	releases, err := c.Search(context.Background(), SearchParams{Query: "foo"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	if releases[0].InfoHash != hash {
		t.Errorf("InfoHash from magnet: want %q, got %q", hash, releases[0].InfoHash)
	}
}
