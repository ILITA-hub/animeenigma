# Phase 1: Backend Aggregator + Static Cards — Pattern Map

**Mapped:** 2026-05-21
**Files analyzed:** 13 new + 4 modified (extracted from CONTEXT.md §"File layout" and RESEARCH.md §"Architectural Responsibility Map")
**Analogs found:** 13 / 13 (every new file maps to an existing pattern in `services/catalog` or `services/gateway`)

## File Classification

### New files

| New File | Role | Data Flow | Closest Analog | Match Quality |
|----------|------|-----------|----------------|---------------|
| `services/catalog/internal/handler/spotlight.go` | HTTP handler | request-response | `services/catalog/internal/handler/news.go` | exact (chi GET → cache → JSON envelope) |
| `services/catalog/internal/service/spotlight/aggregator.go` | service orchestrator | concurrent fan-out + collect | `services/catalog/internal/service/subs_aggregator.go` (lines 109-156) | exact (sync.WaitGroup + buffered chan + drop-on-error) |
| `services/catalog/internal/service/spotlight/types.go` | type definitions | n/a | `services/catalog/internal/service/subs_aggregator.go` (lines 59-73) — `SubtitleTrack` / `AggregateResponse` struct shape | exact |
| `services/catalog/internal/service/spotlight/seed.go` | utility | pure function | none — small new utility; pattern: stdlib `time.Now().UTC().Format("2006-01-02")` (used in `libs/cache/ttl.go` and any date-keyed cache) | n/a (trivial new helper) |
| `services/catalog/internal/service/spotlight/cards/anime_of_day.go` | per-card resolver | DB read + cache | `services/catalog/internal/service/subs_aggregator.go` (`fetchJimaku` + `fetchOpenSubtitles` lines 163-250) for "fetch + transform → typed track list" | role-match (provider-style resolver) |
| `services/catalog/internal/service/spotlight/cards/random_tail.go` | per-card resolver | DB read + cache | same as `anime_of_day.go` | role-match |
| `services/catalog/internal/service/spotlight/cards/latest_news.go` | per-card resolver | HTTP fetch + cache | `services/catalog/internal/handler/news.go` (telegram fetch flow) | role-match |
| `services/catalog/internal/service/spotlight/cards/platform_stats.go` | per-card resolver | DB count + cache | RESEARCH.md §"Code Examples — Example 5" already gives full body | role-match (direct GORM Count usage) |
| `services/catalog/internal/service/spotlight/client/web_client.go` | external HTTP client | request-response | `services/catalog/internal/parser/telegram/client.go` (lines 33-66) | exact (constructor with `*http.Client`, `http.NewRequestWithContext`, json.Decoder, `fmt.Errorf` wrapping) |
| `services/catalog/internal/service/spotlight/aggregator_test.go` | test | n/a | `services/catalog/internal/service/scraper_test.go` (lines 15-100) — handwritten struct fakes pattern | exact |
| `services/catalog/internal/service/spotlight/cards/*_test.go` | test | n/a | `services/catalog/internal/service/scraper_test.go` | exact |
| `services/catalog/internal/service/spotlight/client/web_client_test.go` | test | n/a | `services/catalog/internal/service/scraper_test.go` + `net/http/httptest` stdlib | role-match |
| `services/catalog/internal/handler/spotlight_test.go` | test | n/a | `services/catalog/internal/service/scraper_test.go` + `net/http/httptest` | role-match |

### Modified files

| Modified File | Role | What Changes | Closest Pattern |
|---------------|------|--------------|-----------------|
| `services/catalog/internal/config/config.go` | config | add `SpotlightEnabled bool` env-bound, default `true` | RESEARCH.md §Pattern: existing `getEnv*` helpers (lines 205-228) — but `bool` not yet in helpers → add `getEnvBool(key, defaultVal bool) bool` mirroring `getEnvInt` |
| `services/catalog/internal/transport/router.go` | router | add `r.Get("/home/spotlight", spotlightHandler.Get)` under `/api` group | existing public catalog routes (lines 60-122) |
| `services/catalog/cmd/catalog-api/main.go` | DI wiring | construct webClient + 4 resolvers + aggregator + handler; pass to `transport.NewRouter` | existing wiring of `subsAggregator` (lines 180-188) and `newsHandler` (line 139) |
| `services/gateway/internal/transport/router.go` | gateway router | add `r.HandleFunc("/home/spotlight", proxyHandler.ProxyToCatalog)` inside the `/api` Route block BEFORE the `/anime/*` catch-all | existing `r.HandleFunc("/skip-times/*", proxyHandler.ProxyToCatalog)` at line 218 |
| `docker/.env.example` | config | add `SPOTLIGHT_ENABLED=true` | existing env entries — append to catalog block |

