---
status: passed
phase: 13
phase_name: "Optimistic UI on watchlist"
verified: 2026-05-13
---

# Phase 13 Verification: Optimistic watchlist UI

## Must-have truths scorecard (per 13-PLAN.md)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `useWatchlistStore` exports `setStatusOptimistic(animeId, newStatus)` with prior-snapshot + rollback + re-throw | PASS | `frontend/web/src/stores/watchlist.ts` lines 89-119: captures `prior` from `findIndex`, mutates `statusEntries[idx]` or pushes new entry, calls `addToWatchlist` (prior===null) or `updateWatchlistStatus`, catches → restores prior → re-throws. |
| 2 | `useWatchlistStore` exports `setScoreOptimistic(animeId, newScore)` with edge case for "no entry yet → add as completed first" | PASS | `stores/watchlist.ts` lines 122-162: if `prior === null`, pushes `{anime_id, status:'completed', score:newScore}` → `addToWatchlist(animeId,'completed')` → `updateWatchlistEntry({...score:newScore})`. If prior exists, mutates score in place + PUT. Rollback restores prior or removes pushed entry. |
| 3 | `useWatchlistStore` exports `removeEntryOptimistic(animeId)` with prior capture + rollback | PASS | `stores/watchlist.ts` lines 164-186: captures `prior` before `splice`, calls `removeFromWatchlist`, on catch re-inserts at original index (or `length` if list shrank). Re-throws. |
| 4 | `useToast()` composable with `push(message, type, duration)` + auto-dismiss + `dismiss(id)` | PASS | `frontend/web/src/composables/useToast.ts` lines 27-44: module-level `toasts` ref, `push()` appends + schedules `window.setTimeout(() => dismiss(id), duration)`, `dismiss()` splices by id. Returns `{ toasts: readonly, push, dismiss }`. |
| 5 | `<Toaster />` component renders the queue with `aria-live="polite"`, fixed top-right, TransitionGroup animation | PASS | `frontend/web/src/components/ui/Toaster.vue` lines 1-26: `aria-live="polite"`, `aria-atomic="true"`, `class="fixed top-20 right-4 z-50"`, `<TransitionGroup name="toast">`, error/success/info color variants via `toastClasses()`. CSS at lines 50-62: 200ms opacity+translateX fade-and-slide-in. |
| 6 | Toaster mounted in `App.vue` | PASS | `frontend/web/src/App.vue` line 44 (template): `<Toaster />` between `<main>` and `<footer>`. Line 73 (script): `import Toaster from '@/components/ui/Toaster.vue'`. |
| 7 | `AnimeContextMenu.vue` uses `watchlistStore.setStatusOptimistic` / `removeEntryOptimistic` + `useToast()` on error | PASS | `components/anime/AnimeContextMenu.vue` lines 328-358: `setStatus()` emits + closes menu first, then `await watchlistStore.setStatusOptimistic(animeId, status)`, catch → `toast.push(t('watchlist.errors.updateFailed'))`. `removeFromList()` same pattern with `removeEntryOptimistic` + `watchlist.errors.removeFailed`. |
| 8 | `Anime.vue` uses `watchlistStore.setStatusOptimistic` / `removeEntryOptimistic` + `useToast()` on error | PASS | `views/Anime.vue` lines 1843-1875: `setListStatus()` flips `currentListStatus.value = status` first, closes dropdown, awaits `setStatusOptimistic`, catch → rolls back local mirror + `toast.push('watchlist.errors.updateFailed')`. `removeFromList()` same with `removeEntryOptimistic`. |
| 9 | `Profile.vue` uses `watchlistStore.setStatusOptimistic` / `setScoreOptimistic` / `removeEntryOptimistic` + debounced score commit (500ms) | PASS | `views/Profile.vue` lines 1749-1851: `updateAnimeStatus` → setStatusOptimistic with row-local rollback; `finishEditScore` flips score → `debouncedCommitScore(500ms)` → `setScoreOptimistic`; `removeFromWatchlist` → drops row → `removeEntryOptimistic` → re-insert at original index on failure. All three toast on error. |
| 10 | i18n: 2 new keys × 3 locales = 6 entries; JSON parses clean | PASS | `watchlist.errors.updateFailed` + `watchlist.errors.removeFailed` present in en/ru/ja. Python `json.load` validates all three files. |

**Overall status:** PASSED — 10/10 must-have truths met.

