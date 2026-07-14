import { ref, computed, type Ref, type ComputedRef } from 'vue'
import { userApi } from '@/api/client'
import { parseLastWatchedEpisode } from '@/composables/anime/animeFormatters'
import {
  resolveStartEpisode,
  resolveResumeState,
  clampLastWatched,
  type ResumeBanner,
  type WatchCta,
} from '@/composables/watchState'

/**
 * useWatchState — single source of truth for "where is the user in this anime".
 *
 * Resolves `lastWatched` (highest completed episode) for BOTH authed (server
 * watch_progress) and anonymous (localStorage) viewers, then derives the start
 * episode, the resume banner, and the CTA verb via the pure functions in
 * watchState.ts. Replaces useResumeStateMachine.ts + watchCta.ts.
 */
export interface UseWatchStateOptions {
  animeId: Ref<string>
  totalEpisodes: Ref<number>
  episodesAired: Ref<number>
  status: Ref<string>
  nextEpisodeAt: Ref<string | undefined>
  /** Highest episode actually loaded into our sources (player-emitted); 0 when unknown. */
  loadedEpisodes: Ref<number>
  listStatus: Ref<string | null>
  isAuthenticated: Ref<boolean>
  /** Localized future-air-time formatter (e.g. Anime.vue's formatNextEpisode). */
  formatEta: (iso: string) => string
}

export interface WatchState {
  lastWatched: ComputedRef<number>
  loaded: Ref<boolean>
  startEpisode: ComputedRef<number>
  banner: ComputedRef<ResumeBanner>
  cta: ComputedRef<WatchCta>
  init: (prefetched?: Array<{ episode_number?: number; completed?: boolean }>) => Promise<void>
  reset: () => void
}

/** Highest COMPLETED episode across the watch_progress rows; 0 when none. */
function deriveLastWatched(progress: Array<{ episode_number?: number; completed?: boolean }>): number {
  let max = 0
  for (const p of progress) {
    if (p?.completed && typeof p.episode_number === 'number' && p.episode_number > max) {
      max = p.episode_number
    }
  }
  return max
}

export function useWatchState(options: UseWatchStateOptions): WatchState {
  const loaded = ref(false)
  // Server-derived highest completed episode (authed only).
  const rawServerLastWatched = ref(0)

  // lastWatched (highest COMPLETED), unified across authed + anon.
  //  - authed: server watch_progress (max completed).
  //  - anon:   localStorage only records last-TOUCHED + position, no `completed`
  //            flag (D-1). The last-touched episode IS the in-progress one, so
  //            the highest *completed* is one below it. This makes anon open the
  //            SAME episode it always did while keeping the CTA consistent.
  const lastWatched = computed(() => {
    const total = options.totalEpisodes.value
    if (options.isAuthenticated.value) {
      return clampLastWatched(rawServerLastWatched.value, total)
    }
    const parsed = parseLastWatchedEpisode(localStorage.getItem(`watch_progress:${options.animeId.value}`))
    const completed = parsed && parsed > 0 ? parsed - 1 : 0
    return clampLastWatched(completed, total)
  })

  const startEpisode = computed(() =>
    resolveStartEpisode(lastWatched.value, options.totalEpisodes.value, {
      status: options.status.value,
      episodesAired: options.episodesAired.value,
      loadedEpisodes: options.loadedEpisodes.value,
    }),
  )

  const state = computed(() =>
    resolveResumeState({
      lastWatched: lastWatched.value,
      totalEpisodes: options.totalEpisodes.value,
      episodesAired: options.episodesAired.value,
      loadedEpisodes: options.loadedEpisodes.value,
      status: options.status.value,
      nextEpisodeAt: options.nextEpisodeAt.value,
      listStatus: options.listStatus.value,
      isAuthenticated: options.isAuthenticated.value,
      formatEta: options.formatEta,
    }),
  )
  const banner = computed(() => state.value.banner)
  const cta = computed(() => state.value.cta)

  async function init(prefetched?: Array<{ episode_number?: number; completed?: boolean }>) {
    loaded.value = false
    if (!options.isAuthenticated.value) {
      // Anon reads localStorage lazily in the `lastWatched` computed — nothing to fetch.
      loaded.value = true
      return
    }
    if (prefetched) {
      rawServerLastWatched.value = deriveLastWatched(prefetched)
      loaded.value = true
      return
    }
    try {
      const res = await userApi.getProgress(options.animeId.value)
      const data = (res.data?.data ?? res.data ?? []) as Array<{
        episode_number?: number
        completed?: boolean
      }>
      rawServerLastWatched.value = deriveLastWatched(data)
    } catch {
      // 404 / network failure → first-time is a safe default.
      rawServerLastWatched.value = 0
    } finally {
      loaded.value = true
    }
  }

  function reset() {
    loaded.value = false
    rawServerLastWatched.value = 0
  }

  return { lastWatched, loaded, startEpisode, banner, cta, init, reset }
}
