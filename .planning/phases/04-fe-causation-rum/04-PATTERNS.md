# Phase 4: FE Causation + RUM - Pattern Map

**Mapped:** 2026-06-06
**Files analyzed:** 6 (2 new, 4 modified)
**Analogs found:** 6 / 6 (all in-repo; this is a completion+hardening phase, every analog is a sibling primitive)

This phase has NO greenfield files in the "no analog" sense — every new/modified file copies from an adjacent existing primitive in the SAME package. The planner should treat the existing analytics module + collect handler as the canonical templates, not RESEARCH.md sketches.

## File Classification

| New/Modified File | New? | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|------|-----------|----------------|---------------|
| `frontend/web/src/analytics/rum.ts` | NEW | utility (observer/beacon module) | event-driven → batch | `frontend/web/src/utils/diagnostics.ts:139-159` (PerformanceObserver shape) + `frontend/web/src/analytics/index.ts` (track/enqueue + init lifecycle) | role+flow match (synthesized from two analogs) |
| `frontend/web/src/analytics/__tests__/rum.spec.ts` | NEW | test | — | `frontend/web/src/analytics/__tests__/transport.spec.ts` | exact (same dir, same vitest+jsdom+vi.fn mocking style) |
| `frontend/web/src/analytics/types.ts` | MODIFY | model (wire type) | transform (FE→wire) | self (extend `AnalyticsEvent`) — Go mirror is `collect.go:wireEvent` | exact |
| `frontend/web/src/api/client.ts` (interceptor block) | MODIFY | middleware (request interceptor) | request-response | self, `client.ts:178-182` (existing traceparent+stampTrace block) | exact (extend the very block that already mints trace_id) |
| `services/analytics/internal/handler/collect.go` | MODIFY | handler (ingest mapping) | transform (wire→domain) | self, `collect.go:81-120` (existing event loop + `domain.Event` construction) | exact |
| `services/analytics/internal/handler/collect_test.go` | NEW (or extend) | test (Go) | — | existing collect handler test conventions (httptest + fake `Sink`) | role match |

**No `traceparent.ts` / `traceContext.ts` / `transport.ts` changes required** — REUSE unchanged (RESEARCH "Don't Hand-Roll"). They are *upstream analogs*, not edit targets. `domain/event.go` already carries every register field (verified `event.go:64-83`) — NO change.

## Shared Invariants (apply to ALL files in this phase)

### snake_case wire keys MUST equal Go `json:"..."` tags
**Source of truth:** `services/analytics/internal/handler/collect.go:41-56` (`wireEvent` struct tags)
**Apply to:** `types.ts` (new fields), `client.ts` (FE row props), `rum.ts` (RUM row props), `collect.go` (new wire fields)
Every new field added on the FE MUST be named exactly as its Go `json:` tag. Repeat-offender bug class (MEMORY: "Frontend uses snake_case JSON keys to match Go struct tags"). Canonical names: `trace_id`, `duration_ms`, `target_kind`, `row_count` (**never `rows`** — schema note, `clickhouse_schema.go:21`).

### `source` MUST be explicitly set for FE rows — the default is a trap
**Source:** `services/analytics/internal/repo/clickhouse_store.go:119` — `defaultStr(e.Source, "be")`
**Apply to:** `collect.go` mapping
An FE row that leaves `Source` empty is silently written as `source='be'` (mis-tagged as backend → contaminates byte pivots). `collect.go` MUST set `Source='fe'` for FE call rows and `Source='fe_rum'` for RUM rows. Server-side WHITELIST the value (`fe`/`fe_rum` only) — a forged beacon must not inject an arbitrary `source` (RESEARCH Security: Tampering).

### RUM rows are structurally byte-poor
**Source:** `services/analytics/internal/domain/event.go:78-82` (`BytesIn`/`BytesOut` measures) — leave them ZERO for `fe_rum`.
**Apply to:** `rum.ts` (never emit a byte field) + `collect.go` (never map a byte from an `fe_rum` row).
A `fe_rum` row carries `requests` + `duration_ms` + `target` only. Omitting bytes makes contamination structurally impossible (RESEARCH Pattern 3). The Go test must assert `BytesIn==0 && BytesOut==0` on a mapped `fe_rum` row.

