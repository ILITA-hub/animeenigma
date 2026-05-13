# Phase 13: Optimistic UI on watchlist - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, store-action refactor for optimistic UI)

<domain>
## Phase Boundary

Watchlist actions (status flip, score change, list add/remove) feel instant: UI state changes immediately, API call dispatches in background, rolls back with toast on error. Closes Tier E #9.

**Scope (3 consumers + 1 store):**
- `frontend/web/src/stores/watchlist.ts` — add optimistic mutation actions.
- `frontend/web/src/components/anime/AnimeContextMenu.vue` — context menu over cards (Home / Browse / Search).
- `frontend/web/src/views/Anime.vue` — detail-page status/score control.
- `frontend/web/src/views/Profile.vue` — watchlist tab status/score edits.

Out of scope: review text, notes, tags, MAL import — these are slower-by-nature and don't need optimistic UI.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**Store contract:**
- Add three new actions to the watchlist store:
  - `async setStatusOptimistic(animeId, newStatus): Promise<void>` — updates local `statusEntries` immediately, calls `userApi.addToWatchlist` (if new) or `updateWatchlist` (if existing). On 4xx/5xx error: rollback to prior status, push toast `watchlist.errors.updateFailed`.
  - `async setScoreOptimistic(animeId, newScore): Promise<void>` — same shape, hits `userApi.updateListEntry({ anime_id, score })`.
  - `async removeEntryOptimistic(animeId): Promise<void>` — removes locally, calls `userApi.removeFromWatchlist`, rollback restores the entry.
- Each action stores the prior value before mutation in a local `const prior = ...` for rollback.
- Concurrent edits: simple last-write-wins. If user spams toggle twice, the second mutation overrides the first; rollback applies to whichever ends in error.
- Refresh: do NOT refetch on success — the optimistic value is canonical (or the server returns the same value). Backend errors only.

**Consumer migration:**
- AnimeContextMenu: status-pill clicks → store.setStatusOptimistic(animeId, newStatus). No more `await userApi.* + emit('update')` chain.
- Anime.vue: status dropdown + score input → same pattern.
- Profile.vue: per-entry status pill + score field → same pattern. Profile's local watchlist array reactively follows the store map.

**Toast system:**
- Phase 12 added an inline banner pattern in `App.vue` for admin redirects. Reuse / extend that pattern for watchlist errors. If it's a one-off banner, that's fine; a richer toast system can wait for Phase 20.
- Toast message: `watchlist.errors.updateFailed` ("Could not update — please retry").

**i18n keys (en/ru/ja):**
- `watchlist.errors.updateFailed`
- `watchlist.errors.removeFailed`
- (2 keys × 3 locales = 6 entries)

### Locked from ROADMAP

- Single phase, no dependencies on other open phases.
- Sets up the optimistic pattern for Phase 17 (collections — admin curated lists, also benefits from optimistic UI).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `frontend/web/src/stores/watchlist.ts` — single source of truth for status map. Add 3 mutation actions here.
- `frontend/web/src/api/client.ts` — already has `addToWatchlist`, `updateWatchlist`, `updateListEntry`, `removeFromWatchlist`. No new API methods needed.
- AnimeContextMenu / Anime / Profile already import `useWatchlistStore`. Switching from direct `userApi.*` calls to store actions is mechanical.

### Established Patterns

- Pinia store actions return Promise<void>. Components await for UX feedback (e.g. close the context menu after the action completes — even if optimistic, the await still completes synchronously-ish after the local state update).
- Toast/banner: Phase 12's App.vue admin banner is the closest existing pattern.

### Integration Points

- No backend change. No new endpoint.
- Backend already returns 200/201 on success, 4xx/5xx on conflict/invalid — rollback hook triggers on `catch`.

</code_context>

<specifics>
## Specific Ideas

- Optimistic UI should "feel instant" — the local mutation happens BEFORE the network round-trip, so even on a slow connection the badge flips immediately. The user can't tell the difference between successful API and pending API unless the rollback fires.
- Rollback timing: API failures arrive in ~200ms-3s. Rollback should be ANIMATED (e.g. badge flips back) so the user understands the operation failed, not just "it changed for a sec and reverted silently".
- Score input: debounce 500ms before firing the API call to avoid 10 PUTs for "7 → 7 → 7" typing. Optimistic local state updates on every keystroke; API fires on debounced settle.

</specifics>

<deferred>
## Deferred Ideas

- Toast system overhaul (rich, queued, dismissible toasts) — Phase 20.
- Undo affordance on the rollback toast — Phase 20.
- Optimistic UI for notes / tags / review text — different UX (these are text-heavy, users expect saves to be explicit).

</deferred>
