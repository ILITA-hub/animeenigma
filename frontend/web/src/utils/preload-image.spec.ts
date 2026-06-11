import { describe, it, expect, vi, afterEach } from 'vitest'
import { preloadImage } from './preload-image'

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
})
