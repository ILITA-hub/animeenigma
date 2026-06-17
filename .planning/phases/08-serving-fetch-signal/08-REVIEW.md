---
phase: 08-serving-fetch-signal
reviewed: 2026-06-17T06:54:02Z
depth: deep
files_reviewed: 9
files_reviewed_list:
  - services/library/internal/handler/autocache_internal.go
  - services/library/internal/repo/episode.go
  - services/library/internal/repo/demand.go
  - services/library/internal/domain/autocache_demand.go
  - services/library/internal/metrics/library_metrics.go
  - services/library/internal/transport/router.go
  - services/library/cmd/library-api/main.go
  - services/library/migrations/007_autocache_demand.sql
  - services/catalog/internal/parser/library/client.go
  - services/catalog/internal/service/raw_resolver.go
findings:
  critical: 1
  warning: 2
  info: 3
  total: 6
status: resolved
resolved: 2026-06-17T08:59:57Z
resolution:
  CR-01: fixed (set RequestedAt: time.Now() in DemandRepository.Record + SQLite recency test) — commit b9c2da27
  WR-01: fixed (bounded drop-on-full serveSignalSem + fireSignal helper + drop test) — commit 235d934e
  WR-02: fixed (maxEpisode=100000 upper-bound guard in decodeSignal + overflow tests) — commit 8faa0a8b
  IN-01: addressed incidentally (dead inner `r.library != nil` guards removed when wiring fireSignal)
  IN-02: not actioned (forward-compat note for Phase 09 — no Phase 08 action required)
  IN-03: not actioned (best-effort metric undercount during library outage — documented, no action required)
---

# Phase 8: Code Review Report

**Reviewed:** 2026-06-17T06:54:02Z
**Depth:** deep (cross-file: catalog producer → library consumer, call-chain traced)
**Files Reviewed:** 9
**Status:** resolved (CR-01 + WR-01 + WR-02 fixed and committed 2026-06-17; IN-01/02/03 note-only)

## Summary

Phase 8 wires the ae serve-signal path: catalog's `RawResolver.GetLibraryStream` fires
non-blocking HIT (`RecordFetch`) / MISS (`RecordDemand`) signals to library's two new
Docker-network-only `/internal/library/autocache/{fetch,demand}` endpoints, which bump
the episode ledger / upsert a backfill-demand row and increment `library_autocache_serve_total`.

Both services `go build ./...` and `go vet ./...` clean (verified). The design's stated
safety properties largely hold and were verified by tracing the call chain:

- **SERVE-03 no-regression holds.** Signal failures are dropped (`_ =`); `GetLibraryStream`
  returns the unchanged `NotFound` on MISS and the stream on HIT regardless of signal outcome.
  The auto-raw `GetStream` path (Phase 06) deliberately fires no signals — no scope leak.
- **Goroutine lifetime is bounded.** `context.WithoutCancel` strips the deadline, but the
  library client's `http.Client.Timeout = 2s` caps every fire-and-forget goroutine at ≤2s,
  so a hanging library cannot leak goroutines indefinitely (see WR-01 for the backpressure caveat).
- **`/internal/library/*` is unreachable from the gateway by construction** (gateway only
  proxies `/api/library/*`; verified in gateway router).
- **Enabled fail-closed is correct** — config-read error and `enabled == false` both skip the
  demand record + miss counter and still return 200.
- **No SQL injection** — all writes go through GORM parameterized queries / `gorm.Expr` literals;
  `mal_id`/`episode` are bound, never interpolated.
- **Reason forcing is correct** — the demand handler ignores the wire `reason` and hard-codes
  `DemandReasonBackfill`, so a spoofed `next_ep` can't be injected (T-08-05).

One BLOCKER: the `autocache_demand` insert path writes a year-1 `requested_at` because the
GORM model relies on a SQL `DEFAULT now()` it never triggers. This corrupts the recency
ordering Phase 9's Planner is built to drain on.

## Resolution (2026-06-17)

All blocking + warning findings fixed in the Phase-08 review-fix pass. Both services build,
vet, and test clean (`services/library` full `go test ./...`; `services/catalog`
`go test ./internal/service/ ./internal/parser/library/...`).

- **CR-01 — FIXED** (commit `b9c2da27`). `DemandRepository.Record` now sets
  `RequestedAt: time.Now()` explicitly rather than relying on the never-firing SQL
  `DEFAULT now()`. The `ON CONFLICT DO UPDATE SET requested_at = now()` refresh is unchanged.
  Added `demand_sqlite_test.go` asserting a fresh insert lands a recent (non-year-1) timestamp.
