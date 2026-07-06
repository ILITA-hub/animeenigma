package allanimeokru

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

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// discoveryDeps is the constructor input for newDiscovery(). Required fields
// validated eagerly.
type discoveryDeps struct {
	// BaseURL overrides the AllAnime API endpoint. Empty → https://api.allanime.day.
	BaseURL string
	HTTP    *domain.BaseHTTPClient
	Cache   cache.Cache
	Log     *logger.Logger
}

// discovery is the pure AllAnime GraphQL discovery client (FindID /
// ListEpisodes / episodeSourceURLs). It carries no health/stage state — the
// provider half (Provider, in client.go) marks stages for the combined
// discovery+stream surface.
type discovery struct {
	baseURL string
	http    *domain.BaseHTTPClient
	cache   *cacheLayer
	log     *logger.Logger
}

// newDiscovery constructs a discovery client. Required dependencies validated
// eagerly so main.go fatals on a misconfiguration instead of a deferred 502 on
// the first request (SCRAPER-FOUND mirroring gogoanime/animepahe/animekai).
//
// Default BaseURL is https://api.allanime.day (the canonical AllAnime API
// host; allanime.day / allmanga.to / allanime.to are domain aliases).
func newDiscovery(d discoveryDeps) (*discovery, error) {
	if d.HTTP == nil {
		return nil, errors.New("allanimeokru discovery: Deps.HTTP is required")
	}
	if d.Cache == nil {
		return nil, errors.New("allanimeokru discovery: Deps.Cache is required")
	}
	if d.Log == nil {
		d.Log = logger.Default()
	}
	base := d.BaseURL
	if base == "" {
		base = "https://api.allanime.day"
	}
	return &discovery{
		baseURL: strings.TrimRight(base, "/"),
		http:    d.HTTP,
		cache:   newCacheLayer(d.Cache),
		log:     d.Log,
	}, nil
}

// FindID resolves the catalog's AnimeRef into an AllAnime show `_id` by
// searching by title (AllAnime's search is fuzzy on title; we narrow on
// best match). Falls back to ErrNotFound when no edges match.
func (d *discovery) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	// Cache key uses MAL ID when available; otherwise title as a weaker key.
	cacheKey := ref.ShikimoriID
	if cacheKey == "" {
		cacheKey = ref.Title
	}
	if cacheKey != "" {
		if hit, ok := d.cache.getShowID(ctx, cacheKey); ok {
			return hit, nil
		}
	}

	query := strings.TrimSpace(ref.Title)
	if query == "" {
		err := domain.WrapNotFound(errors.New("empty title"), "allanime: FindID needs a title")
		return "", err
	}

	vars, err := buildSearchVariables(query)
	if err != nil {
		err = domain.WrapExtractFailed(err, "allanime: buildSearchVariables")
		return "", err
	}
	ext := buildExtensions(SHASearchFallback)

	var resp searchShowsResponse
	if doErr := d.doGraphQL(ctx, SearchQuery, vars, ext, &resp); doErr != nil {
		return "", doErr
	}
	if len(resp.Data.Shows.Edges) == 0 {
		err := domain.WrapNotFound(fmt.Errorf("no edges for %q", query), "allanime: FindID")
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
		return "", err
	}

	if cacheKey != "" {
		d.cache.setShowID(ctx, cacheKey, pick.ID)
	}
	return pick.ID, nil
}

// categoriesFromDetail returns the EN categories ("sub"/"dub") the show has.
// Raw is excluded — that's the Raw player's domain, not OurEnglish.
func categoriesFromDetail(d availableEpisodesDetail) []string {
	cats := make([]string, 0, 2)
	if len(d.Sub) > 0 {
		cats = append(cats, "sub")
	}
	if len(d.Dub) > 0 {
		cats = append(cats, "dub")
	}
	return cats
}

// fetchShowDetail runs the EpisodesByID show query and returns the show's
// availableEpisodesDetail. Shared by ListEpisodes and categoriesFor.
func (d *discovery) fetchShowDetail(ctx context.Context, showID string) (availableEpisodesDetail, error) {
	vars, err := buildEpisodesVariables(showID)
	if err != nil {
		return availableEpisodesDetail{}, domain.WrapExtractFailed(err, "allanime: buildEpisodesVariables")
	}
	ext := buildExtensions(SHAEpisodesFallback)
	var resp showResponse
	if doErr := d.doGraphQL(ctx, EpisodesQuery, vars, ext, &resp); doErr != nil {
		return availableEpisodesDetail{}, doErr
	}
	return resp.Data.Show.AvailableEpisodesDetail, nil
}

