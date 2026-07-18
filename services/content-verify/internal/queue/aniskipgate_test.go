package queue

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

func TestFilterAniskipCovered(t *testing.T) {
	units := []SkipUnit{
		{AnimeID: "a", Provider: "kodik", Team: "T", Episode: 1},          // op+ed covered → dropped
		{AnimeID: "a", Provider: "kodik", Team: "T", Episode: 2},          // op only → kept (ed probe-able on HLS)
		{AnimeID: "a", Provider: "animejoy-allvideo", Episode: 2},         // op only + animejoy (mp4, ed terminal) → dropped
		{AnimeID: "a", Provider: "animejoy-allvideo", Episode: 3},         // uncovered → kept
		{AnimeID: "a", Provider: "kodik", Team: "T", Episode: 4},          // ed only → kept (op probe-able)
		{AnimeID: "a", Provider: "animejoy-sibnet", Episode: 5},           // ed only + animejoy → kept (op is the probe-able kind and it's uncovered)
	}
	cov := AniskipCoverage{
		1: {domain.SkipKindOp, domain.SkipKindEd},
		2: {domain.SkipKindOp},
		3: {},
		4: {domain.SkipKindEd},
		5: {domain.SkipKindEd},
	}

	got := FilterAniskipCovered(units, cov)
	want := []SkipUnit{units[1], units[3], units[4], units[5]}
	if len(got) != len(want) {
		t.Fatalf("filtered = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("filtered[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// nil/empty coverage disables the gate entirely.
	if got := FilterAniskipCovered(units, nil); len(got) != len(units) {
		t.Fatalf("nil coverage must pass all units through, got %d of %d", len(got), len(units))
	}
}

func TestPreferProviderStablePartition(t *testing.T) {
	units := []SkipUnit{
		{Provider: "kodik", Episode: 1},
		{Provider: "animejoy-allvideo", Episode: 1},
		{Provider: "kodik", Episode: 2},
		{Provider: "animejoy-allvideo", Episode: 2},
	}
	got := PreferProvider(units, "animejoy-allvideo")
	wantProviders := []string{"animejoy-allvideo", "animejoy-allvideo", "kodik", "kodik"}
	wantEpisodes := []int{1, 2, 1, 2} // relative order preserved within each group
	for i := range got {
		if got[i].Provider != wantProviders[i] || got[i].Episode != wantEpisodes[i] {
			t.Fatalf("preferred[%d] = %+v, want %s ep%d", i, got[i], wantProviders[i], wantEpisodes[i])
		}
	}

	// Empty preference is a no-op passthrough.
	if got := PreferProvider(units, ""); len(got) != len(units) || got[0].Provider != "kodik" {
		t.Fatalf("empty preference must be a passthrough, got %+v", got)
	}
}

func TestAniskipCoverageCoveredKinds(t *testing.T) {
	cov := AniskipCoverage{3: {domain.SkipKindOp}}
	if kinds := cov.CoveredKinds(3); len(kinds) != 1 || kinds[0] != domain.SkipKindOp {
		t.Fatalf("CoveredKinds(3) = %v, want [op]", kinds)
	}
	if kinds := cov.CoveredKinds(9); kinds != nil {
		t.Fatalf("CoveredKinds(9) = %v, want nil for unchecked episode", kinds)
	}
	var nilCov AniskipCoverage
	if kinds := nilCov.CoveredKinds(1); kinds != nil {
		t.Fatalf("nil coverage CoveredKinds = %v, want nil", kinds)
	}
}
