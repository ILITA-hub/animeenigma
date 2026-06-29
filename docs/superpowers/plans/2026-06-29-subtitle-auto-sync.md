# Subtitle Auto-Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically align subtitles to the spoken audio in aePlayer using a client-side Voice-Activity-Detection (VAD) + cross-correlation engine, on top of the existing manual offset.

**Architecture:** A pure-math core (`subtitleAlign.ts`) finds the best timing offset by cross-correlating a "speech active" interval set against a "subtitle active" interval set. A Web Audio tap (`subtitleAudioTap.ts`) feeds the engine speech/silence frames from the playing `<video>`. The orchestrating composable (`useSubtitleAutoSync.ts`) accumulates frames, locks an offset when confident, re-syncs on eyecatch steps, and exposes a bounded change-log. A per-episode toggle (`useSubtitleAutoSyncPref.ts`) gates it. `AePlayer.vue` wires `effectiveOffset = autoOffset + manualOffset` into the untouched `SubtitleOverlay.vue`.

**Tech Stack:** Vue 3 `<script setup>` + Composition API, TypeScript, Web Audio API, Vitest, vue-i18n. Frontend only — no backend, service, DB, or env var.

Spec: `docs/superpowers/specs/2026-06-29-subtitle-auto-sync-design.md`.

## Global Constraints

- **Surface:** `frontend/web/` only. No backend/service/env changes. Deploy = `make redeploy-web`.
- **Player scope:** aePlayer only (sole live `SubtitleOverlay` consumer). Do NOT touch `SubtitleOverlay.vue`, `useSubtitleTimingOffset.ts`, or `SubtitleSettingsMenu.vue` (dead code).
- **Sign convention (must hold everywhere):** offset is seconds; **positive ⇒ subtitles shown later** (`SubtitleOverlay` applies `t = currentTime - offset`). Subs that appear *before* speech need a **positive** offset.
- **Never make subs worse:** any failure / low confidence / unsupported audio ⇒ `autoOffset` stays `0` (no-op).
- **Toggle persistence:** `localStorage`, per-episode key, 24h TTL, **default `true`**.
- **DS-lint:** use the `Switch` primitive (`@/components/ui/Switch.vue`) — no native form controls; no off-palette Tailwind color classes; no raw hex/rgba in `.vue`. Debug text reuses `text-[var(--success)]` + `font-mono` (the existing hacker-panel pattern).
- **i18n:** every new key added to ALL THREE of `locales/{en,ru,ja}.json` (parity gate fails otherwise).
- **Every commit** includes these trailers verbatim:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Test runner:** from `frontend/web/`, `bunx vitest run <path>`. Final gate is `/frontend-verify`.

## File Structure

```
frontend/web/src/
  composables/aePlayer/
    subtitleAlign.ts                      # NEW — pure math: intervals, overlap, bestOffset, speechBandRatio + types
    subtitleAudioTap.ts                   # NEW — Web Audio tap (AudioContext/Analyser/Gain mirror) → speech frames
    useSubtitleAutoSync.ts                # NEW — engine: accumulate frames, lock/resync, syncEvents, lifecycle
    useSubtitleAutoSyncPref.ts            # NEW — per-episode toggle (localStorage, 24h TTL, default true)
    useSubtitleCues.ts                    # NEW — fetch + parse chosen subtitle into cues (for alignment)
    __tests__/
      subtitleAlign.spec.ts               # NEW
      useSubtitleAutoSync.spec.ts         # NEW
      useSubtitleAutoSyncPref.spec.ts     # NEW
      useSubtitleCues.spec.ts             # NEW
      subtitleAudioTap.spec.ts            # NEW
  components/player/aePlayer/
    SubtitlesMenu.vue                     # MODIFY — Auto-sync Switch + hacker debug panel
    AePlayer.vue                          # MODIFY — wire cues + pref + engine + effectiveOffset
    __tests__/SubtitlesMenu.spec.ts       # NEW (or extend if present)
  locales/{en,ru,ja}.json                 # MODIFY — autoSync, autoSyncHint, autoSyncDebug.*
```

---

### Task 1: `subtitleAlign.ts` — pure alignment math + types

The testable core. No Vue, no Web Audio.

**Files:**
- Create: `frontend/web/src/composables/aePlayer/subtitleAlign.ts`
- Test: `frontend/web/src/composables/aePlayer/__tests__/subtitleAlign.spec.ts`

**Interfaces:**
- Produces:
  - `interface Interval { start: number; end: number }`
  - `interface OffsetResult { offset: number; confidence: number }`
  - `interface SyncEvent { atMediaTime: number; fromOffset: number; toOffset: number; delta: number; confidence: number; windowStart: number; windowEnd: number; reason: 'lock' | 'resync' }`
  - `cuesToIntervals(cues: { start: number; end: number }[]): Interval[]`
  - `overlapDuration(a: Interval[], b: Interval[], delta: number): number`
  - `bestOffset(speech: Interval[], cues: Interval[], opts?: { min?: number; max?: number; step?: number }): OffsetResult`
  - `speechBandRatio(freq: Uint8Array, sampleRate: number, fftSize: number): number`
  - `const SEARCH = { min: -30, max: 30, step: 0.1 }`

- [ ] **Step 1: Write the failing tests**

