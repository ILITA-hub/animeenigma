import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  shouldDeferReload,
  scheduleReload,
  setActiveDownloadProbe,
  setLiveSessionProbe,
  cancelPendingReload,
  shouldFullReloadOnNav,
} from './registerPwa'

function makeDoc(): Document {
  return document.implementation.createHTMLDocument('')
}

function docWithVideo({ paused = false, ended = false, readyState = 4 } = {}): Document {
  const doc = makeDoc()
  const v = doc.createElement('video')
  Object.defineProperty(v, 'paused', { value: paused })
  Object.defineProperty(v, 'ended', { value: ended })
  Object.defineProperty(v, 'readyState', { value: readyState })
  doc.body.appendChild(v)
  return doc
}

function setVisibility(doc: Document, state: DocumentVisibilityState): Document {
  Object.defineProperty(doc, 'visibilityState', { value: state, configurable: true })
  return doc
}

const crossPageNav = () => shouldFullReloadOnNav({ path: '/anime/x' }, { path: '/browse' })

afterEach(() => {
  vi.useRealTimers()
  setActiveDownloadProbe(() => false)
  setLiveSessionProbe(() => false)
  cancelPendingReload()
})

describe('shouldDeferReload', () => {
  it('defers while a video is actively playing', () => {
    expect(shouldDeferReload(docWithVideo())).toBe(true)
  })
  it('defers while an offline download is in flight (probe injected by the offline engine)', () => {
    setActiveDownloadProbe(() => true)
    expect(shouldDeferReload(makeDoc())).toBe(true)
  })
  it('does not defer for paused/ended/not-started videos', () => {
    expect(shouldDeferReload(docWithVideo({ paused: true }))).toBe(false)
    expect(shouldDeferReload(docWithVideo({ ended: true }))).toBe(false)
    expect(shouldDeferReload(docWithVideo({ readyState: 0 }))).toBe(false)
  })
  it('defers while a Kodik iframe is mounted (classic fallback playback)', () => {
    const doc = makeDoc()
    const f = doc.createElement('iframe')
    f.src = 'https://kodik.info/serial/x'
    doc.body.appendChild(f)
    expect(shouldDeferReload(doc)).toBe(true)
  })
  it('does not defer on a plain page', () => {
    expect(shouldDeferReload(makeDoc())).toBe(false)
  })

  it('defers while a textarea holds an unsent draft, even unfocused', () => {
    // Review scenario: user types a review, ctrl+clicks a character page
    // (focus leaves the field), returns — the draft must survive the update.
    const doc = makeDoc()
    const ta = doc.createElement('textarea')
    ta.value = 'отличное аниме, но'
    doc.body.appendChild(ta)
    expect(shouldDeferReload(doc)).toBe(true)
  })
  it('does not defer for empty, readonly or disabled text fields', () => {
    const doc = makeDoc()
    const empty = doc.createElement('textarea')
    const ro = doc.createElement('input')
    ro.type = 'text'
    ro.value = 'shared link'
    ro.readOnly = true
    const dis = doc.createElement('textarea')
    dis.value = 'x'
    dis.disabled = true
    doc.body.append(empty, ro, dis)
    expect(shouldDeferReload(doc)).toBe(false)
  })
  it('defers for a non-empty textual input but ignores non-textual inputs', () => {
    const doc = makeDoc()
    const box = doc.createElement('input')
    box.type = 'checkbox' // value defaults to "on" — must not count as a draft
    doc.body.appendChild(box)
    expect(shouldDeferReload(doc)).toBe(false)
    const text = doc.createElement('input')
    text.type = 'search'
    text.value = 'frieren'
    doc.body.appendChild(text)
    expect(shouldDeferReload(doc)).toBe(true)
  })
  it('defers for contenteditable elements with content', () => {
    const doc = makeDoc()
    const el = doc.createElement('div')
    el.setAttribute('contenteditable', 'true')
    el.textContent = 'draft'
    doc.body.appendChild(el)
    expect(shouldDeferReload(doc)).toBe(true)
  })

  it('defers while an open modal dialog is mounted', () => {
    const doc = makeDoc()
    const dlg = doc.createElement('div')
    dlg.setAttribute('role', 'dialog')
    doc.body.appendChild(dlg)
    expect(shouldDeferReload(doc)).toBe(true)
  })

  it('defers inside live room sessions (probe installed by the router from meta.liveSession)', () => {
    setLiveSessionProbe(() => true)
    expect(shouldDeferReload(makeDoc())).toBe(true)
  })
})

