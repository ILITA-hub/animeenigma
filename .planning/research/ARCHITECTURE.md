# Architecture Research — v3.0 Universal Anime Scraper

**Domain:** Go monorepo integration — replacing dead HiAnime/Consumet provider paths with a self-hosted multi-provider scraping subsystem behind the existing EN player surface.
**Researched:** 2026-05-11
**Confidence:** HIGH (every claim anchored to a verbatim file/line in the current tree)

---

## 1. Service Boundary — recommendation

**Recommendation: extend `services/catalog/` with a new `internal/parser/scraper/` family — do NOT spawn `services/scraper/`.**

The catalog service is already the single owner of every external provider parser (Kodik, AnimeLib, HiAnime, Consumet, Hanime, Aniboom, Shikimori, Jikan, Jimaku — see `services/catalog/internal/service/catalog.go:18-27` imports). It also owns the per-anime Shikimori upsert path that every parser relies on for "given my shikimori_id, find the provider-specific ID" — adding a new service would force us to either re-cross that bridge or pull the catalog service in as a dependency anyway. The HiAnime ↔ Consumet pair we're replacing already shares the catalog's anime DB lookup, the `findHiAnimeID` inflight de-dupe (`catalog.go:30-35`), and Redis cache keys.

### Tradeoffs explicitly considered

| Dimension | NEW `services/scraper/` | EXTEND `services/catalog/` ← recommended |
|---|---|---|
| Failure isolation | Scraper crashes can't take down catalog | Catalog already tolerates per-parser errors today (every call has its own `defer metrics.ObserveParser` and `errors.NotFound` fallback) |
| Deployment surface | +1 container, +1 gateway route, +1 Dockerfile, +1 go.mod, +1 healthcheck, +1 Prometheus target | Zero new containers; reuse existing redeploy-catalog cycle |
| Code surface | New `cmd/scraper-api/main.go`, repo, config, transport, handler — ~700 LOC of boilerplate before any business code | A single new `internal/parser/scraper/` subtree |
| Database coupling | Scraper would need read-only Postgres access to look up `shikimori_id → name → external_id` (the catalog already has `animeRepo.GetByID`) OR a synchronous catalog HTTP call per request | In-process function call against the existing repo |
| Frontend impact | `/api/scraper/*` requires gateway route addition (services/gateway changes) | Route stays under `/api/anime/{id}/...` — gateway already routes `/api/anime/*` → catalog |
| Cutover risk | Two services to deploy in lockstep | Single service redeploy |
| Future "scraper-only" needs (admin scrape-everything cron, debug tools) | Cleaner home | Still possible — add admin handler on catalog |

**Why this is the right call for v3.0 specifically:** the cutover already requires touching catalog (delete hianime/ and consumet/ parsers, update wiring at `catalog.go:44-45`, `catalog.go:125-126`). Adding a new service on top doubles the deploy-coordination surface during the most dangerous moment of the milestone (the EN-player blackout window). Once v3.0 ships and we have on-call experience with the scraper for ≥ 1 month, extracting it into its own service is mechanical and a single weekend's work. **Do not over-engineer the boundary up front.**

NEW directory layout (additions only):

```
services/catalog/internal/parser/scraper/      # NEW
├── provider.go            # Provider interface + DTOs (the contract)
├── orchestrator.go        # Multi-provider fan-out, failover, caching
├── orchestrator_test.go   # Table-driven fan-out coverage
├── megacloud_client.go    # HTTP client for docker/megacloud-extractor
├── animekai/              # First provider impl (recommended order, see §9)
│   ├── client.go
│   └── client_test.go
├── animepahe/             # Second provider impl
│   ├── client.go
│   └── client_test.go
└── anitaku/               # Third provider impl (gogoanime mirror)
    ├── client.go
    └── client_test.go
```

---

## 2. Provider interface

The interface is the single integration seam. Every provider must satisfy it. The DTO it returns is byte-identical in shape to the current `domain.HiAnimeStream` / `domain.HiAnimeEpisode` / `domain.HiAnimeServer` types that the frontend already parses, so the orchestrator can emit those exact types without translation glue at the handler layer.

**File: `services/catalog/internal/parser/scraper/provider.go`** (NEW)

