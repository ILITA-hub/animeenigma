import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import GachaSlider from './GachaSlider.vue'
import type { BannerView } from '@/api/gacha'

vi.mock('@/api/gacha', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/gacha')>()
  return { ...actual, cardImageUrl: (p: string) => (p ? `/api/gacha/images/${p}` : '') }
})

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

function makeBanner(over: Partial<BannerView> = {}): BannerView {
  return {
    id: 'b1',
    name: 'Banner One',
    description: 'desc',
    backdrop_path: '',
    is_standard: false,
    cards: [],
    my_pity: 0,
    pity_threshold: 90,
    ...over,
  }
}

function mountSlider(banners: BannerView[], modelValue = 0) {
  return mount(GachaSlider, {
    props: { banners, modelValue },
    global: {
      plugins: [i18n],
      stubs: { ChevronLeft: { template: '<span/>' }, ChevronRight: { template: '<span/>' } },
    },
  })
}

describe('GachaSlider', () => {
  it('renders one slide per banner', () => {
    const w = mountSlider([makeBanner({ id: 'a', name: 'A' }), makeBanner({ id: 'b', name: 'B' })])
    expect(w.findAll('.slide')).toHaveLength(2)
  })

  it('marks the active slide with .on', () => {
    const w = mountSlider([makeBanner({ id: 'a' }), makeBanner({ id: 'b' })], 1)
    const slides = w.findAll('.slide')
    expect(slides[1].classes()).toContain('on')
    expect(slides[0].classes()).not.toContain('on')
  })

  it('uses backdrop_path as the slide background', () => {
    const w = mountSlider([makeBanner({ backdrop_path: 'banners/bd.webp' })])
    const art = w.find('.art')
    // cardImageUrl → /api/gacha/images/banners/bd.webp, then routed through
    // the image-proxy (relative gacha URL) and percent-encoded as the `url` param.
    expect(art.attributes('style')).toContain(encodeURIComponent('banners/bd.webp'))
  })

  it('shows the gradient fallback when no backdrop is set', () => {
    const w = mountSlider([makeBanner({ backdrop_path: '' })])
    expect(w.find('.art-fallback').exists()).toBe(true)
  })

  it('clicking next arrow emits the next index', async () => {
    const w = mountSlider([makeBanner({ id: 'a' }), makeBanner({ id: 'b' })], 0)
    await w.find('.arrow.r').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([1])
  })

  it('next wraps around from the last banner to the first', async () => {
    const w = mountSlider([makeBanner({ id: 'a' }), makeBanner({ id: 'b' })], 1)
    await w.find('.arrow.r').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([0])
  })

  it('hides arrows + dots for a single banner', () => {
    const w = mountSlider([makeBanner()])
    expect(w.find('.arrow').exists()).toBe(false)
    expect(w.find('.dots').exists()).toBe(false)
  })

  it('clicking a dot selects that banner', async () => {
    const w = mountSlider([makeBanner({ id: 'a' }), makeBanner({ id: 'b' }), makeBanner({ id: 'c' })], 0)
    const dots = w.findAll('.dot')
    await dots[2].trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual([2])
  })
})
