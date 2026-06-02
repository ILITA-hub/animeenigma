import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import SubtitleSettingsMenu from './SubtitleSettingsMenu.vue'
import { useSubtitleTimingOffset } from '@/composables/useSubtitleTimingOffset'

const mountMenu = (hasActiveSub = true) =>
  mount(SubtitleSettingsMenu, {
    props: { hasActiveSub },
    global: { mocks: { $t: (k: string) => k } },
  })

beforeEach(() => {
  localStorage.clear()
  // Normalize the shared singleton between tests.
  useSubtitleTimingOffset().reset()
})

describe('SubtitleSettingsMenu', () => {
  it('disables the gear when there is no active subtitle', () => {
    const w = mountMenu(false)
    expect(w.get('[data-test="sub-timing-gear"]').attributes('disabled')).toBeDefined()
  })

  it('opens the popover and nudges the offset later (+0.1s)', async () => {
    const w = mountMenu(true)
    await w.get('[data-test="sub-timing-gear"]').trigger('click')
    await w.get('[data-test="nudge-plus-01"]').trigger('click')
    expect(w.get('[data-test="readout"]').text()).toBe('+0.1s')
  })

  it('nudges the offset earlier (-1s) and shows a negative readout', async () => {
    const w = mountMenu(true)
    await w.get('[data-test="sub-timing-gear"]').trigger('click')
    await w.get('[data-test="nudge-minus-1"]').trigger('click')
    expect(w.get('[data-test="readout"]').text()).toBe('-1.0s')
  })

  it('reset returns the readout to 0.0s', async () => {
    const w = mountMenu(true)
    await w.get('[data-test="sub-timing-gear"]').trigger('click')
    await w.get('[data-test="nudge-plus-1"]').trigger('click')
    expect(w.get('[data-test="readout"]').text()).toBe('+1.0s')
    await w.get('[data-test="reset"]').trigger('click')
    expect(w.get('[data-test="readout"]').text()).toBe('0.0s')
  })

  it('persists the offset to localStorage', async () => {
    const w = mountMenu(true)
    await w.get('[data-test="sub-timing-gear"]').trigger('click')
    await w.get('[data-test="nudge-plus-01"]').trigger('click')
    expect(localStorage.getItem('aenigma_subtitle_timing_offset')).toBe('0.1')
  })
})
