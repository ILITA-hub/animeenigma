<template>
  <button
    v-if="visible"
    class="pl-nextchip"
    data-test="next-episode-chip"
    @click="emit('next')"
  >
    <!-- Skip-to-next icon -->
    <SkipForward class="size-4" aria-hidden="true" />
    {{ label }}
  </button>
</template>

<script setup lang="ts">
import { SkipForward } from 'lucide-vue-next'

defineProps<{
  visible: boolean
  // Parent passes a translated label ("Next episode").
  label: string
}>()

const emit = defineEmits<{
  (e: 'next'): void
}>()
</script>

<style scoped>
/* Mirrors SkipIntroChip's `.pl-skip` look, stacked one row above it
   (skip chip sits at bottom: 92px) so the two never overlap when both show
   during the ending. */
.pl-nextchip {
  position: absolute;
  right: 22px;
  bottom: 140px;
  z-index: 6;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 11px 18px;
  border-radius: var(--r-md);
  background: var(--scrim-bg-strong);
  border: 1px solid var(--white-a30);
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  backdrop-filter: blur(8px);
  cursor: pointer;
  transition: background 0.15s, color 0.15s, border-color 0.15s;
}

.pl-nextchip:hover {
  background: #fff;
  color: var(--color-base, #08080f);
  border-color: #fff;
}
</style>
