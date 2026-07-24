package handler

import (
	"net/http"
	"regexp"
	"strings"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/service"
	"github.com/go-chi/chi/v5"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isUUID returns true when s matches the standard UUID format.
func isUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// AdminHandler serves /api/gacha/admin/* — admin-gated content CRUD.
type AdminHandler struct {
	content *service.ContentService
	images  *service.ImageService
	log     *logger.Logger
}

func NewAdminHandler(content *service.ContentService, images *service.ImageService, log *logger.Logger) *AdminHandler {
	return &AdminHandler{content: content, images: images, log: log}
}

// ─── Cards ────────────────────────────────────────────────────────────────────

// CreateCard handles POST /api/gacha/admin/cards
func (h *AdminHandler) CreateCard(w http.ResponseWriter, r *http.Request) {
	var req service.CreateCardRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	card, err := h.content.CreateCard(r.Context(), req)
	if err != nil {
		h.log.Errorw("create card", "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, card)
}

// ListCards handles GET /api/gacha/admin/cards
func (h *AdminHandler) ListCards(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := repo.CardFilter{
		Rarity:  domain.Rarity(q.Get("rarity")),
		GroupID: q.Get("group_id"),
	}
	if enabled := q.Get("enabled"); enabled == "true" {
		t := true
		filter.Enabled = &t
	} else if enabled == "false" {
		f := false
		filter.Enabled = &f
	}

	cards, err := h.content.ListCards(r.Context(), filter)
	if err != nil {
		h.log.Errorw("list cards", "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, cards)
}

// GetCard handles GET /api/gacha/admin/cards/{id}
func (h *AdminHandler) GetCard(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	card, err := h.content.GetCard(r.Context(), id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, card)
}

// UpdateCard handles PATCH /api/gacha/admin/cards/{id}
func (h *AdminHandler) UpdateCard(w http.ResponseWriter, r *http.Request) {
	var req service.UpdateCardRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	req.ID = chi.URLParam(r, "id")
	if !isUUID(req.ID) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	card, err := h.content.UpdateCard(r.Context(), req)
	if err != nil {
		h.log.Errorw("update card", "id", req.ID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, card)
}

// DeleteCard handles DELETE /api/gacha/admin/cards/{id}
func (h *AdminHandler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	if err := h.content.DeleteCard(r.Context(), id); err != nil {
		h.log.Errorw("delete card", "id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"deleted": true})
}

// BulkUpdateCards handles PATCH /api/gacha/admin/cards/bulk — applies the
// present fields of `set` to every card in `ids` (partial semantics).
func (h *AdminHandler) BulkUpdateCards(w http.ResponseWriter, r *http.Request) {
	var req service.BulkUpdateCardsRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	for _, id := range req.IDs {
		if !isUUID(id) {
			httputil.Error(w, apperrors.InvalidInput("invalid id: "+id))
			return
		}
	}
	n, err := h.content.BulkUpdateCards(r.Context(), req)
	if err != nil {
		h.log.Errorw("bulk update cards", "count", len(req.IDs), "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]int64{"updated": n})
}

type bulkDeleteRequest struct {
	IDs []string `json:"ids"`
}

// BulkDeleteCards handles POST /api/gacha/admin/cards/bulk-delete.
func (h *AdminHandler) BulkDeleteCards(w http.ResponseWriter, r *http.Request) {
	var req bulkDeleteRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	for _, id := range req.IDs {
		if !isUUID(id) {
			httputil.Error(w, apperrors.InvalidInput("invalid id: "+id))
			return
		}
	}
	n, err := h.content.BulkDeleteCards(r.Context(), req.IDs)
	if err != nil {
		h.log.Errorw("bulk delete cards", "count", len(req.IDs), "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]int64{"deleted": n})
}

// ─── Groups ───────────────────────────────────────────────────────────────────

type groupNameRequest struct {
	Name string `json:"name"`
}

type groupCardsRequest struct {
	CardIDs []string `json:"card_ids"`
}

// CreateGroup handles POST /api/gacha/admin/groups
func (h *AdminHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req groupNameRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	g, err := h.content.CreateGroup(r.Context(), req.Name)
	if err != nil {
		h.log.Errorw("create group", "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, g)
}

// ListGroups handles GET /api/gacha/admin/groups
func (h *AdminHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.content.ListGroups(r.Context())
	if err != nil {
		h.log.Errorw("list groups", "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, groups)
}

// RenameGroup handles PATCH /api/gacha/admin/groups/{id}
func (h *AdminHandler) RenameGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	var req groupNameRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if err := h.content.RenameGroup(r.Context(), id, req.Name); err != nil {
		h.log.Errorw("rename group", "id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"updated": true})
}

// DeleteGroup handles DELETE /api/gacha/admin/groups/{id}
func (h *AdminHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	if err := h.content.DeleteGroup(r.Context(), id); err != nil {
		h.log.Errorw("delete group", "id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"deleted": true})
}

// AddCardsToGroup handles POST /api/gacha/admin/groups/{id}/cards
func (h *AdminHandler) AddCardsToGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	var req groupCardsRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if err := h.content.AddCardsToGroup(r.Context(), id, req.CardIDs); err != nil {
		h.log.Errorw("add cards to group", "group_id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"updated": true})
}

// RemoveCardFromGroup handles DELETE /api/gacha/admin/groups/{id}/cards/{cardId}
func (h *AdminHandler) RemoveCardFromGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cardID := chi.URLParam(r, "cardId")
	if !isUUID(id) || !isUUID(cardID) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	if err := h.content.RemoveCardFromGroup(r.Context(), id, cardID); err != nil {
		h.log.Errorw("remove card from group", "group_id", id, "card_id", cardID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"deleted": true})
}

// ─── Banners ──────────────────────────────────────────────────────────────────

type bannerCardsRequest struct {
	CardIDs []string `json:"card_ids"`
}

// CreateBanner handles POST /api/gacha/admin/banners
func (h *AdminHandler) CreateBanner(w http.ResponseWriter, r *http.Request) {
	var req service.CreateBannerRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	b, err := h.content.CreateBanner(r.Context(), req)
	if err != nil {
		h.log.Errorw("create banner", "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, b)
}

// ListBanners handles GET /api/gacha/admin/banners
func (h *AdminHandler) ListBanners(w http.ResponseWriter, r *http.Request) {
	banners, err := h.content.ListBanners(r.Context())
	if err != nil {
		h.log.Errorw("list banners", "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, banners)
}

// GetBanner handles GET /api/gacha/admin/banners/{id}
func (h *AdminHandler) GetBanner(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	b, err := h.content.GetBanner(r.Context(), id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	// Include card IDs in the response
	cardIDs, err := h.content.BannerCardIDs(r.Context(), id)
	if err != nil {
		h.log.Errorw("get banner card ids", "banner_id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]interface{}{
		"banner":   b,
		"card_ids": cardIDs,
	})
}

// UpdateBanner handles PATCH /api/gacha/admin/banners/{id}
func (h *AdminHandler) UpdateBanner(w http.ResponseWriter, r *http.Request) {
	var req service.UpdateBannerRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	req.ID = chi.URLParam(r, "id")
	if !isUUID(req.ID) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	b, err := h.content.UpdateBanner(r.Context(), req)
	if err != nil {
		h.log.Errorw("update banner", "id", req.ID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, b)
}

// DeleteBanner handles DELETE /api/gacha/admin/banners/{id}
func (h *AdminHandler) DeleteBanner(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	if err := h.content.DeleteBanner(r.Context(), id); err != nil {
		h.log.Errorw("delete banner", "id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"deleted": true})
}

// SetBannerCards handles PUT /api/gacha/admin/banners/{id}/cards
func (h *AdminHandler) SetBannerCards(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	var req bannerCardsRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if err := h.content.SetBannerCards(r.Context(), id, req.CardIDs); err != nil {
		h.log.Errorw("set banner cards", "banner_id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"updated": true})
}

// AddBannerCards handles POST /api/gacha/admin/banners/{id}/cards
func (h *AdminHandler) AddBannerCards(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	var req bannerCardsRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if err := h.content.AddBannerCards(r.Context(), id, req.CardIDs); err != nil {
		h.log.Errorw("add banner cards", "banner_id", id, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"updated": true})
}

// AddGroupCardsToBanner handles POST /api/gacha/admin/banners/{id}/groups/{groupId}
func (h *AdminHandler) AddGroupCardsToBanner(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	groupID := chi.URLParam(r, "groupId")
	if !isUUID(id) || !isUUID(groupID) {
		httputil.Error(w, apperrors.InvalidInput("invalid id"))
		return
	}
	if err := h.content.AddGroupCardsToBanner(r.Context(), id, groupID); err != nil {
		h.log.Errorw("add group cards to banner", "banner_id", id, "group_id", groupID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"updated": true})
}

// ─── Upload ───────────────────────────────────────────────────────────────────

type uploadURLRequest struct {
	ImageURL string `json:"image_url"`
	Kind     string `json:"kind"`
}

// Upload handles POST /api/gacha/admin/upload
// Supports multipart/form-data (file upload) and JSON (URL ingestion).
func (h *AdminHandler) Upload(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	var (
		key string
		err error
	)

	if isMultipart(ct) {
		if err = r.ParseMultipartForm(12 << 20); err != nil {
			httputil.BadRequest(w, "failed to parse multipart form")
			return
		}
		file, header, ferr := r.FormFile("file")
		if ferr != nil {
			httputil.BadRequest(w, "missing file field in form")
			return
		}
		defer file.Close()

		kind := r.FormValue("kind")
		if kind == "" {
			kind = "cards"
		}
		key, err = h.images.IngestUpload(r.Context(), file, header.Filename, header.Header.Get("Content-Type"), kind)
	} else {
		var req uploadURLRequest
		if bindErr := httputil.Bind(r, &req); bindErr != nil {
			httputil.Error(w, bindErr)
			return
		}
		if req.Kind == "" {
			req.Kind = "cards"
		}
		key, err = h.images.IngestFromURL(r.Context(), req.ImageURL, req.Kind)
	}

	if err != nil {
		h.log.Errorw("upload image", "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{
		"image_path": key,
		"image_url":  "/api/gacha/images/" + key,
	})
}

// isMultipart returns true when the Content-Type starts with multipart/form-data
// (case-insensitive per RFC 7231 §3.1.1.1).
func isMultipart(ct string) bool {
	return strings.HasPrefix(strings.ToLower(ct), "multipart/form-data")
}
