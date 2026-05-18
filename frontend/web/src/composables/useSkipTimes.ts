// Phase 18 (UX-34) — Skip-Intro / Skip-Outro CTAs on English players
// players. Wraps GET /api/skip-times/{malId}/{episode} (catalog proxy of
// api.aniskip.com) and exposes a reactive { opening, ending } pair so the
// player overlay can compute showSkipIntro = currentTime in opening window.
//
// Style anchor: useContinueWatching.ts — same { ref + watcher + skip-fetch
// when input missing } shape, except no auth gating (skip-times is public).
//
// Graceful degradation contract: if `malId` is null/empty OR the upstream
// has no crowdsourced data for this (anime, episode) pair, opening/ending
// both stay null and the player overlay simply doesn't render the button.
// The composable never throws to the caller.

import { ref, watch, type Ref } from 'vue'
import { animeApi } from '@/api/client'

// One skip segment as returned by aniskip v2 (camelCase passthrough from
// our catalog proxy — see services/catalog/internal/handler/skip_times.go).
interface SkipTimesResultItem {
  interval: { startTime: number; endTime: number }
  skipType: string // "op" | "ed" | "mixed-op" | "mixed-ed" | "recap"
  skipId: string
  episodeLength: number
}

interface SkipTimesResult {
  found: boolean
  results: SkipTimesResultItem[]
}

// Compact { start, end } pair the player overlay consumes. Decoupled from
// the upstream wire shape so changing aniskip's API later doesn't ripple
// through every player component.
export interface SkipSegment {
  start: number
  end: number
}

export function useSkipTimes(
  malId: Ref<string | number | null | undefined>,
  episode: Ref<number | null | undefined>,
) {
  const opening = ref<SkipSegment | null>(null)
  const ending = ref<SkipSegment | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  // Track the in-flight request so a fast scrub (ep 1 → ep 2 → ep 3) doesn't
  // race — only the latest fetch's result is allowed to update the refs.
  // Stale resolutions are dropped on the floor.
  let inFlightToken = 0

  async function fetchSkipTimes() {
    const token = ++inFlightToken

    // Reset state on every input change so a navigation away from a
    // covered episode immediately hides any stale CTA.
    opening.value = null
    ending.value = null
    error.value = null

    const id = malId.value
    const ep = episode.value

    // Graceful skip — no MAL ID or no episode number means we can't query
    // aniskip; the player overlay simply renders no CTA.
    if (id == null || id === '' || ep == null || ep < 1) {
      return
    }

    loading.value = true
    try {
      const res = await animeApi.getSkipTimes(String(id), ep)
      // Defend against stale in-flight: a newer fetch may have started
      // while we were awaiting the response.
      if (token !== inFlightToken) return

      const data = (res.data?.data ?? res.data) as SkipTimesResult
      if (!data || !data.found || !Array.isArray(data.results)) {
        return
      }

      for (const item of data.results) {
        if (!item || !item.interval) continue
        const seg: SkipSegment = {
          start: item.interval.startTime,
          end: item.interval.endTime,
        }
        // Aniskip returns 'op'/'ed' for plain openings/endings and
        // 'mixed-op'/'mixed-ed' for combined segments. We treat both as
        // the same skip target — the user just wants to land at the end
        // of whatever leading non-content block exists.
        if (item.skipType === 'op' || item.skipType === 'mixed-op') {
          opening.value = seg
        } else if (item.skipType === 'ed' || item.skipType === 'mixed-ed') {
          ending.value = seg
        }
      }
    } catch (e) {
      // Network error / unexpected response shape → degrade to no CTA.
      // We log to console for diagnostics but never surface to the player UI
      // because skip-intro is a nice-to-have, not blocking.
      if (token !== inFlightToken) return
      error.value = e instanceof Error ? e.message : 'failed to load skip-times'
      // Keep opening/ending null — already reset above.
    } finally {
      if (token === inFlightToken) {
        loading.value = false
      }
    }
  }

  // Re-fetch whenever malId OR episode changes. `immediate: true` so the
  // first render of the player triggers a fetch (the watcher otherwise
  // only fires on subsequent changes).
  watch([malId, episode], fetchSkipTimes, { immediate: true })

  return { opening, ending, loading, error, refresh: fetchSkipTimes }
}
