# Unified Player — Fix Pack + Buffering / Quality / Skip (Design)

**Date:** 2026-06-10 · **Status:** Approved by owner
**Scope:** Unified player (`frontend/web/src/components/player/unified/`, `frontend/web/src/composables/unifiedPlayer/`) + one new catalog endpoint.
**Metrics:** UXΔ = +3 (Better) · CDI = 0.06 × 13 · MVQ = Griffin 85%/80%

## Problems being fixed

1. Seeking (arrow keys / scrub clicks) shows no feedback while the stream re-buffers — feels like a hang.
2. No debug visibility into buffering/fragments for diagnosing slow providers.
3. The top-right episode-list (hamburger) button is wired to a no-op (`Anime.vue:649` `@open-episodes="() => {}"`) — choosing an episode from the player is impossible.
4. The digit "5" inside the ±5s skip buttons is visually drifted (forward arrow path is mirrored with `scaleX(-1)` but the `<text>` digit is not, and back/forward use different x offsets: 12.5 vs 11.5).
5. hls.js runs default ~30 s forward buffer, so a +5 s seek often falls outside buffer.
6. Settings menu hardcodes quality `['Auto']` even when the HLS master playlist has variants (deferred item D-05).
7. `SkipIntroChip`, the Auto-skip toggle, and scrub-bar chapter markers exist but are hardcoded off — no timings source.

## Design

### 1. Episodes drawer (fixes dead hamburger)

New `unified/EpisodesPanel.vue`, floating panel using the existing `pl-floating` pattern (like SourcePanel). Opened from the top-right list button **and** the top-left back chevron; `openMenu` union gains `'episodes'` (UnifiedPlayer + PlayerControlBar prop type). Renders the player's already-loaded `episodes: EpisodeOption[]` as a scrollable grid of episode-number buttons, current `selectedEpisode` highlighted. Click → set `selectedEpisode` → the existing watcher re-resolves the stream; panel closes. Esc / click-outside close (existing `closeMenus` path). Empty list → "No episodes from this source" placeholder. The `@open-episodes` no-op in `Anime.vue` is removed; the player handles episodes internally (works in fullscreen/theater). The `open-episodes` emit is dropped.

### 2. ±5s glyph fix

In `PlayerControlBar.vue`: wrap only the arrow `path` in a `<g>` (forward mirrored via the `<g>` transform); place the digit `<text>` at identical optically-centered coordinates in both buttons. Verify in-browser at desktop and mobile sizes.

### 3. Buffering spinner

New `unified/overlays/BufferingOverlay.vue` — centered ring spinner (DS keep-list inline spinner style, brand-cyan). UnifiedPlayer derives `isBuffering` from `<video>` events: ON at `waiting` / `seeking` / `stalled`, OFF at `playing` / `canplay` / `seeked`. A ~150 ms show-delay debounce avoids flicker on instant in-buffer seeks. Also shown while `isResolving` (provider/stream resolution). Hidden when `sourceError` is shown. Note: this makes the pre-existing D-07 fragment-stall visible as an endless spinner — D-07 itself is out of scope.

### 4. Hacker mode

- **Setting:** "Hacker mode" toggle in PlaybackSettingsMenu root (under Auto-skip), persisted via `usePlayerState` localStorage like other prefs. Default OFF.
- **`useVideoEngine` hook:** expose an `onStats` callback / stats ref fed by hls.js events — `FRAG_LOADED` (frag size bytes, duration, load time ms, bitrate), `LEVEL_SWITCHED`, `hls.bandwidthEstimate`. Rolling window of last ~30 fragments.
- **New composable `usePlaybackStats`:** merges engine telemetry with video-element data — readyState, buffered ranges, buffer ahead/behind seconds, `getVideoPlaybackQuality()` dropped frames, stream type (hls/mp4), provider, current level resolution. MP4 streams get the video-element subset only (graceful degradation, no fragment data).
- **`unified/overlays/DebugHud.vue`:** monospace stats panel, top-left, visible when hacker mode ON **and** (paused OR buffering/seeking).
- **Tinted scrub bar:** when hacker mode ON, `PlayerScrubBar` renders an extra layer — buffered ranges tinted; loaded-fragment boundaries colored by size (green→amber→red heatmap), size label on hover.
- **Gear menu:** when hacker mode ON, settings root shows a live mini-stats section (bandwidth, buffer ahead, level, last frag size/time).

