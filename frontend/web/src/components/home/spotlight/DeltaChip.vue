<!--
  Workstream hero-spotlight — v1.1-polish Phase 08 (platform-stats-refactor).

  Small inline chip rendering the percent change of `current` vs `previous`
  (the prior 7-day-window total). The percentage is computed IN the component
  (not the backend) so the resolver stays a pure aggregation — the chip just
  needs the two raw numbers.

  Semantics:
    • previous null/undefined/<=0  → "—"  (gray)  — no baseline to compare
    • current > previous           → "↑"  (green)
    • current < previous           → "↓"  (red)
    • current === previous         → "—"  (gray)
-->
<template>
  <span
    class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-semibold"
    :class="cls"
  >
    {{ symbol }} {{ pctLabel }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ current: number; previous: number | null | undefined }>()

const delta = computed<number | null>(() =>
  props.previous && props.previous > 0
    ? (props.current - props.previous) / props.previous
    : null,
)

const symbol = computed<string>(() => {
  if (delta.value === null) return '—'
  if (delta.value > 0) return '↑'
  if (delta.value < 0) return '↓'
  return '—'
})

const pctLabel = computed<string>(() =>
  delta.value === null ? '' : `${Math.abs(Math.round(delta.value * 100))}%`,
)

const cls = computed<string>(() =>
  delta.value === null
    ? 'bg-gray-500/20 text-gray-300'
    : delta.value > 0
      ? 'bg-green-500/20 text-green-200'
      : delta.value < 0
        ? 'bg-red-500/20 text-red-200'
        : 'bg-gray-500/20 text-gray-300',
)
</script>
