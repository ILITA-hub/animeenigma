# PWA + Offline Downloads — Design

**Date:** 2026-07-02
**Source:** feedback `2026-07-01T01-30-04_tNeymik_telegram` — «Сделать мобильное приложение чтобы можно было скачать аниме и смотреть оффлайн».
**Owner decision:** deliver as a **Progressive Web App** (multiplatform), maximally reusing the existing Vue 3 frontend, with auto-updating bundles ("pull on JS bundle rebuild"). Two tasks: **(A) PWA shell/build**, **(B) offline download module**.

**Metrics:** UXΔ = +4 (Better) · CDI = 0.06 * 34 (shell 8 + download module 21 + integration) · MVQ = Kraken 85%/75%.

---

## Goals

1. AnimeEnigma is installable on Android/iOS/desktop as a PWA (manifest + service worker + offline app shell).
2. New JS bundle deploys are picked up automatically by installed clients — no manual "clear cache / reinstall".
3. Users can download episodes to the device and watch them fully offline inside the existing AePlayer.

## Non-goals (v1)

- Background Fetch API (download-while-closed on Chrome) — explicit later upgrade; v1 downloads run while the tab/PWA is open.
- TWA / Play Store / App Store packaging.
- Kodik **iframe** fallback downloads (technically impossible — third-party iframe).
- DRM / storage encryption of downloaded media.
- Push notifications (separate workstream; the SW will exist, but no push logic in v1).

## Locked decisions (owner-approved 2026-07-02)

| # | Decision |
|---|----------|
| 1 | **Download scope**: everything that plays through our own HLS/MP4 proxy — Kodik HLS adapter, EN scraper chain, AllAnime raw-JP, Hanime, ae (MinIO). Excluded: Kodik iframe. |
| 2 | **One foreground download engine everywhere** (works while tab/PWA is open; Wake Lock + segment-level resume). Background Fetch = later upgrade. iOS supported with the caveat that the PWA should be installed to Home Screen (non-installed Safari tabs get script-writable storage evicted after 7 days of non-use). |
| 3 | **Auto-update**: silent (`autoUpdate`), but the page reload is **deferred while video is playing** — apply on next navigation / when playback stops. |
| 4 | **Download quality**: user picks in the download dialog; default **720p**; last choice remembered (localStorage). |
| 5 | **UI surfaces**: download button on the episodes panel (anime page + in-player) + a **Downloads** page (`/downloads`) that is itself offline-capable. |

---

## Task A — PWA shell («сборка»)

### Build integration

- `vite-plugin-pwa` in **`injectManifest`** mode — we author `frontend/web/src/sw.ts` ourselves (Workbox precaching + custom offline-video routes; `generateSW` cannot express the video serving logic).
- `registerType: 'autoUpdate'`, but with a custom reload guard (below).
- Manifest (`manifest.webmanifest`): `name: AnimeEnigma`, `display: standalone`, `theme_color`/`background_color: #08080f`, `start_url: /`, `lang: ru`; icons reuse existing `public/android-chrome-192x192.png` / `512` + add one **maskable** 512 variant. `screenshots` omitted in v1.
- **Precache**: full app shell — hashed JS/CSS chunks, fonts, icons (~5–6 MB). Excluded by glob: `*.gz`, `*.br` twins (nginx-only), `changelog.json` (fetched fresh every load by design), `branding/`, OG images. `navigateFallback: /index.html` with denylist for `/api/`, `/og/`, `/admin/{grafana,prometheus,pgadmin,k8s}`, `/socket.io`.
- **Runtime caching**: none for `/api/*` (network-only; offline metadata comes from IndexedDB, not HTTP cache) — avoids stale-capability-feed class of bugs. Posters: cache-first runtime cache **only** for posters explicitly saved at download time (see Task B); general browsing posters stay network.

### Update flow ("pull on rebuild")

Substrate is already correct: nginx serves hashed assets `immutable,1y` and `index.html` no-cache.

1. Deploy rebuilds bundle → `sw.js` (with embedded precache manifest of new hashes) changes byte-wise.
2. Clients check for SW updates: on every page load, on `visibilitychange` → visible, and hourly via `registration.update()` (long-lived SPA sessions).
3. New SW installs, precaches the diff, calls `skipWaiting()` + `clientsClaim()`.
4. **Reload guard**: on `controllerchange` the app reloads immediately **unless** a `<video>` is actively playing; then the reload is deferred until playback pauses/ends or the next router navigation. Implemented in a small `usePwaUpdate.ts` composable; the player exposes "is playing" via the existing player store.
5. nginx additions: `location = /sw.js` and `location = /manifest.webmanifest` → `Cache-Control: no-cache` (must not fall into the 1y-immutable static block).

