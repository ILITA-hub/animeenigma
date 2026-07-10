import { describe, it, expect, vi } from 'vitest'
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

import CuratedCard from './CuratedCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(CuratedCard, {
    props: props as unknown as InstanceType<typeof CuratedCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const data = {
  anime: {
    id: 'a1',
    name: 'Yani Neko',
    name_ru: 'Табакошка',
    name_jp: 'ヤニねこ',
    status: 'ongoing',
    episodes_aired: 2,
    score: 7.0,
    year: 2026,
    episodes_count: 12,
    description: undefined as string | undefined,
    genres: [] as { id: string; name?: string; russian?: string }[],
  },
}

describe('CuratedCard', () => {
  it('renders the curated kicker title', () => {
    expect(mountCard({ data }).text()).toContain('spotlight.curated.title')
  })

  it('renders the watch CTA with next episode = aired + 1', () => {
    expect(mountCard({ data }).text()).toContain('"n":3')
  })

  it('renders the add-to-list CTA', () => {
    expect(mountCard({ data }).text()).toContain('spotlight.curated.addCta')
  })

  it('renders JP subtitle and score', () => {
    const text = mountCard({ data }).text()
    expect(text).toContain('ヤニねこ')
    expect(text).toContain('7.0')
  })

  it('links the primary CTA to the watch route', () => {
    const links = mountCard({ data }).findAllComponents(RouterLinkStub)
    const watch = links.find((l) => {
      const to = l.props('to') as string
      return typeof to === 'string' && to.includes('/anime/a1') && to.includes('/watch')
    })
    expect(watch).toBeDefined()
  })

  it('has a single root <article> (SpotlightCardShell) — no fragment root', () => {
    expect(mountCard({ data }).element.tagName).toBe('ARTICLE')
  })
})
