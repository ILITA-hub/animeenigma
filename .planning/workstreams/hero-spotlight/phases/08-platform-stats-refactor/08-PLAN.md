---
phase: 08-platform-stats-refactor
plan: 08
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-PS-01, HSB-V11-PS-02, HSB-V11-PS-03, HSB-V11-PS-04]
blocked_by: [01]
status: ready
---

# Plan 08: PlatformStatsCard refactor — hero stat + micro-grid + sparkline

## Goal

Fill the N=1 dead space with a richer layout: hero stat (left, oversized
number) + 2×2 micro-grid of supporting stats (right) + 7-day sparkline +
delta chip. Requires a small backend extension to emit `previous_value`
and `series[7]`.

## Tasks

### Task 1 — Backend extension

`services/catalog/internal/service/spotlight/cards/platform_stats.go`:

Extend `PlatformMetric` struct:

```go
type PlatformMetric struct {
  Key           string `json:"key"`
  Value         int    `json:"value"`
  PreviousValue *int   `json:"previous_value,omitempty"`  // NEW: prior 7-day window
  Series        []int  `json:"series,omitempty"`          // NEW: 7 daily samples (oldest first)
  Delta         *int   `json:"delta,omitempty"`           // existing
}
```

Compute in resolver via the same daily-aggregation SELECT, just with a
date-range subquery. Reuses existing 6-hour cache TTL.

Add a backend test asserting `len(metric.Series) == 7` and
`metric.PreviousValue != nil` for all emitted metrics.

### Task 2 — Frontend layout

```vue
<article class="relative w-full h-full overflow-hidden">
  <SpotlightBackdrop variant="gradient-mesh" accent="teal" />
  <!-- Faint grid pattern overlay -->
  <div
    aria-hidden="true"
    class="absolute inset-0 opacity-5"
    style="background-image: repeating-linear-gradient(0deg, transparent, transparent 39px, rgba(255,255,255,.5) 40px), repeating-linear-gradient(90deg, transparent, transparent 39px, rgba(255,255,255,.5) 40px);"
  />
  <div class="relative z-10 w-full h-full grid md:grid-cols-[2fr_3fr] gap-6 p-4 md:p-6 lg:p-8">
    <!-- Hero stat (left) -->
    <div class="flex flex-col justify-center min-w-0">
      <div class="flex items-center gap-2 mb-2">
        <SpotlightIcon name="chart" class="w-5 h-5 text-teal-300" />
        <h3 class="text-base font-semibold text-white">
          {{ t('spotlight.platformStats.title') }}
        </h3>
      </div>
      <p class="text-xs font-medium text-teal-200 uppercase tracking-wider mb-2">
        {{ t(`spotlight.platformStats.${camelize(heroMetric.key)}`) }}
      </p>
      <p class="text-7xl md:text-8xl font-semibold text-white tabular-nums leading-none">
        {{ heroMetric.value.toLocaleString(localeStr) }}
      </p>
      <!-- Delta chip -->
      <div class="mt-3 flex items-center gap-2">
        <DeltaChip :current="heroMetric.value" :previous="heroMetric.previous_value" />
        <span class="text-xs text-gray-400">{{ t('spotlight.platformStats.vsPriorWeek') }}</span>
      </div>
      <!-- Sparkline -->
      <Sparkline
        v-if="heroMetric.series && heroMetric.series.length >= 2"
        :data="heroMetric.series"
        class="mt-3 h-10 w-full text-teal-300"
      />
    </div>

    <!-- Micro-grid (right, 2×2) -->
    <ul class="grid grid-cols-2 gap-3 content-center min-w-0">
      <li
        v-for="m in supportingMetrics"
        :key="m.key"
        class="flex flex-col p-3 rounded-lg bg-white/5 backdrop-blur-sm"
      >
        <p class="text-[10px] font-medium text-gray-400 uppercase tracking-wider truncate">
          {{ t(`spotlight.platformStats.${camelize(m.key)}`) }}
        </p>
        <p class="mt-1 text-xl font-semibold text-white tabular-nums">
          {{ m.value.toLocaleString(localeStr) }}
        </p>
      </li>
    </ul>
  </div>
</article>
```

### Task 3 — Sparkline + DeltaChip components

`Sparkline.vue` — pure-SVG, no chart-library dep:

```vue
<template>
  <svg :viewBox="`0 0 100 ${20}`" preserveAspectRatio="none" :data-points="data.join(',')">
    <polyline
      :points="points"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linejoin="round"
      stroke-linecap="round"
    />
  </svg>
</template>
<script setup lang="ts">
const props = defineProps<{ data: number[] }>()
const points = computed(() => {
  const min = Math.min(...props.data, 0)
  const max = Math.max(...props.data, 1)
  const range = Math.max(max - min, 1)
  return props.data.map((v, i) => {
    const x = (i / Math.max(props.data.length - 1, 1)) * 100
    const y = 20 - ((v - min) / range) * 20
    return `${x},${y}`
  }).join(' ')
})
</script>
```

`DeltaChip.vue`:

```vue
<template>
  <span
    class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-semibold"
    :class="cls"
  >
    {{ symbol }} {{ pctLabel }}
  </span>
</template>
<script setup lang="ts">
const props = defineProps<{ current: number; previous: number | null | undefined }>()
const delta = computed(() => (props.previous && props.previous > 0)
  ? (props.current - props.previous) / props.previous
  : null)
const symbol = computed(() => {
  if (delta.value === null) return '—'
  if (delta.value > 0) return '↑'
  if (delta.value < 0) return '↓'
  return '—'
})
const pctLabel = computed(() => delta.value === null
  ? ''
  : `${Math.abs(Math.round(delta.value * 100))}%`)
const cls = computed(() => delta.value === null
  ? 'bg-gray-500/20 text-gray-300'
  : delta.value > 0
    ? 'bg-green-500/20 text-green-200'
    : delta.value < 0
      ? 'bg-red-500/20 text-red-200'
      : 'bg-gray-500/20 text-gray-300')
</script>
```

### Task 4 — Hero vs supporting selection

```ts
const heroMetric = computed(() => data.metrics[0])
const supportingMetrics = computed(() => data.metrics.slice(1, 5))
```

So a 5-metric response gives 1 hero + 4 supporting. Backend's existing
ordering is preserved as a stable contract.

### Task 5 — Spec updates

`PlatformStatsCard.spec.ts`:

- Hero stat renders at `text-7xl` / `text-8xl`.
- Sparkline `<svg>` has `data-points` matching `series.join(',')`.
- DeltaChip renders `↑` when `value > previous_value`, `↓` when less.
- Supporting metrics render exactly 4 when 5 metrics provided.

Backend test:

`platform_stats_test.go` asserts `len(Series) == 7` and `PreviousValue != nil`.

## Verification

- Backend: `cd services/catalog && go test ./internal/service/spotlight/cards/... -count=1 -race` — green.
- Frontend: `bunx vitest run src/components/home/spotlight/cards/PlatformStatsCard.spec.ts src/components/home/spotlight/Sparkline.spec.ts src/components/home/spotlight/DeltaChip.spec.ts` — green.
- `bunx tsc --noEmit` — clean.
- `bunx playwright test spotlight-full` — green.
- Visual smoke: cycle to PlatformStats, confirm hero stat + sparkline + delta chip + 2×2 micro-grid all render.

## Metrics

`UXΔ = +3 (Better) · CDI = 0.05 * 8 · MVQ = Kraken 80%/75%`
