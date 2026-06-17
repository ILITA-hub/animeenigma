---
phase: 09-download-triggers
plan: 02
subsystem: library
tags: [autocache, planner, drain-loop, ticker, single-flight, raw-filter, prometheus, migrations, lifecycle]
requires:
  - phase: 09-01
    provides: "DemandRepository.Drain/Delete; JobRepository.HasActiveForEpisode; Job.Episode *int; DemandReasonOngoing; IncDownloadsTotal{trigger,result}; migrations 008/009/010 embedded (not yet applied); reason-honoring demand handler"
  - phase: 08-02
    provides: "autocache_demand intake; EpisodeRepository.GetByShikimoriEpisode present-check; AutocacheConfigRepository.Get (live enabled + sweep_interval_min)"
  - phase: 07-02
    provides: "TieredSearcher.FetchAll (Jackett→Nyaa/AnimeTosho, seeder-ranked DESC); WorkerPool ticker/lifecycle pattern; library_jobs/job_source enum"
provides:
  - "autocache.Planner — config-gated ctx-aware drain loop draining autocache_demand → RAW download jobs (TRIG-03/04)"
  - "selectRAW release filter — uploader allowlist OR no-negative-token, ≤quality_cap, ≥min_seeders (TRIG-05)"
  - "JobSourceAutocache const + allowedSources enqueue-map entry (09-01-deferred wiring)"
  - "migrations 008/009/010 APPLIED at boot in main.go (job_source 'autocache', library_jobs.episode, demand reason 'ongoing')"
  - "downloads_total{trigger,result} attribution verified end-to-end: ongoing→A, next_ep→B, backfill→backfill"
affects:
  - "09-03 (player Logic B producer: its next_ep demand is now drained → trigger=B job)"
  - "09-04 (scheduler Logic A producer: its ongoing demand is now drained → trigger=A job)"
  - "Phase 10 (evictor: consumes the source=autocache rows + library_jobs.episode this Planner writes)"
  - "Phase 11 (Grafana: charts library_autocache_downloads_total{trigger,result})"
tech-stack:
  added: []
  patterns:
    - "Config-gated ctx-aware ticker drain loop (re-reads enabled + sweep_interval_min live each tick; mirrors WorkerPool sleep/Start/Stop)"
    - "Local interface seams in the consuming package (demandDrainer/presenceChecker/jobEnqueuer/searcher/configGetter) so the unit injects fakes + main.go owns concrete wiring (no parser/service import in autocache)"
    - "Per-(mal,ep) last-searched-at backoff + per-sweep fan-out cap (thundering-herd guard, T-09-05)"
    - "Delete-demand-on-confirmed-presence; LEAVE-on-fresh-enqueue (single-flight dedup re-derives, RESEARCH Pitfall 6 option b)"
    - "Best-effort RAW classifier: uploader allowlist + negative-token regex + conservative quality parse (no structured release-type field exists)"
key-files:
  created:
    - services/library/internal/autocache/raw_filter.go
    - services/library/internal/autocache/raw_filter_test.go
    - services/library/internal/autocache/planner.go
    - services/library/internal/autocache/planner_test.go
  modified:
    - services/library/internal/domain/job.go
    - services/library/internal/handler/jobs.go
    - services/library/cmd/library-api/main.go
key-decisions:
  - "selectRAW treats missing/unparseable quality as ineligible (conservative — cannot prove ≤cap); a negative token disqualifies even an allowlisted uploader"
  - "Planner seams are local interfaces (not service/repo imports) so autocache stays leaf-level; main.go adapts TieredSearcher via plannerSearchAdapter (mirrors animeToshoAdapter/torrentClientAdapter)"
  - "runOnce returns the cadence + is driven directly by the unit (no ticker in tests); the loop goroutine sleeps that cadence between sweeps"
  - "Demand row deleted ONLY on confirmed presence; a fresh enqueue LEAVES the row and lets next tick's HasActiveForEpisode dedup clear it once present (Pitfall 6 option b)"
  - "JobSourceAutocache const + allowedSources entry landed in the Task-2 commit (the Planner depends on the const to compile) rather than waiting for the Task-3 main.go wiring"
  - "minSweepInterval floor (1m) guards a misconfigured zero sweep_interval_min from busy-spinning; searchBackoff=1h, drainBatchLimit=50, searchFanoutLimit=5 are documented tunables"
