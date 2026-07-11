// Protocol ladder — QoE tier state machine for HLS segment delivery.
//
// Browsers pick the HTTP protocol per-origin, not per-request — so instead of
// juggling protocol flags, the ladder switches between three dedicated
// origins (stream3 = h3/QUIC, stream2 = h2, stream1 = h1.1), each pinned to a
// protocol ceiling by nginx (see docs/2026-07-11-protocol-ladder-design.md +
// task-1's host-side vhosts). Segment fetches measure their own throughput;
// when a tier is consistently too slow (or a fragment looks stuck), the
// ladder downshifts to a more conservative tier. A separate probe (Task 5)
// periodically samples the tier above the current one and can accept an
// upshift back via `switchTo`.
//
// Pure TypeScript, framework-free (no Vue imports) — safe to import from
// anywhere in the player stack. All browser-only globals (`navigator`,
// `localStorage`) are feature-checked / try-caught so this module is inert
// under jsdom, SSR, or any environment that lacks them.

export type TierId = 'h3' | 'h2' | 'h1'

export interface Tier {
  id: TierId
  base: string
}

export interface FragReport {
  bytes: number
  ms: number
  mediaDurationS: number
  protocol?: string
}

export interface InflightState {
  url: string
  receivedBytes: number
  totalBytes: number
  elapsedMs: number
}

export interface LadderDebug {
  tierId: TierId
  tierIndex: number
  tierCount: number
  protocol: string // nextHopProtocol of last reported fragment, '?' unknown
  measuredMbps: number
  neededMbps: number
  trail: string // "h3→h2 (first-frag projected 17s)" style, '' if none
  probe: string // "h3 2.1 Mbps @03:24 — rejected (<1.1× h2)" | '' when unset
}

/**
 * I4 fix: formats a `LadderDebug` snapshot into the exact hacker-mode HUD row
 * strings, as a pure function so the 1-based tier display ("tier 2/3" for
 * the middle tier of a 3-tier ladder) is unit-testable without going through
 * AePlayer.vue. `debugSnapshot()`'s own `tierIndex` stays 0-based (its
 * established contract) — the `+1` lives ONLY here, at render time.
 */
export function formatLadderRows(
  snap: LadderDebug,
): { proto: string; net: string; laddr: string; probe: string } {
  return {
    proto: `${snap.protocol} · tier ${snap.tierIndex + 1}/${snap.tierCount}`,
    net: `${snap.measuredMbps.toFixed(1)} Mbps ewma / need ${snap.neededMbps.toFixed(1)} ×1.2`,
    laddr: snap.trail,
    probe: snap.probe,
  }
}

// Policy constants (exported for tests + Task 5's probe).
export const SAFETY_FACTOR = 1.2
export const CONSEC_SLOW_FRAGS = 3
export const TIMEOUTS_TO_DOWNSHIFT = 2
export const FIRSTFRAG_MIN_ELAPSED_MS = 3000
export const FIRSTFRAG_PROJECTED_MS = 8000
export const SWITCH_COOLDOWN_MS = 30_000
export const PROBE_ACCEPT_FACTOR = 1.1
export const PERSIST_TTL_MS = 86_400_000
export const EWMA_ALPHA = 0.3
export const LS_KEY = 'ae:protocolLadder:v1'

// Canonical descending-preference rank, independent of input/config order.
const TIER_RANK: Record<TierId, number> = { h3: 0, h2: 1, h1: 2 }

function stripTrailingSlashes(s: string): string {
  return s.replace(/\/+$/, '')
}

/**
 * Parses `VITE_HLS_PROXY_TIERS` (`"h3=https://a,h2=https://b,h1=https://c"`)
 * into an ordered (h3, h2, h1) tier list. Unknown keys and malformed pairs
 * (missing `=`, empty id) are skipped. When no valid tier can be parsed,
 * falls back to a single h2 tier rooted at `fallbackBase`
 * (`VITE_HLS_PROXY_BASE`), or `''` (relative, same-origin) when that is also
 * unset — matching the pre-ladder single-origin behavior.
 */
