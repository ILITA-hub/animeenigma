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
//   - FindID resolves an AnimeRef → AnimePahe anime ID via malsync.moe
//     (24h cache; 24h negative cache) with a Jaro-Winkler ≥ 0.85 fuzzy
//     fallback against /api?m=search results.
//   - ListEpisodes paginates /api?m=release with a 50-page hard cap, caching
//     the assembled list for 6h at key episodes:animepahe:{providerID}.
//   - ListServers scrapes /play/{anime}/{episode} for kwik.cx button[data-src]
//     URLs. Real-empty → []Server{} (NOT error); selector drift → ErrExtractFailed.
//   - GetStream looks up the kwik URL via the embeds.Registry and delegates
//     extraction. Stream URLs are cached with TTL min(expires-30s, 5min);
//     already-expired URLs are NOT cached.
//   - HealthCheck returns an in-memory snapshot of the four stage timings
//     (Phase 17 will extend this with real probes).
//
// DDoS-Guard handling: every upstream HTML/JSON fetch goes through
// getWithDDoSGuard, which on a 403+`Server: ddos-guard` response runs
// ensureDDoSCookie once and retries. The cookie persists in the
// BaseHTTPClient's jar so subsequent requests are clean (RESEARCH.md Pattern 3).
package animepahe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
type malSyncClient interface {
	Lookup(ctx context.Context, malID, provider string) (string, bool, error)
}

// Deps is the constructor input for New(). Every reference field must be
// non-nil except Log (a no-op fallback is constructed if absent).
type Deps struct {
	// BaseURL is the AnimePahe base URL (default https://animepahe.ru per
	// CONTEXT.md). Plan 16-05 wires this from ANIMEPAHE_BASE_URL.
	BaseURL string
	HTTP    *domain.BaseHTTPClient
	Embeds  *domain.Registry
	MalSync malSyncClient
	Cache   cache.Cache
	Log     *logger.Logger
}

// Provider implements domain.Provider for the AnimePahe upstream.
type Provider struct {
	baseURL string
	http    *domain.BaseHTTPClient
	embeds  *domain.Registry
	malsync malSyncClient
	cache   cache.Cache
	log     *logger.Logger

	// stages is the in-memory health snapshot, updated on each method call.
	// Phase 16 only requires the snapshot exist with the four canonical
	// stage keys; Phase 17 will extend with real probes.
	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs a Provider with sane defaults — empty BaseURL falls back to
// https://animepahe.ru. WR-11: required dependencies (HTTP, Embeds, MalSync,
// Cache) are validated eagerly and a non-nil error is returned if any is
// missing. main.go fatals on the error, so misconfiguration surfaces at
// boot rather than later as a confusing nil-pointer dereference 502.
// d.Log is optional and falls back to logger.Default().
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
	base := d.BaseURL
	if base == "" {
		base = "https://animepahe.ru"
	}
	p := &Provider{
		baseURL: strings.TrimRight(base, "/"),
		http:    d.HTTP,
		embeds:  d.Embeds,
		malsync: d.MalSync,
		cache:   d.Cache,
		log:     d.Log,
		stages:  make(map[string]domain.StageHealth, len(stageNames)),
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

// getWithDDoSGuard fetches urlStr, transparently handling a 403 + DDoS-Guard
// response: it runs ensureDDoSCookie() once and retries the request. Caller
// is responsible for closing the returned body.
func (p *Provider) getWithDDoSGuard(ctx context.Context, urlStr string) (*http.Response, error) {
	resp, err := p.http.Get(ctx, urlStr)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusForbidden || !strings.EqualFold(resp.Header.Get("Server"), "ddos-guard") {
		return resp, nil
	}
	// Drain the 403 body and close before retrying.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	target, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if err := ensureDDoSCookie(ctx, p.http, target); err != nil {
		return nil, err
	}
	return p.http.Get(ctx, urlStr)
}

// FindID resolves an AnimeRef → AnimePahe anime ID. First tries malsync.moe
// (positive + negative cache); falls back to /api?m=search with a Jaro-Winkler
// fuzzy match (threshold 0.85).
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	// 1. malsync hit?
	if ref.ShikimoriID != "" {
		if id, ok, err := p.malsync.Lookup(ctx, ref.ShikimoriID, providerName); err == nil && ok {
			p.markStage(health.StageSearch, nil)
			return id, nil
		}
	}
	// 2. Fuzzy /api?m=search fallback.
	if ref.Title == "" {
		err := domain.WrapNotFound(errors.New("no title"), "animepahe: cannot search without a title")
		p.markStage(health.StageSearch, err)
		return "", err
	}
	q := url.QueryEscape(ref.Title)
	searchURL := fmt.Sprintf("%s/api?m=search&q=%s", p.baseURL, q)
	resp, err := p.getWithDDoSGuard(ctx, searchURL)
	if err != nil {
		err = domain.WrapProviderDown(err, "animepahe: search fetch")
		p.markStage(health.StageSearch, err)
		return "", err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		err = domain.WrapProviderDown(fmt.Errorf("status %d", resp.StatusCode), "animepahe: search non-200")
		p.markStage(health.StageSearch, err)
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyAPI))
	if err != nil {
		err = domain.WrapProviderDown(err, "animepahe: search read body")
		p.markStage(health.StageSearch, err)
		return "", err
	}
	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		err = domain.WrapExtractFailed(err, "animepahe: search decode")
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

