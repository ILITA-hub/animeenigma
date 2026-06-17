---
phase: 07-pool-foundation-config-migration
reviewed: 2026-06-17T05:58:24Z
depth: deep
files_reviewed: 12
files_reviewed_list:
  - services/library/internal/autocache/layout.go
  - services/library/internal/autocache/migrator.go
  - services/library/migrations/005_autocache_pool.sql
  - services/library/migrations/006_autocache_config.sql
  - services/library/migrations/migrations.go
  - services/library/internal/domain/episode.go
  - services/library/internal/domain/autocache_config.go
  - services/library/internal/repo/episode.go
  - services/library/internal/repo/autocache_config.go
  - services/library/internal/handler/autocache_config.go
  - services/library/cmd/library-api/main.go
  - services/library/internal/service/encoder_worker.go
findings:
  blocker: 1
  warning: 4
  info: 3
  total: 8
status: resolved
fixed:
  - CR-01
  - WR-01
  - WR-02
  - WR-03
  - WR-04
deferred:
  - IN-01
  - IN-02
  - IN-03
fixed_at: 2026-06-17T08:10:00Z
---

# Phase 7: Code Review Report

**Reviewed:** 2026-06-17T05:58:24Z
**Depth:** deep
**Files Reviewed:** 12
**Status:** resolved (CR-01 + WR-01..04 fixed 2026-06-17; IN-01..03 deferred to backlog)

## Resolution (2026-06-17)

All correctness + hardening findings fixed and committed atomically in the
Phase-7 review-fix pass; `cd services/library && go build ./... && go vet ./...
&& go test ./... -count=1` all pass.

| Finding | Status | Fix |
|---------|--------|-----|
| CR-01 | fixed | `Episode.DownloadedAt` → `*time.Time` (mirrors `LastFetchAt`); omitted Create inserts NULL → §3.3 backfill heals it. Regression test round-trips the field. |
| WR-01 | fixed | Backfill scoped `AND source = 'admin'` (+ dropped redundant `source/track` SETs); a re-run on boot can never relabel a future autocache row. |
| WR-02 | fixed | PATCH validators gained documented upper bounds (budget ≤100 TiB, `*_days` ≤3650, `min_seeders` ≤10000, `sweep_interval_min` ≤1 week). |
| WR-03 | fixed | `quality_cap` restricted to the discrete ladder {480,720,1080,2160}. |
| WR-04 | fixed | Repo `Patch` checks `RowsAffected==0` and returns Internal ("singleton row missing") instead of silently writing nowhere. |
| IN-01 | deferred | Unknown-key PATCH decode — backlog. |
| IN-02 | deferred | `RawPrefix` `malID` validation — backlog (no live injection path). |
| IN-03 | deferred | Move-before-Create ordering in Link handler — pre-dates Phase 7, backlog note only. |

## Summary

Reviewed the Phase 7 (v4.1 autocache pool foundation) surface in `services/library`:
the `RawPrefix` layout helper, the one-time admin-content migrator and its boot
wiring, migrations 005/006, the two new domain models, the episode + config
repos, and the GET/PATCH config handler. `go build ./...` and `go vet ./...`
are clean. Scope discipline is good — there is **no leakage** into Phases 8-11
(no eviction, trigger, download, or serve logic; `last_fetch_at`/`fetch_count`
are declared-but-unwritten reservations, correctly documented as such).

The migrator's copy-before-repoint / restart-safe / non-fatal design is sound and
well-tested, and the boot sequencing (apply 005/006 → migrate → serve, `Warnw`
not `Fatalw`) matches the spec.

The one **BLOCKER** is a GORM↔SQL fidelity defect: `downloaded_at` is a nullable
column but is modeled as a non-pointer `time.Time` with no default, so every new
admin episode created after Phase 7 persists a year-0001 timestamp into the
budget/freshness ledger that the Phase 8+ evictor will read — silently corrupting
the very ledger this phase exists to establish. The warnings cover a re-run
backfill hazard, two missing-upper-bound validation gaps, and a singleton Patch
that silently no-ops when the seed row is absent.

## Structural Findings (fallow)

No `<structural_findings>` block was provided for this review. All findings below
are narrative (direct adversarial code review).

## Narrative Findings (AI reviewer)

