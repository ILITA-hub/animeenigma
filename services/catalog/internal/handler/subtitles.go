package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

// SubtitlesHandler mounts the aggregated subtitle endpoints under
// /api/anime/{animeId}/subtitles*. Workstream raw-jp, Phase 02.
type SubtitlesHandler struct {
	aggregator *service.SubsAggregator
	log        *logger.Logger
}

// NewSubtitlesHandler wires the aggregator into a chi-compatible handler.
func NewSubtitlesHandler(agg *service.SubsAggregator, log *logger.Logger) *SubtitlesHandler {
	return &SubtitlesHandler{aggregator: agg, log: log}
}

// Get — GET /api/anime/{animeId}/subtitles?lang=ja,en,ru&episode=N.
//
// `lang` defaults to ja,en,ru when omitted. `episode` is required.
func (h *SubtitlesHandler) Get(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episode, ok := parseEpisode(r.URL.Query().Get("episode"))
	if !ok {
		httputil.BadRequest(w, "episode must be a positive integer")
		return
	}

	langsRaw := r.URL.Query().Get("lang")
	langs := []string{"ja", "en", "ru"}
	if langsRaw != "" {
		langs = splitTrimLower(langsRaw)
	}

	h.respond(w, r, animeID, episode, langs)
}

// GetAll — GET /api/anime/{animeId}/subtitles/all?episode=N.
// Returns every track regardless of language.
func (h *SubtitlesHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}
	episode, ok := parseEpisode(r.URL.Query().Get("episode"))
	if !ok {
		httputil.BadRequest(w, "episode must be a positive integer")
		return
	}
	h.respond(w, r, animeID, episode, nil)
}

func (h *SubtitlesHandler) respond(w http.ResponseWriter, r *http.Request, animeID string, episode int, langs []string) {
	resp, err := h.aggregator.FetchAll(r.Context(), animeID, episode, langs)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if len(resp.ProvidersDown) > 0 {
		w.Header().Set("X-Subtitle-Providers-Down", strings.Join(resp.ProvidersDown, ","))
	}
	httputil.OK(w, resp)
}

func parseEpisode(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func splitTrimLower(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// GetOpenSubtitlesFile — GET /api/anime/{animeId}/subtitles/opensubtitles/file/{fileID}.
//
// Resolves a numeric OpenSubtitles file_id to the actual subtitle text,
// spending one download quota unit on a cache miss. Returns text/plain so the
// frontend SubtitleOverlay can parse SRT/ASS/VTT directly. Quota exhaustion is
// surfaced as 429 (clear message, not a silent failure); an unconfigured or
// rejected key as 503.
func (h *SubtitlesHandler) GetOpenSubtitlesFile(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.Atoi(chi.URLParam(r, "fileID"))
	if err != nil || fileID <= 0 {
		httputil.BadRequest(w, "fileID must be a positive integer")
		return
	}

	body, _, err := h.aggregator.ResolveOpenSubtitlesFile(r.Context(), fileID)
	if err != nil {
		switch {
		case errors.Is(err, opensubtitles.ErrRateLimited):
			// CodeRateLimited maps to HTTP 429 (libs/errors codeToHTTPStatus).
			httputil.Error(w, liberrors.New(liberrors.CodeRateLimited,
				"OpenSubtitles daily download limit reached — try again later or pick a different subtitle."))
		case errors.Is(err, opensubtitles.ErrUnauthorized):
			httputil.Error(w, liberrors.ServiceUnavailable("OpenSubtitles is currently unavailable."))
		default:
			h.log.Errorw("opensubtitles file resolve failed", "file_id", fileID, "error", err)
			httputil.Error(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
