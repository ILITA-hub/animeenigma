# Theater Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give aePlayer a real middle ground between windowed and fullscreen — full page width, navbar and page content intact, height capped so the control bar never falls below the fold.

**Architecture:** Theater is already fully built but unreachable — CSS, `useTheaterMode.ts` (localStorage + Esc), the `toggle-theater` emit declaration, the `Anime.vue` wiring, and en/ru/ja copy all exist. The button was hidden 2026-06-09 (`94530095`) and a spec test locks it out. We add the missing trigger, fix the cap, and stop hiding the navbar/content. No new state, no backend.

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Tailwind v4, lucide-vue-next, vitest + @vue/test-utils, bun.

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-07-15-theater-mode-design.md`.
- **Cap:** `max-height: calc(100vh - var(--header-offset))`. `--header-offset` (`5rem`) is the SINGLE source of truth for navbar clearance (`main.css:507`) — never hardcode 80px in CSS.
- **i18n keys already exist on en/ru/ja** at `player.theaterModeEnter` / `player.theaterModeExit` — note the path is `player.*`, **NOT** `player.aePlayer.*`. Do not add locale keys.
- **Do not modify:** `useTheaterMode.ts`, any `locales/*.json`, `views/TipsPage.vue`, `WatchTogetherView.vue`, `DownloadsPage.vue`.
- **lucide icons are NAMED imports** (`import { MonitorPlay } from 'lucide-vue-next'`).
- **Never disable the DS lint gate.** Brand hues (cyan/pink/orange/rose/…) are exempt, not violations.
- **Worktree:** `/data/animeenigma/.claude/worktrees/theater-mode`. Build absolute paths under the worktree root — a `/data/animeenigma/...` path silently edits the BASE tree instead.
- Run all commands from `frontend/web/`.

---

### Task 1: Theater button in the control bar

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/PlayerControlBar.vue`
- Test: `frontend/web/src/components/player/aePlayer/PlayerControlBar.spec.ts:117-120`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: props `theaterActive?: boolean` (default `false`) and `canTheater?: boolean` (default `false`); emit `(e: 'toggle-theater'): void`; DOM hook `[data-test="toggle-theater"]`; CSS marker class `.pl-theater-btn`.

`canTheater` defaults to **false** on purpose. `WatchTogetherView.vue` binds `@toggle-theater="() => {}"` and `DownloadsPage.vue` does not listen at all — an always-on button would render dead there. Opt-in means a new mount site is safe by default.

- [ ] **Step 1: Replace the locked test with the new contract**

In `PlayerControlBar.spec.ts`, delete this test entirely:

```ts
  it('does not render a theater-mode button (hidden by request)', () => {
    const w = mount(PlayerControlBar, { props: baseProps })
    expect(w.find('[data-test="toggle-theater"]').exists()).toBe(false)
  })
```

Insert in its place:

```ts
  it('renders a theater button and emits toggle-theater when canTheater is set', async () => {
    const w = mount(PlayerControlBar, { props: { ...baseProps, canTheater: true } })
    const btn = w.find('[data-test="toggle-theater"]')
    expect(btn.exists()).toBe(true)
    expect(btn.attributes('aria-label')).toBe('Theater mode')
    await btn.trigger('click')
    expect(w.emitted('toggle-theater')).toHaveLength(1)
  })

  it('swaps the theater label to the exit copy when theater is active', () => {
    const w = mount(PlayerControlBar, {
      props: { ...baseProps, canTheater: true, theaterActive: true },
    })
    const btn = w.find('[data-test="toggle-theater"]')
    expect(btn.attributes('aria-label')).toBe('Exit theater mode')
    expect(btn.attributes('aria-pressed')).toBe('true')
  })

  it('renders no theater button by default — mount sites without a handler stay clean', () => {
    const w = mount(PlayerControlBar, { props: baseProps })
    expect(w.find('[data-test="toggle-theater"]').exists()).toBe(false)
  })
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/PlayerControlBar.spec.ts`
Expected: FAIL — the first two fail because `[data-test="toggle-theater"]` does not exist (`btn.exists()` is `false`, `attributes()` undefined). The third passes already.

- [ ] **Step 3: Add the icon import**

Modify the lucide import (line ~186) to add `MonitorPlay` — `MonitorPlay` is the icon `/tips` already uses for theater, so the two surfaces agree:

```ts
import { Play, Pause, Volume1, Volume2, VolumeX, ChevronDown, Captions, Settings, PictureInPicture2, Maximize, Minimize, ListVideo, MonitorPlay } from 'lucide-vue-next'
```

- [ ] **Step 4: Add the props**

In the `defineProps<{...}>()` block, after `fullscreenActive?: boolean`:

```ts
    /** fullscreen (native or pseudo) currently active — swaps the FS icon */
    fullscreenActive?: boolean
    /** theater mode currently active — swaps the button copy + pressed state */
    theaterActive?: boolean
    /** whether the HOST view actually implements theater (handler + page CSS).
     *  Opt-in: Watch Together binds a no-op and /downloads does not listen at
     *  all, so the button must not exist there. */
    canTheater?: boolean
