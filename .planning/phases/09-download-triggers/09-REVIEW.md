---
phase: 09-download-triggers
reviewed: 2026-06-17T11:00:00Z
depth: deep
files_reviewed: 13
files_reviewed_list:
  - services/library/internal/autocache/planner.go
  - services/library/internal/autocache/raw_filter.go
  - services/library/internal/handler/autocache_internal.go
  - services/library/internal/repo/demand.go
  - services/library/internal/repo/job.go
  - services/library/internal/repo/episode.go
  - services/library/internal/domain/job.go
  - services/library/migrations/008_autocache_job_source.sql
  - services/library/migrations/009_library_jobs_episode.sql
  - services/library/migrations/010_autocache_demand_ongoing.sql
  - services/library/cmd/library-api/main.go
  - services/player/internal/service/progress.go
  - services/player/internal/service/autocache_demand.go
  - services/player/internal/repo/progress.go
  - services/scheduler/internal/jobs/autocache_logic_a.go
  - services/scheduler/cmd/scheduler-api/main.go
  - services/scheduler/internal/service/job.go
findings:
  critical: 1
  warning: 6
  info: 3
  total: 10
status: resolved
resolved_at: 2026-06-17T11:30:00Z
resolution: "CR-01 + WR-01/02/03/04 fixed; WR-05/WR-06 + IN-01/02/03 accepted"
---

# Phase 9: Code Review Report

**Reviewed:** 2026-06-17T11:00:00Z
**Depth:** deep
**Files Reviewed:** 13 (+ DI / repo seams)
**Status:** resolved (fixes applied 2026-06-17)

## Resolution (2026-06-17)

All correctness/resource findings fixed, each committed atomically; the
remaining findings are accepted-as-is with explanatory in-code comments.

| Finding | Disposition | Commit / Note |
|---------|-------------|---------------|
| CR-01 | **Fixed** | `Retry` now copies `Episode` (extracted `retryRowFrom`); unit + integration tests added. Single-flight survives retry. |
| WR-01 | **Fixed** | `Record` ON CONFLICT no longer bumps `requested_at` → stable first-seen FIFO; no starvation. sqlite FIFO test added. |
| WR-02 | **Fixed** | `Record` ON CONFLICT now refreshes `reason` (last-writer-wins) → correct OBS-04 trigger attribution. sqlite re-assert test added. |
| WR-03 | **Fixed** | Planner `gcBackoff()` per-sweep eviction + `forgetSearched()` on present-delete bound the `lastSearched` map. Tests added. |
| WR-04 | **Fixed** | RAW filter fails closed for unknown uploaders (allowlist OR positive raw token required); denylist expanded. Tests updated. |
| WR-05 | **Accepted** | Cross-DB env-mirror drift is a deliberate boundary consequence; documented in `scheduler/internal/config/config.go`. |
| WR-06 | **Accepted** | `episodes_aired` Shikimori-sync latency is inherent; flagged in `player/internal/service/progress.go`. |
| IN-01 | **Accepted** | Keyword search is a known no-op; MAL-feed path is load-bearing (already documented in `searchQueryFor`). |
| IN-02 | **Accepted** | Cosmetic double-cast; no behavioral impact. |
| IN-03 | **Accepted** | `s.log` is always non-nil in production wiring; low-risk consistency nit. |

Verification: `services/library` + `services/scheduler` build + vet + test
clean; `services/player` build + vet clean (the pre-existing MAL-export handler
test that makes a live scheduler network call is unrelated and out of scope).

## Summary

Phase 9 wires the v4.1 download-trigger stack: a config-gated library Planner drain loop (TRIG-03/04/05), a player Logic-B next-episode pull (TRIG-02), a scheduler Logic-A ongoing-push producer (TRIG-01), the RAW/quality/seeder release filter (TRIG-05), and three idempotent migrations. `go build ./...` and `go vet ./...` are clean across all three services, and the autocache + repo unit suites pass.

The architecture is sound and the obvious traps were avoided: the Planner runs a single goroutine (no concurrent ticks → no double-enqueue), the no-release backoff rate-limits the "as soon as on torrents" retry, the DemandProducer drop-on-full + nil-safety never blocks the heartbeat, the Logic-B N+1 vs `episodes_aired` bound is correct, and the scheduler join is fully parameterized (no SQL injection). The migrations are idempotent and run as separate autocommit `db.Exec` calls, so the `ALTER TYPE ADD VALUE` enum extensions are committed before any consumer uses the new literal (no in-transaction enum landmine).

