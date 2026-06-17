---
phase: 09-download-triggers
plan: 04
subsystem: scheduler
tags: [autocache, logic-a, ongoing-push, cron, robfig-cron, hotcombos-join, jp-audio, d8-recency, episodes-aired, http-internal, docker-network, trig-01]
requires:
  - phase: 09-01
    provides: "Demand handler honors validated wire reason (next_ep/ongoing/backfill); autocache_demand_reason enum accepts 'ongoing'; /internal/library/autocache/demand Docker-network-only route"
  - phase: 08-02
    provides: "POST /internal/library/autocache/demand {mal_id, episode, reason} endpoint shape"
provides:
  - "AutocacheLogicAJob — periodic scheduler cron job that runs the adapted hotcombos DISTINCT join (watch_history × anime_list × animes, shared animeenigma DB) + JP-audio filter (player IN ae/raw OR language=ja) + D8 active_watcher_days recency + episodes_aired projection, then fires one ongoing demand per qualifying ongoing anime"
  - "Per-ongoing demand POST {mal_id: shikimori_id, episode: episodes_aired, reason: 'ongoing'} to the library /internal endpoint (5s client timeout; single-POST failure logged+counted but does NOT abort the sweep; Run errors only on the JOIN failure)"
  - "Config: AutocacheLogicACron (AUTOCACHE_LOGIC_A_CRON, default */20 * * * *), LibraryInternalURL (LIBRARY_INTERNAL_URL → LIBRARY_SERVICE_URL → http://library:8089), AutocacheActiveWatcherDays (AUTOCACHE_ACTIVE_WATCHER_DAYS, default 30)"
  - "Cron registration via the existing robfig/cron AddFunc harness with SchedulerJob{ExecutionsTotal,Duration,LastSuccess} metrics wrap (label autocache_logic_a), nil-guarded + last-run-tracked + surfaced in GetStatus"
affects:
  - "09-02 library Planner — drains the ongoing demands this producer re-asserts each sweep (attributes trigger=A via reason='ongoing')"
tech-stack:
  added: []
  patterns:
    - "Shared-DB enumeration producer in scheduler: the cron job runs the join ITSELF (unlike the analytics-trigger jobs which only POST a recompute endpoint) and fires per-row demand POSTs — the load-bearing reason Logic A lives in a shared-DB service (the library DB cannot see watch_history)"
    - "DB-portable recency predicate: the D8 active_watcher_days cutoff is computed in Go (time.Now().AddDate(0,0,-days)) and bound as a parameter (al.updated_at > ?) so the same SQL runs on both Postgres (prod) and SQLite (tests) without DB-specific interval syntax"
    - "Best-effort per-demand fan-out: a single non-2xx/transport demand POST is logged (Warnw) + counted but continues the loop; Run returns an error ONLY on the enumeration JOIN failure so the JobService metrics wrap records a real failure vs a tolerable transient blip"
    - "Nil-guarded optional cron registration: a missing LibraryInternalURL → nil job (main.go) → JobService.Start skips it cleanly (mirrors readThresholdJob/providerRankingJob)"
key-files:
  created:
    - services/scheduler/internal/jobs/autocache_logic_a.go
    - services/scheduler/internal/jobs/autocache_logic_a_test.go
    - services/scheduler/internal/service/job_test.go
  modified:
    - services/scheduler/internal/config/config.go
    - services/scheduler/internal/service/job.go
    - services/scheduler/cmd/scheduler-api/main.go
key-decisions:
  - "Recency predicate uses a Go-computed cutoff bound as `al.updated_at > ?` instead of Postgres `now() - (interval '1 day' * ?)` — identical semantics, but portable to the SQLite test seam (the package's proven test driver) so the JOIN's filtering (JP-audio / recency / ongoing / watching) is fully unit-tested against real seeded rows, not just mocked"
  - "Chose the heavier-but-stronger SQLite seam over the lighter enumerator-interface seam: the package already has a proven in-memory-SQLite test pattern (scraper_playability_canary_test.go), and the JOIN itself is the riskiest logic, so testing it end-to-end (seed → join → captured demand body) is worth the seeding cost"
  - "active_watcher_days is a scheduler env MIRROR (AUTOCACHE_ACTIVE_WATCHER_DAYS, default 30) — the AUTHORITATIVE live-editable value lives in library autocache_config, but the scheduler is on a DIFFERENT DB (animeenigma) and does NOT read library's DB; the two must be kept in sync if the library default is retuned (documented in config.go + here per the plan's mandate)"
  - "The job is constructed nil when LibraryInternalURL is empty (main.go), so the nil-guarded registration disables it cleanly — a missing/blank library URL silently no-ops rather than firing demands into the void"
  - "Reason='ongoing' is sent on the wire and honored as-is by the Plan-09-01 demand handler (validate-and-honor on the Docker-network-only path) — no server-side override, no follow-up needed (correct by construction)"
