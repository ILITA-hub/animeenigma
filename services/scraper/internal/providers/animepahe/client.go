// client.go — AnimePahe domain.Provider implementation.
//
// SCRAPER-PAHE-01..04 / SCRAPER-NF-02 (Plan 16-03 Task 2). Layered on:
//
//   - Plan 16-01: domain.BaseHTTPClient.Jar() accessor, on-disk goldens.
//   - Plan 16-02: KwikExtractor in services/scraper/internal/embeds (registered
//     by the orchestrator so we route kwik.cx URLs through it).
//
// Responsibilities:
//
//   - FindID resolves an AnimeRef → AnimePahe anime UUID `session` via
//     malsync.moe (24h cache; 24h negative cache) with a Jaro-Winkler
//     ≥ 0.85 fuzzy fallback against the resolver's `/search` results.
//   - ListEpisodes paginates `/release?session=<animeSession>&page=N` with a
//     50-page hard cap, caching the assembled list for 6h at key
//     episodes:animepahe:{animeSession}.
//   - ListServers scrapes `/play?animeSession=<a>&episodeSession=<e>` for
//     kwik.cx button[data-src] URLs. Real-empty → []Server{} (NOT error);
//     selector drift → ErrExtractFailed.
//   - GetStream looks up the kwik URL via the embeds.Registry and delegates
//     extraction. Stream URLs are cached with TTL min(expires-30s, 5min);
//     already-expired URLs are NOT cached.
//   - HealthCheck returns an in-memory snapshot of the four stage timings.
//
// Phase 27 SCRAPER-HEAL-30: every upstream-fetch goes through the
// `resolverClient` (see resolver.go) which talks to the
// `animepahe-resolver` stealth-Chromium sidecar. The Go-side DDoS-Guard
// handshake (Pattern 3) is GONE — the sidecar owns the challenge stack.
// MalSync invalidation on `/release` 404 is single-strike (A9) and backed
// by a persistent reverse-mapping cache key so it survives process
// restarts (see malsync.go::Invalidate).
package animepahe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/fuzzy"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

// providerName is the stable identifier returned by Name() and used as the
// orchestrator's registry key.
const providerName = "animepahe"

// fuzzyMatchThreshold is the minimum Jaro-Winkler score for /api?m=search
// fuzzy fallback to claim a match (per RESEARCH.md Pitfall 5 / A6).
const fuzzyMatchThreshold = 0.85

// maxEpisodePages is the upper bound on /api?m=release pagination — > 1500
// episodes is implausible for any anime and stops a misbehaving upstream
// from running us off into infinity (T-16-03-04 mitigation).
const maxEpisodePages = 50

// episodesCacheTTL is the 6h cache duration for the assembled episode list.
const episodesCacheTTL = 6 * time.Hour

// maxBodyAPI caps the response body of /api requests at 4 MiB. Real release
// pages are < 50 KiB; this is a DoS guard.
const maxBodyAPI = 4 << 20

// maxBodyHTML caps the response body of /play pages at 2 MiB. Real /play
// pages are < 100 KiB.
const maxBodyHTML = 2 << 20

// stageNames lock the canonical stage keys returned by HealthCheck.
// Phase 17 Plan 02: renamed from legacy keys (find_id / list_episodes / etc.)
// to the canonical 5-stage strings from services/scraper/internal/health/stage.go.
// The four pipeline stages exposed by the provider itself are search /
// episodes / servers / stream; the fifth canonical stage (stream_segment)
// is owned by the probe runner, not the provider, so it is NOT in this slice.
//
// These strings appear VERBATIM as Prometheus label values + Grafana queries;
// treat as a versioned contract.
var stageNames = []string{
	health.StageSearch,
	health.StageEpisodes,
	health.StageServers,
	health.StageStream,
}

