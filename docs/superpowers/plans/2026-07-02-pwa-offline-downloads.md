# PWA + Offline Downloads Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make AnimeEnigma an installable PWA with silent auto-updating bundles, and let users download episodes (any source served through our HLS/MP4 proxy) for fully-offline playback inside the existing AePlayer.

**Architecture:** Two phases sharing one service worker. Phase 1 (Tasks 1–4): `vite-plugin-pwa` in `injectManifest` mode with a hand-written `src/sw.ts` (Workbox precaching + navigation fallback), self-managed registration with a kill-switch and a playback-aware deferred reload. Phase 2 (Tasks 5–13): a foreground download engine that resolves streams exactly like the player, rewrites HLS playlists to synthetic `/__offline/{id}/…` URLs, stores segments/keys/subs in Cache Storage and metadata in IndexedDB; the SW serves `/__offline/*` from cache (with Range support for MP4). Offline playback reuses AePlayer via an **offline resolver adapter** that implements the existing `ProviderResolver` interface plus a synthetic one-provider capability report — the player's default-selection machinery then works unchanged.

**Tech Stack:** Vue 3 + Vite 5 + TS, `vite-plugin-pwa@^0.21.2` (workbox 7), Cache Storage API, IndexedDB (hand-rolled wrapper), vitest + `fake-indexeddb`, bun.

**Spec:** `docs/superpowers/specs/2026-07-02-pwa-offline-downloads-design.md` (owner-approved 2026-07-02).

## Global Constraints

- Frontend tooling is **bun** (`bun install`, `bun run build`, `bunx vitest run …`, `bunx tsc --noEmit`) — never npm/npx.
- All work in a **git worktree** off fresh `origin/main`; never edit `/data/animeenigma` directly; **ONE FE agent per worktree** (parallel FE agents in one worktree corrupt each other's `node_modules`/dist). Worktree needs `bun install` before building.
- Commit per task with pathspec (`git commit <paths> -m …`) and the standing co-author trailer:
  `Co-Authored-By: Claude Code <noreply@anthropic.com>` / `Co-Authored-By: 0neymik0 <0neymik0@gmail.com>` / `Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`.
- DS lint is build-enforced (8 rules). New `.vue` files: semantic tokens only, no off-palette Tailwind colors, no raw hex/rgba, `font-medium|font-semibold` only, 4px spacing scale, `@/components/ui` primitives outside `components/player/` (player dir is exempt from the native-controls rule).
- Every new i18n key goes to **all three** of `src/locales/{en,ru,ja}.json` in the same commit — `locale-parity.spec.ts` fails otherwise.
- `hls.js` stays pinned `~1.5.20`. Do not touch `useVideoEngine.ts`.
- API envelope is `{success, data}` — unwrap with `resp.data?.data ?? resp.data`.
- Type-only imports from `.vue` files cause TS2614 — put shared types in `.ts` files. `vue-tsc --noEmit` can false-pass on cache; the real gate is `bun run build`.
- The plan's verbatim code is the contract; where a step says "anchor: …" the executor must locate the anchor in the named file and apply the shown pattern there (used only for `AePlayer.vue`, ~2800 lines, whose internals shift).
- Feature flag: PWA shell ships unflagged; all download UI gates on `VITE_OFFLINE_DOWNLOADS_ENABLED !== 'false'` (default ON) **and** SW availability.
- **Portability (owner, 2026-07-03):** future standalone apps will reuse this code — media-byte I/O goes ONLY through the `OfflineMediaStore` port (Task 7b); no direct Cache Storage / `navigator.storage` calls outside `mediaStore.ts` and the SW's serving half (`offlineServe.ts`, web-adapter-specific by design).

---

# Phase 1 — PWA shell («сборка»)

### Task 1: vite-plugin-pwa + manifest + buildable SW stub

**Files:**
- Modify: `frontend/web/package.json` (deps via bun)
- Modify: `frontend/web/vite.config.ts`
- Create: `frontend/web/src/sw.ts` (stub; Task 2 fills it)
- Create: `frontend/web/src/pwa/swRoutes.ts` (empty exports placeholder-free: created in Task 2 — NOT here)

**Interfaces:**
- Produces: `dist/sw.js`, `dist/manifest.webmanifest`, `<link rel="manifest">` injected into built `index.html`. `self.__WB_MANIFEST` injection point in `src/sw.ts`.

- [ ] **Step 1: Add dependencies**

```bash
cd frontend/web
bun add -d vite-plugin-pwa@^0.21.2 workbox-precaching@^7.3.0 workbox-routing@^7.3.0 workbox-core@^7.3.0
```

- [ ] **Step 2: Extend `vite.config.ts`**

Add the import and plugin entry (keep every existing plugin/option untouched):

```ts
import { VitePWA } from 'vite-plugin-pwa'
```

Inside `plugins: [ … ]`, after the two `compression(...)` entries:

```ts
    VitePWA({
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.ts',
      // We self-manage registration (kill-switch + playback-aware reload in
      // src/pwa/registerPwa.ts) — the plugin only builds sw.js + manifest.
      injectRegister: false,
      manifest: {
        name: 'AnimeEnigma',
        short_name: 'AnimeEnigma',
        description: 'Anime streaming platform',
        lang: 'ru',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        theme_color: '#08080f',
        background_color: '#08080f',
        icons: [
          { src: '/android-chrome-192x192.png', sizes: '192x192', type: 'image/png' },
          { src: '/android-chrome-512x512.png', sizes: '512x512', type: 'image/png' },
          // Same art declared maskable — acceptable v1 (logo sits centered);
          // dedicated safe-zone art can replace it later without code changes.
          { src: '/android-chrome-512x512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      injectManifest: {
        globPatterns: ['**/*.{js,css,html,woff2,svg,png,ico,webmanifest}'],
        // .gz/.br twins are nginx-only; changelog.json is fetched fresh every
        // page load by design; branding/ is heavy static art.
        globIgnores: ['**/*.{gz,br}', 'changelog.json', 'branding/**'],
        maximumFileSizeToCacheInBytes: 3 * 1024 * 1024,
      },
      devOptions: { enabled: false },
    }),
```

- [ ] **Step 3: Create minimal `src/sw.ts`** (compilable stub; real routes in Task 2)

```ts
/// <reference lib="webworker" />
declare let self: ServiceWorkerGlobalScope & {
  __WB_MANIFEST: Array<import('workbox-precaching').PrecacheEntry | string>
}

import { precacheAndRoute } from 'workbox-precaching'
import { clientsClaim } from 'workbox-core'

self.skipWaiting()
clientsClaim()
precacheAndRoute(self.__WB_MANIFEST)
```

- [ ] **Step 4: Build and verify artifacts**

```bash
cd frontend/web && bun install && bun run build
ls dist/sw.js dist/manifest.webmanifest
grep -c 'rel="manifest"' dist/index.html
```
Expected: build passes (vue-tsc + vite); both files exist; grep prints `1`.
If vue-tsc still errors on webworker globals, move the declare into a new `src/sw-env.d.ts` with the same triple-slash reference as line 1 — do NOT add `webworker` to the tsconfig `lib` array (that would leak worker globals into app code).

- [ ] **Step 5: Verify precache excludes**

```bash
node -e "const s=require('fs').readFileSync('dist/sw.js','utf8'); console.log(/changelog\.json/.test(s), /\.br/.test(s))"
```
Expected: `false false`.

- [ ] **Step 6: Commit**

```bash
git add package.json bun.lock vite.config.ts src/sw.ts
git commit package.json bun.lock vite.config.ts src/sw.ts -m "feat(pwa): manifest + injectManifest SW build scaffold"
```

---

### Task 2: SW routes — navigation fallback, edge-asset fallback, offline stub

**Files:**
- Create: `frontend/web/src/pwa/swRoutes.ts`
- Create: `frontend/web/src/pwa/swRoutes.spec.ts`
- Modify: `frontend/web/src/sw.ts`

**Interfaces:**
- Consumes: `self.__WB_MANIFEST` (Task 1).
- Produces: `NAV_DENYLIST: RegExp[]`, `edgeAssetToOriginPath(requestUrl: string, origin: string): string | null`, `isOfflinePath(pathname: string): boolean`. Task 8 replaces the `/__offline/` 404-stub handler with the real one.

- [ ] **Step 1: Write failing tests** — `src/pwa/swRoutes.spec.ts`

```ts
import { describe, it, expect } from 'vitest'
import { NAV_DENYLIST, edgeAssetToOriginPath, isOfflinePath } from './swRoutes'

const denied = (path: string) => NAV_DENYLIST.some((re) => re.test(path))

describe('NAV_DENYLIST', () => {
  it('denies API, OG, socket, admin infra, health, sw files, offline ns', () => {
    for (const p of ['/api/anime/x', '/og/home', '/socket.io/?x=1', '/admin/grafana/d/1',
      '/admin/prometheus/graph', '/admin/pgadmin/', '/admin/k8s/', '/health',
      '/sw.js', '/sw-config.json', '/__offline/abc/master.m3u8']) {
      expect(denied(p), p).toBe(true)
    }
  })
  it('allows SPA routes including /admin/feedback (SPA admin UI)', () => {
    for (const p of ['/', '/anime/uuid-1', '/downloads', '/browse', '/admin/feedback']) {
      expect(denied(p), p).toBe(false)
    }
  })
})

describe('edgeAssetToOriginPath', () => {
  const origin = 'https://animeenigma.org'
  it('maps cross-origin /assets/ chunk to same-path origin URL', () => {
    expect(edgeAssetToOriginPath('https://msk-edge.example/assets/chunk-abc.js', origin))
      .toBe('https://animeenigma.org/assets/chunk-abc.js')
  })
  it('returns null for same-origin, non-assets, and garbage', () => {
    expect(edgeAssetToOriginPath('https://animeenigma.org/assets/a.js', origin)).toBeNull()
    expect(edgeAssetToOriginPath('https://msk-edge.example/api/x', origin)).toBeNull()
    expect(edgeAssetToOriginPath('not a url', origin)).toBeNull()
  })
})

describe('isOfflinePath', () => {
  it('matches only the /__offline/ namespace', () => {
    expect(isOfflinePath('/__offline/id1/master.m3u8')).toBe(true)
    expect(isOfflinePath('/offline')).toBe(false)
  })
})
```

- [ ] **Step 2: Run to verify failure**

```bash
bunx vitest run src/pwa/swRoutes.spec.ts
```
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `src/pwa/swRoutes.ts`**

```ts
// Pure helpers for src/sw.ts — kept out of the SW bundle entry so vitest can
// exercise them under jsdom without a ServiceWorkerGlobalScope.

/** Navigations that must NEVER get the SPA shell. /admin/feedback etc. are SPA
 *  routes; only the four proxied infra dashboards are denied. */
export const NAV_DENYLIST: RegExp[] = [
  /^\/api\//,
  /^\/og\//,
  /^\/socket\.io/,
  /^\/__offline\//,
  /^\/admin\/(grafana|prometheus|pgadmin|k8s)(\/|$)/,
  /^\/health$/,
  /^\/sw\.js$/,
  /^\/sw-config\.json$/,
]

/** RU static-edge (Maskanya) chunk request → equivalent origin URL, so the SW
 *  can serve the precached origin copy when the edge is unreachable offline.
 *  Null ⇒ not an edge asset request (same-origin or unrelated host/path). */
export function edgeAssetToOriginPath(requestUrl: string, origin: string): string | null {
  try {
    const u = new URL(requestUrl)
    if (u.origin === origin) return null
    if (!u.pathname.startsWith('/assets/')) return null
    return origin + u.pathname
  } catch {
    return null
  }
}

export function isOfflinePath(pathname: string): boolean {
  return pathname.startsWith('/__offline/')
}
```

- [ ] **Step 4: Run tests**

```bash
bunx vitest run src/pwa/swRoutes.spec.ts
```
Expected: PASS (all).

- [ ] **Step 5: Wire into `src/sw.ts`** (replace stub body below the imports)

```ts
/// <reference lib="webworker" />
declare let self: ServiceWorkerGlobalScope & {
  __WB_MANIFEST: Array<import('workbox-precaching').PrecacheEntry | string>
}

import { precacheAndRoute, createHandlerBoundToURL, getCacheKeyForURL } from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'
import { clientsClaim } from 'workbox-core'
import { NAV_DENYLIST, edgeAssetToOriginPath, isOfflinePath } from './pwa/swRoutes'

self.skipWaiting()
clientsClaim()
precacheAndRoute(self.__WB_MANIFEST)

// SPA shell for navigations (denylist: API/OG/proxied admin/etc).
registerRoute(new NavigationRoute(createHandlerBoundToURL('/index.html'), { denylist: NAV_DENYLIST }))

// Downloaded-episode namespace. Task 8 replaces this 404 stub with
// handleOfflineRequest (Cache Storage + MP4 Range).
registerRoute(
  ({ url }) => isOfflinePath(url.pathname),
  async () => new Response('offline store not implemented', { status: 404 }),
)

// RU edge chunk fallback: network first, precached origin copy when offline.
registerRoute(
  ({ url }) => edgeAssetToOriginPath(url.href, self.location.origin) !== null,
  async ({ request }) => {
    try {
      return await fetch(request)
    } catch {
      const originUrl = edgeAssetToOriginPath(request.url, self.location.origin)
      const key = originUrl ? getCacheKeyForURL(originUrl) : undefined
      const cached = key ? await caches.match(key) : undefined
      return cached ?? Response.error()
    }
  },
)
```

- [ ] **Step 6: Build**

```bash
bun run build
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/pwa/swRoutes.ts src/pwa/swRoutes.spec.ts src/sw.ts
git commit src/pwa/swRoutes.ts src/pwa/swRoutes.spec.ts src/sw.ts -m "feat(pwa): SW navigation fallback, edge-asset fallback, offline namespace stub"
```

---

### Task 3: Registration — kill-switch, update polling, playback-deferred reload

**Files:**
- Create: `frontend/web/src/pwa/registerPwa.ts`
- Create: `frontend/web/src/pwa/registerPwa.spec.ts`
- Create: `frontend/web/public/sw-config.json`
- Modify: `frontend/web/src/main.ts`

**Interfaces:**
- Produces: `initPwa(): Promise<void>` (main.ts calls it), `shouldDeferReload(doc: Document): boolean`, `scheduleReload(doc: Document, reload: () => void): void` (exported for tests).
- `public/sw-config.json` contract: `{"kill": boolean}` — ops flips `kill: true` to unregister the SW fleet-wide without a rebuild.

- [ ] **Step 1: Create `public/sw-config.json`**

```json
{ "kill": false }
```

- [ ] **Step 2: Write failing tests** — `src/pwa/registerPwa.spec.ts`

```ts
import { describe, it, expect, vi, afterEach } from 'vitest'
import { shouldDeferReload, scheduleReload } from './registerPwa'

function docWithVideo({ paused = false, ended = false, readyState = 4 } = {}): Document {
  const doc = document.implementation.createHTMLDocument('')
  const v = doc.createElement('video')
  Object.defineProperty(v, 'paused', { value: paused })
  Object.defineProperty(v, 'ended', { value: ended })
  Object.defineProperty(v, 'readyState', { value: readyState })
  doc.body.appendChild(v)
  return doc
}

afterEach(() => vi.useRealTimers())

describe('shouldDeferReload', () => {
  it('defers while a video is actively playing', () => {
    expect(shouldDeferReload(docWithVideo())).toBe(true)
  })
  it('does not defer for paused/ended/not-started videos', () => {
    expect(shouldDeferReload(docWithVideo({ paused: true }))).toBe(false)
    expect(shouldDeferReload(docWithVideo({ ended: true }))).toBe(false)
    expect(shouldDeferReload(docWithVideo({ readyState: 0 }))).toBe(false)
  })
  it('defers while a Kodik iframe is mounted (classic fallback playback)', () => {
    const doc = document.implementation.createHTMLDocument('')
    const f = doc.createElement('iframe')
    f.src = 'https://kodik.info/serial/x'
    doc.body.appendChild(f)
    expect(shouldDeferReload(doc)).toBe(true)
  })
  it('does not defer on a plain page', () => {
    expect(shouldDeferReload(document.implementation.createHTMLDocument(''))).toBe(false)
  })
})

describe('scheduleReload', () => {
  it('reloads immediately when nothing is playing', () => {
    const reload = vi.fn()
    scheduleReload(document.implementation.createHTMLDocument(''), reload)
    expect(reload).toHaveBeenCalledTimes(1)
  })
  it('polls until playback stops, then reloads once', () => {
    vi.useFakeTimers()
    const doc = docWithVideo()
    const reload = vi.fn()
    scheduleReload(doc, reload)
    expect(reload).not.toHaveBeenCalled()
    vi.advanceTimersByTime(15_000)
    expect(reload).not.toHaveBeenCalled()
    doc.querySelector('video')!.remove()
    vi.advanceTimersByTime(15_000)
    expect(reload).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 3: Run to verify failure**

```bash
bunx vitest run src/pwa/registerPwa.spec.ts
```
Expected: FAIL — module not found.

- [ ] **Step 4: Implement `src/pwa/registerPwa.ts`**

```ts
// Self-managed SW registration. Not using virtual:pwa-register — its autoUpdate
// client reloads unconditionally on controllerchange, which would kill playback
// mid-episode. We reload only when nothing is playing (deploy mid-anime waits).

/** True while media is actively playing: any HTML5 <video> mid-playback, or a
 *  Kodik iframe mounted (classic fallback — its playback state is opaque, so
 *  its mere presence defers). */
export function shouldDeferReload(doc: Document): boolean {
  const videos = Array.from(doc.querySelectorAll('video'))
  if (videos.some((v) => !v.paused && !v.ended && v.readyState > 2)) return true
  if (doc.querySelector('iframe[src*="kodik"]')) return true
  return false
}

/** Reload now, or poll every 15s until playback stops (deferred deploy pickup). */
export function scheduleReload(doc: Document, reload: () => void): void {
  if (!shouldDeferReload(doc)) {
    reload()
    return
  }
  const timer = setInterval(() => {
    if (!shouldDeferReload(doc)) {
      clearInterval(timer)
      reload()
    }
  }, 15_000)
}

async function killSwitchActive(): Promise<boolean> {
  try {
    const r = await fetch('/sw-config.json', { cache: 'no-cache' })
    if (!r.ok) return false
    const cfg = (await r.json()) as { kill?: boolean }
    return cfg.kill === true
  } catch {
    return false // config unreachable ≠ kill
  }
}

async function unregisterAll(): Promise<void> {
  const regs = await navigator.serviceWorker.getRegistrations()
  await Promise.all(regs.map((r) => r.unregister()))
  // ONLY the workbox app-shell caches. ae-offline-* holds user-downloaded
  // episodes — the kill-switch disables a broken SW, it must never destroy
  // user data (downloads become unplayable until re-registration, not gone).
  const keys = await caches.keys()
  await Promise.all(keys.filter((k) => k.startsWith('workbox-')).map((k) => caches.delete(k)))
}

export async function initPwa(): Promise<void> {
  if (!('serviceWorker' in navigator)) return
  if (!import.meta.env.PROD) return // sw.js only exists in built output

  if (await killSwitchActive()) {
    await unregisterAll().catch(() => {})
    return
  }

  let hadController = !!navigator.serviceWorker.controller
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    if (!hadController) {
      hadController = true // first install claiming the page — not an update
      return
    }
    scheduleReload(document, () => window.location.reload())
  })

  try {
    const reg = await navigator.serviceWorker.register('/sw.js', { scope: '/' })
    // Long-lived SPA sessions: re-check hourly and whenever the tab returns.
    setInterval(() => void reg.update(), 60 * 60 * 1000)
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') void reg.update()
    })
  } catch {
    // registration failure is non-fatal — app works SW-less
  }
}
```

- [ ] **Step 5: Run tests**

```bash
bunx vitest run src/pwa/registerPwa.spec.ts
```
Expected: PASS.

- [ ] **Step 6: Wire into `src/main.ts`**

Inside the existing `deferInit(...)` block (the `requestIdleCallback` section near the bottom that lazy-imports `./utils/diagnostics` and `./analytics`), add:

```ts
  void import('./pwa/registerPwa').then((m) => m.initPwa())
