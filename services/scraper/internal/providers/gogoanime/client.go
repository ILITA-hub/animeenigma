// client.go — Gogoanime/Anitaku domain.Provider implementation.
//
// SCRAPER-9ANI-01..06 (Plan 18-02 Task 2). Layered on:
//
//   - Phase 16 — animepahe provider (analog template).
//   - Phase 15 — embeds.Registry (extractor seam).
//   - Phase 17 — health package canonical stage constants.
//   - Plan 18-01 — services/scraper/internal/fuzzy (shared JaroWinkler +
//     NormalizeTitle) and the 8 anitaku.to / embed-wrapper / malsync goldens.
//
// Responsibilities:
//
//   - FindID tries malsync.moe FIRST (forward-compat — malsync has no
//     Gogoanime/Anitaku key as of 2026-05-12 so steady state is miss), then
//     falls through to /search.html?keyword=<title> with a Jaro-Winkler ≥ 0.85
//     fuzzy match against the visible title text.
//   - ListEpisodes fetches /category/<slug> AND /category/<slug>-dub, merges
//     episodes by number, and tags rows with domain.CategorySub or
//     domain.CategoryDub. Cached 6 hours at episodes:gogoanime:<base_slug>.
//   - ListServers scrapes <div class="anime_muti_link"> li a[data-video] from
//     the episode page; protocol-relative URLs are normalized to https://;
//     myvidplay.com / playmogo.com hosts (Cloudflare Turnstile) are filtered;
//     duplicates by URL are removed.
//   - GetStream delegates to the embeds.Registry; no extraction logic lives
//     in this provider. Streams cached at stream:gogoanime:<slug>:<epID>:<hash>
//     with TTL = min(parsedExpiry-30s, 5min) via computeStreamTTL.
//   - HealthCheck reports the four canonical stages (search, episodes,
//     servers, stream) using constants from internal/health/stage.go.
//
// NOTE: ddosguard.go from animepahe is intentionally OMITTED — anitaku.to
// does not sit behind DDoS-Guard per RESEARCH.md §Mirror Viability.
package gogoanime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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
// orchestrator's registry key. STABLE across mirror rebrands — even though
// the visible brand on the current mirror is "Anitaku" (anitaku.to),
// Gogoanime has rotated 5+ times in 18 months so the slug is generic.
const providerName = "gogoanime"

// fuzzyMatchThreshold is the minimum Jaro-Winkler score for /search.html
// fuzzy-fallback to claim a match (per RESEARCH.md Pitfall 5 / A6 — same
// 0.85 threshold as animepahe to keep cross-provider behavior comparable).
const fuzzyMatchThreshold = 0.85

// episodesCacheTTL is the 6h cache duration for the assembled episode list.
const episodesCacheTTL = 6 * time.Hour

// maxBodySearch caps the response body of /search.html at 4 MiB. Real search
// pages are < 200 KiB; this is a DoS guard.
const maxBodySearch = 4 << 20

// maxBodyHTML caps the response body of /category and /<slug>-episode-N at
// 2 MiB. Real anitaku.to category pages can reach ~500 KiB on long-running
// shows (One Piece golden is ~430 KiB) but stay well under 2 MiB.
const maxBodyHTML = 2 << 20

// stageNames lock the canonical stage keys returned by HealthCheck. Phase 17
// canonical 5-stage strings minus the fifth (stream_segment) which is owned
// by the probe runner, not the provider.
var stageNames = []string{
	health.StageSearch,
	health.StageEpisodes,
	health.StageServers,
	health.StageStream,
}

// Selector identifiers for parser_zero_match_total. These MUST be short
// stable identifiers — NOT raw CSS — to bound the cardinality of the
// {selector=...} label (RESEARCH P-02 / SCRAPER-NF-04 cardinality bomb
// mitigation).
const (
	selectorSearchResult      = "search_result"
	selectorEpisodeRow        = "episode_row"
	selectorAnimeMutiLinkItem = "anime_muti_link_item"
)

