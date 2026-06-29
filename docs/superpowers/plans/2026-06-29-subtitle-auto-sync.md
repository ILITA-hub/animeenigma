# Subtitle Auto-Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically align subtitles to the spoken audio in aePlayer using a client-side Voice-Activity-Detection (VAD) + cross-correlation engine, on top of the existing manual offset.

**Architecture:** A pure-math core (`subtitleAlign.ts`) holds the interval math, the offset cross-correlation, the VAD frame classifier, and the shared port type. A Web Audio tap (`subtitleAudioTap.ts`) feeds the engine speech/silence frames from the playing `<video>`. The orchestrating composable (`useSubtitleAutoSync.ts`) accumulates frames over a sliding window, locks an offset when confident, re-syncs on eyecatch steps, and exposes a bounded change-log. A per-episode toggle (`useSubtitleAutoSyncPref.ts`) gates it. Cue fetch/parse is unified in one `fetchAndParseCues` helper used by both the engine's `useSubtitleCues` and the existing `SubtitleOverlay`. `AePlayer.vue` wires `effectiveOffset = autoOffset + manualOffset` into the overlay.

**Tech Stack:** Vue 3 `<script setup>` + Composition API, TypeScript, Web Audio API, Vitest, vue-i18n. Frontend only — no backend, service, DB, or env var.

Spec: `docs/superpowers/specs/2026-06-29-subtitle-auto-sync-design.md`.

## Global Constraints

- **Surface:** `frontend/web/` only. No backend/service/env changes. Deploy = `make redeploy-web`.
- **Player scope:** aePlayer only. `SubtitleOverlay.vue` is touched ONLY to delegate its fetch/parse to the shared `fetchAndParseCues` helper (behavior-preserving) — no render/teleport/offset change. Do NOT touch `useSubtitleTimingOffset.ts` / `SubtitleSettingsMenu.vue` (dead code).
- **Sign convention (must hold everywhere):** offset is seconds; **positive ⇒ subtitles shown later** (`SubtitleOverlay` applies `t = currentTime - offset`). Subs that appear *before* speech need a **positive** offset.
- **Never make subs worse:** any failure / low confidence / unsupported audio ⇒ `autoOffset` stays `0` (no-op).
- **Toggle persistence:** `localStorage`, per-episode key, 24h TTL, **default `true`**.
- **Reuse, don't re-implement:** quantize offsets via `round1` (defined once in `subtitleAlign.ts`); format mm:ss via the existing exported `fmtResume` (`@/composables/aePlayer/episodeProgress.ts`); fetch/parse cues via the shared `fetchAndParseCues`; episode identity via AePlayer's existing `subEpisode` computed.
- **DS-lint:** use the `Switch` primitive (`@/components/ui/Switch.vue`); no native form controls; no off-palette Tailwind color classes; no raw hex/rgba in `.vue`. Debug text reuses `text-[var(--success)]` + `font-mono` (existing hacker-panel pattern).
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
    subtitleAlign.ts                      # NEW — pure: Interval/OffsetResult/SyncEvent/SpeechTap types,
                                          #       cuesToIntervals, overlapDuration, bestOffset, round1, classifyFrame, SEARCH
    subtitleAudioTap.ts                   # NEW — Web Audio tap (AudioContext/Analyser/Gain mirror), 20Hz, → speech frames
    useSubtitleAutoSync.ts                # NEW — engine: sliding-window accumulate, lock/resync, syncEvents, lifecycle
    useSubtitleAutoSyncPref.ts            # NEW — per-episode toggle (localStorage, 24h TTL, default true)
    useSubtitleCues.ts                    # NEW — cue fetch+parse via fetchAndParseCues (for alignment)
    __tests__/
      subtitleAlign.spec.ts               # NEW
      subtitleAudioTap.spec.ts            # NEW
      useSubtitleAutoSync.spec.ts         # NEW
      useSubtitleAutoSyncPref.spec.ts     # NEW
      useSubtitleCues.spec.ts             # NEW
  utils/
    subtitle-parser.ts                    # MODIFY — add fetchAndParseCues(url, format, signal?)
    __tests__/subtitle-parser.spec.ts     # NEW (or extend) — fetchAndParseCues test
  components/player/
    SubtitleOverlay.vue                   # MODIFY — loadSubtitles delegates to fetchAndParseCues
    aePlayer/
      SubtitlesMenu.vue                   # MODIFY — Auto-sync Switch + hacker debug panel
      AePlayer.vue                        # MODIFY — wire cues + pref + engine + effectiveOffset
      __tests__/SubtitlesMenu.spec.ts     # NEW
  locales/{en,ru,ja}.json                 # MODIFY — autoSync, autoSyncHint, autoSyncDebug.*
