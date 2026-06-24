import { describe, expect, it } from 'vitest'
import type { ProviderDef } from '@/types/aePlayer'
import { resolveDeepLinkProvider } from './deepLinkProvider'

// Minimal hand-written registry mirroring the real provider shapes that matter
// for the clamp logic: cross-language (kodik=ru, raw=ja), audio-restricted
// (miruro=dub-only, raw=sub-only), static-disabled (animelib), and 18+ (hanime).
const def = (over: Partial<ProviderDef> & Pick<ProviderDef, 'id'>): ProviderDef => ({
  name: over.id, hue: '#000', group: 'en', audios: ['sub', 'dub'], langs: ['en'],
  content: ['common'], scraper: true, ...over,
})

const registry: ProviderDef[] = [
  def({ id: 'gogoanime', langs: ['en'], audios: ['sub', 'dub'] }),
  def({ id: 'miruro', langs: ['en'], audios: ['dub'] }),
  def({ id: 'kodik', group: 'ru', langs: ['ru'], audios: ['dub', 'sub'], scraper: false }),
  def({ id: 'ae', group: 'first-party', langs: ['en', 'ru', 'ja'], audios: ['sub', 'dub'], scraper: false }),
  def({ id: 'raw', group: 'raw', langs: ['ja'], audios: ['sub'], scraper: false }),
  def({ id: 'hanime', group: 'adult', langs: ['ru'], audios: ['dub'], content: ['hentai'], scraper: false }),
  def({ id: 'animelib', group: 'ru', langs: ['ru'], audios: ['sub'], scraper: false,
    staticDisabled: { reason: 'Unavailable', description: 'AniLib direct streams are not currently working' } }),
]

const sub_en = { audio: 'sub' as const, lang: 'en' as const }

describe('resolveDeepLinkProvider', () => {
  it('keeps the current audio/lang when the provider supports them', () => {
    expect(resolveDeepLinkProvider('gogoanime', sub_en, 'common', registry))
      .toEqual({ provider: 'gogoanime', audio: 'sub', lang: 'en' })
  })

  it('clamps lang to the provider language for a cross-language pin (kodik → ru)', () => {
    // kodik is RU-only; a ?provider=kodik deep-link under the default en filter
    // must switch lang to ru so the row becomes relevant/active and can be pinned.
    expect(resolveDeepLinkProvider('kodik', sub_en, 'common', registry))
      .toEqual({ provider: 'kodik', audio: 'sub', lang: 'ru' })
  })

  it('clamps both audio and lang for raw (ja, sub-only)', () => {
    expect(resolveDeepLinkProvider('raw', { audio: 'dub', lang: 'en' }, 'common', registry))
      .toEqual({ provider: 'raw', audio: 'sub', lang: 'ja' })
  })

  it('clamps audio for a dub-only provider (miruro)', () => {
    expect(resolveDeepLinkProvider('miruro', sub_en, 'common', registry))
      .toEqual({ provider: 'miruro', audio: 'dub', lang: 'en' })
  })

  it('preserves the current lang when the provider supports it (ae keeps ru)', () => {
    expect(resolveDeepLinkProvider('ae', { audio: 'dub', lang: 'ru' }, 'common', registry))
      .toEqual({ provider: 'ae', audio: 'dub', lang: 'ru' })
  })

  it('pins an 18+ provider only on a hentai title', () => {
    expect(resolveDeepLinkProvider('hanime', sub_en, 'hentai', registry))
      .toEqual({ provider: 'hanime', audio: 'dub', lang: 'ru' })
    // a hentai-only source must never be pinned onto a common title
    expect(resolveDeepLinkProvider('hanime', sub_en, 'common', registry)).toBeNull()
  })

  it('returns null for a statically-disabled provider (animelib)', () => {
    expect(resolveDeepLinkProvider('animelib', sub_en, 'common', registry)).toBeNull()
  })

  it('returns null for a coarse/unknown value like "english"', () => {
    expect(resolveDeepLinkProvider('english', sub_en, 'common', registry)).toBeNull()
  })

  it('returns null when no provider id is given', () => {
    expect(resolveDeepLinkProvider(undefined, sub_en, 'common', registry)).toBeNull()
    expect(resolveDeepLinkProvider('', sub_en, 'common', registry)).toBeNull()
    expect(resolveDeepLinkProvider(null, sub_en, 'common', registry)).toBeNull()
  })
})
