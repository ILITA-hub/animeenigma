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
import { kodikApi } from '@/api/client'
import { useToast } from '@/composables/useToast'
import { useWatchTogetherLaunch } from '@/composables/watch-together/useWatchTogetherLaunch'

const props = defineProps<{
  /** Shikimori UUID for the anime currently displayed in WatchView. */
  animeId: string
  /** Provider-specific episode identifier (string — varies by player). */
  episodeId: string
  /** Active player kind (drives WatchTogetherView's player mount). */
  player: PlayerKind
  /** Active translation/dub identifier — may be empty for the discovery
   *  mount on the anime detail page. When empty, the button fetches the
   *  first available translation from the catalog before creating the
   *  room (sub-second; no player-mount chain). */
  translationId: string
}>()

const { t } = useI18n()
const toast = useToast()
const { launch } = useWatchTogetherLaunch()

const loading = ref(false)

interface Translation {
  id: number
  pinned?: boolean
  type?: string
}

/**
 * Fetch the first usable kodik translation id for this anime from the
 * catalog. Prefers pinned translations (admin-curated default) and falls
 * back to the first row. Returns "" if nothing is available — caller
 * surfaces an error toast.
 *
 * The discovery button always defaults to the kodik player because (a)
 * every Russian-speaking user has kodik, (b) every anime that surfaces
 * the button has `has_kodik=true`, and (c) the user can switch player
 * inside the room via PlayerTabBar without any of this fetch cost. This
 * keeps the click <500ms in the common case.
 */
async function fetchFirstTranslationId(): Promise<string> {
  try {
    const resp = await kodikApi.getTranslations(props.animeId)
    const rows = (resp.data?.data ?? resp.data ?? []) as Translation[]
    if (!Array.isArray(rows) || rows.length === 0) return ''
    const pinned = rows.find(r => r.pinned)
    const winner = pinned ?? rows[0]
    return winner ? String(winner.id) : ''
  } catch {
    return ''
  }
}

async function onClick(): Promise<void> {
  if (loading.value) return
  loading.value = true
  try {
    let translationId = props.translationId
    if (!translationId) {
      translationId = await fetchFirstTranslationId()
    }
    if (!translationId) {
      toast.push(t('watch_together.invite_failed_toast'), 'error', 4000)
      return
    }

    // Shared create-room → navigate → copy-invite flow (also used by the
    // in-player WatchTogether button). Handles its own success/error toasts.
    await launch({
      animeId: props.animeId,
      episodeId: props.episodeId,
      player: props.player,
      translationId,
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