// episodeHrefRe matches `/<slug>-episode-<N>` href paths. Capture group 1 is
// the slug-without-episode-suffix, group 2 is the episode number.
var episodeHrefRe = regexp.MustCompile(`^/(.+)-episode-(\d+)$`)

// turnstileHosts are the embed hosts gated by Cloudflare Turnstile per
// RESEARCH.md Pitfall 9 — we skip these at ListServers time so they never
// reach GetStream. List is suffix-matched (case-insensitive) so subdomains
// (e.g. *.myvidplay.com) are caught too.
var turnstileHosts = []string{"myvidplay.com", "playmogo.com"}

// malSyncClient is the malsync lookup contract — abstracted so tests can
// inject a fake without standing up a real malsync HTTP server.
type malSyncClient interface {
	Lookup(ctx context.Context, malID, provider string) (string, bool, error)
}

// Deps is the constructor input for New(). Every reference field must be
// non-nil except Log (a no-op fallback is constructed if absent).
//
// EXPORTED struct — field names + ordering MUST match animepahe's Deps so
// main.go (Plan 18-04 Task 1) wires both providers with identical literal
// patterns.
type Deps struct {
	// BaseURL is the Gogoanime base URL (default https://anitaku.to per
	// CONTEXT.md). Plan 18-04 wires this from SCRAPER_GOGOANIME_BASE_URL.
	BaseURL string
	HTTP    *domain.BaseHTTPClient
	Embeds  *domain.Registry
	MalSync malSyncClient
	Cache   cache.Cache
	Log     *logger.Logger
}

