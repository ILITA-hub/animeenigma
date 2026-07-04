import { ref, type Ref } from 'vue'
import type { Combo, StreamResult, SubtitleTrack } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import { downloadId, offlinePath, type DownloadError, type OfflineDownload } from './types'
import { putDownload, getDownload, deleteDownloadRecord } from './registry'
import { isVod, rewriteMediaPlaylist, selectVariant, type PlaylistResource } from './playlistRewrite'
import { cacheStorageMediaStore, type OfflineMediaStore } from './mediaStore'

export interface DownloadRequest {
  animeId: string
  animeTitle: string
  poster?: string
  episode: EpisodeOption
  combo: Combo
  quality: string
  /** Episode runtime in minutes — scales the size projection (quota checks +
   *  UI estimates). Absent/invalid → the 24-min baseline. */
  durationMin?: number
  /** Fresh stream resolution — called again when signed URLs expire mid-run. */
  resolve: () => Promise<StreamResult>
}

// Pacing: segment fetches are anonymous (no Authorization header), so the
// per-user GCRA never sees them — the binding limits are the per-IP limiter
// and provider-CDN etiquette. 3 rps sustained, 3 in flight, keeps a full
// episode at ~2-3 min while staying gentle on upstream CDNs.
const MIN_FETCH_SPACING_MS = 334
const CONCURRENCY = 3
const MAX_RETRIES = 3

export const engineState: {
  activeId: Ref<string | null>
  progress: Ref<Record<string, { done: number; total: number }>>
} = {
  activeId: ref(null),
  progress: ref({}),
}

// Injected lazily to avoid a static offline→pwa dependency: registerPwa
// defers a deploy reload while a download is in flight (interrupting it
// would leave an unresumable phantom — see isEngineWorking below).
void import('@/pwa/registerPwa').then((m) =>
  m.setActiveDownloadProbe(() => engineState.activeId.value !== null || queue.length > 0),
)

// `caches` is not declared as a global outside browser/SW contexts (jsdom
// test env included) — a bare reference throws ReferenceError, and passing an
// explicit `undefined` re-triggers the adapter's own `= caches` default (same
// crash), so branch around the call entirely, mirroring the pre-port guard.
let store: OfflineMediaStore = typeof caches !== 'undefined'
  ? cacheStorageMediaStore(caches)
  : cacheStorageMediaStore({} as CacheStorage)
/** Kept name/signature so the existing engine spec is untouched: installing a
 *  fake CacheStorage routes it through the real web adapter. */
export function _installCachesForTests(impl: CacheStorage): void {
  store = cacheStorageMediaStore(impl)
}

const queue: { id: string; req: DownloadRequest }[] = []
const paused = new Set<string>()
let running = false
let wakeLock: { release(): Promise<void> } | null = null

export function _resetEngineForTests(): void {
  queue.length = 0
  paused.clear()
  running = false
  engineState.activeId.value = null
  engineState.progress.value = {}
}

async function acquireWakeLock(): Promise<void> {
  try {
    const wl = (navigator as Navigator & { wakeLock?: { request(t: string): Promise<{ release(): Promise<void> }> } }).wakeLock
    if (wl) wakeLock = await wl.request('screen')
  } catch { /* denied/unsupported — download still runs, screen may sleep */ }
}

async function releaseWakeLock(): Promise<void> {
  try { await wakeLock?.release() } catch { /* already released */ }
  wakeLock = null
}

// Slot reservation happens SYNCHRONOUSLY before the await — with 3 concurrent
// workers, read-sleep-then-stamp would let all 3 burst in the same window
// (~9 rps); reserving first serializes the schedule regardless of concurrency.
let nextFetchSlot = 0
async function pacedFetch(url: string): Promise<Response> {
  const at = Math.max(Date.now(), nextFetchSlot)
  nextFetchSlot = at + MIN_FETCH_SPACING_MS
  const wait = at - Date.now()
  if (wait > 0) await new Promise((r) => setTimeout(r, wait))
  return fetch(url, { headers: { 'X-AE-Download': '1' } })
}

class SignatureExpiredError extends Error {}

/** Cache API rejects partial (206) responses outright — but range-gated MP4
 *  hosts (Sibnet/AllVideo via the proxy) answer a no-Range GET with a
 *  bytes 0-(n-1)/n 206 whose body IS the complete file. Restamp those as 200
 *  so cache.put accepts them; a genuinely partial body still falls through
 *  (put throws → retry → error:'network'). */
