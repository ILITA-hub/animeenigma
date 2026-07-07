package main

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

func TestLangToTrack(t *testing.T) {
	cases := map[string]domain.EpisodeTrack{
		"":    domain.EpisodeTrackRaw,
		"jpn": domain.EpisodeTrackRaw,
		"jp":  domain.EpisodeTrackRaw,
		"ja":  domain.EpisodeTrackRaw,
		"eng": domain.EpisodeTrackDub,
		"en":  domain.EpisodeTrackDub,
		"rus": domain.EpisodeTrackDub,
		"ru":  domain.EpisodeTrackDub,
	}
	for in, want := range cases {
		if got := langToTrack(in); got != want {
			t.Errorf("langToTrack(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeLang(t *testing.T) {
	cases := map[string]string{
		"":      "",
		"en":    "eng",
		"EN":    "eng",
		"eng":   "eng",
		"ENG":   "eng",
		"ru":    "rus",
		"RU":    "rus",
		"rus":   "rus",
		"RUS":   "rus",
		"ja":    "jpn",
		"JA":    "jpn",
		"jp":    "jpn",
		"JP":    "jpn",
		"jpn":   "jpn",
		"JPN":   "jpn",
		"   en": "eng",
		"en   ": "eng",
		"other": "other",
	}
	for in, want := range cases {
		if got := normalizeLang(in); got != want {
			t.Errorf("normalizeLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatHeight(t *testing.T) {
	cases := map[int]string{
		0:    "",
		480:  "480p",
		720:  "720p",
		1080: "1080p",
		2160: "2160p",
	}
	for h, want := range cases {
		if got := formatHeight(h); got != want {
			t.Errorf("formatHeight(%d) = %q, want %q", h, got, want)
		}
	}
}
