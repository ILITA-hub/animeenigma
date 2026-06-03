// Package handler — 18anime (18+) endpoints.
//
// The two /anime18/* endpoints (episodes/stream) are implemented on a
// dedicated Anime18EndpointsHandler that the public *CatalogHandler embeds.
// The split mirrors the scraper endpoints: it keeps the handler logic
// testable against a small anime18ServiceAPI interface without requiring the
// full *service.CatalogService dependency tree (which needs real GORM repos).
package handler

import (
	"context"

	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/go-chi/chi/v5"
)

// anime18ServiceAPI is the minimal interface the 18anime handlers need from
// the catalog service. *service.CatalogService satisfies it.
type anime18ServiceAPI interface {
	Get18AnimeEpisodes(ctx context.Context, animeID string) ([]domain.Anime18Episode, error)
	Get18AnimeStream(ctx context.Context, animeID, episodeSlug string) (*domain.Anime18Stream, error)
}

// Anime18EndpointsHandler holds the /anime18/* handler methods. It is embedded
// inside *CatalogHandler so the routes hang off the same chi mount.
type Anime18EndpointsHandler struct {
	anime18Svc anime18ServiceAPI
	log        *logger.Logger
}

// WireAnime18Endpoints sets the dependencies on an Anime18EndpointsHandler.
// Exposed so the constructor and route tests can wire a service/stub without
// an exported setter pair.
func WireAnime18Endpoints(h *Anime18EndpointsHandler, svc anime18ServiceAPI, log *logger.Logger) {
	h.anime18Svc = svc
	h.log = log
}

// GetAnime18Episodes handles GET /api/anime/{animeId}/anime18/episodes.
func (h *Anime18EndpointsHandler) GetAnime18Episodes(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodes, err := h.anime18Svc.Get18AnimeEpisodes(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, episodes)
}

// GetAnime18Stream handles GET /api/anime/{animeId}/anime18/stream?ep=<slug>.
// On total mirror failure the service returns a typed ServiceUnavailable error
// which httputil.Error renders as an explicit 503 — never a silent empty 200.
func (h *Anime18EndpointsHandler) GetAnime18Stream(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	ep := r.URL.Query().Get("ep")
	if ep == "" {
		httputil.BadRequest(w, "ep (episode slug) is required")
		return
	}

	stream, err := h.anime18Svc.Get18AnimeStream(r.Context(), animeID, ep)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, stream)
}
