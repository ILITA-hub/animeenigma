# aePlayer Subtitles + Subtitle Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the aePlayer subtitle picker real — merge the backend Jimaku+OpenSubtitles aggregation with the provider's own (currently dropped) subtitle tracks, auto-select a best-match track, and add per-provider subtitle resolve/uptime metrics + a Grafana dashboard.

**Architecture:** Frontend wiring (the bug) + backend metrics (observability). The backend aggregation (`GET /api/anime/{id}/subtitles/all`) and provider subtitle tracks (signed `stream.tracks` in the scraper envelope) already exist; the aePlayer just never consumed either. We add a `useSubtitleTracks` composable that fetches+merges both into the already-built `BrowseSubsModal`, a pure `pickDefaultSubtitle` selector, modal loading/error/off states, and instrument `SubsAggregator.FetchAll` with Prometheus metrics feeding a new `subtitle-health` dashboard.

**Tech Stack:** Vue 3 + TypeScript (`bun`/`vitest`/`vue-tsc`), Go (catalog service, `libs/metrics` promauto), Grafana provisioned JSON, Prometheus.

**Spec:** `docs/superpowers/specs/2026-06-22-aeplayer-subtitles-and-observability-design.md`

## Global Constraints

- **Worktree only.** All work in a `git worktree` off fresh `origin/main`; never edit the `/data/animeenigma` base tree. Deploy from the clean worktree (the base tree carries stale untracked player files that break the in-Docker `vue-tsc`).
- **Frontend:** `bun`/`bunx` (never npm). DS-lint is build-enforced — bind to semantic tokens, only `font-medium`/`font-semibold`, no off-palette colors. Player components are EXEMPT from the native-form-control rule (reka portals break in fullscreen) — the modal stays plain HTML.
- **i18n parity:** any new UI string added to `en.json` MUST be added to `ru.json` AND `ja.json` (parity test enforces it).
- **Signed subtitle URLs:** provider `stream.tracks[].file` carry sibling `exp`/`sig` (stamped by catalog `streamsign.SignScraperStreamBody`). They MUST be forwarded to the HLS proxy or it 502s. Aggregation URLs (Jimaku `jimaku.cc` / OpenSubtitles same-origin `/api/...`) are returned ready-to-fetch by the backend and are used as-is.
- **Go metrics:** follow the `libs/metrics` promauto pattern (package-level vars + an `Emit*`/instrument helper). Handwritten fakes in tests — no testify/mock.
- **No active polling** of Jimaku in this work (deferred to `/admin/feedback` TODO). Metrics are driven by live resolve traffic only.
- **Effort metrics:** if any doc/changelog scoring is needed use UXΔ / CDI / MVQ — never time units.

---

## File Structure

**Frontend (Part A):**
- `frontend/web/src/types/aePlayer.ts` — add `SubtitleTrack` + `StreamResult.subtitles?` (modify).
- `frontend/web/src/composables/aePlayer/useProviderResolver.ts` — map signed `stream.tracks` → `StreamResult.subtitles` in the scraper adapter (modify).
- `frontend/web/src/utils/subtitleProxy.ts` — new `buildSubtitleProxyUrl` + `detectSubFormat` (create; ported from `OurEnglishPlayer`).
- `frontend/web/src/composables/aePlayer/useSubtitleTracks.ts` — fetch aggregation + merge provider tracks (create).
- `frontend/web/src/composables/aePlayer/pickDefaultSubtitle.ts` — pure best-match selector (create).
- `frontend/web/src/components/player/aePlayer/BrowseSubsModal.vue` — loading/error/off states (modify).
- `frontend/web/src/components/player/aePlayer/AePlayer.vue` — wire composable, auto-select, proxied select (modify).
- `frontend/web/src/locales/{en,ru,ja}.json` — modal strings (modify).

**Backend + Grafana (Part B):**
- `libs/metrics/subtitles.go` — metric defs + `RecordSubtitleResolve` helper (create).
- `services/catalog/internal/service/subs_aggregator.go` — instrument `FetchAll` live path (modify).
- `docker/grafana/dashboards/subtitle-health.json` — new dashboard (create).

---

## Task 1: Carry provider subtitle tracks through `StreamResult`

**Files:**
- Modify: `frontend/web/src/types/aePlayer.ts` (the `StreamResult` interface, ~line 60)
- Modify: `frontend/web/src/composables/aePlayer/useProviderResolver.ts` (scraper adapter `resolveStream`, ~line 232-270; `ScraperEnvelope.stream.tracks` type ~line 77)
- Test: `frontend/web/src/composables/aePlayer/useProviderResolver.spec.ts`

**Interfaces:**
- Produces: `SubtitleTrack { url: string; provider: string; lang: string; label: string; format: string }` and `StreamResult.subtitles?: SubtitleTrack[]`. Consumed by Tasks 3, 4, 6.

- [ ] **Step 1: Write the failing test** — add to `useProviderResolver.spec.ts`. Find the existing scraper-adapter `resolveStream` describe block and add:

```ts
it('maps signed provider subtitle tracks into StreamResult.subtitles', async () => {
  const api = makeFakeScraperApi({
    servers: [{ id: 's1', name: 'Server 1', type: 'sub' }],
    stream: {
      sources: [{ url: 'https://cdn.example/v.m3u8', type: 'hls', exp: 'E', sig: 'S' }],
      tracks: [
        { file: 'https://cdn.example/en.vtt', label: 'English', kind: 'captions', exp: 'E2', sig: 'S2' },
        { file: 'https://cdn.example/thumbs.vtt', label: 'thumbnails', kind: 'thumbnails' },
      ],
      headers: { Referer: 'https://gogo.example/' },
    },
  })
  const adapter = makeScraperAdapter(api, 'gogoanime')
  const res = await adapter.resolveStream('anime-1', { key: 'ep1', label: 1, number: 1 }, baseCombo({ provider: 'gogoanime' }))
  expect(res.subtitles).toHaveLength(1) // thumbnails excluded
  const t = res.subtitles![0]
  expect(t.provider).toBe('gogoanime')
  expect(t.lang).toBe('en')
  expect(t.format).toBe('vtt')
  // URL is proxied AND carries the track's own exp/sig (not the source's)
  expect(t.url).toContain('exp=E2')
  expect(t.url).toContain('sig=S2')
})
```

