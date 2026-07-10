// Package handler exposes the governor's read-only HTTP surface.
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/domain"
)

// SnapshotProvider is implemented by service.Governor.
type SnapshotProvider interface {
	Snapshot() domain.Snapshot
}

// StatusHandler serves the current published degradation state. Consumed for
// debugging (host 127.0.0.1:8100) and by Docker-network peers that prefer HTTP
// over Redis; not gateway-routed.
type StatusHandler struct {
	gov SnapshotProvider
}

// NewStatusHandler builds a StatusHandler.
func NewStatusHandler(gov SnapshotProvider) *StatusHandler {
	return &StatusHandler{gov: gov}
}

func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	httputil.OK(w, h.gov.Snapshot())
}