```ts
// frontend/web/src/composables/aePlayer/__tests__/subtitleAlign.spec.ts
import { describe, it, expect } from 'vitest'
import { cuesToIntervals, overlapDuration, bestOffset, speechBandRatio } from '../subtitleAlign'

describe('cuesToIntervals', () => {
  it('sorts and merges overlapping/adjacent cues', () => {
    const out = cuesToIntervals([
      { start: 5, end: 6 }, { start: 1, end: 2 }, { start: 2, end: 3 },
    ])
    expect(out).toEqual([{ start: 1, end: 3 }, { start: 5, end: 6 }])
  })
  it('returns [] for empty input', () => {
    expect(cuesToIntervals([])).toEqual([])
  })
})

describe('overlapDuration', () => {
  it('measures intersection with b shifted by +delta', () => {
    const a = [{ start: 10, end: 12 }]
    const b = [{ start: 8, end: 10 }]
    expect(overlapDuration(a, b, 0)).toBeCloseTo(0, 5)   // b ends where a starts
    expect(overlapDuration(a, b, 2)).toBeCloseTo(2, 5)   // b -> [10,12] == a
  })
})

describe('bestOffset', () => {
  it('recovers a constant offset: subs 2s EARLY -> +2 (positive = later)', () => {
    const speech = [{ start: 10, end: 12 }, { start: 20, end: 23 }]
    const cues = [{ start: 8, end: 10 }, { start: 18, end: 21 }]
    const r = bestOffset(speech, cues)
    expect(r.offset).toBeCloseTo(2, 1)
    expect(r.confidence).toBeGreaterThan(0.15)
  })
  it('recovers a negative offset: subs 1.5s LATE -> -1.5', () => {
    const speech = [{ start: 10, end: 12 }, { start: 30, end: 33 }]
    const cues = [{ start: 11.5, end: 13.5 }, { start: 31.5, end: 34.5 }]
    expect(bestOffset(speech, cues).offset).toBeCloseTo(-1.5, 1)
  })
  it('reports low confidence when there is no clear peak', () => {
    const speech = [{ start: 0, end: 60 }]   // one giant blob, no structure
    const cues = [{ start: 0, end: 60 }]
    expect(bestOffset(speech, cues).confidence).toBeLessThan(0.15)
  })
  it('returns offset 0 / confidence 0 for empty inputs', () => {
    expect(bestOffset([], [{ start: 1, end: 2 }])).toEqual({ offset: 0, confidence: 0 })
    expect(bestOffset([{ start: 1, end: 2 }], [])).toEqual({ offset: 0, confidence: 0 })
  })
})

describe('speechBandRatio', () => {
  it('is high when energy sits in the 300-3400Hz band', () => {
    const fftSize = 2048, sampleRate = 48000
    const bins = fftSize / 2
    const freq = new Uint8Array(bins)
    // bin width = sampleRate/fftSize ≈ 23.4Hz; fill ~300-3400Hz strongly
    const lo = Math.floor(300 / (sampleRate / fftSize))
    const hi = Math.ceil(3400 / (sampleRate / fftSize))
    for (let i = lo; i <= hi; i++) freq[i] = 200
    expect(speechBandRatio(freq, sampleRate, fftSize)).toBeGreaterThan(0.8)
  })
  it('is low when energy sits outside the speech band', () => {
    const fftSize = 2048, sampleRate = 48000
    const freq = new Uint8Array(fftSize / 2)
    for (let i = 0; i < 3; i++) freq[i] = 220        // sub-bass only
    expect(speechBandRatio(freq, sampleRate, fftSize)).toBeLessThan(0.3)
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/subtitleAlign.spec.ts`
Expected: FAIL — `Cannot find module '../subtitleAlign'`.

- [ ] **Step 3: Implement `subtitleAlign.ts`**

```ts
// frontend/web/src/composables/aePlayer/subtitleAlign.ts
export interface Interval { start: number; end: number }
export interface OffsetResult { offset: number; confidence: number }

export interface SyncEvent {
  atMediaTime: number
  fromOffset: number
  toOffset: number
  delta: number
  confidence: number
  windowStart: number
  windowEnd: number
  reason: 'lock' | 'resync'
}

export const SEARCH = { min: -30, max: 30, step: 0.1 } as const

/** Sort by start and merge overlapping or touching intervals. */
export function cuesToIntervals(cues: { start: number; end: number }[]): Interval[] {
  const valid = cues.filter((c) => c.end > c.start).sort((a, b) => a.start - b.start)
  const out: Interval[] = []
  for (const c of valid) {
    const last = out[out.length - 1]
    if (last && c.start <= last.end) last.end = Math.max(last.end, c.end)
    else out.push({ start: c.start, end: c.end })
  }
  return out
}

/** Total intersection (seconds) of `a` with `b` shifted later by `delta`. */
export function overlapDuration(a: Interval[], b: Interval[], delta: number): number {
  let i = 0, j = 0, total = 0
  while (i < a.length && j < b.length) {
    const bs = b[j].start + delta, be = b[j].end + delta
    const lo = Math.max(a[i].start, bs), hi = Math.min(a[i].end, be)
    if (hi > lo) total += hi - lo
    if (a[i].end < be) i++; else j++
  }
  return total
}

/**
 * Slide cues against speech over the offset grid; return the offset maximizing
 * overlap. Confidence = prominence of the peak over the best competitor outside
 * a ±1s guard band, normalized by the peak.
 */
export function bestOffset(
  speech: Interval[],
  cues: Interval[],
  opts: { min?: number; max?: number; step?: number } = {},
): OffsetResult {
  if (!speech.length || !cues.length) return { offset: 0, confidence: 0 }
  const min = opts.min ?? SEARCH.min, max = opts.max ?? SEARCH.max, step = opts.step ?? SEARCH.step
  const n = Math.round((max - min) / step) + 1
  const scores = new Array<number>(n)
  let peak = -1, peakIdx = 0
  for (let k = 0; k < n; k++) {
    const delta = min + k * step
    const s = overlapDuration(speech, cues, delta)
    scores[k] = s
    if (s > peak) { peak = s; peakIdx = k }
  }
  if (peak <= 0) return { offset: 0, confidence: 0 }
  const guard = Math.round(1 / step)
  let second = 0
  for (let k = 0; k < n; k++) {
    if (Math.abs(k - peakIdx) <= guard) continue
    if (scores[k] > second) second = scores[k]
  }
  const offset = Math.round((min + peakIdx * step) * 10) / 10
  const confidence = Math.max(0, Math.min(1, (peak - second) / peak))
  return { offset, confidence }
}

/** Fraction of frequency energy that falls in the human speech band. */
export function speechBandRatio(freq: Uint8Array, sampleRate: number, fftSize: number): number {
  const binHz = sampleRate / fftSize
  const lo = Math.floor(300 / binHz), hi = Math.min(freq.length - 1, Math.ceil(3400 / binHz))
  let band = 0, total = 0
  for (let i = 0; i < freq.length; i++) {
    total += freq[i]
    if (i >= lo && i <= hi) band += freq[i]
  }
  return total > 0 ? band / total : 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/subtitleAlign.spec.ts`
Expected: PASS (all cases).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/subtitleAlign.ts frontend/web/src/composables/aePlayer/__tests__/subtitleAlign.spec.ts
git commit   # message: "feat(aeplayer): subtitle alignment math (VAD cross-correlation core)" + co-author trailers
```

---

### Task 2: `useSubtitleAutoSyncPref.ts` — per-episode toggle

**Files:**
- Create: `frontend/web/src/composables/aePlayer/useSubtitleAutoSyncPref.ts`
- Test: `frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSyncPref.spec.ts`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `useSubtitleAutoSyncPref(episodeKey: Ref<string>): { enabled: Ref<boolean>; setEnabled(v: boolean): void }`. Storage key `aenigma_subautosync_{episodeKey}`; stored `{ value: boolean; expiresAt: number }`; missing/expired ⇒ `true`. TTL = 24h.

- [ ] **Step 1: Write the failing tests**

```ts
// frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSyncPref.spec.ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { ref, nextTick } from 'vue'
import { useSubtitleAutoSyncPref, DAY_MS } from '../useSubtitleAutoSyncPref'

