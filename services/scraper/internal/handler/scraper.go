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
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/config"
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
// The optional `providersCfg` field (Task 3 — unified player plan) carries
// the operator-configured registry metadata (enabled/reason/description) so
// GetHealth can surface it alongside the live stage snapshot. A nil value is
// safe: GetHealth degrades gracefully and omits the metadata fields.
//
// REVIEW.md WR-11: `now` is injectable so tests can lock the generated_at
// timestamp in admin responses. Production defaults to time.Now.
type ScraperHandler struct {
	svc          *service.Orchestrator
	cache        *health.InMemoryHealthCache
	providersCfg *config.ProvidersConfig
	log          *logger.Logger
	now          func() time.Time
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

// WithProvidersConfig attaches the operator provider registry to the handler
// so GetHealth can emit enabled/reason/description per provider. Passing nil
// is safe — GetHealth will omit the metadata fields gracefully.
// Production callers should call this once during startup (see main.go).
func (h *ScraperHandler) WithProvidersConfig(cfg *config.ProvidersConfig) {
	h.providersCfg = cfg
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
	malID     string
	title     string
	altTitles []string
	episode   string
	server    string
	category  string
	prefer    string
}

// maxTitleLength caps the `title` query-string parameter so an oversized
// title can't balloon log lines or fuzzy-match comparison cost. Real anime
// titles are well under 200 chars; 512 is generous.
const maxTitleLength = 512

// maxAltTitles bounds the number of alternate title forms (title_alt) honored
// per request so a caller can't balloon fuzzy-match work with a huge list.
// The catalog has at most 3 useful forms (romaji Name, NameEN, NameJP).
const maxAltTitles = 4

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
	// title_alt carries additional title forms (comma-separated) the catalog
	// knows about — providers that fuzzy-match score against all of them
	// (ISS-017). Each form is trimmed + length-capped; blanks and dupes of the
	// primary title are dropped. Capped at maxAltTitles to bound work.
	altTitles := parseAltTitles(q.Get("title_alt"), title)
	return queryParams{
		malID:     strings.TrimSpace(q.Get("mal_id")),
		title:     title,
		altTitles: altTitles,
		episode:   strings.TrimSpace(q.Get("episode")),
		server:    strings.TrimSpace(q.Get("server")),
		category:  strings.TrimSpace(q.Get("category")),
		prefer:    prefer,
	}
}

