# Phase 10: Eviction & Budget - Pattern Map

**Mapped:** 2026-06-17
**Files analyzed:** 8 (3 new, 5 modified)
**Analogs found:** 8 / 8 (all exact or strong role-matches — this is a brownfield phase inside an established `services/library/internal/autocache` subsystem)

All code lives in `services/library/`. Service follows the Go layout in CLAUDE.md
(domain / repo / service / handler / autocache / minio / metrics). Tests are
handwritten fakes + fresh `prometheus.NewRegistry()` per case — **no testify/mock,
no live Postgres** (DB-backed repo tests are `//go:build integration`-gated).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/autocache/evictor.go` (NEW) | service (in autocache pkg) | batch / event-driven (ticker sweep + pre-admit) | `internal/autocache/planner.go` | exact (same pkg, same loop+seam idiom) |
| `internal/autocache/evictor_test.go` (NEW) | test | — | `internal/autocache/planner_test.go` | exact |
| `internal/repo/episode.go` (MODIFY) | repo | CRUD / aggregate query | `internal/repo/episode.go` (`BumpFetch`, `List`, `ListAdminLegacyPath`) | exact (extend the same file) |
| `internal/minio/writer.go` (MODIFY: add `DeletePrefix`) | utility | file-I/O (list-then-act) | `internal/minio/writer.go` (`Move`, line 331) | exact (mirror inverse of Move) |
| `internal/autocache/config.go` accessor reuse | repo (read) | request-response | `internal/repo/autocache_config.go` (`Get`) | exact (consume as-is) |
| `internal/handler/jobs.go` (MODIFY: pre-admit hook ~line 190) | handler | request-response | `internal/handler/jobs.go` `Create` disk-guard block (lines 190-209) | exact (AND a 2nd gate beside DiskGuard) |
| `internal/metrics/library_metrics.go` (MODIFY) | metrics | — | `library_metrics.go` `autocacheServeTotal`/`enqueueRejectedTotal`/`diskFreeBytes` | exact (clone the CounterVec/Gauge idioms) |
| `cmd/library-api/main.go` (MODIFY: construct + Start Evictor) | config / wiring | — | `main.go` lines 266-267 (`diskGuard.Run`) + 470-480 (`planner.Start`) | exact |

---

## Pattern Assignments

### `internal/autocache/evictor.go` (NEW) — the Evictor

**Analog:** `internal/autocache/planner.go` (whole file). The Evictor is a sibling of
the Planner in the same package and MUST copy its structure: local interface seams
(so the package imports no repo/service/minio package), nil-guarded logger/metrics,
ctx-aware + Stop-aware ticker loop, `runOnce` returns its cadence and is unit-driven.

**Imports + seam idiom** (`planner.go:1-76`) — copy verbatim, swap the seam set:
```go
package autocache

import (
	"context"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
)

// configGetter is the live-config seam — IDENTICAL to planner.go:64-68. Reuse it.
type configGetter interface {
	Get(ctx context.Context) (*domain.AutocacheConfig, error)
}

// plannerLogger is the nil-allowed log seam — planner.go:70-76. Reuse the same shape.
```

New seams the Evictor needs (model on `demandDrainer`/`presenceChecker` at `planner.go:24-41`):
```go
// poolAccountant reads pool bytes + the ordered Stale candidate list (repo.EpisodeRepository).
type poolAccountant interface {
	SumPoolBytes(ctx context.Context) (int64, error)
	ListStaleEvictionCandidates(ctx context.Context, cfg *domain.AutocacheConfig, now time.Time) ([]domain.Episode, error)
	DeleteByID(ctx context.Context, id string) error
	// CountByFreshness(...) optional — for the Accountant gauges
}

