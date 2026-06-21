// Phase 18 (UX-34) — Skip-Intro / Skip-Outro CTA. Backend proxy of the
// community-maintained aniskip.com API.
//
// We proxy (rather than call from the browser) for three reasons:
//   1. CORS — aniskip's endpoints are not browser-CORS-enabled.
//   2. Cache — timestamps are crowdsourced and effectively immutable, so a
//      7-day TTL collapses thousands of player loads down to one upstream
//      request per (malId, episode) pair.
//   3. Graceful degradation — aniskip returns 404 for anime not in their DB.
//      The frontend never has to handle that; we coerce to a uniform
//      `{ found: false, results: [] }` shape and the player overlay simply
//      doesn't render.
//
// Style anchor: services/catalog/internal/handler/news.go (cached upstream
// proxy via cache.Cache GetOrSet + a thin http.Client).

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/go-chi/chi/v5"
)

const (
	// aniskip API v2 — community-maintained, no auth required.
	// Endpoint format: /v2/skip-times/{mal_id}/{episode}?types=op,ed
	aniskipBaseURL = "https://api.aniskip.com/v2/skip-times"

	// 7 days — timestamps are effectively immutable once crowdsourced.
	skipTimesTTL = 7 * 24 * time.Hour

	// Upstream timeout — aniskip is usually <300ms; 5s is generous but
	// short enough that a flaky upstream doesn't slow the player UI.
	aniskipUpstreamTimeout = 5 * time.Second
)

// SkipTimesResult mirrors the aniskip /v2/skip-times response shape, with
// `Found` defaulting to false on upstream 404 so the frontend has a single
// uniform shape to render.
type SkipTimesResult struct {
	Found   bool                  `json:"found"`
	Results []SkipTimesResultItem `json:"results"`
}

// SkipTimesResultItem is the per-segment skip record. `op` / `ed` are the
// only types we request, but we don't whitelist on decode — if aniskip ever
// adds new types (e.g. recap), passing them through is harmless because the
// frontend composable only matches on the known values.
//
// JSON shape: aniskip v2 emits camelCase (startTime/endTime/skipType/...);
// we re-emit the same camelCase shape on our /api/skip-times endpoint so
// the frontend gets a 1:1 passthrough that matches the upstream docs.
type SkipTimesResultItem struct {
	Interval struct {
		StartTime float64 `json:"startTime"`
		EndTime   float64 `json:"endTime"`
	} `json:"interval"`
	SkipType      string  `json:"skipType"` // "op" | "ed" | "mixed-op" | "mixed-ed" | "recap"
	SkipID        string  `json:"skipId"`   // Aniskip submission UUID — unused frontend-side
	EpisodeLength float64 `json:"episodeLength"`
}

// SkipTimesHandler proxies aniskip.com and caches responses for 7 days.
type SkipTimesHandler struct {
	cache      cache.Cache
	httpClient *http.Client
	log        *logger.Logger
}

// NewSkipTimesHandler wires the dependencies.
func NewSkipTimesHandler(c cache.Cache, log *logger.Logger) *SkipTimesHandler {
	return &SkipTimesHandler{
		cache: c,
		httpClient: &http.Client{
			Timeout: aniskipUpstreamTimeout,
		},
		log: log,
	}
}