### 5. Buffer window (FE-only)

`useVideoEngine` hls.js config: `maxBufferLength: 60`, `maxMaxBufferLength: 120`, keep `backBufferLength: 90` and `enableWorker: true`. Goal: ±5 s arrow seeks land inside buffer → instant. MP4 (native range requests) unchanged — browser-managed. If still laggy in practice, a later phase adds HLS-proxy segment prefetch (explicitly deferred by owner).

### 6. Real quality ladder

- `useVideoEngine` exposes `levels: Ref<{label: string; index: number}[]>` (from `MANIFEST_PARSED`, label = `${height}p`, deduped, sorted desc) and `setLevel(label | 'Auto')` → `hls.currentLevel = index` / `-1` for Auto. Reset on `load()`.
- UnifiedPlayer passes `['Auto', ...levels]` to PlaybackSettingsMenu instead of hardcoded `['Auto']`. In Auto, show the currently-playing level: "Auto · 720p" (from `LEVEL_SWITCHED`).
- MP4 streams with `StreamResult.qualities` (>1 entry): quality switch swaps `src` URL preserving `currentTime` + play state.
- Single-variant / no data → only "Auto" (D-05 rule: data-driven, no fake levels).

### 7. Intro/outro skip via AniSkip

**Backend (catalog):**
- New client `services/catalog/internal/parser/aniskip/client.go` → `GET https://api.aniskip.com/v2/skip-times/{malId}/{ep}?types=op&types=ed&episodeLength=0`. MAL id = anime's `shikimori_id`. Timeout 5 s.
- New route `GET /api/anime/{uuid}/skip-times?episode=N` (catalog handler → service). Response: `{"op": {"start": s, "end": s} | null, "ed": {...} | null}`.
- Redis cache: hits 24 h, misses/errors 6 h (timings are static; misses may be filled by the community later). AniSkip outage / 404 → `200` with both null — never an error to the player.
- Tests: handwritten-fake HTTP server for the client; handler test for cache + null paths.

**Frontend:**
- UnifiedPlayer fetches skip-times on episode change (and provider change is irrelevant — times are per-episode). Failures → silently no chapters.
- Map ranges to `chapters` prop (already exists on scrub bar): `{kind: 'intro'|'outro', startPct, widthPct}` from `duration` once known.
- `SkipIntroChip` becomes range-driven: visible while `currentTime` is inside the OP range (label "Skip Intro") or ED range ("Skip Outro"); click seeks to range end.
- Existing **Auto-skip intro** toggle: when ON, entering the OP range auto-seeks past it (once per episode view, so manual seek-back isn't fought).
- No data → chip hidden, no markers (F6 rule: no fake markers).

## Testing & verification

- Vitest: `usePlaybackStats`, EpisodesPanel, BufferingOverlay show/hide logic, quality-ladder wiring in engine (mock hls), skip-chip range logic.
- Go: aniskip client + handler tests (mocked upstream, cache, null paths).
- DS-NF-06 in-browser smoke on Frieren (desktop + ≤680 px): spinner on seek, hacker HUD, episodes drawer (incl. fullscreen), ±5 glyph, quality menu on a multi-variant stream, skip chip on an AniSkip-covered title.
- Design-system lint gate must stay green (no off-palette classes; heatmap colors use semantic/brand-exempt hues).

## Out of scope

- D-07 platform-wide HLS fragment stall (pre-existing, tracked separately).
- Backend segment prefetch (deferred until FE-only buffering proves insufficient).
- Admin override table for skip timings (possible later phase).
