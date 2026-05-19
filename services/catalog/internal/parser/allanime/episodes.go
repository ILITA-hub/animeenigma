package allanime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Search queries AllAnime for shows matching `query` and returns the first
// page of results. Caller is expected to filter by best name match.
func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("allanime: empty search query")
	}

	vars, err := buildSearchVariables(query)
	if err != nil {
		return nil, err
	}
	ext := buildExtensions(c.effectiveSearchSHA())

	var resp struct {
		Data struct {
			Shows struct {
				Edges []struct {
					ID                string         `json:"_id"`
					Name              string         `json:"name"`
					EnglishName       string         `json:"englishName"`
					NativeName        string         `json:"nativeName"`
					ThumbnailURL      string         `json:"thumbnail"`
					AvailableEpisodes map[string]int `json:"availableEpisodes"`
				} `json:"edges"`
			} `json:"shows"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}

	if err := c.doGraphQL(ctx, SearchQuery, vars, ext, &resp); err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("allanime: query rejected (likely stale SHA): %s", string(resp.Errors[0]))
	}
	if len(resp.Data.Shows.Edges) == 0 {
		return nil, fmt.Errorf("allanime: no match for %q", query)
	}

	out := make([]SearchResult, 0, len(resp.Data.Shows.Edges))
	for _, e := range resp.Data.Shows.Edges {
		name := e.EnglishName
		if name == "" {
			name = e.Name
		}
		out = append(out, SearchResult{
			ID:       e.ID,
			Name:     name,
			JName:    e.NativeName,
			Poster:   e.ThumbnailURL,
			Episodes: e.AvailableEpisodes["raw"],
		})
	}
	return out, nil
}

// EpisodesByID returns the raw-translation-type episode list for a show.
// Episodes are sorted ascending by numeric order; non-numeric IDs (specials)
// sort to the end.
func (c *Client) EpisodesByID(ctx context.Context, showID string) ([]Episode, error) {
	if strings.TrimSpace(showID) == "" {
		return nil, errors.New("allanime: empty show ID")
	}

	vars, err := buildEpisodesVariables(showID)
	if err != nil {
		return nil, err
	}
	ext := buildExtensions(c.effectiveEpisodesSHA())

	var resp struct {
		Data struct {
			Show struct {
				ID                      string `json:"_id"`
				AvailableEpisodesDetail struct {
					Raw []string `json:"raw"`
				} `json:"availableEpisodesDetail"`
			} `json:"show"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}

	if err := c.doGraphQL(ctx, EpisodesQuery, vars, ext, &resp); err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("allanime: query rejected (likely stale SHA): %s", string(resp.Errors[0]))
	}

	rawList := resp.Data.Show.AvailableEpisodesDetail.Raw
	if len(rawList) == 0 {
		return nil, fmt.Errorf("allanime: no raw episodes for show %s", showID)
	}

	episodes := make([]Episode, 0, len(rawList))
	for _, epStr := range rawList {
		n, _ := strconv.Atoi(strings.TrimSpace(epStr))
		episodes = append(episodes, Episode{
			ID:     fmt.Sprintf("%s/%s", showID, epStr),
			Number: n,
			Title:  fmt.Sprintf("Episode %s", epStr),
		})
	}

	sort.SliceStable(episodes, func(i, j int) bool {
		if episodes[i].Number == 0 && episodes[j].Number == 0 {
			return episodes[i].ID < episodes[j].ID
		}
		if episodes[i].Number == 0 {
			return false
		}
		if episodes[j].Number == 0 {
			return true
		}
		return episodes[i].Number < episodes[j].Number
	})

	return episodes, nil
}