patterns-established:
  - "Pattern 1: a drain loop with local interface seams + handwritten fakes + fresh prometheus.NewRegistry() per case — no Postgres, no testify/mock"
  - "Pattern 2: reason→trigger attribution asserted per-reason via GetDownloadsTotalForTest, proving the 09-01 reason-honoring handler + the Planner mapping wire through end-to-end"
requirements-completed: [TRIG-03, TRIG-04, TRIG-05]
duration: ~6min
completed: 2026-06-17
---

# Phase 9 Plan 02: Library Autocache Planner (drain loop + RAW filter + main.go wiring) Summary

**A config-gated ctx-aware Planner that drains `autocache_demand` into `source=autocache` RAW download jobs — single-flight dedup (present-check + in-flight-check), RAW/≤quality_cap/≥min_seeders gating, per-(mal,ep) backoff + bounded fan-out, and `downloads_total{trigger}` attribution mapped from the demand reason (ongoing→A, next_ep→B, backfill→backfill) — plus the 09-01-deferred wiring: migrations 008/009/010 applied at boot and the `JobSourceAutocache` const + enqueue-map entry.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-06-17T09:46:00Z
- **Completed:** 2026-06-17T09:52:00Z
- **Tasks:** 3 (2 TDD)
- **Files modified:** 7 (4 created, 3 modified)

## Accomplishments
- Built `selectRAW` (TRIG-05): the best-effort RAW classifier — uploader allowlist (Ohys/Leopard/ARC/SubsPlease/Erai-raws) OR no dub/dual-audio/multi-audio/eng-dub/hardsub token, ≤`quality_cap`, ≥`min_seeders`, with conservative rejection of unparseable quality. Full table-driven coverage.
- Built `autocache.Planner` (TRIG-03/04): a config-gated ctx-aware ticker that drains demand oldest-first and, per row, runs present-check (delete-on-present) → in-flight single-flight dedup → search → `selectRAW` → enqueue a `source=autocache, episode=intended` job. No-release LEAVES the demand row (retry "as soon as on torrents") behind a per-(mal,ep) 1h backoff + per-sweep fan-out cap.
- Wired `IncDownloadsTotal` with the reason-mapped trigger end-to-end and asserted it per reason: `ongoing→A`, `next_ep→B`, `backfill→backfill` — proving the Plan-09-01 reason-honoring handler + the Planner mapping wire through correctly (not merely that the counter moved).
- Landed the 09-01-deferred wiring: applied migrations 008/009/010 at boot in `main.go` (Fatalw-on-error, after 007), added the `JobSourceAutocache` const + its `allowedSources` enqueue-map entry, and wired `planner.Start(rootCtx)` + `planner.Stop()` into the boot/shutdown lifecycle via `plannerSearchAdapter`.

## Task Commits

Each task was committed atomically (TDD tasks split test → feat):

1. **Task 1: RAW/quality/seeder filter (TRIG-05)** - `c678a7bf` (test, RED) → `77dd29ea` (feat, GREEN)
2. **Task 2: Planner drain loop + JobSourceAutocache const (TRIG-03/04)** - `68d0291b` (test, RED) → `a133dd5c` (feat, GREEN)
3. **Task 3: apply migrations 008/009/010 + wire Planner lifecycle in main.go** - `2365d33d` (feat)

**Plan metadata:** this SUMMARY commit (docs). STATE.md / ROADMAP.md intentionally NOT modified (isolated-worktree directive — the orchestrator owns state).

## Files Created/Modified
- `services/library/internal/autocache/raw_filter.go` - `selectRAW` + `isRAW` + `resolutionOf`; uploader allowlist, negative-token regex, quality-token regex.
- `services/library/internal/autocache/raw_filter_test.go` - table-driven TRIG-05 cases (RAW pass, dub/dual-audio/hardsub reject, seeder gate, quality cap, missing-quality reject, ranked-slice winner).
- `services/library/internal/autocache/planner.go` - `Planner` struct + local seams (demandDrainer/presenceChecker/jobEnqueuer/searcher/configGetter), `Start`/`Stop`/`loop`/`runOnce`/`plan`, `triggerForReason`, backoff + fan-out.
- `services/library/internal/autocache/planner_test.go` - handwritten fakes + fresh registry per case: disabled no-op, present→delete+count, in-flight→dedup, winning→create(source=autocache,episode set), no-release→leave, explicit reason→trigger mapping.
- `services/library/internal/domain/job.go` - `JobSourceAutocache` const (job_source 'autocache').
- `services/library/internal/handler/jobs.go` - `allowedSources[JobSourceAutocache] = true` enqueue-map entry.
- `services/library/cmd/library-api/main.go` - apply migrations 008/009/010 at boot; `plannerSearchAdapter`; construct + `Start`/`Stop` the Planner in the lifecycle.

