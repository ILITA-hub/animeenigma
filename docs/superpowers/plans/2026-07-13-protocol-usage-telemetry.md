# Protocol (h1/h2/h3) Usage Telemetry + Grafana Panels — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Emit a per-(session × protocol-tier) usage event from aePlayer into the existing analytics pipeline and surface h1/h2/h3 usage on the `playback-health` Grafana dashboard as a detail table + a usage-share chart.

**Architecture:** The streamX protocol ladder already tracks per-tier throughput/segments but resets on every switch. Extend it to hand off a *residency summary* just before each switch (and on session end). aePlayer combines that summary with a dropped-video-frames delta and emits a `protocol_usage` telemetry event through the existing `recordPlayerEvent` → `/api/analytics/player-events` → analytics → ClickHouse `events` path (new `effect_kind='player_protocol'`, metrics in the `properties` JSON — zero schema change). Grafana reads `events` via the existing `aenigma-clickhouse` datasource.

**Tech Stack:** Vue 3 + TypeScript (frontend), Go (analytics service), ClickHouse, Grafana 10.3.3 (provisioned dashboards).

Design spec: [`docs/superpowers/specs/2026-07-13-protocol-usage-telemetry-design.md`](../specs/2026-07-13-protocol-usage-telemetry-design.md).

## Global Constraints

- **Anonymous** — no `user_id`/username/session-owner in the event. The only session correlator is a client-side ephemeral `sess` id (non-identifying). Do not add auth to the analytics route.
- **Reuse `events`** — new `effect_kind='player_protocol'`; metrics packed into the existing `properties` String column. No new table, migration, endpoint, or wire struct.
- **Provider must be whitelisted** — the analytics handler drops events whose `provider` isn't in `playertelemetry_whitelist.go`. `protocol_usage` carries the real combo provider (gogoanime, ae, kodik, …), which is whitelisted. Never send `provider:''`.
- **No user-facing strings** — telemetry + an admin-only dashboard. **No i18n keys** are added (skips the en/ru/ja parity gate).
- **AePlayer edit is logic-only** — no colors/styles/tokens, so the DS-lint PostToolUse gate passes untouched.
- **Go formatting by hand** — write gofmt-correct code; do NOT run `gofmt -w` / `make fmt` (smart-quote landmine).
- **Commit co-authors on every commit:**
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **ClickHouse must be an active analytics sink** — the dashboard reads ClickHouse. The panels only show data when `ANALYTICS_STORE_BACKEND` is `clickhouse` or `dualwrite`. The existing `player_resolve` CH panels already render, confirming it is; re-verify in Task 7.

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `frontend/web/src/utils/protocolLadder.ts` | Per-tier residency accounting + `onResidencyEnd`/`consumeResidency` + `TierResidency` type | 1 |
| `frontend/web/src/utils/protocolLadder.spec.ts` | Residency unit tests | 1 |
| `frontend/web/src/utils/playerTelemetry.ts` | Accept the new `'protocol_usage'` kind | 2 |
| `frontend/web/src/utils/playerTelemetry.spec.ts` | Kind-acceptance test | 2 |
| `frontend/web/src/composables/aePlayer/protocolUsage.ts` | Pure `detail`-payload builder + dropped-frame math (testable, keeps AePlayer glue thin) | 3 |
| `frontend/web/src/composables/aePlayer/protocolUsage.spec.ts` | Builder + math unit tests | 3 |
| `frontend/web/src/components/player/aePlayer/AePlayer.vue` | Wire the ladder→event emission + session-end flush | 4 |
| `services/analytics/internal/handler/playertelemetry.go` | Map `protocol_usage` → `effect_kind='player_protocol'` | 5 |
| `services/analytics/internal/handler/playertelemetry_test.go` | Handler test for the new kind | 5 |
| `docker/grafana/dashboards/playback-health.json` | New row + usage-share pie + detail table | 6 |

---

## Task 1: Protocol-ladder residency accounting

**Files:**
- Modify: `frontend/web/src/utils/protocolLadder.ts`
- Test: `frontend/web/src/utils/protocolLadder.spec.ts`

**Interfaces:**
- Produces:
  - `export interface TierResidency { tierId: TierId; protocol: string; segments: number; avgMbps: number; neededMbps: number; timeouts: number; tierMs: number; trail: string; probe: string }`
  - `ProtocolLadder.onResidencyEnd(cb: (r: TierResidency) => void): () => void`
  - `ProtocolLadder.consumeResidency(): TierResidency | null`

- [ ] **Step 1: Write the failing tests**

Append to `frontend/web/src/utils/protocolLadder.spec.ts` (uses the existing `RAW3`, `makeStorage`, `parseTiers`, `ProtocolLadder` in that file; add `type TierResidency` to the existing import from `./protocolLadder`):