If `makeFakeScraperApi`/`makeScraperAdapter`/`baseCombo` helpers don't already exist in the spec, mirror the existing test setup in that file (read the top of the spec for the established fakes and reuse them; the scraper adapter factory is whatever the file already imports — match its name).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderResolver.spec.ts -t "subtitle tracks"`
Expected: FAIL — `res.subtitles` is `undefined`.

- [ ] **Step 3: Add the type** — in `types/aePlayer.ts`, above `StreamResult`:

```ts
export interface SubtitleTrack {
  url: string       // ready-to-fetch (proxied + signed for provider tracks)
  provider: string  // 'gogoanime' | 'jimaku' | 'opensubtitles' | ...
  lang: string      // 'en' | 'ja' | 'ru' | ...
  label: string
  format: string    // 'vtt' | 'srt' | 'ass'
}
```

Add to `StreamResult`:

```ts
  /** Subtitle tracks the provider shipped alongside the stream (signed). */
  subtitles?: SubtitleTrack[]
```

- [ ] **Step 4: Map tracks in the scraper adapter.** In `useProviderResolver.ts`:

First widen the envelope type (~line 77):

```ts
  stream?: {
    sources: ScraperSource[]
    tracks?: ScraperTrack[]
    headers?: Record<string, string>
  }
```

and add near `ScraperSource`:

```ts
interface ScraperTrack {
  file: string
  label?: string
  kind?: string   // 'captions' | 'subtitles' | 'thumbnails'
  exp?: string
  sig?: string
}
```

Then in the scraper `resolveStream`, just before `return {`, build the subtitle list (import `buildSubtitleProxyUrl`, `detectSubFormat`, `langFromTrack` — created in Task 2; for THIS task add a local minimal mapping and replace with the util in Task 2, OR do Task 2 first. Recommended order: do Task 2 first, then this import works). Map:

```ts
const subtitles: SubtitleTrack[] = (stream.tracks ?? [])
  .filter((t) => t.kind === 'captions' || t.kind === 'subtitles' || t.kind === undefined)
  .map((t) => ({
    url: buildSubtitleProxyUrl(t.file, t.exp, t.sig),
    provider: resolvedPrefer || prefer || 'scraper',
    lang: langFromTrack(t.label, t.file),
    label: t.label || 'subtitle',
    format: detectSubFormat(undefined, t.file) ?? 'vtt',
  }))
```

and add `...(subtitles.length ? { subtitles } : {})` to the returned object.

> Note: `kind === undefined` is included because some providers omit `kind` for their single caption track; `thumbnails` is explicitly excluded by not matching.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderResolver.spec.ts -t "subtitle tracks"`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/types/aePlayer.ts frontend/web/src/composables/aePlayer/useProviderResolver.ts frontend/web/src/composables/aePlayer/useProviderResolver.spec.ts
git commit -m "feat(aeplayer): carry provider subtitle tracks through StreamResult"
```

---

## Task 2: `subtitleProxy.ts` util — proxy URL + format/lang detection

**Files:**
- Create: `frontend/web/src/utils/subtitleProxy.ts`
- Test: `frontend/web/src/utils/subtitleProxy.spec.ts`

**Interfaces:**
- Consumes: `hlsProxyUrl` from `@/utils/streaming`.
- Produces: `buildSubtitleProxyUrl(file: string, exp?: string, sig?: string): string`, `detectSubFormat(format: string | undefined, url: string): 'ass'|'srt'|'vtt'|null`, `langFromTrack(label: string | undefined, url: string): string`. Used by Tasks 1, 3.

> **Do this task before Task 1's Step 4** (Task 1 imports these).

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect, vi } from 'vitest'
vi.mock('@/utils/streaming', () => ({ hlsProxyUrl: (q: string) => `/api/streaming/hls-proxy?${q}` }))
import { buildSubtitleProxyUrl, detectSubFormat, langFromTrack } from './subtitleProxy'