// categoriesFor returns the show's EN categories, cache-first. On a miss it
// does one show-detail query and caches the result. Any failure or an
// empty/unknown detail degrades to sub-only (conservative) and is NOT cached.
func (d *discovery) categoriesFor(ctx context.Context, showID string) []string {
	if cats, ok := d.cache.getCategories(ctx, showID); ok && len(cats) > 0 {
		return cats
	}
	detail, err := d.fetchShowDetail(ctx, showID)
	if err != nil {
		return []string{"sub"}
	}
	cats := categoriesFromDetail(detail)
	if len(cats) == 0 {
		return []string{"sub"}
	}
	d.cache.setCategories(ctx, showID, cats)
	return cats
}

// ListEpisodes returns the episode list for one AllAnime show ID. EpisodeIDs
// are formatted as "<showID>:<episodeString>" so downstream calls can split
// the original episodeString back out (matches the catalog-side convention
// but with `:` rather than `/` to avoid colliding with shikimori_id paths).
func (d *discovery) ListEpisodes(ctx context.Context, showID string) ([]domain.Episode, error) {
	if strings.TrimSpace(showID) == "" {
		err := domain.WrapExtractFailed(errors.New("empty showID"), "allanime: ListEpisodes")
		return nil, err
	}

	if hit, ok := d.cache.getEpisodes(ctx, showID); ok {
		return materializeEpisodes(showID, hit), nil
	}

	detail, derr := d.fetchShowDetail(ctx, showID)
	if derr != nil {
		return nil, derr
	}
	// Cache which EN categories the show actually has, so ListServers probes
	// only those (raw excluded — that's the Raw player's domain, not OurEnglish).
	// setCategories no-ops on empty so a sub-only or not-yet-aired show is fine.
	d.cache.setCategories(ctx, showID, categoriesFromDetail(detail))
	raw := detail.Sub

	if len(raw) == 0 {
		// Real-empty (anime exists, no episodes aired yet) is `([], nil)`.
		return []domain.Episode{}, nil
	}

	d.cache.setEpisodes(ctx, showID, raw)
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

// translationTypeFor maps a domain.Category to AllAnime's translationType enum.
func translationTypeFor(c domain.Category) string {
	switch c {
	case domain.CategoryDub:
		return "dub"
	case domain.CategoryRaw:
		return "raw"
	default:
		return "sub"
	}
}

// namedSource is a view of one decoded AllAnime source: its upstream
// sourceName ("Ok", "Default", "S-mp4", …) and the decoded, fully-qualified
// embed/stream URL. Used by the provider half (Provider, in client.go) to
// reuse this discovery's GraphQL + cache and resolve a specific source family
// without duplicating the persisted-query / decrypt code.
type namedSource struct {
	Name string
	URL  string
}

// episodeSourceURLs returns the decoded sources for one episode+category via
// the same fetchSources path (and Redis cache) used by the legacy
// ListServers/GetStream. episodeID is "<showID>:<episodeString>". A
// foreign/invalid ID → ErrNotFound, so the caller (the provider half) is
// skipped by the orchestrator instead of marked DOWN.
func (d *discovery) episodeSourceURLs(ctx context.Context, episodeID string, category domain.Category) ([]namedSource, error) {
	showID, ep := splitEpisodeID(episodeID)
	if showID == "" || ep == "" {
		return nil, domain.WrapNotFound(
			fmt.Errorf("invalid episode ID %q", episodeID),
			"allanime: episodeSourceURLs")
	}
	tt := translationTypeFor(category)
	srcs, hit := d.cache.getServers(ctx, showID, ep, tt)
	if !hit {
		fetched, ferr := d.fetchSources(ctx, showID, ep, tt)
		if ferr != nil {
			return nil, ferr
		}
		d.cache.setServers(ctx, showID, ep, tt, fetched)
		srcs = fetched
	}
	out := make([]namedSource, 0, len(srcs))
	for _, s := range srcs {
		name := s.SourceName
		if name == "" {
			name = "Default"
		}
		out = append(out, namedSource{Name: name, URL: decodeSourceURL(s.SourceURL)})
	}
	return out, nil
}

// fetchSources POSTs the SourceUrls APQ and returns the (decrypted, if
// needed) sourceUrls array.
func (d *discovery) fetchSources(ctx context.Context, showID, ep, translationType string) ([]sourceURL, error) {
	vars, err := buildSourcesVariables(showID, ep, translationType)
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "allanime: buildSourcesVariables")
	}
	ext := buildExtensions(SHASourcesFallback)

	var env episodeEnvelope
	if doErr := d.doGraphQL(ctx, "", vars, ext, &env); doErr != nil {
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
func (d *discovery) doGraphQL(ctx context.Context, gqlQuery, vars, ext string, out any) error {
	v := url.Values{}
	if gqlQuery != "" {
		v.Set("query", gqlQuery)
	}
	v.Set("variables", vars)
	v.Set("extensions", ext)
	endpoint := fmt.Sprintf("%s/api?%s", d.baseURL, v.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.WrapProviderDown(err, "allanime: build request")
	}
	req.Header.Set("Referer", apiReferer)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", apiUA)

	resp, err := d.http.Do(ctx, req)
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
