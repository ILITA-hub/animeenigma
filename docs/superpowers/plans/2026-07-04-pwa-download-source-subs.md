# PWA Download Source & Subtitle Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Users pick the provider combo and subtitle track when downloading episodes/seasons; the chosen subs auto-enable at offline playback; opening the app offline lands on /downloads; downloads are Wi-Fi-only by default with an explicit mobile-data confirmation.

**Architecture:** `DownloadDialog.vue` (shared by the in-player and card entry points, stays presentational) gains a compact combo picker + subtitle single-select fed by host-supplied props. Subtitle choice is stored as a `SubPref` descriptor (never a URL — aggregated URLs are per-episode) and resolved per episode by the engine via a caller-supplied `resolveSubs` closure; the matched local track is persisted as `autoSubUrl` and auto-enabled by AePlayer in offline mode. A cellular guard pauses the engine on mobile data (`pausedBy:'network'`, auto-resume on Wi-Fi) behind a session-scoped override. A router guard redirects the initial navigation to /downloads when offline.

**Tech Stack:** Vue 3 + TS, vitest + @vue/test-utils + fake-indexeddb, vue-router 4, Network Information API.

**Spec:** `docs/superpowers/specs/2026-07-04-pwa-download-source-subs-design.md` (read it first).

## Global Constraints

- Frontend commands via `bun`/`bunx` from `frontend/web/` (never npm/npx). Tests: `bunx vitest run <path>`; types: `bunx tsc --noEmit`.
- i18n: every new key lands in ALL THREE locales `src/locales/{en,ru,ja}.json` — parity is redeploy-gating.
- Design system: semantic tokens only (no raw hex/rgba in `.vue`), weights only `font-medium`/`font-semibold`, spacing on the 4px scale. `DownloadDialog.vue` is under `components/player/` → native `<select>`/buttons are EXEMPT from DS Rule 5 (reka portals break in fullscreen).
- NEVER hand Vue-reactive objects to IndexedDB — spread into plain copies first (`{...proxy}`); structured clone throws DataCloneError on any Proxy.
- Never import a NAMED TYPE from a `.vue` file (TS2614 under vue-tsc) — shared types live in `.ts` files.
- Subtitles never auto-enable in ONLINE playback — the offline auto-enable is the only sanctioned exception.
- All paths below are relative to `frontend/web/` unless prefixed with `docs/`.
- Commit after each task, path-scoped (`git add <files>` / `git commit -- <files>`), co-authors `Claude Code <noreply@anthropic.com>`, `0neymik0 <0neymik0@gmail.com>`, `NANDIorg <super.egor.mamonov@yandex.ru>`. Executors COMMIT only — the controller pushes.
- `EpisodeOption.key` is provider-specific — when the download provider differs from the provider whose list produced the episode, re-list episodes via the new provider and remap by `number`.

---

### Task 1: Cellular detection module (`offline/network.ts`)

**Files:**
- Create: `src/offline/network.ts`
- Test: `src/offline/network.spec.ts`

**Interfaces:**
- Produces: `isCellular(): boolean`, `allowCellularThisSession(): boolean`, `setAllowCellularThisSession(v: boolean): void`, `onConnectionChange(cb: () => void): () => void`, `_resetNetworkForTests(): void`.

- [ ] **Step 1: Write the failing test**

```ts
// src/offline/network.spec.ts
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { isCellular, allowCellularThisSession, setAllowCellularThisSession, onConnectionChange, _resetNetworkForTests } from './network'

function stubConnection(value: unknown) {
  Object.defineProperty(navigator, 'connection', { value, configurable: true })
}

describe('offline/network', () => {
  beforeEach(() => _resetNetworkForTests())
  afterEach(() => stubConnection(undefined))

  it('unknown/absent connection type is NOT cellular', () => {
    stubConnection(undefined)
    expect(isCellular()).toBe(false)
    stubConnection({}) // API present, type undefined (desktop Chrome)
    expect(isCellular()).toBe(false)
    stubConnection({ type: 'wifi' })
    expect(isCellular()).toBe(false)
  })

  it('type === cellular is cellular', () => {
    stubConnection({ type: 'cellular' })
    expect(isCellular()).toBe(true)
  })

  it('session override defaults off, sticks after set', () => {
    expect(allowCellularThisSession()).toBe(false)
    setAllowCellularThisSession(true)
    expect(allowCellularThisSession()).toBe(true)
  })

  it('onConnectionChange subscribes when API present, no-ops when absent', () => {
    const add = vi.fn(); const remove = vi.fn()
    stubConnection({ type: 'wifi', addEventListener: add, removeEventListener: remove })
    const off = onConnectionChange(() => {})
    expect(add).toHaveBeenCalledWith('change', expect.any(Function))
    off()
    expect(remove).toHaveBeenCalled()
    stubConnection(undefined)
    expect(() => onConnectionChange(() => {})()).not.toThrow()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/offline/network.spec.ts`
Expected: FAIL — cannot resolve `./network`.

- [ ] **Step 3: Write the implementation**

```ts
// src/offline/network.ts
// Wi-Fi-only download default: cellular detection + the session-scoped
// "download over mobile data anyway" override. Session-scoped on purpose —
// the safe default returns on every app launch.
interface ConnectionLike {
  type?: string
  addEventListener?: (t: 'change', cb: () => void) => void
  removeEventListener?: (t: 'change', cb: () => void) => void
}

function connection(): ConnectionLike | undefined {
  return (navigator as Navigator & { connection?: ConnectionLike }).connection
}

/** True only when the Network Information API positively identifies cellular.
 *  Unknown/absent type (desktop Chrome, Safari, Firefox) → false: never nag
 *  users the API can't classify. */
export function isCellular(): boolean {
  return connection()?.type === 'cellular'
}

let allowCellular = false
export function allowCellularThisSession(): boolean { return allowCellular }
export function setAllowCellularThisSession(v: boolean): void { allowCellular = v }

/** Subscribe to connectivity-type changes; returns an unsubscribe (no-op when the API is absent). */
export function onConnectionChange(cb: () => void): () => void {
  const c = connection()
  if (!c?.addEventListener) return () => {}
  c.addEventListener('change', cb)
  return () => c.removeEventListener?.('change', cb)
}

export function _resetNetworkForTests(): void { allowCellular = false }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bunx vitest run src/offline/network.spec.ts` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/offline/network.ts src/offline/network.spec.ts
git commit -m "feat(offline): cellular detection + session mobile-data override"
```

---

### Task 2: SubPref types + shared subtitle matching (`offline/externalSubs.ts`)

**Files:**
- Modify: `src/offline/types.ts`
- Modify: `src/composables/aePlayer/useSubtitleTracks.ts` (export the flatten)
- Create: `src/offline/externalSubs.ts`
- Test: `src/offline/externalSubs.spec.ts`

**Interfaces:**
- Produces (types.ts): `SubPref` union, `OfflineDownload.subPref?/autoSubUrl?/pausedBy?`, `SubOption { key: string; label: string; pref: SubPref }`.
- Produces (useSubtitleTracks.ts): `flattenAggregateSubs(data: AggregateSubsResponse | null | undefined): SubtitleTrack[]`, exported `AggregateSubsResponse`/`BackendSubTrack` interfaces.
- Produces (externalSubs.ts): `matchAutoSub(pref, subs, streamProvider): string | undefined`, `makeExternalSubResolver(animeId, pref): ((ep: EpisodeOption) => () => Promise<SubtitleTrack[]>) | undefined`.

- [ ] **Step 1: Add the types** (no test needed — consumed by Task 3's tests)

In `src/offline/types.ts` add after `DownloadError`:

```ts
/** Download-time subtitle choice. A DESCRIPTOR, never a URL — aggregated
 *  track URLs are per-episode and signed URLs expire in queue; the engine
 *  re-resolves the concrete track for each episode. */
export type SubPref =
  | { kind: 'bundled'; lang: string } // lang 'auto' = first provider-bundled track
  | { kind: 'external'; provider: string; lang: string; label?: string }

/** One entry of the download dialog's subtitle picker (built by the hosts —
 *  labels are i18n'd there; this stays a plain data shape). */
export interface SubOption { key: string; label: string; pref: SubPref }
```

And inside `OfflineDownload` after `subtitles`:

```ts
  /** Download-time subtitle choice (descriptor; see SubPref). */
  subPref?: SubPref
  /** Local /__offline/{id}/sub/{k} URL of the track matching subPref —
   *  offline playback auto-enables exactly this track. */
  autoSubUrl?: string
  /** 'network' when the cellular guard parked it (auto-resumed on Wi-Fi).
   *  Manual pauses leave it unset and are never auto-resumed. */
  pausedBy?: 'network'
```

- [ ] **Step 2: Extract + export the aggregate flatten from `useSubtitleTracks.ts`**

Move the two private interfaces to exports and pull the flatten out of `fetchFor` (behavior identical — `fetchFor` calls it):

```ts
export interface BackendSubTrack {
  url: string; lang: string; label: string; format?: string; provider: string; release?: string
}
export interface AggregateSubsResponse {
  languages: Record<string, BackendSubTrack[]>
  episode: number
  providers_down?: string[]
}

/** Flatten the /subtitles/all languages map into SubtitleTrack[] — shared
 *  with the offline download engine (external subtitle capture). */
