# Phase 17: Observability — Pattern Map

**Mapped:** 2026-05-12
**Files analyzed:** 15 (12 backend Go + 3 infra/config)
**Analogs found:** 15 / 15 (one is "self-modify" where the analog is the current file itself)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `services/scraper/internal/health/probe.go` (NEW) | probe runner goroutine | ticker → provider methods → cache + gauges | `services/catalog/internal/service/health_checker.go` | exact |
| `services/scraper/internal/health/golden.go` (NEW) | static config table | const slice → random select | inline const tables in `services/catalog/internal/service/health_checker.go:18-23` | role-match |
| `services/scraper/internal/health/cache.go` (NEW) | in-memory cache | probe write → RWMutex map → orchestrator read | `services/scraper/internal/service/orchestrator.go:271-287` HealthSnapshot locking | exact (locking discipline) |
| `services/scraper/internal/health/window.go` (NEW) | sliding-window counter | failure event → ring buffer → up/down flip | no in-repo analog (greenfield) | none — use research §Pattern 2 |
| `services/scraper/internal/health/stage.go` (NEW) | const enumeration | n/a | `services/scraper/internal/handler/scraper.go:52-59` errorCode block | role-match |
| `services/scraper/internal/domain/provider.go` (MODIFY) | DTO extension | n/a | itself, lines 96-109 | self |
| `services/scraper/internal/service/orchestrator.go` (MODIFY) | failover loop wiring | request → cache check → provider call | itself, lines 152-199 `runFailover` | self |
| `services/scraper/internal/handler/scraper.go` (MODIFY) | admin handler | request → orchestrator + cache → JSON | itself, lines 206-209 `GetHealth` | self |
| `services/scraper/internal/transport/router.go` (MODIFY) | route registration | chi.Router | itself, lines 55-60 | self |
| `services/scraper/cmd/scraper-api/main.go` (MODIFY) | boot wiring | construct → goroutine spawn → ListenAndServe | itself + `services/catalog/cmd/catalog-api/main.go:115-126` | exact |
| `libs/metrics/provider.go` (NEW) | gauge/counter definitions | promauto.NewGaugeVec | `libs/metrics/player_health.go` | exact |
| `services/gateway/internal/transport/router.go` (MODIFY) | route + auth middleware | request → JWT+Admin → proxy | itself, lines 143-148 `/admin/*` group | self |
| `services/gateway/internal/config/config.go` (MODIFY) | env-driven config | os.Getenv → struct field | itself, lines 33-50 `ServiceURLs` | self |
| `services/gateway/internal/handler/proxy.go` (MODIFY) | proxy shim method | request → ProxyService.Forward | itself, lines 24-67 (ProxyToXxx pattern) | self |
| `services/gateway/internal/service/proxy.go` (MODIFY) | service URL lookup + path rewrite | switch on service name | itself, lines 49-67 (rewrite) + 97-122 (URL) | self |
| `docker/prometheus/prometheus.yml` (MODIFY) | scrape job | n/a | itself, lines 5-51 existing jobs | self |
| `docker/grafana/dashboards/scraper-health.json` (NEW) | dashboard | promQL → stat panels | `docker/grafana/dashboards/player-health.json` | exact |
| `docker/grafana/provisioning/alerting/rules.yml` (MODIFY) | alert rule | promQL → reduce → threshold → for | itself, lines 485-529 `player-unavailable` | exact |
| `services/scraper/internal/health/probe_test.go` (NEW) | unit test | clock-inject → fake provider → assert metric | `services/scraper/internal/service/orchestrator_test.go:18-69` fakeProvider | exact |
| `services/scraper/internal/health/cache_test.go` (NEW) | unit test | RWMutex concurrency | `services/scraper/internal/service/orchestrator_test.go` patterns | role-match |
| `services/scraper/internal/health/testutil_provider.go` (NEW) | test fake | satisfies domain.Provider | `services/scraper/internal/service/orchestrator_test.go:18-59` fakeProvider | exact (reuse the existing struct verbatim) |

---

## Pattern Assignments

### `services/scraper/internal/health/probe.go` (NEW — probe runner goroutine)

**Role:** Long-lived goroutine, one per provider, ticking on a 15-min cadence with ±20% jitter. Per tick: pick random anime from golden pool, run FindID → ListEpisodes → ListServers → GetStream → first-segment fetch, record per-stage success/failure into `window` + `cache` + `provider_health_up` gauge.

**Data flow:** `ctx + Provider + AnimeRef pool → per-stage call sequence → (window.recordSuccess|recordFailure) → cache.Update(provider, ProviderHealth) → metrics.ProviderHealthUp.Set(0|1)`

**Analog:** `services/catalog/internal/service/health_checker.go`

**Imports pattern** (lines 1-16 of analog):
```go
package service  // → package health for new file

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/libs/metrics"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animelib"  // → scraper domain
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik"
)
```

**Ticker + Start loop pattern** (analog lines 65-83):
```go
func (h *PlayerHealthChecker) Start(ctx context.Context) {
    h.log.Infow("player health checker started", "interval", h.interval.String())

    // Run immediately on start
    h.checkAll()

    ticker := time.NewTicker(h.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            h.log.Info("player health checker stopped")
            return
        case <-ticker.C:
            h.checkAll()
        }
    }
}
```

**Per-target checker pattern with status-transition logging** (analog lines 92-117):
```go
func (h *PlayerHealthChecker) checkPlayer(name string, check func() error) {
    start := time.Now()
    err := check()
    duration := time.Since(start).Seconds()

    metrics.PlayerHealthCheckDuration.WithLabelValues(name).Observe(duration)
    metrics.PlayerHealthLastCheck.WithLabelValues(name).SetToCurrentTime()

    up := err == nil
    if up {
        metrics.PlayerHealthUp.WithLabelValues(name).Set(1)
    } else {
        metrics.PlayerHealthUp.WithLabelValues(name).Set(0)
    }

    // Log status transitions only (prev != up) — avoids per-tick log spam
    prev, hasPrev := h.prevStatus[name]
    if !hasPrev || prev != up {
        if up {
            h.log.Infow("player is UP", "player", name, "duration_s", fmt.Sprintf("%.2f", duration))
        } else {
            h.log.Warnw("player is DOWN", "player", name, "error", err, "duration_s", fmt.Sprintf("%.2f", duration))
        }
    }
    h.prevStatus[name] = up
}
```

