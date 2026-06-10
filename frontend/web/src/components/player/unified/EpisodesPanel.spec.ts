import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import EpisodesPanel from './EpisodesPanel.vue'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

const eps: EpisodeOption[] = [
  { key: 1, label: 1, number: 1 },
  { key: 2, label: 2, number: 2 },
  { key: 3, label: 3, number: 3, isFiller: true },
]

describe('EpisodesPanel', () => {
  it('renders one button per episode + a count', () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    expect(w.findAll('[data-test^="episode-"]').length).toBe(3)
    expect(w.text()).toContain('3')
  })

  it('highlights the selected episode', () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 2 } })
    expect(w.find('[data-test="episode-2"]').classes().join(' ')).toContain('brand-cyan')
    expect(w.find('[data-test="episode-1"]').classes().join(' ')).not.toContain('brand-cyan')
  })

  it('emits select with the episode option on click', async () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    await w.find('[data-test="episode-2"]').trigger('click')
    expect(w.emitted('select')?.[0]).toEqual([eps[1]])
  })

  it('dims filler episodes', () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    expect(w.find('[data-test="episode-3"]').classes()).toContain('opacity-50')
  })

  it('shows an empty state when there are no episodes', () => {
    const w = mount(EpisodesPanel, { props: { episodes: [], selectedNumber: null } })
    expect(w.text()).toContain('No episodes from this source')
  })
})
