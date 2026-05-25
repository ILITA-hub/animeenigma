<!--
  Workstream watch-together — Phase 2 (frontend-shell) Plan 02.5 Task 2.

  ReactionBurstOverlay.vue — floating emoji bursts that rise from the
  bottom of the player and fade out over 3s. Mounted as a child of the
  player wrapper so it overlays the video.

  Design notes:
    - `pointer-events: none` on the wrapper is mandatory: this overlay
      sits on top of the video and must never block clicks (play/pause
      tap, scrubber drag, server-side menus). WT-NF-05.
    - Animation is CSS-only (no JS spring lib): a single `@keyframes`
      block defined in `<style scoped>`. The plan explicitly forbids
      adding `motion`/`@vueuse/motion`-style dependencies.
    - `left:` is randomized once per reaction id and cached in a
      module-scoped Map. Re-renders for the same id return the cached
      value so the emoji doesn't jitter horizontally if the parent
      mutates the array (e.g. when the 1Hz prune loop runs).
    - The composable prunes reactions after 5s; the 3s animation fades
      to opacity 0 before pruning, so visually nothing pops.
-->

<script setup lang="ts">
import type { ReactionEvent } from '@/composables/useWatchTogetherRoom'

defineProps<{
  /**
   * Direct binding of the composable's `reactions.value`. The composable
   * owns the lifetime; this overlay is a pure renderer.
   */
  reactions: ReactionEvent[]
}>()

/**
 * Per-reaction-id horizontal placement cache. Module-scoped (not a
 * component-local `ref`) so that even with `<KeepAlive>` or fast
 * remount cycles the same id keeps the same horizontal coordinate.
 *
 * Bounded growth: in practice the ring buffer caps at ~20 entries with
 * 5s lifetime; even pathological cases will only accumulate a few
 * hundred ids per session. No GC needed.
 */
const positions = new Map<number, number>()

/**
 * Returns the cached `left` percentage for a reaction id, or rolls a
 * new random one in [5, 95] (avoid the absolute edges so the emoji
 * doesn't half-clip on small viewports).
 */
function leftFor(id: number): number {
  const cached = positions.get(id)
  if (cached !== undefined) {
    return cached
  }
  const rolled = Math.random() * 90 + 5
  positions.set(id, rolled)
  return rolled
}
</script>

<template>
  <div class="pointer-events-none absolute inset-0 overflow-hidden">
    <span
      v-for="r in reactions"
      :key="r.id"
      class="burst absolute text-4xl select-none"
      :style="{ left: leftFor(r.id) + '%' }"
    >
      {{ r.emoji }}
    </span>
  </div>
</template>

<style scoped>
.burst {
  bottom: 10%;
  animation: burst-rise 3s ease-out forwards;
}

@keyframes burst-rise {
  from {
    opacity: 1;
    transform: translateY(0);
  }
  to {
    opacity: 0;
    transform: translateY(-200px);
  }
}
</style>
