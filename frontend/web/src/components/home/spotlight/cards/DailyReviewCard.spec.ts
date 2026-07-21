import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' },
  }),
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string, nameRu?: string, nameJp?: string) =>
    nameRu || name || nameJp || '',
}))

vi.mock('@/composables/useImageProxy', () => ({
  cardPosterUrl: (url: string, width: number) => `${url}?w=${width}`,
}))

vi.mock('@/utils/preload-image', () => ({
  isImageWarm: () => false,
  markImageWarm: vi.fn(),
}))

import DailyReviewCard from './DailyReviewCard.vue'
import type { DailyReviewData } from '@/types/spotlight'

const baseData: DailyReviewData = {
  review_id: 'review-1',
  anime: {
    id: 'anime-1',
    name: 'The Anime',
    name_ru: 'Это аниме',
    name_jp: 'このアニメ',
    poster_url: '/poster.jpg',
  },
  author: {
    username: 'alice',
    public_id: 'alice-public',
    avatar: '/avatar.png',
  },
  score: 9,
  review_text: 'A thoughtful review of the whole season.',
  created_at: '2026-07-20T12:00:00Z',
}

function mountCard(data: DailyReviewData = baseData) {
  return mount(DailyReviewCard, {
    props: { data },
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('DailyReviewCard', () => {
  it('features the localized anime, public author, avatar and review text', () => {
    const wrapper = mountCard()
    expect(wrapper.text()).toContain('Это аниме')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain(baseData.review_text)
    expect(wrapper.find('img[alt="alice"]').attributes('src')).toBe('/avatar.png')
  })

  it('uses the canonical score diamond and shows a non-zero score', () => {
    const wrapper = mountCard()
    expect(wrapper.text()).toContain('9/10')
    expect(wrapper.find('svg.text-cyan-400').exists()).toBe(true)
  })

  it('omits the score badge for an unscored written review', () => {
    const wrapper = mountCard({ ...baseData, score: 0 })
    expect(wrapper.text()).not.toContain('/10')
  })

  it('links the author by public_id and the CTA to the anime review section', () => {
    const wrapper = mountCard()
    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links.some((link) => link.props('to') === '/user/alice-public')).toBe(true)
    expect(
      links.some(
        (link) => link.props('to') === '/anime/anime-1?ugc=reviews#section-comments',
      ),
    ).toBe(true)
  })

  it('renders an author without a profile link when public_id is absent', () => {
    const wrapper = mountCard({
      ...baseData,
      author: { ...baseData.author, public_id: undefined },
    })
    expect(wrapper.find('[data-testid="daily-review-author-link"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('alice')
  })

  it('escapes review markup instead of rendering it as HTML', () => {
    const wrapper = mountCard({
      ...baseData,
      review_text: '<img src=x onerror=alert(1)>',
    })
    const review = wrapper.find('[data-testid="daily-review-text"]')
    expect(review.find('img').exists()).toBe(false)
    expect(review.text()).toContain('<img src=x onerror=alert(1)>')
  })

  it('has a single root article for transition mode=out-in', () => {
    expect(mountCard().element.tagName).toBe('ARTICLE')
  })
})
