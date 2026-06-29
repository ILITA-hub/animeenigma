package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animejoy"
)

// mapAnimejoyTeams reduces per-episode leg presence to per-team flags. These
// tests pin the PURE mapping (no network): a team's HasSibnet/HasAllVideo is the
// OR over its episodes' Sibnet/AllVideo embed URLs.
func TestMapAnimejoyTeams_SibnetOnly(t *testing.T) {
	in := []animejoy.Team{{
		ID:   "0",
		Name: "AnimeJoy",
		Episodes: []animejoy.Episode{
			{Num: 1, Sibnet: "https://iv.sibnet.ru/shell.php?videoid=1"},
			{Num: 2, Sibnet: "https://iv.sibnet.ru/shell.php?videoid=2"},
		},
	}}
	got := mapAnimejoyTeams(in)
	if len(got) != 1 {
		t.Fatalf("want 1 team, got %d", len(got))
	}
	want := domain.AnimejoyTeam{ID: "0", Name: "AnimeJoy", HasSibnet: true, HasAllVideo: false}
	if got[0] != want {
		t.Fatalf("mapped team = %+v, want %+v", got[0], want)
	}
}

func TestMapAnimejoyTeams_LegsAcrossEpisodes(t *testing.T) {
	// Sibnet appears on ep1 only, AllVideo on ep2 only — both flags must be true
	// (OR over episodes), not per-episode.
	in := []animejoy.Team{{
		ID: "1", Name: "Team B",
		Episodes: []animejoy.Episode{
			{Num: 1, Sibnet: "https://iv.sibnet.ru/x"},
			{Num: 2, AllVideo: "https://fsst.online/embed/y"},
		},
	}}
	got := mapAnimejoyTeams(in)
	if len(got) != 1 || !got[0].HasSibnet || !got[0].HasAllVideo {
		t.Fatalf("expected both legs true, got %+v", got)
	}
}

func TestMapAnimejoyTeams_NoLegs(t *testing.T) {
	in := []animejoy.Team{{ID: "2", Episodes: []animejoy.Episode{{Num: 1}}}}
	got := mapAnimejoyTeams(in)
	if len(got) != 1 || got[0].HasSibnet || got[0].HasAllVideo {
		t.Fatalf("expected no legs, got %+v", got)
	}
}

func TestMapAnimejoyTeams_PreservesOrderAndEmpty(t *testing.T) {
	if got := mapAnimejoyTeams(nil); len(got) != 0 {
		t.Fatalf("nil teams → empty, got %+v", got)
	}
	in := []animejoy.Team{
		{ID: "0", Episodes: []animejoy.Episode{{Sibnet: "a"}}},
		{ID: "1", Episodes: []animejoy.Episode{{AllVideo: "b"}}},
	}
	got := mapAnimejoyTeams(in)
	if len(got) != 2 || got[0].ID != "0" || got[1].ID != "1" {
		t.Fatalf("order not preserved: %+v", got)
	}
}
