import type { WatchCombo, ResolvedCombo } from '@/types/preference'
import type { Combo, AudioKind, TrackLang } from '@/types/aePlayer'

type LegacyPlayer = WatchCombo['player']

/** Map a granular unified provider id -> coarse legacy WatchCombo.player (or null).
 *  EN-chain membership is backend-driven: pass the provider's capability `group`
 *  (from groupOfProvider) — group 'en' ⇒ 'english'. The remaining single-provider
 *  families stay keyed on id. */
export function providerToLegacyPlayer(providerId: string, group?: string): LegacyPlayer | null {
  if (group === 'en') return 'english'
  switch (providerId) {
    case 'kodik': return 'kodik'
    case 'ae': return 'ae'
    case '18anime': return 'hanime'
    case 'hanime': return 'hanime'
    case 'animelib': return 'animelib'
    default: return null
  }
}

const langToLanguage: Record<TrackLang, WatchCombo['language']> = { en: 'en', ru: 'ru', ja: 'ja' }
const languageToLang: Partial<Record<WatchCombo['language'], TrackLang>> = { en: 'en', ru: 'ru', ja: 'ja', '18+': 'en' }

/** There is no Japanese dub — the DUB language facet is EN/RU only. Clamp a
 *  dub/ja combo to dub/en. Applied on every audio/lang entry point (saved-combo
 *  restore, URL facet, the audio slider) so the rule lives in one place. */
export function clampLangForAudio(audio: AudioKind, lang: TrackLang): TrackLang {
  return audio === 'dub' && lang === 'ja' ? 'en' : lang
}

/** Map a unified Combo -> legacy WatchCombo for persistence/resolve. Null if provider unmappable. */
export function comboToWatchCombo(combo: Combo, group?: string): WatchCombo | null {
  const player = providerToLegacyPlayer(combo.provider, group)
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

/** The 5 combo fields carried opaquely in a Watch-Together room's
 *  `translation_id` so every room member resolves the SAME stream. */
export interface WtComboFields {
  audio: AudioKind
  lang: TrackLang
  team: string | null
  provider: string
  server: string
}

/** Serialize a combo into the opaque WT room token (carried in `translation_id`). */
export function comboToToken(c: WtComboFields): string {
  return JSON.stringify({
    provider: c.provider,
    audio: c.audio,
    lang: c.lang,
    team: c.team ?? null,
    server: c.server,
  })
}

/** Parse a WT room token back into combo fields. Returns null on parse error
 *  or when `provider` is not a string. Missing team coerces to null, missing
 *  server to ''. */
export function tokenToCombo(token: string): WtComboFields | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(token)
  } catch {
    return null
  }
  if (typeof parsed !== 'object' || parsed === null) return null
  const o = parsed as Record<string, unknown>
  if (typeof o.provider !== 'string') return null
  return {
    provider: o.provider,
    audio: o.audio as AudioKind,
    lang: o.lang as TrackLang,
    team: (o.team ?? null) as string | null,
    server: typeof o.server === 'string' ? o.server : '',
  }
}