```

Extend the defaults object (line ~227) — append to the existing literal:

```ts
  { progress: 0, buffered: 0, chapters: () => [], stillUrl: undefined, openMenu: null, fragments: () => [], previewUrl: null, previewType: null, previewStoryboardUrl: null, episodeLabel: '', fullscreenActive: false, theaterActive: false, canTheater: false },
```

- [ ] **Step 5: Add the emit**

In `defineEmits<{...}>()`, after `(e: 'toggle-pip'): void`:

```ts
  (e: 'toggle-pip'): void
  (e: 'toggle-theater'): void
  (e: 'toggle-fullscreen'): void
```

- [ ] **Step 6: Add the button to the template**

Insert between the PiP button and the Fullscreen button (after the PiP `</PlayerIconButton>` closing tag, line ~165):

```html
      <!-- Theater — the middle ground between windowed and fullscreen. Desktop
           only (see the media query below) and opt-in via canTheater, so mount
           sites without a handler never show a dead button. -->
      <PlayerIconButton
        v-if="canTheater"
        class="pl-theater-btn"
        :active="theaterActive"
        :aria-label="theaterActive ? $t('player.theaterModeExit') : $t('player.theaterModeEnter')"
        :aria-pressed="theaterActive"
        data-test="toggle-theater"
        @click="emit('toggle-theater')"
      >
        <MonitorPlay class="size-5" aria-hidden="true" />
      </PlayerIconButton>
```

- [ ] **Step 7: Hide it below 1024px**

Append a new media query after the existing mobile-trim block (which ends line ~445). Keep it separate — the trim block is `≤680px`, theater needs `≤1023px`, so they are different breakpoints and must not be merged:

```css
/* Theater is desktop-only. Below 1024px the player already spans the full
   column, so there is nothing to widen — and fullscreen is the comfortable
   watch surface on phones. Separate from the ≤680px trim on purpose. */
