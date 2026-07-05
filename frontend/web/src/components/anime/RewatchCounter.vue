<script setup lang="ts">
// Shared, low-visual-weight rewatch counter (My List rows; the anime page
// hero variant moved into the status dropdown 2026-07-05 — an unlabeled
// ↻ − 0 + confused new users). Pure prop-driven: `count` in, `update:count`
// out. Hidden entirely at 0 — even when `editable` — so never-rewatched anime
// stay uncluttered; the first manual bump happens via the anime-page status
// menu. When `editable` and count > 0, a subtle − N + stepper appears.
// Design 2026-06-05.
import { useI18n } from 'vue-i18n'

const props = defineProps<{ count: number; editable?: boolean }>()
const emit = defineEmits<{ (e: 'update:count', value: number): void }>()

const { t } = useI18n()

function inc() {
  emit('update:count', props.count + 1)
}
// The root v-if="count > 0" already guarantees count ≥ 1 here.
function dec() {
  emit('update:count', props.count - 1)
}
</script>

<template>
  <span
    v-if="count > 0"
    data-testid="rewatch-counter"
    class="inline-flex items-center gap-0.5 text-xs text-muted-foreground"
    :title="t('anime.rewatchCount', { n: count })"
  >
    <span class="opacity-70" aria-hidden="true">↻</span>
    <button
      v-if="editable"
      type="button"
      data-testid="rewatch-dec"
      class="px-1 leading-none transition-colors hover:text-foreground"
      :aria-label="t('anime.rewatchDec')"
      @click="dec"
    >
      −
    </button>
    <span class="tabular-nums">{{ count }}</span>
    <button
      v-if="editable"
      type="button"
      data-testid="rewatch-inc"
      class="px-1 leading-none transition-colors hover:text-foreground"
      :aria-label="t('anime.rewatchInc')"
      @click="inc"
    >
      +
    </button>
  </span>
</template>
