import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import type { CapabilityReport } from '@/types/capabilities'
import type { Combo, AudioKind } from '@/types/aePlayer'

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

function mountDlg(props: Partial<{
  episodeNumber: number
  seasonCount: number
  sheet: boolean
  initialScope: 'episode' | 'season'
  durationMin: number
  report: CapabilityReport | null
  initialCombo: Combo | null
  loadTeams: (provider: string, audio: AudioKind) => Promise<string[]>
}> = {}) {
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

describe('DownloadDialog — duration-scaled estimates', () => {
  it('halves the estimate for a 12-min episode (720p: 225 MB, not 450)', () => {
    const w = mountDlg({ durationMin: 12 })
    expect(w.find('[data-test="episode-estimate"]').text()).toContain('225 MB')
  })

  it('keeps the 24-min baseline without a duration', () => {
    const w = mountDlg()
    expect(w.find('[data-test="episode-estimate"]').text()).toContain('450 MB')
  })

  it('scales the season total too (10 × 12-min at 720p ≈ 2.2 GB)', () => {
    const w = mountDlg({ durationMin: 12, seasonCount: 10 })
    expect(w.find('[data-test="scope-season"]').text()).toContain('2.2 GB')
  })
})

const REPORT = {
  anime_id: 'a1',
  families: [{ providers: [
    { provider: 'gogoanime', display_name: 'Gogoanime', group: 'en', state: 'active', selectable: true, hacker_only: false, order: 90, audios: ['sub', 'dub'] },
    { provider: 'animepahe', display_name: 'AnimePahe', group: 'en', state: 'active', selectable: true, hacker_only: false, order: 70, audios: ['sub', 'dub'] },
    { provider: 'kodik', display_name: 'Kodik', group: 'ru', state: 'active', selectable: true, hacker_only: false, order: 80, audios: ['dub', 'sub'] },
  ] }],
} as unknown as CapabilityReport
// NOTE: a group-'en' provider is NOT in the dub/ru row list (GROUP_LANGS) —
// provider switching is tested between the two 'en' providers, and the lang
// switch tests the re-default to kodik.
const COMBO: Combo = { audio: 'dub', lang: 'en', provider: 'gogoanime', server: '', team: null }

describe('source picker', () => {
  it('hidden without report/initialCombo; emits null combo', async () => {
    const w = mountDlg()
    expect(w.find('[data-test="dl-provider"]').exists()).toBe(false)
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')![0][2]).toBeNull()
  })

  it('renders providers for the initial audio/lang and emits the edited combo', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO })
    const sel = w.find('[data-test="dl-provider"]')
    expect(sel.findAll('option').map((o) => o.attributes('value'))).toEqual(['gogoanime', 'animepahe'])
    await sel.setValue('animepahe')
    await w.find('[data-test="dl-start"]').trigger('click')
    const combo = w.emitted('confirm')![0][2] as Combo
    expect(combo.provider).toBe('animepahe')
    expect(combo.team).toBeNull()
  })

  it('lang switch re-defaults a filtered-out provider', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO })
    await w.find('[data-test="dl-lang-ru"]').trigger('click')
    await w.find('[data-test="dl-start"]').trigger('click')
    const combo = w.emitted('confirm')![0][2] as Combo
    expect(combo.provider).toBe('kodik') // gogoanime has no RU dub rows
    expect(combo.lang).toBe('ru')
  })

  it('RAW switch drops lang filter and derives lang from provider group', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO })
    await w.find('[data-test="dl-audio-sub"]').trigger('click')
    await w.find('[data-test="dl-start"]').trigger('click')
    const combo = w.emitted('confirm')![0][2] as Combo
    expect(combo.audio).toBe('sub')
    expect(['en', 'ru']).toContain(combo.lang) // GROUP_PRIMARY_LANG of the picked row's group
  })

  it('team select appears when loadTeams returns teams; Авто = null', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO, loadTeams: async () => ['AniLibria', 'Dream Cast'] })
    await vi.waitFor(() => expect(w.find('[data-test="dl-team"]').exists()).toBe(true))
    await w.find('[data-test="dl-team"]').setValue('AniLibria')
    await w.find('[data-test="dl-start"]').trigger('click')
    expect((w.emitted('confirm')![0][2] as Combo).team).toBe('AniLibria')
  })
})