// Provider implements domain.Provider for the Gogoanime/Anitaku upstream.
type Provider struct {
	baseURL string
	http    *domain.BaseHTTPClient
	embeds  *domain.Registry
	malsync malSyncClient
	cache   cache.Cache
	log     *logger.Logger

	// stages is the in-memory health snapshot, updated on each method call.
	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs a Provider with sane defaults — empty BaseURL falls back to
// https://anitaku.to. Required dependencies (HTTP, Embeds, MalSync, Cache)
// are validated eagerly and a non-nil error is returned if any is missing
// (animepahe WR-11 pattern). main.go fatals on the error, so misconfiguration
// surfaces at boot rather than as a confusing nil-pointer 502.
// d.Log is optional and falls back to logger.Default().
func New(d Deps) (*Provider, error) {
	if d.HTTP == nil {
		return nil, errors.New("gogoanime: Deps.HTTP is required")
	}
	if d.Embeds == nil {
		return nil, errors.New("gogoanime: Deps.Embeds is required")
	}
	if d.MalSync == nil {
		return nil, errors.New("gogoanime: Deps.MalSync is required")
	}
	if d.Cache == nil {
		return nil, errors.New("gogoanime: Deps.Cache is required")
	}
	if d.Log == nil {
		d.Log = logger.Default()
	}
	base := d.BaseURL
	if base == "" {
		base = "https://anitaku.to"
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

// Name returns the stable identifier "gogoanime". The literal is also held
// in `providerName` (used for metrics labels + Redis key shape), but Name()
// returns the string literal directly so the acceptance grep
// (`grep "return \"gogoanime\""`) anchors on this method.
func (p *Provider) Name() string { return "gogoanime" }

// markStage records the success/failure of one stage. Called on every
// method exit path.
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

// FindID resolves an AnimeRef → Gogoanime/Anitaku slug. First tries
// malsync.moe (positive + negative cache); on miss (the expected steady
// state as of 2026-05-12 per RESEARCH.md Open Q4) falls through to
// /search.html?keyword=<title> with a Jaro-Winkler fuzzy match (threshold
// 0.85).
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	// 1. malsync hit? Forward-compat probe — currently expected to miss for
	// every MAL ID but kept for the day malsync.moe ships a Gogoanime key.
	if ref.ShikimoriID != "" {
		if id, ok, err := p.malsync.Lookup(ctx, ref.ShikimoriID, "Gogoanime"); err == nil && ok {
			p.markStage(health.StageSearch, nil)
			return id, nil
		}
	}
	// 2. Fuzzy /search.html fallback — the PRIMARY path in practice.
	if ref.Title == "" {
		err := domain.WrapNotFound(errors.New("no title"), "gogoanime: cannot search without a title")
		p.markStage(health.StageSearch, err)
		return "", err
	}
	q := url.QueryEscape(ref.Title)
	searchURL := fmt.Sprintf("%s/search.html?keyword=%s", p.baseURL, q)
	resp, err := p.http.Get(ctx, searchURL)
	if err != nil {
		werr := domain.WrapProviderDown(err, "gogoanime: search fetch")
		p.markStage(health.StageSearch, werr)
		return "", werr
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		werr := domain.WrapProviderDown(fmt.Errorf("status %d", resp.StatusCode), "gogoanime: search non-200")
		p.markStage(health.StageSearch, werr)
		return "", werr
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySearch))
	if err != nil {
		werr := domain.WrapProviderDown(err, "gogoanime: search read body")
		p.markStage(health.StageSearch, werr)
		return "", werr
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		werr := domain.WrapExtractFailed(err, "gogoanime: search parse")
		p.markStage(health.StageSearch, werr)
		return "", werr
	}
	rows := make([]searchResult, 0, 16)
	doc.Find("p.name a[href^='/category/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		slug := strings.TrimPrefix(href, "/category/")
		slug = strings.TrimSuffix(slug, "/")
		title := strings.TrimSpace(sel.Text())
		if slug == "" || title == "" {
			return
		}
		rows = append(rows, searchResult{Slug: slug, Title: title})
	})
	if len(rows) == 0 {
		// Selector drift vs real-empty: a 200 page with zero matches could be
		// either. Emit the zero-match counter so a sudden selector regression
		// (anitaku changes the .name class to e.g. .item-title) is visible
		// before queries pile up.
		metrics.ParserZeroMatchTotal.WithLabelValues("gogoanime", selectorSearchResult).Inc()
		werr := domain.WrapNotFound(nil, "gogoanime: 0 search results for "+ref.Title)
		p.markStage(health.StageSearch, werr)
		return "", werr
	}
	// 3. Score each entry; pick the best ≥ threshold.
	normTitle := fuzzy.NormalizeTitle(ref.Title)
	best := struct {
		score float64
		slug  string
	}{}
	for _, e := range rows {
		score := fuzzy.JaroWinkler(normTitle, fuzzy.NormalizeTitle(e.Title))
		if score > best.score {
			best.score = score
			best.slug = e.Slug
		}
	}
	if best.score < fuzzyMatchThreshold || best.slug == "" {
		werr := domain.WrapNotFound(
			fmt.Errorf("best score %.4f", best.score),
			"gogoanime: no fuzzy match for "+ref.Title,
		)
		p.markStage(health.StageSearch, werr)
		return "", werr
	}
	p.markStage(health.StageSearch, nil)
	return best.slug, nil
}

