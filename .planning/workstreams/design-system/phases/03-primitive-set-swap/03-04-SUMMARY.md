---
phase: 03-primitive-set-swap
plan: 04
subsystem: frontend/design-system
tags: [design-system, dropdown-menu, reka-ui, context-menu, right-click, DS-LIB-08, DS-NF-04]
requires:
  - "@/components/ui DropdownMenu + DropdownMenuItem (Wave 3, Plan 03)"
provides:
  - "Native browser right-click restored on anime cards (cursor menu removed)"
  - "Anime-card kebab opens an anchored Reka DropdownMenu with the exact prior action set + auth-gating"
  - "ContextMenu.vue (bespoke cursor menu) retired"
affects:
  - "frontend/web/src/views/{Home,Browse,Schedule,Profile,Anime}.vue kebab flow"
  - "frontend/web/src/composables/useContextMenu.ts state model (anchorEl added; cursor open() removed)"
tech-stack:
  added: []
  patterns:
    - "Reka DropdownMenu anchored mode via :reference (virtual/element bounding-rect source)"
    - "DropdownMenuItem @select for activation; Reka-native roving focus replaces hand-rolled tabindex"
key-files:
  created:
    - frontend/web/src/components/anime/AnimeContextMenu.spec.ts
  modified:
    - frontend/web/src/composables/useContextMenu.ts
    - frontend/web/src/components/anime/AnimeContextMenu.vue
    - frontend/web/src/views/Home.vue
    - frontend/web/src/views/Browse.vue
    - frontend/web/src/views/Schedule.vue
    - frontend/web/src/views/Profile.vue
    - frontend/web/src/views/Anime.vue
  deleted:
    - frontend/web/src/components/ui/ContextMenu.vue
decisions:
  - "Dropped the hand-rolled roving-tabindex (itemEls/moveFocus/onItemKeydown) — Reka DropdownMenuItem provides keyboard nav natively (live gate must confirm Arrow/Home/End/Enter/Esc)."
  - "anchorEl typed as reka-ui ReferenceElement | null — kebab element on click, zero-size virtual element at touch point on long-press."
  - "x/y kept in useContextMenu state + AnimeContextMenu props for back-compat with existing :x/:y view bindings; positioning now flows through anchorEl."
metrics:
  tasks_completed: 3
  files_changed: 8
  commits: 2
  completed: 2026-06-02
---

# Phase 3 Plan 04: Right-Click Rework — Native Right-Click + Anchored DropdownMenu Summary

DS-LIB-08 intentional UX change: removed the custom right-click interception so the browser's
native context menu works again, retired the bespoke cursor-positioned `ContextMenu.vue`, and
rebuilt the anime-card action menu on the Wave-3 anchored Reka `DropdownMenu` — preserving the
exact action set, auth-gating, optimistic store calls, and per-handler emit ordering.

## What shipped

- **`useContextMenu.ts`**: deleted the cursor `open(event)` path (`event.preventDefault()` +
  `clientX/clientY`); added `anchorEl: ReferenceElement | null` to state. `openAtElement` sets it
  to the kebab element; `onTouchstart` (mobile long-press) builds a zero-size virtual element at
  the touch point. `x`/`y` retained for back-compat; `close`/touch handlers + scroll-dismiss kept.
- **`AnimeContextMenu.vue`**: replaced `<ContextMenu :visible :x :y>` with
  `<DropdownMenu :open="visible" :reference="anchorEl ?? undefined" align="start" side="right" :side-offset="4" @update:open="$emit('update:visible', $event)">`. Items render as
  `<DropdownMenuItem @select="activate(...)">`. Preserved header (poster/info/ratings), auth-gated
  `loginToManage` notice, the `actions` computed, per-kind icons, `itemClasses`, and ALL handlers.
  Added a new optional `anchorEl?: ReferenceElement | null` prop.
- **5 consumer views**: added `:anchor-el="contextMenu.anchorEl"` (Profile uses
  `profileContextMenu.anchorEl`). No `@contextmenu` bindings added — native right-click stays native.
- **`ContextMenu.vue`**: deleted (`git rm`); it had a single importer (AnimeContextMenu) and was
  never in the barrel.
- **Spec**: `AnimeContextMenu.spec.ts` asserts the action-set per auth/list state, the auth-gated
  notice, and emit ordering (statusChange/removeFromList synchronous; episodesChange only after the
  awaited update resolves, and not at all on rejection).

