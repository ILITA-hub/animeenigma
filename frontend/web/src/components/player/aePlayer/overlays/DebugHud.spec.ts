import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import DebugHud from './DebugHud.vue'
import { scrubDebug, sreset, slog } from '@/composables/aePlayer/scrubPreviewDebug'

const baseProps = {
  stats: {
    readyState: 4,
    bufferAheadSec: 42.31,
    bufferBehindSec: 12.18,
    droppedFrames: 3,
    totalFrames: 28741,
    resolution: '1920×1080',
  },
  frags: [
    { start: 0, duration: 6, size: 512 * 1024, loadMs: 230 },
    { start: 6, duration: 6, size: 2 * 1024 * 1024, loadMs: 800 },
  ],
  bandwidth: 4_200_000,
  provider: 'Gogoanime',
  streamType: 'hls',
  levelLabel: '1080p',
}

describe('DebugHud', () => {
  beforeEach(() => sreset())

  it('hides the PREVIEW section while the thumbnail engine is idle', () => {
    const w = mount(DebugHud, { props: baseProps })
    expect(w.find('[data-test="hud-preview-head"]').exists()).toBe(false)
  })

  it('shows thumbnail-engine gauges, errors and recent events once active', () => {
    scrubDebug.engine = 'ready'
    scrubDebug.cacheSize = 7
    scrubDebug.queueLen = 3
    scrubDebug.seeks = 5
    scrubDebug.captures = 4
    scrubDebug.watchdogs = 1
    scrubDebug.lastCaptureMs = 1840
    scrubDebug.errors = 2
    scrubDebug.lastError = 'networkError/fragLoadError FATAL http=502'
    slog('seek →100s b20 (prefetch) rs=2')

    const w = mount(DebugHud, { props: baseProps })
    expect(w.find('[data-test="hud-preview-head"]').text()).toContain(
      'PREVIEW ready · cache 7 · queue 3',
    )
    expect(w.find('[data-test="hud-preview-stats"]').text()).toContain('seek 5 → cap 4 · wd 1')
    expect(w.text()).toContain('1840ms')
    expect(w.find('[data-test="hud-preview-error"]').text()).toContain(
      'ERR×2 networkError/fragLoadError FATAL http=502',
    )
    expect(w.find('[data-test="hud-preview-event"]').text()).toContain('seek →100s b20 (prefetch)')
  })

  it('renders provider, type and level in the header row', () => {
    const w = mount(DebugHud, { props: baseProps })
    const head = w.find('.pl-hud-head').text()
    expect(head).toContain('Gogoanime')
    expect(head).toContain('hls')
    expect(head).toContain('1080p')
  })

  it('shows the SELECTED COMBO + why when a decision is provided', () => {
    const w = mount(DebugHud, {
      props: {
        ...baseProps,
        decision: {
          provider: 'kodik',
          audio: 'sub',
          lang: 'ru',
          team: 'AniLibria',
          reason: 'smart default — best playable source (rank 1 of 6)',
        },
      },
    })
    expect(w.find('[data-test="hud-decision-head"]').exists()).toBe(true)
    const combo = w.find('[data-test="hud-decision-combo"]').text()
    expect(combo).toContain('kodik')
    expect(combo).toContain('sub')
    expect(combo).toContain('AniLibria')
    expect(w.find('[data-test="hud-decision-why"]').text()).toContain('smart default')
  })

  it('omits the SELECTED COMBO section when no decision is set', () => {
    const w = mount(DebugHud, { props: baseProps })
    expect(w.find('[data-test="hud-decision-head"]').exists()).toBe(false)
  })

  it('formats bandwidth in Mbit/s and the buffer window', () => {
    const w = mount(DebugHud, { props: baseProps })
    expect(w.text()).toContain('4.2 Mbit/s')
    expect(w.text()).toContain('+42.3s')
    expect(w.text()).toContain('12.2s')
  })

  it('lists recent fragments with sizes (KB/MB) and load times', () => {
    const w = mount(DebugHud, { props: baseProps })
    expect(w.text()).toContain('512KB')
    expect(w.text()).toContain('2.0MB')
    expect(w.text()).toContain('230ms')
  })

  it('shows a placeholder instead of fragments for mp4', () => {
    const w = mount(DebugHud, { props: { ...baseProps, frags: [], streamType: 'mp4' } })
    expect(w.text()).toContain('no fragments')
  })

  it('renders no seek section without a trace', () => {
    const w = mount(DebugHud, { props: baseProps })
    expect(w.find('[data-test="hud-seek-head"]').exists()).toBe(false)
  })

  const seekBase = { t0: 0, fetchMs: null, fetchedRange: null, seekedMs: null, resumeMs: null, frags: 0, bytes: 0, done: false }

  it('renders a buffer-HIT seek trace as the short path (3 steps fit)', () => {
    const w = mount(DebugHud, {
      props: {
        ...baseProps,
        seek: { ...seekBase, target: 754, bufferHit: true, seekedMs: 18, resumeMs: 24, done: true },
      },
    })
    expect(w.find('[data-test="hud-seek-head"]').text()).toContain('SEEK →12:34 · buffer HIT')
    expect(w.text()).toContain('no network')
    expect(w.text()).toContain('decode keyframe→target 18ms')
    expect(w.text()).toContain('resume rs≥3 24ms')
  })

  it('shows only the LATEST 3 pipeline steps on a buffer miss (check/flush cut)', () => {
    const w = mount(DebugHud, {
      props: {
        ...baseProps,
        seek: { ...seekBase, target: 90, bufferHit: false, fetchMs: 610, fetchedRange: [88, 102] as [number, number], seekedMs: 840, frags: 3, bytes: 2.1 * 1024 * 1024 },
      },
    })
    const steps = w.findAll('[data-test="hud-seek-step"]')
    expect(steps.length).toBe(3)
    expect(w.text()).not.toContain('flush')
    expect(w.text()).toContain('3 frags · 2.1MB · 610ms')
    expect(w.text()).toContain('resume rs≥3 — buffering')
  })

  it('details the mp4 fetch as a moov-index range request with timing', () => {
    const w = mount(DebugHud, {
      props: {
        ...baseProps,
        streamType: 'mp4',
        seek: { ...seekBase, target: 90, bufferHit: false, fetchMs: 320, fetchedRange: [85, 110] as [number, number] },
      },
    })
    expect(w.text()).toContain('range req (moov→bytes) · 320ms → 85–110s buffered')
  })

  it('shows a mono spinner next to the in-flight step', () => {
    const w = mount(DebugHud, {
      props: {
        ...baseProps,
        seek: { ...seekBase, target: 90, bufferHit: false },
      },
    })
    expect(w.find('[data-test="hud-seek-step"] .ae-spinner').exists()).toBe(true)
  })

  it('emits update:pinned from the pin checkbox', async () => {
    const w = mount(DebugHud, { props: { ...baseProps, pinned: false } })
    const pin = w.find('[data-test="hud-pin-toggle"]')
    expect(pin.attributes('aria-checked')).toBe('false')
    await pin.trigger('click')
    expect(w.emitted('update:pinned')?.[0]).toEqual([true])
  })

  it('applies the fading class while lingering out', () => {
    const w = mount(DebugHud, { props: { ...baseProps, fading: true } })
    expect(w.find('[data-test="debug-hud"]').classes()).toContain('pl-hud--fading')
  })

  it('hides the SOURCE FALLBACK section when there are no intents', () => {
    const w = mount(DebugHud, { props: baseProps })
    expect(w.find('[data-test="hud-fallback-head"]').exists()).toBe(false)
  })

  it('renders source-fallback intents, marking logged-only vs switched', () => {
    const w = mount(DebugHud, {
      props: {
        ...baseProps,
        intents: [
          { at: 1.2, from: 'ae', to: 'allanime', reason: 'saved source unavailable', acted: false },
          { at: 2.5, from: 'kodik', to: 'miruro', reason: 'saved source unavailable', acted: true },
        ],
      },
    })
    expect(w.find('[data-test="hud-fallback-head"]').exists()).toBe(true)
    const rows = w.findAll('[data-test="hud-fallback-intent"]')
    expect(rows).toHaveLength(2)
    // newest first
    expect(rows[0].text()).toContain('switched kodik → miruro')
    expect(rows[1].text()).toContain('intent ae → allanime')
  })

  it('toggles the seek-pipeline tech reference via the "?" button', async () => {
    const w = mount(DebugHud, { props: baseProps })
    expect(w.find('[data-test="hud-reference"]').exists()).toBe(false)
    await w.find('[data-test="hud-help-toggle"]').trigger('click')
    expect(w.find('[data-test="hud-reference"]').exists()).toBe(true)
    expect(w.find('[data-test="hud-reference"]').text()).toContain('keyframe')
    await w.find('[data-test="hud-help-toggle"]').trigger('click')
    expect(w.find('[data-test="hud-reference"]').exists()).toBe(false)
  })
})
