import { describe, it, expect } from 'vitest'
import { START_LOCATION, type RouteLocationNormalizedLoaded } from 'vue-router'
import router from './index'

// router.resolve() returns RouteLocationResolved (name: … | null); scrollBehavior
// takes the normalized-loaded shape. Structurally compatible for this test.
const loc = (path: string) =>
  router.resolve(path) as unknown as RouteLocationNormalizedLoaded

/**
 * scrollBehavior contract (bugfix 2026-07-24 — "random scroll-down on reload").
 *
 * On a full page reload vue-router runs the FIRST navigation with
 * `from === START_LOCATION`, and repopulates `savedPosition` from
 * history.state.scroll (its own `beforeunload` snapshot of the live scroll).
 * Returning that savedPosition restored the pre-reload scroll — the page (home
 * OR anime) came back scrolled down. The fix forces the initial navigation to
 * the top; genuine in-app back/forward still restores savedPosition.
 */
const scrollBehavior = router.options.scrollBehavior!
const savedDown = { left: 0, top: 640 }

describe('router scrollBehavior', () => {
  it('reload of the home page lands at the top (ignores restored savedPosition)', () => {
    const to = loc('/')
    const result = scrollBehavior(to, START_LOCATION, savedDown)
    expect(result).toEqual({ top: 0 })
  })

  it('reload of an anime page lands at the top (ignores restored savedPosition)', () => {
    const to = loc('/anime/some-id')
    const result = scrollBehavior(to, START_LOCATION, savedDown)
    expect(result).toEqual({ top: 0 })
  })

  it('genuine in-app back/forward still restores savedPosition', () => {
    const to = loc('/')
    const from = loc('/anime/some-id')
    const result = scrollBehavior(to, from, savedDown)
    expect(result).toEqual(savedDown)
  })

  it('query/hash-only change on the same path preserves scroll', () => {
    const to = loc('/anime/some-id?ugc=reviews')
    const from = loc('/anime/some-id')
    const result = scrollBehavior(to, from, null)
    expect(result).toBe(false)
  })

  it('a normal in-app navigation to a new path goes to the top', () => {
    const to = loc('/anime/some-id')
    const from = loc('/')
    const result = scrollBehavior(to, from, null)
    expect(result).toEqual({ top: 0 })
  })
})