patterns-established:
  - "Pattern 1: a scheduler-owned shared-DB enumeration producer — runs the hotcombos-style DISTINCT join from the cron harness (no HTTP for the read) and fires per-row Docker-network demand POSTs; the canonical home for any future 'who-wants-what across the shared DB' periodic trigger"
  - "Pattern 2: DB-portable time-window predicates (Go-computed cutoff bound as a param) so shared-DB join logic is testable on the SQLite seam without DB-specific date arithmetic"
requirements-completed: [TRIG-01]
duration: ~10min
completed: 2026-06-17
---

# Phase 9 Plan 04: Scheduler Logic A — ongoing-push autocache producer Summary

**A periodic scheduler cron job (`AutocacheLogicAJob`, default every 20 min) that runs the adapted hotcombos DISTINCT join over `watch_history × anime_list × animes` (shared `animeenigma` DB) — adding a JP-audio filter (`player IN ('ae','raw') OR language='ja'`), the D8 `active_watcher_days` recency predicate, and the `episodes_aired` projection — to find every ongoing anime with ≥1 active JP-audio watcher, then fires a best-effort `{mal_id, episode=episodes_aired, reason='ongoing'}` demand per anime to the library `/internal/library/autocache/demand` endpoint; registered on the robfig/cron harness with the standard metrics wrap, nil-guarded + last-run-tracked.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-06-17T10:07:00Z
- **Completed:** 2026-06-17T10:14:00Z
- **Tasks:** 2 (1 TDD)
- **Files modified:** 6 (3 created, 3 modified)

## Accomplishments
- `AutocacheLogicAJob` — the TRIG-01 ongoing-push producer. `Run(ctx)` executes the adapted `hotcombos.go` DISTINCT join (`watch_history × anime_list × animes`) with the three Logic-A additions: JP-audio filter (`player IN ('ae','raw') OR language='ja'`), the D8 recency predicate (`al.updated_at > cutoff`, cutoff = `now - active_watcher_days`), and the `episodes_aired` latest-aired-episode projection. For each `(shikimori_id, episodes_aired)` row with `episodes_aired > 0` and a non-empty `shikimori_id`, it POSTs `{mal_id, episode, reason:"ongoing"}` to the library demand endpoint.
- Robustness: a single demand POST failure (non-2xx or transport error) is logged (`Warnw`) + counted but does NOT abort the sweep — re-asserting demand is idempotent via the library composite PK, so a transient blip self-heals next sweep. `Run` returns an error ONLY when the enumeration JOIN itself fails, so the `JobService` metrics wrap records a real failure vs a tolerated demand blip.
- Config + cron registration + DI: `AutocacheLogicACron` (default `*/20 * * * *`), `LibraryInternalURL` (`LIBRARY_INTERNAL_URL` → `LIBRARY_SERVICE_URL` → `http://library:8089`), `AutocacheActiveWatcherDays` (`AUTOCACHE_ACTIVE_WATCHER_DAYS`, default 30). The job is registered via `cron.AddFunc` with the `SchedulerJob{ExecutionsTotal,Duration,LastSuccess}` metrics wrap (label `autocache_logic_a`), nil-guarded like the other optional jobs, last-run-tracked, and surfaced in `GetStatus()`. `main.go` constructs it only when `LibraryInternalURL` is set.

## Task Commits

Each task was committed atomically (TDD task split test → feat):

1. **Task 1: AutocacheLogicAJob — join + per-ongoing demand POST** - `e3c29500` (test, RED) → `f4ede47f` (feat, GREEN)
2. **Task 2: Config + cron registration + main.go DI** - `d5c5a32e` (feat)

**Plan metadata:** committed in this SUMMARY commit (docs).

## Files Created/Modified
- `services/scheduler/internal/jobs/autocache_logic_a.go` - `AutocacheLogicAJob` struct + `NewAutocacheLogicAJob` + `Run` (DISTINCT join, Go-computed recency cutoff bound as a param) + `fireDemand` (5s-timeout POST of `{mal_id, episode, reason:"ongoing"}`)
- `services/scheduler/internal/jobs/autocache_logic_a_test.go` - SQLite-seeded coverage of the JOIN's filtering (JP-audio fire, raw-player, non-JP no-op, D8 recency exclusion, non-ongoing/non-watching exclusion, zero-aired/empty-mal skip, DISTINCT-per-anime) + httptest demand-body capture (`reason="ongoing"`) + partial-failure-does-not-abort + join-failure-returns-error
- `services/scheduler/internal/service/job_test.go` - asserts the Logic A registration arity + `GetStatus` exposure, and that a nil job is skipped cleanly
- `services/scheduler/internal/config/config.go` - `AutocacheLogicACron` / `LibraryInternalURL` / `AutocacheActiveWatcherDays` fields + Load wiring (with the env-mirror documentation block)
- `services/scheduler/internal/service/job.go` - `autocacheLogicAJob` field + `lastAutocacheLogicARun`, new `NewJobService`/`Start` arity, nil-guarded `cron.AddFunc` registration with the metrics wrap, `GetStatus` entry
- `services/scheduler/cmd/scheduler-api/main.go` - construct the job (nil when no library URL) + thread into `NewJobService` + pass the cron expr into `Start`

