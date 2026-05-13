# Phase 6: Navbar drawer a11y - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, single-component a11y retrofit)

<domain>
## Phase Boundary

Make the mobile drawer in `frontend/web/src/components/layout/Navbar.vue` keyboard- and screen-reader-usable. Audit findings closed:

- UA-053 — Drawer missing `role="dialog"`
- UA-054 — Drawer missing ESC handler
- UA-083 — Hamburger button missing `aria-expanded`
- UA-084 — Drawer missing `aria-modal="true"`
- UA-112 — Confirmed via JS click probe (same root cause as 053/084)
- UA-045 — Drawer touch-target sizing on hamburger button (≥44×44 target)

Scope is **one file** (`Navbar.vue`) plus i18n keys for any new accessible names. No new shared components introduced — the drawer is the only modal-drawer surface in the app today; if a second emerges, extract to `<Drawer>` then.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

- **Focus trap implementation**: small inline composable (`useFocusTrap` helper local to Navbar.vue or extracted to `frontend/web/src/composables/useFocusTrap.ts`). Trap is naive: cycle Tab/Shift+Tab between first and last focusable descendants of the drawer. Use `:focus-visible` ring already established by `text-cyan-400`/outline patterns; no new ring styles needed. Prefer extracting to `composables/useFocusTrap.ts` since the same pattern is needed by future modal-like surfaces (Phase 12 AdminRecs picker, Phase 15 sidebar on mobile).
- **ESC handler**: attach via `@keydown.escape` on the drawer container (mirrors the existing search-bar pattern at line 58). Also restore focus to the hamburger button on close — standard dialog behavior.
- **`role="dialog"` + `aria-modal="true"`**: applied to the drawer's outer `<div v-if="mobileMenuOpen">`. Add `aria-labelledby` pointing to the hamburger button's `id` (or a sr-only heading inside the drawer — prefer the latter for clarity).
- **`aria-expanded`**: bind on the hamburger button to `mobileMenuOpen` boolean. Pair with `aria-controls="mobile-drawer"` and matching `id="mobile-drawer"` on the drawer.
- **Touch-target sizing (UA-045)**: hamburger button currently `p-2` ≈ 32×32. Bump to `p-3` or add `min-w-[44px] min-h-[44px]` to clear the 44px WCAG touch-target. Verify visually still looks correct in the navbar row.
- **Body scroll lock**: when drawer is open, lock body scroll (`overflow: hidden` on `<html>` or `document.body`). Standard dialog UX; matches established Vue Modal patterns in the codebase.
- **i18n**: one new key `nav.drawerTitle` (sr-only h2 inside drawer for `aria-labelledby`) in RU/EN/JA.
- **No animation changes**: keep existing `<Transition name="mobile-menu">` and its CSS. The a11y attributes layer on top without visual impact.

### Locked from ROADMAP

- Single component. No new design surfaces. No Schedule drawer entry (UA-055 deferred to Phase 16 when Broadcast Schedule lands).
- Phase 5's `<ButtonGroup>` already wraps the in-drawer lang toggle (line 239) — no change there.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `frontend/web/src/components/layout/Navbar.vue` — Navbar.vue is the only file touched. `mobileMenuOpen` ref controls drawer at line 294; hamburger button at lines 170-181; drawer at lines 185-256.
- `onClickOutside` from `@vueuse/core` is already imported (line 266) — can wire to close-on-outside-click if desired, but a `role="dialog" aria-modal="true"` drawer should NOT close on outside click (modals trap interaction); rely on backdrop + ESC + hamburger toggle for closure instead.
- Existing search-input ESC handler at line 58 (`@keydown.escape="closeSearch"`) is the pattern reference.
- `aria-pressed` pattern from Phase 5's `<ButtonGroup>` work is already established for in-drawer lang buttons.

### Established Patterns

- i18n via `$t()` and `useI18n()` (`vue-i18n`). Locale files: `frontend/web/src/locales/{en,ru,ja}.json`.
- `:aria-label` bindings via i18n keys (already used at line 173 on the hamburger).
- Composables live in `frontend/web/src/composables/` — `useImageProxy`, `useFocusTrap` will join them.

### Integration Points

- No router changes. No new routes.
- No backend changes.
- No new dependencies (focus trap is implementable in ~30 LOC without `focus-trap` npm package; keep bundle lean).
- Verified clean by axe-core after change: Navbar + drawer must show zero violations.

</code_context>

<specifics>
## Specific Ideas

- The sr-only heading inside the drawer (for `aria-labelledby`) reuses the existing `sr-only` Tailwind class. Content: i18n key `nav.drawerTitle` (e.g. RU "Меню навигации", EN "Navigation menu", JA "ナビゲーションメニュー").
- The hamburger button gets a stable `id="navbar-mobile-toggle"` for `aria-labelledby` fallback if the sr-only heading approach is rejected later.
- Backdrop: NOT adding a separate fixed-overlay backdrop in this phase. The drawer is positioned inside the nav and the existing `glass-nav` background covers the page below it on mobile. If a true full-screen drawer pattern is needed later, that's a Phase 20 polish item.

</specifics>

<deferred>
## Deferred Ideas

- UA-055 (Schedule drawer entry) → Phase 16 (Broadcast schedule view lands the `/schedule` route).
- Generic `<Drawer>` component extraction → only if a second drawer-like surface emerges; do not pre-abstract.
- `focus-trap` npm package → keep the inline 30-LOC composable; revisit only if more than 3 modals adopt it.

</deferred>
