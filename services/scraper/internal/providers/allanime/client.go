package allanime

// Lift Decision Log (CONTEXT.md D1 — copy-with-adaptation, NOT a move):
//
//  1. queries.go     — Lift verbatim. Apollo persisted-query hashes + GraphQL
//                       query strings are upstream-defined and identical for
//                       raw-jp and EN-sub use. (Note: translationType=sub here
//                       vs raw-jp's pre-commit-102c590 "raw"; the GraphQL
//                       strings themselves are identical.)
//  2. decrypt.go     — Lift verbatim. AES-256-CTR `tobeparsed` decryption is a
//                       pure function with no consumer-specific coupling.
//  3. dto.go         — Adapt. Catalog-side carries raw-jp-only fields
//                       (RU subtitle URL extraction). Scraper-side DTOs match
//                       domain.Episode / domain.Server / domain.Stream shape;
//                       we keep only the fields needed for EN-sub resolution.
//  4. client.go      — Adapt. Catalog's *Client returns raw-jp Stream{};
//                       this Provider implements scraper's domain.Provider
//                       interface (Name, FindID, ListEpisodes, ListServers,
//                       GetStream, HealthCheck) with the canonical 5-stage
//                       health snapshot.
//  5. cache.go       — Mirror gogoanime/cache.go: 4 key families (show ID,
//                       episodes, servers, stream) with the cross-cutting
//                       TTL invariants from libs/cache.
//  6. HTTP client    — Use *domain.BaseHTTPClient (per SCRAPER-FOUND-06 NF).
//                       NEVER catalog's HTTP client, NEVER bare *http.Client.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/sourceprobe"
)

// providerName is the stable identifier returned by Name() and used as the
// orchestrator's registry key. Backend slug = display label = "allanime".
const providerName = "allanime"

// stageNames is the canonical stage list. Alias of health.AllStages so any
// stage rename in the health package flows here automatically.
var stageNames = health.AllStages

// Deps is the constructor input for New(). Required fields validated eagerly.
type Deps struct {
	// BaseURL overrides the AllAnime API endpoint. Empty → https://api.allanime.day.
	BaseURL string
	HTTP    *domain.BaseHTTPClient
	Cache   cache.Cache
	Log     *logger.Logger
}

