# Phase 10: Eviction & Budget - Context

**Gathered:** 2026-06-17
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped) — enriched from design spec §6 + codebase scout

<domain>
## Phase Boundary

The whole first-party `aeProvider/` pool stays self-managing under ONE budget: Fresh content is
protected, Stale content is evicted in a fair source-ranked order, and an unfittable download is
cleanly rejected rather than blowing the budget. Library-only phase (the evictor operates on the
pool the library owns; producers/triggers are upstream).

**Requirements:** EVICT-01 (configurable budget, default 100 GB), EVICT-02 (source-specific
Fresh/Stale classification), EVICT-03 (Stale-only ordered eviction; Fresh never evicted),
EVICT-04 (reject + metric when Stale queue can't free enough), EVICT-05 (logical budget +
physical `DiskGuard` both gate a download).

**Out of scope:** Grafana panels (Phase 11 — but Phase 10 MUST emit the eviction/rejection
counters + the bytes_used/budget gauges so P11 can chart them). No producer/trigger changes
(Phase 9 done). No new download mechanics.
</domain>

<decisions>
## Locked Decisions (design spec §6 + §3.5)

- **Budget = Σ `library_episodes.size_bytes` over the `aeProvider/` pool (source ∈ {admin,autocache})**
  ≤ `autocache_config.budget_bytes` (default 107374182400 = 100 GiB). One unified pool, admin+auto.
- **Fresh/Stale (per row, evaluated at eviction time):**
  - autocache Fresh ⟺ now−downloaded_at < `auto_fresh_download_days`(10) OR now−last_fetch_at < `auto_fresh_fetch_days`(3).
  - admin Fresh ⟺ now−downloaded_at < `admin_fresh_days`(30) OR now−last_fetch_at < `admin_fresh_days`(30).
  - else Stale. (downloaded_at may be NULL for an unfetched-just-migrated row — treat NULL
    downloaded_at as "very old" so it classifies by last_fetch_at only; NULL last_fetch_at = never fetched.)
- **Eviction order (ONLY Stale rows are eligible; Fresh is NEVER deleted):**
  1. autocache · never-fetched (last_fetch_at IS NULL) → oldest downloaded_at first
  2. autocache · fetched → oldest last_fetch_at first
  3. admin · never-fetched → oldest downloaded_at first
  4. admin · fetched → oldest last_fetch_at first
  Delete from the top until enough room is freed.
- **Reject-when-full (EVICT-04):** if draining the ENTIRE Stale queue still can't fit the incoming
  download, REJECT it and increment `library_autocache_rejected_total{reason="budget_full"}`. This
  applies to autocache Planner enqueues AND (per design §2 D7) admin manual uploads.
- **Pre-admit gate (EVICT-05):** a download proceeds only when BOTH (a) the logical budget can fit
  it (after eviction if needed) AND (b) the existing physical-disk `DiskGuard` passes. The logical
  budget is layered ON TOP of DiskGuard (which stays — do NOT replace it).
- **Incoming size estimate:** at Planner enqueue time the real encoded size isn't known yet. Use
  the selected torrent `Release` size (or `avg_raw_ep_size` fallback ~1.2 GB) as the pre-admit
  estimate; the actual `size_bytes` is reconciled when the episode row is written post-encode
  (existing behavior). Budget math may briefly under/over-count by the estimate delta — acceptable.

### Eviction mechanics
- Evict a row = delete its MinIO objects under its `minio_path` prefix (the writer has
  `ListObjectsByPrefix` + `RemoveObject`) THEN delete the `library_episodes` row. Order: objects
  first, then row (if object-delete fails, leave the row so it retries — never orphan the row
  pointing at deleted objects... actually delete row only after objects gone to avoid a serving
  pointer to missing data; tolerate orphaned objects over orphaned rows). Decide + document.
- Emit `library_autocache_evicted_total{source}` per eviction.
- The **Accountant** computes bytes_used (Σ size_bytes) + Fresh/Stale split and publishes the
  gauges `library_autocache_bytes_used{source,freshness}` + `library_autocache_budget_bytes` +
  `library_autocache_episodes{source,freshness}` (Phase 11 charts these; Phase 10 emits them).