```

- [ ] **Step 7: Build + full unit suite**

```bash
bun run build && bunx vitest run src/pwa/
```
Expected: both PASS.

- [ ] **Step 8: Commit**

```bash
git add public/sw-config.json src/pwa/registerPwa.ts src/pwa/registerPwa.spec.ts src/main.ts
git commit public/sw-config.json src/pwa/registerPwa.ts src/pwa/registerPwa.spec.ts src/main.ts -m "feat(pwa): SW registration with kill-switch and playback-deferred auto-update"
```

---

### Task 4: nginx — sw.js must never be immutable-cached

**Files:**
- Modify: `frontend/web/nginx.conf`

**Interfaces:**
- Produces: `/sw.js` served `no-cache` (exact-match location beats the `\.js$` 1y-immutable regex). `manifest.webmanifest` and `sw-config.json` already fall to `location /` (no-cache) — their extensions are not in the static regex.

- [ ] **Step 1: Add the exact-match location** — in `nginx.conf`, directly above the `# Cache static assets` block:

```nginx
    # Service worker must revalidate on every check — an immutable-cached sw.js
    # would freeze clients on an old precache manifest forever. Exact match
    # takes precedence over the \.js$ 1y-immutable regex below.
    location = /sw.js {
        add_header Cache-Control "no-cache, no-store, must-revalidate";
        try_files $uri =404;
    }
```

- [ ] **Step 2: Validate config syntax**

```bash
docker run --rm -v "$(pwd)/frontend/web/nginx.conf":/etc/nginx/conf.d/default.conf:ro nginx:alpine nginx -t
```
Expected: `syntax is ok` + `test is successful`. (`brotli_static` may warn as unknown in the vanilla image — if it errors, comment that line for the syntax check only, or skip this step and rely on the deploy health check.)

- [ ] **Step 3: Commit**

```bash
git add frontend/web/nginx.conf
git commit frontend/web/nginx.conf -m "feat(pwa): serve sw.js with no-cache so SW updates propagate"
```

---

# Phase 2 — offline download module («модуль скачки»)

### Task 5: Offline types + IndexedDB registry

**Files:**
- Create: `frontend/web/src/offline/types.ts`
- Create: `frontend/web/src/offline/registry.ts`
- Create: `frontend/web/src/offline/registry.spec.ts`
- Modify: `frontend/web/package.json` (`fake-indexeddb` devDep)

**Interfaces:**
- Consumes: `Combo`, `StreamResult`, `SubtitleTrack` from `@/types/aePlayer`; `EpisodeOption` from `@/components/player/EpisodeSelector.types`.
- Produces (used by Tasks 7–12):
  - `downloadId(animeId, epNumber, combo, quality): string`
  - `OfflineDownload` record type, `DownloadState = 'queued'|'downloading'|'paused'|'done'|'error'`
  - `putDownload(d)`, `getDownload(id)`, `listDownloads()`, `deleteDownloadRecord(id)` — all `Promise`-based
  - `enqueuePending(payload)`, `drainPending(handler)` — the `pending_progress` store (Task 9)
  - `offlineCacheName(id): string` → `ae-offline-{id}`; `offlinePath(id, rest): string` → `/__offline/{id}/{rest}`

- [ ] **Step 1: Add test dep**

```bash
bun add -d fake-indexeddb@^6.0.0
```

- [ ] **Step 2: Create `src/offline/types.ts`**

```ts
import type { Combo, SubtitleTrack } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

export type DownloadState = 'queued' | 'downloading' | 'paused' | 'done' | 'error'
export type DownloadError = 'network' | 'quota' | 'evicted' | 'resolve' | 'mismatch'

export interface OfflineDownload {
  /** Canonical key — NEVER a raw URL (signed proxy URLs expire hourly). */
  id: string
  animeId: string
  animeTitle: string
  episode: EpisodeOption
  combo: Combo
  quality: string // target: '480' | '720' | '1080'
  streamType: 'hls' | 'mp4'
  state: DownloadState
  error?: DownloadError
  bytes: number
  resourcesDone: number
  resourcesTotal: number
  createdAt: number
  /** Local resume position, written by offline playback. */
  lastPositionSec?: number
  /** Entry URL for the player: /__offline/{id}/master.m3u8 or /__offline/{id}/media.mp4 */
  playlistLocalPath: string
  /** Subtitle tracks rewritten to /__offline/{id}/sub/{k} local URLs. */
  subtitles: SubtitleTrack[]
  /** /__offline/{id}/poster when the poster fetch succeeded. */
  posterPath?: string
}

export function downloadId(animeId: string, epNumber: number, combo: Combo, quality: string): string {
  return [animeId, epNumber, combo.provider, combo.audio, combo.lang, combo.team ?? '', quality].join(':')
}

export function offlineCacheName(id: string): string {
  return `ae-offline-${id}`
}

export function offlinePath(id: string, rest: string): string {
  return `/__offline/${encodeURIComponent(id)}/${rest}`
}
```

- [ ] **Step 3: Write failing tests** — `src/offline/registry.spec.ts`

```ts
import 'fake-indexeddb/auto'
import { describe, it, expect, beforeEach } from 'vitest'
import { putDownload, getDownload, listDownloads, deleteDownloadRecord, enqueuePending, drainPending, _resetDbForTests } from './registry'
import { downloadId, offlinePath, type OfflineDownload } from './types'

function sample(id: string): OfflineDownload {
  return {
    id, animeId: 'a1', animeTitle: 'Test', quality: '720', streamType: 'hls',
    episode: { key: 1, label: 1, number: 1 },
    combo: { audio: 'sub', lang: 'en', provider: 'gogoanime', server: 's1', team: null },
    state: 'queued', bytes: 0, resourcesDone: 0, resourcesTotal: 0, createdAt: 1,
    playlistLocalPath: offlinePath(id, 'master.m3u8'), subtitles: [],
  }
}

beforeEach(() => _resetDbForTests())

describe('registry CRUD', () => {
  it('put/get/list/delete round-trips', async () => {
    const id = downloadId('a1', 1, sample('x').combo, '720')
    await putDownload(sample(id))
    expect((await getDownload(id))?.animeTitle).toBe('Test')
    expect((await listDownloads()).map((d) => d.id)).toEqual([id])
    await deleteDownloadRecord(id)
    expect(await getDownload(id)).toBeUndefined()
  })
  it('put overwrites by id (state transition)', async () => {
    await putDownload(sample('k'))
    await putDownload({ ...sample('k'), state: 'done' })
    expect((await getDownload('k'))?.state).toBe('done')
    expect((await listDownloads()).length).toBe(1)
  })
})

describe('pending_progress queue', () => {
  it('drains FIFO and deletes handled entries; stops on handler failure', async () => {
    await enqueuePending({ n: 1 })
    await enqueuePending({ n: 2 })
    await enqueuePending({ n: 3 })
    const handled: number[] = []
    const ok = await drainPending(async (p) => {
      const n = (p as { n: number }).n
      if (n === 3) return false // simulate network failure
      handled.push(n)
      return true
    })
    expect(handled).toEqual([1, 2])
    expect(ok).toBe(false)
    // entry 3 survives for the next drain
    const rest: number[] = []
    await drainPending(async (p) => { rest.push((p as { n: number }).n); return true })
    expect(rest).toEqual([3])
  })
})
```

- [ ] **Step 4: Run to verify failure**

```bash
bunx vitest run src/offline/registry.spec.ts
```
Expected: FAIL — module not found.

- [ ] **Step 5: Implement `src/offline/registry.ts`**

```ts
import type { OfflineDownload } from './types'

const DB_NAME = 'ae-offline'
const DB_VERSION = 1
const DOWNLOADS = 'downloads'
const PENDING = 'pending_progress'

let dbPromise: Promise<IDBDatabase> | null = null

function openDb(): Promise<IDBDatabase> {
  if (!dbPromise) {
    dbPromise = new Promise((resolve, reject) => {
      const req = indexedDB.open(DB_NAME, DB_VERSION)
      req.onupgradeneeded = () => {
        const db = req.result
        if (!db.objectStoreNames.contains(DOWNLOADS)) db.createObjectStore(DOWNLOADS, { keyPath: 'id' })
        if (!db.objectStoreNames.contains(PENDING)) db.createObjectStore(PENDING, { autoIncrement: true })
      }
      req.onsuccess = () => resolve(req.result)
      req.onerror = () => reject(req.error)
    })
  }
  return dbPromise
}

/** Test hook: fake-indexeddb persists per-process; reset between specs. */
export async function _resetDbForTests(): Promise<void> {
  if (dbPromise) (await dbPromise).close()
  dbPromise = null
  await new Promise<void>((resolve) => {
    const req = indexedDB.deleteDatabase(DB_NAME)
    req.onsuccess = req.onerror = req.onblocked = () => resolve()
  })
}

function reqAsPromise<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

async function store(name: string, mode: IDBTransactionMode): Promise<IDBObjectStore> {
  return (await openDb()).transaction(name, mode).objectStore(name)
}

export async function putDownload(d: OfflineDownload): Promise<void> {
  await reqAsPromise((await store(DOWNLOADS, 'readwrite')).put(d))
}

export async function getDownload(id: string): Promise<OfflineDownload | undefined> {
  return reqAsPromise((await store(DOWNLOADS, 'readonly')).get(id))
}

export async function listDownloads(): Promise<OfflineDownload[]> {
  return reqAsPromise((await store(DOWNLOADS, 'readonly')).getAll())
}

export async function deleteDownloadRecord(id: string): Promise<void> {
  await reqAsPromise((await store(DOWNLOADS, 'readwrite')).delete(id))
}

// ── pending watch-progress queue (offline playback → flushed when online) ──

export async function enqueuePending(payload: unknown): Promise<void> {
  await reqAsPromise((await store(PENDING, 'readwrite')).add({ payload, queuedAt: Date.now() }))
}

/** FIFO-drain: handler returns true ⇒ entry deleted; false ⇒ stop, keep rest.
 *  Returns true when the queue fully drained. */
export async function drainPending(handler: (payload: unknown) => Promise<boolean>): Promise<boolean> {
  const s = await store(PENDING, 'readonly')
  const keys = await reqAsPromise(s.getAllKeys())
  const values = await reqAsPromise(s.getAll())
  for (let i = 0; i < keys.length; i++) {
    const ok = await handler((values[i] as { payload: unknown }).payload).catch(() => false)
    if (!ok) return false
    await reqAsPromise((await store(PENDING, 'readwrite')).delete(keys[i]))
  }
  return true
}
```

- [ ] **Step 6: Run tests**

```bash
bunx vitest run src/offline/registry.spec.ts
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add package.json bun.lock src/offline/types.ts src/offline/registry.ts src/offline/registry.spec.ts
git commit package.json bun.lock src/offline/types.ts src/offline/registry.ts src/offline/registry.spec.ts -m "feat(offline): download types + IndexedDB registry with pending-progress queue"
```

---

### Task 6: HLS playlist rewrite

**Files:**
- Create: `frontend/web/src/offline/playlistRewrite.ts`
- Create: `frontend/web/src/offline/playlistRewrite.spec.ts`

