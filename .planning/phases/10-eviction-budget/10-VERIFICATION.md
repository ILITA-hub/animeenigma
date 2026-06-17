---
phase: 10-eviction-budget
verified: 2026-06-17T12:00:00Z
status: passed
score: 5/5 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: none
  previous_score: n/a
---

# Phase 10: Eviction & Budget Verification Report

**Phase Goal:** The whole first-party aeProvider/ pool stays self-managing under one budget — Fresh content protected, Stale content evicted in a fair source-ranked order, and an unfittable download cleanly rejected rather than blowing the budget.
**Verified:** 2026-06-17T12:00:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | EVICT-01: budget = Σ size_bytes over the pool bounded by budget_bytes (default 100 GB) | ✓ VERIFIED | `repo/episode.go:140` `SumPoolBytes` → `COALESCE(SUM(size_bytes),0)` over `minio_path LIKE 'aeProvider/%'`. `ensureRoomLocked` (`evictor.go:167`) compares `used+estBytes <= cfg.BudgetBytes`. Default `107374182400` in `domain/autocache_config.go:23` + `migrations/006_autocache_config.sql:20`. |
| 2 | EVICT-02: Classify — auto/admin source-specific Fresh windows incl. NULL handling | ✓ VERIFIED | `evictor.go:76` pure `Classify`: autocache uses AutoFreshDownloadDays/AutoFreshFetchDays, admin uses AdminFreshDays (both windows); nil DownloadedAt/LastFetchAt contribute nothing (lines 88-94). 6 unit tests pass (both sources × NULL × in/out-window). |
| 3 | EVICT-03: ListStaleEvictionCandidates ordered EXACTLY 4-tier, Stale-only, Fresh never evicted | ✓ VERIFIED | `repo/episode.go:203` CASE: autocache·null-fetch→1, autocache→2, admin·null-fetch→3, else→4 ASC; within-tier `COALESCE(last_fetch_at,downloaded_at,created_at) ASC`. NOT-Fresh predicate with BOUND `?` params (lines 190-212, no interpolation). Matches spec §6 lines 180-186. |
| 4 | EVICT-04: EnsureRoom admitted=false on exhaustion → rejected_total{budget_full}; admin 507; Planner leaves demand + arms backoff | ✓ VERIFIED | `evictor.go:195` returns `used+estBytes <= budget` (false when queue exhausts). Planner `planner.go:321-334`: IncRejectedTotal("budget_full") + markSearched (backoff) + return without Delete. Handler `jobs.go:235-243`: IncRejectedTotal + `writeInsufficientStorage` (HTTP 507 at `jobs.go:373`). |
| 5 | EVICT-05: BOTH EnsureRoom AND DiskGuard.Allow gate in BOTH Planner AND admin handler; DiskGuard NOT replaced; Evictor injected NON-NIL into BOTH constructors | ✓ VERIFIED | Handler `jobs.go:201-245`: DiskGuard block FIRST (unchanged), budget gate SECOND, both must pass. Planner gate `planner.go:308-336`. main.go: real `evictor := autocache.NewEvictor(...)` (`main.go:435`) injected into `NewJobsHandlerWithLink` (`main.go:445`, param pos 3) AND `NewPlanner` (`main.go:494`, param pos 6); `evictor.Start(rootCtx)` + `evictor.Stop()` (`main.go:557`). FOOT-GUN CLEAR. |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/repo/episode.go` | SumPoolBytes, ListStaleEvictionCandidates, DeleteByID, ListPool | ✓ VERIFIED | All present; COALESCE-0 sum, bound-param 4-tier candidate query, nil-noop delete, ListPool seam for Accountant. |
| `internal/minio/writer.go` | DeletePrefix (hard-fail-first, empty=nil) | ✓ VERIFIED | `writer.go:386`: normalizes slash, lists, RemoveObject loop, returns on first error, empty→nil. 5 unit tests pass. |
| `internal/metrics/library_metrics.go` | evicted/rejected counters + bytes_used/budget_bytes/episodes gauges | ✓ VERIFIED | All 5 collectors with locked label sets `{source}`,`{reason}`,`{source,freshness}`,none,`{source,freshness}` (`library_metrics.go:189-222`); nil-guarded Inc/Set methods. |
| `internal/autocache/evictor.go` | Evictor: EnsureRoom, Classify, Sweep, Start/Stop, Accountant | ✓ VERIFIED | 364 lines. EnsureRoom→ensureRoomLocked (shared reclaim core), evictOne object-then-row, Sweep reclaim+gauges under one mutex, ticker lifecycle mirrors Planner. |
| `internal/autocache/planner.go` | budgetEvictor seam + pre-admit gate before Create | ✓ VERIFIED | Seam `planner.go:48`, gate `planner.go:308-336` before `p.jobs.Create`, fail-open on error, avgRawEpSize fallback const. |
| `internal/handler/jobs.go` | EvictorAPI seam + gate after DiskGuard | ✓ VERIFIED | Seam `jobs.go:57`, gate `jobs.go:228-245` after DiskGuard, 507 on reject, fail-open on error. |
| `cmd/library-api/main.go` | NewEvictor construct + Start + inject both + Stop | ✓ VERIFIED | `main.go:435,436,445,494,557`. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| Evictor.EnsureRoom | repo SumPoolBytes/ListStaleEvictionCandidates/DeleteByID | poolAccountant seam | ✓ WIRED | `evictor.go:31-38` seam; episodeRepo satisfies (build passes). |
| Evictor.evictOne | minio DeletePrefix | objectDeleter seam | ✓ WIRED | `evictor.go:43`; writer satisfies (build passes); object-delete FIRST then row. |
| Evictor.Sweep | bytes_used/budget_bytes/episodes gauges | SetBytesUsed/SetBudgetBytes/SetEpisodes | ✓ WIRED | `evictor.go:303,344,345`; all 4 (src×freshness) buckets always written incl. zero-reset. |
| Planner plan() | Evictor.EnsureRoom | budgetEvictor seam before Create | ✓ WIRED | `planner.go:313`. |
| handler Create | Evictor.EnsureRoom | EvictorAPI seam after DiskGuard | ✓ WIRED | `jobs.go:229`. |
| main.go | autocache.NewEvictor | construct + Start + inject both + Stop | ✓ WIRED | non-nil into both constructors confirmed by param-position read. |

### Eviction Mechanics

| Mechanic | Status | Evidence |
|----------|--------|----------|
| Object-delete (DeletePrefix) THEN row-delete (DeleteByID) | ✓ VERIFIED | `evictor.go:205-217` evictOne: DeletePrefix first; row delete only on object-delete success; object-fail → return without row delete (caller skips). |
| One mutex serializes EnsureRoom vs Sweep | ✓ VERIFIED | `e.mu` taken by EnsureRoom (`evictor.go:146`) and Sweep (`evictor.go:284`); shared lock-free `ensureRoomLocked` core prevents double-spend (T-10-04). |
| Accountant gauges refreshed on Sweep; evicted_total{source} per eviction | ✓ VERIFIED | `publishAccountantGauges` (`evictor.go:320`) Sets bytes_used/episodes per bucket + SetBudgetBytes; `IncEvictedTotal(source)` per completed eviction (`evictor.go:192`). |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Full service build | `go build ./...` | exit 0 | ✓ PASS |
| Full service vet | `go vet ./...` | exit 0 | ✓ PASS |
| Full test suite | `go test ./... -count=1` | all packages ok | ✓ PASS |
| Phase-10 behavioral tests | `go test -run "Evict\|Classify\|EnsureRoom\|Sweep\|Budget\|Reject\|DeletePrefix\|StaleEviction\|SumPool"` | 30+ tests PASS incl. tier-order, queue-exhaust reject, object-fail skip-row, row-fail skip-byte, sweep gauges, planner reject-leaves-demand-backoff, fail-open, fallback estimate, handler 507, disk-full-before-budget | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| EVICT-01 | 10-01,10-02 | Pool bytes bounded by configurable budget (default 100 GB) | ✓ SATISFIED | SumPoolBytes + budget compare; default 107374182400. |
| EVICT-02 | 10-01,10-02 | Fresh/Stale by source-specific windows | ✓ SATISFIED | Classify + candidate-query predicate. |
| EVICT-03 | 10-01,10-02 | Only Stale evicted in 4-tier order; Fresh never | ✓ SATISFIED | ListStaleEvictionCandidates CASE order. |
| EVICT-04 | 10-02,10-03 | Unfittable download rejected + rejected_total{budget_full} | ✓ SATISFIED | admitted=false → 507 (admin) / demand-left+backoff (planner). |
| EVICT-05 | 10-03 | Logical budget co-exists with DiskGuard; both must pass | ✓ SATISFIED | Both gates ANDed in both paths; DiskGuard unchanged. |

No orphaned requirements — all 5 phase-10 IDs (EVICT-01..05) declared across plan frontmatter and verified.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | No TBD/FIXME/XXX debt markers, no stubs, no nil-evictor injection | — | None. Empty-return patterns (`return nil`) are legitimate no-ops with documented rationale (idempotent delete, fail-open). |

### Phase 11 Leak Check

| Concern | Status | Evidence |
|---------|--------|----------|
| No Grafana panels (OBS-* belong to Phase 11) | ✓ CLEAN | No grafana/dashboard provisioning files touched by phase-10 commits; gauges/counters present in metrics layer ready for Phase 11. (AdminDashboard.vue working-tree change is unrelated parallel-agent work, not library service.) |

### Gaps Summary

None. All 5 observable truths verified against real source. The plan-checker's flagged foot-gun — a nil Evictor silently dropping the admin gate — is confirmed CLEAR: main.go constructs a real `*autocache.Evictor` and injects it non-nil into both `NewJobsHandlerWithLink` (param position 3) and `NewPlanner` (param position 6), with the constructor signatures placing the evictor arg at exactly those positions (build passes). Eviction mechanics (object-then-row, single mutex), Accountant gauges, and the dual-gate EVICT-05 layering (DiskGuard NOT replaced) are all present and unit-tested. Build/vet/test all green.

---

_Verified: 2026-06-17T12:00:00Z_
_Verifier: Claude (gsd-verifier)_