## Blocker Issues

### CR-01: Nullable `downloaded_at` modeled as non-pointer `time.Time` poisons the ledger with year-0001 timestamps

> **RESOLVED 2026-06-17** — `DownloadedAt` is now `*time.Time` (commit 885d6a9a). Chose the pointer/NULL route (over set-at-every-write-site) so the §3.3 backfill stays authoritative; regression test added.


**File:** `services/library/internal/domain/episode.go:51` (with `services/library/migrations/005_autocache_pool.sql:42` and `services/library/internal/service/encoder_worker.go:317-324`)

**Issue:**
Migration 005 adds the ledger column as **nullable** and the comment is explicit
about it:

```sql
-- downloaded_at stays nullable so the ADD on a populated table succeeds
ALTER TABLE library_episodes ADD COLUMN IF NOT EXISTS downloaded_at TIMESTAMPTZ;
```

But the domain model maps it to a **non-pointer** with **no `default` GORM tag**:

```go
DownloadedAt  time.Time     `gorm:"column:downloaded_at" json:"downloaded_at"`
```

Compare `LastFetchAt *time.Time` two lines down — the other nullable timestamp is
correctly a pointer. `DownloadedAt` is the odd one out.

The encoder worker (`encoder_worker.go:317-324`) and the Link handler
(`handler/jobs.go:429-434`) both `Create` an `Episode` **without setting
`DownloadedAt`**. Because the field is a non-pointer with no `default` tag, GORM
does **not** skip it on insert — it writes the Go zero value
`0001-01-01 00:00:00+00:00` into the column (NOT `NULL`, NOT `created_at`).

Consequences, all landing on the Phase 8+ budget/eviction ledger that this phase
exists to seed:
1. **Backfill cannot heal these rows.** The one-time backfill is guarded by
   `WHERE downloaded_at IS NULL` (005 line 51). Year-0001 rows are non-NULL, so
   they are never corrected to `created_at`.
2. **Freshness math breaks.** A `downloaded_at` of year 0001 reads as infinitely
   stale; the Phase-10 evictor will treat brand-new admin episodes as the oldest
   content and evict them first.
3. The corruption is **silent** — `go build`/`go vet`/the existing tests all pass
   because no current test inserts a row through GORM and reads `downloaded_at`
   back.

**Fix:** Set the field explicitly at every write site (cleanest — keeps the column
genuinely populated, matching the backfill's `downloaded_at = created_at`
semantics). In `encoder_worker.go` and `handler/jobs.go`, add to the `Episode`
literal:

```go
ep := &domain.Episode{
    ShikimoriID:   job.ShikimoriID,
    EpisodeNumber: episode,
    JobID:         &jobIDCopy,
    MinioPath:     prefix,
    DurationSec:   &duration,
    SizeBytes:     &size,
    DownloadedAt:  time.Now().UTC(), // ledger anchor — never leave zero
}
```

If a genuinely-NULL "unknown download time" must be representable, instead change
the model to a pointer (`DownloadedAt *time.Time`) so an omitted value inserts
`NULL` (which the backfill *can* heal) rather than year 0001 — but then audit the
future evictor for nil-handling. Either way, the current state (non-pointer + no
default + never set) is the one combination that corrupts the ledger.

## Warnings

### WR-01: Backfill `WHERE downloaded_at IS NULL` will clobber future autocache rows' `source` on any re-run

> **RESOLVED 2026-06-17** — backfill scoped `AND source = 'admin'`, redundant `source/track` SETs dropped (commit e76dd922).


**File:** `services/library/migrations/005_autocache_pool.sql:49-51`

**Issue:** The migration is embedded and re-`Exec`'d on **every** service boot
(`main.go:135`). The backfill is:

```sql
UPDATE library_episodes
   SET source = 'admin', track = 'raw', downloaded_at = created_at
 WHERE downloaded_at IS NULL;
```

The guard assumes "`downloaded_at IS NULL`" ⇒ "pre-Phase-7 admin row not yet
backfilled". That holds today. But once Phase 8 begins inserting **autocache**
rows, any such row that is inserted with `downloaded_at = NULL` (e.g. a partial
insert, or a code path that forgets to set it — exactly the failure mode of
CR-01) will be force-rewritten to `source='admin', track='raw'` on the **next
boot**, mislabeling autocache content as admin content. The discriminator column
(D6) is the single source of truth for admin-vs-autocache, so this silently
corrupts the pool's accounting identity.

