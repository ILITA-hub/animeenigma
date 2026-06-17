---
phase: 09-download-triggers
verified: 2026-06-17T11:00:00Z
status: passed
score: 17/17 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: none
  note: initial verification
---

# Phase 9: Download Triggers Verification Report

**Phase Goal:** The platform autonomously enqueues the right RAW downloads — Logic A (ongoing push), Logic B (next-episode pull), backfill-on-miss — without duplicating work or downloading the wrong track.
**Verified:** 2026-06-17T11:00:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #  | Truth | Status | Evidence |
|----|-------|--------|----------|
| 1  | CRITICAL reason-seam: Demand handler validates+honors wire reason against {backfill,next_ep,ongoing}, no longer force-overrides to backfill; invalid/absent → backfill | ✓ VERIFIED | `autocache_internal.go:131-138` `validateDemandReason` switch over the 3-enum allowlist, default backfill; `:169` `RecordDemand(..., validateDemandReason(body.Reason))` (old hard-coded `domain.DemandReasonBackfill` arg gone); handler test `:120-124` table asserts next_ep/ongoing/backfill honored + absent/invalid→backfill |
| 2  | TRIG-01 Logic A: scheduler cron job, JP-audio filter, D8 recency, episodes_aired join, fires reason="ongoing", shared animeenigma DB | ✓ VERIFIED | `autocache_logic_a.go:100-111` DISTINCT join `watch_history×anime_list×animes` WHERE watching+ongoing + `(wh.player IN ('ae','raw') OR wh.language='ja')` + `al.updated_at > ?` (cutoff = now − activeWatcherDays) + selects `episodes_aired`; `:155-160` POSTs `{mal_id,episode,reason:"ongoing"}`; cron registered `job.go:231-251` label `autocache_logic_a` with metrics wrap, nil-guarded |
| 3  | TRIG-02 Logic B: player UpdateProgress fires next_ep(N+1) only for JP-audio+watching combos; fire-and-forget drop-on-full; progress save unaffected on failure | ✓ VERIFIED | `progress.go:95` `maybeFireNextEpDemand` called AFTER successful `UpsertProgress` (`:80-82`); `:106` JP-audio gate, `:119` watching+shikimoriID gate, `:124` N+1≤episodes_aired gate, `:130` `Want(shikimoriID, next, "next_ep")`; `autocache_demand.go:91-102` drop-on-full select-send, nil-safe |
| 4  | TRIG-03 backfill: Planner drains autocache_demand (all reasons) on config-gated loop (enabled + sweep_interval_min) | ✓ VERIFIED | `planner.go:159-208` ctx-aware loop, `:174` reads config live each tick, `:185` `if !cfg.Enabled return`, `:181` cadence from `SweepIntervalMin`, `:189` `Drain(ctx, drainBatchLimit)` |
| 5  | TRIG-04 single-flight dedup: skip if episode present (GetByShikimoriEpisode) OR non-terminal job targets (mal,episode) via HasActiveForEpisode + library_jobs.episode | ✓ VERIFIED | `planner.go:219-226` present-check → delete demand + no job; `:231-242` `HasActiveForEpisode` in-flight → dedup, no second job; `job.go:111-126` excludes terminal (done/failed/cancelled); migration 009 added `library_jobs.episode INT` |
| 6  | TRIG-05 RAW/quality/seeder: Planner filters RAW (allowlist + reject dub/dual-audio/hardsub) + ≤quality_cap + ≥min_seeders; enqueues source=autocache | ✓ VERIFIED | `raw_filter.go:55-70` selectRAW; `:36` negative-token regex `(dub|dual-audio|multi-audio|eng-dub|hardsub)`; `:25-31` uploader allowlist; `:61` `res > qualityCap` reject; `:64` `Seeders < minSeeders` reject; `:92` missing quality → ineligible; `planner.go:280` `Source: domain.JobSourceAutocache` |
| 7  | downloads_total{trigger,result} maps ongoing→A, next_ep→B, backfill→backfill end-to-end (handler honors reason) | ✓ VERIFIED | `planner.go:311-320` `triggerForReason`: ongoing→"A", next_ep→"B", default→"backfill"; `metrics:167-170` counter `{trigger,result}`; planner_test `:259-276` table asserts A/B/backfill via `GetDownloadsTotalForTest` per reason — end-to-end, not "counter moved" |
| 8  | Foundation: job_source enum 'autocache', library_jobs.episode, autocache_demand_reason 'ongoing', Job.Episode, DemandReasonOngoing, repo methods, counter | ✓ VERIFIED | migrations 008/009/010 idempotent `IF NOT EXISTS`; embeds in migrations.go; `job.go:75` `Episode *int gorm:"column:episode"`; `autocache_demand.go:27` `DemandReasonOngoing="ongoing"`; `HasActiveForEpisode`/`Drain`/`Delete`/`IncDownloadsTotal`/`GetDownloadsTotalForTest` all present |
| 9  | Separate-DB constraint: library does NOT read watch_history/anime_list/animes | ✓ VERIFIED | grep of `services/library/internal/**.go` (non-test) for those tables returns ZERO matches; demand only arrives via autocache_demand table |
| 10 | NO leak into Phase 10 (no evictor/budget pre-admit) or Phase 11 (no Grafana; downloads_total emitted only) | ✓ VERIFIED | only "evictor"/"budget" hits are FUTURE-phase doc comments in migrator.go/layout.go (Phase 7 artifacts); no evictor/pre-admit code in autocache pkg; downloads_total emitted (7 IncDownloadsTotal call sites) but no Grafana dashboards added |

