export interface Interval { start: number; end: number }
export interface OffsetResult { offset: number; confidence: number }

export interface SyncEvent {
  delta: number          // offset change applied (s), signed
  confidence: number     // 0..1 of the chosen offset
  windowStart: number    // analyzed speech-window interval (s)
  windowEnd: number
  reason: 'lock' | 'resync'
}

/** Port shared by the engine and any audio tap (real Web Audio or a test fake). */
export interface SpeechTap {
  onFrame(cb: (mediaTime: number, speaking: boolean) => void): void
  dispose(): void
}

export const SEARCH = { min: -30, max: 30, step: 0.1 } as const

export function round1(v: number): number { return Math.round(v * 10) / 10 }

/** Sort by start and merge overlapping or touching intervals. */
export function cuesToIntervals(cues: { start: number; end: number }[]): Interval[] {
  const valid = cues.filter((c) => c.end > c.start).sort((a, b) => a.start - b.start)
  const out: Interval[] = []
  for (const c of valid) {
    const last = out[out.length - 1]
    if (last && c.start <= last.end) last.end = Math.max(last.end, c.end)
    else out.push({ start: c.start, end: c.end })
  }
  return out
}

/** Total intersection (seconds) of `a` with `b` shifted later by `delta`. Inputs sorted. */
export function overlapDuration(a: Interval[], b: Interval[], delta: number): number {
  let i = 0, j = 0, total = 0
  while (i < a.length && j < b.length) {
    const bs = b[j].start + delta, be = b[j].end + delta
    const hi = Math.min(a[i].end, be), lo = Math.max(a[i].start, bs)
    if (hi > lo) total += hi - lo
    if (a[i].end < be) i++; else j++
  }
  return total
}

// Reusable scratch for the offset sweep — the grid size is constant, so this
// avoids allocating a fresh array on every evaluate() call.
// bestOffset is therefore non-reentrant; safe today (single-threaded/synchronous).
const SCORES = new Float64Array(Math.round((SEARCH.max - SEARCH.min) / SEARCH.step) + 1)

/**
 * Slide cues against speech over the offset grid; return the offset maximizing
 * overlap. Confidence = prominence of the peak over the best competitor outside
 * a ±1s guard band, normalized by the peak (∈ [0,1] since 0 ≤ second ≤ peak).
 */
export function bestOffset(
  speech: Interval[],
  cues: Interval[],
  opts: { min?: number; max?: number; step?: number } = {},
): OffsetResult {
  if (!speech.length || !cues.length) return { offset: 0, confidence: 0 }
  const min = opts.min ?? SEARCH.min, max = opts.max ?? SEARCH.max, step = opts.step ?? SEARCH.step
  const n = Math.round((max - min) / step) + 1
  const scores = n === SCORES.length ? SCORES : new Float64Array(n)
  let peak = -1, peakIdx = 0
  for (let k = 0; k < n; k++) {
    const s = overlapDuration(speech, cues, min + k * step)
    scores[k] = s
    if (s > peak) { peak = s; peakIdx = k }
  }
  if (peak <= 0) return { offset: 0, confidence: 0 }
  const guard = Math.round(1 / step)
  let second = 0
  for (let k = 0; k < n; k++) {
    if (Math.abs(k - peakIdx) <= guard) continue
    if (scores[k] > second) second = scores[k]
  }
  return { offset: round1(min + peakIdx * step), confidence: (peak - second) / peak }
}

/**
 * Pure VAD frame decision in a single pass over the bins: speaking when mean byte
 * energy ≥ floor AND the fraction of energy in the human speech band (≈300-3400Hz)
 * ≥ vadRatio. v1 uses a fixed floor (an adaptive running-percentile floor can later
 * replace `energyMin` here without touching the tap).
 */
export function classifyFrame(
  freq: Uint8Array,
  sampleRate: number,
  fftSize: number,
  opts: { vadRatio?: number; energyMin?: number } = {},
): boolean {
  const vadRatio = opts.vadRatio ?? 0.55
  const energyMin = opts.energyMin ?? 12
  const binHz = sampleRate / fftSize
  const lo = Math.floor(300 / binHz), hi = Math.min(freq.length - 1, Math.ceil(3400 / binHz))
  let band = 0, total = 0
  for (let i = 0; i < freq.length; i++) {
    total += freq[i]
    if (i >= lo && i <= hi) band += freq[i]
  }
  const mean = freq.length ? total / freq.length : 0
  const ratio = total > 0 ? band / total : 0
  return mean >= energyMin && ratio >= vadRatio
}
