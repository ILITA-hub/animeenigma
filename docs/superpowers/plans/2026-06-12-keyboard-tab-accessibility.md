# Keyboard / Tab-Order Accessibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every interactive element in frontend/web is Tab-reachable and activates with Enter/Space; a skip-link jumps past the navbar.

**Architecture:** Semantics-first sweep per spec `docs/superpowers/specs/2026-06-12-keyboard-tab-accessibility-design.md` — swap clickable `div`/`span` to native `<button>` where no interactive children are nested; otherwise `tabindex="0"` + `role` + keydown. No positive tabindex anywhere. The global `:focus-visible` ring in `styles/main.css:211` is NOT touched.

**Tech Stack:** Vue 3 SFC + TypeScript, Tailwind v4, vitest + @vue/test-utils, bun/bunx.

**Working dir:** all paths below are relative to `/data/animeenigma/frontend/web/src` unless prefixed. Repo is a SHARED tree — `git add` exact paths only, never `-A`. Push after every commit.

**Facts you'd otherwise have to discover:**
- `@keydown.enter.space.prevent` (OR-match) is the established codebase pattern — see `components/profile/GachaCollection.vue:70-73`.
- Vue key aliases `.left`/`.right` map to ArrowLeft/ArrowRight on keydown events.
- `MediaTile.vue` / `PosterCard.vue` are already `<router-link>` — RecsRow & card grids need NO changes (Enter on the inner link bubbles a click to the analytics wrapper).
- Tailwind v4 cascade footgun: unlayered rules in `main.css` beat utilities — overriding the global focus ring requires a scoped CSS rule, not a `focus-visible:shadow-none` utility.
- Run tests with `cd /data/animeenigma/frontend/web && bunx vitest run <path>`.

---

### Task 1: Skip-link in App.vue + i18n keys

**Files:**
- Modify: `App.vue` (template lines 2, 48; scoped style block)
- Modify: `locales/en.json`, `locales/ru.json` (new top-level `a11y` namespace)

- [ ] **Step 1: Add i18n keys to both locales**

In `locales/en.json`, after the closing brace of the `"nav"` object, add a sibling top-level key:

```json
"a11y": {
  "skipToContent": "Skip to content"
},
```

In `locales/ru.json`, same position:

```json
"a11y": {
  "skipToContent": "Перейти к контенту"
},
```

- [ ] **Step 2: Add the skip-link as the first focusable element**

In `App.vue`, directly after `<div id="app" class="min-h-screen bg-base">` (line 2) and BEFORE the `<TooltipProvider>`:

```html
<!-- A11y: first Tab stop. Visually hidden until keyboard-focused. -->
<a
  href="#main-content"
  class="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-[100] focus:px-4 focus:py-2 focus:rounded-lg focus:bg-brand-violet focus:text-white focus:text-sm focus:font-medium"
>
  {{ $t('a11y.skipToContent') }}
</a>
```

- [ ] **Step 3: Make `<main>` the skip target**

Change line 48 `<main v-else>` to:

```html
<main v-else id="main-content" tabindex="-1">
```

And in the existing `<style scoped>` block at the bottom of App.vue, add (scoped rule needed because the unlayered global `:focus-visible` box-shadow in main.css would otherwise draw a ring around the whole page after the skip jump; `main:focus-visible` wins on specificity):

```css
main:focus-visible {
  box-shadow: none;
}
```

