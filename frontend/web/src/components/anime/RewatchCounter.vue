<script setup lang="ts">
// Shared, low-visual-weight rewatch counter. Reused on the anime page and My
// List rows. Pure prop-driven: `count` in, `update:count` out. Read-only by
// default (a muted ↻ N ghost; hidden entirely at 0 so never-rewatched anime
// stay uncluttered). When `editable`, a subtle − N + stepper appears; − never
// goes below 0. Design 2026-06-05.
import { useI18n } from 'vue-i18n'

const props = defineProps<{ count: number; editable?: boolean }>()
const emit = defineEmits<{ (e: 'update:count', value: number): void }>()

const { t } = useI18n()

function inc() {
  emit('update:count', props.count + 1)
}
function dec() {
  if (props.count > 0) emit('update:count', props.count - 1)
}
</script>

<template>
  <span
    v-if="editable || count > 0"
    data-testid="rewatch-counter"
    class="inline-flex items-center gap-0.5 text-xs text-muted-foreground"
    :title="t('anime.rewatchCount', { n: count })"
  >
    <span class="opacity-70" aria-hidden="true">↻</span>
    <button
      v-if="editable"
      type="button"
      data-testid="rewatch-dec"
      class="px-1 leading-none transition-colors hover:text-foreground disabled:opacity-40"
      :disabled="count <= 0"
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
