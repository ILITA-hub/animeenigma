---
phase: 17-observability
plan: 01
subsystem: infra
tags: [go, prometheus, scraper, observability, health-check, rwmutex]

# Dependency graph
requires:
  - phase: 16
    provides: "AnimePahe provider + orchestrator failover loop + parser_fallback_total metric"
provides:
  - "Prometheus gauge family provider_health_up{provider, stage} (5 canonical stages)"
  - "Prometheus gauge provider_probe_last_tick_timestamp{provider} (heartbeat for absent()-style alert)"
  - "Prometheus counter parser_zero_match_total{provider, selector} (fills SCRAPER-NF-04 gap)"
  - "services/scraper/internal/health package: stage constants + InMemoryHealthCache + FakeProvider"
  - "Orchestrator constructor takes optional *health.InMemoryHealthCache (nil = Phase 16 behaviour)"
  - "Orchestrator.RegisteredProviders() snapshot accessor"
  - "runFailover skip-unhealthy branch (SCRAPER-OBS-03)"
affects:
  - 17-02-probe-runner
  - 17-03-admin-handler
  - 17-04-grafana-dashboards
  - "Phase 18 (9anime provider — will register against the same gauge family)"

# Tech tracking
tech-stack:
  added: []  # No new deps; uses existing prometheus/client_golang + sync.RWMutex
  patterns:
    - "Fail-open in-memory cache (60s TTL) — RESEARCH P-08"
    - "Snapshot-then-release locking discipline — REVIEW.md CR-02 preserved across new methods"
    - "Versioned stage-string contract (search/episodes/servers/stream/stream_segment) — dashboard-locked"
    - "Boot-time gauge seeding so HELP/TYPE lines appear in /metrics from start"

key-files:
  created:
    - "libs/metrics/provider.go"
    - "libs/metrics/provider_test.go"
    - "services/scraper/internal/health/stage.go"
    - "services/scraper/internal/health/stage_test.go"
    - "services/scraper/internal/health/cache.go"
    - "services/scraper/internal/health/cache_test.go"
    - "services/scraper/internal/health/testutil_provider.go"
  modified:
    - "services/scraper/internal/domain/provider.go"
    - "services/scraper/internal/service/orchestrator.go"
    - "services/scraper/internal/service/orchestrator_test.go"
    - "services/scraper/cmd/scraper-api/main.go"
    - "services/scraper/internal/handler/scraper_test.go"
    - "services/scraper/internal/transport/router_test.go"
    - "frontend/web/public/changelog.json"

key-decisions:
  - "Cache fail-open semantics: missing/stale entries return IsHealthy=true so a probe outage cannot blank the service (RESEARCH P-08 + D3)"
  - "5 canonical stage strings are a versioned dashboard contract — never rename without coordinated Grafana/alert changes"
  - "nil-cache constructor path preserves Phase 16 dispatch verbatim — all 14 existing tests stay green; cache wiring is additive"
  - "All-skipped chains return ErrProviderDown (not ErrNotFound) so callers can distinguish 'providers exist but gated unhealthy' from 'no provider has this anime'"
  - "Seed gauge families at scraper boot with zero-valued children so Prometheus exposition emits HELP/TYPE lines before the probe runner ticks (Plan 17-02)"

patterns-established:
  - "Test fake promotion pattern: FakeProvider moved from _test.go to testutil_provider.go so multiple test packages share one shape"
  - "Generic-fn optional-arg pattern: runFailover takes *cache as 4th positional, nil disables the new branch (cheaper than two overloads)"

requirements-completed:
  - SCRAPER-OBS-02
  - SCRAPER-OBS-03
  - SCRAPER-NF-04

# Metrics
duration: 16min
completed: 2026-05-12
---

# Phase 17 Plan 01: Foundation — Provider Health Metrics + Cache + Skip-Unhealthy Wiring Summary

**Prometheus gauge family `provider_health_up{provider, stage}` + 60s fail-open in-memory cache + orchestrator runFailover skip-unhealthy branch wired through with nil-cache backcompat for Phase 16.**

## Performance

