# Watchlist bulk edit (profile)

**Date:** 2026-06-13
**Status:** Approved (design)
**Author:** AI pair-session with project owner

## Summary

Add bulk operations to the profile watchlist (owner-only): select multiple
anime in the current page and either **change their status** or **remove them**
in one action. Selection works in both table and grid views, scoped to the
currently loaded page.

## Decisions (locked)

- **Operations:** bulk status change + bulk remove. (No bulk score — not requested.)
- **Selection scope:** current page only, plus a "select all on this page"
  control. No cross-page "select all N matching".
- **Views:** selection available in both table and grid.
- **Backend approach:** one endpoint, but the service loops over the existing
  per-item `UpdateListEntry` / `DeleteListEntry` rather than a raw bulk SQL
  `UPDATE … WHERE anime_id IN`. This preserves every per-item side effect
  (`started_at`/`completed_at` auto-set, `episodes` filled to total on
  completion, gacha title-completed credit, activity `status_change` event,
  rewatch-finale bump). Current-page scope bounds the loop (≤ page size), so it
  stays a single HTTP request.
- **Owner-only:** bulk editing is hidden/disabled on public profiles.

## Motivation

Users curating large lists currently change status / remove one row at a time
(per-row dropdown + remove). Bulk editing removes the repetitive per-row work.

## Backend (player service)

### Endpoint

`POST /users/watchlist/bulk` — authenticated, owner-only (operates on
`claims.UserID` so a user can only touch their own entries).

Request body:
```json
{ "anime_ids": ["uuid", "..."], "action": "set_status", "status": "completed" }
```
- `action`: `"set_status"` | `"remove"`
- `status`: required + validated when `action == "set_status"`; ignored otherwise.
- `anime_ids`: non-empty, capped at 200 (defensive bound; current-page scope is
  far smaller).

Response:
```json
{ "updated": 12, "failed": 0 }
```
Partial-failure tolerant: the loop continues on a per-item error, counting
successes vs failures; the call returns 200 with the counts (only a malformed
request / auth failure is a non-200).

### Validation (handler)

`parseBulkRequest`: bind JSON; reject empty `anime_ids`, unknown `action`,
`set_status` without a valid status. Valid statuses reuse the existing status
set (`watching`, `completed`, `plan_to_watch`, `on_hold`, `dropped`).

### Service

`BulkUpdate(ctx, userID, username, animeIDs []string, action, status string) (updated, failed int, err error)`:
- `set_status`: for each id, call existing `UpdateListEntry(ctx, userID, username, &UpdateListRequest{AnimeID: id, Status: status})`. Count successes/failures.
- `remove`: for each id, call existing `DeleteListEntry(ctx, userID, id)`. Count successes/failures.
- Returns an `err` only for a pre-loop validation failure; per-item errors are
  logged and counted into `failed`.

No new repo methods — the per-item service methods already encapsulate the
mutations. (Reuse over duplication.)

### Routing

Register `POST /users/watchlist/bulk` in the authenticated `/watchlist` group
in `transport/router.go`. It is a static path (no wildcard collision with
`/watchlist/{animeId}`).

## Frontend

### Selection model (Profile.vue, own profile only)

- `selectionMode: ref<boolean>` — toggled by a "Выделить" button in the controls row.
- `selectedIds: ref<Set<string>>` — anime_ids selected on the current page.
- Entering selection mode shows checkboxes; leaving it clears `selectedIds`.
- `selectedIds` is cleared whenever the page, status filter, search, or filters
  change, and after any bulk action completes (selection never spans pages).
- A "select all on this page" toggle selects/deselects every `anime_id` in the
  current `watchlist` page.

### Table view (`WatchlistRow.vue`)

- New props `selectable: boolean`, `selected: boolean`; new emit `toggleSelect`.
- When `selectable`, render a leading checkbox cell bound to `selected`.
- A header "select all on page" checkbox lives in Profile.vue's table header
  area (reflects all-selected / indeterminate state).

### Grid view (Profile.vue grid block)

- In selection mode, each grid item is wrapped so a checkbox overlay (top-left)
  shows, and clicking the card toggles selection instead of navigating
  (click interception at the wrapper level — `AnimeCard` itself is not changed).

### Bulk action bar (`WatchlistBulkBar.vue`)

- Floating bar at the bottom, visible only when `selectionMode && selectedIds.size > 0`.
- Contents: "Выбрано N", a status `Select` (emits `set-status`), a "Удалить"
  button (emits `remove`), and a "Снять выделение" button (emits `clear`).
- Props: `count: number`, `statusOptions: SelectOption[]`. Pure presentational
  (no API calls); Profile.vue owns the handlers.

### Handlers (Profile.vue)

- `bulkSetStatus(status)`: `userApi.bulkWatchlist({ anime_ids: [...selectedIds], action: 'set_status', status })`, then refetch the page + clear selection. Optimistically update visible rows' status first.
- `bulkRemove()`: confirm via the existing `useConfirm`; on confirm,
  `userApi.bulkWatchlist({ anime_ids: [...selectedIds], action: 'remove' })`,
  then refetch + clear selection.
- Errors surface a toast (reuse `watchlist.errors.*` or a new `profile.bulk.error`).

### API client

`bulkWatchlist(body: { anime_ids: string[]; action: 'set_status' | 'remove'; status?: string })`
→ `POST /users/watchlist/bulk`.

### i18n

`profile.bulk.*` keys in both `en.json` and `ru.json` (select button, "Выбрано {n}",
change-status label, delete, clear, confirm copy, error). Locale parity is test-enforced.

## Testing

- **Backend** (sqlite + testify, matching existing repo/service tests):
  - `set_status` updates all listed entries and preserves completion semantics
    (e.g. `completed` sets `completed_at`).
  - `remove` deletes all listed entries.
  - validation: empty ids, unknown action, `set_status` without valid status.
  - user-scoping: ids belonging to another user are not touched.
  - partial failure (a non-existent id) is counted into `failed`, others succeed.
- **Frontend**:
  - `WatchlistBulkBar.spec.ts`: emits `set-status`/`remove`/`clear`, renders count.
  - selection logic: select-all-on-page selects every page id; changing
    page/filter clears selection.

## Out of scope (YAGNI)

- Bulk score set.
- Cross-page "select all N matching".
- Undo for bulk remove (a confirm dialog guards it).

## Affected files (anticipated)

- `services/player/internal/domain/` — `BulkUpdateRequest` type (+ valid-status helper if not present)
- `services/player/internal/service/list.go` — `BulkUpdate`
- `services/player/internal/handler/list.go` — `BulkUpdateList` handler + parse
- `services/player/internal/transport/router.go` — route
- `services/player/internal/handler/*_test.go` or `service` test — bulk tests
- `frontend/web/src/api/client.ts` — `bulkWatchlist`
- `frontend/web/src/components/profile/WatchlistBulkBar.vue` (+ `.spec.ts`)
- `frontend/web/src/components/profile/WatchlistRow.vue` — selectable checkbox
- `frontend/web/src/views/Profile.vue` — selection state, grid overlay, wiring
- `frontend/web/src/locales/{en,ru}.json` — `profile.bulk.*`
