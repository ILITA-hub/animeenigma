package probe

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type fakeCH struct{ rows []ProbeRow }

func (f *fakeCH) InsertProbeRows(_ context.Context, rows []ProbeRow) error {
	f.rows = append(f.rows, rows...)
	return nil
}

func TestReporter_SetsGaugeAndRows(t *testing.T) {
	ch := &fakeCH{}
	rep := NewPromReporter(ch)
	run := RunResult{
		ProviderVerdicts: []ProviderVerdict{{Provider: "gogoanime", Status: StatusUp}},
		Verdicts: []Verdict{{Provider: "gogoanime", AnimeName: "Frieren", Slot: SlotAnchor, Server: "s", Reason: streamprobe.ReasonPlayable}},
		At: 1000,
	}
	if err := rep.Report(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(metrics.ProbeProviderUp.WithLabelValues("gogoanime")); got != 1.0 {
		t.Fatalf("gauge=%v", got)
	}
	if len(ch.rows) != 1 {
		t.Fatalf("rows=%d", len(ch.rows))
	}
	if ch.rows[0].AnimeName != "Frieren" {
		t.Fatalf("ProbeRow.AnimeName=%q, want Frieren", ch.rows[0].AnimeName)
	}
	// info metric: up provider with empty reason → reason label becomes "-"
	if got := testutil.ToFloat64(metrics.ProbeProviderStatus.WithLabelValues("gogoanime", string(StatusUp), "-")); got != 1.0 {
		t.Fatalf("info gauge = %v, want 1", got)
	}
}

func TestReporter_StatusInfoMetric_DegradedReason(t *testing.T) {
	ch := &fakeCH{}
	rep := NewPromReporter(ch)
	run := RunResult{
		ProviderVerdicts: []ProviderVerdict{
			{Provider: "animefever", Status: StatusDegraded, Reason: "status_403 on HD-1"},
		},
		Verdicts: []Verdict{},
		At:       2000,
	}
	if err := rep.Report(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	// degraded provider: reason label must be verbatim, not "-"
	if got := testutil.ToFloat64(metrics.ProbeProviderStatus.WithLabelValues("animefever", string(StatusDegraded), "status_403 on HD-1")); got != 1.0 {
		t.Fatalf("info gauge degraded = %v, want 1", got)
	}
}
