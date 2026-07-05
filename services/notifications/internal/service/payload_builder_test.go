package service

import (
	"strings"
	"testing"
)

// BuildWatchURL is now a bare anime-page link with NO query params — the old
// ?provider&team&episode deep-link baked in a stale episode number that the
// frontend treated as a hard override, landing users on the wrong episode.
func TestBuildWatchURL_BareAnimeLink(t *testing.T) {
	got := BuildWatchURL("abc-123")
	want := "/anime/abc-123"
	if got != want {
		t.Fatalf("BuildWatchURL = %q, want %q", got, want)
	}
}

func TestBuildWatchURL_NoQueryParams(t *testing.T) {
	got := BuildWatchURL("uuid-with-dashes-9f")
	if want := "/anime/uuid-with-dashes-9f"; got != want {
		t.Fatalf("BuildWatchURL = %q, want %q", got, want)
	}
	for _, c := range []string{"?", "&", "episode=", "provider=", "team="} {
		if strings.Contains(got, c) {
			t.Fatalf("BuildWatchURL = %q, must not contain %q", got, c)
		}
	}
}