// Provider implements domain.Provider for the AllAnime upstream.
type Provider struct {
	baseURL string
	http    *domain.BaseHTTPClient
	cache   *cacheLayer
	log     *logger.Logger

	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs a Provider. Required dependencies validated eagerly so
// main.go fatals on a misconfiguration instead of a deferred 502 on the
// first request (SCRAPER-FOUND mirroring gogoanime/animepahe/animekai).
//
// Default BaseURL is https://api.allanime.day (the canonical AllAnime API
// host; allanime.day / allmanga.to / allanime.to are domain aliases).
func New(d Deps) (*Provider, error) {
	if d.HTTP == nil {
		return nil, errors.New("allanime: Deps.HTTP is required")
	}
	if d.Cache == nil {
		return nil, errors.New("allanime: Deps.Cache is required")
	}
	if d.Log == nil {
		d.Log = logger.Default()
	}
	base := d.BaseURL
	if base == "" {
		base = "https://api.allanime.day"
	}
	p := &Provider{
		baseURL: strings.TrimRight(base, "/"),
		http:    d.HTTP,
		cache:   newCacheLayer(d.Cache),
		log:     d.Log,
		stages:  make(map[string]domain.StageHealth, len(stageNames)),
	}
	// Optimistic seed: stages start with Up=true so the orchestrator's
	// nil-cache backcompat path treats us as healthy before the first probe
	// tick lands. Probe runner flips this with real data within ~30s.
	for _, s := range stageNames {
		p.stages[s] = domain.StageHealth{Up: true}
	}
	return p, nil
}

// Name returns the stable identifier "allanime".
func (p *Provider) Name() string { return providerName }

// markStage records the success/failure of one stage. Called on every
// method exit path. Copied verbatim from animekai/gogoanime.
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

// FindID resolves the catalog's AnimeRef into an AllAnime show `_id` by
// searching by title (AllAnime's search is fuzzy on title; we narrow on
// best match). Falls back to ErrNotFound when no edges match.
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	// Cache key uses MAL ID when available; otherwise title as a weaker key.
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
		err := domain.WrapNotFound(errors.New("empty title"), "allanime: FindID needs a title")
		p.markStage(health.StageSearch, err)
		return "", err
	}

	vars, err := buildSearchVariables(query)
	if err != nil {
		err = domain.WrapExtractFailed(err, "allanime: buildSearchVariables")
		p.markStage(health.StageSearch, err)
		return "", err
	}
	ext := buildExtensions(SHASearchFallback)

	var resp searchShowsResponse
	if doErr := p.doGraphQL(ctx, SearchQuery, vars, ext, &resp); doErr != nil {
		p.markStage(health.StageSearch, doErr)
		return "", doErr
	}
	if len(resp.Data.Shows.Edges) == 0 {
		err := domain.WrapNotFound(fmt.Errorf("no edges for %q", query), "allanime: FindID")
		p.markStage(health.StageSearch, err)
		return "", err
	}

	// Best match: prefer an edge whose name OR englishName equals the
	// query (case-insensitive); otherwise take the first edge.
	pick := resp.Data.Shows.Edges[0]
	lowQuery := strings.ToLower(query)
	for _, e := range resp.Data.Shows.Edges {
		if strings.EqualFold(e.Name, query) || strings.EqualFold(e.EnglishName, query) {
			pick = e
			break
		}
		if strings.Contains(strings.ToLower(e.Name), lowQuery) ||
			strings.Contains(strings.ToLower(e.EnglishName), lowQuery) {
			pick = e
		}
	}
	if pick.ID == "" {
		err := domain.WrapExtractFailed(errors.New("empty show ID"), "allanime: FindID")
		p.markStage(health.StageSearch, err)
		return "", err
	}

	if cacheKey != "" {
		p.cache.setShowID(ctx, cacheKey, pick.ID)
	}
	p.markStage(health.StageSearch, nil)
	return pick.ID, nil
}

// ListEpisodes returns the episode list for one AllAnime show ID. EpisodeIDs
// are formatted as "<showID>:<episodeString>" so downstream calls can split
// the original episodeString back out (matches the catalog-side convention
// but with `:` rather than `/` to avoid colliding with shikimori_id paths).
func (p *Provider) ListEpisodes(ctx context.Context, showID string) ([]domain.Episode, error) {
	if strings.TrimSpace(showID) == "" {
		err := domain.WrapExtractFailed(errors.New("empty showID"), "allanime: ListEpisodes")
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}

	if hit, ok := p.cache.getEpisodes(ctx, showID); ok {
		p.markStage(health.StageEpisodes, nil)
		return materializeEpisodes(showID, hit), nil
	}

	vars, err := buildEpisodesVariables(showID)
	if err != nil {
		err = domain.WrapExtractFailed(err, "allanime: buildEpisodesVariables")
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}
	ext := buildExtensions(SHAEpisodesFallback)

	var resp showResponse
	if doErr := p.doGraphQL(ctx, EpisodesQuery, vars, ext, &resp); doErr != nil {
		p.markStage(health.StageEpisodes, doErr)
		return nil, doErr
	}
	raw := resp.Data.Show.AvailableEpisodesDetail.Sub
	if len(raw) == 0 {
		// Real-empty (anime exists, no episodes aired yet) is `([], nil)`.
		p.markStage(health.StageEpisodes, nil)
		return []domain.Episode{}, nil
	}

	p.cache.setEpisodes(ctx, showID, raw)
	p.markStage(health.StageEpisodes, nil)
	return materializeEpisodes(showID, raw), nil
}

