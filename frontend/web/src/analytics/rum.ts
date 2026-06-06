// RUM (Real User Monitoring) resource-timing observer. A PerformanceObserver on
// 'resource' entries aggregates browser→3rd-party timings per host per flush
// window and beacons one byte-poor row per host through the EXISTING Transport.
//
// AR-FE-03 contract (RESEARCH Pattern 3 / Pitfall 1 + threat T-04-04/05/06):
//   - source='fe_rum', accuracy=approx: cross-origin sizes are opaque
//     (byte counters report 0 without Timing-Allow-Origin), so these rows are
//     structurally byte-poor — NEVER read any cross-origin byte-size field.
//   - target is the HOST ONLY (new URL().host) — signed CDN segment URLs carry
//     `tham/h` auth windows; never ship the full URL (Information Disclosure).
//   - Self-host (location.host) entries are dropped — only browser→3rd-party.
//   - Aggregate per (flush-window, host) — one row per host, not per HLS
//     segment — to avoid per-resource row explosion during playback.
import { analytics } from './index'

// Emit seam — defaults to the singleton's track() so callers just `initRum()`,
// but the spec injects a vi.fn() to assert on the emitted (name, props) directly.
type EmitFn = (name: string, props: Record<string, unknown>) => void

let started = false

export function initRum(emit?: EmitFn): void {
  if (started) return // idempotent, mirrors index.ts:20
  if (typeof PerformanceObserver === 'undefined') return // silent no-op (diagnostics.ts analog)

  const track: EmitFn = emit ?? ((name, props) => analytics.track(name, props))

  try {
    const observer = new PerformanceObserver((list) => {
      // Aggregate this window's entries per 3rd-party host.
      const byHost = new Map<string, { count: number; dur: number }>()
      for (const e of list.getEntries()) {
        const res = e as PerformanceResourceTiming
        let host: string
        try {
          host = new URL(res.name).host
        } catch {
          continue // unparseable resource name — skip
        }
        if (!host || host === location.host) continue // self-host → drop
        const agg = byHost.get(host) ?? { count: 0, dur: 0 }
        agg.count += 1
        // entry.duration is the ONE reliable cross-origin timing field.
        // NEVER read cross-origin byte-size fields — they are 0 without TAO.
        agg.dur += res.duration
        byHost.set(host, agg)
      }
      for (const [host, agg] of byHost) {
        track('rum.resource', {
          source: 'fe_rum',
          target_kind: 'host',
          target: host, // host ONLY — no path, no query, no signed token
          requests: agg.count,
          duration_ms: Math.round(agg.dur),
        })
      }
    })
    observer.observe({ type: 'resource', buffered: true }) // buffered catches pre-init entries
    started = true
  } catch {
    // PerformanceObserver('resource') unsupported — silent no-op.
  }
}
