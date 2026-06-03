package eighteenanime

import (
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	baseURL   = "https://18anime.me"
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/126 Safari/537.36"
)

// Client is an HTTP client for the 18anime.me provider.
type Client struct{ httpClient *http.Client }

// NewClient returns a Client with a sensible default timeout.
func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 8 * time.Second}}
}

// SearchHit represents a single parsed result from the search results page.
type SearchHit struct {
	Slug string
	URL  string
}

// resultHrefRe matches hentai episode/series anchors on the search results page.
// Example: href="https://18anime.me/hentai/1167-jk-to-inkou-kyoushi-4-feat-ero-giin-sensei-episode-2.html"
var resultHrefRe = regexp.MustCompile(`href="(https?://18anime\.me/hentai/([0-9]+-[a-z0-9-]+)\.html)"`)

// parseSearchResults extracts unique SearchHit values from a search results HTML page.
// Each anchor typically appears twice (thumbnail + title link); duplicates are dropped.
func parseSearchResults(html string) []SearchHit {
	seen := map[string]bool{}
	var hits []SearchHit
	for _, m := range resultHrefRe.FindAllStringSubmatch(html, -1) {
		url, slug := m[1], m[2]
		if seen[slug] {
			continue
		}
		seen[slug] = true
		hits = append(hits, SearchHit{Slug: slug, URL: url})
	}
	return hits
}

// normalize strips all non-alphanumeric characters and lowercases s so that
// title strings can be compared against URL slugs.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bestMatch scores hits against the wanted title using normalized substring
// containment first, then per-token overlap as a fallback.
// Returns nil when no hit exceeds a zero score.
func bestMatch(title string, hits []SearchHit) *SearchHit {
	want := normalize(title)
	var best *SearchHit
	bestScore := -1
	for i := range hits {
		slugNorm := normalize(hits[i].Slug)
		score := 0
		if want != "" && (strings.Contains(slugNorm, want) || strings.Contains(want, slugNorm)) {
			score = len(want)
		} else {
			for _, tok := range strings.Fields(strings.ToLower(title)) {
				if len(tok) > 2 && strings.Contains(slugNorm, normalize(tok)) {
					score++
				}
			}
		}
		if score > bestScore {
			bestScore, best = score, &hits[i]
		}
	}
	if bestScore <= 0 {
		return nil
	}
	return best
}
