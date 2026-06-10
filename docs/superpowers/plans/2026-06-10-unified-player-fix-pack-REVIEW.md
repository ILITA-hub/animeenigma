# Plan Review — Unified Player Fix Pack (2026-06-10)

**Plan:** `docs/superpowers/plans/2026-06-10-unified-player-fix-pack.md`
**Spec:** `docs/superpowers/specs/2026-06-10-unified-player-fix-pack-design.md`
**Reviewed:** 2026-06-10 (pre-implementation plan review against the live tree on `feat/ds-primitives-lucide`)
**Verdict:** Sound architecture; the AniSkip stack, hls.js 1.5.20 API usage, Switch API, lucide exports, and EpisodeOption/StreamResult shapes all check out against the real files. But the plan has **1 CRITICAL** stale-file rewrite that would clobber today's parallel PlayerIconButton migration, **4 HIGH** issues (two render the shipped features invisible/broken, one test fails as written, one phantom-spinner logic bug), 2 MEDIUM races, and 7 LOW items.

**Counts:** CRITICAL 1 · HIGH 4 · MEDIUM 2 · LOW 7

---

## CRITICAL

### C1 — Task 1 Step 3 rewrites the skip buttons against a STALE PlayerControlBar.vue, reverting the PlayerIconButton migration

**Task 1, Step 3** (`PlayerControlBar.vue`)

The plan's replacement snippet uses raw `<button class="pl-icon pl-skip-back">`. The file was refactored **today (Jun 10, 11:56)** by a parallel agent: both skip buttons are now `<PlayerIconButton class="pl-skip-back">` (see `PlayerControlBar.vue:30-59`), and the `.pl-icon` CSS was **deleted from this component** (comment at `PlayerControlBar.vue:242-246`: styles moved into the `PlayerIconButton` primitive). The `.pl-icon` rules that do still exist live in:
- `UnifiedPlayer.vue` `<style scoped>` — scoped, does NOT apply to PlayerControlBar's children; and
- `unified/unified-player.css` — which is **imported nowhere** in the app (verified by grep), i.e. dead CSS.

Executing the snippet verbatim would (a) ship two completely unstyled `<button>`s in the control bar, (b) lose the primitive's hover/active/focus-ring behavior, and (c) **overwrite a concurrent committer's refactor in the shared tree** — exactly the failure mode the repo's worktree rules warn about.

**Fix:** Keep the `<PlayerIconButton>` wrappers exactly as they are; only edit the inner `<svg>` bodies (wrap the path in the mirrored `<g>` for the fwd button, set both `<text x="12">`). The `data-test` attrs fall through to the root `<button>`, so the planned test selectors (`[data-test="seek-back"] text`) still work unchanged.

---

## HIGH

### H1 — `var(--success)` and `var(--warning)` do not exist; DebugHud text, mini-stats, and the heatmap ok/warn tones resolve to nothing

**Task 10 Step 3 & 6, Task 11 Step 1** (`DebugHud.vue` CSS `color: var(--success)`, `PlayerScrubBar.vue` `.pl-frag[data-tone='ok'|'warn']`, settings mini-stats `text-[var(--success)]`)

`frontend/web/src/styles/main.css` defines `--success-foreground`, `--success-soft`, `--warning-foreground`, `--warning-soft`, `--destructive`, `--info`, `--brand-cyan` — but **no base `--success:` or `--warning:` token exists anywhere in `src/styles/`** (verified by grep; only `--destructive` of the planned trio is real). Consequences:

- DebugHud body text: `color: var(--success)` → invalid → inherits (white-ish), losing the green hacker aesthetic — cosmetic.
- Heatmap: `background: var(--success)` / `var(--warning)` → **invalid declaration, no background painted** → 'ok' and 'warn' fragments are completely invisible; only 'bad' (red, `--destructive`) renders. The heatmap silently ships 2/3 broken.
- Settings mini-stats `text-[var(--success)]` → same, falls back to inherited color.

There is also no `--color-success`/`--color-warning` in the Tailwind `@theme` block, so `text-success`/`bg-warning` utilities don't exist either.

**Fix:** Add base tokens to `main.css` Tier-3 block (the `-soft` rgba values imply the intended hues): `--success: #00ff9d; --warning: #ffd600;` plus `@theme` aliases `--color-success: var(--success); --color-warning: var(--warning);`. Do this in a Task-0 step so Tasks 10/11 land on real tokens. (Adding the tokens in a `.css` file does not interact with the DS lint hex rule, which only scans `.vue` files.)

