package repo

import (
	"strings"
	"testing"
)

// The query text is a package-level const so it is testable without a live CH.
func TestPlayabilityWatchQuery_Shape(t *testing.T) {
	q := playabilityWatchQuery
	for _, want := range []string{
		"effect_kind = 'player_resolve'",
		"JSONExtractBool(properties,'reached_playback')",
		"exp(-dateDiff('day', timestamp, now()) / 14.0)",
		"target AS provider",
		"anime_id = ?",       // this_anime_watch term binds the anime id
		"GROUP BY target",
		"INTERVAL 60 DAY",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("watch query missing %q\n---\n%s", want, q)
		}
	}
}

func TestPlayabilityProbeQuery_Shape(t *testing.T) {
	q := playabilityProbeQuery
	for _, want := range []string{
		"FROM probe_runs",
		"playable = 1",
		"exp(-dateDiff('day', run_ts, now()) / 14.0)",
		"GROUP BY provider",
		"INTERVAL 60 DAY",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("probe query missing %q\n---\n%s", want, q)
		}
	}
}
