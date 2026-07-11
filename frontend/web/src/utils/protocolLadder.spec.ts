import { describe, it, expect, vi } from 'vitest'
import {
  parseTiers,
  ProtocolLadder,
  shouldDeferStallToLadder,
  formatLadderRows,
  ladder,
  LS_KEY,
  PERSIST_TTL_MS,
  TIMEOUTS_TO_DOWNSHIFT,
  SWITCH_COOLDOWN_MS,
  type Tier,
  type TierId,
  type LadderDebug,
} from './protocolLadder'

// Raw 3-tier config used across most ProtocolLadder tests: descending
// preference h3 (fastest, least stable) -> h2 -> h1 (slowest, most stable).
const RAW3 = 'h3=https://a,h2=https://b,h1=https://c'

function makeStorage(): Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> {
  const map = new Map<string, string>()
  return {
    getItem: (k: string) => (map.has(k) ? (map.get(k) as string) : null),
    setItem: (k: string, v: string) => {
      map.set(k, v)
    },
    removeItem: (k: string) => {
      map.delete(k)
    },
  }
}

describe('parseTiers()', () => {
  it('parses a full 3-tier string in h3,h2,h1 descending preference order', () => {
    const tiers = parseTiers(RAW3, undefined)
    expect(tiers).toEqual<Tier[]>([
      { id: 'h3', base: 'https://a' },
      { id: 'h2', base: 'https://b' },
      { id: 'h1', base: 'https://c' },
    ])
  })

  it('re-orders to h3,h2,h1 regardless of the input order', () => {
    const tiers = parseTiers('h1=https://c,h3=https://a,h2=https://b', undefined)
    expect(tiers.map((t) => t.id)).toEqual(['h3', 'h2', 'h1'])
  })

  it('falls back to a single h2 tier using fallbackBase when tiersRaw is unset', () => {
    expect(parseTiers(undefined, 'https://stream.example')).toEqual([
      { id: 'h2', base: 'https://stream.example' },
    ])
  })

  it('falls back to a relative same-origin h2 tier when both are unset', () => {
    expect(parseTiers(undefined, undefined)).toEqual([{ id: 'h2', base: '' }])
  })

  it('strips trailing slashes from tier bases and the fallback base', () => {
    expect(parseTiers('h2=https://b/', undefined)).toEqual([{ id: 'h2', base: 'https://b' }])
    expect(parseTiers(undefined, 'https://stream.example/')).toEqual([
      { id: 'h2', base: 'https://stream.example' },
    ])
  })

  it('skips malformed and unknown-key entries', () => {
    const tiers = parseTiers('h3=https://a,bogus,foo=https://z,h1=https://c', undefined)
    expect(tiers).toEqual([
      { id: 'h3', base: 'https://a' },
      { id: 'h1', base: 'https://c' },
    ])
  })
})

describe('ProtocolLadder — entry tier selection', () => {
  it('defaults to h2 when no persisted state exists', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 1_000_000,
      storage: makeStorage(),
    })
    expect(ladderInstance.currentBase()).toBe('https://b')
  })

  it('starts on a fresh (<24h) persisted tier instead of the h2 default', () => {
    const storage = makeStorage()
    const now = () => 1_000_000
    storage.setItem(
      LS_KEY,
      JSON.stringify({ tier: 'h1', ewma: 0, probedH3: false, ts: now() - 1_000 }),
    )
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), { now, storage })
    expect(ladderInstance.currentBase()).toBe('https://c')
  })

  it('ignores a stale (>24h) persisted tier and falls back to h2', () => {
    const storage = makeStorage()
    const now = () => 1_000_000
    storage.setItem(
      LS_KEY,
      JSON.stringify({ tier: 'h1', ewma: 0, probedH3: false, ts: now() - PERSIST_TTL_MS - 1_000 }),
    )
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), { now, storage })
    expect(ladderInstance.currentBase()).toBe('https://b')
  })
})

