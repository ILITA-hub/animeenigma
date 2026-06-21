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
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/fuzzy"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

// Selector identifiers for parser_zero_match_total. Stable, short, and
// kebab-cased per the project convention (services/scraper/internal/embeds/
// vibeplayer.go + services/scraper/internal/providers/gogoanime/client.go).
// These labels are the maintenance-bot's Pattern-7 dispatch keys —
// renaming them silently breaks the bot. See .claude/maintenance-prompt.md
// "Scraper Provider Schema Drift" section.
const (
	selectorMy1AnimeIframe = "my_1anime_iframe"
	selectorVideoMP4Source = "video_mp4_source"
	// selectorYouTubeStub fires when an episode embeds a YouTube trailer
	// placeholder instead of a real source (a known upstream data-quality
	// quirk for stub series — NOT a regression). Distinct label keeps the
	// maintenance bot from misclassifying it as iframe-host drift.
	selectorYouTubeStub = "youtube_stub"
)

// youtubeStubHosts are the trailer-placeholder hosts some stub series embed.
// They are matched (host or strict subdomain) and rejected as a non-source,
// distinct from a real upstream-shape regression.
var youtubeStubHosts = []string{"youtube.com", "youtu.be"}

// embedAllowedHosts is the production allowlist of legitimate embed hosts
// the upstream's HTML may legally point an iframe at. The historical contract
// is `my.1anime.site` (direct-MP4 wrapper). Any other host is treated as an
// upstream-shape regression — new megaplay.buzz redirects, YouTube trailer
// placeholders, and any future rebrand all fail at this gate with a clean
// parser_zero_match_total{selector="my_1anime_iframe"} signal so the
// maintenance bot's Pattern-7 dispatch can fire.
//
// Tests mount the iframe on the same httptest.Server as the series page;
// isAllowedIframeHost permits that case via the same-origin fallback to
// p.baseURL's host, so test isolation stays intact without leaking a
// permissive regex into production.
var embedAllowedHosts = []string{"my.1anime.site"}

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

// iframeSrcRegex extracts an iframe URL from the episode WP post HTML.
// The regex itself is permissive (any URL); the host-allowlist check in
// GetStream (see embedAllowedHosts + isAllowedIframeHost) enforces the
// T-28-05-05 SSRF defense — only `my.1anime.site` (or the test server's
// same-origin host) is accepted. This split keeps the regex testable
// against an httptest.Server URL while production stays anchored on the
// canonical embed host.
//
// Upstream-shape regressions (2026-05 popular-catalog migration to
// 1anime.site/megaplay → megaplay.buzz, YouTube trailer placeholders for
// stub series) all fail at the host-allowlist gate with
// parser_zero_match_total{selector="my_1anime_iframe"} so the maintenance
// bot's Pattern-7 dispatch can fire on a stable, parseable signal rather
// than a downstream `<source>`-regex miss that misattributes the failure.
var iframeSrcRegex = regexp.MustCompile(`(?i)<iframe[^>]+src=["']([^"']+)["']`)

// videoSrcRegex extracts the <source src="videos/<name>.mp4"> from the
// my.1anime.site embed page. The src is RELATIVE; caller composes
// absolute URL from the iframe host.
var videoSrcRegex = regexp.MustCompile(`(?i)<source[^>]+src=["'](videos/[^"']+\.mp4)["']`)

// BrowserResolveFunc resolves a megaplay embed/wrapper URL to a playable Stream
// via the Camoufox sidecar. BrowserFetchFunc routes one discovery GET through the
// sidecar's warm browser session, returning (upstreamStatus, body). main.go binds
// these to the sidecar.Client (provider + baseURL pre-bound) when this provider's
// DB engine column is "browser"; the pure-Go path leaves them nil.
type BrowserResolveFunc func(ctx context.Context, embedURL string, category domain.Category) (*domain.Stream, error)
type BrowserFetchFunc func(ctx context.Context, provider, url string) (int, []byte, error)

// Deps is the constructor input for New(). Per WR-04, the Embeds field
// from the allanime template is deliberately omitted — nineanime does
// MP4 extraction inline, not via the embed registry. Including a dead
// field would mislead future maintainers.
type Deps struct {
	BaseURL string
	HTTP    *domain.BaseHTTPClient
	Cache   cache.Cache
	Log     *logger.Logger

	// Megaplay resolves the megaplay.buzz HLS player that the upstream's
	// popular catalog migrated to (1anime.site wrapper → megaplay.buzz).
	// Typed as the EmbedExtractor interface so tests can inject a fake.
	// Optional: when nil, 1anime.site/megaplay.buzz iframes fail at the host
	// gate exactly as before (legacy my.1anime.site MP4 path is unaffected).
	Megaplay domain.EmbedExtractor

	// Browser routing — set together when this provider's DB engine column is
	// "browser". UseBrowser is the live per-call gate; BrowserFetch carries the
	// challenge-gated discovery GETs; BrowserResolve resolves megaplay players.
	// All nil ⇒ legacy pure-Go path (engine=http) unchanged.
	UseBrowser     func() bool
	BrowserResolve BrowserResolveFunc
	BrowserFetch   BrowserFetchFunc
}

