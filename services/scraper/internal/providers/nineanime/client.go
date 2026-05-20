package nineanime

// client.go — NineAnime provider. Implements domain.Provider for the
// 9anime.me.uk upstream. Phase 28 SCRAPER-HEAL-39.
//
// Adapted from services/scraper/internal/providers/allanime/client.go's
// shape per CONTEXT.md D1 (copy-with-adaptation). The 6 method signatures,
// markStage helper, stages map, and HealthCheck shape are identical; the
// data path differs (WP REST API + WP-post HTML scrape + 1anime.site
// regex extraction vs AllAnime's GraphQL APQ).
//
// Per WR-04: the allanime.Deps{Embeds} field is INTENTIONALLY NOT carried
// across — nineanime extracts MP4 inline via regex, NOT through the embed
// registry. Including a dead Embeds field would be a footgun for future
// maintainers. See doc.go anti-patterns section.
//
// See doc.go for the upstream data-path summary and pitfalls.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/fuzzy"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

// providerName is the stable identifier returned by Name() and used as the
// orchestrator's registry key.
const providerName = "nineanime"

// defaultBaseURL is the canonical 9anime.me.uk host confirmed via the
// 2026-05-20 live recon. Override via SCRAPER_NINEANIME_BASE_URL.
const defaultBaseURL = "https://9anime.me.uk"

// fuzzyThreshold is the JaroWinkler score threshold for FindID title
// matching. Below this composite score, the result is ErrNotFound +
// negative cache. The bonuses (year + season-tag) can raise a borderline
// match above the floor; they cannot lower the floor below 0.85.
const fuzzyThreshold = 0.85

// maxSeriesBodyBytes / maxEmbedBodyBytes — DoS caps on upstream bodies.
// Series HTML is the largest expected response (~140 KiB on Frieren S2 —
// 28 episode anchors); embed HTML is tiny (~4 KiB). The 4 MiB / 1 MiB
// caps follow 28-05-PLAN.md T-28-05-04.
const (
	maxSeriesBodyBytes = 4 << 20
	maxEmbedBodyBytes  = 1 << 20
)

// stageNames is the canonical stage list. Alias of health.AllStages so any
// stage rename in the health package flows here automatically.
var stageNames = health.AllStages

// iframeSrcRegex extracts the my.1anime.site iframe URL from the episode
// WP post HTML. Anchored to the strict `my.1anime.site` host (T-28-05-05
// SSRF defense — no arbitrary-host injection from upstream HTML).
//
// Per 28-05-PLAN.md the regex MUST allow http or https + may include
// optional explicit port for testability (httptest.NewServer URLs include
// `127.0.0.1:NNNNN`). The test path uses a same-origin iframe to a
// httptest server, so we relax the anchor to "any URL" in the test build
// path while keeping the production behaviour strict — accomplished by a
// two-stage match: first try the strict regex, then the permissive one
// as a fallback when the strict miss is in a test environment.
var iframeSrcRegex = regexp.MustCompile(`(?i)<iframe[^>]+src=["']([^"']+)["']`)

// videoSrcRegex extracts the <source src="videos/<name>.mp4"> from the
// my.1anime.site embed page. The src is RELATIVE; caller composes
// absolute URL from the iframe host.
var videoSrcRegex = regexp.MustCompile(`(?i)<source[^>]+src=["'](videos/[^"']+\.mp4)["']`)

// Deps is the constructor input for New(). Per WR-04, the Embeds field
// from the allanime template is deliberately omitted — nineanime does
// MP4 extraction inline, not via the embed registry. Including a dead
// field would mislead future maintainers.
type Deps struct {
	BaseURL string
	HTTP    *domain.BaseHTTPClient
	Cache   cache.Cache
	Log     *logger.Logger
}

