<template>
  <button
    v-if="visible"
    class="pl-bigplay"
    aria-label="Play"
    data-test="bigplay"
    @click="emit('play')"
  >
    <!-- Play triangle -->
    <svg width="36" height="36" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M5 3l14 9-14 9V3z" />
    </svg>
  </button>
</template>

<script setup lang="ts">
defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  (e: 'play'): void
}>()
</script>

<style scoped>
.pl-bigplay {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  z-index: 5;
  width: 88px;
  height: 88px;
  border-radius: 50%;
  background: rgba(0, 212, 255, 0.22);
  backdrop-filter: blur(8px);
  border: 1px solid var(--accent-line);
  color: #fff;
  display: grid;
  place-items: center;
  padding-left: 5px;
  box-shadow: var(--glow-cyan);
  transition: transform 0.2s;
  cursor: pointer;
}

.pl-bigplay:hover {
  transform: translate(-50%, -50%) scale(1.08);
}
</style>
