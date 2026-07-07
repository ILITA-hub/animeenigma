/**
 * Vitest spec for useOpenFeature() — the "open a secret feature" click helper
 * used by the admin policy view (Task 7). Standalone PWA windows have no
 * address bar / tab strip, so a `target="_blank"` anchor would silently
 * swallow the new tab; in that mode we intercept the click and route inside
 * the app shell instead. In a normal browser tab the native anchor already
 * does the right thing, so the helper must do NOTHING (no preventDefault,
 * no router.push) — calling window.open would open a second app instance.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const pushSpy = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushSpy }),
}))

import { useOpenFeature } from './useOpenFeature'

function mockMatchMedia(standaloneMatches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: query === '(display-mode: standalone)' ? standaloneMatches : false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  })
}

function makeClickEvent(): MouseEvent {
  return { preventDefault: vi.fn() } as unknown as MouseEvent
}

describe('useOpenFeature', () => {
  beforeEach(() => {
    pushSpy.mockClear()
  })

  afterEach(() => {
    // @ts-expect-error — cleanup jsdom global between tests
    delete window.matchMedia
    delete (navigator as unknown as { standalone?: boolean }).standalone
  })

  it('standalone=true: openFeature prevents default and routes inside the app', () => {
    mockMatchMedia(true)
    const { isStandalone, openFeature } = useOpenFeature()
    expect(isStandalone).toBe(true)

    const ev = makeClickEvent()
    openFeature(ev, '/fanfics')

    expect(ev.preventDefault).toHaveBeenCalledTimes(1)
    expect(pushSpy).toHaveBeenCalledWith('/fanfics')
  })

  it('standalone=false: openFeature does neither — the native anchor handles it', () => {
    mockMatchMedia(false)
    const { isStandalone, openFeature } = useOpenFeature()
    expect(isStandalone).toBe(false)

    const ev = makeClickEvent()
    openFeature(ev, '/fanfics')

    expect(ev.preventDefault).not.toHaveBeenCalled()
    expect(pushSpy).not.toHaveBeenCalled()
  })

  it('iOS legacy navigator.standalone also counts as standalone', () => {
    // matchMedia present but display-mode never matches on iOS Safari; the
    // legacy navigator.standalone boolean is the only signal there.
    mockMatchMedia(false)
    ;(navigator as unknown as { standalone?: boolean }).standalone = true

    const { isStandalone, openFeature } = useOpenFeature()
    expect(isStandalone).toBe(true)

    const ev = makeClickEvent()
    openFeature(ev, '/gacha')
    expect(ev.preventDefault).toHaveBeenCalledTimes(1)
    expect(pushSpy).toHaveBeenCalledWith('/gacha')
  })
})
