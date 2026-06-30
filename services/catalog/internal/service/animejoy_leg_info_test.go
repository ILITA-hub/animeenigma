package service

import (
	"reflect"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animejoy"
)

// buildAnimejoyLegInfo is the PURE per-leg inventory builder (no network): for
// each team it reports whether the team carries the leg (ANY episode has a
// non-empty leg embed) and collects the DISTINCT episode numbers carrying the
// leg across all teams, sorted ascending. These tests pin those rules.

func TestBuildAnimejoyLegInfo_SibnetOnlyTeam(t *testing.T) {
	in := []animejoy.Team{{
		ID:   "0",
		Name: "AnimeJoy",
		Episodes: []animejoy.Episode{
			{Num: 1, Sibnet: "https://iv.sibnet.ru/shell.php?videoid=1"},
			{Num: 2, Sibnet: "https://iv.sibnet.ru/shell.php?videoid=2"},
		},
	}}

	got := buildAnimejoyLegInfo(in, "sibnet")
	wantEpisodes := []int{1, 2}
	if !reflect.DeepEqual(got.Episodes, wantEpisodes) {
		t.Fatalf("sibnet episodes = %v, want %v", got.Episodes, wantEpisodes)
	}
	wantTeams := []domain.AnimejoyTeamMeta{{ID: "0", Name: "AnimeJoy"}}
	if !reflect.DeepEqual(got.Teams, wantTeams) {
		t.Fatalf("sibnet teams = %+v, want %+v", got.Teams, wantTeams)
	}

	// The same fixture has NO AllVideo anywhere → empty (non-nil) slices.
	gotAV := buildAnimejoyLegInfo(in, "allvideo")
	if len(gotAV.Episodes) != 0 {
		t.Fatalf("allvideo episodes = %v, want none", gotAV.Episodes)
	}
	if len(gotAV.Teams) != 0 {
		t.Fatalf("allvideo teams = %+v, want none", gotAV.Teams)
	}
}

func TestBuildAnimejoyLegInfo_LegFilterDropsTeamsWithoutLeg(t *testing.T) {
	// Team 0 is Sibnet-only; team 1 is AllVideo-only. The AllVideo leg must
	// surface ONLY team 1, with only its episode numbers.
	in := []animejoy.Team{
		{
			ID:   "0",
			Name: "Sibnet Team",
			Episodes: []animejoy.Episode{
				{Num: 1, Sibnet: "https://iv.sibnet.ru/x1"},
				{Num: 2, Sibnet: "https://iv.sibnet.ru/x2"},
			},
		},
		{
			ID:   "1",
			Name: "AllVideo Team",
			Episodes: []animejoy.Episode{
				{Num: 3, AllVideo: "https://fsst.online/embed/y3"},
			},
		},
	}

	got := buildAnimejoyLegInfo(in, "allvideo")
	if want := []int{3}; !reflect.DeepEqual(got.Episodes, want) {
		t.Fatalf("allvideo episodes = %v, want %v", got.Episodes, want)
	}
	if want := []domain.AnimejoyTeamMeta{{ID: "1", Name: "AllVideo Team"}}; !reflect.DeepEqual(got.Teams, want) {
		t.Fatalf("allvideo teams = %+v, want %+v", got.Teams, want)
	}
}

func TestBuildAnimejoyLegInfo_DistinctEpisodesAcrossTeams(t *testing.T) {
	// Two teams both carry the leg with overlapping + out-of-order episode
	// numbers. Episodes must be the DISTINCT union, sorted ascending; both
	// teams appear.
	in := []animejoy.Team{
		{
			ID:   "0",
			Name: "Team A",
			Episodes: []animejoy.Episode{
				{Num: 2, Sibnet: "a2"},
				{Num: 1, Sibnet: "a1"},
			},
		},
		{
			ID:   "1",
			Name: "Team B",
			Episodes: []animejoy.Episode{
				{Num: 2, Sibnet: "b2"}, // duplicate of A's ep2
				{Num: 3, Sibnet: "b3"},
			},
		},
	}

	got := buildAnimejoyLegInfo(in, "sibnet")
	if want := []int{1, 2, 3}; !reflect.DeepEqual(got.Episodes, want) {
		t.Fatalf("distinct sorted episodes = %v, want %v", got.Episodes, want)
	}
	if want := []domain.AnimejoyTeamMeta{{ID: "0", Name: "Team A"}, {ID: "1", Name: "Team B"}}; !reflect.DeepEqual(got.Teams, want) {
		t.Fatalf("teams = %+v, want %+v", got.Teams, want)
	}
}

func TestBuildAnimejoyLegInfo_EpisodeWithoutLegDoesNotCount(t *testing.T) {
	// A team with a mix: only episodes carrying the leg contribute; a team with
	// zero matching episodes is dropped even though it has episodes.
	in := []animejoy.Team{{
		ID:   "0",
		Name: "Mixed",
		Episodes: []animejoy.Episode{
			{Num: 1, Sibnet: "s1"},
			{Num: 2, AllVideo: "av2"}, // no Sibnet → ep2 not counted for sibnet
		},
	}}

	got := buildAnimejoyLegInfo(in, "sibnet")
	if want := []int{1}; !reflect.DeepEqual(got.Episodes, want) {
		t.Fatalf("sibnet episodes = %v, want %v", got.Episodes, want)
	}
	if want := []domain.AnimejoyTeamMeta{{ID: "0", Name: "Mixed"}}; !reflect.DeepEqual(got.Teams, want) {
		t.Fatalf("sibnet teams = %+v, want %+v", got.Teams, want)
	}
}

func TestBuildAnimejoyLegInfo_Empty(t *testing.T) {
	got := buildAnimejoyLegInfo(nil, "sibnet")
	if got.Episodes == nil {
		t.Fatalf("episodes must be non-nil empty slice, got nil")
	}
	if len(got.Episodes) != 0 {
		t.Fatalf("episodes = %v, want empty", got.Episodes)
	}
	if got.Teams == nil {
		t.Fatalf("teams must be non-nil empty slice, got nil")
	}
	if len(got.Teams) != 0 {
		t.Fatalf("teams = %+v, want empty", got.Teams)
	}
}