export function flattenAggregateSubs(data: AggregateSubsResponse | null | undefined): SubtitleTrack[] {
  const flat: SubtitleTrack[] = []
  for (const [lang, list] of Object.entries(data?.languages ?? {})) {
    for (const t of list) {
      flat.push({
        url: t.url,
        provider: t.provider,
        lang: t.lang || lang,
        label: t.label || t.release || t.provider,
        format: (t.format || t.url.split('?')[0].split('.').pop() || 'srt').toLowerCase(),
      })
    }
  }
  return flat
}
```

In `fetchFor`, replace the inline loop with `aggTracks.value = flattenAggregateSubs(data)` (keep `providersDown` handling as-is).

- [ ] **Step 3: Write the failing test**

```ts
// src/offline/externalSubs.spec.ts
import { describe, it, expect, vi } from 'vitest'
import type { SubtitleTrack } from '@/types/aePlayer'
import { matchAutoSub, makeExternalSubResolver } from './externalSubs'

vi.mock('@/api/client', () => ({
  subtitlesApi: {
    all: vi.fn(async (_id: string, ep: number) => ({
      data: { data: { languages: { ja: [
        { url: `/sub/jimaku-${ep}.ass`, lang: 'ja', label: 'Jimaku A', provider: 'jimaku', format: 'ass' },
        { url: `/sub/jimaku2-${ep}.srt`, lang: 'ja', label: 'Jimaku B', provider: 'jimaku' },
      ] }, episode: ep } },
    })),
  },
}))

const tr = (p: Partial<SubtitleTrack>): SubtitleTrack =>
  ({ url: 'u', provider: 'gogoanime', lang: 'en', label: 'L', format: 'vtt', ...p })

describe('matchAutoSub', () => {
  const subs = [
    tr({ url: '/__offline/x/sub/0', provider: 'gogoanime', lang: 'en' }),
    tr({ url: '/__offline/x/sub/1', provider: 'gogoanime', lang: 'ru' }),
    tr({ url: '/__offline/x/sub/2', provider: 'jimaku', lang: 'ja', label: 'Jimaku A' }),
  ]
  it('no pref → undefined', () => expect(matchAutoSub(undefined, subs, 'gogoanime')).toBeUndefined())
  it('bundled auto → first stream-provider track', () =>
    expect(matchAutoSub({ kind: 'bundled', lang: 'auto' }, subs, 'gogoanime')).toBe('/__offline/x/sub/0'))
  it('bundled concrete lang → lang match only among bundled', () => {
    expect(matchAutoSub({ kind: 'bundled', lang: 'ru' }, subs, 'gogoanime')).toBe('/__offline/x/sub/1')
    expect(matchAutoSub({ kind: 'bundled', lang: 'ja' }, subs, 'gogoanime')).toBeUndefined()
  })
  it('external → provider+lang (label preferred)', () =>
    expect(matchAutoSub({ kind: 'external', provider: 'jimaku', lang: 'ja', label: 'Jimaku A' }, subs, 'gogoanime'))
      .toBe('/__offline/x/sub/2'))
})

describe('makeExternalSubResolver', () => {
  it('non-external pref → undefined', () => {
    expect(makeExternalSubResolver('a1', null)).toBeUndefined()
    expect(makeExternalSubResolver('a1', { kind: 'bundled', lang: 'auto' })).toBeUndefined()
  })
  it('fetches per-episode and matches by provider+lang+label', async () => {
    const forEp = makeExternalSubResolver('a1', { kind: 'external', provider: 'jimaku', lang: 'ja', label: 'Jimaku B' })!
    const got = await forEp({ key: 5, label: 5, number: 5 })()
    expect(got).toHaveLength(1)
    expect(got[0].url).toBe('/sub/jimaku2-5.srt')
  })
  it('no match for the episode → empty list', async () => {
    const forEp = makeExternalSubResolver('a1', { kind: 'external', provider: 'opensubtitles', lang: 'en' })!
    expect(await forEp({ key: 5, label: 5, number: 5 })()).toEqual([])
  })
})
```

- [ ] **Step 4: Run test to verify it fails**

Run: `bunx vitest run src/offline/externalSubs.spec.ts` — Expected: FAIL (module missing).

- [ ] **Step 5: Write the implementation**

```ts
// src/offline/externalSubs.ts
import type { SubtitleTrack } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import { subtitlesApi } from '@/api/client'
import { flattenAggregateSubs, type AggregateSubsResponse } from '@/composables/aePlayer/useSubtitleTracks'
import type { SubPref } from './types'

function matchExternal(tracks: SubtitleTrack[], pref: Extract<SubPref, { kind: 'external' }>): SubtitleTrack | undefined {
  const same = tracks.filter((t) => t.provider === pref.provider && t.lang === pref.lang)
  return same.find((t) => t.label === pref.label) ?? same[0]
}

/** Local cached URL of the track the user asked to auto-enable, resolved
 *  against the download's cached track list. Missing match (track absent for
 *  this episode, fetch failed) → undefined; the download itself never fails. */
export function matchAutoSub(pref: SubPref | null | undefined, subs: SubtitleTrack[], streamProvider: string): string | undefined {
  if (!pref) return undefined
  if (pref.kind === 'external') return matchExternal(subs, pref)?.url
  const bundled = subs.filter((s) => s.provider === streamProvider)
  return (pref.lang === 'auto' ? bundled[0] : bundled.find((s) => s.lang === pref.lang))?.url
}

/** Per-episode fetch closure factory for external tracks — aggregated
 *  /subtitles/all URLs are episode-specific, so a season batch re-queries per
 *  episode. Returns undefined for non-external prefs (bundled tracks already
 *  ride the resolved stream). */
export function makeExternalSubResolver(
  animeId: string,
  pref: SubPref | null | undefined,
): ((ep: EpisodeOption) => () => Promise<SubtitleTrack[]>) | undefined {
  if (pref?.kind !== 'external') return undefined
  return (ep) => async () => {
    const resp = await subtitlesApi.all(animeId, ep.number)
    const data = (resp.data?.data ?? resp.data) as AggregateSubsResponse
    const hit = matchExternal(flattenAggregateSubs(data), pref)
    return hit ? [hit] : []
  }
}
```

- [ ] **Step 6: Run tests + verify no regressions**

Run: `bunx vitest run src/offline/externalSubs.spec.ts src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts` — Expected: PASS.
Run: `bunx tsc --noEmit` — Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add src/offline/types.ts src/offline/externalSubs.ts src/offline/externalSubs.spec.ts src/composables/aePlayer/useSubtitleTracks.ts
git commit -m "feat(offline): SubPref descriptor types + shared external-subtitle matching"
```

---

### Task 3: Engine — external subtitle capture + autoSubUrl

**Files:**
- Modify: `src/offline/downloadEngine.ts`
- Modify: `src/offline/seasonDownload.ts`
- Modify: `src/stores/downloads.ts` (resume threads subPref)
- Test: `src/offline/downloadEngine.spec.ts` (add cases)

**Interfaces:**
- Consumes: `matchAutoSub`, `makeExternalSubResolver`, `SubPref` (Task 2).
- Produces: `DownloadRequest.subPref?: SubPref` and `DownloadRequest.resolveSubs?: () => Promise<SubtitleTrack[]>`; `SeasonContext.subPref?: SubPref` and `SeasonContext.resolveSubsFor?: (ep: EpisodeOption) => () => Promise<SubtitleTrack[]>`; records persist `subPref` + `autoSubUrl`.

- [ ] **Step 1: Write the failing tests** (append to `downloadEngine.spec.ts`, reusing its `fakeCaches`/`mockFetch`/`req` helpers and MASTER/MEDIA fixtures)

```ts
describe('external subtitles + autoSubUrl', () => {
  const STREAM: StreamResult = {
    url: 'https://cdn.example/master.m3u8', type: 'hls',
    subtitles: [{ url: 'https://cdn.example/en.vtt', provider: 'gogoanime', lang: 'en', label: 'EN', format: 'vtt' }],
  } as StreamResult

  function routes() {
    return {
      'master.m3u8': () => new Response(MASTER),
      'index.m3u8': () => new Response(MEDIA),
      's0.ts': () => new Response('aa'), 's1.ts': () => new Response('bb'),
      'en.vtt': () => new Response('WEBVTT'), 'jp.ass': () => new Response('[Script Info]'),
    }
  }

  it('caches the external track and stamps autoSubUrl from the pref', async () => {
    vi.stubGlobal('fetch', mockFetch(routes()))
    const id = await enqueueDownload({
      ...req(async () => STREAM),
      subPref: { kind: 'external', provider: 'jimaku', lang: 'ja', label: 'J' },
      resolveSubs: async () => [{ url: 'https://jimaku.cc/jp.ass', provider: 'jimaku', lang: 'ja', label: 'J', format: 'ass' }],
    })
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'))
    const rec = (await getDownload(id))!
    expect(rec.subtitles).toHaveLength(2) // bundled EN + external JA, both local paths
    expect(rec.subtitles.every((s) => s.url.startsWith('/__offline/'))).toBe(true)
    expect(rec.autoSubUrl).toBe(rec.subtitles.find((s) => s.provider === 'jimaku')!.url)
    expect(rec.subPref).toEqual({ kind: 'external', provider: 'jimaku', lang: 'ja', label: 'J' })
  })

  it('resolveSubs failure is non-fatal: download done, autoSubUrl unset', async () => {
    vi.stubGlobal('fetch', mockFetch(routes()))
    const id = await enqueueDownload({
      ...req(async () => STREAM),
      subPref: { kind: 'external', provider: 'jimaku', lang: 'ja' },
      resolveSubs: async () => { throw new Error('api down') },
    })
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'))
    expect((await getDownload(id))!.autoSubUrl).toBeUndefined()
  })

  it('bundled pref matches the stream-provider track', async () => {
    vi.stubGlobal('fetch', mockFetch(routes()))
    const id = await enqueueDownload({ ...req(async () => STREAM), subPref: { kind: 'bundled', lang: 'auto' } })
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'))
    const rec = (await getDownload(id))!
    expect(rec.autoSubUrl).toBe(rec.subtitles[0].url)
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `bunx vitest run src/offline/downloadEngine.spec.ts` — Expected: new cases FAIL (subPref/resolveSubs unknown, autoSubUrl undefined).

- [ ] **Step 3: Implement in `downloadEngine.ts`**

1. Imports: add `import { matchAutoSub } from './externalSubs'` and extend the types import: `import { downloadId, offlinePath, type DownloadError, type OfflineDownload, type SubPref } from './types'`.
2. `DownloadRequest` — add after `resolve`:

```ts
  /** Download-time subtitle choice; persisted on the record and matched to a cached track (autoSubUrl). */
  subPref?: SubPref
  /** Fetches the external track(s) matching subPref for THIS episode — aggregated URLs are per-episode. */
  resolveSubs?: () => Promise<SubtitleTrack[]>