```ts
describe('ProtocolLadder — residency accounting', () => {
  // A "slow" fragment: measured 8 Mbps, needed 8 Mbps → 8 < 8*1.2, counts as slow.
  const SLOW = { bytes: 1_000_000, ms: 1000, mediaDurationS: 1, protocol: 'h2' }
  // A "fast" fragment: measured 8 Mbps, needed 2 Mbps → not slow, no downshift.
  const FAST = { bytes: 500_000, ms: 500, mediaDurationS: 2, protocol: 'h2' }

  it('emits a tier residency summary just before a downshift', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 1_000_000,
      storage: makeStorage(),
    })
    const seen: TierResidency[] = []
    ladderInstance.onResidencyEnd((r) => seen.push(r))
    // entry tier is h2 (index 1); 3 consecutive slow judgements → downshift to h1
    ladderInstance.reportFragment(SLOW) // sample 1 (no judge)
    ladderInstance.reportFragment(SLOW) // sample 2 → consecSlow 1
    ladderInstance.reportFragment(SLOW) // sample 3 → consecSlow 2
    ladderInstance.reportFragment(SLOW) // sample 4 → consecSlow 3 → downshift
    expect(seen).toHaveLength(1)
    expect(seen[0].tierId).toBe('h2') // the tier being LEFT
    expect(seen[0].segments).toBe(4)
    expect(seen[0].protocol).toBe('h2')
    expect(seen[0].timeouts).toBe(0)
    expect(seen[0].avgMbps).toBeCloseTo(8, 1)
  })

  it('consumeResidency returns the summary once, then null until new fragments', () => {
    const ladderInstance = new ProtocolLadder(parseTiers(RAW3, undefined), {
      now: () => 1_000_000,
      storage: makeStorage(),
    })
    ladderInstance.reportFragment(FAST)
    ladderInstance.reportFragment(FAST)
    expect(ladderInstance.consumeResidency()?.segments).toBe(2)
    expect(ladderInstance.consumeResidency()).toBeNull() // already consumed
    ladderInstance.reportFragment(FAST) // new activity re-arms
    expect(ladderInstance.consumeResidency()?.segments).toBe(3)
  })

  it('does not track residency on a single-tier ladder', () => {
    const single = new ProtocolLadder([{ id: 'h2', base: '' }], {
      now: () => 1,
      storage: makeStorage(),
    })
    const seen: TierResidency[] = []
    single.onResidencyEnd((r) => seen.push(r))
    single.reportFragment(FAST) // reportFragment early-returns when !isMultiTier
    expect(single.consumeResidency()).toBeNull()
    expect(seen).toHaveLength(0)
  })
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/utils/protocolLadder.spec.ts`
Expected: FAIL — `onResidencyEnd`/`consumeResidency` do not exist; `TierResidency` import unresolved.

- [ ] **Step 3: Add the `TierResidency` type**

In `frontend/web/src/utils/protocolLadder.ts`, immediately after the `LadderDebug` interface (ends at line 48), add:

```ts
/** Summary of one continuous stay on a tier ("residency"). Emitted just
 *  before each tier switch (onResidencyEnd) and on session end
 *  (consumeResidency). All fields are tier-scoped except neededMbps (the
 *  running required-bitrate EWMA, which is content- not tier-dependent). */
export interface TierResidency {
  tierId: TierId
  protocol: string
  segments: number
  avgMbps: number
  neededMbps: number
  timeouts: number
  tierMs: number
  trail: string
  probe: string
}
```

- [ ] **Step 4: Add residency state fields**

In the `ProtocolLadder` class, after the `private probedH3 = false` line (line 182), add:

```ts

  // Per-tier residency accounting. A "residency" = one continuous stay on a
  // tier; summarized + emitted just before a switch, flushed on session end.
  // Reset on every switch so each summary is tier-scoped (unlike the EWMA).
  private residencyBytes = 0
  private residencyMs = 0
  private residencyStartTs = 0
  private residencyConsumed = false
  private readonly residencyListeners = new Set<(r: TierResidency) => void>()
```

- [ ] **Step 5: Seed the residency start clock in the constructor**

In the constructor, after `this.tierIndex = this.computeEntryIndex()` (line 188), add:

```ts
    this.residencyStartTs = this.now()
```

- [ ] **Step 6: Accumulate bytes/ms per fragment**

In `reportFragment`, immediately after the degenerate-sample guard
`if (!(r.bytes > 0) || !(r.ms > 0) || !(r.mediaDurationS > 0)) return` (line 260), add:

```ts

    // Tier-scoped residency accumulation (a true mean speed, distinct from the
    // EWMA below). A fresh fragment also re-arms consumeResidency.
    this.residencyBytes += r.bytes
    this.residencyMs += r.ms
    this.residencyConsumed = false
```

- [ ] **Step 7: Add the residency build/emit/subscribe/consume methods**

In `frontend/web/src/utils/protocolLadder.ts`, immediately before `debugSnapshot()` (line 408), add:

