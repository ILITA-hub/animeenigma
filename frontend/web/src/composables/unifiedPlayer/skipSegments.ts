// Pure mapping helpers between AniSkip segments (useSkipTimes) and the
// unified player's scrub-bar chapters / skip-chip state. Kept pure for
// direct unit testing — no Vue reactivity in here.

import type { SkipSegment } from '@/composables/useSkipTimes'

export interface Chapter {
  kind: 'intro' | 'outro'
  startPct: number
  widthPct: number
}

/** Map AniSkip segments to scrub-bar chapter markers. Returns [] until the
 *  real duration is known (no fake markers — design rule F6). */
export function segmentsToChapters(
  opening: SkipSegment | null,
  ending: SkipSegment | null,
  durationSec: number,
): Chapter[] {
  if (!durationSec || durationSec <= 0) return []
  const out: Chapter[] = []
  const push = (kind: Chapter['kind'], seg: SkipSegment) => {
    const start = Math.max(0, Math.min(seg.start, durationSec))
    const end = Math.max(start, Math.min(seg.end, durationSec))
    if (end - start < 1) return
    out.push({
      kind,
      startPct: (start / durationSec) * 100,
      widthPct: ((end - start) / durationSec) * 100,
    })
  }
  if (opening) push('intro', opening)
  if (ending) push('outro', ending)
  return out
}

/** The segment the skip chip should offer at time t, if any. Stops offering
 *  1s before the segment end so the chip never seeks ~nowhere. */
export function activeSkipSegment(
  t: number,
  opening: SkipSegment | null,
  ending: SkipSegment | null,
): { kind: 'intro' | 'outro'; end: number } | null {
  if (opening && t >= opening.start && t < opening.end - 1) {
    return { kind: 'intro', end: opening.end }
  }
  if (ending && t >= ending.start && t < ending.end - 1) {
    return { kind: 'outro', end: ending.end }
  }
  return null
}