```

3. In `runDownload`, replace `const localSubs = await cacheSubtitles(id, stream.subtitles ?? [])` with:

```ts
  // External track (Jimaku/OpenSubtitles) rides along when the user picked one;
  // its failure is as non-fatal as a missing bundled track.
  const external = req.resolveSubs ? await req.resolveSubs().catch(() => [] as SubtitleTrack[]) : []
  const localSubs = await cacheSubtitles(id, [...(stream.subtitles ?? []), ...external])
  const autoSubUrl = matchAutoSub(req.subPref, localSubs, req.combo.provider)
```

4. In the `update` closure, extend the `putDownload` payload: `subtitles: localSubs, autoSubUrl,` (next to the existing `subtitles: localSubs`).
5. In `enqueueDownload`'s `baseRecord`, add (spread-copied — DataCloneError guard): `subPref: req.subPref ? { ...req.subPref } : undefined,`.

- [ ] **Step 4: Thread through `seasonDownload.ts`**

`SeasonContext` — add after `resolveFor`:

```ts
  /** Frozen once for the batch, like combo. */
  subPref?: SubPref
  /** Per-episode external-subtitle closure factory (see DownloadRequest.resolveSubs). */
  resolveSubsFor?: (ep: EpisodeOption) => () => Promise<SubtitleTrack[]>
```

In `enqueueSeason`'s `enqueueDownload` call add: `subPref: ctx.subPref, resolveSubs: ctx.resolveSubsFor?.(ep),`. Import `SubPref` from `./types` and `SubtitleTrack` from `@/types/aePlayer` (type-only).

- [ ] **Step 5: Thread through the store resume (`src/stores/downloads.ts`)**

In `resume(d)`'s `enqueueDownload` call add:

```ts
      subPref: d.subPref,
      resolveSubs: makeExternalSubResolver(d.animeId, d.subPref)?.(d.episode),
```

with `import { makeExternalSubResolver } from '@/offline/externalSubs'`.

- [ ] **Step 6: Run tests**

Run: `bunx vitest run src/offline/` — Expected: ALL PASS (old + new).
Run: `bunx tsc --noEmit` — Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add src/offline/downloadEngine.ts src/offline/downloadEngine.spec.ts src/offline/seasonDownload.ts src/stores/downloads.ts
git commit -m "feat(offline): engine captures the chosen external subtitle track + stamps autoSubUrl"
```

---

### Task 4: Engine — cellular gate + guard (`offline/cellularGuard.ts`)

**Files:**
- Modify: `src/offline/downloadEngine.ts`
- Create: `src/offline/cellularGuard.ts`
- Test: `src/offline/downloadEngine.spec.ts` (add cases), `src/offline/cellularGuard.spec.ts`

**Interfaces:**
- Consumes: Task 1's `network.ts`, Task 3's request fields.
- Produces: `engineState.cellularPauses: Ref<number>` (bumps once per auto-pause event); `pauseAllForCellular(): Promise<void>`; `ensureCellularGuard(): void`, `resumeNetworkPaused(): Promise<number>`, `allowCellularAndResume(): Promise<void>` (cellularGuard.ts).

- [ ] **Step 1: Write the failing engine-gate test** (append to `downloadEngine.spec.ts`)

```ts
import * as network from './network'

describe('cellular gate', () => {
  it('on cellular without override: record parks as paused/pausedBy:network, resolve() never called', async () => {
    vi.spyOn(network, 'isCellular').mockReturnValue(true)
    vi.spyOn(network, 'allowCellularThisSession').mockReturnValue(false)
    const resolve = vi.fn(async () => ({ url: 'x', type: 'hls' }) as StreamResult)
    const id = await enqueueDownload(req(resolve))
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('paused'))
    expect((await getDownload(id))?.pausedBy).toBe('network')
    expect(resolve).not.toHaveBeenCalled()
    vi.restoreAllMocks()
  })

  it('with the session override the gate is open', async () => {
    vi.spyOn(network, 'isCellular').mockReturnValue(true)
    vi.spyOn(network, 'allowCellularThisSession').mockReturnValue(true)
    vi.stubGlobal('fetch', mockFetch({
      'master.m3u8': () => new Response(MASTER), 'index.m3u8': () => new Response(MEDIA),
      's0.ts': () => new Response('aa'), 's1.ts': () => new Response('bb'),
    }))
    const id = await enqueueDownload(req(async () => ({ url: 'https://cdn.example/master.m3u8', type: 'hls' }) as StreamResult))
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'))
    vi.restoreAllMocks()
  })
})
```

Note: for `vi.spyOn(network, …)` to work the engine must call through the module namespace — implement the gate with `import * as network from './network'` (or `vi.mock('./network')` per test instead; follow whichever the file already uses for registerPwa-style seams — a namespace import is the simplest spy-able seam).

- [ ] **Step 2: Run to verify failure**

Run: `bunx vitest run src/offline/downloadEngine.spec.ts` — Expected: new cases FAIL.

- [ ] **Step 3: Implement the engine side**

In `downloadEngine.ts`:

1. `import * as network from './network'`.
2. `engineState` gains `cellularPauses: ref(0)` (type `Ref<number>` in the exported annotation).
3. Lazy guard install next to the existing registerPwa side effect:

```ts
// Installed lazily for the same reason as the registerPwa probe: the guard
// imports the resolver chain, which must not become a static engine dep.
void import('./cellularGuard').then((m) => m.ensureCellularGuard())
```

4. Gate at the very top of `runDownload`, right after the `if (!record) return`:

```ts
  // Wi-Fi-only default: park instead of downloading on mobile data. Sits
  // before resolve() so a starved item never burns a scraper resolution.
  if (network.isCellular() && !network.allowCellularThisSession()) {
    await putDownload({ ...record, state: 'paused', pausedBy: 'network' })
    return
  }
```

5. New export:

```ts
/** Cellular guard entry: park the active download and everything queued as
 *  pausedBy:'network' (the guard auto-resumes them on Wi-Fi). Bumps
 *  cellularPauses once per event — UI toasts key off it. */
export async function pauseAllForCellular(): Promise<void> {
  let parked = 0
  const park = async (id: string, toPaused: boolean) => {
    const cur = await getDownload(id)
    if (!cur) return
    await putDownload({ ...cur, state: toPaused ? 'paused' : cur.state, pausedBy: 'network' })
    parked++
  }
  const active = engineState.activeId.value
  if (active) {
    paused.add(active) // worker exits at the next item boundary → its update('paused') spread preserves pausedBy
    await park(active, false)
  }
  while (queue.length > 0) await park(queue.shift()!.id, true)
  if (parked > 0) engineState.cellularPauses.value++
}
```

Also confirm `enqueueDownload`'s full-record `putDownload({ ...baseRecord, state: 'queued' })` drops `pausedBy` on re-enqueue (it does — `baseRecord` never contains it; note it in a comment).

- [ ] **Step 4: Write the failing guard test**

```ts
// src/offline/cellularGuard.spec.ts
import 'fake-indexeddb/auto'
import { describe, it, expect, vi, beforeEach } from 'vitest'

const enqueueDownload = vi.fn(async () => 'id')
vi.mock('./downloadEngine', () => ({
  enqueueDownload, isEngineWorking: () => false, pauseAllForCellular: vi.fn(),
}))
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({ resolveStream: vi.fn(async () => ({ url: 'u', type: 'hls' })) }),
}))
import { resumeNetworkPaused } from './cellularGuard'
import { _resetDbForTests, putDownload } from './registry'
import type { OfflineDownload } from './types'

const rec = (over: Partial<OfflineDownload>): OfflineDownload => ({
  id: over.id ?? 'a:1', animeId: 'a', animeTitle: 'T',
  episode: { key: 1, label: 1, number: 1 }, quality: '720', streamType: 'hls',
  combo: { audio: 'sub', lang: 'en', provider: 'gogo', server: '', team: null },
  state: 'paused', bytes: 0, resourcesDone: 0, resourcesTotal: 0, createdAt: 1,
  playlistLocalPath: '/__offline/a%3A1/master.m3u8', subtitles: [], ...over,
})

describe('resumeNetworkPaused', () => {
  beforeEach(async () => { await _resetDbForTests(); enqueueDownload.mockClear() })

  it('re-enqueues only pausedBy:network records, rebuilding closures', async () => {
    await putDownload(rec({ id: 'net', pausedBy: 'network' }))
    await putDownload(rec({ id: 'manual' })) // user-paused: stays parked
    await putDownload(rec({ id: 'done', state: 'done', pausedBy: 'network' })) // stale flag on a finished record
    const n = await resumeNetworkPaused()
    expect(n).toBe(1)
    expect(enqueueDownload).toHaveBeenCalledTimes(1)
    expect(enqueueDownload.mock.calls[0][0]).toMatchObject({ animeId: 'a', subPref: undefined })
  })
})
```

- [ ] **Step 5: Implement the guard**

