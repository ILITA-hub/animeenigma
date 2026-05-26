package animefever

// client.go — AnimeFever provider. Implements domain.Provider for the
// animefever.cc upstream. Phase 28 SCRAPER-HEAL-36.
//
// Adapted from services/scraper/internal/providers/allanime/client.go's
// shape per CONTEXT.md D1 (copy-with-adaptation). The 6 method signatures,
// markStage helper, stages map, and HealthCheck shape are identical; the
// data path is the only thing that differs (HTML scrape + AJAX POST vs
// AllAnime's GraphQL APQ).
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

// providerName is the stable identifier returned by Name() and used as the
// orchestrator's registry key.
const providerName = "animefever"

// defaultBaseURL is the canonical AnimeFever host as confirmed via the
// 2026-05-20 live recon.
const defaultBaseURL = "https://animefever.cc"

// fuzzyThreshold is the JaroWinkler score threshold for FindID title
// matching. Below this, the result is treated as ErrNotFound.
const fuzzyThreshold = 0.85

// maxBodyBytes caps any one upstream HTML/JSON body read. 4 MiB matches
// BaseHTTPClient's own response-cap convention.
const maxBodyBytes = 4 << 20

// defaultServer is the AnimeFever server used when the caller does not pin
// one. tserver is the upstream primary and — per AUTO-275 — the ONLY server
// that yields a parseable stream.
const defaultServer = "tserver"

// supportedServers is the ordered allowlist of AnimeFever server IDs the
// provider will resolve. hserver is intentionally EXCLUDED (AUTO-275): its
// embeds structurally return no parseable `sources:` literal — they have never
// produced a valid HLS URL and only generated false-positive stream_segment
// DOWN alerts as the health probe / orchestrator walked tserver→hserver onto a
// dead-end. Keeping hserver out of this list means neither the failover loop
// nor the health probe ever hand an hserver embed URL to the vidstream_vip
// extractor. tserver delivers valid HLS via am.vidstream.vip →
// static-cdn-ca1.mofl.pro.
var supportedServers = []string{defaultServer}

// isSupportedServer reports whether serverID is in the supportedServers
// allowlist. hserver (and any other non-tserver id) is blocked (AUTO-275).
func isSupportedServer(serverID string) bool {
	for _, s := range supportedServers {
		if s == serverID {
			return true
		}
	}
	return false
}

// filterSupportedServers drops any server not in the supportedServers
// allowlist, preserving order. Applied to cached server lists on read so a
// pre-AUTO-275 cache entry (which still contains hserver) can never re-surface
// a blocked server after deploy, even within the 1h servers cache TTL.
func filterSupportedServers(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if isSupportedServer(s) {
			out = append(out, s)
		}
	}
	return out
}

// stageNames is the canonical stage list. Alias of health.AllStages so any
// stage rename in the health package flows here automatically.
var stageNames = health.AllStages

// ctkRe extracts the `var ctk = '...'` token from a watch-page HTML
// response. Per RESEARCH.md Pitfall 2; the token is a 32+ hex char CSRF.
var ctkRe = regexp.MustCompile(`var\s+ctk\s*=\s*['"]([0-9a-fA-F]{16,64})['"]`)

// iframeSrcRe extracts an iframe's src attribute value from a fragment of
// HTML (the AJAX response's `value` field). Lower complexity than full
// goquery for a single attribute pull.
var iframeSrcRe = regexp.MustCompile(`(?i)<iframe[^>]+src=["']([^"']+)["']`)

// episodeNumRe pulls the episode number from a "Episode N" text label, or
// from the ?ep=<id> URL parameter when the label is missing.
var episodeNumRe = regexp.MustCompile(`(?i)episode[\s_-]*([0-9]+)`)

// Deps is the constructor input for New().
type Deps struct {
	BaseURL string
	HTTP    *domain.BaseHTTPClient
	Embeds  *domain.Registry
	Cache   cache.Cache
	Log     *logger.Logger
}

