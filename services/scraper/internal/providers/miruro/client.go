package miruro

// Miruro provider (SCRAPER-HEAL-37, Phase 28 Wave 2). Failover slot 5.
//
// See doc.go for the architecture diagram and SPIKE-MIRURO.md (Plan 28-00)
// for the upstream wire-protocol discovery that this provider consumes.
//
// IMPORTANT — interaction with obfuscation.go:
//
//   client.go *constructs* the secure-pipe URL via BuildSecurePipeURL
//   (which embeds the canonical request descriptor in the `e=` query
//   parameter) and *decodes* the response body via
//   DecodeObfuscatedResponse. All bytes-level base64url/gzip/XOR plumbing
//   lives in obfuscation.go; this file deals only with the JSON content
//   layer once the envelope is off.
//
// Lift mapping vs allanime/client.go:
//
//   - FindID: instead of fuzzy title search, we use libs/idmapping ARM
//     to translate MAL→AniList. AniList ID *is* the provider ID.
//   - ListEpisodes/ListServers: GET /api/secure/pipe?e=...
//     instead of GraphQL POST. Inner provider blocks rolled up into a
//     single episode list; server choice = inner-provider name.
//   - GetStream: same shape — pick the best stream entry from the
//     sources response.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"encoding/json"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

// providerName is the stable identifier returned by Name() and used as the
// orchestrator's registry key. Backend slug = display label = "miruro".
const providerName = "miruro"

// requestReferer is the Referer header Miruro's pipe endpoint expects
// from non-SPA clients. Without this, the upstream often returns 403.
const requestReferer = "https://www.miruro.tv/"

// defaultEpisodePreference is the order in which we prefer inner-provider
// blocks when materializing the canonical episode list. Higher = first.
// Based on SPIKE-MIRURO.md observations: kiwi (animepahe-derived) returns
// the most reliable HLS endpoints; dune/hop/bee provide redundancy.
var defaultEpisodePreference = []string{"kiwi", "dune", "hop", "bee"}

// stageNames is the canonical stage list (mirrors allanime).
var stageNames = health.AllStages

// IDMapper is the narrow interface miruro.Provider consumes from
// libs/idmapping. Captured here as an interface so tests can pass a
// stub without standing up a real ARM HTTP server.
type IDMapper interface {
	ResolveByShikimoriID(id string) (*idmapping.MappingResult, error)
}

// Deps is the constructor input for New(). Required fields validated eagerly
// at construction time so main.go fatals on misconfig instead of a deferred
// 502 on the first request.
type Deps struct {
	BaseURL     string // default https://www.miruro.tv
	ProxyURL    string // VITE_PROXY_A — default https://pro.ultracloud.cc
	ProxyURLAlt string // VITE_PROXY_B — default https://pru.ultracloud.cc
	// ObfKey is the hex-decoded VITE_PIPE_OBF_KEY (16 bytes) used when
	// upstream sets `x-obfuscated: 2` on the response. Pass nil to fall
	// back to the upstream-pinned default constant (DefaultPipeObfKey).
	ObfKey    []byte
	HTTP      *domain.BaseHTTPClient
	Cache     cache.Cache
	IDMapping IDMapper
	Log       *logger.Logger
}

// DefaultPipeObfKey is the upstream-observed VITE_PIPE_OBF_KEY (16 bytes
// hex). Captured in testdata/env2.js on 2026-05-20; key was stable
// across ≥3 sequential env2.js fetches (SPIKE-MIRURO.md Gate 3). If
// upstream rotates this, callers can override by passing Deps.ObfKey.
const DefaultPipeObfKeyHex = "71951034f8fbcf53d89db52ceb3dc22c"

