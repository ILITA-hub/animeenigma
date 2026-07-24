// Package eighteenanime is the 18anime.me (18+) scraper provider. It is
// registered ONLY into the dedicated adult Orchestrator (a separate failover
// chain), never the EN chain, so 18+ content can't leak into the OurEnglish
// player. Ported from services/catalog/internal/parser/eighteenanime.
package eighteenanime

import (
	"net/url"
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

// tokenStopwords are common short English words that recur across unrelated
// titles and slugs. Without this guard a single incidental stopword overlap
// (e.g. "the") was enough for the per-token fallback below to score > 0 and
// declare an unrelated series the bestMatch — confirmed live: "Imaizumi
// Brings All the Gyarus to His House" matched an unrelated slug solely via
// its "the-animation" segment, serving the wrong hentai title as "episode 4"
// (AUTO-593). Mirrors the AUTO-630 empty-normalized-token guard in spirit.
var tokenStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "was": true,
	"were": true, "all": true, "his": true, "her": true, "its": true,
	"who": true, "you": true, "she": true, "him": true, "not": true,
	"but": true, "out": true, "has": true, "had": true, "can": true,
}

// minFallbackTokenLen is the minimum NORMALIZED length a title token must
// have to count in the per-token fallback below. Live-verified follow-up to
// the stopword guard above: after excluding stopwords, the SAME title still
// false-matched a second unrelated slug via "chi" (from "Imaizumi**n Chi**
// wa...") landing as a coincidental 3-letter substring inside "e-cchi" —
// not a stopword, just too short to be a reliable signal. Any 1-3 char token
// has a large collision surface against hyphen-stripped slugs; requiring 4+
// chars closes this class generally instead of enumerating more exceptions.
const minFallbackTokenLen = 4

// bestMatch scores hits against the wanted title (normalized containment first,
// then per-token overlap, skipping stopwords and short tokens). Returns nil
// when no hit exceeds a zero score.
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
				if tokenStopwords[tok] {
					continue
				}
				nt := normalize(tok)
				if len(nt) >= minFallbackTokenLen && strings.Contains(slugNorm, nt) {
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

// supportedServer maps a canonical embed host to its client-facing server ID
// and extractor. Matching is by PARSED hostname (equality or strict subdomain),
// never substring over the full URL — so https://mp4upload.evil.com/ or
// https://x/?turbovid=1 do not pretend to match (finding #52).
type supportedServer struct {
	id        string // server ID exposed to clients ("mp4upload" / "turbovid")
	host      string // canonical embed host
	extractor func(string) (*ExtractedSource, error)
}

// supportedServers is the mp4upload->turbovid failover order.
var supportedServers = []supportedServer{
	{id: "mp4upload", host: "mp4upload.com", extractor: extractMP4Upload},
	{id: "turbovid", host: "turbovidhls.com", extractor: extractTurbovid},
}

// hostMatches reports whether link's parsed host equals, or is a strict
// subdomain of, want — rejecting non-http(s) schemes up front. Mirrors the
// embeds/kwik.go Matches() policy so mirror selection can't be spoofed by a
// host token appearing in the path/query or as a fake subdomain.
func hostMatches(link, want string) bool {
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	return host == want || strings.HasSuffix(host, "."+want)
}

// supportedMirrors filters mirrors to supported embed hosts, in failover order.
func supportedMirrors(all []Mirror) []Mirror {
	var out []Mirror
	for _, srv := range supportedServers {
		for _, m := range all {
			if hostMatches(m.Link, srv.host) {
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

// extractorFor picks an extractor function by parsed embed host.
func extractorFor(link string) func(string) (*ExtractedSource, error) {
	for _, srv := range supportedServers {
		if hostMatches(link, srv.host) {
			return srv.extractor
		}
	}
	return nil
}

// serverIDFor returns the server ID for a mirror link, matched by parsed host.
func serverIDFor(link string) string {
	for _, srv := range supportedServers {
		if hostMatches(link, srv.host) {
			return srv.id
		}
	}
	return ""
}

// EpisodeURL builds the canonical episode page URL from an episode slug.
func EpisodeURL(slug string) string {
	return baseURL + "/hentai/" + slug + ".html"
}
