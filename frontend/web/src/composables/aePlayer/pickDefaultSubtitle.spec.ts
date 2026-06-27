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
  it('honors a provider-bundled track regardless of language', () => {
    const bundled = [T('gogoanime', 'en', 'bundled-en')]
    const r = pickAutoSubtitle({ lang: 'en', bundled, aggregated: [...bundled, T('jimaku', 'ja')] })
    expect(r?.url).toBe('bundled-en')
  })

  it('does NOT auto-enable an aggregated overlay on a hardsubbed EN cut', () => {
    // EN SUB cut, provider shipped no soft track → subs are burned into the
    // video. Jimaku JA / OpenSubtitles EN exist but must NOT auto-enable.
    const aggregated = [T('jimaku', 'ja'), T('opensubtitles', 'en')]
    expect(pickAutoSubtitle({ lang: 'en', bundled: [], aggregated })).toBeNull()
  })

  it('does NOT auto-enable an aggregated overlay on a hardsubbed RU cut', () => {
    const aggregated = [T('opensubtitles', 'ru')]
    expect(pickAutoSubtitle({ lang: 'ru', bundled: [], aggregated })).toBeNull()
  })

  it('auto-enables the best aggregated track for a raw original-JP cut', () => {
    // lang 'ja' → nothing burned in → auto-enable the JP overlay (jimaku first).
    const aggregated = [T('opensubtitles', 'ja', 'os-ja'), T('jimaku', 'ja', 'ji-ja')]
    expect(pickAutoSubtitle({ lang: 'ja', bundled: [], aggregated })?.url).toBe('ji-ja')
  })

  it('returns null for a raw JP cut with no tracks at all', () => {
    expect(pickAutoSubtitle({ lang: 'ja', bundled: [], aggregated: [] })).toBeNull()
  })
})