**Interfaces:**
- Produces (consumed by Task 7):
  - `selectVariant(masterBody: string, targetHeight: number): { uri: string } | null` — `null` ⇒ body is already a media playlist.
  - `rewriteMediaPlaylist(body: string, baseUrl: string, id: string): { body: string; resources: PlaylistResource[] }` where `PlaylistResource = { path: string; url: string }` (path is a local `/__offline/{id}/…` path, url is the absolute remote fetch URL).
  - `isVod(body: string): boolean` — `#EXT-X-ENDLIST` present.

- [ ] **Step 1: Write failing tests** — `src/offline/playlistRewrite.spec.ts`

```ts
import { describe, it, expect } from 'vitest'
import { selectVariant, rewriteMediaPlaylist, isVod } from './playlistRewrite'

const MASTER = `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
360/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2500000,RESOLUTION=1280x720
720/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080
1080/index.m3u8
`

const MEDIA = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-KEY:METHOD=AES-128,URI="https://cdn.example/key.bin",IV=0x1234
#EXT-X-MAP:URI="init.mp4"
#EXTINF:9.6,
seg-000.ts
#EXTINF:9.6,
https://other-cdn.example/seg-001.ts
#EXT-X-ENDLIST
`

describe('selectVariant', () => {
  it('picks the largest variant not exceeding target height', () => {
    expect(selectVariant(MASTER, 720)?.uri).toBe('720/index.m3u8')
    expect(selectVariant(MASTER, 900)?.uri).toBe('720/index.m3u8')
  })
  it('falls back to the smallest variant when all exceed target', () => {
    expect(selectVariant(MASTER, 200)?.uri).toBe('360/index.m3u8')
  })
  it('returns null for a media playlist (no STREAM-INF)', () => {
    expect(selectVariant(MEDIA, 720)).toBeNull()
  })
})

describe('rewriteMediaPlaylist', () => {
  const base = 'https://proxy.example/hls/ep1/index.m3u8'
  it('maps every URI (segments, KEY, MAP) to /__offline paths and resolves relatives', () => {
    const { body, resources } = rewriteMediaPlaylist(MEDIA, base, 'dl1')
    expect(body).toContain('URI="/__offline/dl1/k/0"')
    expect(body).toContain('#EXT-X-MAP:URI="/__offline/dl1/m/0"')
    expect(body).toContain('/__offline/dl1/r/0')
    expect(body).toContain('/__offline/dl1/r/1')
    expect(body).not.toContain('seg-000.ts')
    const urls = Object.fromEntries(resources.map((r) => [r.path, r.url]))
    expect(urls['/__offline/dl1/r/0']).toBe('https://proxy.example/hls/ep1/seg-000.ts')
    expect(urls['/__offline/dl1/r/1']).toBe('https://other-cdn.example/seg-001.ts')
    expect(urls['/__offline/dl1/k/0']).toBe('https://cdn.example/key.bin')
    expect(urls['/__offline/dl1/m/0']).toBe('https://proxy.example/hls/ep1/init.mp4')
  })
  it('keeps non-URI tags byte-identical', () => {
    const { body } = rewriteMediaPlaylist(MEDIA, base, 'dl1')
    expect(body).toContain('#EXT-X-TARGETDURATION:10')
    expect(body).toContain('IV=0x1234')
    expect(body).toContain('#EXT-X-ENDLIST')
  })
})

describe('isVod', () => {
  it('true only with ENDLIST', () => {
    expect(isVod(MEDIA)).toBe(true)
    expect(isVod(MASTER)).toBe(false)
  })
})
```

- [ ] **Step 2: Run to verify failure**

```bash
bunx vitest run src/offline/playlistRewrite.spec.ts
```
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `src/offline/playlistRewrite.ts`**

```ts
// Line-based m3u8 rewriting for offline storage. Deliberately NOT a full HLS
// parser — anime provider playlists are simple VOD lists; hls.js re-parses the
// rewritten output at playback, so structural fidelity is what matters.
import { offlinePath } from './types'

export interface PlaylistResource {
  path: string
  url: string
}

export function isVod(body: string): boolean {
  return body.includes('#EXT-X-ENDLIST')
}

/** Master playlist → variant URI closest to target (largest height ≤ target,
 *  else the smallest available). Null ⇒ no #EXT-X-STREAM-INF (media playlist). */
export function selectVariant(masterBody: string, targetHeight: number): { uri: string } | null {
  const lines = masterBody.split('\n')
  const variants: { height: number; uri: string }[] = []
  for (let i = 0; i < lines.length; i++) {
    if (!lines[i].startsWith('#EXT-X-STREAM-INF')) continue
    const res = /RESOLUTION=(\d+)x(\d+)/.exec(lines[i])
    const height = res ? parseInt(res[2], 10) : 0
    for (let j = i + 1; j < lines.length; j++) {
      const l = lines[j].trim()
      if (l === '' || l.startsWith('#')) continue
      variants.push({ height, uri: l })
      break
    }
  }
  if (variants.length === 0) return null
  const fitting = variants.filter((v) => v.height <= targetHeight)
  const pick = fitting.length
    ? fitting.reduce((a, b) => (b.height > a.height ? b : a))
    : variants.reduce((a, b) => (b.height < a.height ? b : a))
  return { uri: pick.uri }
}

function absolute(uri: string, baseUrl: string): string {
  // baseUrl is usually a ROOT-RELATIVE proxy path (/api/streaming/hls-proxy?…)
  // — hlsProxyUrl() emits relative URLs unless VITE_HLS_PROXY_BASE is set, and
  // new URL() throws "Invalid base URL" on a relative base. Anchor on the
  // document origin. (The proxy itself rewrites child URIs — segments AND
  // EXT-X-KEY/EXT-X-MAP — to root-relative /api/streaming/hls-proxy?… URLs,
  // libs/videoutils/proxy.go, so this path is the COMMON case, not the edge.)
  return new URL(uri, new URL(baseUrl, window.location.href)).href
}

/** Rewrite a MEDIA playlist: every segment URI → /__offline/{id}/r/{n}, every
 *  EXT-X-KEY URI → /k/{n}, every EXT-X-MAP URI → /m/{n}. Returns the rewritten
 *  body plus the local-path → remote-URL fetch list. */
export function rewriteMediaPlaylist(
  body: string,
  baseUrl: string,
  id: string,
): { body: string; resources: PlaylistResource[] } {
  const resources: PlaylistResource[] = []
  let seg = 0
  let key = 0
  let map = 0
  const out = body.split('\n').map((line) => {
    const t = line.trim()
    if (t.startsWith('#EXT-X-KEY') && t.includes('URI="')) {
      return line.replace(/URI="([^"]+)"/, (_, uri: string) => {
        const path = offlinePath(id, `k/${key++}`)
        resources.push({ path, url: absolute(uri, baseUrl) })
        return `URI="${path}"`
      })
    }
    if (t.startsWith('#EXT-X-MAP') && t.includes('URI="')) {
      return line.replace(/URI="([^"]+)"/, (_, uri: string) => {
        const path = offlinePath(id, `m/${map++}`)
        resources.push({ path, url: absolute(uri, baseUrl) })
        return `URI="${path}"`
      })
    }
    if (t !== '' && !t.startsWith('#')) {
      const path = offlinePath(id, `r/${seg++}`)
      resources.push({ path, url: absolute(t, baseUrl) })
      return path
    }
    return line
  })
  return { body: out.join('\n'), resources }
}
```

- [ ] **Step 4: Run tests**

```bash
bunx vitest run src/offline/playlistRewrite.spec.ts
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/offline/playlistRewrite.ts src/offline/playlistRewrite.spec.ts
git commit src/offline/playlistRewrite.ts src/offline/playlistRewrite.spec.ts -m "feat(offline): m3u8 variant selection + local-path rewrite"
```

---

### Task 7: Download engine

**Files:**
- Create: `frontend/web/src/offline/downloadEngine.ts`
- Create: `frontend/web/src/offline/downloadEngine.spec.ts`

**Interfaces:**
- Consumes: registry (Task 5), playlistRewrite (Task 6), `StreamResult` from `@/types/aePlayer`.
- Produces (consumed by Tasks 10/12):
  - `enqueueDownload(req: DownloadRequest): Promise<string>` — returns download id; serial episode queue.
  - `pauseDownload(id: string): void`, `resumeDownload(req: DownloadRequest): Promise<string>` (resume = re-enqueue; cached resources are skipped), `removeDownload(id: string): Promise<void>` (cancels + deletes cache and record).
  - `engineState: { activeId: Ref<string|null>, progress: Ref<Record<string, {done: number; total: number}>> }` (module-level reactive; Pinia wrapping happens in Task 12).
  - `DownloadRequest = { animeId; animeTitle; poster?: string; episode: EpisodeOption; combo: Combo; quality: string; resolve: () => Promise<StreamResult> }` — `resolve` is caller-supplied so the engine re-resolves expired signatures without knowing provider internals.

- [ ] **Step 1: Write failing tests** — `src/offline/downloadEngine.spec.ts`

```ts
import 'fake-indexeddb/auto'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { enqueueDownload, removeDownload, _resetEngineForTests, _installCachesForTests } from './downloadEngine'
import { _resetDbForTests, getDownload, putDownload } from './registry'
import type { StreamResult } from '@/types/aePlayer'

// ── in-memory CacheStorage fake ──────────────────────────────────────────────
class FakeCache {
  store = new Map<string, Response>()
  async match(req: string | Request) {
    const key = typeof req === 'string' ? req : new URL(req.url).pathname
    return this.store.get(key)?.clone()
  }
  async put(req: string | Request, resp: Response) {
    const key = typeof req === 'string' ? req : new URL(req.url).pathname
    this.store.set(key, resp)
  }
}
function fakeCaches() {
  const caches = new Map<string, FakeCache>()
  return {
    caches,
    impl: {
      async open(name: string) {
        if (!caches.has(name)) caches.set(name, new FakeCache())
        return caches.get(name)! as unknown as Cache
      },
      async delete(name: string) { return caches.delete(name) },
      async has(name: string) { return caches.has(name) },
      async keys() { return [...caches.keys()] },
      async match() { return undefined },
    } as unknown as CacheStorage,
  }
}

const MASTER = '#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,RESOLUTION=1280x720\nv/index.m3u8\n'
const MEDIA = '#EXTM3U\n#EXTINF:4,\ns0.ts\n#EXTINF:4,\ns1.ts\n#EXT-X-ENDLIST\n'

function mockFetch(routes: Record<string, () => Response>) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input)
    for (const [suffix, make] of Object.entries(routes)) {
      if (url.endsWith(suffix)) return make()
    }
    return new Response('nf', { status: 404 })
  })
}

const req = (resolve: () => Promise<StreamResult>) => ({
  animeId: 'a1', animeTitle: 'T', quality: '720',
  episode: { key: 1, label: 1, number: 1 },
  combo: { audio: 'sub' as const, lang: 'en' as const, provider: 'gogoanime', server: 's', team: null },
  resolve,
})

beforeEach(async () => {
  await _resetDbForTests()
  _resetEngineForTests()
})

describe('downloadEngine — HLS happy path', () => {
  it('resolves, picks variant, caches playlist+segments, marks done', async () => {
    const { caches, impl } = fakeCaches()
    _installCachesForTests(impl)
    const fetcher = mockFetch({
      'master.m3u8': () => new Response(MASTER),
      'v/index.m3u8': () => new Response(MEDIA),
      's0.ts': () => new Response(new Uint8Array(8)),
      's1.ts': () => new Response(new Uint8Array(8)),
    })
    vi.stubGlobal('fetch', fetcher)
    const id = await enqueueDownload(req(async () => ({ url: 'https://p.example/hls/master.m3u8', type: 'hls' })))
    // engine runs the queue inline in tests (no BG); wait for completion state
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'), { timeout: 10_000 })
    const cache = caches.get(`ae-offline-${id}`)!
    expect(await cache.match(`/__offline/${encodeURIComponent(id)}/master.m3u8`)).toBeTruthy()
    expect(await cache.match(`/__offline/${encodeURIComponent(id)}/r/0`)).toBeTruthy()
    expect(await cache.match(`/__offline/${encodeURIComponent(id)}/r/1`)).toBeTruthy()
    // download marker header rides every media fetch
    const segCall = fetcher.mock.calls.find((c) => String(c[0]).endsWith('s0.ts'))!
    expect((segCall[1] as RequestInit).headers).toMatchObject({ 'X-AE-Download': '1' })
  })

  it('MP4: caches single media.mp4 entry', async () => {
    const { caches, impl } = fakeCaches()
    _installCachesForTests(impl)
    vi.stubGlobal('fetch', mockFetch({ 'ep.mp4': () => new Response(new Uint8Array(16)) }))
    const id = await enqueueDownload(req(async () => ({ url: 'https://p.example/ep.mp4', type: 'mp4' })))
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'), { timeout: 10_000 })
    expect(await caches.get(`ae-offline-${id}`)!.match(`/__offline/${encodeURIComponent(id)}/media.mp4`)).toBeTruthy()
  })

  it('resume skips already-cached resources', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    const fetcher = mockFetch({
      'master.m3u8': () => new Response(MASTER),
      'v/index.m3u8': () => new Response(MEDIA),
      's0.ts': () => new Response(new Uint8Array(8)),
      's1.ts': () => new Response(new Uint8Array(8)),
    })
    vi.stubGlobal('fetch', fetcher)
    const id1 = await enqueueDownload(req(async () => ({ url: 'https://p.example/hls/master.m3u8', type: 'hls' })))
    await vi.waitFor(async () => expect((await getDownload(id1))?.state).toBe('done'), { timeout: 10_000 })
    const callsFirst = fetcher.mock.calls.length
    // simulate an interrupted download surviving an app restart: record not
    // done, but segments already sit in Cache Storage
    await putDownload({ ...(await getDownload(id1))!, state: 'paused' })
    _resetEngineForTests() // fresh engine, same caches
    const id2 = await enqueueDownload(req(async () => ({ url: 'https://p.example/hls/master.m3u8', type: 'hls' })))
    await vi.waitFor(async () => expect((await getDownload(id2))?.state).toBe('done'), { timeout: 10_000 })
    // second run refetches playlists (cheap, needed for the resource map) but no segments
    const segRefetches = fetcher.mock.calls.slice(callsFirst).filter((c) => String(c[0]).includes('.ts'))
    expect(segRefetches.length).toBe(0)
  })

  it('re-resolves once on 403 and continues', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    let expired = true
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('master.m3u8')) return new Response(MASTER)
      if (url.endsWith('v/index.m3u8')) return new Response(MEDIA)
      if (url.includes('.ts')) {
        if (expired) { expired = false; return new Response('sig', { status: 403 }) }
        return new Response(new Uint8Array(8))
      }
      return new Response('nf', { status: 404 })
    })
    vi.stubGlobal('fetch', fetcher)
    const resolve = vi.fn(async () => ({ url: 'https://p.example/hls/master.m3u8', type: 'hls' as const }))
    const id = await enqueueDownload(req(resolve))
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'), { timeout: 10_000 })
    expect(resolve.mock.calls.length).toBe(2) // initial + one re-resolve
  })

  it('remove deletes cache and record', async () => {
    const { impl } = fakeCaches()
    _installCachesForTests(impl)
    vi.stubGlobal('fetch', mockFetch({ 'ep.mp4': () => new Response(new Uint8Array(4)) }))
    const id = await enqueueDownload(req(async () => ({ url: 'https://p.example/ep.mp4', type: 'mp4' })))
    await vi.waitFor(async () => expect((await getDownload(id))?.state).toBe('done'), { timeout: 10_000 })
    await removeDownload(id)
    expect(await getDownload(id)).toBeUndefined()
    expect(await impl.has(`ae-offline-${id}`)).toBe(false)
  })
})
```

- [ ] **Step 2: Run to verify failure**

```bash
bunx vitest run src/offline/downloadEngine.spec.ts
```
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `src/offline/downloadEngine.ts`**

```ts
import { ref, type Ref } from 'vue'
import type { Combo, StreamResult, SubtitleTrack } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import { downloadId, offlineCacheName, offlinePath, type DownloadError, type OfflineDownload } from './types'
import { putDownload, getDownload, deleteDownloadRecord } from './registry'
import { isVod, rewriteMediaPlaylist, selectVariant, type PlaylistResource } from './playlistRewrite'

