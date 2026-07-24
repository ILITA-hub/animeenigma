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

function mountViewer(cards: PulledCard[], active = true, mode?: 'reveal' | 'inspect', startIndex?: number) {
  return mount(CardViewer3D, {
    props: { active, cards, ...(mode ? { mode } : {}), ...(startIndex !== undefined ? { startIndex } : {}) },
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

describe('CardViewer3D — inspect mode', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('renders Prev / Next / Close nav — no Skip all button', async () => {
    const w = mountViewer([pulled('a', 'SR'), pulled('b', 'N'), pulled('c', 'SSR')], true, 'inspect')
    await flushPromises()
    expect(document.querySelector('[data-testid="viewer-inspect-prev"]')).not.toBeNull()
    expect(document.querySelector('[data-testid="viewer-inspect-next"]')).not.toBeNull()
    expect(document.querySelector('[data-testid="viewer-inspect-close"]')).not.toBeNull()
    expect(document.querySelector('[data-testid="viewer-skip-all"]')).toBeNull()
    w.unmount()
  })

  it('startIndex positions the counter at the correct card', async () => {
    const w = mountViewer([pulled('a', 'N'), pulled('b', 'SR'), pulled('c', 'SSR')], true, 'inspect', 1)
    await flushPromises()
    expect(document.querySelector('.vcount')?.textContent).toContain('2 / 3')
    w.unmount()
  })

  it('Next wraps around from last to first card', async () => {
    const w = mountViewer([pulled('a', 'N'), pulled('b', 'SR')], true, 'inspect', 1)
    await flushPromises()
    expect(document.querySelector('.vcount')?.textContent).toContain('2 / 2')
    const nextBtn = document.querySelector('[data-testid="viewer-inspect-next"]') as HTMLButtonElement
    nextBtn.click()
    await flushPromises()
    expect(document.querySelector('.vcount')?.textContent).toContain('1 / 2')
    w.unmount()
  })

  it('Prev wraps around from first to last card', async () => {
    const w = mountViewer([pulled('a', 'N'), pulled('b', 'SR'), pulled('c', 'R')], true, 'inspect', 0)
    await flushPromises()
    expect(document.querySelector('.vcount')?.textContent).toContain('1 / 3')
    const prevBtn = document.querySelector('[data-testid="viewer-inspect-prev"]') as HTMLButtonElement
    prevBtn.click()
    await flushPromises()
    expect(document.querySelector('.vcount')?.textContent).toContain('3 / 3')
    w.unmount()
  })

  it('Close emits done', async () => {
    const w = mountViewer([pulled('a', 'R'), pulled('b', 'SR')], true, 'inspect')
    await flushPromises()
    const closeBtn = document.querySelector('[data-testid="viewer-inspect-close"]') as HTMLButtonElement
    closeBtn.click()
    await flushPromises()
    expect(w.emitted('done')).toHaveLength(1)
    w.unmount()
  })

  it('shows ×N dupe badge (no NEW badge) in inspect mode', async () => {
    const w = mountViewer([pulled('a', 'SSR', true, 3)], true, 'inspect')
    await flushPromises()
    // In inspect mode, new=true is ignored — always show ×N
    expect(document.querySelector('.vtagDUP')?.textContent).toContain('×3')
    expect(document.querySelector('.vtagNEW')).toBeNull()
    w.unmount()
  })

  // ── Drag-release face snapping ─────────────────────────────────────────────

  /** Drag the card horizontally by dx pixels (0.42°/px), then release. */
  async function dragAndRelease(dx: number) {
    const card = document.querySelector('.card3d') as HTMLElement & {
      setPointerCapture: (id: number) => void
    }
    card.setPointerCapture = () => {} // jsdom lacks pointer capture
    // jsdom has no PointerEvent — MouseEvent + assigned pointerId is enough
    // for the handlers (they only read clientX/clientY/pointerId).
    const ev = (type: string, x: number) =>
      Object.assign(new MouseEvent(type, { clientX: x, clientY: 0 }), { pointerId: 1 })
    card.dispatchEvent(ev('pointerdown', 0))
    card.dispatchEvent(ev('pointermove', dx))
    card.dispatchEvent(ev('pointerup', dx))
    await flushPromises()
    return card
  }

  it('inspect: dragging past 90° rests on the BACK face on release', async () => {
    const w = mountViewer([pulled('a', 'SSR', true, 1, 'cards/back.jpg')], true, 'inspect')
    await flushPromises()
    // 430px * 0.42 ≈ 180.6° → nearest face (step 180) = 180 → back stays visible.
    const card = await dragAndRelease(430)
    expect(card.style.transform).toContain('rotateY(180deg)')
    w.unmount()
  })

  it('reveal: the ceremonial front snap is unchanged (multiple of 360)', async () => {
    const w = mountViewer([pulled('a', 'SSR')], true)
    await flushPromises()
    const card = await dragAndRelease(430)
    expect(card.style.transform).toContain('rotateY(360deg)')
    w.unmount()
  })
})
