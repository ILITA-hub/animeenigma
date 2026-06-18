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
//     myvidplay.com / playmogo.com hosts (Cloudflare Turnstile) AND embed hosts
//     we have no registered extractor for (vidmoly / filemoon / bysesayeveum)
//     are filtered; duplicates by URL are removed.
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/fuzzy"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

// probeFunc is the streamprobe.Probe signature, exposed for test injection.
// Production callers leave Deps.Probe nil — New() defaults to
// streamprobe.Probe.
type probeFunc func(ctx context.Context, masterURL string, headers http.Header) streamprobe.Result

// streamGateBudget is the total in-call wall-clock budget for the
// cold-path playability gate iteration (top-2 parallel + sequential
// remainder). 8s per CONTEXT.md D2.
const streamGateBudget = 8 * time.Second

// winningServerTTL is the Redis cache TTL for the (anime, episode) →
// winning serverID mapping. 5 min per CONTEXT.md D4 + SCRAPER-HEAL-05.
const winningServerTTL = 5 * time.Minute

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

// unextractableHosts are embed players that appear in the
// gogoanime.me.uk episode-page server list but have NO registered
// embeds.Registry extractor, so GetStream can never resolve a stream from
// them — every attempt just burns the cold-path budget and emits
// parser_unplayable_total{reason="cdn_unreachable"} canary noise (AUTO-459).
// We skip them at ListServers time, mirroring turnstileHosts. Suffix-matched
// (case-insensitive) so subdomains are caught too. If/when an extractor is
// added for one of these, drop it from this list. RESEARCH.md Pitfall 9.
var unextractableHosts = []string{
	"vidmoly.biz",
	"vidmoly.net",
	"filemoon.sx",
	"bysesayeveum.com",
}

// isFilteredEmbedHost reports whether host (already lowercased) is one we
// skip at ListServers time — either Cloudflare-Turnstile-gated
// (turnstileHosts) or served by an embed player we have no registered
// extractor for (unextractableHosts). Both lists are host-equality OR
// strict-subdomain matched.
func isFilteredEmbedHost(host string) bool {
	for _, blocked := range turnstileHosts {
		if host == blocked || strings.HasSuffix(host, "."+blocked) {
			return true
		}
	}
	for _, blocked := range unextractableHosts {
		if host == blocked || strings.HasSuffix(host, "."+blocked) {
			return true
		}
	}
	return false
}

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
//
// Phase 21 (SCRAPER-HEAL-03..05): ServerPriority + HostExtractor + Probe
// extend the surface so the cold-path gate iterates servers in priority
// order, runs the playability probe, and caches the winner.
type Deps struct {
	// BaseURL is the Gogoanime base URL (default https://anitaku.to per
	// CONTEXT.md). Plan 18-04 wires this from SCRAPER_GOGOANIME_BASE_URL.
	BaseURL string
	HTTP    *domain.BaseHTTPClient
	Embeds  *domain.Registry
	MalSync malSyncClient
	Cache   cache.Cache
	Log     *logger.Logger
	// ServerPriority is the lower-cased extractor-name priority list
	// from config.GogoanimeConfig.ServerPriority. nil → no priority sort
	// (Phase 16 behaviour). Validated against the embeds registry by
	// main.go before this struct is passed to New.
	ServerPriority []string
	// HostExtractor is the pre-built host→extractor-name map built by
	// main.go from the embeds registry. Used by SortByPriority +
	// coldPathGated metric labels.
	HostExtractor map[string]string
	// Probe is the streamprobe.Probe function — injectable so tests can
	// drive deterministic gate outcomes. nil → defaults to
	// streamprobe.Probe inside New().
	Probe probeFunc
}

