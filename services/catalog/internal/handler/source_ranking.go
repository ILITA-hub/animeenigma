package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/sourceranking"
)

// rankReader is the subset of *sourceranking.Reader the handler needs.
type rankReader interface {
	Read(ctx context.Context, animeID string) sourceranking.Ranking
}

// SourceRankingHandler serves GET /api/anime/{animeId}/source-ranking — the
// Stage 2b learned provider-reliability ranking (global + per-anime). Read-only,
// no auth; the ranking is advisory data the player merges into its smart default.
type SourceRankingHandler struct {
	reader rankReader
	log    *logger.Logger
}

// NewSourceRankingHandler constructs the handler. log may be nil.
func NewSourceRankingHandler(reader rankReader, log *logger.Logger) *SourceRankingHandler {
	return &SourceRankingHandler{reader: reader, log: log}
}

// Get handles GET /api/anime/{animeId}/source-ranking. The reader never errors
// (a missing/malformed ranking yields empty slices), so this always 200s with
// the {success,data} envelope.
func (h *SourceRankingHandler) Get(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	out := h.reader.Read(r.Context(), animeID)
	httputil.OK(w, out)
}
