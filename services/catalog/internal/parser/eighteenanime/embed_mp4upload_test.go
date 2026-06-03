package eighteenanime

import (
	"os"
	"strings"
	"testing"
)

func TestExtractMP4Upload(t *testing.T) {
	data, err := os.ReadFile("testdata/embed_mp4upload.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	html := string(data)

	src, err := extractMP4Upload(html)
	if err != nil {
		t.Fatalf("extractMP4Upload returned error: %v", err)
	}
	if src == nil {
		t.Fatal("extractMP4Upload returned nil source")
	}
	if !strings.Contains(src.URL, "mp4upload.com") {
		t.Errorf("URL does not contain mp4upload.com: %q", src.URL)
	}
	if !strings.Contains(src.URL, ".mp4") {
		t.Errorf("URL does not contain .mp4: %q", src.URL)
	}
	if src.Referer != "https://www.mp4upload.com/" {
		t.Errorf("Referer = %q, want %q", src.Referer, "https://www.mp4upload.com/")
	}
	if src.IsHLS {
		t.Error("IsHLS should be false for mp4upload progressive MP4")
	}

	t.Logf("extracted URL: %s", src.URL)
}

func TestExtractMP4Upload_NotFound(t *testing.T) {
	_, err := extractMP4Upload("<html>no video here</html>")
	if err == nil {
		t.Error("expected error when src not found, got nil")
	}
}
