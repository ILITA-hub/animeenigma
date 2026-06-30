package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

// AeHandler exposes the first-party ("AnimeEnigma") self-hosted video provider
// endpoints under /api/anime/{animeId}/ae/*. It is backed by the library
// resolver (MinIO HLS). The standalone "raw JP" provider that previously shared
// this handler was removed 2026-06-30 — AllAnime + ok.ru cover JP-original now.
type AeHandler struct {
	resolver *service.RawResolver
	log      *logger.Logger
}

// NewAeHandler wires the library resolver into a chi-compatible handler.
func NewAeHandler(resolver *service.RawResolver, log *logger.Logger) *AeHandler {
	return &AeHandler{resolver: resolver, log: log}
}

// GetAeEpisodes — GET /api/anime/{animeId}/ae/episodes.
//
// First-party ("AnimeEnigma") provider: lists ONLY episodes present in
// the self-hosted library (MinIO). Returns 200 with available=false +
// an empty array when nothing is encoded locally (the player hides the
// provider), so this never 404s on the empty state.
func (h *AeHandler) GetAeEpisodes(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}
	resp, err := h.resolver.GetLibraryEpisodes(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, resp)
}

// GetAeStream — GET /api/anime/{animeId}/ae/stream?episode={n}&quality={q}.
//
// First-party provider stream: resolves STRICTLY from MinIO (no AllAnime
// fallback) and returns a proxy-signed URL. 404 when the episode isn't
// encoded locally yet.
func (h *AeHandler) GetAeStream(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}
	episodeRaw := r.URL.Query().Get("episode")
	if episodeRaw == "" {
		httputil.BadRequest(w, "episode is required")
		return
	}
	episode, err := strconv.Atoi(episodeRaw)
	if err != nil || episode <= 0 {
		httputil.BadRequest(w, "episode must be a positive integer")
		return
	}
	quality := r.URL.Query().Get("quality") // optional

	stream, err := h.resolver.GetLibraryStream(r.Context(), animeID, episode, quality)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, stream)
}