describe('ProtocolLadder — reportFragment EWMA downshift', () => {
  const BYTES = 4_000_000
  const DURATION_S = 6
  // measured = 32,000,000 bits / 16s = 2 Mbps; needed = 32,000,000 / 6s ≈
  // 5.33 Mbps; needed*SAFETY_FACTOR(1.2) ≈ 6.4 Mbps — clearly "slow".
  const SLOW_MS = 16_000

  it('downshifts h2→h1 only after the EWMA gate + 3 consecutive slow evaluations', () => {
    const events: Array<{ tier: TierId; reason: string }> = []
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    ladderInstance.onChange((tier, reason) => events.push({ tier: tier.id, reason }))

    // Call 1 only seeds the EWMA ("require >=2 samples first") — no
    // slow/fast evaluation happens yet.
    ladderInstance.reportFragment({ bytes: BYTES, ms: SLOW_MS, mediaDurationS: DURATION_S, protocol: 'h2' })
    expect(ladderInstance.currentBase()).toBe('https://b')

    // Calls 2 and 3 evaluate as slow (consecSlow -> 1, 2) but don't yet hit
    // CONSEC_SLOW_FRAGS.
    ladderInstance.reportFragment({ bytes: BYTES, ms: SLOW_MS, mediaDurationS: DURATION_S, protocol: 'h2' })
    ladderInstance.reportFragment({ bytes: BYTES, ms: SLOW_MS, mediaDurationS: DURATION_S, protocol: 'h2' })
    expect(ladderInstance.currentBase()).toBe('https://b')
    expect(events).toHaveLength(0)

    // Call 4 is the 3rd consecutive slow evaluation -> downshift.
    ladderInstance.reportFragment({ bytes: BYTES, ms: SLOW_MS, mediaDurationS: DURATION_S, protocol: 'h2' })
    expect(ladderInstance.currentBase()).toBe('https://c') // h1
    expect(events).toHaveLength(1)
    expect(events[0].tier).toBe('h1')
    expect(events[0].reason).toContain('ewma')

    const snap = ladderInstance.debugSnapshot()
    expect(snap?.tierId).toBe('h1')
    expect(snap?.tierIndex).toBe(2)
    expect(snap?.tierCount).toBe(3)
    expect(snap?.protocol).toBe('h2')
    expect(snap?.trail).toContain('h2→h1')
    expect(snap?.measuredMbps).toBeCloseTo(2, 5)
  })

  it('never downshifts on a steady stream of fast fragments', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    for (let i = 0; i < 10; i++) {
      // 32,000,000 bits / 2s = 16 Mbps — well above the ~6.4 Mbps threshold.
      ladderInstance.reportFragment({ bytes: BYTES, ms: 2_000, mediaDurationS: DURATION_S })
    }
    expect(ladderInstance.currentBase()).toBe('https://b')
  })

  it('resets the consecutive-slow counter after one fast fragment between slows', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })

    ladderInstance.reportFragment({ bytes: BYTES, ms: SLOW_MS, mediaDurationS: DURATION_S }) // seed
    ladderInstance.reportFragment({ bytes: BYTES, ms: SLOW_MS, mediaDurationS: DURATION_S }) // consecSlow=1
    ladderInstance.reportFragment({ bytes: BYTES, ms: SLOW_MS, mediaDurationS: DURATION_S }) // consecSlow=2

    // One big fast fragment (~21.3 Mbps) swings the smoothed EWMA
    // (alpha=0.3) back above the need*SAFETY_FACTOR threshold in a single
    // hop: 0.3*21.3M + 0.7*2M ≈ 7.8 Mbps > 6.4 Mbps -> resets consecSlow.
    // See task-2-report.md for the full derivation.
    ladderInstance.reportFragment({ bytes: BYTES, ms: 1_500, mediaDurationS: DURATION_S })
    expect(ladderInstance.currentBase()).toBe('https://b') // no downshift yet

    // Needs 3 fresh consecutive slow evaluations post-reset to downshift.
    ladderInstance.reportFragment({ bytes: BYTES, ms: SLOW_MS, mediaDurationS: DURATION_S }) // consecSlow=1
    expect(ladderInstance.currentBase()).toBe('https://b')
    ladderInstance.reportFragment({ bytes: BYTES, ms: SLOW_MS, mediaDurationS: DURATION_S }) // consecSlow=2
    expect(ladderInstance.currentBase()).toBe('https://b')
    ladderInstance.reportFragment({ bytes: BYTES, ms: SLOW_MS, mediaDurationS: DURATION_S }) // consecSlow=3 -> downshift
    expect(ladderInstance.currentBase()).toBe('https://c')
  })
})

