# Phase 6 Plan: Navbar drawer a11y

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Scope: single file (`frontend/web/src/components/layout/Navbar.vue`) plus one
new composable (`useFocusTrap.ts`) and three locale files. Closes audit
findings UA-053, UA-054, UA-083, UA-084, UA-112, UA-045. No new visuals — all
work is ARIA semantics + keyboard behavior layered on top of the existing
`<Transition name="mobile-menu">` drawer.

## Tasks

### Composable

- [ ] Create `frontend/web/src/composables/useFocusTrap.ts` — naive trap
  cycling Tab / Shift+Tab between the first and last focusable descendants of
  a `Ref<HTMLElement | null>`. Accepts `{ active: Ref<boolean>, container:
  Ref<HTMLElement | null>, returnFocusTo?: Ref<HTMLElement | null> }`. On
  activation, focuses the first focusable child; on deactivation, restores
  focus to `returnFocusTo` if provided. Listens for `keydown` while active,
  detaches when inactive. ~30 LOC, zero deps (no `focus-trap` npm package).
  Focusable-selector: `'a[href], button:not([disabled]), input:not([disabled]),
  select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'`.

### Navbar.vue — hamburger button (UA-083, UA-045)

- [ ] Bind `id="navbar-mobile-toggle"` on the hamburger `<button>` at lines
  170-181 (stable hook for `aria-controls` reverse-lookup if needed).
- [ ] Bind `:aria-expanded="mobileMenuOpen"` and `aria-controls="mobile-drawer"`
  on the hamburger button. Closes **UA-083**.
- [ ] Replace `p-2` with `p-3 min-w-[44px] min-h-[44px] flex items-center
  justify-center` to clear the WCAG 2.5.5 touch-target floor (≥44×44). Verify
  visually that the navbar row still aligns (logo + hamburger row height
  remains `h-16`). Closes **UA-045**.
- [ ] Wire `ref="hamburgerButtonRef"` so the focus-trap composable can restore
  focus on drawer close. Declare `const hamburgerButtonRef =
  ref<HTMLButtonElement | null>(null)` in `<script setup>`.

### Navbar.vue — drawer container (UA-053, UA-054, UA-084, UA-112)

- [ ] Add `id="mobile-drawer"`, `role="dialog"`, `aria-modal="true"`, and
  `aria-labelledby="mobile-drawer-title"` to the drawer `<div v-if="mobileMenuOpen">`
  at line 186. Closes **UA-053**, **UA-084**, **UA-112**.
- [ ] Add `ref="drawerRef"` on the same drawer `<div>` so the focus-trap
  composable can scope the cycle. Declare `const drawerRef = ref<HTMLElement |
  null>(null)` in `<script setup>`.
- [ ] Inside the drawer (immediately after the opening `<div>` on line 186)
  insert `<h2 id="mobile-drawer-title" class="sr-only">{{ $t('nav.drawerTitle') }}</h2>`
  as the labelled-by target. The visible nav links continue to render below.
- [ ] Bind `@keydown.escape="mobileMenuOpen = false"` on the drawer `<div>`
  (mirrors the search-input ESC pattern at line 58). Closes **UA-054**.

### Navbar.vue — focus-trap wiring + body scroll lock

- [ ] Import and invoke the new composable in `<script setup>`:
  `useFocusTrap({ active: mobileMenuOpen, container: drawerRef,
  returnFocusTo: hamburgerButtonRef })`. The composable internally handles
  the watcher on `active` to attach/detach the listener.
- [ ] Body scroll lock via a `watch(mobileMenuOpen, …)` that toggles
  `document.body.style.overflow = open ? 'hidden' : ''`. Clean up in
  `onUnmounted` to clear any residual `overflow: hidden`. (Inline — kept
  local to Navbar.vue; one consumer doesn't earn a composable yet.)
- [ ] Do **not** wire `onClickOutside` to close the drawer — modals with
  `aria-modal="true"` must not close on outside click; ESC + hamburger
  toggle are the only closure paths.

### i18n

- [ ] Add to `frontend/web/src/locales/en.json`:
  - `nav.drawerTitle`: `"Navigation menu"`
- [ ] Add to `frontend/web/src/locales/ru.json`:
  - `nav.drawerTitle`: `"Меню навигации"`
- [ ] Add to `frontend/web/src/locales/ja.json`:
  - `nav.drawerTitle`: `"ナビゲーションメニュー"`

### Verification

- [ ] `bunx vue-tsc --noEmit` — passes (composable types resolve, Navbar
  refs typed correctly).
- [ ] `make redeploy-web` — frontend rebuilt and shipped.
- [ ] `grep -n 'role="dialog"\|aria-modal\|aria-expanded\|aria-controls'
  frontend/web/src/components/layout/Navbar.vue` — confirms all four
  attributes present on the drawer + hamburger button.
- [ ] `grep -n '@keydown.escape' frontend/web/src/components/layout/Navbar.vue`
  — confirms at least two matches (existing search ESC at line 58 + new
  drawer ESC).
- [ ] `grep -n 'min-w-\[44px\]' frontend/web/src/components/layout/Navbar.vue`
  — confirms hamburger touch-target sizing applied (UA-045).
- [ ] `grep -n 'useFocusTrap' frontend/web/src/composables/useFocusTrap.ts
  frontend/web/src/components/layout/Navbar.vue` — confirms composable
  exists and is imported.
- [ ] `grep -nE '"drawerTitle"' frontend/web/src/locales/en.json
  frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json` —
  confirms three locale entries (one per file).
- [ ] Manual / Chrome MCP axe-core re-run on the mobile drawer (viewport
  ≤ 768px, drawer open via hamburger click): zero new violations on Navbar.
  Specifically, the `dialog-name`, `button-name`, and `interactive-supports-focus`
  rules pass.
- [ ] Manual keyboard probe: focus the hamburger → Enter to open → Tab cycles
  only within the drawer (does not reach `<router-link to="/">` logo or
  desktop search button) → ESC closes drawer → focus returns to hamburger
  button.

## Files touched

```
frontend/web/src/composables/useFocusTrap.ts       (new, ~30 LOC)
frontend/web/src/components/layout/Navbar.vue      (a11y attrs + ESC + trap + scroll lock)
frontend/web/src/locales/en.json                   (+1 key)
frontend/web/src/locales/ru.json                   (+1 key)
frontend/web/src/locales/ja.json                   (+1 key)
.planning/workstreams/ui-ux-audit/phases/06-navbar-drawer-a11y/
  06-CONTEXT.md
  06-PLAN.md
  06-SUMMARY.md      (written at execute-phase end)
  06-VERIFICATION.md (written at execute-phase end)
```

## Closes

| Finding | Surface | Mechanism |
|---|---|---|
| UA-053 | Drawer | `role="dialog"` on drawer container |
| UA-054 | Drawer | `@keydown.escape="mobileMenuOpen = false"` |
| UA-083 | Hamburger | `:aria-expanded="mobileMenuOpen"` + `aria-controls="mobile-drawer"` |
| UA-084 | Drawer | `aria-modal="true"` on drawer container |
| UA-112 | Drawer | Same root cause as UA-053 + UA-084 (verified via JS click probe) |
| UA-045 | Hamburger | `p-3 min-w-[44px] min-h-[44px]` touch-target |
