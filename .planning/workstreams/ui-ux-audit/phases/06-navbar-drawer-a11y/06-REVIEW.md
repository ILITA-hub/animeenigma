---
phase: 06-navbar-drawer-a11y
reviewed: 2026-05-13T00:00:00Z
depth: standard
files_reviewed: 5
files_reviewed_list:
  - frontend/web/src/composables/useFocusTrap.ts
  - frontend/web/src/components/layout/Navbar.vue
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/locales/ja.json
findings:
  critical: 0
  warning: 5
  info: 3
  total: 8
status: issues_found
---

# Phase 6: Code Review Report

**Reviewed:** 2026-05-13
**Depth:** standard
**Files Reviewed:** 5
**Status:** issues_found

## Summary

The phase ships a working baseline of ARIA dialog semantics, ESC handling, focus trap, body scroll lock, and a 44x44 touch target. The 6 closed audit findings (UA-053/054/083/084/112/045) are all genuinely addressed in the source — the attributes are present and bound correctly, the i18n keys parse, and there are no JSON syntax errors. The implementation does, however, have several robustness gaps that are worth fixing before this composable propagates to Phase 12/15 consumers as the SUMMARY suggests.

Top issues:

- The focus-trap composable leaks its global `document` keydown listener if the host component unmounts while `active === true` (no `onScopeDispose`).
- The body-scroll-lock writes a raw empty string to `document.body.style.overflow`, which stomps any pre-existing inline value set by `Modal.vue` (which uses the same naive pattern). Two overlapping consumers can unlock each other.
- Viewport resize from mobile to desktop while the drawer is open leaves `mobileMenuOpen === true`, the body scroll permanently locked, and the user with no way to close it (the hamburger trigger is `md:hidden`).
- The drawer's `@keydown.escape` only fires when focus is inside the drawer subtree. Because there is no backdrop and the focus trap only intercepts Tab (not pointer events), the user can click the hamburger button (a sibling of the drawer) and lose ESC responsiveness.
- The composable does not honour `{ immediate: true }` on its watcher, so a host that mounts with `active: ref(true)` (a documented use case per the JSDoc) will get no listener and no initial focus until the next toggle.

None of these are security issues or data-loss risks, but the leak and the resize trap are real correctness problems and should be fixed before this code ships to a wider audience.

## Warnings

### WR-01: Focus-trap composable leaks the document keydown listener on parent unmount

**File:** `frontend/web/src/composables/useFocusTrap.ts:61-75`
**Issue:** The `document.addEventListener('keydown', onKeydown)` is attached inside the `watch` callback but is only removed when `active` transitions from true to false. If the host component unmounts while `active === true` (e.g. navigation tears down Navbar.vue while the drawer is open, or a future consumer hot-swaps the dialog), the listener stays attached to `document` for the lifetime of the page. The Vue `watch` itself is auto-disposed by the component scope, so the cleanup branch never runs.

**Fix:** Register an `onScopeDispose` callback that detaches the listener unconditionally:

```ts
import { watch, nextTick, onScopeDispose, type Ref } from 'vue'
// ...
export function useFocusTrap(options: UseFocusTrapOptions): void {
  // ...existing watch/onKeydown...

  onScopeDispose(() => {
    document.removeEventListener('keydown', onKeydown)
  })
}
```

`onScopeDispose` works inside any reactive scope (including `<script setup>`), unlike `onUnmounted` which is component-only — and that matches the composable's documented intent.

### WR-02: Body scroll lock uses naive empty-string reset that stomps other lockers

**File:** `frontend/web/src/components/layout/Navbar.vue:323-325, 438`
**Issue:** Setting `document.body.style.overflow = ''` unconditionally clears whatever value any other consumer has set. `Modal.vue` (`frontend/web/src/components/ui/Modal.vue:110-114, 131`) uses the exact same naive pattern. If a `<Modal>` is open (modal sets `overflow: hidden`) and the user opens the mobile drawer (drawer also sets `hidden`), then closes the drawer first, the watcher sets `overflow = ''` and the Modal's scroll lock evaporates while the modal is still visible.

It also stomps any non-empty value set by application CSS or 3rd-party code via inline style.

**Fix:** Either reference-count the lockers, or save and restore the previous value:

```ts
let previousOverflow = ''
watch(mobileMenuOpen, (open) => {
  if (open) {
    previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
  } else {
    document.body.style.overflow = previousOverflow
  }
})

onUnmounted(() => {
  if (mobileMenuOpen.value) {
    document.body.style.overflow = previousOverflow
  }
})
```

The CONTEXT explicitly defers a shared `useBodyScrollLock` composable until a second consumer exists — but Modal.vue is already that second consumer, and they're now silently fighting.

### WR-03: Viewport resize from mobile to desktop strands the drawer in open state with body scroll permanently locked

**File:** `frontend/web/src/components/layout/Navbar.vue:170-198, 309, 323-325`
**Issue:** The hamburger and drawer are both `md:hidden`. If the user opens the drawer on mobile and then resizes (or rotates a tablet) to a viewport >= 768px:

1. The hamburger button is hidden, so the user can't toggle the drawer.
2. The drawer is hidden by CSS, but `v-if="mobileMenuOpen"` still evaluates true so the element is in the DOM (just `display: none`).
3. `mobileMenuOpen` stays `true` indefinitely, so the watcher never fires the `else` branch and `document.body.style.overflow` stays `'hidden'`.
4. The focus-trap composable also keeps its `document` keydown listener attached on desktop, where the drawer is invisible — every Tab keystroke is now intercepted by an invisible dialog.