---

## Pattern Assignments

### `services/catalog/internal/handler/spotlight.go` (HTTP handler, request-response)

**Analog:** `services/catalog/internal/handler/news.go` (entire file — 59 lines, perfect template)

**Imports pattern** (`news.go` lines 1-12) — the spotlight handler swaps `parser/telegram` for `service/spotlight`:
```go
package handler

import (
    "net/http"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/errors"
    "github.com/ILITA-hub/animeenigma/libs/httputil"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/telegram"
)
```

**Constructor pattern** (`news.go` lines 19-33) — `New*Handler(deps...) *Handler` returning a pointer:
```go
type NewsHandler struct {
    telegramClient *telegram.Client
    cache          cache.Cache
    log            *logger.Logger
}

func NewNewsHandler(telegramClient *telegram.Client, cache cache.Cache, log *logger.Logger) *NewsHandler {
    return &NewsHandler{
        telegramClient: telegramClient,
        cache:          cache,
        log:            log,
    }
}
```

**Handler shape — IMPORTANT DIVERGENCE for spotlight** (`news.go` lines 35-59 vs spotlight handler requirements):

The news handler uses `httputil.OK(w, items)` — which wraps the payload in `{success:true, data:..., error:null}` envelope. The spotlight design doc §4.1 requires a bare `{cards, generated_at}` envelope WITHOUT the `success/data` wrapper. So spotlight CANNOT call `httputil.OK`; it MUST write JSON directly:

```go
// news.go pattern (uses httputil envelope — NOT to be copied for spotlight):
func (h *NewsHandler) GetNews(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    var items []telegram.NewsItem
    err := h.cache.GetOrSet(ctx, newsRedisKey, &items, newsTTL, func() (interface{}, error) {
        h.log.Infow("fetching news from telegram channel")
        fetched, err := h.telegramClient.FetchNews(ctx)
        if err != nil {
            h.log.Errorw("failed to fetch telegram news", "error", err)
            return nil, errors.ExternalAPI("telegram", err)
        }
        h.log.Infow("fetched telegram news", "count", len(fetched))
        return fetched, nil
    })
    if err != nil {
        httputil.Error(w, err)
        return
    }
    httputil.OK(w, items)  // <-- wraps in {success, data} — NOT WHAT SPOTLIGHT NEEDS
}
```

**Use instead — direct `json.NewEncoder(w).Encode(resp)`** (RESEARCH.md §Code Examples — Example 1, lines 696-755). Copy that example verbatim including:
- Feature-flag short-circuit: `if !h.spotlightEnabled { w.WriteHeader(http.StatusNotFound); return }` (bare 404, no body — see Pitfall: `httputil.NotFound` would emit `{success:false,error:{...}}` which the frontend HSB-FE-02 would still parse as truthy)
- Two log lines: `h.log.Infow("spotlight.request", "user", "anon")` at entry and `h.log.Infow("spotlight.aggregated", "cards_returned", len(resp.Cards), "ms_total", time.Since(started).Milliseconds())` at exit
- Outer `context.WithTimeout(r.Context(), 2*time.Second)` + `defer cancel()`

**Error handling — for catastrophic 500 path:** Do NOT use `httputil.Error` (wrong envelope). Inline:
```go
http.Error(w, `{"cards":[],"generated_at":"...","error":"internal"}`, http.StatusInternalServerError)
```
or write `{cards:[], generated_at: ...}` via `json.NewEncoder` and set status before encode.

---

### `services/catalog/internal/service/spotlight/aggregator.go` (service orchestrator, concurrent fan-out)

**Analog:** `services/catalog/internal/service/subs_aggregator.go` (lines 109-156 — the canonical fan-out template in this codebase)

**Imports pattern** (`subs_aggregator.go` lines 1-20):
```go
package service

import (
    "context"
    "errors"
    "fmt"
    "sort"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/idmapping"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/jimaku"
    ...
)
```

**Aggregator struct + constructor** (`subs_aggregator.go` lines 30-56) — same shape, swap dependencies:
```go
type SubsAggregator struct {
    jimaku    *jimaku.Client
    opensubs  *opensubtitles.Client
    idmap     *idmapping.Client
    animeRepo *repo.AnimeRepository
    cache     *cache.RedisCache
    log       *logger.Logger
}

func NewSubsAggregator(
    jimakuClient *jimaku.Client,
    openSubsClient *opensubtitles.Client,
    idMapClient *idmapping.Client,
    animeRepo *repo.AnimeRepository,
    redisCache *cache.RedisCache,
    log *logger.Logger,
) *SubsAggregator {
    return &SubsAggregator{
        jimaku:    jimakuClient,
        opensubs:  openSubsClient,
        idmap:     idMapClient,
        animeRepo: animeRepo,
        cache:     redisCache,
        log:       log,
    }
}
```

