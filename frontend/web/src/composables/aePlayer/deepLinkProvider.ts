import type { AudioKind, ContentKind, ProviderDef, TrackLang } from '@/types/aePlayer'

/** The audio/lang/provider a `?provider=` deep-link should pin the player to. */
export interface DeepLinkPin {
  provider: string
  audio: AudioKind
  lang: TrackLang
}

/**
 * Resolve a notification / shared-link `?provider=` value into a concrete
 * provider PIN, clamping the current audio/lang to what that provider actually
 * serves.
 *
 * WHY the clamp: a provider row is only `active` (and therefore pinnable) when
 * it matches the live audio/lang/content filter (see computeProviderRows). The
 * default combo is `sub`/`en`, so a `?provider=kodik` deep-link (kodik is RU)
 * would otherwise land its row as `irrelevant` and the pin would silently fall
 * through to the smart default. Switching lang→ru (and audio to a supported
 * kind) makes the row relevant so the explicit choice is honored.
 *
 * Honored ONLY when the id names a real, content-compatible, non-static-disabled
 * provider def. Coarse/unknown values ('english'), 18+ sources on a common
 * title, and hard-disabled backends (AniLib) return null so the smart default
 * picks instead. Pure + sync so it is unit-testable without mounting the player.
 */
export function resolveDeepLinkProvider(
  providerId: string | undefined | null,
  current: { audio: AudioKind; lang: TrackLang },
  content: ContentKind,
  registry: ProviderDef[],
): DeepLinkPin | null {
  if (!providerId) return null
  const def = registry.find((d) => d.id === providerId)
  if (!def) return null
  if (def.staticDisabled) return null
  if (!def.content.includes(content)) return null
  // Keep the current facet when the provider supports it; otherwise fall to the
  // provider's first declared lang/audio so the row becomes relevant.
  const lang = def.langs.includes(current.lang) ? current.lang : def.langs[0]
  const audio = def.audios.includes(current.audio) ? current.audio : def.audios[0]
  return { provider: def.id, audio, lang }
}
