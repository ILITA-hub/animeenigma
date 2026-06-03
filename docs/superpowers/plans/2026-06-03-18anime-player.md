# 18anime (18+) Player Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new, separate 18+ video player "18anime" sourced from `18anime.me`, extracting playable streams from the mp4upload and turbovid embed mirrors with per-episode failover.

**Architecture:** Self-contained catalog parser (`services/catalog/internal/parser/eighteenanime/`) mirroring the Hanime provider — `Search → ListEpisodes → GetServers → GetStream`. Two embed extractors (mp4upload plaintext `player.src`, turbovid jwplayer `file:`), failover-ordered. New `Anime18Player.vue` surface + provider sub-tab in `Anime.vue`, gated by `VITE_ANIME18_ENABLED` (default off). Streams play via the existing `/api/streaming/hls-proxy` (Referer injection + allowlist).

**Tech Stack:** Go (catalog parser, golden-fixture tests), Vue 3 + TypeScript (Plyr/hls.js player), existing `libs/videoutils` HLS proxy.

**Spec:** `docs/superpowers/specs/2026-06-03-18anime-player-design.md`

**Spike results (verified live 2026-06-03 — these are facts, not assumptions):**
- Episode page `https://18anime.me/hentai/<id>-<slug>-episode-N.html` embeds an inline JSON array of mirrors: `{"link":"<embed-url>","quality":"FullHD"}` (hosts incl. `mp4upload.com/embed-<id>.html`, `turbovidhls.com/t/<id>`).
- **mp4upload** embed page contains a plaintext `player.src('https://aN.mp4upload.com:183/d/<token>/video.mp4')`. Stream returns **403 without** `Referer: https://www.mp4upload.com/`, **206 with** it → MUST proxy with Referer.
- **turbovid** embed page (`turbovidhls.com/t/<id>`) is jwplayer with `sources:[{file:"https://cdnN.turboviplay.com/data3/<id>/<id>.m3u8"}]`. Master m3u8 (200, no Referer needed) references nested variants on `*.turbosplayer.com` (also 200, no Referer). CODECS present in manifest (avoids hls.js codec-less regression).
- Proxy supports per-request Referer: `ProxyWithReferer(ctx, sourceURL, referer, w, r)` and `rewriteM3U8URLs` propagates `&referer=` through child URLs. Allowlist entries: `{Domain, Reason, Owner, Added}` in `HLSProxyAllowedDomainsWithProvenance`, prefix-wildcard supported.

---

## Naming conventions (locked)

| Concern | Value |
|---|---|
| Go package / dir | `eighteenanime` — `services/catalog/internal/parser/eighteenanime/` (Go identifiers cannot start with a digit) |
| URL path segment | `anime18` — `/api/anime/{id}/anime18/episodes`, `/anime18/stream` |
| Frontend provider key | `videoProvider === 'anime18'` |
| Frontend component | `Anime18Player.vue` |
| Feature flag | `VITE_ANIME18_ENABLED` (default off) |
| i18n namespace | `player.anime18.*` |
| User-facing label | `18anime` |

---

## Task 1: Capture golden test fixtures

**Files:**
- Create: `services/catalog/internal/parser/eighteenanime/testdata/episode_page.html`
- Create: `services/catalog/internal/parser/eighteenanime/testdata/embed_mp4upload.html`
- Create: `services/catalog/internal/parser/eighteenanime/testdata/embed_turbovid.html`
- Create: `services/catalog/internal/parser/eighteenanime/testdata/search_results.html`

- [ ] **Step 1: Create the package dir and capture real fixtures**

```bash
mkdir -p services/catalog/internal/parser/eighteenanime/testdata
UA="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/126 Safari/537.36"
D=services/catalog/internal/parser/eighteenanime/testdata
# Episode page (mirror-list JSON)
curl -s -A "$UA" "https://18anime.me/hentai/1167-jk-to-inkou-kyoushi-4-feat-ero-giin-sensei-episode-2.html" -o "$D/episode_page.html"
# mp4upload embed (plaintext player.src) — capture a CURRENT one from the episode page's JSON
curl -s -A "$UA" -e "https://18anime.me/" "https://www.mp4upload.com/embed-ul2ul4tmzcxu.html" -o "$D/embed_mp4upload.html"
# turbovid embed (jwplayer file:)
curl -s -A "$UA" -e "https://18anime.me/" "https://turbovidhls.com/t/69d4c45bc180a" -o "$D/embed_turbovid.html"
# Search results page (DataLife Engine search) — capture via the site search
curl -s -A "$UA" --data-urlencode "do=search" --data-urlencode "subaction=search" --data-urlencode "story=inkou kyoushi" "https://18anime.me/index.php?do=search" -o "$D/search_results.html"
```