**Multi-stage pipeline pattern** (analog lines 142-184 — full pipeline check):
```go
// Step 1: Search
results, err := h.animelibClient.Search("naruto")
if err != nil { return fmt.Errorf("search failed: %w", err) }
if len(results) == 0 { return fmt.Errorf("search returned 0 results") }

// Step 2: Get episodes
episodes, err := h.animelibClient.GetEpisodes(results[0].ID)
if err != nil { return fmt.Errorf("get episodes failed (anime %d): %w", results[0].ID, err) }
if len(episodes) == 0 { return fmt.Errorf("anime %d has 0 episodes", results[0].ID) }

// Step 3: Get streams …
// Step 4: Verify video sources …
```

**Adaptation notes:**
- Replace `interval` with `nextSleep(rng)` for ±20% jitter (RESEARCH.md §Pattern 1). Use `time.After(nextSleep(...))` inside the select instead of `time.NewTicker` because cadence is non-uniform.
- The catalog analog runs ONE goroutine for ALL players in serial; per RESEARCH discretion bullet, scraper should spawn ONE goroutine PER provider (so a slow provider can't starve others). Mirrors the "for _, p := range orchestrator.RegisteredProviders() { go runner.Start(rootCtx) }" pattern in RESEARCH "Wiring into main.go".
- Add `defer recover()` wrapper that re-spawns the goroutine on panic (RESEARCH Pitfall P-07 + Pattern 1 example). Catalog analog does NOT do this — Phase 17 explicitly should.
- Replace `prevStatus map[string]bool` with the `window` ring buffer (RESEARCH §Pattern 2) since the threshold is "3 failures in 15 min", not "first failure flips down". Gauge follows window, not raw stage result.
- Inject `now func() time.Time` + `rng *rand.Rand` for deterministic tests (RESEARCH Pitfall P-03 + P-09). Catalog analog uses bare `time.Now()` — that's the regression to avoid.
- The "first tick after randomized initial delay (0 to interval/4)" pattern (RESEARCH Pitfall P-06) replaces the catalog's "run immediately on start" — cookie-jar warmup race.

**Gotchas:**
- Must call `metrics.ProviderProbeLastTick.WithLabelValues(name).Set(float64(now.Unix()))` on every tick so the dead-goroutine alarm (Pitfall P-07) fires. Catalog analog has this via `PlayerHealthLastCheck`.
- `stream_segment` stage requires fetching the first HLS segment URL from the just-retrieved Stream. Reuse the provider's BaseHTTPClient (which already has DDoS-Guard cookie jar wired) — do NOT construct a fresh `http.Client` like the catalog analog's `h.httpClient`.
- Truncate `err.Error()` to 256 chars BEFORE storing in `StageStatus.LastErr` (Pitfall P-05). The analog stores untruncated errors only in transient log lines, which is fine — but admin endpoint exposure makes truncation mandatory here.

---

### `services/scraper/internal/health/golden.go` (NEW — static golden anime pool)

**Role:** Compile-time constant slice of 5-10 `domain.AnimeRef` entries. Provides `Pick(rng)` for random selection per probe tick. No DB, no admin UI (D2).

**Data flow:** `n/a → const []AnimeRef → rng.IntN → AnimeRef`

**Analog:** `services/catalog/internal/service/health_checker.go:18-23` (inline const block — simplest in-repo template).

**Excerpt** (analog lines 18-23):
```go
const (
    playerKodik    = "kodik"
    playerAnimeLib = "animelib"
    playerHiAnime  = "hianime"
    playerConsumet = "consumet"
)
```

**Adaptation notes:**
Use a `var DefaultGoldenPool = []domain.AnimeRef{…}` rather than `const` because `domain.AnimeRef` is a struct (cannot be `const`). Pattern:

```go
var DefaultGoldenPool = []domain.AnimeRef{
    {ShikimoriID: "20",    Title: "Naruto",              Year: 2002}, // 20 = MAL ID for Naruto
    {ShikimoriID: "21",    Title: "One Piece",           Year: 1999},
    {ShikimoriID: "16498", Title: "Attack on Titan",     Year: 2013},
    {ShikimoriID: "38000", Title: "Demon Slayer",        Year: 2019},
    {ShikimoriID: "40748", Title: "Jujutsu Kaisen",      Year: 2020},
}

// Pick returns a random AnimeRef from the pool. Caller injects rng so tests can be deterministic.
func Pick(pool []domain.AnimeRef, rng *rand.Rand) domain.AnimeRef {
    return pool[rng.IntN(len(pool))]
}
```

**Gotchas:**
- MAL IDs above need verification — pull `curl https://api.jikan.moe/v4/anime?q=Naruto&limit=1` style during implementation. Wrong MAL IDs → permanent false-negative on the probe.
- Keep the pool ≤ 10 entries (D2). Larger → move to DB.
- Title + Year are documentation only — only ShikimoriID is consumed downstream by `FindID`.

---

### `services/scraper/internal/health/cache.go` (NEW — in-memory health cache)

**Role:** `map[string]ProviderHealth` guarded by RWMutex. Written by probe (15-min cadence), read by orchestrator's failover loop (every request). Fail-open on stale entries (no entry OR entry > 60s old → treat as healthy).

**Data flow:** `probe → cache.Update(name, ProviderHealth) → orchestrator.runFailover → cache.IsHealthy(name) → bool`

**Analog:** `services/scraper/internal/service/orchestrator.go:271-287` `HealthSnapshot` (carries the canonical locking discipline that REVIEW.md CR-02 enforced).

**Locking discipline excerpt** (analog lines 276-287):
```go
// Locking discipline (REVIEW.md CR-02): snapshot the provider slice under
// the read lock, then RELEASE the lock before invoking p.HealthCheck(ctx).
// Holding the orchestrator's RLock across provider HealthCheck calls would
// block any concurrent Register() (write lock) for the duration of every
// health check — and a future regression where a provider's HealthCheck
// does network I/O would turn into a global service stall.
func (o *Orchestrator) HealthSnapshot(ctx context.Context) map[string]domain.Health {
    o.mu.RLock()
    providers := make([]domain.Provider, len(o.providers))
    copy(providers, o.providers)
    o.mu.RUnlock()

    out := make(map[string]domain.Health, len(providers))
    for _, p := range providers {
        out[p.Name()] = p.HealthCheck(ctx)
    }
    return out
}
```

**Adaptation notes:**
- See RESEARCH.md §Pattern 3 for the full `InMemoryHealthCache` skeleton (the file is greenfield; the locking pattern above is what makes it correct).
- Implement `AdminSnapshot()` returning the full `map[string]ProviderHealth` including `LastErr` strings — used by the admin handler (RESEARCH §"Adding the admin endpoint").
- `IsHealthy(provider string) bool` MUST fail-open: missing entry → true, stale entry (older than 60s by `now()`) → true. This is the project's chosen semantic (RESEARCH Pitfall P-08).
- Inject `now func() time.Time` so cache-staleness tests don't need real time.

**Gotchas:**
- DO NOT call `Provider.HealthCheck()` (or any I/O) while holding the cache's RLock — same anti-pattern that REVIEW.md CR-02 fixed on orchestrator.
- Use `time.Since(t)` for the staleness check (monotonic-safe, Pitfall P-03), not `now().Sub(t)`.

---

### `services/scraper/internal/health/window.go` (NEW — sliding-window failure counter)

**Role:** Per (provider, stage), a bounded slice of failure timestamps. On failure: prune entries older than 15min, append `now`, flip `isDown=true` if ≥ 3 remain. On success: reset to empty + `isDown=false`.

**Data flow:** `(time.Time, success|failure) → []time.Time mutation → bool (isDown)`

**Analog:** None in-repo. Use RESEARCH.md §Pattern 2 as the canonical template (already 40 lines, complete).

**Excerpt** (from RESEARCH §Pattern 2 — verbatim):
```go
const (
    failureThreshold = 3
    failureWindow    = 15 * time.Minute
)

type window struct {
    mu       sync.Mutex
    failures []time.Time
    isDown   bool
}

func (w *window) recordFailure(now time.Time) bool {
    w.mu.Lock()
    defer w.mu.Unlock()
    cutoff := now.Add(-failureWindow)
    pruned := w.failures[:0]
    for _, t := range w.failures {
        if t.After(cutoff) {
            pruned = append(pruned, t)
        }
    }
    w.failures = append(pruned, now)
    if len(w.failures) >= failureThreshold {
        w.isDown = true
    }
    return w.isDown
}

func (w *window) recordSuccess() bool {
    w.mu.Lock()
    defer w.mu.Unlock()
    w.failures = w.failures[:0]
    w.isDown = false
    return false
}
```

**Adaptation notes:**
- Accept `now time.Time` as a parameter rather than calling `time.Now()` internally — caller (`probe.go`) passes `r.now()` so tests can drive the threshold deterministically.
- One `window` instance per (provider, stage) pair. Store as `map[string]*window` keyed by `provider + ":" + stage` inside the probe struct or expose a `WindowSet` helper.

**Gotchas:**
- The `pruned := w.failures[:0]` re-slice is safe because we discard the old slice immediately. If anyone refactors to keep both slices, the aliasing breaks.
- Window state is per-process and lost on restart — by design (D3 implicit). After a restart the probe re-builds state across the first 15min of ticks.

---

### `services/scraper/internal/health/stage.go` (NEW — stage constants)

**Role:** Single source of truth for the five canonical stage names emitted as Prometheus label values. Prevents label-string typos that would fragment the time series.

**Data flow:** n/a

**Analog:** `services/scraper/internal/handler/scraper.go:52-59` — exemplifies the project's "constant block at top of file" idiom for label/code values.

**Excerpt** (analog lines 52-59):
```go
const (
    codeInvalidInput  = "INVALID_INPUT"
    codeNoProviders   = "NO_PROVIDERS"
    codeNotFound      = "NOT_FOUND"
    codeProviderDown  = "PROVIDER_DOWN"
    codeExtractFailed = "EXTRACT_FAILED"
    codeInternal      = "INTERNAL"
)
```

**Adaptation notes:**
```go
const (
    StageSearch        = "search"
    StageEpisodes      = "episodes"
    StageServers       = "servers"
    StageStream        = "stream"
    StageStreamSegment = "stream_segment"
)

var AllStages = []string{StageSearch, StageEpisodes, StageServers, StageStream, StageStreamSegment}
```

**Gotchas:**
- These exact strings appear in PromQL queries (`{stage="stream_segment"}` — Grafana dashboard + alert rule). Changing them breaks dashboards. Document this as a versioned contract in the file comment.

---

### `services/scraper/internal/domain/provider.go` (MODIFY)

**Role:** DTO definition. Extend `Health.Stages` keys to canonical 5-stage names (`search`, `episodes`, `servers`, `stream`, `stream_segment`).

**Data flow:** n/a — pure type definitions

**Analog:** itself

**Current Health excerpt** (lines 96-109):
```go
// StageHealth captures the last success/failure for one stage of a provider's
// scrape pipeline (search, list, servers, sources). Surfaced in /scraper/health.
type StageHealth struct {
    Up      bool      `json:"up"`
    LastOK  time.Time `json:"last_ok"`
    LastErr string    `json:"last_err,omitempty"`
}

// Health is a per-provider health snapshot. Stages are keyed by stage name
// (e.g. "find_id", "list_episodes", "list_servers", "get_stream").
type Health struct {
    Provider string                 `json:"provider"`
    Stages   map[string]StageHealth `json:"stages"`
}
```

**Adaptation notes:**
- Update the comment to reference the canonical stage constants in `internal/health/stage.go`.
- Keys change from `find_id`/`list_episodes`/`list_servers`/`get_stream` to `search`/`episodes`/`servers`/`stream`/`stream_segment` to match the metric label values. AnimePahe provider's `client.go` writes to these keys (search lines 130-186 for stage names today) — that file must be updated as part of the same PR.
- `StageHealth` itself doesn't need new fields — `Up + LastOK + LastErr` is enough. Cache layer wraps it with `LastUpdated` separately.

**Gotchas:**
- The existing test `TestProvider_HealthCheck` in `services/scraper/internal/providers/animepahe/client_test.go:621` asserts on stage names — must be updated when keys change.
- `/scraper/health` JSON response shape changes (key names). The catalog forwarder doesn't care (string passthrough), but external consumers of the public endpoint will. Since the public endpoint is internal-only today, this is acceptable.

---

### `services/scraper/internal/service/orchestrator.go` (MODIFY — skip-unhealthy)

**Role:** Inject `*health.InMemoryHealthCache` into the orchestrator; consult it inside `runFailover` to skip providers whose cache reads DOWN.

**Data flow:** `request → orderedProviders → for p { if !cache.IsHealthy(p.Name()) skip; call(p) }`

**Analog:** itself, lines 152-199 (`runFailover` generic)

**Existing failover loop excerpt** (lines 152-199):
```go
func runFailover[T any](
    ctx context.Context,
    log *logger.Logger,
    providers []domain.Provider,
    call func(p domain.Provider) (T, error),
) (T, error) {
    var zero T
    if len(providers) == 0 {
        return zero, domain.ErrNotFound
    }

    errs := make([]error, 0, len(providers))
    for i, p := range providers {
        if err := ctx.Err(); err != nil {
            return zero, err
        }

        result, err := call(p)
        if err == nil {
            return result, nil
        }

        retry, kind := failoverDecision(err)
        if !retry {
            return zero, err
        }

        next := ""
        if i+1 < len(providers) {
            next = providers[i+1].Name()
        }
        metrics.ParserFallbackTotal.WithLabelValues(p.Name(), next).Inc()
        if log != nil {
            log.Warnw("scraper: provider failover", "from", p.Name(), "to", next, "kind", kind, "error", err.Error())
        }
        errs = append(errs, err)
    }

    return zero, summarizeFailover(errs)
}
```

**Adaptation notes:**
- Add a `cache *health.InMemoryHealthCache` field to `Orchestrator`. Constructor signature changes from `NewOrchestrator(log, registry)` to `NewOrchestrator(log, registry, cache)`. Update the test helper `newTestOrchestrator` (`orchestrator_test.go:61-69`) in the same PR.
- See RESEARCH §Pattern 4 for the exact insertion point (before `call(p)`, with the skip emitting `ParserFallbackTotal{from=p.Name(), to=next}` so dashboards still attribute the skip).
- Nil-cache fallback: when `cache == nil` (zero-provider edge in tests), behave as today (no skipping). Keeps the existing `TestOrchestrator_ZeroProviders_ReturnsErrNotFound` green.

**Gotchas:**
- `runFailover` is generic (`[T any]`) — the cache argument is non-generic, so it threads through unchanged at every call site (`FindID`, `ListEpisodes`, `ListServers`, `GetStream`).
- DO NOT change the public method signatures of `ListEpisodes/ListServers/GetStream/FindID` — the cache lives on the orchestrator struct, threaded into `runFailover` by closure or extra arg.
- REVIEW.md CR-02 already burned the "holding lock across HealthCheck" anti-pattern (`HealthSnapshot`). The cache lookup must NOT call the provider (it reads cached state), so no locking risk — but document it explicitly.

---

### `services/scraper/internal/handler/scraper.go` (MODIFY — admin handler)

**Role:** Add `GetAdminHealth` method exposing per-provider per-stage status + `last_ok` timestamps + truncated `last_err` excerpts. JWT-admin auth is enforced at the gateway, NOT here.

**Data flow:** `r.Context() → cache.AdminSnapshot() → JSON envelope`

**Analog:** itself, lines 206-209 (existing `GetHealth`).

**Existing GetHealth excerpt** (lines 204-209):
```go
// GetHealth handles GET /scraper/health. Returns the orchestrator's live
// HealthSnapshot keyed by provider name.
func (h *ScraperHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
    snap := h.svc.HealthSnapshot(r.Context())
    httputil.OK(w, map[string]any{"providers": snap})
}
```

**Adaptation notes:**
```go
func (h *ScraperHandler) GetAdminHealth(w http.ResponseWriter, r *http.Request) {
    snap := h.svc.HealthSnapshot(r.Context())            // existing public shape
    enriched := h.cache.AdminSnapshot()                   // new — full ProviderHealth incl. LastErr
    httputil.OK(w, map[string]any{
        "providers":    snap,
        "admin":        enriched,
        "generated_at": time.Now().UTC().Format(time.RFC3339),
    })
}
```

- Add `cache *health.InMemoryHealthCache` to `ScraperHandler` struct; thread through `NewScraperHandler(svc, cache, log)`. Update `main.go` accordingly.
- `AdminSnapshot()` returns `map[string]ProviderHealth` (the new struct from cache.go) — DO NOT reuse `domain.Health` directly because it doesn't carry `LastUpdated`.
- Re-use `httputil.OK` envelope (matches existing handler style, lines 207-208). Do NOT mix in the `meta.tried` envelope here — admin endpoint is operator-facing, not user-facing.

**Gotchas:**
- `LastErr` must already be truncated to 256 bytes by the cache layer (Pitfall P-05). Defense-in-depth: re-truncate here as well.
- Strip query strings from any URL-bearing errors before exposing — admin endpoints land in support transcripts.

---

### `services/scraper/internal/transport/router.go` (MODIFY — mount admin route)

**Role:** Register `GET /scraper/health/admin` (D6 + RESEARCH "Adding the admin endpoint"). No auth on the scraper itself — gateway is the auth gate.

**Data flow:** chi.Router → handler binding

**Analog:** itself, lines 55-60.

**Existing route block excerpt** (lines 55-60):
```go
r.Route("/scraper", func(r chi.Router) {
    r.Get("/episodes", scraperHandler.GetEpisodes)
    r.Get("/servers", scraperHandler.GetServers)
    r.Get("/stream", scraperHandler.GetStream)
    r.Get("/health", scraperHandler.GetHealth)
})
```

**Adaptation notes:**
```go
r.Route("/scraper", func(r chi.Router) {
    r.Get("/episodes", scraperHandler.GetEpisodes)
    r.Get("/servers", scraperHandler.GetServers)
    r.Get("/stream", scraperHandler.GetStream)
    r.Get("/health", scraperHandler.GetHealth)
    r.Get("/health/admin", scraperHandler.GetAdminHealth)  // NEW
})
```

**Gotchas:**
- The scraper binds to `127.0.0.1:8088` (per existing config + REVIEW.md WR-02). The admin route is therefore not exposed to the public internet — auth is layered solely at the gateway, which is the intended defense-in-depth model.
- Path must be `/scraper/health/admin` (not `/admin/scraper/health`) — the gateway rewrites the latter to the former (see proxy rewrite below).

---

### `services/scraper/cmd/scraper-api/main.go` (MODIFY — boot wiring)

**Role:** Construct `health.InMemoryHealthCache` + `ProbeRunner` per provider; spawn probe goroutines with shared cancellable context; cancel before `srv.Shutdown` so probes stop cleanly.

**Data flow:** `Load config → build providers → build cache → build orchestrator(cache) → spawn N probe goroutines → ListenAndServe → SIGTERM → cancel(probes) → srv.Shutdown`

**Analog:** itself (lines 102-147) + `services/catalog/cmd/catalog-api/main.go:115-126` for the `context.WithCancel + defer cancel + go svc.Start(ctx)` idiom.

**Catalog analog excerpt** (catalog main.go lines 115-126):
```go
// Start player health checker
healthChecker := service.NewPlayerHealthChecker(
    catalogService.KodikClient(),
    catalogService.AnimeLibClient(),
    cfg.HiAnime.AniwatchAPIURL,
    cfg.Consumet.APIURL,
    cfg.HealthCheck.Interval,
    log,
)
healthCtx, healthCancel := context.WithCancel(context.Background())
defer healthCancel()
go healthChecker.Start(healthCtx)
```

**Scraper main current orchestrator construction** (scraper main.go lines 101-108):
```go
// Build the orchestrator and register the provider before HTTP starts.
orchestrator := service.NewOrchestrator(log, registry)
orchestrator.Register(animePaheProvider)
log.Infow("registered provider", "name", animePaheProvider.Name())

// HTTP handler + router wiring.
scraperHandler := handler.NewScraperHandler(orchestrator, log)
router := transport.NewRouter(scraperHandler, cfg, log, metricsCollector)
```

**Adaptation notes:**
```go
// Build cache + orchestrator
cache := health.NewInMemoryHealthCache()
orchestrator := service.NewOrchestrator(log, registry, cache)  // signature change
orchestrator.Register(animePaheProvider)

// HTTP handler now takes the cache (for admin endpoint)
scraperHandler := handler.NewScraperHandler(orchestrator, cache, log)
router := transport.NewRouter(scraperHandler, cfg, log, metricsCollector)

// Start probes — one goroutine per registered provider, AFTER all providers register
// but BEFORE ListenAndServe (so the probe path runs in the same process as the user path).
probeCtx, probeCancel := context.WithCancel(context.Background())
defer probeCancel()
for _, p := range orchestrator.RegisteredProviders() {  // requires new exposed method
    runner := health.NewProbeRunner(p, health.DefaultGoldenPool, cache, log,
        health.WithRNG(rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))),
        health.WithNow(time.Now),
    )
    go runner.Start(probeCtx)
}
```

- The order is critical: `Register(provider) → spawn probe → ListenAndServe`. RESEARCH Pitfall P-06 (probe runs BEFORE cookie-jar warmup) is mitigated by `nextSleep`-based initial delay in the probe runner itself, not by ordering main.go.
- On SIGTERM: call `probeCancel()` FIRST so probes stop emitting metrics, then `srv.Shutdown(ctx)`. Use the existing `quit` channel handler at lines 133-146 as the unmodified scaffold.

**Gotchas:**
- `orchestrator.RegisteredProviders()` doesn't exist yet — add it as a thin accessor (snapshot under RLock, like `OrderedProviderNames`).
- The `len(orchestrator.HealthSnapshot(context.Background()))` call at line 123 (existing log line) still works because `HealthSnapshot` is unchanged.
- Phase 18+ providers register the SAME way — the for-loop pattern scales linearly without code changes.

---

### `libs/metrics/provider.go` (NEW — gauge family + zero-match counter)

**Role:** Single source of truth for the three new Prometheus collectors: `provider_health_up`, `provider_probe_last_tick_timestamp`, `parser_zero_match_total`.

**Data flow:** `promauto.NewXxxVec → registered against default registry → exposed via libs/metrics.Handler() → scraped by Prometheus`

**Analog:** `libs/metrics/player_health.go` (verbatim — same shape, different name).

**Full analog source** (`libs/metrics/player_health.go` — entire 36-line file):
```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // PlayerHealthUp indicates whether a player/parser is reachable (1=up, 0=down).
    PlayerHealthUp = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "player_health_up",
            Help: "Whether a player source is reachable (1=up, 0=down)",
        },
        []string{"player"},
    )

    PlayerHealthCheckDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "player_health_check_duration_seconds",
            Help:    "Player health check duration in seconds",
            Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
        },
        []string{"player"},
    )

    PlayerHealthLastCheck = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "player_health_last_check_timestamp",
            Help: "Unix timestamp of last player health check",
        },
        []string{"player"},
    )
)
```

**Adaptation notes:**
Copy the file verbatim, then:
1. Rename `PlayerHealthUp` → `ProviderHealthUp`, label `player` → both labels `provider` and `stage`.
2. Rename `PlayerHealthLastCheck` → `ProviderProbeLastTick`, label `player` → `provider`.
3. Replace `PlayerHealthCheckDuration` (histogram) with `ParserZeroMatchTotal` (counter, labels `provider, selector`) — see RESEARCH §"Defining the gauge + counter" for full skeleton.

```go
ProviderHealthUp = promauto.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "provider_health_up",
        Help: "Whether a scraper provider stage is up (1=up, 0=down) per the 3-of-15min liveness probe",
    },
    []string{"provider", "stage"},
)

ProviderProbeLastTick = promauto.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "provider_probe_last_tick_timestamp",
        Help: "Unix timestamp of the last completed probe tick per provider",
    },
    []string{"provider"},
)

ParserZeroMatchTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "parser_zero_match_total",
        Help: "Total count of HTML/JSON selector-miss events per (provider, selector)",
    },
    []string{"provider", "selector"},
)
```

**Gotchas:**
- `promauto` registers against the default registry at package init — DO NOT register a fresh `prometheus.Registerer`. The metric is automatically discovered by `libs/metrics.Handler()`.
- `selector` label cardinality: cap by convention (Pitfall P-02). PRs adding a new selector value go through review.
- Don't extend `libs/metrics/parser.go` (RESEARCH D5 leaves it to planner — recommendation is a new file because the gauge family is structurally distinct from request-counters and grows with provider count).

---

### `services/gateway/internal/transport/router.go` (MODIFY — proxy `/api/admin/scraper/*`)

**Role:** Mount `/api/admin/scraper/*` inside the existing JWT+Admin-gated group, routing to the new `ProxyToScraper` handler.

**Data flow:** `request → JWTValidationMiddleware → AdminRoleMiddleware → ProxyToScraper → scraper service`

**Analog:** itself, lines 143-148 (existing `/admin/*` → catalog group).

**Existing pattern excerpt** (lines 143-148):
```go
// Admin routes (protected, proxied to catalog)
r.Group(func(r chi.Router) {
    r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
    r.Use(AdminRoleMiddleware)
    r.HandleFunc("/admin/*", proxyHandler.ProxyToCatalog)
})
```

**Adaptation notes:**
Critical ordering: `/admin/scraper/*` MUST be registered BEFORE `/admin/*` (chi's longest-prefix match — same gotcha as `/users/recs` before `/users/*` at lines 173-177).
```go
// Admin scraper routes (protected) — MUST come before /admin/* catalog catch-all
r.Group(func(r chi.Router) {
    r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
    r.Use(AdminRoleMiddleware)
    r.HandleFunc("/admin/scraper/*", proxyHandler.ProxyToScraper)
})

// Existing — must stay AFTER the more-specific group above
r.Group(func(r chi.Router) {
    r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
    r.Use(AdminRoleMiddleware)
    r.HandleFunc("/admin/*", proxyHandler.ProxyToCatalog)
})
```

**Gotchas:**
- The `/api/admin/recs/*` group at lines 183-187 already demonstrates this ordering (specific-before-general). Follow that template exactly.
- `defense-in-depth` comment from line 184-185 explains the project's stance — replicate it for the scraper admin route.

---

### `services/gateway/internal/config/config.go` (MODIFY — `ScraperService` field)

**Role:** Add `ScraperService string` to `ServiceURLs`; populate from env in `Load()`.

**Data flow:** `env "SCRAPER_SERVICE_URL" → struct field → ProxyService.getServiceURL`

**Analog:** itself, lines 33-50 (`ServiceURLs`) and 73-90 (`Load`).

**Existing pattern excerpt** (lines 33-50):
```go
type ServiceURLs struct {
    AuthService      string
    CatalogService   string
    PlayerService    string
    RoomsService     string
    StreamingService string
    ThemesService    string
    WebService       string
    GrafanaService    string
    PrometheusService string
    LokiService       string
    SchedulerService string
    RedisAddr        string
    PostgresAddr     string
    NatsAddr         string
}
```

**Existing Load() pattern** (lines 73-90):
```go
Services: ServiceURLs{
    AuthService:      getEnv("AUTH_SERVICE_URL", "http://auth:8080"),
    CatalogService:   getEnv("CATALOG_SERVICE_URL", "http://catalog:8081"),
    PlayerService:    getEnv("PLAYER_SERVICE_URL", "http://player:8083"),
    // ...
},
```

**Adaptation notes:**
1. Add `ScraperService string` to the struct.
2. Add `ScraperService: getEnv("SCRAPER_SERVICE_URL", "http://scraper:8088"),` in `Load()`. Port 8088 matches scraper's existing config (see scraper `cmd/scraper-api/main.go` log line at 121).

**Gotchas:**
- Update `docker/.env` and any kustomize manifests to ship the env var to the gateway container, OR rely on the default `http://scraper:8088` which already resolves via docker-compose service discovery (the safest default — no env required).

---

### `services/gateway/internal/handler/proxy.go` (MODIFY — `ProxyToScraper` helper)

**Role:** Thin wrapper that calls `h.proxy(w, r, "scraper")`.

**Data flow:** Identical to existing ProxyToXxx methods.

**Analog:** itself, lines 24-67.

**Existing pattern excerpt** (lines 24-32):
```go
// ProxyToAuth proxies requests to auth service
func (h *ProxyHandler) ProxyToAuth(w http.ResponseWriter, r *http.Request) {
    h.proxy(w, r, "auth")
}

// ProxyToCatalog proxies requests to catalog service
func (h *ProxyHandler) ProxyToCatalog(w http.ResponseWriter, r *http.Request) {
    h.proxy(w, r, "catalog")
}
```

**Adaptation notes:**
```go
// ProxyToScraper proxies requests to scraper service
func (h *ProxyHandler) ProxyToScraper(w http.ResponseWriter, r *http.Request) {
    h.proxy(w, r, "scraper")
}
```

Drop it in the same block (near alphabetical order). One-liner — no surprises.

---

### `services/gateway/internal/service/proxy.go` (MODIFY — `case "scraper"` + path rewrite)

**Role:** Add `scraper` to `getServiceURL` and rewrite `/api/admin/scraper/{rest}` → `/scraper/{rest}/admin` (or simpler: `/api/admin/scraper/health` → `/scraper/health/admin`).

**Data flow:** `path → switch/rewrite → service URL + path`

**Analog:** itself, lines 49-67 (existing path rewrites) + 97-122 (URL switch).

**Existing rewrite excerpt** (lines 49-67):
```go
switch service {
case "grafana":
    if path == "" || path == "/admin/grafana" {
        path = "/admin/grafana/"
    }
case "prometheus":
    path = strings.TrimPrefix(path, "/admin/prometheus")
    if !strings.HasPrefix(path, "/prometheus") {
        path = "/prometheus" + path
    }
case "loki":
    path = strings.TrimPrefix(path, "/admin/loki")
case "streaming":
    path = strings.Replace(path, "/api/streaming/", "/api/v1/", 1)
}
```

**Existing URL switch excerpt** (lines 97-122):
```go
func (s *ProxyService) getServiceURL(service string) (string, error) {
    switch strings.ToLower(service) {
    case "auth":
        return s.serviceURLs.AuthService, nil
    case "catalog":
        return s.serviceURLs.CatalogService, nil
    // … etc
    default:
        return "", errors.NotFound("service")
    }
}
```

**Adaptation notes:**
1. URL switch — add:
   ```go
   case "scraper":
       return s.serviceURLs.ScraperService, nil
   ```
2. Path rewrite — add:
   ```go
   case "scraper":
       // /api/admin/scraper/health → /scraper/health/admin (mounted at router.go)
       if path == "/api/admin/scraper/health" {
           path = "/scraper/health/admin"
       } else {
           // Generic fallthrough: /api/admin/scraper/X → /scraper/X (no /admin suffix)
           path = strings.Replace(path, "/api/admin/scraper", "/scraper", 1)
       }
   ```

**Gotchas:**
- Pick ONE rewrite scheme and stick to it. Recommended: route the single admin endpoint explicitly (`/api/admin/scraper/health → /scraper/health/admin`) because there's only one admin route in this phase. Future admin routes get added explicitly here.
- Keep the case-insensitive match (`strings.ToLower(service)`) — the existing switch does this.

---

### `docker/prometheus/prometheus.yml` (MODIFY — add scraper scrape job)

**Role:** Make Prometheus scrape the scraper service's `/metrics`. Currently MISSING (Pitfall P-04 BLOCKER).

**Data flow:** Prometheus → `scraper:8088/metrics` → time series

**Analog:** itself, lines 13-51 (existing scrape jobs).

**Existing pattern excerpt** (lines 13-26):
```yaml
- job_name: 'gateway'
  static_configs:
    - targets: ['gateway:8000']
  metrics_path: /metrics

- job_name: 'auth'
  static_configs:
    - targets: ['auth:8080']
  metrics_path: /metrics

- job_name: 'catalog'
  static_configs:
    - targets: ['catalog:8081']
  metrics_path: /metrics
```

**Adaptation notes:**
Append (port 8088 confirmed from scraper main.go log line 121):
```yaml
- job_name: 'scraper'
  static_configs:
    - targets: ['scraper:8088']
  metrics_path: /metrics
```

**Gotchas:**
- After editing, `docker compose restart prometheus` to reload the config (Prometheus does not hot-reload `prometheus.yml` by default in this deployment).
- This single change unblocks every metric in Phase 17. Plan 17-04 (or 17-01 prereq) MUST include this — without it nothing else works.

---

### `docker/grafana/dashboards/scraper-health.json` (NEW — dashboard)

**Role:** Grafana dashboard with per-provider per-stage stat panels (5 stages × N providers grid).

**Data flow:** `PromQL queries → stat panels → Grafana UI`

**Analog:** `docker/grafana/dashboards/player-health.json` (exact same shape, different PromQL).

**Existing panel excerpt** (analog lines 17-56 — one stat panel):
```json
{
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "fieldConfig": {
    "defaults": {
      "color": { "mode": "thresholds" },
      "mappings": [
        { "options": { "0": { "color": "red", "text": "DOWN" }, "1": { "color": "green", "text": "UP" } }, "type": "value" }
      ],
      "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "green", "value": 1 } ] }
    }
  },
  "gridPos": { "h": 4, "w": 6, "x": 0, "y": 1 },
  "id": 2,
  "options": {
    "colorMode": "background", "graphMode": "none", "justifyMode": "center",
    "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
    "textMode": "value_and_name"
  },
  "pluginVersion": "10.3.3",
  "targets": [
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "expr": "player_health_up{player=\"kodik\"}",
      "legendFormat": "Kodik",
      "refId": "A"
    }
  ],
  "title": "Kodik",
  "type": "stat"
}
```

**Adaptation notes:**
- Replace `player_health_up{player="kodik"}` with `provider_health_up{provider="animepahe", stage="search"}` etc. Five stat panels per provider, one per canonical stage.
- Layout: 5 columns × N rows (one row per provider). `gridPos.w = 4`, `x` = stage index × 4, `y` = provider index × 4.
- Add a header row of type `"type": "row"` per provider (analog lines 9-15).
- File name `scraper-health.json` (matching `player-health.json` naming).
- Dashboards are provisioned via the path `/etc/grafana/provisioning/dashboards/...` mount in `docker-compose.yml` — verify the mount picks up the new file (no extra wiring needed if path is right).

**Gotchas:**
- Keep the `${DS_PROMETHEUS}` datasource UID variable so the dashboard is portable across environments.
- The analog uses `pluginVersion: 10.3.3` — match the deployed Grafana version.

---

### `docker/grafana/provisioning/alerting/rules.yml` (MODIFY — add alert rule)

**Role:** Append `provider-health-stream-segment-down` alert rule to the existing rules group.

**Data flow:** `PromQL (provider_health_up{stage="stream_segment"}) → reduce → threshold < 1 → for 15m → severity=critical → Telegram webhook`

**Analog:** itself, lines 485-529 (`player-unavailable` rule).

**Existing rule excerpt** (lines 485-529):
```yaml
- uid: player-unavailable
  title: Player Unavailable
  noDataState: OK
  condition: C
  data:
    - refId: A
      relativeTimeRange:
        from: 600
        to: 0
      datasourceUid: PBFA97CFB590B2093
      model:
        expr: player_health_up{player!~"hianime|consumet"}
        instant: true
        refId: A
    - refId: B
      relativeTimeRange:
        from: 600
        to: 0
      datasourceUid: __expr__
      model:
        refId: B
        type: reduce
        expression: A
        reducer: last
    - refId: C
      relativeTimeRange:
        from: 600
        to: 0
      datasourceUid: __expr__
      model:
        conditions:
          - evaluator:
              params: [1]
              type: lt
            operator:
              type: and
          refId: C
          type: threshold
          expression: B
  for: 30m
  labels:
    severity: critical
  annotations:
    summary: "Player {{ $labels.player }} is DOWN"
    description: "Player {{ $labels.player }} has been unavailable for 30+ minutes. Check external API status."
```

**Adaptation notes:**
Append a new rule with:
- `uid: provider-health-stream-segment-down`
- `title: Provider Stream-Segment Down`
- `expr: provider_health_up{stage="stream_segment"}` (no provider filter — fires per provider label)
- `relativeTimeRange.from: 900` (15 min lookback)
- `for: 15m`
- annotations referencing `{{ $labels.provider }}` and `Check /api/admin/scraper/health`.

See RESEARCH §"Grafana alert rule" for the verbatim YAML.

**Gotchas:**
- `datasourceUid: PBFA97CFB590B2093` is the project's Prometheus datasource UID — DO NOT generate a new one; reuse this exact string.
- The Telegram webhook contact point is already wired (RESEARCH D7) — no contactpoints.yml change needed; just `severity: critical` label binds to existing routing.

---

### `services/scraper/internal/health/probe_test.go` (NEW)

**Role:** Unit test the probe runner with a clock-injected fake provider that returns canned errors to drive the 3-of-15min threshold.

**Data flow:** `setup fakeProvider with scripted errors → tick probe with virtual clock → assert metric value + cache state`

**Analog:** `services/scraper/internal/service/orchestrator_test.go:18-100` (the `fakeProvider` pattern + table-driven orchestrator tests).

**Existing fake-provider pattern excerpt** (lines 18-59):
```go
type fakeProvider struct {
    nameVal          string
    findIDFn         func(ctx context.Context, ref domain.AnimeRef) (string, error)
    listEpisodesFn   func(ctx context.Context, providerID string) ([]domain.Episode, error)
    listServersFn    func(ctx context.Context, providerID, episodeID string) ([]domain.Server, error)
    getStreamFn      func(ctx context.Context, providerID, episodeID, serverID string, cat domain.Category) (*domain.Stream, error)
    healthCheckFn    func(ctx context.Context) domain.Health
    listEpisodeCalls int32
}

func (f *fakeProvider) Name() string { return f.nameVal }
func (f *fakeProvider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
    if f.findIDFn != nil { return f.findIDFn(ctx, ref) }
    return "", nil
}
// … other methods follow the same nil-fn-returns-zero pattern …
```

**Metric assertion excerpt** (lines 1-14 imports — `testutil` is the pattern):
```go
import "github.com/prometheus/client_golang/prometheus/testutil"

// In test body:
got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues("test", "stream_segment"))
if got != 0 {
    t.Errorf("expected gauge 0 after 3 failures, got %v", got)
}
```

**Adaptation notes:**
- Copy `fakeProvider` from `orchestrator_test.go` into a new `testutil_provider.go` file in the health package (rename → `FakeProvider` exported, OR keep it package-internal `fakeProvider` if probe tests share `package health`).
- Test cases:
  1. `TestProbe_ThreeConsecutiveFailures_FlipsGaugeDown` — drive 3 failures within 15 simulated minutes; assert `ProviderHealthUp{stage=…}.Set(0)` was called.
  2. `TestProbe_SuccessAfterFailures_FlipsBackUp` — after threshold, return success; assert gauge flips to 1.
  3. `TestProbe_StaleFailures_DoNotTriggerThreshold` — 3 failures spread over 16 simulated minutes; gauge remains 1.
  4. `TestProbe_LastTickHeartbeat` — assert `ProviderProbeLastTick` gauge advances on each tick.
- Use `clock := &fakeClock{now: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)}` and pass `clock.Now` as the probe's `now func() time.Time`.

**Gotchas:**
- `prometheus/client_golang/prometheus/testutil` is already a dependency (orchestrator_test imports it line 13).
- `testutil.ToFloat64` reads the current value — does NOT reset the metric. Tests must clean up via `metrics.ProviderHealthUp.Reset()` between cases or use unique label values per test.
- `t.Parallel()` is the project's default (orchestrator_test line 75) — only safe when each test uses distinct label values.

---

### `services/scraper/internal/health/cache_test.go` (NEW)

**Role:** Unit test the RWMutex cache: fail-open semantics, stale TTL, concurrent read/write safety.

**Data flow:** `Update(ProviderHealth) → IsHealthy → assert; concurrent goroutines → race detector`

**Analog:** `services/scraper/internal/service/orchestrator_test.go` patterns (no exact match for a cache test, but RWMutex + `t.Parallel()` + table-driven testing style is consistent).

**Adaptation notes:**
- Test the four branches of `IsHealthy`:
  1. No entry → returns `true` (fail-open)
  2. Entry within 60s, `stream_segment.Up = true` → `true`
  3. Entry within 60s, `stream_segment.Up = false` → `false`
  4. Entry older than 60s → `true` (stale = fail-open)
- Run with `go test -race` (CI does this).

**Gotchas:**
- Inject `now func() time.Time` into the cache constructor for test (`NewInMemoryHealthCacheWithNow(fakeNow)`) — same clock-injection discipline as the probe.

---

### `services/scraper/internal/health/testutil_provider.go` (NEW)

**Role:** Exported test fake for `domain.Provider` (so `probe_test.go` and `cache_test.go` can construct one). Identical shape to the existing `fakeProvider` in `orchestrator_test.go`.

**Analog:** `services/scraper/internal/service/orchestrator_test.go:18-59` — copy verbatim, just rename to `FakeProvider` and move to a `_test.go`-tagged file in the health package.

**Adaptation notes:**
Don't duplicate code across `service/orchestrator_test.go` and `health/probe_test.go`. Two reasonable options:
1. Promote the fake to a non-test file under `internal/testharness/` (the directory already exists per scraper `ls`) — accessible from both test packages.
2. Keep two copies (one per test package) — minor duplication, no import-cycle risk.

Recommended option 1 (testharness directory is empty / underused — Phase 17 is a good time to populate it).

**Gotchas:**
- Test fakes that satisfy `domain.Provider` must be kept in sync with the interface. The existing `TestStream_HasNoIframeURL` (provider_test.go:19) and `TestStream_AllowedFields` (line 42) lock the DTO; the fake needs no companion lock since it has no fields beyond the function hooks.

---

## Shared Patterns

### Pattern A: Structured logging via `libs/logger`
**Source:** `services/catalog/internal/service/health_checker.go:111-114`
**Apply to:** probe.go (transition logging), main.go (boot logging), cache.go (debug only)

```go
h.log.Infow("player is UP", "player", name, "duration_s", fmt.Sprintf("%.2f", duration))
h.log.Warnw("player is DOWN", "player", name, "error", err, "duration_s", fmt.Sprintf("%.2f", duration))
```

Idiom: zap-style structured fields (`key, value, key, value, …`). Errors as `"error", err` (not `"error", err.Error()` — let zap format).

### Pattern B: Defer-recover goroutine panic handling
**Source:** RESEARCH §Pattern 1 (no in-repo analog — explicit anti-pattern noted in Pitfall P-07)
**Apply to:** probe.go ONLY

```go
defer func() {
    if rec := recover(); rec != nil {
        r.log.Errorw("probe panicked, restarting", "provider", r.provider.Name(), "panic", rec)
        go r.Start(ctx)  // re-spawn
    }
}()
```

### Pattern C: RWMutex snapshot-then-release
**Source:** `services/scraper/internal/service/orchestrator.go:271-280`
**Apply to:** cache.go (any method that exposes the map), orchestrator.go (already does this — keep it)

```go
o.mu.RLock()
snapshot := make([]T, len(o.things))
copy(snapshot, o.things)
o.mu.RUnlock()
// … now operate on `snapshot` without holding the lock
```

### Pattern D: Constructor with options or explicit deps
**Source:** `services/scraper/internal/providers/animepahe/client.go` `New(Deps{…})` style (referenced indirectly via main.go:89-99)
**Apply to:** probe.go `NewProbeRunner(provider, pool, cache, log, opts…)`

Prefer functional options for the `now func()` and `rng *rand.Rand` injection so production callers stay terse while tests get full control:
```go
type ProbeOption func(*ProbeRunner)
func WithNow(now func() time.Time) ProbeOption { … }
func WithRNG(rng *rand.Rand) ProbeOption { … }
```

### Pattern E: HTTP response envelope `httputil.OK`
**Source:** `services/scraper/internal/handler/scraper.go:208` (`httputil.OK(w, map[string]any{…})`)
**Apply to:** admin handler in scraper.go

```go
httputil.OK(w, map[string]any{"providers": snap, "admin": enriched, "generated_at": ts})
```

No need to mix in the `meta.tried` envelope (lines 225-231) — admin endpoint is operator-facing.

### Pattern F: Metrics observation with `defer`
**Source:** `libs/metrics/parser.go:43-51` (`ObserveParser`)
**Apply to:** any provider method (existing pattern, already in use) AND the probe's per-stage call

```go
defer metrics.ObserveParser("animepahe", "search", time.Now(), &err)
```

For the probe, also defer-record into the window:
```go
err := provider.FindID(ctx, ref)
if err != nil { window.recordFailure(now()) } else { window.recordSuccess() }
```

---

## No Analog Found

| File | Role | Data Flow | Reason | Mitigation |
|------|------|-----------|--------|------------|
| `services/scraper/internal/health/window.go` | sliding-window failure counter | event → ring buffer → bool | Greenfield; project has no "X-of-Y-minutes" semantic anywhere | Use RESEARCH §Pattern 2 verbatim (40-line file with full source) |

---

## Metadata

**Analog search scope:**
- `services/scraper/internal/**`
- `services/catalog/internal/service/health_checker.go`
- `services/gateway/internal/**`
- `services/catalog/cmd/catalog-api/main.go`, `services/scheduler/cmd/scheduler-api/main.go`
- `libs/metrics/**`
- `docker/grafana/**`, `docker/prometheus/**`

**Files scanned:** 18 source files + 3 infra files

**Pattern extraction date:** 2026-05-12

**Key insight:** Phase 17 is plumbing — 4 of 5 patterns have exact in-repo analogs (`player_health.go` for the gauge, `health_checker.go` for the probe, `orchestrator.go` for the cache locking, `player-unavailable` rule for the alert). The only genuinely new code is `window.go` (sliding-window failure counter), which RESEARCH already provides in full. Most planner risk is in (a) ordering of routes in gateway router (specific before general — same gotcha as `/users/recs`), (b) clock injection (catalog analog uses bare `time.Now()` — Phase 17 must not), (c) adding the scraper job to `prometheus.yml` (BLOCKER P-04 — without it everything in this phase is dead weight).
