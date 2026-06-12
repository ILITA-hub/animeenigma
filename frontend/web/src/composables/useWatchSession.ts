import { ref, onUnmounted } from 'vue'
import { postKeepalive } from '@/utils/authBeacon'

/**
 * useWatchSession — Phase 5 playback session correlation + drop-off beacon.
 *
 * Provides:
 *   - sessionId: a UUID per playback session, sent with every heartbeat and
 *     the eventual completion mark so backend can correlate them. (G-04-lite)
 *   - newSession(): rotate the ID when the user switches episodes within the
 *     same player mount — episode-grain is what Tier 2 inference needs.
 *   - sendDropOffBeacon(): records the abandon position when the user closes
 *     the page mid-episode. (G-01)
 *   - registerBeaconHooks(): wires automatic dispatch on `pagehide` /
 *     `visibilitychange:hidden`. The composable cleans up its listeners
 *     onUnmounted so callers do not need to.
 *
 * Why fetch+keepalive, not navigator.sendBeacon: a normal POST may be
 * cancelled by the browser at unload time, but sendBeacon cannot set an
 * Authorization header — and /users/progress/:id/dropoff requires a Bearer
 * JWT, so every sendBeacon attempt was 401-rejected and silently lost.
 * `fetch(..., { keepalive: true })` survives pagehide like a beacon AND
 * carries the token (see utils/authBeacon.ts).
 */

function uuid(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

export interface DropOffSnapshot {
  animeId: string
  episodeNumber: number
  progressSeconds: number
}

export function useWatchSession() {
  const sessionId = ref(uuid())

  function newSession() {
    sessionId.value = uuid()
  }

  /**
   * Send a drop-off save. Best-effort, never throws — the page may be
   * unloading. Authenticated keepalive fetch (see header comment).
   */
  function sendDropOffBeacon(snap: DropOffSnapshot) {
    if (!snap.animeId || snap.episodeNumber <= 0) return
    if (snap.progressSeconds < 5) return // ignore noise from instant-close

    postKeepalive(`/users/progress/${snap.animeId}/dropoff`, {
      episode_number: snap.episodeNumber,
      progress: Math.floor(snap.progressSeconds),
      session_id: sessionId.value,
    })
  }

  /**
   * Register pagehide / visibilitychange listeners that fire the drop-off
   * beacon for whatever (animeId, episode, progress) the snapshot getter
   * returns at unload time. The getter is invoked synchronously inside the
   * event handler so it always reads current state — closures over reactive
   * refs work correctly.
   *
   * Returns a cleanup fn the caller can invoke explicitly; otherwise the
   * composable runs cleanup automatically onUnmounted.
   */
  function registerBeaconHooks(getSnapshot: () => DropOffSnapshot | null) {
    if (typeof window === 'undefined') return () => undefined

    const onPageHide = () => {
      const snap = getSnapshot()
      if (snap) sendDropOffBeacon(snap)
    }
    const onVisibilityChange = () => {
      if (document.visibilityState === 'hidden') {
        const snap = getSnapshot()
        if (snap) sendDropOffBeacon(snap)
      }
    }

    window.addEventListener('pagehide', onPageHide)
    document.addEventListener('visibilitychange', onVisibilityChange)

    const cleanup = () => {
      window.removeEventListener('pagehide', onPageHide)
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
    onUnmounted(cleanup)
    return cleanup
  }

  return { sessionId, newSession, sendDropOffBeacon, registerBeaconHooks }
}