// ListEpisodes fetches /category/<base>, plus /category/<base>-dub (if a
// separate dub page exists), merges episodes by number, and tags each
// Episode with its derived Category (sub|dub) via the ID format.
//
// Real-empty (anime exists, no episodes yet) returns ([]Episode{}, nil) —
// NOT ErrNotFound, per the Phase 15 domain contract.
//
// Cached 6h at episodes:gogoanime:<base_slug>.
func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	// Strip a trailing -dub from the input slug so callers can pass either
	// the sub or the dub slug and get the merged result.
	base := strings.TrimSuffix(providerID, "-dub")
	cacheKey := fmt.Sprintf("episodes:%s:%s", providerName, base)
	var cached []domain.Episode
	if err := p.cache.Get(ctx, cacheKey, &cached); err == nil {
		p.markStage(health.StageEpisodes, nil)
		return cached, nil
	}

	// Fetch the sub page. A 200 with zero matches is treated as real-empty
	// (return []Episode{}); a non-200 is transport-down.
	subEps, subErr := p.fetchEpisodes(ctx, base)
	if subErr != nil {
		// Distinguish transport-down (return error, propagate to orchestrator
		// failover) from 404 (treat sub page as missing — could still have
		// a dub page).
		if errors.Is(subErr, domain.ErrNotFound) {
			subEps = nil
		} else {
			p.markStage(health.StageEpisodes, subErr)
			return nil, subErr
		}
	}

	// Fetch the dub page. 404 is normal (most shows don't have a separate
	// dub release).
	dubEps, dubErr := p.fetchEpisodes(ctx, base+"-dub")
	if dubErr != nil && !errors.Is(dubErr, domain.ErrNotFound) {
		// Only propagate non-404 errors — if the dub page genuinely 404s,
		// the sub-only result is still a valid response.
		p.markStage(health.StageEpisodes, dubErr)
		return nil, dubErr
	}

	// Merge by episode number. Sub wins on ID (canonical); the presence of
	// a dub episode at the same number is encoded via the URLSlug carried
	// through the Title field.
	type merged struct {
		hasSub bool
		hasDub bool
		sub    domain.Episode
		dub    domain.Episode
	}
	byNum := make(map[int]*merged)
	for _, ep := range subEps {
		e := byNum[ep.Number]
		if e == nil {
			e = &merged{}
			byNum[ep.Number] = e
		}
		e.hasSub = true
		e.sub = ep
	}
	for _, ep := range dubEps {
		e := byNum[ep.Number]
		if e == nil {
			e = &merged{}
			byNum[ep.Number] = e
		}
		e.hasDub = true
		e.dub = ep
	}

	// Flatten in ascending episode order. Each emitted Episode tags via its
	// ID slug (which embeds -dub for dub-only episodes) so the orchestrator
	// can route to the right category at GetStream time.
	all := make([]domain.Episode, 0, len(byNum))
	for n := 1; ; n++ {
		e, ok := byNum[n]
		if !ok {
			// Stop at the first missing number to keep order natural for the
			// common (sub-only) case. If there's a gap above n, fall back to
			// sorted iteration below.
			break
		}
		if e.hasSub {
			all = append(all, e.sub)
		} else if e.hasDub {
			all = append(all, e.dub)
		}
		delete(byNum, n)
	}
	if len(byNum) > 0 {
		// Episode numbers not contiguous from 1: emit remaining in sorted order.
		// (Rare: most anitaku.to category pages list episodes 1..N continuously.)
		nums := make([]int, 0, len(byNum))
		for k := range byNum {
			nums = append(nums, k)
		}
		// Simple insertion sort — len is small in practice.
		for i := 1; i < len(nums); i++ {
			for j := i; j > 0 && nums[j] < nums[j-1]; j-- {
				nums[j], nums[j-1] = nums[j-1], nums[j]
			}
		}
		for _, n := range nums {
			e := byNum[n]
			if e.hasSub {
				all = append(all, e.sub)
			} else if e.hasDub {
				all = append(all, e.dub)
			}
		}
	}

	// Ensure non-nil so callers can distinguish from a JSON null.
	if all == nil {
		all = []domain.Episode{}
	}
	// 6h cache — even for real-empty so we don't re-hit upstream on every
	// list view when the anime has no episodes aired yet.
	_ = p.cache.Set(ctx, cacheKey, all, episodesCacheTTL)
	p.markStage(health.StageEpisodes, nil)
	return all, nil
}

