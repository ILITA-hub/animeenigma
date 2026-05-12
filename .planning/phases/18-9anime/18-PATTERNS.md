# Phase 18: 9anime → Anitaku/Gogoanime — Pattern Map

**Mapped:** 2026-05-12
**Files analyzed:** 22 new / modified
**Analogs found:** 21 / 22 (one new file — `fuzzy/` package — has a clear move source rather than an analog)

> Phase 18 pivots from "9anime" to **Anitaku/Gogoanime** at `anitaku.to` (per `18-RESEARCH.md` §Mirror Viability + `18-UI-SPEC.md` provider-naming-contract). Backend package slug is `gogoanime`; user-facing display label is `Anitaku`. This pattern map references the slug `gogoanime` for all file paths and code identifiers.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `services/scraper/internal/providers/gogoanime/client.go` | provider (controller-like) | request-response (HTML scrape + cache) | `services/scraper/internal/providers/animepahe/client.go` | exact |
| `services/scraper/internal/providers/gogoanime/dto.go` | model (DTO) | transform | `services/scraper/internal/providers/animepahe/dto.go` | exact (DTOs differ; layout identical) |
| `services/scraper/internal/providers/gogoanime/malsync.go` | service (external client) | request-response | `services/scraper/internal/providers/animepahe/malsync.go` | exact (same MalSyncClient contract; provider="Gogoanime" likely missing → forward-compat) |
| `services/scraper/internal/providers/gogoanime/cache.go` | utility (TTL math + helpers) | transform | `services/scraper/internal/providers/animepahe/cache.go` | exact (computeStreamTTL identical; normalizeTitle moves to fuzzy pkg) |
| `services/scraper/internal/providers/gogoanime/client_test.go` | test | offline goldens | `services/scraper/internal/providers/animepahe/client_test.go` | exact |
| `services/scraper/internal/providers/gogoanime/dto_test.go` | test | golden parse | `services/scraper/internal/providers/animepahe/dto_test.go` | exact |
| `services/scraper/internal/providers/gogoanime/malsync_test.go` | test | http.testServer | `services/scraper/internal/providers/animepahe/malsync_test.go` | exact |
| `services/scraper/internal/providers/gogoanime/cache_test.go` | test | unit | `services/scraper/internal/providers/animepahe/cache_test.go` | exact |
| `services/scraper/internal/fuzzy/jarowinkler.go` | utility (shared) | transform | move from `services/scraper/internal/providers/animepahe/cache.go::jaroWinkler` | move (no analog — same code, new home) |
| `services/scraper/internal/fuzzy/normalize.go` | utility (shared) | transform | move from `services/scraper/internal/providers/animepahe/cache.go::normalizeTitle` | move |
| `services/scraper/internal/fuzzy/fuzzy_test.go` | test | unit | implicit (currently covered inside animepahe tests) | re-derive |
| `services/scraper/internal/embeds/vibeplayer.go` | service (embed extractor) | request-response (regex extract) | `services/scraper/internal/embeds/kwik.go` (structure) + `services/scraper/internal/embeds/megacloud.go` (Matches/Name shape) | role-match (no goja; regex-only) |
| `services/scraper/internal/embeds/vibeplayer_test.go` | test | offline goldens | `services/scraper/internal/embeds/kwik_test.go` | exact |
| `services/scraper/internal/embeds/streamhg.go` | service (embed extractor) | request-response (goja unpack) | `services/scraper/internal/embeds/kwik.go` | exact (same Dean-Edwards packer; different host allowlist + Referer) |
| `services/scraper/internal/embeds/streamhg_test.go` | test | offline goldens | `services/scraper/internal/embeds/kwik_test.go` | exact |
| `services/scraper/internal/embeds/earnvids.go` | service (embed extractor) | request-response (goja unpack) | `services/scraper/internal/embeds/kwik.go` (identical to streamhg.go) | exact |
| `services/scraper/internal/embeds/earnvids_test.go` | test | offline goldens | `services/scraper/internal/embeds/kwik_test.go` | exact |
| `services/scraper/cmd/scraper-api/main.go` | config (wiring) | startup | self — already wires animepahe + probe runner | exact (incremental append) |
| `services/scraper/internal/config/config.go` | config | env | self — already has `AnimePaheConfig` | exact (incremental append) |
| `libs/videoutils/proxy.go` | utility (CORS allowlist) | config-data | self — `HLSProxyAllowedDomains` slice (line 230) | exact (append-only) |
| `frontend/web/src/components/player/EnglishPlayer.vue` | component (player surface) | request-response (UI control) | self — Phase 16 already added the source dropdown shape at lines 146-172 + `capitalizeProvider` at 477-485 | exact (replace `v-else` chip with multi-option panel; add 1 branch to capitalizeProvider) |
| `services/scraper/testdata/gogoanime/*.html` | test (goldens) | offline fixture | `services/scraper/testdata/animepahe/{search_naruto.json,release_4_p1.json,play_session_ep1.html,kwik_e_abc.html}` | exact |

**No analog (move/extract only):** the `services/scraper/internal/fuzzy/` package is a code-move from `animepahe/cache.go` (per `18-RESEARCH.md` §Architectural Responsibility Map "second consumer is the trigger for the shared package"). The functions are not new code; they relocate so both providers consume the same source.

**Unchanged files (no edits required) — listed for completeness, do NOT include in plans:**
- `frontend/web/src/components/player/ReportButton.vue` — already plumbs `triedChain[]` (Phase 16)
- `frontend/web/src/composables/useWatchPreferences.ts` — `preferredScraperProvider` accepts any string value (Phase 16)
- `frontend/web/src/locales/{en,ru,ja}.json` — all required keys (`sourceMultiTooltip`, `sourceUnhealthy`, `sourceSwitchFailed`, `sourceUnavailable`, `reportProvider`, `reportTried`) verified present in all three locales (`en.json:150-159`, `ru.json:150-159`, `ja.json:150-159`)
- `services/scraper/internal/service/orchestrator.go` — already iterates registered providers; metric `parser_fallback_total{from,to}` emitted on lines 206 + 235 (Phase 17 wiring)
- `services/scraper/internal/health/probe.go` + `health/stage.go` — auto-discover new provider via `RegisteredProviders()`
- `services/scraper/internal/handler/*` — no handler change (HTTP envelope is provider-agnostic)
- `frontend/web/changelog.json` — added at phase end by `/animeenigma-after-update` skill

---

## Pattern Assignments

### `services/scraper/internal/providers/gogoanime/client.go` (provider, request-response)

**Analog:** `services/scraper/internal/providers/animepahe/client.go`

**Imports pattern** (`animepahe/client.go:29-52`):
```go
import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"   // gogoanime: NOT needed — HTML scrape only
    "errors"
    "fmt"
    "io"
    "math"            // gogoanime: NOT needed — episode numbers are int
    "net/http"
    "net/url"
    "strings"
    "sync"
    "time"

    "github.com/PuerkitoBio/goquery"  // gogoanime: REQUIRED — HTML scrape

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/libs/metrics"
    "github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
    "github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)
```

