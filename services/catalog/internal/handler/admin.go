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

// HideAnime hides an anime from the public site
func (h *AdminHandler) HideAnime(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	if err := h.catalogService.SetAnimeHidden(r.Context(), animeID, true); err != nil {
		httputil.Error(w, err)
		return
	}

	h.log.Infow("anime hidden", "anime_id", animeID)
	httputil.OK(w, map[string]bool{"hidden": true})
}

// UnhideAnime unhides an anime, making it visible on the public site
func (h *AdminHandler) UnhideAnime(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	if err := h.catalogService.SetAnimeHidden(r.Context(), animeID, false); err != nil {
		httputil.Error(w, err)
		return
	}

	h.log.Infow("anime unhidden", "anime_id", animeID)
	httputil.OK(w, map[string]bool{"hidden": false})
}

// GetHiddenAnime returns all hidden anime
func (h *AdminHandler) GetHiddenAnime(w http.ResponseWriter, r *http.Request) {
	animes, err := h.catalogService.GetHiddenAnime(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if animes == nil {
		animes = []*domain.Anime{}
	}

	httputil.OK(w, animes)
}

// LinkMALID links a MAL ID to an existing anime
func (h *AdminHandler) LinkMALID(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	var req struct {
		MALID string `json:"mal_id"`
	}
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if req.MALID == "" {
		httputil.BadRequest(w, "mal_id is required")
		return
	}

	if err := h.catalogService.LinkMALID(r.Context(), animeID, req.MALID); err != nil {
		httputil.Error(w, err)
		return
	}

	h.log.Infow("mal_id linked", "anime_id", animeID, "mal_id", req.MALID)
	httputil.OK(w, map[string]string{"mal_id": req.MALID})
}

// UpdateShikimoriID updates the Shikimori ID for an anime
func (h *AdminHandler) UpdateShikimoriID(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	var req struct {
		ShikimoriID string `json:"shikimori_id"`
	}
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	if req.ShikimoriID == "" {
		httputil.BadRequest(w, "shikimori_id is required")
		return
	}

	if err := h.catalogService.UpdateShikimoriID(r.Context(), animeID, req.ShikimoriID); err != nil {
		httputil.Error(w, err)
		return
	}

	h.log.Infow("shikimori_id updated", "anime_id", animeID, "shikimori_id", req.ShikimoriID)
	httputil.OK(w, map[string]string{"shikimori_id": req.ShikimoriID})
}
