import 'fake-indexeddb/auto'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { enqueueDownload, removeDownload, pauseDownload, _resetEngineForTests, _installCachesForTests, _setWatchdogTimeoutsForTests } from './downloadEngine'
import { _resetDbForTests, getDownload, putDownload } from './registry'
import type { StreamResult } from '@/types/aePlayer'

// ── in-memory CacheStorage fake ──────────────────────────────────────────────
class FakeCache {
  store = new Map<string, Response>()
  async match(req: string | Request) {
    const key = typeof req === 'string' ? req : new URL(req.url).pathname
    return this.store.get(key)?.clone()
  }
  async put(req: string | Request, resp: Response) {
    // Spec fidelity: the real Cache API rejects partial responses outright —
    // without this the 206-normalization regression test cannot fail.
    if (resp.status === 206) throw new TypeError('Partial response (status code 206) is unsupported')
    const key = typeof req === 'string' ? req : new URL(req.url).pathname
    this.store.set(key, resp)
  }
}
function fakeCaches() {
  const caches = new Map<string, FakeCache>()
  return {
    caches,
    impl: {
      async open(name: string) {
        if (!caches.has(name)) caches.set(name, new FakeCache())
        return caches.get(name)! as unknown as Cache
      },
      async delete(name: string) { return caches.delete(name) },
      async has(name: string) { return caches.has(name) },
      async keys() { return [...caches.keys()] },
      async match() { return undefined },
    } as unknown as CacheStorage,
  }
}

const MASTER = '#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,RESOLUTION=1280x720\nv/index.m3u8\n'
const MEDIA = '#EXTM3U\n#EXTINF:4,\ns0.ts\n#EXTINF:4,\ns1.ts\n#EXT-X-ENDLIST\n'

function mockFetch(routes: Record<string, () => Response>) {
  return vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    const url = String(input instanceof Request ? input.url : input)
    for (const [suffix, make] of Object.entries(routes)) {
      if (url.endsWith(suffix)) return make()
    }
    return new Response('nf', { status: 404 })
  })
}

const req = (resolve: () => Promise<StreamResult>) => ({
  animeId: 'a1', animeTitle: 'T', quality: '720',
  episode: { key: 1, label: 1, number: 1 },
  combo: { audio: 'sub' as const, lang: 'en' as const, provider: 'gogoanime', server: 's', team: null },
  resolve,
})

beforeEach(async () => {
  await _resetDbForTests()
  _resetEngineForTests()
})

describe('downloadEngine — HLS happy path', () => {
  it('resolves, picks variant, caches playlist+segments, marks done', async () => {
    const { caches, impl } = fakeCaches()
    _installCachesForTests(impl)
    const fetcher = mockFetch({
      'master.m3u8': () => new Response(MASTER),
      'v/index.m3u8': () => new Response(MEDIA),
      's0.ts': () => new Response(new Uint8Array(8)),
      's1.ts': () => new Response(new Uint8Array(8)),
    })
    vi.stubGlobal('fetch', fetcher)
    const id = await enqueueDownload(req(async () => ({ url: 'https://p.example/hls/master.m3u8', type: 'hls' })))
    // engine runs the queue inline in tests (no BG); wait for completion state
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'), { timeout: 10_000 })
    const cache = caches.get(`ae-offline-${id}`)!
    expect(await cache.match(`/__offline/${encodeURIComponent(id)}/master.m3u8`)).toBeTruthy()
    expect(await cache.match(`/__offline/${encodeURIComponent(id)}/r/0`)).toBeTruthy()
    expect(await cache.match(`/__offline/${encodeURIComponent(id)}/r/1`)).toBeTruthy()
    // download marker header rides every media fetch
    const segCall = fetcher.mock.calls.find((c) => String(c[0]).endsWith('s0.ts'))!
    expect((segCall[1] as RequestInit).headers).toMatchObject({ 'X-AE-Download': '1' })
  })

  it('MP4: caches single media.mp4 entry', async () => {
    const { caches, impl } = fakeCaches()
    _installCachesForTests(impl)
    vi.stubGlobal('fetch', mockFetch({ 'ep.mp4': () => new Response(new Uint8Array(16)) }))
    const id = await enqueueDownload(req(async () => ({ url: 'https://p.example/ep.mp4', type: 'mp4' })))
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'), { timeout: 10_000 })
    expect(await caches.get(`ae-offline-${id}`)!.match(`/__offline/${encodeURIComponent(id)}/media.mp4`)).toBeTruthy()
  })

  it('resume skips already-cached resources', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    const fetcher = mockFetch({
      'master.m3u8': () => new Response(MASTER),
      'v/index.m3u8': () => new Response(MEDIA),
      's0.ts': () => new Response(new Uint8Array(8)),
      's1.ts': () => new Response(new Uint8Array(8)),
    })
    vi.stubGlobal('fetch', fetcher)
    const id1 = await enqueueDownload(req(async () => ({ url: 'https://p.example/hls/master.m3u8', type: 'hls' })))
    await vi.waitFor(async () => expect((await getDownload(id1))?.state).toBe('done'), { timeout: 10_000 })
    const callsFirst = fetcher.mock.calls.length
    // simulate an interrupted download surviving an app restart: record not
    // done, but segments already sit in Cache Storage
    await putDownload({ ...(await getDownload(id1))!, state: 'paused' })
    _resetEngineForTests() // fresh engine, same caches
    const id2 = await enqueueDownload(req(async () => ({ url: 'https://p.example/hls/master.m3u8', type: 'hls' })))
    await vi.waitFor(async () => expect((await getDownload(id2))?.state).toBe('done'), { timeout: 10_000 })
    // second run refetches playlists (cheap, needed for the resource map) but no segments
    const segRefetches = fetcher.mock.calls.slice(callsFirst).filter((c) => String(c[0]).includes('.ts'))
    expect(segRefetches.length).toBe(0)
  })

  it('re-resolves once on 403 and continues', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    let expired = true
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('master.m3u8')) return new Response(MASTER)
      if (url.endsWith('v/index.m3u8')) return new Response(MEDIA)
      if (url.includes('.ts')) {
        if (expired) { expired = false; return new Response('sig', { status: 403 }) }
        return new Response(new Uint8Array(8))
      }
      return new Response('nf', { status: 404 })
    })
    vi.stubGlobal('fetch', fetcher)
    const resolve = vi.fn(async () => ({ url: 'https://p.example/hls/master.m3u8', type: 'hls' as const }))
    const id = await enqueueDownload(req(resolve))
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'), { timeout: 10_000 })
    expect(resolve.mock.calls.length).toBe(2) // initial + one re-resolve
  })

  it('remove deletes cache and record', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    vi.stubGlobal('fetch', mockFetch({ 'ep.mp4': () => new Response(new Uint8Array(4)) }))
    const id = await enqueueDownload(req(async () => ({ url: 'https://p.example/ep.mp4', type: 'mp4' })))
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'), { timeout: 10_000 })
    await removeDownload(id)
    expect(await getDownload(id)).toBeUndefined()
    expect(await impl.has(`ae-offline-${id}`)).toBe(false)
  })
})

