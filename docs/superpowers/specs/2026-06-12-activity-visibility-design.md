# Activity Visibility («Скрыть активность») — Design

**Date:** 2026-06-12
**Status:** Approved (owner request: «Добавить "скрыть активность" в настройки юзера (скрыть всю / скрыть только хентай)»)
**UXΔ** = +2 (Better) — users gain control over what others see; default unchanged.
**CDI** = 0.04 * 5 — narrow seams (auth user model, player public read paths, one settings card).
**MVQ** = Sprite 90%/85% — small, well-bounded privacy toggle.

## Problem

A user's watch behaviour leaks to other users through three public surfaces:

1. **Global activity feed** — `GET /activity/feed` (player), rendered by
   `ActivityFeed.vue` on the home page. Events: list-status changes, reviews,
   comments — each with username + anime title/poster.
2. **Public watchlist** — `GET /users/{userId}/watchlist/public` (+ `/stats`),
   rendered on the public profile page. Today the `public_statuses` filter is
   applied **client-side only**.
3. *(Out of scope)* Reviews/comments rendered on an anime's own page, and the
   aggregate `watchers-count`. Reviews/comments are deliberate public posts
   shown only on that title's page; watchers-count is an anonymous aggregate.

There is no way to opt out entirely, and no way to keep 18+ (hentai) titles
out of one's public activity while keeping the rest visible.

## Setting

New per-user setting `users.activity_visibility` (owned by the auth service,
column auto-added by GORM AutoMigrate with default):

| Value | RU label | Meaning |
|-------|----------|---------|
| `all` (default) | Показывать всю активность | Current behaviour. |
| `non_hentai` | Скрывать 18+ (хентай) | 18+ titles are excluded from the user's public activity everywhere. |
| `none` | Скрыть всю активность | The user produces no publicly visible activity. |

Endpoint: `PUT /auth/profile/activity-visibility` `{ "activity_visibility": "..." }`
(authenticated, mirrors the existing `PUT /auth/profile/timezone` pattern).
Value validated against the closed set. Returned in `/auth/me` and login
responses (the user's own object).

## Privacy-by-design — the setting itself must not leak

`PublicUser` (the profile other users see) does **not** include
`activity_visibility`. Exposing `non_hentai` would itself reveal that the user
watches hentai. Consequences:

- `non_hentai` output must be **indistinguishable** from `all` minus the
  hentai rows — no flags, no "N entries hidden" hints.
- For `none`, `User.ToPublic()` returns an empty `public_statuses` array, so
  the existing frontend renders its existing "no public lists" state with zero
  frontend changes and no new signal.

## Enforcement (server-side, read-time)

Read-time filtering (not write-time suppression) so toggling the setting
retroactively hides/unhides historical events and is fully reversible.
All services share one Postgres database; the player service already reads
the `users` table directly (`fetchUserAvatars`), so the same applies here.

### Hentai predicate (player service, SQL)

An anime is 18+ when `animes.rating = 'rx'` **or** it carries the `Hentai`
genre (the frontend's existing `isHentai` check uses the genre; in the live DB
rating `rx` covers the genre set almost exactly — 219 rx vs 214 genre rows,
5 rx-only):

```sql
EXISTS (
  SELECT 1 FROM animes a
  LEFT JOIN anime_genres ag ON ag.anime_id = a.id
  LEFT JOIN genres g ON g.id = ag.genre_id
  WHERE a.id = <X>.anime_id AND (a.rating = 'rx' OR g.name = 'Hentai')
)
```

### Activity feed — `ActivityRepository.GetFeed`

`LEFT JOIN users u ON u.id = activity_events.user_id` plus:

```sql
COALESCE(u.activity_visibility, 'all') <> 'none'
AND NOT (COALESCE(u.activity_visibility, 'all') = 'non_hentai' AND <hentai predicate>)
```

LEFT JOIN + COALESCE keeps current behaviour for orphaned events and for rows
predating the column. Filtering happens before LIMIT, so pagination stays
correct.

### Public watchlist + stats — `ListService`

`GetPublicWatchlistPaginated`, `GetPublicWatchlistStats` (and the legacy
non-paginated `GetPublicWatchlist`) first resolve the target user's
`activity_visibility` (one indexed-PK query, default `all` on any error):

- `none` → empty page / zero stats (server-side; previously the
  `public_statuses` filter was client-side only).
- `non_hentai` → repo queries gain a `NOT EXISTS` hentai-predicate clause via
  a new `excludeHentai bool` parameter on `GetByUserPaginated`,
  `GetByUserAndStatusesPaginated`, `GetUserWatchlistStats`. Own-list paths
  pass `false` — the owner always sees everything.

## Frontend

Settings → existing Privacy block in `Profile.vue` gains a radio group
(«Видимость активности»: three options) below the public-status checkboxes;
the existing «Сохранить» button saves both (`updatePrivacy` +
new `userApi.updateActivityVisibility`). New i18n sub-namespace
`profile.activityVisibility.*` in both `en.json` and `ru.json`.

## Alternatives considered

- **Write-time suppression** (don't create feed events): not retroactive, not
  reversible — rejected.
- **Folding into `PUT /auth/profile/privacy`**: that request validates
  `public_statuses` as required; a dedicated endpoint matches the established
  one-route-per-field pattern (timezone, avatar, public-id) — chosen.
- **`is_hentai` boolean column on animes**: a denormalized flag needs a
  backfill + parser changes; the `rating`/genre predicate is already populated
  by Shikimori ingest — rejected for now.

## Testing

- auth: service validation (closed set), `ToPublic` blanking for `none`.
- player: SQLite repo tests — feed filtering for all three values (test setup
  gains `users`, `anime_genres`, `genres` tables); list service tests for the
  `none` short-circuit and `excludeHentai` plumbing via existing fakes.
- frontend: vitest run for touched specs + `bunx tsc --noEmit`; locale parity
  is enforced by adding keys to both files.
