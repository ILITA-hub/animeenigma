<!--
  Workstream watch-together — InviteButton.vue.

  The watch-together entry point. Mounted next to the "Continue ep. N" CTA
  on the anime page (above-the-fold discovery surface).

  Click flow:
    click
      → if no translationId prop yet, fetch first available translation
        for the active player directly from the catalog (~100ms — does
        NOT mount the player; no preferences-resolution chain, no 12s
        timeout, no spurious red toast)
      → createRoom({anime_id, episode_id, player, translation_id})
      → router.push('/watch/room/${response.room_id}')
      → navigator.clipboard.writeText(response.invite_url)
      → toast.push('Invite link copied', 'success')

  Failure branches (all use the shared `useToast` queue rendered by
  `<Toaster />` in App.vue — same toast surface as the rest of the app):
    - translations fetch returns []  → error toast, stay on page.
    - createRoom rejects             → error toast, stay on page.
    - Clipboard absent / NotAllowed  → info toast that embeds the raw
                                       invite_url for manual copy. The
                                       router.push still fires — the URL
                                       in the address bar IS the invite.

  Loading state:
    - Local `loading` ref disables the button + shows an inline spinner
      while the click promise is in flight. Typical end-to-end time
      <500ms; the spinner exists so a slow-network click still gives
      pressed-state feedback.

  UI-SPEC contract (CLAUDE.md):
    - Tailwind utility classes only
    - Font weights: font-medium / font-semibold only (no font-semibold)
    - Padding: px-4 py-2 for the button itself
-->

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { UserPlus } from 'lucide-vue-next'
import { Spinner } from '@/components/ui'

import type { PlayerKind } from '@/api/watch-together'
import { useWatchTogetherLaunch } from '@/composables/watch-together/useWatchTogetherLaunch'

const props = defineProps<{
  /** Shikimori UUID for the anime currently displayed in WatchView. */
  animeId: string
  /** Episode NUMBER (as a string) the aeplayer room should open on. */
  episodeId: string
  /** Always 'aeplayer' — WatchTogether is aePlayer-only (Kodik retired from
   *  WT 2026-06-19; aePlayer plays Kodik content internally). Kept as a prop
   *  so the parent owns the value. */
  player: PlayerKind
  /** aePlayer combo token from the live player. May be empty: a token-less
   *  aeplayer room resolves a BEST source in-room and broadcasts it, so the
   *  discovery button works before the player has resolved a combo. */
  translationId: string
}>()

const { t } = useI18n()
const { launch } = useWatchTogetherLaunch()

const loading = ref(false)

async function onClick(): Promise<void> {
  if (loading.value) return
  loading.value = true
  try {
    // Shared create-room → navigate → copy-invite flow (also used by the
    // in-player WatchTogether button). Handles its own success/error toasts.
    // translationId may be empty — aeplayer rooms resolve a default in-room.
    await launch({
      animeId: props.animeId,
      episodeId: props.episodeId,
      player: props.player,
      translationId: props.translationId,
    })
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <button
    type="button"
    :disabled="loading"
    :aria-label="t('watch_together.invite_button_label')"
    data-testid="wt-invite-button"
    class="inline-flex items-center gap-2 px-5 py-3 rounded-lg bg-cyan-500/20 text-cyan-300 hover:bg-cyan-500/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 disabled:opacity-50 disabled:cursor-not-allowed font-semibold transition-colors"
    @click="onClick"
  >
    <Spinner v-if="loading" size="sm" tone="mono" aria-hidden="true" />
    <UserPlus v-else aria-hidden="true" class="size-4" />
    {{ t('watch_together.invite_button_label') }}
  </button>
</template>
