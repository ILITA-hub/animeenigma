import { watch, onMounted, onUnmounted, nextTick, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Anime } from '@/composables/useAnime'
import type { WatchState } from '@/composables/useWatchState'
import { useAuthStore } from '@/stores/auth'
import { useViewerContextStore, type ViewerContextData } from '@/stores/viewerContext'
import { animeApi, userApi } from '@/api/client'
import type { UgcTab } from './types'

export interface AnimeDataLoaderDeps {
  anime: Ref<Anime | null>
  fetchAnime: (id: string) => Promise<Anime | null>
  /**
   * Synchronous per-concern state reset so stale data doesn't flash on a
   * route-param change (each page composable's reset() + page-local refs +
   * disarming the lazy-section observer). Runs after `anime` is nulled.
   */
  resetForAnime: () => void
  /** Unified watch state (from useAnimeWatchFlow) — init with prefetched progress. */
  watchState: WatchState
  /** Viewer-context aggregate applier (from useAnimeSocial). */
  applyViewerContext: (ctx: ViewerContextData) => void
  /** Watch-preference init (from usePlayerSurface). */
  initPreferences: (animeId: string, tier1Combo?: ViewerContextData['combo']) => void
  /** Legacy fallback fetches — used only when the viewer-context aggregate failed. */
  fetchWatchlistStatus: () => Promise<void>
  fetchReviews: () => Promise<void>
  fetchWatchersCount: () => Promise<void>
  /** Admin hidden-status mirror (from useAnimeAdmin). */
  fetchHiddenStatus: () => void
  /** Comments deep-link path (from useAnimeComments). */
  ugcTab: Ref<UgcTab>
  commentsFetched: Ref<boolean>
  fetchComments: () => Promise<void>
  /** Lazy below-the-fold observer arm (from useLazyAnimeSections). */
  armLazySections: () => void
}

/**
 * Page data-loading orchestration for the anime page (extracted from
 * Anime.vue): the shared loadAnimeData flow (MAL-id resolution, viewer-context
 * aggregate + legacy fallbacks, pending MAL bind, lazy-section arming), the
 * route-param re-load watcher, the initial mount load, and generation-based
 * cancellation of in-flight loads.
 */