// RawStream resolves a playable HLS stream for an episode. The episodeID is
// the composite "showID/episodeString" returned by EpisodesByID.
func (c *Client) RawStream(ctx context.Context, episodeID string) (Stream, error) {
	parts := strings.SplitN(episodeID, "/", 2)
	if len(parts) != 2 {
		return Stream{}, fmt.Errorf("allanime: invalid episode ID %q (want showID/episodeString)", episodeID)
	}
	showID, epStr := parts[0], parts[1]

	vars, err := buildSourcesVariables(showID, epStr)
	if err != nil {
		return Stream{}, err
	}
	ext := buildExtensions(c.effectiveSourcesSHA())

	var resp struct {
		Data struct {
			Episode struct {
				SourceUrls []struct {
					SourceURL  string  `json:"sourceUrl"`
					SourceName string  `json:"sourceName"`
					Type       string  `json:"type"`
					Priority   float64 `json:"priority"`
					Sandbox    string  `json:"sandbox"`
					Subtitles  []struct {
						SourceURL string `json:"src"`
						Lang      string `json:"lang"`
						Label     string `json:"label"`
					} `json:"subtitles"`
				} `json:"sourceUrls"`
			} `json:"episode"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}

	if err := c.doGraphQL(ctx, SourcesQuery, vars, ext, &resp); err != nil {
		return Stream{}, err
	}
	if len(resp.Errors) > 0 {
		return Stream{}, fmt.Errorf("allanime: query rejected (likely stale SHA): %s", string(resp.Errors[0]))
	}
	if len(resp.Data.Episode.SourceUrls) == 0 {
		return Stream{}, fmt.Errorf("allanime: no sources for episode %s", episodeID)
	}

	// Sort by priority descending; pick the highest-priority source whose URL
	// looks like a playable stream (http(s)://...).
	sources := resp.Data.Episode.SourceUrls
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].Priority > sources[j].Priority
	})

	for _, s := range sources {
		url := decodeSourceURL(s.SourceURL)
		if !strings.HasPrefix(url, "http") {
			continue
		}
		subs := make([]Subtitle, 0, len(s.Subtitles))
		for _, sub := range s.Subtitles {
			subs = append(subs, Subtitle{
				URL:   sub.SourceURL,
				Lang:  sub.Lang,
				Label: sub.Label,
			})
		}
		return Stream{
			URL:       url,
			Type:      streamTypeFromURL(url),
			Quality:   "auto",
			Subtitles: subs,
		}, nil
	}

	return Stream{}, fmt.Errorf("allanime: no playable source URL for episode %s", episodeID)
}

// doGraphQL GETs a single persisted query against the active domain. On
// transport failure it marks the domain failed and bubbles the error up so
// the caller can decide whether to wrap as ServiceUnavailable.
//
// The gqlQuery argument is the full GraphQL operation string; we always
// send it alongside the persisted-query extension so Apollo can auto-
// register the operation under the SHA on cache miss.
func (c *Client) doGraphQL(ctx context.Context, gqlQuery, vars, ext string, out any) error {
	domain, err := c.pickDomain(ctx)
	if err != nil {
		return err
	}

	endpoint := c.endpointURL(domain, gqlQuery, vars, ext)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("allanime: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Referer", c.cfg.Referer)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.markDomainFailed()
		return fmt.Errorf("allanime: http %s: %w", domain, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("allanime: read body: %w", err)
	}

	if resp.StatusCode >= 500 {
		c.markDomainFailed()
		return fmt.Errorf("allanime: upstream %d from %s: %s", resp.StatusCode, domain, truncate(string(body), 200))
	}
	if resp.StatusCode >= 400 {
		// 4xx (often persisted-query miss / stale SHA). Don't mark the
		// domain failed — the host is alive; the request was rejected.
		return fmt.Errorf("allanime: %d from %s: %s", resp.StatusCode, domain, truncate(string(body), 200))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("allanime: parse json from %s: %w", domain, err)
	}
	return nil
}

// decodeSourceURL applies AllAnime's lightweight URL obfuscation. Sources
// returned by the GraphQL `sourceUrl` field are sometimes prefixed with
// "--" followed by a hex-encoded redirect URL that points to the playable
// HLS. If the prefix is absent, the URL is returned as-is.
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
	// AllAnime XORs each byte with 56 (legacy lightweight obfuscation
	// observed in ani-cli). Apply the same transform.
	for i := range out {
		out[i] ^= 56
	}
	decoded := string(out)
	// The decoded form is typically `/apivtwo/clock?id=...` (a relative
	// redirect endpoint). Convert it into an absolute URL on the current
	// active domain. If the decoded value already looks like a full URL,
	// return as-is.
	if strings.HasPrefix(decoded, "http") {
		return decoded
	}
	// Without the active domain context here we leave it relative — the
	// resolver layer can resolve it, but in practice for v0.1 we treat
	// only fully-qualified URLs as playable (the priority sort skips
	// non-http sources). This means we'll return Stream{} if all sources
	// are obfuscated; the caller falls back to other providers.
	return decoded
}

// streamTypeFromURL guesses the stream container type from the URL suffix.
func streamTypeFromURL(u string) string {
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

// truncate caps a string at n runes for safe inclusion in error messages.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