// parseAltTitles splits the comma-separated `title_alt` query value into a
// deduped, trimmed, length-capped slice. The primary title is excluded (it's
// already carried in queryParams.title) and blanks are dropped. At most
// maxAltTitles forms are returned. ISS-017.
func parseAltTitles(raw, primary string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make([]string, 0, maxAltTitles)
	seen := map[string]bool{strings.ToLower(primary): true}
	for _, part := range strings.Split(raw, ",") {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		if len(t) > maxTitleLength {
			t = t[:maxTitleLength]
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
		if len(out) >= maxAltTitles {
			break
		}
	}
	return out
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

	providerID, idWinner, err := h.resolveProviderID(r.Context(), qp.malID, qp.title, qp.altTitles, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}

	// Pin ListEpisodes to the provider that resolved the ID — the providerID
	// is opaque and only that provider can parse it (see resolveProviderID).
	eps, winner, err := h.svc.ListEpisodesNamed(r.Context(), providerID, pinPrefer(idWinner, qp.prefer))
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}
	if eps == nil {
		eps = []domain.Episode{}
	}
	// gated=false: GetEpisodes does not run the playability gate.
	// winner = the provider whose ListEpisodes succeeded; surfaced as
	// meta.provider so the client pins servers/stream to the SAME provider
	// (episode IDs are opaque + provider-specific — pinning the wrong one
	// breaks the whole servers/stream chain).
	h.writeSuccess(w, map[string]any{"episodes": eps}, tried, false, winner)
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

	providerID, idWinner, err := h.resolveProviderID(r.Context(), qp.malID, qp.title, qp.altTitles, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}

	srvs, err := h.svc.ListServers(r.Context(), providerID, qp.episode, pinPrefer(idWinner, qp.prefer))
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
	// Phase 21 SCRAPER-HEAL-04: empty `server` is the cold-path signal —
	// the orchestrator routes through gogoanime.GetStreamWithGate which
	// runs the playability gate over the configured priority list and
	// caches the winning serverID. Non-empty `server` is the caller-pin
	// path (Phase 16 semantics preserved). Both paths flow through
	// GetStreamGated; the gated bool surfaces via data.meta.gated.

	cat := domain.Category(qp.category)
	if cat == "" {
		cat = domain.CategorySub
	}

	providerID, idWinner, err := h.resolveProviderID(r.Context(), qp.malID, qp.title, qp.altTitles, qp.prefer)
	if err != nil {
		h.writeOrchestratorError(w, err, tried)
		return
	}

	stream, gated, err := h.svc.GetStreamGated(r.Context(), providerID, qp.episode, qp.server, cat, pinPrefer(idWinner, qp.prefer))
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
// playabilityFreshTTL bounds how old a probe-cache entry may be before its
// stream_segment oracle is treated as "no recent data" in the public view.
// The probe ticks every ~15 min (health.probeBaseInterval); 2× tolerates one
// missed tick. (Distinct from the orchestrator's cacheStaleTTL=60s fail-open
// window, which — being far shorter than the probe interval — is a separate
// known issue tracked in ISS-021 and intentionally NOT changed here.)
const playabilityFreshTTL = 30 * time.Minute

// providerEnriched is the per-provider JSON shape emitted by GetHealth.
// It extends domain.Health with registry metadata (Task 3 — unified player
// plan): enabled/reason/description from ProvidersConfig plus a top-level
// `up` bool derived from live stage data.
//
// JSON field ordering mirrors the existing shape: provider and stages remain
// first so downstream consumers that parse the existing fields continue to
// work. The four new fields are additive and optional — a nil ProvidersConfig
// leaves enabled at its zero value (true, matching IsEnabled for absent
// providers); reason and description are empty strings.
type providerEnriched struct {
	// Existing fields — preserved verbatim from domain.Health.
	Provider string                    `json:"provider"`
	Stages   map[string]domain.StageHealth `json:"stages"`
	// Registry metadata (Task 3).
	Enabled     bool   `json:"enabled"`
	Up          bool   `json:"up"`
	Reason      string `json:"reason,omitempty"`
	Description string `json:"description,omitempty"`
}

// healthUp derives a coarse boolean "is this provider up?" from its live
// stage snapshot. A provider is considered up when at least one of its
// non-segment stages (search, episodes, servers, stream) is Up=true.
// stream_segment is excluded because it is a probe-only oracle and starts
// as false until the first probe tick — including it would flip every newly
// booted provider to down.
func healthUp(ph domain.Health) bool {
	for stage, sh := range ph.Stages {
		if stage != health.StageStreamSegment && sh.Up {
			return true
		}
	}
	return false
}

// GetHealth handles GET /scraper/health — the public per-provider snapshot.
//
// ISS-021: each provider's HealthCheck() self-report only reflects API liveness
// (search/episodes/servers/stream); its stream_segment is never validated
// (providers don't fetch segments), so the raw table reported all-green even
// when playback was broken. We overlay the probe's real byte-oracle
// (stream_segment, from the cache fetchSegment now follows to an actual media
// segment) onto each provider and expose a per-provider `playable` summary so
// the table reflects real playability. Providers with no fresh oracle data are
// OMITTED from `playable` (absent = unknown, not a fake green/red).
//
// Task 3 (unified player plan): each entry now also carries `enabled`, `up`,
// `reason`, and `description` from the operator ProvidersConfig. These fields
// are additive — existing fields (provider, stages) are unchanged.
func (h *ScraperHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	snap := h.svc.HealthSnapshot(r.Context())

	playable := map[string]bool{}
	if h.cache != nil {
		now := time.Now
		if h.now != nil {
			now = h.now
		}
		cacheSnap := h.cache.AdminSnapshot()
		for prov, ph := range snap {
			ch, ok := cacheSnap[prov]
			fresh := ok && now().Sub(ch.LastUpdated) <= playabilityFreshTTL
			var seg health.StageStatus
			hasSeg := false
			if fresh {
				seg, hasSeg = ch.Stages[health.StageStreamSegment]
			}
			if hasSeg {
				// Overlay the REAL oracle result onto the public stage map.
				ph.Stages[health.StageStreamSegment] = domain.StageHealth{
					Up:      seg.Up,
					LastOK:  seg.LastOK,
					LastErr: seg.LastErr,
				}
				playable[prov] = seg.Up
			} else {
				// No fresh real-bytes confirmation → never claim a green
				// stream_segment. Mark it explicitly and leave `playable`
				// unset (unknown) rather than asserting true or false.
				ph.Stages[health.StageStreamSegment] = domain.StageHealth{
					Up:      false,
					LastErr: "no recent playability probe",
				}
			}
			snap[prov] = ph
		}
	}

	// Build enriched per-provider entries (Task 3). If no ProvidersConfig is
	// wired, the new fields default to enabled=true (matching IsEnabled for absent
	// strings) — callers that don't set WithProvidersConfig still get a valid
	// response; only the metadata fields are missing.
	enriched := make(map[string]providerEnriched, len(snap))
	for prov, ph := range snap {
		entry := providerEnriched{
			Provider: ph.Provider,
			Stages:   ph.Stages,
			Enabled:  true, // default: enabled unless registry says otherwise
			Up:       healthUp(ph),
		}
		if h.providersCfg != nil {
			meta := h.providersCfg.Meta(prov)
			// IsEnabled defaults to true for absent providers; reflect that.
			entry.Enabled = h.providersCfg.IsEnabled(prov)
			entry.Reason = meta.Reason
			entry.Description = meta.Description
		}
		enriched[prov] = entry
	}

	httputil.OK(w, map[string]any{
		"providers": enriched,
		"playable":  playable,
	})
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
//
// Returns the opaque providerID AND the name of the provider that resolved
// it. Callers MUST pass the winner (via pinPrefer) as `prefer` to the
// subsequent ListEpisodes/ListServers/GetStream stage so the provider-specific
// ID is handed to the provider that produced it — otherwise the next stage's
// failover restarts at the head of the order and a wrong provider can return
// an empty-but-no-error result that short-circuits failover. See
// Orchestrator.FindIDNamed.
func (h *ScraperHandler) resolveProviderID(ctx context.Context, malID, title string, altTitles []string, prefer string) (string, string, error) {
	ref := domain.AnimeRef{ShikimoriID: malID, Title: title, AltTitles: altTitles}
	return h.svc.FindIDNamed(ctx, ref, prefer)
}

// pinPrefer chooses the effective `prefer` for a post-FindID stage: the
// provider that resolved the ID (winner) when known, else the caller's
// original prefer. Pinning to the winner keeps the opaque providerID on its
// origin provider while leaving the rest of the chain as a failover safety net.
func pinPrefer(winner, prefer string) string {
	if winner != "" {
		return winner
	}
	return prefer
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
// The optional `provider` variadic carries the name of the provider that
// actually served the request (the failover winner). When non-empty it is
// emitted as meta.provider so the client can pin subsequent calls to the same
// provider — opaque, provider-specific episode/server IDs only resolve on the
// provider that produced them. Omitted when empty so responses that don't
// resolve a single winner stay shape-stable.
func (h *ScraperHandler) writeSuccess(w http.ResponseWriter, data map[string]any, tried []string, gated bool, provider ...string) {
	if tried == nil {
		tried = []string{}
	}
	meta := map[string]any{"tried": tried}
	if gated {
		meta["gated"] = true
	}
	if len(provider) > 0 && provider[0] != "" {
		meta["provider"] = provider[0]
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