export function useAnimeDataLoader(deps: AnimeDataLoaderDeps) {
  const route = useRoute()
  const router = useRouter()
  const authStore = useAuthStore()
  const viewerCtxStore = useViewerContextStore()

  let loadGeneration = 0

  // Shared data-loading function — called on mount and on route param change
  const loadAnimeData = async (animeId: string) => {
    // Increment generation so previous in-flight calls become stale
    const gen = ++loadGeneration

    // Reset state so stale data doesn't flash
    deps.anime.value = null
    deps.resetForAnime()

    // Handle MAL-prefixed IDs: resolve via backend
    if (animeId.startsWith('mal_')) {
      const malId = animeId.replace('mal_', '')
      try {
        const response = await animeApi.resolveMAL(malId)
        if (gen !== loadGeneration) return
        const result = response.data?.data || response.data
        if (result?.status === 'resolved' && result.anime) {
          // Migrate list entry if user is authenticated
          if (authStore.isAuthenticated) {
            try {
              await userApi.migrateListEntry(
                animeId,
                result.anime.id
              )
            } catch (e) {
              console.warn('List migration failed:', e)
            }
          }
          router.replace(`/anime/${result.anime.id}`)
          return
        } else if (result?.status === 'ambiguous') {
          const searchQuery = result.mal_title || ''
          router.replace({ path: '/browse', query: { q: searchQuery, bind_mal: animeId } })
          return
        }
      } catch (e) {
        console.error('MAL resolution failed:', e)
      }
    }

    const fetched = await deps.fetchAnime(animeId)
    if (gen !== loadGeneration) return

    // Pull the viewer-context aggregate — ONE request carrying rating,
    // watchers-count, watch progress, watchlist entry, my review and the saved
    // combo (page-fetch optimization 2026-06-11). The legacy per-endpoint fetches
    // remain as the fallback when the aggregate is unavailable. (watchState reads
    // the anon localStorage last-watched lazily — no explicit load needed.)
    let viewerCtx: ViewerContextData | null = null
    if (fetched) {
      viewerCtx = await viewerCtxStore.load(fetched.id, false, fetched.malId || undefined)
      if (gen !== loadGeneration) return
      if (viewerCtx) {
        deps.applyViewerContext(viewerCtx)
        void deps.watchState.init(viewerCtx.progress ?? [])
        // Legacy MAL-import entry surfaced under anime_id="mal_{malId}" —
        // auto-migrate it to the real UUID (same behavior the statuses-scan
        // path had), fire-and-forget.
        const entryAnimeId = viewerCtx.watchlist_entry?.anime_id
        if (authStore.isAuthenticated && entryAnimeId && entryAnimeId.startsWith('mal_')) {
          userApi.migrateListEntry(entryAnimeId, fetched.id).catch((e) => {
            console.warn('Auto-migration of MAL entry failed:', e)
          })
        }
      } else {
        void deps.watchState.init()
      }
    }

    // Initialize watch preferences for this anime — anon users included so the
    // combo_resolve_total denominator increments alongside combo_override_total
    // (CONTEXT D-12: per-anon-user override rate). The composable + axios
    // interceptor handle the X-Anon-ID header for unauthenticated callers.
    // The viewer-context Tier-1 combo lets resolve() short-circuit client-side.
    if (fetched) {
      deps.initPreferences(fetched.id, viewerCtx?.combo)
    }

    // Check for pending MAL bind from Browse page
    const pendingBind = sessionStorage.getItem('pending_mal_bind')
    if (pendingBind && fetched && authStore.isAuthenticated) {
      sessionStorage.removeItem('pending_mal_bind')
      try {
        await userApi.migrateListEntry(
          pendingBind,
          fetched.id
        )
      } catch (e) {
        console.warn('Pending MAL bind migration failed:', e)
      }
    }

    // Viewer-context already delivered watchlist status / rating / my-review /
    // watchers-count; the legacy fetches below only run as fallback. In the
    // fallback the rating rides on fetchReviews, so it stays eager there; in
    // the normal path the reviews feed is lazy (IntersectionObserver arm below).
    if (!viewerCtx) {
      await deps.fetchWatchlistStatus()
      if (gen !== loadGeneration) return
    }
    await deps.fetchHiddenStatus()
    if (gen !== loadGeneration) return
    if (!viewerCtx) {
      await deps.fetchReviews()
      // Phase 14 / UX-28 — non-blocking; failure leaves the badge hidden.
      void deps.fetchWatchersCount()
    }

    // Deep-link path: if the URL already has ?ugc=comments on first paint,
    // kick off the initial comments fetch (the watch(ugcTab) lazy-fetch only
    // fires on subsequent changes, not on initial value).
    if (deps.ugcTab.value === 'comments' && !deps.commentsFetched.value) {
      void deps.fetchComments()
    }

    // Reviews feed + related rail load lazily as the user scrolls toward them
    // (page-fetch optimization 2026-06-11). Arm after the DOM has rendered the
    // observed elements (they're v-if'd on `anime`).
    await nextTick()
    if (gen !== loadGeneration) return
    deps.armLazySections()
  }

  const retry = () => {
    const animeId = route.params.id as string
    deps.fetchAnime(animeId)
  }

  // Re-load when route param changes (Vue Router reuses the component for /anime/:id)
  watch(() => route.params.id, (newId) => {
    if (newId) {
      loadAnimeData(newId as string)
    }
  })

  onMounted(() => {
    loadAnimeData(route.params.id as string)
  })

  onUnmounted(() => {
    loadGeneration++ // cancel any in-flight loadAnimeData
  })

  return { loadAnimeData, retry }
}