## Artifact verification (per 13-PLAN.md "Files touched")

| Artifact | Path | Contains-check | Status |
|---|---|---|---|
| Watchlist store optimistic actions | `frontend/web/src/stores/watchlist.ts` | `setStatusOptimistic`, `setScoreOptimistic`, `removeEntryOptimistic` | FOUND (6 matches — 3 fn definitions + 3 return-object entries) |
| useToast composable | `frontend/web/src/composables/useToast.ts` | `push`, `dismiss`, `toasts` (new file) | FOUND |
| Toaster component | `frontend/web/src/components/ui/Toaster.vue` | `aria-live`, `TransitionGroup`, `useToast` (new file) | FOUND |
| App.vue Toaster mount | `frontend/web/src/App.vue` | `<Toaster />` | FOUND |
| AnimeContextMenu migration | `frontend/web/src/components/anime/AnimeContextMenu.vue` | `setStatusOptimistic`, `removeEntryOptimistic`, `useToast` | FOUND (4 matches — 2 store calls + import + var) |
| Anime.vue migration | `frontend/web/src/views/Anime.vue` | `setStatusOptimistic`, `removeEntryOptimistic`, `useToast` | FOUND (4 matches) |
| Profile.vue migration + debounced score | `frontend/web/src/views/Profile.vue` | `setStatusOptimistic`, `setScoreOptimistic`, `removeEntryOptimistic`, `useToast`, `useDebounceFn` | FOUND (6 matches) |
| en.json | `frontend/web/src/locales/en.json` | `watchlist.errors.updateFailed`, `watchlist.errors.removeFailed` | FOUND (2 keys) |
| ru.json | `frontend/web/src/locales/ru.json` | same | FOUND (2 keys) |
| ja.json | `frontend/web/src/locales/ja.json` | same | FOUND (2 keys) |

## Test results

### Frontend type-check

```
$ cd frontend/web && bunx vue-tsc --noEmit
(clean — no output, exit 0)
```

Initial run flagged duplicate `useDebounceFn` import in Profile.vue (TS2300); fixed in commit `7bd4b53`, re-ran clean.

### i18n-lint

```
$ bash scripts/i18n-lint.sh
=== Summary ===
  Missing keys:    0..1 (intermittent — different key each run, pre-existing script race)
  Syntax errors:   0
  Hardcoded text:  20 (warning, pre-existing in HanimePlayer + EnglishPlayer + i18n-y comments)
  Unused keys:     16 (warning, pre-existing)
```

The intermittent missing-key warning reports a different key on every run (e.g. `anime.resume.notYetAvailable`, `search.placeholder`) and clears on re-run. Verified via `python3` flatten that **all keys are present in all three locales** (en/ru/ja have identical key sets). This is a pre-existing script flakiness in the bash heredoc handling, not introduced by Phase 13.

### JSON validity

```
$ python3 -c "
import json
for loc in ['en','ru','ja']:
    json.load(open(f'frontend/web/src/locales/{loc}.json'))
print('ok')
"
ok
```

### Deploy + health

```
$ make redeploy-web 2>&1 | tail -5
docker compose -f docker/docker-compose.yml up -d --no-deps web
 Container animeenigma-web Started
Web frontend redeployed

$ make health
Checking service health...
✓ gateway:8000
✓ auth:8080
✓ catalog:8081
✓ streaming:8082
✓ player:8083
✓ rooms:8084
✓ scheduler:8085
✓ scraper:8088
```

All 8 services healthy after redeploy.

### Grep verification (per plan)

**Optimistic-action wiring (≥3 matches expected across store + 3 consumers):**

