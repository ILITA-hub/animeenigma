import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PlaybackSettingsMenu from './PlaybackSettingsMenu.vue'

const baseProps = {
  quality: 'Auto',
  qualities: ['Auto', '720p'],
  speed: 1,
  speeds: [1, 1.5],
  autoNext: false,
  autoSkip: false,
  hackerMode: false,
}

const global = {
  mocks: { $t: (k: string) => k },
  stubs: { Switch: true },
}

describe('PlaybackSettingsMenu — share', () => {
  it('renders the share-moment row and emits `share` on click', async () => {
    const wrapper = mount(PlaybackSettingsMenu, { props: baseProps, global })
    const row = wrapper.get('[data-test="share-moment"]')
    expect(row.text()).toContain('player.aePlayer.shareMoment')
    await row.trigger('click')
    expect(wrapper.emitted('share')).toHaveLength(1)
  })

  it('hides the share row inside the quality sub-view', async () => {
    const wrapper = mount(PlaybackSettingsMenu, { props: baseProps, global })
    // enter the quality sub-view
    await wrapper.findAll('button').find((b) => b.text().includes('player.aePlayer.quality'))!.trigger('click')
    expect(wrapper.find('[data-test="share-moment"]').exists()).toBe(false)
  })
})