**Core fan-out pattern** (`subs_aggregator.go` lines 109-156 — VERBATIM, the load-bearing excerpt):
```go
type providerResult struct {
    name   string
    tracks []SubtitleTrack
    err    error
}
resultsCh := make(chan providerResult, 2)
var wg sync.WaitGroup

// Jimaku — JP only, keyed by AniList ID.
wg.Add(1)
go func() {
    defer wg.Done()
    tracks, err := s.fetchJimaku(ctx, anime, episode)
    resultsCh <- providerResult{name: "jimaku", tracks: tracks, err: err}
}()

// OpenSubtitles — multi-language, keyed by IMDb/TMDB.
wg.Add(1)
go func() {
    defer wg.Done()
    tracks, err := s.fetchOpenSubtitles(ctx, anime, episode, langs)
    resultsCh <- providerResult{name: "opensubtitles", tracks: tracks, err: err}
}()

go func() {
    wg.Wait()
    close(resultsCh)
}()

resp := &AggregateResponse{
    Languages: map[string][]SubtitleTrack{},
    Episode:   episode,
}

for r := range resultsCh {
    if r.err != nil {
        s.log.Warnw("subs aggregator: provider failed",
            "provider", r.name, "anime_id", animeID, "episode", episode, "error", r.err)
        resp.ProvidersDown = append(resp.ProvidersDown, r.name)
        continue
    }
    for _, t := range r.tracks {
        if len(langs) > 0 && !containsLang(langs, t.Lang) {
            continue
        }
        resp.Languages[t.Lang] = append(resp.Languages[t.Lang], t)
    }
}

dedupe(resp.Languages)
_ = s.cache.Set(ctx, cacheKey, resp, 6*time.Hour)
return resp, nil
```

**Spotlight adaptations** (per RESEARCH.md §Pattern 1 and §Code Examples Example 2):
1. Replace `providerResult{name, tracks, err}` with `result{name string, card *Card, err error}` — a nil `card` means "eligible=false, drop me silently."
2. Loop over a `[]struct{name, fn}` resolver list instead of inlining 4 goroutines.
3. Inside each goroutine, wrap with `cctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond); defer cancel()` BEFORE calling `r.Resolve(cctx, userID)` — this is the per-card 800ms ctx (HSB-BE-03).
4. Log dropped cards via `a.log.Errorw("spotlight.card_failed", "type", res.name, "error", res.err)` (NOT `Warnw` like subs_aggregator — design doc requires Errorw).
5. After the collect loop, add the snapshot-fallback path (see RESEARCH.md Example 2 lines 822-836).
6. Detach the snapshot WRITE via `go a.saveSnapshot(context.Background(), userID, resp)` — Pitfall: don't pass `ctx` because the request goroutine returns immediately and would cancel the in-flight Redis Set.

**Cache snapshot fallback pattern** (RESEARCH.md lines 839-864):
```go
func (a *Aggregator) loadSnapshot(ctx context.Context, userID *string) *Response {
    var snap Response
    err := a.cache.Get(ctx, a.snapshotKey(userID), &snap)
    if err != nil {
        if !errors.Is(err, cache.ErrNotFound) {
            a.log.Warnw("spotlight.snapshot_load_failed", "error", err)
        }
        return nil
    }
    return &snap
}
```

The `errors.Is(err, cache.ErrNotFound)` check is the standard sentinel pattern — `cache.ErrNotFound` is defined at `libs/cache/cache.go:209`.

---

### `services/catalog/internal/service/spotlight/types.go` (type definitions)

**Analog:** `services/catalog/internal/service/subs_aggregator.go` lines 59-73 (`SubtitleTrack` + `AggregateResponse`)

**Pattern:** Plain Go struct with JSON tags, `omitempty` on optional fields:
```go
// SubtitleTrack is one subtitle file in the aggregated response.
type SubtitleTrack struct {
    URL      string `json:"url"`
    Lang     string `json:"lang"`
    Label    string `json:"label"`
    Format   string `json:"format,omitempty"`
    Provider string `json:"provider"` // "jimaku" or "opensubtitles"
    Release  string `json:"release,omitempty"`
}

// AggregateResponse is the handler payload.
type AggregateResponse struct {
    Languages     map[string][]SubtitleTrack `json:"languages"`
    Episode       int                        `json:"episode"`
    ProvidersDown []string                   `json:"providers_down,omitempty"`
}
```

