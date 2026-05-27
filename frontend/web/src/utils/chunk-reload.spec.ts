/**
 * Unit spec for tryReloadOnChunkError (frontend/web/src/utils/chunk-reload.ts).
 *
 * Covers the post-deploy stale-chunk recovery, in particular the targetUrl
 * branch added so a failed lazy-route import navigates to the intended route
 * (full load) instead of reloading the origin route — the cause of the
 * "pressing a button just sends me back to / / reloads the page" report.
 */

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { isChunkLoadError, tryReloadOnChunkError } from './chunk-reload'

const assign = vi.fn()
const reload = vi.fn()

beforeEach(() => {
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { assign, reload, href: 'http://localhost/' },
  })
  sessionStorage.clear()
  assign.mockClear()
  reload.mockClear()
})

describe('isChunkLoadError', () => {
  it('matches Vite dynamic-import and CSS-preload failures', () => {
    expect(isChunkLoadError(new Error('Failed to fetch dynamically imported module: /assets/Profile-abc.js'))).toBe(true)
    expect(isChunkLoadError('Unable to preload CSS for /assets/Foo.css')).toBe(true)
    expect(isChunkLoadError(new Error('TypeError: x is not a function'))).toBe(false)
  })
})

describe('tryReloadOnChunkError', () => {
  it('ignores non-chunk errors without navigating', () => {
    expect(tryReloadOnChunkError(new Error('boom'), '/profile')).toBe(false)
    expect(assign).not.toHaveBeenCalled()
    expect(reload).not.toHaveBeenCalled()
  })

  it('navigates to the target route (full load) when one is supplied', () => {
    const err = new Error('Failed to fetch dynamically imported module: /assets/Profile-abc.js')
    expect(tryReloadOnChunkError(err, '/profile?tab=settings')).toBe(true)
    expect(assign).toHaveBeenCalledWith('/profile?tab=settings')
    expect(reload).not.toHaveBeenCalled()
  })

  it('falls back to reloading the current URL when no target is supplied', () => {
    const err = new Error('Failed to fetch dynamically imported module: /assets/diagnostics-abc.js')
    expect(tryReloadOnChunkError(err)).toBe(true)
    expect(reload).toHaveBeenCalledTimes(1)
    expect(assign).not.toHaveBeenCalled()
  })

  it('gives up (no second navigation) within the cooldown window', () => {
    const err = new Error('Unable to preload CSS for /assets/Foo.css')
    expect(tryReloadOnChunkError(err, '/browse')).toBe(true)
    expect(assign).toHaveBeenCalledTimes(1)
    // Second failure for the same reason inside the cooldown → stop looping.
    expect(tryReloadOnChunkError(err, '/browse')).toBe(false)
    expect(assign).toHaveBeenCalledTimes(1)
  })
})