describe('ProtocolLadder — reportTimeout', () => {
  it('does not downshift after a single timeout', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    ladderInstance.reportTimeout()
    expect(ladderInstance.currentBase()).toBe('https://b')
  })

  it('downshifts after TIMEOUTS_TO_DOWNSHIFT consecutive timeouts', () => {
    const events: string[] = []
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    ladderInstance.onChange((_tier, reason) => events.push(reason))
    for (let i = 0; i < TIMEOUTS_TO_DOWNSHIFT; i++) ladderInstance.reportTimeout()
    expect(ladderInstance.currentBase()).toBe('https://c') // h1
    expect(events).toHaveLength(1)
    expect(events[0]).toContain('timeouts')
  })
})

describe('ProtocolLadder — first-fragment stall projection', () => {
  it('downshifts when the first fragment projects well beyond FIRSTFRAG_PROJECTED_MS', () => {
    let t = 0
    const now = () => t
    const events: string[] = []
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now,
      storage: makeStorage(),
    })
    ladderInstance.onChange((_tier, reason) => events.push(reason))

    ladderInstance.onXhrOpen('frag-1')
    t = 4_000
    // projected = 4000ms * (4,700,000/1,000,000) ≈ 18,800ms > 8000ms
    ladderInstance.onXhrProgress('frag-1', 1_000_000, 4_700_000)

    expect(ladderInstance.currentBase()).toBe('https://c') // h1
    expect(events).toHaveLength(1)
    expect(events[0]).toContain('first-frag')
  })

  it('does not downshift before FIRSTFRAG_MIN_ELAPSED_MS has passed', () => {
    let t = 0
    const now = () => t
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now,
      storage: makeStorage(),
    })
    ladderInstance.onXhrOpen('frag-1')
    t = 2_500
    ladderInstance.onXhrProgress('frag-1', 1_000_000, 4_700_000)
    expect(ladderInstance.currentBase()).toBe('https://b')
  })
})

describe('ProtocolLadder — switch cooldown', () => {
  it('only allows one downshift within SWITCH_COOLDOWN_MS', () => {
    let t = 0
    const now = () => t
    const events: Array<{ tier: TierId; reason: string }> = []
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now,
      storage: makeStorage(),
    })
    ladderInstance.onChange((tier, reason) => events.push({ tier: tier.id, reason }))

    ladderInstance.switchTo('h3', 'setup') // entry h2 -> h3; unblocked (no prior switch)
    expect(ladderInstance.currentBase()).toBe('https://a')

    t = 100_000 // safely clear of the setup switch's cooldown
    for (let i = 0; i < TIMEOUTS_TO_DOWNSHIFT; i++) ladderInstance.reportTimeout()
    expect(ladderInstance.currentBase()).toBe('https://b') // h3 -> h2

    t += SWITCH_COOLDOWN_MS / 2 // well within the 30s cooldown of the last switch
    for (let i = 0; i < TIMEOUTS_TO_DOWNSHIFT; i++) ladderInstance.reportTimeout()
    expect(ladderInstance.currentBase()).toBe('https://b') // blocked by cooldown

    expect(events).toHaveLength(2) // the setup switch + the one allowed downshift
  })
})

