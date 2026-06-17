---
phase: 10-eviction-budget
reviewed: 2026-06-17T09:28:34Z
depth: deep
files_reviewed: 7
files_reviewed_list:
  - services/library/internal/autocache/evictor.go
  - services/library/internal/repo/episode.go
  - services/library/internal/minio/writer.go
  - services/library/internal/autocache/planner.go
  - services/library/internal/handler/jobs.go
  - services/library/cmd/library-api/main.go
  - services/library/internal/metrics/library_metrics.go
findings:
  critical: 0
  warning: 4
  info: 3
  total: 7
status: resolved
resolved_at: 2026-06-17T09:52:00Z
resolution:
  warnings_fixed: 4
  info_accepted: 3
  commits:
    - 44d4ca9f  # WR-03 admin-fallback estimate
    - 18219a45  # WR-01 in-flight reservation + WR-02 lock granularity + WR-04 consistent snapshot
---

# Phase 10: Code Review Report

**Reviewed:** 2026-06-17T09:28:34Z
**Depth:** deep (cross-file: budget arithmetic, lock contention, SQL ↔ Go parity, lifecycle)
**Files Reviewed:** 7
**Status:** resolved (all 4 warnings fixed; 3 info items accepted — see Resolution)

> **Resolution (2026-06-17):** All four warnings fixed and verified
> (`go build ./... && go vet ./... && go test ./... -count=1 -race` race-clean).
> - **WR-01** — added `JobRepository.SumInflightJobBytes` (Σ non-terminal autocache job
>   bytes) and a `jobAccountant` seam on the Evictor; `ensureRoomLocked` now counts
>   `materialized + in-flight`. The reservation is a query over non-terminal rows, so it
>   self-releases on success (done → counts in `SumPoolBytes`) AND failure
>   (failed/cancelled → never counts) with no counter to leak. Commit `18219a45`.
> - **WR-02** — `Sweep` now holds `e.mu` across the mutating reclaim ONLY, releasing it
>   before the read-only gauge refresh; the reclaim stays fully serialized so no
>   double-spend / TOCTOU is introduced. Commit `18219a45`.
> - **WR-03** — exported `autocache.AvgRawEpSize`; the admin upload gate applies the same
>   fallback the Planner uses when `size_bytes` is omitted (=0). Commit `44d4ca9f`.
> - **WR-04** — `ensureRoomLocked` re-measures the used total under the still-held mutex
>   after the evict loop, deciding admission off a single consistent snapshot. Commit
>   `18219a45`.
> - **IN-01 / IN-02 / IN-03** — **accepted, not fixed** (cosmetic message text,
>   unreachable-by-schema defensive branches, and immaterial DST/calendar-day skew
>   respectively); none are behavioral defects. Left as-is per scope.

## Summary

Reviewed the v4.1 eviction & budget slice in `services/library`: the budget arithmetic
+ ordered Stale eviction + periodic Accountant sweep (`evictor.go`), the 4-tier SQL +
pool helpers (`repo/episode.go`), the hard-fail `DeletePrefix` (`minio/writer.go`), both
pre-admit gates (`planner.go`, `handler/jobs.go`), the metrics, and the wiring/lifecycle
in `main.go`. `go build ./...`, `go vet ./...`, and `go test ./...` (autocache, repo,
handler, minio, metrics) all pass clean.

The core mechanics are sound and well-tested: the eviction loop iterates a fixed
candidate slice so it cannot infinite-loop; Fresh rows are excluded at the SQL layer so
they are never eligible; the object-then-row delete ordering is consistent (hard-fail on
object delete → leave the row → tolerate orphaned objects, never an orphaned serving
pointer); the `evictedTotal` metric only fires after a *confirmed* object+row delete; the
single shared `Evictor` instance correctly gates both pre-admit paths + the sweep under
one mutex; gauge cardinality is bounded (2 sources × 2 freshness = 4 series, unknown
source folded into `admin` consistently with `Classify`); the Postgres `episode_source`
enum is `{admin,autocache}` only, so the SQL `notFresh` predicate covers every row (no
Stale row can ever be excluded from candidacy). No Grafana / Phase-11 scope leak.

The findings below are robustness/consistency gaps, not correctness breaks. The two that
matter most: the pre-admit budget is enforced **only against already-materialized rows**
with no in-flight reservation (concurrent admits + the long admit→encode→row-insert gap
can overshoot the soft budget by N estimates), and the sweep holds the shared mutex
across unbounded MinIO+DB IO, which head-of-line-blocks the synchronous admin upload
handler.

## Warnings

### WR-01: Pre-admit budget has no in-flight reservation — concurrent admits + admit→materialize gap overshoot the budget

**File:** `services/library/internal/autocache/evictor.go:162-167`, `services/library/internal/autocache/planner.go:308-336`, `services/library/internal/handler/jobs.go:228-245`