- **Duration:** 16 min
- **Started:** 2026-05-12T11:17:57Z
- **Completed:** 2026-05-12T11:33:56Z
- **Tasks:** 4
- **Files modified:** 14 (7 new, 7 modified)

## Accomplishments

- Three new Prometheus collectors registered against the default registry and visible on `curl :8088/metrics` with HELP/TYPE lines from boot:
  - `provider_health_up{provider, stage}` (gauge)
  - `provider_probe_last_tick_timestamp{provider}` (gauge)
  - `parser_zero_match_total{provider, selector}` (counter — fills SCRAPER-NF-04)
- New `services/scraper/internal/health/` package with 5 canonical stage constants (versioned dashboard contract), RWMutex-guarded in-memory cache with 60-second fail-open TTL, and a shared `FakeProvider` for cross-package tests.
- Orchestrator constructor extended to take `*health.InMemoryHealthCache` (nil = Phase 16 dispatch verbatim).
- `runFailover` skip-unhealthy branch implements SCRAPER-OBS-03: cache reads DOWN → skip provider, increment `parser_fallback_total{from, to}`, continue. All-skipped chains surface `ErrProviderDown` (not `ErrNotFound`).
- `Orchestrator.RegisteredProviders()` accessor exported for the probe runner (Plan 17-02) and admin handler (Plan 17-03).
- Scraper service redeployed via `make redeploy-scraper`; `make health` confirms all 8 services healthy; `/scraper/health` returns 200 with the existing schema unchanged.

## Task Commits

Each task was committed atomically:

1. **Task 1: Define Prometheus collectors + canonical stage constants** — `07dad56` (test+feat — TDD: RED test file written first, then GREEN provider.go + stage.go)
2. **Task 2: In-memory health cache + FakeProvider test fake** — `75d0bfc` (feat — cache.go + cache_test.go + testutil_provider.go; 7 tests including 100-reader + 1-writer race test)
3. **Task 3: Domain doc + orchestrator skip + main.go nil-passthrough** — `5d6c45e` (feat — orchestrator constructor signature change, runFailover skip branch, 5 new orchestrator tests, plus 3 call-site fixes in handler/transport tests)
4. **Task 4: Seed metrics + admin changelog + deploy verify** — `a539b94` (chore — boot-time gauge seeding so HELP lines appear pre-probe + Russian admin-facing changelog entry)

_Plan metadata commit (this SUMMARY.md) will be added immediately after this file is created._

## Files Created/Modified

### Created (7 new files)

- `libs/metrics/provider.go` — three new collectors (`ProviderHealthUp`, `ProviderProbeLastTick`, `ParserZeroMatchTotal`) registered via `promauto` against the default registry, mirroring `libs/metrics/player_health.go`.
- `libs/metrics/provider_test.go` — asserts metric name + label cardinality for each collector via `prometheus.Desc.String()` parsing + `testutil.ToFloat64`.
- `services/scraper/internal/health/stage.go` — five canonical stage constants (`StageSearch` → `StageStreamSegment`) plus `AllStages` slice in execution order.
- `services/scraper/internal/health/stage_test.go` — locks slice length=5, exact order, and constant-string identity.
- `services/scraper/internal/health/cache.go` — `StageStatus` + `ProviderHealth` DTOs, `InMemoryHealthCache` with `NewInMemoryHealthCache` + `WithNow` test constructor, `IsHealthy` (4 fail-open branches), `Update`, and deep-copy `AdminSnapshot`. `MaxLastErrChars = 256` constant for probe-side truncation contract (T-17-01-02 mitigation).
- `services/scraper/internal/health/cache_test.go` — 8 tests covering no-entry, fresh up/down, stale (>60s) fail-open, missing-stream_segment fail-open, AdminSnapshot deep-copy, 100-reader + 1-writer race-safety, and FakeProvider interface satisfaction.
- `services/scraper/internal/health/testutil_provider.go` — `FakeProvider` (programmable `domain.Provider` impl) lifted from `orchestrator_test.go` so probe tests (Plan 17-02) and other packages can share one fake. Compile-time interface assertion at package scope.

### Modified (7 files)

