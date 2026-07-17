import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick, ref } from 'vue'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'

// SeasonDownloadHost is a global singleton (mounted once in App.vue, outside
// AePlayer's tree) driving the card-launched season download flow. It can't
// receive AePlayer's live contentVerify.report, so it runs its own
// fetch-while-open poll (Task 15 fix round 3) — this spec proves that poll
// actually gates DownloadDialog's DUB facet the same way the in-player and
// single-episode dialogs do. Mocking mirrors offline/seasonDownloadFlow.spec.ts
// (the real module-level flow is exercised, not mocked) plus contentVerifyApi.
const h = vi.hoisted(() => ({
  capGet: vi.fn(async (_id: string) => ({ data: { success: true, data: null as unknown } })),
  animeGet: vi.fn(async (_id: string) => ({ data: { data: { episode_duration: 12 } } })),
  listEpisodes: vi.fn(async (_p: string, _a: string) => [] as unknown[]),
  listDownloads: vi.fn(async () => [] as unknown[]),
  subsAll: vi.fn(async (_id: string, _ep: number) => ({ data: { data: { languages: {}, episode: 1 } } })),
  verifyGet: vi.fn(async (_id: string) => ({ data: { data: { anime_id: 'a1', providers: [] as unknown[] } } })),
}))

vi.mock('@/offline/flag', () => ({
  offlineDownloadsEnabled: true,
  offlineRuntimeReady: () => true,
}))
vi.mock('@/api/client', () => ({
  capabilitiesApi: { get: (id: string) => h.capGet(id) },
  animeApi: { getById: (id: string) => h.animeGet(id) },
  subtitlesApi: { all: (id: string, ep: number) => h.subsAll(id, ep) },
  contentVerifyApi: { get: (id: string) => h.verifyGet(id) },
}))
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({
    listEpisodes: (p: string, a: string) => h.listEpisodes(p, a),
    resolveStream: vi.fn(),
    listTeams: async () => [],
  }),
}))
vi.mock('@/offline/registry', () => ({
  listDownloads: () => h.listDownloads(),
  // downloadEngine (loaded via the real ./seasonDownload) also imports these:
  putDownload: vi.fn(),
  getDownload: vi.fn(),
  deleteDownloadRecord: vi.fn(),
}))
vi.mock('@/offline/seasonDownload', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/offline/seasonDownload')>()
  return { ...actual, enqueueSeason: vi.fn(async (targets: unknown[]) => targets.length) }
})
vi.mock('@/composables/aePlayer/useMobilePlayer', () => ({
  useMobilePlayer: () => ({ isMobile: ref(false), isCoarse: ref(false) }),
}))
vi.mock('@/composables/useToast', () => ({ useToast: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k }) }))

import SeasonDownloadHost from './SeasonDownloadHost.vue'
import { seasonFlow, openSeasonDownload, _resetSeasonFlowForTests } from '@/offline/seasonDownloadFlow'

function cap(provider: string, group: ProviderCap['group'], audios: string[], order = 10): ProviderCap {
  return {
    provider, display_name: provider, state: 'active', selectable: true,
    hacker_only: false, order, group, audios, reason: '',
  } as unknown as ProviderCap
}
function report(...caps: ProviderCap[]): CapabilityReport {
  return { anime_id: 'a1', families: [{ family: 'mixed', providers: caps }] } as unknown as CapabilityReport
}
const envelope = (rep: CapabilityReport) => ({ data: { success: true, data: rep } })
const ep = (n: number) => ({ key: `e${n}`, number: n, label: String(n) })
const REQ = { animeId: 'a1', title: 'T', poster: 'p.jpg' }

const mountOpts = { global: { stubs: { teleport: true }, mocks: { $t: (k: string) => k } } }

beforeEach(() => {
  _resetSeasonFlowForTests()
  h.capGet.mockReset()
  h.listEpisodes.mockReset()
  h.listDownloads.mockReset().mockResolvedValue([])
  h.subsAll.mockReset().mockResolvedValue({ data: { data: { languages: {}, episode: 1 } } })
  h.verifyGet.mockReset()
})

describe('SeasonDownloadHost — content-verify gating in the card-launched season flow', () => {
  it('stays RAW-only (DUB disabled) while content-verify has not resolved anything yet', async () => {
    h.capGet.mockResolvedValue(envelope(report(
      cap('gogoanime', 'en', ['sub', 'dub']),
      cap('animepahe', 'en', ['sub', 'dub']),
    )))
    h.listEpisodes.mockResolvedValue([ep(1), ep(2)])
    // Empty providers list — same "nothing verified" shape normalizeVerify tolerates.
    h.verifyGet.mockResolvedValue({ data: { data: { anime_id: 'a1', providers: [] } } })
    await openSeasonDownload(REQ, 'en')
    expect(seasonFlow.phase).toBe('choose')

    const w = mount(SeasonDownloadHost, mountOpts)
    await flushPromises()
    await nextTick()
    await flushPromises()

    expect(w.find('[data-test="dl-provider"]').exists()).toBe(true) // dialog is open
    expect(w.find('[data-test="dl-audio-dub"]').attributes('disabled')).toBeDefined()
  })

  it('a verified content-verify report unlocks DUB and lists only the verified provider', async () => {
    h.capGet.mockResolvedValue(envelope(report(
      cap('gogoanime', 'en', ['sub', 'dub']),
      cap('animepahe', 'en', ['sub', 'dub']),
    )))
    h.listEpisodes.mockResolvedValue([ep(1), ep(2)])
    h.verifyGet.mockResolvedValue({
      data: {
        data: {
          anime_id: 'a1',
          providers: [
            { provider: 'gogoanime', summary: { status: 'verified', raw: true, dub_langs: ['en'], hardsub_langs: [] }, units: [] },
          ],
        },
      },
    })
    await openSeasonDownload(REQ, 'en')
    expect(seasonFlow.phase).toBe('choose')

    const w = mount(SeasonDownloadHost, mountOpts)
    await flushPromises()
    await nextTick()
    await flushPromises()

    const dubBtn = w.find('[data-test="dl-audio-dub"]')
    expect(dubBtn.attributes('disabled')).toBeUndefined()
    await dubBtn.trigger('click')

    const sel = w.find('[data-test="dl-provider"]')
    expect(sel.findAll('option').map((o) => o.attributes('value'))).toEqual(['gogoanime']) // animepahe unverified — stays out of DUB
  })
})
