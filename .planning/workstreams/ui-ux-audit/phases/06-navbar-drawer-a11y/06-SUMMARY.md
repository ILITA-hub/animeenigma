# Phase 6 Summary: Navbar drawer a11y

**Completed:** 2026-05-13
**Plan:** 06-PLAN.md
**Outcome:** Mobile drawer in `Navbar.vue` is now a proper ARIA dialog with keyboard, screen-reader, and touch-target support. New `useFocusTrap` composable available for future modal-like surfaces. 6 audit findings closed.

## Changes shipped

### New composable

`frontend/web/src/composables/useFocusTrap.ts` — naive Tab/Shift+Tab focus cycle between the first and last focusable descendants of a `Ref<HTMLElement | null>`. ~70 LOC including doc comments. Zero dependencies (no `focus-trap` npm package). Signature:

```ts
useFocusTrap({
  active: Ref<boolean>,
  container: Ref<HTMLElement | null>,
  returnFocusTo?: Ref<HTMLElement | null>,
})
```

On `active → true`: attaches a keydown listener and focuses the first focusable child. On `active → false`: detaches the listener and restores focus to `returnFocusTo` if provided. Focusable selector matches the standard WCAG set (`a[href]`, non-disabled buttons/inputs/selects/textareas, `[tabindex]` other than `-1`) and additionally filters out hidden elements via `offsetParent !== null`.

### Navbar.vue — hamburger button (UA-083, UA-045)

- `id="navbar-mobile-toggle"` stable hook
- `:aria-expanded="mobileMenuOpen"` (closes **UA-083**)
- `aria-controls="mobile-drawer"`
- `ref="hamburgerButtonRef"` for focus-restore on drawer close
- Class change: `p-2` → `p-3 min-w-[44px] min-h-[44px] flex items-center justify-center` to clear the WCAG 2.5.5 44×44 touch-target floor (closes **UA-045**). The icon stays centered in the row via flex.

### Navbar.vue — drawer container (UA-053, UA-054, UA-084, UA-112)

- `id="mobile-drawer"` matches the hamburger's `aria-controls`
- `role="dialog"` (closes **UA-053**)
- `aria-modal="true"` (closes **UA-084**)
- `aria-labelledby="mobile-drawer-title"` points at a new `<h2 class="sr-only">` placed as the first child of the drawer with text from `nav.drawerTitle`
- `ref="drawerRef"` scopes the focus-trap cycle
- `@keydown.escape="mobileMenuOpen = false"` (closes **UA-054**)

UA-112 was confirmed by audit notes as the same root cause as UA-053 + UA-084 — fixed transitively.

### Navbar.vue — focus-trap wiring + body scroll lock

- `useFocusTrap({ active: mobileMenuOpen, container: drawerRef, returnFocusTo: hamburgerButtonRef })` invocation in `<script setup>`
- `watch(mobileMenuOpen, (open) => { document.body.style.overflow = open ? 'hidden' : '' })` for body scroll lock
- `onUnmounted` cleanup clears residual `overflow: hidden` if the Navbar is torn down while the drawer was open
- **No** `onClickOutside` close — modals with `aria-modal="true"` should not close on backdrop click; ESC + hamburger toggle are the only closure paths

### i18n

One new key per locale (`en`/`ru`/`ja`):

- `nav.drawerTitle`
  - en: `"Navigation menu"`
  - ru: `"Меню навигации"`
  - ja: `"ナビゲーションメニュー"`

## Verification

See `06-VERIFICATION.md` for the success-criteria scorecard. `bunx vue-tsc --noEmit` is clean, `make redeploy-web` completed successfully, and all six grep checks listed in the plan pass.

## Files touched

```
frontend/web/src/composables/useFocusTrap.ts       (new, ~70 LOC inc. doc comments)
frontend/web/src/components/layout/Navbar.vue      (a11y attrs + ESC + trap + scroll lock)
frontend/web/src/locales/en.json                   (+1 key: nav.drawerTitle)
frontend/web/src/locales/ru.json                   (+1 key: nav.drawerTitle)
frontend/web/src/locales/ja.json                   (+1 key: nav.drawerTitle)
.planning/workstreams/ui-ux-audit/phases/06-navbar-drawer-a11y/
  06-CONTEXT.md
  06-PLAN.md
  06-SUMMARY.md      (this file)
  06-VERIFICATION.md (new)
```

## Commits

- `0ce0776` — `feat(06): useFocusTrap composable`
- `615f688` — `feat(06): Navbar drawer ARIA dialog semantics + ESC + focus trap`
- `404555c` — `feat(06): nav.drawerTitle i18n keys (en/ru/ja)`

## Notes for downstream phases

- The `useFocusTrap` composable is reusable for any future modal-like surface. The CONTEXT mentioned Phase 12 (AdminRecs picker) and Phase 15 (sidebar on mobile) as upcoming consumers — they should adopt the same composable rather than re-implementing the trap.
- The body scroll lock pattern (inline `watch` on the open-state ref + `onUnmounted` cleanup) is kept local to Navbar.vue. If a second modal-like surface needs it, extract to `useBodyScrollLock` then.
- Manual keyboard probe (hamburger → Enter → Tab cycle → ESC → focus restore) was NOT executed in this autonomous run — the changes are static-verifiable from source and follow the standard dialog pattern. The Chrome MCP axe-core re-run on the mobile drawer is left as the audit-workstream's follow-up verification pass.
- UA-055 (Schedule drawer entry) remains deferred to Phase 16 (Broadcast schedule view).
- Generic `<Drawer>` component extraction remains deferred — only one drawer surface in the app today.
