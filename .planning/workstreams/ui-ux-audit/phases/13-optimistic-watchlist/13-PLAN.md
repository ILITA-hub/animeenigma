# Phase 13 Plan: Optimistic UI on watchlist

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Scope: 1 store + 3 consumers + 1 toast helper + 3 locale files. Closes UX-27.

## Tasks

### Store actions (foundation)

- [ ] Add `setStatusOptimistic(animeId, newStatus)` to `stores/watchlist.ts`:
  - Lookup existing entry; capture `prior` (entry or null).
  - Update `statusEntries` in place: if entry exists update its `status`, else push new `{ anime_id, status }`.
  - Call `userApi.addToWatchlist(animeId, newStatus)` if `prior===null`, else `userApi.updateWatchlist(animeId, newStatus)`.
  - On error: restore `prior` into `statusEntries` (or remove the pushed entry), then re-throw so callers can show toast.
- [ ] Add `setScoreOptimistic(animeId, newScore)` to `stores/watchlist.ts`:
  - Same shape, calls `userApi.updateListEntry({ anime_id, score: newScore })`.
  - If no entry exists yet (user scores before adding), call `addToWatchlist(animeId, 'completed')` first, then update score. Edge case.
- [ ] Add `removeEntryOptimistic(animeId)` to `stores/watchlist.ts`:
  - Capture `prior` before removal.
  - Remove from `statusEntries`.
  - Call `userApi.removeFromWatchlist(animeId)`. On error, restore prior. Re-throw.
- [ ] Export the 3 new actions in the store's return object.

### Toast helper

- [ ] Create `frontend/web/src/composables/useToast.ts` — small composable wrapping a global reactive array of toasts. Methods: `push(message, type='error', duration=3000)` and `dismiss(id)`. Auto-dismiss on `duration` ms.
- [ ] Mount a `<Toaster />` component in `App.vue` (or extend the existing admin-banner area) that renders the toast list. Fixed position top-right, animated in/out.

### Consumer migration

- [ ] `AnimeContextMenu.vue` — wherever it currently calls `userApi.addToWatchlist`/`updateWatchlist`/`removeFromWatchlist`, replace with `watchlistStore.setStatusOptimistic(...)` / `watchlistStore.removeEntryOptimistic(...)`. Wrap in `try { await ... } catch { useToast().push(t('watchlist.errors.updateFailed')) }`.
- [ ] `Anime.vue` — same migration for the detail-page status dropdown + score input.
- [ ] `Profile.vue` — same migration for the watchlist tab status pills + score editor.
- [ ] Verify Profile.vue's local list re-renders when `watchlistMap` updates (it should — already uses the store).

### Score-input debounce (Anime.vue + Profile.vue)

- [ ] Use `useDebounceFn` from `@vueuse/core` (already imported in Navbar.vue) — debounce the score input handler 500ms.

### i18n (en/ru/ja)

- [ ] Add to each locale:
  - `watchlist.errors.updateFailed`: EN "Couldn't update — please retry" / RU "Не удалось обновить — повторите" / JA "更新できませんでした、再試行してください"
  - `watchlist.errors.removeFailed`: EN "Couldn't remove — please retry" / RU "Не удалось удалить — повторите" / JA "削除できませんでした、再試行してください"
  - Total: 2 × 3 = 6 entries.

### Verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` clean.
- [ ] `bash scripts/i18n-lint.sh` clean.
- [ ] `make redeploy-web` succeeds.
- [ ] grep `setStatusOptimistic\|setScoreOptimistic\|removeEntryOptimistic` in store + 3 consumers — confirms wired (3+ matches).
- [ ] grep `useToast` in consumers — confirms toast on failure.
- [ ] Manual smoke: signed in as `ui_audit_bot`, on `/profile` watchlist tab, click status pill → immediately updates; pause backend → trigger an update → rollback fires + toast.

## Files touched

```
frontend/web/src/stores/watchlist.ts                 (3 optimistic actions)
frontend/web/src/composables/useToast.ts             (new — small toast composable)
frontend/web/src/components/ui/Toaster.vue           (new — small Toaster component)
frontend/web/src/App.vue                             (mount Toaster)
frontend/web/src/components/anime/AnimeContextMenu.vue (migrate)
frontend/web/src/views/Anime.vue                     (migrate + debounced score)
frontend/web/src/views/Profile.vue                   (migrate + debounced score)
frontend/web/src/locales/en.json                     (+2 keys)
frontend/web/src/locales/ru.json                     (+2 keys)
frontend/web/src/locales/ja.json                     (+2 keys)
.planning/workstreams/ui-ux-audit/phases/13-optimistic-watchlist/
  13-CONTEXT.md
  13-PLAN.md
  13-SUMMARY.md       (written at execute end)
  13-VERIFICATION.md  (written at execute end)
```

## Closes

| Req | Surface | Mechanism |
|---|---|---|
| UX-27 | All watchlist consumers | Pinia store optimistic actions + rollback + toast on error |