// Provider implements domain.Provider for Miruro.
type Provider struct {
	baseURL     string
	proxyURL    string
	proxyURLAlt string
	obfKey      []byte
	http        *domain.BaseHTTPClient
	cache       *cacheLayer
	idMap       IDMapper
	log         *logger.Logger

	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs a Provider. Required dependencies validated eagerly.
func New(d Deps) (*Provider, error) {
	if d.HTTP == nil {
		return nil, errors.New("miruro: Deps.HTTP is required")
	}
	if d.Cache == nil {
		return nil, errors.New("miruro: Deps.Cache is required")
	}
	if d.IDMapping == nil {
		return nil, errors.New("miruro: Deps.IDMapping is required")
	}
	if d.Log == nil {
		d.Log = logger.Default()
	}
	base := d.BaseURL
	if base == "" {
		base = DefaultMiruroHost
	}
	proxy := d.ProxyURL
	if proxy == "" {
		proxy = "https://pro.ultracloud.cc"
	}
	proxyAlt := d.ProxyURLAlt
	if proxyAlt == "" {
		proxyAlt = "https://pru.ultracloud.cc"
	}
	obfKey := d.ObfKey
	if len(obfKey) == 0 {
		k, err := DecodePipeKey(DefaultPipeObfKeyHex)
		if err != nil {
			// Should never happen — the constant is a 32-char hex string.
			return nil, fmt.Errorf("miruro: decoding default pipe key: %w", err)
		}
		obfKey = k
	}

	p := &Provider{
		baseURL:     strings.TrimRight(base, "/"),
		proxyURL:    strings.TrimRight(proxy, "/"),
		proxyURLAlt: strings.TrimRight(proxyAlt, "/"),
		obfKey:      obfKey,
		http:        d.HTTP,
		cache:       newCacheLayer(d.Cache),
		idMap:       d.IDMapping,
		log:         d.Log,
		stages:      make(map[string]domain.StageHealth, len(stageNames)),
	}
	// Optimistic seed (Up=true) matching the allanime/animekai convention.
	for _, s := range stageNames {
		p.stages[s] = domain.StageHealth{Up: true}
	}
	return p, nil
}

// Name returns "miruro".
func (p *Provider) Name() string { return providerName }

// markStage records the success/failure of one stage. Mirrors allanime.
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

// FindID resolves the catalog's AnimeRef into Miruro's AniList provider
// ID via libs/idmapping ARM. Cache key = ShikimoriID. Returns ErrNotFound
// when ARM has no mapping (rare for popular anime).
//
// Per Test 2 in the plan: empty ShikimoriID + non-empty Title falls
// back to ErrNotFound (Miruro is AniList-keyed end to end — fuzzy title
// search would require yet another endpoint; we keep the contract
// narrow per RESEARCH.md "Don't Hand-Roll").
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	// If the caller already knows the AniList ID, use it directly. This
	// is the fast path for re-entrant calls after the first ListEpisodes.
	if ref.AniListID != "" {
		p.markStage(health.StageSearch, nil)
		return ref.AniListID, nil
	}

	if ref.ShikimoriID == "" {
		err := domain.WrapNotFound(
			errors.New("empty Shikimori/MAL ID and no AniList ID"),
			"miruro: FindID needs a Shikimori or AniList ID")
		p.markStage(health.StageSearch, err)
		return "", err
	}

	if hit, ok := p.cache.getShowID(ctx, ref.ShikimoriID); ok {
		p.markStage(health.StageSearch, nil)
		return hit, nil
	}

	mapping, err := p.idMap.ResolveByShikimoriID(ref.ShikimoriID)
	if err != nil {
		// ARM is a remote service — treat lookup failure as ProviderDown
		// so the orchestrator falls through. We log enough to debug the
		// upstream issue at INFO level.
		werr := domain.WrapProviderDown(err, "miruro: ARM lookup")
		p.log.Infow("miruro: ARM lookup failed",
			"shikimori_id", ref.ShikimoriID,
			"error", err.Error())
		p.markStage(health.StageSearch, werr)
		return "", werr
	}
	if mapping == nil || mapping.AniList == nil {
		werr := domain.WrapNotFound(
			fmt.Errorf("no AniList mapping for Shikimori/MAL %s", ref.ShikimoriID),
			"miruro: FindID")
		p.markStage(health.StageSearch, werr)
		return "", werr
	}

	aniListID := strconv.Itoa(*mapping.AniList)
	p.cache.setShowID(ctx, ref.ShikimoriID, aniListID)
	p.markStage(health.StageSearch, nil)
	return aniListID, nil
}

