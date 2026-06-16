package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

// CharacterHandler serves anime-character endpoints.
type CharacterHandler struct {
	svc *service.CharacterService
	log *logger.Logger
}

func NewCharacterHandler(svc *service.CharacterService, log *logger.Logger) *CharacterHandler {
	return &CharacterHandler{svc: svc, log: log}
}

// GetAnimeCharacters handles GET /api/anime/{animeId}/characters.
func (h *CharacterHandler) GetAnimeCharacters(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}
	list, err := h.svc.GetAnimeCharacters(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, list)
}

// GetCharacter handles GET /api/characters/{characterId} (Shikimori id).
func (h *CharacterHandler) GetCharacter(w http.ResponseWriter, r *http.Request) {
	characterID := chi.URLParam(r, "characterId")
	if characterID == "" {
		httputil.BadRequest(w, "character ID is required")
		return
	}
	ch, err := h.svc.GetCharacter(r.Context(), characterID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, ch)
}
