import { ref } from 'vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useWatchSession } from '@/composables/useWatchSession'
import { postKeepalive } from '@/utils/authBeacon'

/**
 * useWatchTracking — server + localStorage watch-progress tracking for the
 * unified player (parity with KodikPlayer's tracking, minus the combo
 * preference upsert: the unified player's provider ids don't map onto the
 * legacy ValidPlayers combo enum, and an empty combo is explicitly valid).
 *
 * Responsibilities:
 *  - heartbeat save every SAVE_INTERVAL seconds of playback (server + local)
 *  - immediate save on pause / episode switch / unmount (`saveNow`)
 *  - sendBeacon save on pagehide so closing the tab never loses position
 *  - duration-aware auto-complete: mark the episode watched at ≥90% of the
 *    real video duration (HTML5 — always known here), with the legacy
 *    20-minute rule as a fallback while metadata is missing
 *
 * The caller drives it from its existing rAF progress loop via `onTick` —
 * all internal work is integer comparisons, so per-frame cost is nil.
 */

const SAVE_INTERVAL = 30 // seconds of media time between heartbeat saves
const AUTO_MARK_FALLBACK = 20 * 60 // legacy threshold when duration unknown
const COMPLETE_RATIO = 0.9

export interface WatchTrackingHooks {
  /** Called after an auto/manual mark succeeds — refresh drawer user data. */
  onMarked?: (episode: number) => void
}

export function useWatchTracking(
  animeId: () => string,
  episodeNumber: () => number | null,
  hooks: WatchTrackingHooks = {},
) {
  const auth = useAuthStore()
  const { sessionId } = useWatchSession()

  const maxTime = ref(0)
  /** true once the CURRENT episode got marked completed (auto or manual). */
  const episodeMarked = ref(false)
  const marking = ref(false)
  let lastSavedMediaTime = 0
  let lastKnown = { time: 0, dur: 0 }

  function resetEpisode(alreadyWatched = false) {
    maxTime.value = 0
    episodeMarked.value = alreadyWatched
    marking.value = false
    lastSavedMediaTime = 0
    lastKnown = { time: 0, dur: 0 }
  }

  // ── persistence ───────────────────────────────────────────────────────────

  function saveLocal(time: number) {
    const ep = episodeNumber()
    if (!ep || time <= 0) return
    try {
      const key = `watch_progress:${animeId()}`
      const data = JSON.parse(localStorage.getItem(key) || '{}')
      data[ep] = { time, maxTime: maxTime.value, updatedAt: Date.now() }
      localStorage.setItem(key, JSON.stringify(data))
    } catch {
      /* quota/parse — non-fatal */
    }
  }

  function saveServer(time: number) {
    const ep = episodeNumber()
    if (!ep || time <= 0 || !auth.isAuthenticated) return
    void userApi
      .updateProgress({
        anime_id: animeId(),
        episode_number: ep,
        progress: Math.floor(time),
        duration: Math.floor(maxTime.value) || null,
        session_id: sessionId.value,
      })
      .catch(() => {
        /* heartbeat save is best-effort */
      })
  }

  /** Immediate save (pause, episode switch, unmount). */
  function saveNow() {
    if (lastKnown.time <= 0) return
    lastSavedMediaTime = lastKnown.time
    saveLocal(lastKnown.time)
    saveServer(lastKnown.time)
  }

  /** Page-close save — keepalive fetch survives the unload where XHR doesn't
   *  (and unlike sendBeacon it carries the Authorization header — beacons to
   *  this JWT-protected endpoint were 401-rejected). */
  function beaconSave() {
    const ep = episodeNumber()
    if (!ep || lastKnown.time <= 0) return
    saveLocal(lastKnown.time)
    if (!auth.isAuthenticated) return
    postKeepalive('/users/progress', {
      anime_id: animeId(),
      episode_number: ep,
      progress: Math.floor(lastKnown.time),
      duration: Math.floor(maxTime.value) || null,
      session_id: sessionId.value,
    })
  }

  // ── completion ────────────────────────────────────────────────────────────

  async function markWatched(): Promise<boolean> {
    const ep = episodeNumber()
    if (!ep || !auth.isAuthenticated || episodeMarked.value || marking.value) return false
    marking.value = true
    try {
      await userApi.markEpisodeWatched(animeId(), ep, undefined, sessionId.value)
      episodeMarked.value = true
      hooks.onMarked?.(ep)
      return true
    } catch {
      return false
    } finally {
      marking.value = false
    }
  }

  // ── per-frame driver ──────────────────────────────────────────────────────

  function onTick(time: number, dur: number) {
    lastKnown = { time, dur }
    if (time > maxTime.value) maxTime.value = time

    if (time - lastSavedMediaTime >= SAVE_INTERVAL) {
      lastSavedMediaTime = time
      saveLocal(time)
      saveServer(time)
    }

    if (!episodeMarked.value && !marking.value && auth.isAuthenticated) {
      const nearEnd = dur > 0 && maxTime.value >= COMPLETE_RATIO * dur
      if (nearEnd || maxTime.value >= AUTO_MARK_FALLBACK) void markWatched()
    }
  }

  return {
    maxTime,
    episodeMarked,
    marking,
    onTick,
    saveNow,
    beaconSave,
    markWatched,
    resetEpisode,
  }
}
