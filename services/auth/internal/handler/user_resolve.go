package handler

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"gorm.io/gorm"
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

// resolvedUser is the response shape for a successful resolve. Role is the
// user's canonical role ("user"/"admin") — consumers like the policy admin's
// access-check preview need it to evaluate a specific user's real access
// (role + per-user allow/deny overrides), not a hand-picked hypothetical role.
type resolvedUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	PublicID   string `json:"public_id"`
	TelegramID *int64 `json:"telegram_id,omitempty"`
	Role       string `json:"role"`
}

// Resolve turns any of {UUID, username, public_id, telegram_id} into the
// canonical user. 400 when q is empty, 404 when no user matches, 500 when a
// repo call fails for a reason other than "not found" (e.g. DB outage).
func (h *UserResolveHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httputil.BadRequest(w, "q is required")
		return
	}

	u, err := h.lookup(r.Context(), q)
	if err != nil {
		h.log.Errorw("user resolve lookup failed", "q", q, "error", err)
		httputil.Error(w, liberrors.Internal("failed to resolve user"))
		return
	}
	if u == nil {
		httputil.NotFound(w, "user")
		return
	}

	httputil.OK(w, resolvedUser{
		ID:         u.ID,
		Username:   u.Username,
		PublicID:   u.PublicID,
		TelegramID: u.TelegramID,
		Role:       string(u.Role),
	})
}

// isNotFoundErr reports whether err represents "no such record" as opposed to
// a real backend failure. Repo getters signal not-found either via a wrapped
// gorm.ErrRecordNotFound or (GetByUsername/GetByID/GetByPublicID) a
// *liberrors.AppError with Code == NotFound; any other non-nil error is a
// genuine failure (e.g. DB outage) and must not be swallowed as a 404.
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	if appErr, ok := liberrors.IsAppError(err); ok && appErr.Code == liberrors.CodeNotFound {
		return true
	}
	return false
}

// lookup tries, in order: UUID (by id), all-digits (by telegram_id), then
// username, then public_id. A "not found" from any strategy falls through to
// the next one; a real repo error short-circuits immediately so Resolve can
// return 500 instead of a misleading 404.
func (h *UserResolveHandler) lookup(ctx context.Context, q string) (*domain.User, error) {
	if resolveUUIDRe.MatchString(q) {
		u, err := h.repo.GetByID(ctx, q)
		if err == nil && u != nil {
			return u, nil
		}
		if err != nil && !isNotFoundErr(err) {
			return nil, err
		}
	}
	// NOTE: a digit-only username could in principle collide with another
	// user's telegram_id, but telegram_id is tried first by design here
	// (admin-only tool, acceptable trade-off).
	if resolveDigitsRe.MatchString(q) {
		if n, perr := strconv.ParseInt(q, 10, 64); perr == nil {
			u, err := h.repo.GetByTelegramID(ctx, n)
			if err == nil && u != nil {
				return u, nil
			}
			if err != nil && !isNotFoundErr(err) {
				return nil, err
			}
		}
	}
	u, err := h.repo.GetByUsername(ctx, q)
	if err == nil && u != nil {
		return u, nil
	}
	if err != nil && !isNotFoundErr(err) {
		return nil, err
	}
	u, err = h.repo.GetByPublicID(ctx, q)
	if err == nil && u != nil {
		return u, nil
	}
	if err != nil && !isNotFoundErr(err) {
		return nil, err
	}
	return nil, nil
}