```go
package scraper

import (
    "context"
    "errors"
    "time"
)

// Category narrows the audio track requested from a provider.
// Mirrors the existing HiAnime `category` query param (`sub`/`dub`/`raw`)
// that ConsumetPlayer.vue and HiAnimePlayer.vue already pass through.
type Category string

const (
    CategorySub Category = "sub"
    CategoryDub Category = "dub"
    CategoryRaw Category = "raw"
)

// AnimeRef identifies an anime in our domain (NOT the provider's domain).
// Providers resolve their own internal IDs from these fields.
type AnimeRef struct {
    AnimeID     string // catalog domain ID (UUID from `animes.id`)
    ShikimoriID string // shikimori_id == MAL ID
    AniListID   string // resolved via libs/idmapping when needed
    Title       string // English/romaji title for name-based search fallback
    Year        int    // disambiguator for ambiguous title matches
}

// Episode is the unified episode descriptor returned to the handler layer.
// Shape matches domain.HiAnimeEpisode so existing players need no schema change.
type Episode struct {
    ID       string `json:"id"`        // provider-scoped episode key (opaque to caller)
    Number   int    `json:"number"`
    Title    string `json:"title"`
    IsFiller bool   `json:"is_filler"`
}

// Server is a streaming server option for an episode. Many providers
// expose exactly one "default" server — that's fine, return a single
// element with Name == "default".
type Server struct {
    ID   string `json:"id"`
    Name string `json:"name"`
    Type string `json:"type"` // "sub" | "dub" | "raw"
}

// Stream is the playable result. URL is a proxied HLS m3u8 (or rarely MP4).
// Subtitles, Intro, Outro mirror the existing HiAnimeStream DTO exactly.
type Stream struct {
    URL        string            `json:"url"`
    Type       string            `json:"type"` // "hls" | "mp4"
    Subtitles  []Subtitle        `json:"subtitles"`
    Headers    map[string]string `json:"headers,omitempty"` // Referer/Origin for the HLS proxy
    Intro      *TimeRange        `json:"intro,omitempty"`
    Outro      *TimeRange        `json:"outro,omitempty"`
    ProviderID string            `json:"provider_id"` // "animekai" — exposes the source to the frontend for UX
}

type Subtitle struct {
    URL     string `json:"url"`
    Lang    string `json:"lang"`
    Label   string `json:"label"`
    Default bool   `json:"default"`
}

type TimeRange struct {
    Start int `json:"start"`
    End   int `json:"end"`
}

// Health is reported by the per-provider `/healthz` ping. Used by
// orchestrator.go to skip dead providers before fanning out.
type Health struct {
    Healthy   bool
    LatencyMS int64
    CheckedAt time.Time
    Reason    string // populated when Healthy=false
}

// Provider is the contract every scraper backend implements.
//
// Lifecycle invariant: every call accepts context for cancellation/timeout.
// Errors must wrap one of the sentinel values below so the orchestrator
// can decide failover vs. abort.
type Provider interface {
    // Name returns a stable identifier ("animekai", "animepahe", "anitaku").
    // Used as the `provider` label in libs/metrics ObserveParser calls.
    Name() string

    // FindID resolves the catalog AnimeRef to the provider's internal anime ID.
    // Implementations may search by title + year and disambiguate via
    // AniList/MAL ID when the upstream exposes those (animekai does).
    FindID(ctx context.Context, ref AnimeRef) (providerAnimeID string, err error)

    // ListEpisodes returns every episode for a given provider anime ID.
    ListEpisodes(ctx context.Context, providerAnimeID string) ([]Episode, error)

    // ListServers returns the server options for an episode.
    // Providers with a single fixed server return one Server with Name="default".
    ListServers(ctx context.Context, episodeID string, category Category) ([]Server, error)

    // GetStream resolves the actual playable HLS/MP4 source for an episode+server.
    // This is the call that may invoke the megacloud-extractor helper (animekai)
    // or directly resolve a Kwik link (animepahe).
    GetStream(ctx context.Context, episodeID, serverID string, category Category) (*Stream, error)

    // HealthCheck pings the provider's website (HEAD on root). Cheap and idempotent.
    // Orchestrator calls this at most once per minute per provider.
    HealthCheck(ctx context.Context) Health
}

// Sentinel errors. Providers MUST wrap with errors.Join/fmt.Errorf("%w", ...).
var (
    // ErrNotFound — provider could not resolve the AnimeRef or episode.
    // Orchestrator continues to the next provider.
    ErrNotFound = errors.New("scraper: provider has no record")

    // ErrProviderDown — provider site is unreachable / Cloudflare-blocked.
    // Orchestrator continues to the next provider AND marks the provider
    // unhealthy for the next 5 minutes.
    ErrProviderDown = errors.New("scraper: provider is unreachable")

    // ErrExtractFailed — found the embed page but couldn't decrypt sources
    // (e.g. megacloud-extractor returned an error). Orchestrator continues
    // to the next provider; this is the most common transient failure mode.
    ErrExtractFailed = errors.New("scraper: stream extraction failed")
)
```

### Why each method exists (mapping back to today's HiAnime API surface)

| Provider method | Replaces what HiAnime client does today |
|---|---|
| `FindID(ctx, ref)` | `findHiAnimeID` at `services/catalog/internal/service/catalog.go:1643` (called via name-search) |
| `ListEpisodes(ctx, id)` | `hianime.Client.GetEpisodes` at `services/catalog/internal/parser/hianime/client.go:131` |
| `ListServers(ctx, ep, cat)` | `hianime.Client.GetServers` at `services/catalog/internal/parser/hianime/client.go:169` |
| `GetStream(ctx, ep, srv, cat)` | `hianime.Client.GetStream` at `services/catalog/internal/parser/hianime/client.go:230` |
| `HealthCheck(ctx)` | NEW — does not exist today, hence the silent dead-aniwatch fiasco |

`HealthCheck` is the single most important new method: it's the surface that makes provider-death observable at request time rather than via user reports. Wire it into the existing `libs/metrics/parser.go` `ObserveParser` pattern so Grafana shows when AnimeKai goes Cloudflare-blocked.

---

## 3. Multi-provider orchestrator

**Failover lives in the service layer, NOT the handler.** The handler stays thin (mirrors `GetHiAnimeStream` at `services/catalog/internal/handler/catalog.go:726-756` — pure HTTP-to-service translation, no business logic). The orchestrator is the seam where the "try AnimeKai first, fall back to AnimePahe, fall back to Anitaku" policy lives.

**File: `services/catalog/internal/parser/scraper/orchestrator.go`** (NEW)

```go
package scraper

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/libs/metrics"
)

// Orchestrator fans a request out across providers in declared order,
// returns the first success, caches results, and honors provider health.
type Orchestrator struct {
    providers   []Provider           // declared order = preference order
    cache       *cache.RedisCache
    log         *logger.Logger
    healthMu    sync.RWMutex
    healthCache map[string]Health    // provider name -> last Health
    healthTTL   time.Duration        // 60s — short enough to recover quickly
}

func NewOrchestrator(cache *cache.RedisCache, log *logger.Logger, providers ...Provider) *Orchestrator {
    return &Orchestrator{
        providers:   providers,
        cache:       cache,
        log:         log,
        healthCache: make(map[string]Health),
        healthTTL:   60 * time.Second,
    }
}

// GetStream is the hot path. Tries providers in order until one returns
// a non-empty Stream URL. Caches the winning result for 30 minutes
// (same TTL the HiAnime path uses today — see catalog.go:1794).
func (o *Orchestrator) GetStream(ctx context.Context, ref AnimeRef, epNum int, category Category) (*Stream, error) {
    cacheKey := fmt.Sprintf("scraper:stream:%s:%d:%s", ref.AnimeID, epNum, category)
    var cached Stream
    if err := o.cache.Get(ctx, cacheKey, &cached); err == nil {
        return &cached, nil
    }

    var lastErr error
    for _, p := range o.providers {
        // Skip recently-failed providers
        if h := o.getCachedHealth(ctx, p); !h.Healthy {
            metrics.ParserFallbackTotal.WithLabelValues(p.Name(), "skipped_unhealthy").Inc()
            continue
        }

        start := time.Now()
        stream, err := o.tryProvider(ctx, p, ref, epNum, category)
        metrics.ObserveParser(p.Name(), "get_stream", start, &err)

        if err == nil && stream != nil && stream.URL != "" {
            stream.ProviderID = p.Name()
            _ = o.cache.Set(ctx, cacheKey, stream, 30*time.Minute)
            return stream, nil
        }

        lastErr = err
        // Mark unhealthy on ErrProviderDown so we skip it for the next 60s
        if errors.Is(err, ErrProviderDown) {
            o.markUnhealthy(p, err.Error())
        }
        metrics.ParserFallbackTotal.WithLabelValues(p.Name(), "tried_next").Inc()
    }

    if lastErr != nil {
        return nil, fmt.Errorf("all providers failed: %w", lastErr)
    }
    return nil, ErrNotFound
}

// tryProvider runs FindID -> ListEpisodes -> ListServers -> GetStream for
// one provider. Each sub-call is cached by `scraper:<provider>:<op>:<key>`
// using the existing libs/cache TTL conventions (1h episodes, 30m stream).
func (o *Orchestrator) tryProvider(
    ctx context.Context, p Provider, ref AnimeRef, epNum int, cat Category,
) (*Stream, error) { /* ... */ }

// ListEpisodes is the equivalent fan-out for the episode list. Caches the
// winning provider's list per anime so episode-grid renders are stable.
func (o *Orchestrator) ListEpisodes(ctx context.Context, ref AnimeRef) ([]Episode, string /*providerID*/, error) { /* ... */ }
```