func materializeEpisodes(showID string, raw []string) []domain.Episode {
	out := make([]domain.Episode, 0, len(raw))
	for _, ep := range raw {
		ep = strings.TrimSpace(ep)
		if ep == "" {
			continue
		}
		n, _ := strconv.Atoi(ep)
		out = append(out, domain.Episode{
			ID:     fmt.Sprintf("%s:%s", showID, ep),
			Number: n,
			Title:  fmt.Sprintf("Episode %s", ep),
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

// ListServers returns the streaming servers AllAnime exposes for one episode.
// EpisodeID format: "<showID>:<episodeString>". Each server name is the
// upstream's sourceName ("Default", "S-mp4", "Yt-mp4", etc.); the orchestrator
// picks the first server whose GetStream succeeds.
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	showID, ep := splitEpisodeID(episodeID)
	if showID == "" || ep == "" {
		err := domain.WrapExtractFailed(
			fmt.Errorf("invalid episode ID %q", episodeID),
			"allanime: ListServers")
		p.markStage(health.StageServers, err)
		return nil, err
	}

	if hit, ok := p.cache.getServers(ctx, showID, ep); ok {
		p.markStage(health.StageServers, nil)
		return materializeServers(hit), nil
	}

	sources, err := p.fetchSources(ctx, showID, ep)
	if err != nil {
		p.markStage(health.StageServers, err)
		return nil, err
	}
	if len(sources) == 0 {
		err := domain.WrapExtractFailed(
			fmt.Errorf("empty sourceUrls for %s ep %s", showID, ep),
			"allanime: ListServers")
		p.markStage(health.StageServers, err)
		return nil, err
	}

	p.cache.setServers(ctx, showID, ep, sources)
	p.markStage(health.StageServers, nil)
	return materializeServers(sources), nil
}

func materializeServers(sources []sourceURL) []domain.Server {
	out := make([]domain.Server, 0, len(sources))
	for _, s := range sources {
		name := s.SourceName
		if name == "" {
			name = "Default"
		}
		out = append(out, domain.Server{
			ID:   name,
			Name: name,
			Type: domain.CategorySub, // we query translationType=sub
		})
	}
	return out
}

// GetStream resolves one (server, episode) tuple to a playable stream URL.
// Falls back to ErrExtractFailed if no source matches the serverID or no
// source resolves to a fully-qualified http(s) URL.
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	showID, ep := splitEpisodeID(episodeID)
	if showID == "" || ep == "" {
		err := domain.WrapExtractFailed(
			fmt.Errorf("invalid episode ID %q", episodeID),
			"allanime: GetStream")
		p.markStage(health.StageStream, err)
		return nil, err
	}

	if hit, ok := p.cache.getStream(ctx, showID, ep, serverID); ok {
		p.markStage(health.StageStream, nil)
		return cachedToStream(hit), nil
	}

	sources, ok := p.cache.getServers(ctx, showID, ep)
	if !ok {
		var ferr error
		sources, ferr = p.fetchSources(ctx, showID, ep)
		if ferr != nil {
			p.markStage(health.StageStream, ferr)
			return nil, ferr
		}
		p.cache.setServers(ctx, showID, ep, sources)
	}

	// Build the candidate order: the caller-pinned serverID first (a hint), then
	// the remaining sources by priority. Probe each candidate's actual content
	// and use the first that is a real stream — skipping HTML embed pages
	// (ok.ru, uns.bio, vidnest.io, …) DYNAMICALLY, with no host list. If the
	// pinned server turns out to be an embed, we transparently fall back to the
	// best playable source rather than dead-ending.
	candidates := orderCandidates(sources, serverID)
	var pick *sourceURL
	var streamURL string
	for i := range candidates {
		u := resolveSourceURL(candidates[i])
		if u == "" {
			continue // not a fully-qualified http(s) URL
		}
		if p.classify(ctx, u) != sourceprobe.Stream {
			continue // embed page / unprobeable — skip
		}
		pick = &candidates[i]
		streamURL = u
		break
	}
	if pick == nil {
		err := domain.WrapExtractFailed(
			fmt.Errorf("no playable source for server %q (all embed/unresolvable)", serverID),
			"allanime: GetStream")
		p.markStage(health.StageStream, err)
		return nil, err
	}

	stream := &domain.Stream{
		Sources: []domain.Source{
			{
				URL:     streamURL,
				Type:    streamType(streamURL, pick.FileExtension),
				Quality: "auto",
			},
		},
		Headers: map[string]string{
			"Referer": apiReferer,
		},
	}
	for _, sub := range pick.Subtitles {
		stream.Tracks = append(stream.Tracks, domain.Track{
			File:  sub.SourceURL,
			Label: sub.Label,
			Kind:  "subtitles",
		})
	}

	// Cache the resolved stream.
	cached := &cachedStream{
		URL:     stream.Sources[0].URL,
		Type:    stream.Sources[0].Type,
		Quality: stream.Sources[0].Quality,
		Headers: stream.Headers,
	}
	for _, sub := range pick.Subtitles {
		cached.Subtitles = append(cached.Subtitles, cachedSubtitle{
			URL: sub.SourceURL, Lang: sub.Lang, Label: sub.Label,
		})
	}
	p.cache.setStream(ctx, showID, ep, serverID, cached)

	p.markStage(health.StageStream, nil)
	return stream, nil
}

func cachedToStream(c *cachedStream) *domain.Stream {
	out := &domain.Stream{
		Sources: []domain.Source{
			{URL: c.URL, Type: c.Type, Quality: c.Quality},
		},
		Headers: c.Headers,
	}
	for _, sub := range c.Subtitles {
		out.Tracks = append(out.Tracks, domain.Track{
			File:  sub.URL,
			Label: sub.Label,
			Kind:  "subtitles",
		})
	}
	return out
}

// resolveSourceURL decodes a source's (possibly obfuscated, possibly
// protocol-relative) URL into a fully-qualified http(s) URL, or "" if it is
// not one.
func resolveSourceURL(s sourceURL) string {
	u := decodeSourceURL(s.SourceURL)
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	if !strings.HasPrefix(u, "http") {
		return ""
	}
	return u
}

// orderCandidates returns sources ordered for stream resolution: the
// caller-pinned serverID first (a hint), then the rest by descending priority.
func orderCandidates(sources []sourceURL, serverID string) []sourceURL {
	pinned := make([]sourceURL, 0, 2)
	rest := make([]sourceURL, 0, len(sources))
	for _, s := range sources {
		if serverID != "" && strings.EqualFold(s.SourceName, serverID) {
			pinned = append(pinned, s)
		} else {
			rest = append(rest, s)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].Priority > rest[j].Priority })
	sort.SliceStable(pinned, func(i, j int) bool { return pinned[i].Priority > pinned[j].Priority })
	return append(pinned, rest...)
}

// classify probes a resolved URL (cached) and reports whether it is a real
// stream, an HTML embed page, or unknown. Stream/Embed verdicts are cached;
// Unknown (transient probe failure) is not, so a later retry can re-probe.
func (p *Provider) classify(ctx context.Context, rawURL string) sourceprobe.Kind {
	if k, ok := p.cache.getClassification(ctx, rawURL); ok {
		return sourceprobe.Kind(k)
	}
	k := sourceprobe.Classify(ctx, p.http, rawURL, apiReferer)
	if k != sourceprobe.Unknown {
		p.cache.setClassification(ctx, rawURL, int(k))
	}
	return k
}

// fetchSources POSTs the SourceUrls APQ and returns the (decrypted, if
// needed) sourceUrls array.
func (p *Provider) fetchSources(ctx context.Context, showID, ep string) ([]sourceURL, error) {
	vars, err := buildSourcesVariables(showID, ep)
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "allanime: buildSourcesVariables")
	}
	ext := buildExtensions(SHASourcesFallback)

	var env episodeEnvelope
	if doErr := p.doGraphQL(ctx, "", vars, ext, &env); doErr != nil {
		return nil, doErr
	}

	sources := env.Data.Episode
	if env.Data.Tobeparsed != "" {
		plain, derr := decryptTobeparsed(env.Data.Tobeparsed)
		if derr != nil {
			return nil, domain.WrapExtractFailed(derr, "allanime: decrypt tobeparsed")
		}
		var inner struct {
			Episode *episodeData `json:"episode"`
		}
		if uerr := json.Unmarshal(plain, &inner); uerr != nil {
			return nil, domain.WrapExtractFailed(uerr, "allanime: parse decrypted sources")
		}
		sources = inner.Episode
	}

	if sources == nil {
		if len(env.Errors) > 0 {
			return nil, domain.WrapExtractFailed(
				fmt.Errorf("upstream errors: %v", env.Errors),
				"allanime: sources resolver rejected query")
		}
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("nil episode for %s:%s", showID, ep),
			"allanime: no sources")
	}
	return sources.SourceUrls, nil
}

