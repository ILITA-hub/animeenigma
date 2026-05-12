---
phase: 17-observability
plan: 02
subsystem: scraper-observability
tags: [go, prometheus, scraper, observability, probe, goroutine, sliding-window]

# Dependency graph
requires:
  - phase: 17-observability
    plan: 01
    provides: "libs/metrics/provider.go (3 collectors) + services/scraper/internal/health package skeleton (stage constants + InMemoryHealthCache + FakeProvider)"
provides:
  - "services/scraper/internal/health.ProbeRunner — per-provider liveness goroutine on 15-min ± 20% jitter cadence"
  - "services/scraper/internal/health/window.go — 3-of-15-min sliding-window failure counter with clock injection"
  - "services/scraper/internal/health/golden.go — static 5-entry AnimeRef pool + Pick helper"
  - "Real cache wired into scraper main.go (Plan 17-01 left this as nil for backcompat)"
  - "AnimePahe canonical stage keys (search/episodes/servers/stream) — replaces legacy find_id/list_episodes/etc."
  - "First parser_zero_match_total emission (provider=animepahe, selector=episode_list_item) — closes SCRAPER-NF-04"
affects:
  - 17-03-admin-handler (now reads a cache populated by the live probe rather than a fixed snapshot)
  - 17-04-grafana-dashboards (gauges now flip based on real probe data — no longer "no data")
  - "Phase 18+ providers (9anime, AnimeKai) — same probe runner constructor + golden pool reuse"

# Tech tracking
tech-stack:
  added: []  # No new deps; reuses prometheus/client_golang + math/rand/v2 (Go 1.22+)
  patterns:
    - "Per-provider probe goroutine spawned in main.go after Register, before ListenAndServe"
    - "SIGTERM ordering: probeCancel() BEFORE srv.Shutdown() so probes drain cleanly"
    - "Outer defer-recover on ProbeRunner.Start re-spawns on goroutine panic"
    - "Inner defer-recover on runOneTickSafely contains per-tick panics without restarting"
    - "Randomized initial delay (0..interval/4) to prevent boot-time stampede (P-06)"
    - "SCRAPER_PROBE_INITIAL_DELAY_OVERRIDE_SECONDS env hook for fast-verify path"
    - "Clock injection via WithNow(func() time.Time) — tests drive virtual time deterministically"
    - "Typed selector constants (selectorEpisodeListItem etc.) bound parser_zero_match_total cardinality (P-02)"

key-files:
  created:
    - "services/scraper/internal/health/probe.go"
    - "services/scraper/internal/health/probe_test.go"
    - "services/scraper/internal/health/window.go"
    - "services/scraper/internal/health/window_test.go"
    - "services/scraper/internal/health/golden.go"
    - "services/scraper/internal/health/golden_test.go"
  modified:
    - "services/scraper/cmd/scraper-api/main.go"
    - "services/scraper/internal/providers/animepahe/client.go"
    - "services/scraper/internal/providers/animepahe/client_test.go"

key-decisions:
  - "Probe owns the stream_segment stage — AnimePahe provider HealthCheck still exposes only 4 stages (search/episodes/servers/stream); the probe's cache.ProviderHealth exposes 5"
  - "Outer + inner defer-recover (Start + runOneTickSafely) — outer handles fatal goroutine death by re-spawning; inner contains per-tick panics so the loop keeps running. The single-recover model from RESEARCH would either re-spawn on every tick panic (wasteful) or kill the loop (defeats observability)"
  - "MalID == ShikimoriID per the AnimePahe domain contract — domain.AnimeRef has only ShikimoriID; golden.go documents the MalID separately via goldenEntry so probe-pool maintenance can be reasoned about by anime ID"
  - "Page-1 zero-data emits ParserZeroMatchTotal (not every page) — a real selector drift shows up on page 1; later-page zero-data is legitimate end-of-pagination"
  - "Stages skipped due to short-circuit retain last-known gauge via windowSet.IsDown — never silently reset to 1, and never artificially mark as down"

patterns-established:
  - "Phase 17+ probe runners: NewProbeRunner(provider, pool, cache, log, opts...) + go runner.Start(ctx) — uniform constructor for future providers"
  - "Reset metrics in tests via metrics.X.DeleteLabelValues(...) in defer to prevent cross-test bleed without registry teardown"
  - "Selector identifier as typed const (NOT raw CSS) at top of parser file — bounds parser_zero_match_total{selector} cardinality"

# Metrics
metrics:
  duration_seconds: 337
  completed: 2026-05-12T00:00:00Z
  tasks_completed: 4
  files_created: 6
  files_modified: 3
  tests_added: 12  # 5 window + 3 golden + 8 probe (- 4 that overlap behaviors) = 12 net new test functions
  commits: 4
---

# Phase 17 Plan 02: Liveness Probe + Sliding Window + Canonical Stage Keys Summary