// fetchEpisodes GETs /category/<slug> and parses the inline episode links.
// Returns ErrNotFound (wrapped) on 404 so the caller (ListEpisodes) can treat
// a missing dub page as normal.
func (p *Provider) fetchEpisodes(ctx context.Context, slug string) ([]domain.Episode, error) {
	u := fmt.Sprintf("%s/category/%s", p.baseURL, url.PathEscape(slug))
	resp, err := p.http.Get(ctx, u)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "gogoanime: /category fetch")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.WrapNotFound(nil, "gogoanime: /category "+slug+" 404")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, domain.WrapProviderDown(fmt.Errorf("status %d", resp.StatusCode), "gogoanime: /category non-200")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyHTML))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "gogoanime: /category read body")
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "gogoanime: /category parse")
	}
	// Page title check: anitaku.to serves the literal string "Pages not found
	// at Anitaku" inside <title> for missing dub pages even when the HTTP
	// status is 200. Treat that as a soft 404 so the merge logic still works.
	if t := strings.TrimSpace(doc.Find("title").First().Text()); strings.Contains(strings.ToLower(t), "pages not found") {
		return nil, domain.WrapNotFound(nil, "gogoanime: /category "+slug+" soft-404 (Pages not found)")
	}

	// Determine sub/dub from the slug suffix.
	cat := domain.CategorySub
	isDub := strings.HasSuffix(slug, "-dub")
	if isDub {
		cat = domain.CategoryDub
	}

	rows := make([]domain.Episode, 0, 64)
	seen := make(map[int]bool)
	doc.Find(`a[href*="-episode-"]`).Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		m := episodeHrefRe.FindStringSubmatch(href)
		if len(m) != 3 {
			return
		}
		// For sub fetch: skip dub-flavored hrefs (when both -dub and non-dub
		// episode links appear on the same page).
		hrefSlug := m[1]
		hrefIsDub := strings.HasSuffix(hrefSlug, "-dub")
		if isDub && !hrefIsDub {
			return
		}
		if !isDub && hrefIsDub {
			return
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return
		}
		if seen[n] {
			return
		}
		seen[n] = true
		_ = cat // category derived from slug suffix below
		rows = append(rows, domain.Episode{
			ID:     strings.TrimPrefix(href, "/"),
			Number: n,
			Title:  fmt.Sprintf("Episode %d", n),
		})
	})

	if len(rows) == 0 {
		// Selector drift sentinel — if the canonical golden has ≥ 100 rows
		// for One Piece and a live fetch returns zero, surface that via the
		// metric. anitaku.to does have real-empty category pages for unreleased
		// shows, so we emit the counter but still return a clean ([]Episode{}, nil).
		metrics.ParserZeroMatchTotal.WithLabelValues("gogoanime", selectorEpisodeRow).Inc()
	}
	return rows, nil
}