```

---

### Task 1: `subtitleAlign.ts` — pure alignment math, classifier, types

The testable core. No Vue, no Web Audio. Owns the `SpeechTap` port so both the tap (Task 4) and the engine (Task 5) depend on a neutral layer.

**Files:**
- Create: `frontend/web/src/composables/aePlayer/subtitleAlign.ts`
- Test: `frontend/web/src/composables/aePlayer/__tests__/subtitleAlign.spec.ts`

**Interfaces:**
- Produces:
  - `interface Interval { start: number; end: number }`
  - `interface OffsetResult { offset: number; confidence: number }`
  - `interface SyncEvent { delta: number; confidence: number; windowStart: number; windowEnd: number; reason: 'lock' | 'resync' }`
  - `interface SpeechTap { onFrame(cb: (mediaTime: number, speaking: boolean) => void): void; dispose(): void }`
  - `round1(v: number): number`
  - `cuesToIntervals(cues: { start: number; end: number }[]): Interval[]`
  - `overlapDuration(a: Interval[], b: Interval[], delta: number): number`
  - `bestOffset(speech: Interval[], cues: Interval[], opts?: { min?: number; max?: number; step?: number }): OffsetResult`
  - `classifyFrame(freq: Uint8Array, sampleRate: number, fftSize: number, opts?: { vadRatio?: number; energyMin?: number }): boolean`
  - `const SEARCH = { min: -30, max: 30, step: 0.1 }`

- [ ] **Step 1: Write the failing tests**

```ts
// frontend/web/src/composables/aePlayer/__tests__/subtitleAlign.spec.ts
import { describe, it, expect } from 'vitest'
import { cuesToIntervals, overlapDuration, bestOffset, classifyFrame, round1 } from '../subtitleAlign'

describe('round1', () => {
  it('rounds to one decimal without float fuzz', () => {
    expect(round1(0.1 + 0.2)).toBe(0.3)
    expect(round1(2.449)).toBe(2.4)
  })
})

describe('cuesToIntervals', () => {
  it('sorts and merges overlapping/adjacent cues', () => {
    expect(cuesToIntervals([{ start: 5, end: 6 }, { start: 1, end: 2 }, { start: 2, end: 3 }]))
      .toEqual([{ start: 1, end: 3 }, { start: 5, end: 6 }])
  })
  it('returns [] for empty input', () => { expect(cuesToIntervals([])).toEqual([]) })
})

describe('overlapDuration', () => {
  it('measures intersection with b shifted by +delta', () => {
    const a = [{ start: 10, end: 12 }], b = [{ start: 8, end: 10 }]
    expect(overlapDuration(a, b, 0)).toBeCloseTo(0, 5)
    expect(overlapDuration(a, b, 2)).toBeCloseTo(2, 5)
  })
})

describe('bestOffset', () => {
  it('recovers a constant offset: subs 2s EARLY -> +2 (positive = later)', () => {
    const r = bestOffset([{ start: 10, end: 12 }, { start: 20, end: 23 }], [{ start: 8, end: 10 }, { start: 18, end: 21 }])
    expect(r.offset).toBeCloseTo(2, 1)
    expect(r.confidence).toBeGreaterThan(0.15)
  })
  it('recovers a negative offset: subs 1.5s LATE -> -1.5', () => {
    expect(bestOffset([{ start: 10, end: 12 }, { start: 30, end: 33 }], [{ start: 11.5, end: 13.5 }, { start: 31.5, end: 34.5 }]).offset)
      .toBeCloseTo(-1.5, 1)
  })
  it('reports low confidence with no clear peak', () => {
    expect(bestOffset([{ start: 0, end: 60 }], [{ start: 0, end: 60 }]).confidence).toBeLessThan(0.15)
  })
  it('returns offset 0 / confidence 0 for empty inputs', () => {
    expect(bestOffset([], [{ start: 1, end: 2 }])).toEqual({ offset: 0, confidence: 0 })
    expect(bestOffset([{ start: 1, end: 2 }], [])).toEqual({ offset: 0, confidence: 0 })
  })
})