export interface DownloadRequest {
  animeId: string
  animeTitle: string
  poster?: string
  episode: EpisodeOption
  combo: Combo
  quality: string
  /** Fresh stream resolution — called again when signed URLs expire mid-run. */
  resolve: () => Promise<StreamResult>
}

// Pacing: segment fetches are anonymous (no Authorization header), so the
// per-user GCRA never sees them — the binding limits are the per-IP limiter
// and provider-CDN etiquette. 3 rps sustained, 3 in flight, keeps a full
// episode at ~2-3 min while staying gentle on upstream CDNs.
const MIN_FETCH_SPACING_MS = 334
const CONCURRENCY = 3
const MAX_RETRIES = 3

export const engineState: {
  activeId: Ref<string | null>
  progress: Ref<Record<string, { done: number; total: number }>>
} = {
  activeId: ref(null),
  progress: ref({}),
}

let cachesImpl: CacheStorage = typeof caches !== 'undefined' ? caches : (undefined as unknown as CacheStorage)
export function _installCachesForTests(impl: CacheStorage): void {
  cachesImpl = impl
}

const queue: { id: string; req: DownloadRequest }[] = []
const paused = new Set<string>()
let running = false
let wakeLock: { release(): Promise<void> } | null = null

export function _resetEngineForTests(): void {
  queue.length = 0
  paused.clear()
  running = false
  engineState.activeId.value = null
  engineState.progress.value = {}
}

async function acquireWakeLock(): Promise<void> {
  try {
    const wl = (navigator as Navigator & { wakeLock?: { request(t: string): Promise<{ release(): Promise<void> }> } }).wakeLock
    if (wl) wakeLock = await wl.request('screen')
  } catch { /* denied/unsupported — download still runs, screen may sleep */ }
}

async function releaseWakeLock(): Promise<void> {
  try { await wakeLock?.release() } catch { /* already released */ }
  wakeLock = null
}

// Slot reservation happens SYNCHRONOUSLY before the await — with 3 concurrent
// workers, read-sleep-then-stamp would let all 3 burst in the same window
// (~9 rps); reserving first serializes the schedule regardless of concurrency.
let nextFetchSlot = 0
async function pacedFetch(url: string): Promise<Response> {
  const at = Math.max(Date.now(), nextFetchSlot)
  nextFetchSlot = at + MIN_FETCH_SPACING_MS
  const wait = at - Date.now()
  if (wait > 0) await new Promise((r) => setTimeout(r, wait))
  return fetch(url, { headers: { 'X-AE-Download': '1' } })
}

class SignatureExpiredError extends Error {}

async function fetchResource(url: string): Promise<Response> {
  let lastErr: unknown
  for (let attempt = 0; attempt < MAX_RETRIES; attempt++) {
    try {
      const resp = await pacedFetch(url)
      if (resp.status === 401 || resp.status === 403) throw new SignatureExpiredError()
      if (!resp.ok) throw new Error(`http ${resp.status}`)
      return resp
    } catch (e) {
      if (e instanceof SignatureExpiredError) throw e
      lastErr = e
      await new Promise((r) => setTimeout(r, 1000 * (attempt + 1)))
    }
  }
  throw lastErr
}

async function planHls(id: string, stream: StreamResult, targetHeight: number): Promise<{
  playlistLocalPath: string
  playlists: { path: string; body: string }[]
  resources: PlaylistResource[]
}> {
  const masterBody = await (await fetchResource(stream.url)).text()
  const variant = selectVariant(masterBody, targetHeight)
  // stream.url is typically a root-relative proxy path — anchor on the document
  // origin before resolving (new URL throws on a relative base).
  const mediaUrl = variant
    ? new URL(variant.uri, new URL(stream.url, window.location.href)).href
    : stream.url
  const mediaBody = variant ? await (await fetchResource(mediaUrl)).text() : masterBody
  if (!isVod(mediaBody)) throw new Error('not-vod')
  const { body, resources } = rewriteMediaPlaylist(mediaBody, mediaUrl, id)
  return {
    playlistLocalPath: offlinePath(id, 'master.m3u8'),
    playlists: [{ path: offlinePath(id, 'master.m3u8'), body }],
    resources,
  }
}

async function cacheSubtitles(id: string, cache: Cache, subs: SubtitleTrack[]): Promise<SubtitleTrack[]> {
  const out: SubtitleTrack[] = []
  for (let k = 0; k < subs.length; k++) {
    try {
      const resp = await fetchResource(subs[k].url)
      const path = offlinePath(id, `sub/${k}`)
      await cache.put(path, resp)
      out.push({ ...subs[k], url: path })
    } catch { /* a missing sub track is not fatal to the download */ }
  }
  return out
}

async function runDownload(id: string, req: DownloadRequest): Promise<void> {
  const record = (await getDownload(id))!
  const cache = await cachesImpl.open(offlineCacheName(id))
  const setError = async (error: DownloadError) => {
    await putDownload({ ...(await getDownload(id))!, state: 'error', error })
  }

  let stream: StreamResult
  try {
    stream = await req.resolve()
  } catch {
    return setError('resolve')
  }

  let plan
  try {
    plan =
      stream.type === 'mp4'
        ? { playlistLocalPath: offlinePath(id, 'media.mp4'), playlists: [], resources: [{ path: offlinePath(id, 'media.mp4'), url: stream.url }] }
        : await planHls(id, stream, parseInt(req.quality, 10) || 720)
  } catch (e) {
    return setError(e instanceof Error && e.message === 'not-vod' ? 'mismatch' : 'network')
  }

  for (const p of plan.playlists) {
    await cache.put(p.path, new Response(p.body, { headers: { 'Content-Type': 'application/vnd.apple.mpegurl' } }))
  }
  const localSubs = await cacheSubtitles(id, cache, stream.subtitles ?? [])
  let posterOk = false
  if (req.poster) {
    try {
      await cache.put(offlinePath(id, 'poster'), await fetchResource(req.poster))
      posterOk = true
    } catch { /* poster is cosmetic — CORS on external hosts is expected */ }
  }

  const total = plan.resources.length
  let done = 0
  let bytes = record.bytes
  const update = async (state: OfflineDownload['state'], error?: DownloadError) => {
    engineState.progress.value = { ...engineState.progress.value, [id]: { done, total } }
    const cur = await getDownload(id)
    if (!cur) return // removed mid-run — do not resurrect the record
    await putDownload({
      ...cur,
      state, error, bytes, resourcesDone: done, resourcesTotal: total,
      streamType: stream.type, playlistLocalPath: plan.playlistLocalPath,
      subtitles: localSubs, posterPath: posterOk ? offlinePath(id, 'poster') : undefined,
    })
  }
  await update('downloading')

  // Single-flight re-resolve: signed URLs expire hourly; the FIRST worker that
  // hits 401/403 re-resolves and splices fresh URLs into the shared plan (same
  // local paths); concurrent workers await the same promise, then each retries
  // its own item exactly once.
  let reResolving: Promise<void> | null = null
  function ensureFreshUrls(): Promise<void> {
    if (!reResolving) {
      reResolving = (async () => {
        const fresh = await req.resolve()
        const freshPlan = fresh.type === 'mp4'
          ? { resources: [{ path: offlinePath(id, 'media.mp4'), url: fresh.url }] }
          : await planHls(id, fresh, parseInt(req.quality, 10) || 720)
        if (freshPlan.resources.length !== plan.resources.length) throw new Error('mismatch')
        for (let i = 0; i < plan.resources.length; i++) plan.resources[i].url = freshPlan.resources[i].url
      })()
    }
    return reResolving
  }

  async function storeItem(item: PlaylistResource, resp: Response): Promise<void> {
    if (item.path.endsWith('/media.mp4')) {
      // MP4 is one huge body — stream it straight to Cache Storage; buffering
      // hundreds of MB through arrayBuffer() OOMs mobile tabs.
      const len = parseInt(resp.headers.get('Content-Length') ?? '0', 10)
      bytes += Number.isFinite(len) ? len : 0
      await cache.put(item.path, resp)
      return
    }
    const buf = await resp.arrayBuffer()
    bytes += buf.byteLength
    await cache.put(item.path, new Response(buf, { headers: { 'Content-Type': resp.headers.get('Content-Type') ?? 'application/octet-stream' } }))
  }

  async function fetchItem(item: PlaylistResource): Promise<void> {
    try {
      await storeItem(item, await fetchResource(item.url))
    } catch (e) {
      if (!(e instanceof SignatureExpiredError)) throw e
      await ensureFreshUrls()
      // one retry with the fresh URL; a second 401/403 is a real failure
      await storeItem(item, await fetchResource(item.url))
    }
  }

  let cursor = 0
  const worker = async (): Promise<void> => {
    while (cursor < plan.resources.length) {
      if (paused.has(id)) return
      const item = plan.resources[cursor++]
      if (!(await cache.match(item.path))) await fetchItem(item)
      done++
      engineState.progress.value = { ...engineState.progress.value, [id]: { done, total } }
    }
  }

  try {
    await Promise.all(Array.from({ length: CONCURRENCY }, () => worker()))
    if (paused.has(id)) return void (await update('paused'))
    await update('done')
  } catch (e) {
    const quota = e instanceof DOMException && e.name === 'QuotaExceededError'
    await update('error', quota ? 'quota' : e instanceof Error && e.message === 'mismatch' ? 'mismatch' : 'network')
  }
}

async function pump(): Promise<void> {
  if (running) return
  running = true
  await acquireWakeLock()
  try {
    while (queue.length > 0) {
      const { id, req } = queue.shift()!
      engineState.activeId.value = id
      await runDownload(id, req)
      engineState.activeId.value = null
    }
  } finally {
    running = false
    await releaseWakeLock()
  }
}

// Conservative per-quality size projections for the pre-download quota check
// (also shown as the dialog's size hint). Real size lands in `bytes` as it
// downloads; QuotaExceededError mid-flight is still handled as error:'quota'.
export const PROJECTED_BYTES: Record<string, number> = {
  '480': 250 * 2 ** 20,
  '720': 450 * 2 ** 20,
  '1080': 900 * 2 ** 20,
}

async function quotaHeadroom(): Promise<number | null> {
  try {
    const est = await navigator.storage?.estimate?.()
    if (!est?.quota) return null
    return est.quota - (est.usage ?? 0)
  } catch {
    return null // estimate unsupported — proceed, mid-flight quota check remains
  }
}

export async function enqueueDownload(req: DownloadRequest): Promise<string> {
  const id = downloadId(req.animeId, req.episode.number, req.combo, req.quality)
  try {
    await (navigator as Navigator & { storage?: { persist?: () => Promise<boolean> } }).storage?.persist?.()
  } catch { /* best-effort */ }
  const existing = await getDownload(id)
  if (existing?.state === 'done') return id
  paused.delete(id)
  const baseRecord = {
    id, animeId: req.animeId, animeTitle: req.animeTitle, episode: req.episode,
    combo: req.combo, quality: req.quality, streamType: 'hls' as const,
    bytes: existing?.bytes ?? 0, resourcesDone: 0, resourcesTotal: 0,
    createdAt: existing?.createdAt ?? Date.now(),
    playlistLocalPath: offlinePath(id, 'master.m3u8'), subtitles: [],
  }
  const headroom = await quotaHeadroom()
  if (headroom !== null && headroom < (PROJECTED_BYTES[req.quality] ?? PROJECTED_BYTES['720'])) {
    await putDownload({ ...baseRecord, state: 'error', error: 'quota' })
    return id
  }
  await putDownload({ ...baseRecord, state: 'queued' })
  queue.push({ id, req })
  void pump()
  return id
}

export function pauseDownload(id: string): void {
  paused.add(id)
}

export async function resumeDownload(req: DownloadRequest): Promise<string> {
  return enqueueDownload(req) // resume = re-enqueue; cached resources are skipped
}

export async function removeDownload(id: string): Promise<void> {
  paused.add(id) // stop an in-flight run at the next item boundary
  await cachesImpl.delete(offlineCacheName(id))
  await deleteDownloadRecord(id)
  const { [id]: _, ...rest } = engineState.progress.value
  engineState.progress.value = rest
}

/** Startup scan: registry entries whose cache Chrome evicted → error:'evicted'. */
export async function markEvicted(list: OfflineDownload[]): Promise<OfflineDownload[]> {
  const out: OfflineDownload[] = []
  for (const d of list) {
    if (d.state === 'done' && !(await cachesImpl.has(offlineCacheName(d.id)))) {
      const marked: OfflineDownload = { ...d, state: 'error', error: 'evicted' }
      await putDownload(marked)
      out.push(marked)
    } else {
      out.push(d)
    }
  }
  return out
}
```

- [ ] **Step 4: Run tests**

```bash
bunx vitest run src/offline/downloadEngine.spec.ts
```
Expected: PASS (5 tests). Real timers + 334ms pacing make each test take 1–3s — the `{ timeout: 10_000 }` on every `vi.waitFor` covers that. Do NOT stub `setTimeout` globally (it would also collapse the retry backoff and starve waitFor's polling).

- [ ] **Step 5: Lint + typecheck**

```bash
bunx eslint src/offline/ --ext .ts && bunx tsc --noEmit
```
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add src/offline/downloadEngine.ts src/offline/downloadEngine.spec.ts
git commit src/offline/downloadEngine.ts src/offline/downloadEngine.spec.ts -m "feat(offline): paced download engine with resume, re-resolve, wake lock"
```

---

### Task 7b: OfflineMediaStore port (future standalone apps)

> Added 2026-07-03 per owner requirement: separate full-fledged apps (Capacitor/Tauri-class) will reuse this codebase later — the engine must not hard-bind to Cache Storage. This task refactors the just-written engine onto a storage port. **Behavior must stay byte-identical**; the existing 5 engine tests keep passing UNCHANGED (they install a fake CacheStorage, which now flows through the real web adapter — even better coverage).

**Files:**
- Create: `frontend/web/src/offline/mediaStore.ts`
- Create: `frontend/web/src/offline/mediaStore.spec.ts`
- Modify: `frontend/web/src/offline/downloadEngine.ts`

**Interfaces:**
- Consumes: `offlineCacheName`, `offlinePath` (Task 5).
- Produces: `OfflineMediaStore` interface + `cacheStorageMediaStore(cachesImpl?)` web adapter; engine keeps its public API (`enqueueDownload`, `pauseDownload`, `resumeDownload`, `removeDownload`, `markEvicted`, `engineState`, `_installCachesForTests`, `_resetEngineForTests`) and ADDS `storageEstimate(): Promise<{usage,quota}|null>` (Task 12 uses it instead of touching `navigator.storage` directly).

- [ ] **Step 1: Create `src/offline/mediaStore.ts`**

