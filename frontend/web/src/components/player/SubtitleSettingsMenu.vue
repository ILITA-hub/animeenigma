<template>
  <div ref="rootRef" class="relative">
    <button
      type="button"
      data-test="sub-timing-gear"
      class="inline-flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-white/10 hover:bg-white/15 text-white border border-white/10 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
      :disabled="!hasActiveSub"
      :title="$t('player.subtitleSettings.label')"
      :aria-label="$t('player.subtitleSettings.label')"
      @click="open = !open"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
      </svg>
      {{ $t('player.subtitleSettings.label') }}
    </button>

    <div
      v-if="open"
      data-test="sub-timing-popover"
      class="absolute right-0 z-30 mt-2 w-72 rounded-lg border border-white/10 bg-zinc-900/95 p-3 shadow-xl backdrop-blur"
    >
      <p class="text-white/80 text-sm font-medium mb-1">{{ $t('player.subtitleSettings.title') }}</p>
      <p class="text-white/40 text-xs mb-3">{{ $t('player.subtitleSettings.offsetHint') }}</p>
      <div class="flex items-center justify-between gap-2">
        <button type="button" data-test="nudge-minus-1" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm" @click="nudge(-1)">−1s</button>
        <button type="button" data-test="nudge-minus-01" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm" @click="nudge(-0.1)">−0.1s</button>
        <span data-test="readout" class="min-w-[3.5rem] text-center text-cyan-200 text-sm font-semibold tabular-nums">{{ readout }}</span>
        <button type="button" data-test="nudge-plus-01" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm" @click="nudge(0.1)">+0.1s</button>
        <button type="button" data-test="nudge-plus-1" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm" @click="nudge(1)">+1s</button>
      </div>
      <button type="button" data-test="reset" class="mt-3 text-xs text-white/50 hover:text-white/80 underline" @click="reset()">
        {{ $t('player.subtitleSettings.reset') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { onClickOutside, onKeyStroke } from '@vueuse/core'
import { useSubtitleTimingOffset } from '@/composables/useSubtitleTimingOffset'

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