// ListEpisodes returns the per-anime episode list as a flat slice.
// providerID = AniList ID (the return value of FindID).
//
// The upstream's episodes endpoint returns one block per inner provider
// (dune/kiwi/hop/bee/ANIMEKAI) with sub/dub variants. We surface the
// first preferred-provider block's sub track as the canonical episode
// list, falling back through defaultEpisodePreference. Inner-provider
// servers are surfaced separately via ListServers.
func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	aniListID := strings.TrimSpace(providerID)
	if aniListID == "" {
		err := domain.WrapExtractFailed(errors.New("empty AniList ID"), "miruro: ListEpisodes")
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}

	if hit, ok := p.cache.getEpisodes(ctx, aniListID); ok {
		p.markStage(health.StageEpisodes, nil)
		return materializeEpisodes(hit), nil
	}

	body, err := p.fetchPipe(ctx, "episodes", map[string]any{"anilistId": aniListID})
	if err != nil {
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}

	var resp episodesResponse
	if uerr := json.Unmarshal(body, &resp); uerr != nil {
		werr := domain.WrapExtractFailed(uerr, "miruro: parse episodes response")
		p.markStage(health.StageEpisodes, werr)
		return nil, werr
	}

	cached := normalizeEpisodes(resp)
	if len(cached) == 0 {
		// Real-empty (anime exists, no episodes aired yet) — return
		// ([]Episode{}, nil), not an error.
		p.markStage(health.StageEpisodes, nil)
		return []domain.Episode{}, nil
	}

	p.cache.setEpisodes(ctx, aniListID, cached)
	p.markStage(health.StageEpisodes, nil)
	return materializeEpisodes(cached), nil
}

// normalizeEpisodes flattens the per-inner-provider blocks into a single
// canonical list. We pick one provider's sub track as the "primary"
// episode listing (preferred-provider order). Each cachedEpisode keeps
// the inner-provider tag so ListServers can offer per-server fanout.
func normalizeEpisodes(resp episodesResponse) []cachedEpisode {
	// Pick the preferred provider's sub track. Fall back to any provider's
	// sub track if none of the preferred names appear.
	var picked string
	var pickedEps []rawEpisode
	for _, name := range defaultEpisodePreference {
		if block, ok := resp.Providers[name]; ok {
			if eps, ok := block.Episodes["sub"]; ok && len(eps) > 0 {
				picked = name
				pickedEps = eps
				break
			}
		}
	}
	if pickedEps == nil {
		// Fall back: take the first provider block that has a sub track.
		// Sort keys for deterministic behavior.
		keys := make([]string, 0, len(resp.Providers))
		for k := range resp.Providers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if eps, ok := resp.Providers[k].Episodes["sub"]; ok && len(eps) > 0 {
				picked = k
				pickedEps = eps
				break
			}
		}
	}
	if pickedEps == nil {
		return nil
	}

	out := make([]cachedEpisode, 0, len(pickedEps))
	for _, e := range pickedEps {
		out = append(out, cachedEpisode{
			ID:       e.ID,
			Number:   e.Number.Int(), // truncate fractional (One Piece 1004.5 → 1004; ISS-015)
			Title:    e.Title,
			Filler:   e.Filler,
			Provider: picked,
			Audio:    "sub",
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Number < out[j].Number
	})
	return out
}

// materializeEpisodes converts cached episode rows to domain.Episode.
func materializeEpisodes(eps []cachedEpisode) []domain.Episode {
	out := make([]domain.Episode, 0, len(eps))
	for _, e := range eps {
		title := e.Title
		if title == "" {
			title = fmt.Sprintf("Episode %d", e.Number)
		}
		out = append(out, domain.Episode{
			ID:       e.ID,
			Number:   e.Number,
			Title:    title,
			IsFiller: e.Filler,
		})
	}
	return out
}

