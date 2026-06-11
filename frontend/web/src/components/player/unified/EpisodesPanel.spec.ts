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

  it('marks episodes <= watchedUpTo as watched (check icon)', () => {
    const w = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 3, watchedUpTo: 2 },
    })
    expect(w.find('[data-test="episode-1"] [data-test="ep-watched"]').exists()).toBe(true)
    expect(w.find('[data-test="episode-2"] [data-test="ep-watched"]').exists()).toBe(true)
    expect(w.find('[data-test="episode-3"] [data-test="ep-watched"]').exists()).toBe(false)
  })

  it('marks completed-by-progress episodes as watched too', () => {
    const w = mount(EpisodesPanel, {
      props: {
        episodes: eps,
        selectedNumber: 1,
        progress: { 3: { pct: 1, completed: true } },
      },
    })
    expect(w.find('[data-test="episode-3"] [data-test="ep-watched"]').exists()).toBe(true)
  })

  it('renders a partial-progress underline sized by pct', () => {
    const w = mount(EpisodesPanel, {
      props: {
        episodes: eps,
        selectedNumber: 1,
        progress: { 2: { pct: 0.4, completed: false } },
      },
    })
    const bar = w.find('[data-test="episode-2"] [data-test="ep-progress"]')
    expect(bar.exists()).toBe(true)
    expect(bar.attributes('style')).toContain('40%')
    expect(w.find('[data-test="episode-1"] [data-test="ep-progress"]').exists()).toBe(false)
  })

  it('shows the mark-as-watched action only for logged-in users (canMark)', () => {
    const anon = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    expect(anon.find('[data-test="mark-watched"]').exists()).toBe(false)
    const authed = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 1, canMark: true },
    })
    expect(authed.find('[data-test="mark-watched"]').exists()).toBe(true)
    expect(authed.find('[data-test="mark-watched"]').text()).toContain('Mark ep. 1 as watched')
  })

  it('emits mark-watched on click and disables when already marked', async () => {
    const w = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 2, canMark: true },
    })
    await w.find('[data-test="mark-watched"]').trigger('click')
    expect(w.emitted('mark-watched')).toHaveLength(1)

    const done = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 2, canMark: true, marked: true },
    })
    const btn = done.find('[data-test="mark-watched"]')
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.text()).toContain('Ep. 2 watched')
  })

  it('exposes the episode title as a tooltip', () => {
    const w = mount(EpisodesPanel, {
      props: {
        episodes: [{ key: 1, label: 1, number: 1, title: 'The Journey Begins' }],
        selectedNumber: null,
      },
    })
    expect(w.find('[data-test="episode-1"]').attributes('title')).toBe('1. The Journey Begins')
  })
})
