/**
 * Workstream watch-together — useWatchTogetherLaunch.
 *
 * The shared "create a room and take me there" flow, extracted from
 * InviteButton.vue so the in-player WatchTogether overlay button can reuse
 * the exact same proven path instead of duplicating it.
 *
 * Flow (given a fully-resolved payload):
 *   launch(payload)
 *     → createRoom({anime_id, episode_id, player, translation_id})
 *     → router.push('/watch/room/${room_id}')   (address bar = the invite)
 *     → navigator.clipboard.writeText(invite_url)
 *     → toast success / manual-copy fallback
 *   createRoom rejects → error toast, stay on page.
 *
 * Callers are responsible for resolving `translationId` BEFORE calling
 * (InviteButton fetches a kodik default when its prop is empty; the player
 * always has a resolved combo token). An empty `translationId` short-circuits
 * to an error toast — we never create a room a joiner can't resolve.
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { createRoom } from '@/api/watch-together'
import type { PlayerKind } from '@/api/watch-together'
import { useToast } from '@/composables/useToast'

export interface WatchTogetherLaunchPayload {
  /** Shikimori UUID for the anime. */
  animeId: string
  /** Provider-specific episode identifier (string — varies by player). */
  episodeId: string
  /** Active player kind (drives WatchTogetherView's player mount). */
  player: PlayerKind
  /** Resolved translation/dub/combo identifier. Must be non-empty. */
  translationId: string
}

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

export function useWatchTogetherLaunch() {
  const { t } = useI18n()
  const router = useRouter()
  const toast = useToast()

  /** In-flight guard, exposed so callers can drive a disabled/spinner state. */
  const launching = ref(false)

  async function launch(payload: WatchTogetherLaunchPayload): Promise<void> {
    if (launching.value) return
    if (!payload.translationId) {
      toast.push(t('watch_together.invite_failed_toast'), 'error', 4000)
      return
    }
    launching.value = true
    try {
      const response = await createRoom({
        anime_id: payload.animeId,
        episode_id: payload.episodeId,
        player: payload.player,
        translation_id: payload.translationId,
      })

      // Navigate FIRST so the address bar shows the room URL before the toast.
      router.push(`/watch/room/${response.room_id}`)

      const copied = await tryClipboardWrite(response.invite_url)
      if (copied) {
        toast.push(t('watch_together.invite_copied_toast'), 'success', 4000)
      } else {
        toast.push(
          `${t('watch_together.invite_copy_manual')} ${response.invite_url}`,
          'info',
          8000,
        )
      }
    } catch {
      toast.push(t('watch_together.invite_failed_toast'), 'error', 4000)
    } finally {
      launching.value = false
    }
  }

  return { launching, launch }
}
