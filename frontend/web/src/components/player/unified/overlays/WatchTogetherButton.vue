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
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none"
        stroke="currentColor" stroke-width="2"
        stroke-linecap="round" stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
      </svg>
    </button>
    <!-- WIP badge -->
    <span class="pl-wt-wip-badge" aria-hidden="true">WIP</span>
  </div>
</template>

<script setup lang="ts">
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
