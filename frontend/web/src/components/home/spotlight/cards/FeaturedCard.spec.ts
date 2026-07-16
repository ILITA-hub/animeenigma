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
 *   6. Exactly one CTA — the secondary addCta was removed (AUTO-624)
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
    expect(w.findComponent(RouterLinkStub).props('to')).toBe('/anime/a1/watch')
  })

  it('links announced status PRIMARY CTA to anime detail (not watch)', () => {
    const w = mountCard({ data: mk('announced') })
    expect(w.findComponent(RouterLinkStub).props('to')).toBe('/anime/a1')
  })

  it('renders DS pills: amber lucide star score badge + neutral genre overlay badges', () => {
    const data = mk('ongoing', 9)
    data.anime.genres = [{ id: 'g1', name: 'Action', russian: 'Экшен' }]
    const w = mountCard({ data })
    // Score pill: warning variant text + overlay glass bg + lucide star svg.
    const scoreBadge = w
      .findAll('span')
      .find((s) => s.classes().includes('text-amber-400') && s.text().includes('8.4'))
    expect(scoreBadge).toBeDefined()
    expect(scoreBadge!.find('svg').exists()).toBe(true)
    // Ongoing status pill via airedLabel.
    expect(w.text()).toContain('spotlight.featured.airedLabel')
    // Genre pill is a neutral overlay badge (dark glass), not a rainbow chip.
    const genreBadge = w.findAll('span').find((s) => s.text() === 'Action')
    expect(genreBadge).toBeDefined()
    expect(genreBadge!.classes().join(' ')).toContain('bg-black/[0.62]')
  })

  it('renders a single watch CTA — no dead add-to-list link (AUTO-624)', () => {
    const w = mountCard({ data: mk('released') })
    expect(w.findAllComponents(RouterLinkStub).length).toBe(1)
    expect(w.text()).not.toContain('spotlight.featured.addCta')
  })

  it('parses Shikimori [character=ID] BBCode in the description instead of leaking raw tags', () => {
    const data = mk('released')
    data.anime.description =
      '[character=210882]Цукаса Акэурадзи[/character] мечтал бороться за медали.'
    const w = mountCard({ data })
    const desc = w.find('[data-testid="featured-desc"]')
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
