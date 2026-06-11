import { ref, computed, onScopeDispose, type Ref, type ComputedRef } from 'vue'
import { userApi } from '@/api/client'

/**
 * useResumeStateMachine — Phase 4 (A-03, A-04).
 *
 * Computes the correct resume state for an anime given the user's server-side
 * watch_progress + the anime's airing data, and exposes both the state and a
 * derived `startEpisode` that the player should pre-select on mount.
 *
 * The five states cover what the user actually sees above the player:
 *
 *   - first-time     — never watched. startEpisode=1, no banner.
 *   - watching       — last < total, next episode is available. startEpisode=last+1,
 *                      banner reads "you finished ep N".
 *   - finished       — last == total. Player still mounts (in case of rewatch),
 *                      but a "you finished this" surface renders alongside.
 *                      startEpisode=last (re-loads the final ep).
 *   - not-yet-aired  — last < total but the next episode genuinely hasn't aired
 *                      yet (anime.status='ongoing' AND nextEpisodeAt is in the
 *                      FUTURE — or unknown — AND episodesAired <= last). Player
 *                      still mounts on startEpisode=last so the user can rewatch.
 *   - episode-not-loaded-yet — user is caught up (last == episodesAired),
 *                      nextEpisodeAt is in the PAST (so the next episode HAS
 *                      aired) but it isn't loaded into our catalog/sources yet
 *                      (translation/upload lag). `episodeAiredAgoMs` drives an
 *                      "aired N ago" label. Same startEpisode as not-yet-aired.
 *                      This relies on episodesAired/nextEpisodeAt being fresh —
 *                      the catalog now invalidates the anime cache on Shikimori
 *                      upserts + caps ongoing-anime cache TTL, so stale catalog
 *                      data (which used to produce false banners) self-heals.
 *
 * Anonymous users: this composable is logged-in only — the parent view should
 * keep using its existing localStorage path and not invoke this. Phase 7
 * extends parity to anonymous via D-01.
 */

export type ResumeKind =
  | 'first-time'
  | 'watching'
  | 'finished'
  | 'not-yet-aired'
  | 'episode-not-loaded-yet'

export interface ResumeStateInputs {
  animeId: Ref<string>
  totalEpisodes: Ref<number>
  episodesAired: Ref<number>
  nextEpisodeAt: Ref<string | undefined>
  status: Ref<string>
  isAuthenticated: Ref<boolean>
  /**
   * Highest episode number actually loaded into our video sources (max across
   * the active player's translations/teams), 0 when unknown. This is the
   * GROUND TRUTH for availability — Shikimori's `episodesAired` lags reality by
   * hours after an episode airs, but the providers (Kodik fan teams, etc.) have
   * often already uploaded it. When `loadedEpisodes > episodesAired`, those
   * extra episodes ARE watchable, so we must not show "not loaded yet" for
   * them. Optional/0 falls back to pure Shikimori metadata (prior behavior).
   */
  loadedEpisodes?: Ref<number>
}

export interface ResumeStateMachine {
  /** True once init() has fetched server-side progress (or skipped — anon). */
  loaded: Ref<boolean>
  /** Highest completed episode for this user, clamped at totalEpisodes; 0 if none. */
  lastWatched: ComputedRef<number>
  /** Computed state. */
  kind: ComputedRef<ResumeKind>
  /**
   * For `episode-not-loaded-yet`: milliseconds since the episode's air time
   * (reactive — refreshes on the 60s tick). 0 for other states. The consumer
   * formats it into a localized "aired N ago" label.
   */
  episodeAiredAgoMs: ComputedRef<number>
  /** Episode the player should mount on. */
  startEpisode: ComputedRef<number>
  /** Episode the user just finished — drives the breadcrumb. 0 when N/A. */
  finishedEpisode: ComputedRef<number>
  /**
   * init() — populate lastWatched from watch_progress. When the caller
   * already holds the rows (viewer-context aggregate), pass them to skip
   * the network fetch.
   */
  init: (prefetched?: Array<{ episode_number?: number; completed?: boolean }>) => Promise<void>
  /** Reset to first-time state (used when navigating between anime). */
  reset: () => void
}

/**
 * Internal: parse the /users/progress/{animeId} response into the highest
 * completed episode. The response is `WatchProgress[]`, one row per episode
 * the user has interacted with. We pick max(episode_number) where completed.
 */
function deriveLastWatched(progress: Array<{ episode_number?: number; completed?: boolean }>): number {
  let max = 0
  for (const p of progress) {
    if (p?.completed && typeof p.episode_number === 'number' && p.episode_number > max) {
      max = p.episode_number
    }
  }
  return max
}