// Selector identifiers for parser_zero_match_total. These MUST be short
// stable identifiers — NOT raw CSS — to bound the cardinality of the
// {selector=...} label (RESEARCH P-02 cardinality bomb mitigation).
//
// Adding a new selector miss path? Define a new const here and reference
// it at the call site. Never call ParserZeroMatchTotal.WithLabelValues
// with a string literal.
const (
	selectorEpisodeListItem = "episode_list_item"
	selectorServerLink      = "server_link"
	selectorKwikPackedJS    = "kwik_packed_js"
)

// malSyncClient is the malsync lookup contract — abstracted so tests can
// inject a fake without standing up a real malsync HTTP server.
//
// Phase 27 A9 (SCRAPER-HEAL-30): extended with two helpers backing
// single-strike invalidation on /release 404. LookupMalID is the reverse
// of Lookup (providerID → malID via persistent cache); Invalidate evicts
// both the forward and reverse cache entries in one variadic Delete.
// Both are best-effort: callers IGNORE the returned error and continue.
//
// All three methods are exercised by the parser; the resolver-transport
// migration (Task 1) introduces the interface, and the persistent-cache
// implementation lands in malsync.go (Task 2 conceptually; physically
// landed alongside the interface so the package compiles).
type malSyncClient interface {
	Lookup(ctx context.Context, malID, provider string) (string, bool, error)
	LookupMalID(ctx context.Context, providerID, provider string) (string, error)
	Invalidate(ctx context.Context, malID, provider, providerID string) error
}

// Deps is the constructor input for New(). Every reference field must be
// non-nil except Log (a no-op fallback is constructed if absent).
type Deps struct {
	// ResolverURL is the animepahe-resolver sidecar base URL (default
	// http://animepahe-resolver:3000 per Phase 27 D2). Plan 27-02 wires
	// this from SCRAPER_ANIMEPAHE_RESOLVER_URL. Replaces the Phase 16
	// BaseURL field (no Go-side code talks to animepahe.* upstream
	// directly anymore — the sidecar owns the stealth-Chromium stack).
	ResolverURL string
	HTTP        *domain.BaseHTTPClient
	Embeds      *domain.Registry
	MalSync     malSyncClient
	Cache       cache.Cache
	Log         *logger.Logger
}