---

## Pattern Assignments

### `frontend/web/src/analytics/rum.ts` (NEW — utility, event-driven → batch)

**Analog A (observer shape):** `frontend/web/src/utils/diagnostics.ts:139-159`
**Analog B (emit + lifecycle):** `frontend/web/src/analytics/index.ts:71-73` (`track`) and `:19-57` (`init` idempotency + feature-detect + listener teardown)

**Observer + feature-detect pattern to copy** (`diagnostics.ts:140-159`):
```typescript
if (typeof PerformanceObserver !== 'undefined') {
  try {
    const observer = new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        const res = entry as PerformanceResourceTiming
        // ... res.name, res.duration ...
      }
    })
    observer.observe({ type: 'resource', buffered: false })
  } catch {
    // PerformanceObserver not supported
  }
}
```
**Adapt:** use `buffered: true` (catch pre-init entries, per RESEARCH Code Examples); aggregate per 3rd-party host (skip `host === location.host`); reduce `res.name` to **host only** via `new URL(res.name).host` (NEVER ship full URL — signed CDN URLs carry `tham/h` auth tokens, RESEARCH Security: Information Disclosure); read ONLY `res.duration` (the one reliable cross-origin field — `transferSize` is 0 for opaque cross-origin).

**Emit pattern to copy** (`index.ts:71-73`):
```typescript
track(name: string, props?: Record<string, unknown>): void {
  this.enqueue({ event_type: 'custom', event_name: name, timestamp: nowISO(), path: location.pathname, properties: props })
}
```
**Adapt:** emit one row per (flush-window, host): `analytics.track('rum.resource', { source: 'fe_rum', target: host, requests: count, duration_ms: Math.round(sumDur) })`. Do NOT one-row-per-entry — the resource buffer fires a segment every ~6s × N hosts during HLS playback (RESEARCH Anti-Pattern; mirrors the BE per-(session,host) aggregation from Phase 2 AR-EGRESS-04).

**Init lifecycle to copy** (`index.ts:19-21`, `:36`, `:52-53`): idempotent `init` guard, `document.addEventListener`/`window.addEventListener` registration with stored listener refs for teardown. Wire the RUM observer init into `index.ts` `init()` alongside the existing click listener.

---

### `frontend/web/src/analytics/__tests__/rum.spec.ts` (NEW — test)

**Analog:** `frontend/web/src/analytics/__tests__/transport.spec.ts` (read in full)

**Pattern to copy** (mocking + vitest structure, `transport.spec.ts:1-33`):
```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest'
// ...
beforeEach(async () => {
  vi.resetModules()
  beacon = vi.fn().mockReturnValue(true)
  // @ts-expect-error jsdom has no sendBeacon by default
  navigator.sendBeacon = beacon
  localStorage.clear()
})
```
**Adapt:** stub `PerformanceObserver` (jsdom has none — same `@ts-expect-error` + `vi.fn`/`vi.stubGlobal` approach used for `sendBeacon`); feed synthetic `PerformanceResourceTiming`-like entries (cross-origin host + a `location.host` self-entry); assert: (1) self-host entries are dropped, (2) same-host entries aggregate into one row with summed `requests` + `duration_ms`, (3) emitted props carry `source: 'fe_rum'` and NO `bytes_in`/`bytes_out` key, (4) `target` is host-only (no path/query). ≥5 assertions (matches `index.spec.ts`/`autocapture.spec.ts` density).

---

### `frontend/web/src/analytics/types.ts` (MODIFY — model/wire type)

**Analog:** self — extend the existing `AnalyticsEvent` interface (`types.ts:5-20`). Go mirror is `collect.go:wireEvent` (`:41-56`).

