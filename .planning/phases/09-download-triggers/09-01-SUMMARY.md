---
phase: 09-download-triggers
plan: 01
subsystem: library
tags: [autocache, migrations, gorm, repo, prometheus, single-flight, demand-reason, internal-endpoint, docker-network]
requires:
  - phase: 08-01
    provides: "DemandRepository.Record + DemandReasonBackfill/NextEp; AutocacheDemand model; migration 007 (autocache_demand + autocache_demand_reason enum); LibraryMetrics serve_total shape"
  - phase: 08-02
    provides: "AutocacheInternalHandler.Demand (the force-backfill handler Task 4 inverts); /internal/library/autocache/demand Docker-network-only route (T-08-04)"
  - phase: 07-02
    provides: "AutocacheConfigStore/ConfigEnabled master switch; library_jobs/library_episodes schema; job_source enum"
provides:
  - "migration 008 — job_source enum accepts 'autocache' (Planner provenance)"
  - "migration 009 — library_jobs.episode INT nullable (intended-episode single-flight key)"
  - "migration 010 — autocache_demand_reason enum accepts 'ongoing' (Logic A)"
  - "domain.Job.Episode *int (nullable column:episode mirror)"
  - "domain.DemandReasonOngoing = 'ongoing'"
  - "JobRepository.HasActiveForEpisode — non-terminal (shikimori_id,episode) single-flight gate (TRIG-04)"
  - "DemandRepository.Drain(limit) — FIFO requested_at ASC bounded batch (TRIG-03)"
  - "DemandRepository.Delete(malID,episode) — no-op on absent row (TRIG-03)"
  - "library_autocache_downloads_total{trigger,result} CounterVec + IncDownloadsTotal + test seam (OBS-04)"
  - "Demand handler honors validated wire reason (validateDemandReason): next_ep/ongoing/backfill honored, absent/invalid → backfill — A/B/backfill attribution correct by construction"
affects:
  - "09-02 (library Planner drain loop: imports HasActiveForEpisode + Drain/Delete + IncDownloadsTotal + Job.Episode + job_source='autocache'; applies migrations 008/009/010 in main.go)"
  - "09-03 (player Logic B producer: fires reason='next_ep', now honored)"
  - "09-04 (scheduler Logic A producer: fires reason='ongoing', now honored)"
tech-stack:
  added: []
  patterns:
    - "Idempotent enum ADD VALUE / column ADD COLUMN migrations embedded via go:embed; main.go apply wiring deferred to the consuming plan (09-02)"
    - "Non-terminal-status single-flight gate via `status NOT IN (terminal)` COUNT (TRIG-04)"
    - "Bounded FIFO drain primitive (Order requested_at ASC + Limit) so the consumer caps batch size (T-09-02 DoS guard)"
    - "Wire-reason validation allowlist with safe default on an internal-trusted Docker-network-only path (validate-and-honor, replacing force-override)"
key-files:
  created:
    - services/library/migrations/008_autocache_job_source.sql
    - services/library/migrations/009_library_jobs_episode.sql
    - services/library/migrations/010_autocache_demand_ongoing.sql
  modified:
    - services/library/migrations/migrations.go
    - services/library/internal/domain/job.go
    - services/library/internal/domain/autocache_demand.go
    - services/library/internal/domain/autocache_demand_test.go
    - services/library/internal/repo/job.go
    - services/library/internal/repo/job_test.go
    - services/library/internal/repo/demand.go
    - services/library/internal/repo/demand_test.go
    - services/library/internal/metrics/library_metrics.go
    - services/library/internal/metrics/library_metrics_test.go
    - services/library/internal/handler/autocache_internal.go
    - services/library/internal/handler/autocache_internal_test.go