export function parseTiers(tiersRaw: string | undefined, fallbackBase: string | undefined): Tier[] {
  const out: Tier[] = []
  if (tiersRaw) {
    for (const pair of tiersRaw.split(',')) {
      const eq = pair.indexOf('=')
      if (eq <= 0) continue // no '=' or empty id -> malformed, skip
      const id = pair.slice(0, eq).trim()
      const base = stripTrailingSlashes(pair.slice(eq + 1).trim())
      if (id === 'h3' || id === 'h2' || id === 'h1') {
        out.push({ id, base })
      }
      // unknown key -> skip
    }
  }
  out.sort((a, b) => TIER_RANK[a.id] - TIER_RANK[b.id])
  if (out.length > 0) return out
  if (fallbackBase) return [{ id: 'h2', base: stripTrailingSlashes(fallbackBase) }]
  return [{ id: 'h2', base: '' }]
}

/**
 * Watchdog guard (the tNeymik "stale"-loop regression, spec §4): a first
 * fragment with bytes flowing is SLOW, not dead — the watchdog must defer to
 * the ladder instead of aborting/re-resolving.
 */
export function shouldDeferStallToLadder(inflight: InflightState | null): boolean {
  if (!inflight) return false
  return inflight.receivedBytes > 0
}

type StorageLike = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

interface PersistedState {
  tier: TierId
  ewma: number
  probedH3: boolean
  ts: number
}

interface ConnectionLike {
  addEventListener?: (type: string, cb: () => void) => void
}

function safeLocalStorage(): StorageLike | null {
  try {
    if (typeof localStorage === 'undefined') return null
    return localStorage
  } catch {
    return null
  }
}

function formatClockUTC(ms: number): string {
  const d = new Date(ms)
  const hh = String(d.getUTCHours()).padStart(2, '0')
  const mm = String(d.getUTCMinutes()).padStart(2, '0')
  return `${hh}:${mm}`
}

export class ProtocolLadder {
  private readonly tiers: Tier[]
  private readonly now: () => number
  private readonly storage: StorageLike | null
  private readonly listeners = new Set<(tier: Tier, reason: string) => void>()

  private tierIndex = 0

  // EWMA throughput tracking (persisted across tier switches / reloads).
  private measuredEwmaBps = 0
  private neededEwmaBps = 0
  private lastProtocol = '?'

  // Per-tier counters, reset on every switch.
  private fragSamples = 0
  private consecSlow = 0
  private timeoutCount = 0
  private hasCompletedFragOnTier = false

  // Inflight fragment tracking (single active fragment at a time).
  private inflightUrl: string | null = null
  private inflightOpenTs = 0
  private inflightReceivedBytes = 0
  private inflightTotalBytes = 0

  private lastSwitchTs = Number.NEGATIVE_INFINITY
  private trail = ''
  private lastProbe = ''
  private probedH3 = false

  constructor(tiers: Tier[], deps?: { now?: () => number; storage?: StorageLike }) {
    this.tiers = tiers.length > 0 ? tiers : [{ id: 'h2', base: '' }]
    this.now = deps?.now ?? (() => Date.now())
    this.storage = deps?.storage !== undefined ? deps.storage : safeLocalStorage()
    this.tierIndex = this.computeEntryIndex()
    this.attachConnectionListener()
  }

  isMultiTier(): boolean {
    return this.tiers.length > 1
  }

  currentBase(): string {
    return this.tiers[this.tierIndex].base
  }

  /**
   * Read-only accessor for Task 5's h3 probe: a tier's configured base URL,
   * or null when that tier isn't present in the ladder (single-tier config,
   * or a multi-tier config that simply omits h3).
   */
  tierBase(id: TierId): string | null {
    const tier = this.tiers.find((t) => t.id === id)
    return tier ? tier.base : null
  }

  /**
   * Read-only accessor for Task 5's h3 probe: the running measured-throughput
   * EWMA (see `reportFragment`), in Mbps. Not tier-scoped — it's whatever the
   * currently-active tier has been delivering, which is exactly what the
   * probe wants to beat by `PROBE_ACCEPT_FACTOR`.
   */
  currentEwmaMbps(): number {
    return this.measuredEwmaBps / 1_000_000
  }

