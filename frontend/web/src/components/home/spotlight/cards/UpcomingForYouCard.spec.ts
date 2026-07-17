/**
 * Workstream announcement-recs-spotlight — upcoming_for_you card
 * (spec 2026-07-17).
 *
 * Vitest spec for UpcomingForYouCard.vue, mirroring
 * ContinueWatchingNewCard.spec.ts's mocking conventions (vue-i18n key-echo,
 * RouterLinkStub) plus mocks for the api client + watchlist store the
 * add/dismiss CTAs call.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string, nameRu?: string, nameJp?: string) =>
    name || nameRu || nameJp || '',
}))

const postMock = vi.fn().mockResolvedValue({ data: {} })
vi.mock('@/api/client', () => ({
  apiClient: { post: (...args: unknown[]) => postMock(...args) },
}))

const setStatusMock = vi.fn().mockResolvedValue(undefined)
vi.mock('@/stores/watchlist', () => ({
  useWatchlistStore: () => ({ setStatusOptimistic: setStatusMock }),
}))

import UpcomingForYouCard from './UpcomingForYouCard.vue'

function mountCard(items: unknown[]) {
  return mount(UpcomingForYouCard, {
    props: { data: { items } } as unknown as InstanceType<
      typeof UpcomingForYouCard
    >['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const franchiseItem = {
  anime: { id: 'a-1', name: 'Frieren S2', poster_url: '/p.jpg', year: 2027, kind: 'tv' },
  match_score: 0.61,
  reason: {
    kind: 'franchise',
    seed_anime_id: 's-1',
    seed_anime_name: 'Frieren',
    user_score: 9,
  },
}
const tasteItem = {
  anime: { id: 'a-2', name: 'Other Show', poster_url: '/q.jpg' },
  match_score: 0.4,
  reason: { kind: 'taste' },
}

beforeEach(() => {
  postMock.mockClear()
  setStatusMock.mockClear()
})

describe('UpcomingForYouCard', () => {
  it('renders a single <article> root', () => {
    expect(mountCard([franchiseItem]).element.tagName).toBe('ARTICLE')
  })

  it('renders the franchise reason with seed name and score', () => {
    const w = mountCard([franchiseItem])
    expect(w.find('[data-testid="ufy-reason"]').text()).toContain('reasonFranchise')
    expect(w.find('[data-testid="ufy-reason"]').text()).toContain('Frieren')
    expect(w.find('[data-testid="ufy-reason"]').text()).toContain('9')
  })

  it('add-to-plan calls the watchlist store and advances', async () => {
    const w = mountCard([franchiseItem, tasteItem])
    await w.find('[data-testid="ufy-add"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(setStatusMock).toHaveBeenCalledWith('a-1', 'plan_to_watch')
    expect(w.text()).toContain('Other Show')
  })

  it('dismiss posts to the recs endpoint and advances to done state', async () => {
    const w = mountCard([franchiseItem])
    await w.find('[data-testid="ufy-dismiss"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(postMock).toHaveBeenCalledWith('/users/recs/upcoming/dismiss', {
      anime_id: 'a-1',
    })
    expect(w.find('[data-testid="ufy-done"]').exists()).toBe(true)
  })

  it('taste reason renders the taste key', () => {
    const w = mountCard([tasteItem])
    expect(w.find('[data-testid="ufy-reason"]').text()).toContain('reasonTaste')
  })
})
