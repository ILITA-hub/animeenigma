<template>
  <div
    v-if="visible"
    class="pl-buffering"
    data-test="buffering-overlay"
    role="status"
    aria-label="Buffering"
  >
    <span class="pl-buffering-ring" aria-hidden="true" />
  </div>
</template>

<script setup lang="ts">
defineProps<{ visible: boolean }>()
</script>

<style scoped>
.pl-buffering {
  position: absolute;
  inset: 0;
  z-index: 4;
  display: grid;
  place-items: center;
  pointer-events: none;
}

/* Ring spinner — DS keep-list style (inline, brand-cyan top arc). */
.pl-buffering-ring {
  width: 54px;
  height: 54px;
  border-radius: 50%;
  border: 3px solid rgba(255, 255, 255, 0.2);
  border-top-color: var(--brand-cyan);
  filter: drop-shadow(0 0 8px rgba(0, 212, 255, 0.45));
  animation: pl-buffer-spin 0.8s linear infinite;
}

@keyframes pl-buffer-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
