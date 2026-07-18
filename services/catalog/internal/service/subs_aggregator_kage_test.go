package service

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kage"
	"golang.org/x/text/encoding/charmap"
)

func kageCP1251(t *testing.T, s string) []byte {
	t.Helper()
	out, err := charmap.Windows1251.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("cp1251 encode: %v", err)
	}
	return out
}

func kageZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip: %v", err)
		}
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	return buf.Bytes()
}

const kageASS = "[Script Info]\n\n[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,ep %s\n"

// kageAggTestServer serves search + series page + archive download for
// "Sousou no Frieren" (series 7120, srt 13364, TV 1-28, author Aero).
func kageAggTestServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search.php":
			w.Write(kageCP1251(t, `<a href="base.php?id=7120">Sousou no Frieren </a>`+
				`<a href="base.php?id=7377">Sousou no Frieren (2026) </a>`))
		case r.URL.Path == "/base.php" && r.Method == http.MethodGet:
			w.Write(kageCP1251(t, `
<table class="title">
<form method="post" action="base.php">
<input type="hidden" name="srt" value="13364">
<tr><td><b>ТВ 1-28</b></td>
<td><a href="base.php?cntr=13364"><font color=#F4F4F4>ASS</font></a></td>
<td>13.12.25</td></tr>
</form>
</table>
<table class="row1"><tr><td><a href="base.php?au=2518"><b>Aero</b></a><br>[<a href=https://vk.com/x target=web>YakuSub Studio</a>]</td></tr></table>`))
		case r.URL.Path == "/base.php" && r.Method == http.MethodPost:
			downloads++
			w.Header().Set("Content-Disposition", `attachment; filename="frieren.zip"`)
			w.Write(kageZip(t, map[string]string{
				"frieren - 01.ass": strings.Replace(kageASS, "%s", "one", 1),
				"frieren - 12.ass": strings.Replace(kageASS, "%s", "twelve", 1),
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, &downloads
}

func TestFetchKage_ReturnsRussianSubTracks(t *testing.T) {
	srv, _ := kageAggTestServer(t)
	defer srv.Close()

	kc := kage.NewClient(kage.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, kc, nil, nil, resolveTestRedis(t), nil, logger.Default())

	anime := &domain.Anime{ID: "uuid-1", Name: "Sousou no Frieren", Kind: "tv"}
	tracks, err := agg.fetchKage(context.Background(), anime, 12)
	if err != nil {
		t.Fatalf("fetchKage: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1: %+v", len(tracks), tracks)
	}
	tr := tracks[0]
	if tr.Lang != "ru" || tr.Provider != "kage" || tr.Format != "ass" {
		t.Fatalf("bad track: %+v", tr)
	}
	if tr.URL != "/api/anime/uuid-1/subtitles/kage/file/13364?episode=12" {
		t.Fatalf("url = %q", tr.URL)
	}
	if tr.Label != "Aero · YakuSub Studio" || tr.Release != "ТВ 1-28" {
		t.Fatalf("label/release = %q / %q", tr.Label, tr.Release)
	}
}

func TestFetchKage_EpisodeOutsideRangeYieldsNoTracks(t *testing.T) {
	srv, _ := kageAggTestServer(t)
	defer srv.Close()

	kc := kage.NewClient(kage.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, kc, nil, nil, resolveTestRedis(t), nil, logger.Default())

	anime := &domain.Anime{ID: "uuid-1", Name: "Sousou no Frieren", Kind: "tv"}
	tracks, err := agg.fetchKage(context.Background(), anime, 99)
	if err != nil {
		t.Fatalf("fetchKage: %v", err)
	}
	if len(tracks) != 0 {
		t.Fatalf("tracks = %+v, want empty (episode 99 outside ТВ 1-28)", tracks)
	}
}

func TestFetchKage_UnknownTitleReturnsEmptyNoError(t *testing.T) {
	srv, _ := kageAggTestServer(t)
	defer srv.Close()

	kc := kage.NewClient(kage.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, kc, nil, nil, resolveTestRedis(t), nil, logger.Default())

	// Search returns Frieren refs; no exact title match → not on Kage.
	anime := &domain.Anime{ID: "uuid-2", Name: "Yani Neko", Kind: "tv"}
	tracks, err := agg.fetchKage(context.Background(), anime, 1)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(tracks) != 0 {
		t.Fatalf("tracks = %+v, want empty", tracks)
	}
}

func TestFetchKage_ExactMatchDoesNotPickSequelPage(t *testing.T) {
	srv, _ := kageAggTestServer(t)
	defer srv.Close()

	kc := kage.NewClient(kage.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, kc, nil, nil, resolveTestRedis(t), nil, logger.Default())

	id, err := agg.resolveKageSeries(context.Background(), &domain.Anime{ID: "uuid-3", Name: "Sousou no Frieren"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if id != 7120 {
		t.Fatalf("series id = %d, want 7120 (exact match, not the (2026) sequel)", id)
	}
}

func TestFetchKage_DisabledIsUnconfigured(t *testing.T) {
	kc := kage.NewClient(kage.Config{Enabled: false})
	agg := NewSubsAggregator(nil, nil, kc, nil, nil, resolveTestRedis(t), nil, logger.Default())
	_, err := agg.fetchKage(context.Background(), &domain.Anime{ID: "x", Name: "y"}, 1)
	if err != errProviderUnconfigured {
		t.Fatalf("err = %v, want errProviderUnconfigured", err)
	}
}

func TestResolveKageFile_ExtractsEpisodeAndCachesArchive(t *testing.T) {
	srv, downloads := kageAggTestServer(t)
	defer srv.Close()

	kc := kage.NewClient(kage.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, kc, nil, nil, resolveTestRedis(t), nil, logger.Default())

	body, format, err := agg.ResolveKageFile(context.Background(), 13364, 12)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if format != "ass" || !strings.Contains(string(body), "ep twelve") {
		t.Fatalf("body=%q format=%q", string(body), format)
	}

	// Second episode from the same pack: served from the archive cache.
	body, _, err = agg.ResolveKageFile(context.Background(), 13364, 1)
	if err != nil {
		t.Fatalf("resolve ep1: %v", err)
	}
	if !strings.Contains(string(body), "ep one") {
		t.Fatalf("ep1 body = %q", string(body))
	}
	if *downloads != 1 {
		t.Fatalf("archive downloads = %d, want 1 (second episode from cache)", *downloads)
	}

	// Same episode again: extracted-file cache, still one download.
	if _, _, err := agg.ResolveKageFile(context.Background(), 13364, 12); err != nil {
		t.Fatalf("resolve again: %v", err)
	}
	if *downloads != 1 {
		t.Fatalf("archive downloads = %d, want 1", *downloads)
	}
}

func TestResolveKageFile_MissingEpisodeErrors(t *testing.T) {
	srv, _ := kageAggTestServer(t)
	defer srv.Close()

	kc := kage.NewClient(kage.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, kc, nil, nil, resolveTestRedis(t), nil, logger.Default())

	if _, _, err := agg.ResolveKageFile(context.Background(), 13364, 7); err == nil {
		t.Fatal("expected error: episode 7 is not in the pack")
	}
}
