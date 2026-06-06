# Phase 4: FE Causation + RUM - Context

**Gathered:** 2026-06-06
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)

<domain>
## Phase Boundary

A frontend action and its resulting backend effects share one `trace_id`, and browser→third-party resource timings are beaconed as clearly-flagged approximate rows that never contaminate authoritative backend bytes.

Depends on: Phase 1 (ClickHouse-backed analytics collector is the sink); integrates with Phase 2/3 dimensions so FE rows join to BE effects on `trace_id`/`operation`.

Requirements: AR-FE-01 (axios interceptor sends active trace_id + route + optional semantic action), AR-FE-02 (click auto-capture events carry trace_id), AR-FE-03 (PerformanceObserver beacons browser→3rd-party resource timings flagged source=fe_rum, accuracy=approx; never summed with authoritative BE bytes).
</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — discuss phase was skipped per user setting. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

Key existing seams to reuse (from prior phases / CLAUDE.md):
- Gateway already mints `traceparent` and propagates a root span (commits dd74c301 / e0b621f8) — the FE must read/propagate the same `trace_id`.
- Analytics collector ingest path: `POST /api/analytics/collect` (gateway-proxied) → analytics service → ClickHouse `events` (dualwrite). FE RUM rows land here.
- The `events` schema already carries `operation`, `effect_kind`, `target`, `target_kind` — FE rows must use a distinct `source`/`effect_kind` so byte aggregations can filter `source=be`.
- Frontend uses Vue 3 + axios; `frontend/web/src/` with an existing analytics util. Use `bun` for tooling.

This is instrumentation-only — NO rendered UI surface (axios interceptor + click capture + PerformanceObserver beacon). No UI-SPEC required.
</decisions>

<code_context>
## Existing Code Insights

Codebase context will be gathered during plan-phase research (axios interceptor location, existing analytics client, trace_id propagation from gateway, ClickHouse events schema).
</code_context>

<specifics>
## Specific Ideas

Refer to ROADMAP Phase 4 success criteria (AR-FE-01..03). The `accuracy=approx` / `source=fe_rum` flagging discipline is the crafted, slop-resistant detail — RUM rows must be structurally impossible to confuse with authoritative BE bytes.
</specifics>

<deferred>
## Deferred Ideas

None — discuss phase skipped.
</deferred>