// Provider implements domain.Provider for the AnimeFever upstream.
type Provider struct {
	baseURL string
	http    *domain.BaseHTTPClient
	embeds  *domain.Registry
	cache   *cacheLayer
	log     *logger.Logger

	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs a Provider. Required dependencies validated eagerly so
// main.go fatals on misconfiguration instead of a deferred 502.
func New(d Deps) (*Provider, error) {
	if d.HTTP == nil {
		return nil, errors.New("animefever: Deps.HTTP is required")
	}
	if d.Embeds == nil {
		return nil, errors.New("animefever: Deps.Embeds is required")
	}
	if d.Cache == nil {
		return nil, errors.New("animefever: Deps.Cache is required")
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
		embeds:  d.Embeds,
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

// Name returns the stable identifier "animefever".
func (p *Provider) Name() string { return providerName }

// markStage records the success/failure of one stage. Copied from allanime.
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

// FindID resolves AnimeRef → AnimeFever slug via title search. Uses
// JaroWinkler ≥0.85 against the search-result card-block list.
//
// Per RESEARCH.md Pitfall 1: search path is /search/<term>, NOT
// /search?keyword=<term>.
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	cacheKey := ref.ShikimoriID
	if cacheKey == "" {
		cacheKey = ref.Title
	}
	if cacheKey != "" {
		if hit, ok := p.cache.getShowID(ctx, cacheKey); ok {
			p.markStage(health.StageSearch, nil)
			return hit, nil
		}
	}

	query := strings.TrimSpace(ref.Title)
	if query == "" {
		err := domain.WrapNotFound(errors.New("empty title"), "animefever: FindID needs a title")
		p.markStage(health.StageSearch, err)
		return "", err
	}

	searchURL := fmt.Sprintf("%s/search/%s", p.baseURL, url.PathEscape(query))
	body, err := p.httpGetBody(ctx, searchURL)
	if err != nil {
		p.markStage(health.StageSearch, err)
		return "", err
	}

	doc, derr := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if derr != nil {
		err := domain.WrapExtractFailed(derr, "animefever: parse search HTML")
		p.markStage(health.StageSearch, err)
		return "", err
	}

	// ISS-017: score each candidate against the primary title AND every
	// alternate form (romaji / English / Japanese) the catalog supplied,
	// taking the max. AnimeFever indexes the main series under its ROMAJI
	// title ("Shingeki no Kyojin") while the catalog's primary may be the
	// English NameEN ("Attack on Titan", which on AnimeFever only matches
	// no-embed compilations). Matching against all forms lets the romaji form
	// resolve the romaji-listed main entry.
	normQueries := make([]string, 0, 1+len(ref.AltTitles))
	seenQ := map[string]bool{}
	for _, q := range append([]string{query}, ref.AltTitles...) {
		nq := fuzzy.NormalizeTitle(q)
		if nq == "" || seenQ[nq] {
			continue
		}
		seenQ[nq] = true
		normQueries = append(normQueries, nq)
	}
	type candidate struct {
		slug  string
		title string
		score float64
	}
	var best candidate
	doc.Find("div.card-block").Each(func(_ int, sel *goquery.Selection) {
		anchor := sel.Find("a[href*='/info/']").First()
		href, _ := anchor.Attr("href")
		slug := extractSlugFromHref(href)
		if slug == "" {
			return
		}
		// Score against every plausible title carrier on the card:
		//   1. The <div class="card-block" title="..."> attribute (Japanese romaji on live data)
		//   2. The <a title="..."> attribute (alternate localized title when present)
		//   3. The <h3> text (often the same romaji)
		//   4. The slug itself (matches the English title on animefever.cc)
		// Keep the best of all four — JaroWinkler is monotonic, no precedence ambiguity.
		titles := []string{}
		if t, ok := sel.Attr("title"); ok && t != "" {
			titles = append(titles, t)
		}
		if t, ok := anchor.Attr("title"); ok && t != "" {
			titles = append(titles, t)
		}
		if h3 := strings.TrimSpace(sel.Find("h3").First().Text()); h3 != "" {
			titles = append(titles, h3)
		}
		// Slug carries the English title hyphenated; normalize replaces hyphens.
		titles = append(titles, slug)

		bestTitle := ""
		bestScore := 0.0
		for _, t := range titles {
			nt := fuzzy.NormalizeTitle(t)
			for _, nq := range normQueries {
				s := fuzzy.JaroWinkler(nq, nt)
				if s > bestScore {
					bestScore, bestTitle = s, t
				}
			}
		}
		// Slug-shape bias: animefever publishes the same show across several
		// slugs — the primary entry's slug ends in `.<numericID>` ("…end.14401"),
		// while localization variants append "-dub" / "-season-2" without an ID.
		// JaroWinkler is unfortunately quite forgiving of "-dub"/"-sub" suffixes
		// (only 3 extra chars on a 28-char title), so bias decisively:
		//   +0.05 for canonical `.<digits>` slugs (the primary entry)
		//   -0.05 for "-dub" / "-sub" / "-season-N" / "-spanish" variants
		// Use a single classifier and apply both directions so primary wins.
		isCanonical := false
		if i := strings.LastIndex(slug, "."); i > 0 {
			suffix := slug[i+1:]
			if suffix != "" && suffix[0] >= '0' && suffix[0] <= '9' {
				isCanonical = true
			}
		}
		isVariant := strings.HasSuffix(slug, "-dub") ||
			strings.HasSuffix(slug, "-sub") ||
			strings.Contains(slug, "-season-") ||
			strings.Contains(slug, "-spanish") ||
			strings.Contains(slug, "-french") ||
			strings.Contains(slug, "-german")
		switch {
		case isCanonical:
			bestScore += 0.05
		case isVariant:
			bestScore -= 0.05
		}
		if bestScore > best.score {
			best = candidate{slug: slug, title: bestTitle, score: bestScore}
		}
	})

	if best.slug == "" || best.score < fuzzyThreshold {
		err := domain.WrapNotFound(
			fmt.Errorf("no card-block scored ≥%.2f for %q (best=%q score=%.2f)",
				fuzzyThreshold, query, best.title, best.score),
			"animefever: FindID")
		p.markStage(health.StageSearch, err)
		return "", err
	}

	if cacheKey != "" {
		p.cache.setShowID(ctx, cacheKey, best.slug)
	}
	p.markStage(health.StageSearch, nil)
	return best.slug, nil
}

// extractSlugFromHref pulls the slug out of an `/info/<slug>` href. Returns
// empty when the href is not in the expected /info/ form.
func extractSlugFromHref(href string) string {
	if href == "" {
		return ""
	}
	const prefix = "/info/"
	idx := strings.Index(href, prefix)
	if idx < 0 {
		return ""
	}
	rest := href[idx+len(prefix):]
	// Strip any trailing /?# query.
	if q := strings.IndexAny(rest, "?#"); q >= 0 {
		rest = rest[:q]
	}
	rest = strings.TrimRight(rest, "/")
	return rest
}

// ListEpisodes returns the episode list for one AnimeFever slug. Episode
// IDs are formatted as "<slug>:<eid>" so downstream calls can recover the
// per-episode ?ep= numeric token.
func (p *Provider) ListEpisodes(ctx context.Context, slug string) ([]domain.Episode, error) {
	if strings.TrimSpace(slug) == "" {
		err := domain.WrapExtractFailed(errors.New("empty slug"), "animefever: ListEpisodes")
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}

	if hit, ok := p.cache.getEpisodes(ctx, slug); ok {
		p.markStage(health.StageEpisodes, nil)
		return materializeEpisodes(slug, hit), nil
	}

	infoURL := fmt.Sprintf("%s/info/%s", p.baseURL, slug)
	body, err := p.httpGetBody(ctx, infoURL)
	if err != nil {
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}

	doc, derr := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if derr != nil {
		err := domain.WrapExtractFailed(derr, "animefever: parse info HTML")
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}

	var refs []episodeRef
	seenEID := map[string]bool{}
	doc.Find(`a[href*="/watch/"]`).Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if !strings.Contains(href, "/watch/") {
			return
		}
		// Parse ?ep=<eid> out of the href.
		u, err := url.Parse(href)
		if err != nil {
			return
		}
		eid := u.Query().Get("ep")
		if eid == "" || seenEID[eid] {
			return
		}
		seenEID[eid] = true

		title := strings.TrimSpace(sel.Text())
		num := parseEpisodeNumber(title, eid)
		refs = append(refs, episodeRef{
			EID:    eid,
			Number: num,
			Title:  title,
		})
	})

	if len(refs) == 0 {
		// Real-empty (anime exists, no episodes yet) is `([], nil)`.
		p.markStage(health.StageEpisodes, nil)
		return []domain.Episode{}, nil
	}

	p.cache.setEpisodes(ctx, slug, refs)
	p.markStage(health.StageEpisodes, nil)
	return materializeEpisodes(slug, refs), nil
}