## Decisions Made
See `key-decisions` frontmatter. Headlines: the RAW filter is conservative (unparseable quality ⇒ ineligible; negative token disqualifies even allowlisted uploaders); the Planner uses local interface seams (no service/repo import) with main.go owning the `plannerSearchAdapter`; the demand row is deleted only on confirmed presence and LEFT on a fresh enqueue so single-flight dedup re-derives (Pitfall 6 option b); `runOnce` returns its cadence and is unit-driven directly (no ticker in tests).

## Deviations from Plan

None - plan executed exactly as written. No bugs, missing functionality, blocking issues, or architectural changes encountered. No package installs (T-09-SC N/A — stdlib `time.Ticker`/`sync` + existing gorm/prometheus only; RESEARCH confirmed zero new deps).

All threat-register `mitigate` dispositions were satisfied by the planned design:
- **T-09-04 / T-09-SC** (malicious magnet / installs): the Planner reuses the same `domain.Release.Magnet` shape an admin enqueue uses — the existing `metainfo.ParseMagnetUri` enqueue path is unchanged; no new trust surface, no installs.
- **T-09-05** (Jackett thundering-herd): `drainBatchLimit=50` (Drain bound) + per-(mal,ep) 1h `searchBackoff` + `searchFanoutLimit=5` per sweep — a not-yet-released episode is not re-searched every tick.
- **T-09-06** (master-switch bypass): `runOnce` re-reads `autocache_config.Enabled` live each tick; when disabled it drains/enqueues nothing (asserted by `TestPlannerDisabledNoOp`).
- **T-09-07** (wrong-track DUB download): `selectRAW` negative-token + uploader allowlist + quality/seeder gate reject DUB/dual-audio/oversized releases before enqueue (asserted across the filter table).

## Issues Encountered
- `git add services/library/cmd/library-api/` tripped a `.gitignore` warning (the directory pattern matches the built `library-api` binary). `main.go` itself is NOT ignored and was already staged; committing the explicit `main.go` pathspec succeeded. No content impact.

## User Setup Required
None - no external service configuration required. Migrations 008/009/010 now auto-apply at library boot. The Planner always starts but no-ops while `autocache_config.enabled=false` or while `JACKETT_API_KEY` is empty (the seeder gate is unenforceable without Jackett, so no peerless torrents are enqueued).

## Next Phase Readiness
- The full library drain side is live: backfill demand (Phase 8, already flowing) is drained on first deploy, and Plans 09-03 (player Logic B → next_ep) / 09-04 (scheduler Logic A → ongoing) will have their demand drained into trigger=B / trigger=A jobs with no follow-up needed.
- Phase 10 (evictor) can consume the `source=autocache` rows + `library_jobs.episode` this Planner writes; Phase 11 (Grafana) can chart `library_autocache_downloads_total{trigger,result}`.
- No blockers.

## Self-Check: PASSED

- `services/library/internal/autocache/raw_filter.go` — FOUND
- `services/library/internal/autocache/planner.go` — FOUND
- `services/library/internal/autocache/planner_test.go` — FOUND
- `.planning/phases/09-download-triggers/09-02-SUMMARY.md` — FOUND (this file)
- Commits `c678a7bf`/`77dd29ea` (T1 RED/GREEN), `68d0291b`/`a133dd5c` (T2 RED/GREEN), `2365d33d` (T3) — all present in branch history
- `cd services/library && go build ./... && go vet ./... && go test ./... -count=1` — BUILD-OK / VET-OK / TEST-OK

---
*Phase: 09-download-triggers*
*Completed: 2026-06-17*