**Constants pattern** (`animepahe/client.go:54-92`):
```go
const providerName = "animepahe"           // gogoanime: "gogoanime"
const fuzzyMatchThreshold = 0.85           // gogoanime: 0.85 (same per RESEARCH.md)
const maxEpisodePages = 50                 // gogoanime: NOT needed — single-page episode list
const episodesCacheTTL = 6 * time.Hour     // gogoanime: 6 * time.Hour (same)
const maxBodyAPI = 4 << 20                 // gogoanime: rename → maxBodyHTML 2<<20
const maxBodyHTML = 2 << 20                // gogoanime: 2 << 20

var stageNames = []string{
    health.StageSearch, health.StageEpisodes, health.StageServers, health.StageStream,
}

// Selector identifiers for parser_zero_match_total — bounded cardinality.
const (
    selectorEpisodeListItem = "episode_list_item"   // gogoanime: same key, scopes to <a> in /category page
    selectorServerLink      = "server_link"         // gogoanime: rename → animeMutiLinkItem
    selectorKwikPackedJS    = "kwik_packed_js"      // gogoanime: NOT used (provider doesn't extract; extractors do)
)
```

**Deps + New constructor pattern** (`animepahe/client.go:114-183`):
```go
type Deps struct {
    BaseURL string
    HTTP    *domain.BaseHTTPClient
    Embeds  *domain.Registry
    MalSync malSyncClient
    Cache   cache.Cache
    Log     *logger.Logger
}

func New(d Deps) (*Provider, error) {
    if d.HTTP == nil    { return nil, errors.New("animepahe: Deps.HTTP is required") }
    if d.Embeds == nil  { return nil, errors.New("animepahe: Deps.Embeds is required") }
    if d.MalSync == nil { return nil, errors.New("animepahe: Deps.MalSync is required") }
    if d.Cache == nil   { return nil, errors.New("animepahe: Deps.Cache is required") }
    if d.Log == nil     { d.Log = logger.Default() }
    base := d.BaseURL
    if base == "" { base = "https://animepahe.ru" }   // gogoanime: "https://anitaku.to"
    p := &Provider{
        baseURL: strings.TrimRight(base, "/"),
        http: d.HTTP, embeds: d.Embeds, malsync: d.MalSync, cache: d.Cache, log: d.Log,
        stages: make(map[string]domain.StageHealth, len(stageNames)),
    }
    for _, s := range stageNames {
        p.stages[s] = domain.StageHealth{Up: true}    // pre-seed canonical 4-stage map
    }
    return p, nil
}
```

**HealthCheck + markStage pattern** (`animepahe/client.go:186-214`):
```go
func (p *Provider) Name() string { return providerName }

func (p *Provider) markStage(stage string, err error) {
    p.stagesMu.Lock()
    defer p.stagesMu.Unlock()
    sh := p.stages[stage]
    if err == nil {
        sh.Up = true; sh.LastOK = time.Now(); sh.LastErr = ""
    } else {
        sh.Up = false; sh.LastErr = err.Error()
    }
    p.stages[stage] = sh
}

func (p *Provider) HealthCheck(ctx context.Context) domain.Health {
    p.stagesMu.Lock()
    defer p.stagesMu.Unlock()
    snap := make(map[string]domain.StageHealth, len(p.stages))
    for k, v := range p.stages { snap[k] = v }
    return domain.Health{Provider: providerName, Stages: snap}
}
```
**Apply verbatim.** Replace `providerName` constant only. Gogoanime does NOT need `getWithDDoSGuard` (verified — see RESEARCH.md "Mirror Viability").

**FindID pattern — INVERT order per RESEARCH.md** (`animepahe/client.go:244-315`):
The animepahe version tries malsync FIRST then fuzzy. Gogoanime must INVERT this because malsync has NO Gogoanime key as of 2026-05-12 (RESEARCH.md SCRAPER-9ANI-01 cell). Copy the structure but swap:

```go
// animepahe order (lines 244-256): malsync first, fuzzy fallback
if ref.ShikimoriID != "" {
    if id, ok, err := p.malsync.Lookup(ctx, ref.ShikimoriID, providerName); err == nil && ok {
        p.markStage(health.StageSearch, nil)
        return id, nil
    }
}
// then fuzzy search → /api?m=search

// gogoanime order: still call malsync first (forward-compat — 24h negative cache
// preserves the structural code path), but fuzzy /search.html is the PRIMARY
// path since the malsync hit will be a negative-cached miss until malsync ships
// a Gogoanime/Anitaku key.
```

Fuzzy-match block to copy verbatim (`animepahe/client.go:292-314`):
```go
// 3. Score each entry; pick the best ≥ threshold.
normTitle := normalizeTitle(ref.Title)   // gogoanime: fuzzy.NormalizeTitle (post-move)
best := struct {
    score   float64
    session string
}{}
for _, e := range sr.Data {
    score := jaroWinkler(normTitle, normalizeTitle(e.Title))   // gogoanime: fuzzy.JaroWinkler
    if score > best.score {
        best.score = score
        best.session = e.Session     // gogoanime: e.Slug (search-result slug)
    }
}
if best.score < fuzzyMatchThreshold || best.session == "" {
    err := domain.WrapNotFound(
        fmt.Errorf("best score %.4f", best.score),
        "animepahe: no fuzzy match for "+ref.Title,
    )
    p.markStage(health.StageSearch, err)
    return "", err
}
p.markStage(health.StageSearch, nil)
return best.session, nil
```

**Error-wrapping discipline** (`animepahe/client.go:262-264, 271-273, 277-279, 283-285, 288-290`):
```go
// Wrap external HTTP failures as ProviderDown:
err = domain.WrapProviderDown(err, "animepahe: search fetch")
// Wrap parse/decode failures as ExtractFailed:
err = domain.WrapExtractFailed(err, "animepahe: search decode")
// Wrap "no matches" as NotFound:
err := domain.WrapNotFound(nil, "animepahe: 0 search results for "+ref.Title)
// Always: p.markStage(stage, err); return zero, err
```
Apply verbatim. The orchestrator's `failoverDecision()` (orchestrator.go:114-128) distinguishes these three error families.

**ListServers pattern — adapt selector** (`animepahe/client.go:392-464`):
The shape is identical; only the goquery selector differs. AnimePahe scrapes `button[data-src]` for kwik URLs. Gogoanime scrapes `<ul class="anime_muti_link"> li a[data-video]` per RESEARCH.md Pattern 3:

```go
// animepahe scrape (line 432):
doc.Find("button[data-src]").Each(func(_ int, sel *goquery.Selection) {
    src, _ := sel.Attr("data-src")
    // ... URL validation: reject non-http(s) scheme up-front (WR-05)
    pu, perr := url.Parse(src)
    if perr != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
        return
    }
    host := strings.ToLower(pu.Hostname())
    if host != "kwik.cx" && !strings.HasSuffix(host, ".kwik.cx") &&
       host != "kwik.si" && !strings.HasSuffix(host, ".kwik.si") {
        return
    }
    // ... category derivation from data-audio attribute
    servers = append(servers, domain.Server{ID: src, Name: "kwik", Type: cat})
})

// gogoanime scrape (RESEARCH.md Pattern 3 lines 590-612):
doc.Find("ul.anime_muti_link li a[data-video]").Each(func(_ int, sel *goquery.Selection) {
    dv, _ := sel.Attr("data-video")
    if strings.HasPrefix(dv, "//") { dv = "https:" + dv }   // protocol-relative URLs
    u, perr := url.Parse(dv)
    if perr != nil || (u.Scheme != "http" && u.Scheme != "https") { return }
    host := strings.ToLower(u.Hostname())
    if hostMatchesAny(host, "myvidplay.com", "playmogo.com") { return }   // SKIP — Turnstile-gated
    // ... dedup + category (derived from slug -dub suffix, not data-audio)
    out = append(out, domain.Server{ID: dv, Name: name, Type: cat})
})
```

