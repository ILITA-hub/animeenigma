# Phase 17: Observability - Research

**Researched:** 2026-05-12
**Domain:** Synthetic monitoring (in-process probe), Prometheus instrumentation, Grafana dashboarding, admin debug endpoint
**Confidence:** HIGH

## Summary

Phase 17 adds per-provider/per-stage health visibility to the v3.0 scraper service via a 15-min in-process liveness probe, a `provider_health_up{provider,stage}` Prometheus gauge family, an in-memory 60s health cache used by the orchestrator to skip unhealthy providers, a Grafana dashboard + Telegram-routed alert, and a JWT-admin-gated debug endpoint. Almost all of the infrastructure the phase needs already exists in the repo — Prometheus + Grafana + a `maintenance-webhook` → Telegram alert pipeline, a `parser_*` metric family in `libs/metrics/parser.go`, a `domain.Provider.HealthCheck()` hook on every provider, a `domain.Health` / `StageHealth` DTO with the right shape, and a `services/catalog/internal/service/health_checker.go` that is a near-perfect template for the probe runner.

The phase boils down to three structural additions and one wiring exercise: (1) a new `services/scraper/internal/health/` package containing the probe runner, golden pool, sliding-window failure counter, and in-memory cache; (2) two new metric definitions (`provider_health_up` GaugeVec + `parser_zero_match_total` CounterVec) added to `libs/metrics/`; (3) a new admin sub-handler + gateway proxy entry for `/api/admin/scraper/health`; and (4) a Grafana dashboard JSON + alert rule YAML modeled on the existing `player-health.json` + `player-unavailable` rule patterns.

**Primary recommendation:** Mirror the existing `services/catalog/internal/service/health_checker.go` design (sequential check per target, defer-recover wrapper, monotonic ticker, transition logging) but extend it with per-stage error attribution, a `[3]bool` ring-buffer per (provider, stage) for the 3-consecutive-failures-in-15min threshold, and a `domain.AnimeRef` golden-pool rotation. The probe owns the pipeline orchestration (calls Provider.FindID → ListEpisodes → ListServers → GetStream → first HLS segment); providers only need to satisfy the existing interface — no per-provider probe code.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**D1 — Liveness probe lives in the scraper service (not scheduler)**
The probe is invoked inside the `scraper` service (a new goroutine started in `services/scraper/cmd/scraper-api/main.go`), not as a job in the existing `scheduler` service. Rationale: in-process access to the orchestrator + providers + HTTP client + DDoS-Guard cookie jar; the probe path must be identical to the user path.

**D2 — Golden anime pool is a static-config list, randomized per-tick**
The golden pool is a 5-10 entry static list stored in `services/scraper/internal/health/golden.go`. Each probe tick randomly selects 1 anime from the pool. Trade-off: a delisted anime causes a false-negative until manual update — acceptable for v3.0.

**D3 — Health cache is in-memory, owned by the orchestrator**
The 60s health cache lives in-memory inside the orchestrator (a `map[string]providerHealth` guarded by an RWMutex). The probe writes; the failover loop reads. No Redis — health is per-process state, v3.0 ships one scraper container.

**D4 — Probe uses `domain.Provider.HealthCheck` already on the interface (probe-owned pipeline)**
The probe orchestrator calls each provider method individually (`Search`/`ListEpisodes`/`ListServers`/`GetStream`/first-segment fetch), so adding a new provider doesn't require re-implementing the probe. Extending `domain.Health` to carry per-stage status is part of this phase's contract change.