### H2 — Pre-existing: chapter markers use `var(--warning)` → Task 7's "chapter markers visible" acceptance criterion fails even with correct wiring

**Task 7 Step 6(e) + Task 12 smoke item 6** (`PlayerScrubBar.vue:189`)

`.pl-chapter { background: var(--warning); }` already exists in `PlayerScrubBar.vue` and references the same **undefined** token from H1. The chapter-marker layer the plan feeds with real AniSkip data will render **invisible spans** — the unit tests pass (they only count `[data-test="chapter"]` elements), but the in-browser smoke step 6 ("chapter markers visible") will fail, and the user-facing deliverable is silently absent.

**Fix:** Covered by the H1 token addition — but call it out explicitly in Task 7 so the executor verifies the marker actually paints (this is exactly the class of jsdom-blind bug DS-NF-06 exists for).

### H3 — Task 7's clamping test FAILS against the planned `segmentsToChapters` implementation

**Task 7, Step 1 vs Step 3** (`skipSegments.spec.ts` / `skipSegments.ts`)

```ts
expect(segmentsToChapters({ start: 1395, end: 1500 }, null, 1400)).toEqual([])
```

Trace the planned implementation: `start = max(0, min(1395, 1400)) = 1395`; `end = max(1395, min(1500, 1400)) = 1400`; `end - start = 5`, which is **not** `< 1`, so a chapter `{ startPct: 99.64, widthPct: 0.36 }` IS emitted — the assertion expects `[]` and fails. Step 4 ("Expected: PASS") is unreachable; a TDD executor will either get stuck or "fix" the implementation by inventing an unspecified drop-threshold.

**Fix:** Make the test input genuinely degenerate after clamping, e.g. `segmentsToChapters({ start: 1399.5, end: 1500 }, null, 1400)` (clamped width 0.5s < 1s → dropped). Or, if the intent really is "drop segments starting within 5s of the end", encode that threshold explicitly in `segmentsToChapters` — but pick one; as written, test and implementation contradict each other.

### H4 — `@stalled` → phantom endless spinner during normal playback; nothing clears buffering while playing smoothly

**Task 3, Steps 1-2** (`UnifiedPlayer.vue` video events)

`stalled` fires per spec when the UA is *fetching* but received no data for ~3s — browsers (especially Firefox, and Chrome on the native-MP4 path) fire it **spuriously when the download is throttled because the buffer is full**, i.e. during perfectly healthy playback. Once `setBuffering(true)` runs and the 150ms timer fires, the ring shows — and the only OFF events are `playing`, `canplay`, `seeked`, **none of which re-fire during uninterrupted playback**. Result: a spinner permanently overlaying a video that is playing fine, until the next pause/seek. This will hit the AniLib/mp4-quality paths (native `<video src>`) hardest.

**Fix (pick one):**
1. Drop `@stalled` entirely — `waiting` is the reliable "playback actually starved" signal and already covers real stalls; or
2. Keep it but add a self-healing clear: `@timeupdate="onTimeUpdate"` with `if (isBuffering.value && v.readyState >= 3 && !v.seeking) setBuffering(false)` — timeupdate fires ~4Hz during playback, so a false-positive clears within 250ms.

(For the record: the `if (on === isBuffering.value) return` guard itself is correct — repeated `setBuffering(true)` calls cannot leak timers, and `false` always clears the pending timer. No issue there.)

---

## MEDIUM

### M1 — Episode clicks are silently dropped while a resolve is in-flight

**Task 4, Step 5(f)** (`onSelectEpisode` relying on the combo/episode watcher)

`onSelectEpisode` only sets `selectedEpisode` and trusts the watcher at `UnifiedPlayer.vue:427-441` to re-resolve. That watcher early-returns with `if (isResolving.value) return` — a guard designed to suppress the duplicate fire from `loadEpisodesAndStream` setting `selectedEpisode` itself. But it equally swallows a **user's second episode click** made while the first click's resolve is still in flight (a 1-3s window on slow providers): the UI highlights episode N+1, the panel closes, and the stream stays on episode N forever — no error, no retry.

**Fix:** Mirror the existing `goToNextEpisode` pattern — have `onSelectEpisode` call `void resolveStreamForEpisode(ep)` directly after setting `selectedEpisode`. `resolveStreamForEpisode` sets `isResolving = true` synchronously, so the watcher's subsequent (microtask-deferred) fire is correctly deduped, and the monotonic `resolveToken` already arbitrates the race with any in-flight resolve.