```ts
// Storage port for downloaded media bytes. The download engine performs ALL
// byte I/O through this interface so future standalone apps (Capacitor/Tauri)
// can swap in a filesystem adapter without touching the engine, registry, or
// UI. The web adapter below is Cache Storage + SW-served /__offline/* URLs.
import { offlineCacheName, offlinePath } from './types'

export interface OfflineMediaStore {
  put(id: string, path: string, resp: Response): Promise<void>
  has(id: string, path: string): Promise<boolean>
  /** Drop the whole container for a download id. */
  remove(id: string): Promise<boolean>
  /** Container still present? (eviction scan) */
  exists(id: string): Promise<boolean>
  persist(): Promise<void>
  estimate(): Promise<{ usage: number; quota: number } | null>
  /** Playable local URL for an entry. Web: /__offline/{id}/{rest} (SW-served).
   *  A native adapter returns its own scheme (file/asset URL) here. */
  entryUrl(id: string, rest: string): string
}

export function cacheStorageMediaStore(cachesImpl: CacheStorage = caches): OfflineMediaStore {
  return {
    async put(id, path, resp) {
      const cache = await cachesImpl.open(offlineCacheName(id))
      await cache.put(path, resp)
    },
    async has(id, path) {
      const cache = await cachesImpl.open(offlineCacheName(id))
      return !!(await cache.match(path))
    },
    async remove(id) {
      return cachesImpl.delete(offlineCacheName(id))
    },
    async exists(id) {
      return cachesImpl.has(offlineCacheName(id))
    },
    async persist() {
      try {
        await (navigator as Navigator & { storage?: { persist?: () => Promise<boolean> } }).storage?.persist?.()
      } catch { /* best-effort */ }
    },
    async estimate() {
      try {
        const est = await navigator.storage?.estimate?.()
        if (!est?.quota) return null
        return { usage: est.usage ?? 0, quota: est.quota }
      } catch {
        return null
      }
    },
    entryUrl(id, rest) {
      return offlinePath(id, rest)
    },
  }
}
```

- [ ] **Step 2: Write `src/offline/mediaStore.spec.ts`** (direct adapter coverage — the engine specs cover it indirectly)

```ts
import { describe, it, expect } from 'vitest'
import { cacheStorageMediaStore } from './mediaStore'

function fakeCaches() {
  const stores = new Map<string, Map<string, Response>>()
  return {
    stores,
    impl: {
      async open(name: string) {
        if (!stores.has(name)) stores.set(name, new Map())
        const m = stores.get(name)!
        return {
          async put(k: string, r: Response) { m.set(k, r) },
          async match(k: string) { return m.get(k)?.clone() },
        } as unknown as Cache
      },
      async delete(name: string) { return stores.delete(name) },
      async has(name: string) { return stores.has(name) },
      async keys() { return [...stores.keys()] },
      async match() { return undefined },
    } as unknown as CacheStorage,
  }
}

describe('cacheStorageMediaStore', () => {
  it('put/has round-trips inside the per-id container', async () => {
    const { stores, impl } = fakeCaches()
    const s = cacheStorageMediaStore(impl)
    await s.put('d1', '/__offline/d1/r/0', new Response('x'))
    expect(await s.has('d1', '/__offline/d1/r/0')).toBe(true)
    expect(await s.has('d1', '/__offline/d1/r/1')).toBe(false)
    expect(stores.has('ae-offline-d1')).toBe(true)
  })
  it('remove/exists manage the container lifecycle', async () => {
    const { impl } = fakeCaches()
    const s = cacheStorageMediaStore(impl)
    await s.put('d1', '/p', new Response('x'))
    expect(await s.exists('d1')).toBe(true)
    expect(await s.remove('d1')).toBe(true)
    expect(await s.exists('d1')).toBe(false)
  })
  it('entryUrl emits the SW-served offline scheme', () => {
    const s = cacheStorageMediaStore(fakeCaches().impl)
    expect(s.entryUrl('a:1', 'master.m3u8')).toBe(`/__offline/${encodeURIComponent('a:1')}/master.m3u8`)
  })
})
```

Run: `bunx vitest run src/offline/mediaStore.spec.ts` — RED (module not found) before Step 1 if following strict TDD order (write spec first), GREEN after.

- [ ] **Step 3: Refactor `downloadEngine.ts` onto the port** (behavior-identical)

Mechanical substitutions — no logic changes:
1. Replace the `cachesImpl` module state and its uses:
```ts
import { cacheStorageMediaStore, type OfflineMediaStore } from './mediaStore'

let store: OfflineMediaStore = cacheStorageMediaStore()
/** Kept name/signature so the existing engine spec is untouched: installing a
 *  fake CacheStorage routes it through the real web adapter. */
export function _installCachesForTests(impl: CacheStorage): void {
  store = cacheStorageMediaStore(impl)
}
```
2. `enqueueDownload`: `navigator.storage.persist` try/catch → `await store.persist()`; `quotaHeadroom()` body → `return store.estimate().then((est) => est ? est.quota - est.usage : null)` (keep the function; simplify).
3. Add the Task 12 helper:
```ts
export function storageEstimate(): Promise<{ usage: number; quota: number } | null> {
  return store.estimate()
}
```
4. `runDownload`: delete `const cache = await cachesImpl.open(offlineCacheName(id))`; every `cache.put(path, resp)` → `store.put(id, path, resp)`; the worker resume check `if (!(await cache.match(item.path)))` → `if (!(await store.has(id, item.path)))`; `cacheSubtitles(id, cache, subs)` → `cacheSubtitles(id, subs)` (signature drops the Cache param, uses `store.put` inside).
5. `removeDownload`: `await cachesImpl.delete(offlineCacheName(id))` → `await store.remove(id)`.
6. `markEvicted`: `!(await cachesImpl.has(offlineCacheName(d.id)))` → `!(await store.exists(d.id))`.
7. Remove the now-unused `offlineCacheName` import if nothing else uses it.

- [ ] **Step 4: Full offline suite + gates**

```bash
bunx vitest run src/offline/ && bunx eslint src/offline/ --ext .ts && bunx tsc --noEmit
```
Expected: all previous engine tests pass UNCHANGED (any engine-spec edit means the refactor broke behavior — fix the engine, not the spec) + 3 new adapter tests; lint/typecheck clean.

- [ ] **Step 5: Commit**

```bash
git add src/offline/mediaStore.ts src/offline/mediaStore.spec.ts src/offline/downloadEngine.ts
git commit src/offline/mediaStore.ts src/offline/mediaStore.spec.ts src/offline/downloadEngine.ts -m "refactor(offline): media-byte I/O behind OfflineMediaStore port (future native apps)"
```

---

### Task 8: SW offline serving — cache-first + MP4 Range

**Files:**
- Create: `frontend/web/src/pwa/offlineServe.ts`
- Create: `frontend/web/src/pwa/offlineServe.spec.ts`
- Modify: `frontend/web/src/sw.ts` (replace the Task 2 404 stub)

**Interfaces:**
- Consumes: `offlineCacheName` naming convention (Task 5) — duplicated here as a constant prefix to keep the SW bundle free of IDB imports.
- Produces: `handleOfflineRequest(request: Request, cachesImpl?: CacheStorage): Promise<Response>`, `parseRange(header: string, size: number): { start: number; end: number } | null`, `buildRangeResponse(full: Response, rangeHeader: string): Promise<Response>`.

- [ ] **Step 1: Write failing tests** — `src/pwa/offlineServe.spec.ts`

```ts
import { describe, it, expect } from 'vitest'
import { parseRange, buildRangeResponse, handleOfflineRequest } from './offlineServe'

function fakeCachesWith(entries: Record<string, Response>): CacheStorage {
  const cache = {
    async match(req: string | Request) {
      const path = typeof req === 'string' ? req : new URL(req.url).pathname
      return entries[path]?.clone()
    },
  }
  return { async open() { return cache as unknown as Cache } } as unknown as CacheStorage
}

describe('parseRange', () => {
  it('parses closed, open-ended, and rejects invalid', () => {
    expect(parseRange('bytes=0-99', 1000)).toEqual({ start: 0, end: 99 })
    expect(parseRange('bytes=500-', 1000)).toEqual({ start: 500, end: 999 })
    expect(parseRange('bytes=990-2000', 1000)).toEqual({ start: 990, end: 999 })
    expect(parseRange('bytes=1000-', 1000)).toBeNull() // start beyond size
    expect(parseRange('items=0-1', 1000)).toBeNull()
  })
})

describe('buildRangeResponse', () => {
  it('slices a cached body into a 206 with correct headers', async () => {
    const body = new Uint8Array(100).map((_, i) => i)
    const full = new Response(body, { headers: { 'Content-Type': 'video/mp4' } })
    const r = await buildRangeResponse(full, 'bytes=10-19')
    expect(r.status).toBe(206)
    expect(r.headers.get('Content-Range')).toBe('bytes 10-19/100')
    expect(r.headers.get('Content-Length')).toBe('10')
    expect(r.headers.get('Accept-Ranges')).toBe('bytes')
    expect(new Uint8Array(await r.arrayBuffer())[0]).toBe(10)
  })
  it('416 on unsatisfiable range', async () => {
    const full = new Response(new Uint8Array(10))
    expect((await buildRangeResponse(full, 'bytes=50-')).status).toBe(416)
  })
})

describe('handleOfflineRequest', () => {
  const entries = {
    '/__offline/d1/master.m3u8': new Response('#EXTM3U', { headers: { 'Content-Type': 'application/vnd.apple.mpegurl' } }),
    '/__offline/d1/media.mp4': new Response(new Uint8Array(100), { headers: { 'Content-Type': 'video/mp4' } }),
  }
  it('serves cached entries as-is without Range', async () => {
    const r = await handleOfflineRequest(new Request('https://x/__offline/d1/master.m3u8'), fakeCachesWith(entries))
    expect(r.status).toBe(200)
    expect(await r.text()).toBe('#EXTM3U')
  })
  it('serves 206 slices for ranged mp4 requests', async () => {
    const req = new Request('https://x/__offline/d1/media.mp4', { headers: { Range: 'bytes=0-9' } })
    const r = await handleOfflineRequest(req, fakeCachesWith(entries))
    expect(r.status).toBe(206)
    expect(r.headers.get('Content-Range')).toBe('bytes 0-9/100')
  })
  it('404 on cache miss (evicted or bogus id)', async () => {
    const r = await handleOfflineRequest(new Request('https://x/__offline/nope/master.m3u8'), fakeCachesWith(entries))
    expect(r.status).toBe(404)
  })
})
```

- [ ] **Step 2: Run to verify failure**

```bash
bunx vitest run src/pwa/offlineServe.spec.ts
```
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `src/pwa/offlineServe.ts`**

```ts
// Serves /__offline/{id}/… from the per-download cache. Range support matters
// for MP4 sources: <video> seeks issue byte-range requests, and a 200-only
// server would force full re-buffering on every seek. Blob.slice is lazy
// (disk-backed), so slicing a 500MB body does not load it into RAM.

const CACHE_PREFIX = 'ae-offline-'

export function parseRange(header: string, size: number): { start: number; end: number } | null {
  const m = /^bytes=(\d+)-(\d*)$/.exec(header.trim())
  if (!m) return null
  const start = parseInt(m[1], 10)
  if (start >= size) return null
  const end = m[2] === '' ? size - 1 : Math.min(parseInt(m[2], 10), size - 1)
  if (end < start) return null
  return { start, end }
}

export async function buildRangeResponse(full: Response, rangeHeader: string): Promise<Response> {
  const blob = await full.blob()
  const range = parseRange(rangeHeader, blob.size)
  if (!range) {
    return new Response(null, { status: 416, headers: { 'Content-Range': `bytes */${blob.size}` } })
  }
  const slice = blob.slice(range.start, range.end + 1)
  return new Response(slice, {
    status: 206,
    headers: {
      'Content-Type': full.headers.get('Content-Type') ?? 'application/octet-stream',
      'Content-Range': `bytes ${range.start}-${range.end}/${blob.size}`,
      'Content-Length': String(slice.size),
      'Accept-Ranges': 'bytes',
    },
  })
}

/** /__offline/{id}/{rest} → entry from cache `ae-offline-{id}`. */
export async function handleOfflineRequest(
  request: Request,
  cachesImpl: CacheStorage = caches,
): Promise<Response> {
  const pathname = new URL(request.url).pathname
  const m = /^\/__offline\/([^/]+)\//.exec(pathname)
  if (!m) return new Response('bad offline path', { status: 400 })
  const cache = await cachesImpl.open(CACHE_PREFIX + decodeURIComponent(m[1]))
  const hit = await cache.match(pathname)
  if (!hit) return new Response('not downloaded', { status: 404 })
  const range = request.headers.get('Range')
  if (range) return buildRangeResponse(hit, range)
  return hit
}
```

- [ ] **Step 4: Run tests**

```bash
bunx vitest run src/pwa/offlineServe.spec.ts
```
Expected: PASS.

- [ ] **Step 5: Replace the stub in `src/sw.ts`**

Add import and swap the `/__offline/` route registered in Task 2:

```ts
import { handleOfflineRequest } from './pwa/offlineServe'
```

```ts
registerRoute(
  ({ url }) => isOfflinePath(url.pathname),
  ({ request }) => handleOfflineRequest(request),
)
```

- [ ] **Step 6: Build + suite**

```bash
bun run build && bunx vitest run src/pwa/
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/pwa/offlineServe.ts src/pwa/offlineServe.spec.ts src/sw.ts
git commit src/pwa/offlineServe.ts src/pwa/offlineServe.spec.ts src/sw.ts -m "feat(offline): SW serves downloaded episodes with MP4 Range support"
```

---

### Task 9: Offline watch-progress queue

**Files:**
- Create: `frontend/web/src/offline/progressQueue.ts`
- Create: `frontend/web/src/offline/progressQueue.spec.ts`
- Modify: `frontend/web/src/composables/aePlayer/useWatchTracking.ts` (saveServer catch)
- Modify: `frontend/web/src/main.ts` (install flush)

**Interfaces:**
- Consumes: `enqueuePending`/`drainPending` (Task 5); `userApi.updateProgress(data)` from `@/api/client`.
- Produces: `queueProgress(payload: Record<string, unknown>): void` (fire-and-forget), `flushPendingProgress(post?): Promise<boolean>`, `installProgressFlush(): void`.

- [ ] **Step 1: Write failing tests** — `src/offline/progressQueue.spec.ts`

```ts
import 'fake-indexeddb/auto'
import { describe, it, expect, beforeEach } from 'vitest'
import { flushPendingProgress } from './progressQueue'
import { _resetDbForTests, enqueuePending } from './registry'

beforeEach(() => _resetDbForTests())

// queueProgress is a void fire-and-forget wrapper over enqueuePending (already
// covered in registry.spec.ts) — flush logic is what needs testing here, so
// seed the queue with awaited enqueuePending calls.
describe('progressQueue', () => {
  it('flushes queued payloads FIFO through the poster fn', async () => {
    await enqueuePending({ anime_id: 'a', episode_number: 1, progress: 100 })
    await enqueuePending({ anime_id: 'a', episode_number: 2, progress: 50 })
    const posted: unknown[] = []
    const ok = await flushPendingProgress(async (p) => { posted.push(p) })
    expect(ok).toBe(true)
    expect(posted).toHaveLength(2)
    expect((posted[0] as { episode_number: number }).episode_number).toBe(1)
  })
  it('keeps entries when posting fails', async () => {
    await enqueuePending({ anime_id: 'a', episode_number: 1, progress: 100 })
    expect(await flushPendingProgress(async () => { throw new Error('offline') })).toBe(false)
    const posted: unknown[] = []
    await flushPendingProgress(async (p) => { posted.push(p) })
    expect(posted).toHaveLength(1)
  })
})
```

- [ ] **Step 2: Run to verify failure**

```bash
bunx vitest run src/offline/progressQueue.spec.ts
```
Expected: FAIL.

- [ ] **Step 3: Implement `src/offline/progressQueue.ts`**