**GetStream pattern — copy verbatim** (`animepahe/client.go:486-525`):
```go
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
    _ = category   // informational only — server picked already includes category
    h := sha256.Sum256([]byte(serverID))
    cacheKey := fmt.Sprintf("stream:%s:%s:%s:%s", providerName, providerID, episodeID, hex.EncodeToString(h[:8]))

    var cached domain.Stream
    if err := p.cache.Get(ctx, cacheKey, &cached); err == nil {
        p.markStage(health.StageStream, nil)
        return &cached, nil
    }
    ext, err := p.embeds.Find(serverID)
    if err != nil {
        err = domain.WrapExtractFailed(err, "animepahe: no matching extractor for "+serverID)
        p.markStage(health.StageStream, err)
        return nil, err
    }
    headers := http.Header{"Referer": []string{p.baseURL}}   // gogoanime: same — "https://anitaku.to/"
    stream, err := ext.Extract(ctx, serverID, headers)
    if err != nil {
        p.markStage(health.StageStream, err)
        return nil, err
    }
    if stream == nil || len(stream.Sources) == 0 {
        err = domain.WrapExtractFailed(errors.New("empty stream"), "animepahe: extractor returned empty stream")
        p.markStage(health.StageStream, err)
        return nil, err
    }
    ttl := computeStreamTTL(stream.Sources[0].URL, time.Now())
    if ttl > 0 {
        _ = p.cache.Set(ctx, cacheKey, *stream, ttl)
    }
    p.markStage(health.StageStream, nil)
    return stream, nil
}

// Compile-time assertion: *Provider satisfies domain.Provider.
var _ domain.Provider = (*Provider)(nil)
```
**Apply verbatim** (rename `animepahe` → `gogoanime` in strings).

**Selector-drift sentinel pattern** (`animepahe/client.go:368-370`):
```go
// Emit parser_zero_match_total on the FIRST page when upstream returns zero items.
if page == 1 && len(rr.Data) == 0 {
    metrics.ParserZeroMatchTotal.WithLabelValues(providerName, selectorEpisodeListItem).Inc()
}
```
Apply to gogoanime's `ListEpisodes` and `ListServers` zero-match paths. Use constant `selector*` strings (cardinality-bounded per `animepahe/client.go:101-105` comment).

---

### `services/scraper/internal/providers/gogoanime/dto.go` (model, transform)

**Analog:** `services/scraper/internal/providers/animepahe/dto.go`

**Layout pattern** (`animepahe/dto.go:1-88`):
```go
// Package gogoanime implements the Gogoanime/Anitaku scraper provider (domain.Provider).
//
// SCRAPER-9ANI-01..06 (literal requirement IDs retained per CONTEXT.md S4).
// Wave-2 of Phase 18. Builds on:
//
//   - Phase 16 — animepahe provider (analog template).
//   - Phase 15 — embeds.Registry (extractor seam).
//   - Phase 17 — health package canonical stage constants.
//
// dto.go holds parser-derived structs and helpers (NOT JSON — Anitaku is
// HTML-only, but we mirror animepahe's "DTO module" naming for consistency).
package gogoanime

// searchResult is one <p class="name"><a href="/category/<slug>"> match
// from /search.html?keyword=<title>. Slug is the path-tail after /category/.
type searchResult struct {
    Slug  string
    Title string
}

// episodeRow is one <a href="/<slug>-episode-N"> match from /category/<slug>.
// Number is parsed from the trailing "-episode-<N>" path segment.
type episodeRow struct {
    Number  int
    URLSlug string  // e.g. "one-piece-episode-1" or "one-piece-dub-episode-1"
    Title   string  // optional; falls back to fmt.Sprintf("Episode %d", Number)
}

// serverRow is one <li><a data-video> match from /<slug>-episode-N.
// Name is the visible label (HD-1 / HD-2 / StreamHG / Earnvids).
type serverRow struct {
    Name     string  // visible label
    EmbedURL string  // raw data-video URL (https:-prefixed if protocol-relative)
}
```