```ts
  /** Summary of the current tier residency, or null if nothing played on it. */
  private buildResidency(): TierResidency | null {
    if (this.fragSamples === 0) return null
    const avgMbps =
      this.residencyMs > 0
        ? (this.residencyBytes * 8) / (this.residencyMs / 1000) / 1_000_000
        : 0
    return {
      tierId: this.tiers[this.tierIndex].id,
      protocol: this.lastProtocol,
      segments: this.fragSamples,
      avgMbps,
      neededMbps: this.neededEwmaBps / 1_000_000,
      timeouts: this.timeoutCount,
      tierMs: this.now() - this.residencyStartTs,
      trail: this.trail,
      probe: this.lastProbe,
    }
  }

  private emitResidency(r: TierResidency): void {
    for (const cb of this.residencyListeners) {
      try {
        cb(r)
      } catch {
        // one bad subscriber must not break the ladder
      }
    }
  }

  /** Subscribe to per-tier residency summaries, emitted just before each tier
   *  switch (down- or up-shift). Returns an unsubscribe fn. */
  onResidencyEnd(cb: (r: TierResidency) => void): () => void {
    this.residencyListeners.add(cb)
    return () => {
      this.residencyListeners.delete(cb)
    }
  }

  /** Session-end flush: returns the current residency summary (or null) and
   *  marks it consumed, so a second call (pagehide THEN unmount) is a no-op
   *  until new fragments arrive. */
  consumeResidency(): TierResidency | null {
    if (this.residencyConsumed) return null
    const r = this.buildResidency()
    this.residencyConsumed = true
    return r
  }

```

- [ ] **Step 8: Reset residency counters on every tier switch**

In `resetTierCounters()` (lines 471–477), after `this.inflightUrl = null`, add:

```ts
    this.residencyBytes = 0
    this.residencyMs = 0
    this.residencyStartTs = this.now()
    this.residencyConsumed = false
```

- [ ] **Step 9: Emit the leaving tier's residency in `applySwitch`**

In `applySwitch` (lines 496–505), after `const from = this.tiers[this.tierIndex]`, add the residency emit BEFORE the index changes:

```ts
    const leaving = this.buildResidency()
    if (leaving) this.emitResidency(leaving)
```

Result:

```ts
  private applySwitch(newIndex: number, reason: string, now: number): void {
    const from = this.tiers[this.tierIndex]
    const leaving = this.buildResidency()
    if (leaving) this.emitResidency(leaving)
    this.tierIndex = newIndex
    const to = this.tiers[newIndex]
    this.resetTierCounters()
    this.trail = `${from.id}→${to.id} (${reason})`
    this.lastSwitchTs = now
    this.persist()
    this.emit(to, reason)
  }
```

(`resetTierCounters()` sets `residencyStartTs = this.now()` for the new tier. Network-change resets go through `resetTierCounters()` too, so residency state clears there as well; a residency is intentionally NOT emitted on network change — the session-end flush covers the tail.)

- [ ] **Step 10: Run the tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/utils/protocolLadder.spec.ts`
Expected: PASS (all existing ladder tests + the 3 new residency tests).

- [ ] **Step 11: Commit**

```bash
git add frontend/web/src/utils/protocolLadder.ts frontend/web/src/utils/protocolLadder.spec.ts
git commit -m "feat(player): protocol-ladder per-tier residency accounting

onResidencyEnd emits a TierResidency summary (segments, avg Mbps, timeouts,
protocol, trail, probe) just before each switch; consumeResidency flushes the
current tier once on session end.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Accept the `protocol_usage` telemetry kind

**Files:**
- Modify: `frontend/web/src/utils/playerTelemetry.ts:17,117-123`
- Test: `frontend/web/src/utils/playerTelemetry.spec.ts`

**Interfaces:**
- Consumes: `recordPlayerEvent(e: PlayerEvent)`, `flushPlayerTelemetry`, `__resetPlayerTelemetryForTest` (existing exports).
- Produces: `PlayerEvent['kind']` now includes `'protocol_usage'`.

- [ ] **Step 1: Write the failing test**

Append to `frontend/web/src/utils/playerTelemetry.spec.ts`:

```ts
describe('recordPlayerEvent — protocol_usage kind', () => {
  it('accepts and buffers a protocol_usage event', () => {
    __resetPlayerTelemetryForTest()
    const beacon = vi.fn().mockReturnValue(true)
    vi.stubGlobal('navigator', { sendBeacon: beacon })

    recordPlayerEvent({
      kind: 'protocol_usage',
      provider: 'gogoanime',
      anime_id: 'a1',
      episode: 1,
      audio: 'sub',
      lang: 'en',
      detail: { protocol: 'h2', tier: 'h2', segments: 214 },
    })
    flushPlayerTelemetry('test')

    expect(beacon).toHaveBeenCalledTimes(1)
    const blob = beacon.mock.calls[0][1] as Blob
    // sendBeacon receives a Blob; assert the JSON payload carries our event kind.
    return blob.text().then((body) => {
      expect(body).toContain('"kind":"protocol_usage"')
      expect(body).toContain('"protocol":"h2"')
    })
  })
})
```

Ensure the import line at the top of the spec pulls `recordPlayerEvent`, `flushPlayerTelemetry`, `__resetPlayerTelemetryForTest` and that `vi` is imported from `vitest` (add any missing name to the existing import).

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/utils/playerTelemetry.spec.ts -t "protocol_usage"`
Expected: FAIL — `recordPlayerEvent` rejects the unknown kind, so nothing is buffered and `beacon` is never called.

- [ ] **Step 3: Add `'protocol_usage'` to the kind union**

In `frontend/web/src/utils/playerTelemetry.ts`, change the `PlayerEvent.kind` type (line 17) from:

```ts
  kind: 'resolve' | 'stall' | 'playback_start_rejected' | 'playback_failed'
