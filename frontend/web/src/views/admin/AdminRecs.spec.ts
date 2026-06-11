import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import AdminRecs from './AdminRecs.vue'
import type { AdminRecRow } from '@/composables/useAdminRecs'

const RouterLinkStub = {
  name: 'RouterLink',
  props: ['to'],
  template: '<a :href="to" data-stub="router-link"><slot /></a>',
}

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRoute: () => ({ params: { user_id: 'u-1' } }) }
})

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})

function row(partial: Partial<AdminRecRow>): AdminRecRow {
  return {
    rank: 1,
    pre_s12_rank: 1,
    anime: { id: 'a-' + partial.rank },
    final: 0.5,
    breakdown: { s1: 0.1, s2: 0.2, s3: 0.3, s4: 0.4, s5: 0.5, s7: 0.6 },
    weights: { s1: 0.3, s2: 0.2, s3: 0.2, s4: 0.1, s5: 0.2, s7: -0.15 },
    top_contributor: 's1',
    ...partial,
  }
}

const mockRows = ref<AdminRecRow[]>([
  row({ rank: 1, pre_s12_rank: 1 }),
  // Promoted by S12: pre-rerank 15 → rank 2 (delta +13, rendered ↑13).
  row({ rank: 2, pre_s12_rank: 15, top_contributor: 's7' }),
  // Demoted by S12: pre-rerank 1 → rank 3 (delta −2, rendered ↓2).
  row({ rank: 3, pre_s12_rank: 1 }),
])

vi.mock('@/composables/useAdminRecs', () => ({
  useAdminRecs: () => ({
    rows: mockRows,
    filteredOut: ref([]),
    computedAt: ref('2026-06-11T00:00:00Z'),
    signalVersions: ref({}),
    isLoading: ref(false),
    isRecomputing: ref(false),
    error: ref(null),
    lastRecomputeLatencyMs: ref(null),
    refresh: vi.fn(),
    recompute: vi.fn(),
  }),
}))

function mountView() {
  return mount(AdminRecs, {
    global: {
      stubs: { RouterLink: RouterLinkStub },
      mocks: { $t: (k: string) => k },
    },
  })
}

describe('AdminRecs', () => {
  it('renders an S7 breakdown column (header + cell value)', () => {
    const w = mountView()
    const headers = w.findAll('th').map((h) => h.text())
    expect(headers).toContain('S7')
    expect(w.text()).toContain('0.600') // formatBd(breakdown.s7)
  })

  it('keeps all five original signal columns alongside S7', () => {
    const w = mountView()
    const headers = w.findAll('th').map((h) => h.text())
    for (const s of ['S1', 'S2', 'S3', 'S4', 'S5']) expect(headers).toContain(s)
  })

  it('shows an upward S12 delta when the rerank promoted a row', () => {
    const w = mountView()
    expect(w.text()).toContain('↑13')
  })

  it('shows a downward S12 delta when the rerank demoted a row', () => {
    const w = mountView()
    expect(w.text()).toContain('↓2')
  })

  it('hides the S12 delta when rank is unchanged', () => {
    const w = mountView()
    const firstRankCell = w.findAll('tbody td').at(0)!
    expect(firstRankCell.text()).not.toContain('↑')
    expect(firstRankCell.text()).not.toContain('↓')
  })

  it('renders the signal legend with all nine pipeline entries', () => {
    const w = mountView()
    expect(w.text()).toContain('admin.recs.signalLegendTitle')
    for (const id of ['s1', 's2', 's3', 's4', 's5', 's6', 's7', 's11', 's12']) {
      expect(w.text()).toContain(`admin.recs.${id}Title`)
      expect(w.text()).toContain(`admin.recs.${id}Desc`)
    }
  })

  it('surfaces the negative S7 weight from the response rows in the legend', () => {
    const w = mountView()
    expect(w.text()).toContain('×-0.15')
  })

  it('styles the s7 top-contributor badge as destructive', () => {
    const w = mountView()
    const badge = w.findAll('tbody span').find((s) => s.text() === 's7')
    expect(badge).toBeTruthy()
    expect(badge!.classes().join(' ')).toContain('text-destructive')
  })
})