describe('ProtocolLadder — cooldown-blocked triggers keep their accumulated state', () => {
  // Regression guard (review finding): a degradation signal that trips while
  // the switch cooldown is active must NOT be discarded — once the cooldown
  // expires, a SINGLE additional trigger evaluation must fire the switch
  // promptly, without re-accumulating the whole threshold from zero.

  it('timeout trigger blocked by cooldown fires on the very next timeout after cooldown', () => {
    let t = 0
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => t,
      storage: makeStorage(),
    })
    ladderInstance.switchTo('h3', 'setup')

    t = 100_000 // clear of the setup switch's cooldown
    for (let i = 0; i < TIMEOUTS_TO_DOWNSHIFT; i++) ladderInstance.reportTimeout()
    expect(ladderInstance.currentBase()).toBe('https://b') // h3 -> h2 at t=100_000

    t = 110_000 // inside the 30s cooldown
    for (let i = 0; i < TIMEOUTS_TO_DOWNSHIFT; i++) ladderInstance.reportTimeout()
    expect(ladderInstance.currentBase()).toBe('https://b') // trigger tripped, switch blocked

    t = 100_000 + SWITCH_COOLDOWN_MS + 1_000
    ladderInstance.reportTimeout() // ONE more — accumulated count must still be armed
    expect(ladderInstance.currentBase()).toBe('https://c')
  })

  it('slow-EWMA trigger blocked by cooldown fires on the very next slow fragment after cooldown', () => {
    let t = 0
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => t,
      storage: makeStorage(),
    })
    ladderInstance.switchTo('h3', 'setup')
    const slow = { bytes: 4_000_000, ms: 16_000, mediaDurationS: 6 }

    t = 100_000
    for (let i = 0; i < 4; i++) ladderInstance.reportFragment(slow) // seed + 3 slow evals
    expect(ladderInstance.currentBase()).toBe('https://b') // h3 -> h2 at t=100_000

    t = 110_000 // inside cooldown
    for (let i = 0; i < 4; i++) ladderInstance.reportFragment(slow) // re-seed + 3 slow evals
    expect(ladderInstance.currentBase()).toBe('https://b') // trigger tripped, switch blocked

    t = 100_000 + SWITCH_COOLDOWN_MS + 1_000
    ladderInstance.reportFragment(slow) // ONE more slow evaluation
    expect(ladderInstance.currentBase()).toBe('https://c')
  })

  it('first-frag projection blocked by cooldown retries and fires after cooldown', () => {
    let t = 0
    const events: string[] = []
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => t,
      storage: makeStorage(),
    })
    ladderInstance.switchTo('h3', 'setup')

    t = 100_000
    ladderInstance.onXhrOpen('f1')
    t = 104_000
    ladderInstance.onXhrProgress('f1', 1_000_000, 4_700_000) // projected 18.8s
    expect(ladderInstance.currentBase()).toBe('https://b') // h3 -> h2 at t=104_000

    ladderInstance.onChange((_tier, reason) => events.push(reason))
    t = 105_000
    ladderInstance.onXhrOpen('f2')
    t = 110_000
    // elapsed 5s, projected 23.5s — trigger trips, but the switch is blocked
    // (only 6s since the last switch).
    ladderInstance.onXhrProgress('f2', 1_000_000, 4_700_000)
    expect(ladderInstance.currentBase()).toBe('https://b')

    t = 104_000 + SWITCH_COOLDOWN_MS + 1_000
    // A single later progress re-evaluation must fire (the fired-once flag
    // must not have latched on the blocked attempt).
    ladderInstance.onXhrProgress('f2', 1_200_000, 4_700_000)
    expect(ladderInstance.currentBase()).toBe('https://c')
    expect(events).toHaveLength(1)
    expect(events[0]).toContain('first-frag')
  })
})