func parseEpisodeNumber(label, eid string) int {
	if m := episodeNumRe.FindStringSubmatch(label); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	// Fallback: try to parse eid as an integer directly. Sometimes
	// AnimeFever's ?ep= IDs are sequential per anime.
	if n, err := strconv.Atoi(eid); err == nil {
		return n
	}
	return 0
}

func materializeEpisodes(slug string, refs []episodeRef) []domain.Episode {
	out := make([]domain.Episode, 0, len(refs))
	for _, r := range refs {
		title := r.Title
		if title == "" {
			title = fmt.Sprintf("Episode %d", r.Number)
		}
		out = append(out, domain.Episode{
			ID:     fmt.Sprintf("%s:%s", slug, r.EID),
			Number: r.Number,
			Title:  title,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Number == 0 && out[j].Number == 0 {
			return out[i].ID < out[j].ID
		}
		if out[i].Number == 0 {
			return false
		}
		if out[j].Number == 0 {
			return true
		}
		return out[i].Number < out[j].Number
	})
	return out
}

// ListServers returns the streaming servers AnimeFever exposes for one
// episode. AnimeFever's watch page offers tserver + hserver, but per AUTO-275
// we advertise ONLY tserver: hserver embeds structurally return no parseable
// `sources:` literal (never a valid HLS URL) and only produced false-positive
// stream_segment DOWN alerts when the probe/orchestrator walked onto them. See
// supportedServers. We do NOT probe the AJAX endpoint here.
//
// The watch page is fetched to (a) populate the PHPSESSID cookie via the
// BaseHTTPClient jar and (b) extract + cache the ctk token for GetStream.
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	slug, eid := splitEpisodeID(episodeID)
	if slug == "" || eid == "" {
		err := domain.WrapExtractFailed(
			fmt.Errorf("invalid episode ID %q", episodeID),
			"animefever: ListServers")
		p.markStage(health.StageServers, err)
		return nil, err
	}
	if !validEID(eid) {
		err := domain.WrapExtractFailed(
			fmt.Errorf("malformed eid %q (must be alphanumeric)", eid),
			"animefever: ListServers")
		p.markStage(health.StageServers, err)
		return nil, err
	}

	// Filter cached entries through the allowlist so a pre-AUTO-275 cache entry
	// (which still contains hserver) cannot re-surface a blocked server.
	if hit, ok := p.cache.getServers(ctx, slug, eid); ok {
		if filtered := filterSupportedServers(hit); len(filtered) > 0 {
			p.markStage(health.StageServers, nil)
			return materializeServers(filtered), nil
		}
	}

	// Fetch the watch page (populates PHPSESSID cookie + lets us cache ctk).
	if _, _, err := p.fetchCtk(ctx, slug, eid); err != nil {
		// ctk extraction failure is non-fatal here — GetStream re-tries.
		// We still log and continue; the server list is fixed.
		p.log.Warnw("animefever: failed to pre-extract ctk during ListServers",
			"slug", slug, "eid", eid, "error", err)
	}

	// AUTO-275: advertise only tserver. hserver is a dead-end; excluding it
	// here stops the orchestrator failover loop and the health probe from ever
	// probing it. See supportedServers.
	servers := append([]string(nil), supportedServers...)
	p.cache.setServers(ctx, slug, eid, servers)
	p.markStage(health.StageServers, nil)
	return materializeServers(servers), nil
}

