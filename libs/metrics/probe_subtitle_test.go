package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestProbeSubtitleProviderUp_SetAndGather(t *testing.T) {
	ProbeSubtitleProviderUp.Reset()
	ProbeSubtitleProviderUp.WithLabelValues("jimaku").Set(0.5)
	if got := testutil.ToFloat64(ProbeSubtitleProviderUp.WithLabelValues("jimaku")); got != 0.5 {
		t.Fatalf("probe_subtitle_provider_up{jimaku} = %v; want 0.5", got)
	}
}

func TestProbeSubtitleLatency_SetAndGather(t *testing.T) {
	ProbeSubtitleLatencySeconds.WithLabelValues("opensubtitles").Set(1.25)
	if got := testutil.ToFloat64(ProbeSubtitleLatencySeconds.WithLabelValues("opensubtitles")); got != 1.25 {
		t.Fatalf("probe_subtitle_latency_seconds{opensubtitles} = %v; want 1.25", got)
	}
}

func TestProbeSubtitleLastRun_Set(t *testing.T) {
	ProbeSubtitleLastRun.Set(1700000000)
	if got := testutil.ToFloat64(ProbeSubtitleLastRun); got != 1700000000 {
		t.Fatalf("probe_subtitle_last_run_timestamp = %v; want 1700000000", got)
	}
}
