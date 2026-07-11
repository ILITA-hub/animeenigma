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

describe('PlaybackSettingsMenu — hacker-mode edge telemetry', () => {
  const stats = { bw: '5.0 Mbit/s', buffer: '+1s / −0s', level: '720p', frag: '400 KB · 200 ms' }

  it('renders EDGE/TRY/ROT lines for a Kodik source that rotated', () => {
    const wrapper = mount(PlaybackSettingsMenu, {
      props: {
        ...baseProps,
        hackerMode: true,
        debugStats: { ...stats, edge: 'p12', edgeTrail: 'p13 45.0s✗ → p12 0.21s✓', edgeRot: 1 },
      },
      global,
    })
    const panel = wrapper.get('[data-test="debug-stats"]').text()
    expect(wrapper.get('[data-test="debug-edge"]').text()).toContain('EDGE p12')
    expect(panel).toContain('TRY')
    expect(panel).toContain('p13 45.0s✗ → p12 0.21s✓') // logic + metrics, not just the decision
    expect(panel).toContain('ROT ×1')
  })

  it('omits the ROT line when nothing rotated (served the nominal edge)', () => {
    const wrapper = mount(PlaybackSettingsMenu, {
      props: {
        ...baseProps,
        hackerMode: true,
        debugStats: { ...stats, edge: 'p13', edgeTrail: 'p13 0.21s✓', edgeRot: 0 },
      },
      global,
    })
    const panel = wrapper.get('[data-test="debug-stats"]').text()
    expect(panel).toContain('EDGE p13')
    expect(panel).not.toContain('ROT')
  })

  it('shows no edge lines for a non-Kodik source (empty edge)', () => {
    const wrapper = mount(PlaybackSettingsMenu, {
      props: { ...baseProps, hackerMode: true, debugStats: { ...stats, edge: '', edgeTrail: '', edgeRot: 0 } },
      global,
    })
    expect(wrapper.find('[data-test="debug-edge"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="debug-stats"]').text()).not.toContain('EDGE')
  })
})

describe('PlaybackSettingsMenu — hacker-mode protocol ladder telemetry', () => {
  const stats = { bw: '5.0 Mbit/s', buffer: '+1s / −0s', level: '720p', frag: '400 KB · 200 ms' }
  const ladderStats = {
    proto: 'h2 · tier 2/3',
    net: '4.1 Mbps ewma / need 5.4 ×1.2',
    laddr: 'h3→h2 (first-frag projected 17s)',
    probe: 'h3 2.1 Mbps @03:24 — rejected (<1.1× h2)',
  }

  it('renders PROTO/NET/LADDR/PROBE rows when ladder telemetry is present', () => {
    const wrapper = mount(PlaybackSettingsMenu, {
      props: { ...baseProps, hackerMode: true, debugStats: { ...stats, ...ladderStats } },
      global,
    })
    const panel = wrapper.get('[data-test="debug-stats"]').text()
    expect(wrapper.get('[data-test="debug-proto"]').text()).toContain('PROTO h2 · tier 2/3')
    expect(panel).toContain('NET')
    expect(panel).toContain('4.1 Mbps ewma / need 5.4 ×1.2')
    expect(panel).toContain('LADDR')
    expect(panel).toContain('h3→h2 (first-frag projected 17s)')
    expect(panel).toContain('PROBE')
    expect(panel).toContain('h3 2.1 Mbps @03:24 — rejected (<1.1× h2)')
  })

  it('omits ladder rows when telemetry fields are absent (single-tier/dev)', () => {
    const wrapper = mount(PlaybackSettingsMenu, {
      props: { ...baseProps, hackerMode: true, debugStats: { ...stats } },
      global,
    })
    expect(wrapper.find('[data-test="debug-proto"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="debug-stats"]').text()).not.toContain('PROTO')
  })
})