// ListServers returns the streaming servers Miruro lists for one
// episode. For Miruro, "server" = inner-provider name (kiwi/dune/...).
// Each inner provider can be probed with a separate sources call.
//
// providerID = AniList ID; episodeID = upstream-opaque episode ID
// (the same string we surfaced as Episode.ID in ListEpisodes).
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	aniListID := strings.TrimSpace(providerID)
	if aniListID == "" || strings.TrimSpace(episodeID) == "" {
		err := domain.WrapExtractFailed(
			fmt.Errorf("empty providerID=%q or episodeID=%q", providerID, episodeID),
			"miruro: ListServers")
		p.markStage(health.StageServers, err)
		return nil, err
	}

	if hit, ok := p.cache.getServers(ctx, aniListID, episodeID); ok {
		p.markStage(health.StageServers, nil)
		return materializeServers(hit), nil
	}

	// We re-fetch the episodes payload (cheaper than a separate
	// per-episode lookup) and surface the inner-provider blocks whose
	// sub/dub arrays contain a matching episode ID.
	body, err := p.fetchPipe(ctx, "episodes", map[string]any{"anilistId": aniListID})
	if err != nil {
		p.markStage(health.StageServers, err)
		return nil, err
	}
	var resp episodesResponse
	if uerr := json.Unmarshal(body, &resp); uerr != nil {
		werr := domain.WrapExtractFailed(uerr, "miruro: parse episodes for servers")
		p.markStage(health.StageServers, werr)
		return nil, werr
	}

	srvs := matchingServers(resp, episodeID)
	if len(srvs) == 0 {
		// The episode ID was not found in any inner provider's list. This
		// is ErrNotFound (the orchestrator can fall through to the next
		// provider) rather than ErrExtractFailed (which signals parser
		// regression).
		werr := domain.WrapNotFound(
			fmt.Errorf("no inner-provider matches episode %s", episodeID),
			"miruro: ListServers")
		p.markStage(health.StageServers, werr)
		return nil, werr
	}

	p.cache.setServers(ctx, aniListID, episodeID, srvs)
	p.markStage(health.StageServers, nil)
	return materializeServers(srvs), nil
}

// matchingServers returns inner-provider blocks whose sub/dub arrays
// contain the requested episode ID. Sorted by defaultEpisodePreference
// then alphabetically.
func matchingServers(resp episodesResponse, episodeID string) []cachedServer {
	var out []cachedServer
	for name, block := range resp.Providers {
		for audio, list := range block.Episodes {
			if audio != "sub" && audio != "dub" {
				continue
			}
			for _, e := range list {
				if e.ID == episodeID {
					out = append(out, cachedServer{
						Name: name,
						Type: audio,
						EpID: episodeID,
					})
					break
				}
			}
		}
	}
	// Preferred providers first, then alphabetical.
	prefIndex := func(n string) int {
		for i, p := range defaultEpisodePreference {
			if p == n {
				return i
			}
		}
		return len(defaultEpisodePreference)
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi := prefIndex(out[i].Name)
		pj := prefIndex(out[j].Name)
		if pi != pj {
			return pi < pj
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Type < out[j].Type
	})
	return out
}

func materializeServers(srvs []cachedServer) []domain.Server {
	out := make([]domain.Server, 0, len(srvs))
	for _, s := range srvs {
		cat := domain.CategorySub
		if s.Type == "dub" {
			cat = domain.CategoryDub
		}
		out = append(out, domain.Server{
			ID:   s.Name,
			Name: s.Name,
			Type: cat,
		})
	}
	return out
}