- `services/scraper/internal/domain/provider.go` — `Health` doc comment now points at the five canonical stage strings and flags them as a versioned dashboard contract.
- `services/scraper/internal/service/orchestrator.go` — adds `cache` field + `health` import; `NewOrchestrator` takes a 3rd arg; `runFailover` threads cache through and inserts skip branch (logs at Debug, emits `parser_fallback_total`, wraps skip error with `domain.ErrProviderDown` so `summarizeFailover` correctly classifies all-skipped chains); adds `RegisteredProviders()` accessor.
- `services/scraper/internal/service/orchestrator_test.go` — updates `newTestOrchestrator` helper to pass nil cache; adds `newTestOrchestratorWithCache` builder; adds 5 new tests (NilCache_Backcompat, SkipsUnhealthyProvider, RejoinsHealthyProvider, AllProvidersDown_ReturnsAggregateError, StaleCache_DoesNotSkip).
- `services/scraper/cmd/scraper-api/main.go` — passes `nil` for the new cache arg (Plan 17-02 wires the real cache) + seeds the three new gauge families at boot so HELP/TYPE lines surface in `/metrics` immediately.
- `services/scraper/internal/handler/scraper_test.go` — 3 call sites updated to pass `nil` cache (Rule 3 auto-fix; see Deviations).
- `services/scraper/internal/transport/router_test.go` — 1 call site updated to pass `nil` cache (Rule 3 auto-fix).
- `frontend/web/public/changelog.json` — adds one Russian `improvement` entry under `2026-05-12` flagging the new admin telemetry. Plan 17-04 will add the user-facing entry once dashboards are wired.

## Decisions Made

- **Fail-open over fail-closed for the cache** — RESEARCH P-08 + D3 make this the dominant choice: a probe outage must not blank the service. Four code branches (no entry, stale entry, no `stream_segment` key, `Up=true`) all return `true`; only the single positive-evidence branch returns `false`.
- **`runFailover` parameter, not closure** — picked option (i) from the plan's hint: cache is a 4th positional arg threaded through all four `Orchestrator` method call sites. Mechanical, explicit, no hidden state on the function value.
- **Skip-failure surfaces `ErrProviderDown`** — wrapping the skip error with `fmt.Errorf(..., "%w", domain.ErrProviderDown)` lets `summarizeFailover` classify an all-skipped chain correctly. Without the wrap, all-skipped would degrade to `ErrNotFound`, which would be a wrong signal.
- **Boot-time gauge seeding** — Prometheus exposition suppresses metric families with no labeled children, but the acceptance criteria require HELP lines pre-probe. Seeding `provider_health_up{stage=*} = 1` is the optimistic default consistent with "no probe yet, assume healthy"; the probe overwrites in Plan 17-02. Parser-zero-match uses a sentinel `selector="_seeded"` label that becomes background noise once real selectors land.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] Updated handler + transport test call sites for new `NewOrchestrator` signature**
- **Found during:** Task 3 (post-`go vet` check)
- **Issue:** `services/scraper/internal/handler/scraper_test.go` (3 call sites) and `services/scraper/internal/transport/router_test.go` (1 call site) still passed 2 args to `service.NewOrchestrator`, blocking the package build.
- **Fix:** Added explicit `nil` as the third argument, with an inline comment noting "Phase 17: nil cache preserves Phase 16 dispatch behaviour for handler/router tests".
- **Files modified:** `services/scraper/internal/handler/scraper_test.go`, `services/scraper/internal/transport/router_test.go`
- **Verification:** `go test ./services/scraper/... -count=1 -race` exits 0
- **Committed in:** `5d6c45e` (Task 3)

**2. [Rule 2 — Missing critical functionality] Boot-time gauge seeding so Prometheus exposition shows HELP lines**
- **Found during:** Task 4 (first `curl :8088/metrics` after `make redeploy-scraper` showed zero HELP lines)
- **Issue:** `promauto.NewGaugeVec` registers the metric family with the default registry, but the Prometheus exposition format suppresses families with zero labeled children. The plan's Task 4 acceptance criterion requires all three HELP lines to be visible after redeploy, but no time series exist until the probe runs (Plan 17-02). Without this fix, dashboards and alert rules referencing the new metric family would 404-on-discovery during the Phase 17-01 → 17-02 → 17-04 window.
- **Fix:** Added a small init block in `main.go` after orchestrator + provider registration: for every registered provider, seed `ProviderHealthUp{provider, stage} = 1` across all five stages (optimistic default), `ProviderProbeLastTick{provider} = 0` (never ticked), and a `ParserZeroMatchTotal{provider, "_seeded"} Add(0)` so the counter family appears too.
- **Files modified:** `services/scraper/cmd/scraper-api/main.go` (+ added `services/scraper/internal/health` import)
- **Verification:** `curl :8088/metrics | grep -cE "^# HELP (provider_health_up|provider_probe_last_tick_timestamp|parser_zero_match_total)"` returns 3.
- **Committed in:** `a539b94` (Task 4)

