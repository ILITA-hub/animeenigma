# Phase 08: PlatformStatsCard refactor - Context

**Gathered:** 2026-05-22
**Mode:** Auto-generated from approved REFACTOR-PROPOSAL.md

<domain>
Fill the N=1 dead space:
- Hero stat (left, oversized 8xl number, accent color) + 2×2 micro-grid (right) of 4 supporting metrics.
- 7-day sparkline (pure SVG, no chart-library dep).
- Delta chip vs prior week (↑/↓/—).
- Backend extension: emit `previous_value` + `series[7]`.

In scope: `cards/PlatformStatsCard.vue` + spec, new `Sparkline.vue` + spec, new `DeltaChip.vue` + spec, `services/catalog/internal/service/spotlight/cards/platform_stats.go` (struct extension).
</domain>

<decisions>
- Sparkline: pure SVG polyline, no library. `viewBox="0 0 100 20"` for normalized scaling.
- DeltaChip: green ↑ when positive, red ↓ when negative, gray "—" when zero or null `previous_value`.
- Hero metric = `data.metrics[0]`; supporting = `data.metrics.slice(1, 5)`. Backend ordering is the stable contract.
- Backend: reuse existing daily-aggregation SELECT; add a date-range subquery for `previous_value` and a 7-row SELECT for `series`. Same 6-hour cache TTL.
</decisions>

<code_context>
- `SpotlightBackdrop variant="gradient-mesh" accent="teal"` exists from Phase 01.
- `SpotlightIcon name="chart"` exists from Phase 01.
- Existing `PlatformStatsCard.vue` already uses `camelize(m.key)` for i18n keys — keep.
- Backend struct: `PlatformMetric { Key string; Value int; Delta *int; ... }` — extend with `PreviousValue *int` + `Series []int`.
</code_context>

<specifics>
- DeltaChip computes % change in component (not backend) so resolver stays pure.
- Sparkline scales y based on min/max of series; defensive against single-value series.
- Faint grid pattern overlay on backdrop for chart context.
</specifics>

<deferred>
- Drill-down to per-day breakdown on click (out of v1.1 scope).
</deferred>