// GetStream resolves one (server, episode) tuple to a playable stream.
// serverID = inner-provider name (kiwi/dune/...); falls back to the
// preferred-provider default when empty.
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	aniListID := strings.TrimSpace(providerID)
	if aniListID == "" || strings.TrimSpace(episodeID) == "" {
		err := domain.WrapExtractFailed(
			fmt.Errorf("empty providerID=%q or episodeID=%q", providerID, episodeID),
			"miruro: GetStream")
		p.markStage(health.StageStream, err)
		return nil, err
	}
	if strings.TrimSpace(serverID) == "" {
		serverID = defaultEpisodePreference[0]
	}

	if hit, ok := p.cache.getStream(ctx, aniListID, episodeID, serverID); ok {
		p.markStage(health.StageStream, nil)
		return cachedToStream(hit), nil
	}

	// Build the sources query. anilistId is REQUIRED by upstream as of
	// 2026-06 — the secure-pipe `sources` endpoint returns HTTP 400
	// {"error":"anilistId is required"} without it (the `episodes` endpoint
	// already sends it; this one regressed). Category is optional; upstream
	// defaults to whatever audio track the episode block exposes for the
	// given (provider, episodeID) tuple.
	q := map[string]any{
		"anilistId": aniListID,
		"episodeId": episodeID,
		"provider":  serverID,
	}
	if category != "" {
		q["category"] = string(category)
	}

	body, err := p.fetchPipe(ctx, "sources", q)
	if err != nil {
		p.markStage(health.StageStream, err)
		return nil, err
	}

	var resp sourcesResponse
	if uerr := json.Unmarshal(body, &resp); uerr != nil {
		werr := domain.WrapExtractFailed(uerr, "miruro: parse sources response")
		p.markStage(health.StageStream, werr)
		return nil, werr
	}

	if len(resp.Streams) == 0 {
		werr := domain.WrapExtractFailed(
			fmt.Errorf("empty streams[] for %s ep %s server %s", aniListID, episodeID, serverID),
			"miruro: GetStream")
		p.markStage(health.StageStream, werr)
		return nil, werr
	}

	// Pick the highest-quality stream. Upstream typically labels these
	// "1080p" / "720p" / "auto"; sort by numeric value when available.
	pick := pickBestStream(resp.Streams)
	if pick.URL == "" || !strings.HasPrefix(pick.URL, "http") {
		werr := domain.WrapExtractFailed(
			fmt.Errorf("non-URL stream entry: %+v", pick),
			"miruro: GetStream")
		p.markStage(health.StageStream, werr)
		return nil, werr
	}

	streamType := strings.ToLower(pick.Type)
	if streamType == "" {
		if strings.Contains(strings.ToLower(pick.URL), ".m3u8") {
			streamType = "hls"
		} else if strings.Contains(strings.ToLower(pick.URL), ".mp4") {
			streamType = "mp4"
		} else {
			streamType = "hls"
		}
	}

	stream := &domain.Stream{
		Sources: []domain.Source{
			{
				URL:     pick.URL,
				Type:    streamType,
				Quality: defaultQuality(pick.Quality),
			},
		},
		Headers: map[string]string{},
	}
	if pick.Referer != "" {
		stream.Headers["Referer"] = pick.Referer
	}

	// Cache the resolved stream.
	p.cache.setStream(ctx, aniListID, episodeID, serverID, &cachedStream{
		URL:     stream.Sources[0].URL,
		Type:    stream.Sources[0].Type,
		Quality: stream.Sources[0].Quality,
		Headers: stream.Headers,
	})

	p.markStage(health.StageStream, nil)
	return stream, nil
}

func defaultQuality(q string) string {
	if strings.TrimSpace(q) == "" {
		return "auto"
	}
	return q
}

// pickBestStream selects the highest-quality entry. Higher numeric quality
// wins; entries without numeric labels fall back to "auto" priority.
func pickBestStream(in []rawStream) rawStream {
	if len(in) == 0 {
		return rawStream{}
	}
	best := in[0]
	bestScore := qualityScore(best.Quality)
	for _, s := range in[1:] {
		sc := qualityScore(s.Quality)
		if sc > bestScore {
			best = s
			bestScore = sc
		}
	}
	return best
}