ESC also won't help, because focus is no longer inside the (now `display: none`) drawer subtree, and `@keydown.escape` is bound on the drawer element.

**Fix:** Watch for a desktop breakpoint and force-close on entry. The simplest implementation:

```ts
import { useMediaQuery } from '@vueuse/core' // already in package.json
const isDesktop = useMediaQuery('(min-width: 768px)')
watch(isDesktop, (desktop) => {
  if (desktop && mobileMenuOpen.value) {
    mobileMenuOpen.value = false
  }
})
```

This also implicitly fixes the focus-trap-on-desktop side effect via the existing watcher chain.

### WR-04: ESC handler is scoped to the drawer subtree only — focus can escape and trap the user

**File:** `frontend/web/src/components/layout/Navbar.vue:198`
**Issue:** `@keydown.escape="mobileMenuOpen = false"` is bound on the drawer `<div>`. The Vue listener only fires for keydown events whose target is a descendant of that div (and which bubble up through it). The focus trap only intercepts `Tab`, not pointer events. There is no backdrop intercepting clicks. Therefore the user can:

1. Open the drawer (focus moves into it — good).
2. Click the hamburger button — it's a sibling of the drawer, not a descendant. Focus moves to the hamburger.
3. Press ESC. Event target is the hamburger button, event bubbles up the `<button>` -> `<div class="flex">` -> `<nav>` chain. It never enters the drawer subtree, so `@keydown.escape` on the drawer never fires.
4. Drawer stays open with focus outside it. ESC appears broken from the user's perspective.

This same scenario can be reached by clicking the logo `<router-link to="/">` (another sibling).

**Fix:** Move the ESC listener to `document` for the lifetime of `mobileMenuOpen`, matching the pattern Modal.vue already uses (`Modal.vue:118-127`):

```ts
const handleEscape = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && mobileMenuOpen.value) {
    mobileMenuOpen.value = false
  }
}

onMounted(() => document.addEventListener('keydown', handleEscape))
onUnmounted(() => document.removeEventListener('keydown', handleEscape))
```

Then drop `@keydown.escape` from the drawer div. This also matches the dialog spec — ESC should work globally while a modal is open.

### WR-05: Watcher does not honour `{ immediate: true }` — composable silently no-ops when active starts true

**File:** `frontend/web/src/composables/useFocusTrap.ts:61`
**Issue:** The JSDoc at lines 4-9 states "On activation, focuses the first focusable child" — implying that if `active` is already `true` at invocation, the trap activates. But `watch(active, ...)` without `{ immediate: true }` only fires on transitions. A consumer that passes `active: ref(true)` (legitimate for e.g. a dialog mounted with `:open="true"` initial prop) will get no listener attached and no initial focus until the parent toggles the ref to false and back.

Today's Navbar consumer starts with `mobileMenuOpen = ref(false)` so this is latent, but the composable is documented in the SUMMARY as the foundation for Phase 12 (AdminRecs picker) and Phase 15 (sidebar). One of those is likely to mount with active already true.

**Fix:** Add `{ immediate: true }`:

```ts
watch(active, async (isActive, wasActive) => {
  // ...existing body...
}, { immediate: true })
```

Note that `wasActive` will be `undefined` on the immediate run; the existing `else if (wasActive)` guard correctly skips the cleanup branch in that case, so no further change is needed.

## Info

### IN-01: Redundant `[disabled]` filter in `getFocusables`

**File:** `frontend/web/src/composables/useFocusTrap.ts:33-34`
**Issue:** The `FOCUSABLE_SELECTOR` already excludes `button:not([disabled])`, `input:not([disabled])`, etc. The subsequent `.filter(el => !el.hasAttribute('disabled') && el.offsetParent !== null)` re-applies the disabled check redundantly. It also misses elements made non-focusable via `aria-disabled="true"` and via `<fieldset disabled>` (a disabled fieldset disables descendants without putting `[disabled]` on each child).
**Fix:** Drop the redundant `[disabled]` filter and keep only the `offsetParent` visibility filter. If aria-disabled support matters, add `:not([aria-disabled="true"])` to the selector instead.

### IN-02: `aria-labelledby` references an ID that does not exist while the drawer is closed

**File:** `frontend/web/src/components/layout/Navbar.vue:177, 196`
**Issue:** `aria-controls="mobile-drawer"` on the hamburger button always refers to the drawer's id. While the drawer is closed, `v-if="mobileMenuOpen"` keeps the element out of the DOM and the id reference dangles. ARIA spec permits this and screen readers generally handle it, but `aria-controls` is most useful when the controlled element exists. The dialog-internal `aria-labelledby="mobile-drawer-title"` is fine because both the dialog and the title are inserted together.
**Fix:** Either accept the (correct) dangling reference as-is, or switch the drawer's `v-if` to `v-show` so the id is always present. `v-show` would also avoid the `mount-on-open` focus-trap latency caused by the existing `nextTick` workaround at line 63.

### IN-03: Docstring claims "~30 LOC" but file is ~76 LOC including comments

**File:** `frontend/web/src/composables/useFocusTrap.ts:10`
**Issue:** Cosmetic — the JSDoc at line 10 says "~30 LOC" and the plan said the same. The actual file is 76 lines including a 16-line doc block and a 5-line interface. Just update the comment so future readers don't think the file has bloated.
**Fix:** Change `~30 LOC` to `~50 LOC` (or just drop the line-count claim from the doc block).

---

_Reviewed: 2026-05-13_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
