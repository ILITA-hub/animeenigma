package cvmetrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestBandDepthGauge(t *testing.T) {
	BandDepth.WithLabelValues("ongoing").Set(7)
	if got := testutil.ToFloat64(BandDepth.WithLabelValues("ongoing")); got != 7 {
		t.Errorf("band_depth{ongoing} = %v, want 7", got)
	}
}

func TestVerdictAndHardsubCounters(t *testing.T) {
	VerdictsTotal.WithLabelValues("ja").Inc()
	if testutil.ToFloat64(VerdictsTotal.WithLabelValues("ja")) != 1 {
		t.Error("verdicts_total{ja} not incremented")
	}
	HardsubTotal.WithLabelValues("unknown").Inc()
	if testutil.ToFloat64(HardsubTotal.WithLabelValues("unknown")) != 1 {
		t.Error("hardsub_total{unknown} not incremented")
	}
	// probes_total now takes three labels
	ProbesTotal.WithLabelValues("kodik", "verified", "ongoing").Inc()
	if testutil.ToFloat64(ProbesTotal.WithLabelValues("kodik", "verified", "ongoing")) != 1 {
		t.Error("probes_total{kodik,verified,ongoing} not incremented")
	}
}

func TestConcurrencyAndIdleGauges(t *testing.T) {
	InflightLeases.Set(2)
	if testutil.ToFloat64(InflightLeases) != 2 {
		t.Error("inflight_leases not set")
	}
	IdleCursor.Set(300)
	if testutil.ToFloat64(IdleCursor) != 300 {
		t.Error("idle_cursor not set")
	}
}
