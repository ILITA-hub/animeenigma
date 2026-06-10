import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import CardViewer3D from './CardViewer3D.vue'
import type { PulledCard, Rarity } from '@/api/gacha'

vi.mock('@/api/gacha', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/gacha')>()
  return { ...actual, cardImageUrl: (p: string) => (p ? `/api/gacha/images/${p}` : '') }
})

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

function pulled(id: string, rarity: Rarity, isNew = true, count = 1, backPath = ''): PulledCard {
  return {
    card: {
      id,
      name: `Card ${id}`,
      source_title: '',
      image_path: `cards/${id}.webp`,
      back_path: backPath,
      rarity,
      enabled: true,
      created_at: '',
      updated_at: '',
    },
    new: isNew,
    count,
  }
}

function mountViewer(cards: PulledCard[], active = true) {
  return mount(CardViewer3D, {
    props: { active, cards },
    global: {
      plugins: [i18n],
      stubs: {
        // Forward the parent's @click as a single component-level event.
        Button: {
          emits: ['click'],
          template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
          inheritAttrs: false,
        },
      },
    },
    attachTo: document.body,
  })
}

describe('CardViewer3D', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('does not render when inactive', () => {
    const w = mountViewer([pulled('a', 'N')], false)
    expect(document.querySelector('[data-testid="card-viewer-3d"]')).toBeNull()
    w.unmount()
  })

  it('renders the first card and a 1/N counter', async () => {
    const w = mountViewer([pulled('a', 'SSR'), pulled('b', 'N')])
    await flushPromises()
    expect(document.querySelector('.vcount')?.textContent).toContain('1 / 2')
    w.unmount()
  })

  it('next advances to the following card', async () => {
    const w = mountViewer([pulled('a', 'N'), pulled('b', 'SR')])
    await flushPromises()
    const nextBtn = document.querySelector('[data-testid="viewer-next"]') as HTMLButtonElement
    nextBtn.click()
    await flushPromises()
    expect(document.querySelector('.vcount')?.textContent).toContain('2 / 2')
    expect(w.emitted('done')).toBeUndefined()
    w.unmount()
  })

  it('next on the last card emits done', async () => {
    const w = mountViewer([pulled('a', 'N')])
    await flushPromises()
    const nextBtn = document.querySelector('[data-testid="viewer-next"]') as HTMLButtonElement
    nextBtn.click()
    await flushPromises()
    expect(w.emitted('done')).toHaveLength(1)
    w.unmount()
  })

  it('skip-all emits done immediately', async () => {
    const w = mountViewer([pulled('a', 'N'), pulled('b', 'SR'), pulled('c', 'SSR')])
    await flushPromises()
    const skip = document.querySelector('[data-testid="viewer-skip-all"]') as HTMLButtonElement
    skip.click()
    await flushPromises()
    expect(w.emitted('done')).toHaveLength(1)
    w.unmount()
  })

  it('shows a NEW badge for new cards', async () => {
    const w = mountViewer([pulled('a', 'SSR', true)])
    await flushPromises()
    expect(document.querySelector('.vtagNEW')).not.toBeNull()
    w.unmount()
  })

  it('shows a ×N badge for duplicate cards', async () => {
    const w = mountViewer([pulled('a', 'SSR', false, 3)])
    await flushPromises()
    expect(document.querySelector('.vtagDUP')?.textContent).toContain('×3')
    w.unmount()
  })

  it('renders the uploaded card back when back_path is set, not the branded emblem', async () => {
    const w = mountViewer([pulled('a', 'SSR', true, 1, 'cards/back.webp')])
    await flushPromises()
    expect(document.querySelector('.cardback-img')).not.toBeNull()
    expect(document.querySelector('.cardback .emblem')).toBeNull()
    w.unmount()
  })

  it('renders the branded default back when no back_path', async () => {
    const w = mountViewer([pulled('a', 'SSR')])
    await flushPromises()
    expect(document.querySelector('.cardback-img')).toBeNull()
    expect(document.querySelector('.cardback .emblem')).not.toBeNull()
    expect(document.querySelector('.cardback .wordmark')?.textContent).toContain('ANIMEENIGMA')
    w.unmount()
  })

  it('last card shows the "to results" button label', async () => {
    const w = mountViewer([pulled('a', 'N')])
    await flushPromises()
    expect(document.querySelector('[data-testid="viewer-next"]')?.textContent).toContain(
      en.gacha.viewer_to_results.replace(' ›', ''),
    )
    w.unmount()
  })
})
