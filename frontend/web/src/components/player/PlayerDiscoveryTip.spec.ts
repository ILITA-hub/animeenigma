import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlayerDiscoveryTip from './PlayerDiscoveryTip.vue'

const global = {
  mocks: { $t: (key: string) => key },
  stubs: {
    Button: {
      template: '<button v-bind="$attrs"><slot /></button>',
    },
  },
}

afterEach(() => vi.restoreAllMocks())

describe('PlayerDiscoveryTip', () => {
  it('renders a random localized tip in a labelled, tinted strip', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0)
    const wrapper = mount(PlayerDiscoveryTip, { global })

    expect(wrapper.get('[data-test="player-discovery-tip"]').attributes('aria-label'))
      .toBe('player.discoveryTips.label')
    expect(wrapper.get('[data-test="tip-copy"]').text())
      .toBe('player.discoveryTips.items.downloads')
  })

  it('shuffles to a different tip', async () => {
    vi.spyOn(Math, 'random').mockReturnValueOnce(0).mockReturnValueOnce(0)
    const wrapper = mount(PlayerDiscoveryTip, { global })

    await wrapper.get('[data-test="shuffle-tip"]').trigger('click')

    expect(wrapper.get('[data-test="tip-copy"]').text())
      .toBe('player.discoveryTips.items.feedback')
  })
})
