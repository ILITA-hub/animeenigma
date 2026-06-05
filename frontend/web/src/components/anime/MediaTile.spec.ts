import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => (p ? `${k}::${JSON.stringify(p)}` : k) }),
}))

import MediaTile from './MediaTile.vue'
import type { AnimeCardModel } from '@/types/card'

const RouterLinkStub = { name: 'RouterLink', props: ['to'], template: '<a :href="to"><slot /></a>' }

function mountTile(model: Partial<AnimeCardModel> = {}, progressPct = 0) {
  const full: AnimeCardModel = {
    id: '1', href: '/anime/1?episode=5', title: 'Dandadan', coverImage: 'http://x/p.jpg',
    episodes: 12, nextEpisode: { ep: 5, when: '' }, progress: { current: 5, total: 12 },
    listStatus: null, airing: false,
    ...model,
  }
  return mount(MediaTile, { props: { model: full, progressPct }, global: { stubs: { RouterLink: RouterLinkStub } } })
}

describe('MediaTile', () => {
  it('links to the episode-deep href', () => {
    expect(mountTile().find('a').attributes('href')).toBe('/anime/1?episode=5')
  })

  it('renders the kicker (episode) and the title', () => {
    const w = mountTile()
    expect(w.find('[data-testid="kicker"]').exists()).toBe(true)
    expect(w.text()).toContain('Dandadan')
  })

  it('renders the progress bar only when pct > 0', () => {
    expect(mountTile({}, 42).find('[data-testid="progress"]').exists()).toBe(true)
    expect(mountTile({}, 0).find('[data-testid="progress"]').exists()).toBe(false)
  })

  it('shows the drift skeleton until the image loads', async () => {
    const w = mountTile()
    expect(w.find('.sk-drift').exists()).toBe(true)
    await w.find('img').trigger('load')
    expect(w.find('.sk-drift').exists()).toBe(false)
  })
})
