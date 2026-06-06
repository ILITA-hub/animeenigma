# Phase 4: FE Causation + RUM - Research

**Researched:** 2026-06-06
**Domain:** Frontend observability — distributed-trace propagation (browser→backend), axios interceptor enrichment, click→effect causation, `PerformanceObserver` cross-origin RUM
**Confidence:** HIGH (the FE seams, the analytics wire contract, the ClickHouse schema, and the gateway trace propagation were all read directly in the codebase; the one external fact — cross-origin resource-timing zeroing — is verified against the W3C Resource Timing spec + MDN)

## Summary

This phase is **instrumentation-only** and, unusually, lands on a codebase where ~70% of the scaffolding already exists. The frontend already has a complete `frontend/web/src/analytics/` module (transport via `sendBeacon`, click autocapture, heartbeat, identity, session), and — critically — it already mints a W3C `traceparent` per axios call (`traceparent.ts`) and already back-fills that `trace_id` onto recent click events via a best-effort time-window association (`traceContext.ts` + the request interceptor in `api/client.ts`). The ClickHouse `events` table already carries every dimension this phase needs: `trace_id`, `source` (default `'be'`), `accuracy` (default `'exact'`), `origin`, `operation`, `effect_kind`, `target`, `target_kind`. The gateway already continues the FE's `traceparent` into a root span and propagates it downstream (commit `dd74c301`), so an FE-minted trace_id is genuinely shared with the BE effect rows recorded in Phases 2/3.

What is therefore **already true today**: AR-FE-01's trace_id and AR-FE-02's click→trace association are largely wired. What is **missing** is: (1) the FE analytics wire envelope/collect handler do not yet carry `route` + optional semantic `action`, nor do they let an FE row set `source`/`origin`/`operation`/`effect_kind`/`target` — so an FE call cannot yet be written as a register row distinct from a clickstream row; (2) there is **no** `PerformanceObserver('resource')` beacon emitting `source=fe_rum, accuracy=approx` rows (the only existing `PerformanceObserver` lives in `diagnostics.ts` for the bug-report tool and never reaches analytics); (3) nothing structurally guarantees RUM bytes can never be summed with authoritative BE bytes.

**Primary recommendation:** Treat this as a *completion + hardening* phase, not a greenfield build. Extend the existing analytics wire envelope to carry `route` + `action` + a `source` discriminator; add the FE-row mapping to `collect.go` so FE rows land with `source='fe'`/`source='fe_rum'`; add one new `PerformanceObserver('resource')` beacon module that emits per-(3rd-party-host) approximate rows flagged `source=fe_rum, accuracy=approx`; and prove the non-contamination invariant with a query/test that every byte aggregation filters `source='be'`. Reuse the existing `Transport` (sendBeacon batching) and the existing trace_id plumbing — do **not** introduce a new beacon path or a new tracing library.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Mint/propagate `trace_id` for an action | Browser / Client | API / Gateway | FE mints the W3C `traceparent` per axios call (`traceparent.ts`); the gateway *continues* it into a root span and propagates downstream (`dd74c301`). Trace ROOT is the browser. |
| Stamp route + semantic action on an FE call | Browser / Client | — | Only the SPA router knows the current route + the semantic action label; these are client-side facts. |
| Click→trace association | Browser / Client | — | The click and the API call it triggers both happen in the browser; association is a client-side time-window join (`traceContext.ts`). |
| Browser→3rd-party RUM resource timings | Browser / Client | — | `PerformanceObserver('resource')` is a browser-only API. The 3rd-party fetches (CDN segments, posters) never touch our BE, so only the browser can observe them. |
| Persisting FE rows into the register | API / Backend (analytics) | Database / ClickHouse | `POST /api/analytics/collect` → analytics service → `events` table. FE rows must be tagged so byte aggregations exclude them. |
| Enforcing "RUM bytes never summed with BE bytes" | Database / ClickHouse (query discipline) | API / Backend (the `source` column) | The `source` column is the structural discriminator; the *invariant* is enforced at query time (`WHERE source='be'`) and proven by a dashboard/test. |

## Standard Stack

