import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('@/offline/downloadEngine', () => ({
  PROJECTED_BYTES: { '480': 250 * 2 ** 20, '720': 450 * 2 ** 20, '1080': 900 * 2 ** 20 },
  storageEstimate: vi.fn(async () => ({ usage: 0, quota: 10 * 2 ** 30 })),
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

import DownloadDialog from './DownloadDialog.vue'

const globalStubs = {
  global: { mocks: { $t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k) } },
}

function mountDlg(props: Partial<{ episodeNumber: number; seasonCount: number; sheet: boolean; initialScope: 'episode' | 'season' }> = {}) {
  return mount(DownloadDialog, { props: { episodeNumber: 4, seasonCount: 10, ...props }, ...globalStubs })
}

describe('DownloadDialog v2', () => {
  beforeEach(() => localStorage.clear())

  it('renders both scopes and defaults to episode', () => {
    const w = mountDlg()
    expect(w.find('[data-test="scope-episode"]').attributes('aria-checked')).toBe('true')
    expect(w.find('[data-test="scope-season"]').attributes('aria-checked')).toBe('false')
  })

  it('emits confirm with quality AND scope', async () => {
    const w = mountDlg()
    await w.find('[data-test="scope-season"]').trigger('click')
    await w.find('[data-test="dl-start"]').trigger('click')
    const evt = w.emitted('confirm')![0]
    expect(evt[0]).toBe('720')
    expect(evt[1]).toBe('season')
  })

  it('disables season scope when nothing left to download', () => {
    const w = mountDlg({ seasonCount: 0 })
    expect(w.find('[data-test="scope-season"]').attributes('disabled')).toBeDefined()
  })

  it('preselects season via initialScope', () => {
    const w = mountDlg({ initialScope: 'season' })
    expect(w.find('[data-test="scope-season"]').attributes('aria-checked')).toBe('true')
  })

  it('warns when the projection exceeds free space', async () => {
    const { storageEstimate } = await import('@/offline/downloadEngine')
    ;(storageEstimate as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ usage: 0, quota: 1 * 2 ** 30 })
    const w = mountDlg({ seasonCount: 20, initialScope: 'season' })
    await new Promise((r) => setTimeout(r))
    await w.vm.$nextTick()
    expect(w.find('[data-test="low-space"]').exists()).toBe(true)
  })

  it('applies sheet presentation class', () => {
    const w = mountDlg({ sheet: true })
    expect(w.find('.dl-dialog').classes()).toContain('dl-dialog--sheet')
  })
})
