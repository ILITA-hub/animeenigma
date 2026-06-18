// Watch-page player-preference normalization.
//
// Plan B retires the per-language tabs + provider sub-tabs: AePlayer is now the
// DEFAULT mounted player and "Classic Kodik" (the iframe `KodikPlayer`) is the
// opt-in fallback. This helper migrates users who still carry the OLD
// localStorage shape (`unified_player_selected` / `preferred_video_provider` /
// `preferred_video_language`) onto the single new boolean
// (`classic_kodik_selected`) WITHOUT booting anyone into a now-deleted player.

export interface PlayerPref {
  /** When true the page mounts the Classic Kodik iframe instead of AePlayer. */
  classicKodik: boolean
}

/** localStorage keys this helper reads/normalizes. */
export const CLASSIC_KODIK_KEY = 'classic_kodik_selected'

type LocalStorageShape = Record<string, string | null | undefined>

function isTruthy(v: string | null | undefined): boolean {
  return v === '1' || v === 'true'
}

/**
 * Decide the initial player surface from a (possibly legacy) localStorage map.
 *
 * Decision order (most → least authoritative):
 *   1. New `classic_kodik_selected` present → use it verbatim.
 *   2. Legacy `unified_player_selected` truthy → AePlayer (classicKodik:false).
 *      The retire-direction is "AePlayer is the new home", so an explicit
 *      AnimeEnigma/unified selection stays on AePlayer.
 *   3. Legacy `preferred_video_provider === 'kodik'` (and NOT unified) →
 *      classicKodik:true — the user last watched on the iframe Kodik surface,
 *      which survives as the Classic Kodik fallback.
 *   4. Anything else — a retired provider value (animelib/ourenglish/hanime/
 *      raw/kodik-adfree/anime18), an empty/absent store, or nulls → AePlayer
 *      default (the deleted players no longer exist to boot into).
 */
export function resolveInitialPlayerPref(ls: LocalStorageShape): PlayerPref {
  const newKey = ls[CLASSIC_KODIK_KEY]
  if (newKey !== undefined && newKey !== null) {
    return { classicKodik: isTruthy(newKey) }
  }
  if (isTruthy(ls.unified_player_selected)) {
    return { classicKodik: false }
  }
  if (ls.preferred_video_provider === 'kodik') {
    return { classicKodik: true }
  }
  return { classicKodik: false }
}