// doGraphQL GETs a single persisted query through BaseHTTPClient. The query
// string and the `extensions` parameter live in the URL query string per
// Apollo's GET-with-APQ flow.
func (p *Provider) doGraphQL(ctx context.Context, gqlQuery, vars, ext string, out any) error {
	v := url.Values{}
	if gqlQuery != "" {
		v.Set("query", gqlQuery)
	}
	v.Set("variables", vars)
	v.Set("extensions", ext)
	endpoint := fmt.Sprintf("%s/api?%s", p.baseURL, v.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.WrapProviderDown(err, "allanime: build request")
	}
	req.Header.Set("Referer", apiReferer)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", apiUA)

	resp, err := p.http.Do(ctx, req)
	if err != nil {
		return domain.WrapProviderDown(err, "allanime: http")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB DoS cap
	if err != nil {
		return domain.WrapProviderDown(err, "allanime: read body")
	}

	if resp.StatusCode >= 500 {
		return domain.WrapProviderDown(
			fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"allanime: upstream 5xx")
	}
	if resp.StatusCode >= 400 {
		// 4xx is usually a stale persisted-query SHA (extract-failed), not
		// a transport-down.
		return domain.WrapExtractFailed(
			fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"allanime: 4xx")
	}

	if err := json.Unmarshal(body, out); err != nil {
		return domain.WrapExtractFailed(err, "allanime: parse json")
	}
	return nil
}

// splitEpisodeID parses "<showID>:<episodeString>" → (showID, episodeString).
func splitEpisodeID(id string) (string, string) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// decodeSourceURL applies AllAnime's lightweight URL obfuscation. Sources
// returned by the GraphQL `sourceUrl` field are sometimes prefixed with "--"
// followed by a hex-encoded XOR-56 redirect URL. If the prefix is absent,
// the URL is returned as-is.
//
// Lifted from services/catalog/internal/parser/allanime/episodes.go.
func decodeSourceURL(s string) string {
	const prefix = "--"
	if !strings.HasPrefix(s, prefix) {
		return s
	}
	hex := strings.TrimPrefix(s, prefix)
	out := make([]byte, 0, len(hex)/2)
	for i := 0; i+1 < len(hex); i += 2 {
		b, err := strconv.ParseUint(hex[i:i+2], 16, 8)
		if err != nil {
			return s
		}
		out = append(out, byte(b))
	}
	for i := range out {
		out[i] ^= 56
	}
	decoded := string(out)
	if strings.HasPrefix(decoded, "http") {
		return decoded
	}
	return decoded
}

// streamType picks the container type from the source's fileExtenstion hint
// (preferred) and falls back to URL inspection.
func streamType(u, hint string) string {
	switch strings.ToLower(hint) {
	case "mp4":
		return "mp4"
	case "m3u8", "hls":
		return "hls"
	}
	low := strings.ToLower(u)
	switch {
	case strings.Contains(low, ".m3u8"):
		return "hls"
	case strings.Contains(low, ".mp4"):
		return "mp4"
	default:
		return "hls"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Embed-vs-stream classification is no longer a hardcoded host list — it is
// determined dynamically by probing the actual response content. See
// Provider.classify (above) and internal/sourceprobe.

// Compile-time assertion: Provider satisfies domain.Provider. Failing this
// assertion is a build error — the strongest possible interface-conformance
// test.
var _ domain.Provider = (*Provider)(nil)
