<template>
  <button
    v-if="visible"
    class="pl-skip"
    data-test="skip-intro"
    @click="emit('skip')"
  >
    <!-- Forward-skip icon -->
    <FastForward class="size-4" aria-hidden="true" />
    {{ label }}
  </button>
</template>

<script setup lang="ts">
import { FastForward } from 'lucide-vue-next'

withDefaults(
  defineProps<{
    visible: boolean
    label?: string
  }>(),
  { label: 'Skip Intro' },
)

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
