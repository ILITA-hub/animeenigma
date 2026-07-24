package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/go-chi/chi/v5"
)

const (
	defaultUsersPageSize = 25
	maxUsersPageSize     = 100
)

// adminUsersRepo is the minimal repo surface AdminUsersHandler needs
// (satisfied by *repo.UserRepository).
type adminUsersRepo interface {
	ListUsers(ctx context.Context, query, role string, limit, offset int) ([]domain.User, int64, error)
	UpdateRole(ctx context.Context, id, role string) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
}

// AdminUsersHandler backs the admin-only user directory:
// GET /api/admin/users and PATCH /api/admin/users/{id}/role.
type AdminUsersHandler struct {
	repo adminUsersRepo
	log  *logger.Logger
}

func NewAdminUsersHandler(repo adminUsersRepo, log *logger.Logger) *AdminUsersHandler {
	return &AdminUsersHandler{repo: repo, log: log}
}

// adminUserView is the admin-only projection of a user — it deliberately
// includes role + telegram fields, unlike domain.PublicUser.
type adminUserView struct {
	ID                string    `json:"id"`
	Username          string    `json:"username"`
	PublicID          string    `json:"public_id"`
	Role              string    `json:"role"`
	TelegramID        *int64    `json:"telegram_id,omitempty"`
	TelegramUsername  *string   `json:"telegram_username,omitempty"`
	TelegramFirstName *string   `json:"telegram_first_name,omitempty"`
	Avatar            string    `json:"avatar,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func toAdminUserView(u *domain.User) adminUserView {
	return adminUserView{
		ID:                u.ID,
		Username:          u.Username,
		PublicID:          u.PublicID,
		Role:              string(u.Role),
		TelegramID:        u.TelegramID,
		TelegramUsername:  u.TelegramUsername,
		TelegramFirstName: u.TelegramFirstName,
		Avatar:            u.Avatar,
		CreatedAt:         u.CreatedAt,
	}
}

type adminUsersListResponse struct {
	Items    []adminUserView `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

// isAssignableRole reports whether role is one an admin may store on a user.
// guest is ephemeral (never a DB row) and is rejected.
func isAssignableRole(role string) bool {
	switch role {
	case string(authz.RoleUser), string(authz.RoleAdmin), string(authz.RoleLibrarian):
		return true
	}
	return false
}

func parsePositiveInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}

// List handles GET /api/admin/users?q=&role=&page=&page_size=.
func (h *AdminUsersHandler) List(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if role != "" && !isAssignableRole(role) {
		httputil.BadRequest(w, "invalid role filter")
		return
	}
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	pageSize := parsePositiveInt(r.URL.Query().Get("page_size"), defaultUsersPageSize)
	if pageSize > maxUsersPageSize {
		pageSize = maxUsersPageSize
	}
	offset := (page - 1) * pageSize

	users, total, err := h.repo.ListUsers(r.Context(), q, role, pageSize, offset)
	if err != nil {
		h.log.Errorw("admin list users failed", "q", q, "role", role, "error", err)
		httputil.Error(w, liberrors.Internal("failed to list users"))
		return
	}

	views := make([]adminUserView, 0, len(users))
	for i := range users {
		views = append(views, toAdminUserView(&users[i]))
	}
	httputil.OK(w, adminUsersListResponse{
		Items:    views,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// UpdateRole handles PATCH /api/admin/users/{id}/role with body {"role":"..."}.
func (h *AdminUsersHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := httputil.Bind(r, &body); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}
	role := strings.TrimSpace(body.Role)
	if !isAssignableRole(role) {
		httputil.BadRequest(w, "invalid role")
		return
	}
	// Self-lockout guard: an admin may not change their own role (prevents
	// accidentally demoting yourself out of admin).
	if id == authz.UserIDFromContext(r.Context()) {
		httputil.Error(w, liberrors.Forbidden("cannot change your own role"))
		return
	}
	if err := h.repo.UpdateRole(r.Context(), id, role); err != nil {
		if isNotFoundErr(err) {
			httputil.NotFound(w, "user")
			return
		}
		h.log.Errorw("admin update role failed", "user_id", id, "role", role, "error", err)
		httputil.Error(w, liberrors.Internal("failed to update role"))
		return
	}
	u, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		h.log.Errorw("admin update role reload failed", "user_id", id, "error", err)
		httputil.Error(w, liberrors.Internal("failed to load user"))
		return
	}
	httputil.OK(w, toAdminUserView(u))
}