describe('ProtocolLadder — network-change reset', () => {
  it('clears persisted state and resets to the entry tier on connection change', () => {
    let saved: (() => void) | undefined
    vi.stubGlobal('navigator', {
      connection: {
        addEventListener: (_type: string, cb: () => void) => {
          saved = cb
        },
      },
    })
    try {
      const storage = makeStorage()
      const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
        now: () => 0,
        storage,
      })
      expect(saved).toBeTypeOf('function')

      for (let i = 0; i < TIMEOUTS_TO_DOWNSHIFT; i++) ladderInstance.reportTimeout() // h2 -> h1
      expect(ladderInstance.currentBase()).toBe('https://c')
      expect(storage.getItem(LS_KEY)).toBeTruthy()

      saved?.() // simulate navigator.connection 'change'

      expect(storage.getItem(LS_KEY)).toBeNull() // persisted state invalidated
      expect(ladderInstance.currentBase()).toBe('https://b') // back at the h2 entry tier
    } finally {
      vi.unstubAllGlobals()
    }
  })
})

describe('ProtocolLadder — persistence', () => {
  it('persists the tier after a downshift and a new ladder resumes there', () => {
    const storage = makeStorage()
    const ladder1 = new ProtocolLadder(parseTiers(RAW3, undefined), { now: () => 0, storage })
    for (let i = 0; i < TIMEOUTS_TO_DOWNSHIFT; i++) ladder1.reportTimeout() // h2 -> h1
    expect(ladder1.currentBase()).toBe('https://c')

    const raw = storage.getItem(LS_KEY)
    expect(raw).toBeTruthy()
    expect(JSON.parse(raw as string).tier).toBe('h1')

    const ladder2 = new ProtocolLadder(parseTiers(RAW3, undefined), { now: () => 1_000, storage })
    expect(ladder2.currentBase()).toBe('https://c')
  })
})

describe('ProtocolLadder — probe upshift', () => {
  it('records probe measurements and switches up to h3 on accept', () => {
    const storage = makeStorage()
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), { now: () => 0, storage })
    expect(ladderInstance.debugSnapshot()?.probe).toBe('') // nothing probed yet

    ladderInstance.recordProbe('h3', 2.1, false, '<1.1× h2')
    const rejectedProbe = ladderInstance.debugSnapshot()?.probe ?? ''
    expect(rejectedProbe).toContain('2.1')
    expect(rejectedProbe).toContain('rejected')

    ladderInstance.recordProbe('h3', 6.5, true, '≥1.1× h2')
    const acceptedProbe = ladderInstance.debugSnapshot()?.probe ?? ''
    expect(acceptedProbe).toContain('accepted')

    ladderInstance.switchTo('h3', 'probe')
    expect(ladderInstance.currentBase()).toBe('https://a')

    const persisted = JSON.parse(storage.getItem(LS_KEY) as string)
    expect(persisted.tier).toBe('h3')
  })

  it('labels the probe with the tier actually passed in, not "one above current" (M5)', () => {
    // Regression: the ladder is parked on h1 (the floor) here, so the old
    // "aboveIdx = tierIndex - 1" derivation would have mislabeled this h3
    // probe measurement as an "h2" probe.
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    for (let i = 0; i < TIMEOUTS_TO_DOWNSHIFT; i++) ladderInstance.reportTimeout() // h2 -> h1
    expect(ladderInstance.currentBase()).toBe('https://c') // confirm parked on h1

    ladderInstance.recordProbe('h3', 9.9, false, '<1.1× h1')
    const probe = ladderInstance.debugSnapshot()?.probe ?? ''
    expect(probe.startsWith('h3 ')).toBe(true)
  })
})

