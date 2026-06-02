import { ref, watch } from 'vue'

const STORAGE_KEY = 'aenigma_subtitle_timing_offset'
export const MIN = -30
export const MAX = 30

function clamp(v: number): number {
  return Math.min(MAX, Math.max(MIN, v))
}

function roundTo1Decimal(v: number): number {
  return Math.round(v * 10) / 10
}

function load(): number {
  if (typeof window === 'undefined') return 0
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === null) return 0
    const raw = Number(stored)
    return Number.isFinite(raw) ? clamp(roundTo1Decimal(raw)) : 0
  } catch {
    return 0
  }
}

// Module-level singleton: every caller of useSubtitleTimingOffset() shares this
// reactive ref, so the menu and the player overlays stay in sync.
const offset = ref<number>(load())

// flush: 'sync' makes persistence deterministic (tests + no lost writes on unmount).
watch(
  offset,
  (v) => {
    if (typeof window === 'undefined') return
    try {
      localStorage.setItem(STORAGE_KEY, String(v))
    } catch {
      /* ignore quota / storage-disabled */
    }
  },
  { flush: 'sync' },
)

// Cross-tab sync: if the user changes the offset in another tab, reflect it
// here too. `storage` events don't fire in the same tab that wrote the value,
// so no deduplication is needed.
if (typeof window !== 'undefined') {
  window.addEventListener('storage', (e: StorageEvent) => {
    if (e.key !== STORAGE_KEY) return
    const raw = Number(e.newValue)
    offset.value = Number.isFinite(raw) ? clamp(roundTo1Decimal(raw)) : 0
  })
}

export function useSubtitleTimingOffset() {
  function nudge(delta: number): void {
    offset.value = clamp(roundTo1Decimal(offset.value + delta))
  }
  function reset(): void {
    offset.value = 0
  }
  return { offset, nudge, reset }
}