function normalizeForCache(resp: Response): Response {
  if (resp.status !== 206) return resp
  const m = resp.headers.get('Content-Range')?.match(/bytes (\d+)-(\d+)\/(\d+)/)
  if (!m || m[1] !== '0' || Number(m[2]) + 1 !== Number(m[3])) return resp
  const headers = new Headers(resp.headers)
  headers.delete('Content-Range')
  headers.set('Content-Length', m[3])
  return new Response(resp.body, { status: 200, headers })
}

async function fetchResource(url: string): Promise<Response> {
  let lastErr: unknown
  for (let attempt = 0; attempt < MAX_RETRIES; attempt++) {
    try {
      const resp = await pacedFetch(url)
      if (resp.status === 401 || resp.status === 403) throw new SignatureExpiredError()
      if (!resp.ok) throw new Error(`http ${resp.status}`)
      return normalizeForCache(resp)
    } catch (e) {
      if (e instanceof SignatureExpiredError) throw e
      lastErr = e
      await new Promise((r) => setTimeout(r, 1000 * (attempt + 1)))
    }
  }
  throw lastErr
}

async function planHls(id: string, stream: StreamResult, targetHeight: number): Promise<{
  playlistLocalPath: string
  playlists: { path: string; body: string }[]
  resources: PlaylistResource[]
}> {
  const masterBody = await (await fetchResource(stream.url)).text()
  const variant = selectVariant(masterBody, targetHeight)
  // stream.url is typically a root-relative proxy path — anchor on the document
  // origin before resolving (new URL throws on a relative base).
  const mediaUrl = variant
    ? new URL(variant.uri, new URL(stream.url, window.location.href)).href
    : stream.url
  const mediaBody = variant ? await (await fetchResource(mediaUrl)).text() : masterBody
  if (!isVod(mediaBody)) throw new Error('not-vod')
  const { body, resources } = rewriteMediaPlaylist(mediaBody, mediaUrl, id)
  return {
    playlistLocalPath: offlinePath(id, 'master.m3u8'),
    playlists: [{ path: offlinePath(id, 'master.m3u8'), body }],
    resources,
  }
}

async function cacheSubtitles(id: string, subs: SubtitleTrack[]): Promise<SubtitleTrack[]> {
  const out: SubtitleTrack[] = []
  for (let k = 0; k < subs.length; k++) {
    try {
      const resp = await fetchResource(subs[k].url)
      const path = offlinePath(id, `sub/${k}`)
      await store.put(id, path, resp)
      out.push({ ...subs[k], url: path })
    } catch { /* a missing sub track is not fatal to the download */ }
  }
  return out
}