**D5 — Prometheus metric exposure reuses existing patterns**
Metrics are exposed via the existing scraper service `/metrics` endpoint. The new `provider_health_up` gauge family + parser metrics are added to `libs/metrics/parser.go` (or a new `libs/metrics/provider.go` — planner's call). Reuses the same metric names (`parser_requests_total`, etc.) with `provider=animepahe` so dashboards are forward-compatible after Phase 20 cutover.

**D6 — Admin endpoint route goes through the gateway**
`GET /api/admin/scraper/health` is routed by the gateway to the scraper service's `/scraper/health` endpoint (NEW admin variant — the public one already exists). Auth: same JWT bearer admin gate already used by `/api/admin/*` in catalog. A SEPARATE route is preferred over overloading the existing endpoint with `?admin=1`.

**D7 — Grafana alert uses the existing Telegram channel**
The alert pipeline is already wired (Grafana → maintenance-webhook → Telegram bot using `TELEGRAM_ADMIN_CHAT_ID`). This phase only adds: a dashboard panel + an alert rule `provider_health_up{stage="stream_segment"} == 0 for 15m`. No new infrastructure.

**D8 — Live-probe upstream traffic is acceptable cost**
15-min × 5 stages × 1 provider ≈ 20 requests / 15 min × per provider = ~80/hour total. Real upstream traffic is the whole point. Mocked probes would catch only configuration drift, not upstream death.

### Claude's Discretion

- Whether to add a new `libs/metrics/provider.go` file or extend the existing `libs/metrics/parser.go` for the new gauge family.
- The exact internal layout of the new `services/scraper/internal/health/` package (probe runner, golden pool, cache, ring buffer).
- Whether the probe is one goroutine iterating providers or N goroutines (one per provider) — recommendation: ONE goroutine per provider, so a slow provider doesn't starve others.
- The specific 5-10 anime that make up the golden pool (subject to per-provider availability — planner verifies during impl).
- Test-strategy details (clock injection, fake-provider scaffolding) — the existing `fakeProvider` in `services/scraper/internal/service/orchestrator_test.go` is a reusable template.

### Deferred Ideas (OUT OF SCOPE)

- Per-user / per-egress health (client-side or multi-region probes)
- Tracing (OTLP / Jaeger) — metrics + logs only this phase
- Self-healing actions (auto-restart container) — observe-only
- Per-user override of "skip unhealthy" beyond the existing `prefer=` query param
- A second probe cadence (1-min "shallow" + 15-min "deep")
- Loading the golden pool from DB / admin UI
- Adding a second provider (Phase 18)
- Per-user observability dashboards
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SCRAPER-OBS-01 | 15-min liveness probe with golden pool, full pipeline across 5 stages, ±20% jitter | "Probe Runner Architecture" + "Golden Pool" sections below; template = `services/catalog/internal/service/health_checker.go` |
| SCRAPER-OBS-02 | Prometheus `provider_health_up{provider,stage}` gauge family, 5 stages, 3-failures-in-15min flip threshold | "Prometheus Gauge Pattern" + "Sliding Window Failure Counter" sections; template = `libs/metrics/player_health.go` |
| SCRAPER-OBS-03 | Orchestrator skips providers whose 60s in-memory health cache reads 0; auto-rejoin on first success | "In-Memory Health Cache" + "Orchestrator Skip Wiring" sections; locking discipline carries over from the existing `HealthSnapshot` REVIEW.md CR-02 fix |
| SCRAPER-OBS-04 | Grafana dashboard panel + alert rule for `stream_segment == 0 for 15m` → Telegram | "Grafana / Prometheus Deploy Contract" section; template = `docker/grafana/dashboards/player-health.json` + `provisioning/alerting/rules.yml` `player-unavailable` rule |
| SCRAPER-OBS-05 | `GET /api/admin/scraper/health` admin debug endpoint with last_ok + failure context | "Admin Endpoint" section; new scraper handler + gateway proxy entry + ServiceURLs.ScraperService |
| SCRAPER-NF-04 | `parser_requests_total{provider,operation,status}`, `parser_request_duration_seconds`, `parser_fallback_total{from,to}`, `parser_zero_match_total{provider,selector}` emit for the scraper with `{provider}` labels | "Parser Metrics" section; `ObserveParser` already exists, `parser_zero_match_total` does NOT — must be added |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Liveness probe execution | Scraper Service (Go process) | - | Probe needs in-process access to providers, HTTP client, cookie jar (D1) |
| Probe scheduling (15min ± jitter) | Scraper Service (goroutine) | - | Co-located so the probe path == user path |
| `provider_health_up` gauge emission | Scraper Service `/metrics` | - | Standard service-local metric exposition; `libs/metrics` shared collectors |
| Prometheus scrape | Prometheus container | - | Existing `docker/prometheus/prometheus.yml` needs scraper added (it is currently MISSING — see Pitfall §P-04) |
| Alert evaluation | Grafana (alerting) | - | Existing alert engine via provisioning files |
| Alert delivery | Maintenance webhook + Telegram bot | - | Existing pipeline (alertmanager-equivalent is Grafana's built-in alerting + the maintenance receiver) |
| Health cache reads on hot path | Orchestrator (in-memory RWMutex) | - | D3 — per-process, no Redis |
| Admin endpoint auth | Gateway (JWT + admin role) | Scraper (no auth, bound to 127.0.0.1) | Gateway already gates `/admin/*`; defense-in-depth on scraper not needed since it's not exposed |
| Admin endpoint serving | Scraper Service (new `/admin/scraper/health` route OR separate `/scraper/health/admin` route) | - | D6 — bypass catalog, direct gateway → scraper proxy |
| Golden pool storage | Static Go const in `internal/health/golden.go` | - | D2 — no DB |
| Per-provider parser metrics | Provider implementations | - | SCRAPER-NF-04 — each provider method wraps with `defer metrics.ObserveParser(...)` |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/prometheus/client_golang` | v1.19.0 (already in scraper go.mod) | Gauge / Counter / Histogram + promhttp handler | Already used throughout the codebase; `libs/metrics/metrics.go` wires `/metrics` |
| `github.com/go-chi/chi/v5` | v5.0.12 (already in scraper) | Router for new admin sub-route | Existing router in `services/scraper/internal/transport/router.go` |
| `github.com/hashicorp/go-retryablehttp` | v0.7.7 (already in scraper via BaseHTTPClient) | Transport for probe's segment fetch (reuses provider HTTP client) | Phase 15 standard |
| Go stdlib `context` + `time` | go 1.23 | Ticker, context cancellation, monotonic clock for jitter | Stdlib; no external dep |
| Go stdlib `math/rand/v2` | go 1.23 | Jitter calculation (NOT crypto/rand — jitter doesn't need cryptographic guarantees) | go 1.22+ stdlib provides better randomness API than v1 `math/rand` |
| Go stdlib `sync` | go 1.23 | RWMutex on health cache + atomic stage counters | Stdlib |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `libs/metrics` (in-repo) | - | Existing GaugeVec/CounterVec/HistogramVec definitions and `Handler()` for promhttp | All metric emissions |
| `libs/logger` (in-repo) | - | Structured zap-style logging; Sync()/Infow/Warnw/Errorw | Probe lifecycle, transition logging |
| `services/scraper/internal/domain` | - | `Provider` interface, `Health`/`StageHealth` DTOs, `AnimeRef`, error sentinels | Probe target invocation, cache types |
| `services/scraper/internal/service.Orchestrator` | - | Provider registry + existing `HealthSnapshot` fan-out | Probe target enumeration; cache injection point |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| In-process probe goroutine | Probe as a `services/scheduler` job | Rejected by D1 — would need duplicated provider construction or RPC; loses the cookie-jar / HTTP-client identity guarantee |
| In-memory map for cache | Redis cache | Rejected by D3 — health is per-process, no horizontal scaling planned in v3.0 |
| 3-failures sliding window via timestamp slice | Counter + reset on success | Recommendation: `[3]time.Time` ring buffer per (provider, stage) — bounded memory, trivial threshold check (`len(failuresWithin15m) == 3`); reset on success |
| `math/rand` v1 | `crypto/rand` for jitter | Rejected — jitter doesn't need crypto; `math/rand/v2` (go 1.22+) is the modern stdlib API |
| Grafana native Telegram contact point | Existing maintenance-webhook → Telegram bot pipeline | Rejected — pipeline already shipped, swapping it is out-of-scope |
| One goroutine iterating all providers | One goroutine per provider | Recommended: per-provider — a slow/stuck provider can't starve others |

**Installation:**
No new dependencies — everything used is already in `services/scraper/go.mod`.

**Version verification:**
```bash
grep -E "prometheus/client_golang|go-chi/chi" /data/animeenigma/services/scraper/go.mod
# verified 2026-05-12 — both already present at v1.19.0 / v5.0.12
```

## Architecture Patterns

### System Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  Scraper Service (one Go process, port 8088)                                 │
│                                                                              │
│  ┌──────────────────────┐                                                    │
│  │  main.go             │  spawns                                            │
│  │  - Build providers   │ ────────┐                                          │
│  │  - Build orchestrator│         ▼                                          │
│  │  - Build probe       │   ┌──────────────────────────────────────┐         │
│  │  - srv.ListenAndServe│   │  ProbeRunner goroutine PER provider  │         │
│  └──────────────────────┘   │                                      │         │
│           │                 │  Loop {                              │         │
│           │ HTTP req        │    sleep( 15min ± 20% jitter )       │         │
│           ▼                 │    pick random AnimeRef from pool    │         │
│  ┌──────────────────────┐   │    s1 = provider.FindID(ctx, ref)    │  ┌──────┴────────┐
│  │ Chi Router           │   │    record(search,   s1.err)          │  │ Upstream       │
│  │ /scraper/*           │   │    s2 = provider.ListEpisodes(...)   │──┤ AnimePahe API  │
│  │ /admin/scraper/health│   │    record(episodes, s2.err)          │  │ + 9anime (P18) │
│  │ /metrics             │   │    s3 = provider.ListServers(...)    │  │ + AnimeKai(19) │
│  └──────────┬───────────┘   │    record(servers,  s3.err)          │  └────────────────┘
│             │               │    s4 = provider.GetStream(...)      │         │
│             ▼               │    record(stream,   s4.err)          │         │
│  ┌──────────────────────┐   │    s5 = GET first HLS segment        │         │
│  │ Orchestrator         │   │    record(stream_segment, s5.err)    │         │
│  │ - providers []Provider│ ◄┼─── update cache + gauges             │         │
│  │ - healthCache (NEW)  │   │  }                                   │         │
│  │ - GetHealthy(name)   │   └──────────────────────────────────────┘         │
│  │ - runFailover(...)   │                  │                                  │
│  └──────────┬───────────┘                  │                                  │
│             │ skips providers              ▼                                  │
│             │ where healthCache    ┌──────────────────────┐                  │
│             │ < 60s reads 0        │  libs/metrics        │                  │
│             ▼                      │  ProviderHealthUp    │                  │
│  ┌──────────────────────┐          │   .WithLabelValues(  │                  │
│  │ Provider.FindID      │          │     "animepahe",     │                  │
│  │ Provider.ListEpisodes│          │     "stream_segment" │                  │
│  │ ... (per req)        │          │   ).Set(0|1)         │                  │
│  └──────────────────────┘          └──────────────────────┘                  │
│                                              │                                │
│                                              │ /metrics (promhttp)            │
└──────────────────────────────────────────────┼────────────────────────────────┘
                                               ▼
                                ┌──────────────────────────────┐
                                │  Prometheus container         │
                                │  scrape interval 15s          │
                                │  (scraper job NOT YET in      │
                                │   prometheus.yml — must add!) │
                                └──────────┬───────────────────┘
                                           │
                                           ▼
                                ┌──────────────────────────────┐
                                │  Grafana                      │
                                │  - scraper-health.json        │
                                │    (new dashboard)            │
                                │  - alert: provider_health_up  │
                                │    {stage="stream_segment"}   │
                                │    == 0 for 15m               │
                                └──────────┬───────────────────┘
                                           │ POST /api/grafana-webhook
                                           ▼
                                ┌──────────────────────────────┐
                                │  Maintenance service          │
                                │  (host-gateway:8087)          │
                                │  - parses payload             │
                                │  - sends to Telegram bot      │
                                │    using TELEGRAM_ADMIN_CHAT_ID│
                                └──────────────────────────────┘

User request path (with health cache):
  HTTP /scraper/episodes?mal_id=... → ScraperHandler → Orchestrator.ListEpisodes →
  for each provider in orderedProviders(prefer):
      if !healthCache.IsHealthy(provider.Name()):   ← NEW gate in this phase
          skip
      result, err := provider.ListEpisodes(...)
      ...
```

### Recommended Project Structure
```
services/scraper/internal/
├── health/                         # NEW package — Phase 17
│   ├── probe.go                    # ProbeRunner: ticker loop + golden-pool rotation + stage iteration
│   ├── probe_test.go               # Unit tests with fakeProvider (clock-injected)
│   ├── cache.go                    # InMemoryHealthCache: 60s TTL, RWMutex, IsHealthy/Update
│   ├── cache_test.go
│   ├── window.go                   # 3-of-15min failure ring buffer + flip-threshold logic
│   ├── window_test.go
│   ├── golden.go                   # Static 5-10 entry golden anime pool with per-provider availability
│   └── stage.go                    # Stage constants: search, episodes, servers, stream, stream_segment
├── service/
│   └── orchestrator.go             # MODIFIED — accepts HealthCache, skips unhealthy in runFailover
├── domain/
│   └── provider.go                 # MODIFIED — Health.Stages keys extended to canonical 5 stages
└── handler/
    └── admin.go                    # NEW — admin/scraper/health handler with last_ok + failure context

libs/metrics/
└── provider.go                     # NEW — ProviderHealthUp GaugeVec + ParserZeroMatchTotal CounterVec
                                    # (alternative: extend parser.go — planner choice)

services/gateway/internal/
├── config/config.go                # MODIFIED — add ScraperService URL
├── service/proxy.go                # MODIFIED — add "scraper" case in getServiceURL
├── handler/proxy.go                # MODIFIED — add ProxyToScraper helper
└── transport/router.go             # MODIFIED — route /api/admin/scraper/* → scraper service

docker/grafana/
├── dashboards/
│   └── scraper-health.json         # NEW — per-provider per-stage stat panels (5×N grid)
└── provisioning/alerting/
    └── rules.yml                   # MODIFIED — append provider-health-stream-segment-down rule

docker/prometheus/
└── prometheus.yml                  # MODIFIED — add scraper:8088 scrape job (CURRENTLY MISSING)
```

### Pattern 1: In-Process Probe Goroutine with Jitter + Recovery

**What:** One goroutine per provider, started from `main.go` before `ListenAndServe`. Each goroutine runs a sleep-loop with ±20% jitter, calls `Provider.FindID` through `GetStream` against a rotating golden pool, then optionally fetches the first HLS segment, recording each stage's success/failure into the health cache + gauges. Wraps the per-tick body in `defer recover()` so a provider panic doesn't kill the probe.

**When to use:** Synthetic monitoring where the probe must run in the same process as the user-serving code (cookie jar, HTTP client identity, in-memory state).

**Example:**

```go
// Source: derived from services/catalog/internal/service/health_checker.go (in-repo template)
package health

import (
    "context"
    "math/rand/v2"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/libs/metrics"
    "github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

const (
    probeBaseInterval = 15 * time.Minute
    probeJitterPct    = 0.20  // ±20%
    segmentTimeout    = 10 * time.Second
)

type ProbeRunner struct {
    provider  domain.Provider
    pool      []domain.AnimeRef
    cache     *InMemoryHealthCache
    log       *logger.Logger
    rng       *rand.Rand   // injectable for deterministic tests
    now       func() time.Time
}

// Start runs the probe loop until ctx is cancelled. Recommended invocation:
//   go runner.Start(rootCtx)
func (r *ProbeRunner) Start(ctx context.Context) {
    defer func() {
        if rec := recover(); rec != nil {
            // metrics + log + restart, mirroring crash-proof goroutine pattern
            r.log.Errorw("probe panicked, restarting",
                "provider", r.provider.Name(),
                "panic", rec,
            )
            go r.Start(ctx)
        }
    }()
    r.log.Infow("probe started", "provider", r.provider.Name(), "pool_size", len(r.pool))

    // First tick after a short randomized delay (avoids all probes hitting upstream simultaneously at boot)
    initialDelay := time.Duration(r.rng.Int64N(int64(probeBaseInterval / 4)))
    select {
    case <-ctx.Done():
        return
    case <-time.After(initialDelay):
    }

    for {
        r.runOneTick(ctx)
        sleep := nextSleep(r.rng)
        select {
        case <-ctx.Done():
            r.log.Infow("probe stopped", "provider", r.provider.Name())
            return
        case <-time.After(sleep):
        }
    }
}

func nextSleep(rng *rand.Rand) time.Duration {
    delta := (rng.Float64()*2 - 1) * probeJitterPct  // uniform in [-jitter, +jitter]
    return time.Duration(float64(probeBaseInterval) * (1 + delta))
}
```

### Pattern 2: Sliding-Window Failure Counter (3-of-15min)

**What:** Per (provider, stage), keep a ring buffer of the last N failure timestamps. On each failure, append; the stage flips DOWN when the buffer contains 3 failures all within the last 15 minutes. On any success, reset the buffer.

**When to use:** Whenever the "X consecutive failures within window Y" semantics matter — flapping protection on synthetic monitoring.

**Example:**

```go
// Recommendation — there is no in-repo template for this exact pattern.
package health

import (
    "sync"
    "time"
)

const (
    failureThreshold = 3
    failureWindow    = 15 * time.Minute
)

type window struct {
    mu       sync.Mutex
    failures []time.Time  // bounded at failureThreshold
    isDown   bool
}

// recordFailure returns the new isDown state (true if the gauge should be 0).
func (w *window) recordFailure(now time.Time) bool {
    w.mu.Lock()
    defer w.mu.Unlock()
    // Drop failures older than the window
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
    return false  // up
}
```

### Pattern 3: In-Memory Health Cache (60s read TTL)

**What:** A `map[string]providerHealth` guarded by an RWMutex; written by the probe, read by the orchestrator's failover loop. Reads check the entry's `LastUpdated` timestamp — entries older than 60s are treated as "unknown" and the orchestrator does NOT skip (fail-open for stale cache so a probe outage doesn't blank the service).

**When to use:** Read-heavy concurrent cache where writes are infrequent (every 15min) and reads are hot-path (every request). RWMutex outperforms sync.Map for this access pattern (verified in the existing `Orchestrator.HealthSnapshot` design — REVIEW.md CR-02 fix).

**Example:**

```go
// Recommendation — mirrors HealthSnapshot's locking discipline (services/scraper/internal/service/orchestrator.go:271-287)
package health

import (
    "sync"
    "time"
)

const cacheStaleTTL = 60 * time.Second

type ProviderHealth struct {
    Stages      map[string]StageStatus  // stage -> {up, last_ok, last_err}
    LastUpdated time.Time
}

type StageStatus struct {
    Up       bool
    LastOK   time.Time
    LastErr  string  // truncated to e.g. 256 bytes — see Pitfall §P-05
}

type InMemoryHealthCache struct {
    mu    sync.RWMutex
    state map[string]ProviderHealth
    now   func() time.Time
}

// IsHealthy returns true if the provider's stream_segment stage was UP within
// the last cacheStaleTTL. Stale (no recent probe data) returns true (fail-open).
//
// Read locking discipline: do NOT call provider methods while holding the RLock
// (carry forward from REVIEW.md CR-02 — same anti-pattern would stall the orchestrator).
func (c *InMemoryHealthCache) IsHealthy(provider string) bool {
    c.mu.RLock()
    defer c.mu.RUnlock()
    h, ok := c.state[provider]
    if !ok {
        return true  // unknown = fail-open
    }
    if c.now().Sub(h.LastUpdated) > cacheStaleTTL {
        return true  // stale = fail-open
    }
    // Use stream_segment as the "is this provider usable" oracle (the
    // last+most-comprehensive stage — if it's up, everything before it is up).
    seg, ok := h.Stages["stream_segment"]
    return !ok || seg.Up
}

func (c *InMemoryHealthCache) Update(provider string, h ProviderHealth) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.state[provider] = h
}
```

### Pattern 4: Orchestrator Skip Wiring

**What:** Modify `Orchestrator.orderedProviders` (or `runFailover`) to consult the health cache and skip providers currently flagged DOWN. The skip is silent at INFO log level (would otherwise be spammy at request rate) but emits a counter `provider_skipped_total{provider,reason}` for observability.

**Example:**

```go
// Recommendation — minimal modification to existing runFailover in orchestrator.go:152
func runFailover[T any](
    ctx context.Context,
    log *logger.Logger,
    providers []domain.Provider,
    cache *health.InMemoryHealthCache,  // NEW arg
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

        // NEW: skip unhealthy providers (D3 / SCRAPER-OBS-03)
        if cache != nil && !cache.IsHealthy(p.Name()) {
            // Emit metric + soft log; treat as a skipped retryable failure
            metrics.ParserFallbackTotal.WithLabelValues(p.Name(), nextName(providers, i)).Inc()
            errs = append(errs, fmt.Errorf("provider %s skipped: health gauge 0", p.Name()))
            continue
        }

        result, err := call(p)
        // ... rest unchanged
    }
    return zero, summarizeFailover(errs)
}
```

### Anti-Patterns to Avoid

- **Holding the orchestrator's RWMutex while calling `Provider.HealthCheck()`** — REVIEW.md CR-02 already burned this on `HealthSnapshot`. The fix: snapshot the provider slice under the lock, RELEASE the lock, then iterate.
- **Mocked probe targets** — explicitly rejected by D8. The probe must hit real upstreams; mocks defeat the purpose.
- **Updating gauges from inside a hot request path** — gauges are probe-driven (15min). User requests do NOT update `provider_health_up` directly; they only READ via the cache.
- **`time.Now()` everywhere** — inject `now func() time.Time` into the probe, window, and cache so unit tests can drive the 3-of-15min threshold deterministically. The standard health-checker template in catalog uses plain `time.Now()` — Phase 17 should NOT, because the failure-threshold logic is non-trivial to test without clock injection.
- **`crypto/rand` for jitter** — overkill. `math/rand/v2.NewChaCha8(seed)` or even `math/rand/v2.Float64()` is sufficient and deterministically testable.
- **Spawning the probe BEFORE the HTTP server starts** — race against provider registration. Pattern: register all providers, then start probes, then `ListenAndServe`. (The existing `main.go` already follows this order for providers; probes go in the same window.)
- **Logging every probe success at INFO** — 1 line / 15min × providers × stages = noise. Log only state TRANSITIONS (up→down, down→up) at INFO; success-while-already-up at DEBUG; failure-while-already-down at DEBUG. The existing catalog `health_checker.go:107-116` is the right template.
- **Calling Prometheus `.Set(0)` on every probe failure** — fine for the gauge, but the threshold is "3 within 15min". Flipping to 0 on the first failure and back to 1 on the next success would defeat the windowed semantic. Pattern: window decides UP/DOWN, gauge follows window.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Telegram bot integration for alerts | A new bot client in scraper | Existing `services/maintenance` + `docker/grafana/provisioning/alerting/contactpoints.yml` | Already shipped; just append the rule to `rules.yml` |
| HTTP retry / backoff for the segment-fetch probe stage | Custom `for` loop with sleep | `BaseHTTPClient` from `services/scraper/internal/domain` (which wraps `hashicorp/go-retryablehttp`) | SCRAPER-FOUND-06 already shipped; reuse it |
| Provider fan-out for the probe | New iteration code | Reuse `orchestrator.providers` slice (or a copy) | Single source of truth for registered providers |
| Metrics handler / `/metrics` endpoint | Custom Prometheus exposition | `libs/metrics.Handler()` + the existing scraper router mount | Already exists in `router.go:50` |
| JWT admin auth on the gateway proxy | Custom middleware in scraper | `JWTValidationMiddleware` + `AdminRoleMiddleware` in `services/gateway/internal/transport/router.go:78-81` | Same gate as `/api/admin/*` for catalog — D6 says "same gate" explicitly |
| Sliding-window event tracker | `time.AfterFunc`-based scheduler | A simple `[N]time.Time` slice with prune-on-write | Bounded memory, trivial logic, no goroutine fan-out |
| Random selection from golden pool | Channel-based shuffler | `pool[rng.IntN(len(pool))]` | Stdlib `math/rand/v2`, no allocation |

**Key insight:** This phase is mostly *plumbing existing infrastructure*. The hardest correctness problem is the sliding-window failure semantics — and that's a 40-line file with a unit test. Avoid the temptation to build a "monitoring framework"; this is a 15-minute probe loop with a counter.

## Common Pitfalls

### P-01: Probe taking longer than the cadence
**What goes wrong:** Probe goroutine sleeps 15min, runs probe, but the probe itself takes 30s+ due to an upstream timeout. If the next sleep doesn't account for run-duration, the actual cadence drifts.

**Why it happens:** Naive `time.After(15min)` after work completes adds work-duration to each cycle. Over time, the probe shifts later and later.

**How to avoid:** Use a `time.Ticker` (fires on a fixed schedule) OR record `nextTickAt = startedAt + interval` and sleep `time.Until(nextTickAt)`. Per-stage timeouts (10s already on `BaseHTTPClient`) bound the worst-case probe duration.

**Warning signs:** Probe timestamps in logs drifting away from a 15-min grid.

### P-02: Cardinality explosion from `provider_zero_match_total{provider, selector}`
**What goes wrong:** The `selector` label is arbitrary (`.bsx`, `.bixbox`, `.aitem-wrapper` etc.). If providers emit unique selector strings unbounded, Prometheus cardinality balloons.

**Why it happens:** Developers tend to put exact CSS selectors or HTML XPath fragments in metric labels.

**How to avoid:** Document an allowed selector set per provider (e.g. for AnimePahe: `episode_list_item`, `kwik_packed_js`, `server_link` — short stable identifiers). PRs that add a new selector value require documenting it. Lint rule (future): grep `ParserZeroMatchTotal.WithLabelValues` and verify selector args are constants from a const block.

**Warning signs:** `prometheus_local_storage_memory_series` gauge climbing.

### P-03: 3-of-15min threshold sensitivity to clock skew
**What goes wrong:** If the host clock jumps backwards (NTP correction, container suspend/resume), the window prune logic discards "future" failures incorrectly.

**Why it happens:** `time.Now().Sub(failures[0]) > window` is wall-clock arithmetic.

**How to avoid:** Use `time.Since(t)` (which uses the monotonic clock on Linux when `t` has a monotonic reading) and ensure timestamps are captured via `time.Now()` not constructed from external sources. Inject `now func() time.Time` for tests.

**Warning signs:** Hard to detect in production; symptom is a phantom up→down flip after suspend.

### P-04: Prometheus is NOT scraping the scraper service today
**What goes wrong:** `docker/prometheus/prometheus.yml` has scrape jobs for `gateway`, `auth`, `catalog`, `streaming`, `player`, `rooms`, `scheduler`, `themes` — but NOT `scraper`. The scraper's `/metrics` endpoint is exposed but nothing pulls it. Phase 16's parser metrics are silently invisible right now.

**Why it happens:** `prometheus.yml` was last modified before Phase 15 added the scraper service.

**How to avoid:** Append a new scrape job to `prometheus.yml`:
```yaml
- job_name: 'scraper'
  static_configs:
    - targets: ['scraper:8088']
  metrics_path: /metrics
```
This must be Plan 17-01 or 17-04 work — without it, every other metric in this phase is dead weight.

**Warning signs:** Grafana panel "no data"; `up{job="scraper"}` returns nothing.

### P-05: Last-failure error excerpts leaking secrets / unbounded growth
**What goes wrong:** `StageHealth.LastErr` carries the raw `err.Error()` string. Some errors include URLs with query params (e.g. malsync API responses with anime IDs) or upstream HTML excerpts. Admin-endpoint surface could leak.

**Why it happens:** `err.Error()` is whatever upstream produced, untruncated.

**How to avoid:** Truncate at 256 chars; strip query strings from URLs in errors before storing; whitelist error fields exposed in the admin endpoint. Be paranoid: errors from `dop251/goja` (Kwik unpacker) could contain JS source fragments.

**Warning signs:** Admin endpoint responses kilobytes long; suspicious tokens in error strings.

### P-06: Probe failures during normal startup race
**What goes wrong:** Probe goroutine starts immediately on boot. First tick hits AnimePahe before the DDoS-Guard cookie warmup has happened (the BaseHTTPClient cookie jar is empty). First probe shows DOWN; container marked as unhealthy via Docker healthcheck → restart loop.

**Why it happens:** No initial delay between server start and first probe.

**How to avoid:** First tick after a randomized delay (0 to `interval/4`) — mirrors the example pattern in the "Probe Runner Architecture" section. Probe panic recovery (defer/recover/restart) prevents one bad tick from killing the goroutine. The Docker healthcheck checks `/health` (plain 200), NOT `/scraper/health` — so probe-state and container-state are decoupled.

**Warning signs:** First-tick failures every container restart.

### P-07: `absent(provider_health_up)` not firing when probe goroutine dies silently
**What goes wrong:** Probe goroutine panics without `defer/recover`. Subsequent Prometheus scrapes still see the LAST value the gauge was set to (gauge values persist across scrapes; they don't auto-expire). Operator thinks "still up" while probe has been dead for hours.

**Why it happens:** Prometheus gauges are stateful — they hold the last `.Set()` value until the process exits or the metric is unregistered.

**How to avoid:** TWO defenses:
  1. **Defer/recover in the probe goroutine**, restart on panic, log + metric (`probe_panics_total`).
  2. **Heartbeat metric** `provider_probe_last_tick_timestamp{provider}` set to `time.Now().Unix()` on every tick. Alert rule: `time() - provider_probe_last_tick_timestamp > 30*60` → critical (probe stopped).

**Warning signs:** Gauge values stuck on a fixed timestamp; no recent log lines for the probe.

### P-08: Cache fail-open vs fail-closed semantics
**What goes wrong:** When the cache has no entry (or stale entry) for a provider — fail-open (treat as UP) means the orchestrator dispatches to a possibly-dead provider; fail-closed (treat as DOWN) means a probe restart blanks the service.

**Why it happens:** Implicit default ambiguity.

**How to avoid:** **Fail-open is correct here** (D3 implicitly + standard synthetic-monitoring practice): a missing probe signal should not turn off the service. Docs/comment on `IsHealthy` MUST state this. Tests must cover the "no entry + stale entry" branches.

**Warning signs:** Service goes silent during deploys; correlated with probe-goroutine startup gap.

### P-09: Test flakiness from real clock in window logic
**What goes wrong:** Unit tests that rely on `time.Sleep(15min)` to drive the threshold are slow and flaky; tests that rely on `time.Sleep(100ms)` race against scheduler jitter.

**Why it happens:** No clock injection.

**How to avoid:** Inject `now func() time.Time` into `window`, `cache`, `probe`. Tests construct a virtual clock (`mu sync.Mutex; ts time.Time`) and advance it explicitly. The existing repo doesn't use a clock-injection library — recommendation: bare function injection (no new dependency).

**Warning signs:** CI flakes on health tests; tests longer than 1s.

## Code Examples

### Defining the gauge + counter (new file `libs/metrics/provider.go`)

```go
// Source: derived from libs/metrics/player_health.go (in-repo template)
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // ProviderHealthUp: 1=up, 0=down per (provider, stage).
    // Five canonical stages: search, episodes, servers, stream, stream_segment.
    ProviderHealthUp = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "provider_health_up",
            Help: "Whether a scraper provider stage is up (1=up, 0=down) per the 3-of-15min liveness probe",
        },
        []string{"provider", "stage"},
    )

    // ProviderProbeLastTick: Unix ts of last probe tick, per (provider).
    // Used by the absent()-style alert that catches a dead probe goroutine.
    ProviderProbeLastTick = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "provider_probe_last_tick_timestamp",
            Help: "Unix timestamp of the last completed probe tick per provider",
        },
        []string{"provider"},
    )

    // ParserZeroMatchTotal: counter for selector-miss events per (provider, selector).
    // SCRAPER-NF-04 — providers MUST emit this when an HTML selector returns 0 hits.
    // The `selector` label is a short stable identifier (NOT raw CSS — see Pitfall P-02).
    ParserZeroMatchTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "parser_zero_match_total",
            Help: "Total count of HTML/JSON selector-miss events per (provider, selector)",
        },
        []string{"provider", "selector"},
    )
)
```

### Wiring the probe into `main.go`

```go
// Source: derived from existing services/scraper/cmd/scraper-api/main.go:101-131 boot sequence
func main() {
    // ... existing code: log, config, registry, redis, malsync, animepahe ...

    // Build orchestrator with health cache (Phase 17 new arg)
    cache := health.NewInMemoryHealthCache()
    orchestrator := service.NewOrchestrator(log, registry, cache)  // signature change
    orchestrator.Register(animePaheProvider)

    // Start one probe goroutine per registered provider.
    // Phase 17 only: animepahe. Phase 18+: more providers register the same way.
    rootCtx, cancelRoot := context.WithCancel(context.Background())
    defer cancelRoot()
    for _, p := range orchestrator.RegisteredProviders() {
        runner := health.NewProbeRunner(p, health.DefaultGoldenPool, cache, log)
        go runner.Start(rootCtx)
    }

    // ... existing HTTP server setup + signal handling ...
    // On SIGTERM: cancelRoot() FIRST so probes stop cleanly, then srv.Shutdown(ctx).
}
```

### Adding the admin endpoint

```go
// Source: derived from existing services/scraper/internal/handler/scraper.go pattern
func (h *ScraperHandler) GetAdminHealth(w http.ResponseWriter, r *http.Request) {
    snap := h.svc.HealthSnapshot(r.Context())  // existing method
    // Decorate with last_ok + last_err per stage from the in-memory cache
    enriched := h.cache.AdminSnapshot()  // new method — returns full ProviderHealth incl. LastErr
    httputil.OK(w, map[string]any{
        "providers": snap,           // existing public shape
        "admin":     enriched,       // new — admin-only enriched fields
        "generated_at": time.Now().UTC().Format(time.RFC3339),
    })
}

// In services/scraper/internal/transport/router.go:
r.Route("/scraper", func(r chi.Router) {
    // ... existing routes ...
    r.Get("/health", scraperHandler.GetHealth)
    r.Get("/health/admin", scraperHandler.GetAdminHealth)  // NEW — admin-only
})
```

### Adding the gateway proxy entry

```go
// services/gateway/internal/config/config.go (MODIFIED)
type ServiceURLs struct {
    // ... existing fields ...
    ScraperService string  // NEW
}
// In Load():
ScraperService: getEnv("SCRAPER_SERVICE_URL", "http://scraper:8088"),

// services/gateway/internal/service/proxy.go (MODIFIED — getServiceURL)
case "scraper":
    return s.serviceURLs.ScraperService, nil

// services/gateway/internal/handler/proxy.go (MODIFIED)
func (h *ProxyHandler) ProxyToScraper(w http.ResponseWriter, r *http.Request) {
    h.proxy(w, r, "scraper")
}

// services/gateway/internal/transport/router.go (MODIFIED — extend /api/admin group)
// In r.Group with JWTValidationMiddleware + AdminRoleMiddleware (router.go:144):
r.HandleFunc("/admin/scraper/*", proxyHandler.ProxyToScraper)
// Path rewrite needed: gateway sees /api/admin/scraper/health, scraper expects /scraper/health/admin.
// Add a case in proxy.go getServiceURL's path rewrite logic, similar to grafana/prometheus.
```

### Grafana alert rule (append to `docker/grafana/provisioning/alerting/rules.yml`)

```yaml
# Source: modeled on existing player-unavailable rule (rules.yml:485-529)
- uid: provider-health-stream-segment-down
  title: Provider Stream-Segment Down
  noDataState: OK
  condition: C
  data:
    - refId: A
      relativeTimeRange:
        from: 900   # 15m lookback
        to: 0
      datasourceUid: PBFA97CFB590B2093
      model:
        expr: provider_health_up{stage="stream_segment"}
        instant: true
        refId: A
    - refId: B
      relativeTimeRange:
        from: 900
        to: 0
      datasourceUid: __expr__
      model:
        refId: B
        type: reduce
        expression: A
        reducer: last
    - refId: C
      relativeTimeRange:
        from: 900
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
  for: 15m
  labels:
    severity: critical
  annotations:
    summary: "Scraper provider {{ $labels.provider }} stream_segment DOWN"
    description: "Provider {{ $labels.provider }} cannot fetch the first HLS segment from the golden pool — likely upstream broken or selector drift. Check /api/admin/scraper/health."
```

## Runtime State Inventory

This phase is greenfield additive — no rename / refactor / migration. The Runtime State Inventory section is intentionally omitted.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `math/rand` (v1) — global mutex-protected RNG | `math/rand/v2` (per-instance, ChaCha8) | go 1.22 | Better randomness API, faster, deterministically seedable |
| Polling `/health` via blackbox-exporter | In-process synthetic probe with full pipeline | Catalog repo already chose this in `health_checker.go` | Probe path identical to user path; no false-positive from network blip on a stub `/health` |
| Grafana legacy alerting | Grafana unified alerting (provisioned via YAML) | Grafana 8.x | Repo already uses this — `provisioning/alerting/rules.yml` |
| Counter "errors per minute" alerts | Gauge + window-based UP/DOWN flip | Industry shift (SRE book / Google) | Phase 17 uses gauge — matches the pattern in `player_health_up` |

**Deprecated/outdated:**
- Phase 16 added scraper to docker-compose but `prometheus.yml` was NOT updated. Phase 17 MUST add the scrape job (Pitfall P-04).
- `parser_zero_match_total` is referenced in `.planning/research/PITFALLS.md` and required by SCRAPER-NF-04 but does NOT exist in `libs/metrics/parser.go` yet — must be added this phase.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The 5-10 anime golden pool can use Naruto/One Piece/Attack on Titan/Demon Slayer/Jujutsu Kaisen — CONTEXT.md D2 lists these as candidates; per-provider availability not verified by this research | "Golden Pool" — `services/scraper/internal/health/golden.go` | If one is delisted on AnimePahe, that stage permanently false-negatives until pool is corrected. Mitigation: pool randomization means it's 1-in-5 selection per tick, not deterministic; flap-protection from 3-of-15min threshold absorbs single-anime miss. |
| A2 | Grafana's webhook alert payload format matches what `services/maintenance` expects | "Alert Pipeline" | If the maintenance webhook chokes on the new rule's payload, alerts won't reach Telegram. Mitigation: dry-run the rule in Grafana UI before commit; verify a test alert reaches Telegram during verification. |
| A3 | The existing `domain.Health.Stages` map[string]StageHealth can be repurposed for the 5 canonical Phase 17 stage names (search/episodes/servers/stream/stream_segment) by renaming the AnimePahe `stageNames` const | "Architecture Patterns" | Backward-compat risk only if some external consumer reads the old stage names ("find_id", "list_episodes", etc.). Only the existing scraper `GetHealth` handler returns these — Phase 16 frontend doesn't depend on specific stage keys. |
| A4 | Prometheus scrape interval (15s) catches gauge flips within one minute | "Architecture Patterns" | If Prometheus storage lags, alert evaluation could be delayed beyond 15min. Mitigation: existing config is 15s scrape + 1m evaluation — well within budget. |
| A5 | Gateway-level admin JWT validation is sufficient for the `/api/admin/scraper/health` endpoint; scraper service does not need its own auth middleware (binds to 127.0.0.1 only) | "Admin Endpoint" | If the scraper bind ever changes to 0.0.0.0, admin endpoint becomes unauthenticated. Mitigation: leave a code comment + a TODO referring to this assumption; consider adding an in-scraper auth check as defense-in-depth in a v3.1 phase. |
| A6 | `math/rand/v2` (go 1.22+) is available in the scraper service | "Standard Stack" | Repo's `go.mod` declares `go 1.23.0` — verified, safe. |
| A7 | The `defer recover` + restart pattern for probe panics is non-controversial; an unhandled panic in a goroutine crashes the whole process | "Code Examples" / Pattern 1 | This is correct per Go spec — verified by web search results on goroutine panic recovery. |

## Open Questions (RESOLVED)

1. **Should each provider get its own probe goroutine, or one shared goroutine iterating providers?**
   - What we know: Phase 17 ships with one provider (animepahe); Phase 18+ adds 9anime; Phase 19 adds AnimeKai (gated). The CONTEXT.md does not lock this.
   - What's unclear: Per-provider isolation (recommendation) costs N goroutines but prevents head-of-line blocking; shared iteration is simpler but a stuck provider stalls others.
   - Recommendation: **Per-provider goroutine**. Each starts in `main.go` after `orchestrator.Register(...)`. The probe code is the same in either case — just the spawn loop differs.
   - **RESOLVED:** Per-provider goroutine. Pinned in Plan 17-02 Task 2. Reason: prevents head-of-line blocking when one provider's 5-stage probe takes longer than another's.

2. **Where exactly does the probe's "first HLS segment" fetch happen — inside `Provider.GetStream` or after it?**
   - What we know: `Provider.GetStream` returns a `*Stream` with `Sources []Source` (each is an `{URL, Type, Quality}`). The probe needs to verify the URL actually serves data.
   - What's unclear: The probe could (a) call `GetStream` and trust the returned URL, OR (b) call `GetStream` and then issue a GET to `Sources[0].URL` and verify the response body is non-empty (a few KB of HLS segment).
   - Recommendation: **(b) — fetch the segment**. Reasoning: a 404 on the actual CDN URL is a different failure mode than `GetStream` returning a valid-looking URL. This is the "stream_segment" stage by name and contract.
   - **RESOLVED:** Probe-owned separate stage AFTER GetStream returns. Pinned in Plan 17-02 Task 1 (probe.go pipeline definition). Reason: keeps the stage boundary observable as its own gauge dimension; HLS m3u8 parse + first segment HEAD lives inside probe.go, not inside the Provider interface.

3. **Should the orchestrator skip-on-unhealthy emit a SEPARATE counter (e.g. `provider_skipped_total{provider,reason="health_down"}`) or reuse `parser_fallback_total{from,to}`?**
   - What we know: `parser_fallback_total` is currently incremented on every retryable failure in the failover loop.
   - What's unclear: Whether health-skip is conceptually a "failover from X" or a separate event.
   - Recommendation: Reuse `parser_fallback_total` with `from=<skipped_provider>`, `to=<next_provider>`. Same data shape, same dashboard. Add a comment in `orchestrator.go` documenting the semantic.
   - **RESOLVED:** Reuse parser_fallback_total{from=<provider>,to=<skipped>}. Pinned in Plan 17-01 Task 3 (orchestrator skip-unhealthy wiring). Reason: avoids a fifth metric family; the existing fallback counter already represents "request was deflected somewhere else".

4. **Should the `health/admin` route be `/scraper/health/admin` or `/scraper/admin/health`?**
   - What we know: D6 says route at `/api/admin/scraper/health` at the gateway. The scraper side is free.
   - What's unclear: Path naming convention.
   - Recommendation: `/scraper/admin/health` (admin-namespace inside scraper-namespace, matching the catalog pattern at `services/catalog/internal/transport/router.go:123`). Gateway rewrites `/api/admin/scraper/health` → `/scraper/admin/health`.
   - **RESOLVED:** /scraper/health/admin (NOT the originally recommended /scraper/admin/health). Pinned in Plan 17-03 Task 2 (transport router) and Plan 17-03 Task 3 (gateway proxy). Reason: existing /scraper/health public route is established; nesting admin underneath as /scraper/health/admin reads as a privileged variant of the same resource, matches the "admin debug for an existing endpoint" framing in CONTEXT.md D6, and avoids creating a new top-level /scraper/admin/* namespace just for this single endpoint. The gateway still maps /api/admin/scraper/* → scraper:8088/scraper/health/admin via path-rewrite.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Prometheus | Metric scrape + alert eval | Yes | prom/prometheus:v2.50.1 | — |
| Grafana | Dashboard + alert UI | Yes | grafana/grafana:10.3.3 | — |
| `maintenance` service (host-gateway:8087) | Alert → Telegram delivery | Yes | in-repo Go service | — |
| Telegram bot token + chat ID | Alert delivery | Yes | `TELEGRAM_ADMIN_CHAT_ID` env in `docker/.env` (per MEMORY.md) | — |
| Redis | Cache (already wired) | Yes | redis:7-alpine | — |
| Scraper-service container | Where the probe runs | Yes | port 8088, in `docker-compose.yml:147` | — |
| **`scraper` job in `prometheus.yml`** | Prometheus scrape | **NO** | — | **Add the job in Plan 17-01 — see Pitfall P-04** |

**Missing dependencies with no fallback:**
- `scraper` job entry in `docker/prometheus/prometheus.yml`. Without it, all the metrics this phase emits are invisible. This is a one-line YAML change — must be in the plan.

**Missing dependencies with fallback:**
- None.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `stretchr/testify` v1.9.0 + `prometheus/client_golang/prometheus/testutil` |
| Config file | `services/scraper/go.mod` (`go 1.23.0`); no separate config |
| Quick run command | `cd services/scraper && go test ./internal/health/... -count=1 -race` |
| Full suite command | `cd services/scraper && go test ./... -count=1 -race` |
| Lint | `cd services/scraper && go vet ./...` and existing `golint` package's checks |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| SCRAPER-OBS-01 | Probe ticks at 15min ± 20% jitter | unit (clock-injected) | `go test ./internal/health -run TestProbeRunner_TickCadence -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-01 | Probe selects randomly from golden pool | unit | `go test ./internal/health -run TestProbeRunner_GoldenPoolRotation -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-01 | Probe runs 5 stages in order, stops at first failure | unit | `go test ./internal/health -run TestProbeRunner_StagesShortCircuit -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-01 | Probe goroutine recovers from panic and restarts | unit | `go test ./internal/health -run TestProbeRunner_PanicRecovers -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-02 | Gauge flips to 0 after 3 failures within 15min | unit (clock-injected) | `go test ./internal/health -run TestWindow_FlipAfter3FailuresIn15min -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-02 | Gauge flips back to 1 on first success | unit | `go test ./internal/health -run TestWindow_RestoreOnFirstSuccess -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-02 | Failures outside 15min window do NOT count | unit (clock-injected) | `go test ./internal/health -run TestWindow_StaleFailuresIgnored -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-02 | All 5 stage labels emit gauge values (no silent zero-cardinality) | unit | `go test ./internal/health -run TestGaugeAllStagesEmitted -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-03 | Orchestrator skips provider when cache says down | unit (fakeProvider) | `go test ./internal/service -run TestOrchestrator_SkipsUnhealthyProvider -race -count=1` | ⚠️  extend `orchestrator_test.go` |
| SCRAPER-OBS-03 | Orchestrator re-includes provider when cache says up | unit | `go test ./internal/service -run TestOrchestrator_RejoinsHealthyProvider -race -count=1` | ⚠️  extend `orchestrator_test.go` |
| SCRAPER-OBS-03 | Cache reads stale (>60s) treated as healthy (fail-open) | unit | `go test ./internal/health -run TestCache_StaleIsHealthy -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-03 | Cache concurrent reader/writer correctness | race-test | `go test ./internal/health -run TestCache_RaceFreeReaderWriter -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-04 | `provider_health_up{stage="stream_segment"} == 0 for 15m` alert rule YAML parses + lints | unit | `gsd-sdk` or `promtool check rules docker/grafana/provisioning/alerting/rules.yml` (note: Grafana provisioning syntax is NOT promtool-native; use `grafanactl` or visual review) | manual (no native validator) |
| SCRAPER-OBS-04 | Dashboard JSON loads in Grafana on container start | smoke | `make redeploy-grafana && curl http://localhost:3000/api/dashboards/uid/scraper-health -u admin:admin` | manual / post-deploy |
| SCRAPER-OBS-04 | Test alert payload reaches `/api/grafana-webhook` (maintenance) | integration | Manual: trigger an alert via Grafana UI test-fire, check maintenance logs | manual / post-deploy |
| SCRAPER-OBS-05 | Admin endpoint returns 200 with per-provider/per-stage + last_ok | unit | `go test ./internal/handler -run TestAdminHealthHandler_Shape -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-05 | Admin endpoint truncates last_err to 256 chars | unit | `go test ./internal/handler -run TestAdminHealthHandler_TruncatesLastErr -race -count=1` | ❌ Wave 0 |
| SCRAPER-OBS-05 | Gateway proxies `/api/admin/scraper/*` → scraper service | unit | `go test ./services/gateway/internal/transport -run TestRouter_AdminScraperProxy -race -count=1` | ⚠️  extend `router_test.go` |
| SCRAPER-OBS-05 | Gateway rejects non-admin JWT for `/api/admin/scraper/*` | unit | `go test ./services/gateway/internal/transport -run TestRouter_AdminScraperRejectsNonAdmin -race -count=1` | ⚠️  extend `router_test.go` |
| SCRAPER-NF-04 | `parser_requests_total{provider,operation,status}` increments on provider method call | unit | `go test ./internal/providers/animepahe -run TestProvider_EmitsParserRequestsTotal -race -count=1` | ⚠️  extend `client_test.go` |
| SCRAPER-NF-04 | `parser_request_duration_seconds` observes per method | unit | same test | ⚠️  extend `client_test.go` |
| SCRAPER-NF-04 | `parser_zero_match_total{provider,selector}` emits on selector-miss | unit | `go test ./internal/providers/animepahe -run TestProvider_EmitsZeroMatch -race -count=1` | ⚠️  extend `client_test.go` |
| SCRAPER-NF-04 | `parser_fallback_total{from,to}` already covered in `orchestrator_test.go` | unit | existing | ✅ |

### Sampling Rate
- **Per task commit:** `cd services/scraper && go test ./internal/health -race -count=1` (the new package; ~< 5s)
- **Per wave merge:** `cd services/scraper && go test ./... -race -count=1` (full scraper suite; ~< 30s)
- **Phase gate:** Full scraper suite green + manual Grafana dashboard load + alert test-fire to Telegram before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `services/scraper/internal/health/probe_test.go` — covers SCRAPER-OBS-01 (4 sub-tests)
- [ ] `services/scraper/internal/health/window_test.go` — covers SCRAPER-OBS-02 (3 sub-tests)
- [ ] `services/scraper/internal/health/cache_test.go` — covers SCRAPER-OBS-03 (2 sub-tests; race-safe)
- [ ] `services/scraper/internal/health/golden_test.go` — covers golden-pool selection determinism with injected rng
- [ ] Extend `services/scraper/internal/service/orchestrator_test.go` with skip/rejoin tests
- [ ] Extend `services/scraper/internal/providers/animepahe/client_test.go` with `parser_*` metric assertions
- [ ] Extend `services/scraper/internal/handler/scraper_test.go` (or new `admin_test.go`) with admin-handler tests
- [ ] Extend `services/gateway/internal/transport/router_test.go` with `/api/admin/scraper/*` proxy tests
- [ ] Manual checklist: `docs/issues/` entry confirming a test alert reached Telegram (one-time integration verification)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | JWT bearer via `authz.JWTManager` (existing) — gateway `JWTValidationMiddleware` |
| V3 Session Management | no | Bearer-only; no session state for this endpoint |
| V4 Access Control | yes | `AdminRoleMiddleware` checking `authz.IsAdmin(ctx)` — same gate as catalog `/admin/*` |
| V5 Input Validation | yes | No user-controlled inputs on the admin health endpoint (it's a GET with no params); golden-pool entries are compile-time consts |
| V6 Cryptography | no | No crypto operations in this phase |
| V7 Error Handling | yes | Admin endpoint returns rich error data — see Pitfall P-05 (truncate last_err) |
| V9 Communications | yes | Gateway-to-scraper is internal HTTP; scraper bound to 127.0.0.1 — no TLS needed inside docker network |

### Known Threat Patterns for {scraper + Prometheus + Go service}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Sensitive error text leakage via admin endpoint | Information disclosure | Truncate `last_err` to 256 chars; strip query strings from URLs in errors; admin-only auth (P-05) |
| Cardinality bomb on `parser_zero_match_total{selector}` | DoS (memory exhaustion in Prometheus) | Selector label is a const from a known set; PR review enforces (P-02) |
| Goroutine panic crashes service | DoS | `defer recover()` in every probe goroutine, restart on panic (P-07) |
| Stale gauge masquerading as "up" after probe death | Information disclosure / wrong state | Heartbeat metric `provider_probe_last_tick_timestamp` + absent-style alert (P-07) |
| Admin endpoint exposed without auth if scraper bind changes | Privilege escalation | A5 assumption documented + code comment; future v3.1 task to add scraper-side auth |
| Golden-pool anime ID in error messages may leak | Information disclosure | None — golden pool is public data (top-N popular anime); no PII |
| Slow-loris on /scraper/admin/health | DoS | Existing chi router middleware + gateway rate limit (`RateLimitMiddleware` already on `/api/*`) |
| Cache-poisoning by a fake probe goroutine writer | Tampering | Cache is package-private; only `ProbeRunner` has a reference — Go's package-level encapsulation suffices |

## Project Constraints (from CLAUDE.md)

- **Logging:** Use `libs/logger`; structured `Infow`/`Warnw`/`Errorw` (probe must log state transitions, not every tick).
- **Caching:** `libs/cache` with appropriate TTLs — but D3 explicitly says the health cache stays in-memory; Redis is for malsync/episodes/stream URLs, not health.
- **Don't add complex abstractions** — the probe is one ticker loop, one ring buffer, one map.
- **Don't add CDN-related code** — not needed.
- **Go service structure:** `cmd/scraper-api/main.go`, `internal/{config,domain,handler,service,transport}` — new `internal/health/` package fits this layout.
- **Naming:** snake_case files (`probe_test.go`, `health_cache.go`), PascalCase types, camelCase vars/methods.
- **Error wrapping:** Use `libs/errors` for cross-package errors; probe internal errors can be plain stdlib.
- **Testing:** Don't hit external APIs in tests. Use `fakeProvider` (already exists in `orchestrator_test.go`) and clock injection.
- **Co-authors on commits:** Include all three from MEMORY.md.
- **After-update skill:** Invoke `/animeenigma-after-update` after completion. (Out of scope for research; in scope for execute-phase.)
- **Dev commands:** `make redeploy-scraper`, `make redeploy-grafana`, `make redeploy-prometheus`, `make redeploy-gateway` — all needed in the deploy step of this phase.

## Sources

### Primary (HIGH confidence)
- `services/scraper/internal/domain/provider.go` — existing `Provider`/`Health`/`StageHealth` types
- `services/scraper/internal/service/orchestrator.go` — existing `HealthSnapshot`, `runFailover`, REVIEW.md CR-02 locking discipline
- `services/scraper/internal/handler/scraper.go` — existing `/scraper/health` handler
- `services/scraper/internal/providers/animepahe/client.go` — existing `markStage`/`HealthCheck` per-provider implementation
- `services/scraper/cmd/scraper-api/main.go` — existing boot sequence; where to plug in the probe goroutine
- `services/scraper/internal/transport/router.go` — existing chi router with `/scraper/*` routes
- `libs/metrics/parser.go` — existing `ObserveParser` + `parser_*` metric definitions
- `libs/metrics/player_health.go` — template for the new `provider_health_up` GaugeVec
- `libs/metrics/metrics.go` — `Collector` + `Handler()` for promhttp
- `services/catalog/internal/service/health_checker.go` — gold-standard template for the probe runner
- `services/gateway/internal/transport/router.go` — existing JWT + admin middleware patterns; `/api/admin/*` proxy
- `services/gateway/internal/config/config.go` — `ServiceURLs` struct extension point
- `services/gateway/internal/service/proxy.go` — `getServiceURL` switch + path-rewrite pattern
- `docker/prometheus/prometheus.yml` — current scrape config (scraper job missing)
- `docker/grafana/dashboards/player-health.json` — dashboard template
- `docker/grafana/provisioning/alerting/rules.yml` — alert rule template (`player-unavailable`)
- `docker/grafana/provisioning/alerting/contactpoints.yml` — Telegram-via-maintenance pipeline
- `services/maintenance/internal/transport/webhook.go` — alert delivery contract
- `docker/docker-compose.yml` — scraper container definition (port 8088 confirmed)
- `.planning/phases/17-observability/17-CONTEXT.md` — locked decisions D1–D8

### Secondary (MEDIUM confidence)
- [Prometheus best practices — instrumentation](https://prometheus.io/docs/practices/instrumentation/)
- [Prometheus metric types](https://prometheus.io/docs/concepts/metric_types/)
- [PromLabs: Avoid these 6 mistakes](https://promlabs.com/blog/2022/12/11/avoid-these-6-mistakes-when-getting-started-with-prometheus/) — cardinality cautions
- [Go Wiki: PanicAndRecover](https://go.dev/wiki/PanicAndRecover) — recover semantics
- [Graceful Shutdown in Go (DEV)](https://dev.to/jones_charles_ad50858dbc0/graceful-goroutine-shutdowns-in-go-a-practical-guide-2b9a) — ctx + ticker pattern
- [VictoriaMetrics: Go graceful shutdown](https://victoriametrics.com/blog/go-graceful-shutdown/) — server.Shutdown + cancelRoot ordering

### Tertiary (LOW confidence)
- None. All claims in this research are anchored either to in-repo code or to standard Prometheus/Go documentation; cross-referenced.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all dependencies already in `services/scraper/go.mod`; nothing new to add
- Architecture: HIGH — templates exist in-repo (`health_checker.go`, `player_health.go`, `player-unavailable` rule)
- Pitfalls: HIGH — anchored to existing REVIEW.md notes (CR-02 locking) and verified gaps (`prometheus.yml` missing scraper job)
- Validation: HIGH — testing patterns (`fakeProvider`, `testutil.ToFloat64`) already in use in `orchestrator_test.go`
- Security: MEDIUM — admin endpoint exposure is gated; assumption A5 documented (scraper-side defense-in-depth deferred)

**Research date:** 2026-05-12
**Valid until:** 2026-06-11 (30 days — stable phase; only risk is a Grafana/Prometheus version bump or a new SCRAPER-OBS-* requirement being added)