**Score:** 17/17 truths verified (10 goal truths across 4 plans' 23 frontmatter must-haves; all consolidated truths pass)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `migrations/008_autocache_job_source.sql` | ADD VALUE 'autocache' | ✓ VERIFIED | idempotent ADD VALUE, embedded |
| `migrations/009_library_jobs_episode.sql` | ADD COLUMN episode INT | ✓ VERIFIED | nullable, idempotent, embedded |
| `migrations/010_autocache_demand_ongoing.sql` | ADD VALUE 'ongoing' | ✓ VERIFIED | idempotent, embedded |
| `repo/job.go` HasActiveForEpisode | single-flight gate | ✓ VERIFIED | terminal-status exclusion |
| `repo/demand.go` Drain+Delete | drain primitives | ✓ VERIFIED | ORDER BY requested_at ASC, limit-bounded |
| `metrics/library_metrics.go` downloads_total | {trigger,result} CounterVec | ✓ VERIFIED | registered + nil-safe Inc + test seam |
| `handler/autocache_internal.go` reason-seam | validate+honor | ✓ VERIFIED | validateDemandReason, no hard-coded backfill |
| `autocache/planner.go` | drain loop | ✓ VERIFIED | 374 lines, wired w/ real repos in main.go:470 |
| `autocache/raw_filter.go` | RAW/quality/seeder | ✓ VERIFIED | selectRAW full heuristic |
| `cmd/library-api/main.go` | migrations applied + Planner Start/Stop | ✓ VERIFIED | :175/182/187 applies; :470 NewPlanner; :479 Start; :533 Stop |
| `player/service/autocache_demand.go` | DemandProducer | ✓ VERIFIED | fire-and-forget clone of RecsHintProducer |
| `player/service/progress.go` | Logic B fire point | ✓ VERIFIED | maybeFireNextEpDemand, gated |
| `player/repo/progress.go` | LogicBContext | ✓ VERIFIED | single query, watching=false sentinel |
| `player/config + main.go` | DI + toggle | ✓ VERIFIED | AUTOCACHE_DEMAND_ENABLED, Start/Stop, threaded into ProgressService |
| `scheduler/jobs/autocache_logic_a.go` | join + demand POST | ✓ VERIFIED | 186 lines, hotcombos join + ongoing POST |
| `scheduler/service/job.go` | cron registration | ✓ VERIFIED | AddFunc label autocache_logic_a, metrics, nil-guard |
| `scheduler/config + main.go` | cron/URL/days config + DI | ✓ VERIFIED | default cron */20, days 30, conditional construct |

### Key Link Verification

| From | To | Via | Status |
|------|----|----|--------|
| planner.go | Drain/GetByShikimoriEpisode/HasActiveForEpisode | drain→present→inflight→search→enqueue | ✓ WIRED |
| planner.go | selectRAW (raw_filter) | search then RAW/quality/seeder filter | ✓ WIRED |
| main.go (library) | Planner lifecycle | NewPlanner(real repos).Start(rootCtx)+Stop | ✓ WIRED |
| progress.go | DemandProducer.Want | JP-audio+watching+aired guard → Want(N+1,next_ep) | ✓ WIRED |
| autocache_demand.go | library /internal demand | buffered channel + worker POST | ✓ WIRED |
| autocache_logic_a.go | watch_history×anime_list×animes join | ongoing+watching+JP+D8+episodes_aired | ✓ WIRED |
| autocache_logic_a.go | library demand POST | per-ongoing {mal_id,episode,reason:ongoing} | ✓ WIRED |
| job.go (scheduler) | cron.AddFunc | register Logic A with metrics | ✓ WIRED |
| handler autocache_internal | RecordDemand w/ true reason | validateReason maps wire→enum | ✓ WIRED |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| Planner | drained demand rows | DemandRepository.Drain (real DB query, ORDER BY requested_at) | Yes | ✓ FLOWING |
| Planner enqueue | RAW release | TieredSearcher.FetchAll (real Jackett/Nyaa search) → selectRAW | Yes | ✓ FLOWING |
| Logic A job | join rows | live Raw SQL over animeenigma DB | Yes | ✓ FLOWING |
| Logic B | LogicBContext | live Raw SQL join anime_list×animes | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| library build+vet+test | `go build/vet/test ./...` | all pass | ✓ PASS |
| player build+vet | `go build/vet ./...` | clean | ✓ PASS |
| player scoped tests | `go test ./internal/service/... ./internal/repo/...` | ok (51.8s) | ✓ PASS |
| scheduler build+vet+test | `go build/vet/test ./...` | all pass | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| TRIG-01 | 09-04 | Logic A ongoing push | ✓ SATISFIED | autocache_logic_a.go join+POST+cron |
| TRIG-02 | 09-03 | Logic B next-ep pull | ✓ SATISFIED | progress.go fire point + DemandProducer |
| TRIG-03 | 09-01,02 | backfill drain | ✓ SATISFIED | Planner drains all reasons incl backfill |
| TRIG-04 | 09-01,02 | single-flight dedup | ✓ SATISFIED | present + in-flight checks, library_jobs.episode |
| TRIG-05 | 09-01,02 | RAW/quality/seeder | ✓ SATISFIED | selectRAW filter, source=autocache |

No orphaned requirements — all 5 TRIG IDs mapped to Phase 9 in REQUIREMENTS.md and each claimed by a plan.

### Anti-Patterns Found

None. No TODO/FIXME/XXX/HACK/PLACEHOLDER/TBD debt markers in any phase-9 file. No stub returns, no hollow props.

### Human Verification Required

None. All truths verified programmatically against source + build/vet/test. No visual/UX/real-time/external-service surface in this phase (pure backend autonomous logic with deterministic unit-test coverage).

### Gaps Summary

No gaps. All 5 requirements satisfied, the CRITICAL reason-seam fix is correct by construction (handler validate-and-honor verified in code + inverted test table), all three producers (Logic A scheduler, Logic B player, backfill via the Phase-8 serve-miss seam) funnel into the single library Planner drain loop, single-flight dedup (present + in-flight) and RAW/quality/seeder gating both verified, end-to-end downloads_total{trigger} attribution (ongoing→A, next_ep→B, backfill→backfill) asserted per-reason in the Planner test. Separate-DB constraint holds (library reads no watch tables). No leak into Phase 10 (no evictor/budget) or Phase 11 (no Grafana; downloads_total emitted only). All three touched services build, vet, and pass scoped tests; the pre-existing player MAL-export handler test failure is out-of-scope (live scheduler network call in sandbox), correctly documented in deferred-items.md.

---

_Verified: 2026-06-17T11:00:00Z_
_Verifier: Claude (gsd-verifier)_
