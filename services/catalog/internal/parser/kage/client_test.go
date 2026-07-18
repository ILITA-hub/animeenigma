package kage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

// cp1251 encodes a UTF-8 fixture the way the real site serves pages.
func cp1251(t *testing.T, s string) []byte {
	t.Helper()
	out, err := charmap.Windows1251.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("cp1251 encode: %v", err)
	}
	return out
}

// seriesPageFixture mirrors the live base.php?id=7120 structure (2026-07-18):
// nav table (class row1, no author link), title block, then release forms
// each followed by an author table.
const seriesPageFixture = `
<table class="row1">
  <tr><td>[<a href="./">Главная</a>]</td></tr>
</table>
<table class="title"><tr><td class="title"><b>Sousou no Frieren</b></td></tr></table>
<table class="title">
<form method="post" action="base.php">
<input type="hidden" name="srt" value="13364">
<tr>
<td width="100"><input type="image" alt="Скрипт ASS" src="gif/dl.gif"></td>
<td><b>ТВ 1-28</b></td>
<td width="100" align="center"><a href="base.php?cntr=13364"><font color=#F4F4F4>ASS</font></a></td>
<td width="100" align="center">13.12.25</td>
</tr>
</form>
</table>
<table class="row1">
  <tr>
    <td align="center" valign="middle"><a href="base.php?au=2518"><b>Aero</b></a>
<br>[<a href=https://vk.com/yakusub_studio target=web>YakuSub Studio</a>]</td>
  </tr>
</table>
<table class="title">
<form method="post" action="base.php">
<input type="hidden" name="srt" value="14001">
<tr>
<td><b>Фильм</b></td>
<td width="100" align="center"><a href="base.php?cntr=14001"><font color=#F4F4F4>SRT</font></a></td>
<td width="100" align="center">19.03.26</td>
</tr>
</form>
</table>
<table class="row1">
  <tr><td><a href="base.php?au=99"><b>Соло</b></a></td></tr>
</table>
`

func kageTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search.php" && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "query=") {
				http.Error(w, "no query", http.StatusBadRequest)
				return
			}
			w.Write(cp1251(t, `<a href="base.php?id=7120">Sousou no Frieren </a><br>`+
				`<a href="base.php?id=7377">Sousou no Frieren (2026) </a>`))
		case r.URL.Path == "/base.php" && r.Method == http.MethodGet && r.URL.Query().Get("id") == "7120":
			w.Write(cp1251(t, seriesPageFixture))
		case r.URL.Path == "/base.php" && r.Method == http.MethodPost:
			r.ParseForm()
			if r.Form.Get("srt") != "13364" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Disposition", `attachment; filename="frieren_(13364).zip"`)
			w.Write([]byte("PK\x03\x04 not a real zip — bytes only"))
		case r.URL.Path == "/":
			w.Write([]byte("<title>Kage Project</title>"))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestSearchSeries_ParsesCP1251Results(t *testing.T) {
	srv := kageTestServer(t)
	defer srv.Close()
	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})

	refs, err := c.SearchSeries(context.Background(), "Sousou no Frieren")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2: %+v", len(refs), refs)
	}
	if refs[0].ID != 7120 || refs[0].Title != "Sousou no Frieren" {
		t.Fatalf("ref[0] = %+v", refs[0])
	}
	if refs[1].ID != 7377 || refs[1].Title != "Sousou no Frieren (2026)" {
		t.Fatalf("ref[1] = %+v", refs[1])
	}
}

func TestGetReleases_ParsesFormsAndAuthors(t *testing.T) {
	srv := kageTestServer(t)
	defer srv.Close()
	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})

	rels, err := c.GetReleases(context.Background(), 7120)
	if err != nil {
		t.Fatalf("releases: %v", err)
	}
	if len(rels) != 2 {
		t.Fatalf("got %d releases, want 2: %+v", len(rels), rels)
	}
	tv := rels[0]
	if tv.SrtID != 13364 || tv.Label != "ТВ 1-28" || tv.Format != "ass" || tv.Date != "13.12.25" {
		t.Fatalf("tv release = %+v", tv)
	}
	if tv.Author != "Aero" || tv.Team != "YakuSub Studio" {
		t.Fatalf("tv attribution = %q / %q", tv.Author, tv.Team)
	}
	if tv.EpFrom != 1 || tv.EpTo != 28 {
		t.Fatalf("tv range = %d-%d", tv.EpFrom, tv.EpTo)
	}
	movie := rels[1]
	if movie.SrtID != 14001 || movie.Format != "srt" || movie.Author != "Соло" || movie.Team != "" {
		t.Fatalf("movie release = %+v", movie)
	}
	if movie.EpFrom != 0 || movie.EpTo != 0 || !movie.ContainsEpisode(1) || !movie.ContainsEpisode(42) {
		t.Fatalf("movie range should be unbounded: %+v", movie)
	}
	if tv.ContainsEpisode(29) || !tv.ContainsEpisode(28) {
		t.Fatalf("range containment wrong for %+v", tv)
	}
}

func TestDownloadArchive_ReturnsBytesAndFilename(t *testing.T) {
	srv := kageTestServer(t)
	defer srv.Close()
	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})

	body, name, err := c.DownloadArchive(context.Background(), 13364)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if name != "frieren_(13364).zip" {
		t.Fatalf("filename = %q", name)
	}
	if !strings.HasPrefix(string(body), "PK\x03\x04") {
		t.Fatalf("body = %q", string(body[:8]))
	}
}

func TestPing_ChecksHomepage(t *testing.T) {
	srv := kageTestServer(t)
	defer srv.Close()
	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestParseEpisodeRange(t *testing.T) {
	cases := []struct {
		label    string
		from, to int
	}{
		{"ТВ 1-28", 1, 28},
		{"Спецвыпуск 1-22 (+4,5)", 1, 22},
		{"ТВ 5", 5, 5},
		{"ОВА-2", 2, 2},
		{"Фильм", 0, 0},
		{"", 0, 0},
	}
	for _, c := range cases {
		from, to := parseEpisodeRange(c.label)
		if from != c.from || to != c.to {
			t.Errorf("parseEpisodeRange(%q) = %d-%d, want %d-%d", c.label, from, to, c.from, c.to)
		}
	}
}

func TestIsConfigured(t *testing.T) {
	if NewClient(Config{Enabled: false}).IsConfigured() {
		t.Fatal("disabled client should not be configured")
	}
	if !NewClient(Config{Enabled: true}).IsConfigured() {
		t.Fatal("enabled client should be configured")
	}
	var nilClient *Client
	if nilClient.IsConfigured() {
		t.Fatal("nil client should not be configured")
	}
}