### RU asset-edge interaction

`window.__assetHost` (dark-shipped Maskanya edge) can emit cross-origin chunk URLs that miss the origin-keyed precache. SW fetch handler: for requests to the configured edge host whose path matches `/assets/*`, attempt network; on failure (offline), respond with the origin precache entry for the same path. No-op while the edge stays dark.

### Kill-switch (SW safety)

A broken SW can brick clients until manual unregister. Safety valve: the app fetches `/sw-config.json` (no-cache, tiny) on load; if `{"kill": true}`, the register module unregisters the SW and clears `workbox-*` caches. Ship this from day one — it is ~20 lines and turns a potential support disaster into a one-line ops flip.

---

## Task B — Offline download module («модуль скачки»)

### Architecture (approach A — "local HLS via Service Worker")

Divert at the **source-resolution layer**, not inside the player. `useVideoEngine.ts` already consumes a plain `{streamUrl, streamType: 'hls'|'mp4'}` — offline playback hands it a synthetic local URL; the video engine is untouched.

```
Download:  resolve stream (same path as player: capabilities → scraper/parser
           → signed proxy URLs) → fetch master/variant playlist → pick quality
           → fetch segments + AES-128 keys + subtitles through the proxy
           → rewrite playlist to /__offline/{id}/… URLs
           → segments/keys/playlists → Cache Storage (cache "ae-offline-{id}")
           → registry entry + poster + metadata → IndexedDB

Playback:  /downloads → open AePlayer with streamUrl=/__offline/{id}/master.m3u8
           (HLS) or /__offline/{id}/media.mp4 (MP4 sources; streamType stored in registry)
           → SW intercepts /__offline/* → serves from Cache Storage
           → hls.js plays it exactly like a network stream (offline or online)
```

### Components (all under `frontend/web/src/offline/` unless noted)