  /**
   * Read-only accessor for Task 5's h3 probe: whether an h3 probe has already
   * run this session (persisted alongside the tier, see `recordProbe` /
   * `PersistedState.probedH3`) — the probe is a once-per-session affair.
   */
  hasProbedH3(): boolean {
    return this.probedH3
  }

  /**
   * Read-only accessor for Task 5's h3 probe (M5 fix): the currently active
   * tier id, so the probe can label its rejection/acceptance notes against
   * the tier it's actually comparing against instead of a hardcoded 'h2'.
   */
  currentTierId(): TierId {
    return this.tiers[this.tierIndex].id
  }

  reportFragment(r: FragReport): void {
    if (!this.isMultiTier()) return
    this.lastProtocol = r.protocol ?? '?'
    this.inflightUrl = null // fragment completed -> clear the inflight slot

    // I3: a non-positive bytes/ms/mediaDurationS sample (e.g. an aborted or
    // degenerate load somehow routed here) would divide-by-zero into
    // Infinity/NaN and poison the EWMA forever. The inflight clear above
    // still runs unconditionally so loadend/report ordering keeps working.
    if (!(r.bytes > 0) || !(r.ms > 0) || !(r.mediaDurationS > 0)) return

    const measuredSample = (r.bytes * 8) / (r.ms / 1000)
    const neededSample = (r.bytes * 8) / r.mediaDurationS
    const seeded = this.fragSamples > 0
    this.measuredEwmaBps = seeded
      ? EWMA_ALPHA * measuredSample + (1 - EWMA_ALPHA) * this.measuredEwmaBps
      : measuredSample
    this.neededEwmaBps = seeded
      ? EWMA_ALPHA * neededSample + (1 - EWMA_ALPHA) * this.neededEwmaBps
      : neededSample
    this.fragSamples += 1
    this.hasCompletedFragOnTier = true

    if (this.fragSamples < 2) return // require >=2 samples before judging slow/fast

    if (this.measuredEwmaBps < this.neededEwmaBps * SAFETY_FACTOR) {
      this.consecSlow += 1
      if (this.consecSlow >= CONSEC_SLOW_FRAGS) {
        // On success resetTierCounters() zeroes consecSlow; a blocked attempt
        // (cooldown / h1 floor) keeps the count armed so the very next slow
        // evaluation retries promptly instead of re-accumulating from zero.
        this.downshift('ewma <need×1.2 ×3')
      }
    } else {
      this.consecSlow = 0
    }
  }

  reportTimeout(): void {
    if (!this.isMultiTier()) return
    this.timeoutCount += 1
    if (this.timeoutCount >= TIMEOUTS_TO_DOWNSHIFT) {
      // On success resetTierCounters() zeroes timeoutCount; a cooldown-blocked
      // attempt keeps it armed (see reportFragment).
      this.downshift('frag timeouts ×2')
    }
  }

  onXhrOpen(url: string): void {
    if (!this.isMultiTier()) return
    this.inflightUrl = url
    this.inflightOpenTs = this.now()
    this.inflightReceivedBytes = 0
    this.inflightTotalBytes = 0
  }

  onXhrProgress(url: string, loaded: number, total: number): void {
    if (!this.isMultiTier()) return
    if (this.inflightUrl !== url) {
      // Defensive: progress without a matching open — treat as a fresh open
      // so elapsed-time math stays sane.
      this.inflightUrl = url
      this.inflightOpenTs = this.now()
    }
    this.inflightReceivedBytes = loaded
    this.inflightTotalBytes = total

    if (this.hasCompletedFragOnTier) return
    if (total <= 0 || loaded <= 0) return

    const elapsedMs = this.now() - this.inflightOpenTs
    if (elapsedMs <= FIRSTFRAG_MIN_ELAPSED_MS) return

    const projectedMs = elapsedMs * (total / loaded)
    if (projectedMs > FIRSTFRAG_PROJECTED_MS) {
      const projectedS = Math.round(projectedMs / 1000)
      // "Downshift once" is structural, no latch flag needed: a successful
      // switch ends this tier-residency (resetTierCounters clears the
      // inflight slot, so projection timing restarts on the new tier), while
      // a cooldown-blocked attempt intentionally stays armed so a later
      // progress event retries once the cooldown expires (review finding:
      // blocked degradation signals must not be discarded).
      this.downshift(`first-frag projected ${projectedS}s`)
    }
  }

