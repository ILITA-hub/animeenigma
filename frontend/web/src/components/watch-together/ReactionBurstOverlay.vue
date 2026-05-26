<!--
  Workstream watch-together — Phase 5 Plan 05.3 (polish over Phase 2 02.5).

  ReactionBurstOverlay.vue — floating emoji bursts that rise from the
  bottom of the player with a scale+rise+wiggle+fade animation over
  2.5s. Mounted as a child of the player wrapper so it overlays the
  video.

  Design notes:
    - `pointer-events: none` on the wrapper is mandatory: this overlay
      sits on top of the video and must never block clicks (WT-NF-05).
    - Animation is CSS-only (no JS spring lib): @keyframes in <style scoped>.
    - **Phase 5 polish:** MAX_VISIBLE=8 cap with FIFO drop — when a 9th
      reaction arrives, the oldest is sliced off (prevents pile-up at
      rapid burst rates).
    - **Phase 5 polish:** 8-column stratified horizontal placement —
      replaces Phase 2's pure-random `[5,95]%` which clumped at center.
    - **Phase 5 polish:** Animation upgraded to scale 0.8→1.2→1.0 +
      translate-y rise + gentle horizontal wiggle + fade, total 2.5s.
    - The composable's buffer is larger (~20) with 5s lifetime; this
      overlay's 2.5s animation fades to 0 before pruning so nothing pops.
-->

<script setup lang="ts">
import type { ReactionEvent } from '@/composables/useWatchTogetherRoom'

defineProps<{
  reactions: ReactionEvent[]
}>()

/**
 * Maximum concurrent on-screen bursts. When a 9th reaction arrives,
 * the oldest visible one is dropped (FIFO via .slice(-MAX_VISIBLE)).
 */
const MAX_VISIBLE = 8

/**
 * Eight discrete horizontal lanes (% from left). Deliberately non-uniform
 * with a slight gap mid-screen so the placement reads as "natural" rather
 * than a strict grid.
 */
const COLUMNS = [10, 20, 30, 45, 55, 70, 80, 90]

/**
 * Per-reaction-id column cache. Module-scoped so re-mounts and re-renders
 * keep the same column for the same id. Bounded by the composable's
 * ~20-entry buffer + 5s lifetime — never grows past a few hundred entries
 * per session.
 */
const columns = new Map<number, number>()

/**
 * Returns the cached column% for a reaction id, picking a new column
 * round-robin on the first sighting. Round-robin (id % COLUMNS.length)
 * spreads reactions evenly across all 8 lanes regardless of arrival rate.
 */
function columnFor(id: number): number {
  const cached = columns.get(id)
  if (cached !== undefined) {
    return cached
  }
  const col = COLUMNS[id % COLUMNS.length]
  columns.set(id, col)
  // Defensive bound: prune oldest entries if cache grows unexpectedly.
  if (columns.size > 256) {
    const keysIter = columns.keys()
    for (let i = 0; i < 128; i++) {
      const k = keysIter.next().value
      if (k === undefined) break
      columns.delete(k)
    }
  }
  return col
}
</script>

<template>
  <div class="pointer-events-none absolute inset-0 overflow-hidden">
    <span
      v-for="r in reactions.slice(-MAX_VISIBLE)"
      :key="r.id"
      class="burst absolute text-4xl select-none"
      :style="{ left: columnFor(r.id) + '%' }"
    >
      {{ r.emoji }}
    </span>
  </div>
</template>

<style scoped>
.burst {
  bottom: 10%;
  animation: burst-rise 2.5s ease-out forwards;
}

@keyframes burst-rise {
  0% {
    opacity: 0;
    transform: translate(0, 0) scale(0.8);
  }
  15% {
    opacity: 1;
    transform: translate(-4px, -20px) scale(1.2);
  }
  30% {
    transform: translate(4px, -50px) scale(1.0);
  }
  60% {
    opacity: 1;
    transform: translate(-2px, -120px) scale(1.0);
  }
  100% {
    opacity: 0;
    transform: translate(0, -200px) scale(1.0);
  }
}
</style>
