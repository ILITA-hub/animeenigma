import { describe, it, expect, vi, afterEach } from 'vitest'
import { shouldDeferReload, scheduleReload, setActiveDownloadProbe } from './registerPwa'

function docWithVideo({ paused = false, ended = false, readyState = 4 } = {}): Document {
  const doc = document.implementation.createHTMLDocument('')
  const v = doc.createElement('video')
  Object.defineProperty(v, 'paused', { value: paused })
  Object.defineProperty(v, 'ended', { value: ended })
  Object.defineProperty(v, 'readyState', { value: readyState })
  doc.body.appendChild(v)
  return doc
}

afterEach(() => {
  vi.useRealTimers()
  setActiveDownloadProbe(() => false)
})

describe('shouldDeferReload', () => {
  it('defers while a video is actively playing', () => {
    expect(shouldDeferReload(docWithVideo())).toBe(true)
  })
  it('defers while an offline download is in flight (probe injected by the offline engine)', () => {
    setActiveDownloadProbe(() => true)
    expect(shouldDeferReload(document.implementation.createHTMLDocument(''))).toBe(true)
  })
  it('does not defer for paused/ended/not-started videos', () => {
    expect(shouldDeferReload(docWithVideo({ paused: true }))).toBe(false)
    expect(shouldDeferReload(docWithVideo({ ended: true }))).toBe(false)
    expect(shouldDeferReload(docWithVideo({ readyState: 0 }))).toBe(false)
  })
  it('defers while a Kodik iframe is mounted (classic fallback playback)', () => {
    const doc = document.implementation.createHTMLDocument('')
    const f = doc.createElement('iframe')
    f.src = 'https://kodik.info/serial/x'
    doc.body.appendChild(f)
    expect(shouldDeferReload(doc)).toBe(true)
  })
  it('does not defer on a plain page', () => {
    expect(shouldDeferReload(document.implementation.createHTMLDocument(''))).toBe(false)
  })
})

describe('scheduleReload', () => {
  it('reloads immediately when nothing is playing', () => {
    const reload = vi.fn()
    scheduleReload(document.implementation.createHTMLDocument(''), reload)
    expect(reload).toHaveBeenCalledTimes(1)
  })
  it('polls until playback stops, then reloads once', () => {
    vi.useFakeTimers()
    const doc = docWithVideo()
    const reload = vi.fn()
    scheduleReload(doc, reload)
    expect(reload).not.toHaveBeenCalled()
    vi.advanceTimersByTime(15_000)
    expect(reload).not.toHaveBeenCalled()
    doc.querySelector('video')!.remove()
    vi.advanceTimersByTime(15_000)
    expect(reload).toHaveBeenCalledTimes(1)
  })
})
