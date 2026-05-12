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
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
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
var stageNames = []string{"find_id", "list_episodes", "list_servers", "get_stream"}

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
// https://animepahe.ru, and missing dependencies result in panic at first
// use (we don't silently no-op because that would make production failures
// hard to debug).
func New(d Deps) *Provider {
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
	return p
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
			p.markStage("find_id", nil)
			return id, nil
		}
	}
	// 2. Fuzzy /api?m=search fallback.
	if ref.Title == "" {
		err := domain.WrapNotFound(errors.New("no title"), "animepahe: cannot search without a title")
		p.markStage("find_id", err)
		return "", err
	}
	q := url.QueryEscape(ref.Title)
	searchURL := fmt.Sprintf("%s/api?m=search&q=%s", p.baseURL, q)
	resp, err := p.getWithDDoSGuard(ctx, searchURL)
	if err != nil {
		err = domain.WrapProviderDown(err, "animepahe: search fetch")
		p.markStage("find_id", err)
		return "", err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		err = domain.WrapProviderDown(fmt.Errorf("status %d", resp.StatusCode), "animepahe: search non-200")
		p.markStage("find_id", err)
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyAPI))
	if err != nil {
		err = domain.WrapProviderDown(err, "animepahe: search read body")
		p.markStage("find_id", err)
		return "", err
	}
	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		err = domain.WrapExtractFailed(err, "animepahe: search decode")
		p.markStage("find_id", err)
		return "", err
	}
	if len(sr.Data) == 0 {
		err := domain.WrapNotFound(nil, "animepahe: 0 search results for "+ref.Title)
		p.markStage("find_id", err)
		return "", err
	}
	// 3. Score each entry; pick the best ≥ threshold.
	normTitle := normalizeTitle(ref.Title)
	best := struct {
		score   float64
		session string
	}{}
	for _, e := range sr.Data {
		score := jaroWinkler(normTitle, normalizeTitle(e.Title))
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
		p.markStage("find_id", err)
		return "", err
	}
	p.markStage("find_id", nil)
	return best.session, nil
}

// ListEpisodes paginates /api?m=release for the given AnimePahe anime ID,
// caches the assembled list for 6 hours, and returns ([]Episode, nil) for
// the real-empty case.
func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	cacheKey := fmt.Sprintf("episodes:%s:%s", providerName, providerID)
	var cached []domain.Episode
	if err := p.cache.Get(ctx, cacheKey, &cached); err == nil {
		p.markStage("list_episodes", nil)
		return cached, nil
	}

	all := make([]domain.Episode, 0, 32)
	for page := 1; page <= maxEpisodePages; page++ {
		u := fmt.Sprintf("%s/api?m=release&id=%s&sort=episode_asc&page=%d", p.baseURL, url.PathEscape(providerID), page)
		resp, err := p.getWithDDoSGuard(ctx, u)
		if err != nil {
			err = domain.WrapProviderDown(err, "animepahe: release fetch")
			p.markStage("list_episodes", err)
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			err = domain.WrapProviderDown(
				fmt.Errorf("status %d, body=%q", resp.StatusCode, string(body)),
				fmt.Sprintf("animepahe: release page %d non-200", page),
			)
			p.markStage("list_episodes", err)
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyAPI))
		_ = resp.Body.Close()
		if err != nil {
			err = domain.WrapProviderDown(err, "animepahe: release read body")
			p.markStage("list_episodes", err)
			return nil, err
		}
		var rr releaseResponse
		if err := json.Unmarshal(body, &rr); err != nil {
			err = domain.WrapExtractFailed(err, "animepahe: release decode")
			p.markStage("list_episodes", err)
			return nil, err
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
	p.markStage("list_episodes", nil)
	return all, nil
}

// ListServers scrapes /play/{anime}/{episode} for kwik.cx button[data-src]
// URLs. Each match becomes one domain.Server with ID = raw kwik URL, Name = "kwik".
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	u := fmt.Sprintf("%s/play/%s/%s", p.baseURL, url.PathEscape(providerID), url.PathEscape(episodeID))
	resp, err := p.getWithDDoSGuard(ctx, u)
	if err != nil {
		err = domain.WrapProviderDown(err, "animepahe: /play fetch")
		p.markStage("list_servers", err)
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		err = domain.WrapProviderDown(fmt.Errorf("status %d", resp.StatusCode), "animepahe: /play non-200")
		p.markStage("list_servers", err)
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyHTML))
	if err != nil {
		err = domain.WrapProviderDown(err, "animepahe: /play read body")
		p.markStage("list_servers", err)
		return nil, err
	}
	// Selector drift sentinel: an empty body is structurally distinct from a
	// healthy 200 page with zero buttons (real-empty).
	if len(strings.TrimSpace(string(body))) == 0 {
		err = domain.WrapExtractFailed(
			errors.New("/play response body is empty"),
			"animepahe: /play selector drift (empty body)",
		)
		p.markStage("list_servers", err)
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		err = domain.WrapExtractFailed(err, "animepahe: /play parse")
		p.markStage("list_servers", err)
		return nil, err
	}
	servers := make([]domain.Server, 0, 4)
	doc.Find("button[data-src]").Each(func(_ int, sel *goquery.Selection) {
		src, _ := sel.Attr("data-src")
		if src == "" {
			return
		}
		host := strings.ToLower(hostnameOf(src))
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
	p.markStage("list_servers", nil)
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
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	// Cache key: hash the serverID (kwik URL) for bounded length.
	h := sha256.Sum256([]byte(serverID))
	cacheKey := fmt.Sprintf("stream:%s:%s:%s:%s", providerName, providerID, episodeID, hex.EncodeToString(h[:8]))

	var cached domain.Stream
	if err := p.cache.Get(ctx, cacheKey, &cached); err == nil {
		p.markStage("get_stream", nil)
		return &cached, nil
	}

	ext, err := p.embeds.Find(serverID)
	if err != nil {
		err = domain.WrapExtractFailed(err, "animepahe: no matching extractor for "+serverID)
		p.markStage("get_stream", err)
		return nil, err
	}
	// Provide Referer = p.baseURL so the Kwik upstream accepts the embed
	// fetch (real Kwik requires the AnimePahe referrer chain).
	headers := http.Header{"Referer": []string{p.baseURL}}
	stream, err := ext.Extract(ctx, serverID, headers)
	if err != nil {
		// Pass the error through; the extractor already wrapped it.
		p.markStage("get_stream", err)
		return nil, err
	}
	if stream == nil || len(stream.Sources) == 0 {
		err = domain.WrapExtractFailed(errors.New("empty stream"), "animepahe: extractor returned empty stream")
		p.markStage("get_stream", err)
		return nil, err
	}
	// Cache decision: TTL = min(expires-30s, 5min) of the first source URL.
	ttl := computeStreamTTL(stream.Sources[0].URL, time.Now())
	if ttl > 0 {
		_ = p.cache.Set(ctx, cacheKey, *stream, ttl)
	}
	p.markStage("get_stream", nil)
	return stream, nil
}

// Compile-time assertion: *Provider satisfies domain.Provider.
var _ domain.Provider = (*Provider)(nil)
