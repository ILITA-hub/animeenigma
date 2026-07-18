// Phase 18 (UX-34) — Skip-Intro / Skip-Outro CTA. Backend proxy of the
// community-maintained aniskip.com API.
//
// We proxy (rather than call from the browser) for three reasons:
//   1. CORS — aniskip's endpoints are not browser-CORS-enabled.
//   2. Cache — real timestamps are effectively immutable, so a 7-day positive
//      TTL collapses repeated player loads without hiding later submissions.
//   3. Graceful degradation — aniskip returns 404 for anime not in their DB.
//      The frontend never has to handle that; we coerce to a uniform
//      `{ found: false, results: [] }` shape and the player overlay simply
//      doesn't render.
//
// Style anchor: services/catalog/internal/handler/news.go (cached upstream
// proxy with a thin http.Client).

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/capability"
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

	// detectedSkipTimesTTL is shorter than the AniSkip 7d TTL: content-verify
	// keeps re-probing providers, so a detected verdict can flip (or get
	// superseded by a higher-confidence probe) far sooner than a crowdsourced
	// AniSkip submission ever would.
	detectedSkipTimesTTL = 10 * time.Minute

	// skipStatusDetected mirrors content-verify's domain.SkipDetected wire
	// value. Can't import the constant directly — see the comment on
	// capability.SkipTimingRow.
	skipStatusDetected = "detected"

	// maxProviderTeamLen defensively bounds the provider/team query params
	// before they become part of a cache key. Both are short slugs in
	// practice (provider names, numeric team IDs); this just stops an
	// abusive caller from writing arbitrarily large cache keys.
	maxProviderTeamLen = 128

	// skipDivergenceThreshold flags a detected window that disagrees with
	// AniSkip's on the same side by more than this many seconds on either
	// boundary. Owner directive 2026-07-18: when our probe stored a window
	// FIRST and AniSkip data appeared later with materially different times,
	// that's investigation material — logged with a distinct message so
	// `grep "skip divergence"` finds every case. NOTE: a positional shift is
	// often a legitimately different encode (cold opens / intro cards), so a
	// hit here is a lead, not automatically a detector bug.
	skipDivergenceThreshold = 10.0

	// skipDivergenceLogTTL dedupes the divergence WARN per (anime, provider,
	// team, episode, side) — the check runs on every uncached serve, and one
	// line a day per case is plenty for investigation.
	skipDivergenceLogTTL = 24 * time.Hour

	// Wire values for the per-item Source field.
	skipSourceAniskip  = "aniskip"
	skipSourceDetected = "detected"
)

// animeIDPattern is a defensive shape check on the optional `anime` query
// param before it becomes part of a cache key (mirrors the malId
// digit-validation above). Anime IDs are UUID strings elsewhere in the
// codebase; this doesn't validate UUID structure precisely, just rejects
// anything that isn't hex-and-hyphen of the right length.
var animeIDPattern = regexp.MustCompile(`^[0-9a-f-]{36}$`)

// SkipTimesResult mirrors the aniskip /v2/skip-times response shape, with
// `Found` defaulting to false on upstream 404 so the frontend has a single
// uniform shape to render.
type SkipTimesResult struct {
	Found   bool                  `json:"found"`
	Results []SkipTimesResultItem `json:"results"`
	// Source summarizes where the served windows came from: empty on the
	// pure-AniSkip path (pre-existing wire shape), "detected" when every
	// item is a content-verify probed window, "mixed" when the two sources
	// each contributed a side. Per-item provenance is on each result's own
	// Source field — this is just the roll-up.
	Source string `json:"source,omitempty"`
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
	// Source is per-item provenance: "aniskip" (crowdsourced) or "detected"
	// (content-verify audio probe). Stamped at merge time; the player's
	// hacker-mode HUD surfaces it so a human can tell which source produced
	// the window they're looking at.
	Source string `json:"source,omitempty"`
}

// SkipSource is the optional content-verify blend source for detected skip
// windows (Task 9). Injected as the catalog's shared capability.VerifyClient
// in production; a nil SkipSource disables the blend entirely, leaving this
// handler's behavior byte-identical to the pre-existing pure-AniSkip proxy.
type SkipSource interface {
	SkipTimings(ctx context.Context, animeID string) []capability.SkipTimingRow
}

