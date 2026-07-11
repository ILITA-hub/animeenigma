// probeH3 — Task 5's periodic upshift probe.
//
// While the ladder cruises on a lower tier (h2/h1), this samples the h3 tier
// to see whether it's now actually faster before accepting an upshift back.
// Pure, framework-free, and defensive: a probe must NEVER throw or otherwise
// affect playback — every non-happy path still resolves and records a
// rejected probe so the debug HUD reflects what was tried.
//
// Browsers only speak h3 to an origin AFTER learning that origin advertises
// it via an `Alt-Svc` response header from a PRIOR request on that origin —
// a single fetch against the h3 tier would silently measure h2 (or h1) over
// that origin's fallback protocol instead. So this primes the origin first
// (fetches the h3-tier PLAYLIST URL, ignoring its body/timing) and only then
// issues the real measurement fetch against the h3-tier SEGMENT URL.

import { PROBE_ACCEPT_FACTOR, type TierId } from './protocolLadder'

/** Minimal surface probeH3 needs from the ladder (read accessors + the two
 * mutating calls it's allowed to make). Kept narrow and structural so tests
 * can inject a plain fake without depending on ProtocolLadder internals. */
export interface ProbeLadder {
  tierBase(id: TierId): string | null
  currentEwmaMbps(): number
  hasProbedH3(): boolean
  recordProbe(mbps: number, accepted: boolean, note: string): void
  switchTo(id: TierId, reason: string): void
}

const PROBE_TIMEOUT_MS = 20_000

/**
 * Swaps only the origin (protocol + host[:port]) of `u` for `base`'s origin,
 * keeping path + query + hash untouched — signed stream/segment URLs stay
 * valid because their signature only covers path+query. `u` may be relative
 * (resolved against the current page origin first); `base` may be `''`
 * (same-origin tier).
 */
function withOrigin(u: string, base: string): string {
  const abs = new URL(u, location.origin)
  const target = new URL(base || '/', location.origin)
  // String-concat the target's origin rather than mutating abs.protocol/host:
  // the WHATWG URL setters don't reliably clear an already-explicit port when
  // the replacement host string carries none (e.g. https://stream3.example
  // has no port, but abs keeps its old :3000), which would corrupt the swap.
  return target.origin + abs.pathname + abs.search + abs.hash
}

/**
 * Primes Alt-Svc on the h3 origin, then measures real throughput against it.
 * Accepts the upshift (`ladder.switchTo('h3', ...)`) when the measurement
 * actually rode h3 (`nextHopProtocol === 'h3'`) and cleared
 * `PROBE_ACCEPT_FACTOR` times the ladder's current EWMA. Every other
 * path — no h3 tier, already probed this session, h3 still unavailable,
 * too slow, fetch failure, or the 20s timeout — records a rejected probe (or
 * no-ops entirely for the first two) and never throws.
 */
export async function probeH3(
  ladder: ProbeLadder,
  sampleUrl: string,
  playlistUrl: string,
  f: typeof fetch = fetch,
): Promise<void> {
  try {
    const h3Base = ladder.tierBase('h3')
    if (h3Base === null) return // no h3 tier configured -> nothing to probe
    if (ladder.hasProbedH3()) return // once-per-session

    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), PROBE_TIMEOUT_MS)
    try {
      // Prime: learn Alt-Svc for the h3 origin. Body/timing intentionally unused.
      await f(withOrigin(playlistUrl, h3Base), { signal: controller.signal })

      // Measure: the real throughput sample, never served from cache.
      const measureUrl = withOrigin(sampleUrl, h3Base)
      const t0 = performance.now()
      const res = await f(measureUrl, { cache: 'no-store', signal: controller.signal })
      const buf = await res.arrayBuffer()
      const elapsedMs = performance.now() - t0
      const mbps = (buf.byteLength * 8) / (elapsedMs / 1000) / 1_000_000

      let protocol = '?'
      try {
        const entries = performance.getEntriesByName(res.url || measureUrl) as Array<{
          nextHopProtocol?: string
        }>
        protocol = entries[entries.length - 1]?.nextHopProtocol ?? '?'
      } catch {
        // resource timing unavailable -> falls through as unknown protocol
      }

      if (protocol !== 'h3') {
        ladder.recordProbe(mbps, false, 'h3-unavailable')
        return
      }

      const neededMbps = ladder.currentEwmaMbps() * PROBE_ACCEPT_FACTOR
      if (mbps >= neededMbps) {
        ladder.recordProbe(mbps, true, '≥1.1× h2')
        ladder.switchTo('h3', 'probe ≥1.1× h2')
      } else {
        ladder.recordProbe(mbps, false, '<1.1× h2')
      }
    } finally {
      clearTimeout(timer)
    }
  } catch {
    try {
      ladder.recordProbe(0, false, 'probe error')
    } catch {
      // a broken ladder must not propagate out of a probe
    }
  }
}
