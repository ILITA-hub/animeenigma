package handler

import (
	"context"
	"net/http"
	"time"
)

// probeRunner is the analytics probe engine entry point (probe.Engine.RunOnce).
// Interface so the test injects a fake.
type probeRunner interface {
	RunOnce(ctx context.Context) error
}

// ProbeHandler triggers a single on-demand probe run. Registered ONLY at
// /internal/probe/run (the gateway never proxies /internal/*). The scheduler
// POSTs here on its daily cron; the cron job or an operator can also trigger
// it manually. An 8-minute timeout caps the full provider sweep.
type ProbeHandler struct{ runner probeRunner }

func NewProbeHandler(r probeRunner) *ProbeHandler { return &ProbeHandler{runner: r} }

// ServeHTTP runs one probe cycle (body ignored). 204 on success; 500 on error.
//
// The run is detached from the REQUEST context on purpose: the full sweep takes
// several minutes (ae + kodik-noads add live upstream resolves), and the daily
// scheduler trigger — or a manual curl — can disconnect before it finishes. A
// cancelled request context would abort the final ClickHouse write mid-run,
// leaving the Prometheus gauges updated but no probe_runs rows persisted. A
// fresh background context with generous headroom guarantees the verdict rows
// are written regardless of client timeout.
func (h *ProbeHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	if err := h.runner.RunOnce(ctx); err != nil {
		http.Error(w, "probe run failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
