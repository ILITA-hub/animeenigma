package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)

type AdminHandler struct {
	catalogService *service.CatalogService
	log            *logger.Logger
}

func NewAdminHandler(catalogService *service.CatalogService, log *logger.Logger) *AdminHandler {
	return &AdminHandler{
		catalogService: catalogService,
		log:            log,
	}
}

// CreateAnime handles manual anime creation
func (h *AdminHandler) CreateAnime(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateAnimeRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if req.Name == "" {
		httputil.BadRequest(w, "name is required")
		return
	}

	anime, err := h.catalogService.CreateAnime(r.Context(), &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.Created(w, anime)
}

// AddVideoSource handles adding a video source to an anime
func (h *AdminHandler) AddVideoSource(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	var req domain.AddVideoRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	video, err := h.catalogService.AddVideoSource(r.Context(), animeID, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.Created(w, video)
}

// DeleteVideo handles deleting a video source
func (h *AdminHandler) DeleteVideo(w http.ResponseWriter, r *http.Request) {
	videoID := chi.URLParam(r, "videoId")
	if videoID == "" {
		httputil.BadRequest(w, "video ID is required")
		return
	}

	if err := h.catalogService.DeleteVideo(r.Context(), videoID); err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.NoContent(w)
}

// SyncFromShikimori triggers a sync for a specific anime from Shikimori
func (h *AdminHandler) SyncFromShikimori(w http.ResponseWriter, r *http.Request) {
	shikimoriID := chi.URLParam(r, "shikimoriId")
	if shikimoriID == "" {
		httputil.BadRequest(w, "shikimori ID is required")
		return
	}

	anime, err := h.catalogService.GetAnimeByShikimoriID(r.Context(), shikimoriID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, anime)
}
