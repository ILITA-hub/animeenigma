import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import EpisodeSelector from '../EpisodeSelector.vue'
import type { EpisodeOption } from '../EpisodeSelector.types'

const eps: EpisodeOption[] = [
  { key: 1, label: 1, number: 1 },
  { key: 2, label: 2, number: 2 },
  { key: 3, label: 3, number: 3 },
]

describe('EpisodeSelector', () => {
  it('renders one button per episode', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null } })
    expect(w.findAll('button')).toHaveLength(3)
  })

  it('marks the selected episode with accent-bg', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: 2 } })
    const btns = w.findAll('button')
    expect(btns[1].classes()).toContain('accent-bg')
    expect(btns[0].classes()).not.toContain('accent-bg')
  })

  it('shows watched styling + badge for episodes <= watchedUpTo', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null, watchedUpTo: 2 } })
    const btns = w.findAll('button')
    expect(btns[0].classes()).toContain('accent-bg-muted')
    expect(btns[0].find('svg').exists()).toBe(true)
  })

  it('does not mark episodes beyond watchedUpTo as watched', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null, watchedUpTo: 2 } })
    const btns = w.findAll('button')
    expect(btns[2].classes()).not.toContain('accent-bg-muted')
    expect(btns[2].find('svg').exists()).toBe(false)
  })

  it('does not show the watched badge on the selected episode', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: 1, watchedUpTo: 3 } })
    const first = w.findAll('button')[0]
    expect(first.classes()).toContain('accent-bg')
    expect(first.find('svg').exists()).toBe(false)
  })

  it('emits select with the episode key on click', async () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null } })
    await w.findAll('button')[2].trigger('click')
    expect(w.emitted('select')?.[0]).toEqual([3])
  })

  it('exposes data-wt-id for the presence layer', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null } })
    expect(w.findAll('button')[0].attributes('data-wt-id')).toBe('episode:1')
  })

  it('matches selection across string/number key types', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: '2' } })
    expect(w.findAll('button')[1].classes()).toContain('accent-bg')
  })

  it('marks the boundary watched episode (number === watchedUpTo)', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null, watchedUpTo: 2 } })
    const btns = w.findAll('button')
    expect(btns[1].classes()).toContain('accent-bg-muted') // ep 2, the last watched
    expect(btns[2].classes()).not.toContain('accent-bg-muted') // ep 3, unwatched
  })
})
