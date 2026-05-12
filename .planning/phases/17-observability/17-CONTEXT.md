# Phase 17: Observability — Context

**Gathered:** 2026-05-12
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous --auto)

<domain>
## Phase Boundary

A dead provider stops being silently dead — health visibility per provider per stage exists before a second provider is added so it can't hide the first's degradation.

**Concretely, this phase delivers:**

1. A **liveness probe** that exercises every registered scraper provider end-to-end (search → episodes → servers → stream → first HLS segment) on a 15-minute cadence (±20% jitter), against a rotating 5–10 anime "golden pool" — per provider.
2. A **Prometheus health gauge family** `provider_health_up{provider, stage}` exposed by the scraper service. Stage flips to 0 after 3 consecutive failures within 15 minutes; flips back to 1 on first success.
3. **Orchestrator failover gating**: the orchestrator skips any provider whose health gauge reads 0 in the last 60 s. Skipped providers re-enter rotation when the probe flips them back to 1.
4. **Grafana dashboard + alert** that fires when any `provider_health_up{stage="stream_segment"}` is 0 for 15 minutes; alert pushes to the existing Telegram admin chat.
5. **Admin debug endpoint** `GET /api/admin/scraper/health` exposing the current per-provider/per-stage snapshot plus last successful timestamps.
6. **Per-provider parser metrics**: `parser_requests_total{provider, operation, status}`, `parser_request_duration_seconds{provider, operation}`, `parser_fallback_total{from, to}`, `parser_zero_match_total{provider, selector}` — reusing `libs/metrics/parser.go` patterns.

**Out of scope (deferred to later phases / milestones):**
- Adding a second provider (Phase 18 — 9anime).
- Per-user observability dashboards.
- Tracing (no Jaeger / OTLP in this phase).
- Alert routing beyond Telegram.
- Self-healing actions (Phase 17 only observes and gates — it does NOT auto-restart anything).

**Requirements covered:**
- SCRAPER-OBS-01 (15-min liveness probe with golden anime pool)
- SCRAPER-OBS-02 (provider_health_up gauge family, 5 stages, 3-failures-in-15-min threshold)
- SCRAPER-OBS-03 (orchestrator skips unhealthy providers, 60 s in-memory cache)
- SCRAPER-OBS-04 (Grafana dashboard panel + Telegram alert)
- SCRAPER-OBS-05 (admin /api/admin/scraper/health endpoint)
- SCRAPER-NF-04 (per-provider parser metrics)

</domain>

<decisions>
## Implementation Decisions

### D1 — Liveness probe lives in the scraper service (not scheduler)

The probe is invoked **inside the `scraper` service** (a new goroutine started in `services/scraper/cmd/scraper-api/main.go`), not as a job in the existing `scheduler` service.

**Why:** The probe needs in-process access to the orchestrator and registered providers (the same code-path users hit, including the same HTTP client / DDoS-Guard cookie jar). Running it from the scheduler would require either duplicating provider construction or exposing an authenticated "probe me" RPC. Co-locating in the scraper service keeps the probe path identical to the user path, which is the whole point of a liveness probe.

**Trade-off accepted:** if the scraper container crashes, the probe stops — but the gauge then also stops being scraped by Prometheus, so the dashboard will go red anyway via Prometheus' `absent()` query.

### D2 — Golden anime pool is a static-config list, randomized per-tick

The golden pool is a **5–10 entry static list** stored in `services/scraper/internal/health/golden.go` (e.g., the top-5 most-watched anime: Naruto, One Piece, Attack on Titan, Demon Slayer, Jujutsu Kaisen — IDs need verification per provider). Each probe tick randomly selects 1 anime from the pool to test the full pipeline against.

**Why:** Avoids a per-anime DB schema + admin UI that would explode this phase's scope. Static config is one PR away from a future "load from DB" rewrite if the pool needs to scale.

**Trade-off accepted:** if an anime in the pool gets delisted on a provider, the probe sees a false-negative until the pool is updated by hand. Acceptable for v3.0.

### D3 — Health cache is in-memory, owned by the orchestrator