async function runDownload(id: string, req: DownloadRequest): Promise<void> {
  const record = await getDownload(id)
  if (!record) return // removed while queued — do not resurrect or refetch
  const setError = async (error: DownloadError) => {
    const cur = await getDownload(id)
    if (!cur) return // removed mid-run — do not resurrect the record
    await putDownload({ ...cur, state: 'error', error })
  }

  // Re-check headroom right before doing any real work: the enqueue-time
  // check in enqueueDownload passes a whole season instantly (no bytes
  // downloaded yet); once disk fills mid-batch, every remaining queued item
  // must fail HERE, before paying for a scraper req.resolve().
  const headroom = await quotaHeadroom()
  if (headroom !== null && headroom < projectedBytesFor(req.quality, req.durationMin)) {
    return setError('quota')
  }

  let stream: StreamResult
  try {
    stream = await req.resolve()
  } catch {
    return setError('resolve')
  }

  let plan: { playlistLocalPath: string; playlists: { path: string; body: string }[]; resources: PlaylistResource[] }
  try {
    plan =
      stream.type === 'mp4'
        ? { playlistLocalPath: offlinePath(id, 'media.mp4'), playlists: [], resources: [{ path: offlinePath(id, 'media.mp4'), url: stream.url }] }
        : await planHls(id, stream, parseInt(req.quality, 10) || 720)
  } catch (e) {
    return setError(e instanceof Error && e.message === 'not-vod' ? 'mismatch' : 'network')
  }

  try {
    for (const p of plan.playlists) {
      await store.put(id, p.path, new Response(p.body, { headers: { 'Content-Type': 'application/vnd.apple.mpegurl' } }))
    }
  } catch (e) {
    // Runs while the record is still 'queued' (before update() exists) — a
    // throw here must not escape to pump()'s catch, or the record is
    // stranded at 'queued' forever (spinner, no error, no work).
    const quota = e instanceof DOMException && e.name === 'QuotaExceededError'
    return setError(quota ? 'quota' : 'network')
  }
  const localSubs = await cacheSubtitles(id, stream.subtitles ?? [])
  let posterOk = false
  if (req.poster) {
    try {
      await store.put(id, offlinePath(id, 'poster'), await fetchResource(req.poster))
      posterOk = true
    } catch { /* poster is cosmetic — CORS on external hosts is expected */ }
  }

  const total = plan.resources.length
  let done = 0
  let bytes = record.bytes
  const update = async (state: OfflineDownload['state'], error?: DownloadError) => {
    engineState.progress.value = { ...engineState.progress.value, [id]: { done, total } }
    const cur = await getDownload(id)
    if (!cur) return // removed mid-run — do not resurrect the record
    await putDownload({
      ...cur,
      state, error, bytes, resourcesDone: done, resourcesTotal: total,
      streamType: stream.type, playlistLocalPath: plan.playlistLocalPath,
      subtitles: localSubs, posterPath: posterOk ? offlinePath(id, 'poster') : undefined,
    })
  }
  await update('downloading')

  // Single-flight re-resolve: signed URLs expire hourly; the FIRST worker that
  // hits 401/403 re-resolves and splices fresh URLs into the shared plan (same
  // local paths); concurrent workers await the same promise, then each retries
  // its own item exactly once.
  let reResolving: Promise<void> | null = null
  function ensureFreshUrls(): Promise<void> {
    if (!reResolving) {
      reResolving = (async () => {
        const fresh = await req.resolve()
        const freshPlan = fresh.type === 'mp4'
          ? { resources: [{ path: offlinePath(id, 'media.mp4'), url: fresh.url }] }
          : await planHls(id, fresh, parseInt(req.quality, 10) || 720)
        if (freshPlan.resources.length !== plan.resources.length) throw new Error('mismatch')
        for (let i = 0; i < plan.resources.length; i++) plan.resources[i].url = freshPlan.resources[i].url
      })()
    }
    return reResolving
  }

  async function storeItem(item: PlaylistResource, resp: Response): Promise<void> {
    if (item.path.endsWith('/media.mp4')) {
      // MP4 is one huge body — stream it straight to Cache Storage; buffering
      // hundreds of MB through arrayBuffer() OOMs mobile tabs.
      const len = parseInt(resp.headers.get('Content-Length') ?? '0', 10)
      bytes += Number.isFinite(len) ? len : 0
      await store.put(id, item.path, resp)
      return
    }
    const buf = await resp.arrayBuffer()
    bytes += buf.byteLength
    await store.put(id, item.path, new Response(buf, { headers: { 'Content-Type': resp.headers.get('Content-Type') ?? 'application/octet-stream' } }))
  }

  async function fetchItem(item: PlaylistResource): Promise<void> {
    try {
      await storeItem(item, await fetchResource(item.url))
    } catch (e) {
      if (!(e instanceof SignatureExpiredError)) throw e
      await ensureFreshUrls()
      // one retry with the fresh URL; a second 401/403 is a real failure
      await storeItem(item, await fetchResource(item.url))
    }
  }

  let cursor = 0
  const worker = async (): Promise<void> => {
    while (cursor < plan.resources.length) {
      if (paused.has(id)) return
      const item = plan.resources[cursor++]
      if (!(await store.has(id, item.path))) await fetchItem(item)
      done++
      engineState.progress.value = { ...engineState.progress.value, [id]: { done, total } }
    }
  }

  try {
    await Promise.all(Array.from({ length: CONCURRENCY }, () => worker()))
    if (paused.has(id)) return void (await update('paused'))
    await update('done')
  } catch (e) {
    const quota = e instanceof DOMException && e.name === 'QuotaExceededError'
    await update('error', quota ? 'quota' : e instanceof Error && e.message === 'mismatch' ? 'mismatch' : 'network')
  }
}

