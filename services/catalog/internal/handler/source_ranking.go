package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/sourceranking"
)

// rankReader is the subset of *sourceranking.Reader the handler needs.
type rankReader interface {
	Read(ctx context.Context, animeID string) sourceranking.Ranking
}

// fixWriter is the subset of *sourceranking.Writer the handler needs.
type fixWriter interface {
	SetFix(ctx context.Context, animeID, provider string) error
}

// SourceRankingHandler serves GET /api/anime/{animeId}/source-ranking — the
// Stage 2b learned provider-reliability ranking (global + per-anime). Read-only,
// no auth; the ranking is advisory data the player merges into its smart default.
// It also serves POST /api/anime/{animeId}/source-fix — a same-day override write
// the player fires when a client-side fallback rescued a failed resolve.
type SourceRankingHandler struct {
	reader rankReader
	writer fixWriter
	log    *logger.Logger
}

// NewSourceRankingHandler constructs the handler. log may be nil.
func NewSourceRankingHandler(reader rankReader, writer fixWriter, log *logger.Logger) *SourceRankingHandler {
	return &SourceRankingHandler{reader: reader, writer: writer, log: log}
}

// Get handles GET /api/anime/{animeId}/source-ranking. The reader never errors
// (a missing/malformed ranking yields empty slices), so this always 200s with
// the {success,data} envelope.
func (h *SourceRankingHandler) Get(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	out := h.reader.Read(r.Context(), animeID)
	httputil.OK(w, out)
}

// Post handles POST /api/anime/{animeId}/source-fix. The body is {"provider":"..."}.
// On a malformed body or a rejected (unknown) provider it returns 400; on success
// it writes the same-day srcfix override and returns 204 No Content.
func (h *SourceRankingHandler) Post(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	var req struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		httputil.Error(w, errors.InvalidInput("invalid request body"))
		return
	}
	if err := h.writer.SetFix(r.Context(), animeID, req.Provider); err != nil {
		if h.log != nil {
			h.log.Warnw("source-fix rejected", "anime_id", animeID, "provider", req.Provider, "error", err)
		}
		httputil.Error(w, errors.InvalidInput("invalid provider"))
		return
	}
	httputil.NoContent(w)
}
