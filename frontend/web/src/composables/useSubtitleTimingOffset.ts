import { ref, watch } from 'vue'

const STORAGE_KEY = 'subtitle_timing_offset'
const MIN = -30
const MAX = 30

function clamp(v: number): number {
  return Math.min(MAX, Math.max(MIN, v))
}

function round1(v: number): number {
  return Math.round(v * 10) / 10
}

function load(): number {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === null) return 0
    const raw = Number(stored)
    return Number.isFinite(raw) ? clamp(round1(raw)) : 0
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
    try {
      localStorage.setItem(STORAGE_KEY, String(v))
    } catch {
      /* ignore quota / storage-disabled */
    }
  },
  { flush: 'sync' },
)

export function useSubtitleTimingOffset() {
  function nudge(delta: number): void {
    offset.value = clamp(round1(offset.value + delta))
  }
  function reset(): void {
    offset.value = 0
  }
  return { offset, nudge, reset }
}
