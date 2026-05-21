---
phase: 01-backend-aggregator
plan: 03
subsystem: backend/catalog
tags: [hero-spotlight, aggregator, concurrency, go, redis]
workstream: hero-spotlight
dependency-graph:
  requires:
    - 01-01 (types + Resolver interface + Aggregator skeleton + SnapshotKey)
    - 01-02 (4 resolvers — exercised in Plan 04 wiring)
  provides:
    - Aggregator.Resolve(ctx, userID) — concurrent fan-out + per-card 800ms + overall 2s + eligibility filter + snapshot fallback + snapshot save
    - NewAggregatorWithDeadlines — test-friendly constructor with overridable deadlines
  affects:
    - services/catalog (consumed by Plan 04 handler wiring)
tech-stack:
  added: []
  patterns:
    - sync.WaitGroup + buffered chan fan-out (from subs_aggregator.go:109-156)
    - context.WithTimeout double-layer (overall + per-card)
    - errors.Is(err, cache.ErrNotFound) sentinel check (DELIBERATE DIVERGENCE 2)
    - detached context.Background() goroutine for best-effort snapshot save
key-files:
  created:
    - services/catalog/internal/service/spotlight/aggregator_test.go
  modified:
    - services/catalog/internal/service/spotlight/aggregator.go
decisions:
  - "Use sync.WaitGroup + buffered chan (NOT errgroup) — fail-fast semantics of errgroup fight the partial-success contract"
  - "Per-card 800ms ctx.WithTimeout inside each goroutine — not relying on outer ctx alone"
  - "Overall 2s ctx.WithTimeout wrapping Resolve — bounds total request time"
  - "loadSnapshot uses errors.Is(err, cache.ErrNotFound) — distinguishes clean miss from Redis hard-down (DIVERGENCE 2)"
  - "Snapshot save uses detached context.Background() goroutine — request ctx is cancelled when Resolve returns"
  - "Only save snapshot when len(cards) > 0 — avoids baking empty result into 24h snapshot"
  - "cards := []Card{} (NOT var cards []Card) — empty JSON marshals as [] not null (Phase 2 frontend regression guard)"
  - "Defensive nil-cache guard in loadSnapshot/saveSnapshot — preserves Plan-01 TestNewAggregator_ConstructsEmpty contract"
  - "NewAggregatorWithDeadlines for test injection — lets the timeout tests run on tight budgets without inflating CI runtime"
metrics:
  duration: "~4m"
  tasks_completed: 2
  files_created: 1
  files_modified: 1
  tests_added: 10
  completed_date: 2026-05-21
---

# Phase 1 Plan 01-03: Backend Aggregator — Concurrent Fan-Out + Snapshot Implementation Summary

Replaces the Plan-01 Aggregator.Resolve stub with the load-bearing
concurrent implementation that ships HSB-BE-03/04/05 partial-success
semantics and HSB-NF-03 key-prefix invariants.

## Aggregator.Resolve — Final Shape

```go
func (a *Aggregator) Resolve(ctx context.Context, userID *string) (*Response, error)
```

Return contract:
- `(resp, nil)` for ALL paths — partial-success, total-failure, snapshot-fallback, and empty-response all return non-nil resp with err == nil.
- `resp.Cards` is never nil; an empty result is `[]Card{}` (marshals as JSON `[]`).
- `resp.GeneratedAt` is either:
  - a fresh `time.Now().UTC().Format(time.RFC3339)` (live data), OR
  - the snapshot's preserved timestamp (fallback path proves the snapshot was returned).

## Constants

```go
const (
    perCardDeadline = 800 * time.Millisecond  // HSB-BE-03
    overallBudget   = 2 * time.Second         // HSB-BE-04
    snapshotTTL     = 24 * time.Hour          // HSB-BE-04
)
```

## Test-Injection Constructor

```go
func NewAggregatorWithDeadlines(c cache.Cache, log *logger.Logger, resolvers []Resolver, perCard, overall time.Duration) *Aggregator
```

Production code uses `NewAggregator` which pins the constants above.
Tests use `NewAggregatorWithDeadlines` to drive timeout branches with
tight budgets without inflating CI runtime. The struct now carries
`perCard time.Duration` and `overall time.Duration` fields (default-
initialized to the package constants by `NewAggregator`).

## DELIBERATE DIVERGENCE 2 — Enforced

`loadSnapshot` uses `errors.Is(err, cache.ErrNotFound)` for the
"no snapshot exists" branch:

```go
err := a.cache.Get(ctx, SnapshotKey(userID), &snap)
if err == nil {
    return &snap
}
if !errors.Is(err, cache.ErrNotFound) {
    a.log.Warnw("spotlight.snapshot_load_failed", "error", err)
}
return nil
```

This distinguishes a clean cache miss (silent) from a hard Redis
failure (Warnw). Verified by `grep -q "errors.Is(err, cache.ErrNotFound)" services/catalog/internal/service/spotlight/aggregator.go`.

## Snapshot Save — Detached Goroutine

```go
if len(cards) > 0 {
    go a.saveSnapshot(context.Background(), userID, resp)
}
```

`context.Background()` is intentional — the request ctx is about to be
cancelled by the caller (handler) when `Resolve` returns. Best-effort,
fire-and-forget. Only writes when at least one card resolved so a
zero-card outage doesn't bake an empty result into a 24h snapshot.
Verified by `grep -q "context.Background()" services/catalog/internal/service/spotlight/aggregator.go`.

