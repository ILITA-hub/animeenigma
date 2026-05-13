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
	"regexp"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
)

// ScraperHandler is the HTTP handler for /scraper/* routes.
//
// The optional `cache` field (added in Phase 17 Plan 03) backs the admin
// debug endpoint /scraper/health/admin. The public GetHealth handler does
// NOT consult this cache — it returns the orchestrator's live HealthSnapshot
// for backward compatibility with the catalog forwarder. A nil cache is
// permitted so unit tests that exercise only the public surface can keep
// using the lighter newTestHandler harness.
//
// REVIEW.md WR-11: `now` is injectable so tests can lock the generated_at
// timestamp in admin responses. Production defaults to time.Now.
type ScraperHandler struct {
	svc   *service.Orchestrator
	cache *health.InMemoryHealthCache
	log   *logger.Logger
	now   func() time.Time
}

// NewScraperHandler builds a ScraperHandler. The cache argument may be nil
// for tests that do not exercise /scraper/health/admin; production callers
// MUST thread the same *InMemoryHealthCache that the probe runner writes to
// (see cmd/scraper-api/main.go).
func NewScraperHandler(svc *service.Orchestrator, cache *health.InMemoryHealthCache, log *logger.Logger) *ScraperHandler {
	return &ScraperHandler{svc: svc, cache: cache, log: log, now: time.Now}
}

// SetNow overrides the clock used for admin response timestamps. Test-only.
// WR-11: prefer this over patching globals.
func (h *ScraperHandler) SetNow(now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	h.now = now
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
	title    string
	episode  string
	server   string
	category string
	prefer   string
}

// maxTitleLength caps the `title` query-string parameter so an oversized
// title can't balloon log lines or fuzzy-match comparison cost. Real anime
// titles are well under 200 chars; 512 is generous.
const maxTitleLength = 512

// maxPreferLength caps the `prefer` query-string parameter at parse time so
// a malicious caller can't balloon log lines or response bodies via the
// `meta.tried` echo path. Provider names are short identifiers (e.g.
// "animepahe", "9anime") — 64 chars is generous. See REVIEW.md WR-01.
const maxPreferLength = 64

// preferAllowed is the regex defense-in-depth check for the `prefer` query
// param (REVIEW.md WR-09). Provider names are short identifiers — restrict
// to [a-z0-9_-]{1,64} so a value like "animepahe\n[FORGED_LOG_LINE]" never
// reaches a structured-log field. zap's JSON encoder escapes newlines so the
// impact would be bounded today, but the value would still appear in log
// queries.
//
// A non-matching value is silently coerced to empty string, matching the
// existing "unknown prefer silently ignored" contract.
var preferAllowed = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