// Provider implements domain.Provider for the AnimePahe upstream.
type Provider struct {
	resolver *resolverClient
	http     *domain.BaseHTTPClient
	embeds   *domain.Registry
	malsync  malSyncClient
	cache    cache.Cache
	log      *logger.Logger

	// stages is the in-memory health snapshot, updated on each method call.
	// Phase 16 only requires the snapshot exist with the four canonical
	// stage keys; Phase 17 will extend with real probes.
	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs a Provider with sane defaults — empty ResolverURL falls
// back to http://animepahe-resolver:3000 (the docker-compose service name).
// WR-11: required dependencies (HTTP, Embeds, MalSync, Cache) are validated
// eagerly and a non-nil error is returned if any is missing. main.go fatals
// on the error, so misconfiguration surfaces at boot rather than later as a
// confusing nil-pointer dereference 502. d.Log is optional and falls back to
// logger.Default().
func New(d Deps) (*Provider, error) {
	if d.HTTP == nil {
		return nil, errors.New("animepahe: Deps.HTTP is required")
	}
	if d.Embeds == nil {
		return nil, errors.New("animepahe: Deps.Embeds is required")
	}
	if d.MalSync == nil {
		return nil, errors.New("animepahe: Deps.MalSync is required")
	}
	if d.Cache == nil {
		return nil, errors.New("animepahe: Deps.Cache is required")
	}
	if d.Log == nil {
		d.Log = logger.Default()
	}
	resolverURL := d.ResolverURL
	if resolverURL == "" {
		resolverURL = "http://animepahe-resolver:3000"
	}
	p := &Provider{
		resolver: newResolverClient(resolverURL, d.HTTP),
		http:     d.HTTP,
		embeds:   d.Embeds,
		malsync:  d.MalSync,
		cache:    d.Cache,
		log:      d.Log,
		stages:   make(map[string]domain.StageHealth, len(stageNames)),
	}
	// Pre-seed all four stages so HealthCheck always returns the canonical
	// shape even before any traffic.
	for _, s := range stageNames {
		p.stages[s] = domain.StageHealth{Up: true}
	}
	return p, nil
}

// Name returns the stable identifier "animepahe".
func (p *Provider) Name() string { return providerName }

// markStage records the success/failure of one stage. Called from each
// method on entry-success and entry-failure paths.
func (p *Provider) markStage(stage string, err error) {
	p.stagesMu.Lock()
	defer p.stagesMu.Unlock()
	sh := p.stages[stage]
	if err == nil {
		sh.Up = true
		sh.LastOK = time.Now()
		sh.LastErr = ""
	} else {
		sh.Up = false
		sh.LastErr = err.Error()
	}
	p.stages[stage] = sh
}

// HealthCheck returns a snapshot of the in-memory stage health.
func (p *Provider) HealthCheck(ctx context.Context) domain.Health {
	p.stagesMu.Lock()
	defer p.stagesMu.Unlock()
	snap := make(map[string]domain.StageHealth, len(p.stages))
	for k, v := range p.stages {
		snap[k] = v
	}
	return domain.Health{Provider: providerName, Stages: snap}
}

// FindID resolves an AnimeRef → AnimePahe anime UUID `session`. First tries
// malsync.moe (positive + negative cache); falls back to the resolver's
// /search endpoint with a Jaro-Winkler fuzzy match (threshold 0.85).
//
// Phase 27: when the MalSync forward write fires (positive cache hit), the
// reverse-mapping key `malsync_reverse:animepahe:<session> → <malID>` is
// also persisted so `/release` 404 invalidation works across process
// restarts (see malsync.go::FindID).
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	// 1. malsync hit?
	if ref.ShikimoriID != "" {
		if id, ok, err := p.malsync.Lookup(ctx, ref.ShikimoriID, providerName); err == nil && ok {
			p.markStage(health.StageSearch, nil)
			return id, nil
		}
	}
	// 2. Fuzzy /search fallback (via resolver sidecar).
	if ref.Title == "" {
		err := domain.WrapNotFound(errors.New("no title"), "animepahe: cannot search without a title")
		p.markStage(health.StageSearch, err)
		return "", err
	}
	sr, err := p.resolver.Search(ctx, ref.Title)
	if err != nil {
		p.markStage(health.StageSearch, err)
		return "", err
	}
	if len(sr.Data) == 0 {
		err := domain.WrapNotFound(nil, "animepahe: 0 search results for "+ref.Title)
		p.markStage(health.StageSearch, err)
		return "", err
	}
	// 3. Score each entry; pick the best ≥ threshold.
	normTitle := fuzzy.NormalizeTitle(ref.Title)
	best := struct {
		score   float64
		session string
	}{}
	for _, e := range sr.Data {
		score := fuzzy.JaroWinkler(normTitle, fuzzy.NormalizeTitle(e.Title))
		if score > best.score {
			best.score = score
			best.session = e.Session
		}
	}
	if best.score < fuzzyMatchThreshold || best.session == "" {
		err := domain.WrapNotFound(
			fmt.Errorf("best score %.4f", best.score),
			"animepahe: no fuzzy match for "+ref.Title,
		)
		p.markStage(health.StageSearch, err)
		return "", err
	}
	p.markStage(health.StageSearch, nil)
	return best.session, nil
}

