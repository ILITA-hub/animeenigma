import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => (p ? `${k}::${JSON.stringify(p)}` : k) }),
}))

import PosterRow from './PosterRow.vue'
import type { AnimeCardModel } from '@/types/card'

const RouterLinkStub = { name: 'RouterLink', props: ['to'], template: '<a :href="to"><slot /></a>' }

function mountRow(model: Partial<AnimeCardModel> = {}, variant: 'ongoing' | 'top' | 'announced' = 'ongoing', rank?: number) {
  const full: AnimeCardModel = {
    id: '1', href: '/anime/1', title: 'Frieren: Beyond Journey\'s End',
    coverImage: 'http://x/p.jpg', year: 2023, episodes: 28,
    malScore: 8.9, siteScore: 9.4, airing: true,
    nextEpisode: { ep: 6, when: '2026-06-10T12:00:00Z' }, listStatus: null, progress: null,
    ...model,
  }
  return mount(PosterRow, {
    props: { model: full, variant, rank },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('PosterRow', () => {
  it('renders a single-line title (truncate class)', () => {
    const w = mountRow()
    expect(w.find('[data-testid="row-title"]').classes()).toContain('truncate')
  })

  it('shows the ongoing chip + next-ep line for the ongoing variant', () => {
    const w = mountRow({}, 'ongoing')
    expect(w.find('[data-testid="airing"]').exists()).toBe(true)
    expect(w.find('[data-testid="next-ep"]').exists()).toBe(true)
  })

  it('shows the rank numeral for the top variant', () => {
    const w = mountRow({}, 'top', 1)
    expect(w.find('[data-testid="rank"]').text()).toBe('1')
  })

  it('shows the season chip for the announced variant', () => {
    const full: AnimeCardModel = {
      id: '1', href: '/anime/1', title: 'Frieren: Beyond Journey\'s End',
      coverImage: 'http://x/p.jpg', year: 2023, episodes: 28,
      malScore: 8.9, siteScore: 9.4, airing: true,
      nextEpisode: { ep: 6, when: '2026-06-10T12:00:00Z' }, listStatus: null, progress: null,
    }
    const w = mount(PosterRow, {
      props: { model: full, variant: 'announced', season: 'winter' },
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    expect(w.find('[data-testid="season"]').exists()).toBe(true)
  })

  it('renders the centered-glass kebab and emits openMenu', async () => {
    const w = mountRow()
    const kebab = w.find('[data-testid="row-kebab"]')
    expect(kebab.exists()).toBe(true)
    await kebab.trigger('click')
    expect(w.emitted('openMenu')).toBeTruthy()
  })
})