**One-liner:** Per-provider liveness probe goroutine on a 15-min ± 20% jitter cadence exercises a 5-stage pipeline (search → episodes → servers → stream → stream_segment) against a 5-entry golden pool, applies a 3-of-15-min sliding-window threshold, and emits `provider_health_up{provider, stage}` + `provider_probe_last_tick_timestamp{provider}` + the first `parser_zero_match_total{provider="animepahe", selector="episode_list_item"}` increment.

## What Was Built

### 9 files (6 created, 3 modified)

| File | Status | Role |
|------|--------|------|
| `services/scraper/internal/health/probe.go` | created | `ProbeRunner` type — per-provider goroutine, 5-stage tick, jitter + recovery + heartbeat |
| `services/scraper/internal/health/probe_test.go` | created | 8 tests under -race: threshold flip, recovery, prune, heartbeat, panic, short-circuit, full-success, truncation |
| `services/scraper/internal/health/window.go` | created | Sliding-window failure counter (3-of-15-min) + windowSet wrapper |
| `services/scraper/internal/health/window_test.go` | created | 5 tests: flip, stay-up, stale-prune, success-reset, race-free |
| `services/scraper/internal/health/golden.go` | created | `DefaultGoldenPool` (5 anime) + `Pick(pool, rng)` |
| `services/scraper/internal/health/golden_test.go` | created | 3 tests: pool size, determinism, MalID/ShikimoriID match |
| `services/scraper/cmd/scraper-api/main.go` | modified | Wire real cache (replace Plan-01 nil); spawn probe goroutines; SIGTERM ordering |
| `services/scraper/internal/providers/animepahe/client.go` | modified | Rename 4 legacy stage keys → canonical; emit `parser_zero_match_total` |
| `services/scraper/internal/providers/animepahe/client_test.go` | modified | Update `TestProvider_HealthCheck` assertions; add `TestProvider_ListEpisodes_ZeroMatchEmitsCounter` |

### Canonical stage names (now emitted by the probe)

The 5 canonical strings appear verbatim as `provider_health_up{stage=...}` labels and in Grafana dashboards / alert rules:

| # | Stage | Owner | Source |
|---|-------|-------|--------|
| 1 | `search` | provider FindID + probe | AnimePahe `markStage(health.StageSearch, ...)` |
| 2 | `episodes` | provider ListEpisodes + probe | AnimePahe `markStage(health.StageEpisodes, ...)` |
| 3 | `servers` | provider ListServers + probe | AnimePahe `markStage(health.StageServers, ...)` |
| 4 | `stream` | provider GetStream + probe | AnimePahe `markStage(health.StageStream, ...)` |
| 5 | `stream_segment` | probe-only | `ProbeRunner.fetchSegment(ctx, Sources[0].URL)` |

### Static golden pool (5 anime, MAL IDs verified 2026-05-12)

| Title | MAL ID | Year |
|-------|--------|------|
| Naruto | 20 | 2002 |
| One Piece | 21 | 1999 |
| Attack on Titan | 16498 | 2013 |
| Demon Slayer | 38000 | 2019 |
| Jujutsu Kaisen | 40748 | 2020 |

### Selector identifier for the first `parser_zero_match_total` emit

- `provider="animepahe"`, `selector="episode_list_item"` — emitted from `ListEpisodes` on page-1 zero-data
- Two additional selector constants pre-defined for future expansion: `selectorServerLink`, `selectorKwikPackedJS`

### Three metrics now emitting real time series

1. `provider_health_up{provider, stage}` — gauge per (provider, stage). 1 = up, 0 = down (after 3-of-15-min threshold)
2. `provider_probe_last_tick_timestamp{provider}` — gauge of last tick Unix ts. Heartbeat for the `absent_over_time(...) > 0` alert
3. `parser_zero_match_total{provider, selector}` — counter. First real emission: `{provider="animepahe", selector="episode_list_item"}` on every ListEpisodes page-1 empty `data` array

## Probe Cadence Notes

- **Base interval:** 15 min
- **Jitter:** ± 20% (so actual sleep ranges 12..18 min)
- **Initial delay:** 0..3.75 min in production (`probeBaseInterval / 4`), randomized
- **Fast-verify:** set `SCRAPER_PROBE_INITIAL_DELAY_OVERRIDE_SECONDS=5` to make the first tick land in 5 s

## Deviations from Plan

### Rule 2 — Auto-add missing critical functionality

**1. [Rule 2] MalID field accessor for goldenEntry**

- **Found during:** Task 1 implementation
- **Issue:** Plan 17-02 acceptance criteria require `MalID: "..."` literal strings to exist in `golden.go`, but `domain.AnimeRef` has no `MalID` field — only `ShikimoriID` (which the domain comment says "== MAL ID" per upstream contract).
- **Fix:** Introduced `goldenEntry` struct in `golden.go` that pairs an `AnimeRef` with a documentation-only `MalID` field. `DefaultGoldenPool` is derived from `goldenEntries[*].Ref`. Probe consumes only `DefaultGoldenPool` — same wire behaviour as the plan, but the MAL IDs stay visibly maintainable for human review and the `MalID:` grep acceptance check passes.
- **Files modified:** `services/scraper/internal/health/golden.go`, `services/scraper/internal/health/golden_test.go`
- **Commit:** f583fce

