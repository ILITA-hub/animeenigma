import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DebugHud from './DebugHud.vue'

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
  it('renders provider, type and level in the header row', () => {
    const w = mount(DebugHud, { props: baseProps })
    const head = w.find('.pl-hud-head').text()
    expect(head).toContain('Gogoanime')
    expect(head).toContain('hls')
    expect(head).toContain('1080p')
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

  it('renders a buffer-HIT seek trace as the short path (no flush/fetch)', () => {
    const w = mount(DebugHud, {
      props: {
        ...baseProps,
        seek: { target: 754, bufferHit: true, t0: 0, seekedMs: 18, resumeMs: 24, frags: 0, bytes: 0, done: true },
      },
    })
    expect(w.find('[data-test="hud-seek-head"]').text()).toContain('SEEK →12:34 · buffer HIT')
    expect(w.text()).toContain('no network')
    expect(w.text()).not.toContain('flush')
    expect(w.text()).toContain('decode keyframe→target 18ms')
    expect(w.text()).toContain('resume rs≥3 24ms')
  })

  it('renders a buffer-MISS trace with flush, fetch totals and pending resume', () => {
    const w = mount(DebugHud, {
      props: {
        ...baseProps,
        seek: { target: 90, bufferHit: false, t0: 0, seekedMs: 840, resumeMs: null, frags: 3, bytes: 2.1 * 1024 * 1024, done: false },
      },
    })
    expect(w.find('[data-test="hud-seek-head"]').text()).toContain('buffer MISS')
    expect(w.text()).toContain('flush')
    expect(w.text()).toContain('3 frags · 2.1MB')
    expect(w.text()).toContain('… resume rs≥3 — buffering…')
  })

  it('labels the fetch step as a range request for mp4 streams', () => {
    const w = mount(DebugHud, {
      props: {
        ...baseProps,
        streamType: 'mp4',
        seek: { target: 90, bufferHit: false, t0: 0, seekedMs: null, resumeMs: null, frags: 0, bytes: 0, done: false },
      },
    })
    expect(w.text()).toContain('range request (moov index)')
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
