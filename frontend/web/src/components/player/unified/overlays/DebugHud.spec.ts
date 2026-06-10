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
})