```

to:

```ts
  kind: 'resolve' | 'stall' | 'playback_start_rejected' | 'playback_failed' | 'protocol_usage'
```

- [ ] **Step 4: Allow the kind through the runtime guard**

In `recordPlayerEvent` (lines 117–123), change the guard from:

```ts
    if (
      e.kind !== 'resolve' &&
      e.kind !== 'stall' &&
      e.kind !== 'playback_start_rejected' &&
      e.kind !== 'playback_failed'
    )
      return
```

to:

```ts
    if (
      e.kind !== 'resolve' &&
      e.kind !== 'stall' &&
      e.kind !== 'playback_start_rejected' &&
      e.kind !== 'playback_failed' &&
      e.kind !== 'protocol_usage'
    )
      return
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/utils/playerTelemetry.spec.ts`
Expected: PASS (existing tests + the new protocol_usage test).

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/utils/playerTelemetry.ts frontend/web/src/utils/playerTelemetry.spec.ts
git commit -m "feat(player): accept protocol_usage telemetry kind

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Pure `protocol_usage` payload builder

**Files:**
- Create: `frontend/web/src/composables/aePlayer/protocolUsage.ts`
- Test: `frontend/web/src/composables/aePlayer/protocolUsage.spec.ts`

**Interfaces:**
- Consumes: `TierResidency` from `@/utils/protocolLadder` (Task 1).
- Produces:
  - `readVideoQuality(v: HTMLVideoElement | null): QualitySample` where `QualitySample = { dropped: number; total: number }`
  - `droppedFramesPct(start: QualitySample, end: QualitySample): number`
  - `buildProtocolUsageDetail(r: TierResidency, droppedPct: number, ctx: ProtocolUsageContext): Record<string, unknown>` where `ProtocolUsageContext = { animeName: string; combo: string; sess: string }`

- [ ] **Step 1: Write the failing tests**

Create `frontend/web/src/composables/aePlayer/protocolUsage.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import {
  readVideoQuality,
  droppedFramesPct,
  buildProtocolUsageDetail,
} from './protocolUsage'
import type { TierResidency } from '@/utils/protocolLadder'

const RESIDENCY: TierResidency = {
  tierId: 'h2',
  protocol: 'h2',
  segments: 214,
  avgMbps: 3.2049,
  neededMbps: 2.1,
  timeouts: 2,
  tierMs: 184000,
  trail: 'h3→h2 (first-frag projected 17s)',
  probe: 'h3 2.1 Mbps @03:24 — rejected (<1.1× h2)',
}

describe('droppedFramesPct', () => {
  it('computes the delta percentage over a residency', () => {
    expect(droppedFramesPct({ dropped: 10, total: 1000 }, { dropped: 16, total: 1100 })).toBe(6)
  })
  it('clamps negative deltas (video element recreated on a source switch)', () => {
    expect(droppedFramesPct({ dropped: 50, total: 5000 }, { dropped: 3, total: 300 })).toBe(1)
  })
  it('never divides by zero when no frames advanced', () => {
    expect(droppedFramesPct({ dropped: 0, total: 0 }, { dropped: 0, total: 0 })).toBe(0)
  })
})

describe('readVideoQuality', () => {
  it('reads dropped/total from getVideoPlaybackQuality', () => {
    const v = { getVideoPlaybackQuality: () => ({ droppedVideoFrames: 3, totalVideoFrames: 1200 }) }
    expect(readVideoQuality(v as unknown as HTMLVideoElement)).toEqual({ dropped: 3, total: 1200 })
  })
  it('degrades to zeros when unavailable', () => {
    expect(readVideoQuality(null)).toEqual({ dropped: 0, total: 0 })
  })
})

