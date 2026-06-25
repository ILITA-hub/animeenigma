import { describe, expect, it } from 'vitest'
import type { ProviderCap } from '@/types/capabilities'
import { resolveDeepLinkProvider } from './deepLinkProvider'

// Minimal ProviderCap factory — only the fields that resolveDeepLinkProvider reads:
// `provider`, `group`, `audios`. The required Phase-1 feed fields are filled with
// plausible defaults so the type is satisfied.
function makeCap(over: Partial<ProviderCap> & Pick<ProviderCap, 'provider' | 'group' | 'audios'>): ProviderCap {
  return {
    display_name: over.provider,
    state: 'active', selectable: true, hacker_only: false, order: 50,
    variants: [],
    ...over,
  }
}

// Registry as a Map<string, ProviderCap> — the new single-source-of-truth shape.
// Mirrors the real provider shapes that matter for the clamp logic: cross-language
// (kodik=ru, raw=jp), audio-restricted (miruro=dub-only, raw=sub-only),
// absent-from-map (animelib disabled — omitted), and 18+ (hanime).
const capMap = new Map<string, ProviderCap>([
  ['gogoanime', makeCap({ provider: 'gogoanime', group: 'en', audios: ['sub', 'dub'] })],
  ['miruro',    makeCap({ provider: 'miruro',    group: 'en', audios: ['dub'] })],
  ['kodik',     makeCap({ provider: 'kodik',     group: 'ru', audios: ['dub', 'sub'] })],
  ['ae',        makeCap({ provider: 'ae',        group: 'firstparty', audios: ['sub', 'dub'] })],
  ['raw',       makeCap({ provider: 'raw',       group: 'jp', audios: ['sub'] })],
  ['hanime',    makeCap({ provider: 'hanime',    group: 'adult', audios: ['dub'] })],
  // animelib is OMITTED — disabled providers are not present in the feed
])

const sub_en = { audio: 'sub' as const, lang: 'en' as const }

describe('resolveDeepLinkProvider', () => {
  it('keeps the current audio/lang when the provider supports them', () => {
    expect(resolveDeepLinkProvider('gogoanime', sub_en, 'common', capMap))
      .toEqual({ provider: 'gogoanime', audio: 'sub', lang: 'en' })
  })

  it('clamps lang to the provider language for a cross-language pin (kodik → ru)', () => {
    // kodik is RU-only; a ?provider=kodik deep-link under the default en filter
    // must switch lang to ru so the row becomes relevant/active and can be pinned.
    expect(resolveDeepLinkProvider('kodik', sub_en, 'common', capMap))
      .toEqual({ provider: 'kodik', audio: 'sub', lang: 'ru' })
  })

  it('clamps both audio and lang for raw (ja, sub-only)', () => {
    expect(resolveDeepLinkProvider('raw', { audio: 'dub', lang: 'en' }, 'common', capMap))
      .toEqual({ provider: 'raw', audio: 'sub', lang: 'ja' })
  })

  it('clamps audio for a dub-only provider (miruro)', () => {
    expect(resolveDeepLinkProvider('miruro', sub_en, 'common', capMap))
      .toEqual({ provider: 'miruro', audio: 'dub', lang: 'en' })
  })

  it('preserves the current lang when the provider supports it (ae keeps ru)', () => {
    expect(resolveDeepLinkProvider('ae', { audio: 'dub', lang: 'ru' }, 'common', capMap))
      .toEqual({ provider: 'ae', audio: 'dub', lang: 'ru' })
  })

  it('pins an 18+ provider only on a hentai title', () => {
    expect(resolveDeepLinkProvider('hanime', sub_en, 'hentai', capMap))
      .toEqual({ provider: 'hanime', audio: 'dub', lang: 'en' })
    // a hentai-only source must never be pinned onto a common title
    expect(resolveDeepLinkProvider('hanime', sub_en, 'common', capMap)).toBeNull()
  })

  it('returns null for an absent provider (animelib disabled — not in the feed)', () => {
    expect(resolveDeepLinkProvider('animelib', sub_en, 'common', capMap)).toBeNull()
  })

  it('returns null for a coarse/unknown value like "english"', () => {
    expect(resolveDeepLinkProvider('english', sub_en, 'common', capMap)).toBeNull()
  })

  it('returns null when no provider id is given', () => {
    expect(resolveDeepLinkProvider(undefined, sub_en, 'common', capMap)).toBeNull()
    expect(resolveDeepLinkProvider('', sub_en, 'common', capMap)).toBeNull()
    expect(resolveDeepLinkProvider(null, sub_en, 'common', capMap)).toBeNull()
  })
})
