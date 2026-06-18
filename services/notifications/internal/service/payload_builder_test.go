package service

import "testing"

func TestBuildWatchURL_ProviderTeamEpisode(t *testing.T) {
	got := BuildWatchURL("abc-123", "kodik", 12, "AniLibria")
	want := "/anime/abc-123/watch?provider=kodik&team=AniLibria&episode=12"
	if got != want {
		t.Fatalf("BuildWatchURL = %q, want %q", got, want)
	}
}

func TestBuildWatchURL_EncodesTeamTitleWithSpace(t *testing.T) {
	got := BuildWatchURL("abc-123", "kodik", 3, "Studio Band")
	want := "/anime/abc-123/watch?provider=kodik&team=Studio+Band&episode=3"
	if got != want {
		t.Fatalf("BuildWatchURL = %q, want %q", got, want)
	}
}

func TestBuildWatchURL_EmptyTeam(t *testing.T) {
	got := BuildWatchURL("abc-123", "animelib", 5, "")
	want := "/anime/abc-123/watch?provider=animelib&team=&episode=5"
	if got != want {
		t.Fatalf("BuildWatchURL = %q, want %q", got, want)
	}
}
