import { ref, computed, onScopeDispose, type Ref, type ComputedRef } from 'vue'
import { userApi } from '@/api/client'

/**
 * Grace window for the "currently airing" hint. The episode's announced air
 * time has passed but our catalog hasn't recorded it as aired yet — a real but
 * SHORT data-lag window. Past this, the airing data is stale/stuck (hiatus, a
 * silently-ended show whose status is still 'ongoing', or scheduler lag) and we
 * must not keep promising the episode "appears within an hour" indefinitely, so
 * we degrade to not-yet-aired (which renders the honest "not yet available").
 */
const CURRENTLY_AIRING_WINDOW_MS = 6 * 60 * 60 * 1000 // 6h

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
 *   - not-yet-aired  — last < total but the next episode hasn't aired yet
 *                      (anime.status='ongoing' AND nextEpisodeAt > now AND
 *                      episodesAired <= last). Player still mounts on
 *                      startEpisode=last so the user can rewatch.
 *   - currently-airing — last < total, nextEpisodeAt has passed but
 *                        episodesAired hasn't caught up yet (data lag). Same
 *                        startEpisode as not-yet-aired.
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
  | 'currently-airing'

export interface ResumeStateInputs {
  animeId: Ref<string>
  totalEpisodes: Ref<number>
  episodesAired: Ref<number>
  nextEpisodeAt: Ref<string | undefined>
  status: Ref<string>
  isAuthenticated: Ref<boolean>
}

export interface ResumeStateMachine {
  /** True once init() has fetched server-side progress (or skipped — anon). */
  loaded: Ref<boolean>
  /** Highest completed episode for this user, clamped at totalEpisodes; 0 if none. */
  lastWatched: ComputedRef<number>
  /** Computed state. */
  kind: ComputedRef<ResumeKind>
  /** Episode the player should mount on. */
  startEpisode: ComputedRef<number>
  /** Episode the user just finished — drives the breadcrumb. 0 when N/A. */
  finishedEpisode: ComputedRef<number>
  /** init() — fetch watch_progress and populate lastWatched. */
  init: () => Promise<void>
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
  // currently-airing in-session, and currently-airing never expired. Ticking a
  // ref once a minute makes `kind` recompute on a coarse-but-free cadence,
  // which is plenty for an hours-wide grace window.
  const nowMs = ref(Date.now())
  if (typeof window !== 'undefined') {
    const timer = window.setInterval(() => {
      nowMs.value = Date.now()
    }, 60_000)
    onScopeDispose(() => window.clearInterval(timer))
  }

  async function init() {
    loaded.value = false
    if (!inputs.isAuthenticated.value) {
      rawLastWatched.value = 0
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

  const kind = computed<ResumeKind>(() => {
    const total = inputs.totalEpisodes.value
    const aired = inputs.episodesAired.value
    const last = lastWatched.value
    const status = inputs.status.value
    const nextAt = inputs.nextEpisodeAt.value

    if (last <= 0) return 'first-time'

    // Finished: user watched every aired episode AND total is known.
    if (total > 0 && last >= total) return 'finished'

    // Determine whether ep last+1 is available. Two ways to tell:
    //   1. anime.status is 'released' / 'completed' — all eps exist.
    //   2. anime.episodesAired > last — the next episode has been released.
    const isReleased = status === 'released' || status === 'completed'
    const nextIsAired = aired > 0 && aired > last
    if (isReleased || nextIsAired) return 'watching'

    // Ongoing series, next ep not yet aired. Pick which copy to show based on
    // whether the announced air time has passed — but ONLY treat it as
    // "currently airing" within a short grace window. A nextEpisodeAt that
    // passed long ago is stale/stuck airing data (hiatus, silently-ended show,
    // scheduler lag); keeping the "appears within an hour" promise alive
    // indefinitely would be a lie, so we fall through to not-yet-aired.
    if (status === 'ongoing' && nextAt) {
      const t = new Date(nextAt).getTime()
      if (!Number.isNaN(t)) {
        const elapsed = nowMs.value - t
        if (elapsed >= 0 && elapsed < CURRENTLY_AIRING_WINDOW_MS) return 'currently-airing'
      }
    }
    return 'not-yet-aired'
  })

  const startEpisode = computed(() => {
    const k = kind.value
    if (k === 'first-time') return 1
    if (k === 'watching') return Math.min(lastWatched.value + 1, inputs.totalEpisodes.value || lastWatched.value + 1)
    // finished / not-yet-aired / currently-airing → re-mount on the last
    // watched episode (rewatch lands here, ETA banners show alongside).
    return Math.max(lastWatched.value, 1)
  })

  const finishedEpisode = computed(() => {
    if (kind.value === 'watching' || kind.value === 'finished') return lastWatched.value
    return 0
  })

  return { loaded, lastWatched, kind, startEpisode, finishedEpisode, init, reset }
}