describe('subtitleProxy', () => {
  it('builds a signed proxy url', () => {
    const u = buildSubtitleProxyUrl('https://cdn/x.vtt', 'E', 'S')
    expect(u).toContain('url=https%3A%2F%2Fcdn%2Fx.vtt')
    expect(u).toContain('exp=E')
    expect(u).toContain('sig=S')
  })
  it('omits exp/sig when absent', () => {
    expect(buildSubtitleProxyUrl('https://cdn/x.vtt')).not.toContain('exp=')
  })
  it('detects format from explicit value then extension', () => {
    expect(detectSubFormat('ASS', 'x')).toBe('ass')
    expect(detectSubFormat(undefined, 'https://c/a.srt?token=1')).toBe('srt')
    expect(detectSubFormat(undefined, 'https://c/a.bin')).toBeNull()
  })
  it('infers lang from label keywords, else ja default', () => {
    expect(langFromTrack('English', 'x')).toBe('en')
    expect(langFromTrack('Русский', 'x')).toBe('ru')
    expect(langFromTrack(undefined, 'https://c/jpn.vtt')).toBe('ja')
    expect(langFromTrack('日本語', 'x')).toBe('ja')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/utils/subtitleProxy.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
import { hlsProxyUrl } from '@/utils/streaming'

/** Wrap a subtitle file URL in the signed HLS proxy (CORS + provenance).
 *  SubtitleOverlay fetches any `/`-prefixed url directly, so a pre-signed
 *  proxy url loads an un-allowlisted scraper subtitle CDN without a 502. */
export function buildSubtitleProxyUrl(file: string, exp?: string, sig?: string): string {
  const params = new URLSearchParams()
  params.set('url', file)
  if (exp && sig) {
    params.set('exp', exp)
    params.set('sig', sig)
  }
  return hlsProxyUrl(params.toString())
}

export function detectSubFormat(
  format: string | undefined,
  url: string,
): 'ass' | 'srt' | 'vtt' | null {
  const ext = (format || url.split('?')[0].split('.').pop() || '').toLowerCase()
  return ext === 'ass' || ext === 'srt' || ext === 'vtt' ? ext : null
}

/** Best-effort language code from a track label / filename. Defaults to 'ja'
 *  (the provider's burned-in-less tracks are overwhelmingly JP soft-subs). */
export function langFromTrack(label: string | undefined, url: string): string {
  const hay = `${label ?? ''} ${url}`.toLowerCase()
  if (/\b(en|eng|english)\b/.test(hay)) return 'en'
  if (/\b(ru|rus|russian)\b/.test(hay) || /[Ѐ-ӿ]/.test(label ?? '')) return 'ru'
  if (/\b(ja|jp|jpn|japanese)\b/.test(hay) || /[぀-ヿ一-鿿]/.test(label ?? '')) return 'ja'
  return 'ja'
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/utils/subtitleProxy.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/utils/subtitleProxy.ts frontend/web/src/utils/subtitleProxy.spec.ts
git commit -m "feat(aeplayer): subtitle proxy url + format/lang detection util"
```

---

## Task 3: `useSubtitleTracks` composable — fetch aggregation + merge

**Files:**
- Create: `frontend/web/src/composables/aePlayer/useSubtitleTracks.ts`
- Test: `frontend/web/src/composables/aePlayer/useSubtitleTracks.spec.ts`

**Interfaces:**
- Consumes: `subtitlesApi.all(animeId, episode)` from `@/api/client` (returns `{ data: { data: AggregateResponse } }`, `AggregateResponse = { languages: Record<string, BackendSubTrack[]>, episode: number, providers_down?: string[] }`, `BackendSubTrack = { url, lang, label, format?, provider, release? }`); `SubtitleTrack` (Task 1); `SubTrack` from `BrowseSubsModal.vue`.
- Produces: `useSubtitleTracks(animeId: Ref<string>|string, episode: Ref<number|undefined>, providerSubtitles: Ref<SubtitleTrack[]|undefined>)` → `{ tracks: ComputedRef<SubTrack[]>, loading: Ref<boolean>, error: Ref<string|null>, providersDown: Ref<string[]>, ensureLoaded(): Promise<void>, refetch(): Promise<void> }`. Used by Task 6.

> `SubTrack` (the modal's shape) and `SubtitleTrack` (the stream's shape) are field-identical (`url, provider, lang, label, format`), so merging is a concat + dedupe by `url`.

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'

const allMock = vi.fn()
vi.mock('@/api/client', () => ({ subtitlesApi: { all: (...a: unknown[]) => allMock(...a) } }))

import { useSubtitleTracks } from './useSubtitleTracks'

beforeEach(() => allMock.mockReset())

const flush = () => new Promise((r) => setTimeout(r, 0))

describe('useSubtitleTracks', () => {
  it('merges aggregation + provider tracks, deduped by url', async () => {
    allMock.mockResolvedValue({ data: { data: {
      languages: {
        ja: [{ url: '/api/j1.ass', lang: 'ja', label: 'Jimaku 1', format: 'ass', provider: 'jimaku' }],
        en: [{ url: '/api/os1.srt', lang: 'en', label: 'OS', format: 'srt', provider: 'opensubtitles' }],
      },
      episode: 8,
      providers_down: [],
    } } })
    const providerSubs = ref([{ url: '/proxy?url=en.vtt', provider: 'gogoanime', lang: 'en', label: 'Provider EN', format: 'vtt' }])
    const s = useSubtitleTracks('anime-1', ref(8), providerSubs)
    await s.ensureLoaded()
    await flush()
    expect(s.tracks.value).toHaveLength(3)
    expect(s.tracks.value.map((t) => t.provider).sort()).toEqual(['gogoanime', 'jimaku', 'opensubtitles'])
  })

  it('fails soft: aggregation error sets error but keeps provider tracks', async () => {
    allMock.mockRejectedValue(new Error('jimaku down'))
    const providerSubs = ref([{ url: '/p', provider: 'gogoanime', lang: 'en', label: 'P', format: 'vtt' }])
    const s = useSubtitleTracks('anime-1', ref(8), providerSubs)
    await s.ensureLoaded()
    await flush()
    expect(s.error.value).toBeTruthy()
    expect(s.tracks.value).toHaveLength(1) // provider track survives
  })

  it('surfaces providers_down', async () => {
    allMock.mockResolvedValue({ data: { data: { languages: {}, episode: 8, providers_down: ['jimaku'] } } })
    const s = useSubtitleTracks('anime-1', ref(8), ref([]))
    await s.ensureLoaded()
    await flush()
    expect(s.providersDown.value).toEqual(['jimaku'])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useSubtitleTracks.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
import { ref, computed, unref, watch, type Ref, type ComputedRef } from 'vue'
import { subtitlesApi } from '@/api/client'
import type { SubtitleTrack } from '@/types/aePlayer'
import type { SubTrack } from '@/components/player/aePlayer/BrowseSubsModal.vue'

interface BackendSubTrack {
  url: string; lang: string; label: string; format?: string; provider: string; release?: string
}
interface AggregateResponse {
  languages: Record<string, BackendSubTrack[]>
  episode: number
  providers_down?: string[]
}

export function useSubtitleTracks(
  animeId: Ref<string> | string,
  episode: Ref<number | undefined>,
  providerSubtitles: Ref<SubtitleTrack[] | undefined>,
) {
  const aggTracks = ref<SubTrack[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const providersDown = ref<string[]>([])
  let loadedEpisode: number | null = null
  let inFlight: Promise<void> | null = null

  async function fetchFor(ep: number): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const resp = await subtitlesApi.all(unref(animeId), ep)
      const data: AggregateResponse = resp.data?.data ?? resp.data
      const flat: SubTrack[] = []
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
      aggTracks.value = flat
      providersDown.value = data?.providers_down ?? []
      loadedEpisode = ep
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      // keep aggTracks as-is (provider tracks still merge below)
    } finally {
      loading.value = false
    }
  }

  async function ensureLoaded(): Promise<void> {
    const ep = episode.value
    if (ep == null) return
    if (loadedEpisode === ep && !error.value) return
    if (inFlight) return inFlight
    inFlight = fetchFor(ep).finally(() => { inFlight = null })
    return inFlight
  }

  async function refetch(): Promise<void> {
    loadedEpisode = null
    return ensureLoaded()
  }

  // Reset aggregation cache when the episode changes (provider tracks come
  // from the live stream and are merged reactively).
  watch(episode, () => { loadedEpisode = null; aggTracks.value = []; providersDown.value = [] })

  const tracks: ComputedRef<SubTrack[]> = computed(() => {
    const provider = (providerSubtitles.value ?? []) as SubTrack[]
    const seen = new Set<string>()
    const out: SubTrack[] = []
    for (const t of [...provider, ...aggTracks.value]) {
      if (seen.has(t.url)) continue
      seen.add(t.url)
      out.push(t)
    }
    return out
  })

  return { tracks, loading, error, providersDown, ensureLoaded, refetch }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useSubtitleTracks.spec.ts`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/useSubtitleTracks.ts frontend/web/src/composables/aePlayer/useSubtitleTracks.spec.ts
git commit -m "feat(aeplayer): useSubtitleTracks — fetch aggregation + merge provider tracks"
```

---

## Task 4: `pickDefaultSubtitle` — pure best-match selector

**Files:**
- Create: `frontend/web/src/composables/aePlayer/pickDefaultSubtitle.ts`
- Test: `frontend/web/src/composables/aePlayer/pickDefaultSubtitle.spec.ts`

**Interfaces:**
- Produces: `pickDefaultSubtitle(tracks: SubTrack[], opts: { lang: string }): SubTrack | null`. Used by Task 6.

Precedence: (1) lang matches `opts.lang`; within the matches (2) provider `jimaku` first, then any non-`opensubtitles` provider (provider-own), then `opensubtitles`. If no lang match, fall back to the same provider precedence across all tracks. Returns `null` for empty input.

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect } from 'vitest'
import { pickDefaultSubtitle } from './pickDefaultSubtitle'

const T = (provider: string, lang: string, url = `${provider}-${lang}`) => ({ url, provider, lang, label: url, format: 'srt' })

describe('pickDefaultSubtitle', () => {
  it('returns null for no tracks', () => {
    expect(pickDefaultSubtitle([], { lang: 'ja' })).toBeNull()
  })
  it('prefers lang match, jimaku first', () => {
    const r = pickDefaultSubtitle([T('opensubtitles', 'ja'), T('gogoanime', 'en'), T('jimaku', 'ja')], { lang: 'ja' })
    expect(r?.provider).toBe('jimaku')
  })
  it('prefers provider-own over opensubtitles within lang', () => {
    const r = pickDefaultSubtitle([T('opensubtitles', 'en'), T('gogoanime', 'en')], { lang: 'en' })
    expect(r?.provider).toBe('gogoanime')
  })
  it('falls back across langs when no lang match (jimaku first)', () => {
    const r = pickDefaultSubtitle([T('opensubtitles', 'en'), T('jimaku', 'ja')], { lang: 'ru' })
    expect(r?.provider).toBe('jimaku')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/pickDefaultSubtitle.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
import type { SubTrack } from '@/components/player/aePlayer/BrowseSubsModal.vue'

function providerRank(provider: string): number {
  if (provider === 'jimaku') return 0
  if (provider === 'opensubtitles') return 2
  return 1 // provider-own (gogoanime, etc.)
}

function best(tracks: SubTrack[]): SubTrack | null {
  if (tracks.length === 0) return null
  return [...tracks].sort((a, b) => providerRank(a.provider) - providerRank(b.provider))[0]
}

export function pickDefaultSubtitle(tracks: SubTrack[], opts: { lang: string }): SubTrack | null {
  if (tracks.length === 0) return null
  const matches = tracks.filter((t) => t.lang === opts.lang)
  return best(matches) ?? best(tracks)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/pickDefaultSubtitle.spec.ts`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/pickDefaultSubtitle.ts frontend/web/src/composables/aePlayer/pickDefaultSubtitle.spec.ts
git commit -m "feat(aeplayer): pickDefaultSubtitle best-match selector"
```

---

## Task 5: `BrowseSubsModal` loading / error / off states

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/BrowseSubsModal.vue`
- Modify: `frontend/web/src/components/player/aePlayer/BrowseSubsModal.spec.ts`
- Modify: `frontend/web/src/locales/{en,ru,ja}.json`

**Interfaces:**
- Consumes: existing `SubTrack` interface (unchanged).
- Produces: new props `loading?: boolean`, `error?: string | null`, `providersDown?: string[]`, `selectedUrl: string | null` (existing); new emits `retry`, `off`. Used by Task 6.

- [ ] **Step 1: Write the failing tests** — add to `BrowseSubsModal.spec.ts`:

```ts
it('shows a loading state', () => {
  const w = mount(BrowseSubsModal, { props: { tracks: [], selectedUrl: null, loading: true } })
  expect(w.find('[data-test="subs-loading"]').exists()).toBe(true)
})

it('shows an error with a retry button that emits retry', async () => {
  const w = mount(BrowseSubsModal, { props: { tracks: [], selectedUrl: null, error: 'jimaku down' } })
  expect(w.find('[data-test="subs-error"]').exists()).toBe(true)
  await w.find('[data-test="subs-retry"]').trigger('click')
  expect(w.emitted('retry')).toBeTruthy()
})

it('emits off when "Subtitles off" is clicked', async () => {
  const w = mount(BrowseSubsModal, { props: { tracks: [{ url: 'u', provider: 'jimaku', lang: 'ja', label: 'L', format: 'srt' }], selectedUrl: 'u' } })
  await w.find('[data-test="subs-off"]').trigger('click')
  expect(w.emitted('off')).toBeTruthy()
})
```

(Match the existing import/mount style at the top of the spec — it already mounts `BrowseSubsModal` with `tracks`/`selectedUrl`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/BrowseSubsModal.spec.ts -t "loading|error|off"`
Expected: FAIL — elements not found.

- [ ] **Step 3: Implement.** In `BrowseSubsModal.vue`:

Extend props/emits in `<script setup>`:

```ts
const props = defineProps<{
  tracks: SubTrack[]
  selectedUrl: string | null
  loading?: boolean
  error?: string | null
  providersDown?: string[]
}>()

const emit = defineEmits<{
  (e: 'select', track: SubTrack): void
  (e: 'close'): void
  (e: 'retry'): void
  (e: 'off'): void
}>()
```

In the template `<!-- Body -->`, replace the single empty-state/groups block with state-ordered rendering (loading → error → off-row + groups/empty):

```html
<!-- Body -->
<div class="overflow-y-auto px-5 py-3.5 pb-4.5" style="scrollbar-width: thin;">
  <!-- Loading -->
  <div v-if="loading" data-test="subs-loading" class="text-center text-[var(--muted-foreground)] py-10 text-[14px]">
    {{ $t('player.aePlayer.subs.loading') }}
  </div>

  <!-- Error -->
  <div v-else-if="error" data-test="subs-error" class="text-center py-10">
    <p class="text-[var(--muted-foreground)] text-[14px] mb-3">{{ $t('player.aePlayer.subs.loadError') }}</p>
    <button
      data-test="subs-retry"
      class="px-3.5 py-[7px] rounded-[var(--r-sm)] border-0 text-[13px] font-semibold text-white hover:bg-white/20"
      style="background: var(--border);"
      @click="emit('retry')"
    >
      {{ $t('player.aePlayer.subs.retry') }}
    </button>
  </div>

  <template v-else>
    <!-- Providers-down notice (non-blocking) -->
    <p v-if="providersDown && providersDown.length" class="text-[12px] text-[var(--muted-foreground)] mb-2">
      {{ $t('player.aePlayer.subs.providersDown', { providers: providersDown.join(', ') }) }}
    </p>

    <!-- Subtitles off -->
    <button
      data-test="subs-off"
      :class="[
        'w-full flex items-center gap-3 px-3 py-[11px] mb-3 rounded-[var(--r-md)] border transition-all text-left',
        selectedUrl === null ? 'border-[var(--accent-line)]' : 'bg-white/[0.05] border-transparent',
      ]"
      :style="selectedUrl === null ? 'background: var(--accent-soft)' : ''"
      @click="emit('off')"
    >
      <span class="text-[14px] text-white">{{ $t('player.aePlayer.subs.off') }}</span>
    </button>

    <!-- Empty -->
    <div v-if="groupedTracks.length === 0" class="text-center text-[var(--muted-foreground)] py-10 text-[14px]">
      {{ $t('player.aePlayer.subs.empty') }}
    </div>

    <!-- Groups (UNCHANGED — keep the existing v-for groupedTracks block here) -->
    <div v-for="group in groupedTracks" :key="group.lang" data-test="lang-group" class="mb-4">
      <!-- ...existing group markup verbatim... -->
    </div>
  </template>
</div>
```

> Keep the existing `groupedTracks` `v-for` body exactly as-is; only move it inside the `<template v-else>` and add the loading/error/off/empty siblings. The hardcoded English strings `"No subtitles match your search."`, `"Browse Subtitles"`, the count line, and the `Select`/`Selected` labels may stay literal (the modal already ships them literal) OR be i18n'd in the same pass — only the NEW strings below are required.

- [ ] **Step 4: Add i18n keys** to `en.json`, `ru.json`, `ja.json` under `player.aePlayer.subs` (create the sub-namespace if missing):

`en.json`:
```json
"subs": {
  "loading": "Loading subtitles…",
  "loadError": "Couldn't load subtitles.",
  "retry": "Retry",
  "off": "Subtitles off",
  "empty": "No subtitles match your search.",
  "providersDown": "Couldn't reach: {providers}"
}
```
`ru.json`:
```json
"subs": {
  "loading": "Загрузка субтитров…",
  "loadError": "Не удалось загрузить субтитры.",
  "retry": "Повторить",
  "off": "Без субтитров",
  "empty": "Нет субтитров по запросу.",
  "providersDown": "Недоступно: {providers}"
}
```
`ja.json`:
```json
"subs": {
  "loading": "字幕を読み込み中…",
  "loadError": "字幕を読み込めませんでした。",
  "retry": "再試行",
  "off": "字幕オフ",
  "empty": "条件に一致する字幕がありません。",
  "providersDown": "接続できません: {providers}"
}
```

> Place the `subs` object inside the existing `player.aePlayer` namespace in each file (search `"aePlayer"`). If `player.aePlayer` doesn't exist in a file, add it. Keep the same `{providers}` ICU placeholder in all three (parity test checks placeholders).

- [ ] **Step 5: Run tests + i18n parity**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/BrowseSubsModal.spec.ts src/locales`
Expected: PASS (new + existing modal tests, locale parity green)

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/BrowseSubsModal.vue frontend/web/src/components/player/aePlayer/BrowseSubsModal.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(aeplayer): BrowseSubsModal loading/error/off states + i18n"
```

---

## Task 6: Wire subtitles into `AePlayer.vue`

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (subtitles section ~2160-2198; modal usage ~290-297; resolve flow ~2017; subs-menu open handler)
- Modify/Add: `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.*.spec.ts` (a new `AePlayer.subtitles.spec.ts` is cleanest)

**Interfaces:**
- Consumes: `useSubtitleTracks` (Task 3), `pickDefaultSubtitle` (Task 4), `buildSubtitleProxyUrl` not needed here (aggregation urls used as-is; provider urls already proxied in Task 1).

- [ ] **Step 1: Write the failing test** — new `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts`. Mock the api + child components and assert the modal receives real tracks and auto-select fires. Model it on the existing `AePlayer.room.spec.ts` mounting harness (reuse its mocks for `@/api/client`, router, i18n). Minimum assertions:

```ts
// after mounting AePlayer with a resolved SUB stream that has provider subtitle tracks
// and a mocked subtitlesApi.all returning a jimaku ja track:
it('passes merged tracks to BrowseSubsModal (not [])', async () => {
  // open subs browse, then:
  const modal = wrapper.findComponent(BrowseSubsModal)
  expect(modal.props('tracks').length).toBeGreaterThan(0)
})

it('auto-selects a best-match track for a sub stream with no hardsub', async () => {
  // after resolve + tracks load:
  const overlay = wrapper.findComponent(SubtitleOverlay)
  expect(overlay.props('subtitleUrl')).toBeTruthy()
})
```

> If the existing AePlayer test harness is too heavy to assert auto-select end-to-end, split: keep the modal-tracks assertion here, and rely on Task 4's `pickDefaultSubtitle` unit test for selection logic. Do NOT skip the "tracks not []" assertion — it is the regression guard for the original bug.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts`
Expected: FAIL — modal still receives `[]` / overlay url null.

- [ ] **Step 3: Implement.** In `AePlayer.vue` `<script setup>`:

Imports:
```ts
import { useSubtitleTracks } from '@/composables/aePlayer/useSubtitleTracks'
import { pickDefaultSubtitle } from '@/composables/aePlayer/pickDefaultSubtitle'
```

Near the Subtitles section (~2160), after `const chosenSub = ref<SubTrack | null>(null)`:

```ts
// Episode number the subtitle aggregation keys on.
const subEpisode = computed(() => selectedEpisode.value?.number ?? anime.ep)
// Provider's own signed soft-subs from the resolved stream.
const providerSubtitles = computed(() => currentStream.value?.subtitles)
const {
  tracks: subtitleTracks,
  loading: subsLoading,
  error: subsError,
  providersDown: subsProvidersDown,
  ensureLoaded: ensureSubsLoaded,
  refetch: refetchSubs,
} = useSubtitleTracks(toRef(props, 'animeId'), subEpisode, providerSubtitles)

// User explicitly turned subs off (or picked one) for THIS stream → don't re-auto-select.
let subUserDecided = false

function autoSelectSubtitle() {
  if (subUserDecided || chosenSub.value) return
  if (state.combo.value.audio !== 'sub') return // only SUB/raw cuts
  if (hardsubNote.value) return                 // burned-in already
  const pick = pickDefaultSubtitle(subtitleTracks.value, { lang: state.combo.value.lang })
  if (pick) {
    chosenSub.value = pick
    state.subLang.value = pick.lang
  }
}

// A NEW episode is a fresh subtitle decision — clear the pick + the user latch.
// (Keyed on episode, NOT currentStream: a same-episode re-resolve — server
// fallback, quality swap — must NOT drop a sub the user already picked.)
watch(subEpisode, () => {
  subUserDecided = false
  chosenSub.value = null
})

// Fetch aggregation eagerly once a SUB stream resolves, then auto-select.
watch(
  [currentStream, () => state.combo.value.audio],
  async () => {
    if (!currentStream.value || state.combo.value.audio !== 'sub') return
    await ensureSubsLoaded()
    autoSelectSubtitle()
  },
)
// Provider tracks (and late aggregation) can arrive after the first tick.
watch(subtitleTracks, () => autoSelectSubtitle())
```

Update `onSelectSubTrack` (existing ~2193) to mark the user decision:
```ts
function onSelectSubTrack(track: SubTrack) {
  subUserDecided = true
  chosenSub.value = track
  state.subLang.value = track.lang
  browseOpen.value = false
}
```

Add an off handler:
```ts
function onSubtitlesOff() {
  subUserDecided = true
  chosenSub.value = null
  state.subLang.value = 'off'
  browseOpen.value = false
}
```

Ensure the browse modal opens with data loaded — find the handler that sets `browseOpen.value = true` (the `@open-browse` on `SubtitlesMenu`) and make it `() => { browseOpen.value = true; void ensureSubsLoaded() }`.

Update the modal usage in `<template>` (~290):
```html
<BrowseSubsModal
  v-if="browseOpen"
  :tracks="subtitleTracks"
  :selected-url="chosenSubUrl"
  :loading="subsLoading"
  :error="subsError"
  :providers-down="subsProvidersDown"
  @click.stop
  @select="onSelectSubTrack"
  @retry="refetchSubs"
  @off="onSubtitlesOff"
  @close="browseOpen = false"
/>
```

> `toRef` must be imported from `vue` (add to the existing `import { ... } from 'vue'`). `selectedEpisode` already exists in the component.

- [ ] **Step 4: Run test + type-check**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts && bunx vue-tsc --noEmit`
Expected: PASS + 0 type errors

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts
git commit -m "feat(aeplayer): wire subtitle aggregation + provider tracks + auto-select"
```

---

## Task 7: `libs/metrics/subtitles.go` — metric definitions + helper

**Files:**
- Create: `libs/metrics/subtitles.go`
- Test: `libs/metrics/subtitles_test.go`

**Interfaces:**
- Produces:
  - vars `SubtitleResolveTotal *prometheus.CounterVec` (labels `provider,status`), `SubtitleResolveDuration prometheus.Histogram`, `SubtitleProviderUp *prometheus.GaugeVec` (label `provider`), `SubtitleTracksReturned *prometheus.CounterVec` (label `provider`).
  - `type SubtitleProviderOutcome struct { Provider string; Status string; Tracks int }` (`Status ∈ {"ok","down","empty","unconfigured"}`)
  - `func RecordSubtitleResolve(durationSeconds float64, outcomes []SubtitleProviderOutcome)`
- Used by Task 8.

- [ ] **Step 1: Write the failing test**

```go
package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordSubtitleResolve(t *testing.T) {
	SubtitleProviderUp.Reset()
	SubtitleResolveTotal.Reset()

	RecordSubtitleResolve(0.5, []SubtitleProviderOutcome{
		{Provider: "jimaku", Status: "ok", Tracks: 3},
		{Provider: "opensubtitles", Status: "down", Tracks: 0},
	})

	if got := testutil.ToFloat64(SubtitleProviderUp.WithLabelValues("jimaku")); got != 1 {
		t.Fatalf("jimaku up = %v, want 1", got)
	}
	if got := testutil.ToFloat64(SubtitleProviderUp.WithLabelValues("opensubtitles")); got != 0 {
		t.Fatalf("opensubtitles up = %v, want 0", got)
	}
	if got := testutil.ToFloat64(SubtitleResolveTotal.WithLabelValues("jimaku", "ok")); got != 1 {
		t.Fatalf("jimaku ok total = %v, want 1", got)
	}
}

func TestRecordSubtitleResolve_UnconfiguredSkipsGauge(t *testing.T) {
	SubtitleProviderUp.Reset()
	RecordSubtitleResolve(0.1, []SubtitleProviderOutcome{
		{Provider: "opensubtitles", Status: "unconfigured", Tracks: 0},
	})
	// Unconfigured must NOT set the up gauge (neither up nor down — it's absent).
	if n := testutil.CollectAndCount(SubtitleProviderUp); n != 0 {
		t.Fatalf("expected no gauge series for unconfigured, got %d", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma && go test ./libs/metrics/ -run TestRecordSubtitleResolve`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement**

```go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SubtitleResolveTotal counts subtitle aggregation resolves per provider+status.
	// status ∈ {"ok","down","empty","unconfigured"}.
	SubtitleResolveTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "catalog_subtitle_resolve_total",
		Help: "Subtitle aggregation resolves by provider and status.",
	}, []string{"provider", "status"})

	// SubtitleResolveDuration is the wall time of a full (non-cached) FetchAll.
	SubtitleResolveDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "catalog_subtitle_resolve_duration_seconds",
		Help:    "Wall time of a non-cached subtitle aggregation resolve.",
		Buckets: prometheus.DefBuckets,
	})

	// SubtitleProviderUp is 1 when the provider answered, 0 when it failed.
	// Unconfigured providers emit NO series (see RecordSubtitleResolve).
	SubtitleProviderUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "catalog_subtitle_provider_up",
		Help: "1 if the subtitle provider answered the last resolve, else 0.",
	}, []string{"provider"})

	// SubtitleTracksReturned counts subtitle tracks merged, per provider.
	SubtitleTracksReturned = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "catalog_subtitle_tracks_returned_total",
		Help: "Subtitle tracks returned by each provider.",
	}, []string{"provider"})
)

// SubtitleProviderOutcome is one provider's result within a single resolve.
type SubtitleProviderOutcome struct {
	Provider string
	Status   string // "ok" | "down" | "empty" | "unconfigured"
	Tracks   int
}

// RecordSubtitleResolve emits metrics for one non-cached FetchAll.
func RecordSubtitleResolve(durationSeconds float64, outcomes []SubtitleProviderOutcome) {
	SubtitleResolveDuration.Observe(durationSeconds)
	for _, o := range outcomes {
		SubtitleResolveTotal.WithLabelValues(o.Provider, o.Status).Inc()
		if o.Tracks > 0 {
			SubtitleTracksReturned.WithLabelValues(o.Provider).Add(float64(o.Tracks))
		}
		switch o.Status {
		case "unconfigured":
			// no gauge — provider is intentionally off, not "down"
		case "down":
			SubtitleProviderUp.WithLabelValues(o.Provider).Set(0)
		default: // ok | empty — provider answered
			SubtitleProviderUp.WithLabelValues(o.Provider).Set(1)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/animeenigma && go test ./libs/metrics/ -run TestRecordSubtitleResolve`
Expected: PASS (2 tests)

- [ ] **Step 5: Commit**

```bash
git add libs/metrics/subtitles.go libs/metrics/subtitles_test.go
git commit -m "feat(metrics): subtitle resolve + per-provider uptime metrics"
```

---

## Task 8: Instrument `SubsAggregator.FetchAll`

**Files:**
- Modify: `services/catalog/internal/service/subs_aggregator.go` (`FetchAll`, ~line 83-160; `fetchOpenSubtitles` unconfigured branch ~line 222)
- Test: `services/catalog/internal/service/subs_aggregator_metrics_test.go` (create)

**Interfaces:**
- Consumes: `metrics.RecordSubtitleResolve`, `metrics.SubtitleProviderOutcome` (Task 7).

**Design:** instrument the **live path only** (after the cache-miss). Distinguish unconfigured OpenSubtitles from a real outage. The current `fetchOpenSubtitles` returns `errors.New("opensubtitles not configured")` when unconfigured — turn that into a sentinel so the live path can classify it.

- [ ] **Step 1: Write the failing test.** Create `subs_aggregator_metrics_test.go` with handwritten fakes for the Jimaku/OpenSubtitles clients (mirror the existing `subs_aggregator_test.go` fakes — read it for the constructor + fake shapes). Assert that after a `FetchAll` where Jimaku returns tracks and OpenSubtitles is unconfigured:

```go
func TestFetchAll_EmitsMetrics(t *testing.T) {
	metrics.SubtitleProviderUp.Reset()
	metrics.SubtitleResolveTotal.Reset()

	agg := newTestAggregator(t, /* jimaku returns 2 ja tracks, opensubs unconfigured */)
	_, err := agg.FetchAll(context.Background(), testAnimeID, 8, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(metrics.SubtitleProviderUp.WithLabelValues("jimaku")); got != 1 {
		t.Fatalf("jimaku up = %v want 1", got)
	}
	if got := testutil.ToFloat64(metrics.SubtitleResolveTotal.WithLabelValues("jimaku", "ok")); got != 1 {
		t.Fatalf("jimaku ok = %v want 1", got)
	}
	// unconfigured opensubtitles → no up gauge series
	if got := testutil.ToFloat64(metrics.SubtitleResolveTotal.WithLabelValues("opensubtitles", "unconfigured")); got != 1 {
		t.Fatalf("opensubtitles unconfigured = %v want 1", got)
	}
}
```

> If the existing test file builds the aggregator with real clients pointed at httptest servers, follow that exact approach instead — the key assertions are the three metric values above.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma && go test ./services/catalog/internal/service/ -run TestFetchAll_EmitsMetrics`
Expected: FAIL — no metrics emitted yet.

- [ ] **Step 3: Implement.** In `subs_aggregator.go`:

Add a sentinel error near the top of the file:
```go
// errProviderUnconfigured marks a provider that is intentionally off (no key),
// so metrics classify it as "unconfigured" rather than "down".
var errProviderUnconfigured = errors.New("subtitle provider not configured")
```
and in `fetchOpenSubtitles`, replace `return nil, errors.New("opensubtitles not configured")` with `return nil, errProviderUnconfigured`.

In `FetchAll`, wrap the live path with timing + outcome collection. After the cache-miss (the early `return &cached` stays untouched), add `start := time.Now()` before the goroutines, and replace the results-drain loop so it records per-provider outcomes:

```go
outcomes := make([]metrics.SubtitleProviderOutcome, 0, 2)
for r := range resultsCh {
	if r.err != nil {
		if errors.Is(r.err, errProviderUnconfigured) {
			outcomes = append(outcomes, metrics.SubtitleProviderOutcome{Provider: r.name, Status: "unconfigured"})
			continue // not "down", not in ProvidersDown
		}
		s.log.Warnw("subs aggregator: provider failed",
			"provider", r.name, "anime_id", animeID, "episode", episode, "error", r.err)
		resp.ProvidersDown = append(resp.ProvidersDown, r.name)
		outcomes = append(outcomes, metrics.SubtitleProviderOutcome{Provider: r.name, Status: "down"})
		continue
	}
	kept := 0
	for _, t := range r.tracks {
		if len(langs) > 0 && !containsLang(langs, t.Lang) {
			continue
		}
		resp.Languages[t.Lang] = append(resp.Languages[t.Lang], t)
		kept++
	}
	status := "ok"
	if kept == 0 {
		status = "empty"
	}
	outcomes = append(outcomes, metrics.SubtitleProviderOutcome{Provider: r.name, Status: status, Tracks: kept})
}

metrics.RecordSubtitleResolve(time.Since(start).Seconds(), outcomes)
```

Add the import `"github.com/ILITA-hub/animeenigma/libs/metrics"`. (`time` is already imported.)

> Behavior unchanged: `ProvidersDown` still excludes unconfigured providers (it always did — unconfigured returned an error and was appended; NOW unconfigured is split out so it neither pollutes `ProvidersDown` nor the down gauge). Verify the existing `subs_aggregator_test.go` still passes — if a test asserted `opensubtitles` appears in `ProvidersDown` when unconfigured, update it to expect it ABSENT (this is the intended correctness fix).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /data/animeenigma && go test ./services/catalog/internal/service/ -run "TestFetchAll" && go vet ./services/catalog/...`
Expected: PASS + clean vet

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/subs_aggregator.go services/catalog/internal/service/subs_aggregator_metrics_test.go
git commit -m "feat(catalog): emit subtitle resolve + provider-up metrics from FetchAll"
```

---

## Task 9: `subtitle-health` Grafana dashboard

**Files:**
- Create: `docker/grafana/dashboards/subtitle-health.json`

**Interfaces:** consumes the Prometheus metrics from Tasks 7-8. No code dependency.

- [ ] **Step 1: Inspect the sibling dashboard for the exact provisioning shape**

Run: `cd /data/animeenigma && python3 -m json.tool docker/grafana/dashboards/playback-health.json | head -40`
Note the top-level keys (`title`, `uid`, `panels`, `templating`, `schemaVersion`, datasource `uid`/`type`) and copy that envelope so the new dashboard provisions identically.

- [ ] **Step 2: Create the dashboard JSON.** Mirror `playback-health.json`'s envelope (same Prometheus datasource ref + schemaVersion). Panels:

  1. **Per-provider uptime** (timeseries) — `catalog_subtitle_provider_up` (legend `{{provider}}`, min 0 / max 1) — the Jimaku/OpenSubtitles outage view.
  2. **Resolve rate by status** (timeseries) — `sum by (status) (rate(catalog_subtitle_resolve_total[5m]))`.
  3. **Resolve latency p50/p95** (timeseries) — `histogram_quantile(0.5, sum(rate(catalog_subtitle_resolve_duration_seconds_bucket[5m])) by (le))` and the 0.95 variant.
  4. **Tracks returned by provider** (timeseries) — `sum by (provider) (rate(catalog_subtitle_tracks_returned_total[5m]))`.
  5. **Providers currently down** (stat) — `count(catalog_subtitle_provider_up == 0)` (0 = all healthy).

  Use a fresh `uid` (e.g. `"subtitle-health"`) and `title": "Subtitle Health"`. Keep panel `gridPos` non-overlapping.

- [ ] **Step 3: Validate the JSON**

Run: `cd /data/animeenigma && python3 -m json.tool docker/grafana/dashboards/subtitle-health.json > /dev/null && echo OK`
Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add docker/grafana/dashboards/subtitle-health.json
git commit -m "feat(grafana): subtitle-health dashboard (resolve + per-provider uptime)"
```

---

## Final Verification (before after-update)

- [ ] **FE gates:** `cd frontend/web && bunx vue-tsc --noEmit && bash scripts/design-system-lint.sh && bunx vitest run src/composables/aePlayer src/components/player/aePlayer src/utils/subtitleProxy.spec.ts src/locales` — all green.
- [ ] **BE gates:** `cd /data/animeenigma && go build ./... && go vet ./services/catalog/... ./libs/metrics/... && go test ./libs/metrics/ ./services/catalog/internal/service/`.
- [ ] **Grafana JSON validates** (Task 9 Step 3).
- [ ] **Deploy** (from the clean worktree, per Global Constraints): `make redeploy-web` and `make redeploy-catalog`; `make restart-grafana` (provisioned dashboard pickup). Then `make health`.
- [ ] **Smoke:** load a watch page, open Browse Subtitles → tracks listed (not empty); confirm a SUB/raw episode auto-shows subs; check `curl -s localhost:8081/metrics | grep catalog_subtitle_` shows the new series after one resolve; confirm the Subtitle Health dashboard renders in Grafana.
- [ ] Invoke `/animeenigma-after-update` (deploy + Russian-Trump changelog + commit + push).

## Notes for the implementer

- **Order matters:** Task 2 before Task 1 (Task 1 imports the util). Tasks 3-4 depend on Task 1's types and the modal's `SubTrack`. Task 6 depends on 1-5. Tasks 7-9 are independent of the frontend and can run in parallel with A.
- **Chrome smoke is opt-in** (DS-NF-06). The modal's new states are cascade-sensitive (loading/empty swap); mention this when offering the owner a checkup — jsdom can't catch Tailwind-v4 cascade bugs.
- **Don't add subtitle CDNs to the proxy allowlist** — rely on the catalog signing (`exp`/`sig`). Provider track URLs are already signed; forwarding them is the whole job.