**2. [Rule 2] Inner panic recovery on each tick**

- **Found during:** Task 2 — designing `Start()` panic semantics
- **Issue:** A single `defer recover()` at the top of `Start()` either (a) re-spawns the goroutine on every per-tick panic (wasteful) or (b) lets the loop die after one bad tick (defeats observability). RESEARCH P-07 covers the outer recover but not the inner.
- **Fix:** Two-tier recover model. Outer `defer recover()` on `Start()` catches goroutine-fatal panics and re-spawns. Inner `runOneTickSafely(ctx)` wraps each tick in its own `defer recover()` so a single panicking provider doesn't crash the loop or trigger re-spawn churn.
- **Files modified:** `services/scraper/internal/health/probe.go`
- **Tests:** `TestProbe_PanicInProviderRecovers` exercises the inner recover path
- **Commit:** aaf4d1c

### Rule 3 — Auto-fix blocking issues

**3. [Rule 3] Removed unused-binary artifact from worktree root**

- **Found during:** Task 3 — after `go build`
- **Issue:** `go build ./services/scraper/...` left a `scraper-api` binary at the worktree root, which `git status` would have committed as an untracked artifact.
- **Fix:** Deleted before staging.
- **Files modified:** N/A (deletion of unstaged file)

### Auto-fixed Issues — None other

All other behavior matches the plan verbatim.

## Tests Added

| File | Tests | Coverage |
|------|-------|----------|
| `window_test.go` | 5 | threshold flip / under-threshold / stale-prune / success-reset / -race concurrency |
| `golden_test.go` | 3 | pool size invariant (5..10) / deterministic Pick with seeded PCG / MalID == ShikimoriID contract |
| `probe_test.go` | 8 | 3-failure flip / recovery / stale-prune / heartbeat / panic recovery / short-circuit / 5-stage full success / 256-char truncation |
| `client_test.go` | 1 net new (+ 1 updated assertion) | `TestProvider_ListEpisodes_ZeroMatchEmitsCounter` anchors SCRAPER-NF-04; `TestProvider_HealthCheck` now asserts canonical keys + negative legacy keys |

All tests pass under `go test ... -race -count=1`.

## Authentication Gates / Live-Verification Status

This plan was executed inside a worktree, so per parallel_execution rules **the live-verification step was deferred**:

- **Test suite:** PASS — `go test ./services/scraper/... ./libs/metrics/... -count=1 -race -timeout=180s` returns 0 across all packages.
- **Build:** PASS — `go build ./services/scraper/... ./libs/metrics/... ./libs/logger/...` returns 0.
- **`make redeploy-scraper`:** DEFERRED to the orchestrator's post-merge step. The worktree filesystem must NOT touch the live infra (per the executor brief: "doing a live verify by copying files to main creates merge conflicts"). After the orchestrator merges this branch into `main`, the standard `/animeenigma-after-update` skill should be invoked to:
  1. `make redeploy-scraper`
  2. Wait for the first probe tick (bounded polling loop in PLAN Task 4 step 4, or set `SCRAPER_PROBE_INITIAL_DELAY_OVERRIDE_SECONDS=5` in docker-compose for fast-verify)
  3. Confirm `provider_health_up{provider="animepahe"}` shows 5 series + `provider_probe_last_tick_timestamp{provider="animepahe"}` is non-zero
  4. Verify Grafana dashboard from Plan 17-04 lights up

## Threat Surface

No new threat surface beyond what was declared in the plan's `<threat_model>`. The probe goroutine itself adds an upstream-traffic source (T-17-02-03) — explicitly accepted per D8 — but the per-host rate limiter from SCRAPER-FOUND-06 caps the amplification.

## Compile-Time Invariants Verified

- No `iframe_url` field appears in any new Go type (`grep -r iframe services/scraper/internal/health/` returns 0 — D3 contract preserved)
- `domain.AnimeRef` unchanged — `goldenEntry` documents MalID without mutating the shared type
- `domain.Stream` unchanged — `TestStream_HasNoIframeURL` not affected
- Legacy stage keys (`"find_id"`, `"list_episodes"`, `"list_servers"`, `"get_stream"`) appear ZERO times in the modified `client.go`

## Self-Check: PASSED

- `services/scraper/internal/health/probe.go`: FOUND
- `services/scraper/internal/health/probe_test.go`: FOUND
- `services/scraper/internal/health/window.go`: FOUND
- `services/scraper/internal/health/window_test.go`: FOUND
- `services/scraper/internal/health/golden.go`: FOUND
- `services/scraper/internal/health/golden_test.go`: FOUND
- `services/scraper/cmd/scraper-api/main.go`: MODIFIED
- `services/scraper/internal/providers/animepahe/client.go`: MODIFIED
- `services/scraper/internal/providers/animepahe/client_test.go`: MODIFIED
- Commits: f583fce (Task 1), aaf4d1c (Task 2), 52ebbfb (Task 3), this commit (SUMMARY + Task 4 docs)