**Existing shape to extend** (`types.ts:5-20`):
```typescript
export interface AnalyticsEvent {
  event_type: EventType
  event_name?: string
  timestamp: string // ISO 8601
  // ... el_* fields ...
  trace_id?: string // stamped by the axios interceptor (Plan 3)
  properties?: Record<string, unknown>
}
```
**Add (optional, snake_case to match the Go tags you will add to `wireEvent`):**
`source?: string`, `route?: string`, `action?: string`, `operation?: string`, `target?: string`, `target_kind?: string`, `requests?: number`, `duration_ms?: number`. Keep all optional so clickstream rows (which omit them) are unaffected — mirrors how `domain.Event` register fields default empty for clickstream rows (`event.go:64-67` comment).

---

### `frontend/web/src/api/client.ts` (MODIFY — request interceptor)

**Analog:** self — extend the existing tracing block at `client.ts:178-182` (read in full at `:146-188`).

**Existing block to extend** (`client.ts:178-182`):
```typescript
if (TRACING_ON) {
  const { header, traceId } = newTraceparent()
  config.headers['traceparent'] = header
  stampTrace(traceId)
}
```
**Adapt:** after `stampTrace(traceId)`, emit a lightweight FE call register row via the analytics beacon: `{ source: 'fe', trace_id: traceId, route, action, target: <api path> }`. The `trace_id` here is the SAME id that stamps the pending click — this is what gives the click and the call (and downstream BE effects) a shared `trace_id`.

**Pitfall (RESEARCH P4):** `useRoute()` THROWS in an interceptor (module scope, no active component). Read the router singleton at request time: `import router from '@/router'; const route = router.currentRoute.value` → use `route.name ?? route.fullPath`. Set `action` only when a caller passes a semantic label on the axios `config` (opt-in — satisfies "optional semantic action" without forcing every poster/poll fetch to be labeled; RESEARCH Open Q1).

**A4 ordering invariant:** emitting the extra FE row must NOT trigger an early `size`-flush that ships a click before its stamp lands — the in-place stamp depends on flush being delayed past the 1.5s window (`traceContext.ts:26-33`, `transport.ts:24` `>= maxBatch` flush). Do not shorten `flushMs`/`maxBatch`.

---

### `services/analytics/internal/handler/collect.go` (MODIFY — ingest mapping)

**Analog:** self — extend `wireEvent` (`collect.go:41-56`) + the `domain.Event` construction in the event loop (`collect.go:100-115`).

**Existing wireEvent struct to extend** (`collect.go:41-56`) — add fields with EXACT snake_case tags matching `types.ts`:
```go
type wireEvent struct {
	EventType  string            `json:"event_type"`
	// ... existing ...
	TraceID    string            `json:"trace_id"`
	Properties json.RawMessage   `json:"properties"`
	// NEW (Phase 4):
	// Source string `json:"source"`; Route/Operation string `json:"operation"`;
	// Action string `json:"action"`; Target string `json:"target"`;
	// TargetKind string `json:"target_kind"`; Requests int `json:"requests"`;
	// DurationMS int `json:"duration_ms"`
}
```

**Existing domain.Event construction to extend** (`collect.go:100-115`) — the register dimension fields (`event.go:64-83`) are currently NEVER set by this handler (Pitfall 3, A2 verified). Map the new wire fields:
```go
ev := domain.Event{
	EventID:     uuid.NewString(),
	EventType:   domain.EventType(we.EventType),
	// ... existing ...
	TraceID: we.TraceID, Properties: props,
	// NEW: Source/Operation/Target/TargetKind/Requests/DurationMS from we.*
}
```
**CRITICAL — set Source explicitly + whitelist it.** `clickhouse_store.go:119` does `defaultStr(e.Source, "be")`, so an unset `Source` becomes `'be'` and the row is mis-tagged backend. Map `we.Source` → `ev.Source` ONLY if it is in {`fe`, `fe_rum`} (server-side whitelist; reject/normalize others — Tampering mitigation). For `fe_rum` rows, do NOT map any byte field (leave `BytesIn`/`BytesOut` zero — RESEARCH Pattern 3).

**Reuse the existing guards** (already in `collect.go`, do not weaken): `LimitReader(256*1024)` (`:67`), zero-timestamp fallback + >1-day clock-skew drop (`:83-88`), per-event `ev.Validate()` skip-bad-keep-rest (`:116-118`). Length-cap the new string fields (V5 Input Validation).

