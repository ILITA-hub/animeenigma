// Pure helpers for the aePlayer episode-progress drawer + resume chip. No Vue
// reactivity in here — deterministic on inputs, kept pure for direct unit
// testing and to keep AePlayer.vue's <script setup> focused on orchestration.

/** A per-episode watch-progress row as returned by the progress API / viewer
 *  context aggregate. Structurally identical to the store's ViewerProgressRow,
 *  so rows from either source feed `progressRowsToMap` unchanged. */
export interface ProgressRow {
  episode_number?: number
  progress?: number
  duration?: number
  completed?: boolean
}

/** Per-episode progress, keyed by episode number. */
export interface EpisodeProgress {
  pct: number
  sec: number
  completed: boolean
}

/** Fold a list of progress rows into a by-episode-number map. Rows without an
 *  episode number are skipped; pct is clamped to [0, 1] and 0 when duration is
 *  unknown. */
export function progressRowsToMap(rows: ProgressRow[]): Record<number, EpisodeProgress> {
  const map: Record<number, EpisodeProgress> = {}
  for (const r of rows) {
    if (!r.episode_number) continue
    map[r.episode_number] = {
      pct: r.duration ? Math.min(1, (r.progress ?? 0) / r.duration) : 0,
      sec: r.progress ?? 0,
      completed: !!r.completed,
    }
  }
  return map
}

/** Format a position (seconds) as the resume chip's `m:ss` label. */
export function fmtResume(s: number): string {
  const m = Math.floor(s / 60)
  const sec = Math.floor(s % 60)
  return `${m}:${sec.toString().padStart(2, '0')}`
}
