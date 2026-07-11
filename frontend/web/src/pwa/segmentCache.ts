// SW segment cache — passively tees /api/streaming/hls-proxy SEGMENT responses
// into Cache Storage so the scrub-preview shadow engine's re-fetches become
// local disk hits instead of duplicate provider egress.
//
// Reliability contract (owner directive 2026-07-04):
//  - the MAIN player is never served from this cache (pass-through + tee only);
//  - a cache write NEVER blocks or fails a response (waitUntil + swallow);
//  - writes are skipped outright when storage headroom is unknown or < 1GB.

export const SEG_CACHE = 'ae-seg-v1'
export const SEG_MAX_ENTRIES = 150 // ~2MB/segment → ≤ ~300MB disk
export const SEG_TTL_MS = 3 * 60 * 60 * 1000
export const SCRUB_PARAM = 'aescrub'
const PROXY_PATH = '/api/streaming/hls-proxy'
const MASKED_PREFIX = '/api/streaming/m/'
const MIN_HEADROOM_BYTES = 1_000_000_000
const CACHED_AT = 'x-ae-cached-at'
const SEG_EXT = /\.(ts|m4s)$/i

/** Cache identity of a proxied segment request: the upstream `url` param for
 *  the legacy form; the full token path for the Track A masked form (the
 *  token is opaque — it IS the identity; it stays stable for the lifetime of
 *  one manifest fetch, which covers a VOD watch session).
 *  Returns null for anything that is not a cacheable HLS segment request. */
export function segmentCacheKey(requestUrl: string): string | null {
  try {
    const u = new URL(requestUrl)
    if (u.pathname.includes(MASKED_PREFIX)) {
      // Masked form: /api/streaming/m/<token>/<leaf> — leaf keeps the ext.
      if (!SEG_EXT.test(u.pathname)) return null
      return '/__segcache/?m=' + encodeURIComponent(u.pathname)
    }
    if (!u.pathname.endsWith(PROXY_PATH)) return null
    if (u.searchParams.get('type') === 'mp4') return null
    const upstream = u.searchParams.get('url')
    if (!upstream) return null
    if (!SEG_EXT.test(new URL(upstream).pathname)) return null
    return '/__segcache/?u=' + encodeURIComponent(upstream)
  } catch {
    return null
  }
}

export function isScrubRequest(requestUrl: string): boolean {
  try {
    return new URL(requestUrl).searchParams.get(SCRUB_PARAM) === '1'
  } catch {
    return false
  }
}

/** Tag an hls-proxy URL as scrub-preview traffic (cache-first in the SW). */
export function markScrubUrl(url: string): string {
  try {
    const u = new URL(url, self.location?.href ?? 'https://x.invalid')
    if (!u.pathname.endsWith(PROXY_PATH) && !u.pathname.includes(MASKED_PREFIX)) return url
    if (u.searchParams.get(SCRUB_PARAM) === '1') return url
    u.searchParams.set(SCRUB_PARAM, '1')
    return u.href
  } catch {
    return url
  }
}

async function readSegment(key: string): Promise<Response | null> {
  const cache = await caches.open(SEG_CACHE)
  const hit = await cache.match(key)
  if (!hit) return null
  const at = Number(hit.headers.get(CACHED_AT) ?? 0)
  if (!at || Date.now() - at > SEG_TTL_MS) {
    void cache.delete(key).catch(() => {})
    return null
  }
  return hit
}

async function writeSegment(key: string, resp: Response): Promise<void> {
  const est = await navigator.storage?.estimate?.().catch(() => undefined)
  if (!est || typeof est.quota !== 'number' || typeof est.usage !== 'number') return // unknown → drop on risk
  if (est.quota - est.usage < MIN_HEADROOM_BYTES) return
  const body = await resp.arrayBuffer()
  const headers = new Headers({
    'Content-Type': resp.headers.get('Content-Type') ?? 'video/mp2t',
    [CACHED_AT]: String(Date.now()),
  })
  const cache = await caches.open(SEG_CACHE)
  await cache.put(key, new Response(body, { status: 200, headers }))
  const keys = await cache.keys()
  // Cache API preserves insertion order → the front of keys() is the oldest.
  for (let i = 0; i < keys.length - SEG_MAX_ENTRIES; i++) {
    await cache.delete(keys[i])
  }
}

/** Concurrent tee ceiling — a seek storm must not pile up N×2MB arrayBuffers
 *  in SW memory (drop-on-risk: skip the tee, never the response). */
let inflightTees = 0
const MAX_INFLIGHT_TEES = 4

/** SW fetch handler for proxied segment requests. Scrub-marked → cache-first;
 *  everything else → transparent network with a background tee. Ranged
 *  requests (EXT-X-BYTERANGE streams share one URL across ranges) bypass the
 *  cache entirely — a cached full 200 would corrupt a ranged read. */
export async function handleSegmentRequest(request: Request, event: FetchEvent): Promise<Response> {
  const key = request.headers.has('range') ? null : segmentCacheKey(request.url)
  if (key && isScrubRequest(request.url)) {
    const hit = await readSegment(key).catch(() => null)
    if (hit) return hit
  }
  const resp = await fetch(request)
  if (key && resp.status === 200 && inflightTees < MAX_INFLIGHT_TEES) {
    inflightTees++
    const copy = resp.clone()
    event.waitUntil(
      writeSegment(key, copy)
        .catch(() => {})
        .finally(() => {
          inflightTees--
        }),
    )
  }
  return resp
}
