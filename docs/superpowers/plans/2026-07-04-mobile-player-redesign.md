# Mobile Player Redesign + Season Downloads Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the aePlayer actually usable on phones (full-bleed video, fullscreen restored incl. iPhone pseudo-fullscreen, bottom-sheet menus, touch gestures, reachable download) and add a "download whole season" option.

**Architecture:** All changes are frontend-only, inside `frontend/web/`. One new composable (`useMobilePlayer`) provides reactive `isMobile`/`isCoarse` media-query state; the player's floating menus conditionally `<Teleport to="body">` as bottom sheets on mobile; fullscreen becomes capability-based (native + orientation lock on Android, fixed-position pseudo-FS on iPhone); a mobile-only action row renders under the video inside `AePlayer.vue`; season downloads are a pure helper looping the existing serial download engine.

**Tech Stack:** Vue 3 `<script setup>` SFCs, scoped CSS with existing DS tokens, Vitest + @vue/test-utils, bun/bunx.

**Spec:** `docs/superpowers/specs/2026-07-04-mobile-player-redesign-design.md` (read it first).

## Global Constraints

- **Worktree:** ALL work happens in `/data/ae-mobile-player` (branch `wt-mobile-player`). NEVER touch `/data/animeenigma`. Run `bun install` in `frontend/web/` once before the first task.
- **bun only:** `bun`/`bunx`, never npm/npx/pnpm. Tests: `bunx vitest run <path>`. Lint: `bunx eslint <paths>`. Build: `bun run build`.
- **ONE FE agent at a time** in this worktree (vite/vitest caches collide).
- **DS lint rules (build-enforced):** no off-palette Tailwind color classes; no raw hex/`rgba()`/`hsl()` literals in `.vue` (bind DS tokens: `--white-a8/a20/a30`, `--black-a40/a60/a80`, `--scrim-bg-strong`, `--brand-cyan`, `--cyan-a20/a40/a60`, semantic `text-warning`/`text-success`); only `font-medium`/`font-semibold` weights; no arbitrary spacing values (`px-[10px]`) — token scale only (rem values inside scoped CSS are fine); `components/player/` is EXEMPT from the "no bare native controls" rule only.
- **i18n:** every new key goes to `en.json`, `ru.json`, AND `ja.json` with identical ICU placeholders. Parity specs in `src/locales/__tests__` fail otherwise.
- **lucide:** NAMED imports only (`import { Download } from 'lucide-vue-next'`).
- **Types:** trust `bun run build` (vue-tsc + vite), not `--noEmit` (stale-cache false-pass). Import component types from the `@/components/ui` barrel (deep named type imports throw TS2614).
- **Player invariants:** all combo mutations via `usePlayerState` setters; subtitles never auto-enable; hls.js stays pinned `~1.5.20`; capability feed is authority (untouched here).
- **Git:** commit per task with pathspec (`git commit -m "..." -- <files>`), include the three co-author trailers:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
  Subagents COMMIT but never PUSH.
- **Metrics (no time units):** UXΔ = +4 (Better) · CDI = 0.04 * 21 · MVQ = Griffin 85%/80%.

## File Structure

| File | Responsibility |
|---|---|
| Create `src/composables/aePlayer/useMobilePlayer.ts` | Reactive `(max-width: 680px)` + `(pointer: coarse)` media-query refs (singleton) |
| Create `src/composables/aePlayer/useMobilePlayer.spec.ts` | Composable tests |
| Create `src/offline/seasonDownload.ts` | Pure season target selection + serial enqueue loop over the engine |
| Create `src/offline/seasonDownload.spec.ts` | Helper tests |
| Create `src/components/player/aePlayer/DownloadDialog.spec.ts` | Dialog v2 tests |
| Modify `src/components/player/aePlayer/PlayerControlBar.vue` | Mobile declutter, fullscreen restored, `fullscreenActive` prop, 44px targets |
| Modify `src/components/player/aePlayer/PlayerControlBar.spec.ts` | New prop/visibility tests |
| Modify `src/components/player/aePlayer/PlayerScrubBar.vue` | Touch grab-zone CSS |
| Modify `src/components/player/aePlayer/AePlayer.vue` | Fullscreen logic, gestures, sheet teleports, root restructure + action row, season wiring, mobile CSS |
| Modify `src/components/player/aePlayer/BrowseSubsModal.vue` | (no code change needed — host class handles it; see Task 6) |
| Modify `src/components/player/aePlayer/DownloadDialog.vue` | Scope selector, estimates, quota warning, sheet presentation |
| Modify `src/components/player/aePlayer/EpisodesPanel.vue` | Season chip, touch hit areas |
| Modify `src/components/player/aePlayer/EpisodesPanel.spec.ts` | Season chip test |
| Modify `src/views/Anime.vue` | Full-bleed player card on mobile |
| Modify `src/locales/{en,ru,ja}.json` | New `player.aePlayer.offline.*` keys |

---

### Task 1: `useMobilePlayer` composable

**Files:**
- Create: `frontend/web/src/composables/aePlayer/useMobilePlayer.ts`
- Test: `frontend/web/src/composables/aePlayer/useMobilePlayer.spec.ts`

**Interfaces:**
- Produces: `useMobilePlayer(): { isMobile: Ref<boolean>; isCoarse: Ref<boolean> }` and `_resetMobilePlayerForTests(): void`. `isMobile` = `(max-width: 680px)` (the player CSS breakpoint), `isCoarse` = `(pointer: coarse)`. Singleton — repeated calls return the same refs. In environments without `window.matchMedia` (jsdom without stub) both refs are `false` and nothing throws.

- [ ] **Step 0: Setup** — `cd /data/ae-mobile-player/frontend/web && bun install` (first task only).

- [ ] **Step 1: Write the failing test**

```ts
// frontend/web/src/composables/aePlayer/useMobilePlayer.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useMobilePlayer, _resetMobilePlayerForTests } from './useMobilePlayer'

type Listener = (e: { matches: boolean }) => void

function stubMatchMedia(initial: Record<string, boolean>) {
  const listeners: Record<string, Listener[]> = {}
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: initial[query] ?? false,
    addEventListener: (_: string, fn: Listener) => {
      ;(listeners[query] ??= []).push(fn)
    },
    removeEventListener: () => {},
  }))
  return {
    fire(query: string, matches: boolean) {
      for (const fn of listeners[query] ?? []) fn({ matches })
    },
  }
}

describe('useMobilePlayer', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
    _resetMobilePlayerForTests()
  })

  it('reads initial matches and reacts to changes', () => {
    const mm = stubMatchMedia({ '(max-width: 680px)': true, '(pointer: coarse)': false })
    const { isMobile, isCoarse } = useMobilePlayer()
    expect(isMobile.value).toBe(true)
    expect(isCoarse.value).toBe(false)
    mm.fire('(pointer: coarse)', true)
    expect(isCoarse.value).toBe(true)
    mm.fire('(max-width: 680px)', false)
    expect(isMobile.value).toBe(false)
  })

  it('is a singleton', () => {
    stubMatchMedia({})
    const a = useMobilePlayer()
    const b = useMobilePlayer()
    expect(a.isMobile).toBe(b.isMobile)
  })

  it('defaults to false without matchMedia (jsdom safety)', () => {
    vi.stubGlobal('matchMedia', undefined)
    const { isMobile, isCoarse } = useMobilePlayer()
    expect(isMobile.value).toBe(false)
    expect(isCoarse.value).toBe(false)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bunx vitest run src/composables/aePlayer/useMobilePlayer.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

```ts
// frontend/web/src/composables/aePlayer/useMobilePlayer.ts
import { ref, type Ref } from 'vue'

