package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// HealthHandler responds to GET /health with a flat {status: "ok"} payload
// wrapped in the standard httputil envelope ({success, data}). Phase 1 keeps
// the handler dependency-free; Phase 3 will add a /health/extended endpoint
// (LIB-09) that probes Postgres + torrent client + ffmpeg + MinIO, which is
// why the handler is factored into a named struct rather than an inline
// closure on the router.
type HealthHandler struct{}

// NewHealthHandler constructs a HealthHandler with no dependencies.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Health responds 200 with {"status":"ok"} (wrapped in httputil's envelope).
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	httputil.OK(w, map[string]string{"status": "ok"})
}
