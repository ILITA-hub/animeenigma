package handler

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

var resolveUUIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var resolveDigitsRe = regexp.MustCompile(`^[0-9]+$`)

// userResolveRepo is the minimal surface UserResolveHandler needs (satisfied
// by *repo.UserRepository — no new repo method added).
type userResolveRepo interface {
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetByPublicID(ctx context.Context, publicID string) (*domain.User, error)
	GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error)
}

// UserResolveHandler backs the canonical admin-only user-resolve endpoint:
// GET /api/admin/users/resolve?q=<identifier>, turning any of
// {UUID, username, public_id, telegram_id} into the canonical user record.
type UserResolveHandler struct {
	repo userResolveRepo
	log  *logger.Logger
}

func NewUserResolveHandler(repo userResolveRepo, log *logger.Logger) *UserResolveHandler {
	return &UserResolveHandler{repo: repo, log: log}
}

// resolvedUser is the response shape for a successful resolve.
type resolvedUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	PublicID   string `json:"public_id"`
	TelegramID *int64 `json:"telegram_id,omitempty"`
}

// Resolve turns any of {UUID, username, public_id, telegram_id} into the
// canonical user. 400 when q is empty, 404 when no user matches.
func (h *UserResolveHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httputil.BadRequest(w, "q is required")
		return
	}

	u := h.lookup(r.Context(), q)
	if u == nil {
		httputil.NotFound(w, "user")
		return
	}

	httputil.OK(w, resolvedUser{
		ID:         u.ID,
		Username:   u.Username,
		PublicID:   u.PublicID,
		TelegramID: u.TelegramID,
	})
}

// lookup tries, in order: UUID (by id), all-digits (by telegram_id), then
// username, then public_id. Repo getters vary in not-found behavior (some
// return a NotFound *AppError, GetByTelegramID returns nil,nil) — both are
// treated as "no match, try the next strategy".
func (h *UserResolveHandler) lookup(ctx context.Context, q string) *domain.User {
	if resolveUUIDRe.MatchString(q) {
		if u, err := h.repo.GetByID(ctx, q); err == nil && u != nil {
			return u
		}
	}
	if resolveDigitsRe.MatchString(q) {
		if n, err := strconv.ParseInt(q, 10, 64); err == nil {
			if u, err := h.repo.GetByTelegramID(ctx, n); err == nil && u != nil {
				return u
			}
		}
	}
	if u, err := h.repo.GetByUsername(ctx, q); err == nil && u != nil {
		return u
	}
	if u, err := h.repo.GetByPublicID(ctx, q); err == nil && u != nil {
		return u
	}
	return nil
}
