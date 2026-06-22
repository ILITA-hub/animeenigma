package service

import (
	"sort"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/subprobe"
)

type fakeSnapshotter struct{ snap map[string]subprobe.Health }

func (f fakeSnapshotter) Snapshot() map[string]subprobe.Health { return f.snap }

func TestOverlayHealth_MergesProbeAndPassive(t *testing.T) {
	agg := &SubsAggregator{health: fakeSnapshotter{snap: map[string]subprobe.Health{
		"jimaku":        {Status: subprobe.StatusDegraded, LatencyMS: 4200, CheckedAt: time.Now()},
		"opensubtitles": {Status: subprobe.StatusUp, CheckedAt: time.Now()},
	}}}
	resp := &AggregateResponse{ProvidersDown: []string{"opensubtitles"}} // passive failure this request
	agg.overlayHealth(resp)

	got := map[string]string{}
	for _, h := range resp.ProviderHealth {
		got[h.Provider] = h.Status
	}
	// jimaku: probe degraded → surfaced as degraded.
	if got["jimaku"] != "degraded" {
		t.Errorf("jimaku = %q; want degraded", got["jimaku"])
	}
	// opensubtitles: probe up BUT passive down → worse wins → down.
	if got["opensubtitles"] != "down" {
		t.Errorf("opensubtitles = %q; want down (worse-wins)", got["opensubtitles"])
	}
}

func TestOverlayHealth_OmitsUpAndUnknown(t *testing.T) {
	agg := &SubsAggregator{health: fakeSnapshotter{snap: map[string]subprobe.Health{
		"jimaku":        {Status: subprobe.StatusUp, CheckedAt: time.Now()},
		"opensubtitles": {Status: subprobe.StatusUnknown, CheckedAt: time.Now()},
	}}}
	resp := &AggregateResponse{}
	agg.overlayHealth(resp)
	if len(resp.ProviderHealth) != 0 {
		t.Fatalf("up/unknown must not surface; got %+v", resp.ProviderHealth)
	}
}

func TestOverlayHealth_SortedAndNilSafe(t *testing.T) {
	agg := &SubsAggregator{} // nil health
	resp := &AggregateResponse{}
	agg.overlayHealth(resp) // must not panic
	if resp.ProviderHealth != nil {
		t.Fatal("nil health must leave ProviderHealth nil")
	}

	agg.health = fakeSnapshotter{snap: map[string]subprobe.Health{
		"opensubtitles": {Status: subprobe.StatusDown, CheckedAt: time.Now()},
		"jimaku":        {Status: subprobe.StatusDown, CheckedAt: time.Now()},
	}}
	resp = &AggregateResponse{}
	agg.overlayHealth(resp)
	names := []string{resp.ProviderHealth[0].Provider, resp.ProviderHealth[1].Provider}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("ProviderHealth not sorted: %v", names)
	}
}