```
$ grep -nE "setStatusOptimistic|setScoreOptimistic|removeEntryOptimistic" \
    frontend/web/src/stores/watchlist.ts \
    frontend/web/src/components/anime/AnimeContextMenu.vue \
    frontend/web/src/views/Anime.vue \
    frontend/web/src/views/Profile.vue
frontend/web/src/stores/watchlist.ts:89:  const setStatusOptimistic = ...
frontend/web/src/stores/watchlist.ts:122:  const setScoreOptimistic = ...
frontend/web/src/stores/watchlist.ts:164:  const removeEntryOptimistic = ...
frontend/web/src/stores/watchlist.ts:201:    setStatusOptimistic,
frontend/web/src/stores/watchlist.ts:202:    setScoreOptimistic,
frontend/web/src/stores/watchlist.ts:203:    removeEntryOptimistic,
frontend/web/src/views/Anime.vue:1851:    await watchlistStore.setStatusOptimistic(animeId, status)
frontend/web/src/views/Anime.vue:1868:    await watchlistStore.removeEntryOptimistic(animeId)
frontend/web/src/views/Profile.vue:1758:    await watchlistStore.setStatusOptimistic(animeId, newStatus)
frontend/web/src/views/Profile.vue:1791:    await watchlistStore.setScoreOptimistic(animeId, score)
frontend/web/src/views/Profile.vue:1842:    await watchlistStore.removeEntryOptimistic(animeId)
frontend/web/src/components/anime/AnimeContextMenu.vue:338:    await watchlistStore.setStatusOptimistic(animeId, status)
frontend/web/src/components/anime/AnimeContextMenu.vue:353:    await watchlistStore.removeEntryOptimistic(animeId)
```

13 matches — well above the ≥3 threshold. Confirms wiring across store + 3 consumers.

**Toast on failure (≥3 consumers expected):**

```
$ grep -nE "useToast" \
    frontend/web/src/components/anime/AnimeContextMenu.vue \
    frontend/web/src/views/Anime.vue \
    frontend/web/src/views/Profile.vue \
    frontend/web/src/components/ui/Toaster.vue
frontend/web/src/components/anime/AnimeContextMenu.vue:149:import { useToast } from '@/composables/useToast'
frontend/web/src/components/anime/AnimeContextMenu.vue:200:const toast = useToast()
frontend/web/src/components/ui/Toaster.vue:29:import { useToast, type ToastType } from '@/composables/useToast'
frontend/web/src/components/ui/Toaster.vue:31:const { toasts, dismiss } = useToast()
frontend/web/src/views/Anime.vue:1043:import { useToast } from '@/composables/useToast'
frontend/web/src/views/Anime.vue:1101:const toast = useToast()
frontend/web/src/views/Profile.vue:1010:import { useToast } from '@/composables/useToast'
frontend/web/src/views/Profile.vue:1074:const toast = useToast()
```

3 consumers + Toaster renderer — meets the plan's "confirms toast on failure" check.

### i18n-key spot check across locales

```
$ python3 -c "
import json
for loc in ['en','ru','ja']:
    with open(f'frontend/web/src/locales/{loc}.json') as f:
        d = json.load(f)
    w = d['watchlist']['errors']
    assert w['updateFailed'] and w['removeFailed']
print('all 2 keys × 3 locales present')
"
all 2 keys × 3 locales present
```

## Commits on `main`

| Commit | Subject | Files |
|---|---|---|
| `9535371` | feat(13): watchlist store optimistic actions + rollback | `stores/watchlist.ts` |
| `e4a376f` | feat(13): useToast composable + Toaster component + App mount | `composables/useToast.ts`, `components/ui/Toaster.vue`, `App.vue` |
| `60de25b` | feat(13): AnimeContextMenu uses optimistic store actions | `components/anime/AnimeContextMenu.vue` |
| `e9d7f56` | feat(13): Anime.vue uses optimistic store actions | `views/Anime.vue` |
| `632c38d` | feat(13): Profile.vue uses optimistic store actions (with debounced score) | `views/Profile.vue` |
| `93d12ab` | feat(13): watchlist error i18n keys | `locales/{en,ru,ja}.json` |
| `7bd4b53` | fix(13): remove duplicate useDebounceFn import in Profile.vue | `views/Profile.vue` |

7 commits total; each independently revertable.

## Requirement closure

| Req | Surface | Mechanism | Status |
|---|---|---|---|
| UX-27 | AnimeContextMenu / Anime.vue / Profile.vue | Pinia store optimistic actions (`setStatusOptimistic`, `setScoreOptimistic`, `removeEntryOptimistic`) own the local mutation + API call + rollback; callers handle toast via `useToast()` + i18n keys `watchlist.errors.{updateFailed,removeFailed}`. Profile.vue's score editor is additionally wrapped in `useDebounceFn(500ms)` to collapse rapid edit cycles. | CLOSED |

**Phase 13 outcome:** PASSED — UX-27 closed across 3 watchlist consumer surfaces via 1 store contract + 1 toast helper + 6 locale entries. Zero backend changes, zero new dependencies, zero new lint debt.