// qualityScore parses "1080p" / "720p" / "auto" / "" into a comparable int.
// "auto" sorts low; empty sorts lowest; numeric labels sort by value.
func qualityScore(q string) int {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return -1
	}
	q = strings.TrimSuffix(q, "p")
	if q == "auto" {
		return 0
	}
	if n, err := strconv.Atoi(q); err == nil {
		return n
	}
	return 0
}

func cachedToStream(c *cachedStream) *domain.Stream {
	return &domain.Stream{
		Sources: []domain.Source{
			{URL: c.URL, Type: c.Type, Quality: c.Quality},
		},
		Headers: c.Headers,
	}
}

// fetchPipe issues the secure-pipe GET, retries against the alt proxy
// once on 5xx/transport failure, and returns the decoded JSON body.
//
// The endpoint is the upstream-relative path (e.g. "episodes",
// "info/154587", "sources"); query is the JSON-shaped query map that
// the SPA would have passed as the descriptor's `query` field.
func (p *Provider) fetchPipe(ctx context.Context, endpoint string, query map[string]any) ([]byte, error) {
	// Primary attempt: base host (the SPA's actual call path).
	body, attemptErr := p.doSecurePipe(ctx, p.baseURL, endpoint, query)
	if attemptErr == nil {
		return body, nil
	}

	// On ProviderDown (5xx/transport), retry once via the configured
	// proxy fallback. ExtractFailed (parse / 4xx) does not retry —
	// the upstream answered with something we can't read.
	if !errors.Is(attemptErr, domain.ErrProviderDown) {
		return nil, attemptErr
	}
	if p.proxyURL == "" {
		return nil, attemptErr
	}
	p.log.Debugw("miruro: primary host failed, retrying via proxy",
		"endpoint", endpoint,
		"proxy", p.proxyURL,
		"error", attemptErr.Error())
	body, retryErr := p.doSecurePipe(ctx, p.proxyURL, endpoint, query)
	if retryErr == nil {
		return body, nil
	}
	return nil, retryErr
}

// doSecurePipe performs a single attempt against the named host.
func (p *Provider) doSecurePipe(ctx context.Context, host, endpoint string, query map[string]any) ([]byte, error) {
	pipeURL, err := BuildSecurePipeURL(host, endpoint, query)
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "miruro: build pipe URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pipeURL, nil)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "miruro: build request")
	}
	// Referer is upstream-required (the SPA always sets it; bare clients
	// occasionally get 403 without it).
	req.Header.Set("Referer", requestReferer)
	req.Header.Set("Accept", "*/*")

	resp, err := p.http.Do(ctx, req)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "miruro: http")
	}
	defer resp.Body.Close()

	// Defensive read cap — obfuscation.gunzipCapped enforces a stricter
	// post-gunzip cap, but we also bound the raw response.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "miruro: read body")
	}

	if resp.StatusCode >= 500 {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"miruro: upstream 5xx")
	}
	if resp.StatusCode >= 400 {
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"miruro: 4xx")
	}

	xobf := resp.Header.Get("x-obfuscated")
	decoded, derr := DecodeObfuscatedResponse(body, xobf, p.obfKey)
	if derr != nil {
		return nil, domain.WrapExtractFailed(derr, "miruro: decode response")
	}
	// Soft-cap early warning (ISS-015). When a decoded payload crosses
	// ~75% of MaxDecodedResponseBytes, log a Warn so ops sees the trend
	// before the next One Piece-class growth blows through the hard cap.
	// The log includes both the absolute size and the cap utilization
	// percent so the operator can spot a runaway upstream from one line.
	if len(decoded) > SoftCapWarnBytes {
		p.log.Warnw("miruro: decoded response approaching hard cap",
			"endpoint", endpoint,
			"decoded_bytes", len(decoded),
			"soft_cap_bytes", SoftCapWarnBytes,
			"hard_cap_bytes", MaxDecodedResponseBytes,
			"cap_utilization_pct", (100*len(decoded))/MaxDecodedResponseBytes)
	}
	return decoded, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Compile-time assertion: Provider satisfies domain.Provider.
var _ domain.Provider = (*Provider)(nil)