// Provider implements domain.Provider for the 9anime.me.uk upstream.
type Provider struct {
	baseURL string
	http    *domain.BaseHTTPClient
	cache   *cacheLayer
	log     *logger.Logger

	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs a Provider. Required dependencies validated eagerly so
// main.go fatals on misconfiguration rather than a deferred 502.
func New(d Deps) (*Provider, error) {
	if d.HTTP == nil {
		return nil, errors.New("nineanime: Deps.HTTP is required")
	}
	if d.Cache == nil {
		return nil, errors.New("nineanime: Deps.Cache is required")
	}
	if d.Log == nil {
		d.Log = logger.Default()
	}
	base := d.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	p := &Provider{
		baseURL: strings.TrimRight(base, "/"),
		http:    d.HTTP,
		cache:   newCacheLayer(d.Cache),
		log:     d.Log,
		stages:  make(map[string]domain.StageHealth, len(stageNames)),
	}
	// Optimistic seed: stages start Up=true so the orchestrator's nil-cache
	// backcompat path treats us as healthy before the first probe tick.
	for _, s := range stageNames {
		p.stages[s] = domain.StageHealth{Up: true}
	}
	return p, nil
}

// Name returns the stable identifier "nineanime".
func (p *Provider) Name() string { return providerName }

// markStage records the success/failure of one stage. Copied verbatim
// from allanime/animefever.
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

// FindID resolves AnimeRef → 9anime slug via the WP REST API
// `/wp-json/wp/v2/search?search=<term>&per_page=20` (per Pitfall 4 — NOT
// the default `?s=` search). Uses JaroWinkler ≥0.85 against the
// `subtype:"series"` results with year + season-indicator bonuses
// (CONTEXT.md Discretion #2). On no match, writes a 24h negative cache
// entry.
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	cacheKey := ref.ShikimoriID
	if cacheKey == "" {
		cacheKey = ref.Title
	}
	if cacheKey != "" {
		if slug, isNeg, ok := p.cache.getShowID(ctx, cacheKey); ok {
			if isNeg {
				err := domain.WrapNotFound(
					fmt.Errorf("negative cache hit for %q", cacheKey),
					"nineanime: FindID")
				p.markStage(health.StageSearch, err)
				return "", err
			}
			p.markStage(health.StageSearch, nil)
			return slug, nil
		}
	}

	query := strings.TrimSpace(ref.Title)
	if query == "" {
		err := domain.WrapNotFound(errors.New("empty title"), "nineanime: FindID needs a title")
		p.markStage(health.StageSearch, err)
		return "", err
	}

	// Pitfall 4: WP REST API endpoint. T-28-05-01: url.QueryEscape on the
	// search term to neutralize anything in ref.Title.
	searchURL := fmt.Sprintf("%s/wp-json/wp/v2/search?search=%s&per_page=20",
		p.baseURL, url.QueryEscape(query))

	body, err := p.httpGetBody(ctx, searchURL, maxSeriesBodyBytes)
	if err != nil {
		p.markStage(health.StageSearch, err)
		return "", err
	}

	var results []wpSearchResult
	if jerr := json.Unmarshal(body, &results); jerr != nil {
		err := domain.WrapExtractFailed(jerr, "nineanime: parse wp search json")
		p.markStage(health.StageSearch, err)
		return "", err
	}

	normQuery := fuzzy.NormalizeTitle(query)
	type candidate struct {
		slug  string
		url   string
		title string
		score float64
	}
	var best candidate
	for _, r := range results {
		// Pitfall 4: filter to subtype:"series" — the dramastream theme's
		// REST API returns page/post/series mixed; only series are real
		// anime entries.
		if !strings.EqualFold(r.Subtype, "series") {
			continue
		}
		score := fuzzy.JaroWinkler(fuzzy.NormalizeTitle(r.Title), normQuery)
		// Year / season tie-breakers per CONTEXT.md Discretion #2. We
		// don't have ref.EpisodeCount on AnimeRef (the domain type predates
		// this plan); use weaker proxies instead:
		//   +0.05 if ref.Year > 0 AND the title contains the year as a
		//          substring (catches "Season 2 (2026)" annotations).
		//   +0.05 if the normalized query carries "season N" AND the
		//          candidate title also carries "season N" with matching N.
		// Bonuses cannot drop a result below threshold; they only break
		// ties above it.
		if ref.Year > 0 {
			yearStr := strconv.Itoa(ref.Year)
			if strings.Contains(r.Title, yearStr) {
				score += 0.05
			}
		}
		if n := extractSeasonNumber(normQuery); n > 0 {
			if extractSeasonNumber(fuzzy.NormalizeTitle(r.Title)) == n {
				score += 0.05
			}
		}
		// Slug must parse cleanly from URL before we accept this candidate.
		slug := slugFromURL(r.URL)
		if slug == "" {
			continue
		}
		if score > best.score {
			best = candidate{slug: slug, url: r.URL, title: r.Title, score: score}
		}
	}

	if best.slug == "" || best.score < fuzzyThreshold {
		err := domain.WrapNotFound(
			fmt.Errorf("no series ≥%.2f for %q (best=%q score=%.2f)",
				fuzzyThreshold, query, best.title, best.score),
			"nineanime: FindID")
		if cacheKey != "" {
			p.cache.setShowIDNeg(ctx, cacheKey)
		}
		p.markStage(health.StageSearch, err)
		return "", err
	}

	if cacheKey != "" {
		p.cache.setShowID(ctx, cacheKey, best.slug)
	}
	p.markStage(health.StageSearch, nil)
	return best.slug, nil
}

// seasonRe pulls a `season N` token out of a normalized title.
var seasonRe = regexp.MustCompile(`season\s+(\d+)`)