### M2 — `currentStream.value = stream` placed after `await engine.load(stream)` with no token re-check → stale-stream clobber race

**Task 6, Step 5(a)** (`UnifiedPlayer.vue`, both resolve paths)

The plan inserts the assignment *after* `await engine.load(stream)`. The token check happens **before** that await; while `engine.load` is suspended (dynamic hls.js import + attach), a newer resolve can start, win, and set `currentStream` — then the stale call resumes and overwrites `currentStream` with the old stream. The engine itself is protected by its `loadGen` guard, so you end up with the video playing stream B while `currentStream` (and therefore `mp4Qualities`, the quality menu, the HUD `stream-type`, and the mp4-Auto swap URL) describes stream A.

**Fix:** Set `currentStream.value = stream` immediately after the existing `if (token !== resolveToken) return` check, next to `resolvedServers.value = ...` (before `await engine.load(stream)`), in both `loadEpisodesAndStream` and `resolveStreamForEpisode`.

---

## LOW

### L1 — Quality label survives stream reloads but the new hls instance starts at Auto → display/actual mismatch
**Task 6, Step 5(b).** `state.quality` is only snapped back to `'Auto'` when the new ladder *lacks* the label. If the user pinned `720p` and the next episode's ladder also has `720p`, the menu keeps showing `720p` while the fresh hls instance plays at `currentLevel = -1` (auto). Either reset `state.quality.value = 'Auto'` in the engine `load()` reset block, or re-apply `engine.setLevel(state.quality.value)` when `engine.levels` populates and the label exists.

### L2 — `buildLevelLabels` sort mixes units; HD badge assumes desc ordering for mp4
**Task 6, Steps 3 & 6(c).** `parseInt('1500k') = 1500` sorts a 1500-kbit bitrate-labelled level above `1080p` — wrong axis comparison when a master playlist mixes height-ful and height-less variants. Similarly the HD badge picks `qualities.find(x => x !== 'Auto')` — first entry — but the mp4 branch preserves the provider's `qualities` array order, which is not guaranteed desc. Sort mp4 labels desc too, or only badge HLS ladders.

### L3 — Spec/plan divergence on the AniSkip backend (plan is correct)
The spec's section 7 describes building a **new** catalog client/route (`services/catalog/internal/parser/aniskip/`, `/api/anime/{uuid}/skip-times?episode=N`). The Phase-18 stack already exists and is live: `services/catalog/internal/handler/skip_times.go`, gateway route `/skip-times/*` (`gateway router.go:279`), catalog route `/skip-times/{malId}/{episode}` (`catalog router.go:176`), `animeApi.getSkipTimes` (`client.ts:398`), `useSkipTimes`. The plan's "No Go changes" is right — executors must NOT follow the spec's backend section. The plan's wiring of `useSkipTimes(malIdRef, epNumber)` matches the real signature (`Ref<string|number|null|undefined>, Ref<number|null|undefined>`) and `{ opening, ending }` returns; `anime.shikimoriId` exists (`useAnime.ts:55`) and the composable gracefully no-ops on undefined. ✓

### L4 — Back chevron keeps `aria-label="Back"` while now opening the episodes drawer
**Task 4, Step 5(c).** Rewiring the top-left chevron to `toggleMenu('episodes')` without updating `aria-label="Back"` ships a misleading accessible name. Change to `aria-label="Episodes"` (and consider a `ListVideo`/`List` glyph instead of the back chevron — a left-chevron that opens a drawer is a UX trap, though the spec does mandate this wiring).

### L5 — `hudVisible` keys off `isBuffering` (no 150ms grace) → HUD flashes on every instant in-buffer seek
**Task 11, Step 2(b).** Use `showBuffering` instead of `isBuffering` in `hudVisible` for consistency with the spinner's debounce.

### L6 — Composable specs run outside a component instance → `onUnmounted` warnings
**Task 9 Step 1 (usePlaybackStats.spec), Task 8 Step 1.** `usePlaybackStats` registers `onUnmounted` and the specs call it bare → Vue logs "onUnmounted is called when there is no active component instance" per test. Tests still pass (jsdom env confirmed in `vitest.config.ts:10`; `localStorage` available), but the noise pollutes CI output and the interval-cleanup path is untested. Optionally wrap in `effectScope()` in the specs, or accept the warning. Also note `usePlayerState`'s new `watch` leaks per bare call in specs — harmless.

