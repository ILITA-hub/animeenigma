import 'fake-indexeddb/auto'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import DownloadsPage from '@/views/DownloadsPage.vue'
import { putDownload, _resetDbForTests } from '@/offline/registry'
import { _installCachesForTests } from '@/offline/downloadEngine'
import type { OfflineDownload } from '@/offline/types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => p ? `${k}:${JSON.stringify(p)}` : k }) }))
// __esModule: true is required — without it, Vue's defineAsyncComponent
// ESM-interop check (`comp.__esModule || comp[Symbol.toStringTag] ===
// 'Module'`) fails on a plain vi.mock factory object, so it never unwraps
// `.default` and instead treats the whole { default: {...} } module as the
// component, which explodes downstream (@vue/test-utils' isTeleport() reads
// an unlisted property off the raw mock and Vitest's mock-module proxy
// throws "no export defined").
vi.mock('@/components/player/aePlayer/AePlayer.vue', () => ({
  __esModule: true,
  default: { name: 'AePlayer', template: '<div data-testid="offline-player" />' },
}))
// The store imports useProviderResolver for resume() — that composable pulls
// in @/api/client → @/router → @/i18n (real createI18n call). None of these
// tests exercise resume(), so stub the composable to keep the module graph
// clear of the real i18n singleton (which vue-i18n's mock above can't satisfy).
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({
    listEpisodes: vi.fn(), resolveStream: vi.fn(), listTeams: vi.fn(),
  }),
}))
// The ui barrel (@/components/ui) is a single flat module — importing any
// named export from it evaluates the WHOLE file, including
// SearchAutocomplete.vue, which imports @/api/client → @/router → @/i18n at
// module scope. Same real-i18n hazard as above; stub the client (unused by
// this page) to keep the import graph clear of it.
vi.mock('@/api/client', () => ({}))

// jsdom (this project's test environment) does not implement the CSS global
// at all — real browsers do. Minimal CSS.escape polyfill so the id-with-
// colons selectors below resolve the way they would in production.
if (typeof globalThis.CSS === 'undefined') {
  (globalThis as unknown as { CSS: { escape(s: string): string } }).CSS = {
    escape: (s: string) => s.replace(/[^a-zA-Z0-9_-]/g, (c) => `\\${c}`),
  }
}

const doneDl: OfflineDownload = {
  id: 'a1:1:gogoanime:sub:en::720', animeId: 'a1', animeTitle: 'Frieren', quality: '720',
  episode: { key: 1, label: 1, number: 1 }, streamType: 'hls', state: 'done',
  combo: { audio: 'sub', lang: 'en', provider: 'gogoanime', server: 's', team: null },
  bytes: 1000, resourcesDone: 2, resourcesTotal: 2, createdAt: 5,
  playlistLocalPath: '/__offline/x/master.m3u8', subtitles: [],
}

beforeEach(async () => {
  setActivePinia(createPinia())
  await _resetDbForTests()
  _installCachesForTests({
    async has() { return true }, async open() { return {} as Cache },
    async delete() { return true }, async keys() { return [] }, async match() { return undefined },
  } as unknown as CacheStorage)
})

describe('DownloadsPage', () => {
  it('shows empty state without downloads', async () => {
    const w = mount(DownloadsPage)
    await vi.waitFor(() => expect(w.text()).toContain('downloads.empty'))
  })
  it('lists a done download grouped by anime and opens offline playback', async () => {
    await putDownload(doneDl)
    const w = mount(DownloadsPage)
    await vi.waitFor(() => expect(w.text()).toContain('Frieren'))
    await w.find('[data-testid="watch-a1"]').trigger('click')
    // AePlayer is a defineAsyncComponent — even the mocked module resolves a
    // microtask later; assert with retry, not synchronously
    await vi.waitFor(() => expect(w.find('[data-testid="offline-player"]').exists()).toBe(true))
  })
  it('deletes after confirm', async () => {
    await putDownload(doneDl)
    const w = mount(DownloadsPage)
    await vi.waitFor(() => expect(w.text()).toContain('Frieren'))
    await w.find(`[data-testid="del-${CSS.escape(doneDl.id)}"]`).trigger('click')   // arm
    await w.find(`[data-testid="del-${CSS.escape(doneDl.id)}"]`).trigger('click')   // confirm
    await vi.waitFor(() => expect(w.text()).not.toContain('Frieren'))
  })
})
