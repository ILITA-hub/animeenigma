/**
 * Workstream daily-fanfic-spotlight — Task 15.
 *
 * Vitest spec for DailyFanficCard.vue. Verifies:
 *   1. credited:true renders the author's username.
 *   2. credited:false renders the anon-author i18n key.
 *   3. ai_generated:true renders the AI badge.
 *   4. ai_generated:false does NOT render the AI badge.
 *   5. explicit:true + auth.isAuthenticated=false renders the login-gate
 *      text and does NOT render the excerpt text.
 *   6. explicit:true + auth.isAuthenticated=true renders the reader-gate
 *      text (not the login-gate text) and still hides the excerpt.
 *   7. explicit:false renders the excerpt text.
 *   8. part_count > 1 renders a part-count badge; part_count <= 1 does not.
 *   9. CTA router-links point to /fanfics?daily=1 and /fanfics.
 *  10. single-root <article> (SpotlightCardShell) — no fragment root.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import { ref } from 'vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

// Auth-store mock with a mutable `isAuthenticated` ref so individual tests
// can flip between logged-out and logged-in without a fresh module import
// (mirrors PersonalPickCard.spec.ts's mockAuthUser pattern).
const mockIsAuthenticated = ref(false)
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return mockIsAuthenticated.value
    },
  }),
}))

import DailyFanficCard from './DailyFanficCard.vue'
import type { DailyFanficData } from '@/types/spotlight'

function mountCard(data: DailyFanficData) {
  return mount(DailyFanficCard, {
    props: { data } as unknown as InstanceType<typeof DailyFanficCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const baseData: DailyFanficData = {
  id: 'ff-1',
  fanfic_title: 'A Chance Encounter',
  anime_title: 'Yani Neko',
  anime_japanese: 'ヤニねこ',
  anime_poster: '/poster.jpg',
  excerpt: 'It was a quiet afternoon when everything changed.',
  rating: 'PG-13',
  language: 'en',
  explicit: false,
  author_username: 'moonlit_writer',
  credited: true,
  ai_generated: false,
  part_count: 1,
  created_at: '2026-07-13T00:00:00Z',
}

beforeEach(() => {
  mockIsAuthenticated.value = false
})

describe('DailyFanficCard', () => {
  it('renders the credited author username', () => {
    const wrapper = mountCard({ ...baseData, credited: true, author_username: 'moonlit_writer' })
    expect(wrapper.text()).toContain('moonlit_writer')
  })

  it('renders the anon-author i18n key when not credited', () => {
    const wrapper = mountCard({ ...baseData, credited: false })
    expect(wrapper.text()).toContain('spotlight.dailyFanfic.anonAuthor')
    expect(wrapper.text()).not.toContain('moonlit_writer')
  })

  it('renders the AI badge when ai_generated is true', () => {
    const wrapper = mountCard({ ...baseData, ai_generated: true })
    expect(wrapper.text()).toContain('spotlight.dailyFanfic.aiBadge')
  })

  it('does NOT render the AI badge when ai_generated is false', () => {
    const wrapper = mountCard({ ...baseData, ai_generated: false })
    expect(wrapper.text()).not.toContain('spotlight.dailyFanfic.aiBadge')
  })

  it('explicit + not authed: renders the login gate and hides the excerpt', () => {
    mockIsAuthenticated.value = false
    const wrapper = mountCard({ ...baseData, explicit: true })
    expect(wrapper.text()).toContain('spotlight.dailyFanfic.explicitLogin')
    expect(wrapper.text()).not.toContain('spotlight.dailyFanfic.explicitReader')
    expect(wrapper.text()).not.toContain(baseData.excerpt)
  })

  it('explicit + authed: renders the reader gate (not the login gate) and hides the excerpt', () => {
    mockIsAuthenticated.value = true
    const wrapper = mountCard({ ...baseData, explicit: true })
    expect(wrapper.text()).toContain('spotlight.dailyFanfic.explicitReader')
    expect(wrapper.text()).not.toContain('spotlight.dailyFanfic.explicitLogin')
    expect(wrapper.text()).not.toContain(baseData.excerpt)
  })

  it('non-explicit: renders the excerpt text', () => {
    const wrapper = mountCard({ ...baseData, explicit: false })
    expect(wrapper.text()).toContain(baseData.excerpt)
  })

  it('renders a part-count badge when part_count > 1', () => {
    const wrapper = mountCard({ ...baseData, part_count: 3 })
    expect(wrapper.text()).toContain('spotlight.dailyFanfic.partsLabel')
  })

  it('does NOT render a part-count badge when part_count is 1', () => {
    const wrapper = mountCard({ ...baseData, part_count: 1 })
    expect(wrapper.text()).not.toContain('spotlight.dailyFanfic.partsLabel')
  })

  it('CTA hrefs: primary -> /fanfics?daily=1, secondary -> /fanfics', () => {
    const wrapper = mountCard(baseData)
    const links = wrapper.findAllComponents(RouterLinkStub)
    const primary = links.find((l) => l.props('to') === '/fanfics?daily=1')
    const secondary = links.find((l) => l.props('to') === '/fanfics')
    expect(primary).toBeDefined()
    expect(secondary).toBeDefined()
  })

  it('has a single root <article> (SpotlightCardShell) — no fragment root', () => {
    expect(mountCard(baseData).element.tagName).toBe('ARTICLE')
  })
})