// splitEpisodeID parses "<slug>:<eid>" → (slug, eid). The slug itself may
// contain dots (e.g. "frieren-beyond-journeys-end.14401"), but no colons.
func splitEpisodeID(id string) (string, string) {
	idx := strings.LastIndex(id, ":")
	if idx < 0 {
		return "", ""
	}
	slug := id[:idx]
	eid := id[idx+1:]
	if slug == "" || eid == "" {
		return "", ""
	}
	return slug, eid
}

var eidRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func validEID(s string) bool { return eidRe.MatchString(s) }

func materializeServers(names []string) []domain.Server {
	out := make([]domain.Server, 0, len(names))
	for _, n := range names {
		out = append(out, domain.Server{
			ID:   n,
			Name: n,
			Type: domain.CategorySub,
		})
	}
	return out
}

// GetStream resolves one (slug, episodeID, serverID) tuple via:
//
//	1. fetch watch-page HTML → extract ctk token (Pitfall 2)
//	2. POST /ajax/anime/load_episodes_v2?s=<server> with episode_id+ctk
//	3. extract iframe src from the AJAX response
//	4. delegate to embed Registry — match against am.vidstream.vip
//	5. return the extractor's *Stream unchanged
//
// On status:false (stale ctk), evict the ctk cache and retry ONCE.
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	slug, eid := splitEpisodeID(episodeID)
	if slug == "" || eid == "" {
		err := domain.WrapExtractFailed(
			fmt.Errorf("invalid episode ID %q", episodeID),
			"animefever: GetStream")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	if !validEID(eid) {
		err := domain.WrapExtractFailed(
			fmt.Errorf("malformed eid %q", eid),
			"animefever: GetStream")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	if serverID == "" {
		serverID = defaultServer
	}
	if !isSupportedServer(serverID) {
		// AUTO-275: reject hserver (and any non-tserver) BEFORE any upstream
		// fetch, so an hserver embed URL is never handed to the vidstream_vip
		// extractor. tserver is the sole server that yields a parseable stream.
		err := domain.WrapExtractFailed(
			fmt.Errorf("server %q is not supported (only tserver yields a parseable stream; hserver blocked per AUTO-275)", serverID),
			"animefever: GetStream")
		p.markStage(health.StageStream, err)
		return nil, err
	}

	if hit, ok := p.cache.getStream(ctx, slug, eid, serverID); ok {
		p.markStage(health.StageStream, nil)
		return cachedToStream(hit), nil
	}

	stream, err := p.fetchStreamOnce(ctx, slug, eid, serverID)
	if err != nil {
		// Retry-once path: if the failure is the stale-ctk shape, evict the
		// cached ctk and retry.
		if errors.Is(err, errStaleCtk) {
			p.cache.deleteCtk(ctx, slug, eid)
			stream, err = p.fetchStreamOnce(ctx, slug, eid, serverID)
		}
		if err != nil {
			p.markStage(health.StageStream, err)
			return nil, err
		}
	}

	// Cache the first source for 5min.
	if len(stream.Sources) > 0 {
		c := &cachedStream{
			URL:     stream.Sources[0].URL,
			Type:    stream.Sources[0].Type,
			Quality: stream.Sources[0].Quality,
			Headers: stream.Headers,
		}
		p.cache.setStream(ctx, slug, eid, serverID, c)
	}
	p.markStage(health.StageStream, nil)
	return stream, nil
}

// errStaleCtk is the internal sentinel that drives GetStream's
// "evict-ctk-and-retry-once" logic.
var errStaleCtk = errors.New("animefever: stale ctk token")

// errNoEmbed is the internal sentinel for an upstream "no embed for this
// (episode, server)" signal (status:false / embed:false with a FRESH ctk).
// Distinct from errStaleCtk so GetStream does NOT pointlessly retry a token
// that is already fresh (ISS-017).
var errNoEmbed = errors.New("animefever: no embed for episode")

// selectorNoEmbed is the parser_zero_match_total selector label for the
// no-embed path. The golden-pool liveness probe trips this when a golden
// title fuzzy-matches a recap/compilation entry that AnimeFever has no
// player embed for — a content-availability signal, not a broken provider
// (ISS-017).
const selectorNoEmbed = "no_embed"

// fetchStreamOnce does one full GetStream pass — no retry. On status:false
// returns errStaleCtk wrapped under ErrExtractFailed.
func (p *Provider) fetchStreamOnce(ctx context.Context, slug, eid, serverID string) (*domain.Stream, error) {
	ctk, ctkCached, err := p.fetchCtk(ctx, slug, eid)
	if err != nil {
		return nil, err
	}

	ajaxURL := fmt.Sprintf("%s/ajax/anime/load_episodes_v2?s=%s",
		p.baseURL, url.QueryEscape(serverID))
	form := url.Values{}
	form.Set("episode_id", eid)
	form.Set("ctk", ctk)

	req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, ajaxURL,
		strings.NewReader(form.Encode()))
	if rerr != nil {
		return nil, domain.WrapProviderDown(rerr, "animefever: build ajax request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", fmt.Sprintf("%s/watch/%s?ep=%s", p.baseURL, slug, eid))

	resp, rerr := p.http.Do(ctx, req)
	if rerr != nil {
		return nil, domain.WrapProviderDown(rerr, "animefever: ajax http")
	}
	defer resp.Body.Close()

	body, berr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if berr != nil {
		return nil, domain.WrapProviderDown(berr, "animefever: ajax read body")
	}
	if resp.StatusCode >= 500 {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"animefever: ajax 5xx")
	}
	if resp.StatusCode >= 400 {
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"animefever: ajax 4xx")
	}

	var out ajaxLoadEpisodeResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, domain.WrapExtractFailed(err, "animefever: parse ajax json")
	}
	if !out.Status || !out.Embed {
		// ISS-017: status:false/embed:false does NOT necessarily mean a stale
		// ctk — measured live, it most often means "this server has no embed
		// for this episode" (e.g. recap/compilation entries). Only suspect a
		// stale token when the ctk came from CACHE (it may have rotated since);
		// in that case surface errStaleCtk so GetStream evicts + retries ONCE
		// with a fresh token. With a freshly-scraped ctk the token is by
		// definition not stale, so this is a genuine no-embed → emit the
		// no_embed metric and surface an honest error (still ErrExtractFailed
		// so the probe tries the other server and the orchestrator fails over).
		if !out.Status && ctkCached {
			return nil, domain.WrapExtractFailed(errStaleCtk, "animefever: ajax status=false (cached ctk; retrying fresh)")
		}
		metrics.ParserZeroMatchTotal.WithLabelValues(providerName, selectorNoEmbed).Inc()
		return nil, domain.WrapExtractFailed(errNoEmbed,
			fmt.Sprintf("animefever: no embed for episode on server %q (status=%t embed=%t)", serverID, out.Status, out.Embed))
	}

	m := iframeSrcRe.FindStringSubmatch(out.Value)
	if len(m) != 2 {
		return nil, domain.WrapExtractFailed(
			errors.New("no iframe src in ajax value"),
			"animefever: extract iframe url")
	}
	iframeURL := strings.TrimSpace(m[1])
	if !strings.HasPrefix(iframeURL, "http") {
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("non-http iframe url %q", iframeURL),
			"animefever: iframe url shape")
	}

	extractor, ferr := p.embeds.Find(iframeURL)
	if ferr != nil || extractor == nil {
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("no extractor for iframe %s: %v", iframeURL, ferr),
			"animefever: embed registry miss")
	}

	headers := http.Header{}
	headers.Set("Referer", fmt.Sprintf("%s/watch/%s?ep=%s", p.baseURL, slug, eid))

	stream, eerr := extractor.Extract(ctx, iframeURL, headers)
	if eerr != nil {
		return nil, domain.WrapExtractFailed(eerr, "animefever: extractor")
	}
	if stream == nil || len(stream.Sources) == 0 {
		return nil, domain.WrapExtractFailed(
			errors.New("extractor returned empty stream"),
			"animefever: extractor returned no sources")
	}
	return stream, nil
}

