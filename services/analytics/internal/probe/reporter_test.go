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
		Verdicts: []Verdict{{Provider: "gogoanime", Slot: SlotAnchor, Server: "s", Reason: streamprobe.ReasonPlayable}},
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
}
