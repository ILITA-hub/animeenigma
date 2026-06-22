package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordSubtitleResolve(t *testing.T) {
	SubtitleProviderUp.Reset()
	SubtitleResolveTotal.Reset()

	RecordSubtitleResolve(0.5, []SubtitleProviderOutcome{
		{Provider: "jimaku", Status: "ok", Tracks: 3},
		{Provider: "opensubtitles", Status: "down", Tracks: 0},
	})

	if got := testutil.ToFloat64(SubtitleProviderUp.WithLabelValues("jimaku")); got != 1 {
		t.Fatalf("jimaku up = %v, want 1", got)
	}
	if got := testutil.ToFloat64(SubtitleProviderUp.WithLabelValues("opensubtitles")); got != 0 {
		t.Fatalf("opensubtitles up = %v, want 0", got)
	}
	if got := testutil.ToFloat64(SubtitleResolveTotal.WithLabelValues("jimaku", "ok")); got != 1 {
		t.Fatalf("jimaku ok total = %v, want 1", got)
	}
}

func TestRecordSubtitleResolve_UnconfiguredSkipsGauge(t *testing.T) {
	SubtitleProviderUp.Reset()
	RecordSubtitleResolve(0.1, []SubtitleProviderOutcome{
		{Provider: "opensubtitles", Status: "unconfigured", Tracks: 0},
	})
	// Unconfigured must NOT set the up gauge (neither up nor down — it's absent).
	if n := testutil.CollectAndCount(SubtitleProviderUp); n != 0 {
		t.Fatalf("expected no gauge series for unconfigured, got %d", n)
	}
}