## Decisions Made
See `key-decisions` frontmatter. Headlines: (1) the D8 recency predicate uses a Go-computed cutoff bound as `al.updated_at > ?` (Postgres+SQLite portable) so the JOIN's filtering is fully unit-tested against real seeded rows; (2) chose the SQLite seam (the package's proven test driver) over the lighter enumerator-interface seam because the JOIN is the riskiest logic; (3) `active_watcher_days` is a scheduler env MIRROR of the authoritative library `autocache_config` value (cross-DB boundary) — documented in `config.go` and here.

## Deviations from Plan

None of substance — the plan executed essentially as written. Two minor, in-scope implementation choices to note:

- **Test seam choice (explicitly delegated by the plan):** The plan offered "SQLite seeding OR a tiny `enumerator` seam — pick the lighter and note it." I picked the **SQLite seam** (heavier but stronger) because the package already has a proven in-memory-SQLite test pattern (`scraper_playability_canary_test.go`) and the JOIN's filtering is the riskiest logic worth testing end-to-end. Noted here per the plan's mandate.
- **Recency-predicate SQL:** The plan's interface snippet showed Postgres `now() - (interval '1 day' * ?)`. I used a Go-computed cutoff bound as `al.updated_at > ?` instead — identical semantics, but portable to the SQLite test seam. This is a faithfulness-preserving substitution, not a behavior change.

No bugs, no missing critical functionality, no blocking issues, no architectural changes (Rules 1-4 N/A). No package installs (T-09-SC N/A — stdlib `net/http` + existing `gorm`/`robfig/cron` only; RESEARCH confirmed zero new deps).

## Threat Surface
All threat-register `mitigate` dispositions were satisfied by the planned design:
- **T-09-11** (re-asserting demand every sweep / thundering herd): the producer only re-asserts; the library composite-PK dedup collapses repeats and the Planner backoff (Plan 09-02) bounds re-searches — bounded by design.
- **T-09-12** (wrong-audience demand): the JOIN's `wh.player IN ('ae','raw') OR wh.language='ja'` + `al.status='watching'` + `a.status='ongoing'` + D8 recency predicates ensure only active JP-audio ongoing watchers produce a demand (verified by `TestLogicA_NonJPAudioFiresNothing` / `_StaleWatcherExcluded` / `_NonOngoingExcluded` / `_NotWatchingExcluded`).
- **T-09-14** (a single library blip aborting the whole sweep): per-demand POST failures are logged + counted but do NOT abort the loop; `Run` returns an error only on the JOIN failure (verified by `TestLogicA_SingleDemandFailureDoesNotAbortSweep`).
- **T-09-13** (demand POST over the network): accepted — Docker-network-only `/internal/*` (no gateway proxy); body carries only mal_id/episode/reason.
- **T-09-SC** (package installs): none — stdlib + existing deps only.

No new security-relevant surface introduced beyond the planned `<threat_model>`.

## Issues Encountered
- **Worktree path discipline:** the harness resets the shell cwd to the main-repo root (`/data/animeenigma`) between Bash calls, so an early `git commit <pathspec>` ran against the main repo (where the new worktree file didn't exist) and failed harmlessly; re-running inside the worktree (and `git add` before committing an untracked file) succeeded. No data impact.
- **`.gitignore` matches `services/scheduler/cmd/scheduler-api/` (the built `scheduler-api` binary):** `git add <dir>` warned, but the tracked source `main.go` was already staged and committed cleanly via pathspec. No artifact was committed.

## User Setup Required
None - no external service configuration required. `LIBRARY_SERVICE_URL` is already set on services in `docker/docker-compose.yml`; `AUTOCACHE_LOGIC_A_CRON` / `AUTOCACHE_ACTIVE_WATCHER_DAYS` default sanely. If the library `autocache_config.active_watcher_days` is retuned away from 30, set `AUTOCACHE_ACTIVE_WATCHER_DAYS` on the scheduler to match (cross-DB mirror).

## Next Phase Readiness
- Logic A is live: every sweep (default 20 min) re-asserts an `ongoing` demand for the latest-aired episode of each ongoing anime with ≥1 active JP-audio watcher. Plan 09-02's library Planner drains these (attributing `trigger=A`).
- Both producers (09-03 player Logic B → `next_ep`, 09-04 scheduler Logic A → `ongoing`) now fire, completing the producer side of the Phase-9 demand model.
- No blockers.

## Self-Check: PASSED

- `services/scheduler/internal/jobs/autocache_logic_a.go` — FOUND
- `services/scheduler/internal/jobs/autocache_logic_a_test.go` — FOUND
- `services/scheduler/internal/service/job_test.go` — FOUND
- `services/scheduler/internal/config/config.go` (AutocacheLogicACron) — FOUND
- `.planning/phases/09-download-triggers/09-04-SUMMARY.md` — FOUND
- Commits `e3c29500` (T1 RED), `f4ede47f` (T1 GREEN), `d5c5a32e` (T2) — all present in branch history
- `cd services/scheduler && go build ./... && go vet ./... && go test ./... -count=1` — BUILD-OK / VET-OK / TEST-OK (all packages green)

---
*Phase: 09-download-triggers*
*Completed: 2026-06-17*