**Issue:** `ensureRoomLocked` computes headroom as `SumPoolBytes() + estBytes <= BudgetBytes`,
where `SumPoolBytes` counts **only rows already in the `aeProvider/` pool**. A job admitted
by the Planner/admin handler is enqueued (`jobs.Create`) and does not become a pool row
until after download + encode + `episodeRepo.Create` in `encoder_worker.go:316-323` —
minutes to hours later. There is no in-flight byte accounting (confirmed: no
`reserve`/`pendingBytes` anywhere). Consequences, neither bounded by the mutex (which only
serializes EnsureRoom-vs-Sweep, not EnsureRoom-vs-encoder-materialize):
  1. **Concurrent / sequential admits against a stale snapshot.** With `used` near budget,
     two admits both see the same `used`, both compute `used+est <= budget`, both pass —
     committing `2*est` against headroom that fits only `1*est`. The admin handler runs in
     the HTTP goroutine and the Planner in its ticker goroutine; nothing prevents both (or
     several Planner iterations across ticks before any job materializes) from over-admitting.
  2. **Admit→materialize gap.** Even single-threaded, N jobs can be admitted before the
     first one's row lands, so the pool can transiently exceed budget by ΣestBytes of all
     in-flight jobs.

The single-job *estimate-vs-actual* delta is explicitly accepted in `10-CONTEXT.md:47-50`,
but the *multiplication across un-materialized in-flight jobs* is not addressed by the
design and is the larger overshoot. It is mitigated (not eliminated) by the physical
`DiskGuard` hard gate and by the periodic Sweep reclaiming Stale rows afterward, so this is
a soft-budget-overshoot, not data loss — hence WARNING, not BLOCKER.

**Fix:** Track in-flight admitted bytes so EnsureRoom counts `materialized + in-flight +
estBytes`. Minimal approach: count non-terminal autocache jobs' `size_bytes` (the encoder
clears them on completion when the row materializes) and add that to `used`:

```go
// in ensureRoomLocked, after SumPoolBytes:
inflight, err := e.pool.SumInflightJobBytes(ctx) // Σ size_bytes of queued/downloading/encoding/uploading autocache jobs
if err != nil { return false, err }
used += inflight
```

Alternatively, document explicitly in `evictor.go` that the budget is enforced
**post-materialization only** and that transient overshoot up to ΣinflightEstimates is
expected, so future readers do not assume a hard ceiling.

### WR-02: Sweep holds the shared mutex across unbounded MinIO + DB IO, head-of-line-blocking the synchronous admin upload handler

**File:** `services/library/internal/autocache/evictor.go:283-313` (Sweep) ↔ `services/library/internal/handler/jobs.go:228-245` (admin Create gate)

**Issue:** `Sweep` takes `e.mu` and holds it across `ensureRoomLocked` (which lists Stale
candidates then loops `evictOne` — each doing a MinIO `DeletePrefix` *list + per-object
RemoveObject* plus a DB `DeleteByID`) **and** across `ListPool` + gauge publishing. The
admin upload path calls `e.EnsureRoom(r.Context(), ...)` synchronously inside the HTTP
request (`jobs.go:229`), which blocks on the same `e.mu`. If a sweep is mid-eviction of
many/large prefixes, an admin upload (and the Planner) stalls for the entire sweep
duration. The server's `WriteTimeout` is 120s (`main.go:524`), so a long sweep can push the
admin request toward a write timeout. The serialization is *correct* for double-spend
avoidance, but the granularity is too coarse: a slow reclaim shouldn't block an unrelated
admit's *read*.

**Fix:** Don't hold the lock across the gauge-refresh half of Sweep (it's read-only and
needs no double-spend protection), and consider bounding per-sweep eviction work. Minimal:

```go
func (e *Evictor) Sweep(ctx context.Context) {
    e.mu.Lock()
    _, err := e.ensureRoomLocked(ctx, 0) // reclaim under lock (mutates pool)
    e.mu.Unlock()                        // release BEFORE the read-only gauge refresh
    if err != nil { /* log */ }

    cfg, err := e.config.Get(ctx)
    // ... ListPool + publishAccountantGauges with NO lock held (read-only)
}
```

For the reclaim half itself, give the evict loop a per-sweep deadline/budget (e.g. cap N
evictions or a ctx with timeout) so a backlog of huge prefixes can't pin the lock
indefinitely.

### WR-03: Admin upload pre-admit gate reserves nothing when `size_bytes` is omitted (=0), weakening design-D7 "admin upload rejected when pool can't fit"

**File:** `services/library/internal/handler/jobs.go:229`

