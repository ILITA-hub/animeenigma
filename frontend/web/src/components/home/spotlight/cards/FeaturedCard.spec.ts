/**
 * Workstream hero-spotlight — Neon Tokyo redesign (feat/homepage-neon-tokyo-redesign).
 *
 * Vitest spec for FeaturedCard.vue (cinematic hero for the "featured" spotlight card type).
 * Verifies the status-aware eyebrow/CTA mapping and core structural assertions:
 *
 *   1. "ongoing" → eyebrowOngoing key + watchEpisode with n=aired+1
 *   2. "announced" → eyebrowAnnounced key + remindCta key
 *   3. "released" → eyebrowDefault key + watchCta key
 *   4. JP subtitle (name_jp) and score rendered
 *   5. Primary CTA links to /anime/{id}/watch for ongoing, /anime/{id} for announced
 *   6. Secondary addCta rendered
 *   7. Renders the localized title via getLocalizedTitle stub
 *
 * Mount pattern matches the sibling specs (RandomTailCard, NowWatchingCard):
 *   - vue-i18n stubbed via vi.mock with echo-key t()
 *   - @/utils/title stubbed with first-non-empty passthrough
 *   - router-link satisfied via RouterLinkStub (no full router needed)
 */

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

// Imported AFTER vi.mock so the SFC's useI18n() resolves to the stub.
import FeaturedCard from './FeaturedCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(FeaturedCard, {
    props: props as unknown as InstanceType<typeof FeaturedCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

function mk(status: string, episodes_aired = 0) {
  return {
    anime: {
      id: 'a1',
      name: 'Test',
      name_ru: 'Тест',
      name_jp: 'テスト',
      status,
      episodes_aired,
      score: 8.4,
      year: 2026,
      episodes_count: 12,
      description: undefined as string | undefined,
      genres: [] as { id: string; name?: string; russian?: string }[],
    },
  }
}

describe('FeaturedCard', () => {
  it('shows "now airing" eyebrow + episode CTA for ongoing', () => {
    const w = mountCard({ data: mk('ongoing', 7) })
    const text = w.text()
    // eyebrow key — echo mock renders the key string
    expect(text).toContain('spotlight.featured.eyebrowOngoing')
    // watchEpisode with n=8 (aired+1): rendered as key::{"n":8}
    expect(text).toContain('"n":8')
  })

  it('shows announcement eyebrow + remind CTA for announced', () => {
    const w = mountCard({ data: mk('announced') })
    const text = w.text()
    expect(text).toContain('spotlight.featured.eyebrowAnnounced')
    expect(text).toContain('spotlight.featured.remindCta')
  })

  it('shows default eyebrow + start CTA for released', () => {
    const w = mountCard({ data: mk('released') })
    const text = w.text()
    expect(text).toContain('spotlight.featured.eyebrowDefault')
    expect(text).toContain('spotlight.featured.watchCta')
  })

  it('renders the JP subtitle and score', () => {
    const w = mountCard({ data: mk('released') })
    const text = w.text()
    expect(text).toContain('テスト')
    expect(text).toContain('8.4')
  })

  it('links the primary CTA to the watch route for ongoing', () => {
    const w = mountCard({ data: mk('ongoing', 7) })
    const links = w.findAllComponents(RouterLinkStub)
    const watchLink = links.find((l) => {
      const to = l.props('to') as string
      return typeof to === 'string' && to.includes('/anime/a1') && to.includes('/watch')
    })
    expect(watchLink).toBeDefined()
  })

  it('links announced status PRIMARY CTA to anime detail (not watch)', () => {
    const w = mountCard({ data: mk('announced') })
    // Target the primary button specifically — secondary also links to /anime/a1
    // so a generic link search would pass trivially.
    const primaryBtn = w.find('.btn-primary-hero')
    expect(primaryBtn.exists()).toBe(true)
    const primaryLink = primaryBtn.findComponent(RouterLinkStub)
    const to = primaryLink.props('to') as string
    expect(to).toBe('/anime/a1')
  })

  it('renders the add-to-list secondary CTA', () => {
    const w = mountCard({ data: mk('released') })
    expect(w.text()).toContain('spotlight.featured.addCta')
  })

  it('parses Shikimori [character=ID] BBCode in the description instead of leaking raw tags', () => {
    const data = mk('released')
    data.anime.description =
      '[character=210882]Цукаса Акэурадзи[/character] мечтал бороться за медали.'
    const w = mountCard({ data })
    const desc = w.find('.featured-desc')
    expect(desc.exists()).toBe(true)
    // Must NOT leak the raw BBCode tag (regression: was bound via {{ }} before).
    expect(desc.text()).not.toContain('[character=')
    expect(desc.text()).not.toContain('[/character]')
    // The display name survives, rendered inside a Shikimori link.
    expect(desc.text()).toContain('Цукаса Акэурадзи')
    const link = desc.find('a.shiki-link')
    expect(link.exists()).toBe(true)
    expect(link.attributes('href')).toBe('https://shikimori.one/characters/210882')
  })
})
