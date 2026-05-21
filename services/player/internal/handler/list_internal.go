package handler

// Workstream hero-spotlight v1.0 Phase 3 — internal player endpoint.
//
// GET /internal/users/{user_id}/list?status=watching,planned,postponed
//
// Returns a JSON object `{ "items": [InternalListItem...] }` describing
// each anime in the user's list whose status is in the (allow-listed)
// CSV `?status=` filter, joined with animes (name / name_ru / poster_url /
// episodes_aired / episodes_count) AND the user's furthest-reached episode
// from watch_progress (0 when no progress row exists).
//
// Trust boundary:
//   - Mounted OUTSIDE /api with no AuthMiddleware. The route is reachable
//     only from within the docker network because the gateway does NOT
//     proxy /internal/* (defense-in-depth covered by Plan 04's gateway
//     router test).
//   - The path param {user_id} is taken verbatim — no UUID validation —
//     because the caller is the catalog spotlight resolver passing a
//     JWT-derived user_id. No untrusted user input crosses the gateway
//     boundary onto this surface.
//
// Response shape divergence: we encode `{"items": [...]}` directly via
// json.NewEncoder rather than going through libs/httputil.OK. This is
// DELIBERATE — the catalog spotlight `player_client.go` consumer parses
// the bare shape and `httputil.OK` would wrap the items inside a
// `{"data": ...}` envelope. Same divergence pattern Phase 1's
// /home/spotlight handler uses (divergence-3 discipline).

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

// listInternalService is the narrow surface InternalListHandler needs.
// Declared in the handler package so tests can substitute a fake without
// pulling the heavyweight *service.ListService construction graph.
// Production wires *service.ListService — it satisfies this interface by
// structural typing.
type listInternalService interface {
	GetUserListByStatusesWithProgress(ctx context.Context, userID string, statuses []string) ([]domain.InternalListItem, error)
}

// InternalListHandler implements GET /internal/users/{user_id}/list.
type InternalListHandler struct {
	svc listInternalService
	log *logger.Logger
}

// NewInternalListHandler wires the handler with a production *service.ListService.
// Use NewInternalListHandlerFromService for test injection of fakes/stubs.
func NewInternalListHandler(svc *service.ListService, log *logger.Logger) *InternalListHandler {
	return &InternalListHandler{svc: svc, log: log}
}

// NewInternalListHandlerFromService is the test-friendly constructor — accepts
// anything that satisfies listInternalService (the production *service.ListService
// AND handwritten fakes/stubs). Exported so the transport package's router
// tests can wire a stub without bringing the entire service-construction graph
// into the transport test binary.
func NewInternalListHandlerFromService(svc listInternalService, log *logger.Logger) *InternalListHandler {
	return &InternalListHandler{svc: svc, log: log}
}

// allowedStatusFilters bounds the values the spotlight aggregator may pass
// in the ?status= CSV. Unknown values are dropped silently — the resolver
// has no business asking for non-spotlight statuses, and rejecting hard
// would couple the handler to the resolver's exact roster.
var allowedStatusFilters = map[string]bool{
	"watching":  true,
	"planned":   true,
	"postponed": true,
}

// ListByStatuses handles GET /internal/users/{user_id}/list.
func (h *InternalListHandler) ListByStatuses(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		// Empty user_id is a programming error from the caller — surface a
		// 400 so the bug is visible in tests + production logs. Plain JSON
		// (no httputil envelope) — the consumer parses the bare shape.
		writeInternalError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	statuses := parseStatusFilter(r.URL.Query().Get("status"))

	items, err := h.svc.GetUserListByStatusesWithProgress(r.Context(), userID, statuses)
	if err != nil {
		if h.log != nil {
			h.log.Errorw("internal_list.query_failed",
				"user_id", userID,
				"statuses", statuses,
				"error", err,
			)
		}
		writeInternalError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Defensive: never return JSON `null` for items. The catalog consumer
	// uses Go's stdlib JSON decoder which would surface `null` as a nil
	// slice; an empty slice is the correct sentinel for "no matches".
	if items == nil {
		items = []domain.InternalListItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
}

// parseStatusFilter normalises the ?status= CSV: trims whitespace per value,
// drops empties, and filters to the allowed-list. Returns an empty slice
// (NOT nil) when the input is empty or wholly invalid — the service-layer
// short-circuit will turn this into a fast 200 + empty items.
func parseStatusFilter(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		if !allowedStatusFilters[v] {
			continue
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// writeInternalError writes a bare-JSON error envelope. Distinct from
// libs/httputil.Error so the catalog `player_client.go` consumer sees a
// stable `{"error": "..."}` body it can log without parsing the libs/httputil
// AppError envelope.
func writeInternalError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