The one BLOCKER is a cross-feature interaction: the admin `Retry` path drops the new `Episode` column, which defeats the TRIG-04 single-flight dedup and causes duplicate downloads. The remaining findings are robustness/correctness gaps around demand-row lifecycle (FIFO starvation, reason-not-refreshed, unbounded backoff map) and the RAW filter's permissive default.

No scope leak into Phase 10 (evictor) or Phase 11 (charts) was found — the Planner enqueues only; no eviction/budget logic is present.

## Critical Issues

### CR-01: `JobRepository.Retry` drops the `Episode` column → breaks TRIG-04 single-flight, duplicate autocache downloads

**File:** `services/library/internal/repo/job.go:345-356`
**Issue:** `Retry` constructs the fresh re-enqueue row by copying `Source, Magnet, Title, Uploader, Quality, SizeBytes, ShikimoriID, Status, ProgressPct, ErrorText` — but **not** `Episode` (the nullable intended-episode column added by migration 009). An autocache-sourced job carries a non-null `Episode` precisely so `HasActiveForEpisode(shikimori_id, episode)` can collapse concurrent demand to one job (job.go:111-126, the TRIG-04 gate). When an autocache job fails and an admin retries it, the new `queued` row has `episode = NULL`. The next Planner sweep re-drains the still-present demand row (the present-check fails — episode not in pool — and the in-flight check `HasActiveForEpisode` does **not** match the retried row because its `episode` is NULL), so the Planner **enqueues a second download for the same episode**. The single-flight invariant the entire phase relies on is silently broken for any retried autocache job.

This is a genuine cross-file defect: `Retry` predates Phase 9 and was never updated when `domain.Job.Episode` was added. The `*int` pointer copies cleanly (nil stays nil for non-autocache rows, so manual/admin retries are unaffected).

**Fix:**
```go
fresh := &domain.Job{
    Source:      old.Source,
    Magnet:      old.Magnet,
    Title:       old.Title,
    Uploader:    old.Uploader,
    Quality:     old.Quality,
    SizeBytes:   old.SizeBytes,
    ShikimoriID: old.ShikimoriID,
    Episode:     old.Episode, // <-- preserve intended episode so TRIG-04 dedup survives retry
    Status:      domain.JobStatusQueued,
    ProgressPct: 0,
    ErrorText:   formatRetryErrorText(oldID),
}
```

## Warnings

### WR-01: FIFO starvation — `Record` refreshes `requested_at`, `Drain` orders by it ASC, fanout cap is 5

**File:** `services/library/internal/repo/demand.go:43-47` + `services/library/internal/autocache/planner.go:189,197-206`
**Issue:** `Drain` returns rows `ORDER BY requested_at ASC` (oldest first) and the Planner only *searches* the first `searchFanoutLimit` (5) un-backed-off rows per sweep. But `Record`'s ON CONFLICT path resets `requested_at = now()` on every re-assert. Logic A (scheduler) re-asserts every ongoing demand every 20 min, continuously stamping those rows with a fresh `now()` and pushing them to the **back** of the FIFO. Meanwhile static backfill demands (whose `requested_at` never refreshes) permanently occupy the front of the queue. If a handful of un-findable backfill episodes pile up, they monopolize the front of the ASC order; the only thing saving newer ongoing demands from starvation is the 1-hour `searchBackoff` skipping the stuck backfill rows from re-search (they still consume the present + in-flight DB checks each sweep, but not a search slot). The starvation is *latent* (mitigated by backoff) but fragile: any tuning that shortens `searchBackoff` below `sweep_interval_min × (demand_count / fanout)` reopens it, and the present/in-flight checks for stale rows still grow O(demand_count) per sweep.