**Re-export malsync DTOs verbatim from animepahe** (`animepahe/dto.go:71-87`): the `malSyncEntry` + `malSyncResponse` shapes are upstream-API contracts (api.malsync.moe) — IDENTICAL across providers. Either duplicate them in gogoanime/dto.go (recommended for package isolation) or extract into a shared `services/scraper/internal/malsync/` package (deferred; one consumer doesn't justify the refactor — but two does. The animepahe consumer survives; this phase adds the second).

---

### `services/scraper/internal/providers/gogoanime/malsync.go` (service, request-response)

**Analog:** `services/scraper/internal/providers/animepahe/malsync.go`

**Pattern — copy verbatim, change ONE constant:**

```go
// animepahe/malsync.go:17-21
const defaultMalSyncBaseURL = "https://api.malsync.moe"
const malSyncProviderSlug = "animepahe"      // gogoanime: "Gogoanime" (note CAPITALIZATION
                                              // matches malsync.moe's Sites key convention —
                                              // verify against the live API or assume capitalized
                                              // per malsync's standard naming)
const malSyncCacheTTL = 24 * time.Hour
const malSyncMissTTL  = 24 * time.Hour
```

**Lookup method — copy verbatim** (`animepahe/malsync.go:99-191`). The cache flow (positive hit → negative miss → upstream → cache) is identical. The provider-slug parameter at the call site (`p.malsync.Lookup(ctx, ref.ShikimoriID, providerName)`) drives the Sites[key] lookup; the helper code is provider-agnostic.

**Forward-compat behaviour:** the call WILL likely return `("", false, nil)` (confirmed miss, cached 24h) for every MAL ID until malsync.moe ships a Gogoanime key. The provider's `FindID` MUST handle this — the fuzzy `/search.html` path is the actual workhorse (per RESEARCH.md SCRAPER-9ANI-01 row).

---

### `services/scraper/internal/providers/gogoanime/cache.go` (utility, transform)

**Analog:** `services/scraper/internal/providers/animepahe/cache.go`

**`computeStreamTTL` — copy verbatim** (`animepahe/cache.go:14-62`):
```go
const streamTTLCap     = 5 * time.Minute
const streamTTLGuard   = 30 * time.Second
const streamTTLFallback = streamTTLCap

func computeStreamTTL(streamURL string, now time.Time) time.Duration {
    u, err := url.Parse(streamURL)
    if err != nil { return streamTTLFallback }
    expStr := u.Query().Get("expires")     // gogoanime extractors: parse `e=<seconds_to_live>`
                                            // instead (RESEARCH.md "Embed Extractor Catalog");
                                            // adapt query-param name accordingly
    if expStr == "" { return streamTTLFallback }
    expSec, err := strconv.ParseInt(expStr, 10, 64)
    if err != nil { return streamTTLFallback }
    exp := time.Unix(expSec, 0)
    headroom := exp.Sub(now) - streamTTLGuard
    if headroom <= 0 { return 0 }
    if headroom > streamTTLCap { return streamTTLCap }
    return headroom
}
```
**Important nuance:** StreamHG/Earnvids URLs use `&e=<seconds_to_live>` (delta, not absolute Unix ts). The TTL parser must subtract `streamTTLGuard` and clamp, but the conversion is `time.Duration(e) * time.Second`, NOT `time.Unix(e, 0)`. Vibeplayer has no expiry query param — falls back to 5 min.

**`normalizeTitle` + `jaroWinkler` — MOVE OUT to `services/scraper/internal/fuzzy/`** (`animepahe/cache.go:64-180`):
Per RESEARCH.md §Architectural Responsibility Map: "second consumer is the trigger for the shared package". Move both functions verbatim:

```go
// MOVE FROM animepahe/cache.go:73-93 TO services/scraper/internal/fuzzy/normalize.go
func NormalizeTitle(s string) string {
    s = strings.ToLower(s)
    for _, pat := range []struct{ in, out string }{
        {" 2nd season", " season 2"}, {" 3rd season", " season 3"},
        {" 4th season", " season 4"}, {" 5th season", " season 5"},
        {" part 2", " season 2"}, {" part 3", " season 3"},
        {" part 4", " season 4"}, {" part 5", " season 5"},
    } {
        s = strings.ReplaceAll(s, pat.in, pat.out)
    }
    s = strings.NewReplacer(":", " ", "·", " ", "—", " ", "–", " ", "-", " ").Replace(s)
    s = strings.Join(strings.Fields(s), " ")
    return s
}

// MOVE FROM animepahe/cache.go:101-180 TO services/scraper/internal/fuzzy/jarowinkler.go
func JaroWinkler(a, b string) float64 { /* identical body */ }
```

Update animepahe/client.go references: `normalizeTitle` → `fuzzy.NormalizeTitle`, `jaroWinkler` → `fuzzy.JaroWinkler`. Both providers consume the same source.

---

### `services/scraper/internal/embeds/vibeplayer.go` (service, request-response)

**Primary analog:** `services/scraper/internal/embeds/kwik.go` (structural — `EmbedExtractor` impl shape)
**Secondary analog:** `services/scraper/internal/embeds/megacloud.go` (simpler Matches/Name pattern without goja)

**Imports + constants pattern** (`embeds/kwik.go:22-58`):
```go
package embeds

import (
    "context"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "regexp"
    "strings"
    "time"

    // gogoanime vibeplayer: NO goja needed — regex-only extraction
    // (RESEARCH.md "Pattern 4 — VibePlayer extractor")

    "github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

var vibeplayerHosts = []string{"vibeplayer.site"}   // same shape as kwikHosts
const defaultVibeplayerHTTPTimeout = 15 * time.Second
const maxVibeplayerBody = 2 << 20
```

**Matches pattern — copy verbatim** (`embeds/kwik.go:275-293`):
```go
func (k *KwikExtractor) Matches(embedURL string) bool {
    u, err := url.Parse(embedURL)
    if err != nil { return false }
    if u.Scheme != "http" && u.Scheme != "https" {
        return false   // WR-05: reject non-http(s) up-front (SSRF prevention)
    }
    host := strings.ToLower(u.Hostname())
    if host == "" { return false }
    for _, known := range kwikHosts {
        if host == known || strings.HasSuffix(host, "."+known) {
            return true   // host equality OR strict subdomain
        }
    }
    return false
}
```
**Critical:** the strict-subdomain check (`strings.HasSuffix(host, "."+known)`) prevents `evilvibeplayer.site` from matching `vibeplayer.site`. Phase 16 ships an explicit SSRF regression test (`TestKwik_Matches_RejectsSubdomainImposters`) — Phase 18 must add equivalents.

**Extract pattern — adapt (regex-only, no goja)** (`embeds/kwik.go:308-424`):
The kwik version has 4 phases: HTTP fetch → comment-strip → packer-extract → goja-unpack → m3u8-regex.
Vibeplayer collapses to 2 phases: HTTP fetch → regex extract `const src = "..."`:

```go
// vibeplayer regex pattern (RESEARCH.md Pattern 4 lines 624-626):
var vibePlayerSrcRegex = regexp.MustCompile(`const\s+src\s*=\s*"(https://[^"]+\.m3u8)"`)
var vibePlayerSubRegex = regexp.MustCompile(`const\s+subtitle\s*=\s*"([^"]*)"`)