func parseQuery(r *http.Request) queryParams {
	q := r.URL.Query()
	prefer := strings.TrimSpace(q.Get("prefer"))
	// REVIEW.md iter-2 WR-NEW-03: the regex's `{1,64}` quantifier
	// structurally enforces the maxPreferLength cap, so the previous
	// byte-truncation step (`prefer = prefer[:maxPreferLength]`) was
	// dead code for any non-ASCII input — the truncation could split a
	// UTF-8 codepoint, and the regex would then reject the orphan
	// continuation bytes anyway. Apply the regex first; the length cap
	// is encoded in the regex.
	//
	// Net contract:
	//   - prefer matches ^[a-z0-9_-]{1,64}$ → kept as-is (≤64 chars)
	//   - anything else → coerced to "" (silently rejected, matching
	//     the existing "unknown prefer silently ignored" behaviour)
	if !preferAllowed.MatchString(prefer) {
		prefer = ""
	}
	title := strings.TrimSpace(q.Get("title"))
	if len(title) > maxTitleLength {
		title = title[:maxTitleLength]
	}
	return queryParams{
		malID:    strings.TrimSpace(q.Get("mal_id")),
		title:    title,
		episode:  strings.TrimSpace(q.Get("episode")),
		server:   strings.TrimSpace(q.Get("server")),
		category: strings.TrimSpace(q.Get("category")),
		prefer:   prefer,
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

	providerID, err := h.resolveProviderID(r.Context(), qp.malID, qp.title, qp.prefer)
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
	// gated=false: GetEpisodes does not run the playability gate.
	h.writeSuccess(w, map[string]any{"episodes": eps}, tried, false)
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

	providerID, err := h.resolveProviderID(r.Context(), qp.malID, qp.title, qp.prefer)
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
	// gated=false: GetServers does not run the playability gate.
	h.writeSuccess(w, map[string]any{"servers": srvs}, tried, false)
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

	providerID, err := h.resolveProviderID(r.Context(), qp.malID, qp.title, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}

	stream, gated, err := h.svc.GetStreamGated(r.Context(), providerID, qp.episode, qp.server, cat, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}
	// Phase 21 SCRAPER-HEAL-04 / HEAL-07: gated is true on the cold path
	// (cache miss → priority iteration + streamprobe gate ran), false on
	// the warm path (cached winning serverID re-extracted directly) or
	// caller-pinned serverID. The FE reads data.meta.gated to decide whether
	// to render the three-phase loader's Phase 3.
	h.writeSuccess(w, map[string]any{"stream": stream}, tried, gated)
}

// GetHealth handles GET /scraper/health. Returns the orchestrator's live
// HealthSnapshot keyed by provider name.
func (h *ScraperHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	snap := h.svc.HealthSnapshot(r.Context())
	httputil.OK(w, map[string]any{"providers": snap})
}

// GetAdminHealth handles GET /scraper/health/admin. Returns the orchestrator's
// public HealthSnapshot alongside the in-memory cache's enriched AdminSnapshot
// (per-stage LastOK timestamps + truncated LastErr excerpts).
//
// Auth model (per Plan 17-03 D6): JWT + AdminRoleMiddleware enforced at the
// gateway. The scraper binds to 127.0.0.1 inside the docker network so this
// handler trusts the gateway gate (A5 documented). No defense-in-depth auth
// check inside the scraper handler.
//
// Defense-in-depth LastErr truncation (RESEARCH P-05): the probe runner is
// expected to truncate to MaxLastErrChars BEFORE Update — but a future code
// path that bypasses the probe could leak unbounded upstream error text into
// the response. Re-truncate here so the JSON we emit is always bounded.
func (h *ScraperHandler) GetAdminHealth(w http.ResponseWriter, r *http.Request) {
	public := h.svc.HealthSnapshot(r.Context())

	enriched := map[string]health.ProviderHealth{}
	if h.cache != nil {
		snap := h.cache.AdminSnapshot()
		for prov, ph := range snap {
			// REVIEW.md WR-02: build a fresh stages map instead of
			// modifying ph.Stages while iterating it. Today the in-place
			// write `ph.Stages[st] = ss` only re-writes the current key
			// (well-defined per the Go spec) but the iteration-mutate
			// pattern is brittle — a future change that fans out a
			// sibling redaction key would be undefined behaviour. The
			// AdminSnapshot returned map is already deep-copied so the
			// allocation here doesn't waste anything user-visible.
			redactedStages := make(map[string]health.StageStatus, len(ph.Stages))
			for st, ss := range ph.Stages {
				if len(ss.LastErr) > health.MaxLastErrChars {
					ss.LastErr = ss.LastErr[:health.MaxLastErrChars]
				}
				redactedStages[st] = ss
			}
			enriched[prov] = health.ProviderHealth{
				Stages:      redactedStages,
				LastUpdated: ph.LastUpdated,
			}
		}
	}

	now := h.now
	if now == nil {
		now = time.Now
	}
	httputil.OK(w, map[string]any{
		"providers":    public,
		"admin":        enriched,
		"generated_at": now().UTC().Format(time.RFC3339),
	})
}

// resolveProviderID converts an incoming mal_id query value into a
// provider-internal ID via the orchestrator's FindID chain. The catalog
// already mapped catalog-UUID → MAL/Shikimori ID before forwarding, so we
// pass the value as ShikimoriID (project memory: Shikimori IDs == MAL IDs).
func (h *ScraperHandler) resolveProviderID(ctx context.Context, malID, title, prefer string) (string, error) {
	ref := domain.AnimeRef{ShikimoriID: malID, Title: title}
	return h.svc.FindID(ctx, ref, prefer)
}

// writeSuccess writes 200 with the standard envelope {success:true,
// data:{<provided fields>, meta:{tried:[...], gated?:true}}}. The meta key
// lives INSIDE data so the frontend's existing axios response handler
// (which already peels `data` off the envelope) sees meta as a sibling of
// the business payload — convenient for ReportButton + diagnostics
// consumers.
//
// The `gated` field is emitted only when true (cache miss / cold path
// where the playability gate actually ran). On gated=false the field is
// OMITTED so cache-hit responses stay byte-identical to Phase 16's shape
// and don't churn FE diffs. The FE (Plan 21-04) treats undefined === false
// === "skip Phase 3 of the loader".
//
// NOTE: in Wave 1 (Plan 21-02) all three call sites pass gated=false
// literally; Plan 21-03 wires the real bool from a new orchestrator return
// signature in the GetStream path. SCRAPER-HEAL-07.
func (h *ScraperHandler) writeSuccess(w http.ResponseWriter, data map[string]any, tried []string, gated bool) {
	if tried == nil {
		tried = []string{}
	}
	meta := map[string]any{"tried": tried}
	if gated {
		meta["gated"] = true
	}
	data["meta"] = meta
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
