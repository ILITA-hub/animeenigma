---
phase: 11-observability-prediction
plan: 01
subsystem: infra
tags: [prometheus, grafana, scheduler, gorm, cron, observability, metrics]

# Dependency graph
requires:
  - phase: 09-download-triggers
    provides: "autocache_logic_a.go — the shared-DB DISTINCT watcher/combo join + the robfig/cron registration + SchedulerJob metrics wrap mirrored here"
  - phase: 10
    provides: "library_autocache_budget_bytes + the avg_raw_ep_size constant the prediction gauge is charted against and mirrors"
provides:
  - "library_autocache_predicted_bytes{component=ongoing|nextep} gauge on the scheduler /metrics endpoint (the one new backend piece of Phase 11)"
  - "AutocachePredictionJob — daily cron computing the §7 v1 storage-need heuristic (two distinct-anime counts x avgRawEpBytes)"
  - "AUTOCACHE_PREDICTION_CRON (0 4 * * *) + AUTOCACHE_AVG_RAW_EP_BYTES (~1.2 GiB) scheduler env mirrors + getEnvInt64 helper"
affects: [11-02-grafana-obs-panels]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Leaner-sibling cron job: clone Logic A's struct/join/registration but COUNT rows + set a gauge instead of per-row HTTP fan-out"
    - "promauto auto-registration: a new GaugeVec in libs/metrics is scraped on scheduler /metrics with zero plumbing"
    - "Cross-service metric name (library_autocache_ prefix on a scheduler-emitted gauge) so Grafana can union it with the library-exposed budget gauge"

key-files:
  created:
    - services/scheduler/internal/jobs/autocache_prediction.go
    - services/scheduler/internal/jobs/autocache_prediction_test.go
  modified:
    - libs/metrics/scheduler.go
    - services/scheduler/internal/config/config.go
    - services/scheduler/internal/service/job.go
    - services/scheduler/internal/service/job_test.go
    - services/scheduler/cmd/scheduler-api/main.go

key-decisions:
  - "Two count(*) subqueries over the verbatim Logic A DISTINCT join (ongoing keeps a.status='ongoing'; nextep drops it) — Go-computed cutoff bound as ? keeps it Postgres+SQLite portable"
  - "Prediction job registered + constructed UNCONDITIONALLY (no optional URL — reads shared DB only), unlike Logic A's nil-on-empty-URL guard"
  - "Gauge cardinality kept {component}-only (2 series); per-anime breakdown deferred to v2"
  - "Added a getEnvInt64 config helper rather than casting an int, so the ~1.2 GiB byte default is correct on 32-bit builds"

patterns-established:
  - "Pattern: heuristic-prediction gauge owned by the shared-DB service (scheduler), named with the consuming service's prefix so Grafana can join across /metrics scrapes"

requirements-completed: [OBS-05]

# Metrics
duration: 5min
completed: 2026-06-17
---

# Phase 11 Plan 01: Scheduler Daily Prediction Job Summary

**Daily scheduler cron emitting `library_autocache_predicted_bytes{component=ongoing|nextep}` = two distinct-anime watcher-join counts × avg_raw_ep_size, the one new backend piece OBS-05's Grafana table needs.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-06-17T09:58:53Z
- **Completed:** 2026-06-17T10:03:45Z
- **Tasks:** 2
- **Files modified:** 5 (+2 created)

## Accomplishments
- `AutocachePredictedBytes` GaugeVec in `libs/metrics/scheduler.go` ({component}-only cardinality, `library_autocache_` prefix so OBS-05 can union it with the library-exposed `library_autocache_budget_bytes`).
- `AutocachePredictionJob`: runs the verbatim Logic A DISTINCT join two ways — `ongoing` (keeps `a.status='ongoing'`) and `nextep` (drops it) — wraps each in `count(*)`, multiplies by `avgRawEpBytes`, and sets the `{component}` gauge. No per-row HTTP fan-out; returns an error only on a query failure.
- Config: new `getEnvInt64` helper + `AUTOCACHE_PREDICTION_CRON` (default `0 4 * * *`) + `AUTOCACHE_AVG_RAW_EP_BYTES` (default 1288490188 ≈ 1.2 GiB) env mirrors.
- Wiring: unconditional construction in `main.go`, threaded through `NewJobService` + `Start`, registered on the cron with the standard `SchedulerJob{ExecutionsTotal,Duration,LastSuccess}` metrics wrap (label `autocache_prediction`), surfaced in `GetStatus`.
- SQLite-seeded tests cover ongoing-count, nextep-drops-ongoing-clause, non-JP/stale filters, zero-rows-sets-0-returns-nil, and the join-failure error contract.