// Provider implements domain.Provider for the Gogoanime/Anitaku upstream.
type Provider struct {
	baseURL string
	http    *domain.BaseHTTPClient
	embeds  *domain.Registry
	malsync malSyncClient
	cache   cache.Cache
	log     *logger.Logger

	// Phase 21 SCRAPER-HEAL-03..05: server-priority + probe injection +
	// host→extractor-name map for metric label resolution.
	serverPriority []string
	hostExtractor  map[string]string
	probe          probeFunc

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
	probe := d.Probe
	if probe == nil {
		probe = streamprobe.Probe
	}
	p := &Provider{
		baseURL:        strings.TrimRight(base, "/"),
		http:           d.HTTP,
		embeds:         d.Embeds,
		malsync:        d.MalSync,
		cache:          d.Cache,
		log:            d.Log,
		serverPriority: d.ServerPriority,
		hostExtractor:  d.HostExtractor,
		probe:          probe,
		stages:         make(map[string]domain.StageHealth, len(stageNames)),
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
	// 2. Fuzzy /search.html fallback — the PRIMARY path in practice. The current
	// mirror (gogoanimes.fi) matches the keyword as a LITERAL contiguous substring
	// and HTTP-404s on some punctuation (apostrophes), so a full-title query usually
	// misses. We derive progressively-broader keywords from every title form
	// (see searchKeywords), accumulate the /category results across them, then let
	// the Jaro-Winkler step pick the right slug from the (broader) candidate set.
	titles := make([]string, 0, 1+len(ref.AltTitles))
	if ref.Title != "" {
		titles = append(titles, ref.Title)
	}
	for _, t := range ref.AltTitles {
		if strings.TrimSpace(t) != "" {
			titles = append(titles, t)
		}
	}
	if len(titles) == 0 {
		err := domain.WrapNotFound(errors.New("no title"), "gogoanime: cannot search without a title")
		p.markStage(health.StageSearch, err)
		return "", err
	}

	keywords := make([]string, 0, maxSearchKeywords)
	seenKw := make(map[string]bool)
	for _, t := range titles {
		for _, kw := range searchKeywords(t) {
			lk := strings.ToLower(kw)
			if seenKw[lk] || len(keywords) >= maxSearchKeywords {
				continue
			}
			seenKw[lk] = true
			keywords = append(keywords, kw)
		}
	}

	// 3. Search each keyword in turn, accumulating /category candidates and
	// scoring against ALL title forms. Return as soon as a candidate clears the
	// fuzzy threshold (short-circuit so a confident first-keyword hit doesn't
	// fan out into extra requests). Broader fallback keywords are only reached
	// when the specific ones miss.
	normForms := make([]string, 0, len(titles))
	for _, t := range titles {
		normForms = append(normForms, fuzzy.NormalizeTitle(t))
	}
	rowsBySlug := make(map[string]searchResult)
	best := struct {
		score float64
		slug  string
	}{}
	var lastErr error
	for _, kw := range keywords {
		rows, err := p.searchCandidates(ctx, kw)
		if err != nil {
			// A 404/parse miss on one keyword is not fatal — try the next. Remember
			// the error so a wholly-failed search still surfaces a real cause.
			lastErr = err
			continue
		}
		for _, r := range rows {
			if _, dup := rowsBySlug[r.Slug]; dup {
				continue
			}
			rowsBySlug[r.Slug] = r
			nr := fuzzy.NormalizeTitle(r.Title)
			for _, nf := range normForms {
				if score := fuzzy.JaroWinkler(nf, nr); score > best.score {
					best.score = score
					best.slug = r.Slug
				}
			}
		}
		if best.score >= fuzzyMatchThreshold && best.slug != "" {
			p.markStage(health.StageSearch, nil)
			return best.slug, nil
		}
	}

	if len(rowsBySlug) == 0 {
		if lastErr != nil {
			p.markStage(health.StageSearch, lastErr)
			return "", lastErr
		}
		// Selector drift vs real-empty: a 200 page with zero matches could be
		// either. Emit the zero-match counter so a sudden selector regression
		// (the mirror changes the .name class) is visible before queries pile up.
		metrics.ParserZeroMatchTotal.WithLabelValues("gogoanime", selectorSearchResult).Inc()
		werr := domain.WrapNotFound(nil, "gogoanime: 0 search results for "+ref.Title)
		p.markStage(health.StageSearch, werr)
		return "", werr
	}
	werr := domain.WrapNotFound(
		fmt.Errorf("best score %.4f", best.score),
		"gogoanime: no fuzzy match for "+ref.Title,
	)
	p.markStage(health.StageSearch, werr)
	return "", werr
}

// maxSearchKeywords caps how many distinct /search.html requests FindID issues
// per resolve (across all title forms) so a multi-alt-title anime can't fan out
// into a request storm against the mirror.
const maxSearchKeywords = 6

// searchKeywords derives mirror-safe search keywords from a title. gogoanimes.fi
// matches the keyword as a literal contiguous substring and HTTP-404s on some
// punctuation (e.g. apostrophes), so multi-word full titles usually miss. We
// emit progressively-broader candidates and let FindID's Jaro-Winkler step pick
// the right /category from the (broader) result set:
//
//	"Frieren: Beyond Journey's End" -> ["Frieren"]
//	"Re:Zero kara Hajimeru…"        -> ["Re"]
//	"One Piece"                     -> ["One Piece", "One"]
//	"Dr. Stone"                     -> ["Dr"]
//
// Candidate 1 is the leading run up to the first punctuation mark; candidate 2 is
// the first word (when the lead is multi-word). Results are letters/digits/spaces
// only, trimmed, deduped (case-insensitive), in original case.
func searchKeywords(title string) []string {
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) }

	// Leading run up to the first punctuation mark.
	var lead strings.Builder
	for _, r := range title {
		if !isWord(r) {
			break
		}
		lead.WriteRune(r)
	}
	leadKw := strings.Join(strings.Fields(lead.String()), " ")

	// If the title starts with punctuation, the lead is empty — fall back to the
	// whole title with punctuation replaced by spaces.
	if leadKw == "" {
		var sb strings.Builder
		for _, r := range title {
			if isWord(r) {
				sb.WriteRune(r)
			} else {
				sb.WriteRune(' ')
			}
		}
		leadKw = strings.Join(strings.Fields(sb.String()), " ")
	}

	out := make([]string, 0, 2)
	seen := make(map[string]bool)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		k := strings.ToLower(s)
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, s)
	}
	add(leadKw)
	if fields := strings.Fields(leadKw); len(fields) > 1 {
		add(fields[0]) // first-word fallback for stricter mirrors
	}
	return out
}

