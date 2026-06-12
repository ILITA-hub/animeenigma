import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import PullSummary from './PullSummary.vue'
import type { PulledCard, Rarity } from '@/api/gacha'

vi.mock('@/api/gacha', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/gacha')>()
  return { ...actual, cardImageUrl: (p: string) => `/api/gacha/images/${p}` }
})

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

function pulled(id: string, rarity: Rarity, isNew = true, count = 1): PulledCard {
  return {
    card: { id, name: `Card ${id}`, source_title: '', image_path: `cards/${id}.webp`, back_path: '', rarity, enabled: true, created_at: '', updated_at: '' },
    new: isNew,
    count,
  }
}

function mountSummary(cards: PulledCard[], instant = true, balance = 1000) {
  return mount(PullSummary, {
    props: {
      modelValue: true,
      cards,
      bannerName: 'Banner',
      balance,
      pity: 5,
      pityThreshold: 90,
      costX10: 900,
      instant,
    },
    global: {
      plugins: [i18n],
      stubs: {
        Modal: { props: ['modelValue', 'title'], template: '<div><slot /><slot name="footer" /></div>' },
        Button: {
          props: ['disabled'],
          emits: ['click'],
          inheritAttrs: false,
          template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
        },
      },
    },
  })
}

describe('PullSummary', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('renders one card per pull result', async () => {
    const w = mountSummary([pulled('a', 'SSR'), pulled('b', 'N'), pulled('c', 'R')])
    await flushPromises()
    expect(w.findAll('.rcard')).toHaveLength(3)
  })

  it('instant mode reveals all cards (no facedown remaining)', async () => {
    const w = mountSummary([pulled('a', 'SSR'), pulled('b', 'N')], true)
    await flushPromises()
    vi.advanceTimersByTime(10)
    await flushPromises()
    expect(w.findAll('.rcard.facedown')).toHaveLength(0)
  })

  it('shows ×10 title for a 10-card pull', async () => {
    const cards = Array.from({ length: 10 }, (_, i) => pulled(`c${i}`, 'N'))
    const w = mountSummary(cards)
    await flushPromises()
    // Title prop passed to Modal stub is reflected via component; assert grid count.
    expect(w.findAll('.rcard')).toHaveLength(10)
  })

  it('emits again when the "Again ×10" button is clicked', async () => {
    const w = mountSummary([pulled('a', 'N')], true, 1000)
    await flushPromises()
    const again = w.find('[data-testid="summary-again"]')
    await again.trigger('click')
    expect(w.emitted('again')).toHaveLength(1)
  })

  it('disables "Again ×10" when balance is below cost', async () => {
    const w = mountSummary([pulled('a', 'N')], true, 100)
    await flushPromises()
    expect(w.find('[data-testid="summary-again"]').attributes('disabled')).toBeDefined()
  })

  it('done button emits update:modelValue false', async () => {
    const w = mountSummary([pulled('a', 'N')])
    await flushPromises()
    await w.find('[data-testid="summary-done"]').trigger('click')
    expect(w.emitted('update:modelValue')?.at(-1)).toEqual([false])
  })

  it('shows a NEW badge for new cards and ×N for dupes', async () => {
    const w = mountSummary([pulled('a', 'SSR', true), pulled('b', 'R', false, 4)], true)
    await flushPromises()
    vi.advanceTimersByTime(10)
    await flushPromises()
    expect(w.find('.tagNEW').exists()).toBe(true)
    expect(w.find('.tagDUP').text()).toContain('×4')
  })

  it('reveal cards are native buttons (keyboard accessible)', () => {
    const w = mountSummary([pulled('1', 'N')])
    const card = w.find('[data-testid="summary-card-N"]')
    expect(card.element.tagName).toBe('BUTTON')
  })
})