// fetchCtk returns a ctk token for the (slug, eid) watch page plus whether it
// came from the cache. The fromCache flag lets fetchStreamOnce distinguish a
// genuinely-stale cached token (worth a fresh-fetch retry) from an upstream
// "no embed" signal returned with a freshly-scraped token (ISS-017).
func (p *Provider) fetchCtk(ctx context.Context, slug, eid string) (token string, fromCache bool, err error) {
	if hit, ok := p.cache.getCtk(ctx, slug, eid); ok {
		return hit, true, nil
	}
	watchURL := fmt.Sprintf("%s/watch/%s?ep=%s", p.baseURL, slug, url.QueryEscape(eid))
	body, err := p.httpGetBody(ctx, watchURL)
	if err != nil {
		return "", false, err
	}
	m := ctkRe.FindStringSubmatch(string(body))
	if len(m) != 2 {
		return "", false, domain.WrapExtractFailed(
			errors.New("no var ctk = ... in watch page"),
			"animefever: extract ctk")
	}
	token = m[1]
	p.cache.setCtk(ctx, slug, eid, token)
	return token, false, nil
}

// httpGetBody fetches one URL via BaseHTTPClient and returns the body
// bytes (capped). Maps transport / 5xx / 4xx failures to the canonical
// domain errors.
func (p *Provider) httpGetBody(ctx context.Context, urlStr string) ([]byte, error) {
	resp, err := p.http.Get(ctx, urlStr)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "animefever: http get")
	}
	defer resp.Body.Close()
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if rerr != nil {
		return nil, domain.WrapProviderDown(rerr, "animefever: read body")
	}
	if resp.StatusCode >= 500 {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"animefever: upstream 5xx")
	}
	if resp.StatusCode >= 400 {
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"animefever: upstream 4xx")
	}
	return body, nil
}

// cachedToStream rebuilds a *domain.Stream from the cached single-source
// shape persisted in Redis.
func cachedToStream(c *cachedStream) *domain.Stream {
	return &domain.Stream{
		Sources: []domain.Source{
			{URL: c.URL, Type: c.Type, Quality: c.Quality},
		},
		Headers: c.Headers,
	}
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
