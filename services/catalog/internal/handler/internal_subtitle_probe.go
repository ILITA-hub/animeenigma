package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// SubtitleProbeRunner runs one active subtitle probe sweep. Satisfied by
// *subprobe.Probe.
type SubtitleProbeRunner interface {
	RunOnce(ctx context.Context)
}

// InternalSubtitleProbeHandler exposes POST /internal/subtitle-probe/run. The
// scheduler fires it on a 5-min cron. Docker-network-only (the gateway does not
// proxy /internal/*), so no auth middleware.
type InternalSubtitleProbeHandler struct {
	runner SubtitleProbeRunner
	log    *logger.Logger
}

func NewInternalSubtitleProbeHandler(runner SubtitleProbeRunner, log *logger.Logger) *InternalSubtitleProbeHandler {
	return &InternalSubtitleProbeHandler{runner: runner, log: log}
}

// Run executes one probe sweep synchronously and returns 204. It uses a fresh
// background context (NOT the request ctx) so a client disconnect can't abort
// the sweep mid-write — the same lesson as the playback probe handler.
func (h *InternalSubtitleProbeHandler) Run(w http.ResponseWriter, r *http.Request) {
	h.runner.RunOnce(context.Background())
	w.WriteHeader(http.StatusNoContent)
}