  inflight(): InflightState | null {
    if (!this.isMultiTier() || this.inflightUrl === null) return null
    return {
      url: this.inflightUrl,
      receivedBytes: this.inflightReceivedBytes,
      totalBytes: this.inflightTotalBytes,
      elapsedMs: this.now() - this.inflightOpenTs,
    }
  }

  /**
   * C1 fix: clears the inflight record when a tracked XHR finishes for ANY
   * reason — meant to be wired to the XHR `loadend` event (fires on
   * load/abort/error/timeout alike). Before this, only `reportFragment` (a
   * successful FRAG_LOADED) or a tier switch ever cleared the slot, so a
   * completed playlist/level XHR (which never calls reportFragment) or an
   * aborted fragment XHR left a stale `bytes>0` record in place forever —
   * which made `shouldDeferStallToLadder` defer the AePlayer silent-stall
   * watchdog indefinitely. No-op if `url` doesn't match the currently
   * tracked slot (already superseded by a newer `onXhrOpen`), and harmless
   * to call after `reportFragment` already cleared it (double-clear is a
   * no-op either way).
   */
  onXhrLoadEnd(url: string): void {
    if (!this.isMultiTier()) return
    if (this.inflightUrl === url) this.inflightUrl = null
  }

  /**
   * Unconditionally clears the inflight slot. Called from engine.load() at
   * the start of a new source so a stale inflight record from the PREVIOUS
   * source (a different XHR URL entirely) can never leak into the new
   * source's watchdog checks.
   */
  clearInflight(): void {
    this.inflightUrl = null
  }

  onChange(cb: (tier: Tier, reason: string) => void): () => void {
    this.listeners.add(cb)
    return () => {
      this.listeners.delete(cb)
    }
  }

  /**
   * Used by Task 5's probe to record a measurement of `tier` (the tier that
   * was ACTUALLY probed — e.g. always 'h3' for probeH3). M5 fix: this used
   * to derive the label from "whatever tier is one above the current
   * index," which mislabeled the probed tier whenever the ladder wasn't
   * sitting on h2 (e.g. probing h3 while already parked on h1 logged the
   * measurement as an "h2" probe).
   */
  recordProbe(tier: TierId, mbps: number, accepted: boolean, note: string): void {
    if (!this.isMultiTier()) return
    this.probedH3 = true
    const ts = formatClockUTC(this.now())
    const verdict = accepted ? 'accepted' : 'rejected'
    this.lastProbe = `${tier} ${mbps.toFixed(1)} Mbps @${ts} — ${verdict} (${note})`
  }

  /** Probe upshift entry — jumps directly to the given tier (e.g. a probe-accepted h3). */
  switchTo(id: TierId, reason: string): void {
    if (!this.isMultiTier()) return
    const now = this.now()
    if (now - this.lastSwitchTs < SWITCH_COOLDOWN_MS) return
    const idx = this.tiers.findIndex((t) => t.id === id)
    if (idx < 0 || idx === this.tierIndex) return
    this.applySwitch(idx, reason, now)
  }

  debugSnapshot(): LadderDebug | null {
    if (!this.isMultiTier()) return null
    return {
      tierId: this.tiers[this.tierIndex].id,
      tierIndex: this.tierIndex,
      tierCount: this.tiers.length,
      protocol: this.lastProtocol,
      measuredMbps: this.measuredEwmaBps / 1_000_000,
      neededMbps: this.neededEwmaBps / 1_000_000,
      trail: this.trail,
      probe: this.lastProbe,
    }
  }

  private computeEntryIndex(): number {
    const persisted = this.readPersisted()
    if (persisted && this.now() - persisted.ts < PERSIST_TTL_MS) {
      const idx = this.tiers.findIndex((t) => t.id === persisted.tier)
      if (idx >= 0) {
        this.measuredEwmaBps = persisted.ewma || 0
        this.probedH3 = persisted.probedH3
        return idx
      }
    }
    const h2idx = this.tiers.findIndex((t) => t.id === 'h2')
    return h2idx >= 0 ? h2idx : 0
  }