**Fix:** Order the drain by something that does not get refreshed by re-assert — e.g. add a never-updated `created_at`/`first_seen_at` column and `ORDER BY first_seen_at ASC`, or have `Drain` exclude rows whose `(mal,ep)` is in the in-memory backoff window before applying the limit (push the backoff filter into the query / a pre-filter so backed-off rows don't consume drain-batch slots). At minimum, document the invariant and add a test that interleaves a re-asserted ongoing demand with a stuck backfill demand and asserts the ongoing one is still reached within N sweeps.

### WR-02: `Record` ON CONFLICT never refreshes `reason` → wrong OBS-04 trigger attribution

**File:** `services/library/internal/repo/demand.go:43-46`
**Issue:** The upsert's `DoUpdates` only assigns `requested_at`. If a `(mal,ep)` row already exists as `backfill` (catalog serve-MISS) and Logic A later re-asserts the same episode as `ongoing` (or Logic B as `next_ep`), the stored `reason` stays `backfill`. The Planner then derives `trigger="backfill"` (planner.go:215 → triggerForReason) for a download that was actually driven by an ongoing/next_ep producer, so the `library_autocache_downloads_total{trigger}` metric mis-attributes the download. CONTEXT decision 7 explicitly wanted A/B/backfill separable; this collapses them on any episode that was ever a backfill miss first. Not a data-loss bug, but it defeats the metric the phase added the `ongoing` enum value for.

**Fix:** Either add `"reason": reason` to the `DoUpdates` map (last-writer-wins on reason), or define a precedence (e.g. `next_ep > ongoing > backfill`) and `DoUpdates` with a `CASE`/`GREATEST`-style expression so a stronger reason can upgrade a weaker one but not vice-versa. Pick one and add a round-trip test.

### WR-03: Unbounded growth of the `lastSearched` backoff map (slow memory leak)

**File:** `services/library/internal/autocache/planner.go:107,354-358`
**Issue:** `markSearched` inserts `lastSearched[demandKey] = time.Now()` and nothing ever deletes from the map. Every distinct `(mal,ep)` the Planner ever searches leaves a permanent entry. Over a long-running process with a churning catalog (new episodes weekly across thousands of ongoing titles), the map grows without bound. The `inBackoff` check reads stale entries forever even after the demand is satisfied and deleted. This is a robustness defect, not a hot-path perf issue (lookups stay O(1)), so it is in-scope as unbounded state growth.

**Fix:** Evict on a successful enqueue/present-delete (the `(mal,ep)` is no longer wanted), and/or sweep entries older than `searchBackoff` at the top of each `runOnce`:
```go
func (p *Planner) gcBackoff() {
    p.mu.Lock()
    defer p.mu.Unlock()
    for k, t := range p.lastSearched {
        if time.Since(t) >= searchBackoff {
            delete(p.lastSearched, k)
        }
    }
}
```
Call it once per sweep before processing rows. Also `delete(p.lastSearched, k)` when the present-check deletes the demand row.

### WR-04: RAW filter defaults UNKNOWN uploaders to "is RAW" — admits DUB/dual-audio releases that lack a negative token

**File:** `services/library/internal/autocache/raw_filter.go:76-87`
**Issue:** `isRAW` returns `true` for any title that merely lacks a negative token, regardless of uploader (the allowlist is only a short-circuit, not a requirement). The negative-token regex (`dub|dual-audio|multi-audio|eng-dub|hardsub`) is a small denylist; a great many real dub/dual-audio releases are titled without any of those exact tokens (e.g. "[Group] Anime 05 (BD 1080p) [ENG]", "...[Multi-Subs]", "...[10bit AAC]", Japanese-named groups, or AV1/HEVC encodes with no audio marker). Those pass `isRAW` and, if seeded enough and ≤ cap, get auto-downloaded as a "RAW" — defeating the D3 contract that one RAW (JP-audio) serves SUB demand via the overlay. This is the documented "best-effort heuristic" (RESEARCH Pitfall 3), but defaulting *open* on unknown uploaders is the riskiest possible default for an automated downloader, because a wrong pick burns disk budget and is served to users as raw JP.

**Fix:** Invert the default to fail-closed for unknown uploaders: require allowlist membership OR a positive raw signal (e.g. a `raw`/`webrip-jp` token), and treat "unknown uploader + no negative token + no positive raw token" as **not** RAW (skip, leave the demand for a later better-seeded allowlisted release). If the open default is a deliberate product call, gate it behind a config flag rather than hard-coding it, and expand the negative-token list (`eng`, `multi-?sub`, `bd-eng`, `aac`-style audio markers are weak but `\beng\b`/`\benglish\b` audio markers are worth adding).

### WR-05: `active_watcher_days` env-mirror drift — two independent sources of truth for the same tunable

**File:** `services/scheduler/internal/config/config.go:88-90,135-137` (env `AUTOCACHE_ACTIVE_WATCHER_DAYS`, default 30) vs `services/library/migrations/006_autocache_config.sql` (`active_watcher_days INT NOT NULL DEFAULT 30`, the authoritative live-editable value)
**Issue:** Logic A computes its D8 recency cutoff (`autocache_logic_a.go:93`) from the scheduler env mirror `activeWatcherDays`, but the *authoritative* `active_watcher_days` lives in the library `autocache_config` table and is admin-PATCH-editable at runtime. The two can silently diverge: an admin lowers `active_watcher_days` to 7 in the library config to shrink the active-watcher window, but Logic A keeps enumerating with the stale 30-day env default until someone also edits the scheduler env and redeploys. The result is Logic A firing demand for anime whose last watcher was 8-30 days ago — exactly the over-fetch the tunable exists to prevent. The SUMMARY acknowledges this is "a scheduler env mirror... keep the two in sync," which means the design has a known drift hazard with no enforcement.

**Fix:** Make the scheduler read the authoritative value rather than mirror it. The scheduler already calls the library internal endpoint for demand; expose `active_watcher_days` (e.g. via a `GET /internal/library/autocache/config` or fold it into the demand response) and have Logic A read it per-sweep, falling back to the env default only if the fetch fails. If a cross-service read is too heavy, at minimum log a WARN at boot when the env value is non-default so the drift is visible, and document the redeploy requirement in CLAUDE.md's env section.

### WR-06: Logic-B `episodes_aired` source can mismatch the Planner's `present`/library episode universe (off-by-one is correct; the data source is not)

**File:** `services/player/internal/repo/progress.go:171` + `services/player/internal/service/progress.go:123-127`
**Issue:** The N+1 vs `episodes_aired` bound itself is correct: `next := req.EpisodeNumber + 1; if next > episodesAired { return }` admits exactly `next ∈ [1, episodesAired]`, no off-by-one. The risk is the *meaning* of `episodes_aired`: it is `animes.episodes_aired` (catalog/Shikimori-sourced, `COALESCE(...,0)`). When Shikimori lags the actual airing (common for simulcasts — the episode is on torrents hours before Shikimori bumps the count), `episodes_aired` is stale-low and Logic B never fires for the genuinely-aired N+1, defeating the "max lead time" goal. Conversely if Shikimori counts a not-yet-released episode, Logic B fires a demand that the Planner can never satisfy (no release), arming an hour-long backoff for nothing. Logic A has the same `episodes_aired` dependency (`autocache_logic_a.go:103`). This is a correctness ceiling on the trigger, not a crash, but it means the feature's freshness is bounded by Shikimori sync latency rather than torrent availability.

**Fix:** Acceptable as-is for v1 if documented, but worth a follow-up: treat `episodes_aired` as a soft hint, not a hard gate — e.g. allow firing for `next == episodes_aired + 1` (one episode of lookahead) and lean on the Planner's no-release backoff to absorb the false positive, since a not-yet-on-torrents episode just sits in backoff cheaply. At minimum add a code comment flagging the Shikimori-sync-latency coupling so it isn't mistaken for a guaranteed-fresh bound.

## Info

### IN-01: `searchQueryFor` builds a keyword query from `mal_id + episode` with no title — low search recall

**File:** `services/library/internal/autocache/planner.go:327-329`
**Issue:** The Planner has no title for a bare `mal_id`, so the keyword query is literally `"<malID> <episode>"` (e.g. `"42 5"`). For Jackett/Nyaa text search this is near-useless recall; the only thing that actually finds releases is the `SearchQuery.MALID` AnimeTosho MAL-feed path (planner.go:254). Any provider in the tiered search that relies on the keyword string (not the MAL id) will return nothing. The comment acknowledges "best-effort," but `"42 5"` as a torrent search term is effectively a no-op for keyword-based indexers. Consider resolving the title once (catalog lookup, cached) so the keyword tier contributes; otherwise the Planner silently depends on AnimeTosho's MAL feed being the only working tier.

**Fix:** Document that keyword search is a no-op for autocache and the MAL-feed path is load-bearing, or wire a cached title resolver.

### IN-02: `validateDemandReason` round-trips the raw string through `domain.DemandReason(raw)` twice

**File:** `services/library/internal/handler/autocache_internal.go:131-138`
**Issue:** Minor: the `switch` converts `raw` to `DemandReason` for the case match, then the body re-converts `domain.DemandReason(raw)` to return it. Harmless but slightly redundant; returning the matched constant directly is clearer and avoids re-casting an already-validated value.

**Fix:**
```go
switch r := domain.DemandReason(raw); r {
case domain.DemandReasonBackfill, domain.DemandReasonNextEp, domain.DemandReasonOngoing:
    return r
default:
    return domain.DemandReasonBackfill
}
```

### IN-03: `maybeFireNextEpDemand` dereferences `s.log` without a nil guard (inconsistent with `demand`/`logicB` nil-safety)

**File:** `services/player/internal/service/progress.go:115-117`
**Issue:** The function nil-guards `s.logicB` and `s.demand` (line 109) but calls `s.log.Warnw(...)` unguarded on the lookup-error path. In production `log` is always set (NewProgressService passes it), so this never fires today — but the surrounding code's defensive style (and the DemandProducer's explicit nil-receiver contract) implies log could be nil in a test/alt-wiring, where this would panic on the error branch. Low risk; flagged for consistency.

**Fix:** Guard `if s.log != nil` before the `Warnw`, matching the planner's `if p.log != nil` pattern.

---

_Reviewed: 2026-06-17T11:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
