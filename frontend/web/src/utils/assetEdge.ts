// RU static-asset edge routing (Maskanya). Dark-shipped: only active when
// VITE_MSK_ASSET_HOST is set at build time (wired in vite.config.ts via
// experimental.renderBuiltUrl). When unset, the build is byte-identical to
// today (origin-relative chunk URLs, no runtime indirection).
//
// Strategy (locked): "decide once + cache". A lightweight probe measures warm
// round-trip latency to the origin vs the edge, stores the winning asset base
// URL in localStorage, and the inline <head> script in index.html applies it on
// the NEXT load — so dynamic-import chunk URLs (window.__assetHost) resolve to
// the closer host. First visit always uses origin (no decision yet); the win
// lands on subsequent visits. Falls back to origin on any probe error or
// chunk-load failure (see demoteAssetEdgeToOrigin + utils/chunk-reload.ts).

declare global {
  interface Window {
    __AE_ASSET_BASE__?: string
    __assetHost?: (f: string) => string
  }
}

const MSK: string = (import.meta.env.VITE_MSK_ASSET_HOST as string | undefined) || ''

const CACHE_KEY = 'ae.assetEdge'
const TTL_MS = 24 * 60 * 60 * 1000 // decide-once horizon
const FAIL_TTL_MS = 60 * 60 * 1000 // shorter retry when the edge was unreachable
const SAMPLES = 5 // first sample dropped (cold connection); rest are warm
const FETCH_TIMEOUT_MS = 2500
const MARGIN_MS = 10 // edge must be clearly faster to win (anti-flap)

export interface EdgeCache {
  base: string // '' => origin, otherwise the edge origin URL
  exp: number
}

export function readCache(now: number = Date.now()): EdgeCache | null {
  try {
    const raw = localStorage.getItem(CACHE_KEY)
    if (!raw) return null
    const c = JSON.parse(raw) as Partial<EdgeCache>
    if (c && typeof c.base === 'string' && typeof c.exp === 'number' && c.exp > now) {
      return { base: c.base, exp: c.exp }
    }
  } catch {
    /* ignore malformed JSON / unavailable storage */
  }
  return null
}

export function writeCache(base: string, ttl: number = TTL_MS, now: number = Date.now()): void {
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify({ base, exp: now + ttl }))
  } catch {
    /* ignore */
  }
}

// Pure decision: route to the edge only when it is meaningfully faster.
export function chooseBase(originMin: number, edgeMin: number, edgeHost: string): string {
  if (Number.isFinite(edgeMin) && edgeMin + MARGIN_MS < originMin) return edgeHost
  return ''
}

// Called from utils/chunk-reload.ts when a dynamic import fails. If we were
// routing through the edge, drop to origin (for this session AND the next load)
// so the recovery reload fetches from origin instead of failing again.
export function demoteAssetEdgeToOrigin(): void {
  try {
    if (typeof window !== 'undefined' && window.__AE_ASSET_BASE__) {
      window.__AE_ASSET_BASE__ = ''
      writeCache('', FAIL_TTL_MS)
    }
  } catch {
    /* ignore */
  }
}

async function probeHost(url: string): Promise<number> {
  let best = Infinity
  for (let i = 0; i < SAMPLES; i++) {
    const ctrl = new AbortController()
    const timer = setTimeout(() => ctrl.abort(), FETCH_TIMEOUT_MS)
    const sep = url.includes('?') ? '&' : '?'
    const t0 = performance.now()
    try {
      await fetch(`${url}${sep}p=${i}.${Math.random()}`, {
        cache: 'no-store',
        mode: 'cors',
        credentials: 'omit',
        signal: ctrl.signal,
      })
      const dt = performance.now() - t0
      if (i > 0 && dt < best) best = dt // drop the first (cold-connection) sample
    } catch {
      return Infinity // unreachable / aborted
    } finally {
      clearTimeout(timer)
    }
  }
  return best
}

async function probeAndDecide(): Promise<void> {
  try {
    // Sequential (not parallel) so the two hosts don't contend for bandwidth
    // and skew each other's timing.
    const originMin = await probeHost(`${window.location.origin}/health`)
    const edgeMin = await probeHost(`${MSK}/health`)
    const base = chooseBase(originMin, edgeMin, MSK)
    // Cache origin only briefly if the edge was unreachable, so a transient
    // edge outage isn't sticky for a full day.
    writeCache(base, base === '' && !Number.isFinite(edgeMin) ? FAIL_TTL_MS : TTL_MS)
  } catch {
    writeCache('', FAIL_TTL_MS)
  }
}

export function initAssetEdge(): void {
  if (typeof window === 'undefined') return
  if (!MSK) {
    // Feature off — drop any stale decision so we never keep routing to a
    // now-disabled edge on subsequent loads.
    try {
      localStorage.removeItem(CACHE_KEY)
    } catch {
      /* ignore */
    }
    return
  }
  if (readCache()) return // decide-once: a valid decision is already cached
  const run = (): void => {
    void probeAndDecide()
  }
  if (window.requestIdleCallback) window.requestIdleCallback(run, { timeout: 3000 })
  else setTimeout(run, 1200)
}
