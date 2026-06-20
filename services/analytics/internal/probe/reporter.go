package probe

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// ProbeRow is one ClickHouse history row.
type ProbeRow struct {
	RunTS     int64
	Provider  string
	AnimeUUID string
	AnimeName string
	Slot      string
	Server    string
	Stage     string
	Reason    string
	Playable  bool
}

type CHWriter interface {
	InsertProbeRows(ctx context.Context, rows []ProbeRow) error
}

type RunResult struct {
	ProviderVerdicts []ProviderVerdict
	Verdicts         []Verdict
	At               int64 // unix seconds
}

type Reporter interface {
	Report(ctx context.Context, run RunResult) error
}

type PromReporter struct{ ch CHWriter }

func NewPromReporter(ch CHWriter) *PromReporter { return &PromReporter{ch: ch} }

func (r *PromReporter) Report(ctx context.Context, run RunResult) error {
	metrics.ProbeProviderStatus.Reset()
	for _, pv := range run.ProviderVerdicts {
		metrics.ProbeProviderUp.WithLabelValues(pv.Provider).Set(pv.Status.Gauge())
		reason := pv.Reason
		if reason == "" {
			reason = "-"
		}
		metrics.ProbeProviderStatus.WithLabelValues(pv.Provider, string(pv.Status), reason).Set(1)
	}
	rows := make([]ProbeRow, 0, len(run.Verdicts))
	for _, v := range run.Verdicts {
		result := "fail"
		if v.Playable() {
			result = "pass"
		}
		metrics.ProbeRunsTotal.WithLabelValues(v.Provider, string(v.Slot), v.Server, result, string(v.Reason)).Inc()
		rows = append(rows, ProbeRow{
			RunTS: run.At, Provider: v.Provider, AnimeUUID: v.AnimeUUID, AnimeName: v.AnimeName, Slot: string(v.Slot),
			Server: v.Server, Stage: string(v.Stage), Reason: string(v.Reason), Playable: v.Playable(),
		})
	}
	metrics.ProbeLastRun.Set(float64(run.At))
	if r.ch != nil {
		return r.ch.InsertProbeRows(ctx, rows)
	}
	return nil
}
