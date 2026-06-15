<!--
  Workstream watch-together — Phase 2 (frontend-shell) Plan 02.5 Task 1.

  ReactionPalette.vue — the "reactions" half of the sidebar. Renders the
  24-emoji REACTION_WHITELIST as a clickable chip grid. Clicking a chip
  fires the `sendReaction` prop (wired by the parent to the composable's
  `sendReaction(emoji)` method).

  Why a 200ms client-side throttle even though the server already
  rate-limits at 5/sec: accidental double-clicks on touch devices fire
  twice within ~50ms, which would otherwise look like the click "did
  nothing the second time" (the server silently drops the second one).
  The local throttle gives the user a visual cooldown so they understand
  the chip is intentionally inert for a beat.

  Project rule: only font-medium / font-semibold weights. Tailwind
  utility-only.
-->

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { REACTION_WHITELIST } from '@/types/watch-together'

const props = defineProps<{
  /** Wired by the parent (RoomSidebar) to `useWatchTogetherRoom().sendReaction`. */
  sendReaction: (emoji: string) => void
}>()

const { t } = useI18n()

/**
 * Client-side throttle anchor. `Date.now()` is used (not performance.now)
 * because the test stubs `vi.useFakeTimers()` which faketimes Date by
 * default but not performance.now.
 */
const lastClickAt = ref(0)
const THROTTLE_MS = 200

function onClick(emoji: string) {
  const now = Date.now()
  if (now - lastClickAt.value < THROTTLE_MS) {
    return
  }
  lastClickAt.value = now
  props.sendReaction(emoji)
}
</script>

<template>
  <section
    class="p-4 md:p-6"
    :aria-label="t('watch_together.reaction_palette_aria')"
  >
    <h3 class="font-semibold text-foreground/70 text-sm mb-3">
      {{ t('watch_together.reaction_palette_title') }}
    </h3>
    <div class="grid grid-cols-6 md:grid-cols-8 gap-2">
      <button
        v-for="emoji in REACTION_WHITELIST"
        :key="emoji"
        type="button"
        class="text-2xl p-2 rounded hover:bg-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 transition select-none"
        :aria-label="emoji"
        @click="onClick(emoji)"
      >
        {{ emoji }}
      </button>
    </div>
  </section>
</template>