---

**Total deviations:** 2 auto-fixed (1 blocking signature-change fixup, 1 missing-critical metric exposition)
**Impact on plan:** Both auto-fixes are essential. The signature change made test packages refuse to build; without the boot seed, the Task 4 metric-exposition acceptance criterion fails. No scope creep — both changes are inside the file set the plan already declared.

## Issues Encountered

- **Worktree path confusion early in execution.** First pass wrote files to `/data/animeenigma/libs/metrics/...` (main repo path) instead of `/data/animeenigma/.claude/worktrees/agent-.../libs/metrics/...` (worktree path) because shell `cd` resets between Bash invocations and the Write tool used absolute paths against the wrong root. Caught before any commits — main-repo path was cleaned (`git reset HEAD` + file removal), and all subsequent Writes used the worktree-absolute path. Production state was never affected because no commits landed in main.
- **`make redeploy-scraper` from the worktree shares the docker-compose project name with the main repo.** Both compose files derive project name `docker` from their parent directory. Running the deploy from the worktree replaced the running `animeenigma-scraper` container in place — the new image was built from the worktree's source (including my new files), and the container picked it up cleanly. No drift; only one scraper container exists at any time on this host. Documenting here in case future worktree agents touch shared services concurrently.

## User Setup Required

None — no external service configuration required. All metrics are emitted via the existing `/metrics` endpoint Prometheus already scrapes (per `docker/prometheus/prometheus.yml` job `animeenigma-scraper`).

Plan 17-04 will add the Grafana dashboard JSON + alert YAML; that plan has its own user-setup notes for the dashboard import + alert rule confirmation in the Telegram-bound channel.

## Self-Check: PASSED

Verified before writing this SUMMARY:

- File existence (all 7 created files + main.go modifications): all FOUND on disk in the worktree at `/data/animeenigma/.claude/worktrees/agent-a41c4e6b0869b4ee1/...`.
- Commits exist in worktree history: `07dad56`, `75d0bfc`, `5d6c45e`, `a539b94` — all visible in `git log --oneline -5`.
- Test suite: `go test ./services/scraper/... ./libs/metrics/... -count=1` → all packages OK; orchestrator + health + metrics all pass under `-race`.
- Deploy: `make redeploy-scraper` succeeded, `make health` reports all 8 services UP, `curl :8088/metrics | grep -c "^# HELP (provider_health_up|provider_probe_last_tick_timestamp|parser_zero_match_total)"` returns 3.

## Next Phase Readiness

**Plan 17-02 (probe runner) is unblocked.** The data contract is in place:

- `health.InMemoryHealthCache` is the destination — the probe just calls `cache.Update(provider, ProviderHealth{...})`.
- `health.AllStages` is the iteration order — short-circuit on first failure per RESEARCH guidance.
- `metrics.ProviderHealthUp.WithLabelValues(p.Name(), stage).Set(0|1)` is the gauge write.
- `metrics.ProviderProbeLastTick.WithLabelValues(p.Name()).SetToCurrentTime()` is the heartbeat write.
- `Orchestrator.RegisteredProviders()` enumerates the providers the probe should iterate.

**Plan 17-03 (admin handler)** can consume `cache.AdminSnapshot()` (deep-copy) — the handler can mutate freely (e.g. re-truncate `LastErr`) without affecting the live cache.

**No blockers; no concerns.** The single open follow-up is Plan 17-02's probe runner, which the next agent in the next wave will execute.

---
*Phase: 17-observability*
*Completed: 2026-05-12*