**Issue:** The admin gate calls `EnsureRoom(r.Context(), body.SizeBytes)` with the raw
request value. `createJobRequest.SizeBytes` is `json:"size_bytes,omitempty"` (jobs.go:143),
so an operator pasting a magnet without a declared size sends `0`. `EnsureRoom(0)` only
asserts "is the pool *currently* under budget?" and admits — reserving nothing for a
download whose real encoded size is unknown but non-trivial (~1.2 GiB+). The Planner path
deliberately substitutes the `avgRawEpSize` fallback for exactly this case
(`planner.go:309-312`), but the admin path does not, so the two pre-admit paths disagree on
the same unknown-size scenario. This conforms to `10-03-PLAN.md:126` (which specifies
`body.SizeBytes`), so it's a deliberate-but-leaky choice rather than a deviation — but it
materially weakens the D7 guarantee that "admin upload is rejected when the pool can't fit"
for the common size-omitted case, and it's the asymmetry most likely to surprise an
operator.

**Fix:** Apply the same `avgRawEpSize` fallback the Planner uses, so an unknown-size admin
upload reserves a realistic estimate:

```go
est := body.SizeBytes
if est <= 0 {
    est = autocache.AvgRawEpSize // export the planner const (or a shared one)
}
admitted, err := h.evictor.EnsureRoom(r.Context(), est)
```

If the "trust the operator, no fallback" behavior is intentional, note it explicitly at the
call site so the asymmetry with the Planner is not read as a bug.

### WR-04: `EnsureRoom` reads `used` and the candidate list at two different `time.Now()` instants, and `used` is not re-derived from the same snapshot the candidates came from

**File:** `services/library/internal/autocache/evictor.go:162-191`

**Issue:** `used` comes from `SumPoolBytes()` (one query), the candidates from
`ListStaleEvictionCandidates(ctx, cfg, time.Now())` (a *second* query at a later instant,
with its own freshness cutoffs). Between the two queries an encoder worker can insert a new
pool row or an admin can delete one (neither takes `e.mu`), so the `used` total and the
candidate set can describe slightly different pool states. The loop then decrements `used`
by each evicted `*SizeBytes`, so the running `used` is an arithmetic projection off a
possibly-stale base rather than a re-measured value. In practice the drift is small and
self-correcting on the next sweep, but the two-snapshot arithmetic means the admit/reject
decision at the boundary (`used` exactly at budget) can be off by one concurrent insert.

**Fix:** Compute both `used` and the candidate list inside a single read-consistent
transaction, or re-read `SumPoolBytes()` once more after the evict loop and decide
admission on the freshly-measured total rather than the decremented projection:

```go
// after the evict loop, re-measure instead of trusting the decremented `used`:
final, err := e.pool.SumPoolBytes(ctx)
if err != nil { return false, err }
return final+estBytes <= cfg.BudgetBytes, nil
```

This also tightens WR-01's boundary behavior. (Severity WARNING: bounded, soft-budget, and
the sweep reconciles.)

## Info

### IN-01: Stale `Create` source-enum error message omits valid sources — ACCEPTED (not fixed)

**File:** `services/library/internal/handler/jobs.go:190`

**Issue:** `allowedSources` accepts `nyaa, animetosho, manual, jackett, autocache`
(jobs.go:147-153) but the 400 message reads `"source must be one of: nyaa, animetosho,
manual"`. A client sending `jackett`/`autocache` is accepted while the rejection text for an
*invalid* source under-reports the real allow-list — misleading for API consumers.
(Pre-existing, surfaced in a reviewed file.)

**Fix:** Generate the message from the map keys, or update it to
`"source must be one of: nyaa, animetosho, manual, jackett, autocache"`.

### IN-02: `Classify` unknown-source branch is unreachable defensive code — ACCEPTED (not fixed)

**File:** `services/library/internal/autocache/evictor.go:81-85`, `:330-334`

**Issue:** The `else` branch in `Classify` and the `if _, ok := buckets[src]; !ok` fold in
`publishAccountantGauges` both handle a source that is neither `admin` nor `autocache`. The
Postgres `episode_source` column is an enum constrained to exactly those two values
(`domain/episode.go:56`), so these branches can never execute at runtime. Harmless and
arguably good defense-in-depth, but worth a one-line comment that it's unreachable-by-schema
so a future reader doesn't hunt for the third source.

**Fix:** Add `// unreachable: episode_source enum is {admin,autocache}; kept as defense`
on both branches, or drop them.

### IN-03: `daysAgo` / cutoff math uses `AddDate` (calendar days), not 24h multiples — DST/leap edge skew — ACCEPTED (not fixed)

**File:** `services/library/internal/autocache/evictor.go:57-59`, `services/library/internal/repo/episode.go:183-185`

**Issue:** Both the Go `Classify` cutoff (`now.AddDate(0,0,-d)`) and the SQL cutoffs
(`now.AddDate(0,0,-cfg.*)`) use calendar-day subtraction. This is consistent between the
two layers (good — no SQL↔Go parity drift) and correct for a freshness window measured in
days. The only nuance: across a DST boundary an `AddDate(0,0,-10)` window is 10 *calendar*
days, not exactly 240h, so a row can be classified Fresh/Stale a few minutes earlier/later
than a pure-duration window would. For a multi-day cache-eviction freshness window this is
immaterial; flagged only for completeness. No fix required.

---

_Reviewed: 2026-06-17T09:28:34Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