```ts
import { userApi } from '@/api/client'
import { enqueuePending, drainPending } from './registry'

/** Fire-and-forget: buffer a failed watch-progress write for later sync.
 *  Duplicates are harmless — the endpoint upserts per (anime, episode). */
export function queueProgress(payload: Record<string, unknown>): void {
  void enqueuePending(payload).catch(() => {})
}

export async function flushPendingProgress(
  post: (payload: Record<string, unknown>) => Promise<unknown> = (p) => userApi.updateProgress(p),
): Promise<boolean> {
  return drainPending(async (payload) => {
    try {
      await post(payload as Record<string, unknown>)
      return true
    } catch {
      return false
    }
  })
}

let installed = false
export function installProgressFlush(): void {
  if (installed) return
  installed = true
  window.addEventListener('online', () => void flushPendingProgress())
  void flushPendingProgress() // app start — drain anything left from last session
}
```

- [ ] **Step 4: Run tests**

```bash
bunx vitest run src/offline/progressQueue.spec.ts
```
Expected: PASS.

- [ ] **Step 5: Patch `useWatchTracking.ts` saveServer**

Anchor: the `saveServer(time)` function (~line 87) currently ends `.catch(() => { /* heartbeat save is best-effort */ })`. Restructure to capture the payload and queue on failure:

```ts
  function saveServer(time: number) {
    const ep = episodeNumber()
    if (!ep || time <= 0 || !auth.isAuthenticated) return
    const payload = {
      anime_id: animeId(),
      episode_number: ep,
      progress: Math.floor(time),
      duration: Math.floor(maxTime.value) || null,
      session_id: sessionId.value,
      ...comboFields(),
    }
    void userApi.updateProgress(payload).catch(() => {
      // offline / transient failure — buffer for the online flush so
      // continue-watching doesn't diverge after offline viewing
      void import('@/offline/progressQueue').then((m) => m.queueProgress(payload))
    })
  }
```

(Dynamic import keeps the offline module out of the player's hot path; it's already loaded when the user came from /downloads.)

- [ ] **Step 6: Install flush in `src/main.ts`** — same `deferInit` block as Task 3:

```ts
  void import('./offline/progressQueue').then((m) => m.installProgressFlush())
```

- [ ] **Step 7: Full check**

```bash
bunx vitest run src/offline/ src/composables/aePlayer/ && bun run build
```
Expected: PASS (existing useWatchTracking specs must stay green).

- [ ] **Step 8: Commit**

```bash
git add src/offline/progressQueue.ts src/offline/progressQueue.spec.ts src/composables/aePlayer/useWatchTracking.ts src/main.ts
git commit src/offline/progressQueue.ts src/offline/progressQueue.spec.ts src/composables/aePlayer/useWatchTracking.ts src/main.ts -m "feat(offline): buffer watch-progress offline, flush when back online"
```

---

### Task 10: Download UI in the player (EpisodesPanel button + quality dialog + AePlayer wiring)

**Files:**
- Create: `frontend/web/src/offline/flag.ts`
- Create: `frontend/web/src/components/player/aePlayer/DownloadDialog.vue`
- Modify: `frontend/web/src/vite-env.d.ts` (declare the new VITE_* key)
- Modify: `frontend/web/src/components/player/aePlayer/EpisodesPanel.vue`
- Modify: `frontend/web/src/components/player/aePlayer/EpisodesPanel.spec.ts`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (anchored)
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`

**Interfaces:**
- Consumes: `enqueueDownload`, `engineState` (Task 7); `resolver.resolveStream(provider, animeId, ep, combo)` (existing); `DownloadState` (Task 5).
- Produces: EpisodesPanel new optional props `downloadable?: boolean`, `downloadStates?: Record<number, DownloadState>` + emit `(e: 'download', ep: EpisodeOption)`. DownloadDialog emits `(e: 'confirm', quality: string)` / `(e: 'close')`. New i18n namespace `player.aePlayer.offline.*` + `downloads.*` + `nav.downloads`.

- [ ] **Step 1: Create `src/offline/flag.ts`** + declare the env key

```ts
/** Offline downloads UI gate. Default ON; set VITE_OFFLINE_DOWNLOADS_ENABLED=false
 *  at build time to yank all download surfaces without touching the SW. */
export const offlineDownloadsEnabled: boolean =
  import.meta.env.VITE_OFFLINE_DOWNLOADS_ENABLED !== 'false'

/** Runtime capability: downloads need a controlling SW to play back later. */
export function offlineRuntimeReady(): boolean {
  return offlineDownloadsEnabled && 'serviceWorker' in navigator && !!navigator.serviceWorker.controller
}
```

Also add to the `ImportMetaEnv` interface in `src/vite-env.d.ts` (project convention — mimic the existing `VITE_HLS_PROXY_BASE` line):

```ts
  readonly VITE_OFFLINE_DOWNLOADS_ENABLED?: string
```

- [ ] **Step 2: Add i18n keys to ALL THREE locales** (`en.json` / `ru.json` / `ja.json`)

`en.json` — inside `player.aePlayer` add:
```json
"offline": {
  "download": "Download",
  "downloading": "Downloading…",
  "downloaded": "Downloaded",
  "failed": "Download failed",
  "quality": "Download quality",
  "estimate": "~{size} per episode",
  "start": "Download",
  "cancel": "Cancel"
}
```
Top-level `downloads` namespace + one nav key:
```json
"downloads": {
  "title": "Downloads",
  "empty": "Nothing downloaded yet. Find an episode and hit Download in the player.",
  "storage": "Storage: {used} of {total}",
  "episode": "Episode {n}",
  "watch": "Watch",
  "delete": "Delete",
  "confirmDelete": "Delete this download?",
  "pause": "Pause",
  "resume": "Resume",
  "state": {
    "queued": "Queued",
    "downloading": "Downloading {done}/{total}",
    "paused": "Paused",
    "done": "Ready",
    "error": "Failed"
  },
  "error": {
    "network": "Network error — resume to retry",
    "quota": "Not enough storage space",
    "evicted": "The browser evicted this download — re-download",
    "resolve": "Source unavailable",
    "mismatch": "Source changed — re-download"
  },
  "offlineReady": "Available offline"
}
```
`nav`: `"downloads": "Downloads"`.

`ru.json` — same shapes: `offline`: `"download": "Скачать"`, `"downloading": "Скачивается…"`, `"downloaded": "Скачано"`, `"failed": "Ошибка скачивания"`, `"quality": "Качество загрузки"`, `"estimate": "~{size} на серию"`, `"start": "Скачать"`, `"cancel": "Отмена"`. `downloads`: `"title": "Загрузки"`, `"empty": "Пока ничего не скачано. Откройте серию и нажмите «Скачать» в плеере."`, `"storage": "Хранилище: {used} из {total}"`, `"episode": "Серия {n}"`, `"watch": "Смотреть"`, `"delete": "Удалить"`, `"confirmDelete": "Удалить загрузку?"`, `"pause": "Пауза"`, `"resume": "Продолжить"`, `state`: `"queued": "В очереди"`, `"downloading": "Скачивается {done}/{total}"`, `"paused": "Пауза"`, `"done": "Готово"`, `"error": "Ошибка"`, `error`: `"network": "Ошибка сети — нажмите «Продолжить»"`, `"quota": "Недостаточно места"`, `"evicted": "Браузер удалил загрузку — скачайте заново"`, `"resolve": "Источник недоступен"`, `"mismatch": "Источник изменился — скачайте заново"`, `"offlineReady": "Доступно оффлайн"`. `nav.downloads`: `"Загрузки"`.

`ja.json` — same shapes: `offline`: `"download": "ダウンロード"`, `"downloading": "ダウンロード中…"`, `"downloaded": "ダウンロード済み"`, `"failed": "ダウンロード失敗"`, `"quality": "ダウンロード画質"`, `"estimate": "1話あたり約{size}"`, `"start": "ダウンロード"`, `"cancel": "キャンセル"`. `downloads`: `"title": "ダウンロード"`, `"empty": "まだ何もダウンロードされていません。プレイヤーで「ダウンロード」を押してください。"`, `"storage": "ストレージ: {total} 中 {used}"`, `"episode": "第{n}話"`, `"watch": "視聴"`, `"delete": "削除"`, `"confirmDelete": "このダウンロードを削除しますか？"`, `"pause": "一時停止"`, `"resume": "再開"`, `state`: `"queued": "待機中"`, `"downloading": "ダウンロード中 {done}/{total}"`, `"paused": "一時停止"`, `"done": "完了"`, `"error": "失敗"`, `error`: `"network": "ネットワークエラー — 再開で再試行"`, `"quota": "ストレージ容量が不足しています"`, `"evicted": "ブラウザがデータを削除しました — 再ダウンロードしてください"`, `"resolve": "ソースが利用できません"`, `"mismatch": "ソースが変更されました — 再ダウンロードしてください"`, `"offlineReady": "オフラインで視聴可能"`. `nav.downloads`: `"ダウンロード"`.

Verify parity:
```bash
bunx vitest run src/locales/__tests__/locale-parity.spec.ts
```
Expected: PASS.

- [ ] **Step 3: Write failing EpisodesPanel test** — append to `EpisodesPanel.spec.ts` (follow the file's existing mount helper style):

```ts
// NOTE: the spec file has NO shared mount helper — it mounts inline with a
// real createI18n(en.json) via config.global.plugins (top of file), so $t
// resolves the real keys added in Step 2. Define this local helper:
//   const mountPanel = (extra: Record<string, unknown>) =>
//     mount(EpisodesPanel, { props: {
//       episodes: [{ key: 1, label: 1, number: 1 }, { key: 2, label: 2, number: 2 }],
//       selectedNumber: 1, ...extra,
//     } })
// Selectors use data-test (the file's existing convention), not data-testid.
describe('download affordance', () => {
  it('emits download for an episode when downloadable', async () => {
    const w = mountPanel({ downloadable: true, downloadStates: {} })
    const btn = w.find('[data-test="ep-download-1"]')
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    expect(w.emitted('download')![0][0]).toMatchObject({ number: 1 })
    expect(w.emitted('select')).toBeUndefined() // .stop — no episode switch
  })
  it('renders no download affordance when not downloadable', () => {
    const w = mountPanel({})
    expect(w.find('[data-test="ep-download-1"]').exists()).toBe(false)
  })
  it('shows done state instead of a button for downloaded episodes', () => {
    const w = mountPanel({ downloadable: true, downloadStates: { 1: 'done' } })
    expect(w.find('[data-test="ep-downloaded-1"]').exists()).toBe(true)
  })
})
```

Run: `bunx vitest run src/components/player/aePlayer/EpisodesPanel.spec.ts` — expected: new tests FAIL.

- [ ] **Step 4: Extend `EpisodesPanel.vue`**

Props (add to the existing `defineProps` + `withDefaults`):
```ts
    downloadable?: boolean
    downloadStates?: Record<number, DownloadState>
```
with defaults `downloadable: false, downloadStates: () => ({})`, importing `import type { DownloadState } from '@/offline/types'`.

Emits: add `(e: 'download', ep: EpisodeOption): void`.

Template — inside the **strip view** `.ep-card` block (the `v-for` around line 67), after the `.ep-card-t` title span (`.ep-card` is a `<button>`, so the affordance is a `<span role="button">` — nested `<button>` is invalid HTML; player dir is exempt from the DS native-control rule):

```html
<span
  v-if="downloadable && (downloadStates[ep.number] === 'done')"
  :data-test="`ep-downloaded-${ep.number}`"
  class="ep-dl text-success"
  :title="$t('player.aePlayer.offline.downloaded')"
><Check :size="14" /></span>
<span
  v-else-if="downloadable && (downloadStates[ep.number] === 'downloading' || downloadStates[ep.number] === 'queued')"
  class="ep-dl text-muted-foreground animate-spin"
  :title="$t('player.aePlayer.offline.downloading')"
><Loader2 :size="14" /></span>
<span
  v-else-if="downloadable"
  :data-test="`ep-download-${ep.number}`"
  role="button"
  tabindex="0"
  class="ep-dl text-muted-foreground hover:text-foreground"
  :title="$t('player.aePlayer.offline.download')"
  @click.stop="emit('download', ep)"
  @keydown.enter.stop="emit('download', ep)"
