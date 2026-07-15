import { ref, computed, watch, nextTick, type Ref, type ComputedRef } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Anime } from '@/composables/useAnime'
import { useAuthStore } from '@/stores/auth'
import { useViewerContextStore, type ViewerContextData } from '@/stores/viewerContext'
import { useWatchState } from '@/composables/useWatchState'
import type { WatchCta } from '@/composables/watchState'
import { useToast } from '@/composables/useToast'
import { userApi } from '@/api/client'

export interface AnimeWatchFlowDeps {
  anime: Ref<Anime | null>
  /** Watchlist status ref (from useAnimeListStatus) — watchState input + rewatch flow. */
  currentListStatus: Ref<string | null>
  /** Localized future-air-time formatter (from useAnimeDisplay). */
  formatNextEpisode: (iso: string) => string
  /** ?episode=N deep-link hint (from useAnimeDeepLinks). */
  queryEpisode: ComputedRef<number | undefined>
  /** Optimistic list-status setter (from useAnimeListStatus) — mark-watched CTA. */
  setListStatus: (status: string) => Promise<void>
  /** Viewer-context applier (from useAnimeSocial) — post-rewatch refresh. */
  applyViewerContext: (ctx: ViewerContextData) => void
}

/**
 * Resume / watch-CTA / player-activation flow for the anime page (extracted
 * from Anime.vue). Owns the unified watchState instance — the single source of
 * truth for last-watched, start episode, resume banner, and CTA verb (authed
 * server watch_progress + anon localStorage, resolved inside useWatchState).
 * Inputs are reactive refs it re-evaluates as anime data + watch_progress
 * arrives; Anime.vue only renders.
 */