key-decisions:
  - "migrations.go embeds 008/009/010 but main.go is NOT touched — the apply wiring is Plan 09-02's responsibility (alongside the Planner DI), per the plan's task boundary"
  - "Job.Episode is *int (nullable pointer), so admin/manual rows write NULL (not 0) — the single-flight key is only meaningful for Planner-enqueued autocache rows"
  - "HasActiveForEpisode EXCLUDES terminal statuses so a previously-failed job never permanently blocks a re-enqueue (a failed job goes terminal → next drain re-attempts)"
  - "Drain is lifecycle-agnostic (READ only); the Planner decides when to Delete a satisfied row (RESEARCH Pitfall 6 — delete on confirmed presence, not speculatively)"
  - "Demand handler reason is now VALIDATED-AND-HONORED, not force-overridden: safe because /internal/* is Docker-network-only (T-08-04), so callers are internal-trusted; absent/unknown still defaults to backfill (T-09-15)"
patterns-established:
  - "Pattern 1: schema-foundation plan ships migrations + domain mirror + repo primitives + metric, with no main.go apply and no behavior change to the pipeline — every downstream task imports the contracts"
  - "Pattern 2: reason-seam correctness-by-construction — the producers (09-03/04) need no follow-up because the handler already honors their true reason"
requirements-completed: [TRIG-03, TRIG-04, TRIG-05]
duration: ~8min
completed: 2026-06-17
---

# Phase 9 Plan 01: Download-Trigger Foundation (library schema + repo + metric + reason-seam fix) Summary

**Three idempotent migrations (job_source 'autocache', library_jobs.episode INT, demand reason 'ongoing'), the GORM mirrors + single-flight/drain repo primitives, the library_autocache_downloads_total{trigger,result} counter, and the Phase-8 demand handler inverted to HONOR the producer's validated wire reason instead of force-collapsing every demand to backfill.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-06-17T07:34:24Z
- **Completed:** 2026-06-17T07:42:00Z
- **Tasks:** 4 (2 TDD)
- **Files modified:** 12 (3 created, 9 modified)

## Accomplishments
- Widened the library schema for Phase-9 autocache: `autocache` job_source, nullable `library_jobs.episode`, `ongoing` demand reason — all idempotent (`IF NOT EXISTS`), embedded in migrations.go, main.go untouched (09-02 applies them).
- Added the GORM mirrors (`Job.Episode *int`, `DemandReasonOngoing`) and the three repo primitives the Planner builds on: `JobRepository.HasActiveForEpisode` (TRIG-04 single-flight), `DemandRepository.Drain`/`Delete` (TRIG-03 drain).
- Added the `library_autocache_downloads_total{trigger,result}` counter (OBS-04) with a nil-guarded incrementer + test seam.
- Inverted the Phase-8 demand handler: `validateDemandReason` allowlists {backfill,next_ep,ongoing} and honors the wire reason (absent/invalid → backfill), so Logic A/B/backfill attribution is correct by construction and the new `ongoing` enum is reachable end-to-end.

## Task Commits

Each task was committed atomically (TDD tasks split test → feat):

1. **Task 1: Migrations 008/009/010 + embeds** - `c880ec7d` (feat)
2. **Task 2: Job.Episode, DemandReasonOngoing, HasActiveForEpisode, Drain/Delete** - `2f2e3d94` (test, RED) → `a3ee6cee` (feat, GREEN)
3. **Task 3: library_autocache_downloads_total{trigger,result} counter** - `0b5fbc53` (feat)
4. **Task 4: Demand handler honors validated wire reason** - `2ddb5ba3` (test, RED) → `d7014165` (feat, GREEN)

**Plan metadata:** committed in this SUMMARY commit (docs).