describe('ProtocolLadder — Task 5 probe accessors', () => {
  it('tierBase() returns a configured tier base, or null when the tier is absent', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    expect(ladderInstance.tierBase('h3')).toBe('https://a')
    expect(ladderInstance.tierBase('h2')).toBe('https://b')
    expect(ladderInstance.tierBase('h1')).toBe('https://c')

    const h2Only = new ProtocolLadder(parseTiers(undefined, 'https://stream.example'), {
      now: () => 0,
      storage: makeStorage(),
    })
    expect(h2Only.tierBase('h3')).toBeNull()
  })

  it('currentEwmaMbps() reflects the measured-throughput EWMA in Mbps', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    expect(ladderInstance.currentEwmaMbps()).toBe(0)

    // 32,000,000 bits / 2s = 16 Mbps.
    ladderInstance.reportFragment({ bytes: 4_000_000, ms: 2_000, mediaDurationS: 6 })
    expect(ladderInstance.currentEwmaMbps()).toBeCloseTo(16, 5)
  })

  it('hasProbedH3() flips true after the first recordProbe() call, false before', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    expect(ladderInstance.hasProbedH3()).toBe(false)
    ladderInstance.recordProbe('h3', 2.1, false, '<1.1× h2')
    expect(ladderInstance.hasProbedH3()).toBe(true)
  })

  it('currentTierId() reflects the currently active tier id', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    expect(ladderInstance.currentTierId()).toBe('h2') // default entry tier
    for (let i = 0; i < TIMEOUTS_TO_DOWNSHIFT; i++) ladderInstance.reportTimeout() // h2 -> h1
    expect(ladderInstance.currentTierId()).toBe('h1')
  })
})

describe('ProtocolLadder — onXhrLoadEnd / clearInflight (C1)', () => {
  it('a completed playlist-XHR (no reportFragment ever fires) clears its own inflight slot on loadend', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    ladderInstance.onXhrOpen('https://b/master.m3u8')
    ladderInstance.onXhrProgress('https://b/master.m3u8', 500, 500) // completed, bytes>0

    // Before the fix nothing ever clears this — inflight() stays non-null
    // forever and shouldDeferStallToLadder defers the watchdog indefinitely.
    expect(ladderInstance.inflight()).not.toBeNull()

    ladderInstance.onXhrLoadEnd('https://b/master.m3u8')

    expect(ladderInstance.inflight()).toBeNull()
    expect(shouldDeferStallToLadder(ladderInstance.inflight())).toBe(false)
  })

  it('is a no-op when the url does not match the tracked inflight slot', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    ladderInstance.onXhrOpen('frag-current')
    ladderInstance.onXhrProgress('frag-current', 10, 100)

    ladderInstance.onXhrLoadEnd('frag-stale') // a different (already-superseded) XHR

    expect(ladderInstance.inflight()?.url).toBe('frag-current') // untouched
  })

  it('is a no-op on a single-tier (no-op) ladder', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(undefined, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    expect(() => ladderInstance.onXhrLoadEnd('u')).not.toThrow()
    expect(ladderInstance.inflight()).toBeNull()
  })

  it('clearInflight() unconditionally clears the slot regardless of url', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    ladderInstance.onXhrOpen('https://a/seg-001.ts')
    ladderInstance.onXhrProgress('https://a/seg-001.ts', 10, 100)
    expect(ladderInstance.inflight()).not.toBeNull()

    ladderInstance.clearInflight()

    expect(ladderInstance.inflight()).toBeNull()
  })
})

describe('ProtocolLadder — reportFragment ignores non-positive samples (I3)', () => {
  it('a ms=0 sample does not poison the EWMA to Infinity; a later valid sample stays finite', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })

    // Degenerate sample: bytes flowed but ms=0 — dividing by (ms/1000) would
    // produce Infinity and poison every EWMA update from here on.
    ladderInstance.reportFragment({ bytes: 1_000_000, ms: 0, mediaDurationS: 6, protocol: 'h2' })
    expect(ladderInstance.currentEwmaMbps()).toBe(0) // guard skipped it entirely — no seed happened

    // A normal, slow-but-valid sample.
    ladderInstance.reportFragment({ bytes: 4_000_000, ms: 16_000, mediaDurationS: 6, protocol: 'h2' })

    const snap = ladderInstance.debugSnapshot()
    expect(Number.isFinite(ladderInstance.currentEwmaMbps())).toBe(true)
    expect(Number.isFinite(snap?.measuredMbps ?? Infinity)).toBe(true)
  })

  it('ignores non-positive bytes and non-positive mediaDurationS the same way', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 0,
      storage: makeStorage(),
    })
    ladderInstance.reportFragment({ bytes: 0, ms: 100, mediaDurationS: 6 })
    ladderInstance.reportFragment({ bytes: 1_000, ms: 100, mediaDurationS: 0 })
    expect(ladderInstance.currentEwmaMbps()).toBe(0)
  })
})

