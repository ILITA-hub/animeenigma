/**
 * useOverrideTracker — single source of truth for "user overrode the auto-pick."
 *
 * Detects user-initiated combo changes within 30s of player load and POSTs them to
 * /api/preferences/override. The composable is consumed by KodikPlayer, AnimeLibPlayer,
 * HiAnimePlayer, ConsumetPlayer (per-player instance for episode/team/language) plus
 * a separate instance in Anime.vue (for player-dimension switches).
 *
 * Key invariants enforced here:
 *   - First user-initiated change per (load_session_id, dimension) only — D-07.
 *   - Auto-advance / scrubbing / pause is NOT counted (gates on explicit
 *     recordPickerEvent calls only — NOT on prop watches) — D-08.
 *   - Window starts when resolvedCombo applies to player props, not on mount — D-10.
 *   - Re-mounting (anime change) mints a new load_session_id — D-09.
 *   - Best-effort: never throws to caller, never blocks UX — instrumentation only.
 *
 * See .planning/phases/01-instrumentation-baseline/01-RESEARCH.md §Pattern 1.
 */

import { ref, watch, onUnmounted, type Ref } from 'vue'
import { userApi } from '@/api/client'
import type { WatchCombo, ResolvedCombo } from '@/types/preference'

export type OverrideDimension = 'language' | 'player' | 'team' | 'episode'

export type PlayerName = 'kodik' | 'animelib' | 'hianime' | 'consumet'

// We accept WatchCombo (the prop shape players hold) rather than the stricter
// ResolvedCombo. The tier/tier_number fields are optional — the composable
// reads them via a safe `(combo as Partial<ResolvedCombo>).tier` cast at emit
// time. This unblocks all four players + Anime.vue, which only retain
// WatchCombo on their props/state, not the full ResolvedCombo.
//
// `null | undefined` allows direct passing of `toRef(props, 'preferredCombo')`
// where the prop is declared as `preferredCombo?: WatchCombo | null` (optional
// → ref includes undefined).
export interface OverrideTrackerOptions {
  animeId: string
  player: PlayerName
  resolvedCombo: Ref<WatchCombo | null | undefined>
  currentEpisode: Ref<number>
}

const WINDOW_MS = 30_000
const DEBOUNCE_MS = 250

export function useOverrideTracker(opts: OverrideTrackerOptions) {
  const loadSessionId = crypto.randomUUID()
  const mountedAt = ref<number | null>(null)
  const emittedDimensions = new Set<OverrideDimension>()
  const debounceTimers = new Map<OverrideDimension, number>()

  // D-10: window opens when resolvedCombo first transitions to a non-null value
  // (i.e. the auto-pick was applied to the player props). NOT on mount, because
  // a slow resolve would leave the user with less than 30s and could record
  // pre-resolve clicks as overrides.
  const stopWatch = watch(
    () => opts.resolvedCombo.value,
    (combo) => {
      if (combo && mountedAt.value === null) {
        mountedAt.value = performance.now()
      }
    },
    { immediate: true },
  )

  function recordPickerEvent(
    dimension: OverrideDimension,
    newCombo: Partial<WatchCombo> & { episode?: number },
  ): void {
    // Window not yet open (resolvedCombo hasn't applied) → drop.
    if (mountedAt.value === null) return

    // Past 30s window → drop.
    const msSinceLoad = performance.now() - mountedAt.value
    if (msSinceLoad > WINDOW_MS) return

    // Already emitted for this dimension this session → drop (D-07).
    if (emittedDimensions.has(dimension)) return

    // Debounce: coalesce two rapid clicks on the same dimension within 250ms.
    const existing = debounceTimers.get(dimension)
    if (existing !== undefined) window.clearTimeout(existing)

    debounceTimers.set(
      dimension,
      window.setTimeout(() => {
        // Lock pattern: mark as emitted BEFORE the awaited POST so a second
        // click that lands during the network round-trip is also dropped.
        emittedDimensions.add(dimension)
        void emit(dimension, newCombo, msSinceLoad)
      }, DEBOUNCE_MS),
    )
  }

  async function emit(
    dimension: OverrideDimension,
    newCombo: Partial<WatchCombo> & { episode?: number },
    msSinceLoad: number,
  ): Promise<void> {
    try {
      // tier/tier_number are present on ResolvedCombo (a superset of WatchCombo).
      // The prop is typed as WatchCombo, so we read those fields via a partial
      // cast — they're echoed to the backend for label hygiene and may be null.
      const original = opts.resolvedCombo.value ?? null
      const resolved = original as Partial<ResolvedCombo> | null
      await userApi.recordOverride({
        anime_id: opts.animeId,
        load_session_id: loadSessionId,
        dimension,
        original_combo: original as ResolvedCombo | null,
        new_combo: newCombo,
        ms_since_load: Math.round(msSinceLoad),
        tier: resolved?.tier ?? null,
        tier_number: resolved?.tier_number ?? null,
        player: opts.player,
      })
    } catch {
      // Best-effort instrumentation: never throw, never block UX. Counter loss
      // is acceptable; this is monitoring, not business logic.
    }
  }

  onUnmounted(() => {
    debounceTimers.forEach((id) => window.clearTimeout(id))
    debounceTimers.clear()
    stopWatch()
  })

  return { recordPickerEvent, loadSessionId }
}