```ts
// src/offline/cellularGuard.ts
// Wi-Fi-only default, part 2: reacts to connectivity-type changes. Loaded
// lazily by the engine (mirrors the registerPwa probe) so the resolver chain
// never becomes a static engine dependency in reverse.
import { onConnectionChange, isCellular, allowCellularThisSession, setAllowCellularThisSession } from './network'
import { enqueueDownload, isEngineWorking, pauseAllForCellular } from './downloadEngine'
import { listDownloads } from './registry'
import { makeExternalSubResolver } from './externalSubs'
import { useProviderResolver } from '@/composables/aePlayer/useProviderResolver'

let installed = false
/** Idempotent — the engine installs it on first load. */
export function ensureCellularGuard(): void {
  if (installed) return
  installed = true
  onConnectionChange(() => {
    if (isCellular()) {
      if (!allowCellularThisSession()) void pauseAllForCellular()
    } else {
      void resumeNetworkPaused()
    }
  })
}

/** Re-enqueue every record the guard parked (pausedBy:'network'), rebuilding
 *  the resolve closures from the persisted combo/subPref — the exact recipe
 *  of the store's manual resume. Returns how many were released. */
export async function resumeNetworkPaused(): Promise<number> {
  const resolver = useProviderResolver()
  let n = 0
  for (const d of await listDownloads()) {
    if (d.pausedBy !== 'network' || d.state !== 'paused' || isEngineWorking(d.id)) continue
    await enqueueDownload({
      animeId: d.animeId, animeTitle: d.animeTitle, episode: d.episode, combo: d.combo,
      quality: d.quality, subPref: d.subPref,
      resolve: () => resolver.resolveStream(d.combo.provider, d.animeId, d.episode, d.combo),
      resolveSubs: makeExternalSubResolver(d.animeId, d.subPref)?.(d.episode),
    })
    n++
  }
  return n
}

/** «Качать по мобильным данным» — set the session override and release parked records. */
export async function allowCellularAndResume(): Promise<void> {
  setAllowCellularThisSession(true)
  await resumeNetworkPaused()
}
```

- [ ] **Step 6: Run tests**

Run: `bunx vitest run src/offline/` — Expected: ALL PASS. `bunx tsc --noEmit` — clean.

- [ ] **Step 7: Commit**

```bash
git add src/offline/downloadEngine.ts src/offline/downloadEngine.spec.ts src/offline/cellularGuard.ts src/offline/cellularGuard.spec.ts
git commit -m "feat(offline): Wi-Fi-only default — cellular gate, network pause, Wi-Fi auto-resume"
```

---

### Task 5: DownloadDialog — source (combo) picker

**Files:**
- Modify: `src/components/player/aePlayer/DownloadDialog.vue`
- Modify: `src/locales/en.json`, `src/locales/ru.json`, `src/locales/ja.json`
- Test: `src/components/player/aePlayer/DownloadDialog.spec.ts` (add cases)

**Interfaces:**
- Consumes: `rowsFromReport` (`@/composables/aePlayer/useProviderFeed`), `pickSmartDefault`/`pickSelectableFallback` (`@/composables/aePlayer/smartDefault`), `GROUP_PRIMARY_LANG` (`@/composables/aePlayer/providerGroups`), types `Combo/AudioKind/TrackLang/ContentKind/ProviderRow` (`@/types/aePlayer`), `CapabilityReport` (`@/types/capabilities`).
- Produces: new optional props `report?: CapabilityReport | null`, `initialCombo?: Combo | null`, `loadTeams?: (provider: string, audio: AudioKind) => Promise<string[]>`; emit becomes `confirm(quality: string, scope: 'episode'|'season', combo: Combo | null, subPref: SubPref | null)` (subPref stays `null` until Task 6 — emit the 4-arg shape NOW so hosts wire once). When `report`/`initialCombo` are absent the source section is hidden and `combo` emits as `null` → hosts fall back to their previous default (backward compatible: both hosts keep working before Tasks 7–8 land).

- [ ] **Step 1: Write the failing tests** (append to `DownloadDialog.spec.ts`; extend `mountDlg` to accept the new props)

```ts
import type { CapabilityReport } from '@/types/capabilities'
import type { Combo } from '@/types/aePlayer'

const REPORT = {
  anime_id: 'a1',
  families: [{ providers: [
    { provider: 'gogoanime', display_name: 'Gogoanime', group: 'en', state: 'active', selectable: true, hacker_only: false, order: 90, audios: ['sub', 'dub'] },
    { provider: 'animepahe', display_name: 'AnimePahe', group: 'en', state: 'active', selectable: true, hacker_only: false, order: 70, audios: ['sub', 'dub'] },
    { provider: 'kodik', display_name: 'Kodik', group: 'ru', state: 'active', selectable: true, hacker_only: false, order: 80, audios: ['dub', 'sub'] },
  ] }],
} as unknown as CapabilityReport
// NOTE: a group-'en' provider is NOT in the dub/ru row list (GROUP_LANGS) —
// provider switching is tested between the two 'en' providers, and the lang
// switch tests the re-default to kodik.
const COMBO: Combo = { audio: 'dub', lang: 'en', provider: 'gogoanime', server: '', team: null }

describe('source picker', () => {
  it('hidden without report/initialCombo; emits null combo', async () => {
    const w = mountDlg()
    expect(w.find('[data-test="dl-provider"]').exists()).toBe(false)
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')![0][2]).toBeNull()
  })

  it('renders providers for the initial audio/lang and emits the edited combo', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO })
    const sel = w.find('[data-test="dl-provider"]')
    expect(sel.findAll('option').map((o) => o.attributes('value'))).toEqual(['gogoanime', 'animepahe'])
    await sel.setValue('animepahe')
    await w.find('[data-test="dl-start"]').trigger('click')
    const combo = w.emitted('confirm')![0][2] as Combo
    expect(combo.provider).toBe('animepahe')
    expect(combo.team).toBeNull()
  })

  it('lang switch re-defaults a filtered-out provider', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO })
    await w.find('[data-test="dl-lang-ru"]').trigger('click')
    await w.find('[data-test="dl-start"]').trigger('click')
    const combo = w.emitted('confirm')![0][2] as Combo
    expect(combo.provider).toBe('kodik') // gogoanime has no RU dub rows
    expect(combo.lang).toBe('ru')
  })

  it('RAW switch drops lang filter and derives lang from provider group', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO })
    await w.find('[data-test="dl-audio-sub"]').trigger('click')
    await w.find('[data-test="dl-start"]').trigger('click')
    const combo = w.emitted('confirm')![0][2] as Combo
    expect(combo.audio).toBe('sub')
    expect(['en', 'ru']).toContain(combo.lang) // GROUP_PRIMARY_LANG of the picked row's group
  })

  it('team select appears when loadTeams returns teams; Авто = null', async () => {
    const w = mountDlg({ report: REPORT, initialCombo: COMBO, loadTeams: async () => ['AniLibria', 'Dream Cast'] })
    await vi.waitFor(() => expect(w.find('[data-test="dl-team"]').exists()).toBe(true))
    await w.find('[data-test="dl-team"]').setValue('AniLibria')
    await w.find('[data-test="dl-start"]').trigger('click')
    expect((w.emitted('confirm')![0][2] as Combo).team).toBe('AniLibria')
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `bunx vitest run src/components/player/aePlayer/DownloadDialog.spec.ts` — Expected: new cases FAIL.

- [ ] **Step 3: Implement**

Script changes (`DownloadDialog.vue`):

```ts
import { rowsFromReport } from '@/composables/aePlayer/useProviderFeed'
import { pickSmartDefault, pickSelectableFallback } from '@/composables/aePlayer/smartDefault'
import { GROUP_PRIMARY_LANG } from '@/composables/aePlayer/providerGroups'
import type { Combo, AudioKind, TrackLang, ContentKind, ProviderRow } from '@/types/aePlayer'
import type { CapabilityReport } from '@/types/capabilities'
import type { SubPref } from '@/offline/types'
```

Props additions (same `withDefaults` block): `report?: CapabilityReport | null`, `initialCombo?: Combo | null`, `loadTeams?: (provider: string, audio: AudioKind) => Promise<string[]>` — defaults `report: null, initialCombo: null, loadTeams: undefined`.

Emit: `(e: 'confirm', quality: string, scope: 'episode' | 'season', combo: Combo | null, subPref: SubPref | null): void`.

```ts
const combo = ref<Combo | null>(props.initialCombo ? { ...props.initialCombo } : null)
const showSource = computed(() => !!props.report && combo.value !== null)

function rowsFor(audio: AudioKind, lang: TrackLang): ProviderRow[] {
  // Same content fallback as pickDefaultCombo: hentai rows only when common has none.
  for (const content of ['common', 'hentai'] as ContentKind[]) {
    const rows = rowsFromReport(props.report ?? null, { audio, lang, content })
    if (rows.length > 0) return rows
  }
  return []
}
const rows = computed(() => (combo.value ? rowsFor(combo.value.audio, combo.value.lang) : []))
const dubAvailable = computed(() => rowsFor('dub', 'ru').length > 0 || rowsFor('dub', 'en').length > 0)
const rawAvailable = computed(() => rowsFor('sub', 'en').length > 0)

const teams = ref<string[]>([])
async function refreshTeams(): Promise<void> {
  teams.value = []
  const c = combo.value
  if (!c?.provider || !props.loadTeams) return
  try { teams.value = await props.loadTeams(c.provider, c.audio) } catch { teams.value = [] }
}
onMounted(() => { void refreshTeams() })

/** Keep the picked provider when it survives the new filter, else re-default
 *  the same way the player's smart default does. */