## Files Created/Modified
- `services/library/migrations/008_autocache_job_source.sql` - `ALTER TYPE job_source ADD VALUE IF NOT EXISTS 'autocache'`
- `services/library/migrations/009_library_jobs_episode.sql` - `ALTER TABLE library_jobs ADD COLUMN IF NOT EXISTS episode INT` (nullable)
- `services/library/migrations/010_autocache_demand_ongoing.sql` - `ALTER TYPE autocache_demand_reason ADD VALUE IF NOT EXISTS 'ongoing'`
- `services/library/migrations/migrations.go` - 3 go:embed vars (AutocacheJobSourceSQL/LibraryJobsEpisodeSQL/AutocacheDemandOngoingSQL) + apply-order doc entries 7/8/9
- `services/library/internal/domain/job.go` - `Episode *int` field (column:episode)
- `services/library/internal/domain/autocache_demand.go` - `DemandReasonOngoing = "ongoing"` const
- `services/library/internal/repo/job.go` - `HasActiveForEpisode` single-flight gate
- `services/library/internal/repo/demand.go` - `Drain(limit)` + `Delete(malID,episode)`
- `services/library/internal/metrics/library_metrics.go` - `autocacheDownloadsTotal` CounterVec + `IncDownloadsTotal` + `GetDownloadsTotalForTest`
- `services/library/internal/handler/autocache_internal.go` - `validateDemandReason` + Demand() records the validated wire reason
- `*_test.go` (domain/repo/metrics/handler) - reflection signature pins, source tripwires, table-driven reason-honoring + inverted backfill assertion

## Decisions Made
- See `key-decisions` frontmatter. Headline: migrations are embedded but NOT applied in main.go (09-02's job); the demand reason is now validated-and-honored (not force-overridden) because the route is Docker-network-only and the producers are internal-trusted, with backfill as the safe default on absent/unknown.

## Deviations from Plan

None - plan executed exactly as written. No bugs, missing functionality, blocking issues, or architectural changes encountered. No package installs (T-09-SC N/A — stdlib + existing gorm/prometheus only; RESEARCH confirmed zero new deps).

All threat-register `mitigate` dispositions were satisfied by the planned design:
- **T-09-01** (migration tampering): 008/010 use `ADD VALUE IF NOT EXISTS`, 009 uses `ADD COLUMN IF NOT EXISTS` — re-running across restarts is a no-op (proven 004 pattern).
- **T-09-02** (Drain DoS): `Drain(limit)` is `Order(requested_at ASC).Limit(limit)` — the consumer caps batch size; a non-positive limit returns no rows.
- **T-09-15** (spoofed reason): `validateDemandReason` allowlists {backfill,next_ep,ongoing} and defaults to backfill on absent/unknown — trusted only because /internal/* is Docker-network-only (T-08-04); a malformed internal caller never writes an invalid enum.

## Issues Encountered
None. (One cosmetic note: the plan's `grep -q "Episode \*int"` acceptance regex does not match gofmt's struct-field alignment (`Episode      *int`); validated instead via the reflection test `TestJobEpisodeFieldIsNullableInt` and a space-tolerant grep — the field is correct.)

## User Setup Required
None - no external service configuration required. Migrations 008/009/010 auto-apply at library boot once Plan 09-02 wires them into main.go.

## Next Phase Readiness
- The library-side foundation is complete: Plan 09-02 (Planner drain loop) can import `HasActiveForEpisode`, `Drain`/`Delete`, `IncDownloadsTotal`, `Job.Episode`, and `JobSourceAutocache`, and must add the 008/009/010 apply calls + a `JobSourceAutocache` const + `allowedSources` map entry where the admin enqueue validates source.
- Plans 09-03 (player Logic B → next_ep) and 09-04 (scheduler Logic A → ongoing) can fire their true reason and have it honored — no follow-up needed.
- No blockers.

## Self-Check: PASSED

- `services/library/migrations/008_autocache_job_source.sql` — FOUND
- `services/library/migrations/009_library_jobs_episode.sql` — FOUND
- `services/library/migrations/010_autocache_demand_ongoing.sql` — FOUND
- `.planning/phases/09-download-triggers/09-01-SUMMARY.md` — FOUND
- Commits `c880ec7d` (T1), `2f2e3d94`+`a3ee6cee` (T2 RED/GREEN), `0b5fbc53` (T3), `2ddb5ba3`+`d7014165` (T4 RED/GREEN) — all present in branch history
- `cd services/library && go build ./... && go vet ./... && go test ./... -count=1` — BUILD-OK / VET-OK / TEST-OK

---
*Phase: 09-download-triggers*
*Completed: 2026-06-17*
