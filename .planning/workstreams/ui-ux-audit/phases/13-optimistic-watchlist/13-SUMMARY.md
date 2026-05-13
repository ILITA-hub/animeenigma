---
phase: 13
plan: 1
subsystem: ui-ux-audit
tags: [frontend, vue3, pinia, optimistic-ui, i18n, watchlist]
requires: []
provides:
  - watchlist-store-optimistic-actions
  - useToast-composable
  - Toaster-global-component
  - watchlist-rollback-on-error
affects:
  - watchlist status flip from anime card context menu (Home/Browse/Search)
  - watchlist status flip from /anime/:id detail page
  - watchlist status pill + score editor from /profile watchlist tab
tech-stack:
  added: []
  patterns:
    - pinia-store-optimistic-mutation-with-prior-snapshot-rollback
    - composable-module-level-reactive-toast-queue
    - debounced-network-commit-on-rapid-edit-cycle
    - transition-group-fade-slide-toast-animation
key-files:
  created:
    - frontend/web/src/composables/useToast.ts
    - frontend/web/src/components/ui/Toaster.vue
  modified:
    - frontend/web/src/stores/watchlist.ts
    - frontend/web/src/App.vue
    - frontend/web/src/components/anime/AnimeContextMenu.vue
    - frontend/web/src/views/Anime.vue
    - frontend/web/src/views/Profile.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
decisions:
  - store-actions-own-mutation-and-rollback-callers-only-handle-toast
  - score-edge-case-no-entry-yet-defaults-to-completed-status-then-PUTs-score
  - emit-before-await-in-AnimeContextMenu-so-parent-grids-update-instantly
  - debounce-score-commit-500ms-via-useDebounceFn-from-vueuse
  - top-level-watchlist-namespace-not-nested-under-profile-because-shared-across-3-consumers
  - re-throw-from-store-action-so-caller-decides-toast-or-silent-rollback
  - Anime-vue-keeps-currentListStatus-view-mirror-rolled-back-explicitly-on-error
metrics:
  duration: ~30min
  completed: 2026-05-13
  commits: 7
  tasks_complete: 6
  tasks_total: 6
---

# Phase 13 Summary: Optimistic UI on watchlist

**One-liner:** Watchlist status/score/remove actions feel instant — Pinia store owns the optimistic mutation + rollback, a small `useToast` queue (rendered by a new `<Toaster />` mounted in App.vue) surfaces rollback errors, and three consumers (AnimeContextMenu, Anime.vue detail page, Profile.vue watchlist tab) now call the store actions instead of `userApi.*` directly.

## What landed

| Area | Mechanism |
|---|---|
| **Store contract** (UX-27 foundation) | Three new actions on `useWatchlistStore`: `setStatusOptimistic`, `setScoreOptimistic`, `removeEntryOptimistic`. Each captures `prior` (entry snapshot or null), mutates `statusEntries` in place, calls the appropriate `userApi` method, and on failure restores `prior` and re-throws so the caller can decide between toast + silent. Score's edge case (scoring an anime not on the list yet) is handled by adding as `'completed'` first, then `PUT`ing the score. |
| **Toast system** | `useToast()` composable returns a module-level reactive `toasts` ref + `push(message, type, duration)` + `dismiss(id)`. Auto-dismiss on `duration`. `<Toaster />` in `frontend/web/src/components/ui/Toaster.vue` is a small `aria-live="polite"` fixed-top-right list with TransitionGroup fade+slide animations. Mounted in `App.vue` between main content and footer. Error/success/info color variants ready for future phases. |
| **AnimeContextMenu migration** | `setStatus()` and `removeFromList()` swap `userApi.*` for `watchlistStore.set*Optimistic` / `removeEntryOptimistic`. `emit('statusChange'/'removeFromList')` + `closeWithReason('item')` fire **before** the await, so the parent grid's local mirror flips instantly. Toast on catch. `markNextWatched` (episodes flow) deliberately left as-is — out of UX-27 scope. |
| **Anime.vue migration** | `setListStatus()` and `removeFromList()` flip the view-local `currentListStatus` mirror first, close the dropdown, then call the store action. On error, explicitly roll back `currentListStatus` to the prior value (the store already rolled back its map) and toast. **Detail page has no list-score field** in the current UI (the dropdown is status-only), so no score handler here — see deviations. |
| **Profile.vue migration** | `updateAnimeStatus` and `removeFromWatchlist` flip the visible row immediately, then call the store action. `finishEditScore` is wrapped in a `useDebounceFn`-debounced commit (500 ms) so blur/re-edit cycles collapse to a single PUT. `updateAnimeDate` / `updateAnimeEpisodes` deliberately left as-is — date and episodes are out of UX-27 scope. |
| **i18n** | 2 new keys × 3 locales = 6 entries: `watchlist.errors.updateFailed`, `watchlist.errors.removeFailed`. Top-level `watchlist` namespace (not nested under `profile.*`) because the keys are shared across three consumer surfaces. |

