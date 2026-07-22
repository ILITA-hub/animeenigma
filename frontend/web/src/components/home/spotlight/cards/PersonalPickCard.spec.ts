/**
 * Workstream hero-spotlight — v1.1-polish Phase 04 (HSB-V11-PP-01..04).
 *
 * Vitest spec for the refactored two-zone PersonalPickCard.vue.
 *
 * Verifies (against the new featured + secondary layout):
 *   1. Featured pick is the FIRST item, has an aria-label referencing the
 *      featured anime's title.
 *   2. Secondary list (<li>) count === min(items.length - 1, 2) on desktop.
 *   3. Username appears in the title when source='personal' AND the auth
 *      store exposes user.username → titleWithName key + {name} param.
 *   4. Mobile "+ N more →" footer button uses .cta-card classes (full-width
 *      footer), only rendered when items.length > 1.
 *   5. Mobile footer link routes to /browse?sort=trending for source=trending
 *      and to /recs for source=personal.
 *   6. Reason chip renders t(item.reason_i18n_key) for items that have one.
 *   7. SpotlightBackdrop variant="poster-blur" wraps the article and
 *      receives the featured anime's poster URL.
 *   8. Typography contract: only font-medium / font-semibold weights;
 *      tablet padding p-4 / md:p-6 / lg:p-8 (never p-5).
 *   9. Anonymous (source='trending') title uses titleAnon, NOT titleWithName.
 *  10. Logged-in with no username falls back to plain 'title' key.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import { ref } from 'vue'

// ── Mocks ────────────────────────────────────────────────────────────────

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

// Auth-store mock with a mutable `user` ref so individual tests can flip
// between anonymous (null), logged-in-without-username, and logged-in-with-
// username states without a fresh module import.
const mockAuthUser = ref<{ username?: string } | null>(null)
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get user() {
      return mockAuthUser.value
    },
  }),
}))

import PersonalPickCard from './PersonalPickCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(PersonalPickCard, {
    props: props as unknown as InstanceType<typeof PersonalPickCard>['$props'],
    global: {
      stubs: {
        'router-link': RouterLinkStub,
        // SpotlightBackdrop / SpotlightIcon stubbed to a deterministic root
        // element so we can assert their presence + forwarded props without
        // depending on their inline-SVG / decorative DOM internals.
        SpotlightBackdrop: {
          name: 'SpotlightBackdrop',
          props: ['variant', 'posterUrl', 'accent'],
          template:
            '<div data-testid="backdrop" :data-variant="variant" :data-poster-url="posterUrl"></div>',
        },
        SpotlightIcon: {
          name: 'SpotlightIcon',
          props: ['name'],
          template: '<span data-testid="icon" :data-name="name"></span>',
        },
      },
    },
  })
}

const animeFixture = (i: number) => ({
  id: `anime-${i}`,
  name: `Anime ${i}`,
  name_ru: `Аниме ${i}`,
  poster_url: `/poster-${i}.jpg`,
})

beforeEach(() => {
  // Reset auth user between tests so logged-in state never leaks.
  mockAuthUser.value = null
})

describe('PersonalPickCard (v1.1-polish two-zone layout)', () => {
  it('featured pick is the FIRST item and exposes an aria-label with its title', () => {
    const data = {
      source: 'personal',
      items: [
        { anime: animeFixture(1) },
        { anime: animeFixture(2) },
        { anime: animeFixture(3) },
      ],
    }
    const wrapper = mountCard({ data })
    const links = wrapper.findAllComponents(RouterLinkStub)
    // First router-link is the featured anchor — aria-label references the
    // featured (first) anime's title, NOT the second/third.
    const featuredLink = links.find(
      (l) => l.attributes('aria-label') === 'Anime 1',
    )
    expect(featuredLink).toBeDefined()
    expect(featuredLink!.props('to')).toBe('/anime/anime-1')
  })

  it('renders min(items-1, 2) secondary <li> rows on desktop', () => {
    const data = {
      source: 'personal',
      items: [
        { anime: animeFixture(1) },
        { anime: animeFixture(2) },
        { anime: animeFixture(3) },
      ],
    }
    const wrapper = mountCard({ data })
    const secondaryItems = wrapper.findAll('ul li')
    // 3 items total → 1 featured + 2 secondary.
    expect(secondaryItems.length).toBe(2)
  })

  it('renders only 1 secondary <li> row when 2 items total', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }, { anime: animeFixture(2) }],
    }
    const wrapper = mountCard({ data })
    expect(wrapper.findAll('ul li').length).toBe(1)
  })

  it('renders no secondary <li> rows when only 1 item', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    expect(wrapper.findAll('ul li').length).toBe(0)
  })

  it('username appears in title when source=personal and auth has a username', () => {
    mockAuthUser.value = { username: 'ui_audit_bot' }
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).toContain('spotlight.personalPick.titleWithName')
    expect(html).toContain('ui_audit_bot')
    // The {name} param must be carried through t() — our mock t() stringifies
    // params as ::JSON, so the param JSON must contain `"name":"ui_audit_bot"`.
    expect(html).toContain('"name":"ui_audit_bot"')
  })

  it('falls back to titleAnon when source=trending (even if username present)', () => {
    mockAuthUser.value = { username: 'ui_audit_bot' }
    const data = {
      source: 'trending',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    // `wrapper.text()` strips HTML comments — safe to grep for the i18n key.
    expect(wrapper.text()).toContain('spotlight.personalPick.titleAnon')
    expect(wrapper.text()).not.toContain(
      'spotlight.personalPick.titleWithName',
    )
  })

  it('falls back to plain title when source=personal but auth has no username', () => {
    // mockAuthUser stays null by default (anonymous-in-store).
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    const text = wrapper.text()
    // titleWithName i18n key must NOT render; plain `title` key does.
    expect(text).not.toContain('spotlight.personalPick.titleWithName')
    expect(text).toContain('spotlight.personalPick.title')
    expect(text).not.toContain('spotlight.personalPick.titleAnon')
  })

  it('renders the mobile poster swipe-row for secondary picks (v4 C-2)', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }, { anime: animeFixture(2) }],
    }
    const wrapper = mountCard({ data })
    const swipe = wrapper.find('[data-testid="rec-swipe"]')
    expect(swipe.exists()).toBe(true)
    expect(swipe.classes().join(' ')).toContain('overflow-x-auto')
  })

  it('does NOT render mobile footer button when only 1 item', () => {
    const data = {
      source: 'trending',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    expect(wrapper.text()).not.toContain('spotlight.personalPick.moreLink')
  })

  it('mobile footer link routes to /browse?sort=trending when source=trending', () => {
    const data = {
      source: 'trending',
      items: [{ anime: animeFixture(1) }, { anime: animeFixture(2) }],
    }
    const wrapper = mountCard({ data })
    const links = wrapper.findAllComponents(RouterLinkStub)
    const footer = links.find((l) => {
      const to = l.props('to') as unknown
      return typeof to === 'string' && to.includes('/browse')
    })
    expect(footer).toBeDefined()
    expect(footer!.props('to')).toBe('/browse?sort=trending')
  })

  it('«Все рекомендации» routes to the hidden recommendations page for source=personal', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }, { anime: animeFixture(2) }],
    }
    const wrapper = mountCard({ data })
    const links = wrapper.findAllComponents(RouterLinkStub)
    const footer = links.find((l) => l.props('to') === '/recs')
    expect(footer).toBeDefined()
  })

  it('renders reason chip with t(reason_i18n_key) for featured item that carries one', () => {
    const data = {
      source: 'personal',
      items: [
        {
          anime: animeFixture(1),
          reason_i18n_key: 'spotlight.personalPick.reason.personal',
        },
      ],
    }
    const wrapper = mountCard({ data })
    expect(wrapper.text()).toContain('spotlight.personalPick.reason.personal')
  })

  it('renders reason chip for each secondary item that carries reason_i18n_key', () => {
    const data = {
      source: 'personal',
      items: [
        { anime: animeFixture(1) }, // featured, no reason
        {
          anime: animeFixture(2),
          reason_i18n_key: 'spotlight.personalPick.reason.trending',
        },
        {
          anime: animeFixture(3),
          reason_i18n_key: 'spotlight.personalPick.reason.personal',
        },
      ],
    }
    const wrapper = mountCard({ data })
    // Both secondary reason keys flow through t() and appear in the DOM.
    expect(wrapper.text()).toContain('spotlight.personalPick.reason.trending')
    expect(wrapper.text()).toContain('spotlight.personalPick.reason.personal')
  })

  it('wraps the article with SpotlightBackdrop variant="poster-blur" + featured posterUrl', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    const backdrop = wrapper.find('[data-testid="backdrop"]')
    expect(backdrop.exists()).toBe(true)
    expect(backdrop.attributes('data-variant')).toBe('poster-blur')
    expect(backdrop.attributes('data-poster-url')).toBe('/poster-1.jpg')
  })

  it('uses only font-medium / font-semibold typography weights (no font-bold / font-normal)', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('font-normal')
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('uses p-4 / md:p-6 / lg:p-8 padding (never p-5)', () => {
    const data = {
      source: 'personal',
      items: [{ anime: animeFixture(1) }],
    }
    const wrapper = mountCard({ data })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bp-5\b/)
    expect(html).toMatch(/\bp-4\b/)
    expect(html).toMatch(/\bmd:p-6\b/)
    expect(html).toMatch(/\blg:p-8\b/)
  })
})
