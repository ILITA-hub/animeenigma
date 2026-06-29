package animejoy

import (
	"strings"
	"testing"
)

func TestParsePlaylistFrieren(t *testing.T) {
	teams, err := parsePlaylist(readFixture(t, "playlist_3647.json"))
	if err != nil {
		t.Fatalf("parsePlaylist: %v", err)
	}
	if len(teams) == 0 {
		t.Fatalf("no teams parsed")
	}

	// Collect distinct episode numbers across all teams, and verify every
	// episode carries BOTH a Sibnet (sibnet.ru) and an AllVideo (fsst.online)
	// embed URL.
	nums := map[int]bool{}
	for _, team := range teams {
		for _, ep := range team.Episodes {
			nums[ep.Num] = true
			if ep.Sibnet == "" || !strings.Contains(ep.Sibnet, "sibnet.ru") {
				t.Fatalf("team %q ep %d: bad Sibnet url %q", team.ID, ep.Num, ep.Sibnet)
			}
			if ep.AllVideo == "" || !strings.Contains(ep.AllVideo, "fsst.online") {
				t.Fatalf("team %q ep %d: bad AllVideo url %q", team.ID, ep.Num, ep.AllVideo)
			}
		}
	}
	if len(nums) != 28 {
		t.Fatalf("distinct episodes: want 28, got %d", len(nums))
	}
	// Episodes 1..28 specifically.
	for i := 1; i <= 28; i++ {
		if !nums[i] {
			t.Fatalf("missing episode %d", i)
		}
	}
}

func TestParsePlaylistDropsNonKeptPlayers(t *testing.T) {
	// playlist_632 (Steins;Gate): players AllVideo/Sibnet/Mail/CDA. We keep only
	// Sibnet + AllVideo, so every parsed episode must carry at least one of them
	// and never a mail.ru / cda.pl URL.
	teams, err := parsePlaylist(readFixture(t, "playlist_632.json"))
	if err != nil {
		t.Fatalf("parsePlaylist: %v", err)
	}
	got := 0
	for _, team := range teams {
		for _, ep := range team.Episodes {
			got++
			if ep.Sibnet == "" && ep.AllVideo == "" {
				t.Fatalf("ep %d has neither kept leg", ep.Num)
			}
			if strings.Contains(ep.Sibnet, "mail.ru") || strings.Contains(ep.AllVideo, "cda.pl") {
				t.Fatalf("ep %d leaked a dropped player: sib=%q av=%q", ep.Num, ep.Sibnet, ep.AllVideo)
			}
		}
	}
	if got == 0 {
		t.Fatalf("no episodes parsed from playlist_632")
	}
}

func TestParsePlaylistEpisodesSortedAscending(t *testing.T) {
	teams, err := parsePlaylist(readFixture(t, "playlist_3647.json"))
	if err != nil {
		t.Fatalf("parsePlaylist: %v", err)
	}
	for _, team := range teams {
		prev := 0
		for _, ep := range team.Episodes {
			if ep.Num <= prev {
				t.Fatalf("team %q episodes not strictly ascending: %d after %d", team.ID, ep.Num, prev)
			}
			prev = ep.Num
		}
	}
}

func TestParsePlaylistErrorsOnGarbage(t *testing.T) {
	if _, err := parsePlaylist([]byte("not json")); err == nil {
		t.Fatalf("expected error on non-JSON input")
	}
}