func (e *VibePlayerExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
    for k, vs := range headers { req.Header[k] = vs }   // forward caller-supplied Referer
    // ... HTTP fetch with the same defer-drain-and-close pattern from kwik.go:328-333
    resp, err := e.http.Do(req)
    if err != nil { return nil, domain.WrapProviderDown(err, "vibeplayer: fetch") }
    defer func() {
        _, _ = io.Copy(io.Discard, resp.Body)
        _ = resp.Body.Close()
    }()
    body, err := io.ReadAll(io.LimitReader(resp.Body, maxVibeplayerBody))
    if err != nil { return nil, domain.WrapProviderDown(err, "vibeplayer: read") }

    srcM := vibePlayerSrcRegex.FindSubmatch(body)
    if srcM == nil {
        // Selector drift sentinel — same pattern as animepahe/client.go:368
        metrics.ParserZeroMatchTotal.WithLabelValues("vibeplayer", "src_const_regex").Inc()
        return nil, domain.WrapExtractFailed(errors.New("no src= const"), "vibeplayer: src extract")
    }
    stream := &domain.Stream{
        Sources: []domain.Source{{URL: string(srcM[1]), Type: "hls"}},
        Headers: map[string]string{"Referer": "https://vibeplayer.site/"},
    }
    if subM := vibePlayerSubRegex.FindSubmatch(body); subM != nil && len(subM[1]) > 0 {
        stream.Tracks = []domain.Track{
            {File: string(subM[1]), Label: "English", Kind: "captions", Default: true},
        }
    }
    return stream, nil
}
```

**HTTP client pattern** (`embeds/kwik.go:243-262`):
```go
// Use http.Client with bounded Timeout — same as KwikExtractor's NewKwikExtractor.
type VibePlayerExtractor struct {
    http    *http.Client
    timeout time.Duration
}
func NewVibePlayerExtractor() *VibePlayerExtractor {
    return &VibePlayerExtractor{
        http: &http.Client{Timeout: defaultVibeplayerHTTPTimeout},
    }
}
```
**Note:** RESEARCH.md S5 ("Embed extractor SSRF guard reuse") says new extractors must use the hardened HTTP client pattern. KwikExtractor uses a plain `http.Client` with no SSRF guard at Extract time because it pre-validates the host in `Matches()`. Apply the same gate: never call `e.http.Do(req)` on a URL where `Matches(embedURL)` returned false.

**Compile-time interface assertion** (`embeds/kwik.go:467`):
```go
var _ domain.EmbedExtractor = (*VibePlayerExtractor)(nil)
```

---

### `services/scraper/internal/embeds/streamhg.go` + `earnvids.go` (service, request-response)

**Analog:** `services/scraper/internal/embeds/kwik.go` (exact — same Dean-Edwards packer)

**Reuse the existing packer infrastructure** (`embeds/kwik.go:63-220`):
- `htmlCommentRegex` — strip HTML comments before scanning
- `packerStartRegex` — locate `eval(function(p,a,c,k,e,d)` entry point
- `extractPacker` + `balanceUntil` — paren/brace balancer for the IIFE
- `runGoja` (the receiver method on `*KwikExtractor:434-464`) — goja runtime with watchdog goroutine (Pitfall 2 + 3)

**Recommended structure (per RESEARCH.md Pattern 5):** factor a shared base type. streamhg.go and earnvids.go differ ONLY by `Name()`, host allowlist, and Referer. A `packedExtractor` struct can carry the common impl; both files expose thin constructors:

```go
// services/scraper/internal/embeds/packed_common.go (NEW — helper)
//
// Shared Dean-Edwards packer extractor for embeds whose <script> body matches
// the kwik unpacker shape but emits the HLS URL under a different field name
// (`hls2` instead of `source`). One packer; two consumers (streamhg, earnvids).

type packedExtractor struct {
    name      string
    hosts     []string
    referer   string
    http      *http.Client
    timeout   time.Duration
}

// Same Matches() shape as KwikExtractor (host eq OR strict subdomain).
// Same Extract() body as KwikExtractor minus:
//   - the m3u8 regex changes from `(?:const|var|let|file\s*:)\s*(?:source\s*=...)`
//     to `"hls2"\s*:\s*"(https://[^"]+\.m3u8[^"]*)"` per RESEARCH.md Pattern 5
//   - TTL parsing uses the `&e=<seconds_to_live>` query param (not `expires=`)
```

**Per-extractor allowlists** (RESEARCH.md "Embed Extractor Catalog"):
```go
var streamhgHosts = []string{"otakuhg.site"}
var earnvidsHosts = []string{"otakuvid.online"}
// Both reuse the SAME unpacker logic; differ only in Name(), hosts, and Referer.
```

**Watchdog goroutine — Pitfall 3** (`embeds/kwik.go:434-464`):
```go
func (k *KwikExtractor) runGoja(ctx context.Context, expr string) (string, error) {
    vm := goja.New()    // Pitfall 2: FRESH runtime per call, never pooled

    done := make(chan struct{})
    defer close(done)

    go func() {
        // Pitfall 3: Interrupt() MUST come from a goroutine other than the one
        // running RunString — otherwise an infinite loop blocks cancellation.
        timer := time.NewTimer(k.timeout)
        defer timer.Stop()
        select {
        case <-timer.C:    vm.Interrupt("kwik: unpack timeout")
        case <-ctx.Done(): vm.Interrupt("kwik: ctx cancel")
        case <-done:       // normal completion
        }
    }()

    val, runErr := vm.RunString(expr)
    if runErr != nil { return "", runErr }
    if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
        return "", errors.New("goja runtime returned undefined/null")
    }
    return val.String(), nil
}
```
**Apply verbatim.** This is the safest piece of code in the embeds package — do not re-derive it.

---

### `services/scraper/cmd/scraper-api/main.go` (config, startup wiring)

**Analog:** self — `scraper-api/main.go` already wires `animepahe` (lines 75-109)

**Provider registration pattern** (`main.go:71-109`):
```go
// 1. Build per-host rate-limited HTTP client (line 75-81):
animePaheBaseHTTP := domain.NewBaseHTTPClient(log,
    domain.WithPerHostRPS("animepahe.ru", 1.0, 2),
    domain.WithPerHostRPS("animepahe.com", 1.0, 2),
    domain.WithPerHostRPS("kwik.cx", 1.0, 2),
    domain.WithPerHostRPS("kwik.si", 1.0, 2),
    domain.WithPerHostRPS("api.malsync.moe", 2.0, 4),
)

// 2. MalSync client (line 84):
malSyncClient := animepahe.NewMalSyncClient(redisCache)

// 3. Provider construction with eager validation (lines 90-100):
animePaheProvider, err := animepahe.New(animepahe.Deps{
    BaseURL: cfg.AnimePahe.BaseURL,
    HTTP:    animePaheBaseHTTP,
    Embeds:  registry,
    MalSync: malSyncClient,
    Cache:   redisCache,
    Log:     log,
})
if err != nil {
    log.Fatalw("failed to construct AnimePahe provider", "error", err)
}

// 4. Register provider in orchestrator (line 108):
orchestrator.Register(animePaheProvider)
log.Infow("registered provider", "name", animePaheProvider.Name())
```

**Phase 18 incremental additions** (insert between current line 108 and current line 110):
```go
// Gogoanime/Anitaku — second EN provider (Phase 18).
// New per-host RPS limits cover Anitaku + the 3 embed wrapper hosts.
gogoanimeBaseHTTP := domain.NewBaseHTTPClient(log,
    domain.WithPerHostRPS("anitaku.to", 1.0, 2),
    domain.WithPerHostRPS("vibeplayer.site", 1.0, 2),
    domain.WithPerHostRPS("otakuhg.site", 1.0, 2),
    domain.WithPerHostRPS("otakuvid.online", 1.0, 2),
)
gogoanimeMalsync := gogoanime.NewMalSyncClient(redisCache)
gogoanimeProvider, err := gogoanime.New(gogoanime.Deps{
    BaseURL: cfg.Gogoanime.BaseURL,
    HTTP:    gogoanimeBaseHTTP,
    Embeds:  registry,
    MalSync: gogoanimeMalsync,
    Cache:   redisCache,
    Log:     log,
})
if err != nil {
    log.Fatalw("failed to construct Gogoanime provider", "error", err)
}
orchestrator.Register(gogoanimeProvider)   // ORDER: after animepahe → declares failover order per CONTEXT D5
log.Infow("registered provider", "name", gogoanimeProvider.Name())
```

**Embed extractor registration pattern** (`main.go:43-53`):
```go
registry := domain.NewRegistry()
kwikExtractor := embeds.NewKwikExtractor()
registry.Register(kwikExtractor)              // line 47
log.Infow("registered embed extractor", "name", kwikExtractor.Name())

mcClient := embeds.NewMegacloudClient(cfg.MegacloudExtractor.URL, cfg.MegacloudExtractor.Timeout)
registry.Register(mcClient)
log.Infow("registered embed extractor", "name", mcClient.Name())
```

**Phase 18 incremental additions** (insert after line 53):
```go
vibeplayerExtractor := embeds.NewVibePlayerExtractor()
registry.Register(vibeplayerExtractor)
log.Infow("registered embed extractor", "name", vibeplayerExtractor.Name())

streamhgExtractor := embeds.NewStreamHGExtractor()
registry.Register(streamhgExtractor)
log.Infow("registered embed extractor", "name", streamhgExtractor.Name())

