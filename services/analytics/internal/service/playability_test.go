package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

func allow(known ...string) func(string) bool {
	m := map[string]struct{}{}
	for _, k := range known {
		m[k] = struct{}{}
	}
	return func(p string) bool { _, ok := m[p]; return ok }
}

func TestBuildPlayabilityScores_MergesAndFilters(t *testing.T) {
	watch := []repo.PlayabilityWatchRow{
		{Provider: "allanime-okru", GlobalWatch: 41.3, ThisAnimeWatch: 2.7},
		{Provider: "evil", GlobalWatch: 9, ThisAnimeWatch: 9}, // not in roster → dropped
	}
	probe := []repo.PlayabilityProbeRow{
		{Provider: "allanime-okru", RecentUp: 6.1},
		{Provider: "kodik-noads", RecentUp: 4.0}, // in roster, no watch rows → probe-only entry
	}
	got := BuildPlayabilityScores(watch, probe, allow("allanime-okru", "kodik-noads"))

	if _, ok := got["evil"]; ok {
		t.Fatal("unroster provider must be filtered out")
	}
	aa := got["allanime-okru"]
	if aa.GlobalWatch != 41.3 || aa.ThisAnimeWatch != 2.7 || aa.RecentUp != 6.1 {
		t.Errorf("allanime-okru merge wrong: %+v", aa)
	}
	kn := got["kodik-noads"]
	if kn.RecentUp != 4.0 || kn.GlobalWatch != 0 || kn.ThisAnimeWatch != 0 {
		t.Errorf("probe-only provider wrong: %+v", kn)
	}
}

func TestBuildPlayabilityScores_Empty(t *testing.T) {
	got := BuildPlayabilityScores(nil, nil, allow("ae"))
	if len(got) != 0 {
		t.Errorf("empty inputs → empty map, got %d entries", len(got))
	}
}
