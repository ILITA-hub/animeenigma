package kage

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

const assBody = "[Script Info]\nTitle: t\n\n[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,привет\n"
const srtBody = "1\n00:00:01,000 --> 00:00:02,000\nпривет\n"

func makeZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func TestExtractEpisode_ZipPicksByFilenameNumber(t *testing.T) {
	payload := makeZip(t, map[string][]byte{
		"[SubsPlease] Sousou no Frieren - 01 (720p) [820FC793].ass": []byte(assBody),
		"[SubsPlease] Sousou no Frieren - 02 (720p) [EC4EC720].ass": []byte(strings.Replace(assBody, "привет", "ep two", 1)),
		"readme.txt": []byte("not a subtitle"),
	})
	body, format, err := ExtractEpisode(payload, "frieren.zip", 2)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if format != "ass" || !strings.Contains(string(body), "ep two") {
		t.Fatalf("format=%q body=%q", format, string(body))
	}
}

func TestExtractEpisode_ZipMissingEpisodeErrors(t *testing.T) {
	payload := makeZip(t, map[string][]byte{
		"show - 01.ass": []byte(assBody),
		"show - 02.ass": []byte(assBody),
	})
	if _, _, err := ExtractEpisode(payload, "show.zip", 7); err == nil {
		t.Fatal("expected error for missing episode")
	}
}

func TestExtractEpisode_SingleEntryWinsRegardlessOfNumber(t *testing.T) {
	payload := makeZip(t, map[string][]byte{"movie_final.ass": []byte(assBody)})
	body, format, err := ExtractEpisode(payload, "movie.zip", 1)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if format != "ass" || !strings.Contains(string(body), "Dialogue:") {
		t.Fatalf("format=%q", format)
	}
	_ = body
}

func TestExtractEpisode_BareFilePassthrough(t *testing.T) {
	body, format, err := ExtractEpisode([]byte(srtBody), "show_ep3.srt", 3)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if format != "srt" || !strings.Contains(string(body), "-->") {
		t.Fatalf("format=%q body=%q", format, string(body))
	}
}

func TestExtractEpisode_CP1251BodyIsConvertedToUTF8(t *testing.T) {
	enc, err := charmap.Windows1251.NewEncoder().Bytes([]byte(assBody))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	payload := makeZip(t, map[string][]byte{"show - 01.ass": enc})
	body, _, err := ExtractEpisode(payload, "show.zip", 1)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(string(body), "привет") {
		t.Fatalf("cp1251 body not converted: %q", string(body))
	}
}

func TestExtractEpisode_BOMStripped(t *testing.T) {
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, []byte(assBody)...)
	payload := makeZip(t, map[string][]byte{"show - 01.ass": withBOM})
	body, _, err := ExtractEpisode(payload, "show.zip", 1)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if bytes.HasPrefix(body, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("BOM survived")
	}
}

func TestExtractEpisode_RejectsNonSubtitleBody(t *testing.T) {
	if _, _, err := ExtractEpisode([]byte("<html>login required</html>"), "error.ass", 1); err == nil {
		t.Fatal("HTML body must not pass as a subtitle")
	}
}

func TestEpisodeNumberFromName(t *testing.T) {
	cases := []struct {
		name string
		want int
	}{
		{"[SubsPlease] Sousou no Frieren - 01 (720p) [820FC793].ass", 1},
		{"Mobile Suit Gundam 00 - 05.ass", 5},
		{"show - 12v2.ass", 12},
		{"Show_S01E07_x264.srt", 7}, // resolution/codec noise stripped, last number wins
		{"movie_final.ass", 0},
	}
	for _, c := range cases {
		if got := episodeNumberFromName(c.name); got != c.want {
			t.Errorf("episodeNumberFromName(%q) = %d, want %d", c.name, got, c.want)
		}
	}
}