// Provider implements domain.Provider for the 9anime.me.uk upstream.
type Provider struct {
	baseURL  string
	http     *domain.BaseHTTPClient
	cache    *cacheLayer
	log      *logger.Logger
	megaplay domain.EmbedExtractor

	useBrowser     func() bool
	browserResolve BrowserResolveFunc
	browserFetch   BrowserFetchFunc

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
		baseURL:        strings.TrimRight(base, "/"),
		http:           d.HTTP,
		cache:          newCacheLayer(d.Cache),
		log:            d.Log,
		megaplay:       d.Megaplay,
		useBrowser:     d.UseBrowser,
		browserResolve: d.BrowserResolve,
		browserFetch:   d.BrowserFetch,
		stages:         make(map[string]domain.StageHealth, len(stageNames)),
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

// isAllowedIframeHost reports whether `host` is a legitimate embed target.
// Production: only my.1anime.site (embedAllowedHosts). Tests: also accept
// the provider's own baseURL host so a same-origin httptest iframe is
// extractable without leaking a permissive regex into production code.
// Case-insensitive; ignores port.
func (p *Provider) isAllowedIframeHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, allowed := range embedAllowedHosts {
		if host == allowed {
			return true
		}
	}
	// Test isolation: same-origin httptest iframe (parsedIframe.Hostname()
	// equals the host of p.baseURL). Production never matches this branch
	// because p.baseURL is 9anime.me.uk, which is never embedAllowedHosts'
	// inverse — the upstream wouldn't legitimately self-host an MP4 embed
	// at its own series-page origin.
	if u, err := url.Parse(p.baseURL); err == nil {
		if h := strings.ToLower(u.Hostname()); h != "" && h == host {
			return true
		}
	}
	return false
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
		metrics.ParserZeroMatchTotal.WithLabelValues(providerName, selectorMy1AnimeIframe).Inc()
		err := domain.WrapExtractFailed(errors.New("no iframe src"), "nineanime: iframe regex")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	iframeURL := strings.TrimSpace(string(m[1]))
	if iframeURL == "" || !(strings.HasPrefix(iframeURL, "http://") || strings.HasPrefix(iframeURL, "https://")) {
		metrics.ParserZeroMatchTotal.WithLabelValues(providerName, selectorMy1AnimeIframe).Inc()
		err := domain.WrapExtractFailed(
			fmt.Errorf("malformed iframe url %q", iframeURL),
			"nineanime: iframe url shape")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	parsedIframe, perr := url.Parse(iframeURL)
	if perr != nil {
		metrics.ParserZeroMatchTotal.WithLabelValues(providerName, selectorMy1AnimeIframe).Inc()
		err := domain.WrapExtractFailed(perr, "nineanime: parse iframe url")
		p.markStage(health.StageStream, err)
		return nil, err
	}

	// (2b) Host routing. Three upstream shapes coexist as 9anime migrates:
	//   - my.1anime.site             → legacy direct-MP4 wrapper (steps 3–6)
	//   - 1anime.site / megaplay.buzz → megaplay HLS player (delegated extractor)
	//   - youtube.com / youtu.be      → trailer-stub placeholder (no real source)
	// Anything else is a genuine upstream-shape regression and emits the
	// stable parser_zero_match_total{selector="my_1anime_iframe"} signal the
	// maintenance bot's Pattern-7 dispatch keys on.
	iframeHost := parsedIframe.Hostname()
	switch {
	case p.isAllowedIframeHost(iframeHost):
		// Legacy my.1anime.site MP4 host — fall through to steps 3–6 below.
	case p.megaplay != nil && p.megaplay.Matches(iframeURL):
		if p.browserEnabled() {
			// JS player (megaplay/vidwish) — resolve + restream via Camoufox.
			return p.streamViaBrowser(ctx, iframeURL, category)
		}
		return p.streamViaMegaplay(ctx, providerID, episodeURL, serverID, iframeURL)
	case isYouTubeStubHost(iframeHost):
		metrics.ParserZeroMatchTotal.WithLabelValues(providerName, selectorYouTubeStub).Inc()
		err := domain.WrapExtractFailed(
			fmt.Errorf("episode embeds a YouTube trailer stub (%s), not a real source", iframeHost),
			"nineanime: youtube stub")
		p.markStage(health.StageStream, err)
		return nil, err
	default:
		metrics.ParserZeroMatchTotal.WithLabelValues(providerName, selectorMy1AnimeIframe).Inc()
		err := domain.WrapExtractFailed(
			fmt.Errorf("iframe host %q not in allowlist {my.1anime.site, 1anime.site, megaplay.buzz}; upstream shape regression",
				iframeHost),
			"nineanime: iframe host")
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
		metrics.ParserZeroMatchTotal.WithLabelValues(providerName, selectorVideoMP4Source).Inc()
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
	p.cache.setStream(ctx, providerID, episodeURL, serverID, streamToCached(stream))
	p.markStage(health.StageStream, nil)
	return stream, nil
}

// streamViaMegaplay resolves an HLS stream off the 1anime.site/megaplay.buzz
// player via the megaplay extractor, caches it, and marks the stream stage.
// The master.m3u8 is stable per episode, so the 5min cache is safe; rotating
// segment CDNs are handled downstream by the HLS proxy's provenance token.
func (p *Provider) streamViaMegaplay(ctx context.Context, providerID, episodeURL, serverID, iframeURL string) (*domain.Stream, error) {
	// Tag the ctx with this provider so the megaplay extractor's recording
	// transport pivots its egress effects by provider+host (D-02/D-09, WR-07),
	// matching how BaseHTTPClient-routed provider calls are tagged. No-op when
	// the extractor's transport is unrecorded (tests / global sink absent).
	ctx = domain.ProviderContext(ctx, p.Name())
	stream, err := p.megaplay.Extract(ctx, iframeURL, nil)
	if err != nil {
		p.markStage(health.StageStream, err)
		return nil, err
	}
	p.cache.setStream(ctx, providerID, episodeURL, serverID, streamToCached(stream))
	p.markStage(health.StageStream, nil)
	return stream, nil
}

// isYouTubeStubHost reports whether host is a YouTube trailer-stub origin.
func isYouTubeStubHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, h := range youtubeStubHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// streamToCached flattens a *domain.Stream into the Redis cache shape.
// Sources[0] is the canonical playable URL for both the MP4 and HLS paths;
// Tracks/Intro/Outro carry through for megaplay HLS (empty for legacy MP4).
func streamToCached(s *domain.Stream) *cachedStream {
	if s == nil || len(s.Sources) == 0 {
		return &cachedStream{}
	}
	return &cachedStream{
		URL:     s.Sources[0].URL,
		Type:    s.Sources[0].Type,
		Quality: s.Sources[0].Quality,
		Headers: s.Headers,
		Tracks:  s.Tracks,
		Intro:   s.Intro,
		Outro:   s.Outro,
	}
}

// cachedToStream rebuilds a *domain.Stream from the cached shape persisted in
// Redis (single source + optional megaplay HLS tracks/skip markers).
func cachedToStream(c *cachedStream) *domain.Stream {
	return &domain.Stream{
		Sources: []domain.Source{
			{URL: c.URL, Type: c.Type, Quality: c.Quality},
		},
		Headers: c.Headers,
		Tracks:  c.Tracks,
		Intro:   c.Intro,
		Outro:   c.Outro,
	}
}

// browserEnabled reports whether this call should route through the Camoufox
// sidecar (DB engine=browser + all three callbacks wired). Requires the full
// trio so a half-wired Deps never silently runs a mixed path.
func (p *Provider) browserEnabled() bool {
	return p.useBrowser != nil && p.browserResolve != nil &&
		p.browserFetch != nil && p.useBrowser()
}

// streamViaBrowser resolves a megaplay embed URL through the sidecar, mirroring
// the in-process GetStream contract (stage health + non-empty guard). Used in
// place of streamViaMegaplay when engine=browser.
func (p *Provider) streamViaBrowser(ctx context.Context, embedURL string, category domain.Category) (*domain.Stream, error) {
	stream, err := p.browserResolve(ctx, embedURL, category)
	if err != nil {
		p.markStage(health.StageStream, errors.New("nineanime: browser resolve failed"))
		return nil, err
	}
	if stream == nil || len(stream.Sources) == 0 {
		werr := domain.WrapExtractFailed(errors.New("empty stream"), "nineanime: browser empty stream")
		p.markStage(health.StageStream, werr)
		return nil, werr
	}
	p.markStage(health.StageStream, nil)
	return stream, nil
}

// httpGetBody fetches one URL via BaseHTTPClient and returns the body
// bytes (capped). Maps transport / 5xx / 4xx failures to the canonical
// domain errors. When engine=browser, the discovery GET is routed through the
// Camoufox sidecar instead (same status→error mapping).
func (p *Provider) httpGetBody(ctx context.Context, urlStr string, cap int64) ([]byte, error) {
	if p.browserEnabled() {
		status, body, err := p.browserFetch(ctx, providerName, urlStr)
		if err != nil {
			return nil, err // already wrapped (ProviderDown/NotFound) by the client
		}
		if status >= 500 {
			return nil, domain.WrapProviderDown(
				fmt.Errorf("upstream %d: %s", status, truncate(string(body), 200)),
				"nineanime: browser upstream 5xx")
		}
		if status >= 400 {
			return nil, domain.WrapExtractFailed(
				fmt.Errorf("http %d: %s", status, truncate(string(body), 200)),
				"nineanime: browser upstream 4xx")
		}
		return body, nil
	}
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