**Fix:** Make the one-time backfill genuinely one-time and source-scoped, so a
re-run can never touch a non-admin row:

```sql
UPDATE library_episodes
   SET downloaded_at = created_at
 WHERE downloaded_at IS NULL
   AND source = 'admin';
```

(Drop the redundant `source='admin', track='raw'` SETs — the `ADD COLUMN ...
NOT NULL DEFAULT` already populated every pre-existing row with those exact
values, so the only real work here is anchoring `downloaded_at`.) This also
composes correctly with the CR-01 fix.

### WR-02: PATCH validation has no upper bounds — `quality_cap`, `budget_bytes`, `sweep_interval_min` accept absurd values

> **RESOLVED 2026-06-17** — documented upper bounds added to every floor-only validator (commit b5509712). quality_cap handled by WR-03's ladder.


**File:** `services/library/internal/handler/autocache_config.go:113-132`

**Issue:** Every numeric validator checks only the lower bound. `quality_cap`
accepts `999999` (no real quality), `sweep_interval_min` accepts `2_000_000_000`
(the evictor sweep effectively never runs), and `budget_bytes` accepts any
positive `int64` up to ~9.2 EB. The handler doc claims it "range-validates each
provided field" but only floor-validates. Since these values directly drive the
future downloader/evictor budget ledger, an out-of-range value is a foot-gun an
admin can set with no redeploy and no warning.

**Fix:** Add sane upper bounds matching the domain. For example:

```go
if body.QualityCap != nil {
    if *body.QualityCap <= 0 || *body.QualityCap > 2160 {
        httputil.BadRequest(w, "quality_cap must be in 1..2160")
        return
    }
    fields["quality_cap"] = *body.QualityCap
}
if body.SweepIntervalMin != nil {
    if *body.SweepIntervalMin < 1 || *body.SweepIntervalMin > 1440 {
        httputil.BadRequest(w, "sweep_interval_min must be in 1..1440")
        return
    }
    fields["sweep_interval_min"] = *body.SweepIntervalMin
}
// budget_bytes: enforce a documented ceiling (e.g. 10 TiB) rather than only > 0
```

### WR-03: `quality_cap` accepts arbitrary integers, not the discrete quality ladder

> **RESOLVED 2026-06-17** — `quality_cap` validated against {480,720,1080,2160} (commit 264e3ade).


**File:** `services/library/internal/handler/autocache_config.go:113-119`

**Issue:** `quality_cap` is conceptually a member of a discrete set (480 / 720 /
1080 / 2160). The validator accepts any positive int, so `quality_cap = 137` is
persisted and later compared against real stream heights by the Phase-8
downloader — a comparison that will behave unpredictably (no torrent is exactly
137p; the cap silently filters everything or nothing depending on the comparison
operator the downloader chooses). This is type-confusion-by-omission: the field
*looks* validated but admits nonsense.

**Fix:** Validate against the allowed ladder:

```go
if body.QualityCap != nil {
    switch *body.QualityCap {
    case 480, 720, 1080, 2160:
        fields["quality_cap"] = *body.QualityCap
    default:
        httputil.BadRequest(w, "quality_cap must be one of 480, 720, 1080, 2160")
        return
    }
}
```

### WR-04: Singleton `Patch` silently no-ops (HTTP 200) when the seed row is missing instead of surfacing the broken-migration state

> **RESOLVED 2026-06-17** — `Patch` now checks `RowsAffected==0` → Internal error (commit fa19fc4a). SQLite unit test covers both present/missing-row paths.


**File:** `services/library/internal/repo/autocache_config.go:57-64`

**Issue:** `Patch` issues `UPDATE ... WHERE id = 1` and ignores `RowsAffected`. If
the seed row (`INSERT ... id=1`) is somehow absent — e.g. someone truncated the
table, or migration 006's seed failed but the table create succeeded — the UPDATE
matches **zero rows** and returns no error. `Patch` then calls `r.Get(ctx)`, which
*does* error (NotFound → wrapped Internal), so the caller does get a 500 in that
exact path. **However**, the asymmetry is fragile: `Get`'s "missing row =
Internal error" contract (documented at `autocache_config.go:26-29`) is the only
thing catching it, and a future refactor of `Get` (e.g. returning a zero-value
default) would turn `Patch` into a silent write-to-nowhere that still returns the
admin a 200 with stale data. The write side should assert its own invariant.