## Test Count — 10 (≥ 7 required by VALIDATION.md)

| Test | What it pins |
|------|-------------|
| `TestAggregator_Concurrency_DispatchesInParallel` | 4×100ms resolvers return in <250ms (parallel, NOT 400ms sequential) |
| `TestAggregator_PerCardTimeout_DropsSlowCard` | 1 slow resolver dropped by 800ms ctx; 3 siblings returned; elapsed in [800ms, 1100ms] |
| `TestAggregator_OverallTimeout_DropsAllSlowCards` | All 4 timed out by per-card deadline (1500ms > 800ms) |
| `TestAggregator_OverallTimeout_HitsOverallBudget` | Tight 1s overall + lenient 3s per-card → elapsed <1200ms (overall enforced) |
| `TestAggregator_EligibilityFilter_DropsNilCardSilently` | (nil, nil) silent drop; resolver still invoked; siblings returned |
| `TestAggregator_SnapshotFallback_ReturnsSnapshotOnZeroCards` | Pre-seeded snapshot returned verbatim including stale GeneratedAt |
| `TestAggregator_SnapshotFallback_NoSnapshot_EmptyResponse` | 0 resolvers + no snapshot → Cards == []Card{}, JSON marshals as `[]` |
| `TestAggregator_SnapshotSave_WritesAfterSuccessfulResolve` | Detached snapshot write verifiable via 500ms poll |
| `TestKeyPrefix_AllWritesUseSpotlightPrefix` | HSB-NF-03 regression guard — every key starts with `spotlight:` |
| `TestAggregator_ErroringResolver_EmitsErrorw` | (nil, err) drops card, siblings returned |

All tests pass under `-race`:
```
$ go test ./services/catalog/internal/service/spotlight/... -count=1 -race -timeout 30s
ok      .../spotlight         3.797s
ok      .../spotlight/cards   1.045s
ok      .../spotlight/client  1.082s
```

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Implement concurrent Aggregator.Resolve with timeouts + snapshot | `35b9c89` | `services/catalog/internal/service/spotlight/aggregator.go` |
| 2 | Aggregator concurrency + timeout + snapshot tests | `d78fb31` | `services/catalog/internal/service/spotlight/aggregator_test.go` + defensive nil-cache fix in `aggregator.go` |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Defensive nil-cache guard in loadSnapshot/saveSnapshot**
- **Found during:** Task 2 test run
- **Issue:** The existing Plan-01 `TestNewAggregator_ConstructsEmpty` in `types_test.go` passes `nil` for the cache parameter and then calls `Resolve(...)`. The original stub returned an empty Response unconditionally, but the new concurrent implementation calls `loadSnapshot` on the zero-resolver path → `a.cache.Get(...)` NPEs on the nil receiver.
- **Fix:** Added a `if a.cache == nil { return nil }` guard at the top of both `loadSnapshot` and `saveSnapshot`. Nil cache (test wiring or feature-half-built) is treated as a clean miss / no-op — symmetric and consistent with the "snapshot is best-effort" semantics from CONTEXT.md.
- **Files modified:** `services/catalog/internal/service/spotlight/aggregator.go`
- **Commit:** `d78fb31`

No other deviations — the rest of the plan executed exactly as written.

## Verification

```bash
# Build clean
$ go build ./services/catalog/internal/service/spotlight/...
$ go vet ./services/catalog/internal/service/spotlight/...
$ go build ./services/catalog/...

# All tests pass under -race
$ go test ./services/catalog/internal/service/spotlight/... -count=1 -race -timeout 30s
ok      services/catalog/internal/service/spotlight         3.797s
ok      services/catalog/internal/service/spotlight/cards   1.045s
ok      services/catalog/internal/service/spotlight/client  1.082s

# Acceptance criteria spot-checks
$ grep -q "STUB: Plan 03" services/catalog/internal/service/spotlight/aggregator.go; echo $?  # 1 (absent)
$ grep -q "sync.WaitGroup" services/catalog/internal/service/spotlight/aggregator.go; echo $?  # 0
$ grep -q "perCardDeadline" services/catalog/internal/service/spotlight/aggregator.go; echo $?  # 0
$ grep -q "overallBudget" services/catalog/internal/service/spotlight/aggregator.go; echo $?    # 0
$ grep -q "errors.Is(err, cache.ErrNotFound)" services/catalog/internal/service/spotlight/aggregator.go; echo $?  # 0
$ grep -q "context.Background()" services/catalog/internal/service/spotlight/aggregator.go; echo $?  # 0
$ grep -c "^func TestAggregator_\|^func TestKeyPrefix" services/catalog/internal/service/spotlight/aggregator_test.go  # 10
```

All ≥7 (VALIDATION.md threshold), actual count is 10.

## Self-Check: PASSED

Created files exist:
- `services/catalog/internal/service/spotlight/aggregator_test.go` — FOUND

Modified files exist:
- `services/catalog/internal/service/spotlight/aggregator.go` — FOUND

Commits exist in git log:
- `35b9c89` — feat(01-03): implement concurrent spotlight aggregator with fan-out + snapshot — FOUND
- `d78fb31` — test(01-03): aggregator concurrency, timeout, eligibility, snapshot tests — FOUND

## Next Plan

Plan 01-04 wires the Aggregator into the catalog handler (`GET /api/home/spotlight`) and into `cmd/catalog-api/main.go`. The Aggregator constructor signature was preserved (still `NewAggregator(c, log, resolvers)`) so Plan 01-04 can drop the existing wiring in unchanged.
