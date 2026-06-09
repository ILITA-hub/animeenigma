import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PlayerControlBar from './PlayerControlBar.vue'

const baseProps = {
  playing: false,
  currentTime: 0,
  duration: 1421,
  volume: 0.8,
  muted: false,
  providerName: 'AllAnime',
  providerHue: '#00d4ff',
  audioLabel: 'Sub',
}

describe('PlayerControlBar', () => {
  it('shows a play affordance when playing is false', () => {
    const w = mount(PlayerControlBar, { props: { ...baseProps, playing: false } })
    // The play/pause button should be rendered
    const btn = w.find('[data-test="play-pause"]')
    expect(btn.exists()).toBe(true)
    expect(btn.attributes('aria-label')).toBe('Play')
  })

  it('shows a pause affordance when playing is true', () => {
    const w = mount(PlayerControlBar, { props: { ...baseProps, playing: true } })
    const btn = w.find('[data-test="play-pause"]')
    expect(btn.attributes('aria-label')).toBe('Pause')
  })

  it('emits toggle-play when the play button is clicked', async () => {
    const w = mount(PlayerControlBar, { props: baseProps })
    await w.find('[data-test="play-pause"]').trigger('click')
    expect(w.emitted('toggle-play')).toBeTruthy()
    expect(w.emitted('toggle-play')!.length).toBe(1)
  })

  it('emits toggle-source when the source pill is clicked', async () => {
    const w = mount(PlayerControlBar, { props: baseProps })
    await w.find('[data-test="source-pill"]').trigger('click')
    expect(w.emitted('toggle-source')).toBeTruthy()
  })

  it('renders the source pill with providerName and audioLabel', () => {
    const w = mount(PlayerControlBar, { props: baseProps })
    const pill = w.find('[data-test="source-pill"]')
    expect(pill.text()).toContain('AllAnime')
    expect(pill.text()).toContain('Sub')
  })

  it('emits seek-rel with -5 when back button is clicked', async () => {
    const w = mount(PlayerControlBar, { props: baseProps })
    await w.find('[data-test="seek-back"]').trigger('click')
    expect(w.emitted('seek-rel')?.[0]).toEqual([-5])
  })

  it('emits seek-rel with +5 when forward button is clicked', async () => {
    const w = mount(PlayerControlBar, { props: baseProps })
    await w.find('[data-test="seek-fwd"]').trigger('click')
    expect(w.emitted('seek-rel')?.[0]).toEqual([5])
  })

  it('formats and displays time labels', () => {
    const w = mount(PlayerControlBar, { props: { ...baseProps, currentTime: 125, duration: 1421 } })
    expect(w.find('[data-test="time-current"]').text()).toBe('2:05')
    expect(w.find('[data-test="time-duration"]').text()).toBe('23:41')
  })

  it('does not render a theater-mode button (hidden by request)', () => {
    const w = mount(PlayerControlBar, { props: baseProps })
    expect(w.find('[data-test="toggle-theater"]').exists()).toBe(false)
  })
})
