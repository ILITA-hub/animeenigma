// Serves /__offline/{id}/… from the per-download cache. Range support matters
// for MP4 sources: <video> seeks issue byte-range requests, and a 200-only
// server would force full re-buffering on every seek. Blob.slice is lazy
// (disk-backed), so slicing a 500MB body does not load it into RAM.

const CACHE_PREFIX = 'ae-offline-'

export function parseRange(header: string, size: number): { start: number; end: number } | null {
  const m = /^bytes=(\d+)-(\d*)$/.exec(header.trim())
  if (!m) return null
  const start = parseInt(m[1], 10)
  if (start >= size) return null
  const end = m[2] === '' ? size - 1 : Math.min(parseInt(m[2], 10), size - 1)
  if (end < start) return null
  return { start, end }
}

export async function buildRangeResponse(full: Response, rangeHeader: string): Promise<Response> {
  const blob = await full.blob()
  const range = parseRange(rangeHeader, blob.size)
  if (!range) {
    return new Response(null, { status: 416, headers: { 'Content-Range': `bytes */${blob.size}` } })
  }
  const slice = blob.slice(range.start, range.end + 1)
  return new Response(slice, {
    status: 206,
    headers: {
      'Content-Type': full.headers.get('Content-Type') ?? 'application/octet-stream',
      'Content-Range': `bytes ${range.start}-${range.end}/${blob.size}`,
      'Content-Length': String(slice.size),
      'Accept-Ranges': 'bytes',
    },
  })
}

/** /__offline/{id}/{rest} → entry from cache `ae-offline-{id}`. */
export async function handleOfflineRequest(
  request: Request,
  cachesImpl: CacheStorage = caches,
): Promise<Response> {
  const pathname = new URL(request.url).pathname
  const m = /^\/__offline\/([^/]+)\//.exec(pathname)
  if (!m) return new Response('bad offline path', { status: 400 })
  const cache = await cachesImpl.open(CACHE_PREFIX + decodeURIComponent(m[1]))
  const hit = await cache.match(pathname)
  if (!hit) return new Response('not downloaded', { status: 404 })
  const range = request.headers.get('Range')
  if (range) return buildRangeResponse(hit, range)
  return hit
}
