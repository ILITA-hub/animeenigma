import type { AudioKind, ContentKind, TrackLang, ProviderGroup } from '@/types/aePlayer'
import type { ProviderCap } from '@/types/capabilities'

/** The audio/lang/provider a `?provider=` deep-link should pin the player to. */
export interface DeepLinkPin {
  provider: string
  audio: AudioKind
  lang: TrackLang
}

// Backend group → which (lang, content) it serves. Mirrors useProviderFeed's
// GROUP_LANGS/GROUP_CONTENT so the clamp stays consistent with the relevance
// filter (a pinned row must become relevant under the clamped facet).
const GROUP_LANGS: Record<ProviderGroup, TrackLang[]> = {
  en: ['en'], ru: ['ru'], adult: ['en', 'ru'], jp: ['ja'], firstparty: ['en', 'ru', 'ja'],
}
const GROUP_CONTENT: Record<ProviderGroup, ContentKind[]> = {
  en: ['common'], ru: ['common'], adult: ['hentai'], jp: ['common'], firstparty: ['common'],
}

/**
 * Resolve a notification / shared-link `?provider=` value into a concrete
 * provider PIN, clamping the current audio/lang to what that provider actually
 * serves — read straight from the backend capability feed (single source of
 * truth), no FE registry.
 *
 * WHY the clamp: a provider row is only relevant (and therefore pinnable) when
 * it matches the live audio/lang/content filter (see rowsFromReport). The
 * default combo is `sub`/`en`, so a `?provider=kodik` deep-link (kodik is RU)
 * would otherwise be filtered out and the pin would silently fall through to the
 * smart default. Switching lang→ru (and audio to a supported kind) makes the row
 * relevant so the explicit choice is honored.
 *
 * Honored ONLY when the id names a provider present in the feed (disabled
 * providers are omitted backend-side) that is content-compatible and serves at
 * least one audio kind. Coarse/unknown values ('english'), 18+ sources on a
 * common title, and content-incompatible providers return null so the smart
 * default picks instead. Pure + sync so it is unit-testable without mounting.
 */
export function resolveDeepLinkProvider(
  providerId: string | undefined | null,
  current: { audio: AudioKind; lang: TrackLang },
  content: ContentKind,
  capMap: Map<string, ProviderCap>,
): DeepLinkPin | null {
  if (!providerId) return null
  const cap = capMap.get(providerId)
  if (!cap) return null
  const group = cap.group
  if (!GROUP_CONTENT[group]?.includes(content)) return null
  const langs = GROUP_LANGS[group] ?? []
  // Audio kinds the provider serves, restricted to pickable sub/dub.
  const audios = cap.audios.filter((a): a is AudioKind => a === 'sub' || a === 'dub')
  if (langs.length === 0 || audios.length === 0) return null
  // Keep the current facet when the provider supports it; otherwise fall to the
  // provider's first declared lang/audio so the row becomes relevant.
  const lang = langs.includes(current.lang) ? current.lang : langs[0]
  const audio = audios.includes(current.audio) ? current.audio : audios[0]
  return { provider: cap.provider, audio, lang }
}
