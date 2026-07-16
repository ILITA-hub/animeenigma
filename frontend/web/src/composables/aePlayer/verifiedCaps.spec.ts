import { describe, expect, it } from 'vitest'
import { effectiveAudios, isUnverified, verifiedDubLangs, verifiedHardsubLangs, verifyFor } from './verifiedCaps'
import type { ProviderVerify, VerifyReport } from '@/types/contentVerify'

const v = (p: Partial<ProviderVerify>): ProviderVerify => ({
  status: 'unverified', raw: false, dub_langs: [], hardsub_langs: [], ...p,
})

describe('effectiveAudios', () => {
  const cap = { group: 'en' as const, audios: ['sub', 'dub'] as ('sub' | 'dub')[] }
  it('unverified ⇒ RAW-assumed only', () => {
    expect(effectiveAudios(cap, null)).toEqual(['sub'])
    expect(effectiveAudios(cap, v({}))).toEqual(['sub'])
  })
  it('verified dub only ⇒ dub only', () => {
    expect(effectiveAudios(cap, v({ status: 'verified', dub_langs: ['en'] }))).toEqual(['dub'])
  })
  it('partial with dub keeps RAW for unverified units', () => {
    expect(effectiveAudios(cap, v({ status: 'partial', dub_langs: ['en'] }))).toEqual(expect.arrayContaining(['sub', 'dub']))
  })
  it('verified raw ⇒ sub', () => {
    expect(effectiveAudios(cap, v({ status: 'verified', raw: true }))).toEqual(['sub'])
  })
  it('firstparty trusts cap.audios without a row', () => {
    expect(effectiveAudios({ group: 'firstparty', audios: ['dub'] }, null)).toEqual(['dub'])
  })
})

describe('isUnverified / verifiedDubLangs / verifyFor', () => {
  it('marks non-firstparty rows without verdicts', () => {
    expect(isUnverified({ group: 'en' }, null)).toBe(true)
    expect(isUnverified({ group: 'firstparty' }, null)).toBe(false)
    expect(isUnverified({ group: 'en' }, v({ status: 'partial' }))).toBe(false)
  })
  it('extracts langs and reads the report', () => {
    expect(verifiedDubLangs(v({ dub_langs: ['en', 'ru'] }))).toEqual(['en', 'ru'])
    const rep: VerifyReport = { animeId: 'a', providers: { kodik: v({ raw: true }) } }
    expect(verifyFor(rep, 'kodik')?.raw).toBe(true)
    expect(verifyFor(rep, 'gogoanime')).toBeNull()
  })
})

describe('verifiedHardsubLangs', () => {
  it('extracts only known TrackLang values', () => {
    expect(verifiedHardsubLangs(v({ hardsub_langs: ['ja', 'ru'] }))).toEqual(['ja', 'ru'])
  })
  it('filters out a bogus/unrecognized lang value — no badge for garbage data', () => {
    expect(verifiedHardsubLangs(v({ hardsub_langs: ['ja', 'xx', 'klingon'] }))).toEqual(['ja'])
  })
  it('returns [] for a null verify row', () => {
    expect(verifiedHardsubLangs(null)).toEqual([])
  })
})