## Plan deviations

**Anime.vue has no list-score input.** The plan called for "detail-page status dropdown + score input" handlers. The current Anime.vue UI exposes only a status dropdown (the dropdown listed in lines 170-200 — five status options + Remove) and a *review* score form (under the reviews tab — a separate feature with different semantics). No list-score input exists, so no score handler was migrated for Anime.vue. The Profile.vue watchlist tab is the sole surface where users edit their list score, and it got the debounced score handler.

**Score input "every-keystroke optimistic" adapted to blur-commit + debounce.** The plan's specifics describe "Optimistic local state updates on every keystroke; API fires on debounced settle." Profile.vue's score editor commits on blur/Enter (an `<input type=number>` that toggles in/out of edit mode), not on every keystroke — that's the existing UX. Adapted: the visible row score flips immediately on blur; the network commit is wrapped in `useDebounceFn(..., 500)` so that if the user rapidly re-edits (blur → re-focus → new blur) within 500 ms, only the final PUT fires. This preserves the existing edit affordance while still collapsing redundant network calls.

**Existing `userApi` import in AnimeContextMenu kept.** The plan suggested swapping all `userApi.*` calls; only `setStatus` + `removeFromList` were swapped because `markNextWatched` uses `userApi.updateWatchlistEntry` for the episodes-watched counter — that's the episodes flow, not the UX-27 status/score/remove surface. Out of scope.

**i18n-lint shows an intermittent missing-key warning** (different key each run, race in the script's bash heredoc). Verified via `python3` flatten that all keys are present in all three locales (en/ru/ja have the same key set). Pre-existing script flakiness, not introduced by Phase 13.

## Commits

| Commit | Subject | Files |
|---|---|---|
| `9535371` | feat(13): watchlist store optimistic actions + rollback | `stores/watchlist.ts` |
| `e4a376f` | feat(13): useToast composable + Toaster component + App mount | `composables/useToast.ts`, `components/ui/Toaster.vue`, `App.vue` |
| `60de25b` | feat(13): AnimeContextMenu uses optimistic store actions | `components/anime/AnimeContextMenu.vue` |
| `e9d7f56` | feat(13): Anime.vue uses optimistic store actions | `views/Anime.vue` |
| `632c38d` | feat(13): Profile.vue uses optimistic store actions (with debounced score) | `views/Profile.vue` |
| `93d12ab` | feat(13): watchlist error i18n keys | `locales/{en,ru,ja}.json` |
| `7bd4b53` | fix(13): remove duplicate useDebounceFn import in Profile.vue | `views/Profile.vue` |

7 commits; each independently revertable. Touched 9 files (2 new, 7 modified).

## Findings closure

| Req | Surface | Status | Mechanism |
|---|---|---|---|
| UX-27 | All 3 watchlist consumers | CLOSED | Pinia store optimistic actions + rollback + toast on failure; UI flips before the network round-trip; rollback animated via natural Vue reactivity (badge re-renders to prior state) |

**Phase 13 outcome:** PASSED. UX-27 closed across 3 frontend surfaces (AnimeContextMenu, Anime.vue, Profile.vue) via 1 store contract (`stores/watchlist.ts`) + 1 toast helper (`composables/useToast.ts` + `components/ui/Toaster.vue`) + 6 locale entries. Zero backend changes. Zero new dependencies (useDebounceFn from `@vueuse/core` already in the bundle).

## Self-Check: PASSED

- File `frontend/web/src/stores/watchlist.ts` — FOUND (3 optimistic actions exported)
- File `frontend/web/src/composables/useToast.ts` — FOUND (43 lines)
- File `frontend/web/src/components/ui/Toaster.vue` — FOUND (61 lines)
- File `frontend/web/src/App.vue` — FOUND (Toaster mounted)
- File `frontend/web/src/components/anime/AnimeContextMenu.vue` — FOUND (setStatus + removeFromList migrated)
- File `frontend/web/src/views/Anime.vue` — FOUND (setListStatus + removeFromList migrated)
- File `frontend/web/src/views/Profile.vue` — FOUND (3 handlers migrated + debounced score)
- Locales `frontend/web/src/locales/{en,ru,ja}.json` — FOUND (`watchlist.errors.updateFailed` + `watchlist.errors.removeFailed` in all three)
- Commit `9535371` — FOUND
- Commit `e4a376f` — FOUND
- Commit `60de25b` — FOUND
- Commit `e9d7f56` — FOUND
- Commit `632c38d` — FOUND
- Commit `93d12ab` — FOUND
- Commit `7bd4b53` — FOUND
