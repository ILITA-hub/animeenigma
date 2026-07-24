import { describe, it, expect, vi, afterEach } from 'vitest'
import { isReloadOntoUrl } from './reloadDetection'

/**
 * Anime-page "scroll-down on reload" bugfix (2026-07-24). isReloadOntoUrl must
 * fire ONLY for a genuine browser reload of the exact URL currently shown, so a
 * mid-watch F5 (URL carries ?episode) mounts the player in place — while cold
 * and in-app deep-links keep scrolling to the player.
 */
const HREF = 'https://ae.test/anime/abc?episode=5'

function stubNav(entry: Partial<PerformanceNavigationTiming> | null) {
  vi.spyOn(performance, 'getEntriesByType').mockReturnValue(
    (entry ? [entry] : []) as unknown as PerformanceEntryList,
  )
}

afterEach(() => vi.restoreAllMocks())

describe('isReloadOntoUrl', () => {
  it('true — reload of the exact current URL (mid-watch F5)', () => {
    stubNav({ type: 'reload', name: HREF })
    expect(isReloadOntoUrl(HREF)).toBe(true)
  })

  it('false — reload, but the reloaded document was a different URL (in-app nav after a home reload)', () => {
    stubNav({ type: 'reload', name: 'https://ae.test/' })
    expect(isReloadOntoUrl(HREF)).toBe(false)
  })

  it('false — cold deep-link open (notification / shared link / typed URL)', () => {
    stubNav({ type: 'navigate', name: HREF })
    expect(isReloadOntoUrl(HREF)).toBe(false)
  })

  it('false — back/forward navigation', () => {
    stubNav({ type: 'back_forward', name: HREF })
    expect(isReloadOntoUrl(HREF)).toBe(false)
  })

  it('false — no navigation-timing entry available', () => {
    stubNav(null)
    expect(isReloadOntoUrl(HREF)).toBe(false)
  })
})
