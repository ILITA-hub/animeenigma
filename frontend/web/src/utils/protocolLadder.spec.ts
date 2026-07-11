import { describe, it, expect } from 'vitest'
import {
  parseTiers,
  ProtocolLadder,
  shouldDeferStallToLadder,
  ladder,
  LS_KEY,
  PERSIST_TTL_MS,
  TIMEOUTS_TO_DOWNSHIFT,
  SWITCH_COOLDOWN_MS,
  type Tier,
  type TierId,
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

    ladderInstance.recordProbe(2.1, false, '<1.1× h2')
    const rejectedProbe = ladderInstance.debugSnapshot()?.probe ?? ''
    expect(rejectedProbe).toContain('2.1')
    expect(rejectedProbe).toContain('rejected')

    ladderInstance.recordProbe(6.5, true, '≥1.1× h2')
    const acceptedProbe = ladderInstance.debugSnapshot()?.probe ?? ''
    expect(acceptedProbe).toContain('accepted')

    ladderInstance.switchTo('h3', 'probe')
    expect(ladderInstance.currentBase()).toBe('https://a')

    const persisted = JSON.parse(storage.getItem(LS_KEY) as string)
    expect(persisted.tier).toBe('h3')
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
    ladderInstance.recordProbe(1, true, 'n/a')
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
