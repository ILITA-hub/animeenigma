import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { userApi } from '@/api/client'
import type { WatchCombo, ResolvedCombo } from '@/types/preference'

const CACHE_TTL = 24 * 60 * 60 * 1000 // 24 hours
const ANON_LAST_COMBO_KEY = 'anon_last_combo'

// Phase 7 SC2 — anonymous users get a localStorage-backed Tier 2 substitute:
// "language + watch_type + last-used team" from the most recent combo they
// played. setAnonLastCombo is called by player components on play; getAnonLastCombo
// is consulted by useWatchPreferences for anon users before falling through to
// the backend community resolver.
export function setAnonLastCombo(combo: WatchCombo) {
  try {
    localStorage.setItem(ANON_LAST_COMBO_KEY, JSON.stringify(combo))
  } catch { /* quota errors swallowed — non-essential */ }
}

export function getAnonLastCombo(): WatchCombo | null {
  const raw = localStorage.getItem(ANON_LAST_COMBO_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw) as WatchCombo
  } catch {
    return null
  }
}

// pickAnonLockMatch implements the same boundary discipline as the backend
// resolver Tier 2: only matches available combos that share BOTH language and
// watch_type with the anon user's last-used combo. Within the lock, prefers
// exact translation_title match (last-used team), then falls through to nil
// so the server-side community resolver picks the in-lock most-popular.
export function pickAnonLockMatch(
  last: WatchCombo | null,
  available: WatchCombo[],
): ResolvedCombo | null {
  if (!last || available.length === 0) return null
  const inLock = available.filter(
    a => a.language === last.language && a.watch_type === last.watch_type,
  )
  if (inLock.length === 0) return null

  const exact = inLock.find(
    a => a.translation_title === last.translation_title && a.player === last.player,
  )
  const titleMatch = inLock.find(a => a.translation_title === last.translation_title)
  const winner = exact ?? titleMatch
  if (!winner) return null

  return { ...winner, tier: 'user_global', tier_number: 2 } as ResolvedCombo
}

export function useWatchPreferences(animeId: string) {
  const auth = useAuthStore()
  const resolvedCombo = ref<ResolvedCombo | null>(null)
  const isLoading = ref(false)

  // Try cached result first
  const cacheKey = `pref:${animeId}`
  const cached = localStorage.getItem(cacheKey)
  if (cached) {
    try {
      const { data, timestamp } = JSON.parse(cached)
      if (Date.now() - timestamp < CACHE_TTL) {
        resolvedCombo.value = data
      }
    } catch { /* ignore corrupt cache */ }
  }

  async function resolve(available: WatchCombo[]) {
    // Anon users now hit /api/preferences/resolve — the axios interceptor attaches
    // X-Anon-ID and the backend OptionalAuthMiddleware allows the call through.
    // Per CONTEXT Critical Finding 3: required for D-12 (per-anon-user override rate).
    if (available.length === 0) return

    // Phase 7 SC2 — anon shortcut: try localStorage last-used combo first.
    // If it matches an available option respecting the language+watch_type lock,
    // use it and skip the round-trip. Backend still gets called below so the
    // override-rate metric stays accurate even when we picked client-side.
    if (!auth.isAuthenticated) {
      const localPick = pickAnonLockMatch(getAnonLastCombo(), available)
      if (localPick) {
        resolvedCombo.value = localPick
        localStorage.setItem(cacheKey, JSON.stringify({
          data: localPick,
          timestamp: Date.now(),
        }))
        // We still inform the backend so combo_resolve_total counts this anon
        // resolution — fire-and-forget, errors swallowed.
        userApi.resolvePreference(animeId, available).catch(() => undefined)
        return
      }
    }

    isLoading.value = true
    try {
      const { data } = await userApi.resolvePreference(animeId, available)
      // Backend wraps responses via httputil.OK as { success, data: { resolved } } —
      // unwrap the `data` envelope. Be lenient on any callsite that already returns
      // the inner shape.
      const envelope = (data as { data?: { resolved: ResolvedCombo | null } }).data ?? data
      resolvedCombo.value = envelope.resolved ?? null
      localStorage.setItem(cacheKey, JSON.stringify({
        data: envelope.resolved ?? null,
        timestamp: Date.now()
      }))
      // Phase 7 SC2 — first-time anon viewer who fell through to community
      // gets that combo recorded so the next visit can pick it client-side
      // before the round-trip.
      if (!auth.isAuthenticated && envelope.resolved) {
        setAnonLastCombo(envelope.resolved as WatchCombo)
      }
    } catch (err) {
      console.error('Failed to resolve preference:', err)
    } finally {
      isLoading.value = false
    }
  }

  return { resolvedCombo, isLoading, resolve }
}