- [ ] **Step 2: Verify fixtures contain expected markers**

Run:
```bash
D=services/catalog/internal/parser/eighteenanime/testdata
grep -c '"link"' "$D/episode_page.html"          # expect >= 2 (mirror array)
grep -c 'player.src' "$D/embed_mp4upload.html"    # expect >= 1
grep -c 'turboviplay.com' "$D/embed_turbovid.html" # expect >= 1
```
Expected: each count ≥ 1. If `search_results.html` is empty/blocked, note the actual DLE search URL form discovered and re-capture; record the working search request shape in a comment for Task 4.

- [ ] **Step 3: Commit fixtures**

```bash
git add services/catalog/internal/parser/eighteenanime/testdata/
git commit -m "test(18anime): capture golden HTML fixtures for parser + extractors"
```

---

## Task 2: mp4upload embed extractor

**Files:**
- Create: `services/catalog/internal/parser/eighteenanime/embed_mp4upload.go`
- Test: `services/catalog/internal/parser/eighteenanime/embed_mp4upload_test.go`

- [ ] **Step 1: Write the failing test**

```go
package eighteenanime

import (
	"os"
	"testing"
)

func TestExtractMP4Upload(t *testing.T) {
	html, err := os.ReadFile("testdata/embed_mp4upload.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	src, err := extractMP4Upload(string(html))
	if err != nil {
		t.Fatalf("extractMP4Upload: %v", err)
	}
	if src.URL == "" {
		t.Fatal("empty URL")
	}
	if !contains(src.URL, "mp4upload.com") || !contains(src.URL, ".mp4") {
		t.Fatalf("unexpected URL: %s", src.URL)
	}
	if src.Referer != "https://www.mp4upload.com/" {
		t.Fatalf("expected mp4upload referer, got %q", src.Referer)
	}
	if src.IsHLS {
		t.Fatal("mp4upload should be MP4, not HLS")
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int   { for i := 0; i+len(sub) <= len(s); i++ { if s[i:i+len(sub)] == sub { return i } }; return -1 }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/eighteenanime/ -run TestExtractMP4Upload -v`
Expected: FAIL — `extractMP4Upload` / `ExtractedSource` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
package eighteenanime

import (
	"fmt"
	"regexp"
)

// ExtractedSource is a resolved playable stream from an embed mirror.
type ExtractedSource struct {
	URL     string // direct mp4 or m3u8 URL
	Referer string // Referer the proxy must inject ("" if none required)
	IsHLS   bool   // true => m3u8, false => progressive mp4
	Quality string // e.g. "1080", "FullHD"
}

// player.src('https://a4.mp4upload.com:183/d/<token>/video.mp4')
var mp4uploadSrcRe = regexp.MustCompile(`player\.src\(\s*['"](https?://[^'"]+\.mp4[^'"]*)['"]`)