## Emit-ordering preservation (W-1)

Unchanged from the live component:
- `statusChange` (setStatus) and `removeFromList` emit BEFORE their optimistic store await.
- `episodesChange` (markNextWatched) emits AFTER a successful `updateWatchlistEntry` await, only on
  success. NOT moved before the await.

## Verification

- `bunx vue-tsc` (clean, after deleting `.tsbuildinfo`): **EXIT 0**
- `bunx vitest run` (full): **58 files / 799 tests passed**
- `bun run build`: **EXIT 0 (clean)**
- `main.css`: untouched across the wave (`git diff HEAD~2 --name-only | grep -c main.css` → 0)
- `grep "function open(" useContextMenu.ts` → none; `grep "ui/ContextMenu" src` → none;
  `ContextMenu.vue` deleted; `@contextmenu` in views → none; `DropdownMenu` present in AnimeContextMenu.

## Deviations from Plan

### Commit grouping (minor)

The plan specified 3 commits (Task 1 SFC trio, Task 2 views, Task 3 `git rm` chore). The
`git rm` deletion landed in the **same commit as the view wiring** (`75e4b795`) rather than as a
separate `chore` commit — a soft-reset/re-stage attempt couldn't separate the already-removed file
cleanly without risking the many unrelated dirty files from parallel sessions in the working tree.
Result: **2 commits this wave** instead of 3. All staging was explicit-path (no `git add -A`), all
commits carry the 3 co-authors, nothing pushed. Functionally equivalent and logically coherent
(deletion + de-wiring of the cursor menu belong together).

### Roving-tabindex removed (per plan's allowance)

Dropped the hand-rolled `itemEls`/`moveFocus`/`onItemKeydown` roving tabindex — Reka
`DropdownMenuItem` provides keyboard nav natively. The plan explicitly permits this; keyboard nav
must be confirmed in the live gate.

## DS-LIB-08(c) OtherSubsPanel — N/A (intentional)

OtherSubsPanel is a Modal-based subtitle picker, not a cursor/context menu, so there is no menu to
re-wire onto a trigger. Left untouched by design — documented so a verifier doesn't read DS-LIB-08(c)
and flag a false coverage gap.

## Commits (unpushed)

- `6322d955` feat(design-system/03): rebuild anime action-menu on Reka DropdownMenu, drop cursor right-click path (DS-LIB-08)
- `75e4b795` feat(design-system/03): wire kebab anchor through 5 views, native right-click restored (DS-LIB-08) — includes the ContextMenu.vue `git rm`

## Orchestrator browser gate (run AFTER deploy)

1. **Native right-click restored**: right-click an anime card (Home, Browse, a Profile watchlist
   card) → the BROWSER's native context menu appears. No custom cursor menu, no preventDefault.
2. **Kebab → Reka DropdownMenu** anchored to the kebab with the EXACT actions:
   - Authenticated: 5 status options (current one highlighted) + Remove (when listed) + Mark-next
     (when watching with a next episode) + Go-to-page + Open-in-new-tab. Confirm a status change,
     a remove, and a mark-next actually mutate the watchlist (optimistic + persisted), menu closes after.
   - Unauthenticated: only the login-to-manage notice + Go-to-page + Open-in-new-tab.
   - Header shows poster + title + year/episodes + ratings.
   - Keyboard: Arrow/Home/End/Enter/Esc navigate + activate + dismiss (Reka native roving focus).
3. **Mobile long-press**: long-press a card → action menu opens anchored at the touch point.
4. **All 5 views**: spot-check kebab on Home columns, Browse grid, Schedule list, Profile watchlist
   (with episodesWatched/Total → mark-next), Anime-detail related cards.
5. **Standing 5-surface smoke**: Home/Browse/Anime-detail/player/404 otherwise unchanged.
6. **6 composites untouched**: ButtonGroup/GenreFilterPopup/PaginationBar/SearchAutocomplete/
   Skeleton/Toaster still render normally (e.g. GenreFilterPopup popover on Browse, PaginationBar,
   the search box).

## Self-Check: PASSED

- AnimeContextMenu.spec.ts: FOUND
- useContextMenu.ts / AnimeContextMenu.vue modified: FOUND
- ContextMenu.vue: DELETED (confirmed)
- Commits 6322d955, 75e4b795: FOUND in git log