**Spotlight adaptation:** Copy the struct-with-tags pattern verbatim. Full type set from RESEARCH.md §Pattern 5 (lines 525-563):
```go
type Card struct {
    Type string `json:"type"`
    Data any    `json:"data"`
}

type AnimeOfDayData struct {
    Anime         domain.Anime `json:"anime"`
    ReasonI18nKey string       `json:"reason_i18n_key,omitempty"`
}

type RandomTailData struct {
    Anime domain.Anime `json:"anime"`
}

type LatestNewsData struct {
    Entries []client.ChangelogEntry `json:"entries"`
}

type StatsMetric struct {
    Key   string `json:"key"`
    Value int64  `json:"value"`
    Delta *int64 `json:"delta,omitempty"`
}

type PlatformStatsData struct {
    Metrics []StatsMetric `json:"metrics"`
}

type Response struct {
    Cards       []Card `json:"cards"`
    GeneratedAt string `json:"generated_at"`
}
```

---

### `services/catalog/internal/service/spotlight/client/web_client.go` (external HTTP client, request-response)

**Analog:** `services/catalog/internal/parser/telegram/client.go` (lines 33-66 — Client struct + constructor + FetchNews entry)

**Constructor + http.Client setup** (`telegram/client.go` lines 33-47) — load-bearing excerpt:
```go
// Client is the Telegram channel scraper client
type Client struct {
    httpClient  *http.Client
    newsChannel string
}

// NewClient creates a new Telegram scraper client
func NewClient(newsChannel string) *Client {
    return &Client{
        httpClient: &http.Client{
            Timeout: 15 * time.Second,
        },
        newsChannel: newsChannel,
    }
}
```

**HTTP request + decode pattern** (`telegram/client.go` lines 52-77):
```go
func (c *Client) FetchNews(ctx context.Context) ([]NewsItem, error) {
    url := fmt.Sprintf("%s/%s", baseURL, c.newsChannel)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("telegram: failed to create request: %w", err)
    }
    req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AnimeEnigma/1.0)")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("telegram: failed to fetch channel: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
        return nil, fmt.Errorf("telegram: unexpected status %d: %s", resp.StatusCode, string(body))
    }
    ...
}
```

**Spotlight adaptations** (per RESEARCH.md §Pattern 4 lines 456-514):
1. Constructor accepts injectable `*http.Client` (for tests with `httptest.Server`) — diverges from telegram's hard-coded client:
   ```go
   func NewWebClient(baseURL string, hc *http.Client) *WebClient {
       if hc == nil {
           hc = &http.Client{Timeout: 500 * time.Millisecond}  // < 800ms per-card budget
       }
       if baseURL == "" {
           baseURL = "http://web:80"
       }
       return &WebClient{baseURL: baseURL, http: hc}
   }
   ```