func extractMP4Upload(html string) (*ExtractedSource, error) {
	m := mp4uploadSrcRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil, fmt.Errorf("eighteenanime: mp4upload player.src not found")
	}
	return &ExtractedSource{
		URL:     m[1],
		Referer: "https://www.mp4upload.com/",
		IsHLS:   false,
		Quality: "FullHD",
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/parser/eighteenanime/ -run TestExtractMP4Upload -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/parser/eighteenanime/embed_mp4upload.go services/catalog/internal/parser/eighteenanime/embed_mp4upload_test.go
git commit -m "feat(18anime): mp4upload embed extractor (plaintext player.src)"
```

---

## Task 3: turbovid embed extractor

**Files:**
- Create: `services/catalog/internal/parser/eighteenanime/embed_turbovid.go`
- Test: `services/catalog/internal/parser/eighteenanime/embed_turbovid_test.go`

- [ ] **Step 1: Write the failing test**

```go
package eighteenanime

import (
	"os"
	"testing"
)

func TestExtractTurbovid(t *testing.T) {
	html, err := os.ReadFile("testdata/embed_turbovid.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	src, err := extractTurbovid(string(html))
	if err != nil {
		t.Fatalf("extractTurbovid: %v", err)
	}
	if !contains(src.URL, ".m3u8") {
		t.Fatalf("expected m3u8, got %s", src.URL)
	}
	if !src.IsHLS {
		t.Fatal("turbovid should be HLS")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/eighteenanime/ -run TestExtractTurbovid -v`
Expected: FAIL — `extractTurbovid` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
package eighteenanime

import (
	"fmt"
	"regexp"
)

// jwplayer sources: [{ file: "https://cdn4.turboviplay.com/data3/<id>/<id>.m3u8" ... }]
var turbovidFileRe = regexp.MustCompile(`["']?file["']?\s*:\s*["'](https?://[^"']+\.m3u8[^"']*)["']`)

func extractTurbovid(html string) (*ExtractedSource, error) {
	m := turbovidFileRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil, fmt.Errorf("eighteenanime: turbovid m3u8 file not found")
	}
	return &ExtractedSource{
		URL:     m[1],
		Referer: "", // master + nested turbosplayer.com variants need no Referer (verified)
		IsHLS:   true,
		Quality: "FullHD",
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/parser/eighteenanime/ -run TestExtractTurbovid -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/parser/eighteenanime/embed_turbovid.go services/catalog/internal/parser/eighteenanime/embed_turbovid_test.go
git commit -m "feat(18anime): turbovid embed extractor (jwplayer file: m3u8)"
```

---

## Task 4: Client scaffold + Search (title → slug)

**Files:**
- Create: `services/catalog/internal/parser/eighteenanime/client.go`
- Test: `services/catalog/internal/parser/eighteenanime/client_test.go`

> **Implementer note:** Inspect `testdata/search_results.html` (and `testdata/episode_page.html`) to confirm the exact DataLife Engine search-result anchor structure before writing the regex/selector. The site lists results as `<a href="https://18anime.me/hentai/<id>-<slug>.html">`. Match the input title against the slug, normalizing both (lowercase, strip non-alphanumerics) and scoring by token overlap — mirror the title-scoring approach in `services/scraper/internal/providers/` (see `project_scraper_multititle_matching` learnings: score against romaji + English forms).

- [ ] **Step 1: Write the failing test** (against the captured search fixture; adjust the expected slug to whatever the fixture actually contains)

```go
package eighteenanime

import (
	"os"
	"testing"
)

func TestParseSearchResults(t *testing.T) {
	html, err := os.ReadFile("testdata/search_results.html")
	if err != nil {
		t.Skip("no search fixture captured")
	}
	hits := parseSearchResults(string(html))
	if len(hits) == 0 {
		t.Fatal("expected >=1 hit")
	}
	for _, h := range hits {
		if h.Slug == "" || h.URL == "" {
			t.Fatalf("bad hit: %+v", h)
		}
	}
}

func TestBestMatch(t *testing.T) {
	hits := []SearchHit{
		{Slug: "1167-jk-to-inkou-kyoushi-4-feat-ero-giin-sensei", URL: "https://18anime.me/hentai/1167-...html"},
		{Slug: "99-some-other-title", URL: "https://18anime.me/hentai/99-...html"},
	}
	got := bestMatch("JK to Inkou Kyoushi 4", hits)
	if got == nil || got.Slug != hits[0].Slug {
		t.Fatalf("bestMatch picked wrong hit: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/eighteenanime/ -run 'TestParseSearchResults|TestBestMatch' -v`
Expected: FAIL — undefined `parseSearchResults`, `SearchHit`, `bestMatch`.

- [ ] **Step 3: Write minimal implementation**

```go
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

type Client struct{ httpClient *http.Client }

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 8 * time.Second}}
}

type SearchHit struct {
	Slug string
	URL  string
}

// matches: href="https://18anime.me/hentai/1167-jk-to-...-episode-2.html"
var resultHrefRe = regexp.MustCompile(`href="(https?://18anime\.me/hentai/([0-9]+-[a-z0-9-]+)\.html)"`)

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

func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func bestMatch(title string, hits []SearchHit) *SearchHit {
	want := normalize(title)
	var best *SearchHit
	bestScore := -1
	for i := range hits {
		slugNorm := normalize(hits[i].Slug)
		score := 0
		if strings.Contains(slugNorm, want) || strings.Contains(want, slugNorm) {
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/parser/eighteenanime/ -run 'TestParseSearchResults|TestBestMatch' -v`
Expected: PASS (ParseSearchResults skips if no fixture).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/parser/eighteenanime/client.go services/catalog/internal/parser/eighteenanime/client_test.go
git commit -m "feat(18anime): client scaffold + search/title-match"
```

---

## Task 5: ListEpisodes + GetServers (mirror-list parse)

**Files:**
- Modify: `services/catalog/internal/parser/eighteenanime/client.go`
- Test: `services/catalog/internal/parser/eighteenanime/episodes_test.go` (create)

> **Implementer note (RESOLVED from fixtures 2026-06-03):** 18anime has **NO series page** — each episode is its own page (`/hentai/<id>-<slug>-episode-N.html`), with a distinct numeric id per episode. Confirmed against `testdata/search_results.html`: searching a series title returns **every** episode as a separate anchor (`1164-jk-to-inkou-kyoushi-4-episode-1`, `1165-…-episode-2`, …). So episode enumeration reuses the **search results page**, not a series page:
> 1. `baseSlug(slug)` = strip the leading `<id>-` and the trailing episode marker (`-episode-N` OR a bare `-N`), lowercased. e.g. `1167-jk-to-inkou-kyoushi-4-feat-ero-giin-sensei-episode-2` → `jk-to-inkou-kyoushi-4-feat-ero-giin-sensei`.
> 2. Group all search hits by `baseSlug`; keep the group whose base **exactly equals** the matched hit's base. **Exact, not prefix** — `jk-to-inkou-kyoushi-4` (eps 1164/1165) and `jk-to-inkou-kyoushi-4-feat-ero-giin-sensei` (eps 1166/1167) are distinct series sharing a prefix.
> 3. `episodeNumber(slug)` parses the trailing `-episode-N` or bare `-N`; default to 1 if absent. Sort ascending, dedupe by number.
>
> The mirror-list JSON (`{"link":…,"quality":…}`) is confirmed present in `episode_page.html` (10 `"link"` entries). `parseEpisodeMirrors` parses that; `ListEpisodes` does the grouping above.

- [ ] **Step 1: Write the failing test**

```go
package eighteenanime

import (
	"os"
	"testing"
)

func TestParseEpisodeMirrors(t *testing.T) {
	html, err := os.ReadFile("testdata/episode_page.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	mirrors := parseEpisodeMirrors(string(html))
	if len(mirrors) == 0 {
		t.Fatal("expected >=1 mirror")
	}
	var sawMP4Upload, sawTurbo bool
	for _, m := range mirrors {
		if contains(m.Link, "mp4upload") {
			sawMP4Upload = true
		}
		if contains(m.Link, "turbovid") {
			sawTurbo = true
		}
	}
	if !sawMP4Upload && !sawTurbo {
		t.Fatal("expected at least one supported mirror (mp4upload/turbovid)")
	}
}

func TestEpisodeEnumeration(t *testing.T) {
	// Exact-base grouping: the "-feat-ero-giin-sensei" series must NOT absorb
	// the shorter "jk-to-inkou-kyoushi-4" series even though it's a prefix.
	hits := []SearchHit{
		{Slug: "1164-jk-to-inkou-kyoushi-4-episode-1", URL: "u1"},
		{Slug: "1165-jk-to-inkou-kyoushi-4-episode-2", URL: "u2"},
		{Slug: "1166-jk-to-inkou-kyoushi-4-feat-ero-giin-sensei-episode-1", URL: "u3"},
		{Slug: "1167-jk-to-inkou-kyoushi-4-feat-ero-giin-sensei-episode-2", URL: "u4"},
		{Slug: "2171-inkou-kyoushi-no-saimin-seikatsu-shidouroku-1", URL: "u5"}, // bare -N form
	}
	eps := episodesFromHits("jk-to-inkou-kyoushi-4", hits)
	if len(eps) != 2 || eps[0].Number != 1 || eps[1].Number != 2 {
		t.Fatalf("expected 2 eps [1,2], got %+v", eps)
	}
	if got := episodeNumber("2171-inkou-kyoushi-no-saimin-seikatsu-shidouroku-1"); got != 1 {
		t.Fatalf("bare -N episode number: want 1 got %d", got)
	}
	if got := baseSlug("2171-inkou-kyoushi-no-saimin-seikatsu-shidouroku-1"); got != "inkou-kyoushi-no-saimin-seikatsu-shidouroku" {
		t.Fatalf("bare -N base slug wrong: %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/eighteenanime/ -run TestParseEpisodeMirrors -v`
Expected: FAIL — undefined `parseEpisodeMirrors`, `Mirror`.

- [ ] **Step 3: Write minimal implementation** (append to `client.go` — merge any new imports into the file's single existing `import` block; do NOT add a second `import (...)`)

```go
type Mirror struct {
	Link    string
	Quality string
}

// inline JSON objects: {"link":"https://...","quality":"FullHD"}
var mirrorRe = regexp.MustCompile(`\{\s*"link"\s*:\s*"([^"]+)"\s*,\s*"quality"\s*:\s*"([^"]*)"\s*\}`)

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

// supportedMirrors keeps only embeds we have extractors for, in failover order.
func supportedMirrors(all []Mirror) []Mirror {
	order := []string{"mp4upload", "turbovid"}
	var out []Mirror
	for _, host := range order {
		for _, m := range all {
			if strings.Contains(m.Link, host) {
				out = append(out, m)
			}
		}
	}
	return out
}

// --- Episode enumeration (no series page; reuse the search-results anchors) ---

var episodeSuffixRe = regexp.MustCompile(`-(?:episode-)?([0-9]+)$`)

// baseSlug strips the leading "<id>-" and the trailing "-episode-N" / "-N".
func baseSlug(slug string) string {
	if i := strings.IndexByte(slug, '-'); i >= 0 {
		slug = slug[i+1:] // drop leading numeric id
	}
	return episodeSuffixRe.ReplaceAllString(slug, "")
}

// episodeNumber parses the trailing episode number; defaults to 1.
func episodeNumber(slug string) int {
	if m := episodeSuffixRe.FindStringSubmatch(slug); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 1
}

type Episode struct {
	Slug   string `json:"slug"`
	URL    string `json:"url"`
	Number int    `json:"number"`
}

// episodesFromHits groups search hits by exact base slug and returns the group
// matching wantBase, sorted ascending by episode number (deduped).
func episodesFromHits(wantBase string, hits []SearchHit) []Episode {
	byNum := map[int]Episode{}
	for _, h := range hits {
		if baseSlug(h.Slug) != wantBase {
			continue
		}
		n := episodeNumber(h.Slug)
		if _, ok := byNum[n]; !ok {
			byNum[n] = Episode{Slug: h.Slug, URL: h.URL, Number: n}
		}
	}
	out := make([]Episode, 0, len(byNum))
	for _, e := range byNum {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}
```
(New imports needed: `strconv`, `sort` — add them to the existing import block.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/parser/eighteenanime/ -run TestParseEpisodeMirrors -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/parser/eighteenanime/client.go services/catalog/internal/parser/eighteenanime/episodes_test.go
git commit -m "feat(18anime): parse episode mirror list + supported-mirror filter"
```

---

## Task 6: Public API — Search, ListEpisodes, GetStream (HTTP + failover)

**Files:**
- Modify: `services/catalog/internal/parser/eighteenanime/client.go`
- Test: `services/catalog/internal/parser/eighteenanime/client_test.go` (extend with an httptest server)

- [ ] **Step 1: Write the failing test** (network-free: spin an `httptest` server serving the fixtures, point the client at it)

```go
package eighteenanime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGetStreamFailover(t *testing.T) {
	mu, _ := os.ReadFile("testdata/embed_mp4upload.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/embed-"):
			w.Write(mu) // mp4upload-style page
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient()
	mirrors := []Mirror{{Link: srv.URL + "/embed-x.html", Quality: "FullHD"}}
	src, err := c.resolveStream(context.Background(), mirrors)
	if err != nil {
		t.Fatalf("resolveStream: %v", err)
	}
	if src.URL == "" {
		t.Fatal("empty resolved URL")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/eighteenanime/ -run TestGetStreamFailover -v`
Expected: FAIL — undefined `resolveStream`.

- [ ] **Step 3: Write minimal implementation** (append to `client.go`. **Merge these imports into the file's single existing `import (...)` block — Go does not allow a second import block in the same file.** New imports: `context`, `fmt`, `io`, `net/url`. Do NOT import `libs/logger` unless you actually log.)

```go
func (c *Client) fetch(ctx context.Context, u, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("eighteenanime: GET %s -> %d", u, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(b), err
}

// searchFetch issues the DataLife Engine POST search (confirmed POST by the spike).
func (c *Client) searchFetch(ctx context.Context, title string) (string, error) {
	form := url.Values{"do": {"search"}, "subaction": {"search"}, "story": {title}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/index.php?do=search", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("eighteenanime: search -> %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}

// extractorFor picks the extractor by embed host.
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

// resolveStream tries supported mirrors in order; first success wins. The whole
// loop is bounded by a 9s deadline so two dead mirrors can't serialize past the
// catalog/frontend 10s budget (fail-fast — see spec §7).
func (c *Client) resolveStream(ctx context.Context, mirrors []Mirror) (*ExtractedSource, error) {
	ctx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	supported := supportedMirrors(mirrors)
	if len(supported) == 0 {
		return nil, fmt.Errorf("eighteenanime: no supported mirrors")
	}
	var lastErr error
	for _, m := range supported {
		ex := extractorFor(m.Link)
		if ex == nil {
			continue
		}
		page, err := c.fetch(ctx, m.Link, baseURL+"/")
		if err != nil {
			lastErr = err
			continue
		}
		src, err := ex(page)
		if err != nil {
			lastErr = err
			continue
		}
		return src, nil
	}
	return nil, fmt.Errorf("eighteenanime: all mirrors failed: %w", lastErr)
}

// --- Public methods used by the handler/service ---

// Search returns the best-matching episode hit for a title (single anchor).
func (c *Client) Search(ctx context.Context, title string) (*SearchHit, error) {
	page, err := c.searchFetch(ctx, title)
	if err != nil {
		return nil, err
	}
	hit := bestMatch(title, parseSearchResults(page))
	if hit == nil {
		return nil, fmt.Errorf("eighteenanime: no match for %q", title)
	}
	return hit, nil
}

// ListEpisodes searches once and returns every episode sharing the matched
// series' exact base slug (18anime has no series page — see Task 5).
func (c *Client) ListEpisodes(ctx context.Context, title string) ([]Episode, error) {
	page, err := c.searchFetch(ctx, title)
	if err != nil {
		return nil, err
	}
	hits := parseSearchResults(page)
	best := bestMatch(title, hits)
	if best == nil {
		return nil, fmt.Errorf("eighteenanime: no match for %q", title)
	}
	eps := episodesFromHits(baseSlug(best.Slug), hits)
	if len(eps) == 0 {
		return nil, fmt.Errorf("eighteenanime: no episodes for %q", title)
	}
	return eps, nil
}

// GetStream resolves a playable source for an episode page URL.
func (c *Client) GetStream(ctx context.Context, episodeURL string) (*ExtractedSource, error) {
	page, err := c.fetch(ctx, episodeURL, baseURL+"/")
	if err != nil {
		return nil, err
	}
	return c.resolveStream(ctx, parseEpisodeMirrors(page))
}
```

> The episode `ep` param passed by the frontend is the episode **slug** (`<id>-<slug>-episode-N`); the service rebuilds the page URL as `baseURL + "/hentai/" + slug + ".html"` before calling `GetStream`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/parser/eighteenanime/ -v`
Expected: PASS (all package tests)

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/parser/eighteenanime/
git commit -m "feat(18anime): HTTP fetch + Search/GetStream with per-embed failover"
```

---

## Task 7: Catalog handlers + routes

**Files:**
- Modify: `services/catalog/internal/service/catalog.go` (instantiate client, add service methods)
- Modify: `services/catalog/internal/handler/catalog.go` (add `GetAnime18Episodes`, `GetAnime18Stream`)
- Modify: `services/catalog/internal/transport/router.go` (register routes)
- Test: `services/catalog/internal/handler/catalog_anime18_test.go` (create)

> **Pattern source:** mirror `GetHanimeEpisodes`/`GetHanimeStream` (handler `catalog.go:842-880`), the hanime route block (`router.go:148-150`), and hanime client wiring in `service/catalog.go:108-125`. The 18anime client takes no credentials (`eighteenanime.NewClient()`), so it is always available.

- [ ] **Step 1: Write the failing handler test** (uses a fake service interface returning canned episodes/stream; asserts 200 + JSON shape, and that an all-mirror-failure yields an explicit error status, NOT empty-200)

```go
// Assert: GetAnime18Stream with a service that returns ErrSourceUnavailable
// responds 503 (or a typed error body), never 200-with-empty-sources.
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/handler/ -run Anime18 -v`
Expected: FAIL — handlers undefined.

- [ ] **Step 3: Implement** — add `eighteenAnimeClient *eighteenanime.Client` to the service (init in `NewCatalogService`), service methods `Get18AnimeEpisodes(ctx, animeID)` / `Get18AnimeStream(ctx, animeID, episodeSlug)` (resolve the catalog anime's title, call `Search` → `ListEpisodes`/`GetStream`), and handlers mirroring the hanime ones. On total failure return an explicit error (`errors.Unavailable` / HTTP 503), never an empty 200. Register:

```go
// router.go, in the /{animeId} group, after the hanime block:
r.Get("/{animeId}/anime18/episodes", catalogHandler.GetAnime18Episodes)
r.Get("/{animeId}/anime18/stream", catalogHandler.GetAnime18Stream)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/handler/ ./internal/service/ -run 'Anime18|18anime' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/catalog.go services/catalog/internal/handler/catalog.go services/catalog/internal/transport/router.go services/catalog/internal/handler/catalog_anime18_test.go
git commit -m "feat(18anime): catalog service methods, handlers, and routes"
```

---

## Task 8: HLS-proxy allowlist additions

**Files:**
- Modify: `libs/videoutils/proxy.go` (add 3 domains to `HLSProxyAllowedDomainsWithProvenance`)
- Test: `libs/videoutils/proxy_test.go` (assert the new domains resolve as allowed)

- [ ] **Step 1: Write the failing test**

```go
func TestAnime18DomainsAllowed(t *testing.T) {
	// isHLSDomainAllowed is the package-level checker ProxyWithReferer uses; it
	// strips the port before matching, so mp4upload's :183 host resolves too.
	for _, host := range []string{
		"a4.mp4upload.com", "a4.mp4upload.com:183",
		"cdn4.turboviplay.com", "g276.turbosplayer.com",
	} {
		if !isHLSDomainAllowed(host) {
			t.Fatalf("expected %s allowed", host)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd libs/videoutils && go test -run TestAnime18DomainsAllowed -v`
Expected: FAIL — domains not allowed.

- [ ] **Step 3: Add the entries** (next to the Hanime CDN family block)

```go
// 18anime (18+) embed CDN families.
{Domain: "mp4upload.com", Reason: "18anime mp4upload mirror — requires Referer: https://www.mp4upload.com/", Owner: "@18anime", Added: "2026-06-03"},
{Domain: "turboviplay.com", Reason: "18anime turbovid master m3u8 host", Owner: "@18anime", Added: "2026-06-03"},
{Domain: "turbosplayer.com", Reason: "18anime turbovid nested variant/segment host", Owner: "@18anime", Added: "2026-06-03"},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd libs/videoutils && go test -run TestAnime18DomainsAllowed -v`
Expected: PASS. (`isHLSDomainAllowed` is package-level in `proxy.go` and reads the auto-derived `HLSProxyAllowedDomains` view of the provenance slice, so the 3 new entries flow through automatically.)

- [ ] **Step 5: Commit**

```bash
git add libs/videoutils/proxy.go libs/videoutils/proxy_test.go
git commit -m "feat(18anime): allowlist mp4upload + turbovid CDN hosts in HLS proxy"
```

---

## Task 9: Frontend API client + `Anime18Player.vue`

**Files:**
- Modify: `frontend/web/src/api/client.ts` (add `anime18Api`)
- Create: `frontend/web/src/components/player/Anime18Player.vue`
- Test: `frontend/web/src/components/player/__tests__/Anime18Player.spec.ts` (create)

- [ ] **Step 1: Add `anime18Api`** (next to `hanimeApi`, mirroring it)

```ts
export const anime18Api = {
  getEpisodes: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/anime18/episodes`),
  getStream: (animeId: string, episodeSlug: string) =>
    apiClient.get(`/anime/${animeId}/anime18/stream`, { params: { ep: episodeSlug } }),
}
```

- [ ] **Step 2: Create `Anime18Player.vue` by cloning `HanimePlayer.vue`** with these exact changes:
  - Replace `hanimeApi` import/usages with `anime18Api`; rename local types `HanimeEpisode`/`HanimeSource` → `Anime18Episode`/`Anime18Source`.
  - `getStream` second arg is the episode `slug` (string), matching the new API.
  - Stream proxy URL: when `source.isHLS` is false (mp4upload MP4) feed the MP4 through the proxy with `referer=https://www.mp4upload.com/`; when HLS (turbovid) feed the m3u8 (no referer needed). Build via the same `/api/streaming/hls-proxy?...` helper used at `HanimePlayer.vue:205`, adding the `referer` query param when present.
  - i18n source label `'18anime'`; `$t('player.noEpisodes', { source: '18anime' })`.

- [ ] **Step 3: Write the spec** (≥5 assertions: renders, loads episodes via `anime18Api`, picks best quality, builds proxy URL with referer for mp4upload, shows explicit error on unavailable). Run:

`cd frontend/web && bunx vitest run src/components/player/__tests__/Anime18Player.spec.ts`
Expected: PASS

- [ ] **Step 4: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: no errors. (Import shared player types from the `@/components/ui` barrel where applicable — see `feedback_vuetsc_noemit_false_pass`.)

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/api/client.ts frontend/web/src/components/player/Anime18Player.vue frontend/web/src/components/player/__tests__/Anime18Player.spec.ts
git commit -m "feat(18anime): Anime18Player.vue + anime18Api client"
```

---

## Task 10: Wire provider sub-tab in `Anime.vue` + i18n + flag

**Files:**
- Modify: `frontend/web/src/views/Anime.vue`
- Modify: `frontend/web/src/locales/en.json`, `frontend/web/src/locales/ru.json`
- Modify: `frontend/web/.env` / deploy env docs (declare `VITE_ANIME18_ENABLED`)
- Test: `frontend/web/src/locales/__tests__/*` parity (existing) + manual smoke

- [ ] **Step 1: Add the chip + player branch** (next to the Hanime chip near `Anime.vue:451`, and the `<HanimePlayer>` branch near `:558`):

```vue
<!-- chip -->
<button
  v-if="anime18Enabled"
  @click="videoProvider = 'anime18'"
  :class="videoProvider === 'anime18' ? activeChipClass : idleChipClass"
>18anime</button>

<!-- player branch — placed correctly within the v-if/v-else-if chain (see memory: Vue template chain rule) -->
<Anime18Player
  v-else-if="videoProvider === 'anime18'"
  :anime-id="anime.id"
  :initial-episode="initialEpisode"
/>
```

Add `const anime18Enabled = import.meta.env.VITE_ANIME18_ENABLED === 'true'` and a `defineAsyncComponent` import for `Anime18Player`. Confirm no non-conditional element is interleaved in the `v-if`/`v-else-if` chain.

- [ ] **Step 2: Add i18n keys to BOTH locales**

```jsonc
// en.json -> "player": { ... "anime18": { "tab": "18anime", "label": "18anime" } }
// ru.json -> "player": { ... "anime18": { "tab": "18anime", "label": "18anime" } }
```

- [ ] **Step 3: Run locale parity + type-check + lint**

Run:
```bash
cd frontend/web && bunx vitest run src/locales/__tests__/ && bunx tsc --noEmit && bash scripts/design-system-lint.sh
```
Expected: PASS, lint ERRORS=0.

- [ ] **Step 4: Build**

Run: `cd frontend/web && bun run build`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/views/Anime.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json
git commit -m "feat(18anime): provider sub-tab + i18n + VITE_ANIME18_ENABLED flag"
```

---

## Task 11: End-to-end verification (live)

**Files:** none (verification only)

- [ ] **Step 1: Redeploy backend + frontend**

```bash
make redeploy-catalog && make redeploy-web
```

- [ ] **Step 2: Backend smoke** (find an 18+ anime UUID in catalog, then):

```bash
curl -s "http://localhost:8081/api/anime/<UUID>/anime18/episodes" | head -c 300
curl -s "http://localhost:8081/api/anime/<UUID>/anime18/stream?ep=<slug>" | head -c 400
```
Expected: episodes JSON non-empty; stream JSON returns a resolved `url`. Confirm timing is well under 10s (fail-fast), and an unresolvable title returns an explicit 503, not empty-200.

- [ ] **Step 3: In-browser smoke** (set `VITE_ANIME18_ENABLED=true` in the web env): open an 18+ title, select the **18anime** tab, confirm playback (both an mp4upload-backed and a turbovid-backed title), desktop + mobile (DS-NF-06).

- [ ] **Step 4: Mark done.** Do NOT run `/animeenigma-after-update` yet — report results and await "ship it".

---

## Self-Review notes

- **Spec coverage:** §3 arch → Tasks 4–7; §4 backend → Tasks 2–7; §5 frontend → Tasks 9–10; §6 proxy/flag → Tasks 8,10; §7 resilience (fail-fast 8s client timeout, explicit 503, embed failover) → Tasks 4,6,7; §8 testing → every task is TDD + Task 11; §10 risk → de-risked by the live spike already done (extractors confirmed trivial).
- **Episode enumeration (RESOLVED):** confirmed from `search_results.html` — no series page; episodes are enumerated by exact-base-slug grouping of the search anchors (Task 5). Handles both `-episode-N` and bare `-N` suffix forms.
- **Known follow-ups (out of MVP scope):** abyss/bysesayeveum/upns embeds deferred; mp4upload de-pack fallback only if the bundle reverts to packed JS; if mp4upload itself rate-limits the shared Referer, revisit per-stream tokenization.