describe('formatLadderRows() (I4)', () => {
  it('formats the tier display as 1-based ("tier 2/3" for h2 of [h3,h2,h1])', () => {
    const snap: LadderDebug = {
      tierId: 'h2',
      tierIndex: 1,
      tierCount: 3,
      protocol: 'h2',
      measuredMbps: 5.678,
      neededMbps: 3.21,
      trail: 'h3→h2 (first-frag projected 17s)',
      probe: 'h3 2.1 Mbps @03:24 — rejected (<1.1× h2)',
    }
    const rows = formatLadderRows(snap)
    expect(rows.proto).toBe('h2 · tier 2/3')
    expect(rows.net).toBe('5.7 Mbps ewma / need 3.2 ×1.2')
    expect(rows.laddr).toBe('h3→h2 (first-frag projected 17s)')
    expect(rows.probe).toBe('h3 2.1 Mbps @03:24 — rejected (<1.1× h2)')
  })

  it('formats the entry tier (h2, index 0 of a 1-tier or leading position) as tier 1/N', () => {
    const snap: LadderDebug = {
      tierId: 'h3',
      tierIndex: 0,
      tierCount: 3,
      protocol: '?',
      measuredMbps: 0,
      neededMbps: 0,
      trail: '',
      probe: '',
    }
    expect(formatLadderRows(snap).proto).toBe('? · tier 1/3')
  })
})

describe('ProtocolLadder — single-tier no-op', () => {
  it('treats every report as a no-op and exposes no debug info', () => {
    const storage = makeStorage()
    const ladderInstance = new ProtocolLadder(parseTiers(undefined, undefined), {
      now: () => 0,
      storage,
    })
    expect(ladderInstance.isMultiTier()).toBe(false)
    expect(ladderInstance.debugSnapshot()).toBeNull()

    ladderInstance.reportFragment({ bytes: 1, ms: 1, mediaDurationS: 1 })
    ladderInstance.reportTimeout()
    ladderInstance.onXhrOpen('u')
    ladderInstance.onXhrProgress('u', 1, 100)
    ladderInstance.onXhrLoadEnd('u')
    ladderInstance.recordProbe('h2', 1, true, 'n/a')
    ladderInstance.switchTo('h1', 'noop')

    expect(ladderInstance.currentBase()).toBe('')
    expect(ladderInstance.inflight()).toBeNull()
    expect(storage.getItem(LS_KEY)).toBeNull()
  })
})

describe('shouldDeferStallToLadder()', () => {
  it('defers when bytes are flowing (slow, not dead)', () => {
    expect(
      shouldDeferStallToLadder({
        url: 'u',
        receivedBytes: 800_000,
        totalBytes: 4_700_000,
        elapsedMs: 12_000,
      }),
    ).toBe(true)
  })

  it('does not defer when zero bytes have arrived', () => {
    expect(
      shouldDeferStallToLadder({ url: 'u', receivedBytes: 0, totalBytes: 4_700_000, elapsedMs: 12_000 }),
    ).toBe(false)
  })

  it('does not defer when there is no inflight fragment', () => {
    expect(shouldDeferStallToLadder(null)).toBe(false)
  })
})

describe('ladder singleton', () => {
  it('constructs without throwing under jsdom / import.meta.env', () => {
    expect(ladder).toBeDefined()
    expect(typeof ladder.currentBase()).toBe('string')
  })
})