// ListEpisodes paginates /api?m=release for the given AnimePahe anime ID,
// caches the assembled list for 6 hours, and returns ([]Episode, nil) for
// the real-empty case.
func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	cacheKey := fmt.Sprintf("episodes:%s:%s", providerName, providerID)
	var cached []domain.Episode
	if err := p.cache.Get(ctx, cacheKey, &cached); err == nil {
		p.markStage(health.StageEpisodes, nil)
		return cached, nil
	}

	all := make([]domain.Episode, 0, 32)
	for page := 1; page <= maxEpisodePages; page++ {
		u := fmt.Sprintf("%s/api?m=release&id=%s&sort=episode_asc&page=%d", p.baseURL, url.PathEscape(providerID), page)
		resp, err := p.getWithDDoSGuard(ctx, u)
		if err != nil {
			err = domain.WrapProviderDown(err, "animepahe: release fetch")
			p.markStage(health.StageEpisodes, err)
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			err = domain.WrapProviderDown(
				fmt.Errorf("status %d, body=%q", resp.StatusCode, string(body)),
				fmt.Sprintf("animepahe: release page %d non-200", page),
			)
			p.markStage(health.StageEpisodes, err)
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyAPI))
		_ = resp.Body.Close()
		if err != nil {
			err = domain.WrapProviderDown(err, "animepahe: release read body")
			p.markStage(health.StageEpisodes, err)
			return nil, err
		}
		var rr releaseResponse
		if err := json.Unmarshal(body, &rr); err != nil {
			err = domain.WrapExtractFailed(err, "animepahe: release decode")
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

// ListServers scrapes /play/{anime}/{episode} for kwik.cx button[data-src]
// URLs. Each match becomes one domain.Server with ID = raw kwik URL, Name = "kwik".
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	u := fmt.Sprintf("%s/play/%s/%s", p.baseURL, url.PathEscape(providerID), url.PathEscape(episodeID))
	resp, err := p.getWithDDoSGuard(ctx, u)
	if err != nil {
		err = domain.WrapProviderDown(err, "animepahe: /play fetch")
		p.markStage(health.StageServers, err)
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		err = domain.WrapProviderDown(fmt.Errorf("status %d", resp.StatusCode), "animepahe: /play non-200")
		p.markStage(health.StageServers, err)
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyHTML))
	if err != nil {
		err = domain.WrapProviderDown(err, "animepahe: /play read body")
		p.markStage(health.StageServers, err)
		return nil, err
	}
	// Selector drift sentinel: an empty body is structurally distinct from a
	// healthy 200 page with zero buttons (real-empty).
	if len(strings.TrimSpace(string(body))) == 0 {
		err = domain.WrapExtractFailed(
			errors.New("/play response body is empty"),
			"animepahe: /play selector drift (empty body)",
		)
		p.markStage(health.StageServers, err)
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
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
		// WR-05: reject any non-http(s) scheme up-front. `url.Parse` accepts
		// arbitrary schemes (e.g. `kwik://kwik.cx/`) so a path-traversal-style
		// embedURL could otherwise satisfy the host filter and propagate to
		// the orchestrator's extract step.
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
	// Provide Referer = p.baseURL so the Kwik upstream accepts the embed
	// fetch (real Kwik requires the AnimePahe referrer chain).
	headers := http.Header{"Referer": []string{p.baseURL}}
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
