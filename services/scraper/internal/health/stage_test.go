package health

import (
	"reflect"
	"testing"
)

// TestAllStages_Length guards against silent additions that would fragment
// dashboards. The five canonical stages are a versioned contract — adding a
// sixth (or removing one) MUST be a deliberate phase change with dashboard
// updates.
func TestAllStages_Length(t *testing.T) {
	if got := len(AllStages); got != 5 {
		t.Fatalf("len(AllStages) = %d; want 5", got)
	}
}

// TestAllStages_OrderAndValues locks the exact strings and execution order
// the probe relies on (search → episodes → servers → stream → stream_segment).
// The order is structurally significant: the probe short-circuits on first
// failure, so a permuted slice would change which stage gets exercised.
func TestAllStages_OrderAndValues(t *testing.T) {
	want := []string{"search", "episodes", "servers", "stream", "stream_segment"}
	if !reflect.DeepEqual(AllStages, want) {
		t.Errorf("AllStages = %v; want %v", AllStages, want)
	}
}

// TestStageConstants_MatchAllStages ensures the exported constants align with
// the slice entries (no copy-paste drift between the two definitions).
func TestStageConstants_MatchAllStages(t *testing.T) {
	pairs := []struct {
		got, want string
	}{
		{StageSearch, "search"},
		{StageEpisodes, "episodes"},
		{StageServers, "servers"},
		{StageStream, "stream"},
		{StageStreamSegment, "stream_segment"},
	}
	for _, p := range pairs {
		if p.got != p.want {
			t.Errorf("constant = %q; want %q", p.got, p.want)
		}
	}
}
