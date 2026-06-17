---
phase: 10-eviction-budget
plan: 01
subsystem: database
tags: [gorm, postgres, minio, prometheus, library-autocache, eviction, budget]

# Dependency graph
requires:
  - phase: 07-autocache-pool
    provides: "library_episodes ledger fields (source/downloaded_at/last_fetch_at/fetch_count/size_bytes), aeProvider/ unified-pool prefix, EpisodeRepository idioms"
  - phase: 08-09-autocache
    provides: "LibraryMetrics CounterVec/Gauge patterns + nil-guarded method + GetXForTest seam conventions"
provides:
  - "EpisodeRepository.SumPoolBytes — Σ size_bytes over the aeProvider/ pool (COALESCE→0)"
  - "EpisodeRepository.ListStaleEvictionCandidates(cfg, now) — Stale-only rows in the locked 4-tier eviction order with bound freshness windows"
  - "EpisodeRepository.DeleteByID — scoped row delete, nil no-op on absent id"
  - "minio.Writer.DeletePrefix — recursive prefix delete, hard-fail on first RemoveObject error, empty=nil"
  - "LibraryMetrics: evicted_total{source}, rejected_total{reason} counters + bytes_used{source,freshness}, budget_bytes, episodes{source,freshness} gauges"
affects: [10-02-evictor, 10-03-pre-admit, 11-grafana-panels]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Eviction-candidate SQL: source-branched NOT-Fresh predicate with bound ? cutoffs (now.AddDate) + 4-tier CASE ordering + COALESCE within-tier sort"
    - "Destructive MinIO prefix delete: hard-fail on first error (inverse of Move's soft-fail) so a half-deleted prefix never reports success"
    - "GaugeVec on LibraryMetrics (first one): factory.NewGaugeVec for source×freshness splits"

key-files:
  created: []
  modified:
    - services/library/internal/repo/episode.go
    - services/library/internal/repo/episode_test.go
    - services/library/internal/minio/writer.go
    - services/library/internal/minio/writer_test.go
    - services/library/internal/metrics/library_metrics.go
    - services/library/internal/metrics/library_metrics_test.go

key-decisions:
  - "DeletePrefix hard-fails (unlike Move which soft-fails removes): a partial delete reported as success would let Plan-02 delete the row and orphan a serving pointer — leaving the row for the next sweep is the safe failure mode."
  - "Empty prefix → nil (idempotent), distinct from Move which errors on empty src: an evict of an already-gone prefix must be harmless under sweep/pre-admit races."
  - "DeleteByID is a nil no-op on absent id (zero RowsAffected is 'already gone', not NotFound) so a double-delete race between evictor and sweep is harmless."
  - "Freshness day-windows computed in Go via now.AddDate and passed as bound ? params (never interpolated) — T-10-01 mitigation, grep-guarded by a no-DB tripwire."

patterns-established:
  - "No-DB repo tripwire tests: reflect signature pins + os.ReadFile source-string assertions for the SQL shape; DB-backed behavior stays //go:build integration (matches Phase-07 episode_test convention)."

requirements-completed: [EVICT-01, EVICT-02, EVICT-03, EVICT-04]

# Metrics
duration: ~12min
completed: 2026-06-17
---

# Phase 10 Plan 01: Eviction & Budget Primitives Summary

**Three finished, tested building blocks for the Evictor — repo SumPoolBytes/ListStaleEvictionCandidates(4-tier order, bound windows)/DeleteByID, MinIO Writer.DeletePrefix (hard-fail recursive delete), and 5 new eviction/budget Prometheus collectors — with zero new dependencies and no behavior wiring.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-06-17T10:57Z
- **Completed:** 2026-06-17T11:01Z
- **Tasks:** 3
- **Files modified:** 6

