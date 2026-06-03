package eighteenanime

import (
	"os"
	"strings"
	"testing"
)

func TestExtractTurbovid(t *testing.T) {
	data, err := os.ReadFile("testdata/embed_turbovid.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	html := string(data)

	src, err := extractTurbovid(html)
	if err != nil {
		t.Fatalf("extractTurbovid returned error: %v", err)
	}
	if src == nil {
		t.Fatal("extractTurbovid returned nil source")
	}
	if !strings.Contains(src.URL, ".m3u8") {
		t.Errorf("expected URL to contain .m3u8, got %q", src.URL)
	}
	if !src.IsHLS {
		t.Errorf("expected IsHLS to be true")
	}
	if src.Referer != "" {
		t.Errorf("expected empty Referer, got %q", src.Referer)
	}
}

func TestExtractTurbovid_NotFound(t *testing.T) {
	_, err := extractTurbovid("garbage html with no m3u8 url here")
	if err == nil {
		t.Fatal("expected error for input with no m3u8 URL, got nil")
	}

	_, err2 := extractTurbovid("")
	if err2 == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}
