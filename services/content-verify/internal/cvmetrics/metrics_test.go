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