@media (max-width: 1023px) {
  .pl-theater-btn {
    display: none;
  }
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/PlayerControlBar.spec.ts`
Expected: PASS — all tests green, including the three theater tests.

- [ ] **Step 9: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/PlayerControlBar.vue frontend/web/src/components/player/aePlayer/PlayerControlBar.spec.ts
git commit -m "feat(player): theater button in the control bar, opt-in via canTheater

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: Wire AePlayer through and fix the cap

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (template ~line 273, props ~line 559, CSS ~line 1465)

**Interfaces:**
- Consumes: Task 1's `theater-active` / `can-theater` props and `toggle-theater` emit on `PlayerControlBar`.
- Produces: prop `canTheater?: boolean` on `AePlayer`; the already-declared `(e: 'toggle-theater'): void` emit now actually fires.

`AePlayer` already declares `theater: boolean` (line 559) and `(e: 'toggle-theater'): void` (line 592). The emit has **never been fired from anywhere** — that is the whole bug. Do not re-declare either.

- [ ] **Step 1: Add the canTheater prop**

In `defineProps<{...}>()` (starts line 556), immediately after `theater: boolean`:

```ts
  theater: boolean
  /** Whether the host view implements theater (a real @toggle-theater handler
   *  plus the page-level CSS). Forwarded to the control bar; false ⇒ no button.
   *  Only Anime.vue passes true. */
  canTheater?: boolean
```

- [ ] **Step 2: Forward props and the emit to the control bar**

In the `<PlayerControlBar>` usage, add the two bindings after `:fullscreen-active="fullscreenActive"` (line 273) and the handler after `@toggle-fullscreen="onToggleFullscreen"` (line 284):

```html
      :fullscreen-active="fullscreenActive"
      :theater-active="theater"
      :can-theater="canTheater"
      @toggle-play="togglePlay"
      @seek-rel="onSeekRel"
      @seek="onSeek"
      @set-volume="onSetVolume"
      @toggle-mute="onToggleMute"
      @toggle-source="toggleMenu('source')"
      @toggle-episodes="toggleMenu('episodes')"
      @toggle-subs="toggleMenu('subs')"
      @toggle-settings="toggleMenu('settings')"
      @toggle-pip="onTogglePip"
      @toggle-fullscreen="onToggleFullscreen"
      @toggle-theater="emit('toggle-theater')"
```

- [ ] **Step 3: Replace the fullscreen-clone cap**

Replace the whole `.pl--theater` rule (line ~1465):

```css
.pl--theater {
  border-radius: 0;
  border: 0;
  aspect-ratio: auto;
  height: 100vh;
}
```

with:

```css
/* Theater — full-bleed width, capped height. The base .pl aspect-ratio 16/9 is
   deliberately KEPT and merely clamped by max-height: on a wide monitor the box
   ends up wider than 16/9 and the video object-contains into it (side bars), the
   same shape YouTube's theater has. The old `height: 100vh` made this a second
   fullscreen — which is why the button was pulled in June.
   --header-offset is the navbar clearance token: subtracting it keeps the
   control bar (the player's bottom edge) on screen once the player is scrolled
   under the fixed navbar. Plain 100vh pushes it 80px below the fold. */
.pl--theater {
  border-radius: 0;
  border: 0;
  max-height: calc(100vh - var(--header-offset));
}
```

- [ ] **Step 4: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit -p tsconfig.json`
Expected: no errors mentioning `AePlayer.vue`, `canTheater`, or `theaterActive`.

- [ ] **Step 5: Run the player test suite for regressions**

Run: `cd frontend/web && bunx vitest run src/components/player/`
Expected: PASS — every existing player spec still green (`AePlayer.fullscreen.spec.ts` in particular must not regress).

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/AePlayer.vue
git commit -m "feat(player): fire toggle-theater and cap theater at 100vh minus navbar

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: Anime page — stop hiding the page, frame the player

**Files:**
- Modify: `frontend/web/src/views/Anime.vue` (header row ~line 393, AePlayer mount ~line 447, script ~line 895 + ~line 970, global style block ~line 1178)

**Interfaces:**
- Consumes: Task 2's `canTheater` prop on `AePlayer`.
- Produces: nothing downstream — this is the last task.

**Why the cap needs a scroll.** The player is NOT at the top of the page: the hero and `#section-overview` (the description) sit above it. So entering theater from an arbitrary scroll position leaves the player wherever it happens to be, and the capped height only frames perfectly when the player's top sits exactly under the fixed navbar. Entering theater therefore scrolls the section to that line, using `scroll-margin-top: var(--header-offset)` so the browser does the offset math from the same token.

- [ ] **Step 1: Tag the section header row**

At line ~393, add the `player-head` marker class (keep every existing utility):

```html
        <div class="player-head flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4">
```

- [ ] **Step 2: Import nextTick**

Change line ~895:

```ts
import { ref, watch, defineAsyncComponent } from 'vue'
```

to:

```ts
import { ref, watch, nextTick, defineAsyncComponent } from 'vue'
```

- [ ] **Step 3: Add the toggle handler**

After the `useTheaterMode()` destructure (line ~970):

```ts
// Phase 11 / UX-23 — Theater Mode (body class + ESC + localStorage persistence).
const { theaterMode, setTheater } = useTheaterMode()
```

append:

```ts
// The player sits below the hero + description, so entering theater must bring
// it up to the navbar line — that is the position the capped height is framed
// for. scroll-margin-top on the section (see the global style block) supplies
// the offset, so this stays free of hardcoded pixels.
async function onToggleTheater() {
  const on = !theaterMode.value
  setTheater(on)
  if (!on) return
  await nextTick()
  playerSectionRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
```

- [ ] **Step 4: Wire the handler and canTheater on the mount**

At the `<AePlayer>` mount, change line ~457 and add the prop next to `:theater`:

```html
            :theater="theaterMode"
            :can-theater="true"
```

```html
            @toggle-theater="onToggleTheater"
```

- [ ] **Step 5: Rewrite the theater style block**

Replace the whole global rule set (line ~1178 onward):

```css
body.theater-mode .navbar-root {
  display: none !important;
}
body.theater-mode .non-player-content {
  display: none;
}
body.theater-mode [data-anime-player-wrapper="true"] {
  max-width: none !important;
  margin-left: 0 !important;
  margin-right: 0 !important;
  padding-left: 0 !important;
  padding-right: 0 !important;
}
```

with:

```css
/* Theater = full-bleed player, page INTACT. The navbar and .non-player-content
   used to be display:none here, which made this a fullscreen clone with no
   reason to exist; both rules are deliberately gone. */
body.theater-mode [data-anime-player-wrapper="true"] {
  max-width: none !important;
  margin-left: 0 !important;
  margin-right: 0 !important;
  padding-left: 0 !important;
  padding-right: 0 !important;
  /* mt-8 would push the player below the navbar line the cap is framed for. */
  margin-top: 0 !important;
  /* Offset for scrollIntoView in onToggleTheater — same token as the cap. */
  scroll-margin-top: var(--header-offset);
}

/* The section's own title row + Classic-Kodik toggle step aside so the section
   top IS the player top; otherwise they eat into the capped height and push the
   control bar back under the fold. Leaving theater brings them straight back. */
body.theater-mode .player-head {
  display: none;
}

/* The glass card's padding and side border would frame a full-bleed player. */
body.theater-mode [data-anime-player-wrapper="true"] .player-card {
  padding: 0;
  border-left: 0;
  border-right: 0;
  border-radius: 0;
}
```

- [ ] **Step 6: Verify the page suite still passes**

Run: `cd frontend/web && bunx vitest run src/views/ src/components/player/`
Expected: PASS.

- [ ] **Step 7: Full frontend gate**

Run: `/frontend-verify`
Expected: DS-lint `ERRORS=0`, i18n en/ru/ja parity clean (we added no keys), real `bun run build` succeeds.

- [ ] **Step 8: Commit**

```bash
git add frontend/web/src/views/Anime.vue
git commit -m "feat(web): theater keeps the page, frames the player under the navbar

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Self-Review

**Spec coverage:**
- Cap `calc(100vh - var(--header-offset))` → Task 2 Step 3. ✔
- Stop hiding navbar + `.non-player-content` → Task 3 Step 5. ✔
- `MonitorPlay` button, `<1024px` hidden → Task 1 Steps 3/6/7. ✔
- `canTheater` default false, dead-button trap → Task 1 Steps 4/6, Task 2 Step 1, Task 3 Step 4. ✔
- Locked test inverted → Task 1 Step 1. ✔
- Untouched files (`useTheaterMode.ts`, locales, `/tips`, WT, Downloads) → not referenced by any task. ✔
- Gates → Task 3 Step 7. ✔

**Amendment vs the spec:** the spec assumed the capped player would land under the navbar on its own. It does not — the hero and description sit above it. Task 3 adds `scrollIntoView` + `scroll-margin-top` and hides `.player-head`, without which the chosen cap does not deliver "controls always visible". Fold this back into the spec after execution.

**Placeholder scan:** no TBD/TODO; every code step carries real code and a real command.

**Type consistency:** `theaterActive` / `canTheater` / `toggle-theater` / `.pl-theater-btn` / `[data-test="toggle-theater"]` / `player-head` are spelled identically in Tasks 1–3. i18n paths are `player.theaterModeEnter` / `player.theaterModeExit` throughout — verified against en/ru/ja, and NOT under `player.aePlayer.*`.