The 60 s health cache lives **in-memory inside the orchestrator** (a `map[string]providerHealth` guarded by an RWMutex, where `providerHealth` tracks per-stage status + timestamp). The probe writes to it after each tick; the failover loop reads from it before dispatching.

**Why:** Redis is not needed — health is per-process state, and a multi-replica scraper is not in scope (v3.0 ships one scraper container). When/if we scale out, the probe per-container will independently maintain its own view, which is correct: each container has its own HTTP client / cookie jar / network egress.

### D4 — Probe uses `domain.Provider.HealthCheck` already on the interface

The `domain.Provider` interface already has `HealthCheck(ctx) Health` (added in Phase 15). The probe orchestrator calls `Provider.HealthCheck()` per-tick on each registered provider. **Extending `Health`** to carry per-stage status (search/episodes/servers/stream/stream_segment) is part of this phase's contract change.

**Open for planner to decide:** Whether `HealthCheck` does the full pipeline itself (provider-owned), or whether the probe orchestrator calls each provider method individually (`Search`, `ListEpisodes`, ...). Recommended: probe-owned, so adding a new provider doesn't require re-implementing the probe; the provider just satisfies the existing methods.

### D5 — Prometheus metric exposure reuses existing patterns

Metrics are exposed via the **existing scraper service `/metrics` endpoint** (the scraper service already registers Prometheus collectors via `libs/metrics/metrics.go`). The new `provider_health_up` gauge family + parser metrics are added to `libs/metrics/parser.go` (or a new `libs/metrics/provider.go` next to it — planner's call).

**Why:** Existing services already pattern this; the catalog service has parser metrics for HiAnime/Consumet/AnimeLib that we'll deprecate in Phase 20 — reusing the same metric names with `provider=animepahe` keeps Grafana dashboards forward-compatible.

### D6 — Admin endpoint route goes through the gateway

`GET /api/admin/scraper/health` is routed by the gateway to the scraper service's `/scraper/health` endpoint (the public one already exists; this just adds the admin namespace + auth gate). Auth: same JWT bearer admin gate already used by `/api/admin/*` in catalog.

**Why:** the existing `/scraper/health` endpoint is already public-readable (no PII, just provider status), but the admin variant adds `last_ok` timestamps + last-failure error excerpts for debugging — that's why it's admin-gated. A separate route is cleaner than overloading the existing endpoint with `?admin=1`.

### D7 — Grafana alert uses the existing Telegram channel

The alert pipeline is **already wired** for AnimeEnigma — Grafana → alertmanager → Telegram bot uses `TELEGRAM_ADMIN_CHAT_ID`. This phase only adds:
- A new dashboard panel (per-stage gauge per provider) in the existing scraper dashboard.
- A new alert rule: `provider_health_up{stage="stream_segment"} == 0 for 15m`.

No new infrastructure. The planner should produce the Grafana JSON dashboard fragment + alert rule YAML; the deploy operator imports them.

### D8 — Live-probe upstream traffic is acceptable cost

15-min cadence × 5 stages × ~1 provider ≈ 20 requests / 15 min × per provider. Each probe exercises real upstream (AnimePahe, etc.), so this generates real traffic. **Accepted** because: (a) the probe is the whole point — synthetic monitoring needs real traffic; (b) the scale is negligible (~80/hour); (c) catching dead providers early outweighs the cost.

**Trade-off NOT accepted:** mocked probe targets. Mocks would catch only configuration drift, not upstream death — defeating the phase's purpose.

</decisions>

<code_context>
## Existing Code Insights

- **`services/scraper/internal/domain/provider.go`** — defines `domain.Provider` interface with `HealthCheck(ctx) Health` already; `domain.Health` exists. This phase extends `Health` with per-stage status.
- **`services/scraper/internal/service/orchestrator.go`** — has `HealthSnapshot(ctx)` that fans out to all registered providers. Currently called by `/scraper/health` handler. This phase wires the probe to populate an in-memory cache rather than recomputing on each request.
- **`services/scraper/internal/handler/scraper.go`** — has `Health` handler that returns the snapshot. This phase adds an admin variant exposing `last_ok` + failure context.
- **`services/scraper/internal/transport/router.go`** — Chi router; mounts handlers under `/scraper/*`. Admin sub-route lives elsewhere — gateway routes `/api/admin/scraper/*` to a new admin handler block.
- **`libs/metrics/parser.go`** — `ObserveParser(provider, operation, start, errp)` pattern; emits `parser_requests_total{provider,operation,status}` + `parser_request_duration_seconds{provider,operation}`. Reused by all current parsers (HiAnime, Consumet, AnimeLib, AnimePahe).
- **`libs/metrics/player_health.go`** — a similar health-gauge pattern for player tracking. Could be a template for the new provider gauge or reused if the shape fits.
- **Gateway routing** (`services/gateway/internal/transport/router.go`) — already proxies `/api/admin/*` → catalog. This phase adds a `/api/admin/scraper/*` → scraper route.
- **Grafana + Prometheus** — already deployed in `docker/docker-compose.yml`; scraper service already exposes `/metrics`. Telegram alerts already configured (see `TELEGRAM_ADMIN_CHAT_ID` in CLAUDE.md).
- **Phase 16 already touches** `domain.Server.Type` (CR-02 fix); `meta.tried` in handler responses; ReportButton with `scraperProvider` + `triedChain` props. The "tried providers" surface is already plumbed end-to-end — this phase doesn't need to extend it, only emit metrics from it.

</code_context>

<specifics>
## Specific Ideas

### S1 — Plan structure (planner discretion, but suggested)

A reasonable 4-plan split:

- **17-01**: Domain + cache. Extend `domain.Health` to carry per-stage status; add `Orchestrator.GetHealthCache()` + skip-unhealthy logic in the failover loop; tests.
- **17-02**: Liveness probe. New `internal/health/probe.go` with golden-pool runner + tick loop; tests (using fake provider that returns canned responses).
- **17-03**: Metrics + admin handler. Add `provider_health_up` gauge family in `libs/metrics/`; new admin handler exposing `last_ok` snapshot; gateway route; tests.
- **17-04**: Grafana panel + alert rule. New JSON dashboard fragment + alert YAML; smoke-test instructions for the operator.

Single wave only (each plan reasonably small; sequential dependency 17-01 → 17-02 → 17-03 → 17-04 is fine).

### S2 — Test approach

Probe + orchestrator integration test uses a **fake provider** whose `Search/ListEpisodes/...` methods can be programmed to return errors after N calls (so we can drive the 3-consecutive-failures threshold). Real upstream is not tested in-CI; the live probe runs in production via the deployed container.

### S3 — Migration of existing parser metrics

The existing parser metrics (`parser_requests_total{provider="hianime"|"consumet"|"animelib"}`) keep emitting from the legacy parsers (HiAnime, Consumet, AnimeLib) until Phase 20 deletes them. The new `provider="animepahe"` label starts emitting from this phase. No migration; the time series naturally coexist.

### S4 — Failure mode for the probe itself

If the probe goroutine panics, it logs + restarts (defer-recover). If the probe **cannot reach any provider for a full hour**, it logs an explicit error — but does NOT take action (no auto-restart of the service). The Grafana absent() alert catches this case.

</specifics>

<deferred>
## Deferred Ideas

- **Per-user health (e.g., "your country sees AnimePahe as 0")** — out of scope. The probe is server-side, not client-side.
- **Tracing (OTLP/Jaeger)** — out of scope. Metrics + logs only this phase.
- **Self-healing (auto-restart container on absent)** — explicitly not done. Observability does not equal remediation in this phase.
- **Per-user override of "skip unhealthy"** — out of scope. If a provider is gauged unhealthy but the user wants to try anyway, the existing 24-h `preferredScraperProvider` (Phase 16) can override; the orchestrator's skip logic respects an explicit `prefer=` query param.
- **A second probe cadence (e.g., 1-min "shallow" + 15-min "deep")** — deferred. Single 15-min full-pipeline tick is enough for v3.0.
- **Loading the golden pool from DB / admin UI** — deferred. Static config first; database-backed only if the pool needs to grow > 20 entries.
- **Multi-region probe (probe from US + EU egress)** — deferred. Single-egress is enough until we deploy multi-region.

</deferred>