// ListEpisodes paginates `/release?session=<animeSession>&page=N` for the
// given AnimePahe anime UUID `session`, caches the assembled list for 6
// hours, and returns ([]Episode, nil) for the real-empty case.
//
// Phase 27 A9: on a /release 404 the persistent MalSync reverse map is
// consulted; if a known mal_id maps to this animeSession the forward +
// reverse entries are evicted (single-strike invalidation). The 404 is
// surfaced unchanged.
func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	cacheKey := fmt.Sprintf("episodes:%s:%s", providerName, providerID)
	var cached []domain.Episode
	if err := p.cache.Get(ctx, cacheKey, &cached); err == nil {
		p.markStage(health.StageEpisodes, nil)
		return cached, nil
	}

	all := make([]domain.Episode, 0, 32)
	for page := 1; page <= maxEpisodePages; page++ {
		rr, err := p.resolver.Release(ctx, providerID, page)
		if err != nil {
			// A9 single-strike: on /release 404 evict the MalSync mapping
			// so the next FindID call re-runs /search. The reverse-key
			// lookup is persistent (see malsync.go::LookupMalID) so this
			// works across process restarts. Best-effort: any lookup or
			// delete error is ignored — failing to evict a stale mapping
			// is strictly less bad than failing the user-visible 404.
			if errors.Is(err, domain.ErrNotFound) {
				if malID, lookupErr := p.malsync.LookupMalID(ctx, providerID, providerName); lookupErr == nil && malID != "" {
					_ = p.malsync.Invalidate(ctx, malID, providerName, providerID)
				}
			}
			p.markStage(health.StageEpisodes, err)
			return nil, err
		}
		// SCRAPER-NF-04: emit parser_zero_match_total when the upstream
		// returns zero episode items on the FIRST page. Distinct from
		// "anime exists but no episodes aired yet" only by context — both
		// look the same from JSON. We bias toward instrumenting all
		// zero-result first pages so a real upstream selector drift (the
		// items key changed name, returns empty) is observable as a sudden
		// jump in this counter. Real-empty new anime contribute a baseline
		// drift that's still well-bounded by golden-pool selection.
		if page == 1 && len(rr.Data) == 0 {
			metrics.ParserZeroMatchTotal.WithLabelValues(providerName, selectorEpisodeListItem).Inc()
		}
		for _, ep := range rr.Data {
			all = append(all, domain.Episode{
				ID:       ep.Session,
				Number:   int(math.Round(ep.EpisodeNumber)),
				Title:    ep.Title,
				IsFiller: ep.Filler == 1,
			})
		}
		if rr.CurrentPage >= rr.LastPage {
			break
		}
	}
	// 6h cache — even for the real-empty case, so we don't re-hit upstream
	// on every list view when the anime has no episodes aired yet.
	_ = p.cache.Set(ctx, cacheKey, all, episodesCacheTTL)
	p.markStage(health.StageEpisodes, nil)
	return all, nil
}

// ListServers scrapes the resolver's /play HTML response for kwik.cx
// button[data-src] URLs. Each match becomes one domain.Server with ID =
// raw kwik URL, Name = "kwik". The scheme check (T-27-02-01) + host filter
// + data-audio sub/dub derivation (CR-02) are preserved byte-for-byte
// from the pre-Phase-27 implementation.
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	body, err := p.resolver.Play(ctx, providerID, episodeID)
	if err != nil {
		p.markStage(health.StageServers, err)
		return nil, err
	}
	// Selector drift sentinel: an empty body is structurally distinct from a
	// healthy 200 page with zero buttons (real-empty).
	if len(strings.TrimSpace(body)) == 0 {
		err = domain.WrapExtractFailed(
			errors.New("/play response body is empty"),
			"animepahe: /play selector drift (empty body)",
		)
		p.markStage(health.StageServers, err)
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		err = domain.WrapExtractFailed(err, "animepahe: /play parse")
		p.markStage(health.StageServers, err)
		return nil, err
	}
	servers := make([]domain.Server, 0, 4)
	doc.Find("button[data-src]").Each(func(_ int, sel *goquery.Selection) {
		src, _ := sel.Attr("data-src")
		if src == "" {
			return
		}
		// WR-05 / T-27-02-01: reject any non-http(s) scheme up-front.
		// `url.Parse` accepts arbitrary schemes (e.g. `kwik://kwik.cx/`)
		// so a path-traversal-style embedURL could otherwise satisfy the
		// host filter and propagate to the orchestrator's extract step.
		pu, perr := url.Parse(src)
		if perr != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
			return
		}
		host := strings.ToLower(pu.Hostname())
		if host != "kwik.cx" && !strings.HasSuffix(host, ".kwik.cx") &&
			host != "kwik.si" && !strings.HasSuffix(host, ".kwik.si") {
			return
		}
		// CR-02: derive sub/dub from the surrounding `data-audio` attribute
		// (AnimePahe surfaces `jpn`/`eng` per kwik variant). Default to
		// CategorySub for safety — sub is the dominant case on AnimePahe and
		// an unknown attribute should not vanish from the frontend filter.
		audio, _ := sel.Attr("data-audio")
		cat := domain.CategorySub
		switch strings.ToLower(strings.TrimSpace(audio)) {
		case "eng", "dub", "english":
			cat = domain.CategoryDub
		}
		servers = append(servers, domain.Server{ID: src, Name: "kwik", Type: cat})
	})
	p.markStage(health.StageServers, nil)
	return servers, nil
}