><Download :size="14" /></span>
```

Icons: extend the existing named lucide import in this file with `Download, Loader2` (`Check` is already imported for the watched tick). Add a scoped style matching the file's existing class conventions:

```css
.ep-dl { display: inline-flex; align-items: center; margin-left: auto; padding: 2px; }
```

(Grid view — the dense 100+-episode cells — intentionally gets no download affordance in v1; the strip view is the default and grid cells have no room. Note this in the commit message.)

Run: `bunx vitest run src/components/player/aePlayer/EpisodesPanel.spec.ts` — expected: PASS.

- [ ] **Step 5: Create `DownloadDialog.vue`** (player dir → native controls fine; styled like existing player menus)

```vue
<template>
  <div class="dl-dialog" role="dialog" :aria-label="$t('player.aePlayer.offline.quality')">
    <div class="dl-title text-sm font-semibold">{{ $t('player.aePlayer.offline.quality') }}</div>
    <div class="dl-est text-muted-foreground">{{ $t('player.aePlayer.offline.estimate', { size: SIZE_HINT[quality] }) }}</div>
    <div class="dl-opts">
      <button
        v-for="q in QUALITIES"
        :key="q"
        type="button"
        class="dl-opt"
        :class="{ 'dl-opt-active': q === quality }"
        @click="quality = q"
      >{{ q }}p</button>
    </div>
    <div class="dl-actions">
      <button type="button" class="dl-btn dl-btn-primary font-medium" @click="confirm">
        {{ $t('player.aePlayer.offline.start') }}
      </button>
      <button type="button" class="dl-btn font-medium" @click="emit('close')">
        {{ $t('player.aePlayer.offline.cancel') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const QUALITIES = ['480', '720', '1080'] as const
const SIZE_HINT: Record<string, string> = { '480': '250 MB', '720': '450 MB', '1080': '900 MB' }
const LS_KEY = 'ae.downloadQuality'

const emit = defineEmits<{
  (e: 'confirm', quality: string): void
  (e: 'close'): void
}>()

const saved = localStorage.getItem(LS_KEY)
const quality = ref<string>(saved && (QUALITIES as readonly string[]).includes(saved) ? saved : '720')

function confirm() {
  localStorage.setItem(LS_KEY, quality.value)
  emit('confirm', quality.value)
}
</script>

<style scoped>
.dl-dialog {
  position: absolute;
  inset-inline: 0;
  bottom: 4rem;
  margin-inline: auto;
  width: 16rem;
  padding: 0.75rem;
  border-radius: 0.5rem;
  background: var(--scrim-bg-strong);
  border: 1px solid var(--white-a8);
  z-index: 30;
}
.dl-title { margin-bottom: 0.25rem; }
.dl-est { font-size: 0.75rem; margin-bottom: 0.5rem; }
.dl-opts { display: flex; gap: 0.5rem; margin-bottom: 0.75rem; }
.dl-opt {
  flex: 1;
  padding: 0.25rem 0;
  border-radius: 0.375rem;
  border: 1px solid var(--white-a8);
  color: var(--color-muted-foreground, currentColor);
}
.dl-opt-active { border-color: var(--brand-cyan); color: var(--brand-cyan); }
.dl-actions { display: flex; gap: 0.5rem; }
.dl-btn { flex: 1; padding: 0.375rem 0; border-radius: 0.375rem; border: 1px solid var(--white-a8); }
.dl-btn-primary { border-color: var(--brand-cyan); color: var(--brand-cyan); }
</style>
```

(If the DS lint flags any token here, snap to the nearest token listed in `DESIGN-SYSTEM.md` — the alpha tokens `--white-a8`, `--scrim-bg-strong`, `--brand-cyan` are the canonical ones per the DS rules.)

- [ ] **Step 6: Wire into `AePlayer.vue`** (anchored edits — locate, don't guess line numbers)

1. Imports block: `import DownloadDialog from './DownloadDialog.vue'`, `import { offlineRuntimeReady } from '@/offline/flag'`, `import { enqueueDownload, engineState } from '@/offline/downloadEngine'`, `import { listDownloads } from '@/offline/registry'`, `import type { DownloadState } from '@/offline/types'`.
2. State (near other panel state refs): 

```ts
const downloadDialogEp = ref<EpisodeOption | null>(null)
const downloadStates = ref<Record<number, DownloadState>>({})
// offlineRuntimeReady() is non-reactive (navigator.serviceWorker.controller) —
// a plain :downloadable="offlineRuntimeReady()" would never appear after the
// SW's first claim. Track it in a ref, refreshed when the SW becomes ready.
const canDownload = ref(false)

async function refreshDownloadStates() {
  if (!offlineRuntimeReady()) return
  const all = await listDownloads()
  const mine: Record<number, DownloadState> = {}
  for (const d of all) if (d.animeId === props.animeId) mine[d.episode.number] = d.state
  downloadStates.value = mine
}
onMounted(() => {
  canDownload.value = offlineRuntimeReady()
  navigator.serviceWorker?.ready.then(() => { canDownload.value = offlineRuntimeReady() }).catch(() => {})
  void refreshDownloadStates()
})
// Throttle: engineState.progress ticks per segment (~300×/episode); a raw
// watcher would hammer IndexedDB with listDownloads() on every tick.
let dlRefreshQueued = false
watch(engineState.progress, () => {
  if (dlRefreshQueued) return
  dlRefreshQueued = true
  setTimeout(() => { dlRefreshQueued = false; void refreshDownloadStates() }, 1000)
})

function onDownloadEpisode(ep: EpisodeOption) {
  downloadDialogEp.value = ep
}

async function onConfirmDownload(quality: string) {
  const ep = downloadDialogEp.value
  downloadDialogEp.value = null
  if (!ep) return
  const comboSnapshot = { ...combo.value } // freeze — user may switch sources mid-download
  await enqueueDownload({
    animeId: props.animeId,
    animeTitle: props.anime.title,
    poster: props.anime.still,
    episode: ep,
    combo: comboSnapshot,
    quality,
    resolve: () => resolver.resolveStream(comboSnapshot.provider, props.animeId, ep, comboSnapshot),
  })
  void refreshDownloadStates()
}
```

Anchor notes: `combo` and `resolver` already exist in AePlayer's setup (the reference doc §5/§9 names them; locate the actual local identifiers — the resolver instance is the `useProviderResolver()` result, the combo ref comes from `usePlayerState()`). If AePlayer is mounted offline (Task 11's `offline` prop), `downloadable` must be `false` — gate on `!props.offline`.

3. Template — extend the existing `<EpisodesPanel …>` mount:

```html
  :downloadable="canDownload"
  :download-states="downloadStates"
  @download="onDownloadEpisode"
```
(Task 11 tightens this to `:downloadable="!offline && canDownload"` once the `offline` prop exists — no downloading while already offline-playing.) Add near it:

```html
<DownloadDialog
  v-if="downloadDialogEp"
  @confirm="onConfirmDownload"
  @close="downloadDialogEp = null"
/>
```

- [ ] **Step 7: Full gate**

```bash
bunx vitest run src/components/player/aePlayer/ src/locales/__tests__/ && bun run build
```
Expected: PASS + clean build (DS-lint hook fires on the edited .vue files — fix any violation it reports before committing).

- [ ] **Step 8: Commit**

```bash
git add src/offline/flag.ts src/vite-env.d.ts src/components/player/aePlayer/DownloadDialog.vue src/components/player/aePlayer/EpisodesPanel.vue src/components/player/aePlayer/EpisodesPanel.spec.ts src/components/player/aePlayer/AePlayer.vue src/locales/en.json src/locales/ru.json src/locales/ja.json
git commit src/offline/flag.ts src/vite-env.d.ts src/components/player/aePlayer/DownloadDialog.vue src/components/player/aePlayer/EpisodesPanel.vue src/components/player/aePlayer/EpisodesPanel.spec.ts src/components/player/aePlayer/AePlayer.vue src/locales/en.json src/locales/ru.json src/locales/ja.json -m "feat(offline): per-episode download button + quality dialog in aePlayer (strip view only)"
```

---

### Task 11: Offline playback through AePlayer (offline resolver adapter)

**Files:**
- Create: `frontend/web/src/offline/offlineAdapter.ts`
- Create: `frontend/web/src/offline/offlineAdapter.spec.ts`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (anchored, two guard points)

**Interfaces:**
- Consumes: `OfflineDownload` records (Task 5); `ProviderResolver` interface from `@/composables/aePlayer/useProviderResolver`; `CapabilityReport`/`ProviderCap` from `@/types/capabilities`.
- Produces:
  - `OfflinePlayback = { animeId: string; title: string; downloads: OfflineDownload[] }` — built by DownloadsPage (Task 12).
  - `makeOfflineResolver(p: OfflinePlayback): ProviderResolver` — provider id is always `'offline'`; `resolveStream` returns local `/__offline/…` URLs + local subtitle tracks.
  - `offlineCapabilityReport(p: OfflinePlayback): CapabilityReport` — synthetic one-provider report so AePlayer's existing selection machinery picks `'offline'` unaided.
  - New AePlayer prop: `offline?: OfflinePlayback | null`.

- [ ] **Step 1: Write failing tests** — `src/offline/offlineAdapter.spec.ts`

```ts
import { describe, it, expect } from 'vitest'
import { makeOfflineResolver, offlineCapabilityReport, type OfflinePlayback } from './offlineAdapter'
import type { OfflineDownload } from './types'

function dl(n: number, over: Partial<OfflineDownload> = {}): OfflineDownload {
  return {
    id: `a1:${n}:gogoanime:sub:en::720`, animeId: 'a1', animeTitle: 'T', quality: '720',
    episode: { key: n, label: n, number: n }, streamType: 'hls', state: 'done',
    combo: { audio: 'sub', lang: 'en', provider: 'gogoanime', server: 's', team: null },
    bytes: 1, resourcesDone: 2, resourcesTotal: 2, createdAt: n,
    playlistLocalPath: `/__offline/a1:${n}/master.m3u8`,
    subtitles: [{ url: `/__offline/a1:${n}/sub/0`, provider: 'jimaku', lang: 'ja', label: 'JA', format: 'ass' }],
    ...over,
  }
}

const p: OfflinePlayback = { animeId: 'a1', title: 'T', downloads: [dl(2), dl(1)] }

describe('makeOfflineResolver', () => {
  const r = makeOfflineResolver(p)
  it('lists downloaded episodes sorted by number', async () => {
    expect((await r.listEpisodes('offline', 'a1')).map((e) => e.number)).toEqual([1, 2])
  })
  it('resolves a local StreamResult with local subtitles', async () => {
    const eps = await r.listEpisodes('offline', 'a1')
    const s = await r.resolveStream('offline', 'a1', eps[0], dl(1).combo)
    expect(s.url).toBe('/__offline/a1:1/master.m3u8')
    expect(s.type).toBe('hls')
    expect(s.subtitles?.[0].url).toBe('/__offline/a1:1/sub/0')
  })
  it('throws for an episode that is not downloaded', async () => {
    await expect(r.resolveStream('offline', 'a1', { key: 9, label: 9, number: 9 }, dl(1).combo))
      .rejects.toThrow()
  })
  it('ignores non-done downloads', async () => {
    const r2 = makeOfflineResolver({ ...p, downloads: [dl(1, { state: 'error' })] })
    expect(await r2.listEpisodes('offline', 'a1')).toEqual([])
  })
})

describe('offlineCapabilityReport', () => {
  it('exposes exactly one active selectable provider named offline', () => {
    const rep = offlineCapabilityReport(p)
    expect(rep.anime_id).toBe('a1')
    expect(rep.families).toHaveLength(1)
    expect(rep.families[0].providers).toHaveLength(1)
    expect(rep.families[0].providers[0]).toMatchObject({ provider: 'offline', state: 'active', selectable: true })
  })
})
```

- [ ] **Step 2: Run to verify failure**

```bash
bunx vitest run src/offline/offlineAdapter.spec.ts
```
Expected: FAIL.

- [ ] **Step 3: Implement `src/offline/offlineAdapter.ts`**

```ts
// Offline playback masquerades as one more provider through the SAME seams the
// live player uses: a ProviderResolver + a capability report. AePlayer's
// default-selection machinery then needs zero special-casing — the synthetic
// feed has exactly one active provider, so it wins every pick.
import type { ProviderResolver } from '@/composables/aePlayer/useProviderResolver'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { StreamResult } from '@/types/aePlayer'
import type { CapabilityReport } from '@/types/capabilities'
import type { OfflineDownload } from './types'

export interface OfflinePlayback {
  animeId: string
  title: string
  downloads: OfflineDownload[]
}

export const OFFLINE_PROVIDER_ID = 'offline'

function ready(p: OfflinePlayback): OfflineDownload[] {
  return p.downloads
    .filter((d) => d.state === 'done')
    .sort((a, b) => a.episode.number - b.episode.number)
}

export function makeOfflineResolver(p: OfflinePlayback): ProviderResolver {
  return {
    async listEpisodes(): Promise<EpisodeOption[]> {
      return ready(p).map((d) => d.episode)
    },
    async resolveStream(_provider, _animeId, ep): Promise<StreamResult> {
      const d = ready(p).find((x) => x.episode.number === ep.number)
      if (!d) throw new Error(`episode ${ep.number} is not downloaded`)
      return { url: d.playlistLocalPath, type: d.streamType, subtitles: d.subtitles }
    },
    async listTeams(): Promise<string[]> {
      return []
    },
  }
}

/** Synthetic one-provider feed. The REAL CapabilityReport shape is
 *  `{ anime_id, families: SourceFamily[] }` (types/capabilities.ts) — NOT a
 *  flat providers array; rowsFromReport() hard-requires Array.isArray(families)
 *  and returns [] otherwise, which would leave the offline player sourceless.
 *  Group 'firstparty' serves every lang, so the saved-combo restore can never
 *  filter the offline row out. */
export function offlineCapabilityReport(p: OfflinePlayback): CapabilityReport {
  const first = ready(p)[0]
  const audio = first?.combo.audio ?? 'sub'
  return {
    anime_id: p.animeId,
    families: [
      {
        family: 'offline',
        providers: [
          {
            provider: OFFLINE_PROVIDER_ID,
            display_name: 'Offline',
            state: 'active',
            selectable: true,
            hacker_only: false,
            order: 1,
            group: 'firstparty',
            audios: audio === 'dub' ? ['dub', 'sub'] : ['sub', 'dub'],
            reason: '',
            variants: [],
          },
        ],
      },
    ],
  } as unknown as CapabilityReport
}
```

(Copy the exact `SourceFamily`/`ProviderCap` required-field set from the fixtures in `useProviderFeed.spec.ts` — fill any missing required fields with those neutral values and drop the `as unknown as` bridge once the literal satisfies the type.)

- [ ] **Step 4: Run tests**

```bash
bunx vitest run src/offline/offlineAdapter.spec.ts
```
Expected: PASS.

- [ ] **Step 5: Add the `offline` prop + two guards in `AePlayer.vue`** (anchored)

1. Props: add to `defineProps<{ … }>`:
```ts
  /** Offline playback bundle (from /downloads). When set: episodes + streams
   *  come from the local download store, the capability feed is synthetic
   *  (one 'offline' provider), and no network resolution is attempted. */
  offline?: import('@/offline/offlineAdapter').OfflinePlayback | null
```

2. **Guard point 1 — resolver construction.** Anchor: the single `useProviderResolver()` call in AePlayer's setup. Replace with:
```ts
const resolver = props.offline ? makeOfflineResolver(props.offline) : useProviderResolver()
```
(import `makeOfflineResolver`, `offlineCapabilityReport` from `@/offline/offlineAdapter`).

3. **Guard point 2 — capability feed.** Anchor: the `useCapabilities(...)` usage that produces the `report` consumed by `useProviderFeed`/`rowsFromReport`. When `props.offline` is set, the report must be `offlineCapabilityReport(props.offline)` and **no fetch/poll may fire**. Pattern (adapt to the actual local names):
```ts
const cap = useCapabilities(/* existing args */)
const report = computed(() =>
  props.offline ? offlineCapabilityReport(props.offline) : cap.report.value,
)
```
…and every downstream consumer of `cap.report` switches to `report`; the initial `cap.load()`/poll call gets `if (!props.offline)`. If `useCapabilities` auto-fetches on construction, construct it lazily: `const cap = props.offline ? null : useCapabilities(...)` and `report` falls back accordingly (`cap?.report.value ?? offlineCapabilityReport(props.offline!)`).

4. Task 10 interlock: the `:downloadable` binding on EpisodesPanel becomes `!offline && canDownload` (no downloading while already offline-playing).

5. Watch tracking stays ON (animeId is real; offline writes buffer through Task 9). WT/url-sync need no change — DownloadsPage passes no `room` and ignores `url-sync`.

- [ ] **Step 6: Regression suite + build**

```bash
bunx vitest run src/components/player/aePlayer/ src/composables/aePlayer/ src/offline/ && bun run build
```
Expected: all existing player specs stay green; build clean. **This is the riskiest task of the plan — if any existing player spec breaks, fix the guard, never the spec.**

- [ ] **Step 7: Commit**

```bash
git add src/offline/offlineAdapter.ts src/offline/offlineAdapter.spec.ts src/components/player/aePlayer/AePlayer.vue
git commit src/offline/offlineAdapter.ts src/offline/offlineAdapter.spec.ts src/components/player/aePlayer/AePlayer.vue -m "feat(offline): AePlayer offline playback via synthetic provider adapter"
```

---

### Task 12: Downloads store + /downloads page + nav

**Files:**
- Create: `frontend/web/src/stores/downloads.ts`
- Create: `frontend/web/src/views/DownloadsPage.vue`
- Create: `frontend/web/src/views/__tests__/DownloadsPage.spec.ts`
- Modify: `frontend/web/src/router/index.ts`
- Modify: `frontend/web/src/components/layout/Navbar.vue`

**Interfaces:**
- Consumes: registry list/remove (Task 5), engine (`engineState`, `removeDownload`, `markEvicted`, Task 7), `OfflinePlayback` + AePlayer `offline` prop (Task 11), i18n keys (Task 10), `offlineDownloadsEnabled` flag.
- Produces: `useDownloadsStore()` Pinia store; route `/downloads` (name `downloads`, `meta.titleKey: 'downloads.title'`); Navbar link.

- [ ] **Step 1: Implement `src/stores/downloads.ts`**

```ts
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { OfflineDownload } from '@/offline/types'
import { listDownloads } from '@/offline/registry'
import { engineState, markEvicted, removeDownload, pauseDownload, enqueueDownload, storageEstimate } from '@/offline/downloadEngine'
import { useProviderResolver } from '@/composables/aePlayer/useProviderResolver'

export const useDownloadsStore = defineStore('downloads', () => {
  const entries = ref<OfflineDownload[]>([])
  const storage = ref<{ usage: number; quota: number } | null>(null)
  const loading = ref(false)

  async function refresh(): Promise<void> {
    loading.value = true
    try {
      entries.value = await markEvicted(await listDownloads())
      storage.value = await storageEstimate() // via the OfflineMediaStore port (Task 7b)
    } finally {
      loading.value = false
    }
  }

  async function remove(id: string): Promise<void> {
    await removeDownload(id)
    await refresh()
  }

  function pause(id: string): void {
    pauseDownload(id)
    void refresh()
  }

  /** Resume a paused/failed download — needs network: re-resolves the stream
   *  via the live resolver with the entry's frozen combo; cached resources
   *  are skipped by the engine. */
  async function resume(d: OfflineDownload): Promise<void> {
    const resolver = useProviderResolver()
    await enqueueDownload({
      animeId: d.animeId,
      animeTitle: d.animeTitle,
      episode: d.episode,
      combo: d.combo,
      quality: d.quality,
      resolve: () => resolver.resolveStream(d.combo.provider, d.animeId, d.episode, d.combo),
    })
    await refresh()
  }

  /** animeId → downloads, newest anime first. */
  const byAnime = computed(() => {
    const groups = new Map<string, OfflineDownload[]>()
    for (const d of [...entries.value].sort((a, b) => a.episode.number - b.episode.number)) {
      const g = groups.get(d.animeId) ?? []
      g.push(d)
      groups.set(d.animeId, g)
    }
    return [...groups.entries()].sort(
      (a, b) => Math.max(...b[1].map((d) => d.createdAt)) - Math.max(...a[1].map((d) => d.createdAt)),
    )
  })

  return { entries, storage, loading, refresh, remove, pause, resume, byAnime, progress: engineState.progress }
})
```

- [ ] **Step 2: Write failing view test** — `src/views/__tests__/DownloadsPage.spec.ts`

```ts
import 'fake-indexeddb/auto'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import DownloadsPage from '@/views/DownloadsPage.vue'
import { putDownload, _resetDbForTests } from '@/offline/registry'
import { _installCachesForTests } from '@/offline/downloadEngine'
import type { OfflineDownload } from '@/offline/types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => p ? `${k}:${JSON.stringify(p)}` : k }) }))
vi.mock('@/components/player/aePlayer/AePlayer.vue', () => ({ default: { name: 'AePlayer', template: '<div data-testid="offline-player" />' } }))

const doneDl: OfflineDownload = {
  id: 'a1:1:gogoanime:sub:en::720', animeId: 'a1', animeTitle: 'Frieren', quality: '720',
  episode: { key: 1, label: 1, number: 1 }, streamType: 'hls', state: 'done',
  combo: { audio: 'sub', lang: 'en', provider: 'gogoanime', server: 's', team: null },
  bytes: 1000, resourcesDone: 2, resourcesTotal: 2, createdAt: 5,
  playlistLocalPath: '/__offline/x/master.m3u8', subtitles: [],
}

beforeEach(async () => {
  setActivePinia(createPinia())
  await _resetDbForTests()
  _installCachesForTests({
    async has() { return true }, async open() { return {} as Cache },
    async delete() { return true }, async keys() { return [] }, async match() { return undefined },
  } as unknown as CacheStorage)
})

describe('DownloadsPage', () => {
  it('shows empty state without downloads', async () => {
    const w = mount(DownloadsPage)
    await vi.waitFor(() => expect(w.text()).toContain('downloads.empty'))
  })
  it('lists a done download grouped by anime and opens offline playback', async () => {
    await putDownload(doneDl)
    const w = mount(DownloadsPage)
    await vi.waitFor(() => expect(w.text()).toContain('Frieren'))
    await w.find('[data-testid="watch-a1"]').trigger('click')
    // AePlayer is a defineAsyncComponent — even the mocked module resolves a
    // microtask later; assert with retry, not synchronously
    await vi.waitFor(() => expect(w.find('[data-testid="offline-player"]').exists()).toBe(true))
  })
  it('deletes after confirm', async () => {
    await putDownload(doneDl)
    const w = mount(DownloadsPage)
    await vi.waitFor(() => expect(w.text()).toContain('Frieren'))
    await w.find(`[data-testid="del-${CSS.escape(doneDl.id)}"]`).trigger('click')   // arm
    await w.find(`[data-testid="del-${CSS.escape(doneDl.id)}"]`).trigger('click')   // confirm
    await vi.waitFor(() => expect(w.text()).not.toContain('Frieren'))
  })
})
```

Run to verify failure: `bunx vitest run src/views/__tests__/DownloadsPage.spec.ts` — FAIL (no view).

- [ ] **Step 3: Implement `src/views/DownloadsPage.vue`** (outside player dir → ui primitives + semantic tokens ONLY)

```vue
<template>
  <div class="container mx-auto p-4 md:p-6 lg:p-8">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-xl font-semibold">{{ t('downloads.title') }}</h1>
      <p v-if="store.storage" class="text-sm text-muted-foreground">
        {{ t('downloads.storage', { used: fmtBytes(store.storage.usage), total: fmtBytes(store.storage.quota) }) }}
      </p>
    </div>

    <section v-if="playing" class="mb-8">
      <AePlayer
        :key="playing.animeId"
        :anime-id="playing.animeId"
        :anime="{ title: playing.title, eps: playing.downloads.length }"
        :theater="false"
        :offline="playing"
        :initial-episode="playingEpisode"
      />
      <Button variant="ghost" size="sm" class="mt-2" @click="playing = null">
        {{ t('common.close') }}
      </Button>
    </section>

    <p v-if="!store.loading && store.entries.length === 0" class="text-muted-foreground">
      {{ t('downloads.empty') }}
    </p>

    <div class="grid gap-4">
      <Card v-for="[animeId, group] in store.byAnime" :key="animeId" class="p-4 md:p-6">
        <div class="flex items-start gap-4">
          <img
            v-if="group[0].posterPath"
            :src="group[0].posterPath"
            :alt="group[0].animeTitle"
            class="w-16 rounded-md object-cover"
            loading="lazy"
          >
          <div class="flex-1 min-w-0">
            <h2 class="font-semibold truncate">{{ group[0].animeTitle }}</h2>
            <ul class="mt-2 grid gap-2">
              <li v-for="d in group" :key="d.id" class="flex items-center gap-3 text-sm">
                <span class="text-muted-foreground">{{ t('downloads.episode', { n: d.episode.number }) }}</span>
                <Badge v-if="d.state === 'done'" variant="secondary">{{ t('downloads.offlineReady') }}</Badge>
                <Badge v-else-if="d.state === 'downloading' || d.state === 'queued'" variant="secondary">
                  {{ t('downloads.state.downloading', progressOf(d)) }}
                </Badge>
                <Badge v-else-if="d.state === 'error'" variant="destructive">
                  {{ t(`downloads.error.${d.error ?? 'network'}`) }}
                </Badge>
                <Badge v-else variant="secondary">{{ t(`downloads.state.${d.state}`) }}</Badge>
                <span class="text-muted-foreground">{{ fmtBytes(d.bytes) }}</span>
                <span class="ml-auto flex items-center gap-2">
                  <Button
                    v-if="d.state === 'downloading' || d.state === 'queued'"
                    variant="ghost"
                    size="sm"
                    @click="store.pause(d.id)"
                  >
                    {{ t('downloads.pause') }}
                  </Button>
                  <Button
                    v-else-if="d.state === 'paused' || (d.state === 'error' && d.error === 'network')"
                    variant="ghost"
                    size="sm"
                    @click="store.resume(d)"
                  >
                    {{ t('downloads.resume') }}
                  </Button>
                  <Button
                    :data-testid="`del-${d.id}`"
                    variant="ghost"
                    size="sm"
                    :class="armed === d.id ? 'text-destructive' : 'text-muted-foreground'"
                    @click="onDelete(d.id)"
                  >
                    {{ armed === d.id ? t('downloads.confirmDelete') : t('downloads.delete') }}
                  </Button>
                </span>
              </li>
            </ul>
            <Button
              v-if="group.some((d) => d.state === 'done')"
              :data-testid="`watch-${animeId}`"
              class="mt-3"
              size="sm"
              @click="play(animeId, group)"
            >
              {{ t('downloads.watch') }}
            </Button>
          </div>
        </div>
      </Card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { defineAsyncComponent, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
// The ui dir is FLAT with one barrel (src/components/ui/index.ts) — there are
// no per-component subdirs, so '@/components/ui/button' does not resolve.
import { Button, Card, Badge } from '@/components/ui'
import { useDownloadsStore } from '@/stores/downloads'
import type { OfflineDownload } from '@/offline/types'
import type { OfflinePlayback } from '@/offline/offlineAdapter'

const AePlayer = defineAsyncComponent(() => import('@/components/player/aePlayer/AePlayer.vue'))

const { t } = useI18n()
const store = useDownloadsStore()
const playing = ref<OfflinePlayback | null>(null)
const playingEpisode = ref<number | undefined>(undefined)
const armed = ref<string | null>(null)

onMounted(() => void store.refresh())

function progressOf(d: OfflineDownload): { done: number; total: number } {
  return store.progress[d.id] ?? { done: d.resourcesDone, total: d.resourcesTotal }
}

function fmtBytes(n: number): string {
  if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(1)} GB`
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(0)} MB`
  return `${Math.max(1, Math.round(n / 1024))} KB`
}

function play(animeId: string, group: OfflineDownload[]) {
  playing.value = { animeId, title: group[0].animeTitle, downloads: group }
  playingEpisode.value = group.find((d) => d.state === 'done')?.episode.number
}

function onDelete(id: string) {
  if (armed.value !== id) {
    armed.value = id
    setTimeout(() => { if (armed.value === id) armed.value = null }, 4000)
    return
  }
  armed.value = null
  void store.remove(id)
}
</script>
```

Badge note: `variant="secondary"` renders brand-pink per `badge-variants.ts` — wrong tone for "Ready". Check `badge-variants.ts` for a `success` variant and use it for the done/offlineReady badge; if none exists, use the neutral/outline variant from that file (NOT secondary). `common.close` already exists in all locales (verify with `grep '"close"' src/locales/en.json`; if absent, use a `downloads.close` key added to all three).

- [ ] **Step 4: Route + navbar**

`src/router/index.ts` — before the catch-all `/:pathMatch(.*)*` route:
```ts
  {
    path: '/downloads',
    name: 'downloads',
    component: () => import('@/views/DownloadsPage.vue'),
    meta: { titleKey: 'downloads.title' }
  },
```

`src/components/layout/Navbar.vue` — the links array (~line 392, `{ to: '/browse', label: 'nav.catalog' }`): append
```ts
  { to: '/downloads', label: 'nav.downloads' },
```
Gate: only when the flag is on — wrap with the existing pattern used for conditional links in that array (if none exists, filter the array: `.filter((l) => l.to !== '/downloads' || offlineDownloadsEnabled)` with `import { offlineDownloadsEnabled } from '@/offline/flag'`).

- [ ] **Step 5: Run tests + build**

```bash
bunx vitest run src/views/__tests__/DownloadsPage.spec.ts src/locales/__tests__/ && bun run build
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/stores/downloads.ts src/views/DownloadsPage.vue src/views/__tests__/DownloadsPage.spec.ts src/router/index.ts src/components/layout/Navbar.vue
git commit src/stores/downloads.ts src/views/DownloadsPage.vue src/views/__tests__/DownloadsPage.spec.ts src/router/index.ts src/components/layout/Navbar.vue -m "feat(offline): downloads library page with offline playback + nav entry"
```

---

### Task 13: E2E smoke + full verification

**Files:**
- Create: `frontend/web/e2e/pwa.spec.ts`

**Interfaces:**
- Consumes: everything above, built `dist/`.

- [ ] **Step 1: Create `e2e/pwa.spec.ts`** (mirror the config style of the existing `e2e/spotlight.spec.ts` — same baseURL/fixtures conventions):

```ts
import { test, expect } from '@playwright/test'

// PWA smoke: requires a PRODUCTION build being served (SW registration is
// PROD-gated). Skips itself on dev servers where /sw.js 404s.
test.describe('PWA shell', () => {
  test('manifest is served and linked', async ({ page }) => {
    // The default e2e webServer is the DEV server, where the PWA plugin emits
    // neither sw.js nor the manifest link — probe and self-skip, same as below.
    const swProbe = await page.request.get('/sw.js')
    test.skip(!swProbe.ok(), 'no sw.js — dev server / SW not built')
    await page.goto('/')
    const href = await page.locator('link[rel="manifest"]').getAttribute('href')
    expect(href).toBeTruthy()
    const resp = await page.request.get(href!)
    expect(resp.ok()).toBeTruthy()
    const manifest = await resp.json()
    expect(manifest.name).toBe('AnimeEnigma')
    expect(manifest.display).toBe('standalone')
  })

  test('service worker takes control and app shell survives offline reload', async ({ page, context }) => {
    const swProbe = await page.request.get('/sw.js')
    test.skip(!swProbe.ok(), 'no sw.js — dev server / SW not built')
    await page.goto('/')
    await page.waitForFunction(() => !!navigator.serviceWorker?.controller, undefined, { timeout: 20_000 })
    await context.setOffline(true)
    await page.reload()
    await expect(page.locator('#app')).not.toBeEmpty()
    // downloads page is part of the shell — reachable offline with empty state
    await page.goto('/downloads')
    await expect(page.locator('#app')).not.toBeEmpty()
    await context.setOffline(false)
  })
})
```

- [ ] **Step 2: Run e2e against a local prod preview**

```bash
cd frontend/web && bun run build && bunx playwright test e2e/pwa.spec.ts --reporter=list
```
Expected: PASS (or the offline test self-skips if the configured e2e target is a dev server — in that case run once against `vite preview` per the playwright config's baseURL override convention).

- [ ] **Step 3: Full frontend gate**

Run the `/frontend-verify` skill (DS-lint, i18n parity, real `bun run build`, trap checks) and fix anything it flags.

- [ ] **Step 4: Manual smoke checklist (owner-facing, post-deploy)**

1. Android Chrome: install prompt → installed app opens standalone, dark theme.
2. Download one episode per family (kodik / scraper EN / ae) → airplane mode → play from /downloads, seek works (MP4 source too), subtitles selectable where downloaded.
3. Deploy a trivial rebuild → open installed app mid-playback → no reload until playback stops; reload after.
4. Flip `sw-config.json` to `{"kill": true}` on one client → SW unregisters (devtools → Application).

- [ ] **Step 5: Commit + finish**

```bash
git add e2e/pwa.spec.ts
git commit e2e/pwa.spec.ts -m "test(pwa): e2e smoke — manifest, SW control, offline shell"
```

Then run the `animeenigma-after-update` skill (simplify → lint/build → `make redeploy-web` → health → Russian Trump-mode changelog → push). Mark feedback `2026-07-01T01-30-04_tNeymik_telegram` → `ai_done` after deploy verification.

---

## Task dependency order

1 → 2 → 3 → 4 (shell, sequential) ; 5 → 6 → 7 → 7b (engine core + port) ; 8 needs 5's naming only; 9 needs 5; 10 needs 5+7; 11 needs 5+10 (interlock note); 12 needs 7b+11 (storageEstimate); 13 last. Tasks 8/9 can run parallel to 6/7 **in separate sessions but the same worktree is single-agent** — execute serially unless using separate worktrees per phase.

## Deferred from spec (explicit)

- **Auto-delete-watched toggle** (spec "Storage & quota", listed as optional): needs a watched-state join against `anime_list`/watch-progress — follow-up after v1 lands. Manual delete + pause/resume ship in Task 12.
- Eviction scan runs on DownloadsPage mount rather than app start (spec said "app start") — same user-visible outcome (marked before the list renders), zero cost on non-download sessions.
- **E2E download→offline-play→progress-flush and deploy-update reload-guard specs** (spec "Testing"): playwright cannot reliably intercept SW-initiated fetches and a real-media fixture pipeline is out of proportion for v1 — that behavior is covered by the unit suites (engine, offlineServe, progressQueue, registerPwa) plus the owner manual-smoke checklist in Task 13 Step 4.

## Metrics

UXΔ = +4 (Better) · CDI = 0.06 * 34 · MVQ = Kraken 85%/75%