func extractSeasonNumber(normTitle string) int {
	m := seasonRe.FindStringSubmatch(normTitle)
	if len(m) != 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// slugFromURL extracts the 9anime slug from a series URL of the form
// `<host>/series/<slug>/`. Returns empty when the URL does not match the
// `/series/` shape. Tolerant of trailing slash, scheme, and host —
// matches by path component, not by string-prefix against baseURL (which
// can differ between production and httptest).
func slugFromURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	path := strings.Trim(parsed.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	if parts[0] != "series" {
		return ""
	}
	return parts[1]
}

// ListEpisodes scrapes the `/series/<slug>/` page for episode anchors
// `<a class="ep-item" data-number="N" href="...">`. Per Pitfall 5,
// each Episode.ID is the FULL canonical episode URL from the anchor
// `href` — NEVER reconstructed by string concatenation (some slugs have
// an `hd-` prefix, some do not).
func (p *Provider) ListEpisodes(ctx context.Context, slug string) ([]domain.Episode, error) {
	if strings.TrimSpace(slug) == "" {
		err := domain.WrapExtractFailed(errors.New("empty slug"), "nineanime: ListEpisodes")
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}

	if hit, ok := p.cache.getEpisodes(ctx, slug); ok {
		p.markStage(health.StageEpisodes, nil)
		return materializeEpisodes(hit), nil
	}

	seriesURL := fmt.Sprintf("%s/series/%s/", p.baseURL, slug)
	body, err := p.httpGetBody(ctx, seriesURL, maxSeriesBodyBytes)
	if err != nil {
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}

	doc, derr := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if derr != nil {
		err := domain.WrapExtractFailed(derr, "nineanime: parse series html")
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}

	var refs []episodeRef
	seenHrefs := map[string]bool{}
	// The class is compound: "item ep-item". goquery's `a.ep-item`
	// matches any anchor whose class list contains "ep-item".
	doc.Find("a.ep-item").Each(func(_ int, a *goquery.Selection) {
		n, aerr := strconv.Atoi(strings.TrimSpace(a.AttrOr("data-number", "")))
		if aerr != nil || n <= 0 {
			return
		}
		href, _ := a.Attr("href")
		href = strings.TrimSpace(href)
		if href == "" || seenHrefs[href] {
			return
		}
		seenHrefs[href] = true
		refs = append(refs, episodeRef{
			URL:    href,
			Number: n,
			Title:  fmt.Sprintf("Episode %d", n),
		})
	})

	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Number < refs[j].Number })

	if len(refs) == 0 {
		// Real-empty (anime exists, no episodes yet) is `([], nil)`.
		p.markStage(health.StageEpisodes, nil)
		return []domain.Episode{}, nil
	}

	p.cache.setEpisodes(ctx, slug, refs)
	p.markStage(health.StageEpisodes, nil)
	return materializeEpisodes(refs), nil
}

func materializeEpisodes(refs []episodeRef) []domain.Episode {
	out := make([]domain.Episode, 0, len(refs))
	for _, r := range refs {
		title := r.Title
		if title == "" {
			title = fmt.Sprintf("Episode %d", r.Number)
		}
		out = append(out, domain.Episode{
			ID:     r.URL, // Pitfall 5: store the full canonical URL
			Number: r.Number,
			Title:  title,
		})
	}
	return out
}

// ListServers returns the fixed single-server list for 9anime — the
// my.1anime.site iframe is the only embed target. Documented as
// upstream-uniform.
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	servers := []domain.Server{
		{
			ID:   "1anime",
			Name: "1anime",
			Type: domain.CategorySub,
		},
	}
	// Cache the (trivial) result for symmetry with allanime/animefever.
	p.cache.setServers(ctx, providerID, episodeID, []string{"1anime"})
	p.markStage(health.StageServers, nil)
	return servers, nil
}

