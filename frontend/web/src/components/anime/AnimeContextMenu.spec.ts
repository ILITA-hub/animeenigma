import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'

// ── Hoisted state holders ────────────────────────────────────────────────
// vitest hoists vi.mock factories above every module-scope statement, so the
// mutable knobs the tests flip (auth, watchlist spies) must live in a holder
// the factories close over.
const sharedState = vi.hoisted(() => ({
  isAuthenticated: true,
  setStatusOptimistic: vi.fn(async () => {}),
  removeEntryOptimistic: vi.fn(async () => {}),
  invalidate: vi.fn(),
  fetchWatchlist: vi.fn(async () => {}),
  updateWatchlistEntry: vi.fn(async (_arg?: unknown) => {}),
  pushToast: vi.fn(),
  openSeasonDownload: vi.fn(async (_req?: unknown, _lang?: unknown) => {}),
  standalone: true,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return sharedState.isAuthenticated
    },
  }),
}))

vi.mock('@/stores/watchlist', () => ({
  useWatchlistStore: () => ({
    setStatusOptimistic: sharedState.setStatusOptimistic,
    removeEntryOptimistic: sharedState.removeEntryOptimistic,
    invalidate: sharedState.invalidate,
    fetchWatchlist: sharedState.fetchWatchlist,
  }),
}))

vi.mock('@/api/client', () => ({
  userApi: {
    updateWatchlistEntry: (arg: unknown) => sharedState.updateWatchlistEntry(arg),
  },
}))

vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: sharedState.pushToast, dismiss: vi.fn(), toasts: { value: [] } }),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}::${JSON.stringify(params)}` : key,
      locale: ref('en'),
    }),
  }
})

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string) => name ?? '',
}))

// Mocked wholesale so the real flow's transitive imports (api client,
// provider resolver, download engine) never load in this component test.
vi.mock('@/offline/seasonDownloadFlow', () => ({
  openSeasonDownload: (req: unknown, lang: unknown) => sharedState.openSeasonDownload(req, lang),
}))

vi.mock('@/composables/useImageProxy', () => ({
  getImageFallbackUrl: (url: string) => url,
  cardPosterUrl: (url: string) => url,
}))

// Standalone detection is a matchMedia singleton — replace it with a knob so
// tests cover both the installed-app and plain-browser-tab paths.
vi.mock('@/pwa/standalone', async () => {
  const { computed } = await import('vue')
  return {
    useStandaloneDisplay: () => computed(() => sharedState.standalone),
    installHintKey: () => 'downloads.installHint',
  }
})

// SUT — imported AFTER all vi.mock calls.
import AnimeContextMenu from './AnimeContextMenu.vue'

// Stub DropdownMenu so its default slot renders INLINE (no body portal), letting
// the tests query the action items in the wrapper rather than portaled DOM.
const DropdownMenuStub = {
  name: 'DropdownMenu',
  props: ['open', 'reference', 'anchorPoint', 'align', 'side', 'sideOffset'],
  emits: ['update:open'],
  template: '<div data-testid="dd-stub"><slot /></div>',
}

// DropdownMenuItem is re-exported from reka-ui in the barrel — stub it as a
// plain button so @select-style activation maps to a native click.
const DropdownMenuItemStub = {
  name: 'DropdownMenuItem',
  emits: ['select'],
  template: '<button class="dd-item" @click="$emit(\'select\', $event)"><slot /></button>',
}

const baseAnime = {
  id: 'anime-1',
  title: 'Test Anime',
  coverImage: 'https://example.com/p.jpg',
  releaseYear: 2024,
  episodes: 12,
}

const mounted: VueWrapper[] = []
function mountMenu(props: Record<string, unknown>): VueWrapper {
  const w = mount(AnimeContextMenu, {
    props: { visible: true, x: 0, y: 0, anime: baseAnime, listStatus: null, ...props },
    global: {
      mocks: {
        $t: (key: string, params?: Record<string, unknown>) =>
          params ? `${key}::${JSON.stringify(params)}` : key,
      },
      stubs: {
        DropdownMenu: DropdownMenuStub,
        DropdownMenuItem: DropdownMenuItemStub,
      },
    },
  }) as unknown as VueWrapper
  mounted.push(w)
  return w
}

function actions(w: VueWrapper): { key: string; kind: string }[] {
  return (w.vm as unknown as { actions: { key: string; kind: string }[] }).actions
}

beforeEach(() => {
  sharedState.isAuthenticated = true
  sharedState.setStatusOptimistic.mockClear()
  sharedState.removeEntryOptimistic.mockClear()
  sharedState.invalidate.mockClear()
  sharedState.fetchWatchlist.mockClear()
  sharedState.updateWatchlistEntry.mockClear()
  sharedState.pushToast.mockClear()
  sharedState.openSeasonDownload.mockClear()
  sharedState.standalone = true
})

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
})

describe('AnimeContextMenu.vue (Reka DropdownMenu rebuild)', () => {
  it('authenticated + watching with a next episode: 5 status + remove + mark-next + goto + newtab', () => {
    const w = mountMenu({
      listStatus: 'watching',
      episodesWatched: 2,
      episodesTotal: 12,
    })
    const a = actions(w)
    const statusCount = a.filter((x) => x.kind === 'status').length
    expect(statusCount).toBe(5)
    expect(a.some((x) => x.kind === 'remove')).toBe(true)
    expect(a.some((x) => x.kind === 'mark-next')).toBe(true)
    expect(a.some((x) => x.kind === 'goto')).toBe(true)
    expect(a.some((x) => x.kind === 'newtab')).toBe(true)
  })

  it('authenticated + listStatus=null: 5 status, NO remove, NO mark-next, goto+newtab', () => {
    const w = mountMenu({ listStatus: null })
    const a = actions(w)
    expect(a.filter((x) => x.kind === 'status').length).toBe(5)
    expect(a.some((x) => x.kind === 'remove')).toBe(false)
    expect(a.some((x) => x.kind === 'mark-next')).toBe(false)
    expect(a.some((x) => x.kind === 'goto')).toBe(true)
    expect(a.some((x) => x.kind === 'newtab')).toBe(true)
  })

  it('NOT authenticated: no status/remove/mark-next; loginToManage notice shown; goto+newtab present', () => {
    sharedState.isAuthenticated = false
    const w = mountMenu({ listStatus: 'watching', episodesWatched: 1, episodesTotal: 12 })
    const a = actions(w)
    expect(a.some((x) => x.kind === 'status')).toBe(false)
    expect(a.some((x) => x.kind === 'remove')).toBe(false)
    expect(a.some((x) => x.kind === 'mark-next')).toBe(false)
    expect(a.some((x) => x.kind === 'goto')).toBe(true)
    expect(a.some((x) => x.kind === 'newtab')).toBe(true)
    expect(w.text()).toContain('anime.loginToManage')
  })

  it('statusChange fires synchronously on activate (BEFORE the optimistic await)', async () => {
    const w = mountMenu({ listStatus: null })
    const a = actions(w)
    const watching = a.find((x) => x.key === 'status-watching')!
    // Invoke the action's onActivate (same path the DropdownMenuItem @select triggers).
    ;(watching as unknown as { onActivate: () => void }).onActivate()
    // Emit happens synchronously, before any awaited store call resolves.
    expect(w.emitted('statusChange')).toBeTruthy()
    expect(w.emitted('statusChange')![0]).toEqual(['anime-1', 'watching'])
    expect(w.emitted('update:visible')!.at(-1)).toEqual([false])
  })

  it('removeFromList fires synchronously on activate (BEFORE the optimistic await)', () => {
    const w = mountMenu({ listStatus: 'completed' })
    const a = actions(w)
    const remove = a.find((x) => x.kind === 'remove')!
    ;(remove as unknown as { onActivate: () => void }).onActivate()
    expect(w.emitted('removeFromList')).toBeTruthy()
    expect(w.emitted('removeFromList')![0]).toEqual(['anime-1'])
  })

  it('episodesChange fires ONLY AFTER the awaited updateWatchlistEntry resolves', async () => {
    const w = mountMenu({ listStatus: 'watching', episodesWatched: 3, episodesTotal: 12 })
    const a = actions(w)
    const markNext = a.find((x) => x.kind === 'mark-next')!
    const p = (markNext as unknown as { onActivate: () => Promise<void> }).onActivate()
    // Synchronously, before await resolves, the emit must NOT have fired yet.
    expect(w.emitted('episodesChange')).toBeFalsy()
    await p
    expect(sharedState.updateWatchlistEntry).toHaveBeenCalledTimes(1)
    expect(w.emitted('episodesChange')).toBeTruthy()
    expect(w.emitted('episodesChange')![0]).toEqual(['anime-1', 4])
  })

  it('mark-next does NOT emit episodesChange when the update rejects', async () => {
    sharedState.updateWatchlistEntry.mockRejectedValueOnce(new Error('boom'))
    const w = mountMenu({ listStatus: 'watching', episodesWatched: 5, episodesTotal: 12 })
    const a = actions(w)
    const markNext = a.find((x) => x.kind === 'mark-next')!
    await (markNext as unknown as { onActivate: () => Promise<void> }).onActivate()
    expect(w.emitted('episodesChange')).toBeFalsy()
  })

  it('closing the DropdownMenu re-emits update:visible false', async () => {
    const w = mountMenu({ listStatus: null })
    w.findComponent(DropdownMenuStub).vm.$emit('update:open', false)
    await w.vm.$nextTick()
    expect(w.emitted('update:visible')).toBeTruthy()
    expect(w.emitted('update:visible')!.at(-1)).toEqual([false])
  })

  // The component positions via DropdownMenu's anchorPoint mode — its own docs
  // deprecate the bare `reference` prop ("menu rendered unanchored at 0,0.
  // Use anchorPoint"). This replaces the stale reference-forwarding assertion
  // (pre-existing failure) with the real positioning contract.
  it('forwards the x/y anchor point to the DropdownMenu', () => {
    const w = mountMenu({ listStatus: null, x: 11, y: 22 })
    expect(w.findComponent(DropdownMenuStub).props('anchorPoint')).toEqual({ x: 11, y: 22 })
  })

  it('pins Open in new tab as the first action (C-top)', () => {
    const w = mountMenu({ listStatus: 'watching', episodesWatched: 2, episodesTotal: 12 })
    const items = w.findAllComponents({ name: 'DropdownMenuItem' })
    expect(items[0].text()).toContain('contextMenu.openInNewTab')
  })

  it('offers download-season even for anonymous users (downloads are local, not auth-gated)', () => {
    sharedState.isAuthenticated = false
    const w = mountMenu({ listStatus: null })
    expect(actions(w).some((x) => x.kind === 'download-season')).toBe(true)
  })

  it('download-season closes the menu and launches the flow with the anime request', () => {
    const w = mountMenu({ listStatus: null })
    const dl = actions(w).find((x) => x.kind === 'download-season')!
    ;(dl as unknown as { onActivate: () => void }).onActivate()
    expect(w.emitted('update:visible')!.at(-1)).toEqual([false])
    expect(sharedState.openSeasonDownload).toHaveBeenCalledTimes(1)
    const [req, lang] = sharedState.openSeasonDownload.mock.calls[0]
    expect(req).toMatchObject({ animeId: 'anime-1', poster: 'https://example.com/p.jpg' })
    expect(typeof lang).toBe('string')
  })

  it('in a browser tab the item points at the app: install hint toast, no flow', () => {
    sharedState.standalone = false
    const w = mountMenu({ listStatus: null })
    const dl = actions(w).find((x) => x.kind === 'download-season')!
    expect((dl as unknown as { label: string }).label).toBe('downloads.inAppOnly')
    ;(dl as unknown as { onActivate: () => void }).onActivate()
    expect(w.emitted('update:visible')!.at(-1)).toEqual([false])
    expect(sharedState.openSeasonDownload).not.toHaveBeenCalled()
    expect(sharedState.pushToast).toHaveBeenCalledWith('downloads.installHint', 'info', 6000)
  })
})
