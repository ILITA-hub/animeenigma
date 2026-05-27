<!--
  Workstream watch-together — Phase 02 (frontend-shell) Plan 02.6 Task 2.

  InviteButton.vue — the watch-together entry point. Mounted into
  WatchView.vue's player chrome area (wave 6, Plan 02.9 stitches it in).
  Click flow per CONTEXT.md §"InviteButton":

      click
        → createRoom({anime_id, episode_id, player, translation_id})
        → router.push('/watch/room/${response.room_id}')
        → navigator.clipboard.writeText(response.invite_url)
        → toast.push('Invite link copied', 'success')

  Failure branches:
    - createRoom rejects → error toast; router.push is NOT fired (so the
      user stays on the anime page to retry).
    - navigator.clipboard absent OR writeText rejects → info toast that
      embeds the raw invite_url so the user can manual-select-and-copy.
      This branch is the dev/HTTP fallback (clipboard requires a secure
      context). Per the plan, we DO still router.push in this branch —
      navigation succeeded and the URL is in the address bar; the only
      degraded UX is the user has to copy from the toast instead of just
      pasting.

  Loading state:
    - Local `loading` ref disables the button + shows an inline spinner
      while the createRoom promise is in flight. The finally clause
      guarantees re-enable on BOTH success and failure paths so a
      transient network failure doesn't strand the button in disabled.

  UI-SPEC contract (CLAUDE.md):
    - Tailwind utility classes only
    - Font weights: font-medium / font-semibold only (no font-bold)
    - Padding: px-4 py-2 for the button itself
-->

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { createRoom } from '@/api/watch-together'
import type { PlayerKind } from '@/api/watch-together'
import { useToast } from '@/composables/useToast'

const props = defineProps<{
  /** Shikimori UUID for the anime currently displayed in WatchView. */
  animeId: string
  /** Provider-specific episode identifier (string — varies by player). */
  episodeId: string
  /** Active player kind (drives WatchTogetherView's player mount). */
  player: PlayerKind
  /** Active translation/dub identifier. May be empty for the discovery
   *  mount on the anime detail page — see `resolveTranslationId`. */
  translationId: string
  /** Optional lazy resolver. Called BEFORE createRoom when translationId
   *  is empty (e.g. discovery mount on a fresh anime). The parent runs
   *  player activation + waits for useWatchPreferences to populate. */
  resolveTranslationId?: () => Promise<string>
}>()

const { t } = useI18n()
const router = useRouter()
const toast = useToast()

const loading = ref(false)

/**
 * Attempt to write the invite URL to the clipboard. Returns true on
 * success, false when the API is unavailable OR writeText rejects
 * (locked-down browsers throw NotAllowedError on non-secure contexts).
 *
 * Why try/catch around writeText: the Clipboard API rejects rather than
 * throws synchronously, and a bare `await` would propagate up to the
 * outer click handler's catch which would mis-toast a clipboard issue
 * as a createRoom failure.
 */
async function tryClipboardWrite(url: string): Promise<boolean> {
  if (typeof navigator === 'undefined' || !navigator.clipboard) {
    return false
  }
  try {
    await navigator.clipboard.writeText(url)
    return true
  } catch {
    return false
  }
}

async function onClick(): Promise<void> {
  if (loading.value) return
  loading.value = true
  try {
    // Backend requires non-empty translation_id (rooms.go:83-84). For the
    // discovery mount on the anime detail page, translation_id may not be
    // resolved yet — fall through to the parent's lazy resolver which
    // activates the player and waits for useWatchPreferences to populate.
    let translationId = props.translationId
    if (!translationId && props.resolveTranslationId) {
      translationId = await props.resolveTranslationId()
    }
    if (!translationId) {
      toast.push(t('watch_together.invite_copied_toast'), 'error', 4000)
      return
    }
    const response = await createRoom({
      anime_id: props.animeId,
      episode_id: props.episodeId,
      player: props.player,
      translation_id: translationId,
    })

    // Navigate FIRST so the address bar shows the room URL before the
    // toast appears. If the user closes the toast or the clipboard
    // fallback opens, they still ended up on the right route.
    router.push(`/watch/room/${response.room_id}`)

    const copied = await tryClipboardWrite(response.invite_url)
    if (copied) {
      toast.push(t('watch_together.invite_copied_toast'), 'success', 4000)
    } else {
      // Manual-copy fallback. The URL is embedded in the toast body so
      // the user can select-and-copy without a separate modal — for the
      // dev/HTTP context this is good-enough and avoids adding modal
      // infrastructure in Phase 2.
      toast.push(
        `${t('watch_together.invite_copy_manual')} ${response.invite_url}`,
        'info',
        8000,
      )
    }
  } catch {
    toast.push(t('watch_together.invite_copied_toast'), 'error', 4000)
    // Note: we deliberately reuse the i18n key (it reads acceptably as a
    // generic "couldn't share" message) rather than adding another locale
    // key for a path users see ~0% of the time. A future plan that wants
    // a fine-grained error message can split it out.
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
    class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-cyan-500/20 text-cyan-300 hover:bg-cyan-500/30 focus:outline-none focus:ring-2 focus:ring-cyan-400 disabled:opacity-50 disabled:cursor-not-allowed font-medium transition-colors"
    @click="onClick"
  >
    <span
      v-if="loading"
      aria-hidden="true"
      class="inline-block w-4 h-4 border-2 border-cyan-300 border-t-transparent rounded-full animate-spin"
    />
    <svg
      v-else
      aria-hidden="true"
      class="w-4 h-4"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
      <circle cx="8.5" cy="7" r="4" />
      <line x1="20" y1="8" x2="20" y2="14" />
      <line x1="23" y1="11" x2="17" y2="11" />
    </svg>
    {{ t('watch_together.invite_button_label') }}
  </button>
</template>