async function pump(): Promise<void> {
  if (running) return
  running = true
  await acquireWakeLock()
  try {
    while (queue.length > 0) {
      const { id, req } = queue.shift()!
      engineState.activeId.value = id
      try {
        await runDownload(id, req)
      } catch {
        // a throw here is a bug in runDownload's own error handling — never
        // let it abandon the rest of the queue
      }
      engineState.activeId.value = null
    }
  } finally {
    running = false
    await releaseWakeLock()
  }
}

// Conservative per-quality size projections for the pre-download quota check
// (also shown as the dialog's size hint), calibrated for a ~24-min episode.
// Real size lands in `bytes` as it downloads; QuotaExceededError mid-flight
// is still handled as error:'quota'.
export const PROJECTED_BYTES: Record<string, number> = {
  '480': 250 * 2 ** 20,
  '720': 450 * 2 ** 20,
  '1080': 900 * 2 ** 20,
}

/** Duration-aware projection: PROJECTED_BYTES is calibrated for ~24-min
 *  episodes; a 12-min short projects half. Unknown/invalid duration → 24. */
export function projectedBytesFor(quality: string, durationMin?: number): number {
  const base = PROJECTED_BYTES[quality] ?? PROJECTED_BYTES['720']
  const mins = typeof durationMin === 'number' && durationMin > 0 && durationMin < 600 ? durationMin : 24
  return Math.round((base * mins) / 24)
}

function quotaHeadroom(): Promise<number | null> {
  return store.estimate().then((est) => (est ? est.quota - est.usage : null))
}

/** Storage headroom for callers outside the engine (Task 12 quota UI). */
export function storageEstimate(): Promise<{ usage: number; quota: number } | null> {
  return store.estimate()
}

export async function enqueueDownload(req: DownloadRequest): Promise<string> {
  const id = downloadId(req.animeId, req.episode.number, req.combo, req.quality)
  await store.persist()
  const existing = await getDownload(id)
  if (existing?.state === 'done') return id
  paused.delete(id)
  const baseRecord = {
    // Plain copies: callers pass Vue-reactive episode/combo objects (player
    // episode list, card season flow) and IndexedDB's structured clone throws
    // DataCloneError on any Proxy. Both types are flat primitive-field shapes,
    // so a spread fully de-proxies them.
    id, animeId: req.animeId, animeTitle: req.animeTitle, episode: { ...req.episode },
    combo: { ...req.combo }, quality: req.quality, streamType: 'hls' as const,
    bytes: existing?.bytes ?? 0, resourcesDone: 0, resourcesTotal: 0,
    createdAt: existing?.createdAt ?? Date.now(),
    playlistLocalPath: offlinePath(id, 'master.m3u8'), subtitles: [],
    projectedBytes: projectedBytesFor(req.quality, req.durationMin),
  }
  const headroom = await quotaHeadroom()
  if (headroom !== null && headroom < baseRecord.projectedBytes) {
    await putDownload({ ...baseRecord, state: 'error', error: 'quota' })
    return id
  }
  await putDownload({ ...baseRecord, state: 'queued' })
  queue.push({ id, req })
  void pump()
  return id
}

export function pauseDownload(id: string): void {
  paused.add(id)
}

/** True when the engine is actively working or holding this id in its queue. */
export function isEngineWorking(id: string): boolean {
  return engineState.activeId.value === id || queue.some((q) => q.id === id)
}

export async function resumeDownload(req: DownloadRequest): Promise<string> {
  return enqueueDownload(req) // resume = re-enqueue; cached resources are skipped
}

export async function removeDownload(id: string): Promise<void> {
  paused.add(id) // stop an in-flight run at the next item boundary
  for (let i = queue.length - 1; i >= 0; i--) if (queue[i].id === id) queue.splice(i, 1)
  await store.remove(id)
  await deleteDownloadRecord(id)
  const { [id]: _, ...rest } = engineState.progress.value
  engineState.progress.value = rest
}

/** Startup scan: registry entries whose cache Chrome evicted → error:'evicted'. */
export async function markEvicted(list: OfflineDownload[]): Promise<OfflineDownload[]> {
  const out: OfflineDownload[] = []
  for (const d of list) {
    if (d.state === 'done' && !(await store.exists(d.id))) {
      const marked: OfflineDownload = { ...d, state: 'error', error: 'evicted' }
      await putDownload(marked)
      out.push(marked)
    } else {
      out.push(d)
    }
  }
  return out
}
