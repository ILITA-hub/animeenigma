package metrics_test

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// newTestRegistry returns a fresh registry with all upscale_* metrics
// registered. Using a separate registry per test avoids label-collision
// panics from the default promauto global registry.
func newUpscaleRegistry(t *testing.T) *prometheus.Registry {
	t.Helper()
	reg := prometheus.NewRegistry()

	mustRegister := func(cs ...prometheus.Collector) {
		t.Helper()
		for _, c := range cs {
			if err := reg.Register(c); err != nil {
				// Already registered on the default global registry (promauto) —
				// that's expected; we're just gathering from the global one in tests.
				_ = err
			}
		}
	}
	mustRegister(
		metrics.UpscaleWorkersConnected,
		metrics.UpscaleLeaseExpiredTotal,
		metrics.UpscaleCommandTotal,
		metrics.UpscaleEnrollTotal,
		metrics.UpscaleEdgeRequestsTotal,
		metrics.UpscaleSegmentDuration,
		metrics.UpscaleJobProgressRatio,
		metrics.UpscaleJobEtaSeconds,
		metrics.UpscaleQueueDepth,
		metrics.UpscaleWorkerGPUUtil,
		metrics.UpscaleWorkerVRAMUsedBytes,
		metrics.UpscaleDecodeFPS,
		metrics.UpscaleInferenceFPS,
		metrics.UpscaleEncodeFPS,
	)
	return reg
}

// TestUpscaleMetrics_Register verifies every metric descriptor is registered
// without error on a fresh prometheus.Registry and that Gather succeeds.
func TestUpscaleMetrics_Register(t *testing.T) {
	reg := prometheus.NewRegistry()
	collectors := []prometheus.Collector{
		metrics.UpscaleWorkersConnected,
		metrics.UpscaleLeaseExpiredTotal,
		metrics.UpscaleCommandTotal,
		metrics.UpscaleEnrollTotal,
		metrics.UpscaleEdgeRequestsTotal,
		metrics.UpscaleSegmentDuration,
		metrics.UpscaleJobProgressRatio,
		metrics.UpscaleJobEtaSeconds,
		metrics.UpscaleQueueDepth,
		metrics.UpscaleWorkerGPUUtil,
		metrics.UpscaleWorkerVRAMUsedBytes,
		metrics.UpscaleDecodeFPS,
		metrics.UpscaleInferenceFPS,
		metrics.UpscaleEncodeFPS,
	}
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			t.Errorf("Register(%T) failed: %v", c, err)
		}
	}
	if _, err := reg.Gather(); err != nil {
		t.Errorf("Gather() failed: %v", err)
	}
}

// TestUpscaleCommandTotal_IndependentLabels verifies that
// UpscaleCommandTotal{type=cancel} and {type=drain} increment independently.
func TestUpscaleCommandTotal_IndependentLabels(t *testing.T) {
	// Use testutil.ToFloat64 against the promauto global registry.
	metrics.UpscaleCommandTotal.WithLabelValues("cancel").Add(0) // ensure initialised
	metrics.UpscaleCommandTotal.WithLabelValues("drain").Add(0)

	before := testutil.ToFloat64(metrics.UpscaleCommandTotal.WithLabelValues("cancel"))
	metrics.UpscaleCommandTotal.WithLabelValues("cancel").Inc()
	after := testutil.ToFloat64(metrics.UpscaleCommandTotal.WithLabelValues("cancel"))
	if after-before != 1 {
		t.Errorf("UpscaleCommandTotal{cancel}: got delta %v, want 1", after-before)
	}

	drainVal := testutil.ToFloat64(metrics.UpscaleCommandTotal.WithLabelValues("drain"))
	if drainVal != 0 {
		// drain was not incremented above — its value should remain at its
		// initialised baseline (not grow with cancel).
		// We only assert the two differ, not an absolute value.
		t.Errorf("UpscaleCommandTotal{drain} changed unexpectedly: %v", drainVal)
	}
}

// TestUpscaleLeaseExpiredTotal_Increments verifies the counter registers and increments.
func TestUpscaleLeaseExpiredTotal_Increments(t *testing.T) {
	before := testutil.ToFloat64(metrics.UpscaleLeaseExpiredTotal)
	metrics.UpscaleLeaseExpiredTotal.Inc()
	after := testutil.ToFloat64(metrics.UpscaleLeaseExpiredTotal)
	if after-before != 1 {
		t.Errorf("UpscaleLeaseExpiredTotal: got delta %v, want 1", after-before)
	}
}