describe('downloadEngine — mid-batch quota re-check', () => {
  // jsdom has no navigator.storage at all; stub it per-test the same way
  // DownloadsPage.spec.ts stubs the missing navigator.serviceWorker, and
  // remove it afterwards so later tests see the engine's normal
  // headroom === null (no-op) behavior again.
  afterEach(() => {
    delete (navigator as unknown as { storage?: unknown }).storage
  })

  it('fails fast in runDownload when headroom drops between enqueue and run, without ever calling resolve()', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    // A 24-episode season passes enqueueDownload's own pre-check instantly
    // (call #1: plenty of headroom, no bytes landed yet); by the time this
    // item's turn comes up in runDownload (call #2), the disk has filled —
    // the engine-side re-check (this fix) must catch it there.
    let call = 0
    Object.defineProperty(navigator, 'storage', {
      value: {
        persist: async () => true,
        estimate: async () => {
          call++
          return call === 1
            ? { usage: 0, quota: 10 * 2 ** 30 }
            : { usage: 9.9 * 2 ** 30, quota: 10 * 2 ** 30 }
        },
      },
      configurable: true,
    })
    const resolve = vi.fn(async () => ({ url: 'https://p.example/ep.mp4', type: 'mp4' as const }))
    const id = await enqueueDownload(req(resolve))
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('error'), { timeout: 10_000 })
    expect((await getDownload(id))?.error).toBe('quota')
    expect(resolve).not.toHaveBeenCalled()
  })
})

describe('downloadEngine — remove while queued', () => {
  it('removing a still-queued download does not crash the pump or leave an orphan cache, and the queue keeps working', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    vi.stubGlobal('fetch', mockFetch({ 'ep.mp4': () => new Response(new Uint8Array(16)) }))

    const reqFor = (epNumber: number) => ({
      ...req(async () => ({ url: 'https://p.example/ep.mp4', type: 'mp4' as const })),
      episode: { key: epNumber, label: epNumber, number: epNumber },
    })

    // Enqueue two: pump picks up id1 first and starts working it; id2 sits in
    // the queue array behind it (pacedFetch's real spacing means id1 is still
    // mid-flight by the time the synchronous code below runs).
    const id1 = await enqueueDownload(reqFor(1))
    const id2 = await enqueueDownload(reqFor(2))

    // Remove the second while it's still just a queue entry (not yet started).
    await removeDownload(id2)

    // The first, unrelated download must still complete normally.
    await vi.waitFor(async () => expect((await getDownload(id1))?.state).toBe('done'), { timeout: 10_000 })

    // The removed one must leave no record and no orphan cache namespace.
    expect(await getDownload(id2)).toBeUndefined()
    expect(await impl.has(`ae-offline-${id2}`)).toBe(false)

    // The pump must not be stalled/wedged by the removal — a later enqueue
    // still runs to completion.
    const id3 = await enqueueDownload(reqFor(3))
    await vi.waitFor(async () => expect((await getDownload(id3))?.state).toBe('done'), { timeout: 10_000 })
  })
})

