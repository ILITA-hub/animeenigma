import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import DropsModal from './DropsModal.vue'
import type { BannerCardView, Rarity } from '@/api/gacha'

vi.mock('@/api/gacha', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/gacha')>()
  return { ...actual, cardImageUrl: (p: string) => `/api/gacha/images/${p}` }
})

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

function card(id: string, rarity: Rarity, owned: boolean): BannerCardView {
  return { id, name: `Card ${id}`, rarity, image_path: `cards/${id}.webp`, back_path: '', owned }
}

function mountModal(cards: BannerCardView[]) {
  return mount(DropsModal, {
    props: { modelValue: true, bannerName: 'Test Banner', cards },
    global: {
      plugins: [i18n],
      stubs: {
        Modal: { props: ['modelValue', 'title'], template: '<div><slot /><slot name="footer" /></div>' },
        Button: { template: '<button><slot /></button>' },
      },
    },
  })
}

describe('DropsModal', () => {
  const pool: BannerCardView[] = [
    card('ssr1', 'SSR', true),
    card('sr1', 'SR', false),
    card('r1', 'R', true),
    card('n1', 'N', false),
    card('n2', 'N', false),
  ]

  it('groups cards by rarity SSR→N with a header per tier', () => {
    const w = mountModal(pool)
    const heads = w.findAll('.tier-name').map((e) => e.text())
    expect(heads).toEqual(['SSR', 'SR', 'R', 'N'])
  })

  it('renders unowned cards (visible, dimmed) — not hidden', () => {
    const w = mountModal(pool)
    expect(w.findAll('[data-testid="drops-card-unowned"]').length).toBe(3) // sr1, n1, n2
    expect(w.findAll('[data-testid="drops-card-owned"]').length).toBe(2)
  })

  it('applies the .unowned class (dashed/dimmed) to unowned cards', () => {
    const w = mountModal(pool)
    const unowned = w.find('[data-testid="drops-card-unowned"]')
    expect(unowned.classes()).toContain('unowned')
  })

  it('SSR header shows the 1% guarantee-at-90 label', () => {
    const w = mountModal(pool)
    const ssrRate = w.findAll('.tier-rate')[0].text()
    expect(ssrRate).toContain('1%')
    expect(ssrRate).toContain(en.gacha.drops_rate_ssr_extra)
  })

  it('SR header shows the at-least-1-in-x10 label', () => {
    const w = mountModal(pool)
    const srRate = w.findAll('.tier-rate')[1].text()
    expect(srRate).toContain('8%')
    expect(srRate).toContain(en.gacha.drops_rate_sr_extra)
  })

  it('footer stat reports owned / total', () => {
    const w = mountModal(pool)
    expect(w.html()).toContain('pulled 2 of 5')
  })

  it('skips tiers with no cards', () => {
    const w = mountModal([card('r1', 'R', true)])
    const heads = w.findAll('.tier-name').map((e) => e.text())
    expect(heads).toEqual(['R'])
  })
})