## Task Commits

Each task was committed atomically:

1. **Task 1: Prediction GaugeVec + config + AutocachePredictionJob** - `c7c16a48` (feat) — gauge, getEnvInt64 + cron/avg-bytes config, job + SQLite tests (RED→GREEN in one cohesive TDD unit).
2. **Task 2: Register the job + wire DI** - `753e4b7e` (feat) — job.go registration block (label `autocache_prediction`) + GetStatus key + unconditional main.go DI + job_test.go arity/assertions.

_Plan metadata commit follows this SUMMARY._

## Files Created/Modified
- `libs/metrics/scheduler.go` - Added `AutocachePredictedBytes` GaugeVec ({component} label).
- `services/scheduler/internal/config/config.go` - `getEnvInt64` helper; `AutocachePredictionCron` + `AutocacheAvgRawEpBytes` fields + Load wiring.
- `services/scheduler/internal/jobs/autocache_prediction.go` - New `AutocachePredictionJob` (two-variant DISTINCT-join counts → gauge).
- `services/scheduler/internal/jobs/autocache_prediction_test.go` - New SQLite-seeded behavior tests (5 cases).
- `services/scheduler/internal/service/job.go` - Struct field + last-run tracker, constructor param, Start param, registration block, GetStatus entry.
- `services/scheduler/internal/service/job_test.go` - Updated arity for the two existing tests + a new `TestJobService_RegistersAutocachePrediction`.
- `services/scheduler/cmd/scheduler-api/main.go` - Unconditional job construction + threading through `NewJobService`/`Start`.

## Decisions Made
- **Count subqueries over row-scan:** wrapped the DISTINCT projection in `SELECT count(*) FROM (...) t` for each component rather than scanning full rows + `len()`. Cleaner, DB-portable, and avoids carrying Logic A's `episodes_aired`/`shikimori_id` projection that the prediction never needs.
- **`getEnvInt64` helper added** (deviation-adjacent gap flagged in 11-PATTERNS "No Analog Found") for byte-quantity correctness on 32-bit.
- **Unconditional registration** (vs Logic A's nil-guard) — kept the nil-check in `Start` for symmetry/panic-safety, but `main.go` always constructs it since there is no optional URL dependency.

## Deviations from Plan

None - plan executed exactly as written.

(One in-scope clarity adjustment, not a behavior deviation: reworded the config field doc-comment to avoid a second literal `1288490188` so the acceptance grep `== 1` holds precisely. No functional change.)

## Issues Encountered
None. Both verification gates (`go build`/`go vet`/`go test` for `services/scheduler` and `go build`/`go vet` for `libs/metrics`) passed clean on first full run after each task.

## User Setup Required
None for the heuristic logic. Two optional env overrides exist with sane defaults baked in: `AUTOCACHE_PREDICTION_CRON` (default `0 4 * * *`) and `AUTOCACHE_AVG_RAW_EP_BYTES` (default `1288490188`). The gauge appears on the existing scheduler `/metrics` (already Prometheus-scraped) with zero new ingress.

## Next Phase Readiness
- **Plan 11-02 (Grafana OBS panels) unblocked:** `library_autocache_predicted_bytes{component="ongoing"|"nextep"}` is now emitted and queryable alongside `library_autocache_budget_bytes`, ready for the OBS-05 table panel and the rest of the Autocache Pool dashboard row.
- No blockers.

---
*Phase: 11-observability-prediction*
*Completed: 2026-06-17*

## Self-Check: PASSED

All 7 created/modified source files exist on disk and both task commits (`c7c16a48`, `753e4b7e`) are present in the git log. `services/scheduler` and `libs/metrics` build, vet, and test green.