export function useResumeStateMachine(inputs: ResumeStateInputs): ResumeStateMachine {
  const loaded = ref(false)
  // Raw value from the server-side progress aggregator. May exceed
  // totalEpisodes if catalog/progress data is out of sync (UA-110); we expose
  // `lastWatched` below as a CLAMPED computed so consumers never see a value
  // greater than totalEpisodes.
  const rawLastWatched = ref(0)

  // Reactive "now" so the airing-state predicates self-heal as time passes,
  // without a page refresh. `kind` previously read Date.now() directly, which
  // isn't a reactive dependency — so not-yet-aired never flipped to
  // episode-not-loaded-yet in-session, and the "aired N ago" label never
  // advanced. Ticking a ref once a minute makes `kind` / `episodeAiredAgoMs`
  // recompute on a coarse-but-free cadence, plenty for a minutes/hours label.
  const nowMs = ref(Date.now())
  if (typeof window !== 'undefined') {
    const timer = window.setInterval(() => {
      nowMs.value = Date.now()
    }, 60_000)
    onScopeDispose(() => window.clearInterval(timer))
  }

  async function init(prefetched?: Array<{ episode_number?: number; completed?: boolean }>) {
    loaded.value = false
    if (!inputs.isAuthenticated.value) {
      rawLastWatched.value = 0
      loaded.value = true
      return
    }
    // Page-fetch optimization (2026-06-11): the anime page's viewer-context
    // aggregate already carries the progress rows — consume them instead of
    // re-fetching /users/progress/{animeId}.
    if (prefetched) {
      rawLastWatched.value = deriveLastWatched(prefetched)
      loaded.value = true
      return
    }
    try {
      const res = await userApi.getProgress(inputs.animeId.value)
      const data = (res.data?.data ?? res.data ?? []) as Array<{
        episode_number?: number
        completed?: boolean
      }>
      rawLastWatched.value = deriveLastWatched(data)
    } catch {
      // Best-effort: 404 / network failure leaves lastWatched at 0 (first-time
      // is a safe default — the user can still resume from localStorage in the
      // existing path, which the view continues to honor).
      rawLastWatched.value = 0
    } finally {
      loaded.value = true
    }
  }

  function reset() {
    loaded.value = false
    rawLastWatched.value = 0
  }

  // UA-110 (UX-07 Phase 3): clamp lastWatched at totalEpisodes. Without this,
  // out-of-sync data (e.g. ui_audit_bot seeded with 12 completed eps on a
  // single-episode anime) produced "Continue from ep 12" alongside the
  // "you finished, rewatch from ep 1" banner. Clamping here means every
  // downstream computed (kind, startEpisode, finishedEpisode) and every
  // consumer (Anime.vue: lastEpisode binding) sees a coherent value.
  const lastWatched = computed(() => {
    const total = inputs.totalEpisodes.value
    if (total > 0 && rawLastWatched.value > total) return total
    return rawLastWatched.value
  })

  // Parsed nextEpisodeAt (ms). NaN when absent/unparseable.
  const airTimeMs = computed(() => {
    const nextAt = inputs.nextEpisodeAt.value
    if (!nextAt) return Number.NaN
    return new Date(nextAt).getTime()
  })

  const kind = computed<ResumeKind>(() => {
    const total = inputs.totalEpisodes.value
    // `episodesAired` (Shikimori) lags reality for hours after an episode airs.
    // The active player's real loaded-episode count is authoritative for
    // availability, so take the max: if our sources actually have ep N, ep N is
    // "aired" for resume purposes regardless of Shikimori's slow metadata.
    const aired = Math.max(inputs.episodesAired.value, inputs.loadedEpisodes?.value ?? 0)
    const last = lastWatched.value
    const status = inputs.status.value

    if (last <= 0) return 'first-time'

    // Finished: user watched every aired episode AND total is known.
    if (total > 0 && last >= total) return 'finished'

    // Determine whether ep last+1 is available. Three ways to tell:
    //   1. anime.status is 'released' / 'completed' — all eps exist.
    //   2. effective aired count > last — the next episode has been released
    //      (per Shikimori OR already loaded into our sources).
    const isReleased = status === 'released' || status === 'completed'
    const nextIsAired = aired > 0 && aired > last
    if (isReleased || nextIsAired) return 'watching'

    // Ongoing series, our catalog hasn't recorded the next episode as aired.
    // The announced air time tells us which side of the line we're on:
    //   - air time in the PAST  → the episode HAS aired; it's just not loaded
    //     into our catalog/sources yet (episodes_aired lag, provider delay).
    //   - air time in the FUTURE (or unknown) → it genuinely hasn't aired.
    // We must never tell the user an episode whose air time has passed "hasn't
    // aired" — that's demonstrably false.
    const t = airTimeMs.value
    if (status === 'ongoing' && !Number.isNaN(t) && t <= nowMs.value) {
      return 'episode-not-loaded-yet'
    }
    return 'not-yet-aired'
  })

  // For episode-not-loaded-yet: how long ago the episode aired (ms), reactive
  // via nowMs. The consumer turns this into a localized "aired N ago" label.
  const episodeAiredAgoMs = computed(() => {
    if (kind.value !== 'episode-not-loaded-yet') return 0
    const t = airTimeMs.value
    return Number.isNaN(t) ? 0 : Math.max(0, nowMs.value - t)
  })

  const startEpisode = computed(() => {
    const k = kind.value
    if (k === 'first-time') return 1
    if (k === 'watching') return Math.min(lastWatched.value + 1, inputs.totalEpisodes.value || lastWatched.value + 1)
    // finished / not-yet-aired / episode-not-loaded-yet → re-mount on the last
    // watched episode (rewatch lands here, status banners show alongside).
    return Math.max(lastWatched.value, 1)
  })

  const finishedEpisode = computed(() => {
    if (kind.value === 'watching' || kind.value === 'finished') return lastWatched.value
    return 0
  })

  return { loaded, lastWatched, kind, episodeAiredAgoMs, startEpisode, finishedEpisode, init, reset }
}
