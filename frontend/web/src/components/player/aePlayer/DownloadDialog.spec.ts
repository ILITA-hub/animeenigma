import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import type { CapabilityReport } from '@/types/capabilities'
import type { Combo, AudioKind } from '@/types/aePlayer'
import * as network from '@/offline/network'
import type { SubOption } from '@/offline/types'

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
  seasonCount: number
  sheet: boolean
  durationMin: number
  report: CapabilityReport | null
  initialCombo: Combo | null
  loadTeams: (provider: string, audio: AudioKind) => Promise<string[]>
  subOptions: SubOption[]
}> = {}) {
  return mount(DownloadDialog, { props: { seasonCount: 10, ...props }, ...globalStubs })
}

describe('DownloadDialog (season-only)', () => {
  beforeEach(() => localStorage.clear())

  it('shows the season summary with count and estimate', () => {
    const w = mountDlg()
    expect(w.find('[data-test="season-summary"]').text()).toContain('scopeSeason:{"n":10}')
    expect(w.find('[data-test="season-estimate"]').exists()).toBe(true)
  })

  it('emits confirm with the picked quality (default combo/subPref null)', async () => {
    const w = mountDlg()
    await w.find('[data-test="dl-start"]').trigger('click')
    const evt = w.emitted('confirm')![0]
    expect(evt).toEqual(['720', null, null])
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

const REPORT = {
  anime_id: 'a1',
  families: [{ providers: [
    { provider: 'gogoanime', display_name: 'Gogoanime', group: 'en', state: 'active', selectable: true, hacker_only: false, order: 90, audios: ['sub', 'dub'] },
    { provider: 'animepahe', display_name: 'AnimePahe', group: 'en', state: 'active', selectable: true, hacker_only: false, order: 70, audios: ['sub', 'dub'] },
    { provider: 'kodik', display_name: 'Kodik', group: 'ru', state: 'active', selectable: true, hacker_only: false, order: 80, audios: ['dub', 'sub'] },
  ] }],
} as unknown as CapabilityReport
// NOTE: none of these providers is `firstparty`, and DownloadDialog.vue calls
// rowsFromReport(props.report, filter) with NO verify report (it's not wired
// to the content-verify feed — that's out of this dialog's scope). Per the
// owner-approved hard gate (verifiedCaps.ts), an unverified non-firstparty cap
// is DUB-blind: effectiveAudios collapses to ['sub'] regardless of the cap's
// claimed audios, so the DUB facet is permanently empty here and RAW (sub)
// lists all three providers regardless of group (the language slider is
// hidden under RAW, so group/lang no longer partitions the row list).
const COMBO: Combo = { audio: 'dub', lang: 'en', provider: 'gogoanime', server: '', team: null }
const RAW_COMBO: Combo = { audio: 'sub', lang: 'en', provider: 'gogoanime', server: '', team: null }

describe('source picker', () => {
  it('hidden without report/initialCombo; emits null combo', async () => {
    const w = mountDlg()
    expect(w.find('[data-test="dl-provider"]').exists()).toBe(false)
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')![0][1]).toBeNull()
  })

  it('DUB is fully gated without a content-verify report (unverified → RAW-only)', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO })
    expect(w.find('[data-test="dl-audio-dub"]').attributes('disabled')).toBeDefined()
    const sel = w.find('[data-test="dl-provider"]')
    expect(sel.findAll('option').map((o) => o.attributes('value'))).toEqual([])
  })

  it('RAW lists every unverified provider regardless of group; picking one emits the edited combo', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: RAW_COMBO })
    const sel = w.find('[data-test="dl-provider"]')
    expect(sel.findAll('option').map((o) => o.attributes('value'))).toEqual(['gogoanime', 'kodik', 'animepahe'])
    await sel.setValue('kodik')
    await w.find('[data-test="dl-start"]').trigger('click')
    const combo = w.emitted('confirm')![0][1] as Combo
    expect(combo.provider).toBe('kodik')
    expect(combo.team).toBeNull()
  })

  it('RAW switch drops lang filter and derives lang from provider group', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO })
    await w.find('[data-test="dl-audio-sub"]').trigger('click')
    await w.find('[data-test="dl-start"]').trigger('click')
    const combo = w.emitted('confirm')![0][1] as Combo
    expect(combo.audio).toBe('sub')
    expect(['en', 'ru']).toContain(combo.lang) // GROUP_PRIMARY_LANG of the picked row's group
  })

  it('team select appears when loadTeams returns teams; Авто = null', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO, loadTeams: async () => ['AniLibria', 'Dream Cast'] })
    await vi.waitFor(() => expect(w.find('[data-test="dl-team"]').exists()).toBe(true))
    await w.find('[data-test="dl-team"]').setValue('AniLibria')
    await w.find('[data-test="dl-start"]').trigger('click')
    expect((w.emitted('confirm')![0][1] as Combo).team).toBe('AniLibria')
  })
})

const SUBS: SubOption[] = [
  { key: 'b:auto', label: 'Bundled', pref: { kind: 'bundled', lang: 'auto' } },
  { key: 'e:jimaku:ja', label: 'Jimaku · JA', pref: { kind: 'external', provider: 'jimaku', lang: 'ja', label: 'Jimaku A' } },
]

describe('subtitle picker', () => {
  it('hidden with no options; defaults to off (null pref)', async () => {
    const w = mountDlg()
    expect(w.find('[data-test="dl-subs"]').exists()).toBe(false)
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')![0][2]).toBeNull()
  })
  it('emits the picked pref', async () => {
    const w = mountDlg({ subOptions: SUBS })
    await w.find('[data-test="dl-subs"]').setValue('e:jimaku:ja')
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')![0][2]).toEqual({ kind: 'external', provider: 'jimaku', lang: 'ja', label: 'Jimaku A' })
  })
})

describe('mobile-data confirm step', () => {
  beforeEach(() => network._resetNetworkForTests())
  it('on cellular the first confirm arms the warning; the explicit button confirms + sets the override', async () => {
    vi.spyOn(network, 'isCellular').mockReturnValue(true)
    const w = mountDlg()
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')).toBeUndefined()
    expect(w.find('[data-test="cellular-warn"]').exists()).toBe(true)
    await w.find('[data-test="dl-cellular-confirm"]').trigger('click')
    expect(w.emitted('confirm')).toHaveLength(1)
    expect(network.allowCellularThisSession()).toBe(true)
    vi.restoreAllMocks()
  })
  it('not on cellular: single-step confirm as before', async () => {
    const w = mountDlg()
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')).toHaveLength(1)
  })
})
