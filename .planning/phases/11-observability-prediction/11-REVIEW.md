---
phase: 11-observability-prediction
reviewed: 2026-06-17T00:00:00Z
depth: deep
files_reviewed: 6
files_reviewed_list:
  - services/scheduler/internal/jobs/autocache_prediction.go
  - libs/metrics/scheduler.go
  - services/scheduler/internal/config/config.go
  - services/scheduler/internal/service/job.go
  - services/scheduler/cmd/scheduler-api/main.go
  - infra/grafana/dashboards/library.json
findings:
  critical: 0
  warning: 2
  info: 4
  total: 6
status: resolved
resolution:
  resolved_at: 2026-06-17T00:00:00Z
  fixed:
    - WR-01
    - WR-02
    - IN-01
  accepted:
    - IN-02
    - IN-03
    - IN-04
  fix_commits:
    - "b3d51faa: WR-01 + WR-02 (library dashboard OBS-05 merge transform + OBS-02 divide-by-zero guard)"
    - "2559dc6a: IN-01 (DISTINCT a.id + skip empty shikimori_id, tests + doc)"
---

# Phase 11: Code Review Report

**Reviewed:** 2026-06-17
**Depth:** deep
**Files Reviewed:** 6
**Status:** resolved

> **Resolution (2026-06-17):** WR-01, WR-02, and IN-01 fixed and committed atomically
> (commits `b3d51faa`, `2559dc6a`). IN-02, IN-03, IN-04 reviewed and **accepted** as
> documented non-blockers (pre-existing patterns / optional polish — see each finding's
> Resolution note below). Scheduler builds + vets + tests clean; `library.json` valid
> JSON with panels 1..7 unchanged and panel id:13 now carrying merge/organize transforms.

## Summary

Phase 11 adds the OBS-05 daily storage-need prediction job (`autocache_prediction.go`),
a `library_autocache_predicted_bytes{component}` gauge, two scheduler config knobs
(`AUTOCACHE_AVG_RAW_EP_BYTES`, `AUTOCACHE_PREDICTION_CRON` + a `getEnvInt64` helper),
cron registration/DI wiring, and 6 new Grafana panels (ids 8-13).

Grounding: `go build ./...` and `go vet ./...` pass clean; `jq .` validates the
dashboard JSON; the prediction unit tests pass. The core SQL is correct against
the stated spec contract — the two count queries are well-parameterized (no
injection), `nextep` correctly drops the `a.status='ongoing'` clause while the
`ongoing` query is byte-for-byte the Logic A WHERE block, and both count
**distinct anime**. The int64×avgBytes "overflow" concern raised in scope is a
**non-issue**: the multiplication is done in float64 space (`float64(ongoing) *
float64(avgRawEpBytes)`), so no int64 product is ever formed. `getEnvInt64`
correctly falls back to the default on empty/invalid input (never 0).

No scope leak detected — this is the final v4.1 observability phase and the
changes are confined to the scheduler + the library dashboard. The findings below
are dashboard-UX correctness issues and a subtle SQL-divergence note; none are
blockers.

## Warnings

### WR-01: Panel 13 (OBS-05 table) has no merge/join transformation — three disjoint instant queries render as fragmented rows

