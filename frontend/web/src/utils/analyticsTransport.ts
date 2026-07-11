// Shared analytics endpoint resolver with adblock fallback (Track B5,
// spec docs/superpowers/specs/2026-07-10-playback-resilience-constrained-browsers-design.md §4).
//
// Static filter lists (EasyPrivacy-class) block /api/analytics/* by URL
// shape, silently zeroing telemetry for exactly the users whose playback
// failures we most need to see (the @gerahertz report class: every analytics
// request status 0). The gateway exposes a rotating HMAC-masked alias
// (/api/<hmac-hour-bucket>/{c|e|p}) and advertises the current base via the
// X-AE-Cfg response header on every /api response; the axios client captures
// it here. Detection is a one-shot probe fetch per session — a TypeError
// rejection means the request was blocked client-side before the network —
// plus opportunistic detection on any later fetch-fallback failure. Once
// blocked, all three analytics clients (playerTelemetry, feErrorLog, the
// clickstream Transport) resolve to the masked base for the session.
// Fail-open: with no learned masked base, behavior is identical to before
// this module existed.

export type AnalyticsLeaf = 'collect' | 'client-errors' | 'player-events'

const LEAF_CODE: Record<AnalyticsLeaf, string> = {
  collect: 'c',
  'client-errors': 'e',
  'player-events': 'p',
}

let maskedBase: string | null = null
let blocked = false
let probeFired = false

/** The configured (or default `/api`) analytics API base. */
function apiBase(): string {
  return (import.meta.env.VITE_API_URL || '/api') as string
}

/** Store the masked base learned from an X-AE-Cfg response header.
 *  Strictly validated — never trust an arbitrary header value as a URL. */
export function noteMaskedAnalyticsPath(value: string | undefined | null): void {
  if (typeof value === 'string' && /^\/api\/[0-9a-f]{24}$/.test(value)) {
    maskedBase = value
  }
}

/** True when url targets the masked alias (feErrorLog self-traffic guard). */
export function isMaskedAnalyticsUrl(url: string): boolean {
  return maskedBase !== null && url.includes(maskedBase)
}

/** Masked override when this session is blocked, else null (callers keep
 *  their primary endpoint). */
export function maskedOverrideFor(leaf: AnalyticsLeaf): string | null {
  if (!blocked || !maskedBase) return null
  // maskedBase is an absolute /api/<seg> path; keep any non-default origin.
  return `${apiBase().replace(/\/api$/, '')}${maskedBase}/${LEAF_CODE[leaf]}`
}

/** Resolve the endpoint for a leaf: masked when blocked, primary otherwise. */
export function analyticsEndpoint(leaf: AnalyticsLeaf): string {
  const override = maskedOverrideFor(leaf)
  if (override) return override
  return `${apiBase()}/analytics/${leaf}`
}

/** Mark the session blocked from a fetch rejection. TypeError = the request
 *  was blocked client-side before reaching the network (the adblock
 *  signature — an HTTP error status resolves, it never rejects). Returns
 *  whether the session just flipped (caller then retries once, masked). */
export function markBlockedFromError(err: unknown): boolean {
  if (!blocked && err instanceof TypeError && maskedBase !== null) {
    blocked = true
    return true
  }
  return false
}

/** One-shot reachability probe (fired by the axios client once the masked
 *  base is known). An empty batch reaches the collect handler and enqueues
 *  nothing; ANY HTTP response — even a 4xx — proves the URL is not
 *  extension-blocked, so only a TypeError flips the session. */
export function probeAnalyticsReachability(): void {
  if (probeFired || maskedBase === null || typeof fetch === 'undefined') return
  probeFired = true
  try {
    void fetch(`${apiBase()}/analytics/collect`, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: '{"events":[]}',
      keepalive: true,
      credentials: 'include',
    }).catch((err) => {
      markBlockedFromError(err)
    })
  } catch {
    // never throw into callers
  }
}

/** Test-only reset. */
export function __resetAnalyticsTransportForTest(): void {
  maskedBase = null
  blocked = false
  probeFired = false
}