export function useAnimeWatchFlow(deps: AnimeWatchFlowDeps) {
  const { anime, currentListStatus, formatNextEpisode, queryEpisode, setListStatus, applyViewerContext } = deps
  const { t } = useI18n()
  const authStore = useAuthStore()
  const viewerCtxStore = useViewerContextStore()
  const toast = useToast()

  const playerSectionRef = ref<HTMLElement | null>(null)
  const playerActivated = ref(false)

  /** Bring the player section to the top of the viewport. The offset below the
   *  fixed navbar is NOT computed here — it comes from the section's
   *  `scroll-margin-top: var(--header-offset)`, so every caller lands on the
   *  same line. Callers pass the behavior because they differ: the watch CTA
   *  always animates, theater mode honours prefers-reduced-motion. */
  async function scrollToPlayerSection(behavior: ScrollBehavior = 'smooth') {
    await nextTick()
    playerSectionRef.value?.scrollIntoView({ behavior, block: 'start' })
  }

  async function activatePlayer() {
    playerActivated.value = true
    await scrollToPlayerSection()
  }

  const resumeAnimeId = computed(() => anime.value?.id ?? '')
  const resumeTotal = computed(() => anime.value?.totalEpisodes ?? 0)
  const resumeAired = computed(() => anime.value?.episodesAired ?? 0)
  const resumeStatus = computed(() => anime.value?.status ?? '')
  const resumeNextAt = computed(() => anime.value?.nextEpisodeAt ?? undefined)
  const resumeAuth = computed(() => authStore.isAuthenticated)
  // Highest episode actually loaded into our sources, surfaced by the active
  // player via `available-translations` (max episodes_count across teams). This
  // is the ground truth for availability and overrides Shikimori's lagging
  // `episodesAired` in the resume state — so a freshly-uploaded episode is never
  // mislabeled "not available yet". 0 until the player emits.
  const resumeLoadedEpisodes = ref(0)
  const watchState = useWatchState({
    animeId: resumeAnimeId,
    totalEpisodes: resumeTotal,
    episodesAired: resumeAired,
    status: resumeStatus,
    nextEpisodeAt: resumeNextAt,
    loadedEpisodes: resumeLoadedEpisodes,
    listStatus: currentListStatus,
    isAuthenticated: resumeAuth,
    formatEta: (iso: string) => formatNextEpisode(iso),
  })

  // User-driven override of the state machine's start episode. Set when the
  // user clicks "Rewatch from ep. 1" on the finished banner; otherwise null.
  const resumeOverrideEpisode = ref<number | null>(null)

  // Single start-episode authority: explicit overrides win, else the unified
  // watchState. Manual override (rewatch click) > deep-link query param >
  // watchState.startEpisode. Always a number >= 1 — never episodesAired.
  const resumeStartEpisode = computed<number>(() => {
    if (resumeOverrideEpisode.value && resumeOverrideEpisode.value > 0) {
      return resumeOverrideEpisode.value
    }
    if (queryEpisode.value !== undefined) {
      return queryEpisode.value
    }
    return watchState.startEpisode.value
  })

  // Phase 8 (CR-01) — auto-activate the player when arriving with ?episode=N.
  // The deep link's intent is "land me on episode N ready to watch", so we
  // expand the click-to-load placeholder automatically. Also re-mount when the
  // query flips between values without a full route change (e.g. user clicks
  // two different Continue-Watching cards for the same anime in succession).
  watch(
    () => queryEpisode.value,
    (n) => {
      if (n !== undefined && !playerActivated.value) {
        // Defer to onMounted's anime-load path so the player has its data.
        // activatePlayer() is idempotent; calling it here is safe even before
        // the anime resolves because the template gates on `v-if="anime"`.
        activatePlayer()
      }
    },
    { immediate: true },
  )

  // Resume banner passthrough — the unified watchState owns banner kind /
  // episode / ETA. Player slots bind it directly; ResumePill decides per-kind
  // visibility, AePlayer overlays only the next-unavailable family.
  const resumeBanner = computed(() => watchState.banner.value)

  // --- Watched-aware play button + tracked rewatch (design 2026-06-05) -------
  // The verb comes from actual episode progress; list status only disambiguates
  // the fully-watched terminal: not-completed → mark-watched, completed → rewatch.
  const rewatchPending = ref(false)

  const watchCta = computed(() => watchState.cta.value)

  const watchCtaLabel = computed(() => t(watchCta.value.labelKey, watchCta.value.labelParams ?? {}))

  // startRewatchFlow — server-side cycle reset (status→watching, episodes=0,
  // watch_progress reset, is_rewatching=true; the count bumps on the new finale),
  // then re-init the resume machine and mount the player at ep. 1.
  async function startRewatchFlow() {
    if (!anime.value || rewatchPending.value) return
    rewatchPending.value = true
    const animeId = anime.value.id
    try {
      await userApi.startRewatch(animeId)
      currentListStatus.value = 'watching'
      resumeOverrideEpisode.value = 1
      await watchState.init()
      void viewerCtxStore.load(animeId, true).then((ctx) => { if (ctx) applyViewerContext(ctx) })
      void activatePlayer()
    } catch (err) {
      console.error('Failed to start rewatch:', err)
      toast.push(t('watchlist.errors.updateFailed'))
    } finally {
      rewatchPending.value = false
    }
  }

  async function dispatchWatchCta(cta: WatchCta) {
    if (cta.action === 'mark-watched') {
      await setListStatus('completed')
      return
    }
    if (cta.action === 'rewatch') {
      await startRewatchFlow()
      return
    }
    if (cta.action === 'start-from-1') {
      resumeOverrideEpisode.value = cta.startEpisode
    }
    void activatePlayer()
  }

  const onWatchCtaClick = () => dispatchWatchCta(watchCta.value)

  /** Per-anime reset (route param change) — mirrors the loadAnimeData reset block. */
  function reset() {
    watchState.reset()
    resumeLoadedEpisodes.value = 0
    resumeOverrideEpisode.value = null
  }

  return {
    playerSectionRef,
    scrollToPlayerSection,
    resumeLoadedEpisodes,
    watchState,
    resumeStartEpisode,
    resumeBanner,
    watchCta,
    watchCtaLabel,
    onWatchCtaClick,
    reset,
  }
}