### Why the service layer, not the handler

1. **Handler is HTTP shape only** — `GetHiAnimeStream` at `catalog.go:726` shows the existing pattern: parse query params, call service method, write JSON. Failover policy doesn't belong there.
2. **Handler is currently 950 lines of provider-specific endpoints** — adding orchestration here makes that worse, not better.
3. **Service layer is where caching lives today** — `s.cache.Get`/`s.cache.Set` is invoked from `GetHiAnimeStream` at `catalog.go:1731-1735, 1794`. The orchestrator is the natural place to consolidate the per-provider cache reads.

### Why NOT the per-provider client

Failover at the client layer (the current Consumet pattern — see `services/catalog/internal/parser/consumet/client.go:55-65` `providerList()`) **couples each provider to the existence of every other provider**. The new design keeps providers ignorant of one another; the orchestrator is the only thing that knows about ordering.

---

## 4. Integration with existing libs

| Lib | How v3.0 uses it | Where it's wired |
|---|---|---|
| **`libs/idmapping`** | `Provider.FindID` may resolve `ShikimoriID → AniListID` when the provider indexes by AniList (animekai does). Reuse the existing client already constructed at `services/catalog/internal/service/catalog.go:131` (`idMappingClient: idmapping.NewClient()`) — pass it into `scraper.NewOrchestrator` as a dependency. **No new client.** |
| **`libs/cache`** | Orchestrator + each provider use `*cache.RedisCache` (already injected into `CatalogService` at `catalog.go:51`). New TTL constants OR reuse existing: `cache.TTLEpisodeList` (1h, `ttl.go:21`) for episode lists, ad-hoc `30 * time.Minute` for stream URLs (matches today's pattern). New key prefix `scraper:` — already free per the `PrefixVideo` taxonomy in `ttl.go:36-50`. |
| **`libs/metrics`** | Every provider call already-pattern: `defer metrics.ObserveParser("animekai", "get_stream", start, &err)` (`libs/metrics/parser.go:43-51`). Failover events use `metrics.ParserFallbackTotal.WithLabelValues("from", "to").Inc()` (`parser.go:31-37`). **No new metrics needed**, but DO add per-provider `parser_health_status{provider=...}` gauge — small addition to `parser.go`. |
| **`libs/videoutils`** | `libs/videoutils/proxy.go:230-255` `HLSProxyAllowedDomains` list is **the** unlock for new provider CDNs. AnimePahe CDNs (`kwik.cx`, `owocdn.top`, `uwucdn.top`) are already in the list (lines 244-246). For AnimeKai add `megacloud.tv`, `netmagcdn.com` (already present, lines 232, 242), and any new mirror domains discovered in scraping. For Anitaku/Gogoanime expect to add `*.gogocdn.*` and `*.embtaku.*`. This is a **single-file allowlist append**, no logic change. |
| **`libs/logger`** | Inject `*logger.Logger` into orchestrator and each provider — follow `CatalogService.log` at `catalog.go:52, 133`. Use structured logging: `log.Warnw("scraper provider failed", "provider", p.Name(), "operation", "get_stream", "error", err)`. |
| **`libs/errors`** | `errors.NotFound(...)` and `errors.Wrap(...)` used at handler boundary today (`catalog.go:1701, 1746`). Continue to use at the service-layer entry into the orchestrator, not inside the orchestrator itself (use sentinel errors there). |

**No new libs are required.** This is enforced by the rule in CLAUDE.md: "Don't add complex abstractions for simple operations."

### Initialization (dependency injection)

Modify `CatalogServiceOptions` at `services/catalog/internal/service/catalog.go:58-66` (MODIFIED) to add:

```go
type CatalogServiceOptions struct {
    AniwatchAPIURL          string // KEPT during cutover, removed in Phase D
    ConsumetAPIURL          string // KEPT during cutover, removed in Phase D
    ConsumetProvider        string
    JimakuAPIKey            string
    AnimeLibToken           string
    HanimeEmail             string
    HanimePassword          string
    MegacloudExtractorURL   string // NEW — defaults to "http://megacloud-extractor:3200"
    ScraperProviderOrder    []string // NEW — e.g. ["animekai","animepahe","anitaku"]; empty = default order
}
```

Modify `NewCatalogService` at `services/catalog/internal/service/catalog.go:68-135` (MODIFIED): add field `scraper *scraper.Orchestrator` to `CatalogService` struct (line 37-55), construct it at the bottom of `NewCatalogService` with the configured providers.

Modify `services/catalog/cmd/catalog-api/main.go` (MODIFIED — pass new env var through, mirroring the existing `ANIWATCH_API_URL` plumbing at `docker/docker-compose.yml:351`).

---

## 5. Cutover plan — never break EN players mid-deploy

**Constraint:** at every commit boundary, at least one EN-language player must work. The EN-player tab is currently broken anyway, so the floor is "no worse than today" — but the ceiling is "remove dead code AFTER its replacement is proven."

### Cutover sequence (commit boundary discipline)

| Commit | What ships | EN players working after this commit |
|---|---|---|
| **C1** Interface + skeleton orchestrator + first provider (AnimeKai) WITHOUT removing anything | New endpoints `/api/anime/{id}/scraper/episodes|servers|stream|search` registered alongside existing `/hianime` and `/consumet` (router.go:82-88) | Old: still dead. New: AnimeKai. **Net: improvement.** |
| **C2** Wire frontend's HiAnimePlayer.vue to call `scraperApi.*` (NOT `hianimeApi.*`); keep ConsumetPlayer.vue on old API for one more commit as control | The "HiAnime" tab now actually streams from AnimeKai under the hood (label stays "HiAnime" for now to preserve mental model — see §6) | "HiAnime" tab works via new scraper. "Consumet" tab still dead. **Net: 1 of 2 EN tabs alive — first time since C0.** |
| **C3** Add second provider (AnimePahe) to orchestrator; rewire ConsumetPlayer.vue to call `scraperApi.*` with a `?prefer=animepahe` param | Both EN tabs now point at the scraper. "HiAnime" tab prefers AnimeKai, "Consumet" tab prefers AnimePahe | Both tabs work. **Net: both EN players alive.** |
| **C4** Frontend label cleanup: "HiAnime" → "English (Primary)", "Consumet" → "English (Backup)" — preserves the two-tab user model while replacing branding | UX-only commit | Same as C3. |
| **C5** Add third provider (Anitaku) to orchestrator as cold-spare | Hidden behind orchestrator; no user-visible change | Same as C4. |
| **C6** REMOVE `services/catalog/internal/parser/hianime/`, `services/catalog/internal/parser/consumet/`, the `aniwatch` + `consumet` containers from `docker/docker-compose.yml:109-145`, and `ANIWATCH_API_URL` + `CONSUMET_API_URL` env vars | Code deletion only; everything served by orchestrator | Same as C5. **Net: dead-weight removed, EN players still alive.** |
| **C7** (Optional) Merge HiAnimePlayer.vue + ConsumetPlayer.vue into single `ScraperPlayer.vue` (see §8) — defer or skip | Component refactor only | Same as C6. |

### Cutover guardrails

- **No commit between C1 and C6 may delete the old aniwatch/consumet code.** They're dead but their absence breaks the deploy-rollback story.
- **The frontend must NOT be changed in the same commit as a backend API change.** Backend changes ship first, frontend is point-2 after backend is verified healthy.
- **C3 is the critical "demote risk" point** — if AnimePahe is flaky, this commit reduces "Consumet tab" reliability. Defer C3 until AnimeKai has been live for ≥ 48h with `parser_requests_total{provider="animekai",status="error"} / total < 5%`.

---

## 6. Gateway routing — preserve or break?

**Recommendation: introduce new `/api/anime/{id}/scraper/*` paths under the existing `/api/anime` route group, and have the frontend players migrate one at a time. Do NOT pretend to be HiAnime or Consumet.**

### Why a clean break

The existing gateway routing rule is `/api/anime/* → catalog:8081` (see CLAUDE.md "Gateway Routing"). New endpoints `/api/anime/{id}/scraper/*` route there automatically — **zero gateway changes**.

### Why NOT impersonate the old routes

Looking at `services/catalog/internal/transport/router.go:82-88`:

```go
// HiAnime video sources
r.Get("/{animeId}/hianime/episodes", catalogHandler.GetHiAnimeEpisodes)
r.Get("/{animeId}/hianime/servers", catalogHandler.GetHiAnimeServers)
r.Get("/{animeId}/hianime/stream", catalogHandler.GetHiAnimeStream)
// Consumet video sources
r.Get("/{animeId}/consumet/episodes", catalogHandler.GetConsumetEpisodes)
r.Get("/{animeId}/consumet/servers", catalogHandler.GetConsumetServers)
r.Get("/{animeId}/consumet/stream", catalogHandler.GetConsumetStream)
```

These names are tied to the dead upstream products. Reusing them means:

- Future devs see `hianime` in code paths and assume `hianime.to` is involved → wasted hours of confusion
- Grafana metric labels (`parser_requests_total{provider="hianime"}`) lie about what's actually running
- Server-name string mappings (`"hd-2"`, `"hd-1"` at `hianime/client.go:21-30`) leak HiAnime-specific concepts into provider-agnostic code

### Compromise: keep the user-visible labels, change the internals

The **frontend tab labels** ("HiAnime", "Consumet") stay during C2-C3 to preserve the mental model. The **backend route names** become `/scraper/*`. The frontend's `consumetApi` and `hianimeApi` objects (frontend/web/src/api/client.ts:394, 413) become thin wrappers around `scraperApi` with a hard-coded `?prefer=` param during C2-C3, then collapse to a single `scraperApi` in C4.

Final route set (added to `services/catalog/internal/transport/router.go` alongside existing `/animelib/*` subroutes at line 90):

```go
// Scraper video sources (replaces dead HiAnime + Consumet routes)
r.Get("/{animeId}/scraper/episodes", catalogHandler.GetScraperEpisodes)
r.Get("/{animeId}/scraper/servers",  catalogHandler.GetScraperServers)
r.Get("/{animeId}/scraper/stream",   catalogHandler.GetScraperStream)
r.Get("/{animeId}/scraper/health",   catalogHandler.GetScraperHealth) // admin/debug
```

Plus, alongside line 100 (Kodik search) etc.:

```go
r.Get("/scraper/search", catalogHandler.SearchScraper)
```

All `?prefer=animekai|animepahe|anitaku` is supported but optional — orchestrator uses declared order when omitted.

---

## 7. megacloud-extractor — keep as-is, call via HTTP

**Recommendation: KEEP the existing Node helper, call it via HTTP from Go. Do NOT replicate the decryption logic in Go.**

### What megacloud-extractor already does (verified from `docker/megacloud-extractor/server.js`)

- Fetches the megacloud embed page (lines 44-77)
- Extracts the client key via 4 different regex patterns (lines 55-76) — these change frequently
- Calls `getSources` (lines 86-103) and handles both encrypted and unencrypted responses
- Performs AES-256-CBC decryption with OpenSSL-compatible key/IV derivation using `cinemaxhq/keys` (lines 152-205)
- Returns clean JSON: `{ sources: [...], tracks: [...], intro, outro }` (lines 36-42, 232-233)

### Why HTTP, not Go-native

| Reason | Detail |
|---|---|
| The decryption keys are remote and rotated | `https://raw.githubusercontent.com/cinemaxhq/keys/e1/key` (server.js:158) — rewriting this in Go means re-implementing the key fetch in Go, no win |
| The regex patterns rotate too | All 4 patterns in server.js:55-60 have been changed multiple times by the upstream. JS is more permissive about parsing fragmented HTML than Go's `regexp.Compile` |
| `crypto.createDecipheriv` in Node = ~15 LOC | Equivalent in Go: ~30 LOC of `crypto/aes` + `crypto/cipher` + manual MD5 KDF. Net negative for maintenance |
| It already works | The container is healthy, the patch-aniwatch.sh wiring is the only thing being thrown out (it's aniwatch-specific) |

### What needs to change about megacloud-extractor

- **Decouple from aniwatch.** The `docker/megacloud-extractor/patch-aniwatch.sh` script (referenced at `docker/docker-compose.yml:119-120`) modifies the aniwatch container at runtime — irrelevant once aniwatch is removed in C6.
- **Add a Go HTTP client wrapper.** NEW file `services/catalog/internal/parser/scraper/megacloud_client.go`:

```go
package scraper

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "time"
)

type MegacloudClient struct {
    baseURL string
    http    *http.Client
}

func NewMegacloudClient(baseURL string) *MegacloudClient {
    if baseURL == "" {
        baseURL = "http://megacloud-extractor:3200"
    }
    return &MegacloudClient{
        baseURL: baseURL,
        http:    &http.Client{Timeout: 20 * time.Second},
    }
}

type MegacloudResult struct {
    Sources []struct {
        URL    string `json:"url"`
        Type   string `json:"type"`
        IsM3U8 bool   `json:"isM3U8"`
    } `json:"sources"`
    Tracks []struct {
        URL     string `json:"url"`
        Lang    string `json:"lang"`
        Default bool   `json:"default"`
    } `json:"tracks"`
    Intro *TimeRange `json:"intro,omitempty"`
    Outro *TimeRange `json:"outro,omitempty"`
}

func (m *MegacloudClient) Extract(ctx context.Context, embedURL string) (*MegacloudResult, error) {
    u := fmt.Sprintf("%s/extract?url=%s", m.baseURL, url.QueryEscape(embedURL))
    req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
    resp, err := m.http.Do(req)
    if err != nil { return nil, fmt.Errorf("%w: %v", ErrExtractFailed, err) }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("%w: status %d", ErrExtractFailed, resp.StatusCode)
    }
    var out MegacloudResult
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return nil, fmt.Errorf("%w: %v", ErrExtractFailed, err)
    }
    return &out, nil
}
```

- **Docker compose stays exactly as-is** (`docker/docker-compose.yml:91-107`) except: remove the `depends_on: megacloud-extractor` from the `aniwatch` block (line 121-123) when aniwatch is deleted in C6.

---

## 8. Frontend impact — smallest viable change

**Recommendation: keep both `HiAnimePlayer.vue` and `ConsumetPlayer.vue` for v3.0. Defer the merge into `ScraperPlayer.vue` to a follow-on cleanup. Rename only at the label level.**

### Why both stay (for now)

| Reason | Detail |
|---|---|
| The two components are 1100+ lines each (`HiAnimePlayer.vue` 1434 lines per the grep, `ConsumetPlayer.vue` similar) | Merging them is a serious refactor with regression risk |
| They already do nearly the same thing | Both consume the existing HiAnime/Consumet DTOs which are identical-by-shape to the new `scraper.Stream` (see §2) |
| The frontend cutover in C2/C3 is JUST pointing each at the new API client | Net change per file: ~10 lines (swap `hianimeApi.getStream` for `scraperApi.getStream`) |
| Users have a mental model of "two EN tabs = two chances at a working stream" | Collapsing to one tab loses the redundancy signal even though we now do it backend-side |

### Concrete changes per player file

- **`frontend/web/src/components/player/HiAnimePlayer.vue`** (MODIFIED in C2):
  - Line 376 area (`import { hianimeApi, jimakuApi, userApi } from '@/api/client'`) → swap `hianimeApi` for `scraperApi`
  - Line 384 area (`player-type="hianime"`) → keep label as-is for v3.0 UX continuity; rename to `english-primary` in a later UX pass
  - The 4-5 call sites that invoke `hianimeApi.getEpisodes/getServers/getStream/search` → swap to `scraperApi.*` with optional `prefer: 'animekai'`
- **`frontend/web/src/components/player/ConsumetPlayer.vue`** (MODIFIED in C3): same shape of swap, with `prefer: 'animepahe'`
- **`frontend/web/src/api/client.ts`** (MODIFIED): add new `scraperApi` export at line 394 area, mirroring the `hianimeApi` shape exactly:

```typescript
export const scraperApi = {
  getEpisodes: (animeId: string, opts?: { prefer?: string }) =>
    apiClient.get(`/anime/${animeId}/scraper/episodes`, { params: opts }),
  getServers: (animeId: string, episodeId: string, opts?: { prefer?: string }) =>
    apiClient.get(`/anime/${animeId}/scraper/servers`, { params: { episode: episodeId, ...opts } }),
  getStream: (animeId: string, episodeId: string, serverId: string, category: string, opts?: { prefer?: string }) =>
    apiClient.get(`/anime/${animeId}/scraper/stream`, { params: { episode: episodeId, server: serverId, category, ...opts } }),
  search: (query: string) => apiClient.get('/scraper/search', { params: { q: query } }),
}
```

- **`frontend/web/src/views/Anime.vue`** lines 437-456 (the `<HiAnimePlayer>` and `<ConsumetPlayer>` tab branches): **no change for v3.0** — the players still receive the same props, they just talk to a different backend internally.

**The result:** the frontend touches 3 files (HiAnimePlayer.vue, ConsumetPlayer.vue, client.ts), each with surgical line-level edits. Zero new components, zero new routes, zero new types.

---

## 9. Build order — independently shippable phases

Each phase is a deployable unit. Numbers reference the cutover commits in §5.

### Phase A — Contract & skeleton (no behavior change, no user-visible effect)
**Ships C1's scaffolding.**

Dependencies: none.

Files (all NEW):
- `services/catalog/internal/parser/scraper/provider.go` — interface + DTOs + sentinel errors (§2)
- `services/catalog/internal/parser/scraper/orchestrator.go` — skeleton with zero providers registered (§3)
- `services/catalog/internal/parser/scraper/megacloud_client.go` — HTTP client for the Node helper (§7)
- `services/catalog/internal/parser/scraper/provider_test.go` — interface assertion + DTO marshal round-trip

Acceptance: `go build ./...` passes; `services/catalog/cmd/catalog-api/main.go` starts unchanged; new orchestrator is instantiated but unused.

### Phase B — First provider (AnimeKai) + new HTTP endpoints
**Ships C1.**

Dependencies: Phase A.

Files:
- `services/catalog/internal/parser/scraper/animekai/client.go` (NEW)
- `services/catalog/internal/parser/scraper/animekai/client_test.go` (NEW)
- `services/catalog/internal/handler/catalog.go` (MODIFIED): 4 new handlers `GetScraperEpisodes`, `GetScraperServers`, `GetScraperStream`, `SearchScraper` — mirror the structure of `GetHiAnimeStream` at lines 726-756
- `services/catalog/internal/transport/router.go` (MODIFIED): register the 4 new routes alongside existing `/hianime/*` (line 82-84) and `/consumet/*` (lines 86-88) — DO NOT remove old routes yet
- `services/catalog/internal/service/catalog.go` (MODIFIED): add `scraper *scraper.Orchestrator` field (line 37-55), wire in `NewCatalogService` (line 68-135) with the AnimeKai provider registered, add `GetScraperEpisodes`, `GetScraperServers`, `GetScraperStream`, `SearchScraper` service methods (mirror lines 1640-1820 patterns)
- `libs/videoutils/proxy.go` (MODIFIED): append AnimeKai CDN domains to `HLSProxyAllowedDomains` (lines 230-255)

Acceptance: hitting `GET /api/anime/{id}/scraper/episodes` returns a valid `[]Episode` for at least 3 representative anime; `parser_requests_total{provider="animekai"}` counter ticks up.

### Phase C — Frontend cutover for one tab (HiAnime → scraper)
**Ships C2.**

Dependencies: Phase B verified healthy for ≥ 24h.

Files:
- `frontend/web/src/api/client.ts` (MODIFIED): add `scraperApi` export (§8)
- `frontend/web/src/components/player/HiAnimePlayer.vue` (MODIFIED): swap `hianimeApi` → `scraperApi` with `prefer: 'animekai'`

Acceptance: "HiAnime" tab on `/anime/:id` returns a playable HLS stream for a test anime that previously failed.

### Phase D — Second provider (AnimePahe) + Consumet-tab cutover
**Ships C3.**

Dependencies: Phase C verified healthy for ≥ 48h (per §5 guardrail).

Files:
- `services/catalog/internal/parser/scraper/animepahe/client.go` (NEW)
- `services/catalog/internal/parser/scraper/animepahe/client_test.go` (NEW)
- `services/catalog/internal/service/catalog.go` (MODIFIED): register AnimePahe as the second provider in the orchestrator at line ~118
- `frontend/web/src/components/player/ConsumetPlayer.vue` (MODIFIED): swap `consumetApi` → `scraperApi` with `prefer: 'animepahe'`
- `libs/videoutils/proxy.go` (MODIFIED): AnimePahe CDNs `kwik.cx`, `owocdn.top`, `uwucdn.top` are already in the allowlist (lines 244-246) — no change needed

Acceptance: both EN player tabs serve playable streams; failover from AnimeKai → AnimePahe is observable in `parser_fallback_total`.

### Phase E — Third provider (Anitaku/Gogoanime) as cold spare
**Ships C5 (C4 is a label-only UX commit — independent of backend).**

Dependencies: Phase D verified healthy for ≥ 48h.

Files:
- `services/catalog/internal/parser/scraper/anitaku/client.go` (NEW)
- `services/catalog/internal/parser/scraper/anitaku/client_test.go` (NEW)
- `services/catalog/internal/service/catalog.go` (MODIFIED): register Anitaku as the third provider
- `libs/videoutils/proxy.go` (MODIFIED): append `*.gogocdn.*`, `*.embtaku.*` (or whatever the actual mirror CDNs are — discover during impl)

Acceptance: forcing AnimeKai + AnimePahe to fail (block their domains) still produces a playable stream; `provider_id` in the response shows `"anitaku"`.

### Phase F — Dead-code removal
**Ships C6.**

Dependencies: Phase E shipped AND at least 7 days of production traffic served via scraper without rollback.

Files:
- `services/catalog/internal/parser/hianime/` (DELETED)
- `services/catalog/internal/parser/consumet/` (DELETED)
- `services/catalog/internal/service/catalog.go` (MODIFIED): drop `hianimeClient`, `consumetClient` fields and their methods (lines 44-45, ~1640-1820, ~1900-2300)
- `services/catalog/internal/handler/catalog.go` (MODIFIED): drop the 7 handler functions `GetHiAnimeEpisodes`/`GetHiAnimeServers`/`GetHiAnimeStream`/`SearchHiAnime`/`GetConsumetEpisodes`/`GetConsumetServers`/`GetConsumetStream`/`SearchConsumet` (lines 685-842)
- `services/catalog/internal/transport/router.go` (MODIFIED): drop the 6 `/hianime/*` and `/consumet/*` routes (lines 82-88, 104, 107)
- `services/catalog/internal/config/config.go` (MODIFIED): drop `AniwatchAPIURL` and `ConsumetAPIURL` (lines 112, 115)
- `docker/docker-compose.yml` (MODIFIED): delete the `aniwatch` block (lines 109-130), the `consumet` block (lines 132-145), and the `depends_on` references at lines 365-368; remove `ANIWATCH_API_URL` and `CONSUMET_API_URL` env vars at lines 351-352
- `docker/megacloud-extractor/patch-aniwatch.sh` (DELETED) — aniwatch is gone, the patch script has no use
- `frontend/web/src/api/client.ts` (MODIFIED): delete `hianimeApi` and `consumetApi` exports (lines ~394, 413)

Acceptance: `docker compose up` no longer pulls aniwatch or consumet images; the catalog container's image size drops; no production endpoints return 404.

### Phase G (optional) — Frontend component merge
Defer indefinitely. The two-component setup is fine. Only collapse when there's a UX-driven reason (e.g. a redesign that wants a single "English source" tab).

---

## 10. Data-flow diagrams

### Diagram 1 — Single stream request, happy path (AnimeKai serves it)

```
                  ┌──────────────────────────────────────────────────────────────────┐
                  │                          Frontend (Vue 3)                         │
                  │                                                                   │
                  │  Anime.vue   ──renders──>  HiAnimePlayer.vue (post Phase C)       │
                  │                                  │                                 │
                  │                                  │ scraperApi.getStream(           │
                  │                                  │   animeId, ep, server,          │
                  │                                  │   category, {prefer:'animekai'})│
                  └──────────────────────────────────┼─────────────────────────────────┘
                                                     ▼
                  ┌──────────────────────────────────────────────────────────────────┐
                  │  Gateway (:8000)  GET /api/anime/{id}/scraper/stream             │
                  │              ──────route────>  catalog:8081                       │
                  └──────────────────────────────────┼─────────────────────────────────┘
                                                     ▼
   ┌────────────────────────────────────── services/catalog ──────────────────────────────────────┐
   │                                                                                              │
   │  handler.GetScraperStream  ──>  service.GetScraperStream  ──>  scraper.Orchestrator.GetStream│
   │                                                                          │                   │
   │                                                            check Redis cache (libs/cache)    │
   │                                                                          │ miss              │
   │                                                                          ▼                   │
   │                                                     for p := range providers (animekai 1st): │
   │                                                          getCachedHealth(p) == Healthy?      │
   │                                                                          │ yes               │
   │                                                                          ▼                   │
   │                                                              animekai.Client.GetStream       │
   │                                                              (FindID → ListServers           │
   │                                                                ──>  embed page URL)          │
   │                                                                          │                   │
   │                                                                          ▼                   │
   │                                          ───────HTTP────>  docker/megacloud-extractor:3200   │
   │                                                            (Node helper, server.js)          │
   │                                          <──{sources,tracks,intro,outro}── decrypted JSON    │
   │                                                                          │                   │
   │                                          ObserveParser("animekai", "get_stream", ..., &err)  │
   │                                                                          │                   │
   │                                          libs/cache.Set scraper:stream:* (30m TTL)           │
   │                                                                          ▼                   │
   │                                                            scraper.Stream { URL=m3u8, ... }  │
   └──────────────────────────────────────────────────┬───────────────────────────────────────────┘
                                                      │
                                                      ▼ JSON
                  ┌──────────────────────────────────────────────────────────────────┐
                  │  Frontend receives { url, type:"hls", subtitles, headers,         │
                  │                       intro, outro, provider_id:"animekai" }      │
                  │                                                                   │
                  │  HiAnimePlayer rewrites url through HLS proxy (no change):        │
                  │     /api/streaming/hls-proxy?url=<m3u8>&referer=<...>             │
                  └──────────────────────────────────┼─────────────────────────────────┘
                                                     ▼
                  ┌──────────────────────────────────────────────────────────────────┐
                  │  services/streaming  (libs/videoutils/proxy.go ProxyWithReferer)  │
                  │      isHLSDomainAllowed(host)?                                    │
                  │          if megacloud.tv / netmagcdn.com / mcloud.to → allow      │
                  │      rewriteM3U8URLs → rewrite all child URLs through proxy too   │
                  └──────────────────────────────────┼─────────────────────────────────┘
                                                     ▼ HLS m3u8 + .ts segments
                  ┌──────────────────────────────────────────────────────────────────┐
                  │  Frontend HLS.js / Video.js plays the stream                      │
                  │  SubtitleOverlay renders JP subs (independent path via jimakuApi) │
                  └──────────────────────────────────────────────────────────────────┘
```

### Diagram 2 — Failover (AnimeKai dies, AnimePahe takes over) — post-Phase D

```
  handler.GetScraperStream  ──>  service.GetScraperStream  ──>  Orchestrator.GetStream
                                                                          │
                                                                          ▼
                                          for p := [animekai, animepahe, anitaku]:
                                          ┌─────────────────────────────────────────┐
                                          │ p == animekai:                          │
                                          │    health Healthy (last check 30s ago)  │
                                          │    GetStream(...) → ErrProviderDown     │
                                          │    (HTTP 403 — Cloudflare challenge)    │
                                          │    markUnhealthy(animekai, "403")       │
                                          │    metrics.ParserFallbackTotal{from=    │
                                          │       "animekai", to="tried_next"}++    │
                                          │    continue                             │
                                          ├─────────────────────────────────────────┤
                                          │ p == animepahe:                         │
                                          │    health Healthy                       │
                                          │    GetStream(...) → Stream{URL=...,     │
                                          │       provider_id="animepahe"}          │
                                          │    cache.Set scraper:stream:* 30m       │
                                          │    return                               │
                                          └─────────────────────────────────────────┘
                                                                          │
                                                                          ▼
                                          frontend receives provider_id="animepahe"
                                          frontend optionally shows a small badge
                                          ("Source: AnimePahe") to surface the swap
```

---

## Patterns to follow

### Pattern 1 — Inflight de-dupe for FindID
**What:** Two concurrent stream requests for anime X must not trigger two AnimeKai name-searches.
**When:** Any `FindID` call path. Reuse the existing `hianimeInflight` sync.Map pattern at `services/catalog/internal/service/catalog.go:30-35, 54`.
**Example:** existing `findHiAnimeID` code at `catalog.go:1643+` is the template; copy it for each scraper provider that does name-search.

### Pattern 2 — Defer ObserveParser at every external-call entry
**What:** Every Provider method that hits the wire MUST `defer metrics.ObserveParser(provider, op, start, &err)`.
**When:** First line of `ListEpisodes`, `ListServers`, `GetStream`. Matches existing pattern at `catalog.go:1680-1681, 1721-1722`.
**Why:** Loses provider observability if missing; without it, the silent aniwatch death (2026-05-09) repeats.

### Pattern 3 — Sentinel errors at the provider boundary, wrapped errors above
**What:** Providers return `fmt.Errorf("%w: detail", scraper.ErrProviderDown)`. The orchestrator uses `errors.Is(err, ErrProviderDown)` to decide failover behavior. The service layer wraps the orchestrator's final error in `errors.NotFound(...)` for the handler.
**Why:** Same pattern catalog already uses: `return nil, errors.NotFound("servers not available")` at `catalog.go:1701`.

---

## Anti-patterns to avoid

### Anti-pattern 1 — In-client provider fallback
**What:** Having `animekai.Client` fall back to AnimePahe internally (mirroring `consumet.Client.providerList()` at `consumet/client.go:55-66`).
**Why bad:** Each provider becomes aware of every other provider. The orchestrator becomes a useless pass-through. New providers require modifying old providers.
**Instead:** Each provider implements its own logic only. The orchestrator owns the ordering.

### Anti-pattern 2 — Service layer parses HTML
**What:** Pulling embed-page HTML parsing into `CatalogService` methods.
**Why bad:** Service layer must stay testable without network — today it's a clean orchestration of typed parser clients (see `catalog.go:1640-1820`).
**Instead:** All HTML/embed parsing lives in `internal/parser/scraper/{provider}/client.go`; service calls the typed methods only.

### Anti-pattern 3 — Replicating megacloud decryption in Go
**What:** Porting `decryptSources` (`docker/megacloud-extractor/server.js:152-205`) to Go.
**Why bad:** The decryption KEY is fetched from `cinemaxhq/keys/e1/key` at runtime and rotates. The 4 regex patterns for the client key rotate. Maintaining this in two languages = guaranteed drift.
**Instead:** Call the Node helper via HTTP. See §7.

### Anti-pattern 4 — Caching dead-provider responses
**What:** Storing `scraper:stream:*` for a failed call.
**Why bad:** Locks the failure in for 30 minutes. Today's HiAnime path correctly checks `err == nil && stream.URL != ""` before caching at `catalog.go:1793-1794` — preserve this discipline.
**Instead:** Cache only on success; rely on the 60-second health-check unhealthy-skip for fast recovery.

### Anti-pattern 5 — Synchronous deletion of aniwatch + consumet before scraper is proven
**What:** Combining Phase F with earlier phases.
**Why bad:** Removes the rollback path. If scraper has a latent bug discovered at week 2 of production, we have no working EN player to roll back to.
**Instead:** Phase F requires ≥ 7 days of clean production traffic on the scraper before deletion.

---

## Scalability considerations

| Concern | At 100 users (today) | At 10K users | At 1M users |
|---|---|---|---|
| Provider rate limits | Stay under default; rotate User-Agent | Per-IP egress pool; 1 outbound per provider per 100ms ceiling | Dedicated outbound IP per provider; queue + circuit breaker; consider becoming a paying user of one upstream |
| Redis cache size | `scraper:stream:*` 30m TTL — ~3KB per entry × 50 active anime ≈ 150KB | Same TTL; ~10MB at 10K active anime | Tiered cache (in-memory LRU + Redis); cache by (anime, ep, category) only — strip user context |
| megacloud-extractor throughput | 1 container, 1 vCPU is plenty (2-3 req/sec) | Add `replicas: 3` to docker-compose; round-robin via internal DNS | Replace with hot-pool of pre-warmed Playwright contexts; horizontal autoscale on queue depth |
| HLS proxy bandwidth | ~5 MB/s per stream × concurrent users (CLAUDE.md mentions rate-limited copy at 5MB/s) | Promote streaming service to dedicated node | Pay for actual CDN; v3.0 explicitly excludes this per PROJECT.md |

The recommended design imposes zero new scalability problems beyond what already exists today — every external call already passes through caches, and the orchestrator's fan-out adds at most one Redis read + one healthcheck cache read per request.

---

## Sources

- `services/catalog/internal/transport/router.go` (current route registrations, lines 50-98) — HIGH (read in pass)
- `services/catalog/internal/handler/catalog.go` (HiAnime + Consumet handlers, lines 685-842) — HIGH
- `services/catalog/internal/service/catalog.go` (CatalogService struct, NewCatalogService, GetHiAnime* methods at lines 37-135, 1640-1820) — HIGH
- `services/catalog/internal/parser/hianime/client.go` (current EN client; aniwatch coupling at line 44) — HIGH
- `services/catalog/internal/parser/consumet/client.go` (current EN client; provider list pattern at lines 55-66) — HIGH
- `services/catalog/internal/parser/kodik/client.go` (reference for "working RU parser" shape) — HIGH
- `libs/idmapping/client.go` (ARM client, `ResolveByShikimoriID` at line 40, `ResolveByMALID` at line 45) — HIGH
- `libs/cache/cache.go` + `libs/cache/ttl.go` (Set/Get/GetOrSet patterns, TTL constants) — HIGH
- `libs/metrics/parser.go` (ObserveParser pattern at lines 43-51) — HIGH
- `libs/videoutils/proxy.go` (HLS proxy + HLSProxyAllowedDomains at lines 230-255) — HIGH
- `docker/megacloud-extractor/server.js` (Node helper, extraction + decryption logic) — HIGH
- `docker/docker-compose.yml` (aniwatch lines 109-130, consumet lines 132-145, catalog env vars lines 351-352) — HIGH
- `frontend/web/src/components/player/HiAnimePlayer.vue` + `ConsumetPlayer.vue` (current API consumers) — HIGH
- `frontend/web/src/api/client.ts` (`hianimeApi` line 394, `consumetApi` line 413) — HIGH
- `frontend/web/src/views/Anime.vue` (player composition lines 437-456, async imports lines 757-760) — HIGH
- `.planning/PROJECT.md` (v3.0 milestone goals) — HIGH
- `.planning/STATE.md` (verified provider status as of 2026-05-09 triage) — HIGH
- `CLAUDE.md` (Go conventions, gateway routing, "Don't add complex abstractions") — HIGH
