package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
)

// AdminFlagsHandler is the admin CRUD surface (JWT + admin gated at the router).
type AdminFlagsHandler struct {
	svc *service.PolicyService
	log *logger.Logger
}

func NewAdminFlagsHandler(svc *service.PolicyService, log *logger.Logger) *AdminFlagsHandler {
	return &AdminFlagsHandler{svc: svc, log: log}
}

type listFlagsResponse struct {
	Flags           []domain.FeatureFlag `json:"flags"`
	RouletteEnabled bool                 `json:"rouletteEnabled"`
}

func (h *AdminFlagsHandler) List(w http.ResponseWriter, r *http.Request) {
	flags, rouletteEnabled, err := h.svc.ListFlags(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, listFlagsResponse{Flags: flags, RouletteEnabled: rouletteEnabled})
}

type setFlagRequest struct {
	Roles      []string `json:"roles"`
	AllowUsers []string `json:"allowUsers"`
	DenyUsers  []string `json:"denyUsers"`
	Roulette   bool     `json:"roulette"`
	FailSafe   string   `json:"failSafe"`
	Label      string   `json:"label"`
}

func (h *AdminFlagsHandler) SetFlag(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var req setFlagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid JSON body")
		return
	}
	err := h.svc.SetFlag(r.Context(), key,
		domain.Audience{Roles: req.Roles, AllowUsers: req.AllowUsers, DenyUsers: req.DenyUsers},
		req.Roulette, req.FailSafe, req.Label)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"key": key})
}

type setRouletteRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *AdminFlagsHandler) SetRoulette(w http.ResponseWriter, r *http.Request) {
	var req setRouletteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid JSON body")
		return
	}
	if err := h.svc.SetRoulette(r.Context(), req.Enabled); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"enabled": req.Enabled})
}
