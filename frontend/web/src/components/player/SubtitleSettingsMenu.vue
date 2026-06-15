<template>
  <div ref="rootRef" class="relative">
    <!--
      Gear toggle: genuine labeled action button → swapped to the <Button> ghost
      primitive (07-02). Class overrides nudge ghost's bg-white/5→/10, hover→/15,
      rounded-lg→rounded-md, and pin the border static (hover:border-white/10) so
      the rendered look is byte-identical to the prior bespoke markup. Button
      forwards data-test / :disabled / :title / :aria-* / aria-haspopup / @click.
    -->
    <Button
      variant="ghost"
      size="sm"
      data-test="sub-timing-gear"
      class="px-4 py-2 rounded-md bg-white/10 hover:bg-white/15 hover:border-white/10 disabled:opacity-40"
      :disabled="!hasActiveSub"
      :title="t('player.subtitleSettings.label')"
      :aria-label="t('player.subtitleSettings.label')"
      :aria-expanded="open"
      aria-haspopup="true"
      @click="open = !open"
    >
      <Settings class="size-4" aria-hidden="true" />
      {{ t('player.subtitleSettings.label') }}
    </Button>

    <div
      v-if="open"
      data-test="sub-timing-popover"
      class="absolute right-0 z-30 mt-2 w-72 rounded-lg border border-white/10 bg-popover/95 p-3 shadow-xl backdrop-blur"
    >
      <p class="text-white/80 text-sm font-medium mb-1">{{ t('player.subtitleSettings.title') }}</p>
      <p class="text-white/40 text-xs mb-3">{{ t('player.subtitleSettings.offsetHint') }}</p>
      <!--
        Kept bespoke (07-02): compact player-chrome steppers (px-2 py-1, no border,
        bespoke cyan-400/60 focus ring). Below the Button size scale (smallest `sm`
        is px-3 py-1.5) and the `ghost` variant forces a border + ring-ring — a swap
        would need to neutralize bg/hover/rounded/border/focus-ring, i.e. the variant
        doesn't model these. Keeping avoids a visible diff.
      -->
      <div class="flex items-center justify-between gap-2">
        <button type="button" data-test="nudge-minus-1" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50" @click="nudge(-1)">−1s</button>
        <button type="button" data-test="nudge-minus-01" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50" @click="nudge(-0.1)">−0.1s</button>
        <span data-test="readout" class="min-w-[3.5rem] text-center text-cyan-200 text-sm font-semibold tabular-nums">{{ readout }}</span>
        <button type="button" data-test="nudge-plus-01" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50" @click="nudge(0.1)">+0.1s</button>
        <button type="button" data-test="nudge-plus-1" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50" @click="nudge(1)">+1s</button>
      </div>
      <!--
        Kept bespoke (07-02): bare underlined text link (no bg/border). The Button
        primitive has no text-only variant — `ghost`/`outline` add a filled/bordered
        box → visible diff. Documented governance-only keep.
      -->
      <button type="button" data-test="reset" class="mt-3 text-xs text-white/50 hover:text-white/80 underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50" @click="reset()">
        {{ t('player.subtitleSettings.reset') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { onClickOutside, onKeyStroke } from '@vueuse/core'
import { Settings } from 'lucide-vue-next'
import { useSubtitleTimingOffset } from '@/composables/useSubtitleTimingOffset'
import { Button } from '@/components/ui'

const { t } = useI18n()

defineProps<{ hasActiveSub: boolean }>()

const { offset, nudge, reset } = useSubtitleTimingOffset()

const open = ref(false)
const rootRef = ref<HTMLElement | null>(null)

onClickOutside(rootRef, () => { open.value = false })
onKeyStroke('Escape', () => { if (open.value) open.value = false })

// Signed, 1-decimal readout: '+1.5s', '0.0s', '-0.5s'. toFixed already emits the
// minus for negatives, so only positives need an explicit '+'.
const readout = computed(() => `${offset.value > 0 ? '+' : ''}${offset.value.toFixed(1)}s`)
</script>