describe('buildProtocolUsageDetail', () => {
  it('maps and rounds a residency into the detail payload', () => {
    const d = buildProtocolUsageDetail(RESIDENCY, 0.42, {
      animeName: 'Naruto',
      combo: 'sub·en·gogoanime',
      sess: 's_abc123',
    })
    expect(d).toEqual({
      schema_version: 1,
      protocol: 'h2',
      tier: 'h2',
      segments: 214,
      avg_mbps: 3.2,
      needed_mbps: 2.1,
      dropped_frames_pct: 0.42,
      seg_timeouts: 2,
      tier_ms: 184000,
      trail: 'h3→h2 (first-frag projected 17s)',
      probe: 'h3 2.1 Mbps @03:24 — rejected (<1.1× h2)',
      anime_name: 'Naruto',
      combo: 'sub·en·gogoanime',
      sess: 's_abc123',
    })
  })
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/protocolUsage.spec.ts`
Expected: FAIL — module `./protocolUsage` does not exist.

- [ ] **Step 3: Implement the module**

Create `frontend/web/src/composables/aePlayer/protocolUsage.ts`:

```ts
// Pure helpers for the protocol_usage telemetry event — the dropped-frame math
// and the detail-payload mapping, kept out of AePlayer.vue so they are unit
// testable without mounting the player.

import type { TierResidency } from '@/utils/protocolLadder'

export interface QualitySample {
  dropped: number
  total: number
}

export interface ProtocolUsageContext {
  animeName: string
  combo: string
  /** Ephemeral, non-identifying per-mount id that groups a session's tier rows. */
  sess: string
}

function round2(n: number): number {
  return Math.round(n * 100) / 100
}

/** Reads dropped/total video frames from a video element, defensively. */
export function readVideoQuality(v: HTMLVideoElement | null): QualitySample {
  const q =
    v && typeof v.getVideoPlaybackQuality === 'function' ? v.getVideoPlaybackQuality() : null
  return { dropped: q?.droppedVideoFrames ?? 0, total: q?.totalVideoFrames ?? 0 }
}

/** Dropped-frame percentage over one tier residency, from the cumulative
 *  quality counters at tier start vs end. Deltas are clamped non-negative so a
 *  counter reset (the <video> element recreated on a source switch) can't
 *  produce a bogus negative/huge value; total delta floors at 1 to avoid /0. */
export function droppedFramesPct(start: QualitySample, end: QualitySample): number {
  const dropDelta = Math.max(0, end.dropped - start.dropped)
  const totalDelta = Math.max(1, end.total - start.total)
  return round2((dropDelta / totalDelta) * 100)
}

/** Builds the `detail` payload merged verbatim into the analytics
 *  `properties` JSON for a protocol_usage event. */
export function buildProtocolUsageDetail(
  r: TierResidency,
  droppedPct: number,
  ctx: ProtocolUsageContext,
): Record<string, unknown> {
  return {
    schema_version: 1,
    protocol: r.protocol,
    tier: r.tierId,
    segments: r.segments,
    avg_mbps: round2(r.avgMbps),
    needed_mbps: round2(r.neededMbps),
    dropped_frames_pct: droppedPct,
    seg_timeouts: r.timeouts,
    tier_ms: Math.round(r.tierMs),
    trail: r.trail,
    probe: r.probe,
    anime_name: ctx.animeName,
    combo: ctx.combo,
    sess: ctx.sess,
  }
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/protocolUsage.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/protocolUsage.ts frontend/web/src/composables/aePlayer/protocolUsage.spec.ts
git commit -m "feat(player): pure protocol_usage detail builder + dropped-frame math

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: Wire emission into AePlayer.vue

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (imports ~line 542/557; onMounted ~line 3408; ladder subscription ~line 3417; teardown ~line 3442)

**Interfaces:**
- Consumes: `ladder.onResidencyEnd`, `ladder.consumeResidency`, `TierResidency` (Task 1); `recordPlayerEvent`, `flushPlayerTelemetry` (Task 2); `buildProtocolUsageDetail`, `readVideoQuality`, `droppedFramesPct` (Task 3).
- Existing refs used (already in AePlayer): `videoRef`, `state.combo` (`{ audio, lang, provider, server, team }`), `selectedEpisode`, `props.animeId`, `props.anime.title`.

No new unit test — this is view glue; correctness is covered by Tasks 1–3 and verified by build + a runtime smoke in Task 7.

- [ ] **Step 1: Extend the telemetry import**

Change line 542 from:

```ts
import { recordPlayerEvent } from '@/utils/playerTelemetry'
```

to:

```ts
import { recordPlayerEvent, flushPlayerTelemetry } from '@/utils/playerTelemetry'
```

- [ ] **Step 2: Extend the ladder import with the `TierResidency` type**

Change line 557 from:

```ts
import { ladder, shouldDeferStallToLadder, formatLadderRows } from '@/utils/protocolLadder'
```

to:

```ts
import { ladder, shouldDeferStallToLadder, formatLadderRows, type TierResidency } from '@/utils/protocolLadder'
```

- [ ] **Step 3: Import the pure helpers**

Add, directly below the ladder import (after line 557):

```ts
import { buildProtocolUsageDetail, readVideoQuality, droppedFramesPct } from '@/composables/aePlayer/protocolUsage'
```

- [ ] **Step 4: Add the emission block next to the ladder subscription**

Immediately after the existing `onUnmounted(unsubLadder)` (line 3422), add:

```ts

// ── Protocol (h1/h2/h3) usage telemetry ───────────────────────────────────────
// One anonymous event per (session × tier): a residency summary from the ladder
// + the dropped-video-frame delta over that residency. Emitted on every tier
// switch (onResidencyEnd) and once at session end (pagehide / unmount).
const telemetrySess =
  's_' +
  (typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID().slice(0, 8)
    : Math.random().toString(36).slice(2, 10))
// Cumulative video-quality counters snapshotted at the current tier's start.
let tierStartQuality = readVideoQuality(null) // { dropped: 0, total: 0 }

function emitProtocolUsage(r: TierResidency): void {
  const q = readVideoQuality(videoRef.value)
  const pct = droppedFramesPct(tierStartQuality, q)
  tierStartQuality = q // the next tier's residency measures from here
  const c = state.combo.value
  recordPlayerEvent({
    kind: 'protocol_usage',
    provider: c.provider,
    anime_id: props.animeId,
    episode: selectedEpisode.value?.number,
    audio: c.audio,
    lang: c.lang,
    detail: buildProtocolUsageDetail(r, pct, {
      animeName: props.anime.title,
      combo: `${c.audio}·${c.lang}·${c.provider}`,
      sess: telemetrySess,
    }),
  })
}

// Session-end flush of the final (never-switched-away) tier. consumeResidency
// dedups a pagehide-then-unmount double call; the explicit flush guarantees the
// last row ships even if this pagehide listener runs after telemetry's own.
function flushProtocolUsage(): void {
  const r = ladder.consumeResidency()
  if (r) {
    emitProtocolUsage(r)
    flushPlayerTelemetry('protocol-usage-final')
  }
}

const unsubResidency = ladder.onResidencyEnd(emitProtocolUsage)
onUnmounted(unsubResidency)
```

- [ ] **Step 5: Register the pagehide flush in `onMounted`**

In the `onMounted` body, after `window.addEventListener('pagehide', onPageHide)` (line 3409), add:

```ts
  window.addEventListener('pagehide', flushProtocolUsage)
```

- [ ] **Step 6: Flush + unregister in the teardown**

In the main `onUnmounted` teardown block (lines 3442–3458), after `window.removeEventListener('pagehide', onPageHide)` (line 3454), add:

```ts
  flushProtocolUsage()
  window.removeEventListener('pagehide', flushProtocolUsage)
```

- [ ] **Step 7: Type-check and lint the changed file**

Run: `cd frontend/web && bunx vue-tsc --noEmit 2>&1 | grep -i "AePlayer\|protocolUsage\|protocolLadder" || echo "no type errors in changed files"`
Expected: `no type errors in changed files`.

Run: `cd frontend/web && bunx eslint src/components/player/aePlayer/AePlayer.vue src/composables/aePlayer/protocolUsage.ts`
Expected: clean (no errors).

- [ ] **Step 8: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/AePlayer.vue
git commit -m "feat(player): emit protocol_usage telemetry per session x tier

Subscribes to ladder.onResidencyEnd, attaches the dropped-video-frame delta and
combo/anime context, and flushes the final tier on pagehide/unmount.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 5: Analytics ingest — map `protocol_usage` → `player_protocol`

**Files:**
- Modify: `services/analytics/internal/handler/playertelemetry.go:117-123`
- Test: `services/analytics/internal/handler/playertelemetry_test.go`

**Interfaces:**
- Consumes: the `protocol_usage` wire event (Task 4). `Detail` merges into `Properties` via the handler's existing loop; `provider` must be whitelisted (gogoanime is).
- Produces: `events` rows with `effect_kind='player_protocol'`, `target=<provider>`, protocol metrics in `properties`.

- [ ] **Step 1: Write the failing test**

Append to `services/analytics/internal/handler/playertelemetry_test.go`:

```go
// TestPlayerTelemetry_ProtocolUsage verifies "protocol_usage" events map to
// effect_kind="player_protocol" and the protocol detail bundle is merged into
// Properties (provider must be whitelisted — gogoanime is).
func TestPlayerTelemetry_ProtocolUsage(t *testing.T) {
	sink := &effectSink{}
	h := NewPlayerTelemetryHandler(sink)

	body := `{"events":[{
		"kind":"protocol_usage","provider":"gogoanime","anime_id":"a1","episode":1,
		"audio":"sub","lang":"en",
		"detail":{"protocol":"h2","tier":"h2","segments":214,"avg_mbps":3.2,"dropped_frames_pct":0.4,"seg_timeouts":0,"anime_name":"Naruto","combo":"sub·en·gogoanime","sess":"s_abc"}
	}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if sink.count() != 1 {
		t.Fatalf("got %d events, want 1", sink.count())
	}
	ev := sink.at(0)
	if ev.EffectKind != "player_protocol" {
		t.Errorf("effect_kind = %q, want player_protocol", ev.EffectKind)
	}
	if ev.Target != "gogoanime" {
		t.Errorf("target = %q, want gogoanime", ev.Target)
	}
	if !strings.Contains(ev.Properties, `"protocol":"h2"`) ||
		!strings.Contains(ev.Properties, `"segments":214`) ||
		!strings.Contains(ev.Properties, `"anime_name":"Naruto"`) {
		t.Errorf("properties missing merged protocol detail: %s", ev.Properties)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/analytics && go test ./internal/handler/ -run TestPlayerTelemetry_ProtocolUsage -v`
Expected: FAIL — `protocol_usage` hits the `default` branch and is skipped, so `sink.count()` is 0.

- [ ] **Step 3: Add the handler case**

In `services/analytics/internal/handler/playertelemetry.go`, in the `switch we.Kind` block, after the `case "playback_failed":` block (ends line 120) and before `default:` (line 121), add:

```go
		case "protocol_usage":
			// Per-(session×tier) protocol-ladder usage summary. No duration
			// dimension; all metrics ride in Detail → Properties.
			effectKind = "player_protocol"
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/analytics && go test ./internal/handler/ -run TestPlayerTelemetry_ProtocolUsage -v`
Expected: PASS.

- [ ] **Step 5: Run the full handler package tests (no regressions)**

Run: `cd services/analytics && go test ./internal/handler/`
Expected: `ok` — all existing player-telemetry tests still pass.

- [ ] **Step 6: Commit**

```bash
git add services/analytics/internal/handler/playertelemetry.go services/analytics/internal/handler/playertelemetry_test.go
git commit -m "feat(analytics): ingest protocol_usage -> effect_kind player_protocol

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 6: Grafana panels — usage share + detail table

**Files:**
- Modify: `docker/grafana/dashboards/playback-health.json` (insert before the panels-array close at line 1811, after the last panel id 60 whose gridPos is `y:77,h:9`)

No unit test; validated by `jq` + a provisioning-log check in Task 7.

- [ ] **Step 1: Insert the row + two panels**

In `docker/grafana/dashboards/playback-health.json`, the last panel object (`"id": 60`) ends at line 1810 with:

```json
      "type": "table"
    }
  ],
```

Change that to append a comma after the panel's closing `}` and insert the three new objects before `  ],`:

```json
      "type": "table"
    },
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 86 },
      "id": 106,
      "panels": [],
      "title": "Protocol Ladder (h1/h2/h3)",
      "type": "row"
    },
    {
      "datasource": { "type": "grafana-clickhouse-datasource", "uid": "aenigma-clickhouse" },
      "description": "Segment-volume-weighted share of each negotiated protocol over the selected window.",
      "fieldConfig": { "defaults": { "custom": { "hideFrom": { "legend": false, "tooltip": false, "viz": false } }, "mappings": [] }, "overrides": [] },
      "gridPos": { "h": 8, "w": 8, "x": 0, "y": 87 },
      "id": 61,
      "options": {
        "legend": { "displayMode": "table", "placement": "right", "values": ["value", "percent"] },
        "pieType": "donut",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "tooltip": { "mode": "single", "sort": "none" }
      },
      "targets": [
        {
          "datasource": { "type": "grafana-clickhouse-datasource", "uid": "aenigma-clickhouse" },
          "rawQuery": true,
          "queryType": "sql",
          "format": "table",
          "refId": "A",
          "rawSql": "SELECT JSONExtractString(properties,'protocol') AS protocol, sum(JSONExtractUInt(properties,'segments')) AS segments FROM events WHERE $__timeFilter(timestamp) AND effect_kind='player_protocol' GROUP BY protocol ORDER BY segments DESC"
        }
      ],
      "title": "Protocol usage share",
      "type": "piechart"
    },
    {
      "datasource": { "type": "grafana-clickhouse-datasource", "uid": "aenigma-clickhouse" },
      "description": "One row per (session × protocol tier): what the ladder delivered on each tier a viewer used. Anonymous.",
      "fieldConfig": {
        "defaults": { "custom": { "align": "left", "cellOptions": { "type": "auto" }, "filterable": true }, "mappings": [] },
        "overrides": [
          {
            "matcher": { "id": "byName", "options": "Dropped frames %" },
            "properties": [
              { "id": "unit", "value": "percent" },
              { "id": "custom.cellOptions", "value": { "type": "color-text" } },
              { "id": "thresholds", "value": { "mode": "absolute", "steps": [ { "color": "green", "value": null }, { "color": "yellow", "value": 2 }, { "color": "red", "value": 5 } ] } }
            ]
          },
          {
            "matcher": { "id": "byName", "options": "Seg timeouts" },
            "properties": [
              { "id": "custom.cellOptions", "value": { "type": "color-text" } },
              { "id": "thresholds", "value": { "mode": "absolute", "steps": [ { "color": "green", "value": null }, { "color": "red", "value": 1 } ] } }
            ]
          }
        ]
      },
      "gridPos": { "h": 10, "w": 24, "x": 0, "y": 95 },
      "id": 62,
      "options": { "cellHeight": "sm", "footer": { "show": false }, "showHeader": true },
      "pluginVersion": "10.3.3",
      "targets": [
        {
          "datasource": { "type": "grafana-clickhouse-datasource", "uid": "aenigma-clickhouse" },
          "rawQuery": true,
          "queryType": "sql",
          "format": "table",
          "refId": "A",
          "rawSql": "SELECT timestamp AS \"Time\", JSONExtractString(properties,'protocol') AS \"Protocol\", coalesce(nullIf(JSONExtractString(properties,'anime_name'),''), anime_id) AS \"Anime\", JSONExtractString(properties,'combo') AS \"Combo\", JSONExtractUInt(properties,'segments') AS \"Segments\", round(JSONExtractFloat(properties,'avg_mbps'),2) AS \"Avg Mbps\", round(JSONExtractFloat(properties,'dropped_frames_pct'),2) AS \"Dropped frames %\", JSONExtractUInt(properties,'seg_timeouts') AS \"Seg timeouts\", round(JSONExtractFloat(properties,'needed_mbps'),2) AS \"Needed Mbps\", JSONExtractString(properties,'trail') AS \"Ladder trail\", JSONExtractString(properties,'probe') AS \"h3 probe\", target AS \"Provider\" FROM events WHERE $__timeFilter(timestamp) AND effect_kind='player_protocol' ORDER BY timestamp DESC LIMIT 500"
        }
      ],
      "title": "Protocol usage (per session × tier)",
      "type": "table"
    }
  ],
```

- [ ] **Step 2: Validate the JSON**

Run: `jq -e '.panels | map(.id) | index(61) and index(62) and index(106)' docker/grafana/dashboards/playback-health.json && echo "panels present + JSON valid"`
Expected: prints `true` then `panels present + JSON valid`. (A `jq` parse error means a trailing-comma/brace mistake — fix and re-run.)

- [ ] **Step 3: Commit**

```bash
git add docker/grafana/dashboards/playback-health.json
git commit -m "feat(grafana): protocol usage table + share panels on playback-health

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 7: End-to-end verification & deploy

**Files:** none (verification + deploy only).

- [ ] **Step 1: Frontend pre-flight**

Run: `/frontend-verify`
Expected: DS-lint clean, i18n parity unaffected (no new keys), `bun run build` succeeds. Fix any reported issue before proceeding.

- [ ] **Step 2: Full frontend unit tests for the touched modules**

Run: `cd frontend/web && bunx vitest run src/utils/protocolLadder.spec.ts src/utils/playerTelemetry.spec.ts src/composables/aePlayer/protocolUsage.spec.ts`
Expected: all PASS.

- [ ] **Step 3: Analytics tests**

Run: `cd services/analytics && go test ./internal/handler/`
Expected: `ok`.

- [ ] **Step 4: Confirm ClickHouse is an active analytics sink**

Run: `docker exec animeenigma-analytics printenv ANALYTICS_STORE_BACKEND 2>/dev/null || echo "check deploy env"`
Expected: `clickhouse` or `dualwrite`. If `postgres`, the CH dashboard panels will stay empty — flag to the owner (the change is still correct; visualization just needs a CH-writing backend, which the existing `player_resolve` panels already rely on).

- [ ] **Step 5: Deploy via the after-update skill**

Run: `/animeenigma-after-update`
This runs `/simplify` over the diff, redeploys `web` + `analytics` (`make redeploy-web`, `make redeploy-analytics`), reloads Grafana (`make restart-grafana`), health-checks, updates the changelog (Russian Trump-mode), and commits + pushes.

- [ ] **Step 6: Verify the dashboard provisioned cleanly**

Run: `docker logs animeenigma-grafana 2>&1 | grep -iE 'playback-health|provision' | grep -iE 'error|fail|invalid' || echo "grafana provisioned clean"`
Expected: `grafana provisioned clean`.

- [ ] **Step 7: Runtime smoke — one real playback produces a row**

Play a real multi-tier HLS title through aePlayer (e.g. a `gogoanime` episode), let it run ~30s so the ladder settles / probes, then navigate away (triggers the session-end flush). Confirm a row lands:

Run:
```bash
docker exec animeenigma-clickhouse clickhouse-client -q "SELECT timestamp, JSONExtractString(properties,'protocol') AS proto, target AS provider, JSONExtractUInt(properties,'segments') AS segs FROM analytics.events WHERE effect_kind='player_protocol' ORDER BY timestamp DESC LIMIT 5"
```
Expected: ≥1 row with a non-empty `proto` (`h2`/`h3`/`h1`) and `segs > 0`. Then open `https://animeenigma.org/admin/grafana/d/playback-health` and confirm both new panels render under "Protocol Ladder (h1/h2/h3)".

---

## Known limitations (documented, acceptable for v1)

- A residency spanning a **source/combo change without a tier change** (e.g. user switches provider, or advances episode on the same tier) is labelled with the combo/anime at emit time. The usage-**share** metric (segments per protocol) is unaffected; only the per-row provider/anime label can reflect the later source. Flushing on combo change is a possible future refinement.
- A **network-change** mid-session resets the ladder to entry without emitting the interrupted residency (the connection-change handler is best-effort). The next residency/session-end flush resumes normally.

## Self-Review

- **Spec coverage:** emission (Tasks 1,3,4) · ingest/`player_protocol` (Task 5) · `protocol_usage` kind (Task 2) · table panel (Task 6) · usage-share panel (Task 6) · anonymous/no-user (constraints + payload) · dropped-frames%+timeouts (Task 3 payload, Task 6 columns) · per-session×tier granularity (Task 1 residency model) · reuse-events storage (Task 5) · deploy order (Task 7). All spec sections map to a task.
- **Placeholder scan:** none — every step carries real code/commands/expected output.
- **Type consistency:** `TierResidency` fields (`tierId/protocol/segments/avgMbps/neededMbps/timeouts/tierMs/trail/probe`) are produced in Task 1 and consumed identically in Task 3; `buildProtocolUsageDetail`/`readVideoQuality`/`droppedFramesPct` signatures match between Task 3 definition and Task 4 use; `effect_kind='player_protocol'` and the `properties` keys are identical across Task 4 (emit), Task 5 (ingest/test), and Task 6 (queries).