// searchCandidates issues one /search.html?keyword= request and parses the
// /category result rows. Transport/non-200/parse errors are returned (FindID
// tries the next keyword); a 200 with zero rows returns (nil, nil).
func (p *Provider) searchCandidates(ctx context.Context, keyword string) ([]searchResult, error) {
	q := url.QueryEscape(keyword)
	searchURL := fmt.Sprintf("%s/search.html?keyword=%s", p.baseURL, q)
	resp, err := p.http.Get(ctx, searchURL)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "gogoanime: search fetch")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, domain.WrapProviderDown(fmt.Errorf("status %d", resp.StatusCode), "gogoanime: search non-200")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySearch))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "gogoanime: search read body")
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "gogoanime: search parse")
	}
	rows := make([]searchResult, 0, 16)
	doc.Find("p.name a[href^='/category/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		slug := strings.TrimSuffix(strings.TrimPrefix(href, "/category/"), "/")
		title := strings.TrimSpace(sel.Text())
		if slug == "" || title == "" {
			return
		}
		rows = append(rows, searchResult{Slug: slug, Title: title})
	})
	return rows, nil
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

	// emit picks the canonical (sub-preferred) episode and tags it with the
	// categories found during the merge, so downstream (notifications) can
	// compute latest-sub vs latest-dub.
	emit := func(e *merged) domain.Episode {
		ep := e.sub
		if !e.hasSub {
			ep = e.dub
		}
		ep.HasSub = e.hasSub
		ep.HasDub = e.hasDub
		return ep
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
		if e.hasSub || e.hasDub {
			all = append(all, emit(e))
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
		// WR-05 — stdlib introsort. Was a hand-rolled insertion sort under
		// the "len is small in practice" assumption, but any future shape
		// change to anitaku.to that produces non-contiguous episode numbers
		// (e.g. paginated layout) would push this to O(n^2) on every cold
		// cache hit.
		sort.Ints(nums)
		for _, n := range nums {
			if e := byNum[n]; e.hasSub || e.hasDub {
				all = append(all, emit(e))
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

	// Determine sub/dub from the slug suffix. WR-09 — the previous
	// implementation also bound `cat` (domain.CategorySub|Dub) and then
	// discarded it with `_ = cat` inside the goquery loop because the
	// category is re-derived in ListServers from the episode-ID slug.
	// Dropping the unused binding keeps the intent ("isDub gates the
	// loop's href filter") clear.
	isDub := strings.HasSuffix(slug, "-dub")

	rows := make([]domain.Episode, 0, 64)
	seen := make(map[int]bool)
	// Every /category/<slug> page has a "Recent Releases" sidebar that links
	// to episodes of unrelated anime (e.g. /case-closed-episode-1201). Those
	// links all match `a[href*="-episode-"]` so we MUST gate the loop on the
	// href's slug-root matching the target slug. Without this check the
	// sidebar pollutes the returned list with foreign episodes (verified
	// 2026-05-13 against anitaku.to/category/attack-on-titan).
	expectSlug := strings.TrimSuffix(slug, "-dub")
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
		// Strip the -dub suffix on the href side for the equality check so
		// the sub and dub variants of the same anime are recognized as the
		// same series.
		if strings.TrimSuffix(hrefSlug, "-dub") != expectSlug {
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
		// Skip Turnstile-gated AND no-registered-extractor embed hosts per
		// RESEARCH.md Pitfall 9 / AUTO-459 — they can never resolve a stream,
		// so returning them only burns the cold-path budget.
		if isFilteredEmbedHost(host) {
			return
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
		// WR-04 — record only the high-level category in stage health.
		// The raw extractor error includes the wrapped serverID / signed-
		// URL path, which can leak token-bearing query params (e.g.
		// `?s=...&e=...&token=...` from StreamHG/Earnvids) through the
		// admin /api/admin/scraper/health endpoint. The original err is
		// still returned to the orchestrator (and logged at the boundary)
		// so failover behaviour is unchanged.
		p.markStage(health.StageStream, errors.New("gogoanime: stream "+classifyStreamErr(err)))
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

// classifyStreamErr maps an extractor error to a generic category string
// for stage-health storage. WR-04 — the full extractor error wraps the raw
// serverID / signed-URL path which can carry token-bearing query params,
// and that value is surfaced to admins via /api/admin/scraper/health.
// Keeping only the category strips PII / signing tokens before storage.
func classifyStreamErr(err error) string {
	switch {
	case errors.Is(err, domain.ErrExtractFailed):
		return "extract_failed"
	case errors.Is(err, domain.ErrProviderDown):
		return "provider_down"
	case errors.Is(err, domain.ErrNotFound):
		return "not_found"
	default:
		return "unknown"
	}
}

// GetStreamWithGate is the priority-aware + gated entry point for the
// EnglishPlayer cold path.
//
// serverID semantics:
//
//   - non-empty (caller pin): bypass priority + gate, delegate to plain
//     GetStream, return gated=false. Matches Phase 16's per-server pin
//     contract.
//   - empty: cold path. First check Redis at
//     scraper:winning_server:gogoanime:<anime>:<ep>. On hit, validate the
//     cached serverID is still present in `servers`; if yes, delegate to
//     GetStream and return gated=false. If the cache hit but the cached
//     serverID is no longer present OR the extract failed, DELETE the
//     stale entry and fall through to the cold-path iteration.
//   - empty + cache miss: iterate `servers` in priority order (sorted
//     internally by SortByPriority — callers do NOT pre-sort), running
//     streamprobe.Probe on each. Top-2 probed in parallel (CONTEXT.md
//     risks: probe budget overshoot mitigation); positions 3+ sequential.
//     First playable result is cached at the winning_server key for
//     winningServerTTL (5 min) and returned with gated=true.
//
// On exhaustion: returns ErrProviderDown wrapping the last failure reason,
// gated=true (the gate DID run on this call, it just didn't find a winner).
//
// Total wall-clock budget: streamGateBudget (8s).
//
// SCRAPER-HEAL-04 + SCRAPER-HEAL-05.
func (p *Provider) GetStreamWithGate(
	ctx context.Context, providerID, episodeID, serverID string,
	category domain.Category, servers []domain.Server,
) (*domain.Stream, bool, error) {
	// Caller-pinned path: no priority, no gate.
	if serverID != "" {
		s, err := p.GetStream(ctx, providerID, episodeID, serverID, category)
		return s, false, err
	}
	if len(servers) == 0 {
		return nil, false, domain.WrapNotFound(nil, "gogoanime: no servers for gated stream")
	}

	winnerKey := fmt.Sprintf("scraper:winning_server:%s:%s:%s", providerName, providerID, episodeID)

	// Warm path: cached winner.
	var cachedServerID string
	if err := p.cache.Get(ctx, winnerKey, &cachedServerID); err == nil && cachedServerID != "" {
		// Validate the cached serverID is still in the supplied list — the
		// upstream HTML may have rotated servers since we cached, in which
		// case we DO NOT trust the stale entry.
		if hasServer(servers, cachedServerID) {
			s, err := p.GetStream(ctx, providerID, episodeID, cachedServerID, category)
			if err == nil {
				return s, false, nil
			}
			// Cached winner errored on extract — fall through to cold path
			// AND delete the stale cache entry.
		}
		_ = p.cache.Delete(ctx, winnerKey)
	}

	// Cold path: priority iteration + gate.
	return p.coldPathGated(ctx, providerID, episodeID, category, servers, winnerKey)
}

// gateAttempt is the result of one server's extract+probe attempt.
type gateAttempt struct {
	serverID string
	stream   *domain.Stream
	reason   streamprobe.Reason
	err      error
}

// coldPathGated runs the priority + gate iteration. Probes top-2 candidates
// in parallel (CONTEXT.md risks: probe budget overshoot mitigation),
// iterates positions 3+ sequentially.
//
// Priority sorting happens HERE (the first statement) so callers never
// need to know about priority — they pass the raw ListServers output
// and we apply our configured SCRAPER_SERVER_PRIORITY internally. This
// supersedes the earlier draft that put sorting in the orchestrator/handler.
//
// Total in-call budget: streamGateBudget (8s) via ctx with timeout.
func (p *Provider) coldPathGated(
	ctx context.Context, providerID, episodeID string,
	category domain.Category, servers []domain.Server,
	winnerKey string,
) (*domain.Stream, bool, error) {
	// Priority sort internal to the provider — callers pass unsorted
	// ListServers output. SCRAPER-HEAL-03.
	servers = SortByPriority(servers, p.serverPriority, p.hostExtractor)

	callCtx, cancel := context.WithTimeout(ctx, streamGateBudget)
	defer cancel()

	probe := p.probe
	if probe == nil {
		probe = streamprobe.Probe
	}

	// attemptOne: extract URL via the embed registry, then probe master
	// m3u8 via streamprobe. Returns stream+ReasonPlayable on success or
	// err+reason on failure. Increments the unplayable / ad-decoy counters
	// on failure (server label = extractor.Name(), NOT the visible HTML
	// label — closed cardinality per T-21-10).
	attemptOne := func(attemptCtx context.Context, srv domain.Server) gateAttempt {
		s, err := p.GetStream(attemptCtx, providerID, episodeID, srv.ID, category)
		if err != nil {
			extName := p.serverLabel(srv.ID)
			metrics.ParserUnplayableTotal.WithLabelValues(providerName, extName, string(streamprobe.ReasonZeroMatch)).Inc()
			return gateAttempt{serverID: srv.ID, err: err, reason: streamprobe.ReasonZeroMatch}
		}
		if s == nil || len(s.Sources) == 0 {
			extName := p.serverLabel(srv.ID)
			metrics.ParserUnplayableTotal.WithLabelValues(providerName, extName, string(streamprobe.ReasonEmptyResponse)).Inc()
			return gateAttempt{serverID: srv.ID, err: errors.New("empty sources"), reason: streamprobe.ReasonEmptyResponse}
		}
		hdrs := http.Header{}
		if ref := s.Headers["Referer"]; ref != "" {
			hdrs.Set("Referer", ref)
		}
		extName := p.serverLabel(srv.ID)
		// Plan 22-01: iterate ALL Sources for this server before declaring
		// the server failed. Phase 21's coldPathGated probed only Sources[0]
		// — multi-URL extraction (hls2 + hls3 in streamhg/earnvids) would be
		// dead code without this iteration. Each per-source failure emits one
		// parser_unplayable_total increment so the dashboard sees the full
		// attempted set; the LAST source's reason rides up to gateAttempt for
		// upstream lastReason tracking.
		var lastReason streamprobe.Reason
		for _, src := range s.Sources {
			if attemptCtx.Err() != nil {
				return gateAttempt{serverID: srv.ID, err: attemptCtx.Err(), reason: streamprobe.ReasonCDNUnreachable}
			}
			res := probe(attemptCtx, src.URL, hdrs)
			if res.Playable {
				// Return a trimmed Stream containing ONLY the playable
				// Source — downstream FE never sees the failed URL. Preserve
				// Tracks/Intro/Outro/Headers from the original extraction.
				trimmed := &domain.Stream{
					Sources: []domain.Source{src},
					Tracks:  s.Tracks,
					Intro:   s.Intro,
					Outro:   s.Outro,
					Headers: s.Headers,
				}
				return gateAttempt{serverID: srv.ID, stream: trimmed, reason: streamprobe.ReasonPlayable}
			}
			metrics.ParserUnplayableTotal.WithLabelValues(providerName, extName, string(res.Reason)).Inc()
			if res.Reason == streamprobe.ReasonAdDecoy {
				metrics.ParserAdDecoyTotal.WithLabelValues(providerName, extName).Inc()
			}
			lastReason = res.Reason
		}
		return gateAttempt{serverID: srv.ID, err: fmt.Errorf("all %d sources failed: %s", len(s.Sources), lastReason), reason: lastReason}
	}

	// Parallel top-2 — probe both concurrently so a slow server doesn't
	// serialize the budget.
	topN := 2
	if len(servers) < topN {
		topN = len(servers)
	}
	parallelResults := make(chan gateAttempt, topN)
	parCtx, parCancel := context.WithCancel(callCtx)
	for i := 0; i < topN; i++ {
		go func(srv domain.Server) {
			parallelResults <- attemptOne(parCtx, srv)
		}(servers[i])
	}
	var lastReason streamprobe.Reason
	for i := 0; i < topN; i++ {
		select {
		case <-callCtx.Done():
			parCancel()
			return nil, true, domain.WrapProviderDown(callCtx.Err(), "gogoanime: gated stream budget exceeded")
		case r := <-parallelResults:
			if r.stream != nil {
				parCancel() // cancel the other in-flight probe
				_ = p.cache.Set(ctx, winnerKey, r.serverID, winningServerTTL)
				return r.stream, true, nil
			}
			lastReason = r.reason
		}
	}
	parCancel()

	// Sequential positions 3+ — typical anitaku.to episode page has 3-4
	// servers so this loop runs 1-2 times max.
	for i := topN; i < len(servers); i++ {
		if callCtx.Err() != nil {
			return nil, true, domain.WrapProviderDown(callCtx.Err(), "gogoanime: gated stream budget exceeded")
		}
		r := attemptOne(callCtx, servers[i])
		if r.stream != nil {
			_ = p.cache.Set(ctx, winnerKey, r.serverID, winningServerTTL)
			return r.stream, true, nil
		}
		lastReason = r.reason
	}

	return nil, true, domain.WrapProviderDown(
		fmt.Errorf("all %d servers gate-failed; last reason=%s", len(servers), lastReason),
		"gogoanime: no playable server",
	)
}

// serverLabel resolves a server URL to the extractor-name label used by the
// parser_unplayable_total / parser_ad_decoy_total metrics. Falls back to
// the literal "unknown" so a future host-list drift never produces an
// empty label (which would render as `server=""` in Prometheus).
func (p *Provider) serverLabel(serverID string) string {
	if name := hostnameToExtractorName(serverID, p.hostExtractor); name != "" {
		return name
	}
	return "unknown"
}

// hasServer reports whether servers contains an entry whose ID == id.
// Used by the warm-path cache validation in GetStreamWithGate.
func hasServer(servers []domain.Server, id string) bool {
	for _, s := range servers {
		if s.ID == id {
			return true
		}
	}
	return false
}

// Compile-time assertion: *Provider satisfies domain.Provider.
var _ domain.Provider = (*Provider)(nil)
