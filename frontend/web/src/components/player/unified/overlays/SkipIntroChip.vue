<template>
  <button
    v-if="visible"
    class="pl-skip"
    data-test="skip-intro"
    @click="emit('skip')"
  >
    <!-- Forward-skip icon -->
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none"
      stroke="currentColor" stroke-width="2"
      stroke-linecap="round" stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M5.4 19.5L10.95 12 5.4 4.5h3.6l5.55 7.5L9 19.5z" fill="currentColor" stroke="none" />
      <path d="M12.6 19.5L18.15 12 12.6 4.5h3.6l5.55 7.5-5.55 7.5z" fill="currentColor" stroke="none" />
    </svg>
    Skip Intro
  </button>
</template>

<script setup lang="ts">
defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  (e: 'skip'): void
}>()
</script>

<style scoped>
.pl-skip {
  position: absolute;
  right: 22px;
  bottom: 92px;
  z-index: 6;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 11px 18px;
  border-radius: var(--r-md);
  background: rgba(8, 8, 15, 0.75);
  border: 1px solid rgba(255, 255, 255, 0.35);
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  backdrop-filter: blur(8px);
  cursor: pointer;
  transition: background 0.15s, color 0.15s, border-color 0.15s;
}

.pl-skip:hover {
  background: #fff;
  color: var(--color-base, #08080f);
  border-color: #fff;
}
</style>
