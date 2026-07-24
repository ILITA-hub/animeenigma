# Gacha «Лудка»: массовая загрузка обложек + массовое редактирование карточек

**Date:** 2026-07-24 · **Status:** approved (owner picked Variant A)
**Metrics:** UXΔ = +3 (Better) · CDI = 0.03 * 8 · MVQ = Sprite 90%/85%

## Problem

Cards in `/admin/gacha` are created strictly one-by-one (dialog: name + source
title + rarity + single file/URL upload). There are no bulk operations at all.
With gacha now public, populating the library (each character = 4–5 rarity
variants, each its own image) is the production bottleneck. Owner asked for
mass editing of cards and a way to mass-upload cover art.

## Decision (Variant A — everything in the existing Cards tab)

One tool covers both pains: bulk upload creates **draft cards** that are then
finished in the same table that gains **inline editing + bulk actions**.
No new pages, no import wizard, no CSV.

### 1. Bulk upload → draft cards

- New **«Загрузить пачку»** button next to «Создать» in the Cards tab.
- Opens a small dialog with a drag&drop zone / multi-file picker (images only).
- Files upload with limited concurrency (3 at a time) and a progress counter
  («7 из 20»); per-file errors are listed with a retry button for failed ones.
- Each successful file becomes a card via the **existing** endpoints
  (`POST /api/gacha/admin/upload` multipart → `POST /api/gacha/admin/cards`):
  - `name` = file name without extension (trimmed; fallback `"Без имени"`
    if empty, since backend requires non-empty name)
  - `rarity` = `N`, `source_title` = `""`, `enabled` = **false** (draft —
    never enters pull pools until explicitly enabled)
- **No new backend endpoints for upload.** Admin-scale N+N requests are fine.

### 2. Cards table: selection + inline editing + bulk actions bar

- **Checkbox column** + master «select all» checkbox (selects the currently
  filtered set). Selection survives inline edits, resets on filter change.
- **Inline editing** in cells (no dialog round-trip):
  - name and source_title: click → input, Enter/blur saves via the new bulk
    endpoint with a single id (partial semantics — unlike the full-replace
    single PATCH, no other field can be clobbered)
  - rarity: compact select in the cell; enabled: checkbox in the cell —
    both save on change through the same single-id bulk call
  - The existing per-card edit dialog stays (images, card back, groups).
- **Bulk actions bar** appears when ≥1 selected (sticky above the table):
  «выбрано N» + actions:
  - set rarity (select N/R/SR/SSR)
  - set name (input + apply) — useful for «5 arts of one character» flow
  - set source_title (input + apply)
  - add to group (select of existing groups → existing
    `POST /api/gacha/admin/groups/{id}/cards`)
  - enable / disable
  - delete (with confirm dialog)
- Draft cards (enabled=false) get a visible «черновик» badge; the existing
  enabled filter serves as the drafts filter.

### 3. Backend: two new admin endpoints (`services/gacha`)

Existing `UpdateCardRequest` is full-replace; bulk needs partial semantics,
hence a dedicated request shape:

- `PATCH /api/gacha/admin/cards/bulk`
  ```json
  { "ids": ["uuid", ...],
    "set": { "name?": "…", "source_title?": "…", "rarity?": "SR", "enabled?": true } }
  ```
  - pointer fields — only present keys are applied (empty-string name is
    rejected; `source_title` may be set to empty)
  - validates: non-empty ids, all UUIDs, rarity valid when present, at least
    one field in `set`
  - single UPDATE across ids (soft-delete-scoped); response
    `{ "updated": <rows affected> }`
- `POST /api/gacha/admin/cards/bulk-delete`
  ```json
  { "ids": ["uuid", ...] }
  ```
  - same semantics as single `DeleteCard`: plain soft-delete, join rows stay
    (every join query already filters on `deleted_at`); response
    `{ "deleted": <n> }`
- Both admin-gated by the gateway exactly like the rest of `/api/gacha/admin/*`
  (prefix-routed — **no gateway change needed**).

### 4. Frontend API client (`src/api/gacha.ts`)

Add `bulkUpdateCards(ids, set)` and `bulkDeleteCards(ids)`.

## Error handling

- Bulk endpoints are all-or-nothing on validation (bad UUID / bad rarity /
  empty set → 400, nothing applied). Missing ids are silently skipped by the
  UPDATE (affected-count returned; FE refetches the list after every bulk op).
- Upload dialog: per-file failure does not abort the batch; failed files stay
  listed with the error and a retry button.
- Inline edit failure: toast + revert cell to server value.

## Testing

- BE: service-level unit tests for `BulkUpdateCards` / `BulkDeleteCards`
  (validation matrix, partial `set`, nonexistent ids affected-count,
  join-row cleanup on delete) in `content_test.go` style (sqlite).
- FE: extend `AdminGacha.spec.ts` — bulk bar visibility/actions wiring,
  draft creation from files (mocked API), inline edit save path.
- Gates: `/frontend-verify` (DS-lint, i18n en/ru/ja parity, real build,
  vue-tsc), `go test ./...` in `services/gacha`.

## Out of scope

- Shikimori character-art auto-fetch (possible future enhancement).
- CSV/archive import; separate import wizard.
- Bulk image/back replacement (per-card dialog still owns images).
