<template>
  <div class="pl-wt-btn" data-test="wt-wip">
    <button
      disabled
      title="Watch Together — coming soon"
      class="pl-icon"
      aria-label="Watch Together — coming soon"
      aria-disabled="true"
      style="opacity: 0.5; cursor: not-allowed;"
    >
      <!-- Users icon -->
      <Users class="size-5" aria-hidden="true" />
    </button>
    <!-- WIP badge -->
    <span class="pl-wt-wip-badge" aria-hidden="true">WIP</span>
  </div>
</template>

<script setup lang="ts">
import { Users } from 'lucide-vue-next'
// WIP STUB — WatchTogetherButton is always disabled.
// Emits nothing. Opens nothing.
// data-test="wt-wip" is on the wrapper div for easy test selection.
</script>

<style scoped>
.pl-wt-btn {
  position: relative;
  display: inline-flex;
}

.pl-icon {
  width: 40px;
  height: 40px;
  display: grid;
  place-items: center;
  border-radius: var(--r-md);
  background: transparent;
  border: 0;
  color: #fff;
  transition: background 0.15s;
}

.pl-wt-wip-badge {
  position: absolute;
  top: 2px;
  right: 2px;
  min-width: 22px;
  height: 14px;
  padding: 0 3px;
  border-radius: 999px;
  background: var(--brand-pink, #ff4d8d);
  color: #fff;
  font-size: 8px;
  font-weight: 700;
  display: grid;
  place-items: center;
  font-family: var(--font-mono);
  letter-spacing: 0.04em;
  pointer-events: none;
}
</style>