earnvidsExtractor := embeds.NewEarnvidsExtractor()
registry.Register(earnvidsExtractor)
log.Infow("registered embed extractor", "name", earnvidsExtractor.Name())
```

**No probe-spawn changes needed** — `main.go:127-161` snapshots `RegisteredProviders()` AFTER all `Register` calls, so the new provider is auto-discovered by the probe runner. Same for metric seeding loop (lines 128-140).

---

### `services/scraper/internal/config/config.go` (config, env)

**Analog:** self — `config.go:58-61` already has `AnimePaheConfig`

**Pattern — copy the `AnimePaheConfig` shape** (`config.go:54-103`):
```go
// AnimePaheConfig is the per-provider override surface for animepahe.Provider.
type AnimePaheConfig struct {
    BaseURL string
}

// Inside Config struct (line 18-23):
type Config struct {
    Server             ServerConfig
    MegacloudExtractor MegacloudExtractorConfig
    Redis              RedisConfig
    AnimePahe          AnimePaheConfig
    // gogoanime addition:
    Gogoanime          GogoanimeConfig
}

// Inside Load (line 85-87): env-driven default with URL validation
AnimePahe: AnimePaheConfig{
    BaseURL: getEnv("ANIMEPAHE_BASE_URL", "https://animepahe.ru"),
},
// gogoanime addition:
Gogoanime: GogoanimeConfig{
    BaseURL: getEnv("SCRAPER_GOGOANIME_BASE_URL", "https://anitaku.to"),
},

// URL validation pattern (line 99-107) — copy verbatim, change env var name + default
if u := cfg.AnimePahe.BaseURL; u != "" {
    parsed, err := url.Parse(u)
    if err != nil {
        return nil, fmt.Errorf("invalid ANIMEPAHE_BASE_URL %q: %w", u, err)
    }
    if parsed.Scheme == "" || parsed.Host == "" {
        return nil, fmt.Errorf("invalid ANIMEPAHE_BASE_URL %q: missing scheme or host", u)
    }
}
```

---

### `libs/videoutils/proxy.go` (utility, CORS allowlist)

**Analog:** self — `proxy.go:227-255` declares `HLSProxyAllowedDomains`

**Append-only pattern** (`proxy.go:230-255`):
```go
var HLSProxyAllowedDomains = []string{
    // Known streaming domains
    "megacloud.tv", "megacloud.blog", "megacloud.club",
    "rapid-cloud.co", "rapidcloud.live",
    "vidstream.pro", "vidstreamz.online",
    "mcloud.to", "mcloud2.to",
    "mgstatics.xyz",
    "netmagcdn.com",   // MegaCloud HLS CDN
    "owocdn.top",      // AnimePahe/Kwik CDN
    "uwucdn.top",      // AnimePahe/Kwik CDN (mirror)
    "kwik.cx",         // AnimePahe CDN
    "jimaku.cc",       // Japanese subtitle files
    "cdnlibs.org",     // AnimeLib video CDN
    "hentaicdn.org",   // AnimeLib video CDN (mirror)
    // Hanime video CDN
    "hanime.tv", "highwinds-cdn.com",
    "htv-*", "hydaelyn-*", "zodiark-*",
}
```

**Phase 18 incremental append** (insert before the closing `}` brace at line 255):
```go
    // Phase 18 — Anitaku/Gogoanime CDN entries (RESEARCH.md "Hostnames to Append").
    // Wildcard-suffix match is handled by isHLSDomainAllowed at proxy.go:578-579,
    // so rotating-subdomain CDNs (e.g. abc.premilkyway.com) match via
    // strings.HasSuffix(host, "."+allowed).
    "anitaku.to",        // optional — only needed if frontend ever proxies anitaku poster URLs
    "vibeplayer.site",   // vibeplayer same-origin HLS host
    "premilkyway.com",   // StreamHG primary CDN (rotating subdomain on this eTLD+1)
    "dramiyos-cdn.com",  // Earnvids primary CDN (rotating subdomain on this eTLD+1)
    "cdn.cimovix.store", // subtitle .vtt host (used by all 3 servers)
```

**Regression-lock invariant:** Phase 16's `kwik.cx` entry at line 245 (and the `pacha.kwik.cx` wildcard-suffix behaviour) MUST still match after the append. Do NOT reorder existing entries.

---

### `frontend/web/src/components/player/EnglishPlayer.vue` (component, UI)

**Analog:** self — Phase 16 already declares the dropdown shape at lines 146-172 and the helper at lines 477-485

**Source-dropdown trigger pattern — currently single-option** (`EnglishPlayer.vue:146-172`):
```vue
<!-- Source dropdown — Phase 16 single-option collapse; multi-option panel arrives in Phase 18 -->
<div class="mb-4">
  <h3 class="text-white/60 text-sm flex items-center gap-2 mb-2">
    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
        d="M12 21a9 9 0 100-18 9 9 0 000 18zm0 0c2.5-2.5 4-6 4-9s-1.5-6.5-4-9m0 18c-2.5-2.5-4-6-4-9s1.5-6.5 4-9m-9 9h18" />
    </svg>
    {{ $t('player.source') }}
  </h3>
  <div
    v-if="availableProviders.length === 1"
    class="bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm font-medium text-white cursor-default flex items-center justify-between"
    :title="$t('player.sourceSingleTooltip', { provider: capitalizeProvider(availableProviders[0]) })"
    data-testid="source-chip"
  >
    <span class="capitalize">{{ capitalizeProvider(availableProviders[0]) }}</span>
    <svg class="w-4 h-4 text-white/40" fill="currentColor" viewBox="0 0 24 24"><circle cx="12" cy="12" r="6" /></svg>
  </div>
  <!-- TODO Phase 18 — replace this fallback chip with the multi-option dropdown panel per UI-SPEC §ProviderSourceDropdown -->
  <div
    v-else
    class="bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white"
    data-testid="source-chip"
  >
    <span class="capitalize">{{ capitalizeProvider(selectedProvider || availableProviders[0]) }}</span>
  </div>