---

### `services/analytics/internal/handler/collect_test.go` (NEW/extend — Go test)

**Analog:** the existing `Sink` fake seam (`collect.go:18-22` — `Sink interface { Enqueue(domain.Event) bool }`) + httptest. Mirror Phase 2/3 handler-test conventions (`go test ./internal/handler/... -count=1`).

**Two proofs to encode (RESEARCH Validation §AR-FE-03):**
1. `TestCollectMapsFERegisterFields` — POST an envelope with a `source:'fe'` call row; assert the captured `domain.Event` carries `Source=="fe"`, `Operation`/`Target` populated (not the `'be'` default).
2. `TestFERUMRowCarriesZeroBytes` — POST a `source:'fe_rum'` row with `requests`+`duration_ms`; assert `ev.Source=="fe_rum" && ev.BytesIn==0 && ev.BytesOut==0`. Also assert a forged `source:'evil'` is rejected/normalized (not written as-is).

Use a fake `Sink` that records the last enqueued `domain.Event` (no testify/mock — match the repo's handwritten-fake convention from CLAUDE.md spotlight note).

---

## Shared Patterns

### sendBeacon batching (REUSE unchanged — do NOT build a second egress path)
**Source:** `frontend/web/src/analytics/transport.ts` (full file)
**Apply to:** `rum.ts` (route RUM rows through `analytics.track`/`enqueue` → existing `Transport`), `client.ts` (FE call row same path)
The `Transport` already does sendBeacon + fetch(keepalive) fallback (`transport.ts:57-82`), 60KB split-and-recurse (`:60-65`), auto-flush interval + size flush (`:22-37`), `text/plain` no-preflight (`:67-70`). A parallel beacon endpoint doubles CORS/keepalive surface for zero benefit (RESEARCH Anti-Pattern).

### trace_id minting (REUSE unchanged)
**Source:** `frontend/web/src/analytics/traceparent.ts:13-18` — `newTraceparent()` returns `{ header: '00-{32hex}-{16hex}-01', traceId }`, CSPRNG via `crypto.getRandomValues`.
**Apply to:** `client.ts` only (already calls it at `:179`). RUM rows do NOT mint their own trace_id (resource timings aren't request-scoped).

### click↔trace window association (REUSE; widen only if needed)
**Source:** `frontend/web/src/analytics/traceContext.ts:26-33` — `stampTrace` in-place back-fills the pending click within `withinMs=1500`; bounded to 50 entries (`:20-21`).
**Apply to:** `client.ts` interceptor (already calls `stampTrace(traceId)` at `:181`). The new FE call row emission must not disturb the flush-delay ordering this depends on (A4).

### register dimension defaults (the backend safety net you must NOT rely on for source)
**Source:** `services/analytics/internal/repo/clickhouse_store.go:114-120` — `defaultStr(e.Origin,"api")`, `defaultStr(e.Source,"be")`, `defaultStr(e.Accuracy,"exact")`.
**Apply to:** `collect.go` — these defaults are correct for clickstream/BE rows but DANGEROUS for FE rows: an unset `Source` silently becomes `'be'`. FE rows MUST set `Source` explicitly. `Accuracy` for `fe_rum` should be set to `'approximate'` (default is `'exact'`).

## No Analog Found

None. Every file in this phase has a direct in-repo analog (typically a sibling in the same package). This is the defining property of a completion+hardening phase.

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| — | — | — | (none) |

## Metadata

**Analog search scope:** `frontend/web/src/analytics/`, `frontend/web/src/api/`, `frontend/web/src/utils/`, `services/analytics/internal/{handler,domain,repo}/`
**Files scanned (read):** `transport.ts`, `traceContext.ts`, `types.ts`, `traceparent.ts`, `index.ts`, `api/client.ts` (interceptor), `utils/diagnostics.ts` (observer), `handler/collect.go`, `domain/event.go`, `repo/clickhouse_store.go` (mapping), `repo/clickhouse_schema.go` (defaults), `__tests__/transport.spec.ts`
**Pattern extraction date:** 2026-06-06
