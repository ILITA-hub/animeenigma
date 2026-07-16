<template>
  <div
    data-test="provider-chip"
    :data-id="row.id"
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
      <!-- State-colored status dot (no per-provider identity hue) -->
      <span
        class="flex-shrink-0 w-[9px] h-[9px] rounded-full"
        :class="dotClass"
        aria-hidden="true"
      />

      <!-- Provider name + capability labels -->
      <span class="flex-1 min-w-0 flex flex-col gap-0.5">
        <span class="font-semibold truncate">{{ row.label }}</span>
        <span v-if="labels" class="flex items-center gap-[5px] flex-wrap">
          <template v-if="!labels.unverified">
            <span
              v-for="c in labels.categories"
              :key="c"
              data-test="cap-cat"
              class="text-[9px] font-semibold font-mono uppercase tracking-wide px-1 py-px rounded bg-white/[0.08] text-[var(--muted-foreground)]"
            >
              {{ c === 'sub' ? $t('player.sub') : $t('player.dub') }}<template v-if="c === 'sub' && labels.subDelivery"> · {{ labels.subDelivery === 'hard' ? $t('player.sources.subBurnedIn') : $t('player.sources.subSelectable') }}</template>
            </span>
          </template>
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
          <span
            v-for="lang in labels.verifiedDub"
            :key="'dub-' + lang"
            data-test="cap-verified-dub"
            class="text-[9px] font-semibold font-mono uppercase tracking-wide px-1 py-px rounded bg-white/[0.08] text-[var(--muted-foreground)]"
          >{{ $t('player.sources.verifiedDub', { lang: lang.toUpperCase() }) }}</span>
          <span
            v-for="lang in labels.verifiedHardsub"
            :key="'hardsub-' + lang"
            data-test="cap-verified-hardsub"
            class="text-[9px] font-semibold font-mono uppercase tracking-wide px-1 py-px rounded bg-white/[0.08] text-[var(--muted-foreground)]"
          >{{ $t('player.sources.verifiedHardsub', { lang: lang.toUpperCase() }) }}</span>
          <span
            v-if="labels.unverified"
            data-test="cap-unverified"
            class="text-[9px] font-semibold font-mono uppercase tracking-wide px-1 py-px rounded border border-border text-muted-foreground"
          >{{ $t('player.sources.unverified') }}</span>
        </span>
      </span>

      <!-- Selected check affordance -->
      <span
        v-if="isSelected"
        class="ml-auto flex-shrink-0 text-[var(--brand-cyan)]"
        :aria-label="$t('player.aePlayer.selected')"
      >
        <Check class="size-[14px]" aria-hidden="true" />
      </span>

      <!-- State badges (recovering / degraded / no_content) -->
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
      <span
        v-else-if="row.state === 'no_content'"
        data-test="cap-nocontent"
        class="ml-auto flex-shrink-0 text-[10px] font-semibold uppercase tracking-wide text-[var(--muted-foreground)] font-mono"
      >{{ $t('player.sources.noContent') }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Check } from 'lucide-vue-next'
import type { ProviderRow, ChipState } from '@/types/aePlayer'
import type { ProviderCap } from '@/types/capabilities'
import type { ProviderVerify } from '@/types/contentVerify'
import { deriveCapLabels } from '@/composables/aePlayer/capLabels'

const props = defineProps<{
  row: ProviderRow
  selected?: boolean
  cap?: ProviderCap
  /** Content-verify probe summary for this row (Task 13/14) — gates the
   *  asserted SUB/DUB category chips behind proven verdicts and drives the
   *  verified/unverified badges. */
  verify?: ProviderVerify | null
  best?: boolean
  /** When on, degraded/recovering (hacker-only) providers become selectable. */
  hackerMode?: boolean
  /** Top-3 fallback: force selectability for a hacker-only row when too few
   *  sources are active, so a fully-degraded fleet is never a dead end. */
  forced?: boolean
}>()

const emit = defineEmits<{
  (e: 'select'): void
}>()

const isSelected = computed(() => props.selected === true)
const labels = computed(() => deriveCapLabels(props.cap, props.verify ?? null))

// Selectability is the backend feed's `selectable`, gated by hacker mode for
// hacker-only rows (degraded/recovering). The feed is the single source of
// truth — the chip no longer recomputes state from a registry. AUTO-484.
const selectable = computed(
  () => props.row.selectable && (!props.row.hackerOnly || props.hackerMode === true || props.forced === true),
)

// State-colored status dot — semantic DS tokens, never per-provider hue.
const DOT: Record<ChipState, string> = {
  active: 'bg-[var(--brand-cyan)]',
  recovering: 'bg-lime-400',
  degraded: 'bg-warning',
  no_content: 'bg-[var(--muted-foreground)]',
}
const dotClass = computed(() => DOT[props.row.state])

function onClick() {
  if (selectable.value) {
    emit('select')
  }
}
</script>
