import { ref, computed, onScopeDispose, type Ref, type ComputedRef } from 'vue'
import { userApi } from '@/api/client'

/**
 * Grace window for the "episode-not-loaded-yet" hint. Once an episode's
 * announced air time passes we KNOW it aired — it's just not loaded into our
 * catalog/sources yet (Shikimori episodes_aired lag, provider delay). Within
 * this window we tell the user it usually shows up within the hour; past it the
 * load is clearly delayed (or the airing data is stale — hiatus, a
 * silently-ended show still flagged 'ongoing', scheduler lag), so we switch to
 * a softer copy that makes no time promise. Either way we never claim the
 * episode hasn't aired — its air time is demonstrably in the past.
 */
const EPISODE_LOAD_GRACE_MS = 3 * 60 * 60 * 1000 // 3h

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
 *   - episode-not-loaded-yet — last < total, nextEpisodeAt is in the PAST (so the
 *                      episode HAS aired) but episodesAired hasn't caught up yet:
 *                      it aired but isn't loaded into our catalog/sources yet.
 *                      `episodeLoadDelayed` distinguishes "just aired, loading"
 *                      from "aired a while ago, still missing". Same startEpisode
 *                      as not-yet-aired.
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
}

export interface ResumeStateMachine {
  /** True once init() has fetched server-side progress (or skipped — anon). */
  loaded: Ref<boolean>
  /** Highest completed episode for this user, clamped at totalEpisodes; 0 if none. */
  lastWatched: ComputedRef<number>
  /** Computed state. */
  kind: ComputedRef<ResumeKind>
  /**
   * For `episode-not-loaded-yet`: true once the load has run past the grace
   * window (aired a while ago, still not in our sources) — drives the softer,
   * no-time-promise copy. Always false for other states.
   */
  episodeLoadDelayed: ComputedRef<boolean>
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
  // episode-not-loaded-yet in-session, and the load-delayed copy never kicked
  // in. Ticking a ref once a minute makes `kind` / `episodeLoadDelayed`
  // recompute on a coarse-but-free cadence, plenty for an hours-wide window.
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

  // Parsed nextEpisodeAt (ms). NaN when absent/unparseable.
  const airTimeMs = computed(() => {
    const nextAt = inputs.nextEpisodeAt.value
    if (!nextAt) return Number.NaN
    return new Date(nextAt).getTime()
  })

  const kind = computed<ResumeKind>(() => {
    const total = inputs.totalEpisodes.value
    const aired = inputs.episodesAired.value
    const last = lastWatched.value
    const status = inputs.status.value

    if (last <= 0) return 'first-time'

    // Finished: user watched every aired episode AND total is known.
    if (total > 0 && last >= total) return 'finished'

    // Determine whether ep last+1 is available. Two ways to tell:
    //   1. anime.status is 'released' / 'completed' — all eps exist.
    //   2. anime.episodesAired > last — the next episode has been released.
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

  // For episode-not-loaded-yet: has the load run past the grace window? Beyond
  // it the "usually within the hour" promise no longer holds, so the consumer
  // shows a softer "still loading / delayed" copy instead.
  const episodeLoadDelayed = computed(() => {
    if (kind.value !== 'episode-not-loaded-yet') return false
    const t = airTimeMs.value
    return !Number.isNaN(t) && nowMs.value - t >= EPISODE_LOAD_GRACE_MS
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

  return { loaded, lastWatched, kind, episodeLoadDelayed, startEpisode, finishedEpisode, init, reset }
}