### L7 — `maxBufferLength: 60` can be silently capped by the default `maxBufferSize` (60 MB)
**Task 5.** On high-bitrate streams hls.js stops appending at `maxBufferSize` before reaching 60s. Not a bug — just don't be surprised if the HUD shows <60s ahead on 1080p; bump `maxBufferSize` only if the smoke shows it mattering.

---

## Verified-correct (no action — listed so the executor doesn't "re-fix" them)

- **hls.js 1.5.20 API** (checked against `node_modules/hls.js/dist/hls.js.d.ts`): `Events.MANIFEST_PARSED` (`data.levels` ✓), `Events.LEVEL_SWITCHED` (`data.level` ✓), `Events.FRAG_LOADED` (`data.frag.stats` is `LoaderStats { total, loading: {start, first, end} }` ✓), `hls.bandwidthEstimate` getter ✓, `maxBufferLength`/`maxMaxBufferLength` config keys ✓, `currentLevel = -1` for auto ✓, `startLoad(-1)` preserved in the replacement handler ✓ (keep the Kodik CODECS-less comment when replacing).
- **`export interface` inside `<script setup>`**: allowed — `@vue/compiler-sfc` (Vue 3.5.35) only rejects `ExportNamedDeclaration` with `exportKind !== "type"`; repo precedent `ui/Select.vue:72`. The plan's two-script-block hedge is unnecessary but harmless.
- **Switch API**: `ui/Switch.vue` takes `modelValue: boolean` + emits `update:modelValue` — the planned `:model-value` / `@update:model-value` usage matches the existing Autoplay/Auto-skip rows. ✓
- **lucide-vue-next**: `Terminal` and `FastForward` are real named exports. ✓
- **`currentStream?.type` in template**: `currentStream` is a top-level ref → auto-unwrapped; `.value` correctly used in script computeds and explicitly on nested `engine.*`/`state.*` refs in the template, matching existing file style. ✓
- **v-if/v-else placement**: DebugHud's `<template v-if>` + sibling `<div v-else>` is a valid chain; BufferingOverlay/DebugHud/EpisodesPanel insertions don't interpose into any existing v-if/v-else-if chain. ✓
- **DebugHud number formatting tests**: `4.2 Mbit/s`, `+42.3s`, `12.2s`, `512KB`, `2.0MB`, `230ms` all trace correctly through the planned `toFixed`/`Math.round` paths. ✓
- **usePlaybackStats math test**: ahead 95−50=45, behind 50−40=10 ✓; `getVideoPlaybackQuality: undefined` degrades via the `typeof === 'function'` guard ✓.
- **EpisodesPanel tests**: `EpisodeOption` shape (`key/label/number/isFiller?` from `EpisodeSelector.types.ts`) ✓; `brand-cyan` class-substring assertions match `text-[var(--brand-cyan)]` ✓; `data-test="episode-N"` template literal ✓.
- **Buffering debounce timer lifecycle**: the `on === isBuffering.value` guard prevents duplicate timers; `false` path always clears; unmount cleanup added in Task 3 Step 4. ✓ (Only the `stalled` *source* event is problematic — H4.)
- **Quality watcher loop safety**: `watch(qualities, …)` writing `state.quality` cannot re-trigger itself (`qualities` doesn't depend on `quality`). ✓
- **Auto-skip on rAF-updated `currentTime`**: watcher batches per flush (~once per frame), guarded by `autoSkippedEp`; acceptable. ✓
- **`Anime.vue:649`** is exactly `@open-episodes="() => {}"` as the plan states; removal + emit-drop are consistent within Task 4. ✓
- **DS lint**: all new colors are `rgba()`/`var()` (rule 2 only bans raw hex in `.vue`); no off-palette Tailwind classes introduced; `font-mono` is a family, not a banned weight. ✓ (But see H1 — two of the `var()` targets don't exist.)

---

## Suggested plan amendments (summary)

1. **Task 1:** keep `<PlayerIconButton>`; edit only the SVG innards (C1).
2. **New Task 0 (or fold into Task 10):** define `--success`/`--warning` (+ `@theme` aliases) in `main.css`; verify `.pl-chapter` paints (H1, H2).
3. **Task 7:** fix the clamping test input to `{ start: 1399.5, end: 1500 }` (H3).
4. **Task 3:** drop `@stalled` or add the `timeupdate` self-heal (H4).
5. **Task 4:** `onSelectEpisode` calls `resolveStreamForEpisode(ep)` directly (M1); update the chevron's aria-label (L4).
6. **Task 6:** set `currentStream` before `engine.load` (M2); reset quality to Auto on load or re-apply level (L1).