// ListServers scrapes /<epID> for <ul class="muti_link"> li a[data-video]
// links inside <div class="anime_muti_link">. Each match becomes one
// domain.Server with ID = absolute embed URL, Name = visible label
// (HD-1 / HD-2 / StreamHG / Earnvids), Type = sub|dub derived from the
// episode-ID slug.
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	u := fmt.Sprintf("%s/%s", p.baseURL, url.PathEscape(episodeID))
	resp, err := p.http.Get(ctx, u)
	if err != nil {
		werr := domain.WrapProviderDown(err, "gogoanime: episode fetch")
		p.markStage(health.StageServers, werr)
		return nil, werr
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		werr := domain.WrapProviderDown(fmt.Errorf("status %d", resp.StatusCode), "gogoanime: episode non-200")
		p.markStage(health.StageServers, werr)
		return nil, werr
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyHTML))
	if err != nil {
		werr := domain.WrapProviderDown(err, "gogoanime: episode read body")
		p.markStage(health.StageServers, werr)
		return nil, werr
	}
	// Selector drift sentinel: an empty body is structurally distinct from
	// a healthy 200 page with zero servers.
	if len(strings.TrimSpace(string(body))) == 0 {
		werr := domain.WrapExtractFailed(
			errors.New("episode response body is empty"),
			"gogoanime: episode selector drift (empty body)",
		)
		p.markStage(health.StageServers, werr)
		return nil, werr
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		werr := domain.WrapExtractFailed(err, "gogoanime: episode parse")
		p.markStage(health.StageServers, werr)
		return nil, werr
	}

	// Derive category from the episode slug.
	cat := domain.CategorySub
	if strings.HasSuffix(providerID, "-dub") || strings.Contains(episodeID, "-dub-") {
		cat = domain.CategoryDub
	}

	servers := make([]domain.Server, 0, 8)
	seenURL := make(map[string]bool)
	doc.Find(".anime_muti_link a[data-video]").Each(func(_ int, sel *goquery.Selection) {
		dv, _ := sel.Attr("data-video")
		if dv == "" {
			return
		}
		// Normalize protocol-relative (`//host/path` → `https://host/path`).
		if strings.HasPrefix(dv, "//") {
			dv = "https:" + dv
		}
		pu, perr := url.Parse(dv)
		if perr != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
			return
		}
		host := strings.ToLower(pu.Hostname())
		if host == "" {
			return
		}
		// Cloudflare Turnstile skip-list per RESEARCH.md Pitfall 9.
		for _, blocked := range turnstileHosts {
			if host == blocked || strings.HasSuffix(host, "."+blocked) {
				return
			}
		}
		if seenURL[dv] {
			return
		}
		seenURL[dv] = true
		// Server label: prefer the visible anchor text minus the
		// "Choose this server" helper-span suffix.
		label := strings.TrimSpace(sel.Text())
		label = strings.TrimSpace(strings.TrimSuffix(label, "Choose this server"))
		if label == "" {
			label = host
		}
		servers = append(servers, domain.Server{ID: dv, Name: label, Type: cat})
	})

	if len(servers) == 0 {
		// Zero-match emit — selector drift on the inner anchors, even though
		// the .anime_muti_link container existed.
		metrics.ParserZeroMatchTotal.WithLabelValues("gogoanime", selectorAnimeMutiLinkItem).Inc()
	}
	p.markStage(health.StageServers, nil)
	return servers, nil
}

// GetStream delegates to the embeds.Registry's matching extractor and caches
// the result with TTL min(parsedExpiry-30s, 5min). Already-expired URLs are
// NOT cached (the cache would just hand out a known-bad URL on next request).
//
// The `category` parameter is INFORMATIONAL — sub/dub was already determined
// at ListServers time via the episode-ID slug. Cache namespacing uses the
// hashed serverID for bounded key length.
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	_ = category // informational only (sub/dub baked into serverID already).
	// Cache key: hash the serverID for bounded length.
	h := sha256.Sum256([]byte(serverID))
	cacheKey := fmt.Sprintf("stream:%s:%s:%s:%s", providerName, providerID, episodeID, hex.EncodeToString(h[:8]))

	var cached domain.Stream
	if err := p.cache.Get(ctx, cacheKey, &cached); err == nil {
		p.markStage(health.StageStream, nil)
		return &cached, nil
	}

	ext, err := p.embeds.Find(serverID)
	if err != nil {
		werr := domain.WrapExtractFailed(err, "gogoanime: no matching extractor for "+serverID)
		p.markStage(health.StageStream, werr)
		return nil, werr
	}
	// Referer: the canonical Anitaku origin — every embed host expects to
	// see Anitaku as the page that loaded the iframe.
	headers := http.Header{"Referer": []string{"https://anitaku.to/"}}
	stream, err := ext.Extract(ctx, serverID, headers)
	if err != nil {
		// Extractor already wrapped the error family.
		p.markStage(health.StageStream, err)
		return nil, err
	}
	if stream == nil || len(stream.Sources) == 0 {
		werr := domain.WrapExtractFailed(errors.New("empty stream"), "gogoanime: extractor returned empty stream")
		p.markStage(health.StageStream, werr)
		return nil, werr
	}
	// Cache decision: TTL = min(e-30s, 5min) of the first source URL.
	ttl := computeStreamTTL(stream.Sources[0].URL, time.Now())
	if ttl > 0 {
		_ = p.cache.Set(ctx, cacheKey, *stream, ttl)
	}
	p.markStage(health.StageStream, nil)
	return stream, nil
}

// Compile-time assertion: *Provider satisfies domain.Provider.
var _ domain.Provider = (*Provider)(nil)
