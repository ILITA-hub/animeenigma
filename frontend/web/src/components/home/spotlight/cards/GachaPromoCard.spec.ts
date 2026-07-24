/**
 * «Лудка» launch promo card. Mirrors UpcomingForYouCard.spec.ts's mocking
 * conventions (vue-i18n key-echo, RouterLinkStub). The card is static —
 * everything renders from the GachaPromoData economy payload.
 */

import { describe, it, expect } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import { vi } from 'vitest'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

import GachaPromoCard from './GachaPromoCard.vue'

function mountCard() {
  return mount(GachaPromoCard, {
    props: { data: { pull_cost_single: 100, pull_cost_ten: 900, pity_ssr_at: 90 } },
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

describe('GachaPromoCard', () => {
  it('renders the kicker title and the NEW badge', () => {
    const w = mountCard()
    expect(w.text()).toContain('spotlight.gachaPromo.title')
    expect(w.text()).toContain('spotlight.gachaPromo.newBadge')
  })

  it('renders the headline and description', () => {
    const w = mountCard()
    expect(w.find('[data-testid="gacha-headline"]').text()).toBe(
      'spotlight.gachaPromo.headline',
    )
    expect(w.text()).toContain('spotlight.gachaPromo.description')
  })

  it('renders all four rarity chips', () => {
    const row = mountCard().find('[data-testid="rarity-row"]')
    expect(row.exists()).toBe(true)
    for (const rarity of ['N', 'R', 'SR', 'SSR']) {
      expect(row.text()).toContain(rarity)
    }
  })

  it('feeds the backend economy numbers into pity + costs copy', () => {
    const w = mountCard()
    expect(w.text()).toContain('spotlight.gachaPromo.pityHint::{"n":90}')
    expect(w.find('[data-testid="gacha-costs"]').text()).toBe(
      'spotlight.gachaPromo.costsLine::{"one":100,"ten":900}',
    )
  })

  it('CTA router-links to /gacha', () => {
    const cta = mountCard().findComponent(RouterLinkStub)
    expect(cta.props('to')).toBe('/gacha')
    expect(cta.text()).toContain('spotlight.gachaPromo.cta')
  })

  it('the decorative deck is aria-hidden', () => {
    const deck = mountCard().find('.promo-deck')
    expect(deck.exists()).toBe(true)
    expect(deck.attributes('aria-hidden')).toBe('true')
  })
})