function applyFilter(audio: AudioKind, lang: TrackLang): void {
  const c = combo.value
  if (!c) return
  const rs = rowsFor(audio, lang)
  const row = rs.find((r) => r.id === c.provider) ?? pickSmartDefault(rs) ?? pickSelectableFallback(rs)
  if (!row) return // nothing under this filter — leave the combo untouched
  const nextLang = audio === 'sub' ? GROUP_PRIMARY_LANG[row.group] : lang
  const providerChanged = row.id !== c.provider
  combo.value = { ...c, audio, lang: nextLang, provider: row.id, team: providerChanged ? null : c.team }
  if (providerChanged) void refreshTeams()
}
function setAudio(audio: AudioKind): void {
  const lang: TrackLang = audio === 'dub' ? (combo.value?.lang === 'ru' ? 'ru' : 'en') : (combo.value?.lang ?? 'en')
  applyFilter(audio, lang)
}
function setProvider(id: string): void {
  const c = combo.value
  const row = rows.value.find((r) => r.id === id)
  if (!c || !row) return
  combo.value = { ...c, provider: id, lang: c.audio === 'sub' ? GROUP_PRIMARY_LANG[row.group] : c.lang, team: null }
  void refreshTeams()
}
function setTeam(v: string): void {
  if (combo.value) combo.value = { ...combo.value, team: v === '' ? null : v }
}
```

`confirm()` becomes (subPref stays null until Task 6):

```ts
function confirm() {
  localStorage.setItem(LS_KEY, quality.value)
  emit('confirm', quality.value, scope.value, combo.value ? { ...combo.value } : null, null)
}
```

Template — insert BEFORE the quality title (`dl-title` for quality):

```html
    <template v-if="showSource">
      <div class="dl-title text-sm font-semibold">{{ $t('player.aePlayer.offline.source') }}</div>
      <div class="dl-opts">
        <button type="button" class="dl-opt" :class="{ 'dl-opt-active': combo!.audio === 'sub' }"
          :disabled="!rawAvailable" data-test="dl-audio-sub" @click="setAudio('sub')">{{ $t('player.aePlayer.offline.audioRaw') }}</button>
        <button type="button" class="dl-opt" :class="{ 'dl-opt-active': combo!.audio === 'dub' }"
          :disabled="!dubAvailable" data-test="dl-audio-dub" @click="setAudio('dub')">{{ $t('player.aePlayer.offline.audioDub') }}</button>
      </div>
      <div v-if="combo!.audio === 'dub'" class="dl-opts">
        <button type="button" class="dl-opt" :class="{ 'dl-opt-active': combo!.lang === 'ru' }" data-test="dl-lang-ru" @click="applyFilter('dub', 'ru')">RU</button>
        <button type="button" class="dl-opt" :class="{ 'dl-opt-active': combo!.lang === 'en' }" data-test="dl-lang-en" @click="applyFilter('dub', 'en')">EN</button>
      </div>
      <select class="dl-select" :value="combo!.provider" data-test="dl-provider"
        @change="setProvider(($event.target as HTMLSelectElement).value)">
        <option v-for="r in rows" :key="r.id" :value="r.id" :disabled="!r.selectable">{{ r.label }}</option>
      </select>
      <select v-if="teams.length > 0" class="dl-select" :value="combo!.team ?? ''" data-test="dl-team"
        @change="setTeam(($event.target as HTMLSelectElement).value)">
        <option value="">{{ $t('player.aePlayer.offline.teamAuto') }}</option>
        <option v-for="tm in teams" :key="tm" :value="tm">{{ tm }}</option>
      </select>
    </template>
```

Scoped style addition (token-only):

```css
.dl-select {
  width: 100%;
  margin-bottom: 0.75rem;
  padding: 0.375rem 0.625rem;
  border-radius: 0.375rem;
  border: 1px solid var(--white-a8);
  background: transparent;
  color: var(--foreground);
  font-size: 0.8125rem;
}
```

i18n — add to `player.aePlayer.offline` in ALL THREE locales:

| key | en | ru | ja |
|---|---|---|---|
| `source` | Source | Источник | ソース |
| `audioRaw` | RAW | RAW | RAW |
| `audioDub` | Dub | Озвучка | 吹き替え |
| `teamAuto` | Auto | Авто | 自動 |

- [ ] **Step 4: Run tests**

Run: `bunx vitest run src/components/player/aePlayer/DownloadDialog.spec.ts` — Expected: ALL PASS (old cases still emit `[quality, scope, null, null]` — update any assertion that pinned `emitted('confirm')![0].length`).
Run: `bunx vitest run src/locales/` — locale parity suites PASS.

- [ ] **Step 5: Commit**

```bash
git add src/components/player/aePlayer/DownloadDialog.vue src/components/player/aePlayer/DownloadDialog.spec.ts src/locales/en.json src/locales/ru.json src/locales/ja.json
git commit -m "feat(offline): source combo picker in the download dialog"
```

---

### Task 6: DownloadDialog — subtitle select + mobile-data confirm step

**Files:**
- Modify: `src/components/player/aePlayer/DownloadDialog.vue`
- Modify: `src/locales/en.json`, `src/locales/ru.json`, `src/locales/ja.json`
- Test: `src/components/player/aePlayer/DownloadDialog.spec.ts` (add cases)

**Interfaces:**
- Consumes: `SubOption` (Task 2 types), `isCellular`/`allowCellularThisSession`/`setAllowCellularThisSession` (Task 1).
- Produces: prop `subOptions?: SubOption[]` (default `[]`); the 4th confirm arg now carries the picked `SubPref | null`.

- [ ] **Step 1: Write the failing tests**

```ts
import * as network from '@/offline/network'
import type { SubOption } from '@/offline/types'

const SUBS: SubOption[] = [
  { key: 'b:auto', label: 'Bundled', pref: { kind: 'bundled', lang: 'auto' } },
  { key: 'e:jimaku:ja', label: 'Jimaku · JA', pref: { kind: 'external', provider: 'jimaku', lang: 'ja', label: 'Jimaku A' } },
]

describe('subtitle picker', () => {
  it('hidden with no options; defaults to off (null pref)', async () => {
    const w = mountDlg()
    expect(w.find('[data-test="dl-subs"]').exists()).toBe(false)
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')![0][3]).toBeNull()
  })
  it('emits the picked pref', async () => {
    const w = mountDlg({ subOptions: SUBS })
    await w.find('[data-test="dl-subs"]').setValue('e:jimaku:ja')
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')![0][3]).toEqual({ kind: 'external', provider: 'jimaku', lang: 'ja', label: 'Jimaku A' })
  })
})