// SkipTimesHandler proxies aniskip.com and caches usable responses for 7 days.
// Misses and upstream failures are deliberately never cached: ongoing episodes
// can receive community timestamps shortly after the first viewer asks for
// them, so a negative cache would hide newly submitted data for the full TTL.
//
// Blend order (owner directive 2026-07-18, REVERSES the original
// detected-first order): AniSkip wins per SIDE (op/ed independently);
// content-verify's detected windows fill only the sides AniSkip lacks. When
// both sources have the same side and disagree by more than
// skipDivergenceThreshold, a distinct "skip divergence" WARN is logged (see
// warnDivergences) — that's the case where our probe stored a window before
// AniSkip data appeared. The skip source is consulted only when skip is
// non-nil and the request carries `anime`+`provider` query params.
type SkipTimesHandler struct {
	cache      cache.Cache
	httpClient *http.Client
	log        *logger.Logger
	skip       SkipSource
}

// NewSkipTimesHandler wires the dependencies. skip may be nil (feature off).
func NewSkipTimesHandler(c cache.Cache, log *logger.Logger, skip SkipSource) *SkipTimesHandler {
	return &SkipTimesHandler{
		cache: c,
		httpClient: &http.Client{
			Timeout: aniskipUpstreamTimeout,
		},
		log:  log,
		skip: skip,
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
	ep, err := strconv.Atoi(episode)
	if err != nil || ep < 1 {
		httputil.BadRequest(w, "episode must be a positive integer")
		return
	}

	var detected SkipTimesResult
	var animeID, provider, team string
	if h.skip != nil {
		detected, animeID, provider, team = h.tryDetected(r, ep)
	}

	aniskip := h.aniskipResult(r.Context(), malID, episode)

	if aniskip.Found && detected.Found {
		h.warnDivergences(r.Context(), malID, animeID, provider, team, ep, aniskip, detected)
	}

	httputil.OK(w, mergeSkipSources(aniskip, detected))
}

// aniskipResult serves the AniSkip side: 7-day positive cache, then the
// upstream proxy. Misses/failures are never cached (see the handler comment).
func (h *SkipTimesHandler) aniskipResult(ctx context.Context, malID, episode string) SkipTimesResult {
	cacheKey := fmt.Sprintf("skip-times:%s:%s", malID, episode)

	var cached SkipTimesResult
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil {
		if hasUsableSkipTimes(cached) {
			return cached
		}
		// Remove negative/invalid values written by older versions so the cache
		// converges immediately to positive-only storage after deployment.
		if err := h.cache.Delete(ctx, cacheKey); err != nil {
			h.log.Warnw("skip-times stale cache delete error, fetching upstream",
				"mal_id", malID, "episode", episode, "error", err)
		}
	} else if err != cache.ErrNotFound {
		// Cache availability must not decide whether this soft feature can use
		// the upstream. Fetch directly and still serve a successful response.
		h.log.Warnw("skip-times cache get error, fetching upstream",
			"mal_id", malID, "episode", episode, "error", err)
	}

	result := h.fetchFromUpstream(ctx, malID, episode)
	if hasUsableSkipTimes(result) {
		if err := h.cache.Set(ctx, cacheKey, result, skipTimesTTL); err != nil {
			// Cache writes are best-effort. The current viewer should still receive
			// the real timestamps that were fetched successfully.
			h.log.Warnw("skip-times cache set error, serving upstream result",
				"mal_id", malID, "episode", episode, "error", err)
		}
	}
	return result
}

// skipSideOf buckets a skipType into the two mergeable sides. Unknown types
// (e.g. recap) return "" — they pass through from AniSkip untouched and
// never participate in merging or divergence checks. Mirrored by
// content-verify's catalogclient.AniskipKinds (separate module, no shared
// lib owns the AniSkip wire shape) — a new mixed-* type must land in both.
func skipSideOf(skipType string) string {
	switch skipType {
	case "op", "mixed-op":
		return "op"
	case "ed", "mixed-ed":
		return "ed"
	}
	return ""
}

// firstOfSide returns the first item of the given side, or nil.
func firstOfSide(r SkipTimesResult, side string) *SkipTimesResultItem {
	for i := range r.Results {
		if skipSideOf(r.Results[i].SkipType) == side {
			return &r.Results[i]
		}
	}
	return nil
}

// mergeSkipSources implements the AniSkip-first per-side blend: every AniSkip
// item is served as-is; a detected item is appended only for a side AniSkip
// has nothing for. Items are stamped with per-item Source here (not earlier)
// so cached entries written by any older version still come out labeled.
func mergeSkipSources(aniskip, detected SkipTimesResult) SkipTimesResult {
	out := SkipTimesResult{Results: []SkipTimesResultItem{}}
	var nAniskip, nDetected int

	for _, item := range aniskip.Results {
		item.Source = skipSourceAniskip
		out.Results = append(out.Results, item)
		nAniskip++
	}
	for _, side := range []string{"op", "ed"} {
		if firstOfSide(aniskip, side) != nil {
			continue
		}
		if d := firstOfSide(detected, side); d != nil {
			item := *d
			item.Source = skipSourceDetected
			out.Results = append(out.Results, item)
			nDetected++
		}
	}

	out.Found = len(out.Results) > 0
	switch {
	case nDetected > 0 && nAniskip > 0:
		out.Source = "mixed"
	case nDetected > 0:
		out.Source = skipSourceDetected
	}
	return out
}

// warnDivergences logs the distinct "skip divergence" WARN for every side
// both sources cover with materially different times (either boundary off by
// more than skipDivergenceThreshold seconds). Deduped per unit+side for
// skipDivergenceLogTTL via a cache guard key — the comparison itself is
// cheap, but one WARN per player load would drown the signal.
func (h *SkipTimesHandler) warnDivergences(ctx context.Context, malID, animeID, provider, team string, episode int, aniskip, detected SkipTimesResult) {
	for _, side := range []string{"op", "ed"} {
		a := firstOfSide(aniskip, side)
		d := firstOfSide(detected, side)
		if a == nil || d == nil {
			continue
		}
		dStart := d.Interval.StartTime - a.Interval.StartTime
		dEnd := d.Interval.EndTime - a.Interval.EndTime
		if dStart < skipDivergenceThreshold && dStart > -skipDivergenceThreshold &&
			dEnd < skipDivergenceThreshold && dEnd > -skipDivergenceThreshold {
			continue
		}
		guardKey := fmt.Sprintf("skip-div:%s:%s:%s:%d:%s",
			animeID, url.QueryEscape(provider), url.QueryEscape(team), episode, side)
		var seen bool
		if err := h.cache.Get(ctx, guardKey, &seen); err == nil {
			continue
		}
		h.log.Warnw("skip divergence: detected vs aniskip",
			"mal_id", malID, "anime_id", animeID, "provider", provider, "team", team,
			"episode", episode, "side", side,
			"detected_start", d.Interval.StartTime, "detected_end", d.Interval.EndTime,
			"aniskip_start", a.Interval.StartTime, "aniskip_end", a.Interval.EndTime)
		_ = h.cache.Set(ctx, guardKey, true, skipDivergenceLogTTL)
	}
}

// tryDetected fetches content-verify's detected windows for the optional
// `anime`/`provider`/`team` query params. The returned result is the zero
// value (Found=false) whenever the lookup doesn't apply for any reason
// (params absent/invalid, no matching row, matching row with no detected
// side) — the caller merges it with the AniSkip side either way. The parsed
// animeID/provider/team come back too, for the divergence log's identity.
func (h *SkipTimesHandler) tryDetected(r *http.Request, episode int) (result SkipTimesResult, animeID, provider, team string) {
	q := r.URL.Query()
	animeID = q.Get("anime")
	provider = q.Get("provider")
	team = q.Get("team")

	if animeID == "" || provider == "" {
		return SkipTimesResult{}, animeID, provider, team
	}
	if !animeIDPattern.MatchString(animeID) {
		return SkipTimesResult{}, animeID, provider, team
	}
	if len(provider) > maxProviderTeamLen || len(team) > maxProviderTeamLen {
		return SkipTimesResult{}, animeID, provider, team
	}

	// provider/team are free-form (e.g. Kodik fansub team titles can contain
	// ':'), so escape them before splicing into the ':'-delimited cache key —
	// otherwise (provider="kodik", team="A:B") and (provider="kodik:A",
	// team="B") collide on the same key. animeID is already regex-validated
	// above and can't contain ':' or '%'.
	cacheKey := fmt.Sprintf("skip-times:detected:%s:%s:%s:%d",
		animeID, url.QueryEscape(provider), url.QueryEscape(team), episode)

	var cached SkipTimesResult
	if err := h.cache.Get(r.Context(), cacheKey, &cached); err == nil {
		return cached, animeID, provider, team
	} else if err != cache.ErrNotFound && h.log != nil {
		// Cache availability must not decide whether this lookup can run —
		// fall through to a fresh content-verify lookup below.
		h.log.Warnw("skip-times detected cache get error, querying content-verify",
			"anime_id", animeID, "provider", provider, "episode", episode, "error", err)
	}

	row, found := matchSkipTimingRow(h.skip.SkipTimings(r.Context(), animeID), provider, team, episode)
	if !found {
		return SkipTimesResult{}, animeID, provider, team
	}

	built, ok := buildDetectedResult(row)
	if !ok {
		return SkipTimesResult{}, animeID, provider, team
	}

	if err := h.cache.Set(r.Context(), cacheKey, built, detectedSkipTimesTTL); err != nil && h.log != nil {
		// Best-effort cache write — the current request already has the
		// result, only the next lookup pays the content-verify round trip.
		h.log.Warnw("skip-times detected cache set error, serving result",
			"anime_id", animeID, "provider", provider, "episode", episode, "error", err)
	}
	return built, animeID, provider, team
}

// matchSkipTimingRow finds the row for the exact (provider, team, episode)
// unit the player is streaming from. team legitimately empty-matches for
// scraper providers that don't have a team/fansub concept.
//
// Fallback: when the exact-team match misses, retry against the row with an
// empty Team for the same (provider, episode). content-verify enumerates
// animejoy skip units with Team="" (no fansub concept on that leg), but the
// frontend sends the selected fansub name as `team` for animejoy when the
// user has picked one — without this fallback, every animejoy detected
// window would silently never serve. This is safe for kodik: kodik rows
// always carry a non-empty Team (one per translation), so no kodik row can
// ever exist at Team="" for the fallback to incorrectly match against.
func matchSkipTimingRow(rows []capability.SkipTimingRow, provider, team string, episode int) (capability.SkipTimingRow, bool) {
	for _, row := range rows {
		if row.Provider == provider && row.Team == team && row.Episode == episode {
			return row, true
		}
	}
	if team != "" {
		for _, row := range rows {
			if row.Provider == provider && row.Team == "" && row.Episode == episode {
				return row, true
			}
		}
	}
	return capability.SkipTimingRow{}, false
}

// buildDetectedResult builds one SkipTimesResultItem per detected side
// (op/ed independently — a row can have one detected and the other
// no_match/pending_fp/unreachable/aniskip). ok=false when neither side is
// detected, so the caller doesn't cache an empty result.
func buildDetectedResult(row capability.SkipTimingRow) (SkipTimesResult, bool) {
	result := SkipTimesResult{Found: true, Source: "detected", Results: []SkipTimesResultItem{}}
	if row.OpStatus == skipStatusDetected {
		item := SkipTimesResultItem{SkipType: "op", SkipID: "", EpisodeLength: 0}
		item.Interval.StartTime = row.OpStart
		item.Interval.EndTime = row.OpEnd
		result.Results = append(result.Results, item)
	}
	if row.EdStatus == skipStatusDetected {
		item := SkipTimesResultItem{SkipType: "ed", SkipID: "", EpisodeLength: 0}
		item.Interval.StartTime = row.EdStart
		item.Interval.EndTime = row.EdEnd
		result.Results = append(result.Results, item)
	}
	if len(result.Results) == 0 {
		return SkipTimesResult{}, false
	}
	return result, true
}

// hasUsableSkipTimes is the positive-cache gate. `found:true` alone is not
// sufficient: only a recognized OP/ED interval with a real positive duration
// is worth retaining. Unknown future result types still pass through to the
// caller, but they do not create a seven-day cache entry the current player
// cannot use.
func hasUsableSkipTimes(result SkipTimesResult) bool {
	if !result.Found {
		return false
	}
	for _, item := range result.Results {
		if item.Interval.StartTime < 0 || item.Interval.EndTime <= item.Interval.StartTime {
			continue
		}
		switch item.SkipType {
		case "op", "ed", "mixed-op", "mixed-ed":
			return true
		}
	}
	return false
}

// fetchFromUpstream calls aniskip and coerces all failure modes (404, non-2xx,
// network error, malformed JSON, missing fields) to the uniform empty shape.
// The caller serves that shape without caching it, allowing newly submitted
// timestamps for ongoing episodes to become visible on the next request.
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