</div>
```

**Phase 18 surgery:** replace the `v-else` chip (lines 164-171) with the full multi-option panel per UI-SPEC §ProviderSourceDropdown. The single-option `v-if` chip stays untouched (it's the fallback when only one provider is available — e.g. health probe race or Anitaku temporarily skipped).

**Tailwind class tokens to use (per UI-SPEC §Color #10 + States table):**

| State | Class binding |
|-------|---------------|
| Trigger button (rest) | `bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm font-medium text-white min-h-[44px]` |
| Trigger button (open) | `accent-bg-muted accent-border` |
| Panel container | `mt-1 bg-white/5 border border-white/10 rounded-lg overflow-hidden` |
| Item — selected | `accent-bg-muted accent-text border accent-border` |
| Item — available (hover) | `text-white hover:bg-white/10` |
| Item — unhealthy | `opacity-40 cursor-not-allowed` + `text-xs text-pink-400` (offline) suffix |
| Tried-chain debug line | `text-xs text-white/40 mt-1` |

These utilities are already declared in `EnglishPlayer.vue:1603-1609` (`.accent-bg-muted`, `.accent-text`, `.accent-border`).

**`capitalizeProvider` helper modification** (`EnglishPlayer.vue:477-485`):
```ts
function capitalizeProvider(name: string | null | undefined): string {
  if (!name) return ''
  const slug = name.toLowerCase()
  if (slug === 'animepahe') return 'AnimePahe'
  if (slug === '9anime') return '9anime'              // REMOVE (planner discretion per UI-SPEC §EnglishPlayer.vue internal state delta)
  if (slug === 'animekai') return 'AnimeKai'
  // Phase 18 add:
  if (slug === 'gogoanime') return 'Anitaku'           // <- ONE NEW BRANCH
  return slug.charAt(0).toUpperCase() + slug.slice(1)
}
```

**Resume-on-switch contract — new function** (UI-SPEC §Component Contracts → switchProvider):
```ts
async function switchProvider(next: string) {
  const prior = selectedProvider.value
  const resumeAt =
    vjsPlayer?.currentTime() ??
    nativeVideoRef.value?.currentTime ??
    0

  selectedProvider.value = next
  setPreferredScraperProvider(next)

  try {
    await fetchServersAndStream()   // honors `prefer: next` via existing scraperApi calls
  } catch (err) {
    selectedProvider.value = prior
    setPreferredScraperProvider(prior)
    showToast(t('player.sourceSwitchFailed', { provider: capitalizeProvider(prior) }))
    return
  }

  await nextTick()
  if (vjsPlayer) vjsPlayer.currentTime(resumeAt)
  else if (nativeVideoRef.value) nativeVideoRef.value.currentTime = resumeAt
}
```

**Existing state refs (`EnglishPlayer.vue:541-543`) — UNCHANGED:**
```ts
const availableProviders = ref<string[]>(['animepahe'])  // overwritten by getHealth on mount
const selectedProvider = ref<string | null>(null)
const triedChain = ref<string[]>([])
```

**Existing scraperApi usage to preserve (`EnglishPlayer.vue:747, 792, 898-903`):**
```ts
// getEpisodes:
const response = await scraperApi.getEpisodes(props.animeId, selectedProvider.value || undefined)
// getServers:
const response = await scraperApi.getServers(props.animeId, episodeId, selectedProvider.value || undefined)
// getStream:
const response = await scraperApi.getStream(
  props.animeId, episodeId, serverId, category,
  selectedProvider.value || undefined,
)
```
**No API signature change needed.** The `prefer` 5th arg already accepts arbitrary strings; passing `'gogoanime'` is sufficient.

---

## Shared Patterns

### Pattern A — Stage health markStage discipline (apply to ALL provider methods)

**Source:** `services/scraper/internal/providers/animepahe/client.go:190-203`

```go
func (p *Provider) markStage(stage string, err error) {
    p.stagesMu.Lock()
    defer p.stagesMu.Unlock()
    sh := p.stages[stage]
    if err == nil {
        sh.Up = true; sh.LastOK = time.Now(); sh.LastErr = ""
    } else {
        sh.Up = false; sh.LastErr = err.Error()
    }
    p.stages[stage] = sh
}
```

**Apply to:** every entry point in `gogoanime/client.go` — `FindID`, `ListEpisodes`, `ListServers`, `GetStream`. Pattern: on success, `p.markStage(stage, nil)` BEFORE returning. On error, `err = domain.Wrap...(...)` then `p.markStage(stage, err)` BEFORE returning. Stage constants come from `services/scraper/internal/health/stage.go` — the 4 stages `StageSearch`, `StageEpisodes`, `StageServers`, `StageStream` (NOT `StageStreamSegment` — that's owned by the probe runner).

---

### Pattern B — Error wrapping families

**Source:** `services/scraper/internal/domain/errors.go` (existing) + `animepahe/client.go` call sites

| Wrap function | When to use |
|---------------|-------------|
| `domain.WrapProviderDown(err, msg)` | HTTP transport failures, non-2xx upstream status, read-body failures, network timeouts |
| `domain.WrapExtractFailed(err, msg)` | JSON decode failures, goquery parse failures, regex no-match, goja runtime errors, empty stream returned by extractor |
| `domain.WrapNotFound(err, msg)` | Upstream returned 404, search returned 0 results, fuzzy score < 0.85 |

**Apply to:** every error-return path in the new provider and 3 new extractors. The orchestrator's `failoverDecision()` (`orchestrator.go:114-128`) routes these as follows:
- ProviderDown → retryable, `parser_fallback_total` increments
- ExtractFailed → retryable, `parser_fallback_total` increments
- NotFound → retryable, `parser_fallback_total` increments
- `context.Canceled`/`DeadlineExceeded` → terminal, propagated as-is

---

### Pattern C — Body cap discipline (DoS guard)

**Source:** `embeds/kwik.go:55-58`, `animepahe/client.go:72-76`

```go
const maxKwikBody = 2 << 20      // 2 MiB
const maxBodyAPI  = 4 << 20      // 4 MiB
const maxBodyHTML = 2 << 20      // 2 MiB

// Usage:
body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyHTML))
```

**Apply to:** every HTTP body read in the new provider + 3 new extractors. Anitaku pages are ~85 KB; embed wrapper pages are <100 KB. 2 MiB cap is correct.

---

### Pattern D — defer-drain-and-close (keep-alive hygiene)

**Source:** `animepahe/client.go:266-269`, `embeds/kwik.go:328-333`

```go
defer func() {
    _, _ = io.Copy(io.Discard, resp.Body)
    _ = resp.Body.Close()
}()
```

**Apply to:** every HTTP response in the new code. Required so the keep-alive connection can be reused by `retryablehttp` (otherwise the pool fills with half-read connections).

---

### Pattern E — SSRF prevention via host allowlist

**Source:** `embeds/kwik.go:275-293`

```go
func (k *KwikExtractor) Matches(embedURL string) bool {
    u, err := url.Parse(embedURL)
    if err != nil { return false }
    if u.Scheme != "http" && u.Scheme != "https" {
        return false   // WR-05: reject non-http(s) schemes up-front
    }
    host := strings.ToLower(u.Hostname())
    if host == "" { return false }
    for _, known := range kwikHosts {
        if host == known || strings.HasSuffix(host, "."+known) {
            return true
        }
    }
    return false
}
```

**Apply to:** all 3 new extractors (`Matches()` method) AND `gogoanime/client.go::ListServers`'s data-video host filter (RESEARCH.md Pattern 3 line 600-604).

**Critical:** the strict-subdomain check (`HasSuffix(host, "."+known)`) prevents impostor domains. Phase 16 ships explicit regression tests (`TestKwik_Matches_RejectsSubdomainImposters` and the implicit equivalents for animepahe's host filter). Phase 18 must add:
- `TestVibePlayer_Matches_RejectsSubdomainImposters`
- `TestStreamHG_Matches_RejectsSubdomainImposters`
- `TestEarnvids_Matches_RejectsSubdomainImposters`
- `TestGogoanime_ListServers_SkipsTurnstileHosts` (verifies `myvidplay.com` + `playmogo.com` are filtered)

---

### Pattern F — Selector-drift sentinel (observability)

**Source:** `animepahe/client.go:101-105` + `client.go:368-370`

```go
const (
    selectorEpisodeListItem = "episode_list_item"   // ADD constants here, NEVER inline strings
    selectorServerLink      = "server_link"
)