describe('downloadEngine — Vue-reactive inputs (DataCloneError regression)', () => {
  // The player's episode list and the card season flow hand the engine
  // Vue-reactive (Proxy) episode/combo objects; IndexedDB's structured clone
  // throws DataCloneError on any Proxy. The engine must store plain copies.
  it('de-proxies episode/combo before the IDB put', async () => {
    const { reactive } = await import('vue')
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    const src = reactive({
      episode: { key: 7, label: 7, number: 7 },
      combo: { audio: 'sub' as const, lang: 'en' as const, provider: 'gogoanime', server: '', team: null },
    })
    const id = await enqueueDownload({
      animeId: 'a1', animeTitle: 'T', quality: '720',
      episode: src.episode,
      combo: src.combo,
      resolve: async () => { throw new Error('halt before network') },
    })
    const rec = await getDownload(id)
    expect(rec).toBeTruthy()
    expect(() => structuredClone(rec!.episode)).not.toThrow()
    expect(() => structuredClone(rec!.combo)).not.toThrow()
    expect(rec!.episode).toMatchObject({ key: 7, number: 7 })
  })
})

describe('projectedBytesFor — duration scaling', () => {
  it('scales by runtime with a 24-min baseline', async () => {
    const { projectedBytesFor } = await import('./downloadEngine')
    expect(projectedBytesFor('1080', 12)).toBe(Math.round((900 * 2 ** 20) / 2))
    expect(projectedBytesFor('1080')).toBe(900 * 2 ** 20)
    expect(projectedBytesFor('480', 24)).toBe(250 * 2 ** 20)
    expect(projectedBytesFor('720', 0)).toBe(450 * 2 ** 20) // invalid → baseline
  })

  it('stamps the duration-scaled projection onto the record', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    const id = await enqueueDownload({
      ...req(async () => { throw new Error('halt') }),
      durationMin: 12,
    })
    const rec = await getDownload(id)
    expect(rec?.projectedBytes).toBe(Math.round((450 * 2 ** 20) / 2))
  })
})

describe('downloadEngine — queue wedge protection (eternal "queued" regression)', () => {
  afterEach(() => {
    _setWatchdogTimeoutsForTests({ headersMs: 45_000, bodyStallMs: 60_000, resolveMs: 120_000 })
  })

  it('claims the active record as downloading before resolve settles', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    let halt!: () => void
    const gate = new Promise<StreamResult>((_, rej) => { halt = () => rej(new Error('halt')) })
    const id = await enqueueDownload(req(() => gate))
    // While the resolver is still in flight, the UI must see activity — a
    // record stuck at 'queued' here was indistinguishable from waiting in line.
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('downloading'))
    halt()
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('error'))
  })

  it('a resolver that never answers times out as error:resolve and frees the queue', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    _setWatchdogTimeoutsForTests({ resolveMs: 50 })
    vi.stubGlobal('fetch', mockFetch({ 'ep.mp4': () => new Response(new Uint8Array(16)) }))
    const id1 = await enqueueDownload(req(() => new Promise<StreamResult>(() => {})))
    const id2 = await enqueueDownload({
      ...req(async () => ({ url: 'https://p.example/ep.mp4', type: 'mp4' as const })),
      episode: { key: 2, label: 2, number: 2 },
    })
    // The hung head of the queue must fail on its own…
    await vi.waitFor(async () => expect((await getDownload(id1))?.error).toBe('resolve'), { timeout: 10_000 })
    // …and the strictly-serial pump must move on to the next item.
    await vi.waitFor(async () => expect((await getDownload(id2))?.state).toBe('done'), { timeout: 10_000 })
  })

  it('parks a paused still-queued record as paused without resolving', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    const resolve = vi.fn(async () => { throw new Error('should not resolve') })
    const id = await enqueueDownload(req(resolve as unknown as () => Promise<StreamResult>))
    pauseDownload(id)
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('paused'))
    expect(resolve).not.toHaveBeenCalled()
  })
})

describe('downloadEngine — full-body 206 normalization (range-gated MP4 hosts)', () => {
  // Sibnet/AllVideo answer a plain GET through the proxy with a
  // bytes 0-(n-1)/n 206; Cache API rejects 206 outright, which used to fail
  // every MP4 download as error:'network' while the server logged success.
  it('caches an MP4 served as a complete 206 and finishes the download', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    const body = new Uint8Array(64)
    vi.stubGlobal('fetch', mockFetch({
      'media.mp4': () => new Response(body, {
        status: 206,
        headers: { 'Content-Range': 'bytes 0-63/64', 'Content-Length': '64' },
      }),
    }))
    const id = await enqueueDownload(req(async () => ({ url: 'https://p.example/media.mp4', type: 'mp4' })))
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'), { timeout: 10_000 })
  })
})
