// Package handler implements the HTTP handlers for the scraper service.
//
// Phase 16 plan 05 swaps the Phase 15 not-yet-implemented stubs for live
// orchestrator-backed handlers:
//
//   - GetEpisodes / GetServers / GetStream call the orchestrator after a
//     FindID resolution from the incoming `mal_id` query parameter. Success
//     returns 200 with the result wrapped in {success, data:{<list>,
//     meta:{tried:[...]}}}. ErrNotFound → 404; ErrProviderDown /
//     ErrExtractFailed → 502; unexpected errors → 500. Every error body
//     STILL includes meta.tried so SCRAPER-NF-05 (provider-chain
//     attribution in every response) holds.
//   - GetHealth: unchanged — 200 with the orchestrator's live HealthSnapshot.
//   - When zero providers are registered we short-circuit with 503
//     NO_PROVIDERS and meta.tried=[]. (Phase 15's "not-yet-implemented"
//     503 body is intentionally retired here — the catalog passthrough
//     still works because the catalog forwards status + body verbatim.)
//
// The handler's `meta.tried` array is derived from
// orchestrator.OrderedProviderNames(prefer) — the same iteration order the
// orchestrator would use for failover — so the response surface lines up
// with what actually happened upstream.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
)

// ScraperHandler is the HTTP handler for /scraper/* routes.
type ScraperHandler struct {
	svc *service.Orchestrator
	log *logger.Logger
}

// NewScraperHandler builds a ScraperHandler.
func NewScraperHandler(svc *service.Orchestrator, log *logger.Logger) *ScraperHandler {
	return &ScraperHandler{svc: svc, log: log}
}

// errorCode constants surface in `error.code` for every non-2xx response.
// They mirror the codes the frontend's ReportButton + locale strings
// recognize (Plan 16-04 SUMMARY locale keys).
const (
	codeInvalidInput  = "INVALID_INPUT"
	codeNoProviders   = "NO_PROVIDERS"
	codeNotFound      = "NOT_FOUND"
	codeProviderDown  = "PROVIDER_DOWN"
	codeExtractFailed = "EXTRACT_FAILED"
	codeInternal      = "INTERNAL"
)

// queryParams pulls the standard set of query-string inputs the scraper
// handlers care about. Whitespace is trimmed because some catalog/frontend
// callers append trailing spaces accidentally.
type queryParams struct {
	malID    string
	episode  string
	server   string
	category string
	prefer   string
}

func parseQuery(r *http.Request) queryParams {
	q := r.URL.Query()
	return queryParams{
		malID:    strings.TrimSpace(q.Get("mal_id")),
		episode:  strings.TrimSpace(q.Get("episode")),
		server:   strings.TrimSpace(q.Get("server")),
		category: strings.TrimSpace(q.Get("category")),
		prefer:   strings.TrimSpace(q.Get("prefer")),
	}
}

// GetEpisodes handles GET /scraper/episodes?mal_id=...&prefer=....
//
// Resolves the MAL ID to a provider-internal ID via the orchestrator's
// FindID chain, then calls ListEpisodes. Returns 200 with episodes +
// meta.tried on success; 404/502/503 with meta.tried on error.
func (h *ScraperHandler) GetEpisodes(w http.ResponseWriter, r *http.Request) {
	qp := parseQuery(r)
	tried := h.svc.OrderedProviderNames(qp.prefer)

	if len(tried) == 0 {
		h.writeError(w, http.StatusServiceUnavailable, codeNoProviders, "no providers available", tried)
		return
	}
	if qp.malID == "" {
		h.writeError(w, http.StatusBadRequest, codeInvalidInput, "mal_id is required", tried)
		return
	}

	providerID, err := h.resolveProviderID(r.Context(), qp.malID, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}

	eps, err := h.svc.ListEpisodes(r.Context(), providerID, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}
	if eps == nil {
		eps = []domain.Episode{}
	}
	h.writeSuccess(w, map[string]any{"episodes": eps}, tried)
}

// GetServers handles GET /scraper/servers?mal_id=...&episode=...&prefer=....
func (h *ScraperHandler) GetServers(w http.ResponseWriter, r *http.Request) {
	qp := parseQuery(r)
	tried := h.svc.OrderedProviderNames(qp.prefer)

	if len(tried) == 0 {
		h.writeError(w, http.StatusServiceUnavailable, codeNoProviders, "no providers available", tried)
		return
	}
	if qp.malID == "" {
		h.writeError(w, http.StatusBadRequest, codeInvalidInput, "mal_id is required", tried)
		return
	}
	if qp.episode == "" {
		h.writeError(w, http.StatusBadRequest, codeInvalidInput, "episode is required", tried)
		return
	}

	providerID, err := h.resolveProviderID(r.Context(), qp.malID, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}

	srvs, err := h.svc.ListServers(r.Context(), providerID, qp.episode, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}
	if srvs == nil {
		srvs = []domain.Server{}
	}
	h.writeSuccess(w, map[string]any{"servers": srvs}, tried)
}

