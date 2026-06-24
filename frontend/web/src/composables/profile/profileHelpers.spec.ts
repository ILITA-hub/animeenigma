/**
 * Unit spec for the pure helpers extracted from views/Profile.vue
 * (simplify pass 2026-06-24). i18n functions are passed in as fakes so the
 * assertions don't need a real vue-i18n instance.
 */

import { describe, it, expect } from 'vitest'
import { timeAgo, importErrorMessage, type ApiError } from './profileHelpers'

// Fake `t` that echoes the key + JSON-serialized params so assertions can
// inspect exactly what was requested without depending on real translations.
const t = (key: string, named?: Record<string, unknown>): string =>
  named ? `${key}|${JSON.stringify(named)}` : key

describe('timeAgo', () => {
  it('returns "just now" for unparseable input', () => {
    expect(timeAgo('not-a-date', t)).toBe('profile.import.justNow')
  })

  it('returns "just now" under a minute', () => {
    const iso = new Date(Date.now() - 30 * 1000).toISOString()
    expect(timeAgo(iso, t)).toBe('profile.import.justNow')
  })

  it('returns minutesAgo under an hour', () => {
    const iso = new Date(Date.now() - 5 * 60 * 1000).toISOString()
    expect(timeAgo(iso, t)).toBe('profile.import.minutesAgo|{"n":5}')
  })

  it('returns hoursAgo under a day', () => {
    const iso = new Date(Date.now() - 3 * 3600 * 1000).toISOString()
    expect(timeAgo(iso, t)).toBe('profile.import.hoursAgo|{"n":3}')
  })

  it('returns daysAgo beyond a day', () => {
    const iso = new Date(Date.now() - 2 * 86400 * 1000).toISOString()
    expect(timeAgo(iso, t)).toBe('profile.import.daysAgo|{"n":2}')
  })
})

describe('importErrorMessage', () => {
  it('uses a localized reason key when it exists', () => {
    const err: ApiError = {
      response: { data: { error: { details: { reason: 'rate_limited', host: 'mal.net' } } } },
    }
    const te = (key: string) => key === 'profile.import.errors.rate_limited'
    expect(importErrorMessage(err, 'mal', t, te)).toBe(
      'profile.import.errors.rate_limited|{"host":"mal.net","username":"","source":"mal"}',
    )
  })

  it('falls back to the structured server message when reason is unknown', () => {
    const err: ApiError = {
      response: { data: { error: { message: 'boom', details: { reason: 'unknown' } } } },
    }
    const te = () => false
    expect(importErrorMessage(err, 'shikimori', t, te)).toBe('boom')
  })

  it('falls back to a string error body', () => {
    const err: ApiError = { response: { data: { error: 'plain string error' } } }
    expect(importErrorMessage(err, 'mal', t, () => false)).toBe('plain string error')
  })

  it('falls back to data.message', () => {
    const err: ApiError = { response: { data: { message: 'top-level message' } } }
    expect(importErrorMessage(err, 'mal', t, () => false)).toBe('top-level message')
  })

  it('falls back to a generic localized message when nothing else is present', () => {
    const err: ApiError = {}
    expect(importErrorMessage(err, 'shikimori', t, () => false)).toBe(
      'profile.import.errors.generic|{"source":"shikimori"}',
    )
  })
})