const KEY = (k: string) => `aenigma_subautosync_${k}`

describe('useSubtitleAutoSyncPref', () => {
  beforeEach(() => { localStorage.clear(); vi.useFakeTimers(); vi.setSystemTime(1_000_000) })
  afterEach(() => { vi.useRealTimers() })

  it('defaults to true when nothing stored', () => {
    expect(useSubtitleAutoSyncPref(ref('a:1')).enabled.value).toBe(true)
  })

  it('persists setEnabled(false) with a 24h expiry', () => {
    const { enabled, setEnabled } = useSubtitleAutoSyncPref(ref('a:1'))
    setEnabled(false)
    expect(enabled.value).toBe(false)
    const raw = JSON.parse(localStorage.getItem(KEY('a:1'))!)
    expect(raw.value).toBe(false)
    expect(raw.expiresAt).toBe(1_000_000 + DAY_MS)
  })

  it('reverts to default true once the stored value expires', () => {
    localStorage.setItem(KEY('a:1'), JSON.stringify({ value: false, expiresAt: 1_000_000 - 1 }))
    expect(useSubtitleAutoSyncPref(ref('a:1')).enabled.value).toBe(true)
  })

  it('is isolated per episode: disabling a:1 leaves a:2 default-on', () => {
    useSubtitleAutoSyncPref(ref('a:1')).setEnabled(false)
    expect(useSubtitleAutoSyncPref(ref('a:2')).enabled.value).toBe(true)
  })

  it('re-reads when episodeKey changes', async () => {
    localStorage.setItem(KEY('a:2'), JSON.stringify({ value: false, expiresAt: 1_000_000 + DAY_MS }))
    const ek = ref('a:1')
    const { enabled } = useSubtitleAutoSyncPref(ek)
    expect(enabled.value).toBe(true)
    ek.value = 'a:2'; await nextTick()
    expect(enabled.value).toBe(false)
  })

  it('falls back to true if storage throws', () => {
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked') })
    expect(useSubtitleAutoSyncPref(ref('a:1')).enabled.value).toBe(true)
    spy.mockRestore()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/useSubtitleAutoSyncPref.spec.ts`
Expected: FAIL — `Cannot find module '../useSubtitleAutoSyncPref'`.

- [ ] **Step 3: Implement `useSubtitleAutoSyncPref.ts`**

```ts
// frontend/web/src/composables/aePlayer/useSubtitleAutoSyncPref.ts
import { ref, watch, type Ref } from 'vue'

export const DAY_MS = 24 * 60 * 60 * 1000
const PREFIX = 'aenigma_subautosync_'

function read(key: string): boolean {
  if (typeof window === 'undefined') return true
  try {
    const raw = localStorage.getItem(PREFIX + key)
    if (!raw) return true
    const parsed = JSON.parse(raw) as { value?: unknown; expiresAt?: unknown }
    if (typeof parsed.expiresAt !== 'number' || Date.now() > parsed.expiresAt) return true
    return parsed.value !== false   // anything but an explicit false → on
  } catch {
    return true
  }
}

function write(key: string, value: boolean): void {
  if (typeof window === 'undefined') return
  try {
    localStorage.setItem(PREFIX + key, JSON.stringify({ value, expiresAt: Date.now() + DAY_MS }))
  } catch { /* quota / disabled storage — in-memory only */ }
}

export function useSubtitleAutoSyncPref(episodeKey: Ref<string>) {
  const enabled = ref<boolean>(read(episodeKey.value))
  watch(episodeKey, (k) => { enabled.value = read(k) })

  function setEnabled(v: boolean): void {
    enabled.value = v
    write(episodeKey.value, v)
  }
  return { enabled, setEnabled }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/useSubtitleAutoSyncPref.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/useSubtitleAutoSyncPref.ts frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSyncPref.spec.ts
git commit   # "feat(aeplayer): per-episode auto-sync toggle (24h TTL, default on)" + co-author trailers
```

---

### Task 3: `useSubtitleCues.ts` — fetch + parse cues for alignment

Mirrors `SubtitleOverlay.vue`'s fetch/parse (proxy for external URLs, format auto-detect) but returns the cue array for the engine. `SubtitleOverlay` stays untouched.

**Files:**
- Create: `frontend/web/src/composables/aePlayer/useSubtitleCues.ts`
- Test: `frontend/web/src/composables/aePlayer/__tests__/useSubtitleCues.spec.ts`

**Interfaces:**
- Consumes: `parseASS/parseSRT/parseVTT`, `SubtitleCue` from `@/utils/subtitle-parser`; `hlsProxyUrl` from `@/utils/streaming`.
- Produces: `useSubtitleCues(url: Ref<string | null>, format: Ref<'ass'|'srt'|'vtt'|null>): { cues: Ref<SubtitleCue[]> }`.

- [ ] **Step 1: Write the failing tests**

```ts
// frontend/web/src/composables/aePlayer/__tests__/useSubtitleCues.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref, nextTick } from 'vue'
import { useSubtitleCues } from '../useSubtitleCues'

const SRT = `1
00:00:08,000 --> 00:00:10,000
hello

2
00:00:18,000 --> 00:00:21,000
world
`

describe('useSubtitleCues', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, text: async () => SRT })))
  })

  it('fetches and parses cues when url is set', async () => {
    const url = ref<string | null>('https://cdn.example/x.srt')
    const { cues } = useSubtitleCues(url, ref('srt'))
    await nextTick(); await Promise.resolve(); await nextTick()
    expect(cues.value.length).toBe(2)
    expect(cues.value[0]).toMatchObject({ start: 8, end: 10 })
  })

  it('clears cues when url becomes null', async () => {
    const url = ref<string | null>('https://cdn.example/x.srt')
    const { cues } = useSubtitleCues(url, ref('srt'))
    await nextTick(); await Promise.resolve(); await nextTick()
    expect(cues.value.length).toBe(2)
    url.value = null; await nextTick()
    expect(cues.value).toEqual([])
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/useSubtitleCues.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `useSubtitleCues.ts`**

```ts
// frontend/web/src/composables/aePlayer/useSubtitleCues.ts
import { shallowRef, watch, type Ref } from 'vue'
import { parseASS, parseSRT, parseVTT, type SubtitleCue } from '@/utils/subtitle-parser'
import { hlsProxyUrl } from '@/utils/streaming'

export function useSubtitleCues(
  url: Ref<string | null>,
  format: Ref<'ass' | 'srt' | 'vtt' | null>,
) {
  const cues = shallowRef<SubtitleCue[]>([])
  let abort: AbortController | null = null

  async function load(u: string, fmt: string) {
    abort?.abort()
    abort = new AbortController()
    cues.value = []
    try {
      const fetchUrl = u.startsWith('/') ? u : hlsProxyUrl(`url=${encodeURIComponent(u)}`)
      const resp = await fetch(fetchUrl, { signal: abort.signal })
      if (!resp.ok) return
      const content = await resp.text()
      if (fmt === 'ass') cues.value = await parseASS(content)
      else if (fmt === 'vtt') cues.value = parseVTT(content)
      else if (fmt === 'srt') cues.value = parseSRT(content)
      else if (content.includes('[Script Info]') || content.includes('[V4+ Styles]')) cues.value = await parseASS(content)
      else if (content.startsWith('WEBVTT')) cues.value = parseVTT(content)
      else cues.value = parseSRT(content)
    } catch { /* abort / network / parse — leave cues empty (no-op for auto-sync) */ }
  }

  watch(url, (u) => { if (u) void load(u, format.value || 'auto'); else { abort?.abort(); cues.value = [] } }, { immediate: true })

  return { cues }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/useSubtitleCues.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/useSubtitleCues.ts frontend/web/src/composables/aePlayer/__tests__/useSubtitleCues.spec.ts
git commit   # "feat(aeplayer): subtitle cue fetch+parse composable for alignment" + co-author trailers
```

---

### Task 4: `useSubtitleAutoSync.ts` — the engine

Accumulates speech frames into intervals, locks/re-syncs an offset via `bestOffset`, maintains `syncEvents`, resets per episode. The audio source is **injected** (`createTap`) so it's testable without Web Audio.

**Files:**
- Create: `frontend/web/src/composables/aePlayer/useSubtitleAutoSync.ts`
- Test: `frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSync.spec.ts`

**Interfaces:**
- Consumes: `cuesToIntervals`, `bestOffset`, `SEARCH`, `Interval`, `SyncEvent` from `./subtitleAlign`.
- Produces:
  - `interface SpeechTap { onFrame(cb: (mediaTime: number, speaking: boolean) => void): void; dispose(): void }`
  - `interface AutoSyncConfig { minSpeech: number; confMin: number; resyncDelta: number; maxEvents: number; seekGapSec: number }`
  - `const DEFAULT_AUTOSYNC_CONFIG: AutoSyncConfig`
  - `useSubtitleAutoSync(opts: { videoElement: Ref<HTMLVideoElement|null>; cues: Ref<{start:number;end:number}[]>; enabled: Ref<boolean>; episodeKey: Ref<string>; createTap?: (el: HTMLVideoElement) => SpeechTap; config?: Partial<AutoSyncConfig> }): { autoOffset: Ref<number>; status: Ref<'idle'|'listening'|'locked'|'unsupported'>; confidence: Ref<number>; syncEvents: Ref<SyncEvent[]> }`

- [ ] **Step 1: Write the failing tests**

```ts
// frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSync.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref, nextTick } from 'vue'
import { useSubtitleAutoSync, type SpeechTap } from '../useSubtitleAutoSync'

function fakeTap() {
  let cb: ((t: number, s: boolean) => void) | null = null
  const tap: SpeechTap & { frame(t: number, s: boolean): void; disposed: boolean } = {
    onFrame: (fn) => { cb = fn },
    dispose() { this.disposed = true },
    frame(t, s) { cb?.(t, s) },
    disposed: false,
  }
  return tap
}
// Drive a speech interval [a,b] then a silence frame to close it.
function speak(tap: ReturnType<typeof fakeTap>, a: number, b: number) {
  tap.frame(a, true); tap.frame((a + b) / 2, true); tap.frame(b, true); tap.frame(b + 0.05, false)
}
const cfg = { minSpeech: 3, confMin: 0.1, resyncDelta: 0.5, maxEvents: 10, seekGapSec: 1 }
const vel = () => ref(document.createElement('video'))

describe('useSubtitleAutoSync', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(1000) })

  it('locks a constant offset once enough confident speech accrues', async () => {
    const tap = fakeTap()
    const cues = ref([{ start: 8, end: 10 }, { start: 18, end: 21 }]) // 2s early vs speech below
    const s = useSubtitleAutoSync({ videoElement: vel(), cues, enabled: ref(true), episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)   // ~5s speech total
    await nextTick()
    expect(s.status.value).toBe('locked')
    expect(s.autoOffset.value).toBeCloseTo(2, 1)
    expect(s.syncEvents.value[0]).toMatchObject({ reason: 'lock', toOffset: expect.any(Number) })
  })

  it('does NOT lock below the speech minimum (no-op)', async () => {
    const tap = fakeTap()
    const cues = ref([{ start: 8, end: 10 }])
    const s = useSubtitleAutoSync({ videoElement: vel(), cues, enabled: ref(true), episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 11)   // 1s < minSpeech 3
    await nextTick()
    expect(s.status.value).toBe('listening')
    expect(s.autoOffset.value).toBe(0)
  })

  it('re-syncs on a mid-episode step (eyecatch) and logs reason=resync', async () => {
    const tap = fakeTap()
    const cues = ref([{ start: 8, end: 10 }, { start: 18, end: 21 }, { start: 40, end: 42 }, { start: 50, end: 53 }])
    const s = useSubtitleAutoSync({ videoElement: vel(), cues, enabled: ref(true), episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)            // lock at +2
    expect(s.autoOffset.value).toBeCloseTo(2, 1)
    speak(tap, 45, 47); speak(tap, 55, 58)            // now subs need +5 (cue 40 -> speech 45)
    await nextTick()
    expect(s.autoOffset.value).toBeCloseTo(5, 1)
    expect(s.syncEvents.value[0].reason).toBe('resync')
  })

  it('resets state and disposes tap on episode change', async () => {
    const tap = fakeTap()
    const cues = ref([{ start: 8, end: 10 }, { start: 18, end: 21 }])
    const ek = ref('a:1')
    const s = useSubtitleAutoSync({ videoElement: vel(), cues, enabled: ref(true), episodeKey: ek, createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)
    expect(s.autoOffset.value).toBeCloseTo(2, 1)
    ek.value = 'a:2'; await nextTick()
    expect(s.autoOffset.value).toBe(0)
    expect(s.syncEvents.value).toEqual([])
    expect(tap.disposed).toBe(true)
  })

  it('disabling drops autoOffset to 0 and disposes', async () => {
    const tap = fakeTap()
    const cues = ref([{ start: 8, end: 10 }, { start: 18, end: 21 }])
    const enabled = ref(true)
    const s = useSubtitleAutoSync({ videoElement: vel(), cues, enabled, episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)
    enabled.value = false; await nextTick()
    expect(s.autoOffset.value).toBe(0)
    expect(s.status.value).toBe('idle')
    expect(tap.disposed).toBe(true)
  })

  it('marks unsupported if the tap factory throws', async () => {
    const cues = ref([{ start: 8, end: 10 }])
    const s = useSubtitleAutoSync({ videoElement: vel(), cues, enabled: ref(true), episodeKey: ref('a:1'), createTap: () => { throw new Error('no audio') }, config: cfg })
    await nextTick()
    expect(s.status.value).toBe('unsupported')
    expect(s.autoOffset.value).toBe(0)
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/useSubtitleAutoSync.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `useSubtitleAutoSync.ts`**

```ts
// frontend/web/src/composables/aePlayer/useSubtitleAutoSync.ts
import { ref, computed, watch, onUnmounted, type Ref } from 'vue'
import { cuesToIntervals, bestOffset, SEARCH, type Interval, type SyncEvent } from './subtitleAlign'
import { createAudioTap } from './subtitleAudioTap'

export interface SpeechTap {
  onFrame(cb: (mediaTime: number, speaking: boolean) => void): void
  dispose(): void
}

export interface AutoSyncConfig {
  minSpeech: number      // seconds of accumulated speech before first lock
  confMin: number        // min peak prominence to act
  resyncDelta: number    // min offset change (s) to adopt a re-sync
  maxEvents: number      // change-log cap
  seekGapSec: number     // frame gap that counts as a seek (don't bridge)
}

export const DEFAULT_AUTOSYNC_CONFIG: AutoSyncConfig = {
  minSpeech: 8, confMin: 0.15, resyncDelta: 0.5, maxEvents: 10, seekGapSec: 1,
}

type Status = 'idle' | 'listening' | 'locked' | 'unsupported'

export function useSubtitleAutoSync(opts: {
  videoElement: Ref<HTMLVideoElement | null>
  cues: Ref<{ start: number; end: number }[]>
  enabled: Ref<boolean>
  episodeKey: Ref<string>
  createTap?: (el: HTMLVideoElement) => SpeechTap
  config?: Partial<AutoSyncConfig>
}) {
  const cfg: AutoSyncConfig = { ...DEFAULT_AUTOSYNC_CONFIG, ...(opts.config ?? {}) }
  const makeTap = opts.createTap ?? createAudioTap

  const autoOffset = ref(0)
  const confidence = ref(0)
  const status = ref<Status>('idle')
  const syncEvents = ref<SyncEvent[]>([])

  const cueIntervals = computed<Interval[]>(() => cuesToIntervals(opts.cues.value))

  // Accumulated speech intervals (media time).
  let speech: Interval[] = []
  let openStart: number | null = null
  let lastT = -Infinity
  let totalSpeech = 0
  let locked = false
  let tap: SpeechTap | null = null

  function resetData() {
    speech = []; openStart = null; lastT = -Infinity; totalSpeech = 0; locked = false
    autoOffset.value = 0; confidence.value = 0; syncEvents.value = []
  }

  function apply(offset: number, conf: number, reason: 'lock' | 'resync') {
    const from = autoOffset.value
    const to = Math.round(offset * 10) / 10
    if (to === from) return
    const ev: SyncEvent = {
      atMediaTime: lastT, fromOffset: from, toOffset: to, delta: Math.round((to - from) * 10) / 10,
      confidence: conf, windowStart: speech[0]?.start ?? 0, windowEnd: lastT, reason,
    }
    syncEvents.value = [ev, ...syncEvents.value].slice(0, cfg.maxEvents)
    autoOffset.value = to; confidence.value = conf; locked = true; status.value = 'locked'
  }

  function evaluate() {
    if (!speech.length || !cueIntervals.value.length) return
    const r = bestOffset(speech, cueIntervals.value, SEARCH)
    if (!locked) {
      if (totalSpeech >= cfg.minSpeech && r.confidence >= cfg.confMin) apply(r.offset, r.confidence, 'lock')
    } else if (r.confidence >= cfg.confMin && Math.abs(r.offset - autoOffset.value) >= cfg.resyncDelta) {
      apply(r.offset, r.confidence, 'resync')
    }
  }

  function ingest(t: number, speaking: boolean) {
    // Seek / discontinuity: close any open run without bridging.
    if (t < lastT || t - lastT > cfg.seekGapSec) {
      if (openStart !== null) { speech.push({ start: openStart, end: lastT }); openStart = null }
    }
    if (speaking) {
      if (openStart === null) openStart = t
    } else if (openStart !== null) {
      const seg = { start: openStart, end: t }
      if (seg.end > seg.start) { speech.push(seg); totalSpeech += seg.end - seg.start }
      openStart = null
      evaluate()   // re-evaluate when a speech run closes (deterministic)
    }
    lastT = t
  }

  function startTap() {
    if (tap || !opts.videoElement.value) return
    try {
      tap = makeTap(opts.videoElement.value)
      tap.onFrame(ingest)
      status.value = 'listening'
    } catch {
      status.value = 'unsupported'
    }
  }

  function stopTap() {
    tap?.dispose(); tap = null
  }

  function arm() {
    if (opts.enabled.value && opts.videoElement.value) {
      if (status.value !== 'unsupported') { status.value = 'listening'; startTap() }
    } else {
      stopTap(); resetData(); status.value = 'idle'
    }
  }

  watch(opts.episodeKey, () => { stopTap(); resetData(); status.value = 'idle'; arm() })
  watch(opts.enabled, arm)
  watch(opts.videoElement, arm, { immediate: true })
  // re-evaluate if cues arrive after speech (e.g. subtitle picked mid-playback)
  watch(cueIntervals, () => { if (!locked) evaluate() })

  onUnmounted(stopTap)

  return { autoOffset, status, confidence, syncEvents }
}
```

> Note: Task 5 creates `./subtitleAudioTap` (the default `createAudioTap`). These tests inject `createTap`, so they pass before Task 5 exists **only if** the import resolves. Implement Task 5 first OR add a temporary `export function createAudioTap(): SpeechTap { throw new Error('unsupported') }` stub in `subtitleAudioTap.ts` before running Task 4 tests. The plan orders Task 5 immediately after; if running strictly in order, create the stub file now and flesh it out in Task 5.

- [ ] **Step 4: Create the tap stub so the import resolves, then run tests**

Create `frontend/web/src/composables/aePlayer/subtitleAudioTap.ts` with a minimal stub (replaced in Task 5):
```ts
import type { SpeechTap } from './useSubtitleAutoSync'
export function createAudioTap(_el: HTMLVideoElement): SpeechTap {
  throw new Error('audio tap not implemented')   // replaced in Task 5
}
```
Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/useSubtitleAutoSync.spec.ts`
Expected: PASS (tests inject `createTap`, never hitting the stub).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/useSubtitleAutoSync.ts frontend/web/src/composables/aePlayer/subtitleAudioTap.ts frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSync.spec.ts
git commit   # "feat(aeplayer): subtitle auto-sync engine (lock/resync + change-log)" + co-author trailers
```

---

### Task 5: `subtitleAudioTap.ts` — Web Audio tap (real implementation)

Replaces the Task-4 stub. Taps the playing `<video>` audio, mirrors volume/mute so playback is unaffected, emits speech/silence frames via the speech-band VAD.

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/subtitleAudioTap.ts`
- Test: `frontend/web/src/composables/aePlayer/__tests__/subtitleAudioTap.spec.ts`

**Interfaces:**
- Consumes: `speechBandRatio` from `./subtitleAlign`; `SpeechTap` from `./useSubtitleAutoSync`.
- Produces: `createAudioTap(el: HTMLVideoElement): SpeechTap` (real). Builds `AudioContext → MediaElementSource → GainNode(mirror volume/mute) → destination`, plus `MediaElementSource → AnalyserNode`; polls on `requestAnimationFrame`, classifies frames (`speechBandRatio > VAD_RATIO` with an adaptive floor), emits `(el.currentTime, speaking)`. `dispose()` cancels rAF, disconnects nodes, closes the context, removes listeners.

- [ ] **Step 1: Write the failing test (AudioContext mocked)**

```ts
// frontend/web/src/composables/aePlayer/__tests__/subtitleAudioTap.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'

// Minimal Web Audio mock capturing the graph + gain.
class FakeNode { connect = vi.fn(); disconnect = vi.fn() }
class FakeGain extends FakeNode { gain = { value: 1 } }
class FakeAnalyser extends FakeNode {
  fftSize = 2048; frequencyBinCount = 1024
  getByteFrequencyData = vi.fn((arr: Uint8Array) => arr.fill(0))
}
let lastGain: FakeGain
class FakeCtx {
  sampleRate = 48000; state = 'running'
  destination = new FakeNode()
  createMediaElementSource = vi.fn(() => new FakeNode())
  createGain = vi.fn(() => (lastGain = new FakeGain()))
  createAnalyser = vi.fn(() => new FakeAnalyser())
  close = vi.fn(async () => { this.state = 'closed' })
}

beforeEach(() => {
  vi.stubGlobal('AudioContext', FakeCtx as unknown as typeof AudioContext)
  vi.stubGlobal('requestAnimationFrame', () => 1)
  vi.stubGlobal('cancelAnimationFrame', () => {})
})

describe('createAudioTap', () => {
  it('mirrors element volume/mute into the gain node', async () => {
    const { createAudioTap } = await import('../subtitleAudioTap')
    const el = document.createElement('video')
    Object.defineProperty(el, 'volume', { value: 0.5, configurable: true })
    const tap = createAudioTap(el)
    expect(lastGain.gain.value).toBeCloseTo(0.5, 5)
    Object.defineProperty(el, 'muted', { value: true, configurable: true })
    el.dispatchEvent(new Event('volumechange'))
    expect(lastGain.gain.value).toBe(0)
    tap.dispose()
  })

  it('dispose() closes the context', async () => {
    const { createAudioTap } = await import('../subtitleAudioTap')
    const tap = createAudioTap(document.createElement('video'))
    tap.dispose()
    // no throw; context.close called (state flipped in mock)
    expect(true).toBe(true)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/subtitleAudioTap.spec.ts`
Expected: FAIL — current stub throws `audio tap not implemented`.

- [ ] **Step 3: Implement the real `subtitleAudioTap.ts`**

```ts
// frontend/web/src/composables/aePlayer/subtitleAudioTap.ts
import { speechBandRatio } from './subtitleAlign'
import type { SpeechTap } from './useSubtitleAutoSync'

const VAD_RATIO = 0.55       // speech-band fraction above which a frame is "voiced"
const ENERGY_MIN = 12        // mean byte energy floor — below this = silence regardless

type Ctor = typeof AudioContext
function ctxCtor(): Ctor | null {
  const w = window as unknown as { AudioContext?: Ctor; webkitAudioContext?: Ctor }
  return w.AudioContext ?? w.webkitAudioContext ?? null
}

export function createAudioTap(el: HTMLVideoElement): SpeechTap {
  const Ctor = ctxCtor()
  if (!Ctor) throw new Error('Web Audio unavailable')

  const ctx = new Ctor()
  const src = ctx.createMediaElementSource(el)   // throws if cross-origin tainted / already tapped
  const gain = ctx.createGain()
  const analyser = ctx.createAnalyser()
  analyser.fftSize = 2048

  // Preserve player volume/mute: route audio through a mirrored gain to output.
  src.connect(gain); gain.connect(ctx.destination)
  src.connect(analyser)

  const syncGain = () => { gain.gain.value = el.muted ? 0 : el.volume }
  syncGain()
  el.addEventListener('volumechange', syncGain)

  const freq = new Uint8Array(analyser.frequencyBinCount)
  let cb: ((t: number, s: boolean) => void) | null = null
  let raf: number | null = null

  function tick() {
    analyser.getByteFrequencyData(freq)
    let sum = 0
    for (let i = 0; i < freq.length; i++) sum += freq[i]
    const mean = sum / freq.length
    const speaking = mean >= ENERGY_MIN && speechBandRatio(freq, ctx.sampleRate, analyser.fftSize) >= VAD_RATIO
    if (!el.paused && !el.seeking) cb?.(el.currentTime, speaking)
    raf = requestAnimationFrame(tick)
  }
  raf = requestAnimationFrame(tick)

  return {
    onFrame(fn) { cb = fn },
    dispose() {
      if (raf !== null) cancelAnimationFrame(raf)
      el.removeEventListener('volumechange', syncGain)
      try { src.disconnect(); gain.disconnect(); analyser.disconnect() } catch { /* already gone */ }
      void ctx.close().catch(() => {})
    },
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/subtitleAudioTap.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/subtitleAudioTap.ts frontend/web/src/composables/aePlayer/__tests__/subtitleAudioTap.spec.ts
git commit   # "feat(aeplayer): Web Audio VAD tap with volume/mute mirror" + co-author trailers
```

---

### Task 6: `SubtitlesMenu.vue` — Auto-sync Switch + hacker debug panel + i18n

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/SubtitlesMenu.vue`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`
- Test: `frontend/web/src/components/player/aePlayer/__tests__/SubtitlesMenu.spec.ts`

**Interfaces:**
- Consumes: `SyncEvent` from `@/composables/aePlayer/subtitleAlign`; `Switch` from `@/components/ui/Switch.vue`.
- Produces (new props): `autoSync: boolean`; `autoSyncInfo?: { status: string; offset: number; confidence: number; events: SyncEvent[] } | null`. New emit: `(e: 'update:autoSync', value: boolean): void`.

- [ ] **Step 1: Add i18n keys (all three locales)**

In each of `en.json`, `ru.json`, `ja.json`, inside `player.aePlayer.subs`, add these keys (values per locale below). Keep existing keys.

`en.json`:
```json
"autoSync": "Auto-sync",
"autoSyncHint": "Automatically align subtitles to the audio",
"autoSyncDebug": {
  "state": "{status} · {offset}s · {conf}%",
  "event": "{delta}s (VAD) @ {from}–{to} ({conf}%)"
}
```
`ru.json`:
```json
"autoSync": "Автосинхрон",
"autoSyncHint": "Автоматически подгонять субтитры под звук",
"autoSyncDebug": {
  "state": "{status} · {offset}с · {conf}%",
  "event": "{delta}с (VAD) @ {from}–{to} ({conf}%)"
}
```
`ja.json`:
```json
"autoSync": "自動同期",
"autoSyncHint": "字幕を音声に自動的に合わせる",
"autoSyncDebug": {
  "state": "{status} · {offset}秒 · {conf}%",
  "event": "{delta}秒 (VAD) @ {from}–{to} ({conf}%)"
}
```

- [ ] **Step 2: Write the failing menu test**

```ts
// frontend/web/src/components/player/aePlayer/__tests__/SubtitlesMenu.spec.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import SubtitlesMenu from '../SubtitlesMenu.vue'

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en } })
const base = {
  subLang: 'off', availableSubLangs: [], langSources: {}, browseCount: 0,
  hardsubNote: null, subSize: 100, subBg: 45, subOffset: 0, autoSync: true,
}
const mountMenu = (props = {}) =>
  mount(SubtitlesMenu, { props: { ...base, ...props }, global: { plugins: [i18n] } })

describe('SubtitlesMenu auto-sync', () => {
  it('emits update:autoSync when the switch is toggled (appearance face)', async () => {
    const w = mountMenu()
    await w.find('[data-test="style-toggle"]').trigger('click')
    await w.find('[data-test="autosync-switch"] button, [data-test="autosync-switch"]').trigger('click')
    expect(w.emitted('update:autoSync')).toBeTruthy()
  })

  it('shows the VAD debug panel only when autoSyncInfo is provided', async () => {
    const w = mountMenu({
      autoSyncInfo: { status: 'locked', offset: 2, confidence: 0.8, events: [
        { atMediaTime: 12, fromOffset: 0, toOffset: 2, delta: 2, confidence: 0.8, windowStart: 0, windowEnd: 12, reason: 'lock' },
      ] },
    })
    await w.find('[data-test="style-toggle"]').trigger('click')
    expect(w.find('[data-test="autosync-debug"]').exists()).toBe(true)
    expect(w.find('[data-test="autosync-debug"]').text()).toContain('VAD')
  })

  it('hides the debug panel when autoSyncInfo is null', async () => {
    const w = mountMenu({ autoSyncInfo: null })
    await w.find('[data-test="style-toggle"]').trigger('click')
    expect(w.find('[data-test="autosync-debug"]').exists()).toBe(false)
  })
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/SubtitlesMenu.spec.ts`
Expected: FAIL — no `autosync-switch` / unknown prop.

- [ ] **Step 4: Edit `SubtitlesMenu.vue`**

(a) In the Appearance face (`<template v-else>`), insert ABOVE the `<!-- Timing offset -->` block:
```html
      <!-- Auto-sync -->
      <div class="flex items-center gap-3 px-3 py-2">
        <div class="flex-1">
          <div class="text-[13px] text-[var(--ink-2)]">{{ $t('player.aePlayer.subs.autoSync') }}</div>
          <div class="text-[11px] text-[var(--muted-foreground)]">{{ $t('player.aePlayer.subs.autoSyncHint') }}</div>
        </div>
        <span data-test="autosync-switch">
          <Switch :model-value="autoSync" @update:model-value="emit('update:autoSync', $event)" />
        </span>
      </div>

      <!-- Auto-sync debug (hacker mode) -->
      <div
        v-if="autoSyncInfo"
        data-test="autosync-debug"
        class="px-3 pb-2 font-mono text-[11px] leading-[1.7] text-[var(--success)]"
      >
        <div>{{ $t('player.aePlayer.subs.autoSyncDebug.state', {
          status: autoSyncInfo.status,
          offset: autoSyncInfo.offset.toFixed(1),
          conf: Math.round(autoSyncInfo.confidence * 100),
        }) }}</div>
        <div v-for="(ev, i) in autoSyncInfo.events" :key="i">
          {{ $t('player.aePlayer.subs.autoSyncDebug.event', {
            delta: (ev.delta >= 0 ? '+' : '') + ev.delta.toFixed(1),
            from: fmtClock(ev.windowStart),
            to: fmtClock(ev.windowEnd),
            conf: Math.round(ev.confidence * 100),
          }) }}
        </div>
      </div>
```

(b) In `<script setup>`: add the `Switch` import and `SyncEvent` type, extend props/emits, add `fmtClock`.
```ts
import Switch from '@/components/ui/Switch.vue'
import type { SyncEvent } from '@/composables/aePlayer/subtitleAlign'
```
Extend `defineProps` with:
```ts
  autoSync: boolean
  autoSyncInfo?: { status: string; offset: number; confidence: number; events: SyncEvent[] } | null
```
Extend `defineEmits` with:
```ts
  (e: 'update:autoSync', value: boolean): void
```
Add helper (near `offsetHint`):
```ts
function fmtClock(s: number): string {
  const t = Math.max(0, Math.round(s))
  return `${Math.floor(t / 60)}:${String(t % 60).padStart(2, '0')}`
}
```

- [ ] **Step 5: Run test + DS-lint**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/SubtitlesMenu.spec.ts`
Expected: PASS.
Run: `cd frontend/web && bash scripts/design-system-lint.sh`
Expected: `ERRORS: 0` (Switch is a primitive; colors use tokens).

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/SubtitlesMenu.vue frontend/web/src/components/player/aePlayer/__tests__/SubtitlesMenu.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit   # "feat(aeplayer): auto-sync toggle + hacker VAD change-log in Subtitles menu" + co-author trailers
```

---

### Task 7: `AePlayer.vue` — wire engine into the offset path

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`
- Test: `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts` (add an assertion)

**Interfaces:**
- Consumes: `useSubtitleCues`, `useSubtitleAutoSyncPref`, `useSubtitleAutoSync` (all from `@/composables/aePlayer/...`); existing `videoRef`, `state`, `chosenSubUrl`, `chosenSubFormat`, `subsOn`, `selectedEpisode`, `props.animeId`, `props.anime`.
- Produces: `effectiveOffset` computed bound to `SubtitleOverlay :offset`; new props/handlers on `<SubtitlesMenu>`.

- [ ] **Step 1: Add imports**

Near the other composable imports in `AePlayer.vue` `<script setup>`:
```ts
import { useSubtitleCues } from '@/composables/aePlayer/useSubtitleCues'
import { useSubtitleAutoSyncPref } from '@/composables/aePlayer/useSubtitleAutoSyncPref'
import { useSubtitleAutoSync } from '@/composables/aePlayer/useSubtitleAutoSync'
```

- [ ] **Step 2: Wire the engine** (place AFTER `chosenSubFormat` / `subsOn` are defined, ~line 2196)

```ts
// ─── Subtitle auto-sync (frontend VAD; spec 2026-06-29) ──────────────────────
const { cues: subtitleCues } = useSubtitleCues(chosenSubUrl, chosenSubFormat)
const autoSyncEpisodeKey = computed(
  () => `${props.animeId}:${selectedEpisode.value?.number ?? props.anime.ep ?? 0}`,
)
const autoSyncPref = useSubtitleAutoSyncPref(autoSyncEpisodeKey)
const autoSync = useSubtitleAutoSync({
  videoElement: videoRef,
  cues: subtitleCues,
  enabled: computed(() => autoSyncPref.enabled.value && subsOn.value),
  episodeKey: autoSyncEpisodeKey,
})
// Manual offset layers on top of the auto result.
const effectiveOffset = computed(() => autoSync.autoOffset.value + state.subOffset.value)
const autoSyncInfo = computed(() =>
  state.hackerMode.value
    ? {
        status: autoSync.status.value,
        offset: autoSync.autoOffset.value,
        confidence: autoSync.confidence.value,
        events: autoSync.syncEvents.value,
      }
    : null,
)
```

- [ ] **Step 3: Bind into the template**

(a) `SubtitleOverlay` — change line 56 from `:offset="state.subOffset.value"` to:
```html
      :offset="effectiveOffset"
```
(b) `<SubtitlesMenu>` (the `openMenu === 'subs'` block) — add inside the tag:
```html
        :auto-sync="autoSyncPref.enabled.value"
        :auto-sync-info="autoSyncInfo"
        @update:auto-sync="v => autoSyncPref.setEnabled(v)"
```

- [ ] **Step 4: Add a regression assertion**

In `AePlayer.subtitles.spec.ts`, add a case asserting the overlay receives `state.subOffset` when auto-sync is idle (auto-offset 0), i.e. `effectiveOffset === subOffset` initially. (Mirror the file's existing mount/stub setup; assert the `SubtitleOverlay` stub's `offset` prop equals the manual value when no audio lock has occurred.)
```ts
it('effective subtitle offset equals the manual offset before any auto lock', async () => {
  // ...existing mount of AePlayer with a chosen subtitle...
  // set manual offset, expect SubtitleOverlay offset prop to match (autoOffset starts 0)
  // (use the spec file's existing overlay stub + helpers)
})
```

- [ ] **Step 5: Run subtitle tests + type-check**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts`
Expected: PASS.
Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: 0 errors. (Per [[feedback_vuetsc_noemit_false_pass]]: if it passes suspiciously fast, `rm -rf node_modules/.tmp` / clear the vue-tsc cache and re-run; import the menu's object-prop type from the barrel/composable, not inline.)

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts
git commit   # "feat(aeplayer): apply auto-sync offset on top of manual in SubtitleOverlay" + co-author trailers
```

---

### Task 8: Full frontend verification gate

**Files:** none (verification only).

- [ ] **Step 1: Run the full aePlayer + composable test suite**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer src/components/player/aePlayer src/locales`
Expected: PASS (incl. the locale parity spec — all three locales carry the new `autoSync`/`autoSyncHint`/`autoSyncDebug.*` keys).

- [ ] **Step 2: Run `/frontend-verify`**

Invoke the `/frontend-verify` skill (DS-lint, i18n en/ru/ja parity, vue-tsc, real `bun run build`, lucide/TS2614/Tailwind-v4 traps).
Expected: all gates green. Fix any finding inline and re-run.

- [ ] **Step 3: Commit any fixes from verification**

```bash
git add -A
git commit   # "fix(aeplayer): frontend-verify findings for subtitle auto-sync" + co-author trailers  (skip if nothing changed)
```

---

## Self-Review (completed during planning)

- **Spec coverage:** §3.1 engine → Tasks 1/4/5; §3.1 `syncEvents` → Task 4 + Task 6 panel; §3.2 toggle → Task 2; §3.3 menu UI → Task 6; §3.4 wiring/`effectiveOffset` → Task 7; §3.5 hacker readout → Task 6; §4 manual interaction → Task 7 (`effectiveOffset = auto + manual`) + Task 2 (OFF→0); §6 risk-1 volume/mute → Task 5 test; §6 risk-2 tainted → Task 4 `unsupported` + Task 5 `createMediaElementSource` throw path; §7 tests → all task test blocks. No gaps.
- **Type consistency:** `SyncEvent`/`Interval`/`OffsetResult` defined once in Task 1, imported by Tasks 4/6; `SpeechTap`/`AutoSyncConfig` defined in Task 4, imported by Task 5; engine output names (`autoOffset`/`status`/`confidence`/`syncEvents`) match across Tasks 4/6/7.
- **Placeholder scan:** Task 7 Step 4 references "the spec file's existing mount/stub setup" rather than inlining unknown helpers — intentional (that helper set is local to the existing test file the executor will read); all production code is complete.
- **Ordering note:** Task 4 depends on `./subtitleAudioTap` existing — handled by the Step-4 stub, fully implemented in Task 5.