This phase adds **zero new runtime dependencies**. Everything is already present in the repo.

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `axios` | as-vendored in `frontend/web` | The single API client (`api/client.ts`) where the request interceptor already mints `traceparent` | Already the project's only HTTP client; the interceptor seam already exists [VERIFIED: codebase `frontend/web/src/api/client.ts:146-188`] |
| `PerformanceObserver` (`type: 'resource'`) | Web platform API (no package) | Observe browser→3rd-party resource timings for RUM | Standard browser RUM primitive; already used (for a different purpose) in `diagnostics.ts` [VERIFIED: codebase `frontend/web/src/utils/diagnostics.ts:140-158`] |
| `navigator.sendBeacon` (existing `Transport`) | Web platform API (no package) | Ship RUM/click batches without blocking unload | Already the analytics transport (`transport.ts`); reuse it [VERIFIED: codebase `frontend/web/src/analytics/transport.ts`] |
| `crypto.getRandomValues` (existing `traceparent.ts`) | Web platform API (no package) | Mint the 128-bit trace_id / 64-bit span_id | Already implemented; W3C-format `00-{traceId}-{spanId}-01` [VERIFIED: codebase `frontend/web/src/analytics/traceparent.ts`] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `vue-router` `useRoute()` / `router.currentRoute` | as-vendored | Source the current route name/path for AR-FE-01's `route` stamp | When the interceptor needs the route; prefer `router.currentRoute.value` read at request time (interceptors are outside component setup) |
| `vitest` + `@vue/test-utils` (jsdom) | as-vendored | Unit tests for the interceptor, traceContext, and the RUM observer | All existing analytics tests use vitest/jsdom (`src/analytics/__tests__/`) [VERIFIED: codebase] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| FE-minted `traceparent` (current design) | Read a `traceparent` returned by the gateway in a response header | The current design (FE is the trace ROOT) is correct for causation: the click *causes* the call, so the trace must originate in the browser. A gateway-returned id would arrive *after* the request and couldn't tag the request that produced it. **Keep the current FE-mint design.** [ASSUMED — design rationale, see Open Questions Q1] |
| `PerformanceObserver` | `performance.getEntriesByType('resource')` polling | The observer (`buffered: true`) is push-based, lower-overhead, and won't miss entries evicted from the (default 250-entry) resource buffer. Use the observer. [CITED: w3.org/TR/resource-timing] |
| Reusing `Transport`/`sendBeacon` | A dedicated OTLP/web-vitals SDK | Adds a dependency + a second egress path + bundle weight, for capability the existing beacon already covers. Reuse `Transport`. |

**Installation:**
```bash
# No installs. All capability is web-platform or already-vendored.
# Verify the workspace is intact:
cd /data/animeenigma/frontend/web && bun install --frozen-lockfile
```

**Version verification:** Not applicable — no new packages. The two web-platform APIs (`PerformanceObserver`, `sendBeacon`) require no registry check; both are already in use in the codebase.

## Package Legitimacy Audit

> This phase installs **no external packages**. The Package Legitimacy Gate is therefore a no-op.

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| — (none added) | — | — | — | — | N/A | No packages installed this phase |

