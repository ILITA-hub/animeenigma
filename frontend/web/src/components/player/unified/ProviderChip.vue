<template>
  <div
    data-test="provider-chip"
    :data-id="row.def.id"
    :class="['w-full', isSelected ? 'is-selected' : '']"
  >
    <button
      :disabled="row.state !== 'active'"
      :title="row.reason"
      :class="[
        'relative inline-flex items-center gap-2 w-full px-[10px] py-[9px]',
        'rounded-[var(--r-md)] border text-sm text-left transition-all duration-150',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-cyan)]',
        isSelected
          ? 'is-selected bg-[rgba(0,212,255,0.10)] border-[var(--accent-line)] text-white'
          : row.state === 'active'
            ? 'bg-white/[0.04] border-transparent text-[var(--ink-2)] hover:bg-white/[0.09] hover:text-white'
            : 'bg-white/[0.04] border-transparent text-[var(--ink-2)] opacity-40 cursor-not-allowed',
      ]"
      @click="onClick"
    >
      <!-- Identity-hue dot -->
      <span
        class="flex-shrink-0 w-[9px] h-[9px] rounded-full"
        :style="{ background: row.def.hue, boxShadow: `0 0 8px ${row.def.hue}` }"
        aria-hidden="true"
      />

      <!-- Provider name -->
      <span class="flex-1 font-semibold truncate">{{ row.def.name }}</span>

      <!-- Selected check affordance -->
      <span
        v-if="isSelected"
        class="ml-auto flex-shrink-0 text-[var(--brand-cyan)]"
        aria-label="Selected"
      >
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
          <path d="M2.5 7L5.5 10L11.5 4" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </span>

      <!-- State badge for wip/down -->
      <span
        v-if="row.state === 'wip'"
        class="ml-auto flex-shrink-0 text-[10px] font-semibold uppercase tracking-wide text-[var(--muted-foreground)] font-mono"
      >WIP</span>
      <span
        v-else-if="row.state === 'down'"
        class="ml-auto flex-shrink-0 text-[10px] font-semibold uppercase tracking-wide text-[var(--destructive)] font-mono"
      >DOWN</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ProviderRow } from '@/types/unifiedPlayer'

const props = defineProps<{
  row: ProviderRow
  selected?: boolean
}>()

const emit = defineEmits<{
  (e: 'select'): void
}>()

const isSelected = computed(() => props.selected === true)

function onClick() {
  if (props.row.state === 'active') {
    emit('select')
  }
}
</script>
