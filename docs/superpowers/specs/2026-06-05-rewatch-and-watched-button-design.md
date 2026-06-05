# Design — Watched-aware play button & Rewatch tracking

**Date:** 2026-06-05
**Status:** For review (tests written as TDD red; implementation pending)
**Author:** pairing session (0neymik0 + Claude)

---

## 1. Problem

On the anime page, the primary play button shows **"Продолжить с эп. {n}"** ("Continue from
ep. N") whenever the user has *any* watch progress — including anime they have **fully
watched**. For a finished show "Continue from ep. N" is wrong/redundant: there is nothing to
continue. The user wants the "Продолжить с" framing gone for watched anime, and a coherent
**Rewatch** concept that is tracked in the database, surfaced in the UI (anime page + My List),
and counted in list stats.

Today the button (`frontend/web/src/views/Anime.vue:121`) decides purely on
`lastEpisode ? continueEp : watchNow` and ignores both the resume state machine's `finished`
kind and the user's list status.

## 2. Goals

- The play-button label reflects **actual episode progress**, and distinguishes the
  fully-watched terminal state (mark-as-watched vs rewatch).
- A first-class **Rewatch** lifecycle: clicking Rewatch returns the entry to `watching`,
  resets the cycle, and on completion increments a persisted `rewatch_count`.
- `rewatch_count` is **shown** on the anime page and in My List, **editable** by the user, and
  **counted** in list stats.
- Recover the Shikimori `rewatches` value the importer currently discards.

## 3. Non-goals

- No change to the in-player `ResumePill` beyond reconciling it with the new top button.
- No new `rewatching` list status (we reuse `watching` + the existing `is_rewatching` flag).
- No per-episode rewatch analytics beyond the existing append-only `watch_history`.
- Furigana, subtitles, and other unrelated player concerns.

## 4. Governing principle

> **The button's verb is driven by actual episode progress (`watch_progress`), not by list
> status. List status only disambiguates the fully-watched terminal state:**
> not-completed + full → *mark-watched*; completed + full → *rewatch*.

`full` requires `total > 0`. Unknown-total shows (`episodes_count = 0`, common for ongoing or
loosely-mapped titles) can never be classified "full" and stay on Continue/Watch.

---

## 5. Part A — Play-button label (`computeWatchCta`)

A **pure function** owns the decision (testable in isolation; `Anime.vue` only renders it).

```ts
// frontend/web/src/composables/watchCta.ts
export type WatchCtaAction = 'watch' | 'start-from-1' | 'continue' | 'mark-watched' | 'rewatch'

export interface WatchCtaInput {
  isAuthenticated: boolean
  lastWatched: number     // highest completed ep; 0 if none
  totalEpisodes: number   // 0 if unknown → "full" unreachable
  listStatus: string | null
}

export interface WatchCta {
  action: WatchCtaAction
  startEpisode: number
  labelKey: string
  labelParams?: Record<string, number>
}

export function computeWatchCta(input: WatchCtaInput): WatchCta
```

### Decision matrix

`P` = `lastWatched` clamped to `[0, total]` when `total > 0`.

| Progress | list ≠ completed | list = completed |
|---|---|---|
| **P0** (0 watched) | `watch` → `anime.watchNow` | `start-from-1` → `anime.startFromEp1` |
| **Pp** (partial, x) | `continue` ep x+1 → `anime.continueEp {n}` | `continue` ep x+1 (real progress wins) |
| **Pf** (all, total>0) | `mark-watched` → `anime.markAsWatched` | `rewatch` → `anime.resume.rewatch` |

### Edge rules (pinned by tests)

- **Unknown total** (`total = 0`): never `Pf` → `continue`/`watch`/`start-from-1` only.
- **`lastWatched > total`** (data anomaly): clamp to `total`, treat as full.
- **Not in list** (`listStatus = null`) + full → `mark-watched` (clicking creates the entry
  and marks it completed).
- **Status label never changes the verb**: `dropped`/`on_hold` + partial → `continue`;
  `dropped` + full → `mark-watched`.
- **Anonymous** (`isAuthenticated = false`): no list, so never `mark-watched`/`rewatch`.
  `lastWatched=0` → `watch`; partial → `continue`; full → `watch` (cannot mark without an
  account).

### Wiring

`Anime.vue` replaces the inline `lastEpisode ? continueEp : watchNow` with
`computeWatchCta({ isAuthenticated, lastWatched, totalEpisodes, listStatus })`, rendering
`labelKey`/`labelParams` and dispatching on `action`:
- `mark-watched` → `setListStatus('completed')` (the existing path; auto-sets `completed_at`).
- `rewatch` → calls the new `Rewatch` endpoint, then activates the player at ep 1.
- others → activate the player at `startEpisode`.

The in-player `ResumePill` keeps its `finished` banner (mark-complete / find-similar) but its
rewatch action and the top button now share one code path.

---

## 6. Part B — Rewatch lifecycle (backend)

### Data model

`anime_list` gains one column (`services/player/internal/domain/watch.go`):

```go
// number of COMPLETED rewatches (MAL "times rewatched").
// Total times watched = 1 + RewatchCount.
RewatchCount int `gorm:"default:0" json:"rewatch_count"`
```

`is_rewatching bool` already exists and becomes the "rewatch in progress" flag.
GORM `AutoMigrate` adds the column on player restart (additive — safe).

### Lifecycle

```
[completed, episodes=total, rewatch_count=N]
        │  click "Пересмотреть"  → ListService.Rewatch(user, anime)
        ▼
RESET: status='watching', episodes=0, is_rewatching=true,
       watch_progress.completed=false & progress=0 (rows kept),
       rewatch_count UNCHANGED.            (watch_history audit trail preserved)
        │
        ▼
[watching, rewatch — My List: ↻ ×(N+1), progress 0/total]
   button: P0→watch/start-from-1 → Pp→continue ep x+1
        │  re-watch finale → IncrementEpisodes auto-completes (episodes_count>0)
        ▼
ON watching→completed WHILE is_rewatching:
       rewatch_count++, is_rewatching=false, completed_at=NOW()
        ▼
[completed, rewatch_count=N+1]  → button "Пересмотреть" again
```

Two new write points; everything else reuses existing code:

1. **`ListService.Rewatch(ctx, userID, animeID)`** — the RESET above. Idempotent-safe.
2. **`IncrementEpisodes`** (`repo/list.go:118`) auto-complete branch — when the transition to
   `completed` happens and `is_rewatching = true`, also `rewatch_count = rewatch_count + 1` and
   `is_rewatching = false`, in the same `UPDATE`.

A normal **first** completion (`is_rewatching = false`) must NOT touch `rewatch_count`.
Re-marking an already-completed finale must NOT double-count (idempotent).

### Edge: unknown total during rewatch

If `episodes_count = 0`, the auto-complete branch never fires, so a rewatch of an unknown-total
show never auto-increments — the count can only be set manually there. Accepted.

---

## 7. Part C — `rewatch_count` editing (3 sources)

1. **Auto** — `++` on rewatch completion (Part B).
2. **Import** — Shikimori `rewatches` (currently parsed then discarded,
   `handler/shikimori_import.go:36`). Extract a pure `buildShikimoriListReq(entry, animeID)`
   that maps `rewatches → RewatchCount` and `status=='rewatching' → IsRewatching`. Re-import is
   authoritative: a source value of `0` overwrites (non-nil), so a stale local count can't
   survive a fresh import. (MAL: map `num_times_rewatched` if the source exposes it; otherwise
   only the existing 0/1 `is_rewatching`.)
3. **Manual** — `UpdateListRequest` gains `RewatchCount *int` (nil = leave untouched). Clamped
   to `[0, MaxRewatchCount]` (`MaxRewatchCount = 9999`). Handled in `ListService.UpdateListEntry`.

### Shared component — `RewatchCounter.vue`

One component, reused on the **anime page** (beside the status badge) and **My List** rows
(grid + table). Pure prop-driven; **low visual weight**.

```ts
defineProps<{ count: number; editable?: boolean }>()
defineEmits<{ (e: 'update:count', value: number): void }>()
```

- **Read-only + count=0** → renders nothing (no clutter on never-rewatched anime).
- **Read-only + count>0** → muted `↻ N` ghost badge; no stepper.
- **Editable** → subtle `− N +` stepper (revealed on interaction); `−` never goes below 0.
- Semantic tokens only — no card/border/loud color. Emits `update:count`; the host PATCHes the
  list entry (debounced).
- Pure: identical props → identical output on both surfaces.

---

## 8. Stats

`GetUserWatchlistStats` (`repo/list.go:256`) lifetime episodes changes from `SUM(episodes)` to:

```sql
SUM(episodes * (1 + rewatch_count))
```

No `animes` join needed — a completed entry's `episodes == total`. So a completed rewatch
doubles its contribution (12-ep show watched twice = 24).

**Accepted transient dip:** during an active rewatch the entry is `watching`, `episodes`
reset to the new cycle position, and `rewatch_count` not yet bumped — so its contribution dips
(e.g. 3) and self-heals to the doubled value (24) when the rewatch completes. The `completed`
count likewise drops by one during the rewatch. Both are correct ("currently rewatching, not
completed now").

---

## 9. i18n

New keys in BOTH `frontend/web/src/locales/en.json` and `ru.json` (parity test enforces):

| Key | RU | EN |
|---|---|---|
| `anime.startFromEp1` | `Смотреть с 1 серии` | `Start from ep. 1` |
| `anime.markAsWatched` | `Отметить просмотренным` | `Mark as watched` |
| `anime.rewatchCount` (+aria) | `Пересмотрено: {n}` | `Rewatched: {n}` |

Reuse existing `anime.continueEp`, `anime.watchNow`, `anime.resume.rewatch`.

i18n key paths are string-typed — smoke-verify in a browser post-deploy (RU + EN).

---

## 10. File touch list

**Backend (`services/player`)**
- `internal/domain/watch.go` — `AnimeListEntry.RewatchCount`, `UpdateListRequest.RewatchCount`, `const MaxRewatchCount` *(stubs landed)*.
- `internal/service/list.go` — implement `Rewatch`; clamp + apply `RewatchCount` in `UpdateListEntry` *(Rewatch stub landed)*.
- `internal/repo/list.go` — `IncrementEpisodes` rewatch-aware increment; `GetUserWatchlistStats` formula.
- `internal/handler/shikimori_import.go` — implement `buildShikimoriListReq`, call it in the import loop *(stub landed)*.
- `internal/handler/list.go` — expose `POST /users/list/{animeId}/rewatch` (or equivalent) → `Rewatch`.

**Gateway** — route the new rewatch endpoint under `/api/users/*` (already proxied to player).

**Frontend (`frontend/web`)**
- `src/composables/watchCta.ts` — implement `computeWatchCta` *(stub landed)*.
- `src/views/Anime.vue` — wire button to `computeWatchCta`; add `RewatchCounter` near status; dispatch `mark-watched`/`rewatch`.
- `src/components/anime/RewatchCounter.vue` — implement *(stub landed)*.
- `src/views/Profile.vue` — mount `RewatchCounter` (editable) in My List grid + table rows.
- `src/api/client.ts` (+ watchlist store) — `rewatch()` call and `rewatch_count` in the list-update payload.
- `src/locales/{en,ru}.json` — new keys.

---

## 11. Test plan (TDD — all red, written first)

45 cases across 6 groups (34 drive implementation; 11 stay green):

1. **`computeWatchCta`** — `composables/__tests__/watchCta.spec.ts` (17).
2. **Rewatch lifecycle** — `service/list_rewatch_test.go` (7): reset, progress reset, history
   preserved, finale-increments-and-clears, first-completion-no-increment, idempotent, two
   sequential rewatches.
3. **Stats** — `service/list_rewatch_stats_test.go` (5): none/×1/×2/mixed + documented dip.
4. **Manual edit** — `service/list_rewatch_edit_test.go` (4): set, nil-untouched, clamp 0,
   clamp max.
5. **Import** — `handler/shikimori_rewatch_import_test.go` (4): maps count, zero overwrites,
   rewatching flag+count, preserves score/episodes/id.
6. **`RewatchCounter.vue`** — `components/anime/__tests__/RewatchCounter.spec.ts` (8): hidden at
   0 read-only, shows count, no stepper read-only, control at 0 editable, inc/dec emit, no
   negative, pure render.

Schema-mirror patches applied to 5 hand-rolled test `anime_list` schemas so GORM stays happy
with the new column.

---

## 12. Accepted edges / known limits

- Unknown-total shows never reach the `mark-watched`/`rewatch` terminal (button stays
  Continue/Watch); rewatch never auto-increments there (manual only).
- Lifetime-episode stat dips transiently during an active rewatch (self-heals on completion).
- Re-import overwrites a manually-edited `rewatch_count` (import is authoritative).

---

## 13. Project metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — removes the wrong "Continue from" framing on finished anime and adds
  a coherent, tracked rewatch loop; small but high-frequency surface.
- **CDI = 0.03 × 13** — low spread/shift (one new column, one pure fn, one shared component,
  two write points), moderate effort (cross-stack: player service + gateway route + two FE
  surfaces + i18n).
- **MVQ = Griffin 85%/80%** — well-scoped, reuses existing machinery (auto-complete, resume
  state, inline-edit pattern); slop-resistance from the 45 red tests pinning the contract.
