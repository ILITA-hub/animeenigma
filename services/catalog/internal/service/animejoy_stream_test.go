package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animejoy"
)

// pickLegEmbed is the PURE leg/team/episode selector that maps a (leg, episode,
// teamID) request onto a concrete Sibnet/AllVideo embed URL from the discovery
// playlist. These tests pin its selection rules with no network.

func twoTeamsFixture() []animejoy.Team {
	return []animejoy.Team{
		{
			ID:   "0",
			Name: "AnimeJoy",
			Episodes: []animejoy.Episode{
				{Num: 1, Sibnet: "https://iv.sibnet.ru/shell.php?videoid=10", AllVideo: "https://fsst.online/embed/a1"},
				{Num: 2, Sibnet: "https://iv.sibnet.ru/shell.php?videoid=20"},
			},
		},
		{
			ID:   "1",
			Name: "Team B",
			Episodes: []animejoy.Episode{
				{Num: 1, AllVideo: "https://fsst.online/embed/b1"},
				{Num: 2, Sibnet: "https://iv.sibnet.ru/shell.php?videoid=21", AllVideo: "https://fsst.online/embed/b2"},
			},
		},
	}
}

func TestPickLegEmbed_FirstTeamWithEpisode(t *testing.T) {
	// No teamID: choose the first team that HAS the episode for this leg.
	// Sibnet ep1 exists on team "0" only at ep1 → return team 0's embed.
	got, err := pickLegEmbed(twoTeamsFixture(), "sibnet", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://iv.sibnet.ru/shell.php?videoid=10" {
		t.Fatalf("sibnet ep1 = %q, want team 0 embed", got)
	}
}

func TestPickLegEmbed_AllVideoLeg(t *testing.T) {
	got, err := pickLegEmbed(twoTeamsFixture(), "allvideo", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// team 0 ep1 has AllVideo; it is first → return it.
	if got != "https://fsst.online/embed/a1" {
		t.Fatalf("allvideo ep1 = %q, want team 0 embed", got)
	}
}

func TestPickLegEmbed_SkipsTeamMissingLeg(t *testing.T) {
	// Sibnet ep2: team 0 HAS it (videoid=20), so team 0 is picked first.
	got, err := pickLegEmbed(twoTeamsFixture(), "sibnet", 2, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://iv.sibnet.ru/shell.php?videoid=20" {
		t.Fatalf("sibnet ep2 = %q, want team 0 embed", got)
	}
}

func TestPickLegEmbed_LegFilter_FirstTeamLacksLeg(t *testing.T) {
	// AllVideo ep2: team 0 ep2 has NO AllVideo, team 1 ep2 does → skip team 0,
	// fall through to team 1.
	got, err := pickLegEmbed(twoTeamsFixture(), "allvideo", 2, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://fsst.online/embed/b2" {
		t.Fatalf("allvideo ep2 = %q, want team 1 embed", got)
	}
}

func TestPickLegEmbed_TeamIDPick(t *testing.T) {
	// teamID "1" present → use it even though team 0 also has the episode.
	got, err := pickLegEmbed(twoTeamsFixture(), "allvideo", 1, "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://fsst.online/embed/b1" {
		t.Fatalf("teamID=1 allvideo ep1 = %q, want team 1 embed", got)
	}
}

func TestPickLegEmbed_TeamIDMissingLegForEpisode(t *testing.T) {
	// teamID "0" pinned but team 0 ep1 lacks no leg... use sibnet ep2 on a team
	// that has it. teamID "1" ep1 has NO Sibnet → error (no intra-team fallback
	// to another team when a team is explicitly pinned).
	_, err := pickLegEmbed(twoTeamsFixture(), "sibnet", 1, "1")
	if err == nil {
		t.Fatalf("expected error: pinned team 1 ep1 has no sibnet leg")
	}
}

func TestPickLegEmbed_TeamIDNotFound(t *testing.T) {
	_, err := pickLegEmbed(twoTeamsFixture(), "sibnet", 1, "nope")
	if err == nil {
		t.Fatalf("expected error for unknown teamID")
	}
}

func TestPickLegEmbed_EpisodeNotFound(t *testing.T) {
	_, err := pickLegEmbed(twoTeamsFixture(), "sibnet", 99, "")
	if err == nil {
		t.Fatalf("expected error for missing episode")
	}
}

func TestPickLegEmbed_UnknownLeg(t *testing.T) {
	_, err := pickLegEmbed(twoTeamsFixture(), "dzen", 1, "")
	if err == nil {
		t.Fatalf("expected error for unknown leg")
	}
}

func TestPickLegEmbed_EmptyTeams(t *testing.T) {
	_, err := pickLegEmbed(nil, "sibnet", 1, "")
	if err == nil {
		t.Fatalf("expected error for empty teams")
	}
}
