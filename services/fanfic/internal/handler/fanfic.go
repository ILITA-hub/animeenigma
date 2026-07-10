package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/pagination"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/service"
	"github.com/go-chi/chi/v5"
)

// generator is the subset of service.Generator this handler depends on. The
// emit parameter MUST be typed as the named service.Emit (not an unnamed
// func(string, any) error literal): Go's interface satisfaction requires
// identical parameter types, and a defined type is never identical to an
// unnamed type with the same underlying signature, so *service.Generator
// would fail to satisfy this interface if the literal type were used here.
type generator interface {
	Generate(ctx context.Context, userID string, req domain.GenerateRequest, emit service.Emit) error
	Continue(ctx context.Context, userID, id string, emit service.Emit) error
}

// libraryStore is the subset of repo.Repository this handler depends on.
type libraryStore interface {
	List(ctx context.Context, userID string, limit, offset int) ([]domain.Fanfic, int64, error)
	Get(ctx context.Context, userID, id string) (*domain.Fanfic, error)
	SoftDelete(ctx context.Context, userID, id string) error
}

type Handler struct {
	gen  generator
	repo libraryStore
	log  *logger.Logger
}

func NewHandler(gen generator, repo libraryStore, log *logger.Logger) *Handler {
	return &Handler{gen: gen, repo: repo, log: log}
}

// Generate streams a fanfic as SSE and persists it on completion.
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}
	var req domain.GenerateRequest
	if err := httputil.BindAndValidate(r, &req); err != nil {
		httputil.BadRequest(w, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // belt-and-suspenders for any buffering proxy
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	emit := func(event string, data any) error {
		payload, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
			return err
		}
		return rc.Flush()
	}

	// Detach from the request context so a client disconnect does NOT abort
	// server-side accumulation + persistence (spec §4).
	ctx := context.WithoutCancel(r.Context())
	if err := h.gen.Generate(ctx, userID, req, emit); err != nil && h.log != nil {
		h.log.Warnw("fanfic generation ended with error", "user_id", userID, "error", err)
	}
}

// Continue streams the next part of an existing fanfic as SSE and appends it
// on completion. Ownership + complete-status are checked BEFORE switching to
// SSE so a rejected request returns a real 404/409 (not an SSE error frame).
func (h *Handler) Continue(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}
	id := chi.URLParam(r, "id")

	f, err := h.repo.Get(r.Context(), userID, id)
	if err != nil {
		httputil.Error(w, err) // owner-scoped NotFound -> 404
		return
	}
	if f.Status != domain.StatusComplete {
		httputil.Error(w, liberrors.New(liberrors.CodeConflict, "fanfic is not complete"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	emit := func(event string, data any) error {
		payload, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
			return err
		}
		return rc.Flush()
	}

	// Detach from the request context so a client disconnect does NOT abort
	// server-side accumulation + persistence (spec §4), same as Generate.
	ctx := context.WithoutCancel(r.Context())
	if err := h.gen.Continue(ctx, userID, id, emit); err != nil && h.log != nil {
		h.log.Warnw("fanfic continue ended with error", "user_id", userID, "fanfic_id", id, "error", err)
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	page := pagination.ParseIntParam(r.URL.Query().Get("page"), 1)
	limit := pagination.ParseIntParam(r.URL.Query().Get("limit"), 20)
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if page < 1 {
		page = 1
	}
	items, total, err := h.repo.List(r.Context(), userID, limit, (page-1)*limit)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "limit": limit})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	f, err := h.repo.Get(r.Context(), userID, chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, f)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if err := h.repo.SoftDelete(r.Context(), userID, chi.URLParam(r, "id")); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.NoContent(w)
}

func (h *Handler) Tags(w http.ResponseWriter, _ *http.Request) {
	httputil.OK(w, domain.CuratedTags)
}
