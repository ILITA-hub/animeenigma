import type { WatchCombo, ResolvedCombo } from '@/types/preference'
import type { Combo, AudioKind, TrackLang } from '@/types/aePlayer'

type LegacyPlayer = WatchCombo['player']

// EN scraper chain -> coarse 'english'. Keep in sync with SCRAPER_IDS.
const EN_SCRAPER_IDS = new Set(['allanime', 'animepahe', 'gogoanime', 'nineanime', 'animefever', 'miruro'])

/** Map a granular unified provider id -> coarse legacy WatchCombo.player (or null if unmappable). */
export function providerToLegacyPlayer(providerId: string): LegacyPlayer | null {
  if (EN_SCRAPER_IDS.has(providerId)) return 'english'
  switch (providerId) {
    case 'kodik': return 'kodik'
    case 'raw': return 'raw'
    case 'ae': return 'ae'
    case '18anime': return 'hanime'
    case 'animelib': return 'animelib'
    case 'hanime': return 'hanime'
    default: return null
  }
}

const langToLanguage: Record<TrackLang, WatchCombo['language']> = { en: 'en', ru: 'ru', ja: 'ja' }
const languageToLang: Partial<Record<WatchCombo['language'], TrackLang>> = { en: 'en', ru: 'ru', ja: 'ja', '18+': 'en' }

/** Map a unified Combo -> legacy WatchCombo for persistence/resolve. Null if provider unmappable. */
export function comboToWatchCombo(combo: Combo): WatchCombo | null {
  const player = providerToLegacyPlayer(combo.provider)
  if (!player) return null
  return {
    player,
    language: langToLanguage[combo.lang],
    watch_type: combo.audio,
    translation_id: '',
    translation_title: combo.team ?? '',
  }
}

/** Map a resolved WatchCombo -> the unified fields it can restore (audio/lang/team).
 *  The provider id is NOT derivable from a coarse player and is chosen by the caller. */
export function watchComboToPartialCombo(rc: ResolvedCombo | WatchCombo): { audio: AudioKind; lang: TrackLang; team: string | null } {
  return {
    audio: rc.watch_type === 'dub' ? 'dub' : 'sub',
    lang: languageToLang[rc.language] ?? 'en',
    team: rc.translation_title ? rc.translation_title : null,
  }
}
