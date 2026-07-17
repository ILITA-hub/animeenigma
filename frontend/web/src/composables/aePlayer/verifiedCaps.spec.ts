import { describe, expect, it } from 'vitest'
import { effectiveAudios, isUnverified, seedVerifyFromReport, verifiedDubLangs, verifiedHardsubLangs, verifyFor } from './verifiedCaps'
import type { ProviderVerify, VerifyReport } from '@/types/contentVerify'
import type { CapabilityReport } from '@/types/capabilities'

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

describe('seedVerifyFromReport', () => {
  const capBase = {
    display_name: 'x', state: 'active' as const, selectable: true, hacker_only: false,
    order: 90, group: 'en' as const, audios: ['sub', 'dub'] as ('sub' | 'dub')[], variants: [],
  }

  it('null report ⇒ null (nothing to seed)', () => {
    expect(seedVerifyFromReport(null)).toBeNull()
  })

  it('caps without any verify blend ⇒ null (poll-blind, same as before the fix)', () => {
    const report: CapabilityReport = {
      anime_id: 'a1',
      families: [{ family: 'others', providers: [{ ...capBase, provider: 'gogoanime' }] }],
    }
    expect(seedVerifyFromReport(report)).toBeNull()
  })

  it('caps carrying verify ⇒ a VerifyReport keyed by provider, units always []', () => {
    const report: CapabilityReport = {
      anime_id: 'a1',
      families: [
        {
          family: 'others',
          providers: [
            { ...capBase, provider: 'gogoanime', verify: { status: 'verified', raw: true, dub_langs: ['en'], hardsub_langs: [] } },
            { ...capBase, provider: 'nineanime' }, // no verify blend — omitted from the seed
          ],
        },
        {
          family: 'aeProvider',
          providers: [{ ...capBase, provider: 'kodik', group: 'ru', verify: { status: 'partial', raw: false, dub_langs: [], hardsub_langs: ['ru'] } }],
        },
      ],
    }
    expect(seedVerifyFromReport(report)).toEqual({
      animeId: 'a1',
      providers: {
        gogoanime: { status: 'verified', raw: true, dub_langs: ['en'], hardsub_langs: [], units: [] },
        kodik: { status: 'partial', raw: false, dub_langs: [], hardsub_langs: ['ru'], units: [] },
      },
    })
  })

  it('integration-ish: effectiveAudios reads the seeded row exactly like a poll-derived one', () => {
    const report: CapabilityReport = {
      anime_id: 'a1',
      families: [{ family: 'others', providers: [{ ...capBase, provider: 'gogoanime', verify: { status: 'verified', raw: false, dub_langs: ['en'], hardsub_langs: [] } }] }],
    }
    const seeded = seedVerifyFromReport(report)
    expect(effectiveAudios({ group: 'en', audios: ['sub', 'dub'] }, verifyFor(seeded, 'gogoanime'))).toEqual(['dub'])
  })
})