**Packages removed due to slopcheck [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

*No third-party install occurs; slopcheck/registry verification is not required for this phase. If the planner later decides to vendor a web-vitals helper, run the full Package Legitimacy Gate at that point.*

## Architecture Patterns

### System Architecture Diagram

```
                    BROWSER (trace ROOT)
  ┌───────────────────────────────────────────────────────────────┐
  │  user click                                                    │
  │     │                                                          │
  │     ▼                                                          │
  │  autocapture click listener (index.ts)                        │
  │     │  enqueue click evt (no trace_id yet)                    │
  │     │  registerClickForTrace(evt)  ──────────┐                │
  │     ▼                                         │ (1.5s window)  │
  │  Vue component fires API call                 │                │
  │     │                                         ▼                │
  │  axios request interceptor (api/client.ts)                    │
  │     │  newTraceparent() → {header, traceId}                   │
  │     │  config.headers['traceparent'] = header   ──────────────┼──► to gateway
  │     │  stampTrace(traceId)  ── back-fills the pending click   │
  │     │  ★NEW: also emit an FE "call" register row:             │
  │     │        {source:'fe', trace_id, route, action,           │
  │     │         operation, target='<api path>'}                 │
  │     ▼                                                          │
  │  Transport (sendBeacon batch) ──────────────────────────────┼──► POST /api/analytics/collect
  │                                                               │
  │  ★NEW: PerformanceObserver('resource')                       │
  │     │  for each 3rd-party host (CDN segment, poster, etc.):  │
  │     │    aggregate {host, count, sum(duration)} per flush     │
  │     │    transferSize is 0 for opaque cross-origin (no TAO)   │
  │     │    ⇒ emit row {source:'fe_rum', accuracy:'approx',      │
  │     │        target=host, requests=count, duration_ms}        │
  │     │    NEVER write bytes_in/bytes_out from RUM              │
  │     ▼                                                          │
  │  Transport (same beacon path) ──────────────────────────────┼──► POST /api/analytics/collect
  └───────────────────────────────────────────────────────────────┘
                                                                  │
                                                                  ▼
                    GATEWAY (continues the FE traceparent)
            root span (dd74c301) → propagates trace_id downstream
                                                                  │
                                                                  ▼
                    CATALOG / SCRAPER / STREAMING (Phase 2/3)
            BE effect rows recorded with SAME trace_id, source='be'
                                                                  │
                                                                  ▼
                    ANALYTICS  POST /api/analytics/collect → /internal/effects
                    ★NEW collect.go maps source/route/action/operation/target
                                                                  │
                                                                  ▼
                    ClickHouse  events  (one row per effect)
            FE call rows (source='fe'), FE RUM rows (source='fe_rum'),
            BE effect rows (source='be') — JOINABLE on trace_id
            BYTE AGGREGATIONS MUST: WHERE source='be'
```

A reader can trace the primary use case (a click → its API call → its backend effects, all sharing one `trace_id`) by following the arrows from "user click" down to ClickHouse.

### Recommended Project Structure
```
frontend/web/src/analytics/
├── index.ts            # (exists) wire the new beacons in; init the RUM observer
├── traceparent.ts      # (exists) trace_id minting — REUSE unchanged
├── traceContext.ts     # (exists) click↔trace window association — REUSE; may widen
├── transport.ts        # (exists) sendBeacon batching — REUSE unchanged
├── types.ts            # (extend) add route/action/source to AnalyticsEvent
├── rum.ts              # ★NEW: PerformanceObserver('resource') → fe_rum rows
└── __tests__/
    ├── rum.spec.ts            # ★NEW: 3rd-party host aggregation + flag assertions
    ├── traceContext.spec.ts   # (exists) extend if window logic changes
    └── transport.spec.ts      # (exists)

services/analytics/internal/
├── handler/collect.go  # (extend) map source/route/action/operation/target/target_kind for FE rows
└── domain/event.go     # (exists) Event already carries all needed fields — no change
```

### Pattern 1: FE is the trace ROOT (causation, not observation)
**What:** The browser mints the `traceparent`; the gateway *continues* it. The trace_id therefore exists *before* the request leaves the browser, so it can tag the click that caused the request.
**When to use:** Always — this is the existing, correct design. Do not invert it.
**Example:**
```typescript
// Source: codebase frontend/web/src/api/client.ts:178-182 (existing)
if (TRACING_ON) {
  const { header, traceId } = newTraceparent()
  config.headers['traceparent'] = header
  stampTrace(traceId) // back-fills the pending click within ~1.5s
}
```

### Pattern 2: Best-effort click↔trace window association
**What:** A click is enqueued with no `trace_id`; the next API call within ~1.5s stamps its `trace_id` onto the pending click by **in-place mutation** before the (delayed ≥5s) flush ships it.
**When to use:** AR-FE-02. The existing implementation is sound; the in-place mutation works because flush is delayed.
**Example:**
```typescript
// Source: codebase frontend/web/src/analytics/traceContext.ts:26-33 (existing)
export function stampTrace(traceId: string, withinMs = 1500, now = Date.now()): void {
  for (const p of pending) {
    if (!p.evt.trace_id && now - p.ts <= withinMs) p.evt.trace_id = traceId
  }
  pending = pending.filter((p) => now - p.ts <= withinMs)
}
```

### Pattern 3: RUM rows are structurally byte-poor
**What:** A `source=fe_rum` row carries `requests` (count) + `duration_ms` (timing) + `target` (host), but **never** `bytes_in`/`bytes_out`. For opaque cross-origin resources the browser reports `transferSize=0` anyway (see Common Pitfall 1), so writing a byte field would be a lie. Omitting bytes from RUM rows makes contamination *structurally impossible*, not merely *filtered*.
**When to use:** AR-FE-03. This is the "crafted, slop-resistant detail" the CONTEXT calls out.
**Example:**
```typescript
// fe_rum row shape (illustrative — NEW rum.ts)
analytics.track('rum.resource', {
  // mapped by collect.go to source='fe_rum', accuracy='approx', target_kind='host'
  source: 'fe_rum',
  target: host,            // e.g. 'cdn.mewstream.buzz'
  requests: count,         // how many 3rd-party fetches to this host this flush
  duration_ms: Math.round(sumDuration),
  // NO bytes_in / bytes_out — intentionally absent
})
```

### Anti-Patterns to Avoid
- **Inverting the trace root (reading traceparent from a response header):** breaks causation — the id would arrive after the request that caused it. Keep FE-mint.
- **Summing `transferSize` into `bytes_in` on RUM rows:** `transferSize` is `0` for opaque cross-origin resources; a sum would silently undercount and, worse, could be added to a BE byte total. Never put bytes on `fe_rum` rows.
- **One RUM row per resource entry:** the resource buffer fires constantly during HLS playback (a segment every ~6s × N hosts). Aggregate per (flush-window, host) — mirror the BE HLS per-(session,host) aggregation pattern from Phase 2 (AR-EGRESS-04).
- **A second egress path / new beacon endpoint:** reuse `Transport` + `POST /api/analytics/collect`. A parallel path doubles the CORS/keepalive surface for no benefit.
- **Mutating the click event after flush:** the in-place stamp relies on flush being delayed. Don't shorten `flushMs` below the stamp window without re-checking the ordering invariant.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| W3C trace_id / span_id generation | A custom hex/UUID mangler | `traceparent.ts:newTraceparent()` (exists) | Already W3C-format `00-{32hex}-{16hex}-01`, already CSPRNG via `crypto.getRandomValues` |
| Batched, unload-safe beaconing | A bespoke `fetch(keepalive)` loop | `Transport` (exists) | Already does sendBeacon + fetch fallback + 60KB split + auto-flush + pagehide flush |
| Click→trace correlation | A new map/timer | `traceContext.ts` (exists) | Already a bounded (50-entry) time-window association with in-place stamping |
| Cross-origin byte measurement | Parsing `transferSize` for CDN bytes | **Nothing — omit bytes entirely on RUM rows** | The browser zeroes `transferSize` for opaque cross-origin; any "byte" you compute is fiction (Pitfall 1) |
| Distributed-trace propagation FE→BE | A custom header scheme | The existing `traceparent` header + gateway root span (`dd74c301`) | W3C standard, already continued by the gateway and propagated downstream |

**Key insight:** This phase's risk is *over-building*. The temptation is to add a RUM SDK or invert the trace model. The slop-resistant move is to reuse the four existing primitives and add exactly one new observer module + a thin backend field-mapping.

## Common Pitfalls

### Pitfall 1: Cross-origin resource timings are opaque (the whole reason for `accuracy=approx`)
**What goes wrong:** A `PerformanceResourceTiming` entry for a 3rd-party CDN that does **not** send `Timing-Allow-Origin` reports `transferSize`, `encodedBodySize`, `decodedBodySize` as **0** and zeroes the detailed timing phases (`requestStart`, `responseStart`, `connectStart`, `domainLookupStart`, …). Only `name`, `startTime`, `duration`, and `responseEnd` remain meaningful.
**Why it happens:** Browser privacy/security: without `Timing-Allow-Origin: <our-origin>` (or `*`) on the resource response, the user agent withholds the detailed cross-origin timing/size to prevent cross-site information leaks. Our 3rd-party CDNs (Kodik, Kwik, owocdn, AllAnime, etc.) do not send TAO for us. [CITED: w3.org/TR/resource-timing — "Cross-origin resources"; CITED: developer.mozilla.org PerformanceResourceTiming/transferSize]
**How to avoid:** This is *expected* and is precisely why AR-FE-03 mandates `accuracy=approx`. Emit only `requests` (count) + a coarse `duration_ms` (from `duration`, the one reliable cross-origin field) + `target` (host parsed from `entry.name`). **Never** read `transferSize`/`encodedBodySize` into a byte measure on these rows.
**Warning signs:** A dashboard panel showing non-zero RUM "bytes" — that means someone summed `transferSize` (which is 0 for our CDNs) or, worse, mislabeled a BE row as `fe_rum`.

### Pitfall 2: RUM bytes leaking into authoritative byte totals
**What goes wrong:** A future Phase-5 dashboard does `SELECT sum(bytes_in) FROM events GROUP BY target` *without* a `source` filter, silently mixing approximate (in practice zero) FE_RUM rows into authoritative BE egress bytes.
**Why it happens:** The `source` column defaults to `'be'`; a forgotten `WHERE source='be'` is invisible until the numbers are wrong.
**How to avoid:** (a) Structurally — never write bytes on `fe_rum` rows (Pattern 3), so the worst case is a no-op. (b) By convention — every byte aggregation MUST carry `WHERE source='be'`. (c) By proof — the Validation Architecture below requires a test/query demonstrating the filter (AR-FE-03's "dashboard/query proves these rows are never summed").
**Warning signs:** Any aggregation over `bytes_in`/`bytes_out` lacking a `source` predicate.

### Pitfall 3: FE rows can't be distinguished from clickstream rows
**What goes wrong:** `collect.go` today maps only clickstream fields + `trace_id`; it does **not** read `source`, `origin`, `operation`, `effect_kind`, `target`, `target_kind` from the FE wire. So an FE "call" or "RUM" row would land as a generic clickstream `custom` event with `source='be'` (the column default) — invisible to register pivots and *mis-tagged as backend*.
**Why it happens:** The wire envelope (`wireEvent` in `collect.go`, `AnalyticsEvent` in `types.ts`) predates the register; it has no register-dimension fields.
**How to avoid:** Extend `wireEvent`/`AnalyticsEvent` with optional `source`, `route`/`operation`, `action`, `target`, `target_kind`, `requests`, `duration_ms`; map them in `collect.go` into the already-present `domain.Event` register fields. **Default `source` to `'be'` only when absent is dangerous here** — for FE-originated rows you must explicitly set `source='fe'`/`'fe_rum'`. Consider defaulting the collect path's clickstream rows to `source='fe'` and the RUM rows to `source='fe_rum'`, reserving `'be'` exclusively for backend-recorded effect rows.
**Warning signs:** ClickHouse `SELECT DISTINCT source FROM events` shows only `'be'` after the FE ships — the mapping didn't take.

### Pitfall 4: `useRoute()` is unavailable inside the axios interceptor
**What goes wrong:** Calling `useRoute()` outside a Vue `setup()` throws (no active component instance).
**Why it happens:** Interceptors run in module scope, not component scope.
**How to avoid:** Import the router singleton and read `router.currentRoute.value.fullPath` / `.name` at request time, or stash the current route in a module-level ref updated by a `router.afterEach` hook. Read at request time so the route reflects the call's moment.
**Warning signs:** A "no active instance" / undefined error thrown from the interceptor; the `route` field always empty.

### Pitfall 5: `el_attrs`/`properties` JSON shape mismatch (the snake_case trap, repeat offender)
**What goes wrong:** The FE sends camelCase keys; Go struct tags are snake_case, so fields silently drop to zero values. MEMORY notes this class of bug ("Frontend uses snake_case JSON keys to match Go struct tags") and the spotlight cache-shape note warns stale/empty structs ship blank.
**Why it happens:** Go `json` tags are authoritative; an unmatched key is ignored, not errored.
**How to avoid:** Name every new wire field in `types.ts` exactly as the Go `json:"..."` tag (`trace_id`, `duration_ms`, `target_kind`, `row_count` — note: **`row_count`, never `rows`**, per the ClickHouse schema note A2). Add a wire round-trip test on both sides.
**Warning signs:** FE rows arrive with empty `target`/`route`; `source` defaults silently.

## Code Examples

### Reading the current route inside the interceptor (Pitfall 4 fix)
```typescript
// Source: pattern derived from codebase api/client.ts (interceptor) + vue-router singleton
import router from '@/router'
// at request time, inside the existing interceptor block:
const route = router.currentRoute.value
const routeLabel = (route.name as string) || route.fullPath  // coarse, pattern-like preferred
```

### Aggregating resource entries per 3rd-party host (NEW rum.ts skeleton)
```typescript
// Source: pattern mirrors codebase diagnostics.ts:140-158 (existing observer) + Phase-2 per-host aggregation
const SELF = location.host
const obs = new PerformanceObserver((list) => {
  const agg = new Map<string, { count: number; dur: number }>()
  for (const e of list.getEntries()) {
    const r = e as PerformanceResourceTiming
    let host: string
    try { host = new URL(r.name).host } catch { continue }
    if (host === SELF) continue          // only browser→3rd-party
    const a = agg.get(host) ?? { count: 0, dur: 0 }
    a.count++; a.dur += r.duration        // duration is the one reliable cross-origin field
    agg.set(host, a)
  }
  for (const [host, a] of agg) {
    // emit via the existing analytics beacon, flagged source=fe_rum, accuracy=approx, NO bytes
    analytics.track('rum.resource', { source: 'fe_rum', target: host, requests: a.count, duration_ms: Math.round(a.dur) })
  }
})
obs.observe({ type: 'resource', buffered: true })  // buffered:true catches pre-init entries
```

### The join that proves AR-FE-01/02 (ClickHouse)
```sql
-- Source: schema codebase services/analytics/internal/repo/clickhouse_schema.go
-- An FE click/call row and its BE effects share one trace_id.
SELECT trace_id, source, origin, operation, effect_kind, target, requests, bytes_in, bytes_out
FROM events
WHERE trace_id = {trace:String}
ORDER BY timestamp;
-- Expect: one source='fe' (or 'fe_rum') row + N source='be' effect rows, same trace_id.
```

### The query that proves AR-FE-03 non-contamination
```sql
-- Authoritative byte total MUST filter source='be'. FE_RUM rows carry no bytes,
-- and are excluded regardless — this query demonstrates the discipline.
SELECT target, sum(bytes_in) AS authoritative_in, sum(bytes_out) AS authoritative_out
FROM events
WHERE effect_kind = 'egress' AND source = 'be'
GROUP BY target ORDER BY authoritative_in DESC;
-- Cross-check: sum(bytes_in) WHERE source='fe_rum' == 0 (rows carry no bytes).
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| FE had no trace propagation; gateway propagated nothing | FE mints `traceparent`, gateway roots a span + propagates downstream | commit `dd74c301` (Phase pre-work) | FE-minted trace_id is genuinely shared with BE effect rows — the foundation AR-FE-01/02 build on |
| `transferSize` once exposed cross-origin | Zeroed for opaque cross-origin without `Timing-Allow-Origin` (and UA MAY zero even with TAO) | Resource Timing L2/L3 | Cross-origin RUM is timing-only; bytes are unavailable → `accuracy=approx` is mandatory, not optional |

**Deprecated/outdated:**
- Treating `PerformanceResourceTiming.transferSize` as a reliable byte count for 3rd-party CDNs — it is `0` for our providers (no TAO). Don't.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The FE-mint trace-root design (browser mints `traceparent`, gateway continues) is the intended, correct model and should be kept | Alternatives / Pattern 1 | Low — the gateway already continues the FE traceparent (`dd74c301`), and causation requires the id to exist before the request. If the team wants the gateway to be the root, the click→trace stamp breaks; flag for confirmation. |
| A2 | `collect.go` must be extended to map FE register fields (`source`/`route`/`action`/`operation`/`target`/`target_kind`) — they are not mapped today | Pitfall 3 / structure | Low — verified by reading `collect.go` (only clickstream fields + `trace_id` mapped). If a parallel change already added them, the planner deduplicates. |
| A3 | FE clickstream rows should be tagged `source='fe'` (not the column default `'be'`) so backend byte aggregations cleanly exclude all FE-origin rows | Pitfall 2/3 | Medium — this is a semantic choice. If existing clickstream rows are intentionally `source='be'`, changing the default could shift historical interpretation. Recommend: only NEW FE call/RUM rows get `fe`/`fe_rum`; leave the historical clickstream default untouched, OR explicitly backfill — a planner/user decision. |
| A4 | The ~1.5s click→trace window and ≥5s flush delay remain compatible after adding the FE "call" row emission | Pattern 2 | Low — emitting an extra FE row doesn't change the stamp timing, but verify the new RUM/call beacons don't trigger an early `size`-flush that ships a click before its stamp lands. |

**If you change `flushMs` or `maxBatch`, re-verify A4 — the in-place stamp depends on flush being delayed past the 1.5s window.**

## Open Questions (RESOLVED)

1. **Should the FE "call" register row be emitted for every axios request, or only for semantic/whitelisted actions?**
   - What we know: AR-FE-01 says "stamps each call with the current route + optional semantic action." Every call already mints a trace_id; emitting a register row for *every* call could be high-volume (every poster fetch, every poll).
   - What's unclear: whether the register wants one FE row per API call, or only per semantically-meaningful action (e.g. "play episode", "add to list").
   - Recommendation: Emit a lightweight `source='fe'` call row per API request with `route` always set and `action` set only when a caller provides a semantic label (an opt-in `config` field on the axios call). This satisfies "optional semantic action" without forcing every call to be labeled. Confirm volume tolerance against the 90-day ClickHouse TTL + drop-on-full ingest. [Tie to A3.]

2. **Does `source='fe'` vs `source='be'` for the existing clickstream change any current dashboard?**
   - What we know: clickstream rows currently default to `source='be'` (column default), and Phase-1 dashboards render from them.
   - What's unclear: whether any existing panel filters on `source` today.
   - Recommendation: Grep the Grafana dashboards for `source=` before changing clickstream tagging; if no panel depends on it, tagging new FE rows `fe`/`fe_rum` is safe and historical rows are untouched.

3. **Throttling the RUM observer during HLS playback.**
   - What we know: during playback the resource buffer fills with a segment every ~6s across rotating CDN hosts; per-(flush,host) aggregation bounds row count.
   - What's unclear: whether even aggregated rows at the analytics flush cadence (≥5s) are acceptable volume, or whether a coarser RUM flush interval is wanted.
   - Recommendation: Aggregate per host per analytics flush; if volume is still high, add a dedicated coarser RUM flush (e.g. 30s) separate from the click/heartbeat flush. Decide during planning.

## Environment Availability

> This phase is browser code + a Go handler edit. The only "dependencies" are web-platform APIs and the already-running analytics stack.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `PerformanceObserver` (`resource`) | AR-FE-03 RUM | ✓ (all evergreen browsers; already used in `diagnostics.ts`) | Web platform | Feature-detect (`typeof PerformanceObserver !== 'undefined'`) → skip RUM silently, as `diagnostics.ts` does |
| `navigator.sendBeacon` | beacon transport | ✓ (already used) | Web platform | `fetch(keepalive)` fallback already in `Transport.send` |
| `crypto.getRandomValues` | trace_id mint | ✓ (already used) | Web platform | none needed (universally available in secure contexts) |
| ClickHouse `events` table w/ `trace_id`/`source`/`accuracy` | the sink | ✓ | per Phase 1 schema | none — already deployed and verified live (Phase 2/3) |
| Gateway root-span + traceparent propagation | trace sharing | ✓ | commit `dd74c301` | none — already shipped |
| analytics `POST /api/analytics/collect` | FE beacon ingest | ✓ | live | none — already serving |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** `PerformanceObserver` is feature-detected; absence degrades to "no RUM rows," never an error.

## Validation Architecture

> `workflow.nyquist_validation: true` — this section is required.

### Test Framework
| Property | Value |
|----------|-------|
| Framework (FE) | `vitest` (jsdom) + `@vue/test-utils`, run via `bunx vitest` |
| Framework (BE) | Go `testing` (`go test ./...`), httptest for the collect handler |
| Config file (FE) | `frontend/web/vitest.config.*` (existing; all `src/analytics/__tests__/*.spec.ts` run under it) |
| Quick run command (FE) | `cd frontend/web && bunx vitest run src/analytics/` |
| Quick run command (BE) | `cd services/analytics && go test ./internal/handler/... -count=1` |
| Full suite command | `cd frontend/web && bunx vitest run && bunx tsc --noEmit` ; `cd services/analytics && go test ./... -race` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AR-FE-01 | Interceptor mints trace_id + stamps route + optional action; FE call row emitted | unit | `bunx vitest run src/analytics/__tests__/index.spec.ts` (extend) + a new interceptor test | ⚠️ partial — trace_id minting tested (`traceparent.spec.ts`); route/action/FE-row emission NOT yet — ❌ Wave 0 |
| AR-FE-01 | `collect.go` maps source/route/action/operation/target into `domain.Event` register fields | unit (Go) | `go test ./internal/handler/... -run TestCollectMapsFERegisterFields` | ❌ Wave 0 (new test + handler edit) |
| AR-FE-01 | A real FE call appears in the register joined (same trace_id) to its BE effects | integration / live | ClickHouse join query (Code Examples §"join that proves AR-FE-01/02") | ❌ Wave 0 (live phase-gate, non-autonomous — mirror Phase 2/3 closeout) |
| AR-FE-02 | A captured click carries the trace_id of the call it triggered | unit | `bunx vitest run src/analytics/__tests__/traceContext.spec.ts` (extend to assert end-to-end click→stamp) | ⚠️ partial (`traceContext.spec.ts` exists) — ❌ Wave 0 to extend |
| AR-FE-02 | click ↔ BE effect share one trace_id end-to-end | integration / live | ClickHouse join query filtered to a click `event_type='click'` row + its `source='be'` effects | ❌ Wave 0 (live) |
| AR-FE-03 | `PerformanceObserver` emits per-host rows flagged `source=fe_rum, accuracy=approx`, no bytes | unit | `bunx vitest run src/analytics/__tests__/rum.spec.ts` | ❌ Wave 0 (new module + test) |
| AR-FE-03 | RUM rows are never summed with BE bytes | unit (Go) + query | assert collect maps `fe_rum`→`source='fe_rum'` + writes no bytes; ClickHouse `sum(bytes_in) WHERE source='fe_rum' == 0` | ❌ Wave 0 |

### How each AR-FE criterion is validated (the specific proofs the planner must encode)

- **The trace_id join query (AR-FE-01):** After a real FE call, run the `WHERE trace_id = {trace}` query (Code Examples). PASS = exactly one `source IN ('fe','fe_rum')` row **and** ≥1 `source='be'` effect row, all sharing the trace_id. This is the live phase-gate, mirroring the Phase 2/3 non-autonomous ClickHouse closeout.
- **The click→effect join (AR-FE-02):** Trigger a tracked click that fires an API call. PASS = the `event_type='click'` row's `trace_id` is non-empty **and** equals the `trace_id` on the downstream `source='be'` effect row(s). Unit-level proof: `traceContext.spec.ts` asserts `stampTrace` back-fills the pending click within the window; live proof: the join query.
- **The `source=be` filter proof (AR-FE-03):** Two-part. (1) Structural: a Go test asserts `collect.go` writes a `fe_rum` row with `bytes_in==0 && bytes_out==0`. (2) Query: `sum(bytes_in) WHERE source='be'` (authoritative) vs `sum(bytes_in) WHERE source='fe_rum'` (== 0). PASS = the authoritative total excludes RUM rows **and** the RUM byte sum is provably zero. A Phase-5 dashboard panel may render this, but the phase-gate is the query/test.

### Sampling Rate
- **Per task commit:** `bunx vitest run src/analytics/` (FE) and/or `go test ./internal/handler/... -count=1` (BE) — whichever the task touched.
- **Per wave merge:** `cd frontend/web && bunx vitest run && bunx tsc --noEmit` + `cd services/analytics && go test ./... -race`.
- **Phase gate:** Full suite green, then the live ClickHouse join + non-contamination queries (non-autonomous human-verify, mirroring `02-04`/`03-06`).

### Wave 0 Gaps
- [ ] `frontend/web/src/analytics/rum.ts` + `__tests__/rum.spec.ts` — new RUM observer + per-host aggregation + flag assertions (AR-FE-03)
- [ ] Extend `frontend/web/src/analytics/types.ts` — add `route`, `action`, `source`, `target`, `target_kind`, `requests`, `duration_ms` to `AnalyticsEvent` (snake_case, matching Go tags)
- [ ] Extend `frontend/web/src/api/client.ts` interceptor — read route from the router singleton + emit the FE call register row (AR-FE-01); add a test
- [ ] Extend `services/analytics/internal/handler/collect.go` + `collect_test.go` — map the new register fields into `domain.Event`, default FE rows to `source='fe'`/`'fe_rum'` (AR-FE-01/03)
- [ ] Extend `frontend/web/src/analytics/__tests__/traceContext.spec.ts` — assert end-to-end click→stamp if the window logic changes (AR-FE-02)
- [ ] Live phase-gate runbook — the two ClickHouse queries (join + non-contamination), non-autonomous, mirroring Phase 2/3 closeout

*(Framework itself needs no install — vitest + Go testing both already configured and in use.)*

## Project Constraints (from CLAUDE.md)

These directives carry locked-decision authority; the plan must comply.

- **Frontend tooling is `bun`/`bunx`, never npm/pnpm/npx.** All FE test/build/typecheck commands use `bunx vitest` / `bunx tsc --noEmit`.
- **No days/hours/sprints.** Any plan metrics use `UXΔ` / `CDI` / `MVQ` per `.planning/CONVENTIONS.md`. This phase is instrumentation-only (CONTEXT: "NO rendered UI surface" — no UI-SPEC required).
- **Design-system lint gate** applies to any `.vue` touched — but this phase touches **no `.vue` rendering** (pure TS modules + a Go handler), so the DS lint is effectively N/A here. Do not introduce styling.
- **Effort metrics + changelog in Russian Trump-mode via `/animeenigma-after-update`** at phase close (batch if multiple small updates).
- **`/internal/*` is Docker-network-only, never gateway-proxied.** FE beacons go to the **public** `POST /api/analytics/collect` (gateway-proxied), NOT `/internal/effects`. Do not add a gateway route for any internal effect endpoint.
- **ClickHouse measure column is `row_count`, never `rows`** (schema note A2); wire tags must match exactly (`trace_id`, `duration_ms`, `target_kind`, `row_count`).
- **snake_case JSON keys to match Go struct tags** (MEMORY: repeat-offender bug class) — every new FE wire field must equal its Go `json:"..."` tag.
- **`go.work` / Dockerfile COPY rule** for new `libs/` modules — **N/A**: this phase adds no new `libs/` module (FE code + an existing-service handler edit only).
- **Verify rendered changes in a real browser** — N/A (no rendered surface); the live phase-gate verifies in ClickHouse instead.

## Security Domain

> `security_enforcement` is not set to `false` in config → included. This phase moves data browser→backend, so input-validation and PII categories apply.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | The collect endpoint is intentionally unauthenticated (anonymous clickstream); no new auth surface |
| V3 Session Management | no | Reuses the existing `anonymous_id`/`session_id`; no new session primitive |
| V4 Access Control | no | No new privileged route; `/internal/*` stays Docker-network-only (CLAUDE.md) |
| V5 Input Validation | **yes** | `collect.go` already `LimitReader`s 256KB, validates event_type, drops clock-skewed events; NEW fields must be length-capped + the `source` value must be **whitelisted** server-side (`fe`/`fe_rum`/`be` only) so a malicious beacon cannot inject an arbitrary `source` that pollutes pivots |
| V6 Cryptography | no | trace_id is a correlation id, not a secret; `crypto.getRandomValues` is for uniqueness, not security — never hand-roll crypto |
| V7/V8 Data Protection / Privacy | **yes** | RUM `target` is a 3rd-party host (already public knowledge — these are the streaming CDNs). `el_text` already PII-stripped (`autocapture.stripPII`). Ensure RUM `entry.name` is reduced to **host only** (no full URL with query params/tokens — signed CDN URLs carry auth tokens) |

### Known Threat Patterns for {Vue SPA → Go collect handler}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Forged beacon injects arbitrary `source`/`origin` to corrupt register pivots | Tampering | Server-side whitelist the `source` enum in `collect.go` (accept only `fe`/`fe_rum`); reject/normalize unknown values. Same for `effect_kind`/`target_kind`. |
| Signed CDN URL (with auth token) leaked into a RUM `target` row | Information Disclosure | Parse `entry.name` to **host only** before beaconing — never ship the full resource URL (segment URLs carry `tham/h` signed windows per MEMORY `vidstream_vip` note) |
| PII in click `el_text` shipped to analytics | Information Disclosure | Already mitigated by `autocapture.stripPII` (email/digit-run scrub, 200-char cap) — reuse, do not bypass |
| Beacon flood / oversized payload (DoS) | Denial of Service | `collect.go` already `LimitReader(256KB)`; the ingest batcher is drop-on-full (Phase 1 AR-STORE-05). RUM aggregation per-host bounds event count. No new DoS surface if aggregation is honored. |
| trace_id used to correlate/deanonymize a user across sessions | Information Disclosure | trace_id is per-call ephemeral (minted fresh per axios request); it is not a stable user identifier. Do not persist or reuse a trace_id across calls. |

## Sources

### Primary (HIGH confidence)
- Codebase `frontend/web/src/analytics/{traceparent,traceContext,types,index,autocapture,transport,session,identity}.ts` — existing FE analytics module (read in full)
- Codebase `frontend/web/src/api/client.ts` — axios interceptor that mints traceparent + stamps clicks (read in full)
- Codebase `frontend/web/src/utils/diagnostics.ts:139-159` — existing `PerformanceObserver('resource')` reference pattern
- Codebase `services/analytics/internal/handler/collect.go` — current wire contract (clickstream + trace_id only; register fields NOT mapped)
- Codebase `services/analytics/internal/domain/event.go` — `domain.Event` already carries all register dimensions/measures
- Codebase `services/analytics/internal/repo/clickhouse_schema.go` — `events` table has `trace_id`/`source`/`accuracy`/`origin`/`operation`/`effect_kind`/`target`/`target_kind`/`row_count`
- Codebase `libs/tracing/{middleware,setup}.go` + `services/gateway/internal/service/proxy.go` — gateway root span + W3C traceparent propagation (`dd74c301`)
- `.planning/phases/02-be-egress-recorder/02-{01,04}-SUMMARY.md`, `.planning/phases/03-…/03-01-SUMMARY.md` — the BE effect/trace/operation plane FE rows join to (per-(session,host) aggregation pattern; PII-on-private-ctx; `row_count` not `rows`)
- `.planning/ROADMAP.md` Phase 4 + success criteria AR-FE-01..03; `04-CONTEXT.md`; `.planning/config.json`
- `./CLAUDE.md` + auto-memory — project constraints (bun, snake_case wire tags, `/internal/*` Docker-only, design-system gate, signed CDN URL token caution)

### Secondary (MEDIUM confidence)
- [W3C Resource Timing](https://www.w3.org/TR/resource-timing/) — cross-origin opaque-entry restrictions; `transferSize`/sizes zeroed without `Timing-Allow-Origin`
- [MDN PerformanceResourceTiming.transferSize](https://developer.mozilla.org/en-US/docs/Web/API/PerformanceResourceTiming/transferSize) — `transferSize` is 0 for cross-origin without TAO; UA MAY zero even with TAO

### Tertiary (LOW confidence)
- None — all critical claims verified against the codebase or the W3C spec.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new packages; every primitive read directly in the repo.
- Architecture: HIGH — the trace-root model, the wire contract, the schema, and the gateway propagation were all read in source; the gaps (route/action mapping, RUM module, non-contamination proof) are precisely located.
- Pitfalls: HIGH — the cross-origin zeroing is W3C/MDN-confirmed; the snake_case + `row_count` + `/internal` traps are documented in CLAUDE.md/MEMORY and corroborated in code.

**Research date:** 2026-06-06
**Valid until:** 2026-07-06 (stable — web-platform APIs + an internal schema that already shipped; re-verify only if the analytics wire contract or the gateway propagation changes)
