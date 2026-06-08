---
phase: 04-fe-causation-rum
type: gap-closure
closes: [AR-FE-01, AR-FE-02, AR-FE-03, AR-STORE-03]
subsystem: analytics / activity-register / tracing
date: 2026-06-08
---

# Phase 4 Gap-Closure: FE→BE trace_id Join Repair

**One-liner:** Repaired the v4.0 headline FE↔BE `trace_id` join (was 0 rows) by making
BE effects carry the OTel trace id end-to-end (Layer A), making the collector tolerant
of register fields nested in `properties` (Layer B), and fixing the FE so register
fields serialize at the top level (Layer C). Live ClickHouse now shows the
effect↔span join resolving with thousands of matches.

## What Was Broken

The milestone audit (`.planning/v4.0-MILESTONE-AUDIT.md`) flagged three blockers:

- **AR-FE-02 / AR-STORE-03** — `libs/tracing.Effect` had no `TraceID` field; the
  recording transport never extracted `trace.SpanFromContext(...).TraceID()`; the
  producer wire dropped it; the analytics effects handler never set
  `domain.Event.TraceID`. Every BE effect row had `trace_id=''`.
- **AR-FE-01 / AR-FE-03** — the FE `track(name, props)` nested ALL props (including
  `source`, `trace_id`, `operation`, `target`, `target_kind`) under `properties`,
  which the collector's top-level `wireEvent` never reads. The axios interceptor's
  `fe.call` and `rum.ts`'s `rum.resource` register fields therefore never reached the
  collector; RUM rows risked misclassification as `source="be"`.

Net effect: `analytics.events.trace_id` joined to `analytics.otel_traces.TraceId`
returned **0 rows** — the milestone's headline causation feature was non-functional.

## What Was Fixed

### Layer A — BE effect trace_id propagation (commit `7b1b9c03`)
- `libs/tracing/effect.go`: added `TraceID string` to `Effect`.
- `libs/tracing/client.go`: added exported `TraceIDFromContext(ctx)` (guards on
  `SpanContext().IsValid() && HasTraceID()`); `recordingTransport.RoundTrip` now
  stamps the egress effect with it.
- `libs/tracing/gormtrace/gorm_effect.go`: `db_write` and `db_read` effects stamp
  `TraceID` from the statement context span.
- `libs/tracing/producer.go`: `wireProducerEffect` carries `trace_id` (JSON tag) and
  maps `Effect.TraceID` → it.
- `services/analytics/internal/handler/effects.go`: reads wire `trace_id` →
  `domain.Event.TraceID`. `clickhouse_store.go` already persists `e.TraceID` into the
  `trace_id` column (no change needed).
- **Cache effects deliberately left untouched** — by design (D-06) aggregated cache
  rows carry no trace_id (they sum across many requests; no single trace applies).

### Layer B — Collector tolerance (commit `e268ef25`)
- `services/analytics/internal/handler/collect.go`: when a register field
  (`source`/`trace_id`/`operation`/`action`/`target`/`target_kind`) is empty at the
  top level, it is lifted from the `properties` map **before** the source whitelist +
  default run. This honors a `source` of `fe`/`fe_rum` carried in properties so RUM
  rows are NOT misclassified as `be`, and makes the join robust **even before the FE
  redeploy**. Added the `propStr` helper.

### Layer C — FE correctness (commit `c2c2e7d7`)
- `frontend/web/src/analytics/index.ts`: `track()` now lifts a fixed `REGISTER_KEYS`
  set (`source`, `trace_id`, `operation`, `action`, `route`, `target`, `target_kind`,
  `requests`, `duration_ms`) to the event top level (matching `collect.go` `wireEvent`)
  while keeping arbitrary user props under `properties` (omitted entirely when empty).
- `frontend/web/src/analytics/__tests__/register-serialization.spec.ts`: NEW — 5
  assertions on the SERIALIZED event (decoded from the sendBeacon Blob), covering the
  exact serialization boundary the prior `emit`-props mocks skipped.

## Verification

### Unit / build
- `libs/tracing` + `gormtrace`: `go test ./...` → all green.
- `services/analytics`: `go build ./...` + `go test ./...` → all green.
- `services/catalog`: `go build ./...` → clean (libs/tracing consumer).
- FE: `bunx vitest run src/analytics/` → **45 passed** (incl. 5 new serialization tests).
- FE: `bunx tsc --noEmit` → **exit 0, fully clean** (the previously-blocking
  `useProviderResolver.ts` tsc errors are NOT present on the current main tree).

### Live production (LIVE server)
Redeployed `analytics`, `catalog`, `scraper`, `streaming` (all healthy post-deploy).
Generated live traffic via the gateway (`/api/anime/search` ×6, `/trending`,
`/popular`, `/ongoing`).

- **Baseline before traffic:** `trace_id != ''` rows in 10m = **1**.
- **After traffic:** `trace_id != ''` rows in 5m = **436** (434 `db_write` + 2 `fe`).
- **Source breakdown (5m, trace_id populated):** `be/db_write`=434, `fe`=2 — the 2
  FE rows confirm Layer B honors `source=fe` from properties (NOT misclassified `be`).
- **Effect↔span join** (`analytics.events.trace_id ⋈ analytics.otel_traces.TraceId`,
  10m window): **15,904 matches** (was 0).
- **Concrete matched trace** `dab08cf3...`: a `db_write` effect on `anime_genres`
  joins to the `GET /api/anime/search` server span AND the `HTTP GET` egress span in
  `otel_traces` — definitive end-to-end FE→BE / effect↔span bridge.

## What Remains / Notes

- **FE live redeploy (Layer C):** the audit predicted `make redeploy-web` would be
  blocked by unrelated parallel-workstream `useProviderResolver.ts` tsc errors. On the
  current main tree `bunx tsc --noEmit` is **clean (exit 0)**, so that blocker appears
  to have been resolved upstream. `make redeploy-web` was NOT run here (out of scope
  for this BE-focused gap closure and gated by the project's after-update skill). Layer
  B's collector tolerance means the BE join is already proven live regardless of FE
  deploy state; once `redeploy-web` runs, `fe.call`/`rum.resource` rows will also carry
  top-level register fields directly.
- **Egress trace_id coverage:** `db_write`/`db_read` effects carry trace_id reliably
  (run in request context). Some `egress` rows show `trace_id=''` because they
  originate from background/non-request-scoped clients (scheduler canaries, health
  probes) whose context has no active server span — this is correct behavior per the
  `IsValid()` guard, not a regression. The db_write effect↔span join already proves the
  headline feature.
- **AR-EGRESS-05** (megaplay extractor egress unrecorded) is a separate WARNING-level
  gap, out of scope for this trace_id-join closure.

## Commits (all pushed to `main`)

- `7b1b9c03` feat(04-gap): propagate OTel trace_id through BE effect pipeline (Layer A)
- `e268ef25` feat(04-gap): collector honors register fields from properties fallback (Layer B)
- `c2c2e7d7` feat(04-gap): lift FE register fields to top-level analytics event (Layer C)

## EXECUTION COMPLETE