### Claude's Discretion
- Where the pre-admit gate hooks into the Planner (`planner.go` `plan()` ~line 221, before
  `jobRepo.Create`) vs a shared `Evictor.EnsureRoom(estimatedBytes) (admitted bool)` helper used by
  both the Planner and the admin upload path.
- Whether eviction also runs on a periodic sweep (recommended — reconciles drift + reclaims Stale
  proactively) in addition to pre-admit.
- Exact repo queries (a single ordered "stale eviction candidates" query vs in-Go sort) and the
  bytes-sum query. Prefer a SQL ordered query for the eviction candidate list.
</decisions>

<code_context>
## Existing Code Insights
- `services/library/internal/autocache/planner.go` — `plan()` (~line 221) is the enqueue path;
  hook the pre-admit budget/eviction gate before `jobRepo.Create`. `Start/Stop/loop/runOnce` show
  the ticker pattern to mirror for a periodic eviction sweep.
- `services/library/internal/minio/writer.go` — `ListObjectsByPrefix(ctx, prefix)` + the
  `RemoveObject` adapter (line 56/89) = the object-delete primitives. `Move` (line 331) shows the
  list-then-act pattern to mirror for delete.
- `services/library/internal/repo/episode.go` — `List`, `GetByShikimoriEpisode`, `BumpFetch`. ADD:
  a bytes-sum query over the pool + an ordered "stale eviction candidates" query + a row-delete.
  `library_episodes` has source/track/downloaded_at/last_fetch_at/fetch_count/size_bytes (Phase 7).
- `services/library/internal/service/disk_guard.go` — `DiskGuard.Allow(minFreePct)` / `Check()` —
  the physical gate that must ALSO pass (EVICT-05). Layer the logical budget on top.
- `services/library/internal/repo/autocache_config.go` — `Get()` for budget_bytes + freshness
  windows (the evictor reads these live).
- `services/library/internal/metrics/library_metrics.go` — add `evicted_total{source}`,
  `rejected_total{reason}` counters + `bytes_used{source,freshness}` / `budget_bytes` /
  `episodes{source,freshness}` gauges (clone the existing CounterVec/Gauge patterns).
- `services/library/internal/handler/jobs.go` — the admin manual-upload/enqueue path (jobs.go:158)
  must ALSO consult the budget gate per design D7 (admin upload rejected when pool can't fit).

## Pitfalls
- downloaded_at NULL (just-migrated admin rows pre-first-fetch) — classify carefully (NULL ≠ now).
- Don't evict a row whose download/encode is still in-flight (no episode row yet → not counted; but
  a freshly-written row could be evicted immediately if Stale — guard: a row younger than its
  downloaded_at window is Fresh by rule 1, so newly-downloaded autocache rows are Fresh 10d, safe).
- Concurrency: the eviction sweep and the pre-admit gate can race; serialize via a mutex or run
  eviction only inside the single Planner goroutine.
- Object-delete vs row-delete ordering (avoid serving pointer to missing data).
</code_context>

<specifics>
## Specific Ideas
- `Evictor` (autocache pkg): `BytesUsed()`, `Classify(ep) Fresh|Stale`, `EnsureRoom(estBytes) (admitted bool, err)`
  (evicts Stale in the locked order until room or queue exhausted → reject), periodic `Sweep()`.
- Repo: `SumPoolBytes()`, `ListStaleEvictionCandidates(cfg, now) []Episode` (SQL ordered by the
  4-tier rule), `DeleteByID(id)`.
- minio: `DeletePrefix(ctx, prefix)` (list + RemoveObject each).
- Planner pre-admit: before enqueue, `Evictor.EnsureRoom(estimate)` + `DiskGuard.Allow(...)`; on
  reject, increment rejected_total + leave/drop the demand per policy (drop to avoid hot-loop on a
  permanently-too-big pool? or leave with backoff — decide).
- Accountant gauges refreshed on the sweep.
- Admin upload path consults the same `EnsureRoom`.
</specifics>

<deferred>
## Deferred Ideas
- Grafana storage/eviction panels → Phase 11 (emit the gauges/counters now).
- Predictive eviction / AI → v2.
</deferred>
