import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

// useI18n stub — the component localizes its title/aria labels; tests only need
// `t` to echo the key so the assertions (count + stepper behavior) stay pure.
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

// SUT imported AFTER vi.mock.
import RewatchCounter from '../RewatchCounter.vue'

// Group 6 — shared low-visual-weight rewatch counter (design 2026-06-05,
// hidden-at-0 tightened 2026-07-05). Used on My List rows (the anime-page
// hero usage moved into the status dropdown). Pure prop-driven: `count` in,
// `update:count` out. Read-only by default; editable shows a subtle − N +
// stepper. Hidden entirely when count=0 — editable or not — so it adds no
// clutter to never-rewatched anime.

const TID = '[data-testid="rewatch-counter"]'
const INC = '[data-testid="rewatch-inc"]'
const DEC = '[data-testid="rewatch-dec"]'

describe('RewatchCounter — read-only display', () => {
  it('renders nothing when count=0 and not editable (no clutter)', () => {
    const w = mount(RewatchCounter, { props: { count: 0, editable: false } })
    expect(w.find(TID).exists()).toBe(false)
  })

  it('shows the count when count>0', () => {
    const w = mount(RewatchCounter, { props: { count: 3, editable: false } })
    expect(w.find(TID).exists()).toBe(true)
    expect(w.text()).toContain('3')
  })

  it('shows no stepper buttons in read-only mode', () => {
    const w = mount(RewatchCounter, { props: { count: 3, editable: false } })
    expect(w.find(INC).exists()).toBe(false)
    expect(w.find(DEC).exists()).toBe(false)
  })
})

describe('RewatchCounter — editable stepper', () => {
  it('renders nothing at count=0 even when editable (no unlabeled ↻ − 0 +)', () => {
    const w = mount(RewatchCounter, { props: { count: 0, editable: true } })
    expect(w.find(TID).exists()).toBe(false)
  })

  it('shows the stepper once count>0 and editable', () => {
    const w = mount(RewatchCounter, { props: { count: 1, editable: true } })
    expect(w.find(TID).exists()).toBe(true)
    expect(w.find(INC).exists()).toBe(true)
    expect(w.find(DEC).exists()).toBe(true)
  })

  it('increment emits update:count with count+1', async () => {
    const w = mount(RewatchCounter, { props: { count: 2, editable: true } })
    await w.find(INC).trigger('click')
    expect(w.emitted('update:count')?.[0]).toEqual([3])
  })

  it('decrement emits update:count with count-1', async () => {
    const w = mount(RewatchCounter, { props: { count: 2, editable: true } })
    await w.find(DEC).trigger('click')
    expect(w.emitted('update:count')?.[0]).toEqual([1])
  })

})

describe('RewatchCounter — surface-agnostic (same props → same render)', () => {
  it('is pure: identical props produce identical output on both surfaces', () => {
    const a = mount(RewatchCounter, { props: { count: 4, editable: false } })
    const b = mount(RewatchCounter, { props: { count: 4, editable: false } })
    expect(a.html()).toBe(b.html())
  })
})
