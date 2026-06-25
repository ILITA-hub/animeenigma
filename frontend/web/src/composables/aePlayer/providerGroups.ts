import type { TrackLang, ContentKind, ProviderGroup } from '@/types/aePlayer'

// Backend wire `group` → the (lang, content) facets that group serves. Single
// FE source of truth: the relevance filter (useProviderFeed), the deep-link
// clamp (deepLinkProvider), and AePlayer's served-lang derivation all import
// these, so adding a `ProviderGroup` updates one table, not three. The backend
// owns `group`; mapping a group to its served facets is the only provider
// knowledge the feed does not yet surface directly (provider-SoT spec, Phase 2).
export const GROUP_LANGS: Record<ProviderGroup, TrackLang[]> = {
  en: ['en'], ru: ['ru'], adult: ['en', 'ru'], jp: ['ja'], firstparty: ['en', 'ru', 'ja'],
}
export const GROUP_CONTENT: Record<ProviderGroup, ContentKind[]> = {
  en: ['common'], ru: ['common'], adult: ['hentai'], jp: ['common'], firstparty: ['common'],
}
