package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// HealthHandler serves GET /health.
type HealthHandler struct{}

func NewHealthHandler() *HealthHandler { return &HealthHandler{} }

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	httputil.OK(w, map[string]string{"status": "ok", "service": "anidle"})
}
