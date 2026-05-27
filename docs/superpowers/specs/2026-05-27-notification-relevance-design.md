# Relevance-Aware Notifications — Design

**Date:** 2026-05-27
**Workstream:** notifications
**Status:** Approved (design); pending implementation plan
**Author:** brainstorming session

## Problem

`new_episode` notifications are filtered for relevance only at **creation** time
(the hourly detector's `HotCombosCollector` join requires `anime_list.status =
'watching'`, `animes.status = 'ongoing'`, and computes
`first_unwatched = max_watched + 1`). But notification rows **persist after
creation** in `user_notifications`, and the user's state changes afterward:

- The user **watches** the episode the notification advertised → the notification
  is now stale, yet still shows as unread.
- The user **drops** the anime (any `anime_list` status change away from
  `watching`, or removes it from the list) → the notification is now irrelevant,
  yet still shows.

Result: the bell surfaces notifications the user has already acted on or no longer
cares about. (Observed: account `tNeymik` carried 4 unread notifications back to
2026-05-21, several for episodes already watched.)

## Goal

A `new_episode` notification should be shown **only while it is still relevant**:

1. **Still watching:** an `anime_list` row exists for `(user, anime)` with
   `status = 'watching'`.
2. **Not caught up:** the user's max watched episode **for the anime (any combo)**
   is `< latest_available_episode` (the newest episode the notification advertises).
   Watching the episode in *any* player/language/translation counts as watched —
   users rarely rewatch the same episode, and watching it in another combo signals
   a possible preference change rather than a need to be re-notified.

If either fails, the notification disappears from the dropdown **and** the bell
badge, and is eventually reaped from storage.

## Non-Goals

- **Toast re-fire on reload** (the in-memory `shownToastIds` resets on page load,
  re-toasting still-unread notifications). Distinct frontend bug; tracked
  separately. This work shrinks its blast radius (watched/dropped items stop
  re-toasting) but does not fix the genuinely-still-relevant-but-old re-toast.
- **UPSERT `created_at` preservation** (re-fired episodes keep the original
  `created_at`). Out of scope; noted only where it interacts with revival below.
- No new notification types. No cross-service event/queue plumbing.

## Decisions (locked)

| Decision | Choice |
|---|---|
| "Already watched" rule | **Caught up to latest, anime-level** — hide when the user's max watched episode for the anime (across **any** combo) `≥ latest_available_episode`. Partially-watched ranges stay visible. Combo is NOT part of the watched check. |
| "No longer watching" rule | **Only `status='watching'` shows.** Any other state (`dropped`, `completed`, `on_hold`, `plan_to_watch`) or removed-from-list → hide. |
| Enforcement | **Delivery filter + hourly cleanup** (hybrid). |
| Read-filter mechanism | **A — JSONB-in-SQL.** Extract combo + episode from `payload` in the WHERE clause; no producer/schema change for the *read* path. |
| Stale-state column | **Dedicated `invalidated_at`** (not reuse of `dismissed_at`) for clean telemetry and to distinguish system-retirement from user-dismissal. |

## Architecture

All data already lives in the shared `animeenigma` DB (D-01), so the notifications
service reads `anime_list` and `watch_history` directly through the existing
read-only views in `repo/views.go`. No new connections or services.

Two enforcement layers share **one** relevance predicate:

### Layer 1 — Read-time filter (authoritative, ~instant)

The predicate is applied to every user-facing read so the list and the badge
count always agree:

- `repo.NotificationRepository.List` — rows query **and** both counts
  (`unreadCount`, `total`).
- `repo.NotificationRepository.UnreadCount` — the bell-poll endpoint.

Base predicate becomes:

```
user_id = ?
AND dismissed_at IS NULL
AND invalidated_at IS NULL
AND <relevance predicate>
```

(`read_at IS NULL` additionally for the unread branch, as today.)

The live relevance predicate (correlated subqueries, applied only to
`new_episode` rows; all other types pass through untouched):

```sql
(
  n.type <> 'new_episode'
  OR (
    -- (1) still watching
    EXISTS (
      SELECT 1 FROM anime_list al
      WHERE al.user_id = n.user_id
        AND al.anime_id::text = n.payload->>'anime_id'
        AND al.status = 'watching'
    )
    -- (2) not caught up to the advertised latest (anime-level: any combo counts)
    AND COALESCE((
      SELECT MAX(wh.episode_number) FROM watch_history wh
      WHERE wh.user_id = n.user_id
        AND wh.anime_id::text = n.payload->>'anime_id'
    ), -1) < (n.payload->>'latest_available_episode')::int
  )
)
```

Notes:
- `anime_id::text = payload->>'anime_id'` (compare as text) avoids a per-row
  `::uuid` cast that would throw on a malformed payload — **fail-open**.
- The `::int` cast on `latest_available_episode` is guarded by a
  `~ '^[0-9]+$'` digit check; a malformed/absent value keeps the row (fail-open).
  Real `NewEpisodePayload` rows always carry a numeric value, so this only guards
  against corruption.
- The working set per user is tiny (single-digit unread rows), so the correlated
  subqueries are cheap; no extra index on the foreign tables is required.

The predicate lives in **one** place — a repo helper (e.g.
`relevantNewEpisode(db *gorm.DB) *gorm.DB` GORM scope, plus a raw-SQL constant for
the cleanup UPDATE) — reused by List, UnreadCount, and the invalidation job so the
three never drift.

### Layer 2 — Hourly invalidation job (housekeeping + telemetry)

A new `RelevanceInvalidationJob` runs on the **same hourly tick** as the detector
(invoked by `Scheduler` immediately after `runDetector`, so it sees freshly
upserted rows). It stamps `invalidated_at = NOW()` on currently-active
`new_episode` rows that are no longer relevant:

```sql
UPDATE user_notifications n
SET invalidated_at = NOW()
WHERE n.type = 'new_episode'
  AND n.dismissed_at IS NULL
  AND n.invalidated_at IS NULL
  AND NOT ( <relevance predicate body, correlated to n> )
```

- Returns rows-affected → increments a new counter
  `notifications_stale_invalidated_total` (in `job/metrics.go`).
- Idempotent: already-invalidated rows are skipped by `invalidated_at IS NULL`.
- Layer 1 already hides these between runs; Layer 2 persists the state for storage
  reclamation and telemetry, and short-circuits the read predicate.

### Revival (the `invalidated_at` subtlety)

The unique dedupe index **stays** `WHERE dismissed_at IS NULL` (unchanged) — it
deliberately still matches invalidated rows. So when a user re-adds a dropped
anime and a new episode fires, the detector's `Upsert` hits the existing row via
`ON CONFLICT DO UPDATE`. We add `invalidated_at = NULL` to the update assignments
(alongside the existing `read_at = NULL`):

```go
DoUpdates: clause.Assignments(map[string]interface{}{
    "payload":        datatypes.JSON(payload),
    "updated_at":     now,
    "read_at":        gorm.Expr("NULL"),
    "invalidated_at": gorm.Expr("NULL"),   // NEW — revive a retired row
    "type":           ntype,
}),
```

This revives a retired notification in place rather than orphaning the dedupe
slot. (Revival keeps the original `created_at` — the separate, out-of-scope
timestamp bug; not addressed here.)

## Data Model

Add one nullable column to `domain.UserNotification`:

```go
InvalidatedAt *time.Time `gorm:"index" json:"invalidated_at"`
```

- GORM `AutoMigrate` adds the column on boot (additive; existing rows = `NULL` =
  active). No backfill: Layer 1 starts filtering watched/dropped rows immediately;
  the first hourly run tombstones them.
- The API already serializes the full struct, so `invalidated_at` appears in the
  JSON response (harmless; frontend ignores it).

### Indexes (`repo.EnsureIndexes`)

- `uk_user_dedupe` — **unchanged** (`WHERE dismissed_at IS NULL`). Must keep
  matching invalidated rows for revival.
- `idx_user_unread` — tighten predicate to
  `WHERE dismissed_at IS NULL AND invalidated_at IS NULL` so the hot read path
  index matches the new base predicate. Predicate change requires
  `DROP INDEX IF EXISTS idx_user_unread` then re-`CREATE` (a bare
  `CREATE INDEX IF NOT EXISTS` is a no-op against the old predicate). Safe +
  idempotent on this small table.

### Retention (`DismissedRetentionCleanupJob`)

Extend the nightly DELETE to also reap long-retired rows, keeping the pgx-safe
`INTERVAL '1 day' * ?` form:

```sql
DELETE FROM user_notifications
WHERE (dismissed_at   IS NOT NULL AND dismissed_at   < NOW() - (INTERVAL '1 day' * ?))
   OR (invalidated_at IS NOT NULL AND invalidated_at < NOW() - (INTERVAL '1 day' * ?))
```

## Frontend

**No changes required.** The store polls the same endpoints; filtered-out rows
simply stop appearing and `unread_count` drops on the next 60 s poll. The toast
picker (`latestUndismissedToast`) iterates the already-filtered unread list, so
stale notifications also stop toasting.

## Error Handling / Edge Cases

| Case | Behavior |
|---|---|
| Watched to latest in any combo | Hidden (rule 2 — anime-level). |
| Watched the episode in a *different* combo (e.g. Kodik, notified for AniLib) | Hidden — "watched" is anime-level; any combo counts. |
| Partially watched range (watched 7, 7–9 out) | Still shown (newer episodes unwatched). |
| Anime dropped / completed / on_hold / plan_to_watch | Hidden (rule 1). |
| Anime removed from list entirely | Hidden (no `watching` row). |
| Malformed / missing payload field | Row kept (fail-open). |
| Re-add dropped anime → new episode fires | `Upsert` revives the row (`invalidated_at`/`read_at` cleared). |
| DB read error in subquery | Propagated as today (`CodeInternal`); no silent partial results. |

## Testing

Repo-level table tests against the relevance predicate (List + UnreadCount):

- watching + behind (anime-level `max_watched < latest`) → **show**
- watching + caught-up (anime-level `max_watched == latest`) → **hide**
- watching + caught-up (anime-level `max_watched > latest`) → **hide**
- partial range (`first ≤ max_watched < latest`) → **show**
- watched the episode in a *different* combo (different player/lang/type/translation) → **hide** (watched is anime-level)
- dropped / completed / on_hold / plan_to_watch → **hide**
- not in `anime_list` → **hide**
- `type <> 'new_episode'` → **always show**
- `invalidated_at` set → **hide**
- malformed payload → **show** (fail-open)
- counts consistency: `unread_count` / `total` match the filtered row set

Job tests:

- `RelevanceInvalidationJob`: stale active rows get `invalidated_at` + counter
  increments; relevant rows untouched; already-invalidated untouched (idempotent).
- `Upsert` revival: invalidated row + re-upsert → `invalidated_at` and `read_at`
  cleared, row visible again.
- Retention: rows with `invalidated_at`/`dismissed_at` older than N days deleted;
  recent kept.

All tests use handwritten fakes / miniredis-style table tests per existing
notifications-service convention (no testify/mock). Run with:
`cd services/notifications && go test ./... -count=1 -race`.

## Deployment

1. Add `InvalidatedAt` to the domain struct → `AutoMigrate` adds the column.
2. `EnsureIndexes` drops + recreates `idx_user_unread` with the tightened
   predicate.
3. `make redeploy-notifications`.
4. No data backfill. Read filter takes effect immediately; first hourly run
   tombstones existing stale rows; nightly retention reaps after the window.
5. Rollback: feature is read-path + an additive column + a housekeeping job;
   reverting the binary restores prior behavior (the `invalidated_at` column and
   any stamped values are simply ignored by the old code, which never reads them).

## Metrics (project convention — `.planning/CONVENTIONS.md`)

- **UXΔ = +3 (Better)** — the bell becomes trustworthy: it only shows episodes the
  user can actually still act on. Removes a recurring "why is this still here?"
  irritation.
- **CDI = 0.04 * 8** — Spread×Shift small (contained to the notifications service
  backend: repo read path, one new job, one column, two index/retention tweaks;
  zero frontend, zero other services). Effort_Fib 8.
- **MVQ = Phoenix 88%/85%** — thematically a Phoenix: notifications are *retired*
  (`invalidated_at`) and *reborn* (revival on re-upsert). High match to the
  retire/revive lifecycle; strong slop-resistance (single shared predicate, no
  duplicated relevance logic to drift).
