package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

// RawHandler exposes the "raw JP" video provider endpoints under
// /api/anime/{animeId}/raw/*. Workstream raw-jp, Phase 01.
type RawHandler struct {
	resolver *service.RawResolver
	log      *logger.Logger
}

// NewRawHandler wires the raw resolver into a chi-compatible handler.
func NewRawHandler(resolver *service.RawResolver, log *logger.Logger) *RawHandler {
	return &RawHandler{resolver: resolver, log: log}
}

// GetEpisodes — GET /api/anime/{animeId}/raw/episodes.
//
// Returns `{"episodes":[...],"available":bool,"source":"allanime"}`.
// When the anime simply isn't on AllAnime, returns 200 with available=false
// (the frontend hides the chip). Genuine upstream failures return 503 via
// the AppError unwrap path in httputil.Error.
func (h *RawHandler) GetEpisodes(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	resp, err := h.resolver.GetEpisodes(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, resp)
}

// GetStream — GET /api/anime/{animeId}/raw/stream?episode={n}&quality={q}.
//
// Returns the resolved playable stream (HLS URL, embedded subs, expires_at).
// On any upstream failure, AppError(CodeUnavailable) → HTTP 503 with body
// `{"success":false,"error":{"code":"UNAVAILABLE","message":"raw provider unavailable"}}`.
func (h *RawHandler) GetStream(w http.ResponseWriter, r *http.Request) {
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

	stream, err := h.resolver.GetStream(r.Context(), animeID, episode, quality)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, stream)
}

// GetAeEpisodes — GET /api/anime/{animeId}/ae/episodes.
//
// First-party ("AnimeEnigma") provider: lists ONLY episodes present in
// the self-hosted library (MinIO). Returns 200 with available=false +
// an empty array when nothing is encoded locally (the player hides the
// provider), so this never 404s on the empty state.
func (h *RawHandler) GetAeEpisodes(w http.ResponseWriter, r *http.Request) {
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
func (h *RawHandler) GetAeStream(w http.ResponseWriter, r *http.Request) {
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