describe('mobile-data confirm step', () => {
  beforeEach(() => network._resetNetworkForTests())
  it('on cellular the first confirm arms the warning; the explicit button confirms + sets the override', async () => {
    vi.spyOn(network, 'isCellular').mockReturnValue(true)
    const w = mountDlg()
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')).toBeUndefined()
    expect(w.find('[data-test="cellular-warn"]').exists()).toBe(true)
    await w.find('[data-test="dl-cellular-confirm"]').trigger('click')
    expect(w.emitted('confirm')).toHaveLength(1)
    expect(network.allowCellularThisSession()).toBe(true)
    vi.restoreAllMocks()
  })
  it('not on cellular: single-step confirm as before', async () => {
    const w = mountDlg()
    await w.find('[data-test="dl-start"]').trigger('click')
    expect(w.emitted('confirm')).toHaveLength(1)
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `bunx vitest run src/components/player/aePlayer/DownloadDialog.spec.ts` — Expected: new cases FAIL.

- [ ] **Step 3: Implement**

Script: add prop `subOptions?: SubOption[]` (default `() => []`), import `{ isCellular, allowCellularThisSession, setAllowCellularThisSession } from '@/offline/network'` (namespace-import if spying requires: `import * as network from '@/offline/network'` and call `network.isCellular()` — match the test seam).

```ts
const subKey = ref('off')
const pickedSubPref = computed<SubPref | null>(() =>
  props.subOptions.find((o) => o.key === subKey.value)?.pref ?? null)

const cellularStep = ref(false)
function confirm() {
  if (network.isCellular() && !network.allowCellularThisSession()) {
    cellularStep.value = true
    return
  }
  doConfirm()
}
function confirmCellular() {
  network.setAllowCellularThisSession(true)
  doConfirm()
}
function doConfirm() {
  localStorage.setItem(LS_KEY, quality.value)
  emit('confirm', quality.value, scope.value, combo.value ? { ...combo.value } : null, pickedSubPref.value)
}
```

Template — subtitle select after the scopes block:

```html
    <template v-if="subOptions.length > 0">
      <div class="dl-title text-sm font-semibold">{{ $t('player.aePlayer.offline.subs') }}</div>
      <select v-model="subKey" class="dl-select" data-test="dl-subs">
        <option value="off">{{ $t('player.aePlayer.offline.subsOff') }}</option>
        <option v-for="o in subOptions" :key="o.key" :value="o.key">{{ o.label }}</option>
      </select>
    </template>
```

Footer swap (replace the existing `.dl-actions` block):

```html
    <div v-if="cellularStep" class="dl-warn text-warning" data-test="cellular-warn">
      {{ $t('player.aePlayer.offline.cellularWarn') }}
    </div>
    <div class="dl-actions">
      <template v-if="cellularStep">
        <button type="button" class="dl-btn dl-btn-warn font-medium" data-test="dl-cellular-confirm" @click="confirmCellular">
          {{ $t('player.aePlayer.offline.cellularConfirm') }}
        </button>
        <button type="button" class="dl-btn font-medium" @click="cellularStep = false">
          {{ $t('player.aePlayer.offline.cancel') }}
        </button>
      </template>
      <template v-else>
        <button type="button" class="dl-btn dl-btn-primary font-medium" data-test="dl-start" @click="confirm">
          {{ $t('player.aePlayer.offline.start') }}
        </button>
        <button type="button" class="dl-btn font-medium" @click="emit('close')">
          {{ $t('player.aePlayer.offline.cancel') }}
        </button>
      </template>
    </div>
```

Style: `.dl-btn-warn { border-color: currentColor; }` and give it the `text-warning` class in the template if border-through-currentColor isn't visible enough — no raw colors.

i18n (all three locales, `player.aePlayer.offline`):

| key | en | ru | ja |
|---|---|---|---|
| `subs` | Subtitles | Субтитры | 字幕 |
| `subsOff` | Don't enable | Не включать | オフ |
| `subsBundled` | Bundled with stream | Встроенные в поток | ストリーム内蔵 |
| `cellularWarn` | You're on mobile data. Downloads default to Wi-Fi only. | Вы на мобильных данных. По умолчанию скачивание только по Wi-Fi. | モバイルデータ通信中です。ダウンロードは既定でWi-Fiのみです。 |
| `cellularConfirm` | Download over mobile data | Качать по мобильным данным | モバイルデータでダウンロード |

(`subsBundled` is consumed by the hosts in Tasks 7–8.)

- [ ] **Step 4: Run tests + commit**

Run: `bunx vitest run src/components/player/aePlayer/DownloadDialog.spec.ts src/locales/` — PASS.

```bash
git add src/components/player/aePlayer/DownloadDialog.vue src/components/player/aePlayer/DownloadDialog.spec.ts src/locales/en.json src/locales/ru.json src/locales/ja.json
git commit -m "feat(offline): subtitle picker + mobile-data confirm step in the download dialog"
```

---

### Task 7: AePlayer wiring (in-player entry point)

**Files:**
- Modify: `src/components/player/aePlayer/AePlayer.vue`
- Modify: `src/locales/en.json`, `src/locales/ru.json`, `src/locales/ja.json`

**Interfaces:**
- Consumes: Task 5/6 dialog props + 4-arg confirm; `makeExternalSubResolver` (Task 2); `engineState.cellularPauses` (Task 4); existing AePlayer locals — `report` (capability report computed), `state.combo`, `resolver`, `providerBundledTracks`, `subtitleTracks`, `ensureSubsLoaded()`, `episodes`, `downloadStates`, `seasonTargets`, `enqueueSeason`, `enqueueDownload`, `toast`, `t`.
- Produces: the in-player dialog now edits combo + subs; downloads honor the dialog's combo (re-listing episodes when the provider differs).

- [ ] **Step 1: Wire the dialog props** (template, the existing `<DownloadDialog>` block)

```html
      <DownloadDialog
        v-if="downloadDialogEp"
        :episode-number="downloadDialogEp.number"
        :season-count="seasonCount"
        :duration-min="anime.durationMin"
        :report="report"
        :initial-combo="dlInitialCombo"
        :sub-options="dlSubOptions"
        :load-teams="dlLoadTeams"
        :sheet="sheetTeleport"
        :initial-scope="downloadScope"
        @confirm="onConfirmDownload"
        @close="downloadDialogEp = null"
      />
```

- [ ] **Step 2: Script — dialog data + new confirm** (in the offline-downloads section around `onConfirmDownload`)

Imports to add: `import { makeExternalSubResolver } from '@/offline/externalSubs'`, extend the offline types import with `SubPref, SubOption`, and `engineState` from `@/offline/downloadEngine` (already imported for the progress watcher — reuse it).

```ts
const dlInitialCombo = computed<Combo>(() => ({ ...state.combo.value }))

// Bundled entries come from the CURRENT stream (per-episode availability is
// re-matched by the engine); external entries from the aggregated list.
const dlSubOptions = computed<SubOption[]>(() => {
  const opts: SubOption[] = []
  const seen = new Set<string>()
  for (const tr of providerBundledTracks.value) {
    const key = `b:${tr.lang}`
    if (seen.has(key)) continue
    seen.add(key)
    opts.push({ key, label: `${t('player.aePlayer.offline.subsBundled')} · ${tr.lang.toUpperCase()}`, pref: { kind: 'bundled', lang: tr.lang } })
  }
  const bundledUrls = new Set(providerBundledTracks.value.map((b) => b.url))
  for (const tr of subtitleTracks.value) {
    if (bundledUrls.has(tr.url)) continue
    const key = `e:${tr.provider}:${tr.lang}:${tr.label}`
    if (seen.has(key)) continue
    seen.add(key)
    opts.push({ key, label: `${tr.label} · ${tr.lang.toUpperCase()}`, pref: { kind: 'external', provider: tr.provider, lang: tr.lang, label: tr.label } })
  }
  return opts
})

function dlLoadTeams(provider: string, audio: AudioKind): Promise<string[]> {
  return resolver.listTeams(provider, props.animeId, audio)
}
```

In `onDownloadEpisode(ep)` add `void ensureSubsLoaded()` (so aggregated tracks populate while the dialog is open).

Replace `onConfirmDownload` (preserve any existing `durationMin` threading):

```ts
async function onConfirmDownload(quality: string, scope: 'episode' | 'season', combo: Combo | null, subPref: SubPref | null) {
  const ep = downloadDialogEp.value
  downloadDialogEp.value = null
  if (!ep) return
  const comboSnapshot = combo ? { ...combo } : { ...state.combo.value } // freeze — user may switch sources mid-download
  const resolveSubsFor = makeExternalSubResolver(props.animeId, subPref)
  // A different provider lists episodes with its own keys — re-list and remap
  // by episode NUMBER before resolving against it.
  let eps = episodes.value
  let target: EpisodeOption | undefined = ep
  if (comboSnapshot.provider !== state.combo.value.provider) {
    try {
      eps = await resolver.listEpisodes(comboSnapshot.provider, props.animeId)
    } catch {
      toast.push(t('player.aePlayer.offline.sourceListFailed'), 'error')
      return
    }
    target = eps.find((e) => e.number === ep.number)
  }
  if (scope === 'season') {
    const targets = seasonTargets(eps, downloadStates.value)
    await enqueueSeason(targets, {
      animeId: props.animeId,
      animeTitle: props.anime.title,
      poster: props.anime.still,
      combo: comboSnapshot,
      quality,
      durationMin: props.anime.durationMin,
      subPref: subPref ?? undefined,
      resolveSubsFor,
      resolveFor: (tg) => () => resolver.resolveStream(comboSnapshot.provider, props.animeId, tg, comboSnapshot),
    })
  } else {
    if (!target) {
      toast.push(t('player.aePlayer.offline.epUnavailable'), 'error')
      return
    }
    const one = target
    await enqueueDownload({
      animeId: props.animeId,
      animeTitle: props.anime.title,
      poster: props.anime.still,
      episode: one,
      combo: comboSnapshot,
      quality,
      durationMin: props.anime.durationMin,
      subPref: subPref ?? undefined,
      resolveSubs: resolveSubsFor?.(one),
      resolve: () => resolver.resolveStream(comboSnapshot.provider, props.animeId, one, comboSnapshot),
    })
  }
  void refreshDownloadStates()
}
```

(If the current file's enqueue calls don't pass `durationMin`, keep whatever they pass today — check before editing; `DownloadRequest.durationMin` exists since 030863fc.)

- [ ] **Step 3: Cellular auto-pause toast** (next to the existing `engineState.progress` watcher)

```ts
watch(() => engineState.cellularPauses.value, () => {
  // DownloadsPage mounts an inline offline AePlayer — skip there or the
  // page's own watcher (Task 10) double-toasts the same event.
  if (props.offline) return
  toast.push(t('player.aePlayer.offline.cellularAutoPaused'), 'info', 5000)
})
```

i18n (all three locales, `player.aePlayer.offline`):

| key | en | ru | ja |
|---|---|---|---|
| `sourceListFailed` | Couldn't list episodes for that source | Не удалось получить список серий у этого источника | このソースのエピソード一覧を取得できませんでした |
| `epUnavailable` | This episode isn't available from that source | Эта серия недоступна у выбранного источника | 選択したソースにはこのエピソードがありません |
| `cellularAutoPaused` | Mobile data: downloads paused | Мобильные данные: загрузки приостановлены | モバイルデータ:ダウンロードを一時停止しました |

- [ ] **Step 4: Verify**

Run: `bunx vitest run src/components/player/aePlayer/ src/locales/` — Expected: PASS (AePlayer suites unaffected — dialog props are additive).
Run: `bunx tsc --noEmit` — Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add src/components/player/aePlayer/AePlayer.vue src/locales/en.json src/locales/ru.json src/locales/ja.json
git commit -m "feat(player): download dialog edits source combo + subtitles; downloads honor it"
```

---

### Task 8: Season flow + host (card entry point)

**Files:**
- Modify: `src/offline/seasonDownloadFlow.ts`
- Modify: `src/components/SeasonDownloadHost.vue`
- Test: `src/offline/seasonDownloadFlow.spec.ts` (add cases)

**Interfaces:**
- Consumes: Task 2 (`flattenAggregateSubs`, `makeExternalSubResolver`, `SubPref`), Task 3 (`SeasonContext.subPref/resolveSubsFor`), Task 5/6 dialog props.
- Produces: flow state gains `report: CapabilityReport | null` and `subTracks: SubtitleTrack[]`; `confirmSeasonDownload(quality, scope, combo?: Combo | null, subPref?: SubPref | null)`.

- [ ] **Step 1: Write the failing tests** (append to `seasonDownloadFlow.spec.ts`, following its existing mock scaffolding for `capabilitiesApi`/`useProviderResolver`/`enqueueSeason` — read the file's existing mocks first and extend them, don't rebuild)

Cases to add (adapt to the file's helpers):

```ts
it('open() stores the report and the first-target external tracks (fetch failure tolerated)', async () => {
  // subtitlesApi.all mocked → resolves tracks: expect seasonFlow.subTracks to be the flattened list
  // subtitlesApi.all mocked → rejects: expect seasonFlow.subTracks to be [] and phase still 'choose'
})

it('confirm with a DIFFERENT provider re-lists episodes and recomputes targets', async () => {
  // open() resolves with default provider 'gogoanime' (episodes 1..3, none downloaded)
  // confirmSeasonDownload('720', 'season', { ...defaultCombo, provider: 'kodik' }, null)
  // expect resolver.listEpisodes called with 'kodik'
  // expect enqueueSeason received targets from the kodik list and ctx.combo.provider === 'kodik'
})

it('confirm threads subPref + resolveSubsFor into enqueueSeason', async () => {
  // confirmSeasonDownload('720', 'season', null, { kind: 'external', provider: 'jimaku', lang: 'ja' })
  // expect enqueueSeason ctx.subPref to equal the pref and ctx.resolveSubsFor to be a function
})
```

- [ ] **Step 2: Run to verify failure**

Run: `bunx vitest run src/offline/seasonDownloadFlow.spec.ts` — Expected: new cases FAIL.

- [ ] **Step 3: Implement the flow**

`SeasonFlowState` gains:

```ts
  report: CapabilityReport | null
  /** External (aggregated) tracks for the first target — the dialog's subtitle menu. */
  subTracks: SubtitleTrack[]
```

(init `report: null, subTracks: []`; reset() clears both). Imports: `subtitlesApi` (extend the existing `@/api/client` import), `flattenAggregateSubs, type AggregateSubsResponse` from `@/composables/aePlayer/useSubtitleTracks`, `makeExternalSubResolver` from `./externalSubs`, `type SubPref` from `./types`, `type SubtitleTrack` from `@/types/aePlayer`, `listDownloads` already imported.

In `openSeasonDownload`, after `targets` is computed and before `state.phase = 'choose'`:

```ts
    // External subtitle menu rides along; its failure must never block the flow.
    const subTracks = await subtitlesApi
      .all(request.animeId, targets[0].number)
      .then((r) => flattenAggregateSubs((r.data?.data ?? r.data) as AggregateSubsResponse))
      .catch(() => [] as SubtitleTrack[])
    if (mySeq !== seq) return
    state.report = report
    state.subTracks = subTracks
```

Replace `confirmSeasonDownload`:

```ts
export async function confirmSeasonDownload(
  quality: string,
  scope: 'episode' | 'season',
  combo?: Combo | null,
  subPref?: SubPref | null,
): Promise<void> {
  const req = state.request
  const chosen = combo ? { ...combo } : state.combo ? { ...state.combo } : null
  if (!req || !chosen || state.phase !== 'choose') return
  state.phase = 'queueing'
  const resolver = useProviderResolver()
  try {
    // The episode list came from the DEFAULT provider — a dialog-picked
    // provider numbers episodes with its own keys, so re-list and re-filter.
    let targets = state.targets
    if (chosen.provider !== state.combo?.provider) {
      const episodes = await resolver.listEpisodes(chosen.provider, req.animeId)
      const all = await listDownloads()
      const states: Record<number, DownloadState> = {}
      for (const d of all) if (d.animeId === req.animeId) states[d.episode.number] = d.state
      targets = seasonTargets(episodes, states)
      if (targets.length === 0) {
        reset({ kind: 'nothing-left' })
        return
      }
    }
    const picked = scope === 'season' ? targets : targets.slice(0, 1)
    const eps = picked.map((ep) => ({ ...ep })) // de-proxy before IndexedDB
    const n = await enqueueSeason(eps, {
      animeId: req.animeId,
      animeTitle: req.title,
      poster: req.poster,
      combo: chosen,
      quality,
      durationMin: state.durationMin ?? undefined,
      subPref: subPref ?? undefined,
      resolveSubsFor: makeExternalSubResolver(req.animeId, subPref),
      resolveFor: (ep) => () => resolver.resolveStream(chosen.provider, req.animeId, ep, chosen),
    })
    reset({ kind: 'queued', n })
  } catch (e) {
    console.error('[seasonDownload] confirm failed', e)
    reset({ kind: 'failed', message: e instanceof Error ? e.message : String(e) })
  }
}
```

(Keep the current file's `durationMin`/error-message behavior — merge, don't clobber the 030863fc changes.)

- [ ] **Step 4: Wire the host (`SeasonDownloadHost.vue`)**

```ts
import { computed } from 'vue'
import { useProviderResolver } from '@/composables/aePlayer/useProviderResolver'
import type { Combo, AudioKind, SubtitleTrack } from '@/types/aePlayer'
import type { CapabilityReport } from '@/types/capabilities'
import type { SubPref, SubOption } from '@/offline/types'

const resolver = useProviderResolver()
function loadTeams(provider: string, audio: AudioKind): Promise<string[]> {
  const req = seasonFlow.request
  return req ? resolver.listTeams(provider, req.animeId, audio) : Promise.resolve([])
}

// Labels are i18n'd here — the flow module stays translation-free.
const subOptions = computed<SubOption[]>(() => {
  const opts: SubOption[] = [
    { key: 'b:auto', label: t('player.aePlayer.offline.subsBundled'), pref: { kind: 'bundled', lang: 'auto' } },
  ]
  const seen = new Set<string>()
  for (const tr of seasonFlow.subTracks as readonly SubtitleTrack[]) {
    const key = `e:${tr.provider}:${tr.lang}:${tr.label}`
    if (seen.has(key)) continue
    seen.add(key)
    opts.push({ key, label: `${tr.label} · ${tr.lang.toUpperCase()}`, pref: { kind: 'external', provider: tr.provider, lang: tr.lang, label: tr.label } })
  }
  return opts
})

function onConfirm(quality: string, scope: 'episode' | 'season', combo: Combo | null, subPref: SubPref | null) {
  void confirmSeasonDownload(quality, scope, combo, subPref)
}
```

Template bindings on `<DownloadDialog>` (note the readonly-proxy casts — `readonly(state)` yields `DeepReadonly`, cast at the boundary):

```html
        <DownloadDialog
          :episode-number="seasonFlow.targets[0]?.number ?? 1"
          :season-count="seasonFlow.targets.length"
          :duration-min="seasonFlow.durationMin ?? undefined"
          :report="(seasonFlow.report as CapabilityReport | null)"
          :initial-combo="(seasonFlow.combo as Combo | null)"
          :sub-options="subOptions"
          :load-teams="loadTeams"
          :sheet="isMobile"
          initial-scope="season"
          @confirm="onConfirm"
          @close="cancelSeasonDownload()"
        />
```

- [ ] **Step 5: Run tests + commit**

Run: `bunx vitest run src/offline/seasonDownloadFlow.spec.ts src/offline/seasonDownload.spec.ts` — PASS. `bunx tsc --noEmit` — clean.

```bash
git add src/offline/seasonDownloadFlow.ts src/offline/seasonDownloadFlow.spec.ts src/components/SeasonDownloadHost.vue
git commit -m "feat(offline): card season flow gets source + subtitle selection"
```

---

### Task 9: Offline playback subtitle auto-enable

**Files:**
- Modify: `src/offline/offlineAdapter.ts` (pure helper)
- Modify: `src/components/player/aePlayer/AePlayer.vue`
- Test: `src/offline/offlineAdapter.spec.ts` (add cases)

**Interfaces:**
- Consumes: `OfflineDownload.autoSubUrl` (Task 3), AePlayer locals `chosenSub`, `state.subLang`, `onSubtitlesOff`, `resolveStreamForEpisode`, `props.offline`.
- Produces: `pickOfflineAutoSub(p: OfflinePlayback, epNumber: number, streamSubs: SubtitleTrack[] | undefined): SubtitleTrack | null` exported from `offlineAdapter.ts`.

- [ ] **Step 1: Write the failing test** (append to `offlineAdapter.spec.ts`, reusing its record fixtures)

```ts
import { pickOfflineAutoSub } from './offlineAdapter'

describe('pickOfflineAutoSub', () => {
  const sub = { url: '/__offline/a%3A1/sub/0', provider: 'jimaku', lang: 'ja', label: 'J', format: 'ass' }
  const dl = (over: Partial<OfflineDownload>) => ({ /* reuse the file's record factory */ ...baseRecord, ...over })

  it('returns the stream track matching the record autoSubUrl', () => {
    const p = { animeId: 'a', title: 'T', downloads: [dl({ state: 'done', autoSubUrl: sub.url })] }
    expect(pickOfflineAutoSub(p, 1, [sub])).toEqual(sub)
  })
  it('null when no autoSubUrl / episode not done / track missing from stream', () => {
    expect(pickOfflineAutoSub({ animeId: 'a', title: 'T', downloads: [dl({ state: 'done' })] }, 1, [sub])).toBeNull()
    expect(pickOfflineAutoSub({ animeId: 'a', title: 'T', downloads: [dl({ state: 'paused', autoSubUrl: sub.url })] }, 1, [sub])).toBeNull()
    expect(pickOfflineAutoSub({ animeId: 'a', title: 'T', downloads: [dl({ state: 'done', autoSubUrl: sub.url })] }, 1, [])).toBeNull()
  })
})
```

- [ ] **Step 2: Run to verify failure, then implement the helper** (in `offlineAdapter.ts`)

```ts
/** The local track a download asked to auto-enable, resolved against the
 *  offline stream's track list. The ONLY sanctioned subtitle auto-enable:
 *  an explicit download-time choice, offline playback only. */
export function pickOfflineAutoSub(
  p: OfflinePlayback,
  epNumber: number,
  streamSubs: SubtitleTrack[] | undefined,
): SubtitleTrack | null {
  const url = p.downloads.find((d) => d.state === 'done' && d.episode.number === epNumber)?.autoSubUrl
  if (!url) return null
  return (streamSubs ?? []).find((s) => s.url === url) ?? null
}
```

- [ ] **Step 3: Wire into AePlayer**

Near the subtitle handlers add the session opt-out flag, and set it in `onSubtitlesOff`:

```ts
// Session opt-out: once the viewer explicitly turns subs off, offline
// auto-enable must not re-arm on the next episode.
let userDisabledSubs = false
```

(in `onSubtitlesOff()` body, first line: `userDisabledSubs = true`).

In `resolveStreamForEpisode`, right after `currentStream.value = stream`:

```ts
    if (props.offline && !userDisabledSubs) {
      const auto = pickOfflineAutoSub(props.offline, ep.number, stream.subtitles)
      if (auto) {
        chosenSub.value = auto as SubTrack
        state.subLang.value = auto.lang // session ref — the global "subs off by default" pref is untouched
      }
    }
```

Import `pickOfflineAutoSub` alongside the existing `offlineAdapter` imports.

- [ ] **Step 4: Run tests + commit**

Run: `bunx vitest run src/offline/offlineAdapter.spec.ts src/components/player/aePlayer/` — PASS. `bunx tsc --noEmit` — clean.

```bash
git add src/offline/offlineAdapter.ts src/offline/offlineAdapter.spec.ts src/components/player/aePlayer/AePlayer.vue
git commit -m "feat(offline): auto-enable the downloaded subtitle track at offline playback"
```

---

### Task 10: DownloadsPage — meta line, cellular banner, resume-all

**Files:**
- Modify: `src/views/DownloadsPage.vue`
- Modify: `src/locales/en.json`, `src/locales/ru.json`, `src/locales/ja.json`

**Interfaces:**
- Consumes: `isCellular`/`onConnectionChange`/`allowCellularThisSession` (Task 1), `allowCellularAndResume` (Task 4), `engineState.cellularPauses`, record fields `combo/quality/subtitles/autoSubUrl/pausedBy`.

- [ ] **Step 1: Meta line** — in the episode `<li>`, right after the `sizeLabel` span:

```html
                <span class="text-muted-foreground text-xs truncate">{{ metaLabel(d) }}</span>
```

```ts
/** «what exactly did I download»: provider · quality · chosen subs. */
function metaLabel(d: OfflineDownload): string {
  const base = `${d.combo.provider} · ${d.quality}p`
  const sub = d.autoSubUrl ? d.subtitles.find((s) => s.url === d.autoSubUrl) : undefined
  return sub ? `${base} · CC ${sub.lang.toUpperCase()}` : base
}
```

- [ ] **Step 2: Cellular banner + toast**

Template — above the downloads grid (`<div class="grid gap-4">`):

```html
    <Card v-if="cellularBlocked" class="p-4 flex items-center justify-between gap-3" data-test="cellular-banner">
      <span class="text-sm text-warning">{{ t('downloads.cellularPaused') }}</span>
      <Button size="sm" variant="outline" @click="onCellularResume">{{ t('downloads.cellularResume') }}</Button>
    </Card>
```

Script:

```ts
import { computed, onUnmounted, watch } from 'vue' // extend the existing vue import
import { isCellular, onConnectionChange, allowCellularThisSession } from '@/offline/network'
import { allowCellularAndResume } from '@/offline/cellularGuard'
import { engineState } from '@/offline/downloadEngine'
import { useToast } from '@/composables/useToast'

const toast = useToast()
const onCellular = ref(isCellular())
const cellularAllowed = ref(allowCellularThisSession())
let offConnChange: (() => void) | undefined
onMounted(() => { offConnChange = onConnectionChange(() => { onCellular.value = isCellular() }) })
onUnmounted(() => offConnChange?.())

const cellularBlocked = computed(() =>
  onCellular.value && !cellularAllowed.value &&
  store.entries.some((d) => d.state === 'paused' && d.pausedBy === 'network'))

async function onCellularResume() {
  await allowCellularAndResume()
  cellularAllowed.value = true
  await store.refresh()
}

watch(() => engineState.cellularPauses.value, () => {
  toast.push(t('downloads.cellularPaused'), 'info', 5000)
  void store.refresh()
})
```

i18n (all three locales, `downloads` namespace):

| key | en | ru | ja |
|---|---|---|---|
| `cellularPaused` | Downloads paused — waiting for Wi-Fi | Скачивание приостановлено — ждём Wi-Fi | ダウンロード一時停止中 — Wi-Fi待ち |
| `cellularResume` | Download over mobile data | Качать по мобильным данным | モバイルデータでダウンロード |

- [ ] **Step 3: Verify + commit**

Run: `bunx vitest run src/views/ src/locales/ 2>/dev/null || bunx vitest run src/locales/` (DownloadsPage has no dedicated suite today — locale parity + tsc are the gate). `bunx tsc --noEmit` — clean.

```bash
git add src/views/DownloadsPage.vue src/locales/en.json src/locales/ru.json src/locales/ja.json
git commit -m "feat(offline): downloads page shows source/subs meta + mobile-data banner"
```

---

### Task 11: Offline boot → /downloads

**Files:**
- Create: `src/pwa/offlineBoot.ts`
- Modify: `src/router/index.ts`
- Test: `src/pwa/offlineBoot.spec.ts`

**Interfaces:**
- Consumes: `offlineDownloadsEnabled` (`@/offline/flag`), `START_LOCATION` (vue-router).
- Produces: `shouldRedirectToDownloads(opts: { isInitialNav: boolean; online: boolean; enabled: boolean; toPath: string }): boolean`.

- [ ] **Step 1: Write the failing test**

```ts
// src/pwa/offlineBoot.spec.ts
import { describe, it, expect } from 'vitest'
import { shouldRedirectToDownloads } from './offlineBoot'

const base = { isInitialNav: true, online: false, enabled: true, toPath: '/' }

describe('shouldRedirectToDownloads', () => {
  it('offline initial nav to any page → redirect', () => {
    expect(shouldRedirectToDownloads(base)).toBe(true)
    expect(shouldRedirectToDownloads({ ...base, toPath: '/anime/x' })).toBe(true)
  })
  it('never redirects: already /downloads, online, in-app nav, or feature disabled', () => {
    expect(shouldRedirectToDownloads({ ...base, toPath: '/downloads' })).toBe(false)
    expect(shouldRedirectToDownloads({ ...base, online: true })).toBe(false)
    expect(shouldRedirectToDownloads({ ...base, isInitialNav: false })).toBe(false)
    expect(shouldRedirectToDownloads({ ...base, enabled: false })).toBe(false)
  })
})
```

- [ ] **Step 2: Run to verify failure, then implement**

```ts
// src/pwa/offlineBoot.ts
/** Initial-navigation-only offline landing: opening the app with no network
 *  goes straight to /downloads — the only fully usable surface offline.
 *  In-app navigation while offline is never hijacked. */
export function shouldRedirectToDownloads(opts: {
  isInitialNav: boolean
  online: boolean
  enabled: boolean
  toPath: string
}): boolean {
  return opts.isInitialNav && !opts.online && opts.enabled && opts.toPath !== '/downloads'
}
```

- [ ] **Step 3: Router hookup** (`src/router/index.ts`)

Imports: add `START_LOCATION` to the vue-router import; `import { offlineDownloadsEnabled } from '@/offline/flag'`; `import { shouldRedirectToDownloads } from '@/pwa/offlineBoot'`.

Register as the FIRST `router.beforeEach` (before the existing guards at ~line 310):

```ts
// PWA offline boot: land on /downloads when the app opens with no network.
router.beforeEach((to, from) => {
  if (shouldRedirectToDownloads({
    isInitialNav: from === START_LOCATION,
    online: navigator.onLine,
    enabled: offlineDownloadsEnabled,
    toPath: to.path,
  })) {
    return { path: '/downloads' }
  }
})
```

- [ ] **Step 4: Run tests + commit**

Run: `bunx vitest run src/pwa/` — PASS. `bunx tsc --noEmit` — clean.

```bash
git add src/pwa/offlineBoot.ts src/pwa/offlineBoot.spec.ts src/router/index.ts
git commit -m "feat(pwa): opening the app offline lands on /downloads"
```

---

### Task 12: Full verification sweep

**Files:** none new — verification only (fix-forward anything it surfaces).

- [ ] **Step 1: Full unit suite** — `cd frontend/web && bunx vitest run` — Expected: PASS (pre-existing known failure: `SubtitleSettingsMenu.spec.ts` collection crash is pre-existing at merge-base; do not chase it).
- [ ] **Step 2: Types** — `bunx tsc --noEmit` — clean.
- [ ] **Step 3: DS lint** — `bash scripts/design-system-lint.sh` — 0 errors.
- [ ] **Step 4: Locale parity** — `bunx vitest run src/locales/` — PASS.
- [ ] **Step 5: Build** — `bun run build` — succeeds.
- [ ] **Step 6: Commit any fixes**

```bash
git add -u && git commit -m "test: verification sweep fixes for download source/subs feature" || true
```

(Path-scope the add if the shared tree has parallel WIP: `git add <specific files>`.)

---

## Manual smoke checklist (owner / controller, post-deploy)

1. In-player download dialog: change provider + pick Jimaku JA subs → download an episode → airplane mode → play from /downloads → JA subs are ON automatically.
2. Card context menu «Скачать сезон» → change provider in the dialog → confirm → queued count sane.
3. Android/Chrome on mobile data: start a download → two-step «Качать по мобильным данным» appears; mid-download Wi-Fi→cellular flips records to paused + banner on /downloads; Wi-Fi back → auto-resume.
4. Airplane mode → open the PWA → lands on /downloads.
