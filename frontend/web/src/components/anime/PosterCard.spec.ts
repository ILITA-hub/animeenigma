import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => (p ? `${k}::${JSON.stringify(p)}` : k) }),
}))

import PosterCard from './PosterCard.vue'
import type { AnimeCardModel } from '@/types/card'

const RouterLinkStub = { name: 'RouterLink', props: ['to'], template: '<a :href="to"><slot /></a>' }

function mountCard(model: Partial<AnimeCardModel> = {}) {
  const full: AnimeCardModel = {
    id: '1', href: '/anime/1', title: 'Frieren', coverImage: 'http://x/p.jpg',
    year: 2023, episodes: 28, primaryGenre: 'Adventure',
    malScore: 8.9, siteScore: 9.4, listStatus: null, progress: null, airing: false,
    ...model,
  }
  return mount(PosterCard, {
    props: { model: full },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('PosterCard', () => {
  it('renders title, meta and a link to the anime', () => {
    const w = mountCard()
    expect(w.text()).toContain('Frieren')
    expect(w.text()).toContain('2023')
    expect(w.text()).toContain('Adventure')
    expect(w.find('a').attributes('href')).toBe('/anime/1')
  })

  it('shows both scores with tabular-nums (alignment)', () => {
    const w = mountCard()
    const scoreEls = w.findAll('[data-testid="score"]')
    expect(scoreEls.length).toBe(2)
    scoreEls.forEach((el) => expect(el.classes()).toContain('tabular-nums'))
  })

  it('keeps scores visible on hover (no opacity-0 on the score cluster)', () => {
    const w = mountCard()
    const cluster = w.find('[data-testid="score-cluster"]')
    expect(cluster.classes()).not.toContain('opacity-0')
    expect(cluster.classes()).not.toContain('group-hover:opacity-0')
  })

  it('renders the centered play+kebab cluster with the kebab', () => {
    const w = mountCard()
    expect(w.find('[data-testid="play-cluster"]').exists()).toBe(true)
    expect(w.findComponent({ name: 'AnimeKebab' }).exists()).toBe(true)
  })

  it('emits openMenu with the kebab element', async () => {
    const w = mountCard()
    await w.findComponent({ name: 'AnimeKebab' }).find('button').trigger('click')
    expect(w.emitted('openMenu')).toBeTruthy()
    expect(w.emitted('openMenu')![0][0]).toBeInstanceOf(HTMLButtonElement)
  })

  it('renders the ongoing badge only while airing', () => {
    expect(mountCard({ airing: true }).find('[data-testid="ongoing"]').exists()).toBe(true)
    expect(mountCard({ airing: false }).find('[data-testid="ongoing"]').exists()).toBe(false)
  })

  it('quality badge renders the quality string', () => {
    const w = mountCard({ quality: '1080p' })
    expect(w.text()).toContain('1080p')
  })

  it('DUB badge renders when hasDub is true', () => {
    const w = mountCard({ hasDub: true })
    expect(w.text()).toContain('card.dubBadge')
  })

  it('status badge renders the i18n key for dropped', () => {
    const w = mountCard({ listStatus: 'dropped' })
    expect(w.text()).toContain('profile.watchlist.dropped')
  })

  it('renders the user-score badge as a plain integer', () => {
    const w = mountCard({ malScore: undefined, siteScore: undefined, userScore: 7 })
    const badge = w.find('[data-testid="user-score"]')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toContain('7')
    expect(badge.text()).not.toContain('7.0')
  })

  it('user-score badge emits editScore on click only when scoreEditable', async () => {
    const editable = mountCard({ userScore: 7 })
    await editable.setProps({ scoreEditable: true })
    await editable.find('[data-testid="user-score"]').trigger('click')
    expect(editable.emitted('editScore')).toBeTruthy()

    const readonly = mountCard({ userScore: 7 })
    await readonly.find('[data-testid="user-score"]').trigger('click')
    expect(readonly.emitted('editScore')).toBeFalsy()
    expect(readonly.find('[data-testid="user-score"]').attributes('role')).toBeUndefined()
  })

  it('hides the user-score badge when no score is set', () => {
    const w = mountCard({ malScore: undefined, siteScore: undefined, userScore: undefined })
    expect(w.find('[data-testid="user-score"]').exists()).toBe(false)
    expect(w.find('[data-testid="score-cluster"]').exists()).toBe(false)
  })

  it('progress badge shows when in-progress and suppressed when completed', () => {
    const inProgress = mountCard({ listStatus: 'watching', progress: { current: 5, total: 12 } })
    expect(inProgress.text()).toContain('card.episodeProgress')

    const completed = mountCard({ listStatus: 'completed', progress: { current: 12, total: 12 } })
    expect(completed.text()).not.toContain('card.episodeProgress')
  })
})
