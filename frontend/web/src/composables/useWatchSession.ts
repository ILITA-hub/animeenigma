import { ref, onUnmounted } from 'vue'
import { apiClient } from '@/api/client'

/**
 * useWatchSession — Phase 5 playback session correlation + drop-off beacon.
 *
 * Provides:
 *   - sessionId: a UUID per playback session, sent with every heartbeat and
 *     the eventual completion mark so backend can correlate them. (G-04-lite)
 *   - newSession(): rotate the ID when the user switches episodes within the
 *     same player mount — episode-grain is what Tier 2 inference needs.
 *   - sendDropOffBeacon(): fires a `navigator.sendBeacon` call to record the
 *     abandon position when the user closes the page mid-episode. (G-01)
 *   - registerBeaconHooks(): wires automatic dispatch on `pagehide` /
 *     `visibilitychange:hidden`. The composable cleans up its listeners
 *     onUnmounted so callers do not need to.
 *
 * Why a beacon, not a normal POST: by the time the page is unloading, normal
 * fetch() may be cancelled by the browser. `navigator.sendBeacon` is the only
 * reliable way to ship a tiny payload at unload time. It accepts Blob; we use
 * a JSON Blob so the backend can parse it as `application/json` if desired,
 * but the player handler also tolerates `text/plain` (the default content
 * type sendBeacon picks when given a string).
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
   * Send a drop-off beacon. Best-effort: if the browser refuses or
   * sendBeacon is unsupported, falls back to a fire-and-forget fetch with
   * keepalive. Either way, never throws — the page is unloading.
   */
  function sendDropOffBeacon(snap: DropOffSnapshot) {
    if (!snap.animeId || snap.episodeNumber <= 0) return
    if (snap.progressSeconds < 5) return // ignore noise from instant-close

    const body: Record<string, unknown> = {
      episode_number: snap.episodeNumber,
      progress: Math.floor(snap.progressSeconds),
      session_id: sessionId.value,
    }
    const url = `${apiClient.defaults.baseURL ?? ''}/users/progress/${snap.animeId}/dropoff`

    try {
      const blob = new Blob([JSON.stringify(body)], { type: 'application/json' })
      const ok =
        typeof navigator !== 'undefined' &&
        typeof navigator.sendBeacon === 'function' &&
        navigator.sendBeacon(url, blob)
      if (!ok) {
        // sendBeacon refused (queue full or disabled) — fall back to
        // fetch + keepalive, which Chrome / Firefox will let run to
        // completion even after pagehide.
        void fetch(url, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
          keepalive: true,
          credentials: 'include',
        }).catch(() => undefined)
      }
    } catch {
      // navigator.sendBeacon throws when the URL is cross-origin without CORS
      // approval — same fallback as above.
      void fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        keepalive: true,
        credentials: 'include',
      }).catch(() => undefined)
    }
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
