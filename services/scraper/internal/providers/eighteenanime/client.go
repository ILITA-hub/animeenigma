// Package eighteenanime is the 18anime.me (18+) scraper provider. It is
// registered ONLY into the dedicated adult Orchestrator (a separate failover
// chain), never the EN chain, so 18+ content can't leak into the OurEnglish
// player. Ported from services/catalog/internal/parser/eighteenanime.
package eighteenanime

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	baseURL   = "https://18anime.me"
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/126 Safari/537.36"
)

// SearchHit represents a single parsed result from the search results page.
type SearchHit struct {
	Slug string
	URL  string
}

// resultHrefRe matches hentai episode/series anchors on the search results page.
var resultHrefRe = regexp.MustCompile(`href="(https?://18anime\.me/hentai/([0-9]+-[a-z0-9-]+)\.html)"`)

// parseSearchResults extracts unique SearchHit values from a search results page.
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

// normalize strips all non-alphanumeric characters and lowercases s.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bestMatch scores hits against the wanted title (normalized containment first,
// then per-token overlap). Returns nil when no hit exceeds a zero score.
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

// Mirror is a single playback mirror for an episode.
type Mirror struct {
	Link    string
	Quality string
}

// mirrorRe matches multi-line pretty-printed mirror JSON ("link"/"quality").
var mirrorRe = regexp.MustCompile(`"link"\s*:\s*"([^"]+)"\s*,?\s*"quality"\s*:\s*"([^"]*)"`)

// parseEpisodeMirrors extracts unique Mirror values from an episode page.
func parseEpisodeMirrors(html string) []Mirror {
	seen := map[string]bool{}
	var out []Mirror
	for _, m := range mirrorRe.FindAllStringSubmatch(html, -1) {
		link := strings.ReplaceAll(m[1], `\/`, `/`)
		if seen[link] {
			continue
		}
		seen[link] = true
		out = append(out, Mirror{Link: link, Quality: m[2]})
	}
	return out
}

// supportedServerOrder is the failover order of supported embed hosts; the
// server ID exposed to clients is the host token ("mp4upload" / "turbovid").
var supportedServerOrder = []string{"mp4upload", "turbovid"}

// supportedMirrors filters mirrors to supported embed hosts, in failover order.
func supportedMirrors(all []Mirror) []Mirror {
	var out []Mirror
	for _, host := range supportedServerOrder {
		for _, m := range all {
			if strings.Contains(m.Link, host) {
				out = append(out, m)
			}
		}
	}
	return out
}

var idPrefixRe = regexp.MustCompile(`^[0-9]+-`)
var epSuffixRe = regexp.MustCompile(`-(?:episode-)?([0-9]+)$`)

// baseSlugAndEpisode splits a hit slug into (baseSlug, episodeNumber). The
// numeric id prefix and trailing episode suffix are stripped; number defaults to 1.
func baseSlugAndEpisode(slug string) (string, int) {
	s := idPrefixRe.ReplaceAllString(slug, "")
	ep := 1
	if m := epSuffixRe.FindStringSubmatch(s); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			ep = n
		}
		s = strings.TrimSuffix(s, m[0])
	}
	return s, ep
}

// extractorFor picks an extractor function by embed host.
func extractorFor(link string) func(string) (*ExtractedSource, error) {
	switch {
	case strings.Contains(link, "mp4upload"):
		return extractMP4Upload
	case strings.Contains(link, "turbovid"):
		return extractTurbovid
	default:
		return nil
	}
}

// serverIDFor returns the host-token server ID for a mirror link.
func serverIDFor(link string) string {
	for _, host := range supportedServerOrder {
		if strings.Contains(link, host) {
			return host
		}
	}
	return ""
}

// EpisodeURL builds the canonical episode page URL from an episode slug.
func EpisodeURL(slug string) string {
	return baseURL + "/hentai/" + slug + ".html"
}