// TestUpscaleEnrollTotal_Results verifies ok/bad_token/error are independent.
func TestUpscaleEnrollTotal_Results(t *testing.T) {
	results := []string{"ok", "bad_token", "error"}
	for _, r := range results {
		before := testutil.ToFloat64(metrics.UpscaleEnrollTotal.WithLabelValues(r))
		metrics.UpscaleEnrollTotal.WithLabelValues(r).Inc()
		after := testutil.ToFloat64(metrics.UpscaleEnrollTotal.WithLabelValues(r))
		if after-before != 1 {
			t.Errorf("UpscaleEnrollTotal{%s}: got delta %v, want 1", r, after-before)
		}
	}
}

// TestUpscaleJobGauges verifies progress/eta/queue gauges can be set.
func TestUpscaleJobGauges(t *testing.T) {
	metrics.UpscaleJobProgressRatio.Set(0.5)
	if got := testutil.ToFloat64(metrics.UpscaleJobProgressRatio); got != 0.5 {
		t.Errorf("UpscaleJobProgressRatio: got %v, want 0.5", got)
	}

	metrics.UpscaleJobEtaSeconds.Set(120)
	if got := testutil.ToFloat64(metrics.UpscaleJobEtaSeconds); got != 120 {
		t.Errorf("UpscaleJobEtaSeconds: got %v, want 120", got)
	}

	metrics.UpscaleQueueDepth.WithLabelValues("queued").Set(3)
	if got := testutil.ToFloat64(metrics.UpscaleQueueDepth.WithLabelValues("queued")); got != 3 {
		t.Errorf("UpscaleQueueDepth{queued}: got %v, want 3", got)
	}
}

// TestRecordWorkerTelemetry verifies the helper sets all five worker gauges.
func TestRecordWorkerTelemetry(t *testing.T) {
	metrics.RecordWorkerTelemetry("RTX4090", "v1.2.3", 0.85, 16e9, 30, 25, 28)

	lbls := prometheus.Labels{"gpu_model": "RTX4090", "image_version": "v1.2.3"}
	if got := testutil.ToFloat64(metrics.UpscaleWorkerGPUUtil.With(lbls)); got != 0.85 {
		t.Errorf("UpscaleWorkerGPUUtil: got %v, want 0.85", got)
	}
	if got := testutil.ToFloat64(metrics.UpscaleWorkerVRAMUsedBytes.With(lbls)); got != 16e9 {
		t.Errorf("UpscaleWorkerVRAMUsedBytes: got %v, want 16e9", got)
	}
	if got := testutil.ToFloat64(metrics.UpscaleDecodeFPS.With(lbls)); got != 30 {
		t.Errorf("UpscaleDecodeFPS: got %v, want 30", got)
	}
	if got := testutil.ToFloat64(metrics.UpscaleInferenceFPS.With(lbls)); got != 25 {
		t.Errorf("UpscaleInferenceFPS: got %v, want 25", got)
	}
	if got := testutil.ToFloat64(metrics.UpscaleEncodeFPS.With(lbls)); got != 28 {
		t.Errorf("UpscaleEncodeFPS: got %v, want 28", got)
	}
}

// TestNoWorkerIDLabel_CardinalityGuard asserts that no counter or histogram
// in the upscale_* metric set has a "worker_id" label — enforcing cardinality
// discipline (worker_id is unbounded and must never appear on these metrics).
func TestNoWorkerIDLabel_CardinalityGuard(t *testing.T) {
	reg := prometheus.NewRegistry()

	type namedCollector struct {
		name string
		c    prometheus.Collector
	}
	countersAndHistograms := []namedCollector{
		{"UpscaleLeaseExpiredTotal", metrics.UpscaleLeaseExpiredTotal},
		{"UpscaleCommandTotal", metrics.UpscaleCommandTotal},
		{"UpscaleEnrollTotal", metrics.UpscaleEnrollTotal},
		{"UpscaleEdgeRequestsTotal", metrics.UpscaleEdgeRequestsTotal},
		{"UpscaleSegmentDuration", metrics.UpscaleSegmentDuration},
	}

	for _, nc := range countersAndHistograms {
		// Trigger at least one label-set so desc is populated.
		if err := reg.Register(nc.c); err != nil {
			// already registered globally — that's fine, we just gather descs
		}

		ch := make(chan *prometheus.Desc, 10)
		nc.c.Describe(ch)
		close(ch)

		for desc := range ch {
			// Desc.String() contains "variableLabels=[...]" — we parse that.
			s := desc.String()
			if contains(s, "worker_id") {
				t.Errorf("%s has forbidden label 'worker_id' in descriptor: %s", nc.name, s)
			}
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsRune(s, sub))
}

func containsRune(s, sub string) bool {
	for i := range s {
		if i+len(sub) <= len(s) && s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
