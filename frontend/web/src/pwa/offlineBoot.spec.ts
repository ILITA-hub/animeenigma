import { describe, it, expect } from 'vitest'
import { shouldRedirectToDownloads } from './offlineBoot'

const base = { isInitialNav: true, online: false, enabled: true, toPath: '/' }

describe('shouldRedirectToDownloads', () => {
  it('offline initial nav to any page → redirect', () => {
    expect(shouldRedirectToDownloads(base)).toBe(true)
    expect(shouldRedirectToDownloads({ ...base, toPath: '/anime/x' })).toBe(true)
  })
  it('never redirects: already /downloads, online, in-app nav, or feature disabled', () => {
    expect(shouldRedirectToDownloads({ ...base, toPath: '/downloads' })).toBe(false)
    expect(shouldRedirectToDownloads({ ...base, online: true })).toBe(false)
    expect(shouldRedirectToDownloads({ ...base, isInitialNav: false })).toBe(false)
    expect(shouldRedirectToDownloads({ ...base, enabled: false })).toBe(false)
  })
})