describe('scheduleReload', () => {
  it('never reloads a visible tab spontaneously', () => {
    vi.useFakeTimers()
    const doc = setVisibility(makeDoc(), 'visible')
    const reload = vi.fn()
    scheduleReload(doc, reload)
    vi.advanceTimersByTime(120_000)
    expect(reload).not.toHaveBeenCalled()
    expect(crossPageNav()).toBe(true) // update still pending
  })
  it('reloads immediately when the tab is already hidden and nothing defers', () => {
    const reload = vi.fn()
    scheduleReload(setVisibility(makeDoc(), 'hidden'), reload)
    expect(reload).toHaveBeenCalledTimes(1)
    expect(crossPageNav()).toBe(false) // pending consumed
  })
  it('reloads when the tab becomes hidden', () => {
    const doc = setVisibility(makeDoc(), 'visible')
    const reload = vi.fn()
    scheduleReload(doc, reload)
    expect(reload).not.toHaveBeenCalled()
    setVisibility(doc, 'hidden')
    doc.dispatchEvent(new Event('visibilitychange'))
    expect(reload).toHaveBeenCalledTimes(1)
  })
  it('while hidden, polls past a playing video and reloads once it stops', () => {
    vi.useFakeTimers()
    const doc = setVisibility(docWithVideo(), 'hidden')
    const reload = vi.fn()
    scheduleReload(doc, reload)
    expect(reload).not.toHaveBeenCalled()
    vi.advanceTimersByTime(15_000)
    expect(reload).not.toHaveBeenCalled()
    doc.querySelector('video')!.remove()
    vi.advanceTimersByTime(15_000)
    expect(reload).toHaveBeenCalledTimes(1)
  })
  it('does not reload a hidden tab while a draft exists (review scenario)', () => {
    vi.useFakeTimers()
    const doc = setVisibility(makeDoc(), 'hidden')
    const ta = doc.createElement('textarea')
    ta.value = 'draft review'
    doc.body.appendChild(ta)
    const reload = vi.fn()
    scheduleReload(doc, reload)
    vi.advanceTimersByTime(60_000)
    expect(reload).not.toHaveBeenCalled()
    ta.value = '' // draft submitted/cleared
    vi.advanceTimersByTime(15_000)
    expect(reload).toHaveBeenCalledTimes(1)
  })
  it('re-scheduling replaces the previous pending reload', () => {
    vi.useFakeTimers()
    const doc = setVisibility(makeDoc(), 'visible')
    const first = vi.fn()
    const second = vi.fn()
    scheduleReload(doc, first)
    scheduleReload(doc, second)
    setVisibility(doc, 'hidden')
    doc.dispatchEvent(new Event('visibilitychange'))
    expect(first).not.toHaveBeenCalled()
    expect(second).toHaveBeenCalledTimes(1)
  })
})

describe('shouldFullReloadOnNav', () => {
  it('is false when no update is pending', () => {
    expect(crossPageNav()).toBe(false)
  })
  it('is true for a cross-page navigation with an update pending', () => {
    scheduleReload(setVisibility(makeDoc(), 'visible'), vi.fn())
    expect(crossPageNav()).toBe(true)
  })
  it('leaves same-path navigations alone (in-player episode/provider query sync)', () => {
    scheduleReload(setVisibility(makeDoc(), 'visible'), vi.fn())
    expect(shouldFullReloadOnNav({ path: '/anime/x/watch' }, { path: '/anime/x/watch' })).toBe(false)
  })
  it('is false while an offline download is in flight (a full load would kill the engine)', () => {
    scheduleReload(setVisibility(makeDoc(), 'visible'), vi.fn())
    setActiveDownloadProbe(() => true)
    expect(crossPageNav()).toBe(false)
  })
})