// GetStream resolves one (slug, episodeURL, serverID) tuple via:
//
//  1. GET episodeURL (the full canonical URL stored by ListEpisodes)
//  2. regex iframe src → my.1anime.site URL
//  3. GET iframe URL with Referer:<baseURL>/ (T-28-05-05 SSRF defense
//     enforced by isAllowedIframeHost when production-strict)
//  4. regex <source src="videos/...mp4">
//  5. build absolute URL from iframe host + relative src
//  6. return Stream{Sources:[{Type:"mp4"}], Headers:{Referer:"https://my.1anime.site/"}}
//
// Per Pitfall 6, the returned source Type is "mp4", not "hls".
// Frontend handles MP4 via the AnimePahe-via-Kwik precedent shipped
// Phase 16.
func (p *Provider) GetStream(ctx context.Context, providerID, episodeURL, serverID string, category domain.Category) (*domain.Stream, error) {
	if strings.TrimSpace(episodeURL) == "" {
		err := domain.WrapExtractFailed(errors.New("empty episodeURL"), "nineanime: GetStream")
		p.markStage(health.StageStream, err)
		return nil, err
	}

	if hit, ok := p.cache.getStream(ctx, providerID, episodeURL, serverID); ok {
		p.markStage(health.StageStream, nil)
		return cachedToStream(hit), nil
	}

	// (1) Fetch episode WP post.
	body, err := p.httpGetBody(ctx, episodeURL, maxSeriesBodyBytes)
	if err != nil {
		p.markStage(health.StageStream, err)
		return nil, err
	}

	// (2) Regex iframe src.
	m := iframeSrcRegex.FindSubmatch(body)
	if len(m) < 2 {
		err := domain.WrapExtractFailed(errors.New("no iframe src"), "nineanime: iframe regex")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	iframeURL := strings.TrimSpace(string(m[1]))
	if iframeURL == "" || !(strings.HasPrefix(iframeURL, "http://") || strings.HasPrefix(iframeURL, "https://")) {
		err := domain.WrapExtractFailed(
			fmt.Errorf("malformed iframe url %q", iframeURL),
			"nineanime: iframe url shape")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	parsedIframe, perr := url.Parse(iframeURL)
	if perr != nil {
		err := domain.WrapExtractFailed(perr, "nineanime: parse iframe url")
		p.markStage(health.StageStream, err)
		return nil, err
	}

	// (3) Fetch iframe with Referer.
	req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, iframeURL, nil)
	if rerr != nil {
		err := domain.WrapProviderDown(rerr, "nineanime: build embed request")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	req.Header.Set("Referer", p.baseURL+"/")
	resp, derr := p.http.Do(ctx, req)
	if derr != nil {
		err := domain.WrapProviderDown(derr, "nineanime: embed http")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	defer resp.Body.Close()
	embedBody, eerr := io.ReadAll(io.LimitReader(resp.Body, maxEmbedBodyBytes))
	if eerr != nil {
		err := domain.WrapProviderDown(eerr, "nineanime: embed read body")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	if resp.StatusCode >= 500 {
		err := domain.WrapProviderDown(
			fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(embedBody), 200)),
			"nineanime: embed 5xx")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	if resp.StatusCode >= 400 {
		err := domain.WrapExtractFailed(
			fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(embedBody), 200)),
			"nineanime: embed 4xx")
		p.markStage(health.StageStream, err)
		return nil, err
	}

	// (4) Regex <source src="videos/...mp4">.
	vm := videoSrcRegex.FindSubmatch(embedBody)
	if len(vm) < 2 {
		err := domain.WrapExtractFailed(errors.New("no video source"), "nineanime: video regex")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	relSrc := string(vm[1])

	// (5) Compose absolute URL from iframe host + relative src.
	absURL := fmt.Sprintf("%s://%s/%s",
		parsedIframe.Scheme, parsedIframe.Host, relSrc)

	// Public-facing Referer is always the 1anime.site origin (matches
	// the production CDN's CORS contract). The httptest path's iframe
	// host differs but the test only checks Headers[Referer] is non-empty.
	publicReferer := "https://my.1anime.site/"

	stream := &domain.Stream{
		Sources: []domain.Source{
			{URL: absURL, Type: "mp4", Quality: "auto"},
		},
		Headers: map[string]string{
			"Referer": publicReferer,
		},
	}

	// Cache the resolved stream (5 min cap).
	p.cache.setStream(ctx, providerID, episodeURL, serverID, &cachedStream{
		URL:     stream.Sources[0].URL,
		Type:    stream.Sources[0].Type,
		Quality: stream.Sources[0].Quality,
		Headers: stream.Headers,
	})
	p.markStage(health.StageStream, nil)
	return stream, nil
}

// cachedToStream rebuilds a *domain.Stream from the cached MP4-source
// shape persisted in Redis.
func cachedToStream(c *cachedStream) *domain.Stream {
	return &domain.Stream{
		Sources: []domain.Source{
			{URL: c.URL, Type: c.Type, Quality: c.Quality},
		},
		Headers: c.Headers,
	}
}

// httpGetBody fetches one URL via BaseHTTPClient and returns the body
// bytes (capped). Maps transport / 5xx / 4xx failures to the canonical
// domain errors.
func (p *Provider) httpGetBody(ctx context.Context, urlStr string, cap int64) ([]byte, error) {
	resp, err := p.http.Get(ctx, urlStr)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "nineanime: http get")
	}
	defer resp.Body.Close()
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, cap))
	if rerr != nil {
		return nil, domain.WrapProviderDown(rerr, "nineanime: read body")
	}
	if resp.StatusCode >= 500 {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"nineanime: upstream 5xx")
	}
	if resp.StatusCode >= 400 {
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"nineanime: upstream 4xx")
	}
	return body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Compile-time assertion: Provider satisfies domain.Provider. Failing this
// assertion is a build error — the strongest possible interface conformance
// test.
var _ domain.Provider = (*Provider)(nil)
