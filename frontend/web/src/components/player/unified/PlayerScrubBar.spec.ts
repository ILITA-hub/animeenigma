import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PlayerScrubBar from './PlayerScrubBar.vue'

describe('PlayerScrubBar', () => {
  it('renders fill at the given progress', () => {
    const w = mount(PlayerScrubBar, { props: { progress: 40, buffered: 55, durationSec: 1421, chapters: [] } })
    expect(w.find('[data-test="fill"]').attributes('style')).toContain('40%')
  })
  it('renders NO chapter markers when none provided', () => {
    const w = mount(PlayerScrubBar, { props: { progress: 0, buffered: 0, durationSec: 1421, chapters: [] } })
    expect(w.findAll('[data-test="chapter"]').length).toBe(0)
  })
  it('renders chapter markers when provided', () => {
    const w = mount(PlayerScrubBar, { props: { progress: 0, buffered: 0, durationSec: 1421, chapters: [{ kind: 'intro', startPct: 2, widthPct: 5 }] } })
    expect(w.findAll('[data-test="chapter"]').length).toBe(1)
  })
  it('emits seek with a 0..100 pct on click', async () => {
    const w = mount(PlayerScrubBar, { props: { progress: 0, buffered: 0, durationSec: 1421, chapters: [] } })
    await w.find('[data-test="track"]').trigger('click', { clientX: 50 })
    expect(w.emitted('seek')).toBeTruthy()
  })
  it('renders hacker-mode fragment segments with tones and labels', () => {
    const w = mount(PlayerScrubBar, {
      props: {
        progress: 10,
        buffered: 30,
        durationSec: 1400,
        chapters: [],
        fragments: [
          { startPct: 0, widthPct: 2, tone: 'ok' as const, label: '300 KB · 120 ms' },
          { startPct: 2, widthPct: 2, tone: 'bad' as const, label: '2048 KB · 900 ms' },
        ],
      },
    })
    const frags = w.findAll('[data-test="frag"]')
    expect(frags.length).toBe(2)
    expect(frags[0].attributes('data-tone')).toBe('ok')
    expect(frags[1].attributes('data-tone')).toBe('bad')
    expect(frags[0].attributes('title')).toBe('300 KB · 120 ms')
  })
  it('renders no fragment layer by default', () => {
    const w = mount(PlayerScrubBar, {
      props: { progress: 10, buffered: 30, durationSec: 1400, chapters: [] },
    })
    expect(w.findAll('[data-test="frag"]').length).toBe(0)
  })
})
