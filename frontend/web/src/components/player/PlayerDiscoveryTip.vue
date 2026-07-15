<template>
  <aside
    class="non-player-content mt-3 flex items-start gap-3 rounded-lg border border-primary/20 bg-primary/10 px-3 py-2.5"
    :aria-label="$t('player.discoveryTips.label')"
    data-test="player-discovery-tip"
  >
    <Sparkles class="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" />
    <div class="min-w-0 flex-1">
      <p class="mb-0.5 text-xs font-medium uppercase tracking-wide text-primary">
        {{ $t('player.discoveryTips.label') }}
      </p>
      <p class="text-sm leading-relaxed text-foreground/80" aria-live="polite" data-test="tip-copy">
        {{ $t(currentTipKey) }}
      </p>
    </div>
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      radius="full"
      class="shrink-0 text-primary"
      :aria-label="$t('player.discoveryTips.another')"
      :title="$t('player.discoveryTips.another')"
      data-test="shuffle-tip"
      @click="showAnother"
    >
      <Shuffle class="size-4" aria-hidden="true" />
    </Button>
  </aside>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Shuffle, Sparkles } from 'lucide-vue-next'
import { Button } from '@/components/ui'
import { PLAYER_DISCOVERY_TIP_KEYS, pickDiscoveryTipIndex } from './playerDiscoveryTips'

const currentIndex = ref(pickDiscoveryTipIndex(PLAYER_DISCOVERY_TIP_KEYS.length))
const currentTipKey = computed(() => PLAYER_DISCOVERY_TIP_KEYS[currentIndex.value])

function showAnother() {
  currentIndex.value = pickDiscoveryTipIndex(
    PLAYER_DISCOVERY_TIP_KEYS.length,
    currentIndex.value,
  )
}
</script>