describe('classifyFrame', () => {
  const fftSize = 2048, sampleRate = 48000, bins = fftSize / 2, binHz = sampleRate / fftSize
  const lo = Math.floor(300 / binHz), hi = Math.ceil(3400 / binHz)
  it('true when loud energy sits in the 300-3400Hz band', () => {
    const freq = new Uint8Array(bins)
    for (let i = lo; i <= hi; i++) freq[i] = 200
    expect(classifyFrame(freq, sampleRate, fftSize)).toBe(true)
  })
  it('false when loud energy sits OUTSIDE the band (ratio gate)', () => {
    const freq = new Uint8Array(bins)
    for (let i = hi + 1; i < bins; i++) freq[i] = 200   // high mean, but out of band
    expect(classifyFrame(freq, sampleRate, fftSize)).toBe(false)
  })
  it('false on near-silence even if band-weighted (energy gate)', () => {
    const freq = new Uint8Array(bins)
    for (let i = lo; i <= hi; i++) freq[i] = 3          // ratio high, mean tiny
    expect(classifyFrame(freq, sampleRate, fftSize)).toBe(false)
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
  delta: number          // offset change applied (s), signed
  confidence: number     // 0..1 of the chosen offset
  windowStart: number    // analyzed speech-window interval (s)
  windowEnd: number
  reason: 'lock' | 'resync'
}

/** Port shared by the engine and any audio tap (real Web Audio or a test fake). */
export interface SpeechTap {
  onFrame(cb: (mediaTime: number, speaking: boolean) => void): void
  dispose(): void
}

export const SEARCH = { min: -30, max: 30, step: 0.1 } as const

export function round1(v: number): number { return Math.round(v * 10) / 10 }

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

/** Total intersection (seconds) of `a` with `b` shifted later by `delta`. Inputs sorted. */
export function overlapDuration(a: Interval[], b: Interval[], delta: number): number {
  let i = 0, j = 0, total = 0
  while (i < a.length && j < b.length) {
    const bs = b[j].start + delta, be = b[j].end + delta
    const hi = Math.min(a[i].end, be), lo = Math.max(a[i].start, bs)
    if (hi > lo) total += hi - lo
    if (a[i].end < be) i++; else j++
  }
  return total
}

// Reusable scratch for the offset sweep — the grid size is constant, so this
// avoids allocating a fresh array on every evaluate() call.
const SCORES = new Float64Array(Math.round((SEARCH.max - SEARCH.min) / SEARCH.step) + 1)

/**
 * Slide cues against speech over the offset grid; return the offset maximizing
 * overlap. Confidence = prominence of the peak over the best competitor outside
 * a ±1s guard band, normalized by the peak (∈ [0,1] since 0 ≤ second ≤ peak).
 */
export function bestOffset(
  speech: Interval[],
  cues: Interval[],
  opts: { min?: number; max?: number; step?: number } = {},
): OffsetResult {
  if (!speech.length || !cues.length) return { offset: 0, confidence: 0 }
  const min = opts.min ?? SEARCH.min, max = opts.max ?? SEARCH.max, step = opts.step ?? SEARCH.step
  const n = Math.round((max - min) / step) + 1
  const scores = n === SCORES.length ? SCORES : new Float64Array(n)
  let peak = -1, peakIdx = 0
  for (let k = 0; k < n; k++) {
    const s = overlapDuration(speech, cues, min + k * step)
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
  return { offset: round1(min + peakIdx * step), confidence: (peak - second) / peak }
}

/**
 * Pure VAD frame decision in a single pass over the bins: speaking when mean byte
 * energy ≥ floor AND the fraction of energy in the human speech band (≈300-3400Hz)
 * ≥ vadRatio. v1 uses a fixed floor (an adaptive running-percentile floor can later
 * replace `energyMin` here without touching the tap).
 */
export function classifyFrame(
  freq: Uint8Array,
  sampleRate: number,
  fftSize: number,
  opts: { vadRatio?: number; energyMin?: number } = {},
): boolean {
  const vadRatio = opts.vadRatio ?? 0.55
  const energyMin = opts.energyMin ?? 12
  const binHz = sampleRate / fftSize
  const lo = Math.floor(300 / binHz), hi = Math.min(freq.length - 1, Math.ceil(3400 / binHz))
  let band = 0, total = 0
  for (let i = 0; i < freq.length; i++) {
    total += freq[i]
    if (i >= lo && i <= hi) band += freq[i]
  }
  const mean = freq.length ? total / freq.length : 0
  const ratio = total > 0 ? band / total : 0
  return mean >= energyMin && ratio >= vadRatio
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/subtitleAlign.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/subtitleAlign.ts frontend/web/src/composables/aePlayer/__tests__/subtitleAlign.spec.ts
git commit   # "feat(aeplayer): subtitle alignment math + VAD classifier (pure core)" + co-author trailers
```

---

### Task 2: `useSubtitleAutoSyncPref.ts` — per-episode toggle

**Files:**
- Create: `frontend/web/src/composables/aePlayer/useSubtitleAutoSyncPref.ts`
- Test: `frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSyncPref.spec.ts`

**Interfaces:**
- Produces: `useSubtitleAutoSyncPref(episodeKey: Ref<string>): { enabled: Ref<boolean>; setEnabled(v: boolean): void }`; storage key `aenigma_subautosync_{episodeKey}`; stored `{ value: boolean; expiresAt: number }`; missing/expired ⇒ `true`. `export const DAY_MS`.

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
    expect(raw.value).toBe(false); expect(raw.expiresAt).toBe(1_000_000 + DAY_MS)
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
    const ek = ref('a:1'); const { enabled } = useSubtitleAutoSyncPref(ek)
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
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `useSubtitleAutoSyncPref.ts`**

```ts
// frontend/web/src/composables/aePlayer/useSubtitleAutoSyncPref.ts
// Third subtitle-pref persistence model in the player, by design: the dead global-sticky
// useSubtitleTimingOffset and the live ephemeral usePlayerState.subOffset are the others.
// Per-episode + 24h TTL = a "decaying opt-out": a local disable (auto-sync misfired on one
// episode) self-heals after a day rather than permanently killing a default-good feature.
// Expiry is read-time only (expired keys aren't evicted — they're tiny).
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
  } catch { return true }
}

function write(key: string, value: boolean): void {
  if (typeof window === 'undefined') return
  try { localStorage.setItem(PREFIX + key, JSON.stringify({ value, expiresAt: Date.now() + DAY_MS })) }
  catch { /* quota / disabled storage — in-memory only */ }
}

export function useSubtitleAutoSyncPref(episodeKey: Ref<string>) {
  const enabled = ref<boolean>(read(episodeKey.value))
  watch(episodeKey, (k) => { enabled.value = read(k) })
  function setEnabled(v: boolean): void { enabled.value = v; write(episodeKey.value, v) }
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

### Task 3: shared `fetchAndParseCues` + `useSubtitleCues` (and overlay delegation)

Unify the subtitle fetch/format-sniff in one helper so the engine aligns the exact cue set the overlay renders (no drift).

**Files:**
- Modify: `frontend/web/src/utils/subtitle-parser.ts` (add `fetchAndParseCues`)
- Modify: `frontend/web/src/components/player/SubtitleOverlay.vue` (`loadSubtitles` delegates)
- Create: `frontend/web/src/composables/aePlayer/useSubtitleCues.ts`
- Test: `frontend/web/src/utils/__tests__/subtitle-parser.spec.ts` (new or extend — `fetchAndParseCues`)
- Test: `frontend/web/src/composables/aePlayer/__tests__/useSubtitleCues.spec.ts`

**Interfaces:**
- Produces: `fetchAndParseCues(url: string, format: 'ass'|'srt'|'vtt'|'auto'|string, signal?: AbortSignal): Promise<SubtitleCue[]>` (throws on `!resp.ok`); `useSubtitleCues(url: Ref<string|null>, format: Ref<'ass'|'srt'|'vtt'|null>): { cues: Ref<SubtitleCue[]> }`.
- Consumes: `parseASS/parseSRT/parseVTT`, `SubtitleCue` (already in `subtitle-parser.ts`); `hlsProxyUrl` from `@/utils/streaming`.

- [ ] **Step 1: Write the failing tests**

```ts
// frontend/web/src/utils/__tests__/subtitle-parser.spec.ts   (add this describe; create file if absent)
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fetchAndParseCues } from '../subtitle-parser'

const SRT = `1\n00:00:08,000 --> 00:00:10,000\nhello\n\n2\n00:00:18,000 --> 00:00:21,000\nworld\n`

describe('fetchAndParseCues', () => {
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, text: async () => SRT }))) })
  it('fetches and parses by explicit format', async () => {
    const cues = await fetchAndParseCues('https://cdn.example/x.srt', 'srt')
    expect(cues.length).toBe(2)
    expect(cues[0]).toMatchObject({ start: 8, end: 10 })
  })
  it('throws on a non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 404, text: async () => '' })))
    await expect(fetchAndParseCues('https://cdn.example/x.srt', 'srt')).rejects.toThrow()
  })
})
```
```ts
// frontend/web/src/composables/aePlayer/__tests__/useSubtitleCues.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref, nextTick } from 'vue'
import { useSubtitleCues } from '../useSubtitleCues'

const SRT = `1\n00:00:08,000 --> 00:00:10,000\nhello\n\n2\n00:00:18,000 --> 00:00:21,000\nworld\n`

describe('useSubtitleCues', () => {
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, text: async () => SRT }))) })
  it('fetches and parses cues when url is set', async () => {
    const { cues } = useSubtitleCues(ref<string | null>('https://cdn.example/x.srt'), ref('srt'))
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

Run: `cd frontend/web && bunx vitest run src/utils/__tests__/subtitle-parser.spec.ts src/composables/aePlayer/__tests__/useSubtitleCues.spec.ts`
Expected: FAIL — `fetchAndParseCues` / module not found.

- [ ] **Step 3a: Add `fetchAndParseCues` to `subtitle-parser.ts`**

Add this import near the top (verify no circular import — `@/utils/streaming` must not import `subtitle-parser`):
```ts
import { hlsProxyUrl } from '@/utils/streaming'
```
Append the helper (uses the existing `parseASS/parseSRT/parseVTT`):
```ts
/**
 * Fetch a subtitle file and parse it to cues. Same-origin backend URLs (leading `/`)
 * are fetched directly; external provider URLs go through the CORS proxy. Throws on a
 * non-ok response. Single source of truth for SubtitleOverlay + useSubtitleCues.
 */
export async function fetchAndParseCues(
  url: string,
  format: 'ass' | 'srt' | 'vtt' | 'auto' | string,
  signal?: AbortSignal,
): Promise<SubtitleCue[]> {
  const fetchUrl = url.startsWith('/') ? url : hlsProxyUrl(`url=${encodeURIComponent(url)}`)
  const resp = await fetch(fetchUrl, signal ? { signal } : undefined)
  if (!resp.ok) throw new Error(`Failed to fetch subtitle file: ${resp.status}`)
  const content = await resp.text()
  switch (format) {
    case 'ass': return parseASS(content)
    case 'srt': return parseSRT(content)
    case 'vtt': return parseVTT(content)
    default:
      if (content.includes('[Script Info]') || content.includes('[V4+ Styles]')) return parseASS(content)
      if (content.startsWith('WEBVTT')) return parseVTT(content)
      return parseSRT(content)
  }
}
```

- [ ] **Step 3b: Refactor `SubtitleOverlay.vue` `loadSubtitles` to delegate (behavior-preserving)**

Replace the import line
```ts
import { parseASS, parseSRT, parseVTT } from '@/utils/subtitle-parser'
import { hlsProxyUrl } from '@/utils/streaming'
```
with
```ts
import { fetchAndParseCues } from '@/utils/subtitle-parser'
```
(keep the existing `import type { SubtitleCue } from '@/utils/subtitle-parser'`). Then replace the body of `loadSubtitles` (the fetch + switch block) so the whole function reads:
```ts
async function loadSubtitles(url: string, format: string) {
  subtitleAbortController?.abort()
  subtitleAbortController = new AbortController()
  emit('loading', true)
  cues.value = []
  try {
    cues.value = await fetchAndParseCues(url, format, subtitleAbortController.signal)
  } catch (err: unknown) {
    if (err instanceof DOMException && err.name === 'AbortError') return
    const e = err as { message?: string }
    emit('error', e.message || 'Failed to load subtitles')
  } finally {
    emit('loading', false)
  }
}
```

- [ ] **Step 3c: Implement `useSubtitleCues.ts`**

```ts
// frontend/web/src/composables/aePlayer/useSubtitleCues.ts
import { shallowRef, watch, type Ref } from 'vue'
import { fetchAndParseCues, type SubtitleCue } from '@/utils/subtitle-parser'

export function useSubtitleCues(
  url: Ref<string | null>,
  format: Ref<'ass' | 'srt' | 'vtt' | null>,
) {
  const cues = shallowRef<SubtitleCue[]>([])
  let abort: AbortController | null = null

  watch(url, async (u) => {
    abort?.abort()
    if (!u) { cues.value = []; return }
    abort = new AbortController()
    try { cues.value = await fetchAndParseCues(u, format.value || 'auto', abort.signal) }
    catch { cues.value = [] }   // abort / network / parse → no-op for auto-sync
  }, { immediate: true })

  return { cues }
}
```

- [ ] **Step 4: Run tests (new + the overlay's existing spec for the refactor)**

Run: `cd frontend/web && bunx vitest run src/utils/__tests__/subtitle-parser.spec.ts src/composables/aePlayer/__tests__/useSubtitleCues.spec.ts src/components/player/SubtitleOverlay.spec.ts`
Expected: PASS (overlay spec unchanged behavior).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/utils/subtitle-parser.ts frontend/web/src/utils/__tests__/subtitle-parser.spec.ts frontend/web/src/components/player/SubtitleOverlay.vue frontend/web/src/composables/aePlayer/useSubtitleCues.ts frontend/web/src/composables/aePlayer/__tests__/useSubtitleCues.spec.ts
git commit   # "refactor(player): unify subtitle fetch/parse in fetchAndParseCues + useSubtitleCues" + co-author trailers
```

---

### Task 4: `subtitleAudioTap.ts` — Web Audio tap

Taps the playing `<video>` audio, mirrors volume/mute so playback is unaffected, emits speech/silence frames via the pure `classifyFrame`. Throttled to ~20 Hz.

**Files:**
- Create: `frontend/web/src/composables/aePlayer/subtitleAudioTap.ts`
- Test: `frontend/web/src/composables/aePlayer/__tests__/subtitleAudioTap.spec.ts`

**Interfaces:**
- Consumes: `classifyFrame`, `SpeechTap` from `./subtitleAlign`.
- Produces: `createAudioTap(el: HTMLVideoElement): SpeechTap`.

- [ ] **Step 1: Write the failing test (AudioContext mocked)**

```ts
// frontend/web/src/composables/aePlayer/__tests__/subtitleAudioTap.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'

class FakeNode { connect = vi.fn(); disconnect = vi.fn() }
class FakeGain extends FakeNode { gain = { value: 1 } }
class FakeAnalyser extends FakeNode {
  fftSize = 2048; frequencyBinCount = 1024
  getByteFrequencyData = vi.fn((arr: Uint8Array) => arr.fill(0))
}
let lastGain: FakeGain
class FakeCtx {
  sampleRate = 48000; state = 'running'; destination = new FakeNode()
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
  it('dispose() runs without throwing', async () => {
    const { createAudioTap } = await import('../subtitleAudioTap')
    expect(() => createAudioTap(document.createElement('video')).dispose()).not.toThrow()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/subtitleAudioTap.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `subtitleAudioTap.ts`**

```ts
// frontend/web/src/composables/aePlayer/subtitleAudioTap.ts
import { classifyFrame, type SpeechTap } from './subtitleAlign'

const TICK_HZ = 20   // VAD segments are >> 50ms; 20Hz is ample and ~3x cheaper than display rate

function ctxCtor(): typeof AudioContext | null {
  const w = window as unknown as { AudioContext?: typeof AudioContext; webkitAudioContext?: typeof AudioContext }
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
  const minGap = 1000 / TICK_HZ
  let cb: ((t: number, s: boolean) => void) | null = null
  let raf: number | null = null
  let lastTick = -Infinity

  function tick(now: number) {
    raf = requestAnimationFrame(tick)
    if (now - lastTick < minGap) return
    lastTick = now
    if (el.paused || el.seeking) return
    analyser.getByteFrequencyData(freq)
    cb?.(el.currentTime, classifyFrame(freq, ctx.sampleRate, analyser.fftSize))
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
git commit   # "feat(aeplayer): Web Audio VAD tap (20Hz, volume/mute mirror)" + co-author trailers
```

---

### Task 5: `useSubtitleAutoSync.ts` — the engine

Accumulates speech frames into a **sliding window** of intervals, locks/re-syncs an offset via `bestOffset`, maintains `syncEvents`, resets per episode. The audio source is injected (`createTap`, default = real `createAudioTap`) so it's testable without Web Audio.

**Files:**
- Create: `frontend/web/src/composables/aePlayer/useSubtitleAutoSync.ts`
- Test: `frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSync.spec.ts`

**Interfaces:**
- Consumes: `cuesToIntervals`, `bestOffset`, `round1`, `SEARCH`, `Interval`, `SyncEvent`, `SpeechTap` from `./subtitleAlign`; `createAudioTap` from `./subtitleAudioTap`.
- Produces: re-exports `type { SpeechTap }`; `interface AutoSyncConfig { minSpeech; confMin; resyncDelta; maxEvents; seekGapSec; windowSec }`; `DEFAULT_AUTOSYNC_CONFIG`; `useSubtitleAutoSync(opts): { autoOffset: Ref<number>; status: Ref<'idle'|'listening'|'locked'|'unsupported'>; confidence: Ref<number>; syncEvents: Ref<SyncEvent[]> }`.

- [ ] **Step 1: Write the failing tests**

```ts
// frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSync.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref, nextTick } from 'vue'
import { useSubtitleAutoSync, type SpeechTap } from '../useSubtitleAutoSync'

function fakeTap() {
  let cb: ((t: number, s: boolean) => void) | null = null
  return {
    onFrame: (fn: (t: number, s: boolean) => void) => { cb = fn },
    dispose() { (this as { disposed: boolean }).disposed = true },
    frame(t: number, s: boolean) { cb?.(t, s) },
    disposed: false,
  } as SpeechTap & { frame(t: number, s: boolean): void; disposed: boolean }
}
function speak(tap: ReturnType<typeof fakeTap>, a: number, b: number) {
  tap.frame(a, true); tap.frame((a + b) / 2, true); tap.frame(b, true); tap.frame(b + 0.05, false)
}
const cfg = { minSpeech: 3, confMin: 0.1, resyncDelta: 0.5, maxEvents: 10, seekGapSec: 1, windowSec: 120 }
const vel = () => ref(document.createElement('video'))

describe('useSubtitleAutoSync', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(1000) })

  it('locks a constant offset once enough confident speech accrues', async () => {
    const tap = fakeTap()
    const cues = ref([{ start: 8, end: 10 }, { start: 18, end: 21 }]) // 2s early vs speech
    const s = useSubtitleAutoSync({ videoElement: vel(), cues, enabled: ref(true), episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)
    await nextTick()
    expect(s.status.value).toBe('locked')
    expect(s.autoOffset.value).toBeCloseTo(2, 1)
    expect(s.syncEvents.value[0]).toMatchObject({ reason: 'lock' })
    expect(s.syncEvents.value[0].delta).toBeCloseTo(2, 1)
  })

  it('does NOT lock below the speech minimum (no-op)', async () => {
    const tap = fakeTap()
    const s = useSubtitleAutoSync({ videoElement: vel(), cues: ref([{ start: 8, end: 10 }]), enabled: ref(true), episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 11)
    await nextTick()
    expect(s.status.value).toBe('listening')
    expect(s.autoOffset.value).toBe(0)
  })

  it('re-syncs on a mid-episode step (eyecatch) and logs reason=resync', async () => {
    const tap = fakeTap()
    const cues = ref([{ start: 8, end: 10 }, { start: 18, end: 21 }, { start: 40, end: 42 }, { start: 50, end: 53 }])
    const s = useSubtitleAutoSync({ videoElement: vel(), cues, enabled: ref(true), episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)
    expect(s.autoOffset.value).toBeCloseTo(2, 1)
    speak(tap, 45, 47); speak(tap, 55, 58)            // cue 40 -> speech 45 ⇒ +5
    await nextTick()
    expect(s.autoOffset.value).toBeCloseTo(5, 1)
    expect(s.syncEvents.value[0].reason).toBe('resync')
  })

  it('resets state and disposes tap on episode change', async () => {
    const tap = fakeTap()
    const ek = ref('a:1')
    const s = useSubtitleAutoSync({ videoElement: vel(), cues: ref([{ start: 8, end: 10 }, { start: 18, end: 21 }]), enabled: ref(true), episodeKey: ek, createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)
    expect(s.autoOffset.value).toBeCloseTo(2, 1)
    ek.value = 'a:2'; await nextTick()
    expect(s.autoOffset.value).toBe(0)
    expect(s.syncEvents.value).toEqual([])
    expect(tap.disposed).toBe(true)
  })

  it('disabling drops autoOffset to 0 and disposes', async () => {
    const tap = fakeTap()
    const enabled = ref(true)
    const s = useSubtitleAutoSync({ videoElement: vel(), cues: ref([{ start: 8, end: 10 }, { start: 18, end: 21 }]), enabled, episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)
    enabled.value = false; await nextTick()
    expect(s.autoOffset.value).toBe(0)
    expect(s.status.value).toBe('idle')
    expect(tap.disposed).toBe(true)
  })

  it('marks unsupported if the tap factory throws', async () => {
    const s = useSubtitleAutoSync({ videoElement: vel(), cues: ref([{ start: 8, end: 10 }]), enabled: ref(true), episodeKey: ref('a:1'), createTap: () => { throw new Error('no audio') }, config: cfg })
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
import { cuesToIntervals, bestOffset, round1, SEARCH, type Interval, type SyncEvent, type SpeechTap } from './subtitleAlign'
import { createAudioTap } from './subtitleAudioTap'

export type { SpeechTap }

export interface AutoSyncConfig {
  minSpeech: number      // seconds of accumulated speech before first lock
  confMin: number        // min peak prominence to act
  resyncDelta: number    // min offset change (s) to adopt a re-sync
  maxEvents: number      // change-log cap
  seekGapSec: number     // frame gap that counts as a seek (don't bridge)
  windowSec: number      // sliding speech-window length kept for correlation
}

export const DEFAULT_AUTOSYNC_CONFIG: AutoSyncConfig = {
  minSpeech: 8, confMin: 0.15, resyncDelta: 0.5, maxEvents: 10, seekGapSec: 1, windowSec: 120,
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

  function pruneWindow() {
    const cutoff = lastT - cfg.windowSec
    if (speech.length && speech[0].end < cutoff) {
      let i = 0
      while (i < speech.length && speech[i].end < cutoff) i++
      speech = speech.slice(i)
    }
  }

  function apply(offset: number, conf: number, reason: 'lock' | 'resync') {
    if (offset === autoOffset.value) return       // bestOffset already rounds to 0.1
    const ev: SyncEvent = {
      delta: round1(offset - autoOffset.value), confidence: conf,
      windowStart: speech[0]?.start ?? 0, windowEnd: lastT, reason,
    }
    syncEvents.value = [ev, ...syncEvents.value].slice(0, cfg.maxEvents)
    autoOffset.value = offset; confidence.value = conf; locked = true; status.value = 'locked'
  }

  function evaluate() {
    if (!locked && totalSpeech < cfg.minSpeech) return    // skip the sweep until warmed up
    if (!speech.length || !cueIntervals.value.length) return
    const r = bestOffset(speech, cueIntervals.value, SEARCH)
    if (!locked) {
      if (r.confidence >= cfg.confMin) apply(r.offset, r.confidence, 'lock')
    } else if (r.confidence >= cfg.confMin && Math.abs(r.offset - autoOffset.value) >= cfg.resyncDelta) {
      apply(r.offset, r.confidence, 'resync')
    }
  }

  function ingest(t: number, speaking: boolean) {
    if (t < lastT || t - lastT > cfg.seekGapSec) {        // seek / discontinuity: close, don't bridge
      if (openStart !== null) { speech.push({ start: openStart, end: lastT }); openStart = null }
    }
    if (speaking) {
      if (openStart === null) openStart = t
    } else if (openStart !== null) {
      const seg = { start: openStart, end: t }
      if (seg.end > seg.start) { speech.push(seg); totalSpeech += seg.end - seg.start }
      openStart = null
      pruneWindow()
      evaluate()
    }
    lastT = t
  }

  function startTap() {
    if (tap || !opts.videoElement.value) return
    try { tap = makeTap(opts.videoElement.value); tap.onFrame(ingest); status.value = 'listening' }
    catch { status.value = 'unsupported' }
  }
  function stopTap() { tap?.dispose(); tap = null }

  function arm() {
    if (opts.enabled.value && opts.videoElement.value) {
      if (status.value !== 'unsupported') startTap()       // startTap owns the listening/unsupported transition
    } else {
      stopTap(); resetData(); status.value = 'idle'
    }
  }

  watch(opts.episodeKey, () => { stopTap(); resetData(); status.value = 'idle'; arm() })
  watch([opts.enabled, opts.videoElement], arm, { immediate: true })
  watch(cueIntervals, () => { if (!locked) evaluate() })   // cues may arrive after speech
  onUnmounted(stopTap)

  return { autoOffset, status, confidence, syncEvents }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/useSubtitleAutoSync.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/useSubtitleAutoSync.ts frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSync.spec.ts
git commit   # "feat(aeplayer): subtitle auto-sync engine (sliding window, lock/resync, change-log)" + co-author trailers
```

---

### Task 6: `SubtitlesMenu.vue` — Auto-sync Switch + hacker panel + i18n

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/SubtitlesMenu.vue`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`
- Test: `frontend/web/src/components/player/aePlayer/__tests__/SubtitlesMenu.spec.ts`

**Interfaces:**
- Consumes: `SyncEvent` from `@/composables/aePlayer/subtitleAlign`; `Switch` from `@/components/ui/Switch.vue`; `fmtResume` from `@/composables/aePlayer/episodeProgress`.
- Produces (new props): `autoSync: boolean`; `autoSyncInfo?: { status: string; offset: number; confidence: number; events: SyncEvent[] } | null`. New emit: `(e: 'update:autoSync', value: boolean): void`.

- [ ] **Step 1: Add i18n keys (all three locales)**

Inside `player.aePlayer.subs` in each file, add:

`en.json`:
```json
"autoSync": "Auto-sync",
"autoSyncHint": "Automatically align subtitles to the audio",
"autoSyncDebug": { "state": "{status} · {offset}s · {conf}%", "event": "{delta}s (VAD) @ {from}–{to} ({conf}%)" }
```
`ru.json`:
```json
"autoSync": "Автосинхрон",
"autoSyncHint": "Автоматически подгонять субтитры под звук",
"autoSyncDebug": { "state": "{status} · {offset}с · {conf}%", "event": "{delta}с (VAD) @ {from}–{to} ({conf}%)" }
```
`ja.json`:
```json
"autoSync": "自動同期",
"autoSyncHint": "字幕を音声に自動的に合わせる",
"autoSyncDebug": { "state": "{status} · {offset}秒 · {conf}%", "event": "{delta}秒 (VAD) @ {from}–{to} ({conf}%)" }
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
const mountMenu = (props = {}) => mount(SubtitlesMenu, { props: { ...base, ...props }, global: { plugins: [i18n] } })

describe('SubtitlesMenu auto-sync', () => {
  it('emits update:autoSync when the switch is toggled (appearance face)', async () => {
    const w = mountMenu()
    await w.find('[data-test="style-toggle"]').trigger('click')
    await w.find('[data-test="autosync-switch"] button').trigger('click')
    expect(w.emitted('update:autoSync')).toBeTruthy()
  })
  it('shows the VAD debug panel only when autoSyncInfo is provided', async () => {
    const w = mountMenu({ autoSyncInfo: { status: 'locked', offset: 2, confidence: 0.8, events: [
      { delta: 2, confidence: 0.8, windowStart: 0, windowEnd: 12, reason: 'lock' },
    ] } })
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
            from: fmtResume(ev.windowStart),
            to: fmtResume(ev.windowEnd),
            conf: Math.round(ev.confidence * 100),
          }) }}
        </div>
      </div>
```

(b) In `<script setup>`, add imports + the type, and extend props/emits:
```ts
import Switch from '@/components/ui/Switch.vue'
import { fmtResume } from '@/composables/aePlayer/episodeProgress'
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
(No `fmtClock` — `fmtResume` already formats seconds → mm:ss.)

- [ ] **Step 5: Run test + DS-lint**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/SubtitlesMenu.spec.ts`
Expected: PASS.
Run: `cd frontend/web && bash scripts/design-system-lint.sh`
Expected: `ERRORS: 0`.

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
- Consumes: `useSubtitleCues`, `useSubtitleAutoSyncPref`, `useSubtitleAutoSync`; existing `videoRef`, `state`, `chosenSubUrl`, `chosenSubFormat`, `subsOn`, `subEpisode`, `props.animeId`.
- Produces: `effectiveOffset` computed bound to `SubtitleOverlay :offset`; new props/handlers on `<SubtitlesMenu>`.

- [ ] **Step 1: Add imports** (near the other composable imports)

```ts
import { useSubtitleCues } from '@/composables/aePlayer/useSubtitleCues'
import { useSubtitleAutoSyncPref } from '@/composables/aePlayer/useSubtitleAutoSyncPref'
import { useSubtitleAutoSync } from '@/composables/aePlayer/useSubtitleAutoSync'
```

- [ ] **Step 2: Wire the engine** — place AFTER `subsOn`, `chosenSubFormat`, and `subEpisode` are defined (around line 2205, just below the existing `subEpisode` computed)

```ts
// ─── Subtitle auto-sync (frontend VAD; spec 2026-06-29) ──────────────────────
const { cues: subtitleCues } = useSubtitleCues(chosenSubUrl, chosenSubFormat)
const autoSyncEpisodeKey = computed(() => `${props.animeId}:${subEpisode.value}`)
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
    ? { status: autoSync.status.value, offset: autoSync.autoOffset.value, confidence: autoSync.confidence.value, events: autoSync.syncEvents.value }
    : null,
)
```

- [ ] **Step 3: Bind into the template**

(a) `SubtitleOverlay` — change `:offset="state.subOffset.value"` to:
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

In `AePlayer.subtitles.spec.ts`, mirroring the file's existing AePlayer mount + `SubtitleOverlay` stub, assert that before any audio lock the overlay's `offset` prop equals the manual `subOffset` (autoOffset starts at 0, so `effectiveOffset === subOffset`). Use the spec file's existing helpers/stub setup.
```ts
it('effective subtitle offset equals the manual offset before any auto lock', async () => {
  // mount AePlayer with a chosen subtitle (existing helper), no audio frames pumped
  // → autoOffset stays 0 → SubtitleOverlay stub receives offset === state.subOffset
})
```

- [ ] **Step 5: Run subtitle tests + type-check**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts`
Expected: PASS.
Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: 0 errors. (Per [[feedback_vuetsc_noemit_false_pass]]: if it passes suspiciously fast, clear the vue-tsc cache and re-run; the menu's object-prop type is imported from `subtitleAlign`, not declared inline.)

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts
git commit   # "feat(aeplayer): apply auto-sync offset on top of manual in SubtitleOverlay" + co-author trailers
```

---

### Task 8: Full frontend verification gate

**Files:** none (verification only).

- [ ] **Step 1: Run the full affected suite**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer src/components/player src/utils src/locales`
Expected: PASS (incl. locale parity — all three locales carry `autoSync`/`autoSyncHint`/`autoSyncDebug.*`).

- [ ] **Step 2: Run `/frontend-verify`**

Invoke the `/frontend-verify` skill (DS-lint, i18n en/ru/ja parity, vue-tsc, real `bun run build`, lucide/TS2614/Tailwind-v4 traps). Fix any finding inline and re-run.

- [ ] **Step 3: Commit any verification fixes**

```bash
git add -A
git commit   # "fix(aeplayer): frontend-verify findings for subtitle auto-sync" + co-author trailers  (skip if clean)
```

---

## Self-Review (completed during planning)

- **Spec coverage:** §3.1 engine → Tasks 1/4/5; `classifyFrame` (pure, tested) → Task 1; `syncEvents` (5-field) → Task 5 + Task 6 panel; §3.2 toggle → Task 2; §3.3 menu UI → Task 6; §3.4 shared `fetchAndParseCues` + overlay delegation + wiring → Tasks 3/7; §3.5 hacker readout → Task 6; §4 manual interaction → Task 7 (`effectiveOffset`) + Task 2 (OFF→0); §6 risks → Task 4 (volume/mute mirror; `unsupported` on tap throw) + sliding window (Task 5) + 20Hz throttle (Task 4); §7 tests → all task test blocks. No gaps.
- **Type consistency:** `Interval`/`OffsetResult`/`SyncEvent`/`SpeechTap` defined once in Task 1, imported by Tasks 4/5/6 (engine re-exports `SpeechTap`); engine output names (`autoOffset`/`status`/`confidence`/`syncEvents`) match across Tasks 5/6/7; `fetchAndParseCues` signature consistent across Tasks 3/7.
- **No stub artifact:** tap (Task 4) precedes engine (Task 5); the engine imports the real `createAudioTap` directly — no throwaway stub.
- **Reuse:** `fmtResume` (not a new formatter), `round1` (one helper), `fetchAndParseCues` (one fetch path used by overlay + composable), `subEpisode` (existing episode-identity computed).
- **Placeholder scan:** Task 7 Step 4 defers to "the spec file's existing mount/stub setup" (local to that test file the executor reads) rather than inventing helper names — intentional; all production code is complete.