| Unit | Purpose | Depends on |
|------|---------|-----------|
| `downloadEngine.ts` | Queue (1 episode at a time), segment fetch loop (concurrency 3, paced ≤ 3 req/s), Wake Lock, pause/resume/cancel, segment-level resume | registry, existing stream-resolution API client |
| `playlistRewrite.ts` | Parse m3u8 (reuse parsing already available via hls.js utils or a ~100-line hand parser), map every URI (segments, keys, sub-playlists) to `/__offline/{id}/r/{n}`, emit rewritten playlists | — |
| `registry.ts` | IndexedDB (`ae-offline` DB): `downloads` store — id (canonical key), animeId, episode, combo `{audio,lang,provider,server,team}`, quality, state `queued|downloading|paused|done|error`, bytes, segmentsDone/total, createdAt, title/titles, posterCacheKey, subtitle refs | hand-rolled thin IndexedDB wrapper (no new dep) |
| `progressQueue.ts` | Offline watch-progress buffer: writes that fail (offline) are queued in IndexedDB and flushed on `online`/app start — keeps «продолжить просмотр» consistent | player progress API |
| `sw.ts` route (in Task A's SW) | `GET /__offline/{id}/…` → `caches.match`; MP4 entries get Range support (serve 206 slices from the cached full body — workbox-range-requests pattern) | Cache Storage |
| `DownloadButton.vue` + `DownloadDialog.vue` | Per-episode button on episodes panel (anime page + in-player); dialog = quality pick (default 720p, remembered) + size estimate | capabilities feed, engine |
| `DownloadsPage.vue` (`/downloads`) | Offline library: list with poster/title/episode/size/state, play, delete, pause/resume; storage meter (`navigator.storage.estimate()`); "delete watched" toggle | registry |

### Keying, signatures, expiry

- **Canonical key** `id = hash(animeId, episodeId, combo, quality)` — never the raw URL (signed proxy URLs expire in 1h and rotate).
- Downloads start immediately after resolve; on resume after >~50 min or on 401/403 from the proxy, **re-resolve** the stream and continue by segment index (playlists for the same variant are stable enough; if the re-resolved variant's segment list length/URIs mismatch, restart that episode's download from scratch — correctness over cleverness).
- AES-128 HLS: key URIs are rewritten and the key bytes cached like segments — playback needs zero network.
- Subtitles (Jimaku/OpenSubtitles/provider-bundled): the sub files selected at download time are fetched through the existing signed subtitle proxy path and cached under `/__offline/{id}/sub/{k}`; SubtitleOverlay consumes them as normal URLs.
- Poster: one image cached per download for the offline library UI.

### Storage & quota

- Request `navigator.storage.persist()` on first download (best-effort; on installed PWAs usually granted).
- Before download: `estimate()` check — refuse with a clear message if projected size > remaining quota.
- Downloads page shows used/available; manual delete always available; optional auto-delete-watched.
- Eviction detection: on app start, registry entries whose cache is missing (`caches.has` false) are marked `error: evicted` and surfaced in the UI instead of silently failing at play time.

### Rate-limit & backend etiquette

An episode ≈ 300–400 segments; an unthrottled download would trip the gateway per-user GCRA (240/min) and hammer provider CDNs. The engine paces at **≤ 3 req/s** (episode ≈ 2–3 min) with concurrency 3. No gateway carve-out in v1. The engine sends `X-AE-Download: 1` on segment fetches so the egress recorder/analytics can distinguish download bursts from live watching (header is additive; backend change optional and non-blocking).

### Error handling

| Failure | Behavior |
|---------|----------|
| Resolve fails (provider down) | Download errors immediately with provider-level message; user can retry with another combo |
| Segment fetch fails | Retry ×3 with backoff; then pause download in `error` state, resumable |
| Signature expired mid-download | Re-resolve + continue (above) |
| Quota exceeded mid-download | Pause in `error: quota`, prompt to free space |
| Cache evicted | Marked `evicted` on startup scan (above) |
| SW not yet controlling (first visit) | Download UI hidden until `navigator.serviceWorker.controller` exists |

### Adult content

Hanime downloads inherit the existing adult-gating at *download time* (button only where the source is visible today). The offline library lists whatever the device downloaded — same trust model as the browser profile itself. No extra gating in v1.

---

## Testing

- **Unit (vitest)**: `playlistRewrite` (variant/key/sub URI mapping, weird playlist shapes), registry state machine, canonical keying, progressQueue flush, reload-guard logic.
- **SW route**: unit-test the Range-slicing and `/__offline/*` matcher in isolation (mock Cache Storage).
- **E2E (playwright)**: install SW → download a short fixture episode (mock proxy responses) → `context.setOffline(true)` → play from `/downloads` → assert playback + progress queued → back online → progress flushed. Second spec: deploy-update simulation (swap sw.js) → reload guard defers while playing.
- **Manual smoke (owner)**: real Android Chrome install + one real episode download per source family; iOS Safari installed-PWA sanity pass.
- Existing gates: `/frontend-verify` (DS-lint, i18n en/ru/ja parity incl. all new `downloads.*` / `pwa.*` strings, real `bun run build`).

## Rollout

1. Task A ships first (PWA shell + auto-update + kill-switch) — low risk, immediately useful.
2. Task B ships behind `VITE_OFFLINE_DOWNLOADS_ENABLED` (default **on**; flag exists to yank the UI without a rebuild-revert if a storage/provider surprise appears).
3. nginx changes (`sw.js` no-cache) ride the same deploy as Task A.

## Portability — future standalone apps (owner requirement, added 2026-07-03)

The owner intends to ship **separate full-fledged apps** (Capacitor/Tauri-class wrappers over this codebase) later. The offline module therefore must not hard-bind to browser storage APIs:

- The download engine performs ALL media-byte I/O through an **`OfflineMediaStore` port** (`src/offline/mediaStore.ts`): `put/has/remove/exists/persist/estimate/entryUrl`. The engine, registry, playlist rewrite, offline adapter, progress queue, and all UI are platform-neutral.
- The **web adapter** (`cacheStorageMediaStore`) implements the port over Cache Storage (`ae-offline-{id}` caches) with the SW serving `/__offline/*` — exactly the behavior specced above.
- A future **native adapter** (Capacitor Filesystem / Tauri fs) implements the same port and supplies its own `entryUrl` scheme (file/asset URLs); the SW-serving half is simply unused there. Path construction is centralized (`offlinePath`) so the URL scheme is a one-function swap.
- Fully-native (Kotlin/Swift) apps reuse the **backend contracts** instead (capabilities feed, scraper/stream routes, signed proxy URLs) — those are already the platform-independent surface.

## Out of scope, recorded for later

Background Fetch upgrade (Chrome) · season-batch downloads · storage encryption · push notifications · TWA/Capacitor/Tauri packaging (the port above is the enabler, not the packaging itself).