- [ ] **Step 4: Verify**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/locales/`
Expected: tsc clean; locale tests pass (spotlight parity spec unaffected).

- [ ] **Step 5: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/App.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json
git commit -m "feat(a11y): skip-to-content link as first Tab stop

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

(If push is rejected non-fast-forward: `git pull --rebase --autostash && git push`. Same applies to every later commit step.)

---

### Task 2: ScheduleFilters — chip remove ✕ and "reset all" become buttons

**Files:**
- Modify: `components/schedule/ScheduleFilters.vue:28,30`
- Modify: `locales/en.json`, `locales/ru.json` (`schedule.removeFilter`)
- Test: `components/schedule/ScheduleFilters.spec.ts`

- [ ] **Step 1: Write the failing test**

Append inside the existing `describe('ScheduleFilters', ...)` block (the spec already has `mountFilters()` returning `{ filters, w }` with a reactive filters object; add `nextTick` to the existing `import { reactive, ref } from 'vue'` line):

```ts
it('chip remove is a native button and clears the filter', async () => {
  const { filters, w } = mountFilters()
  filters.myList = true
  await nextTick()
  const removeBtn = w.findAll('button').filter((b) => b.text() === '✕')
  expect(removeBtn.length).toBe(1)
  await removeBtn[0].trigger('click')
  expect(filters.myList).toBe(false)
})
it('reset-all is a native button', async () => {
  const { filters, w } = mountFilters()
  filters.myList = true
  await nextTick()
  const reset = w.findAll('button').filter((b) => b.text().includes('resetAll'))
  expect(reset.length).toBe(1)
  await reset[0].trigger('click')
  expect(w.emitted('reset')).toBeTruthy()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/schedule/ScheduleFilters.spec.ts`
Expected: FAIL — the ✕ and reset elements are `<span>`, so the button filters find 0.

- [ ] **Step 3: Implement**

Add to both locale files inside the existing `"schedule"` object: `"removeFilter": "Remove filter"` (en) / `"removeFilter": "Убрать фильтр"` (ru).

In `ScheduleFilters.vue` replace line 28's inner span:

```html
{{ chip.label }}<button type="button" class="cursor-pointer opacity-70 hover:opacity-100" :aria-label="$t('schedule.removeFilter')" @click="chip.remove()">✕</button>
```

and line 30:

```html
<button type="button" class="text-xs text-muted-foreground underline cursor-pointer hover:text-foreground" @click="$emit('reset')">{{ $t('schedule.resetAll') }}</button>
```

- [ ] **Step 4: Run tests**

Run: `bunx vitest run src/components/schedule/ScheduleFilters.spec.ts`
Expected: PASS (all, including pre-existing).

- [ ] **Step 5: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/schedule/ScheduleFilters.vue frontend/web/src/components/schedule/ScheduleFilters.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json
git commit -m "fix(a11y): schedule filter chips — remove/reset are real buttons

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 3: DayCell + WeekView — focusable day cells (tabindex fallback; nested EpisodeRows planned)

**Files:**
- Modify: `components/schedule/DayCell.vue:3-9`
- Modify: `components/schedule/WeekView.vue:4-9`
- Test: `components/schedule/DayCell.spec.ts`

- [ ] **Step 1: Write the failing tests** (append to existing describe; `mountCell`/`model`/`a` helpers already exist in the spec)

```ts
it('cell with episodes is keyboard-focusable and opens on Enter', async () => {
  const occ = [{ anime: a('1'), episode: 1, date: new Date(2026, 5, 13, 1) }]
  const w = mountCell(model({ occurrences: occ }))
  expect(w.attributes('tabindex')).toBe('0')
  expect(w.attributes('role')).toBe('button')
  await w.trigger('keydown', { key: 'Enter' })
  expect(w.emitted('open')).toBeTruthy()
})
it('cell opens on Space', async () => {
  const occ = [{ anime: a('1'), episode: 1, date: new Date(2026, 5, 13, 1) }]
  const w = mountCell(model({ occurrences: occ }))
  await w.trigger('keydown', { key: ' ' })
  expect(w.emitted('open')).toBeTruthy()
})
it('empty cell stays out of the tab order', () => {
  const w = mountCell(model({ occurrences: [] }))
  expect(w.attributes('tabindex')).toBeUndefined()
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `bunx vitest run src/components/schedule/DayCell.spec.ts`
Expected: FAIL — no tabindex/role attributes, no keydown handler.

- [ ] **Step 3: Implement DayCell**

On the root div of `DayCell.vue`, after the `:class` binding and alongside the existing `@click="onClick"`:

```html
:tabindex="hasEpisodes ? 0 : undefined"
:role="hasEpisodes ? 'button' : undefined"
@click="onClick"
@keydown.enter.space.prevent="onClick"
```

(`hasEpisodes` already exists in the script; `onClick` already guards on it.)

- [ ] **Step 4: Implement WeekView** (same pattern, inline — extract the guard into nothing, just repeat the existing inline expression)

On the per-column div in `WeekView.vue`, alongside the existing `@click`:

```html
:tabindex="c.occurrences.length ? 0 : undefined"
:role="c.occurrences.length ? 'button' : undefined"
@click="c.occurrences.length && $emit('open', c.date)"
@keydown.enter.space.prevent="c.occurrences.length && $emit('open', c.date)"
```

- [ ] **Step 5: Run tests**

Run: `bunx vitest run src/components/schedule/`
Expected: PASS.

- [ ] **Step 6: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/schedule/DayCell.vue frontend/web/src/components/schedule/DayCell.spec.ts frontend/web/src/components/schedule/WeekView.vue
git commit -m "fix(a11y): schedule day cells focusable, open on Enter/Space

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 4: ThemeCard — accordion header keyboard support

**Files:**
- Modify: `components/themes/ThemeCard.vue:6`

(ThemeCard has no spec and a heavyweight `theme` prop; the pattern is identical to Task 3 which is test-covered. Implementation + tsc only.)

- [ ] **Step 1: Implement**

Replace line 6 of `ThemeCard.vue`:

```html
<div class="flex gap-3 p-4 cursor-pointer" @click="expanded = !expanded">
```

with:

```html
<div
  class="flex gap-3 p-4 cursor-pointer"
  role="button"
  tabindex="0"
  :aria-expanded="expanded"
  @click="expanded = !expanded"
  @keydown.enter.space="onHeaderKey($event)"
>
```

WHY no `.prevent` on the binding: the header contains a nested `router-link` (line ~25). When THAT link is focused, its Enter keydown bubbles up to the header; a `.prevent` modifier would kill the link's navigation before any guard runs. So the handler guards first, then prevents.

Add to the script (after `const expanded = ref(false)`; `ref` already imported, nothing new needed):

```ts
function onHeaderKey(e: KeyboardEvent) {
  // Enter on the nested anime router-link must keep navigating.
  if ((e.target as HTMLElement).closest('a')) return
  e.preventDefault()
  expanded.value = !expanded.value
}
```

- [ ] **Step 2: Verify**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit`
Expected: clean.

- [ ] **Step 3: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/themes/ThemeCard.vue
git commit -m "fix(a11y): theme card accordion header keyboard-expandable

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 5: PullSummary — reveal cards become native buttons

**Files:**
- Modify: `components/gacha/PullSummary.vue:16-23` (template), `.rcard` CSS rule (~line 168)
- Test: `components/gacha/PullSummary.spec.ts`

- [ ] **Step 1: Write the failing test** (append inside the existing describe; `mountSummary`/`pulled` helpers exist; fake timers are set up in beforeEach)

```ts
it('reveal cards are native buttons (keyboard accessible)', () => {
  const w = mountSummary([pulled('1', 'N')])
  const card = w.find('[data-testid="summary-card-N"]')
  expect(card.element.tagName).toBe('BUTTON')
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `bunx vitest run src/components/gacha/PullSummary.spec.ts`
Expected: FAIL — tagName is `DIV`.

- [ ] **Step 3: Implement**

In the template, change the card element from `<div ... @click="reveal(i)">...</div>` to `<button type="button" ... @click="reveal(i)">...</button>` keeping ALL existing bindings (`:key`, `class="rcard"`, `:class`, `:style`, `:data-testid`) unchanged. Update the matching closing tag.

In the scoped CSS, extend the `.rcard` rule (~line 168) with button resets so the swap is layout-neutral:

```css
.rcard {
  /* ...existing props unchanged... */
  display: block;
  width: 100%;
  padding: 0;
  text-align: left;
  font: inherit;
}
```

- [ ] **Step 4: Run tests**

Run: `bunx vitest run src/components/gacha/PullSummary.spec.ts`
Expected: PASS (all pre-existing flip/reveal tests still green — they trigger `click`, which buttons also handle).

- [ ] **Step 5: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/gacha/PullSummary.vue frontend/web/src/components/gacha/PullSummary.spec.ts
git commit -m "fix(a11y): gacha pull-summary reveal cards are real buttons

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 6: PlayerScrubBar — ARIA slider + arrow-key seek

**Files:**
- Modify: `components/player/unified/PlayerScrubBar.vue` (track element lines 2-8; script)
- Test: `components/player/unified/PlayerScrubBar.spec.ts`

- [ ] **Step 1: Write the failing test** (append inside the existing describe)

```ts
it('is a keyboard slider: role + arrows emit seek ±5s', async () => {
  const w = mount(PlayerScrubBar, { props: { progress: 50, buffered: 0, durationSec: 1000, chapters: [] } })
  const track = w.find('[data-test="track"]')
  expect(track.attributes('role')).toBe('slider')
  expect(track.attributes('tabindex')).toBe('0')
  expect(track.attributes('aria-valuenow')).toBe('50')
  await track.trigger('keydown', { key: 'ArrowRight' })
  await track.trigger('keydown', { key: 'ArrowLeft' })
  const seeks = w.emitted('seek') as number[][]
  expect(seeks[0][0]).toBeCloseTo(50.5) // +5s of 1000s = +0.5pct
  expect(seeks[1][0]).toBeCloseTo(49.5)
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `bunx vitest run src/components/player/unified/PlayerScrubBar.spec.ts`
Expected: FAIL — no role/tabindex, no seek emissions on keydown.

- [ ] **Step 3: Implement**

Track element (root div, lines 2-8) gains:

```html
role="slider"
tabindex="0"
aria-label="Seek"
:aria-valuemin="0"
:aria-valuemax="100"
:aria-valuenow="Math.round(progress)"
:aria-valuetext="fmt((progress / 100) * durationSec)"
@keydown.left.prevent="onKeySeek(-1)"
@keydown.right.prevent="onKeySeek(1)"
```

Script — add after `onTrackClick` (uses existing `clamp` + `emit`):

```ts
// Keyboard slider: ←/→ = ±5 s expressed in track percent.
function onKeySeek(dir: 1 | -1) {
  if (!props.durationSec) return
  const stepPct = (5 / props.durationSec) * 100
  emit('seek', clamp(props.progress + dir * stepPct, 0, 100))
}
```

(`Math` and the script-defined `fmt` are both available in the template.)

- [ ] **Step 4: Run tests**

Run: `bunx vitest run src/components/player/unified/PlayerScrubBar.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/player/unified/PlayerScrubBar.vue frontend/web/src/components/player/unified/PlayerScrubBar.spec.ts
git commit -m "feat(a11y): scrub bar is an ARIA slider with arrow-key seek

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 7: BrowseSubsModal — close on Escape

**Files:**
- Modify: `components/player/unified/BrowseSubsModal.vue` (script)
- Test: `components/player/unified/BrowseSubsModal.spec.ts`

- [ ] **Step 1: Write the failing test** (append inside the existing describe; `tracks` fixture exists)

```ts
it('closes on Escape', () => {
  const w = mount(BrowseSubsModal, { props: { tracks, selectedUrl: null } })
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
  expect(w.emitted('close')).toBeTruthy()
  w.unmount()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `bunx vitest run src/components/player/unified/BrowseSubsModal.spec.ts`
Expected: FAIL — no `close` emitted.

- [ ] **Step 3: Implement**

In the script block, extend the existing `vue` import with `onMounted, onBeforeUnmount` and add:

```ts
function onWindowKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}
onMounted(() => window.addEventListener('keydown', onWindowKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onWindowKey))
```

- [ ] **Step 4: Run tests**

Run: `bunx vitest run src/components/player/unified/BrowseSubsModal.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/player/unified/BrowseSubsModal.vue frontend/web/src/components/player/unified/BrowseSubsModal.spec.ts
git commit -m "fix(a11y): browse-subs modal closes on Escape

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 8: Admin surfaces — AdminFeedback rows/kanban + RawLibrary link results

**Files:**
- Modify: `views/admin/AdminFeedback.vue` (~line 121 table `<tr>`, ~line 196 kanban card div)
- Modify: `views/admin/RawLibrary.vue` (~line 230 link-result `<li>`)

(Admin-only screens, no specs — implementation + tsc. Same tested pattern as Task 3.)

- [ ] **Step 1: AdminFeedback table rows**

The clickable `<tr v-for="r in items" ... @click="openReport(r.id)">` gains:

```html
tabindex="0"
@keydown.enter="openReport(r.id)"
```

(No `role` — keep table semantics; no `.prevent` needed, Enter has no default on `<tr>`.)

- [ ] **Step 2: AdminFeedback kanban cards**

The draggable card `<div v-for="r in col.items" ... @click="openReport(r.id)">` gains:

```html
tabindex="0"
role="button"
@keydown.enter.prevent="openReport(r.id)"
```

- [ ] **Step 3: RawLibrary link-result rows**

The `<li v-for="anime in pendingLinkResults[job.id]" ... @click="linkJob(job, anime)">` gains:

```html
tabindex="0"
role="button"
@keydown.enter.prevent="linkJob(job, anime)"
```

- [ ] **Step 4: Verify**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit`
Expected: clean.

- [ ] **Step 5: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/views/admin/AdminFeedback.vue frontend/web/src/views/admin/RawLibrary.vue
git commit -m "fix(a11y): admin feedback rows/kanban + raw-library results keyboard-reachable

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 9: Full verification + ship

- [ ] **Step 1: Full frontend gate**

```bash
cd /data/animeenigma/frontend/web
bunx vitest run src/components/schedule/ src/components/gacha/PullSummary.spec.ts src/components/player/unified/PlayerScrubBar.spec.ts src/components/player/unified/BrowseSubsModal.spec.ts src/locales/
bunx tsc --noEmit
bash scripts/design-system-lint.sh
```

Expected: all green, lint ERRORS=0.

- [ ] **Step 2: Invoke `/animeenigma-after-update`**

The after-update skill handles: full lint/build, `make redeploy-web`, health check, Russian Trump-mode changelog entry in `frontend/web/public/changelog.json`, final commit + push. Changelog content hint: keyboard navigation / Tab order / skip-link across the whole site.

- [ ] **Step 3: Offer (do not run) a Chrome smoke**

Per DS-NF-06 this change is visual-adjacent (skip-link, focus rings are cascade-sensitive, jsdom can't see them) — ASK the owner whether they want a Chrome checkup; do not run one unprompted.
