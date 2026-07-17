// Package handler serves the internal content-verify API: verdicts, the
// visitor/watching hint sink, and the queue snapshot.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/signals"
)

type VerifyHandler struct {
	store  *repo.Store
	sig    *signals.Signals
	engine *queue.Engine
	log    *logger.Logger
}

func NewVerifyHandler(store *repo.Store, sig *signals.Signals, engine *queue.Engine, log *logger.Logger) *VerifyHandler {
	return &VerifyHandler{store: store, sig: sig, engine: engine, log: log}
}

type providerVerdicts struct {
	Provider string                 `json:"provider"`
	Summary  domain.ProviderSummary `json:"summary"`
	Units    []domain.UnitVerdict   `json:"units"`
}

type verdictsResponse struct {
	AnimeID   string             `json:"anime_id"`
	Providers []providerVerdicts `json:"providers"`
}

func (h *VerifyHandler) Verdicts(w http.ResponseWriter, r *http.Request) {
	animeID := r.URL.Query().Get("anime_id")
	if animeID == "" {
		http.Error(w, "anime_id required", http.StatusBadRequest)
		return
	}
	rows, err := h.store.ByAnime(r.Context(), animeID)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	resp := verdictsResponse{AnimeID: animeID, Providers: []providerVerdicts{}}
	for _, row := range rows {
		resp.Providers = append(resp.Providers, providerVerdicts{
			Provider: row.Provider, Summary: domain.Summarize(row.Units), Units: row.Units})
	}
	httputil.OK(w, resp)
}

type skipResponse struct {
	AnimeID string              `json:"anime_id"`
	Timings []domain.SkipTiming `json:"timings"`
}

func (h *VerifyHandler) Skip(w http.ResponseWriter, r *http.Request) {
	animeID := r.URL.Query().Get("anime_id")
	if animeID == "" {
		http.Error(w, "anime_id required", http.StatusBadRequest)
		return
	}
	rows, err := h.store.SkipByAnime(r.Context(), animeID)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	resp := skipResponse{AnimeID: animeID, Timings: []domain.SkipTiming{}}
	resp.Timings = append(resp.Timings, rows...)
	httputil.OK(w, resp)
}

type hintRequest struct {
	AnimeID string `json:"anime_id"`
	Visitor string `json:"visitor"`
	Source  string `json:"source"`
}

func (h *VerifyHandler) Hint(w http.ResponseWriter, r *http.Request) {
	var req hintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AnimeID == "" || req.Visitor == "" {
		http.Error(w, "anime_id and visitor required", http.StatusBadRequest)
		return
	}
	if err := h.sig.RecordVisit(r.Context(), req.AnimeID, req.Visitor); err != nil && h.log != nil {
		h.log.Warnw("hint record failed", "anime_id", req.AnimeID, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *VerifyHandler) Queue(w http.ResponseWriter, r *http.Request) {
	httputil.OK(w, map[string]any{"entries": h.engine.Snapshot(r.Context(), 50)})
}
