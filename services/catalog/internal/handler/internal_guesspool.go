package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

// poolBuilder is the subset of GuessPoolService the handler needs (interface
// for testability).
type poolBuilder interface {
	BuildPool(ctx context.Context) ([]service.GuessPoolEntry, error)
}

// InternalGuessPoolHandler serves GET /internal/guessgame/pool (Docker-network
// only; NOT proxied by the gateway).
type InternalGuessPoolHandler struct {
	svc poolBuilder
	log *logger.Logger
}

func NewInternalGuessPoolHandler(svc poolBuilder, log *logger.Logger) *InternalGuessPoolHandler {
	return &InternalGuessPoolHandler{svc: svc, log: log}
}

func (h *InternalGuessPoolHandler) GetPool(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.BuildPool(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, entries)
}
