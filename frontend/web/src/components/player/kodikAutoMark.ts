// Pure auto-mark-threshold calculation for KodikPlayer.vue, extracted for
// unit-testability. Kodik's iframe never exposes an HTML5 <video> element, so
// there is no ground-truth "real duration" available to the player the way
// AePlayer's useWatchTracking gets it for free — only two duration sources
// exist: the catalog's `episode_duration` (Shikimori-sourced nominal minutes,
// sometimes longer than the actual streamed cut — e.g. short-form anime
// listed at a rounded 24 min while the real episode runs ~18 min) and Kodik's
// own `kodik_player_duration_update` postMessage event (the real per-episode
// video length, in seconds). Prefer the live Kodik signal when present.
export const AUTO_MARK_FALLBACK = 20 * 60 // legacy threshold when no duration is known
export const COMPLETE_RATIO = 0.9

export function computeAutoMarkThreshold(catalogDurationMin: number, kodikDurationSec: number): number {
  if (kodikDurationSec > 0) {
    return Math.min(AUTO_MARK_FALLBACK, Math.round(COMPLETE_RATIO * kodikDurationSec))
  }
  if (catalogDurationMin > 0) {
    return Math.min(AUTO_MARK_FALLBACK, Math.round(COMPLETE_RATIO * catalogDurationMin * 60))
  }
  return AUTO_MARK_FALLBACK
}