  private readPersisted(): PersistedState | null {
    if (!this.storage) return null
    try {
      const raw = this.storage.getItem(LS_KEY)
      if (!raw) return null
      const parsed = JSON.parse(raw) as Partial<PersistedState>
      if (parsed && typeof parsed.tier === 'string' && typeof parsed.ts === 'number') {
        return {
          tier: parsed.tier as TierId,
          ewma: typeof parsed.ewma === 'number' ? parsed.ewma : 0,
          probedH3: !!parsed.probedH3,
          ts: parsed.ts,
        }
      }
    } catch {
      // malformed JSON / storage unavailable -> ignore, fall back to entry rule
    }
    return null
  }

  private persist(): void {
    if (!this.storage) return
    try {
      const state: PersistedState = {
        tier: this.tiers[this.tierIndex].id,
        ewma: this.measuredEwmaBps,
        probedH3: this.probedH3,
        ts: this.now(),
      }
      this.storage.setItem(LS_KEY, JSON.stringify(state))
    } catch {
      // storage unavailable / quota exceeded -> ignore, in-memory state still holds
    }
  }

  private resetTierCounters(): void {
    this.fragSamples = 0
    this.consecSlow = 0
    this.timeoutCount = 0
    this.hasCompletedFragOnTier = false
    this.inflightUrl = null
  }

  /**
   * Attempts a one-tier-down switch. Returns true when the switch actually
   * happened; false when blocked (cooldown still active, or already at the h1
   * floor). On success applySwitch() -> resetTierCounters() clears all
   * per-tier trigger state; on failure the caller's accumulated trigger state
   * (consecSlow / timeoutCount / inflight timing) is intentionally left
   * intact, so the next trigger evaluation retries promptly once the cooldown
   * expires instead of re-accumulating a full threshold's worth of signal.
   */
  private downshift(reason: string): boolean {
    const now = this.now()
    if (now - this.lastSwitchTs < SWITCH_COOLDOWN_MS) return false
    if (this.tierIndex >= this.tiers.length - 1) return false // already at the h1 floor
    this.applySwitch(this.tierIndex + 1, reason, now)
    return true
  }

  private applySwitch(newIndex: number, reason: string, now: number): void {
    const from = this.tiers[this.tierIndex]
    this.tierIndex = newIndex
    const to = this.tiers[newIndex]
    this.resetTierCounters()
    this.trail = `${from.id}→${to.id} (${reason})`
    this.lastSwitchTs = now
    this.persist()
    this.emit(to, reason)
  }

  private emit(tier: Tier, reason: string): void {
    for (const cb of this.listeners) {
      try {
        cb(tier, reason)
      } catch {
        // ignore listener errors -- one bad subscriber shouldn't break the ladder
      }
    }
  }

  private resetToEntryOnNetworkChange(): void {
    try {
      this.storage?.removeItem(LS_KEY)
    } catch {
      // ignore
    }
    const h2idx = this.tiers.findIndex((t) => t.id === 'h2')
    this.tierIndex = h2idx >= 0 ? h2idx : 0
    this.resetTierCounters()
    this.trail = ''
    this.lastProbe = ''
    this.probedH3 = false
    this.measuredEwmaBps = 0
    this.neededEwmaBps = 0
    this.lastProtocol = '?'
    this.lastSwitchTs = Number.NEGATIVE_INFINITY
  }

  private attachConnectionListener(): void {
    try {
      if (typeof navigator === 'undefined') return
      const conn = (navigator as Navigator & { connection?: ConnectionLike }).connection
      if (conn && typeof conn.addEventListener === 'function') {
        conn.addEventListener('change', () => this.resetToEntryOnNetworkChange())
      }
    } catch {
      // navigator/connection unsupported (jsdom, older browsers) -> ignore
    }
  }
}

export const ladder = new ProtocolLadder(
  parseTiers(import.meta.env.VITE_HLS_PROXY_TIERS, import.meta.env.VITE_HLS_PROXY_BASE),
)