// Mobile-player environment as reactive refs. 680px mirrors the player CSS
// breakpoint (PlayerControlBar/AePlayer media queries); pointer:coarse gates
// touch-only behaviors (tap-to-toggle chrome, double-tap seek).
// Singleton: the MQL listeners live for the app's lifetime — one per query,
// shared across every player mount, so mounts never leak extra listeners.

interface MobilePlayerEnv {
  isMobile: Ref<boolean>
  isCoarse: Ref<boolean>
}

let cached: MobilePlayerEnv | null = null

function watchMedia(query: string): Ref<boolean> {
  const matches = ref(false)
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return matches
  const mql = window.matchMedia(query)
  matches.value = mql.matches
  mql.addEventListener?.('change', (e) => {
    matches.value = e.matches
  })
  return matches
}

export function useMobilePlayer(): MobilePlayerEnv {
  if (!cached) {
    cached = {
      isMobile: watchMedia('(max-width: 680px)'),
      isCoarse: watchMedia('(pointer: coarse)'),
    }
  }
  return cached
}

export function _resetMobilePlayerForTests(): void {
  cached = null
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bunx vitest run src/composables/aePlayer/useMobilePlayer.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Lint + commit**

```bash
bunx eslint src/composables/aePlayer/useMobilePlayer.ts src/composables/aePlayer/useMobilePlayer.spec.ts
git add src/composables/aePlayer/useMobilePlayer.ts src/composables/aePlayer/useMobilePlayer.spec.ts
git commit -m "feat(player): useMobilePlayer composable — reactive mobile/coarse media state" -- src/composables/aePlayer/useMobilePlayer.ts src/composables/aePlayer/useMobilePlayer.spec.ts
```
(Co-author trailers per Global Constraints — on every commit in this plan.)

---

### Task 2: `seasonDownload` helper

**Files:**
- Create: `frontend/web/src/offline/seasonDownload.ts`
- Test: `frontend/web/src/offline/seasonDownload.spec.ts`

**Interfaces:**
- Consumes: `enqueueDownload(req: DownloadRequest): Promise<string>` from `./downloadEngine`; `getDownload(id): Promise<OfflineDownload | undefined>` from `./registry`; `DownloadState` from `./types`.
- Produces:
  - `seasonTargets(episodes: EpisodeOption[], states: Record<number, DownloadState>): EpisodeOption[]` — episodes whose state is NOT `done`/`queued`/`downloading` (paused/error/absent re-enqueue).
  - `interface SeasonContext { animeId: string; animeTitle: string; poster?: string; combo: Combo; quality: string; resolveFor: (ep: EpisodeOption) => () => Promise<StreamResult> }`
  - `enqueueSeason(targets: EpisodeOption[], ctx: SeasonContext): Promise<number>` — serial enqueue, stops early on an engine quota error, returns count enqueued.

- [ ] **Step 1: Write the failing test**

```ts
// frontend/web/src/offline/seasonDownload.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { Combo } from '@/types/aePlayer'

const enqueueDownload = vi.fn()
const getDownload = vi.fn()
vi.mock('./downloadEngine', () => ({ enqueueDownload: (r: unknown) => enqueueDownload(r) }))
vi.mock('./registry', () => ({ getDownload: (id: string) => getDownload(id) }))

import { seasonTargets, enqueueSeason, type SeasonContext } from './seasonDownload'

const ep = (n: number): EpisodeOption => ({ id: `e${n}`, number: n, label: String(n) }) as EpisodeOption
const combo: Combo = { audio: 'sub', lang: 'en', provider: 'p', server: 's', team: null }

function ctx(): SeasonContext {
  return {
    animeId: 'a1',
    animeTitle: 'T',
    combo,
    quality: '720',
    resolveFor: vi.fn((e: EpisodeOption) => async () => ({ url: `u${e.number}` }) as never),
  }
}

describe('seasonTargets', () => {
  it('skips done/queued/downloading, keeps paused/error/untouched', () => {
    const eps = [ep(1), ep(2), ep(3), ep(4), ep(5)]
    const out = seasonTargets(eps, { 1: 'done', 2: 'queued', 3: 'downloading', 4: 'paused' })
    expect(out.map((e) => e.number)).toEqual([4, 5])
  })

  it('returns [] for a fully downloaded season', () => {
    expect(seasonTargets([ep(1)], { 1: 'done' })).toEqual([])
  })
})

describe('enqueueSeason', () => {
  beforeEach(() => {
    enqueueDownload.mockReset().mockImplementation(async (r: { episode: EpisodeOption }) => `id-${r.episode.number}`)
    getDownload.mockReset().mockResolvedValue({ state: 'queued' })
  })

  it('enqueues every target in order with per-episode resolve closures', async () => {
    const c = ctx()
    const n = await enqueueSeason([ep(1), ep(2)], c)
    expect(n).toBe(2)
    expect(enqueueDownload).toHaveBeenCalledTimes(2)
    expect(enqueueDownload.mock.calls[0][0]).toMatchObject({ animeId: 'a1', quality: '720', episode: { number: 1 } })
    expect(c.resolveFor).toHaveBeenCalledTimes(2)
  })

  it('stops early when the engine reports a quota error', async () => {
    getDownload
      .mockResolvedValueOnce({ state: 'queued' })
      .mockResolvedValueOnce({ state: 'error', error: 'quota' })
    const n = await enqueueSeason([ep(1), ep(2), ep(3)], ctx())
    expect(n).toBe(1)
    expect(enqueueDownload).toHaveBeenCalledTimes(2) // third never attempted
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bunx vitest run src/offline/seasonDownload.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

```ts
// frontend/web/src/offline/seasonDownload.ts
import type { Combo, StreamResult } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { DownloadState } from './types'
import { enqueueDownload } from './downloadEngine'
import { getDownload } from './registry'

/** Episodes a season download should enqueue: everything not already stored,
 *  queued, or in flight. Paused and errored episodes re-enqueue (resume path). */
export function seasonTargets(
  episodes: EpisodeOption[],
  states: Record<number, DownloadState>,
): EpisodeOption[] {
  return episodes.filter((ep) => {
    const s = states[ep.number]
    return s !== 'done' && s !== 'queued' && s !== 'downloading'
  })
}

export interface SeasonContext {
  animeId: string
  animeTitle: string
  poster?: string
  combo: Combo
  quality: string
  /** Factory for the engine's fresh-resolve closure (re-called on signed-URL expiry). */
  resolveFor: (ep: EpisodeOption) => () => Promise<StreamResult>
}

/** Serially enqueue every target. The engine's per-download quota pre-check
 *  marks a record `error:'quota'` instead of queueing it — once that happens
 *  every later enqueue would fail the same check, so stop instead of spamming
 *  error records. Returns how many episodes were actually enqueued. */
export async function enqueueSeason(targets: EpisodeOption[], ctx: SeasonContext): Promise<number> {
  let enqueued = 0
  for (const ep of targets) {
    const id = await enqueueDownload({
      animeId: ctx.animeId,
      animeTitle: ctx.animeTitle,
      poster: ctx.poster,
      episode: ep,
      combo: ctx.combo,
      quality: ctx.quality,
      resolve: ctx.resolveFor(ep),
    })
    const rec = await getDownload(id)
    if (rec?.state === 'error' && rec.error === 'quota') break
    enqueued++
  }
  return enqueued
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bunx vitest run src/offline/seasonDownload.spec.ts`
Expected: PASS (4 tests). Also run the neighbors: `bunx vitest run src/offline/` — all green.

- [ ] **Step 5: Lint + commit**

```bash
bunx eslint src/offline/seasonDownload.ts src/offline/seasonDownload.spec.ts
git add src/offline/seasonDownload.ts src/offline/seasonDownload.spec.ts
git commit -m "feat(offline): season download helper — target selection + quota-aware serial enqueue" -- src/offline/seasonDownload.ts src/offline/seasonDownload.spec.ts
```

---

### Task 3: Control bar mobile declutter + fullscreen restored

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/PlayerControlBar.vue`
- Modify: `frontend/web/src/components/player/aePlayer/PlayerScrubBar.vue` (style block only)
- Test: `frontend/web/src/components/player/aePlayer/PlayerControlBar.spec.ts` (extend existing)

**Interfaces:**
- Produces: new optional prop `fullscreenActive?: boolean` (default `false`) on `PlayerControlBar` — swaps the fullscreen icon Maximize↔Minimize. Task 4 wires it from AePlayer.
- Behavior at ≤680px after this task: hidden = ±5s, PiP, episodes pill, volume slider; VISIBLE = play, mute, time (plain text), source pill (dot+chevron only), CC, gear, **fullscreen**.

- [ ] **Step 1: Extend the spec with failing tests**

Open `PlayerControlBar.spec.ts`, reuse its existing mount helper/props pattern, add:

```ts
it('shows the fullscreen button and swaps its icon with fullscreenActive', async () => {
  const wrapper = mountBar() // the file's existing helper with required props
  const fsBtn = wrapper.find('[data-test="toggle-fullscreen"]')
  expect(fsBtn.exists()).toBe(true)
  // default: enter-fullscreen icon
  expect(wrapper.findComponent({ name: 'Maximize' }).exists() || fsBtn.html().includes('maximize')).toBe(true)
  await wrapper.setProps({ fullscreenActive: true })
  expect(fsBtn.html().includes('minimize')).toBe(true)
})
```

(Adapt the icon assertion to the file's existing style — lucide icons render `class="lucide-maximize-icon"`-style class names; asserting on `fsBtn.html()` containing `minimize` after the prop flip is enough.)

- [ ] **Step 2: Run to verify the new test fails**

Run: `bunx vitest run src/components/player/aePlayer/PlayerControlBar.spec.ts`
Expected: the new test FAILS (prop doesn't exist yet); existing tests pass.

- [ ] **Step 3: Implement PlayerControlBar changes**

3a. Script: add `Minimize` to the lucide import; add the prop:

```ts
import { Play, Pause, Volume1, Volume2, VolumeX, ChevronDown, Captions, Settings, PictureInPicture2, Maximize, Minimize, ListVideo } from 'lucide-vue-next'
```

In `defineProps` add:
```ts
    /** fullscreen (native or pseudo) currently active — swaps the FS icon */
    fullscreenActive?: boolean
```
and in `withDefaults` add `fullscreenActive: false`.

3b. Template — fullscreen button icon swap:

```html
      <!-- Fullscreen — the ONLY comfortable watch surface on phones, so it is
           NOT part of the mobile trim (it used to be — that was the bug). -->
      <PlayerIconButton
        class="pl-fs-btn"
        :aria-label="$t('player.aePlayer.fullscreen')"
        data-test="toggle-fullscreen"
        @click="emit('toggle-fullscreen')"
      >
        <Minimize v-if="fullscreenActive" class="size-5" aria-hidden="true" />
        <Maximize v-else class="size-5" aria-hidden="true" />
      </PlayerIconButton>
```

3c. Styles — DELETE the existing `@media (pointer: coarse)` block that force-shows `.pl-vol-range`, and REPLACE the whole `@media (max-width: 680px)` block at the bottom with:

```css
/* Mobile trim (≤680px): ±5s, PiP and the episodes pill go (seek moves to
   double-tap gestures; episodes has the top trigger + under-player action
   row). FULLSCREEN STAYS. The volume slider is hidden — phones use hardware
   volume (iOS ignores JS volume entirely); mute stays. */
@media (max-width: 680px) {
  .pl-skip-back,
  .pl-skip-fwd,
  .pl-pip-btn,
  .pl-epbtn,
  .pl-vol-range {
    display: none;
  }

  .pl-timepill {
    background: transparent;
    border: 0;
    padding: 0 2px;
  }

  .pl-srcbtn {
    max-width: 64px;
  }

  .pl-srcbtn-text {
    display: none;
  }
}

/* Touch ergonomics: ≥44px hit targets for every bar control. */
@media (pointer: coarse) {
  .pl-btns :deep(button) {
    min-width: 44px;
    min-height: 44px;
  }
}
```

- [ ] **Step 4: PlayerScrubBar touch grab zone** — append to its scoped styles:

```css
/* Touch: taller grab zone + always-visible thumb (there is no hover). */
@media (pointer: coarse) {
  .pl-track {
    height: 28px;
  }

  .pl-thumb {
    opacity: 1;
    width: 15px;
    height: 15px;
  }
}
```

- [ ] **Step 5: Run tests to verify pass**

Run: `bunx vitest run src/components/player/aePlayer/PlayerControlBar.spec.ts src/components/player/aePlayer/PlayerScrubBar.spec.ts`
Expected: PASS.

- [ ] **Step 6: Lint + commit**

```bash
bunx eslint src/components/player/aePlayer/PlayerControlBar.vue src/components/player/aePlayer/PlayerScrubBar.vue
git add -A src/components/player/aePlayer/PlayerControlBar.vue src/components/player/aePlayer/PlayerScrubBar.vue src/components/player/aePlayer/PlayerControlBar.spec.ts
git commit -m "feat(player): mobile control-bar declutter, fullscreen restored on mobile, 44px touch targets" -- src/components/player/aePlayer/PlayerControlBar.vue src/components/player/aePlayer/PlayerScrubBar.vue src/components/player/aePlayer/PlayerControlBar.spec.ts
```

---

### Task 4: Capability-based fullscreen in AePlayer (native + iOS pseudo-FS)

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`

**Interfaces:**
- Consumes: `useMobilePlayer()` (Task 1), `fullscreenActive` prop on PlayerControlBar (Task 3).
- Produces (used by Tasks 5–8 inside AePlayer): `const { isMobile, isCoarse } = useMobilePlayer()`; refs `pseudoFs: Ref<boolean>`, `nativeFsActive: Ref<boolean>`; computed `fullscreenActive`.

- [ ] **Step 1: Script setup additions**

Add import + composable near the other composable wiring:

```ts
import { useMobilePlayer } from '@/composables/aePlayer/useMobilePlayer'
```
```ts
const { isMobile, isCoarse } = useMobilePlayer()
```

Replace `onToggleFullscreen()` (currently ~L2506) with:

```ts
// ─── Fullscreen (capability-based) ───────────────────────────────────────────
// Android/desktop: native element fullscreen (+ landscape lock on touch).
// iPhone Safari has NO element fullscreen API — fall back to a fixed-position
// pseudo-fullscreen takeover. video.webkitEnterFullscreen() (the native iOS
// player) is deliberately NOT used: it drops SubtitleOverlay, the Source
// panel and the WT button.

const pseudoFs = ref(false)
const nativeFsActive = ref(false)
const fullscreenActive = computed(() => nativeFsActive.value || pseudoFs.value)

function onFullscreenChange() {
  nativeFsActive.value = !!document.fullscreenElement
  if (!nativeFsActive.value) unlockOrientation()
}

function lockLandscape() {
  const o = screen.orientation as ScreenOrientation & { lock?: (v: string) => Promise<void> }
  void o?.lock?.('landscape').catch(() => {})
}

function unlockOrientation() {
  try {
    screen.orientation?.unlock?.()
  } catch {
    /* not locked / unsupported */
  }
}

function onToggleFullscreen() {
  const el = rootRef.value ?? videoRef.value?.parentElement
  if (!el) return
  if (document.fullscreenElement) {
    void document.exitFullscreen()
    return
  }
  if (pseudoFs.value) {
    exitPseudoFs()
    return
  }
  if (el.requestFullscreen) {
    void el.requestFullscreen()
    if (isCoarse.value) lockLandscape()
    return
  }
  enterPseudoFs()
}

// Pseudo-FS pushes a history entry so the phone's back gesture exits the
// takeover instead of leaving the page.
function onPseudoFsPop() {
  exitPseudoFs(true)
}

function enterPseudoFs() {
  pseudoFs.value = true
  document.documentElement.classList.add('pl-noscroll')
  history.pushState({ plPseudoFs: true }, '')
  window.addEventListener('popstate', onPseudoFsPop)
}

function exitPseudoFs(viaPop = false) {
  if (!pseudoFs.value) return
  pseudoFs.value = false
  document.documentElement.classList.remove('pl-noscroll')
  window.removeEventListener('popstate', onPseudoFsPop)
  if (!viaPop && (history.state as { plPseudoFs?: boolean } | null)?.plPseudoFs) history.back()
}

/** Unmount-safe teardown: never touches history (a route change already moved it). */
function teardownPseudoFs() {
  if (!pseudoFs.value) return
  pseudoFs.value = false
  document.documentElement.classList.remove('pl-noscroll')
  window.removeEventListener('popstate', onPseudoFsPop)
}
```

In the existing `onMounted` add `document.addEventListener('fullscreenchange', onFullscreenChange)`; in the existing `onUnmounted` add `document.removeEventListener('fullscreenchange', onFullscreenChange)` and `teardownPseudoFs()`.

- [ ] **Step 2: Template wiring**

On the root `.pl` div's `:class` binding add `'pl--pseudo-fs': pseudoFs`.
On `<PlayerControlBar>` add `:fullscreen-active="fullscreenActive"`.

- [ ] **Step 3: CSS**

Append to the scoped style block (near `.pl--theater`):

```css
/* iPhone pseudo-fullscreen — fixed takeover (no element FS API on iOS).
   Black behind the notch/status bar is intended (video surface). */
.pl--pseudo-fs {
  position: fixed;
  inset: 0;
  z-index: 100;
  height: 100%;
  aspect-ratio: auto;
  border-radius: 0;
  border: 0;
}

:global(html.pl-noscroll) {
  overflow: hidden;
}
```

- [ ] **Step 4: Verify nothing regressed**

Run: `bunx vitest run src/components/player/aePlayer/`
Expected: PASS (no new tests — jsdom has no fullscreen API; this is browser-glue verified by the existing suite + manual smoke).

- [ ] **Step 5: Lint + commit**

```bash
bunx eslint src/components/player/aePlayer/AePlayer.vue
git add src/components/player/aePlayer/AePlayer.vue
git commit -m "feat(player): capability-based fullscreen — native + landscape lock, iOS pseudo-FS with back-gesture exit" -- src/components/player/aePlayer/AePlayer.vue
```

---

### Task 5: Touch gestures — tap toggles chrome, double-tap seeks, center pause

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`

**Interfaces:**
- Consumes: `isCoarse` (Task 4 wiring), existing `uiVisible`/`wakeUi`/`clearUiIdleTimer`/`onSeekRel`/`togglePlay`/`hasStarted`.
- Produces: mobile tap semantics — desktop click behavior byte-identical.

- [ ] **Step 1: Replace `onVideoClick` and gate the root touch-wake**

Replace the existing `onVideoClick()` (~L2432) with:

```ts
// A tap on the video: backdrop-dismiss if a menu is open (no side effect).
// Desktop click = play/pause. Touch (coarse): single tap toggles the chrome,
// double-tap on the side thirds seeks ±10s (center double-tap = play/pause) —
// so the play/pause affordance on phones is the center overlay button, never
// a stray tap.
const DOUBLE_TAP_MS = 280
let lastTapAt = 0
let lastTapX = 0
let singleTapTimer: ReturnType<typeof setTimeout> | null = null

const seekFlash = ref<'back' | 'fwd' | null>(null)
let seekFlashTimer: ReturnType<typeof setTimeout> | null = null

function flashSeek(dir: 'back' | 'fwd') {
  seekFlash.value = dir
  if (seekFlashTimer) clearTimeout(seekFlashTimer)
  seekFlashTimer = setTimeout(() => {
    seekFlash.value = null
  }, 500)
}

function onVideoClick(e: MouseEvent) {
  if (openMenu.value !== null || browseOpen.value) {
    closeMenus()
    return
  }
  if (!isCoarse.value) {
    togglePlay()
    return
  }

  const now = performance.now()
  const isDouble = now - lastTapAt < DOUBLE_TAP_MS && Math.abs(e.clientX - lastTapX) < 64
  lastTapAt = now
  lastTapX = e.clientX

  if (isDouble) {
    if (singleTapTimer) {
      clearTimeout(singleTapTimer)
      singleTapTimer = null
    }
    lastTapAt = 0
    const rect = rootRef.value?.getBoundingClientRect()
    const x = rect && rect.width > 0 ? (e.clientX - rect.left) / rect.width : 0.5
    if (x < 0.4) {
      onSeekRel(-10)
      flashSeek('back')
    } else if (x > 0.6) {
      onSeekRel(10)
      flashSeek('fwd')
    } else {
      togglePlay()
    }
    return
  }

  singleTapTimer = setTimeout(() => {
    singleTapTimer = null
    if (uiVisible.value && state.playing.value) {
      clearUiIdleTimer()
      uiVisible.value = false
    } else {
      wakeUi()
    }
  }, DOUBLE_TAP_MS)
}
```

Root touch-wake gate — change the root element's `@touchstart.passive="wakeUi"` to `@touchstart.passive="onRootTouch"` and add:

```ts
// On touch the video tap handler owns chrome toggling — pre-waking from the
// root touchstart would make every tap see "chrome already visible" and
// immediately hide it again (toggle would never show the chrome).
function onRootTouch(e: TouchEvent) {
  if (isCoarse.value && e.target === videoRef.value) return
  wakeUi()
}
```

- [ ] **Step 2: Center pause + seek flash overlays (template)**

Add `Pause` to the lucide import in AePlayer. After the `<BigPlayButton …/>` element insert:

```html
    <!-- Mobile play/pause affordance while playing (tap = chrome toggle on touch) -->
    <button
      v-if="isCoarse && state.playing.value && uiVisible && hasStarted && !sourceError"
      class="pl-center-pause"
      :aria-label="$t('player.aePlayer.pause')"
      data-test="center-pause"
      @click.stop="togglePlay"
    >
      <Pause :size="30" aria-hidden="true" />
    </button>

    <!-- Double-tap seek indicator -->
    <div
      v-if="seekFlash"
      class="pl-seekflash"
      :class="seekFlash === 'back' ? 'pl-seekflash--back' : 'pl-seekflash--fwd'"
      aria-hidden="true"
    >
      {{ seekFlash === 'back' ? '−10s' : '+10s' }}
    </div>
```

- [ ] **Step 3: CSS** (append to scoped styles):

```css
/* Mobile center pause — the touch play/pause affordance while playing. */
.pl-center-pause {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  z-index: 5;
  width: 64px;
  height: 64px;
  border-radius: 50%;
  border: 1px solid var(--white-a30);
  background: var(--scrim-bg-strong);
  color: #fff;
  display: grid;
  place-items: center;
  cursor: pointer;
}

/* Double-tap ±10s flash */
.pl-seekflash {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  z-index: 5;
  padding: 10px 16px;
  border-radius: 999px;
  background: var(--scrim-bg-strong);
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  pointer-events: none;
  animation: pl-pop 0.18s ease;
}

.pl-seekflash--back {
  left: 12%;
}

.pl-seekflash--fwd {
  right: 12%;
}
```

- [ ] **Step 4: Verify**

Run: `bunx vitest run src/components/player/aePlayer/`
Expected: PASS (desktop click path unchanged — `isCoarse` is false in jsdom, so existing specs exercise the old behavior).

- [ ] **Step 5: Lint + commit**

```bash
bunx eslint src/components/player/aePlayer/AePlayer.vue
git add src/components/player/aePlayer/AePlayer.vue
git commit -m "feat(player): touch gestures — tap toggles chrome, double-tap ±10s, center pause overlay" -- src/components/player/aePlayer/AePlayer.vue
```

---

### Task 6: Bottom-sheet menus on mobile (Teleport + scrim)

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`

**Interfaces:**
- Consumes: `isMobile`, `nativeFsActive` (Task 4).
- Produces: computed `sheetTeleport: ComputedRef<boolean>` (= `isMobile && !nativeFsActive` — body-teleported nodes would be invisible under a native-fullscreen element, so sheets stay in-player there, where `position: fixed` resolves against the fullscreen viewport anyway) and `closeAllSheets()`. Task 8 reuses `sheetTeleport` for the DownloadDialog `sheet` prop.

- [ ] **Step 1: Script additions**

```ts
// Mobile sheets: teleport the floating menus to <body> and present them as
// bottom sheets. Disabled inside NATIVE fullscreen — body children render
// under the fullscreen element — where fixed positioning already fills the
// fullscreen viewport correctly in place.
const sheetTeleport = computed(() => isMobile.value && !nativeFsActive.value)
const anySheetOpen = computed(() => openMenu.value !== null || browseOpen.value || downloadDialogEp.value !== null)

function closeAllSheets() {
  closeMenus()
  downloadDialogEp.value = null
}
```

- [ ] **Step 2: Template — wrap each floating surface**

Wrap the four menu wrapper divs, keeping their `ref`s and classes and adding the sheet class + a `--prov` carry (the provider hue var lives on `.pl` and does not cascade to body):

```html
    <!-- Source panel (floating card on desktop, bottom sheet on mobile) -->
    <Teleport to="body" :disabled="!sheetTeleport">
      <div
        v-if="openMenu === 'source'"
        ref="sourceMenuEl"
        class="pl-floating pl-floating--source"
        :class="{ 'pl-floating--mobile-sheet': sheetTeleport }"
        :style="{ '--prov': activeProviderHue }"
        @click.stop
      >
        <SourcePanel … (unchanged props/handlers) … />
      </div>
    </Teleport>
```

Apply the identical `<Teleport to="body" :disabled="!sheetTeleport">` + `:class="{ 'pl-floating--mobile-sheet': sheetTeleport }"` + `:style="{ '--prov': activeProviderHue }"` treatment to the `episodes` (`pl-floating--sheet`), `settings` and `subs` (`pl-floating--btnmenu`) wrappers.

BrowseSubsModal (root is `absolute inset-0` — needs a fixed host when teleported):

```html
    <Teleport to="body" :disabled="!sheetTeleport">
      <div v-if="browseOpen" :class="sheetTeleport ? 'pl-bsm-host' : 'contents'">
        <BrowseSubsModal … (unchanged props/handlers) … />
      </div>
    </Teleport>
```

DownloadDialog — Teleport only (v2 styling lands in Task 8):

```html
    <Teleport to="body" :disabled="!sheetTeleport">
      <DownloadDialog
        v-if="downloadDialogEp"
        @confirm="onConfirmDownload"
        @close="downloadDialogEp = null"
      />
    </Teleport>
```

Scrim (add once, after the menus):

```html
    <!-- Mobile sheet scrim — tap closes whatever sheet is open -->
    <Teleport to="body" :disabled="!sheetTeleport">
      <div
        v-if="sheetTeleport && anySheetOpen"
        class="pl-sheet-scrim"
        data-test="sheet-scrim"
        @click="closeAllSheets"
      />
    </Teleport>
```

- [ ] **Step 3: CSS**

In the existing `@media (max-width: 680px)` block at the bottom of AePlayer's styles, DELETE the now-dead `.pl-floating--source { width: calc(100% - 28px); }` rule. Append (outside any media query — the class is only ever bound on mobile):

```css
/* ── Mobile bottom sheets ── */
.pl-sheet-scrim {
  position: fixed;
  inset: 0;
  z-index: 105; /* above pseudo-FS (100), below the sheet (110) */
  background: var(--black-a60);
}

/* Double class beats every .pl-floating--* placement rule regardless of
   source order (scoped attribute + two classes). */
.pl-floating.pl-floating--mobile-sheet {
  position: fixed;
  top: auto;
  left: 0;
  right: 0;
  bottom: 0;
  width: auto;
  max-width: none;
  max-height: 72dvh;
  border-radius: 16px 16px 0 0;
  z-index: 110;
  padding-bottom: env(safe-area-inset-bottom);
  animation: pl-sheet-up 0.22s ease;
}

@keyframes pl-sheet-up {
  from {
    transform: translateY(24px);
    opacity: 0.6;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}

/* Fixed host for the browse-subs modal when teleported (its root is
   absolute inset-0 and needs a viewport-sized positioned ancestor). */
.pl-bsm-host {
  position: fixed;
  inset: 0;
  z-index: 110;
}
```

- [ ] **Step 4: Verify**

Run: `bunx vitest run src/components/player/aePlayer/`
Expected: PASS — with `isMobile` false in jsdom every Teleport is `:disabled`, so existing menu specs see the same DOM as before.

- [ ] **Step 5: Lint + commit**

```bash
bunx eslint src/components/player/aePlayer/AePlayer.vue
git add src/components/player/aePlayer/AePlayer.vue
git commit -m "feat(player): mobile bottom-sheet menus — body teleport + scrim, native-FS aware" -- src/components/player/aePlayer/AePlayer.vue
```

---

### Task 7: Root restructure + mobile action row + top-bar trim + full-bleed CSS

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`

**Interfaces:**
- Consumes: `isMobile` (Task 4), `toggleMenu`, `onDownloadEpisode`, `canDownload`, `offline` prop, `selectedEpisode`, `episodes`, `activeProviderName`, `activeProviderHue`. i18n keys already exist: `player.aePlayer.epAbbrev`, `player.aePlayer.source`, `player.aePlayer.offline.download`.
- Produces: new root `.pl-wrap` carrying `data-test="ae-player"`; the video box `.pl` keeps `ref="rootRef"` and ALL behavior (fullscreen target, hotkeys, theater). `onDownloadCurrent()` used by the action row (Task 8 keeps it working with the scope dialog).

- [ ] **Step 1: Template restructure**

Change the outermost element from the `.pl` div to a wrapper; the `.pl` div keeps every existing attribute EXCEPT `data-test="ae-player"` (moves to the wrapper):

```html
<template>
  <div class="pl-wrap" data-test="ae-player">
    <div
      ref="rootRef"
      class="pl"
      :class="{ 'pl--theater': theater, 'pl--ui-hidden': !uiVisible, 'pl--pseudo-fs': pseudoFs }"
      :style="{ '--prov': activeProviderHue }"
      tabindex="0"
      role="region"
      :aria-label="$t('player.aePlayer.rootAria')"
      @click.self="closeMenus"
      @mouseenter="onPointerEnter"
      @mouseleave="onPointerLeave"
      @mousemove="wakeUi"
      @touchstart.passive="onRootTouch"
    >
      <!-- …ALL existing content stays inside, unchanged… -->
    </div>

    <!-- Mobile action row (D6) — big under-player entries. Lives inside the
         player component so every mount point (anime page, /downloads) gets
         it with zero cross-component wiring. -->
    <div v-if="isMobile" class="pl-actions" data-test="pl-actions">
      <Button variant="soft" size="sm" class="pl-action" data-test="action-episodes" @click="toggleMenu('episodes')">
        <ListVideo class="size-4" aria-hidden="true" />
        {{ $t('player.aePlayer.epAbbrev') }} {{ selectedEpisode?.number ?? initialEpisode ?? 1 }}
        <ChevronDown class="size-3.5 opacity-70" aria-hidden="true" />
      </Button>
      <Button v-if="!offline" variant="soft" size="sm" class="pl-action pl-action--src" data-test="action-source" @click="toggleMenu('source')">
        <span class="pl-prov-dot" :style="{ background: activeProviderHue }" aria-hidden="true" />
        <span class="pl-action-srcname">{{ activeProviderName || $t('player.aePlayer.source') }}</span>
        <ChevronDown class="size-3.5 opacity-70" aria-hidden="true" />
      </Button>
      <Button v-if="!offline && canDownload" variant="soft" size="sm" class="pl-action" data-test="action-download" @click="onDownloadCurrent">
        <Download class="size-4" aria-hidden="true" />
        {{ $t('player.aePlayer.offline.download') }}
      </Button>
    </div>
  </div>
</template>
```

Script: add `ListVideo`, `Download` to the lucide import; add:

```ts
function onDownloadCurrent() {
  const ep = selectedEpisode.value ?? episodes.value.find((e) => e.number === (props.initialEpisode ?? 1)) ?? episodes.value[0]
  if (ep) onDownloadEpisode(ep)
}
```

(Check the actual prop name for the initial episode in this file — it is `initialEpisode` — and that `Button` is already imported from `@/components/ui`; it is.)

- [ ] **Step 2: CSS — action row + full-bleed + top-bar trim**

Append near the end of the scoped styles:

```css
/* ── Under-player mobile action row ── */
.pl-actions {
  display: flex;
  align-items: stretch;
  gap: 8px;
  padding: 10px 16px 0;
}

.pl-actions .pl-action {
  flex: 1;
  min-height: 44px;
  justify-content: center;
}

.pl-action--src {
  min-width: 0;
}

.pl-action-srcname {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
```

Extend the `@media (max-width: 680px)` block:

```css
@media (max-width: 680px) {
  /* Full-bleed: Anime.vue drops the card gutter (Task 9); square the box. */
  .pl {
    border-radius: 0;
    border-left: 0;
    border-right: 0;
  }

  /* Top bar trim: smaller title, no per-episode subtitle text, tighter pad. */
  .pl-top {
    padding: 10px 12px 28px;
    gap: 8px;
  }

  .pl-title {
    font-size: 15px;
  }

  .pl-ep-title {
    display: none;
  }

  .pl-resume {
    left: 12px;
    bottom: 76px;
  }
}
```

- [ ] **Step 3: Verify** — the existing AePlayer specs must still pass (they query descendant classes like `.pl-airing-status`, not the root; `data-test="ae-player"` has no test consumers — verified during planning):

Run: `bunx vitest run src/components/player/aePlayer/ src/views/`
Expected: PASS. If a spec fails on root-element assumptions, fix the SPEC's selector (`.pl` is now the first child), not the template.

- [ ] **Step 4: Lint + commit**

```bash
bunx eslint src/components/player/aePlayer/AePlayer.vue
git add src/components/player/aePlayer/AePlayer.vue
git commit -m "feat(player): mobile action row under the video + full-bleed box + top-bar trim" -- src/components/player/aePlayer/AePlayer.vue
```

---

### Task 8: DownloadDialog v2 (scope + quota) + season wiring + episodes-panel chip

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/DownloadDialog.vue`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`
- Modify: `frontend/web/src/components/player/aePlayer/EpisodesPanel.vue`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`
- Test: create `frontend/web/src/components/player/aePlayer/DownloadDialog.spec.ts`, extend `EpisodesPanel.spec.ts`

**Interfaces:**
- Consumes: `seasonTargets`/`enqueueSeason`/`SeasonContext` (Task 2), `sheetTeleport` (Task 6), `PROJECTED_BYTES`/`storageEstimate` from `@/offline/downloadEngine`.
- Produces:
  - `DownloadDialog` props `{ episodeNumber: number; seasonCount: number; sheet?: boolean; initialScope?: 'episode' | 'season' }`, emit `confirm(quality: string, scope: 'episode' | 'season')`.
  - `EpisodesPanel` new emit `(e: 'download-season'): void`.
  - AePlayer: `onDownloadSeason()`, `seasonCount` computed, `onConfirmDownload(quality, scope)`.

- [ ] **Step 1: Write the failing DownloadDialog spec**

```ts
// frontend/web/src/components/player/aePlayer/DownloadDialog.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('@/offline/downloadEngine', () => ({
  PROJECTED_BYTES: { '480': 250 * 2 ** 20, '720': 450 * 2 ** 20, '1080': 900 * 2 ** 20 },
  storageEstimate: vi.fn(async () => ({ usage: 0, quota: 10 * 2 ** 30 })),
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

import DownloadDialog from './DownloadDialog.vue'

const globalStubs = {
  global: { mocks: { $t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k) } },
}

function mountDlg(props: Partial<{ episodeNumber: number; seasonCount: number; sheet: boolean; initialScope: 'episode' | 'season' }> = {}) {
  return mount(DownloadDialog, { props: { episodeNumber: 4, seasonCount: 10, ...props }, ...globalStubs })
}

describe('DownloadDialog v2', () => {
  beforeEach(() => localStorage.clear())

  it('renders both scopes and defaults to episode', () => {
    const w = mountDlg()
    expect(w.find('[data-test="scope-episode"]').attributes('aria-checked')).toBe('true')
    expect(w.find('[data-test="scope-season"]').attributes('aria-checked')).toBe('false')
  })

  it('emits confirm with quality AND scope', async () => {
    const w = mountDlg()
    await w.find('[data-test="scope-season"]').trigger('click')
    await w.find('[data-test="dl-start"]').trigger('click')
    const evt = w.emitted('confirm')![0]
    expect(evt[0]).toBe('720')
    expect(evt[1]).toBe('season')
  })

  it('disables season scope when nothing left to download', () => {
    const w = mountDlg({ seasonCount: 0 })
    expect(w.find('[data-test="scope-season"]').attributes('disabled')).toBeDefined()
  })

  it('preselects season via initialScope', () => {
    const w = mountDlg({ initialScope: 'season' })
    expect(w.find('[data-test="scope-season"]').attributes('aria-checked')).toBe('true')
  })

  it('warns when the projection exceeds free space', async () => {
    const { storageEstimate } = await import('@/offline/downloadEngine')
    ;(storageEstimate as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ usage: 0, quota: 1 * 2 ** 30 })
    const w = mountDlg({ seasonCount: 20, initialScope: 'season' })
    await new Promise((r) => setTimeout(r))
    await w.vm.$nextTick()
    expect(w.find('[data-test="low-space"]').exists()).toBe(true)
  })

  it('applies sheet presentation class', () => {
    const w = mountDlg({ sheet: true })
    expect(w.find('.dl-dialog').classes()).toContain('dl-dialog--sheet')
  })
})
```

- [ ] **Step 2: Run to verify it fails** — `bunx vitest run src/components/player/aePlayer/DownloadDialog.spec.ts` → FAIL (missing props/markup).

- [ ] **Step 3: Rewrite DownloadDialog.vue**

```vue
<template>
  <div class="dl-dialog" :class="{ 'dl-dialog--sheet': sheet }" role="dialog" :aria-label="$t('player.aePlayer.offline.quality')">
    <div class="dl-title text-sm font-semibold">{{ $t('player.aePlayer.offline.quality') }}</div>
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

    <div class="dl-scopes" role="radiogroup" :aria-label="$t('player.aePlayer.offline.scope')">
      <button
        type="button"
        class="dl-scope"
        :class="{ 'dl-scope-active': scope === 'episode' }"
        role="radio"
        :aria-checked="scope === 'episode' ? 'true' : 'false'"
        data-test="scope-episode"
        @click="scope = 'episode'"
      >
        <span>{{ $t('player.aePlayer.offline.scopeEpisode', { n: episodeNumber }) }}</span>
        <span class="dl-scope-est">~{{ SIZE_HINT[quality] }}</span>
      </button>
      <button
        type="button"
        class="dl-scope"
        :class="{ 'dl-scope-active': scope === 'season' }"
        :disabled="seasonCount === 0"
        role="radio"
        :aria-checked="scope === 'season' ? 'true' : 'false'"
        data-test="scope-season"
        @click="seasonCount > 0 && (scope = 'season')"
      >
        <span>{{ seasonCount > 0 ? $t('player.aePlayer.offline.scopeSeason', { n: seasonCount }) : $t('player.aePlayer.offline.seasonDone') }}</span>
        <span v-if="seasonCount > 0" class="dl-scope-est">~{{ seasonEstimate }}</span>
      </button>
    </div>

    <div v-if="lowSpace" class="dl-warn text-warning" data-test="low-space">
      {{ $t('player.aePlayer.offline.lowSpace', { free: freeLabel }) }}
    </div>

    <div class="dl-actions">
      <button type="button" class="dl-btn dl-btn-primary font-medium" data-test="dl-start" @click="confirm">
        {{ $t('player.aePlayer.offline.start') }}
      </button>
      <button type="button" class="dl-btn font-medium" @click="emit('close')">
        {{ $t('player.aePlayer.offline.cancel') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { PROJECTED_BYTES, storageEstimate } from '@/offline/downloadEngine'

const QUALITIES = ['480', '720', '1080'] as const
const SIZE_HINT: Record<string, string> = { '480': '250 MB', '720': '450 MB', '1080': '900 MB' }
const LS_KEY = 'ae.downloadQuality'

const props = withDefaults(
  defineProps<{
    episodeNumber: number
    /** Episodes a season download would enqueue (pre-filtered). 0 = complete. */
    seasonCount: number
    /** Bottom-sheet presentation (mobile). */
    sheet?: boolean
    initialScope?: 'episode' | 'season'
  }>(),
  { sheet: false, initialScope: 'episode' },
)

const emit = defineEmits<{
  (e: 'confirm', quality: string, scope: 'episode' | 'season'): void
  (e: 'close'): void
}>()

const saved = localStorage.getItem(LS_KEY)
const quality = ref<string>(saved && (QUALITIES as readonly string[]).includes(saved) ? saved : '720')
const scope = ref<'episode' | 'season'>(props.initialScope === 'season' && props.seasonCount > 0 ? 'season' : 'episode')

const free = ref<number | null>(null)
onMounted(() => {
  void storageEstimate().then((est) => {
    if (est) free.value = est.quota - est.usage
  })
})

function fmtGb(bytes: number): string {
  return `${(bytes / 2 ** 30).toFixed(1)} GB`
}

const perEpisode = computed(() => PROJECTED_BYTES[quality.value] ?? PROJECTED_BYTES['720'])
const projected = computed(() => perEpisode.value * (scope.value === 'season' ? props.seasonCount : 1))
const seasonEstimate = computed(() => fmtGb(perEpisode.value * props.seasonCount))
const lowSpace = computed(() => free.value !== null && projected.value > free.value)
const freeLabel = computed(() => (free.value === null ? '' : fmtGb(free.value)))

function confirm() {
  localStorage.setItem(LS_KEY, quality.value)
  emit('confirm', quality.value, scope.value)
}
</script>

<style scoped>
.dl-dialog {
  position: absolute;
  inset-inline: 0;
  bottom: 4rem;
  margin-inline: auto;
  width: 18rem;
  padding: 0.75rem;
  border-radius: 0.5rem;
  background: var(--scrim-bg-strong);
  border: 1px solid var(--white-a8);
  z-index: 30;
}
.dl-dialog--sheet {
  position: fixed;
  inset-inline: 0;
  top: auto;
  bottom: 0;
  width: auto;
  margin-inline: 0;
  border-radius: 16px 16px 0 0;
  padding: 1rem 1rem calc(1rem + env(safe-area-inset-bottom));
  z-index: 110;
}
.dl-title { margin-bottom: 0.5rem; }
.dl-opts { display: flex; gap: 0.5rem; margin-bottom: 0.75rem; }
.dl-opt {
  flex: 1;
  padding: 0.375rem 0;
  border-radius: 0.375rem;
  border: 1px solid var(--white-a8);
  color: var(--color-muted-foreground, currentColor);
}
.dl-opt-active { border-color: var(--brand-cyan); color: var(--brand-cyan); }
.dl-scopes { display: flex; flex-direction: column; gap: 0.375rem; margin-bottom: 0.75rem; }
.dl-scope {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.625rem;
  border-radius: 0.375rem;
  border: 1px solid var(--white-a8);
  color: var(--muted-foreground);
  background: transparent;
  cursor: pointer;
  font-size: 0.8125rem;
  text-align: left;
}
.dl-scope:disabled { opacity: 0.5; cursor: default; }
.dl-scope-active { border-color: var(--brand-cyan); color: #fff; }
.dl-scope-est { color: var(--muted-foreground); font-size: 0.75rem; flex-shrink: 0; }
.dl-warn { font-size: 0.75rem; margin-bottom: 0.5rem; }
.dl-actions { display: flex; gap: 0.5rem; }
.dl-btn { flex: 1; padding: 0.375rem 0; border-radius: 0.375rem; border: 1px solid var(--white-a8); }
.dl-btn-primary { border-color: var(--brand-cyan); color: var(--brand-cyan); }
</style>
```

(Note: the old `offline.estimate` i18n usage is gone — delete that key from all three locales in Step 6. `#fff` on `.dl-scope-active` matches the file's pre-existing pattern of literal white in player CSS — allowed, only hex OUTSIDE the allowlist is flagged; `#fff` is used across the player already. If DS-lint flags it, use `color: var(--foreground)` instead.)

- [ ] **Step 4: AePlayer wiring**

Script:

```ts
import { seasonTargets, enqueueSeason } from '@/offline/seasonDownload'
```

```ts
const downloadScope = ref<'episode' | 'season'>('episode')
const seasonCount = computed(() => seasonTargets(episodes.value, downloadStates.value).length)

function onDownloadSeason() {
  const ep = selectedEpisode.value ?? episodes.value[0]
  if (!ep) return
  downloadScope.value = 'season'
  downloadDialogEp.value = ep
}
```

Change `onDownloadEpisode` to reset the scope:

```ts
function onDownloadEpisode(ep: EpisodeOption) {
  downloadScope.value = 'episode'
  downloadDialogEp.value = ep
}
```

Replace `onConfirmDownload` with:

```ts
async function onConfirmDownload(quality: string, scope: 'episode' | 'season') {
  const ep = downloadDialogEp.value
  downloadDialogEp.value = null
  if (!ep) return
  const comboSnapshot = { ...state.combo.value } // freeze — user may switch sources mid-download
  if (scope === 'season') {
    const targets = seasonTargets(episodes.value, downloadStates.value)
    await enqueueSeason(targets, {
      animeId: props.animeId,
      animeTitle: props.anime.title,
      poster: props.anime.still,
      combo: comboSnapshot,
      quality,
      resolveFor: (target) => () => resolver.resolveStream(comboSnapshot.provider, props.animeId, target, comboSnapshot),
    })
  } else {
    await enqueueDownload({
      animeId: props.animeId,
      animeTitle: props.anime.title,
      poster: props.anime.still,
      episode: ep,
      combo: comboSnapshot,
      quality,
      resolve: () => resolver.resolveStream(comboSnapshot.provider, props.animeId, ep, comboSnapshot),
    })
  }
  void refreshDownloadStates()
}
```

Template — extend the (already Teleport-wrapped, Task 6) dialog:

```html
      <DownloadDialog
        v-if="downloadDialogEp"
        :episode-number="downloadDialogEp.number"
        :season-count="seasonCount"
        :sheet="sheetTeleport"
        :initial-scope="downloadScope"
        @confirm="onConfirmDownload"
        @close="downloadDialogEp = null"
      />
```

EpisodesPanel binding — add `@download-season="onDownloadSeason"` to the `<EpisodesPanel>` element.

- [ ] **Step 5: EpisodesPanel — season chip + touch hit areas**

Template: in the sheet-head row, right after `<span class="flex-1" />`, insert:

```html
      <button
        v-if="downloadable && episodes.length > 1"
        type="button"
        class="ep-chip ep-chip-season"
        data-test="season-download"
        :title="$t('player.aePlayer.offline.scopeSeasonTitle')"
        @click="emit('download-season')"
      >
        <Download :size="11" aria-hidden="true" />
        {{ $t('player.aePlayer.offline.season') }}
      </button>
```

Emits type gains `(e: 'download-season'): void`. (`Download` is already imported in this file.)

Styles — append:

```css
.ep-chip-season {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

/* Touch: grow the per-card download affordance to a real hit target
   without shifting layout (negative margins reclaim the padding). */
@media (pointer: coarse) {
  .ep-dl {
    padding: 12px;
    margin: -10px -10px -10px auto;
  }

  .ep-chip {
    min-height: 36px;
  }
}
```

Extend `EpisodesPanel.spec.ts` (follow its existing mount pattern):

```ts
it('emits download-season from the header chip when downloadable', async () => {
  const wrapper = mountPanel({ downloadable: true }) // existing helper + ≥2 episodes
  await wrapper.find('[data-test="season-download"]').trigger('click')
  expect(wrapper.emitted('download-season')).toHaveLength(1)
})

it('hides the season chip when not downloadable', () => {
  const wrapper = mountPanel({ downloadable: false })
  expect(wrapper.find('[data-test="season-download"]').exists()).toBe(false)
})
```

- [ ] **Step 6: i18n — add to `player.aePlayer.offline` in ALL THREE locale files** (and DELETE the now-unused `estimate` key from all three):

`en.json`:
```json
"scope": "What to download",
"scopeEpisode": "This episode ({n})",
"scopeSeason": "Whole season — {n} ep.",
"scopeSeasonTitle": "Download the whole season",
"seasonDone": "Season already downloaded",
"season": "Season",
"lowSpace": "May run out of space (~{free} free)"
```

`ru.json`:
```json
"scope": "Что скачать",
"scopeEpisode": "Эта серия ({n})",
"scopeSeason": "Весь сезон — {n} сер.",
"scopeSeasonTitle": "Скачать весь сезон",
"seasonDone": "Весь сезон уже скачан",
"season": "Сезон",
"lowSpace": "Может не хватить места (свободно ~{free})"
```

`ja.json`:
```json
"scope": "ダウンロード対象",
"scopeEpisode": "この話（{n}）",
"scopeSeason": "全シーズン — {n}話",
"scopeSeasonTitle": "シーズン全体をダウンロード",
"seasonDone": "全話ダウンロード済み",
"season": "シーズン",
"lowSpace": "容量が不足する可能性があります（空き ~{free}）"
```

- [ ] **Step 7: Run all touched tests**

Run: `bunx vitest run src/components/player/aePlayer/ src/offline/ src/locales/__tests__`
Expected: PASS, including locale-parity specs.

- [ ] **Step 8: Lint + commit**

```bash
bunx eslint src/components/player/aePlayer/DownloadDialog.vue src/components/player/aePlayer/AePlayer.vue src/components/player/aePlayer/EpisodesPanel.vue
git add -A src/components/player/aePlayer/DownloadDialog.vue src/components/player/aePlayer/DownloadDialog.spec.ts src/components/player/aePlayer/AePlayer.vue src/components/player/aePlayer/EpisodesPanel.vue src/components/player/aePlayer/EpisodesPanel.spec.ts src/locales/en.json src/locales/ru.json src/locales/ja.json
git commit -m "feat(offline): download whole season — scope dialog, quota hint, episodes-panel chip" -- src/components/player/aePlayer/ src/locales/
```

---

### Task 9: Full-bleed player card on the anime page

**Files:**
- Modify: `frontend/web/src/views/Anime.vue`

**Interfaces:**
- Consumes: nothing new. Produces: mobile-only `.player-card` override class (scoped) applied to the two player card wrappers.

**Cascade note (why not Tailwind utilities):** `.glass-card` is authored UNLAYERED in `src/styles/main.css` and beats layered utilities like `rounded-none`/`border-x-0` (Tailwind v4 cascade trap). A SCOPED class compiles to `.player-card[data-v-…]` — class+attribute specificity — and wins over `.glass-card` deterministically.

- [ ] **Step 1: Apply the class**

Line ~404, the AePlayer card:
```html
        <div class="glass-card player-card p-4 md:p-6" v-if="!classicKodik && aePlayerEnabled">
```
Line ~436, the Classic-Kodik card:
```html
        <div class="glass-card player-card p-4 md:p-6" v-else>
```

- [ ] **Step 2: Scoped CSS** — append to Anime.vue's scoped style block:

```css
/* Full-bleed player on phones: kill the card gutter and the page padding
   (px-4 on the page container) so the video spans the full viewport width.
   Scoped selector (.player-card[data-v]) outranks the unlayered .glass-card. */
@media (max-width: 680px) {
  .player-card {
    margin-inline: -1rem;
    padding: 0;
    border-radius: 0;
    border-left: 0;
    border-right: 0;
    background: transparent;
    backdrop-filter: none;
  }

  .player-card > .aspect-video {
    border-radius: 0;
  }
}
```

- [ ] **Step 3: Verify**

Run: `bunx vitest run src/views/` — PASS.
Then `bun run build` (first full type-check of the branch) — MUST pass; fix any vue-tsc errors surfaced across Tasks 4–8 now.

- [ ] **Step 4: Lint + commit**

```bash
bunx eslint src/views/Anime.vue
git add src/views/Anime.vue
git commit -m "feat(web): full-bleed mobile player card on the anime page" -- src/views/Anime.vue
```

---

### Task 10: Full gate sweep

**Files:** none new — verification only (fixes allowed anywhere touched above).

- [ ] **Step 1: DS lint** — `bash scripts/design-system-lint.sh` (from `frontend/web/`) → 0 errors. If a token rule fires on new CSS, migrate to the token (never allowlist without trying tokens first).
- [ ] **Step 2: i18n** — `bash scripts/i18n-lint.sh` (retry once if flaky) + `bunx vitest run src/locales/__tests__` → parity green.
- [ ] **Step 3: eslint** — `bun lint` → 0 errors.
- [ ] **Step 4: Types + build** — `bun run build` → clean.
- [ ] **Step 5: Full player/offline/views test sweep** — `bunx vitest run src/components/player/ src/offline/ src/views/ src/composables/aePlayer/` → all green.
- [ ] **Step 6: Commit any gate fixes**

```bash
git add -A frontend/web
git commit -m "chore(web): gate fixes for mobile player redesign" -- frontend/web
```
(Skip if nothing changed.)

---

## Self-review notes (done at plan time)

- Spec coverage: D1→T9+T7 CSS, D2→T3+T4, D3→T3, D4→T5, D5→T6, D6→T7, D7→T2+T8, D8→T7. ✔
- Type consistency: `fullscreenActive` (T3 prop = T4 computed), `sheetTeleport` (T6 → T8), `seasonTargets`/`enqueueSeason`/`SeasonContext` (T2 → T8), `onRootTouch` (T5 → referenced by T7's template snapshot — T7 runs after T5, snapshot already reflects it). ✔
- Known risk: Tasks 4–8 all edit `AePlayer.vue` — execute strictly in order, one agent at a time.
- Manual smoke (owner, phone): full-bleed; fullscreen Android (native+landscape) & iPhone (pseudo-FS, back-gesture exit); sheets open full-screen; double-tap seek; action row; season download on a real title; deploy-reload deferral still holds during a season batch.
