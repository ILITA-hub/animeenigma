package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/service"
)

// SearchHandler serves GET /api/library/search, the admin-only search
// endpoint that fans out across Nyaa + AnimeTosho via the
// SearchAggregator. Auth is enforced by the gateway's
// JWTValidationMiddleware + AdminRoleMiddleware on the /api/library/*
// prefix (services/gateway/internal/transport/router.go); the library
// service itself trusts what the gateway forwards.
type SearchHandler struct {
	agg *service.SearchAggregator
	log *logger.Logger
}

// NewSearchHandler constructs a SearchHandler with no dependencies
// beyond the aggregator + logger.
func NewSearchHandler(agg *service.SearchAggregator, log *logger.Logger) *SearchHandler {
	return &SearchHandler{agg: agg, log: log}
}

// Search reads `q`, `mal_id`, `limit` from the query string and returns
// {releases, providers_down} wrapped in the httputil envelope. At least
// one of {q, mal_id} must be present. `mal_id` parse errors → 400; bad
// `limit` falls back to the aggregator's default of 50 (clamped 1..200
// inside the aggregator).
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	malIDStr := r.URL.Query().Get("mal_id")
	limitStr := r.URL.Query().Get("limit")

	var malID int
	if malIDStr != "" {
		parsed, err := strconv.Atoi(malIDStr)
		if err != nil || parsed < 0 {
			httputil.BadRequest(w, "mal_id must be a non-negative integer")
			return
		}
		malID = parsed
	}

	if q == "" && malID == 0 {
		httputil.BadRequest(w, "q or mal_id required")
		return
	}

	// limit parse errors are tolerated — the aggregator clamps anyway.
	limit, _ := strconv.Atoi(limitStr)

	result, err := h.agg.FetchAll(r.Context(), service.SearchParams{
		Query: q,
		MALID: malID,
		Limit: limit,
	})
	if err != nil {
		// Both providers down → 502 (ExternalAPI maps to 502 via the
		// AppError table). Log structured for ops visibility.
		h.log.Errorw("library search: both providers failed",
			"q", q, "mal_id", malID, "error", err,
		)
		httputil.Error(w, errors.ExternalAPI("library_search", err))
		return
	}

	// Always include providers_down + releases as non-nil slices so the
	// client doesn't have to special-case `null`. Importing the domain
	// package here would be circular-free but unnecessary — the
	// aggregator's Releases field is already []domain.Release, just
	// possibly nil.
	if result.ProvidersDown == nil {
		result.ProvidersDown = []string{}
	}
	releases := result.Releases
	if releases == nil {
		releases = make([]domain.Release, 0)
	}

	httputil.OK(w, map[string]any{
		"releases":       releases,
		"providers_down": result.ProvidersDown,
	})
}
