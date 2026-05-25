---
phase: 08-platform-stats-refactor
plan: 08
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-PS-01, HSB-V11-PS-02, HSB-V11-PS-03, HSB-V11-PS-04]
human_verified: "pending eyeball confirmation on animeenigma.ru after merge + redeploy"
key_files:
  created:
    - frontend/web/src/components/home/spotlight/Sparkline.vue
    - frontend/web/src/components/home/spotlight/Sparkline.spec.ts
    - frontend/web/src/components/home/spotlight/DeltaChip.vue
    - frontend/web/src/components/home/spotlight/DeltaChip.spec.ts
  modified:
    - services/catalog/internal/service/spotlight/types.go
    - services/catalog/internal/service/spotlight/cards/platform_stats.go
    - services/catalog/internal/service/spotlight/cards/platform_stats_test.go
    - frontend/web/src/types/spotlight.ts
    - frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue
    - frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.spec.ts
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
commits:
  - b17bbb3 feat(spotlight/08) emit previous_value + series[7] on StatsMetric (HSB-V11-PS-01)
  - e6ef573 test(spotlight/08) assert Series[7] + non-nil PreviousValue (HSB-V11-PS-01)
  - abab304 feat(spotlight/08) add previous_value + series to PlatformMetric TS type (HSB-V11-PS-01)
  - 130dd3f feat(spotlight/08) add pure-SVG Sparkline component + spec (HSB-V11-PS-02)
  - 52ff8d9 feat(spotlight/08) add DeltaChip component + spec (HSB-V11-PS-02)
  - 97007c9 feat(spotlight/08) refactor PlatformStatsCard to hero-stat + micro-grid + sparkline (HSB-V11-PS-03, PS-04)
  - e8a1a53 chore(spotlight/08) merge worktree
  - caa9e56 fix(spotlight/08) add ja platformStats.vsPriorWeek for locale parity
metrics:
  metric_string: "UXΔ = +3 (Better) · CDI = 0.05 * 8 · MVQ = Kraken 80%/75%"
  completed_date: 2026-05-25
---

# Phase 08 Plan 08: PlatformStatsCard refactor Summary

Filled the N=1 dead space with a richer layout: oversized hero stat (left) +
2×2 micro-grid of supporting metrics (right) + 7-day pure-SVG sparkline +
delta chip vs the prior week. Required a small dialect-portable backend
extension to emit `previous_value` and `series[7]`.

## What shipped

### HSB-V11-PS-01 — backend `series` + `previous_value`
- Extended the real Go type `spotlight.StatsMetric` (the PLAN called it
  `PlatformMetric`; that name is the TS-side type) with `PreviousValue *int64
  json:"previous_value,omitempty"` and `Series []int json:"series,omitempty"`.
  The TS `PlatformMetric` in `types/spotlight.ts` mirrors the JSON tags
  (`previous_value?: number | null`, `series?: number[]`).
- A new `enrichMetric(ctx, m, "animes")` helper runs after the existing
  `Value` count, reusing `animes.created_at` + the 6-hour cache TTL:
  - **`series[7]`** — seven bounded `COUNT`s, one per day, bucket `i` (0..6)
    covering `[now-(7-i)·24h, now-(6-i)·24h)`, oldest-first. Series sum equals
    `Value` (asserted in the test).
  - **`previous_value`** — one `COUNT` over `[now-14d, now-7d)`.
- **Dialect portability decision:** used per-day bounded `COUNT`s rather than
  `date_trunc`/`DATE()` GROUP BY so the identical code runs on production
  Postgres AND the SQLite test harness. Per-query failure is non-fatal (logs
  WARN) — the metric still emits with the bare `Value`.
- `platform_stats_test.go` asserts `len(Series) == 7` and `PreviousValue != nil`.

### HSB-V11-PS-02 — Sparkline + DeltaChip
- `Sparkline.vue` — pure-SVG `<polyline>`, no chart library, `viewBox` y-scaled
  to series min/max, defensive against single-value series. `data-points`
  attribute mirrors `series.join(',')` for testability.
- `DeltaChip.vue` — computes % change in-component (resolver stays pure): green
  `↑` positive, red `↓` negative, gray `—` when zero or null `previous_value`.

### HSB-V11-PS-03 / PS-04 — card layout
- `PlatformStatsCard.vue` → `md:grid-cols-[2fr_3fr]` hero+grid split: hero stat
  at `text-7xl md:text-8xl tabular-nums`, faint grid-pattern overlay,
  teal gradient-mesh backdrop, DeltaChip + Sparkline under the hero number,
  2×2 `supportingMetrics` micro-grid. `heroMetric = metrics[0]`,
  `supportingMetrics = metrics.slice(1,5)` — backend ordering is the contract.

## Deviations (from executor, all sound)
1. Backend struct is `StatsMetric` (`Value int64`), not the PLAN's
   `PlatformMetric`; extended the real type, JSON tags exactly as specified.
2. Per-day bounded `COUNT`s instead of GROUP-BY aggregation (no existing
   aggregation to reuse; `date_trunc` would break SQLite tests).
3. Documentation comment moved into `<script>` (single-root `<article>` rule —
   a leading template comment makes Vue treat the SFC as multi-root).
4. Added `spotlight.platformStats.vsPriorWeek` — PLAN referenced the key but it
   didn't exist. Executor added en+ru; orchestrator added **ja** (`前週比`) for
   3-locale parity (commit caa9e56) per the i18n-smoke rule.

## Verification
- Backend: `go test ./internal/service/spotlight/... -count=1` — green (race clean).
- Frontend: 302/302 spotlight Vitest (17 files); Sparkline 7/7, DeltaChip 7/7,
  PlatformStatsCard 11/11; `tsc --noEmit` clean.
- Deployed via `make redeploy-catalog` + `make redeploy-web`.

## Metrics
`UXΔ = +3 (Better) · CDI = 0.05 * 8 · MVQ = Kraken 80%/75%`

## Deferred
- Drill-down to per-day breakdown on click (v1.2).