// GetStream handles GET /scraper/stream?mal_id=...&episode=...&server=...&category=...&prefer=....
func (h *ScraperHandler) GetStream(w http.ResponseWriter, r *http.Request) {
	qp := parseQuery(r)
	tried := h.svc.OrderedProviderNames(qp.prefer)

	if len(tried) == 0 {
		h.writeError(w, http.StatusServiceUnavailable, codeNoProviders, "no providers available", tried)
		return
	}
	if qp.malID == "" {
		h.writeError(w, http.StatusBadRequest, codeInvalidInput, "mal_id is required", tried)
		return
	}
	if qp.episode == "" {
		h.writeError(w, http.StatusBadRequest, codeInvalidInput, "episode is required", tried)
		return
	}
	if qp.server == "" {
		h.writeError(w, http.StatusBadRequest, codeInvalidInput, "server is required", tried)
		return
	}

	cat := domain.Category(qp.category)
	if cat == "" {
		cat = domain.CategorySub
	}

	providerID, err := h.resolveProviderID(r.Context(), qp.malID, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}

	stream, err := h.svc.GetStream(r.Context(), providerID, qp.episode, qp.server, cat, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}
	h.writeSuccess(w, map[string]any{"stream": stream}, tried)
}

// GetHealth handles GET /scraper/health. Returns the orchestrator's live
// HealthSnapshot keyed by provider name.
func (h *ScraperHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	snap := h.svc.HealthSnapshot(r.Context())
	httputil.OK(w, map[string]any{"providers": snap})
}

// resolveProviderID converts an incoming mal_id query value into a
// provider-internal ID via the orchestrator's FindID chain. The catalog
// already mapped catalog-UUID → MAL/Shikimori ID before forwarding, so we
// pass the value as ShikimoriID (project memory: Shikimori IDs == MAL IDs).
func (h *ScraperHandler) resolveProviderID(ctx context.Context, malID, prefer string) (string, error) {
	ref := domain.AnimeRef{ShikimoriID: malID}
	return h.svc.FindID(ctx, ref, prefer)
}

// writeSuccess writes 200 with the standard envelope {success:true,
// data:{<provided fields>, meta:{tried:[...]}}}. The meta key lives INSIDE
// data so the frontend's existing axios response handler (which already
// peels `data` off the envelope) sees meta as a sibling of the business
// payload — convenient for ReportButton + diagnostics consumers.
func (h *ScraperHandler) writeSuccess(w http.ResponseWriter, data map[string]any, tried []string) {
	if tried == nil {
		tried = []string{}
	}
	data["meta"] = map[string]any{"tried": tried}
	httputil.OK(w, data)
}

// writeError writes the error envelope {success:false,
// error:{code,message}, meta:{tried:[...]}}. We bypass httputil.Error
// because it does not surface the meta field and SCRAPER-NF-05 demands
// meta.tried on every response including failures.
func (h *ScraperHandler) writeError(w http.ResponseWriter, status int, code, msg string, tried []string) {
	if tried == nil {
		tried = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{
		"success": false,
		"error":   map[string]any{"code": code, "message": msg},
		"meta":    map[string]any{"tried": tried},
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log := h.log
		if log == nil {
			log = logger.Default()
		}
		log.Errorw("scraper handler: encode error body", "error", err)
	}
}

// writeOrchestratorError classifies a domain error and writes the
// appropriate status code with the meta.tried envelope intact.
//
//	context.Canceled / DeadlineExceeded → 499 (per Nginx convention)
//	ErrNotFound      → 404 NOT_FOUND
//	ErrProviderDown  → 502 PROVIDER_DOWN  (upstream unavailable)
//	ErrExtractFailed → 502 EXTRACT_FAILED (upstream shape change)
//	anything else    → 500 INTERNAL
func (h *ScraperHandler) writeOrchestratorError(w http.ResponseWriter, err error, tried []string) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		h.writeError(w, 499, codeInternal, "request canceled", tried)
	case errors.Is(err, domain.ErrNotFound):
		h.writeError(w, http.StatusNotFound, codeNotFound, err.Error(), tried)
	case errors.Is(err, domain.ErrProviderDown):
		h.writeError(w, http.StatusBadGateway, codeProviderDown, err.Error(), tried)
	case errors.Is(err, domain.ErrExtractFailed):
		h.writeError(w, http.StatusBadGateway, codeExtractFailed, err.Error(), tried)
	default:
		h.writeError(w, http.StatusInternalServerError, codeInternal, err.Error(), tried)
	}
}