2. Use `json.NewDecoder(resp.Body).Decode(&groups)` instead of goquery (we're parsing JSON, not HTML).
3. Error-wrap convention `fmt.Errorf("web client: <stage>: %w", err)` mirrors telegram's `"telegram: <stage>"` style — keep the prefix so log greps work.

**Note on `libs/errors` for external API failures:** `news.go` line 47 uses `errors.ExternalAPI("telegram", err)`. The web client can EITHER use this wrapper OR plain `fmt.Errorf`. Per CONTEXT.md the handler returns 200 on partial success — so error wrapping is for log readability only, not status mapping. Match the telegram client style (plain `fmt.Errorf`) for symmetry.

---

### `services/catalog/internal/service/spotlight/cards/platform_stats.go` (per-card resolver, DB count)

**Analog:** RESEARCH.md §Code Examples — Example 5 (lines 908-988) gives the complete body. The GORM Count usage pattern itself is canonical Go-GORM (no codebase analog needed for the SQL — but for the manual cache.Get + cache.Set pattern, see Pitfall 5 below).

**Cache strategy — manual Get/Set instead of GetOrSet** (per RESEARCH.md Open Question 1 + Pitfall 5):
```go
// Cache hit check
if err := p.cache.Get(ctx, key, &data); err == nil {
    return &spotlight.Card{Type: p.Type(), Data: data}, nil
} else if !errors.Is(err, cache.ErrNotFound) {
    return nil, fmt.Errorf("stats cache get: %w", err)
}

// ... compute metrics ...

if len(metrics) == 0 {
    // No metrics computable — card not eligible. Do NOT cache nil (Pitfall 5).
    return nil, nil
}

// Only cache on success (non-empty result).
if err := p.cache.Set(ctx, key, data, 24*time.Hour); err != nil {
    p.log.Warnw("platform_stats.cache_set_failed", "error", err)
}
```

**GORM Count pattern** (from RESEARCH.md Example 5 line 953-955 — verified against `services/catalog/internal/repo/anime.go:129-132` which uses `.Count(&total)`):
```go
var animeCount int64
err := p.db.WithContext(ctx).Model(&domain.Anime{}).
    Where("created_at > ?", time.Now().Add(-7*24*time.Hour)).
    Count(&animeCount).Error
```

The `db.WithContext(ctx)` prefix is the catalog-service convention (see `anime.go` line 75: `r.db.WithContext(ctx).Model(...)`).

---

### `services/catalog/internal/service/spotlight/cards/anime_of_day.go` + `random_tail.go` (DB-backed resolvers)

**Analog for repo call:** `services/catalog/internal/repo/anime.go:74` — `AnimeRepository.Search(ctx, domain.SearchFilters)` signature returns `([]*domain.Anime, int64, error)`.

**Search call pattern** (RESEARCH.md §Pattern 2, lines 388-401):
```go
sm := 8.0
animes, _, err := r.repo.Search(ctx, domain.SearchFilters{
    Sort:     "score",
    Order:    "desc",
    ScoreMin: &sm,
    Page:     1,
    PageSize: 200,
})
if err != nil {
    return nil, err
}
if len(animes) == 0 {
    return nil, nil // eligible=false
}
```

**CRITICAL PITFALL — `AnimeRepository.Search` injects `sort_priority DESC` as primary order** (`anime.go` lines 134-147):
```go
// Phase 11 / UX-21 — sort_priority DESC is the primary pin (CLAUDE.md
// "Pinning anime to the top" convention). When an explicit sort axis is
// requested it overrides the SECOND criterion only — never the pin.
orderBy := "sort_priority DESC, score DESC"
if filters.Sort != "" {
    column := mapSortColumn(filters.Sort)
    order := "DESC"
    if filters.Order == "asc" || (filters.Order == "" && filters.Sort == "title") {
        order = "ASC"
    }
    orderBy = fmt.Sprintf("sort_priority DESC, %s %s", column, order)
}
```

This means `Page=2, PageSize=100` returns ranks 101..200 by `(sort_priority DESC, score DESC)`, NOT pure score. Per RESEARCH.md decision: accept the shift, document in code comment.

**Date seed pattern** (RESEARCH.md §Pattern 3, lines 437-446):
```go
func DateSeedUTC(t time.Time) int {
    u := t.UTC()
    return u.Year()*100*32 + int(u.Month())*32 + u.Day()
}

func DateKeyUTC(t time.Time) string {
    return t.UTC().Format("2006-01-02")
}
```

---

### `services/catalog/internal/service/spotlight/aggregator_test.go` + per-card tests (handwritten struct fakes)

**Analog:** `services/catalog/internal/service/scraper_test.go` (lines 15-100)

**Imports pattern** (`scraper_test.go` lines 1-13):
```go
package service

import (
    "context"
    "errors"
    "net/http"
    "strings"
    "sync/atomic"
    "testing"

    liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)
```

**Fake interface implementation pattern** (`scraper_test.go` lines 16-37) — load-bearing excerpt:
```go
// fakeAnimeFetcher is a minimal implementation of the animeFetcher
// interface that scraper.go is contracted against. It returns whatever
// (anime, err) tuple the test sets up.
type fakeAnimeFetcher struct {
    anime           *domain.Anime
    err             error
    calls           int32
    hasEnglishCalls int32
    hasEnglishVal   bool
}

func (f *fakeAnimeFetcher) GetByID(ctx context.Context, id string) (*domain.Anime, error) {
    atomic.AddInt32(&f.calls, 1)
    return f.anime, f.err
}

func (f *fakeAnimeFetcher) SetHasEnglish(ctx context.Context, animeID string, has bool) error {
    atomic.AddInt32(&f.hasEnglishCalls, 1)
    f.hasEnglishVal = has
    return nil
}
```

**Key conventions:**
1. **Plain `struct` + interface impl**, NOT `testify/mock`. Confirmed by RESEARCH.md A8: handwritten struct fakes pattern dominates in `services/catalog/internal/service/`.
2. **`sync/atomic` for call counters** — required for concurrent tests (aggregator tests run resolvers in parallel).
3. **"Reply for the next call" config field** — fakes carry state for next-call behavior (see `replyStatus`, `replyBody`, `replyErr` lines 60-63).
4. **Constructor helper at end of fakes block** (line 97-99):
   ```go
   func newScraperOps(repo animeFetcher, scr scraperForwarder) *scraperOps {
       return &scraperOps{animeRepo: repo, scraperClient: scr}
   }
   ```

**Test naming convention** (`scraper_test.go` line 104):
```go
func TestCatalogService_GetScraperEpisodes_HappyPath(t *testing.T) { ... }
```
Pattern: `Test<Subject>_<Method>_<Scenario>`. RESEARCH.md §"Phase Requirements → Test Map" already gives test names following this convention (e.g. `TestSpotlightHandler_Get_Envelope`).

---

### `services/catalog/cmd/catalog-api/main.go` (DI wiring — MODIFIED)

**Analog within same file:** `subsAggregator` wiring at lines 180-188 — same shape (construct deps → aggregator → handler):
```go
// Workstream raw-jp, Phase 02 — multi-provider subtitle aggregator.
jimakuClient := jimaku.NewClient(cfg.Jimaku.APIKey)
openSubsClient := opensubtitles.NewClient(opensubtitles.Config{
    APIKey:    cfg.OpenSubtitles.APIKey,
    UserAgent: cfg.OpenSubtitles.UserAgent,
    Timeout:   cfg.OpenSubtitles.Timeout,
})
idMapClient := idmapping.NewClient()
subsAggregator := service.NewSubsAggregator(jimakuClient, openSubsClient, idMapClient, animeRepo, redisCache, log)
subtitlesHandler := handler.NewSubtitlesHandler(subsAggregator, log)
```

**Router signature extension** — current call at line 194:
```go
router := transport.NewRouter(catalogHandler, adminHandler, newsHandler, collectionHandler, skipTimesHandler, rawHandler, subtitlesHandler, internalCacheHandler, cfg, log, metricsCollector)
```
Add `spotlightHandler` after `internalCacheHandler` (alphabetic position doesn't matter; positional). RESEARCH.md Example 3 (lines 869-895) shows the full wiring block.

---

### `services/catalog/internal/transport/router.go` (chi route registration — MODIFIED)

**Analog:** Line 62 `r.Get("/anime/news", newsHandler.GetNews)` — a public sibling route under `/api` with no auth middleware.

**Excerpt** (lines 60-65):
```go
// API routes
r.Route("/api", func(r chi.Router) {
    // News endpoint (before /anime route to avoid wildcard conflict)
    r.Get("/anime/news", newsHandler.GetNews)

    // Public catalog routes
    r.Route("/anime", func(r chi.Router) {
```

**Spotlight addition:** Add ONE line near the top of the `/api` Route block, alongside `/anime/news`:
```go
// Hero spotlight aggregator (workstream hero-spotlight, v1.0 Phase 1).
// Public — no auth. Feature-flag gated inside the handler; returns 404
// when SPOTLIGHT_ENABLED=false.
r.Get("/home/spotlight", spotlightHandler.Get)
```

NewRouter function signature extension: add `spotlightHandler *handler.SpotlightHandler` after `internalCacheHandler *handler.InternalCacheHandler` (lines 16-27).

---

### `services/gateway/internal/transport/router.go` (gateway proxy entry — MODIFIED)

**Analog:** Line 218 `r.HandleFunc("/skip-times/*", proxyHandler.ProxyToCatalog)` — a public, single-line catalog proxy entry inside the `/api` Route block.

**Excerpt** (lines 204-222):
```go
// Catalog service routes (public)
r.HandleFunc("/anime", proxyHandler.ProxyToCatalog)
r.HandleFunc("/anime/*", proxyHandler.ProxyToCatalog)
r.HandleFunc("/genres", proxyHandler.ProxyToCatalog)
r.HandleFunc("/kodik/*", proxyHandler.ProxyToCatalog)
r.HandleFunc("/animelib/*", proxyHandler.ProxyToCatalog)
// Phase 18 (UX-34) — Skip-Intro / Skip-Outro CTA timestamps.
r.HandleFunc("/skip-times/*", proxyHandler.ProxyToCatalog)
// Phase 17 (UX-33) — public editorial collections.
r.HandleFunc("/collections", proxyHandler.ProxyToCatalog)
r.HandleFunc("/collections/*", proxyHandler.ProxyToCatalog)
```

**Spotlight addition** (RESEARCH.md Example 4 lines 900-906):
```go
// Hero spotlight (workstream hero-spotlight, v1.0 Phase 1). Public, no JWT.
// Must be registered BEFORE the /anime/* catch-all to follow the
// "specific-before-general" precedent (see /anime/ratings/batch at line 176).
r.HandleFunc("/home/spotlight", proxyHandler.ProxyToCatalog)
```

**Critical ordering:** Place this line INSIDE the `r.Route("/api", func(r chi.Router) { ... })` block, alongside the other catalog public passthroughs (e.g. directly after `r.HandleFunc("/skip-times/*", ...)` line 218 or before/after `/collections`). NOT inside any `r.Group` that uses `JWTValidationMiddleware`.

---

### `services/catalog/internal/config/config.go` (config — MODIFIED)

**Analog:** existing `Telegram TelegramConfig` field (line 24) + `getEnvDuration` helper (lines 221-228).

**Existing helper pattern** (lines 205-228):
```go
func getEnv(key, defaultVal string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
    if val := os.Getenv(key); val != "" {
        if i, err := strconv.Atoi(val); err == nil {
            return i
        }
    }
    return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
    if val := os.Getenv(key); val != "" {
        if d, err := time.ParseDuration(val); err == nil {
            return d
        }
    }
    return defaultVal
}
```

**Additions:**
1. Add `SpotlightEnabled bool` field directly to `Config` struct (line 15-36) — NOT a nested config struct since there's only one field. Pattern: leave it ungrouped if a single bool:
   ```go
   type Config struct {
       ...
       Library LibraryConfig
       // Workstream hero-spotlight, v1.0 Phase 1 — gates GET /api/home/spotlight.
       // When false the handler returns bare 404 (frontend hides the block).
       SpotlightEnabled bool
   }
   ```
2. Add `getEnvBool` helper at the bottom alongside the existing helpers:
   ```go
   func getEnvBool(key string, defaultVal bool) bool {
       if val := os.Getenv(key); val != "" {
           if b, err := strconv.ParseBool(val); err == nil {
               return b
           }
       }
       return defaultVal
   }
   ```
3. Wire into `Load()` (line 188 — after the `Library` block):
   ```go
   SpotlightEnabled: getEnvBool("SPOTLIGHT_ENABLED", true),
   ```

---

## Shared Patterns

### Cache pattern (Get-then-Set vs GetOrSet)

**Sources:**
- `libs/cache/cache.go:151-174` — `GetOrSet` implementation (Get → on ErrNotFound run fn → Set → marshal-roundtrip into dest)
- `libs/cache/cache.go:55-78` — `Get` returns `cache.ErrNotFound` sentinel on miss, wrapped error on Redis hard-down
- `libs/cache/cache.go:209` — `var ErrNotFound = fmt.Errorf("cache: key not found")`

**Apply to:** All 4 resolvers + snapshot fallback.

**Pattern decision per RESEARCH.md Open Question 1:** Use **manual `Get` + `Set` (NOT GetOrSet)** because `GetOrSet` writes `nil`/zero values, baking a 24h "no data" cache when resolver returns `(nil, nil)` for ineligible. The manual pattern:

```go
var data CardSpecificData
err := r.cache.Get(ctx, key, &data)
if err == nil {
    // Hit
    return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
if !errors.Is(err, cache.ErrNotFound) {
    // Hard error — log and continue without cache (don't fail the resolver)
    r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "error", err)
}

// Miss path — compute
data, err = r.compute(ctx)
if err != nil {
    return nil, err
}
if isEmpty(data) {
    return nil, nil  // eligible=false — DO NOT write to cache
}

// Only cache non-empty result
if err := r.cache.Set(ctx, key, data, 24*time.Hour); err != nil {
    r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "error", err)
}
return &spotlight.Card{Type: r.Type(), Data: data}, nil
```

### Logger pattern

**Source:** `libs/logger/logger.go` — `*logger.Logger` with `Infow`, `Errorw`, `Warnw`, `Debugw` (zap.SugaredLogger underneath).

**Apply to:** All new files. Field naming convention: `Infow("event.name", "key1", value1, "key2", value2)`.

**Examples from existing code:**
- `news.go:42` — `h.log.Infow("fetching news from telegram channel")` (no fields)
- `news.go:46` — `h.log.Errorw("failed to fetch telegram news", "error", err)`
- `subs_aggregator.go:145` — `s.log.Warnw("subs aggregator: provider failed", "provider", r.name, "anime_id", animeID, "episode", episode, "error", r.err)`

**Spotlight events** (per CONTEXT.md §Logging):
- `Infow("spotlight.request", "user", "anon")` — at handler entry
- `Infow("spotlight.aggregated", "cards_returned", N, "ms_total", N)` — at handler exit
- `Errorw("spotlight.card_failed", "type", cardType, "error", err)` — per dropped card in aggregator
- `Warnw("spotlight.snapshot_load_failed", "error", err)` — on Redis non-miss error during fallback
- `Warnw("spotlight.snapshot_save_failed", "error", err)` — on detached snapshot write error

### Error handling pattern

**Source:** `libs/errors/errors.go` — `AppError` with `Code`, `Message`, `StatusCode`, `Details`.

**Apply to:** Limited — Phase 1 handler returns 200 on partial success and bare 404 on feature-flag-disabled. Only catastrophic-500 path uses `errors.Internal(...)`. The aggregator's per-card errors are LOGGED, not propagated.

**Web client (`web_client.go`) error wrapping pattern:**
```go
return nil, fmt.Errorf("web client: fetch changelog: %w", err)
```
Mirrors telegram client's `fmt.Errorf("telegram: failed to fetch channel: %w", err)` (telegram/client.go:64). `%w` verb so callers can `errors.Is/As` if needed.

### HTTP response pattern (DIVERGENCE from `httputil.OK`)

**Source:** `libs/httputil/response.go:128-130` — `httputil.OK(w, data)` wraps in `{success:true, data: ...}` envelope.

**Apply to:** NOT directly applicable to spotlight. The spotlight design doc §4.1 specifies a bare `{cards: [...], generated_at: "..."}` envelope without the `success/data` wrapper. Spotlight handler MUST write JSON directly via `json.NewEncoder(w).Encode(resp)` with `w.Header().Set("Content-Type", "application/json")` + `w.WriteHeader(http.StatusOK)`.

This is a deliberate divergence — document in a code comment so future maintainers don't "fix" it to use `httputil.OK`.

### Repository call pattern

**Source:** `services/catalog/internal/repo/anime.go:74-159` — `AnimeRepository.Search(ctx, filters)` returns `([]*domain.Anime, int64, error)`.

**Apply to:** `anime_of_day.go` and `random_tail.go` resolvers.

**Filter struct usage** (`anime.go:75` uses `r.db.WithContext(ctx)` — ctx-aware GORM):
```go
animes, _, err := r.repo.Search(ctx, domain.SearchFilters{
    Sort:     "score",
    Order:    "desc",
    ScoreMin: &sm,  // pointer because optional
    Page:     1,
    PageSize: 200,
})
```

**Documented gotcha:** `sort_priority DESC` is hard-coded as the primary sort axis (line 139). Per RESEARCH.md Pitfall 3, accept the shift in `random_tail`; pinned anime never appear in discovery.

---

## No Analog Found

None — every new file has a clear analog in the codebase. The 4 most load-bearing analogs are:

| New File | Analog | Confidence |
|----------|--------|------------|
| `handler/spotlight.go` | `handler/news.go` | HIGH — same chi GET + cache + log pattern, only the response envelope diverges |
| `service/spotlight/aggregator.go` | `service/subs_aggregator.go` lines 109-156 | HIGH — concurrency model is identical (WaitGroup + buffered chan + drop on error) |
| `service/spotlight/client/web_client.go` | `parser/telegram/client.go` lines 33-77 | HIGH — same http.Client constructor + http.NewRequestWithContext + decoder shape |
| `service/spotlight/*_test.go` | `service/scraper_test.go` lines 15-100 | HIGH — handwritten struct fakes pattern is the verified project convention |

---

## Metadata

**Analog search scope:**
- `services/catalog/internal/handler/` (read `news.go` in full)
- `services/catalog/internal/service/` (read `subs_aggregator.go` in full + `scraper_test.go` lines 1-120)
- `services/catalog/internal/parser/telegram/` (read `client.go` in full)
- `services/catalog/internal/config/` (read `config.go` in full)
- `services/catalog/internal/transport/` (read `router.go` in full)
- `services/catalog/internal/repo/` (read `anime.go` lines 70-170)
- `services/catalog/cmd/catalog-api/main.go` (read lines 1-227)
- `services/gateway/internal/transport/router.go` (read in full)
- `libs/cache/cache.go` (read in full — confirmed `Cache` interface, `ErrNotFound` sentinel)
- `libs/httputil/response.go` (read in full — confirmed `JSON/OK/Error` envelope shape)

**Files scanned:** 10
**Pattern extraction date:** 2026-05-21