- **WR-01 — FIXED** (commit `235d934e`). Added a buffered-channel counting semaphore
  (`serveSignalSem`, cap 64) + `fireSignal` helper with DROP-ON-FULL semantics; both the HIT
  and MISS spawn sites now route through it. `context.WithoutCancel` + the client's 2s timeout
  are retained; the signal still never blocks or fails resolution. Added drop/spawn/nil tests.
- **WR-02 — FIXED** (commit `8faa0a8b`). `decodeSignal` now rejects `episode > maxEpisode`
  (100000) so an int4-overflowing value is 400'd at the edge instead of silently failing the
  DB write. Extended `TestAutocacheInternalBadBody` with overflow + over-max cases.
- **IN-01** — addressed incidentally: the dead inner `if r.library != nil` guards were removed
  when the spawn sites were rewired through `fireSignal` (the function already returns early on
  a nil library client).
- **IN-02 / IN-03** — note-only, no Phase 08 action required (forward-compat for Phase 09 /
  best-effort metric undercount during a library outage). Left as documented.

## Critical Issues

### CR-01: `autocache_demand.requested_at` is written as `0001-01-01` on insert (SQL `DEFAULT now()` never fires)

**Status:** FIXED (commit `b9c2da27`).

**File:** `services/library/internal/domain/autocache_demand.go:38` (with `services/library/internal/repo/demand.go:31-43` and `migrations/007_autocache_demand.sql:37`)

**Issue:**
The migration declares `requested_at TIMESTAMPTZ NOT NULL DEFAULT now()`, and the repo/domain
docs claim every row "reflects most-recent want". But the GORM model field —

```go
RequestedAt time.Time `gorm:"not null;column:requested_at" json:"requested_at"`
```

— is **not** a GORM auto-timestamp field. GORM only auto-populates fields literally named
`CreatedAt` / `UpdatedAt` (that's why `Job.CreatedAt` / `Episode.CreatedAt` work — same repo,
domain/job.go:68, domain/episode.go:61), and it only **omits** a zero-value column from the
INSERT (letting the SQL default apply) when the field carries a `default:` tag. `RequestedAt`
has neither: no magic name, no `default:` tag, no `autoCreateTime`, and no `BeforeCreate` hook
(verified — GORM v1.30.0, no hooks on the model).

Consequently `Record()` → `Create(row)` sends the zero `time.Time` and Postgres stores
`0001-01-01 00:00:00+00` instead of `now()`. The SQL `DEFAULT now()` is dead — it fires only
when the column is omitted from the INSERT, which GORM will not do here.

The `ON CONFLICT DO UPDATE SET requested_at = now()` path is correct, so a *second* demand for
the same `(mal_id, episode)` heals it — but the **first** insert of any newly-wanted episode
lands with a year-1 timestamp. Phase 9's Planner is explicitly specified to drain "most-recent
want" by `requested_at` (migration header + domain doc), so a freshly-demanded episode would
sort as the *oldest* row and any freshness/age window keys off a bogus timestamp. No test
guards the insert-path value (`demand_test.go` only greps the source for the on-conflict clause).

**Fix applied:** `DemandRepository.Record` now constructs the row with `RequestedAt: time.Now()`,
so the first insert lands a real timestamp (matching how Phase 7 CR-01 was handled — be explicit
rather than rely on a GORM-omitted SQL default). The on-conflict `now()` refresh is unchanged.
Added `services/library/internal/repo/demand_sqlite_test.go`
(`TestDemandRepository_Record_FirstInsertRequestedAtIsRecent`) which inserts a fresh row over an
in-memory SQLite DB and asserts `requested_at` is recent (year > 1, within a few seconds of now).

## Warnings

### WR-01: Unbounded fire-and-forget goroutine spawn on every ae stream resolution (no backpressure)

**Status:** FIXED (commit `235d934e`).

**File:** `services/catalog/internal/service/raw_resolver.go:398-402, 412-416`

**Issue:**
`GetLibraryStream` is reached from the public, gateway-proxied `GET /api/anime/{id}/ae/stream`
handler (`internal/handler/raw.go:122`). Every HIT and every MISS spawned a bare `go func(){...}`
with no worker pool, no semaphore, and no in-flight cap:

```go
go func(mal string, ep int) {
	_ = r.library.RecordFetch(context.WithoutCancel(ctx), mal, ep)
}(anime.ShikimoriID, episodeNumber)
```

