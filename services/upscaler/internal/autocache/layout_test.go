package autocache

import (
	"strings"
	"testing"
)

func TestUpscaledPrefix_Shape(t *testing.T) {
	cases := []struct {
		shikimoriID string
		episode     int
		scaleHeight int
		want        string
	}{
		{"57466", 1, 1080, "aeProvider/57466/UPSCALED-1080p/1/"},
		{"12345", 5, 2160, "aeProvider/12345/UPSCALED-2160p/5/"},
		{"99999", 12, 720, "aeProvider/99999/UPSCALED-720p/12/"},
	}
	for _, c := range cases {
		got := UpscaledPrefix(c.shikimoriID, c.episode, c.scaleHeight)
		if got != c.want {
			t.Errorf("UpscaledPrefix(%q, %d, %d) = %q, want %q",
				c.shikimoriID, c.episode, c.scaleHeight, got, c.want)
		}
	}
}

func TestUpscaledPrefix_TrailingSlash(t *testing.T) {
	got := UpscaledPrefix("1234", 3, 1080)
	if !strings.HasSuffix(got, "/") {
		t.Errorf("UpscaledPrefix must end with '/'; got %q", got)
	}
}

func TestUpscaledPrefix_ContainsUPSCALED(t *testing.T) {
	got := UpscaledPrefix("1", 1, 1080)
	if !strings.Contains(got, "UPSCALED") {
		t.Errorf("UpscaledPrefix must contain UPSCALED; got %q", got)
	}
	if !strings.Contains(got, "1080p") {
		t.Errorf("UpscaledPrefix must contain scale suffix; got %q", got)
	}
}
