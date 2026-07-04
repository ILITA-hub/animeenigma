import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useMobilePlayer, _resetMobilePlayerForTests } from './useMobilePlayer'

type Listener = (e: { matches: boolean }) => void

function stubMatchMedia(initial: Record<string, boolean>) {
  const listeners: Record<string, Listener[]> = {}
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: initial[query] ?? false,
    addEventListener: (_: string, fn: Listener) => {
      ;(listeners[query] ??= []).push(fn)
    },
    removeEventListener: () => {},
  }))
  return {
    fire(query: string, matches: boolean) {
      for (const fn of listeners[query] ?? []) fn({ matches })
    },
  }
}

describe('useMobilePlayer', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
    _resetMobilePlayerForTests()
  })

  it('reads initial matches and reacts to changes', () => {
    const mm = stubMatchMedia({ '(max-width: 680px)': true, '(pointer: coarse)': false })
    const { isMobile, isCoarse } = useMobilePlayer()
    expect(isMobile.value).toBe(true)
    expect(isCoarse.value).toBe(false)
    mm.fire('(pointer: coarse)', true)
    expect(isCoarse.value).toBe(true)
    mm.fire('(max-width: 680px)', false)
    expect(isMobile.value).toBe(false)
  })

  it('is a singleton', () => {
    stubMatchMedia({})
    const a = useMobilePlayer()
    const b = useMobilePlayer()
    expect(a.isMobile).toBe(b.isMobile)
  })

  it('defaults to false without matchMedia (jsdom safety)', () => {
    vi.stubGlobal('matchMedia', undefined)
    const { isMobile, isCoarse } = useMobilePlayer()
    expect(isMobile.value).toBe(false)
    expect(isCoarse.value).toBe(false)
  })
})
