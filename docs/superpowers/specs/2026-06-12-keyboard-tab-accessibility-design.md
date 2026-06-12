# Keyboard / Tab-Order Accessibility — Pragmatic Pass (2026-06-12)

**Status:** approved by owner 2026-06-12 («Да, поехали»).
**Scope:** whole site, pragmatic depth (no roving tabindex, no new ARIA widgets
beyond the scrub-bar slider).

## Goal

Every interactive element is reachable with Tab, activates with Enter/Space,
and the focus order follows the visual/DOM order. A keyboard-only user can get
from page load to "watching an episode" without a mouse.

## Approach (chosen of 3)

**Semantics-first sweep.** Replace clickable `div`/`span` with native
`<button>` / `<router-link>` wherever feasible — native elements give focus,
key activation, and screen-reader role for free. Only where nested interactive
children forbid a native wrapper (button-in-button / link-in-button is invalid
HTML), fall back to `tabindex="0"` + `role` + `@keydown.enter` /
`@keydown.space.prevent`.

Rejected alternatives:
- *Global `v-clickable` directive* — less churn but leaves wrong semantics and
  hides the problem from future readers.
- *Full ARIA rework + eslint-plugin-vuejs-accessibility gate* — high regression
  risk, disproportionate to a self-hosted group's needs.

## Hard rules

1. **No positive `tabindex` values.** Only `0` (join the Tab sequence in DOM
   order) and `-1` (programmatic focus only). Tab order is fixed by fixing DOM
   order, never by numbering.
2. **Do not touch the global focus ring** (`styles/main.css:211`
   `:focus-visible` cyan ring). Owner reverted a restyle on 2026-06-11; ask
   before any change.
3. Design-system rules apply: semantic tokens, existing `@/components/ui`
   primitives, `font-medium`/`font-semibold` only.
4. New user-visible strings go to BOTH `locales/en.json` and `locales/ru.json`.

## Changes

| # | File | Fix |
|---|------|-----|
| 1 | `src/App.vue` | Skip-link «Перейти к контенту» as the very first focusable element; visually hidden until focused (`sr-only focus:not-sr-only` pattern); targets `#main-content` anchor on the router-view container. i18n key `a11y.skipToContent`. |
| 2 | `components/home/RecsRow.vue` | Card tile `div @click` → `<router-link>`. |
| 3 | `components/gacha/PullSummary.vue` | `.rcard` clickable div → `<button type="button">`. |
| 4 | `components/schedule/ScheduleFilters.vue` | Filter-chip remove `span @click` → `<button>` with `aria-label`. |
| 5 | `components/schedule/DayCell.vue` + `WeekView.vue` | Clickable day cells: contain nested interactive episode rows → `tabindex="0"` + `role="button"` + Enter/Space keydown. |
| 6 | `components/themes/ThemeCard.vue` | Accordion header div (contains nested `router-link`): `tabindex="0"` + `role="button"` + `aria-expanded` + Enter/Space. |
| 7 | `components/player/unified/PlayerScrubBar.vue` | Track gets `role="slider"`, `tabindex="0"`, `aria-valuemin/max/now` (+ `aria-valuetext` mm:ss), ←/→ seek ±5 s. |
| 8 | `views/admin/AdminFeedback.vue` | Clickable table rows (contain nested select/buttons) and kanban cards: `tabindex="0"` + Enter. |
| 9 | `views/Browse.vue`, `views/Profile.vue`, `views/admin/RawLibrary.vue`, `components/player/unified/BrowseSubsModal.vue` | Same rule applied to the 1–3 clickable non-focusable elements each (exact elements enumerated at implementation time; same decision tree: native swap first, tabindex fallback). |

Known-good (verified, no changes): PosterCard/MediaTile (already
`router-link`), Navbar (semantic buttons + FocusTrap drawer), shadcn/reka-ui
Dialogs (built-in focus trap + Escape), GenreFilterPopup (listbox pattern),
GachaCollection, UnifiedPlayer global hotkeys, Home.vue section DOM order.

## Error handling / edge cases

- Enter/Space handlers must call the same method as `@click` (no logic forks).
- `@keydown.space.prevent` to stop page scroll on Space.
- Elements converted to `<button>` need `type="button"` (no accidental form
  submits) and CSS check: buttons are `display: inline-block` with UA styles —
  reset via existing utility classes; verify no layout shift.
- Skip-link must not steal the autofocus the Auth view sets on its first input.

## Verification

- Update co-located vitest specs of changed components: keydown Enter/Space
  triggers the same emit/handler as click; scrub bar emits seek on ArrowRight.
- `bunx tsc --noEmit`, design-system lint, `bunx vitest run` for touched specs.
- Chrome smoke: opt-in per DS-NF-06 — offer to owner after deploy (skip-link
  and focus rings are cascade-sensitive; jsdom cannot verify them).

## Metrics

- UXΔ = +3 (Better) — keyboard users gain full reachability; zero change for
  mouse users.
- CDI = 0.04 * 8 — wide but shallow mechanical spread across ~12 files.
- MVQ = Sprite 90%/85%.