// hostnameOf returns u.Hostname() for a URL string, or "" on parse failure.
func hostnameOf(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// GetStream delegates to the registry's extractor for the kwik URL and
// caches the result with TTL min(expires-30s, 5min). Already-expired URLs
// are NOT cached (a cached expired URL would just be a known-bad URL).
//
// WR-06: the `category` parameter is INFORMATIONAL on this provider —
// sub/dub selection happens at ListServers time (each kwik URL is tagged
// with its Server.Type derived from the play page's `data-audio` attribute).
// We accept the parameter to satisfy domain.Provider but do not branch on
// it: the caller has already picked a serverID whose audio matches their
// preference. Cache namespacing also ignores category for the same reason
// (the kwik URL is sufficient to disambiguate sub vs dub).
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	_ = category // informational only; see WR-06 note above.
	// Cache key: hash the serverID (kwik URL) for bounded length.
	h := sha256.Sum256([]byte(serverID))
	cacheKey := fmt.Sprintf("stream:%s:%s:%s:%s", providerName, providerID, episodeID, hex.EncodeToString(h[:8]))

	var cached domain.Stream
	if err := p.cache.Get(ctx, cacheKey, &cached); err == nil {
		p.markStage(health.StageStream, nil)
		return &cached, nil
	}

	ext, err := p.embeds.Find(serverID)
	if err != nil {
		err = domain.WrapExtractFailed(err, "animepahe: no matching extractor for "+serverID)
		p.markStage(health.StageStream, err)
		return nil, err
	}
	// Provide Referer = kwikReferer (https://animepahe.pw/) so the Kwik
	// upstream accepts the embed fetch (Kwik requires the parent-site
	// Referer chain; D2 alignment).
	headers := http.Header{"Referer": []string{kwikReferer}}
	stream, err := ext.Extract(ctx, serverID, headers)
	if err != nil {
		// Pass the error through; the extractor already wrapped it.
		p.markStage(health.StageStream, err)
		return nil, err
	}
	if stream == nil || len(stream.Sources) == 0 {
		err = domain.WrapExtractFailed(errors.New("empty stream"), "animepahe: extractor returned empty stream")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	// Cache decision: TTL = min(expires-30s, 5min) of the first source URL.
	ttl := computeStreamTTL(stream.Sources[0].URL, time.Now())
	if ttl > 0 {
		_ = p.cache.Set(ctx, cacheKey, *stream, ttl)
	}
	p.markStage(health.StageStream, nil)
	return stream, nil
}

// Compile-time assertion: *Provider satisfies domain.Provider.
var _ domain.Provider = (*Provider)(nil)
