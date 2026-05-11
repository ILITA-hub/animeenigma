# Notifications Engine — Design

**Date:** 2026-05-11
**Status:** Approved for planning
**Author:** Claude (with 0neymik0)

## Goal

Build a generic in-app notifications engine for AnimeEnigma. First concrete notification type: **new episode of an ongoing anime is available on the same player + translation the user already used**. Engine is designed to support future types (comments, friend activity, admin announcements) without schema changes.

## Non-goals

- Off-tab push notifications (Web Push API, Service Workers) — out of scope for v1
- Email or Telegram delivery — out of scope for v1
- WebSocket/SSE real-time delivery — v1 uses HTTP polling
- Notification preferences UI (per-type opt-out) — out of scope for v1; can be added later via metadata column

## Constraints / Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Combo match strictness | **Strict** — exact `(player, language, watch_type, translation_id)` | User wants predictable behavior; no surprise translations |
| UI surfaces | **Toast + bell with dropdown** | Full notification engine, reusable for future types |
| Detection cadence | **Cron-job every hour** | Acceptable latency, manageable parser load |
| Scope of anime monitored | **Only `anime_list.status = 'watching'`** | Avoid spam from one-off sample-watches |
| Batching | **Aggregate consecutive episodes in 1 notification** | Click goes to first unwatched |
| Service placement | **New `services/notifications/`** | Player service already large with growing rec engine |
| Cross-service data reads | **Direct shared-Postgres reads** for `watch_history`, `anime_list`, `animes` | Standard project pattern; parser calls go via catalog HTTP |
| Kodik inclusion | **Included** | Kodik API exposes `last_episode` per translation; iframe limitation only affects playback observability, not availability discovery |

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                          Frontend (Vue 3)                        │
│  ┌───────────────┐   ┌──────────────┐   ┌──────────────────┐    │
│  │ NotifyToast   │   │ NotifyBell + │   │ useNotifications │    │
│  │   (App.vue)   │◄──┤   Dropdown   │◄──┤      store       │    │
│  └───────────────┘   └──────────────┘   └────────┬─────────┘    │
└─────────────────────────────────────────────────┬┴───────────────┘
                                                  │ HTTP polling 60s
                                                  ▼
                              ┌───────────────────────────────────┐
                              │       Gateway (8000)              │
                              │  /api/notifications/* → :8087     │
                              └────────────────┬──────────────────┘
                                               │
        ┌──────────────────────────────────────▼──────────────────────────┐
        │              services/notifications/ (port 8087)                │
        │                                                                  │
        │  ┌──────────────────┐  ┌──────────────────────────────────────┐│
        │  │  HTTP Handlers   │  │   NewEpisodeDetectorJob (cron 1h)    ││
        │  │  - list          │  │                                       ││
        │  │  - read/dismiss  │  │   1. Query hot combos (SQL)          ││
        │  │  - click         │  │   2. Call catalog /internal/episodes ││
        │  │  - mark-all-read │  │   3. Diff vs snapshot                ││
        │  └────────┬─────────┘  │   4. UPSERT notifications             ││
        │           │            │   5. Update snapshot                  ││
        │           │            └────────────────┬─────────────────────┘│
        │           ▼                              ▼                       │
        │  ┌─────────────────────────────────────────────────────────────┐│
        │  │             repo/  (GORM)                                    ││
        │  └────────┬─────────────────────────────────────────────────────┘│
        └───────────┼──────────────────────────────────────────────────────┘
                    │                              │
       ┌────────────▼──────┐                      │
       │  Shared Postgres  │                      │ HTTP
       │                   │                      │
       │ • user_notif.     │                      ▼
       │ • parser_snap.    │           ┌──────────────────────┐
       │ • watch_history   │           │  catalog service     │
       │ • anime_list      │           │  /internal/anime/... │
       │ • animes          │           │  /episodes (router   │
       └───────────────────┘           │   over 4 parsers)    │
                                       └──────────────────────┘
```

## Data Model

### Table: `user_notifications`

Owned by notifications service. Generic, type-pluggable.

```go
type UserNotification struct {
    ID          string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    UserID      string         `gorm:"type:uuid;not null;index:idx_user_unread"`
    Type        string         `gorm:"size:32;not null;index"`
    DedupeKey   string         `gorm:"size:255;not null"`
    Payload     datatypes.JSON `gorm:"type:jsonb;not null"`
    ReadAt      *time.Time
    DismissedAt *time.Time     `gorm:"index"`
    ClickedAt   *time.Time
    CreatedAt   time.Time      `gorm:"index"`
    UpdatedAt   time.Time
}
```

**Indexes:**
- `UNIQUE INDEX uk_user_dedupe ON user_notifications (user_id, dedupe_key) WHERE dismissed_at IS NULL` — UPSERT path for batching
- `INDEX idx_user_unread ON user_notifications (user_id, created_at DESC) WHERE dismissed_at IS NULL` — main read path

**Type-specific payload (`new_episode`):**
```json
{
  "anime_id": "uuid",
  "shikimori_id": "57466",
  "anime_title": "Frieren",
  "anime_poster_url": "https://...",
  "first_unwatched_episode": 14,
  "latest_available_episode": 16,
  "player": "animelib",
  "language": "ru",
  "watch_type": "dub",
  "translation_id": "9999",
  "translation_title": "AniLibria",
  "watch_url": "/anime/{uuid}/watch?player=animelib&episode=14&translation=9999"
}
```

**Dedupe key format for `new_episode`:**
```
new_episode:{anime_id}:{player}:{language}:{watch_type}:{translation_id}
```

### Table: `parser_episode_snapshots`

Snapshot cache to enable diff-based detection.

```go
type ParserEpisodeSnapshot struct {
    ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    AnimeID       string    `gorm:"type:uuid;not null;uniqueIndex:uk_combo"`
    Player        string    `gorm:"size:20;not null;uniqueIndex:uk_combo"`
    Language      string    `gorm:"size:5;not null;uniqueIndex:uk_combo"`
    WatchType     string    `gorm:"size:5;not null;uniqueIndex:uk_combo"`
    TranslationID string    `gorm:"size:50;not null;uniqueIndex:uk_combo"`
    LatestEpisode int       `gorm:"not null"`
    CheckedAt     time.Time
    UpdatedAt     time.Time
}
```

Used only by the detector job. Memory between runs.

### Read-only models in notifications service

For cross-service reads (player tables, catalog tables), define **read-only GORM models** in `repo/watch_view.go` matching the existing schemas. No migrations from notifications service touch these.

## Detection Flow

### Cron: hourly (`0 * * * *` with random jitter ±5m to spread parser load)

**Step 1 — Collect hot combos:**
```sql
SELECT DISTINCT wh.anime_id, wh.player, wh.language, wh.watch_type, wh.translation_id,
       a.shikimori_id
FROM watch_history wh
JOIN anime_list al ON al.user_id = wh.user_id AND al.anime_id = wh.anime_id
JOIN animes a ON a.id = wh.anime_id
WHERE al.status = 'watching'
  AND a.status = 'ongoing'
  AND wh.translation_id != '';
```

**Step 2 — Fetch latest available episode per combo:**

Call `GET /internal/anime/{shikimori_id}/episodes?player=...&translation_id=...&watch_type=...` on catalog service. Catalog routes internally to the right parser:
- `kodik` → `client.Search(shikimori_id, translation_id)` → read `translations[].last_episode`
- `animelib` → `GetEpisodes(slug)` filtered by team_id/translation_type
- `hianime` → `GetEpisodes(animeID)` (translation_id not granular for HiAnime; episode count is the same across dub/sub but availability differs — check via `GetSources` semantics or accept episode list as floor)
- `consumet` → analogous to hianime

Use worker pool (concurrency=5), 10s timeout per request. Failures logged and skipped.

**Step 3 — Diff vs snapshot:**

```go
for combo, latestAvail := range latestPerCombo {
    prev := snapshotMap[combo].LatestEpisode
    if latestAvail > prev {
        affectedCombos = append(affectedCombos, combo)
    }
    snapshotUpdates[combo] = latestAvail
}
```

**Step 4 — Fetch per-user max watched (single bulk query):**

```sql
SELECT wh.user_id, wh.anime_id, wh.player, wh.language, wh.watch_type, wh.translation_id,
       MAX(wh.episode_number) AS max_watched
FROM watch_history wh
WHERE (wh.anime_id, wh.player, wh.language, wh.watch_type, wh.translation_id)
      IN (... affected combos ...)
GROUP BY 1,2,3,4,5,6;
```

**Step 5 — Upsert notifications (batched insert):**

```sql
INSERT INTO user_notifications (user_id, type, dedupe_key, payload, ...)
VALUES (...)
ON CONFLICT (user_id, dedupe_key) WHERE dismissed_at IS NULL
DO UPDATE SET
  payload = jsonb_set(user_notifications.payload, '{latest_available_episode}', $latest),
  updated_at = NOW(),
  read_at = NULL;  -- re-show toast if updated
```

Note: `read_at = NULL` on update so a refreshed batch shows toast again. If user dismissed, the conflict doesn't match (WHERE clause), and a fresh notification row is inserted.

**Step 6 — Update snapshots:**

Upsert `parser_episode_snapshots` with new `LatestEpisode` values.

### Bootstrap protection

**First run** (empty `parser_episode_snapshots`): populate snapshots **without** creating notifications. Otherwise users wake up to "ep 16 of Frieren!" for shows they already watched. Implementation: if `prev == 0` (no row in snapshot), only insert snapshot, skip notification path.

Same logic per-combo: a newly-discovered combo (first time we see it) doesn't create notifications.

### Failure modes

| Case | Behavior |
|---|---|
| Catalog API down | Log + skip combo; retry next hour |
| Parser returns lower episode | Ignore; do not lower snapshot |
| User removes anime from list mid-run | Notification still created (read from snapshot, not live join); harmless |
| Service restart mid-job | UPSERT is idempotent; snapshot updates atomic |
| HiAnime/Consumet translation_id ambiguity | If translation_id stored is "sub" or "dub" (not a granular ID), match on watch_type. Combo dedupe still works. |

### Cleanup job

Daily (`30 3 * * *`): `DELETE FROM user_notifications WHERE dismissed_at < NOW() - INTERVAL '30 days'`. Unread non-dismissed rows are kept indefinitely.

## API Surface

### Public routes (via gateway, JWT required)

| Method | Path | Description |
|---|---|---|
| GET | `/api/notifications` | List notifications. Query: `status=unread\|all`, `limit`, `offset` |
| GET | `/api/notifications/unread-count` | Count of unread (for badge) |
| POST | `/api/notifications/:id/read` | Mark single read |
| POST | `/api/notifications/mark-all-read` | Bulk mark read |
| POST | `/api/notifications/:id/dismiss` | Hard dismiss |
| POST | `/api/notifications/:id/click` | Telemetry for click-through |

### Internal routes (not exposed via gateway)

| Method | Path | Description |
|---|---|---|
| POST | `/internal/notifications` | Create notification (used by detector; future: other producers) |
| GET | `/health` | Health check |
| GET | `/metrics` | Prometheus |

### Response shape: `GET /api/notifications`

```json
{
  "notifications": [
    {
      "id": "uuid",
      "type": "new_episode",
      "payload": { ... },
      "read_at": null,
      "dismissed_at": null,
      "clicked_at": null,
      "created_at": "2026-05-11T18:00:00Z",
      "updated_at": "2026-05-11T19:00:00Z"
    }
  ],
  "unread_count": 3,
  "total": 8
}
```

## Frontend

### Pinia store: `useNotificationsStore`

```ts
state:
  notifications: UserNotification[]    // active set
  unreadCount: number
  shownToastIds: Set<string>           // session-only

actions:
  fetchUnread()                        // GET /api/notifications?status=unread
  markRead(id)
  dismiss(id)
  markAllRead()
  handleClick(notification)            // → POST .click → router.push(payload.watch_url)
  startPolling()                       // 60s interval, paused on document.hidden
  stopPolling()

getters:
  latestUndismissedToast               // first unread NOT in shownToastIds
```

### Components

- `NotificationBell.vue` — header navbar position. Icon + badge `unreadCount`. Click → dropdown.
- `NotificationDropdown.vue` — list of cards. Per-type render via factory pattern.
- `NotificationToast.vue` — slide-in bottom-right (desktop) / top (mobile). Auto-hide 8s. Suppress when route param `animeId` matches payload.
- `notifications/NewEpisodeCard.vue` — type-specific renderer (poster + title + "Ep 14-16 available on AniLibria" + dismiss×).

### Type-pluggable rendering

```ts
// frontend/web/src/lib/notification-renderers.ts
import NewEpisodeCard from '@/components/notifications/NewEpisodeCard.vue'

export const renderers: Record<string, Component> = {
  new_episode: NewEpisodeCard,
}
```

Add new type = new renderer + payload schema. No bell/toast/store changes.

### Polling and lifecycle

- `App.vue` mounts store, calls `fetchUnread()` + `startPolling()` if authenticated
- 60s interval, paused on `document.hidden`, resumed + immediate fetch on visibility
- On logout: `stopPolling()` + clear state

## Gateway Routing

`services/gateway/internal/router/router.go`:

```go
notificationsURL := os.Getenv("NOTIFICATIONS_SERVICE_URL")
notificationsProxy := proxy.New(notificationsURL)
api.Handle("/notifications", authMiddleware(notificationsProxy))
api.Handle("/notifications/", authMiddleware(notificationsProxy))
```

`docker/docker-compose.yml`:
```yaml
notifications:
  build: ./services/notifications
  ports: ["8087:8087"]
  environment:
    DB_HOST: postgres
    REDIS_HOST: redis
    CATALOG_URL: http://catalog:8081
  depends_on: [postgres, redis, catalog]
```

Gateway env: `NOTIFICATIONS_SERVICE_URL=http://notifications:8087`.

## New endpoint to add in catalog

`GET /internal/anime/{shikimori_id}/episodes?player=...&translation_id=...&watch_type=...`

Returns:
```json
{ "latest_available_episode": 16, "checked_at": "..." }
```

Used **only** by the notifications detector. Cached internally in catalog (5-10 min TTL) to absorb parser-rate-limit risk.

## Testing strategy

- Unit tests for: dedupe key generation, snapshot diff logic, batching upsert, bootstrap-protection guard.
- Integration test against test Postgres: full detector flow with seeded watch_history + manipulated snapshot table → assert correct notifications created.
- Frontend: store unit tests for polling lifecycle, dismiss/read state updates.
- Manual E2E with `ui_audit_bot` user: seed watch_history at ep 5, bump snapshot pretending ep 6 was already there, trigger detector, verify bell + toast appear.

## Open questions

None at design time.

## Risk register

1. **Parser rate limits / availability** — mitigated by catalog-level cache + worker-pool concurrency cap + skip-on-failure.
2. **HiAnime/Consumet translation_id granularity** — these parsers don't distinguish sub-team. Combo key includes `watch_type` so sub/dub is still split, but multiple sub fansubs aren't. Acceptable: notify on availability of "any sub" if user had `watch_type=sub` before.
3. **Bootstrap notification storm** — explicitly mitigated; first detector run only populates snapshot.
4. **Notification spam from dropped anime** — user moves anime from "watching" → "dropped". Bulk query at Step 1 filters by `status='watching'`, so no notifications after status change.
5. **Storage growth** — daily cleanup of dismissed > 30d; unread non-dismissed indefinite (acceptable; users can mark-all-read).
6. **Polling cost on frontend** — 60s interval pauses on tab-hidden; lightweight endpoint returns small JSON. Acceptable.

## Implementation milestones

To be detailed in implementation plan (next step).