Goroutine *lifetime* is bounded (the client's 2s `http.Client.Timeout` caps each one), so this
is not an unbounded-leak BLOCKER. But under a request burst against a *slow* library `/internal`
endpoint, in-flight goroutines accumulate to roughly `request_rate × 2s` with no upper bound —
each holding an idle TCP connection (the client sets no `MaxConnsPerHost`/`MaxIdleConnsPerHost`).
The gateway per-IP/per-user rate limits soften this, but they're the only backpressure, and they
sit a service hop away from this spawn site.

**Fix applied:** Added a `serveSignalSem chan struct{}` (cap `serveSignalConcurrency = 64`) to
`RawResolver`, initialized in `NewRawResolver`, plus a `fireSignal(fn func()) bool` helper that
acquires a slot and spawns the goroutine, or DROPS the signal (returns false, runs nothing) when
saturated — matching the SERVE-03 best-effort contract. Both the HIT (`RecordFetch`) and MISS
(`RecordDemand`) spawn sites now route through `fireSignal`, retaining `context.WithoutCancel`.
Added `raw_resolver_signal_test.go` covering spawn-when-free, drop-when-saturated, recovery after
release, and the nil-semaphore drop.

### WR-02: `episode` upper bound unvalidated — oversized value silently fails the DB write (int64 → Postgres int4 overflow)

**Status:** FIXED (commit `8faa0a8b`).

**File:** `services/library/internal/handler/autocache_internal.go:80-83`

**Issue:**
`decodeSignal` validated `Episode <= 0` but not an upper bound. Go decodes JSON `episode`
into a 64-bit `int`; the `autocache_demand.episode` and `library_episodes.episode_number`
columns are Postgres `INT` (int4, max 2147483647). A body like `{"mal_id":"x","episode":9999999999}`
passes validation, then `RecordDemand`/`BumpFetch` push an out-of-range value to Postgres,
which returns a `numeric value out of range` error. The handlers swallow it (best-effort,
return 200), so there is no crash or 500 — but it's an unvalidated input reaching the DB and a
silent no-op. The endpoint is Docker-network-only and the real caller derives `episode` from a
catalog-side `strconv.Atoi` + `> 0` check, so impact is low; still, the internal endpoint should
not trust unbounded input.

**Fix applied:** Added `const maxEpisode = 100000` and extended the `decodeSignal` guard to
`body.Episode <= 0 || body.Episode > maxEpisode`, returning a 400 ("mal_id and a sane positive
episode are required"). Extended `TestAutocacheInternalBadBody` with `overflow episode (int4)`
(9999999999) and `over maxEpisode` (100001) cases on both the fetch and demand endpoints.

## Info

### IN-01: Redundant `r.library != nil` guards inside `GetLibraryStream`

**Status:** addressed incidentally during the WR-01 fix.

**File:** `services/catalog/internal/service/raw_resolver.go:398, 412`

**Issue:** Both signal-spawn blocks re-checked `if r.library != nil`, but the function already
returns early at line 369 (`if r.library == nil { return ... }`). `r.library` is set once at
construction and never mutated, so the inner checks were dead defensive code. They were removed
when the spawn sites were rewired through `fireSignal`.

### IN-02: `ON CONFLICT DO UPDATE` does not refresh `reason` (forward-compat note for Phase 9)

**Status:** not actioned (no Phase 08 action required).

**File:** `services/library/internal/repo/demand.go:34-37`

**Issue:** The upsert updates only `requested_at`, not `reason`. Harmless in Phase 8 (only
`backfill` is ever written). But once Phase 9 writes `next_ep`, a re-demand of a row that was
first inserted as `backfill` (or vice-versa) will preserve the original `reason`. If the Planner
ever prioritizes by reason, this is a latent surprise.

**Fix:** No action needed for Phase 8. When Phase 9 lands `next_ep`, decide explicitly whether a
later demand should overwrite `reason` and update the `DoUpdates` assignment list accordingly.

### IN-03: `serve_total` undercounts during a library outage (metric tied to signal HTTP success)

**Status:** not actioned (no Phase 08 action required).

**File:** `services/library/internal/handler/autocache_internal.go:100, 135`

**Issue:** `IncServeTotal("hit"/"miss")` fires inside the library handler, so the counter only
increments when the signal HTTP round-trip reaches the library. During the exact failure window
where the library `/internal` endpoint is down/slow, resolutions still happen on the catalog side
but the serve counter records nothing — `library_autocache_serve_total` will undercount real
serves during library outages. Acceptable for a popularity/observability metric (and documented
as best-effort), but worth noting for the Phase 11 serve-hit-rate panel: the denominator is
"signals successfully delivered", not "resolutions attempted".

**Fix:** None required. If exact accounting is later wanted, emit a catalog-side
"resolution attempted" counter independent of the signal delivery.

---

_Reviewed: 2026-06-17T06:54:02Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Resolved: 2026-06-17 (gsd-code-fixer — CR-01/WR-01/WR-02 fixed)_
_Depth: deep_