## Accomplishments
- `EpisodeRepository` gained the three Phase-10 data-access primitives: a COALESCE-zero pool byte-sum, an SQL-ordered Stale-only eviction-candidate query honoring the locked 4-tier order with bound freshness windows, and a nil-safe row delete.
- `minio.Writer.DeletePrefix` deletes every object under a prefix and hard-fails on the first RemoveObject error (so Plan 02 never orphans a serving pointer), with empty-prefix idempotency.
- `LibraryMetrics` gained 2 counters (`evicted_total{source}`, `rejected_total{reason}`) and 3 gauges (`bytes_used{source,freshness}`, `budget_bytes`, `episodes{source,freshness}`) — the first GaugeVec on this type — all nil-guarded with test seams, ready for Plan 02's Evictor/Accountant to emit and Phase 11 to chart.

## Task Commits

Each task was committed atomically:

1. **Task 1: EpisodeRepository SumPoolBytes + ListStaleEvictionCandidates + DeleteByID** - `bb00e6ce` (feat)
2. **Task 2: MinIO Writer.DeletePrefix** - `77dcee82` (feat)
3. **Task 3: LibraryMetrics eviction/budget collectors** - `f8e91460` (feat)

_Note: TDD tasks were committed as single feat commits (implementation + co-located no-DB unit/tripwire tests landed together, matching the package's existing test style)._

## Files Created/Modified
- `services/library/internal/repo/episode.go` - Added `SumPoolBytes`, `ListStaleEvictionCandidates`, `DeleteByID`.
- `services/library/internal/repo/episode_test.go` - Signature + COALESCE + 4-tier-order + bound-param no-DB tripwires.
- `services/library/internal/minio/writer.go` - Added `DeletePrefix` (recursive, hard-fail, empty=nil).
- `services/library/internal/minio/writer_test.go` - fakeUploader cases: multi-key delete, slash-normalize, empty=nil, remove-error propagation (stops at first error), list-error propagation.
- `services/library/internal/metrics/library_metrics.go` - 5 new collectors + nil-guarded Inc/Set methods + GetEvicted/GetRejected test seams.
- `services/library/internal/metrics/library_metrics_test.go` - Extended exposes-all list + per-collector Inc/Set + nil-receiver guards.

## Decisions Made
See `key-decisions` in frontmatter. The load-bearing one: DeletePrefix hard-fails (Move soft-fails) so a half-deleted prefix is never reported as success — the row is left for the next sweep, trading a tolerable object orphan for never orphaning a row pointer (CONTEXT §eviction mechanics).

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
- The plan + frontmatter reference `10-PATTERNS.md`, which does not exist in the phase directory (only `10-CONTEXT.md`). The plan's `<action>` blocks contained the full verbatim SQL guidance inline (4-tier CASE literal, COALESCE within-tier sort, bound-param computation, exact metric names/labels), so the missing PATTERNS file did not block implementation. Not a deviation — no work changed.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Plan 02 (Evictor) can compose against finished, tested primitives: `SumPoolBytes` (BytesUsed numerator), `ListStaleEvictionCandidates` (the locked-order drain list), `DeleteByID` (row delete after `DeletePrefix`), and the 5 collectors (`IncEvictedTotal`, `SetBytesUsed`/`SetBudgetBytes`/`SetEpisodes` for the Accountant).
- Plan 03 (pre-admit gate) can call `IncRejectedTotal("budget_full")` for EVICT-04.
- DB-backed behavior (LIKE-prefix scoping, exact 4-tier ordering against rows, Stale filter, nil-id no-op) is covered by no-DB tripwires here; the behavioral assertions belong in a `//go:build integration` test alongside the existing `episode_integration_test.go` — recommended before Plan 02 ships eviction live.
- `cd services/library && go build ./... && go vet ./... && go test ./... -count=1` all clean. No new go.mod dependency.

## Self-Check: PASSED

- Files: episode.go, writer.go, library_metrics.go, 10-01-SUMMARY.md all present.
- Commits: bb00e6ce, 77dcee82, f8e91460 all in git history.
- Verification: `go build ./...`, `go vet ./...`, `go test ./... -count=1` clean; no go.mod change.

---
*Phase: 10-eviction-budget*
*Completed: 2026-06-17*
