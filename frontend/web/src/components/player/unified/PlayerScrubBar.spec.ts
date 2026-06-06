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
})
