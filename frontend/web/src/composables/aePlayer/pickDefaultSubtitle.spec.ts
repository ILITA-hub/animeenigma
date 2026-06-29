import { describe, it, expect } from 'vitest'
import { pickDefaultSubtitle, pickBestForLang, pickAutoSubtitle } from './pickDefaultSubtitle'

const T = (provider: string, lang: string, url = `${provider}-${lang}`) => ({ url, provider, lang, label: url, format: 'srt' })

describe('pickDefaultSubtitle', () => {
  it('returns null for no tracks', () => {
    expect(pickDefaultSubtitle([], { lang: 'ja' })).toBeNull()
  })
  it('prefers lang match, jimaku first', () => {
    const r = pickDefaultSubtitle([T('opensubtitles', 'ja'), T('gogoanime', 'en'), T('jimaku', 'ja')], { lang: 'ja' })
    expect(r?.provider).toBe('jimaku')
  })
  it('prefers provider-own over opensubtitles within lang', () => {
    const r = pickDefaultSubtitle([T('opensubtitles', 'en'), T('gogoanime', 'en')], { lang: 'en' })
    expect(r?.provider).toBe('gogoanime')
  })
  it('falls back across langs when no lang match (jimaku first)', () => {
    const r = pickDefaultSubtitle([T('opensubtitles', 'en'), T('jimaku', 'ja')], { lang: 'ru' })
    expect(r?.provider).toBe('jimaku')
  })
})

describe('pickBestForLang', () => {
  const tracks = [
    T('opensubtitles', 'en', 'en-os'),
    T('gogoanime', 'en', 'en-own'),
    T('jimaku', 'ja', 'ja-ji'),
  ]
  it('returns null when no track matches the language', () => {
    expect(pickBestForLang(tracks, 'ru')).toBeNull()
  })
  it('prefers provider-own over opensubtitles for the same language', () => {
    expect(pickBestForLang(tracks, 'en')?.url).toBe('en-own')
  })
  it('returns the only match for a language', () => {
    expect(pickBestForLang(tracks, 'ja')?.url).toBe('ja-ji')
  })
  it('does NOT fall back to another language (unlike pickDefaultSubtitle)', () => {
    expect(pickBestForLang([tracks[2]], 'en')).toBeNull()
    expect(pickDefaultSubtitle([tracks[2]], { lang: 'en' })?.url).toBe('ja-ji')
  })
})

describe('pickAutoSubtitle', () => {
  // Subtitles default OFF with no exceptions — pickAutoSubtitle never enables one.
  it('never auto-enables a provider-bundled track', () => {
    const bundled = [T('gogoanime', 'en', 'bundled-en')]
    expect(pickAutoSubtitle({ lang: 'en', bundled, aggregated: [...bundled, T('jimaku', 'ja')] })).toBeNull()
  })

  it('never auto-enables on a hardsubbed EN/RU cut', () => {
    expect(pickAutoSubtitle({ lang: 'en', bundled: [], aggregated: [T('jimaku', 'ja'), T('opensubtitles', 'en')] })).toBeNull()
    expect(pickAutoSubtitle({ lang: 'ru', bundled: [], aggregated: [T('opensubtitles', 'ru')] })).toBeNull()
  })

  it('never auto-enables on a raw original-JP cut (no exception)', () => {
    const aggregated = [T('opensubtitles', 'ja', 'os-ja'), T('jimaku', 'ja', 'ji-ja')]
    expect(pickAutoSubtitle({ lang: 'ja', bundled: [], aggregated })).toBeNull()
    expect(pickAutoSubtitle({ lang: 'ja', bundled: [], aggregated: [] })).toBeNull()
  })
})
