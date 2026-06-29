package animejoy

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// fuzzyThreshold is the JaroWinkler floor for accepting a search candidate,
// matching the scraper's fuzzy fallback (RESEARCH Phase 16, 0.85).
const fuzzyThreshold = 0.85

// resultLinkRe matches an AnimeJoy news anchor:
//
//	<a … href="https://animejoy.ru/<section>/<news_id>-<slug>.html" …>inner</a>
//
// We bind the host to animejoy(a)?.ru so off-site decoys (e.g. the ajsub.online
// "Телеграм бот" promo) never enter the result set, capture the section and
// news_id, and keep the inner HTML for title extraction.
var resultLinkRe = regexp.MustCompile(
	`(?is)<a\b[^>]*\bhref="https?://animejoy[a]?\.ru/([a-z0-9-]+)/(\d+)-[^"]*\.html"[^>]*>(.*?)</a>`)

// tagRe strips HTML tags from a result's inner markup.
var tagRe = regexp.MustCompile(`(?s)<[^>]+>`)

// parseSearchResults extracts deduped news hits from a DLE search page. PURE:
// takes the raw HTML bytes, returns the hits in first-seen order. Each AnimeJoy
// result renders the news_id twice — once as the title link, once as a
// "Смотреть" (Watch) button — so we dedupe by news_id and prefer the row that
// carries a real title (the "Смотреть" placeholder is discarded).
func parseSearchResults(body []byte) []searchHit {
	src := string(body)
	order := []string{}
	byID := map[string]searchHit{}

	for _, m := range resultLinkRe.FindAllStringSubmatch(src, -1) {
		section, newsID, inner := m[1], m[2], m[3]
		title := cleanTitle(inner)
		// Skip the bare "Смотреть" button and any empty anchor.
		if title == "" || strings.EqualFold(title, "Смотреть") {
			// Still register the id so a later real title can attach, but never
			// overwrite an existing good title with the placeholder.
			if _, seen := byID[newsID]; !seen {
				byID[newsID] = searchHit{NewsID: newsID, Section: section}
				order = append(order, newsID)
			}
			continue
		}
		if existing, seen := byID[newsID]; seen {
			// Fill in a previously title-less placeholder row.
			if existing.Title == "" {
				existing.Title = title
				existing.Section = section
				byID[newsID] = existing
			}
			continue
		}
		byID[newsID] = searchHit{NewsID: newsID, Title: title, Section: section}
		order = append(order, newsID)
	}

	hits := make([]searchHit, 0, len(order))
	for _, id := range order {
		h := byID[id]
		// Drop rows that never got a usable title (orphan "Смотреть"-only ids).
		if h.Title == "" || strings.EqualFold(h.Title, "Смотреть") {
			continue
		}
		hits = append(hits, h)
	}
	return hits
}

// cleanTitle turns a result anchor's inner HTML into a plain title. AnimeJoy
// concatenates the Russian title, the "[N из M]" counter, and sometimes a
// romaji/english alt title with no separator (e.g.
// "Ты и я — полные противоположности (1 сезон) [12 из 12]Seihantai na Kimi…").
// We keep the leading Russian portion up to the "[…]" counter for the displayed
// title; matching uses foldSeason on this, and synonyms cover the alt forms.
func cleanTitle(inner string) string {
	t := tagRe.ReplaceAllString(inner, " ")
	t = html.UnescapeString(t)
	t = wsRe.ReplaceAllString(t, " ")
	t = strings.TrimSpace(t)
	// Truncate at the episode counter "[…]" — the alt title trails it.
	if i := strings.Index(t, "["); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	return t
}

// isTVSection reports whether a section is a series (vs film/ova/special).
func isTVSection(section string) bool {
	return section == "tv-serialy" || section == "tv-serials"
}

// kindIsTV reports whether the catalog Kind denotes an episodic series.
func kindIsTV(kind string) bool {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case "TV", "ONA", "SERIES", "":
		// Empty kind: assume series (the common case); section filter then
		// only excludes films/ova when we positively know it's a movie.
		return true
	default:
		return false
	}
}