// objectDeleter is the MinIO seam (*minio.Writer.DeletePrefix).
type objectDeleter interface {
	DeletePrefix(ctx context.Context, prefix string) error
}
```

**Lifecycle (Start/Stop/loop/runOnce/sleep)** — copy `planner.go:138-215` + `:394-407`
verbatim. The struct carries `stop`/`done` channels + a `sync.Mutex` (the Evictor
mutex ALSO serializes pre-admit vs sweep — see Shared Patterns / Concurrency below).
`runOnce` reads `cfg` live each tick (`planner.go:174-187`), no-ops on `!cfg.Enabled`,
floors cadence with `minSweepInterval`.

**Core method `EnsureRoom`** (the heart — no direct analog, compose from the pieces):
```go
// EnsureRoom evicts Stale rows in the locked 4-tier order until estBytes fits under
// budget, or returns admitted=false when the entire Stale queue is exhausted and
// still short (EVICT-04). Holds p.mu for the whole read-evict cycle so a concurrent
// sweep/pre-admit can't double-spend the same headroom (CONTEXT line 99).
func (e *Evictor) EnsureRoom(ctx context.Context, estBytes int64) (admitted bool, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	cfg, err := e.config.Get(ctx)          // live budget + freshness windows
	if err != nil { return false, err }

	used, err := e.pool.SumPoolBytes(ctx)  // Σ size_bytes over aeProvider pool
	if err != nil { return false, err }

	if used+estBytes <= cfg.BudgetBytes { return true, nil }   // fits already

	cands, err := e.pool.ListStaleEvictionCandidates(ctx, cfg, time.Now())
	if err != nil { return false, err }

	for i := range cands {
		if used+estBytes <= cfg.BudgetBytes { break }
		if err := e.evictOne(ctx, cands[i]); err != nil {
			// object-delete failed → leave the row, skip it, keep going (don't orphan)
			continue
		}
		if cands[i].SizeBytes != nil { used -= *cands[i].SizeBytes }
		e.metrics.IncEvictedTotal(string(cands[i].Source))
	}
	return used+estBytes <= cfg.BudgetBytes, nil
}
```

**`evictOne` (object-then-row ordering)** — see Shared Patterns / Eviction ordering.
Mirrors the `Move` "copy-all then remove-all, abort on copy error" discipline at
`writer.go:331-369`, inverted: delete objects FIRST, delete row ONLY if objects gone.

**Accountant gauges** — refresh on the sweep (`runOnce` after evicting): call
`SumPoolBytes` per (source,freshness) bucket (or a `CountByFreshness` repo query) and
`Set` the gauges. Model the metric-Set idiom on `disk_guard.go:99-110` `tick()` →
`metrics.SetDiskFreeBytes(free)`.

---

### `internal/autocache/evictor_test.go` (NEW)

**Analog:** `internal/autocache/planner_test.go` (per 09-02 SUMMARY: handwritten fakes
implementing the seams + `metrics.NewLibraryMetricsWithRegisterer(prometheus.NewRegistry())`
per case, `runOnce`/`EnsureRoom` driven directly — no ticker, no Postgres). Cases:
fits-no-evict, evict-until-fit, queue-exhausted→reject (admitted=false + rejected_total),
object-delete-fail→skip-row, Fresh-never-evicted, 4-tier order honored,
`GetEvictedTotalForTest`/`GetRejectedTotalForTest` assertions.

---

### `internal/repo/episode.go` (MODIFY) — add 3 methods

**Analog:** the SAME file — copy the GORM idiom from `BumpFetch` (`episode.go:100-111`),
`List` (`:65-74`), and `ListAdminLegacyPath` (`:120-129`). All use
`r.db.WithContext(ctx).…` + `liberrors.Wrap(err, liberrors.CodeInternal, "…")`.

**`SumPoolBytes`** — aggregate over the pool (model on `List`'s `Where`+`Find`, but
`Select` a SUM into a scalar):
```go
// SumPoolBytes returns Σ size_bytes over the aeProvider pool (admin + autocache).
// COALESCE so an empty pool returns 0, not NULL. Scoped to the unified-pool prefix
// (the same fixed literal ListAdminLegacyPath uses, inverted).
func (r *EpisodeRepository) SumPoolBytes(ctx context.Context) (int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&domain.Episode{}).
		Where("minio_path LIKE 'aeProvider/%'").
		Select("COALESCE(SUM(size_bytes), 0)").
		Scan(&total).Error; err != nil {
		return 0, liberrors.Wrap(err, liberrors.CodeInternal, "sum pool bytes")
	}
	return total, nil
}
```

**`ListStaleEvictionCandidates`** — the 4-tier ordered query. This is the load-bearing
SQL; the locked rule (CONTEXT lines 35-40, spec §6 lines 179-184) is:
- eligible ⟺ Stale (NOT Fresh); Fresh per source per `autocache_config` windows.
- ordering tiers: `(1)` autocache·never-fetched→oldest downloaded_at, `(2)` autocache·fetched→oldest last_fetch_at, `(3)` admin·never-fetched→oldest downloaded_at, `(4)` admin·fetched→oldest last_fetch_at.
- NULL handling: NULL `downloaded_at` = "very old" (classify by last_fetch_at only); NULL `last_fetch_at` = never-fetched (tier 1/3). Use parameterized interval math off `now` + the cfg day-windows. Sketch (raw `Where` like `ListAdminLegacyPath` but with bound params for the windows):
```go
// Pass the windows as params (NOT string-interpolated). Stale predicate excludes:
//   autocache Fresh: downloaded_at > now - auto_fresh_download_days  OR last_fetch_at > now - auto_fresh_fetch_days
//   admin     Fresh: downloaded_at > now - admin_fresh_days          OR last_fetch_at > now - admin_fresh_days
// Order by a CASE expressing the 4 tiers, then COALESCE(last_fetch_at, downloaded_at, created_at) ASC.
func (r *EpisodeRepository) ListStaleEvictionCandidates(ctx context.Context, cfg *domain.AutocacheConfig, now time.Time) ([]domain.Episode, error) {
	var eps []domain.Episode
	err := r.db.WithContext(ctx).
		Where("minio_path LIKE 'aeProvider/%'").
		Where(/* NOT-Fresh predicate, source-branched, bound params */).
		Order("CASE WHEN source='autocache' AND last_fetch_at IS NULL THEN 1 "+
		      "WHEN source='autocache' THEN 2 "+
		      "WHEN source='admin' AND last_fetch_at IS NULL THEN 3 ELSE 4 END ASC").
		Order("COALESCE(last_fetch_at, downloaded_at, created_at) ASC").
		Find(&eps).Error
	if err != nil { return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list stale eviction candidates") }
	return eps, nil
}
```
> Planner note: prefer the SQL ordered query over an in-Go sort (CONTEXT line 70). The
> Fresh/Stale predicate is the trickiest part — keep the day-windows as bound `?` params,
> NOT string-interpolated literals. Co-locate a `Classify(ep, cfg, now) Fresh|Stale` pure
> Go helper in `evictor.go` for the unit tests even though the SQL does the filtering, so
> the freshness rule is testable without Postgres.

**`DeleteByID`** — model on `UpdateMinioPath` (`episode.go:81-89`), scoped single-row:
```go
func (r *EpisodeRepository) DeleteByID(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&domain.Episode{}).Error; err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "delete episode")
	}
	return nil
}
```

---

### `internal/minio/writer.go` (MODIFY) — add `DeletePrefix`

**Analog:** `Move` (`writer.go:331-369`) + `ListObjectsByPrefix` (`:302-317`). `DeletePrefix`
is the inverse of `Move`'s remove-loop — list then `RemoveObject` each. The `Uploader`
seam already has `ListObjects` + `RemoveObject` (`writer.go:54,56,89-91`), so NO new SDK
surface. Mirror the `Move` normalization (`strings.HasSuffix(prefix,"/")`) and the
soft-fail logging:
```go
// DeletePrefix removes every object under prefix (recursive). Mirrors Move's remove
// loop. Hard-fails on the FIRST RemoveObject error (so the caller can leave the
// library_episodes row intact and retry) — unlike Move's source-orphan soft-fail,
// because here a partial delete + row-delete WOULD orphan a serving pointer.
func (w *Writer) DeletePrefix(ctx context.Context, prefix string) error {
	if !strings.HasSuffix(prefix, "/") { prefix = prefix + "/" }
	keys, err := w.ListObjectsByPrefix(ctx, prefix)
	if err != nil { return fmt.Errorf("delete list: %w", err) }
	for _, k := range keys {
		if rmErr := w.uploader.RemoveObject(ctx, w.cfg.Bucket, k, minio.RemoveObjectOptions{}); rmErr != nil {
			return fmt.Errorf("delete %s: %w", k, rmErr)   // hard-fail → caller keeps the row
		}
	}
	return nil
}
```
> Decision flagged: empty prefix (0 keys) should return `nil` (idempotent — already gone),
> NOT an error like `Move` does. The Evictor then deletes the row anyway (objects absent).

---

### `internal/handler/jobs.go` (MODIFY) — admin pre-admit hook (~line 190)

**Analog:** the existing DiskGuard block in `Create` (`jobs.go:190-209`). Add a SECOND
gate immediately AFTER the DiskGuard `Allow` check, ANDed with it: call
`Evictor.EnsureRoom(ctx, estBytes)`; on `!admitted` increment the new rejected counter
and reuse `writeInsufficientStorage(w)` (HTTP 507, `jobs.go:335-337`). Wire a new
`EvictorAPI` seam into the `JobsHandler` struct (`jobs.go:68-77`) mirroring `DiskGuardAPI`
(`jobs.go:48-51`):
```go
// EvictorAPI is the slice of *autocache.Evictor the handler needs (mirrors DiskGuardAPI).
type EvictorAPI interface {
	EnsureRoom(ctx context.Context, estBytes int64) (admitted bool, err error)
}
```
Hook (insert after the disk-guard block at `jobs.go:209`):
```go
if h.evictor != nil {
	admitted, err := h.evictor.EnsureRoom(r.Context(), body.SizeBytes) // SizeBytes = pre-admit estimate
	if err != nil {
		if h.log != nil { h.log.Warnw("budget gate check failed", "error", err) }
		// fail-open: don't 500 the admin upload on a budget-read blip (mirror disk-guard fail-open)
	} else if !admitted {
		if h.metrics != nil { h.metrics.IncRejectedTotal("budget_full") }
		writeInsufficientStorage(w)
		return
	}
}
```
> The handler reuses `createJobRequest.SizeBytes` (`jobs.go:132`) as the estimate (admin
> uploads carry the release size). DiskGuard stays — both gate (EVICT-05).

---

### `internal/handler/jobs.go` — Planner pre-admit (Planner's `plan()`, NOT the handler)

**Analog:** the Planner enqueue point at `planner.go:286-308` (`p.jobs.Create(...)`).
Per CONTEXT line 64 + spec §6, insert the SAME `EnsureRoom` + reject gate BEFORE
`p.jobs.Create(ctx, job)` in `planner.go plan()`. Use the selected release size as the
estimate: `rel.SizeBytes` (already on the `domain.Release`, `planner.go:296`), falling
back to `avg_raw_ep_size`(~1.2 GB) when 0 (CONTEXT line 50). On reject:
`p.metrics.IncRejectedTotal("budget_full")` + return (LEAVE or DROP demand — see flagged
question). Add an `evictor`-shaped seam to the Planner struct mirroring its existing
seams (`planner.go:95-102`).

---

### `internal/metrics/library_metrics.go` (MODIFY) — 2 counters + 3 gauges

**Analog:** clone the existing collectors verbatim — same file, same `promauto.With(reg)`
factory pattern (`library_metrics.go:75-172`), same `m == nil`-guarded `Inc`/`Set`
methods, same `GetXForTest` seams.

- `evictedTotal *CounterVec{source}` ← clone `enqueueRejectedTotal` (`:103-109`) → method `IncEvictedTotal(source)` like `IncEnqueueRejected` (`:208-210`) + `GetEvictedTotalForTest`.
- `rejectedTotal *CounterVec{reason}` ← clone `autocacheServeTotal` (`:156-162`); name `library_autocache_rejected_total`, label `reason="budget_full"`. Method `IncRejectedTotal(reason)` + test seam. (Distinct from the existing `library_enqueue_rejected_total{disk_full}`.)
- `bytesUsed *GaugeVec{source,freshness}` ← there is NO GaugeVec yet; use `factory.NewGaugeVec` (the `Gauge` analogs are `diskFreeBytes` `:97-102` + `SetDiskFreeBytes` `:200-202`). Method `SetBytesUsed(source, freshness string, n int64)`.
- `budgetBytes prometheus.Gauge` ← clone `diskFreeBytes` exactly (plain Gauge). Method `SetBudgetBytes(n int64)`.
- `episodes *GaugeVec{source,freshness}` ← same GaugeVec idiom as bytesUsed. Method `SetEpisodes(source, freshness string, n int64)`.

Names locked by spec §7 / CONTEXT line 88-90: `library_autocache_evicted_total{source}`,
`library_autocache_rejected_total{reason}`, `library_autocache_bytes_used{source,freshness}`,
`library_autocache_budget_bytes`, `library_autocache_episodes{source,freshness}`.
Follow the in-file convention of a dated comment block on the struct field (e.g. the
"Phase 09" block at `:55-61`) — add a "Phase 10 (eviction/budget)" block.

---

### `cmd/library-api/main.go` (MODIFY) — construct + Start Evictor + handler wiring

**Analog:** TWO existing wirings in the same file:
1. `diskGuard := service.NewDiskGuard(...)` + `go diskGuard.Run(rootCtx, …)` (`main.go:266-267`) — the periodic-sweep loop pattern.
2. `planner := autocache.NewPlanner(demandRepo, episodeRepo, jobRepo, …)` + `planner.Start(rootCtx)` (`main.go:470-480`) — construct-from-already-built-repos, no reconstruction.

Construct the `Evictor` from the already-built `episodeRepo` (now carrying the 3 new
methods), `minioWriter` (DeletePrefix), `autocacheConfigRepo`, `libMetrics`, `log`; call
`evictor.Start(rootCtx)` for the sweep; pass the same `evictor` into the `JobsHandler`
constructor for the admin pre-admit gate, and into `autocache.NewPlanner` for the Planner
gate. Mirror the `planner.Stop()` shutdown wiring.

---

## Shared Patterns

### Concurrency — serialize pre-admit vs sweep (CONTEXT line 99)
**Source:** new — but the lock idiom is `planner.go:104-107,357-369` (`sync.Mutex` guarding
`lastSearched`). The Evictor's `mu` guards the WHOLE read-budget→evict→delete cycle in
both `EnsureRoom` (pre-admit) and `Sweep` (`runOnce`), so the admin handler, the Planner,
and the sweep can't race-double-spend the same freed headroom. One mutex, held across the
SumPoolBytes→ListCandidates→evict loop.

### Eviction ordering — objects FIRST, then row (CONTEXT line 56-57, 101)
**Source:** invert `minio/writer.go` `Move` (`:331-369`). The locked decision:
1. `minioWriter.DeletePrefix(ctx, ep.MinioPath)` — delete MinIO objects.
2. ONLY if (1) succeeds, `episodeRepo.DeleteByID(ctx, ep.ID)` — delete the row.
3. If (1) fails: LEAVE the row (skip this candidate, `continue`), so it retries next
   sweep. Rationale: tolerate orphaned OBJECTS over an orphaned ROW (a row pointing at
   deleted objects = serving pointer to missing data). This is why `DeletePrefix`
   hard-fails on first RemoveObject error (unlike `Move`'s soft-fail).

### Live config read (budget + freshness windows)
**Source:** `repo/autocache_config.go` `Get` (`:30-36`) — already a finished accessor.
The Evictor reads `cfg.BudgetBytes`, `cfg.AutoFreshDownloadDays`, `cfg.AutoFreshFetchDays`,
`cfg.AdminFreshDays` LIVE each sweep / pre-admit (NOT cached at boot) so an admin PATCH
applies with no redeploy — same live-read discipline the Planner uses (`planner.go:174`).
Reuse the `configGetter` seam shape (`planner.go:64-68`) verbatim.

### Ticker lifecycle (Start/Stop/loop/runOnce/sleep + cfg.Enabled no-op)
**Source:** `planner.go:138-215, 394-407`. Copy the channel-based Start/Stop, the
`runOnce`-returns-cadence pattern (unit-driven, no ticker in tests), `minSweepInterval`
floor, and the `!cfg.Enabled` early-return. The disk guard's simpler `Run(ctx, interval)`
+ `time.NewTicker` (`disk_guard.go:78-97`) is an alternate but the Planner pattern is
preferred (it re-reads cadence live + is already unit-tested in this package).

### Metric Inc/Set + nil-guard + GetXForTest
**Source:** `library_metrics.go` throughout — every method is `m == nil`-guarded
(`:235-238`), empty-label-normalized where relevant (`:255-262`), and paired with a
`GetXForTest` seam returning the raw `prometheus.Counter` for `testutil.ToFloat64`.

---

## No Analog Found

None. Every Phase-10 file extends or mirrors an existing file in `services/library`.
The only genuinely-new logic with no line-for-line analog is the `EnsureRoom` budget
arithmetic and the 4-tier `ListStaleEvictionCandidates` SQL — both are composed from
existing repo/minio/loop primitives (sketched above), not copied.

---

## Flags for the Planner (open decisions from CONTEXT §Claude's Discretion)

1. **Shared `EnsureRoom` helper location — RECOMMEND: a method on `autocache.Evictor`**,
   injected via a local seam into BOTH `handler.JobsHandler` (admin upload, `jobs.go:190`)
   and `autocache.Planner.plan()` (`planner.go` before Create). This keeps the budget
   logic in ONE place (the autocache pkg, beside the freshness rules) and matches the
   "local interface seam, main.go owns concrete wiring" idiom both the Planner and the
   JobsHandler already use (`DiskGuardAPI`, `demandDrainer`). Do NOT duplicate the
   arithmetic in the handler.

2. **Pre-admit only vs also periodic sweep — RECOMMEND BOTH** (CONTEXT line 68, spec §6a/b).
   Pre-admit `EnsureRoom` gates every download; the periodic `Sweep` (Evictor ticker)
   reconciles drift, proactively reclaims newly-Stale rows, AND refreshes the Accountant
   gauges (`bytes_used`/`episodes`/`budget_bytes`) so Phase 11 has live series even when
   no download is flowing. Both share the one mutex.

3. **Object-then-row delete ordering — DECIDED (see Shared Patterns):** DeletePrefix
   first (hard-fail on error), DeleteByID only on success, skip+retry on object-delete
   failure. Tolerate orphaned objects, never an orphaned row.

4. **4-tier SQL + NULL handling — sketched above.** Keep freshness day-windows as bound
   `?` params (not interpolated); `last_fetch_at IS NULL` ⇒ never-fetched tier (1/3),
   order those by `downloaded_at`; fetched tiers (2/4) order by `last_fetch_at`. NULL
   `downloaded_at` (just-migrated rows) ⇒ treat as very-old (classify by `last_fetch_at`
   only); use `COALESCE(last_fetch_at, downloaded_at, created_at)` as the within-tier
   sort key. Co-locate a pure-Go `Classify(ep, cfg, now)` for unit tests.

5. **Reject policy on Planner enqueue (LEAVE vs DROP the demand) — OPEN.** CONTEXT line 112
   flags it: dropping risks losing the demand if the pool later shrinks; leaving risks a
   hot-loop re-search on a permanently-too-big pool. RECOMMEND: LEAVE the demand row but
   arm the existing per-(mal,ep) `searchBackoff` (`planner.go:88,365`) so a budget-rejected
   episode isn't retried every tick — reuses the Planner's existing backoff machinery, no
   new state.

6. **Estimate source:** admin path → `createJobRequest.SizeBytes` (`jobs.go:132`); Planner
   path → `rel.SizeBytes` (`planner.go:296`) with an `avg_raw_ep_size`(~1.2 GB) fallback
   when 0 (CONTEXT line 50). Note: `avg_raw_ep_size` is NOT yet a config column — either
   add a const in `evictor.go` or read it from a new `autocache_config` field (flag if a
   migration is wanted; a const is lower-risk for Phase 10).

## Metadata

**Analog search scope:** `services/library/internal/{autocache,repo,minio,metrics,handler,service,domain}` + `cmd/library-api/main.go`
**Files scanned:** 11 (planner.go, episode.go, writer.go, disk_guard.go, autocache_config.go {repo+domain}, library_metrics.go, jobs.go, episode.go {domain}, main.go) + design spec §3.5/§6/§7 + 3 phase SUMMARYs
**Pattern extraction date:** 2026-06-17
