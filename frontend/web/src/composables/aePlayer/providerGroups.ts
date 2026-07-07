import type { TrackLang, ContentKind, ProviderGroup } from '@/types/aePlayer'

// Backend wire `group` → the (lang, content) facets that group serves. Single
// FE source of truth: the relevance filter (useProviderFeed), the deep-link
// clamp (deepLinkProvider), and AePlayer's served-lang derivation all import
// these, so adding a `ProviderGroup` updates one table, not three. The backend
// owns `group`; mapping a group to its served facets is the only provider
// knowledge the feed does not yet surface directly (provider-SoT spec, Phase 2).
export const GROUP_LANGS: Record<ProviderGroup, TrackLang[]> = {
  en: ['en'], ru: ['ru'], adult: ['en', 'ru'], firstparty: ['en', 'ru', 'ja'],
}
export const GROUP_CONTENT: Record<ProviderGroup, ContentKind[]> = {
  en: ['common'], ru: ['common'], adult: ['hentai'], firstparty: ['common'],
}

// Primary served language per group — the lang a RAW (original-audio) pick
// resolves to when the current lang isn't in the provider's group.
export const GROUP_PRIMARY_LANG: Record<ProviderGroup, TrackLang> = {
  en: 'en', ru: 'ru', adult: 'en', firstparty: 'ja',
}

// Under RAW the language slider is hidden — combo.lang follows the chosen
// provider's group. Keep the current lang if the group serves it, else fall
// back to the group's primary language.
export function langForProviderUnderRaw(group: ProviderGroup, currentLang: TrackLang): TrackLang {
  return GROUP_LANGS[group].includes(currentLang) ? currentLang : GROUP_PRIMARY_LANG[group]
}

// A cap's real per-title `lang` (Phase C source-panel truth — set only for
// ae's probed dub variant) overrides the group's default language set, so
// e.g. an ae English dub routes under EN only, not every language the
// `firstparty` group nominally serves. Caps without `lang` fall back to the
// group's full set. Single helper so every language-relevance call site
// (relevance filter, smart default, deep-link clamp, AePlayer's combo
// enumeration) agrees on this rule.
export function langsForCap(cap: { group: ProviderGroup; lang?: TrackLang }): TrackLang[] {
  return cap.lang ? [cap.lang] : GROUP_LANGS[cap.group]
}