// scoreAndPick selects the best news_id for q. PURE. Logic:
//  1. Section filter — when q is a TV/series kind, prefer tv-serialy and reject
//     film/ova rows; for a Movie/OVA kind, restrict to the matching section.
//  2. Fuzzy gate — max JaroWinkler over {q.Titles × candidate-title}, on
//     foldSeason-normalised strings, must be ≥ 0.85.
//  3. Tiebreak — among survivors, prefer the row whose detected season matches
//     q.Season; then the higher fuzzy score; then the lower news_id (stable).
//
// Returns ("", false) when nothing clears the gate.
func scoreAndPick(hits []searchHit, q Query) (string, bool) {
	type scored struct {
		hit    searchHit
		score  float64
		season int
		seasOK bool
	}

	wantTV := kindIsTV(q.Kind)
	var cands []scored
	for _, h := range hits {
		// Section filter.
		if wantTV {
			if !isTVSection(h.Section) {
				continue
			}
		} else {
			if !sectionMatchesKind(h.Section, q.Kind) {
				continue
			}
		}

		score := bestFuzzy(q.Titles, h.Title)
		if score < fuzzyThreshold {
			continue
		}
		seas := detectSeason(h.Title)
		cands = append(cands, scored{
			hit:    h,
			score:  score,
			season: seas,
			seasOK: q.Season > 0 && seas == q.Season,
		})
	}
	if len(cands) == 0 {
		return "", false
	}

	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		// 1. exact season match wins.
		if a.seasOK != b.seasOK {
			return a.seasOK
		}
		// 2. when no season requested (or neither matches exactly), prefer the
		// candidate whose detected season is closest to the requested one; a
		// season-1 default beats a higher season for a season-less query.
		if q.Season > 0 && a.season != b.season {
			return absDiff(a.season, q.Season) < absDiff(b.season, q.Season)
		}
		// 3. higher fuzzy score.
		if a.score != b.score {
			return a.score > b.score
		}
		// 4. stable: lower news_id.
		return a.hit.NewsID < b.hit.NewsID
	})
	return cands[0].hit.NewsID, true
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// sectionMatchesKind maps a non-TV catalog kind to the AnimeJoy section.
func sectionMatchesKind(section, kind string) bool {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case "MOVIE", "FILM":
		return section == "anime-films"
	case "OVA":
		return section == "ova"
	case "ONA":
		return section == "ona" || section == "tv-serialy"
	case "SPECIAL":
		return section == "ova" || section == "anime-films"
	default:
		return true
	}
}

// bestFuzzy returns the highest JaroWinkler score between any query title and
// the candidate, both folded via foldSeason.
func bestFuzzy(titles []string, cand string) float64 {
	fc := foldSeason(cand)
	best := 0.0
	for _, t := range titles {
		if s := jaroWinkler(foldSeason(t), fc); s > best {
			best = s
		}
	}
	return best
}

// seasonNumRe pulls the season number out of a Russian title, e.g.
// "… (2 сезон)" → 2, "… (1 сезон) [25 из 25]" → 1. Falls back to the Latin
// "season N" form for synonym-shaped titles.
var seasonNumRe = regexp.MustCompile(`(?i)(\d+)\s*сезон|сезон\s*(\d+)|season\s*(\d+)|(\d+)(?:nd|rd|st|th)\s*season`)

// detectSeason returns the season number embedded in a title, or 1 when none is
// present (AnimeJoy omits "(1 сезон)" on some single-season entries, but our
// fixtures carry it; defaulting to 1 keeps a bare title from out-ranking the
// explicit season-1 row for a season-1 query).
func detectSeason(title string) int {
	m := seasonNumRe.FindStringSubmatch(strings.ToLower(title))
	if m == nil {
		return 1
	}
	for _, g := range m[1:] {
		if g != "" {
			return atoiSafe(g)
		}
	}
	return 1
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// ResolveNewsID is the thin HTTP wrapper around the pure search core: it issues
// the DLE search request for the first query title, parses + scores the results,
// and returns the winning news_id. Phase 1 keeps it dependency-light; the 24h
// positive/negative cache from the spec is layered on in a later phase via the
// optional Cache interface on Client.
func (c *Client) ResolveNewsID(ctx context.Context, q Query) (string, error) {
	if len(q.Titles) == 0 {
		return "", fmt.Errorf("animejoy: ResolveNewsID called with no titles")
	}
	story := q.Titles[0]

	u := fmt.Sprintf("%s/index.php?do=search&subaction=search&story=%s",
		c.base(), url.QueryEscape(story))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("animejoy: build search request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("animejoy: search request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("animejoy: search HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("animejoy: read search body: %w", err)
	}

	hits := parseSearchResults(body)
	id, ok := scoreAndPick(hits, q)
	if !ok {
		return "", fmt.Errorf("animejoy: no search match for %q (season=%d kind=%s)", story, q.Season, q.Kind)
	}
	return id, nil
}