// At zero-match call site:
if page == 1 && len(rr.Data) == 0 {
    metrics.ParserZeroMatchTotal.WithLabelValues(providerName, selectorEpisodeListItem).Inc()
}
```

**Apply to:** every selector-drift call site in `gogoanime/client.go` (zero category links, zero episode rows, zero anime_muti_link items, zero data-video attrs) AND every regex-no-match path in the 3 new extractors. Use **named constants** (NOT string literals) for the `selector` label — cardinality is per-distinct-value, an inline string would explode the metric on a typo.

---

### Pattern G — Cache key naming (Redis namespacing)

**Source:** `animepahe/client.go:321, 490` + `animepahe/malsync.go:103-104`

```
malsync:{mal_id}:{provider}                     // animepahe/malsync.go:103
malsync:{mal_id}:{provider}:miss                // animepahe/malsync.go:104
episodes:{provider}:{providerID}                // animepahe/client.go:321
stream:{provider}:{providerID}:{episodeID}:{hash(serverID)[:8]}  // animepahe/client.go:490
```

**Apply to:** gogoanime cache keys — substitute `{provider}` = `gogoanime`. Per CONTEXT.md D9 the cache prefixes are `malsync:gogoanime:*`, `episodes:gogoanime:*`, `stream:gogoanime:*`.

---

### Pattern H — Compile-time interface assertion

**Source:** `animepahe/client.go:528`, `embeds/kwik.go:467`

```go
var _ domain.Provider       = (*Provider)(nil)         // in gogoanime/client.go
var _ domain.EmbedExtractor = (*VibePlayerExtractor)(nil)   // in embeds/vibeplayer.go
var _ domain.EmbedExtractor = (*StreamHGExtractor)(nil)     // in embeds/streamhg.go
var _ domain.EmbedExtractor = (*EarnvidsExtractor)(nil)     // in embeds/earnvids.go
```

**Apply to:** end of every new client/extractor file. Catches signature drift at compile time.

---

### Pattern I — Test pattern (offline goldens)

**Source:** `services/scraper/internal/providers/animepahe/dto_test.go:15-43` + `services/scraper/testdata/animepahe/`

```go
func TestEpDTO_Unmarshal_GoldenFixture(t *testing.T) {
    t.Parallel()
    path := filepath.Join("..", "..", "..", "testdata", "animepahe", "release_4_p1.json")
    data, err := os.ReadFile(path)
    if err != nil { t.Fatalf("read golden: %v", err) }
    var rr releaseResponse
    if err := json.Unmarshal(data, &rr); err != nil { t.Fatalf("decode: %v", err) }
    // ... assert structural invariants
}
```

**Apply to:** all new tests. Goldens live at `services/scraper/testdata/gogoanime/`:
- `search_attack_on_titan.html` (search result page)
- `category_one_piece.html` (anime detail page — sub variant)
- `category_one_piece_dub.html` (anime detail page — dub variant; verifies merge behaviour)
- `one_piece_episode_1.html` (episode page with `<ul class="anime_muti_link">`)
- `vibeplayer_embed.html` (vibeplayer wrapper with `const src=`)
- `streamhg_packed.html` (Dean-Edwards packer body)
- `earnvids_packed.html` (Dean-Edwards packer body, different host)
- `malsync_no_gogo.json` (sample malsync response WITHOUT a Gogoanime key — verifies negative-cache path)

**No live network in CI** (per CONTEXT.md D8). The probe runner catches upstream death in production.

---

### Pattern J — Provider failover orchestration (zero code change required)

**Source:** `services/scraper/internal/service/orchestrator.go:84-108, 178-248`

The orchestrator iterates `o.providers` in registration order, gates each via `cache.IsHealthy()` (`orchestrator.go:201`), and emits `parser_fallback_total{from, to}` on skip OR retryable failure (`orchestrator.go:206 + 235`). **No code change in orchestrator.go.**

Verification post-deploy (RESEARCH.md SCRAPER-9ANI-06):
```bash
# Force AnimePahe unhealthy
curl http://localhost:8088/scraper/health/admin   # confirm gauge state
# Issue real stream request via /api/anime/{id}/scraper/stream
curl http://localhost:8088/metrics | grep parser_fallback_total
# Expect: parser_fallback_total{from="animepahe",to="gogoanime"} > 0
```

---

## No Analog Found

| File | Role | Data Flow | Reason / Source |
|------|------|-----------|-----------------|
| `services/scraper/internal/fuzzy/jarowinkler.go` | utility | transform | MOVE source: `animepahe/cache.go:101-180` (verbatim relocation; no shape change) |
| `services/scraper/internal/fuzzy/normalize.go` | utility | transform | MOVE source: `animepahe/cache.go:64-93` (verbatim relocation) |
| `services/scraper/internal/fuzzy/fuzzy_test.go` | test | unit | New test file — re-derive coverage from `animepahe/client_test.go` fuzzy-fallback test cases |
| `services/scraper/Makefile capture-goldens-gogoanime target` | tooling | script | New Makefile target (planner discretion per CONTEXT.md D8 + RESEARCH.md S2). Pattern: mirrors `capture-goldens-animepahe` if one exists; if not, this is a new file with a documented one-shot fetch script. |

---

## Metadata

**Analog search scope:**
- `services/scraper/internal/providers/animepahe/` (5 source files + 5 tests + 4 goldens)
- `services/scraper/internal/embeds/` (kwik.go, megacloud.go + tests)
- `services/scraper/internal/health/` (stage.go, probe.go header, cache.go)
- `services/scraper/internal/service/orchestrator.go`
- `services/scraper/internal/domain/{provider,embed,errors,httpclient}.go`
- `services/scraper/cmd/scraper-api/main.go`
- `services/scraper/internal/config/config.go`
- `libs/videoutils/proxy.go` (HLSProxyAllowedDomains)
- `libs/metrics/parser.go` + `libs/metrics/provider.go` (metric definitions)
- `frontend/web/src/components/player/EnglishPlayer.vue` (1685 LOC)
- `frontend/web/src/composables/useWatchPreferences.ts`
- `frontend/web/src/locales/{en,ru,ja}.json`

**Files scanned:** 24 analog source files
**Pattern extraction date:** 2026-05-12

**Key cross-cutting observations:**

1. **Every backend pattern in this phase has a one-to-one analog in Phase 16's animepahe package.** The implementation is mostly mechanical translation: HTML scraping selectors differ; orchestration / cache / health / metrics / error wrapping are identical contracts.

2. **The frontend has near-zero code paths to add.** Phase 16 already declared the multi-option dropdown shape; Phase 18 ACTIVATES it. The dormant `v-else` branch (lines 164-171) replaces with the panel. Every required locale key already exists in all three languages.

3. **The orchestrator + probe runner + health cache require ZERO source changes.** They iterate `RegisteredProviders()` and auto-discover the new provider on the next probe tick.

4. **The shared `fuzzy/` package is the only structural refactor in this phase.** Move source code from `animepahe/cache.go` to `scraper/internal/fuzzy/`; both providers consume the same source.

5. **3 new embed extractors share substantial code with kwik.go.** Recommendation per RESEARCH.md Pattern 5: factor `packedExtractor` base type for StreamHG + Earnvids (identical Dean-Edwards packer; differ only in Name/hosts/Referer); VibePlayer is regex-only (no goja) and stays separate.
