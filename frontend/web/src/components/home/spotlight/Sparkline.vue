<!--
  Workstream hero-spotlight — v1.1-polish Phase 08 (platform-stats-refactor).

  Pure-SVG sparkline (no chart-library dependency). Renders a single
  <polyline> normalized into a `0 0 100 20` viewBox; `preserveAspectRatio="none"`
  lets the parent stretch it to any width/height via Tailwind sizing.

  The y-axis is scaled by the series' own min/max (defensive against a
  single-value / flat series so we never divide by zero). `stroke="currentColor"`
  means the colour is inherited from the parent's text-* utility.

  The raw series is mirrored onto `data-points` (comma-joined) so tests and
  DOM inspectors can assert what was plotted without re-deriving geometry.
-->
<template>
  <svg
    :viewBox="`0 0 100 ${VIEW_H}`"
    preserveAspectRatio="none"
    :data-points="data.join(',')"
    aria-hidden="true"
  >
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
import { computed } from 'vue'

const VIEW_H = 20

const props = defineProps<{ data: number[] }>()

const points = computed<string>(() => {
  const min = Math.min(...props.data, 0)
  const max = Math.max(...props.data, 1)
  const range = Math.max(max - min, 1)
  return props.data
    .map((v, i) => {
      const x = (i / Math.max(props.data.length - 1, 1)) * 100
      const y = VIEW_H - ((v - min) / range) * VIEW_H
      return `${x},${y}`
    })
    .join(' ')
})
</script>
