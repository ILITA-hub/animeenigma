package animetosho

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

func xzCompress(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz writer: %v", err)
	}
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatalf("xz write: %v", err)
	}
	_ = w.Close()
	return buf.Bytes()
}

// Live-captured shapes (2026-07-18, TenSura S4 / AniDB 18884): the search
// list is a bare JSON array; the torrent detail nests attachments per file.
const toshoSearchJSON = `[
  {"id": 764596, "title": "[Erai-raws] Tensei Shitara Slime Datta Ken 4th Season - 05 [1080p CR WEBRip][MultiSub]", "timestamp": 1778256418, "num_files": 1},
  {"id": 764594, "title": "[ASW] Tensei Shitara Slime Datta Ken S4 - 05 [1080p HEVC]", "timestamp": 1778255938, "num_files": 1},
  {"id": 764000, "title": "[Erai-raws] Tensei Shitara Slime Datta Ken 4th Season - 01~04 [1080p][MultiSub]", "timestamp": 1777000000, "num_files": 4}
]`

const toshoTorrentJSON = `{
  "title": "[Erai-raws] Tensei Shitara Slime Datta Ken 4th Season - 05 [1080p CR WEBRip][MultiSub]",
  "files": [
    {
      "filename": "[Erai-raws] Tensei Shitara Slime Datta Ken 4th Season - 05 [1080p CR WEBRip][MultiSub].mkv",
      "attachments": [
        {"id": 77333, "type": "font", "info": {"name": "arialbd_3.ttf"}},
        {"id": 2905415, "type": "subtitle", "info": {"codec": "ASS", "lang": "eng", "name": "CR"}},
        {"id": 2905425, "type": "subtitle", "info": {"codec": "ASS", "lang": "rus", "name": "CR"}},
        {"id": 2905754, "type": "subtitle", "info": {"codec": "ASS", "lang": "", "name": ""}}
      ]
    }
  ]
}`

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/json" && q.Get("aid") == "18884":
			_, _ = w.Write([]byte(toshoSearchJSON))
		case r.URL.Path == "/json" && q.Get("show") == "torrent" && q.Get("id") == "764596":
			_, _ = w.Write([]byte(toshoTorrentJSON))
		case strings.HasPrefix(r.URL.Path, "/storage/attach/002c5551/"):
			_, _ = w.Write(xzCompress(t, "[Script Info]\nDialogue: rus line"))
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTestClient(srvURL string) *Client {
	return NewClient(Config{FeedBaseURL: srvURL, StorageBaseURL: srvURL, Enabled: true})
}

func TestSearchByAniDB(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	rels, err := newTestClient(srv.URL).SearchByAniDB(context.Background(), 18884)
	if err != nil {
		t.Fatalf("SearchByAniDB: %v", err)
	}
	if len(rels) != 3 || rels[0].ID != 764596 {
		t.Fatalf("unexpected releases: %+v", rels)
	}
}

func TestTorrentFilesParsesAttachments(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	files, err := newTestClient(srv.URL).TorrentFiles(context.Background(), 764596)
	if err != nil {
		t.Fatalf("TorrentFiles: %v", err)
	}
	if len(files) != 1 || len(files[0].Attachments) != 4 {
		t.Fatalf("unexpected files: %+v", files)
	}
	rus := files[0].Attachments[2]
	if rus.ID != 2905425 || rus.Type != "subtitle" || rus.Info.Lang != "rus" || rus.Info.Codec != "ASS" {
		t.Fatalf("unexpected rus attachment: %+v", rus)
	}
}

func TestDownloadAttachmentDecompressesXZ(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	// 2905425 = 0x2c5551 → the storage path embeds it as 8-digit hex.
	body, err := newTestClient(srv.URL).DownloadAttachment(context.Background(), 2905425)
	if err != nil {
		t.Fatalf("DownloadAttachment: %v", err)
	}
	if !strings.Contains(string(body), "Dialogue: rus line") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestEpisodeFromTitle(t *testing.T) {
	cases := []struct {
		title string
		ep    int
		ok    bool
	}{
		{"[Erai-raws] Tensei Shitara Slime Datta Ken 4th Season - 05 [1080p CR WEBRip]", 5, true},
		{"[Erai-raws] Title - 05v2 [720p]", 5, true},
		{"[DKB] Tensei shitara Slime Datta Ken - S04E05 [1080p][HEVC]", 5, true},
		{"[Erai-raws] Title - 1088 [1080p]", 1088, true},
		{"Title - 05.mkv", 5, true},
		{"[Erai-raws] Title - 01~04 [1080p]", 0, false}, // tilde batch
		{"[Group] Title - 01-12 [1080p]", 0, false},     // dash batch
		{"[Erai-raws] Title - 05.5 [1080p]", 0, false},  // half-episode special
		{"[Erai-raws] Movie Title [1080p][MultiSub]", 0, false},
		{"91 Days - 03 [720p]", 3, true},
	}
	for _, c := range cases {
		ep, ok := EpisodeFromTitle(c.title)
		if ep != c.ep || ok != c.ok {
			t.Errorf("EpisodeFromTitle(%q) = (%d, %v), want (%d, %v)", c.title, ep, ok, c.ep, c.ok)
		}
	}
}

func TestReleaseGroup(t *testing.T) {
	if g := ReleaseGroup("[Erai-raws] Title - 05"); g != "Erai-raws" {
		t.Fatalf("group = %q", g)
	}
	if g := ReleaseGroup("Title without group"); g != "" {
		t.Fatalf("group = %q", g)
	}
}
