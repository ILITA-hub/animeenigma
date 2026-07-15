import { ref } from 'vue'
import { userApi } from '@/api/client'
import { useViewerContextStore } from '@/stores/viewerContext'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
import { progressRowsToMap, type EpisodeProgress, type ProgressRow } from '@/composables/aePlayer/episodeProgress'
import type { useAuthStore } from '@/stores/auth'

// ── User watch data (read-only): watched marks + per-episode progress ────────
// Feeds the episodes drawer (watched ticks, resume percentages) and the
// resume-from-saved-position chip.

export interface EpisodeWatchDataDeps {
  auth: ReturnType<typeof useAuthStore>
  getAnimeId: () => string
}

export function useEpisodeWatchData(deps: EpisodeWatchDataDeps) {
  const { auth } = deps

  const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => deps.getAnimeId())

  const epProgress = ref<Record<number, EpisodeProgress>>({})

  // Page-fetch optimization (2026-06-11): the FIRST load per anime consumes the
  // viewer-context aggregate Anime.vue already fetched, killing the duplicate
  // GET /users/progress/{id} on mount. Post-mutation reloads go to the network.
  let progressPrefetchConsumedFor: string | null = null

  async function loadEpisodeProgress() {
    if (!auth.isAuthenticated) {
      epProgress.value = {}
      return
    }
    if (progressPrefetchConsumedFor !== deps.getAnimeId()) {
      progressPrefetchConsumedFor = deps.getAnimeId()
      // whenLoaded (not forAnime): on deep-link mounts the aggregate request is
      // often still in flight — await it instead of duplicating the fetch.
      const ctx = await useViewerContextStore().whenLoaded(deps.getAnimeId())
      if (ctx) {
        epProgress.value = progressRowsToMap(ctx.progress ?? [])
        return
      }
    }
    try {
      const res = await userApi.getProgress(deps.getAnimeId())
      const rows = (res.data?.data ?? res.data ?? []) as ProgressRow[]
      epProgress.value = progressRowsToMap(rows)
    } catch {
      // 404 / anonymous / network — no user data, panel renders plain numbers
      epProgress.value = {}
    }
  }

  /** Whether the user already has this episode marked watched (drawer data). */
  function isEpisodeWatched(n: number): boolean {
    return n <= watchedUpTo.value || !!epProgress.value[n]?.completed
  }

  return { watchedUpTo, refreshWatched, epProgress, loadEpisodeProgress, isEpisodeWatched }
}
