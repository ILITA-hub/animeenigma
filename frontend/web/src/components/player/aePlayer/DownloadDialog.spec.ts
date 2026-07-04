import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('@/offline/downloadEngine', () => {
  const PROJECTED_BYTES: Record<string, number> = { '480': 250 * 2 ** 20, '720': 450 * 2 ** 20, '1080': 900 * 2 ** 20 }
  return {
    PROJECTED_BYTES,
    projectedBytesFor: (q: string, d?: number) =>
      Math.round(((PROJECTED_BYTES[q] ?? PROJECTED_BYTES['720']) * (d && d > 0 && d < 600 ? d : 24)) / 24),
    storageEstimate: vi.fn(async () => ({ usage: 0, quota: 10 * 2 ** 30 })),
  }
})
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

import DownloadDialog from './DownloadDialog.vue'

const globalStubs = {
  global: { mocks: { $t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k) } },
}

function mountDlg(props: Partial<{ seasonCount: number; sheet: boolean; durationMin: number }> = {}) {
  return mount(DownloadDialog, { props: { seasonCount: 10, ...props }, ...globalStubs })
}

describe('DownloadDialog (season-only)', () => {
  beforeEach(() => localStorage.clear())

  it('shows the season summary with count and estimate', () => {
    const w = mountDlg()
    expect(w.find('[data-test="season-summary"]').text()).toContain('scopeSeason:{"n":10}')
    expect(w.find('[data-test="season-estimate"]').exists()).toBe(true)
  })

  it('emits confirm with the picked quality only', async () => {
    const w = mountDlg()
    await w.find('[data-test="dl-start"]').trigger('click')
    const evt = w.emitted('confirm')![0]
    expect(evt).toEqual(['720'])
  })

  it('disables start and says done when nothing is left to download', () => {
    const w = mountDlg({ seasonCount: 0 })
    expect(w.find('[data-test="dl-start"]').attributes('disabled')).toBeDefined()
    expect(w.find('[data-test="season-summary"]').text()).toContain('seasonDone')
  })

  it('warns when the projection exceeds free space', async () => {
    const { storageEstimate } = await import('@/offline/downloadEngine')
    ;(storageEstimate as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ usage: 0, quota: 1 * 2 ** 30 })
    const w = mountDlg({ seasonCount: 20 })
    await new Promise((r) => setTimeout(r))
    await w.vm.$nextTick()
    expect(w.find('[data-test="low-space"]').exists()).toBe(true)
  })

  it('applies sheet presentation class', () => {
    const w = mountDlg({ sheet: true })
    expect(w.find('.dl-dialog').classes()).toContain('dl-dialog--sheet')
  })
})

describe('DownloadDialog — duration-scaled estimates', () => {
  it('scales the season total by episode duration (10 × 12-min at 720p ≈ 2.2 GB)', () => {
    const w = mountDlg({ durationMin: 12, seasonCount: 10 })
    expect(w.find('[data-test="season-estimate"]').text()).toContain('2.2 GB')
  })

  it('keeps the 24-min baseline without a duration (10 × 450 MB ≈ 4.4 GB)', () => {
    const w = mountDlg({ seasonCount: 10 })
    expect(w.find('[data-test="season-estimate"]').text()).toContain('4.4 GB')
  })
})