**File:** `infra/grafana/dashboards/library.json` (panel `id: 13`, targets A/B/C)
**Issue:** The "Autocache storage-need prediction vs budget" table issues three
separate `instant` queries (`library_autocache_predicted_bytes` per-component,
`sum(library_autocache_predicted_bytes)`, `library_autocache_budget_bytes`) but
defines **no `transformations`** (`.transformations` is `null`). In a Grafana
table panel, multiple instant Prometheus queries without a `merge` transform are
rendered as separate, non-aligned frames — you get fragmented rows with empty
cells rather than the intended "predicted vs budget" comparison. The
`legendFormat` values (`{{component}}`, `total`, `budget`) are also ignored in
table format (legend formatting applies to time-series, not table columns), so
the columns surface as raw `Value #A / Value #B / Value #C` with `__name__`
labels. The stated design goal ("union predicted_bytes with budget_bytes in a
single query / sane join") is not actually achieved by the panel as written.
**Fix:** Add a `merge` transformation (and optionally `organize` to rename/hide
columns), e.g.:
```json
"transformations": [
  { "id": "merge", "options": {} },
  { "id": "organize", "options": {
      "renameByName": { "Value #A": "predicted", "Value #B": "total_predicted", "Value #C": "budget" },
      "excludeByName": { "Time": true, "job": true, "instance": true } } }
]
```
Alternatively collapse to a single expression that emits both series the table
can align on a shared label.

**Resolution (FIXED, commit `b3d51faa`):** Added a `merge` + `organize` transform
pair to panel id:13. `merge` aligns the three instant frames; `organize` renames
`Value #A/B/C` → `Predicted / Total predicted / Budget`, renames `component` →
`Component`, hides `Time/job/instance/__name__`, and pins column order via
`indexByName`. JSON re-validated with `jq .`; panels 1..7 (and 8..12) unchanged.

### WR-02: Panel 10 hit-rate divides by zero when there is no serve traffic — renders as the worst (red) state instead of "no data"

**File:** `infra/grafana/dashboards/library.json` (panel `id: 10`, target A)
**Issue:** The expression
`sum(rate(library_autocache_serve_total{result="hit"}[1h])) / sum(rate(library_autocache_serve_total[1h]))`
has an unguarded denominator. With zero serve traffic in the window (a brand-new
deploy, an idle hour, or before the ae serve-path is exercised) both sums are 0,
so Prometheus returns an **empty result** (0/0 → no sample). The panel's
`reduceOptions.calcs: ["lastNotNull"]` then yields no value, and the threshold
config has `value: null → color: red` as its base step — so an idle pool with a
perfectly healthy (just unused) cache paints **red**, which reads as an alarm /
broken cache. This is the exact divide-by-zero-when-no-traffic case called out in
the review scope.
**Fix:** Guard the denominator and surface a stable 0 (or explicit no-data) when
idle, e.g.:
```promql
sum(rate(library_autocache_serve_total{result="hit"}[1h]))
  / clamp_min(sum(rate(library_autocache_serve_total[1h])), 1)
```
or `(... ) / (sum(rate(library_autocache_serve_total[1h])) > 0)` paired with a
`No data → gray` mapping so an idle cache is visually distinct from a failing one.

**Resolution (FIXED, commit `b3d51faa`):** Wrapped the denominator in
`clamp_min(..., 1)`. An idle pool now yields a stable `0` (hits/1) instead of an
empty result, so `lastNotNull` reports 0 and the panel reads green-base/low rather
than the false null→red alarm. With real traffic the +1 floor is negligible
(rate denominators are ≫1).

## Info

### IN-01: Prediction counts `DISTINCT a.shikimori_id`; Logic A projects `a.id` — diverges when shikimori_id is empty or shared

**File:** `services/scheduler/internal/jobs/autocache_prediction.go:69,82`
**Issue:** Both prediction queries count `SELECT DISTINCT a.shikimori_id`. The
`animes.shikimori_id` column is `gorm:"size:50;index"` (NOT `not null`, NOT
`uniqueIndex` — see `services/catalog/internal/domain/anime.go:36`), so it can be
empty (`''`) or, in principle, duplicated across rows. The "verbatim Logic A
join" comment (lines 61-64) is accurate for the WHERE block, but Logic A's
projection is `a.id`/`a.shikimori_id` per ongoing **row** and it explicitly
*skips* empty `shikimori_id` downstream (`autocache_logic_a.go:121`). The
prediction count does not skip empties: multiple watched anime with empty
`shikimori_id` collapse into a single distinct bucket, undercounting the
heuristic; conversely it is a `count(*)` over distinct shikimori_id, so it cannot
overcount. Net effect: the predicted-bytes estimate can be slightly **low** in a
catalog that has JP-watched anime with no Shikimori mapping. For a coarse v1
heuristic this is tolerable, but the "EXACT Logic A join" framing in the package
doc overstates the equivalence.
**Fix:** Count `DISTINCT a.id` (the guaranteed-non-null PK) to match Logic A's
per-anime granularity, or add `AND a.shikimori_id <> ''` to mirror the Logic A
skip and make the doc claim true. Document whichever is chosen.

**Resolution (FIXED, commit `2559dc6a`):** Did **both** — switched both queries to
`COUNT(DISTINCT a.id)` AND added `AND a.shikimori_id <> ''`. The count now equals
the set of distinct anime Logic A actually fans out demand for (per-`a.id`
granularity, empty shikimori_id excluded — no longer collapsed into one bucket).
Updated the package doc that overstated "EXACT Logic A join", and added two tests:
`TestAutocachePrediction_ExcludesEmptyShikimoriID` (empties excluded, not collapsed)
and `TestAutocachePrediction_CountsDistinctByID` (two anime sharing a shikimori_id
count as 2). Scheduler build/vet/test clean.

### IN-02: `getEnvInt64` (and `getEnvInt`) silently swallow malformed env values

**File:** `services/scheduler/internal/config/config.go:186-193`
**Issue:** `getEnvInt64` returns the default on a parse error with no log line, so
a typo like `AUTOCACHE_AVG_RAW_EP_BYTES=1.2GiB` or `=1_288_490_188` silently runs
with the 1.2 GiB default instead of the operator's intended value — a
hard-to-notice misconfiguration that skews every predicted-bytes reading. The
edge-case behavior asked about in scope (empty/invalid → default, not 0) is
**correct**; this is only the observability gap. Pre-existing pattern shared with
`getEnvInt`, so not introduced by this phase, but the new int64 knob inherits it.
**Fix:** Optionally log a warning on parse failure (`fmt.Fprintf(os.Stderr, ...)`
or via the logger once available) so a bad value is visible at boot rather than
silently defaulted.

**Resolution (ACCEPTED):** Pre-existing project-wide pattern shared with
`getEnvInt`, not introduced by Phase 11, and the documented edge-case behavior
(empty/invalid → default, never 0) is correct. Optional observability polish only;
deferred to avoid a config-helper refactor outside this phase's scope.

### IN-03: Prediction gauge is never reset between runs — a downward count change is reflected, but a query-failure mid-run leaves a stale value

**File:** `services/scheduler/internal/jobs/autocache_prediction.go:103-114`
**Issue:** If the `ongoing` count succeeds but the `nextep` count fails, `Run`
returns early at line 109-110 **after** nothing was set (both `.Set` calls are at
113-114, after both queries) — so this specific ordering is actually safe (no
partial set). However, on a *fully failed* run the gauge retains its last
successful value indefinitely (Prometheus gauge semantics), so a persistently
broken job shows stale-but-plausible predictions on the OBS-05 table with no
staleness indicator. The `scheduler_job_last_success_timestamp{job="autocache_prediction"}`
metric does expose this, but the OBS-05 table does not surface it.
**Fix:** Optional — add a staleness annotation/threshold on panel 13 keyed off
`time() - scheduler_job_last_success_timestamp{job="autocache_prediction"}`, or
accept as documented (the gauge is a daily heuristic and last-success is tracked).

**Resolution (ACCEPTED):** The partial-set ordering is already safe (both `.Set`
calls run only after both queries succeed). Staleness is already observable via
`scheduler_job_last_success_timestamp{job="autocache_prediction"}`; surfacing it on
panel 13 is optional polish for a daily heuristic gauge. Accepted as documented.

### IN-04: Cron parse failure on the prediction schedule aborts registration of *all* later jobs (none after it, but ordering is load-bearing)

**File:** `services/scheduler/internal/service/job.go:262-283`
**Issue:** `Start` returns on the first `AddFunc` error. The prediction job is
registered last in the chain, so a malformed `AUTOCACHE_PREDICTION_CRON` only
fails itself today — but the pattern means any future job appended after it would
be silently un-registered on a bad cron here. Not a defect now; noting the
fragility of the sequential `if err != nil { return err }` chain. The nil-guard
(`if s.autocachePredictionJob != nil`) is dead defensively (main.go:137
constructs it unconditionally) but harmless and consistent with the Logic A
pattern.
**Fix:** None required. If desired, validate all crons up front (or accumulate
errors) so one bad expression doesn't short-circuit registration.

**Resolution (ACCEPTED):** Not a defect today (prediction job is registered last;
a bad cron only fails itself). Noted as forward fragility; no change required per
the finding's own "Fix: None required". Accepted as documented.

---

_Reviewed: 2026-06-17_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
_Resolved: 2026-06-17 — WR-01/WR-02/IN-01 fixed (b3d51faa, 2559dc6a); IN-02/03/04 accepted_
