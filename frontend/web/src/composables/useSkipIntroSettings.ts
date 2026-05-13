// Phase 20 — Skip-Intro CTA auto-dismiss setting.
//
// The Skip-Intro / Skip-Outro CTAs (Phase 18 / UX-34) in HiAnimePlayer.vue +
// ConsumetPlayer.vue stay visible for the entire OP/ED window (typically
// ~90 s). Users who deliberately want to watch the OP find the CTA visually
// noisy. This composable exposes a localStorage-backed reactive `seconds`
// value (default 8) that the players use to auto-dismiss the CTA after a
// short, configurable timeout once it first appears for an OP/ED window.
//
// Persistence: localStorage key `aenigma_skip_intro_dismiss_sec`. No backend
// — this is a purely client-side polish setting, mirroring how Theater Mode
// (UX-23) is persisted. Min 2, max 60. Out-of-range values fall back to 8.
//
// Reactivity model: one source of truth across all callers. Profile.vue
// writes via `set`; players read via the same `seconds` ref. We don't use
// `useStorage` from VueUse to keep the bundle small and the behavior
// explicit (range clamping + cross-tab sync).

import { ref } from 'vue'

const STORAGE_KEY = 'aenigma_skip_intro_dismiss_sec'
const DEFAULT_SEC = 8
const MIN_SEC = 2
const MAX_SEC = 60

function clamp(n: number): number {
  if (!Number.isFinite(n)) return DEFAULT_SEC
  if (n < MIN_SEC) return MIN_SEC
  if (n > MAX_SEC) return MAX_SEC
  return Math.round(n)
}

function readFromStorage(): number {
  if (typeof window === 'undefined') return DEFAULT_SEC
  const raw = window.localStorage.getItem(STORAGE_KEY)
  if (raw == null) return DEFAULT_SEC
  const parsed = Number(raw)
  return clamp(parsed)
}

// Module-level singleton — all `useSkipIntroSettings()` callers share the
// same reactive ref so Profile changes propagate to every active player
// instance without a manual refetch.
const seconds = ref<number>(readFromStorage())

// Cross-tab sync: if the user opens the setting in another tab and edits it,
// reflect the change in this tab too. `storage` events don't fire in the
// same tab that wrote the value, so we don't need to dedupe.
if (typeof window !== 'undefined') {
  window.addEventListener('storage', (e: StorageEvent) => {
    if (e.key !== STORAGE_KEY) return
    seconds.value = readFromStorage()
  })
}

function set(value: number): void {
  const clamped = clamp(value)
  seconds.value = clamped
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(STORAGE_KEY, String(clamped))
  }
}

export function useSkipIntroSettings() {
  return {
    seconds,
    set,
    MIN_SEC,
    MAX_SEC,
    DEFAULT_SEC,
  }
}
