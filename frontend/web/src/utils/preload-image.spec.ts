import { describe, it, expect, vi, afterEach } from 'vitest'
import { preloadImage, isImageWarm, markImageWarm } from './preload-image'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

/** Stub the global Image so the test can fire load/error by hand. */
function stubImage(): { current: { onload?: (() => void) | null; onerror?: (() => void) | null } } {
  const holder: { current: { onload?: (() => void) | null; onerror?: (() => void) | null } } = {
    current: {},
  }
  vi.stubGlobal(
    'Image',
    class {
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      set src(_v: string) {
        holder.current = this
      }
    },
  )
  return holder
}

describe('preloadImage', () => {
  it('resolves immediately on empty src', async () => {
    await expect(preloadImage('')).resolves.toBeUndefined()
  })

  it('resolves when the image loads', async () => {
    const holder = stubImage()
    const p = preloadImage('/poster.jpg')
    holder.current.onload?.()
    await expect(p).resolves.toBeUndefined()
  })

  it('resolves when the image errors (best-effort, never rejects)', async () => {
    const holder = stubImage()
    const p = preloadImage('/broken.jpg')
    holder.current.onerror?.()
    await expect(p).resolves.toBeUndefined()
  })

  it('resolves via timeout when the image never loads', async () => {
    vi.useFakeTimers()
    stubImage()
    const p = preloadImage('/hangs-forever.jpg', 1000)
    vi.advanceTimersByTime(1100)
    await expect(p).resolves.toBeUndefined()
  })

  it('warm registry: successful load marks the URL warm; repeat preloads short-circuit', async () => {
    const holder = stubImage()
    expect(isImageWarm('/warm-test.jpg')).toBe(false)
    const p = preloadImage('/warm-test.jpg')
    holder.current.onload?.()
    await p
    expect(isImageWarm('/warm-test.jpg')).toBe(true)
    // Second preload resolves without creating a new Image (short-circuit):
    holder.current = {}
    await preloadImage('/warm-test.jpg')
    expect(holder.current.onload).toBeUndefined()
  })

  it('errored loads are NOT marked warm', async () => {
    const holder = stubImage()
    const p = preloadImage('/error-test.jpg')
    holder.current.onerror?.()
    await p
    expect(isImageWarm('/error-test.jpg')).toBe(false)
  })

  it('markImageWarm registers a URL directly (img @load path)', () => {
    expect(isImageWarm('/manual-test.jpg')).toBe(false)
    markImageWarm('/manual-test.jpg')
    expect(isImageWarm('/manual-test.jpg')).toBe(true)
  })
})
