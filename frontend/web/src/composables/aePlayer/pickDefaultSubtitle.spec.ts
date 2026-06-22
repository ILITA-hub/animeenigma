import { describe, it, expect } from 'vitest'
import { pickDefaultSubtitle } from './pickDefaultSubtitle'

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