// Get handles GET /api/skip-times/{malId}/{episode}.
//
// Path-param validation: malId must be all digits (aniskip uses positive
// MAL integer IDs); episode must be a positive integer. Anything else 400s
// before we touch the cache or the upstream — defense against path-injection
// into the URL we build.
func (h *SkipTimesHandler) Get(w http.ResponseWriter, r *http.Request) {
	malID := chi.URLParam(r, "malId")
	episode := chi.URLParam(r, "episode")

	if malID == "" || episode == "" {
		httputil.BadRequest(w, "malId and episode are required")
		return
	}

	// Validate malId is a positive integer. Aniskip rejects non-numeric IDs
	// upstream anyway, but we want to short-circuit malformed traffic and
	// not pollute the cache with arbitrary string keys.
	if _, err := strconv.ParseUint(malID, 10, 64); err != nil {
		httputil.BadRequest(w, "malId must be a positive integer")
		return
	}
	if ep, err := strconv.Atoi(episode); err != nil || ep < 1 {
		httputil.BadRequest(w, "episode must be a positive integer")
		return
	}

	cacheKey := fmt.Sprintf("skip-times:%s:%s", malID, episode)

	var result SkipTimesResult
	err := h.cache.GetOrSet(r.Context(), cacheKey, &result, skipTimesTTL, func() (interface{}, error) {
		return h.fetchFromUpstream(r.Context(), malID, episode), nil
	})
	if err != nil {
		// fetchFromUpstream never returns an error (it coerces failures to
		// an empty result so the cache stores them and we don't hammer
		// aniskip on misses). Cache layer errors are still possible —
		// degrade gracefully to the same empty shape.
		h.log.Warnw("skip-times cache GetOrSet error, returning empty",
			"mal_id", malID, "episode", episode, "error", err)
		httputil.OK(w, SkipTimesResult{Found: false, Results: []SkipTimesResultItem{}})
		return
	}

	httputil.OK(w, result)
}

// fetchFromUpstream calls aniskip and coerces all failure modes (404, non-2xx,
// network error, malformed JSON, missing fields) to the uniform empty shape.
// Returning a value for every code path lets cache.GetOrSet store the miss so
// repeat lookups for the same (malId, episode) don't re-hit the upstream
// during the 7-day TTL window.
func (h *SkipTimesHandler) fetchFromUpstream(ctx context.Context, malID, episode string) SkipTimesResult {
	empty := SkipTimesResult{Found: false, Results: []SkipTimesResultItem{}}

	// Build URL: /v2/skip-times/{malId}/{episode}?types=op&types=ed
	// (aniskip accepts both ?types=op,ed and the repeated form; we use
	// the repeated form because it's what their docs canonicalize.)
	u, err := url.Parse(fmt.Sprintf("%s/%s/%s", aniskipBaseURL,
		url.PathEscape(malID), url.PathEscape(episode)))
	if err != nil {
		h.log.Warnw("aniskip url build failed", "mal_id", malID, "episode", episode, "error", err)
		return empty
	}
	q := u.Query()
	q.Add("types", "op")
	q.Add("types", "ed")
	// Aniskip v2 requires `episodeLength` as a numeric query param. We don't
	// know the canonical length client-side (varies per release), but `0`
	// acts as a wildcard upstream and returns all crowdsourced submissions
	// regardless of declared episode length. Omitting it returns HTTP 400
	// "episodeLength must be a number". Verified 2026-05-13 against
	// api.aniskip.com/v2.
	q.Set("episodeLength", "0")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		h.log.Warnw("aniskip request build failed", "error", err)
		return empty
	}
	// Identify ourselves to the upstream — not required but polite.
	req.Header.Set("User-Agent", "AnimeEnigma/1.0 (+https://animeenigma.ru)")
	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		// Network error / timeout. Don't surface — this is a soft feature.
		h.log.Infow("aniskip upstream network error, treating as not-found",
			"mal_id", malID, "episode", episode, "error", err)
		return empty
	}
	defer func() { _ = resp.Body.Close() }()

	// 404 is the expected "no crowdsourced data for this anime" signal —
	// not an error, just a normal miss. Anything else non-2xx is treated
	// the same way (we don't propagate upstream failures to the player UI).
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode != http.StatusNotFound {
			h.log.Infow("aniskip non-200, treating as not-found",
				"mal_id", malID, "episode", episode, "status", resp.StatusCode)
		}
		// Drain so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		return empty
	}

	var parsed SkipTimesResult
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		h.log.Warnw("aniskip response decode failed",
			"mal_id", malID, "episode", episode, "error", err)
		return empty
	}

	// Normalize nil Results to empty slice so JSON encoding emits `[]`
	// instead of `null` — the frontend treats both safely but `[]` is the
	// documented contract.
	if parsed.Results == nil {
		parsed.Results = []SkipTimesResultItem{}
	}
	return parsed
}
