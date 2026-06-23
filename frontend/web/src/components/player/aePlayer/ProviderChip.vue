<template>
  <div
    data-test="provider-chip"
    :data-id="row.def.id"
    :class="['w-full', isSelected ? 'is-selected' : '']"
  >
    <button
      :disabled="!selectable"
      :title="row.reason"
      :class="[
        'relative inline-flex items-center gap-2 w-full px-2.5 py-[9px]',
        'rounded-[var(--r-md)] border text-sm text-left transition-all duration-150',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-cyan)]',
        isSelected
          ? 'is-selected bg-[var(--cyan-a08)] border-[var(--accent-line)] text-white'
          : selectable
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

      <!-- Provider name + capability labels -->
      <span class="flex-1 min-w-0 flex flex-col gap-0.5">
        <span class="font-semibold truncate">{{ row.def.name }}</span>
        <!-- Hacker-mode plain-language description for every provider -->
        <span
          v-if="hackerMode && row.def.blurb"
          data-test="provider-blurb"
          class="text-[10px] font-medium text-[var(--muted-foreground)] leading-snug"
        >{{ row.def.blurb }}</span>
        <span v-if="labels" class="flex items-center gap-[5px] flex-wrap">
          <span
            v-for="c in labels.categories"
            :key="c"
            data-test="cap-cat"
            class="text-[9px] font-semibold font-mono uppercase tracking-wide px-1 py-px rounded bg-white/[0.08] text-[var(--muted-foreground)]"
          >
            {{ c === 'sub' ? $t('player.sub') : c === 'dub' ? $t('player.dub') : $t('player.sources.raw') }}<template v-if="c === 'sub' && labels.subDelivery"> · {{ labels.subDelivery === 'hard' ? $t('player.sources.subBurnedIn') : $t('player.sources.subSelectable') }}</template>
          </span>
          <span
            v-if="labels.quality"
            data-test="cap-quality"
            class="text-[9px] font-semibold font-mono text-[var(--muted-foreground)]"
          >{{ labels.quality }}</span>
          <span
            v-if="best"
            data-test="cap-best"
            class="text-[9px] font-semibold font-mono uppercase tracking-wide text-[var(--brand-cyan)]"
          >{{ $t('player.sources.best') }}</span>
        </span>
      </span>

      <!-- Selected check affordance -->
      <span
        v-if="isSelected"
        class="ml-auto flex-shrink-0 text-[var(--brand-cyan)]"
        aria-label="Selected"
      >
        <Check class="size-[14px]" aria-hidden="true" />
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
      <span
        v-else-if="row.state === 'recovering'"
        data-test="cap-recovering"
        class="ml-auto flex-shrink-0 text-[10px] font-semibold uppercase tracking-wide text-lime-400 font-mono"
      >{{ $t('player.sources.recovering') }}</span>
      <span
        v-else-if="row.state === 'degraded'"
        data-test="cap-degraded"
        class="ml-auto flex-shrink-0 text-[10px] font-semibold uppercase tracking-wide text-warning font-mono"
      >{{ $t('player.sources.degraded') }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Check } from 'lucide-vue-next'
import type { ProviderRow } from '@/types/aePlayer'
import type { ProviderCap } from '@/types/capabilities'
import { deriveCapLabels } from '@/composables/aePlayer/capLabels'

const props = defineProps<{
  row: ProviderRow
  selected?: boolean
  cap?: ProviderCap
  best?: boolean
  /** When on, degraded providers become manually selectable (AUTO-484). */
  hackerMode?: boolean
}>()

const emit = defineEmits<{
  (e: 'select'): void
}>()

const isSelected = computed(() => props.selected === true)
const labels = computed(() => deriveCapLabels(props.cap))

// Active is always selectable; degraded and recovering providers are selectable
// ONLY in hacker mode (never auto-selected — that path filters on state 'active').
// AUTO-484. 'recovering' ranks above 'degraded' but shares the same hacker-mode gate.
const selectable = computed(
  () =>
    props.row.state === 'active' ||
    ((props.row.state === 'degraded' || props.row.state === 'recovering') && props.hackerMode === true),
)

function onClick() {
  if (selectable.value) {
    emit('select')
  }
}
</script>