**Fix:** Check `RowsAffected` in `Patch` so the singleton invariant is enforced at
the write, independent of `Get`'s behavior:

```go
res := r.db.WithContext(ctx).
    Model(&domain.AutocacheConfig{}).
    Where("id = ?", 1).
    Updates(updates)
if res.Error != nil {
    return nil, liberrors.Wrap(res.Error, liberrors.CodeInternal, "update autocache config")
}
if res.RowsAffected == 0 {
    return nil, liberrors.Internal("autocache config singleton row missing (broken migration 006)")
}
return r.Get(ctx)
```

## Info

### IN-01: PATCH decoder silently ignores unknown JSON keys

> **DEFERRED 2026-06-17** — backlog (low severity, no data-correctness impact).


**File:** `services/library/internal/handler/autocache_config.go:67-71` (via `libs/httputil/response.go:133-138`)

**Issue:** `httputil.Bind` → chi `render.DecodeJSON` does not call
`DisallowUnknownFields`, so a PATCH body with a typo'd key (e.g.
`"budget_byte": 5`) is accepted with 200 and **silently applies nothing** for
that field. An admin can believe they changed a tunable when they did not.
Low severity (not a correctness bug in the persisted data), but it degrades the
"no redeploy, edit live" UX the feature is built around.

**Fix:** If a stricter decode is desired here without changing the shared helper,
decode locally with `dec := json.NewDecoder(r.Body); dec.DisallowUnknownFields()`
and 400 on unknown keys. Otherwise document that unknown keys are ignored.

### IN-02: `RawPrefix` does not validate/escape `malID`

> **DEFERRED 2026-06-17** — backlog (no live injection path; callers pass server-controlled numeric IDs).


**File:** `services/library/internal/autocache/layout.go:26-28`

**Issue:** `RawPrefix` interpolates `malID` straight into the MinIO key via
`fmt.Sprintf("aeProvider/%s/RAW/%d/", malID, episode)`. Today both call sites pass
`job.ShikimoriID` / `body.ShikimoriID`, which are server-controlled and (for valid
Shikimori IDs) numeric, so there is no live injection path. But the helper is the
"single source of truth for the object layout" and offers no defense if a future
caller passes an unsanitized ID containing `/` or `..` — that would let a key like
`aeProvider/../other/RAW/...` reshape the prefix. Not exploitable today; worth a
guard since this is the chokepoint by design.

**Fix:** Reject non-numeric / path-bearing IDs at the helper, or document the
caller contract that `malID` must be a validated numeric string. e.g.:

```go
// callers MUST pass a validated numeric shikimori_id; reject path metacharacters
if strings.ContainsAny(malID, "/\\.") || malID == "" {
    // return error or panic-with-context per house style
}
```

### IN-03: Pre-existing Move-before-Create ordering in the Link handler can delete sources then fail on duplicate (NOT introduced by Phase 7)

> **DEFERRED 2026-06-17** — explicitly left as a backlog note; pre-dates Phase 7, out of this phase's change scope (per fix directive).


**File:** `services/library/internal/handler/jobs.go:418-436`

**Issue:** The Link handler `Move`s objects to `dstPrefix` (line 420, which
deletes sources on success) **before** `episodeStore.Create` (line 435). If
`Create` fails with AlreadyExists (re-linking an episode that already has a row),
the source `pending/` objects are already gone and the destination has been
overwritten, while the handler returns an error — leaving the operation
half-applied. **This ordering predates Phase 7** (verified against `ac0805bd^`);
Phase 7 only substituted the `dstPrefix` value (`autocache.RawPrefix(...)` instead
of the inline `shikimori/ep/` string), which is a correct, behavior-preserving
swap. Flagged for awareness only — out of Phase 7's change scope, so no fix is
required for this phase, but it is a latent data-consistency issue worth a backlog
note.

---

_Reviewed: 2026-06-17T05:58:24Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
