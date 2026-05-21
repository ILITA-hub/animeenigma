# Requirements: AnimeEnigma `notifications` workstream — v1.0

**Milestone:** v1.0 Notifications Engine
**Defined:** 2026-05-20
**Core value:** A user with ongoing shows in `anime_list.status = 'watching'` sees a bell badge + a toast (if foregrounded) within ~1 hour of a new episode landing on the same player/translation they were already watching with. Click → straight to the right episode. Engine generic enough that adding `new_comment` or `system_announcement` in v1.1 needs only a payload schema + a frontend renderer component.
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-11-notifications-engine-design.md`

## v1.0 Requirements

### Backend — Notifications Service Foundation (Phase 1)

- [x] **NOTIF-FOUND-01**: New Go microservice at `services/notifications/` on port **8090** (port 8087 from the design doc was already claimed by the host-native `services/maintenance/` binary — same blocker that pushed `library` to 8089 in v0.2; see Phase 1 SUMMARY decision D-PORT). Standard project layout: `cmd/notifications-api/main.go`, `internal/{config,domain,handler,repo,service,job,transport}/`, no `migrations/` directory (GORM `AutoMigrate` handles schema per project convention). `GET /health` returns 200 `{"status":"ok"}`. Prometheus `/metrics` registered via `libs/metrics` with `service="notifications"` label. **Uses the shared `animeenigma` Postgres DB** that every other backend service (auth, catalog, player, themes, scheduler) connects to — this matches existing project practice where services cross-query each other's tables via the same `*gorm.DB` handle (e.g. `services/themes/internal/repo/theme.go` LEFT-JOINs `animes`). The design doc's "shared Postgres reads" decision (Decision #7) is honored at the DB level. Joins the existing multi-module workspace (`go.work` extended). Ships as `services/notifications/Dockerfile` mirroring `services/themes/Dockerfile`'s shape. `services/notifications/go.mod` includes `require + replace` directives for every `libs/*` module it depends on. <br>*(Phase 1 PLAN decision D-01 — supersedes earlier draft wording about a dedicated `notifications` DB.)*

- [x] **NOTIF-FOUND-02**: `user_notifications` table created via GORM `AutoMigrate` against the `UserNotification` domain model:
  - `id uuid pk default gen_random_uuid()`
  - `user_id uuid not null` — indexed for the main read path
  - `type varchar(32) not null` — indexed
  - `dedupe_key varchar(255) not null`
  - `payload jsonb not null` — type-specific payload (see design doc §Data Model for `new_episode` schema)
  - `read_at timestamptz`, `dismissed_at timestamptz`, `clicked_at timestamptz`
  - `created_at timestamptz`, `updated_at timestamptz`
  - **Indexes** (created via raw SQL in a `repo.EnsureIndexes(ctx)` helper called after AutoMigrate, because GORM doesn't support partial indexes):
    - `UNIQUE INDEX uk_user_dedupe ON user_notifications (user_id, dedupe_key) WHERE dismissed_at IS NULL`
    - `INDEX idx_user_unread ON user_notifications (user_id, created_at DESC) WHERE dismissed_at IS NULL`

- [x] **NOTIF-FOUND-03**: `parser_episode_snapshots` table created via GORM `AutoMigrate` against the `ParserEpisodeSnapshot` domain model with composite unique index on `(anime_id, player, language, watch_type, translation_id)` via `uniqueIndex:uk_combo` GORM tags. Used only by the detector job (Phase 2) — table exists in Phase 1 so the schema is settled before Phase 2 starts.

- [x] **NOTIF-FOUND-04**: Public HTTP API mounted under the service's root (gateway-routed in NOTIF-FOUND-06):
  - `GET /api/notifications?status=unread|all&limit=&offset=` — list active (not-dismissed) notifications for the authed user. Default limit 20, max 100. Returns `{notifications, unread_count, total}` per design doc §API Surface.
  - `GET /api/notifications/unread-count` — badge counter; returns `{unread_count: N}` only.
  - `POST /api/notifications/:id/read` — mark single read (sets `read_at = NOW()` if NULL). 404 if owned by another user.
  - `POST /api/notifications/mark-all-read` — bulk update for the authed user.
  - `POST /api/notifications/:id/dismiss` — hard dismiss (sets `dismissed_at = NOW()`; row no longer matches the partial unique index, so future detector runs can create a fresh notification for the same combo if it re-fires).
  - `POST /api/notifications/:id/click` — telemetry; sets `clicked_at = NOW()` if NULL. Body empty; response 200.

  All routes resolve the authenticated user via `authz.UserIDFromContext(ctx)` (the project convention — JWT claims are extracted by the auth middleware and stuffed into the request context; see `services/themes/internal/handler/` and `services/player/internal/handler/`). *(Phase 1 PLAN decision D-03 — supersedes the earlier draft wording about an `X-User-ID` header which no service actually uses.)*

- [x] **NOTIF-FOUND-05**: Internal HTTP API (NOT exposed via the gateway):
  - `POST /internal/notifications` — body `{user_id, type, dedupe_key, payload}`. Performs the dedupe-key UPSERT (`ON CONFLICT (user_id, dedupe_key) WHERE dismissed_at IS NULL DO UPDATE SET payload = ..., updated_at = NOW(), read_at = NULL`). Returns the resulting row. Used by the Phase 2 detector and by any future internal producer (e.g. social service for `new_comment`).
  - `GET /internal/health` — internal health (no auth, distinct from `/health`).

  Internal routes are secured by **gateway-non-routing**: the gateway only proxies `/api/notifications/*`, never `/internal/*`, so internal routes are reachable only from inside the Docker network. This matches the existing precedent in `services/auth/internal/transport/router.go` where `/internal/resolve-api-key` is mounted without a middleware guard — security is provided by the network boundary, not by per-request token verification. *(Phase 1 PLAN decision D-05 — supersedes the earlier draft wording about an `Internal` middleware pattern, which does not exist as a project convention.)*

- [x] **NOTIF-FOUND-06**: Gateway routing in `services/gateway/internal/router/routes.go`:
  - `/api/notifications` and `/api/notifications/*` proxied to `notifications:8090`, behind the existing `authMiddleware` (JWT required, no anonymous access).
  - New env `NOTIFICATIONS_SERVICE_URL` (default `http://notifications:8090`) added to `services/gateway/internal/config/config.go` and `docker-compose.yml` gateway block.
  - Internal routes (`/internal/*` on the service) are NOT exposed via the gateway — only reachable on the Docker network.

- [x] **NOTIF-FOUND-07**: Read-only GORM models for cross-service tables, defined in `services/notifications/internal/repo/`:
  - `WatchHistoryView` matching the existing `watch_history` schema (player service-owned table).
  - `AnimeListView` matching the existing `anime_list` schema (player service-owned table).
  - `AnimeView` matching the existing `animes` schema (catalog service-owned table).
  - Models are marked with a clear comment ("READ-ONLY VIEW — owned by `<other service>`. Do not include in AutoMigrate.") and never appear in the AutoMigrate call. Phase 2 detector reads from these.

- [x] **NOTIF-FOUND-08**: Smoke seeder + manual verification path. Provide a tiny seed script `scripts/seed-notification-for-ui-audit-user.sh` (uses existing `ui_audit_bot` user — see project_test_user_pattern.md) that inserts one `new_episode` notification via the **public API** (`POST /api/notifications/:id/read` won't help — use `/internal/notifications` from inside the network via `docker compose exec`). After running it, `curl -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications` shows the row, `POST /api/notifications/{id}/dismiss` removes it from subsequent unread queries. Documents the verification path so Phase 3 can develop the frontend against a known-good backend without the Phase 2 detector being live yet.

### Backend — Catalog Internal Endpoint + Detector + Cleanup (Phase 2)

- [x] **NOTIF-DET-01**: New catalog handler `GET /internal/anime/{shikimori_id}/episodes?player=&translation_id=&watch_type=&language=` at `services/catalog/internal/handler/internal_episodes.go`. Routes internally to the right parser by `player` value (`kodik | animelib | hianime | consumet` — initial set; English players via scraper service handled separately when their availability surface stabilizes). Returns `{latest_available_episode: N, checked_at: ISO}`. Cached internally in catalog Redis at key `notifications:episodes:{shikimori_id}:{player}:{translation_id}:{watch_type}` with **5-minute TTL** (chosen at 5m not 10m — single detector run completes well inside that window, and a parser hiccup is forgiven on the next hour). Internal middleware required.

- [x] **NOTIF-DET-02**: SQL hot-combo collector at `services/notifications/internal/job/hotcombos.go`. Implements the design-doc Step-1 query (DISTINCT over `watch_history JOIN anime_list ON al.status='watching' JOIN animes ON a.status='ongoing'` with `wh.translation_id != ''`). Returns `[]Combo{AnimeID, ShikimoriID, Player, Language, WatchType, TranslationID}`. Read directly against the shared `animeenigma` Postgres DB via the notifications service's single GORM handle — no second connection needed, since per NOTIF-FOUND-01 the service connects to the same logical DB that owns `watch_history` / `anime_list` / `animes`. Uses the read-only views registered in Phase 1 (NOTIF-FOUND-07).

- [x] **NOTIF-DET-03**: `NewEpisodeDetectorJob` cron at `services/notifications/internal/job/detector.go`. Runs at `0 * * * *` (top of every hour) with ±5-minute random jitter applied per-process at boot (so two detector instances would not pile up). Scheduling via `github.com/robfig/cron/v3` (already in the project — used by scheduler service; check `services/scheduler/go.mod` for the version to match). Single-instance assumption is fine for v1.0 (one container = one cron loop); document the assumption in `services/notifications/internal/job/doc.go`.

- [x] **NOTIF-DET-04**: Worker pool fanning out parser calls. Concurrency cap 5 (via `errgroup.Group{SetLimit(5)}`). Per-call timeout 10s. Parser failures logged at WARN with `combo` context and skipped (do NOT lower the snapshot — see DET-10). Per-call retry policy: none in v1.0 (next hour's run is the retry).

- [x] **NOTIF-DET-05**: Diff-vs-snapshot logic. Load all rows from `parser_episode_snapshots` matching the affected combos into a `map[Combo]int`. For each combo where `latestAvailable > prevSnapshot`, mark as "affected". Update snapshot rows in a single bulk UPSERT after the diff is computed.

- [x] **NOTIF-DET-06**: Bootstrap protection. First run on an empty `parser_episode_snapshots` table populates snapshots only — emits zero notifications. Same per-combo: a newly-discovered combo (no prior snapshot row for `(anime_id, player, language, watch_type, translation_id)`) populates a snapshot row only on this run. Notifications fire on the SECOND visit to a combo where `latestAvailable > prevSnapshot`. Unit test asserts: starting from empty tables, the first detector pass against a seeded hot-combo set produces exactly zero rows in `user_notifications`.

- [x] **NOTIF-DET-07**: Per-user max-watched bulk fetch + dedupe-key UPSERT. For each affected combo, fetch `MAX(wh.episode_number) GROUP BY (user_id, anime_id, player, language, watch_type, translation_id)` over the affected combo set in a single SQL query. Compute `first_unwatched_episode = max_watched + 1` and skip if `first_unwatched > latest_available` (user is already caught up — defensive guard against `watch_history` racing the detector). Build payload per design doc §Data Model `new_episode` schema (anime_id, shikimori_id, anime_title, anime_poster_url, first_unwatched_episode, latest_available_episode, player, language, watch_type, translation_id, translation_title, watch_url). Issue a single batched INSERT-ON-CONFLICT statement (Postgres `INSERT ... VALUES (...), (...), ... ON CONFLICT (user_id, dedupe_key) WHERE dismissed_at IS NULL DO UPDATE SET payload = jsonb_set(...), updated_at = NOW(), read_at = NULL`).

- [x] **NOTIF-DET-08**: Aggregation of consecutive new episodes into a single notification row. Multiple episode jumps (e.g. snapshot was ep 10, parser now reports ep 13) produce one row with `first_unwatched_episode = max_watched+1` and `latest_available_episode = 13`. The frontend renders "Ep N–M available" when M > N (see NOTIF-UI-05). The user clicks → routes to `first_unwatched_episode` (i.e. the oldest unwatched one) — see watch_url field generation in NOTIF-DET-07.

- [x] **NOTIF-DET-09**: Daily cleanup cron at `30 3 * * *` (off-peak). `DELETE FROM user_notifications WHERE dismissed_at < NOW() - INTERVAL '30 days'`. Unread non-dismissed rows are kept indefinitely (acceptable per design doc §Risk #5; user can always mark-all-read). Cron registered alongside the detector in the same `cron.Cron` instance.

- [x] **NOTIF-DET-10**: Failure isolation + ordering invariants. Parser returning a LOWER episode than the snapshot MUST NOT lower the snapshot (treat as a parser hiccup). Catalog timeout / 5xx on a combo MUST NOT abort the run — log and skip the combo. Detector run is idempotent: re-running with no upstream changes produces no new notification rows (UPSERT with no real change is a no-op given `payload` is byte-equal). Service restart mid-job is safe: per-combo diff + bulk UPSERT is atomic per statement; worst case a few extra notifications get UPSERTed with the same dedupe-key, which is a no-op.

### Frontend — Bell + Dropdown + Toast + Polling (Phase 3)

- [ ] **NOTIF-UI-01**: `useNotificationsStore` Pinia store at `frontend/web/src/stores/notifications.ts`. State: `notifications: UserNotification[]`, `unreadCount: number`, `shownToastIds: Set<string>` (session-only, not persisted). Actions: `fetchUnread()` (calls `GET /api/notifications?status=unread`), `markRead(id)`, `dismiss(id)`, `markAllRead()`, `handleClick(notification)` (calls `POST /:id/click` then `router.push(payload.watch_url)`), `startPolling()`, `stopPolling()`. Getter: `latestUndismissedToast` — first unread notification not in `shownToastIds`. Polling interval 60s; pauses on `document.hidden` via `visibilitychange` listener and immediately re-fetches on visibility regain. On logout: `stopPolling()` + clear state.

- [ ] **NOTIF-UI-02**: `NotificationBell.vue` component at `frontend/web/src/components/NotificationBell.vue`. Mounted in the existing site header (find the slot in `App.vue` or the header component — adjacent to user avatar / language switcher per UI convention). Renders a bell icon + a red badge with `unreadCount` (hidden when zero, shows `99+` when > 99). Click opens the dropdown. Accessible: `role="button"`, `aria-label="Notifications (N unread)"`, focus ring matches site convention.

- [ ] **NOTIF-UI-03**: `NotificationDropdown.vue` at `frontend/web/src/components/NotificationDropdown.vue`. Renders the active set in a scrollable list, max-height ~480px. Empty state: localized "No notifications yet" with a muted icon. Each card rendered via the type-pluggable registry (NOTIF-UI-06) — looks up `payload.type` → renderer component. Footer "Mark all as read" button. Closes on outside-click and Esc. Position: anchored right-edge to the bell.

- [ ] **NOTIF-UI-04**: `NotificationToast.vue` at `frontend/web/src/components/NotificationToast.vue`. Slide-in animation. Position: bottom-right on desktop (≥ 768px viewport), top-full-width on mobile. Auto-hide after 8 seconds with a pause-on-hover. Suppressed when the current route param `animeId` matches `payload.anime_id` (user is already on the page for this anime — toast would be redundant). On click → same `handleClick` as the dropdown card. Dismiss × in corner sets `shownToastIds.add(id)` so the same notification doesn't pop again in this session even if still unread.

- [ ] **NOTIF-UI-05**: `NewEpisodeCard.vue` at `frontend/web/src/components/notifications/NewEpisodeCard.vue`. Renders the `new_episode` type:
  - Anime poster (52×72 thumbnail) on the left
  - Anime title (1 line, truncate)
  - Range line: `Ep N` if N === M, else `Ep N–M` (using payload's `first_unwatched_episode` and `latest_available_episode`)
  - Translation source line: localized "AniLibria (RU dub)" or "EN sub via AnimePahe" — composed from `payload.translation_title` + `language` + `watch_type` keys
  - Relative-time stamp: "5m ago", "2h ago" (use `frontend/web/src/lib/relativeTime.ts` if it exists, else add it as a tiny helper)
  - Dismiss × button → calls `dismiss(id)`
  - Whole card is the click target → `handleClick(notification)`

- [ ] **NOTIF-UI-06**: Type-pluggable renderer registry at `frontend/web/src/lib/notification-renderers.ts`. Exports `renderers: Record<string, Component>` keyed by `payload.type`. v1.0 ships with `new_episode: NewEpisodeCard`. Adding a new type in v1.1 requires only adding a new key + component here — zero changes to the bell, dropdown, toast, or store. Unknown types render a graceful fallback `UnknownNotificationCard.vue` (one-liner: "New notification — view in dropdown") that the dropdown can show; the toast suppresses unknown types entirely.

- [ ] **NOTIF-UI-07**: i18n keys added to all three locale files (`frontend/web/src/locales/{en,ru,ja}.json`) under `notifications.*`. Required keys:
  - `notifications.bell.tooltip` ("Notifications" / "Уведомления" / "通知")
  - `notifications.dropdown.markAllRead` ("Mark all as read" / "Прочитать все" / "すべて既読にする")
  - `notifications.dropdown.empty` ("No notifications yet" / "Пока нет уведомлений" / "通知はまだありません")
  - `notifications.newEpisode.singleEp` ("Episode {n} is out" / "Вышла серия {n}" / "第{n}話が公開されました")
  - `notifications.newEpisode.rangeEp` ("Episodes {n}–{m} are out" / "Вышли серии {n}–{m}" / "第{n}〜{m}話が公開されました")
  - `notifications.newEpisode.via` ("via {translation}" / "перевод {translation}" / "翻訳: {translation}")
  - Relative-time keys ("just now", "5m ago", etc. — reuse if already present, else add)

- [ ] **NOTIF-UI-08**: App-level wiring in `App.vue`:
  - On mount + on auth state change: if `authStore.isAuthenticated`, call `notificationsStore.fetchUnread()` then `notificationsStore.startPolling()`. Else `notificationsStore.stopPolling()` + clear.
  - Mount `<NotificationToast />` at the app root so it's always above other content.
  - `NotificationBell` mounted in the existing header component (one-line addition).
  - Polling lifecycle is paused on `document.hidden` (handled inside the store, not App.vue).

## Non-functional requirements

- [x] **NOTIF-NF-01**: Prometheus metrics on the notifications service `/metrics`:
  - `notifications_created_total{type,producer}` — counter; `producer` is `"detector"` for v1.0, will be e.g. `"social"` later
  - `notifications_detector_runs_total{outcome}` — counter; outcome `"success" | "partial" | "failed"`
  - `notifications_detector_duration_seconds` — histogram
  - `notifications_detector_combos_scanned` — gauge (last value)
  - `notifications_detector_parser_failures_total{player}` — counter
  - `notifications_active_unread_gauge` — gauge polled every 5 minutes via `SELECT COUNT(*) FROM user_notifications WHERE dismissed_at IS NULL AND read_at IS NULL`

- [x] **NOTIF-NF-02**: Logging via `libs/logger` with structured fields. Detector runs log at INFO: `combos_scanned`, `affected_combos`, `notifications_upserted`, `duration_ms`, `parser_failures`. Per-combo errors log at WARN with combo context. No PII in logs (user_id is fine; usernames are not logged).

- [ ] **NOTIF-NF-03**: Manual E2E verification path documented in the Phase 3 SUMMARY: log in as `ui_audit_bot`, seed `watch_history` to ep 5 of Frieren (Kodik + AniLibria + ru + dub), insert a `parser_episode_snapshots` row with `latest_episode = 6`, run the detector once (admin endpoint or `make` shortcut), confirm bell + toast appear with "Ep 6 is out", click → lands on `/anime/{uuid}/watch?player=kodik&episode=6&translation=...`, click dismiss → bell badge returns to zero.

- [ ] **NOTIF-NF-04**: `CLAUDE.md` updates (Service Ports table + Gateway Routing section):
  - Add row: `notifications | 8090 | /metrics | Generic notification engine (new episodes, future types) |` (port changed from design-doc 8087 — see D-PORT)
  - Add line under Gateway Routing: `/api/notifications/*` → `notifications:8090` (JWT required)
  - Env var section: document `NOTIFICATIONS_SERVICE_URL` (gateway), `CATALOG_URL` (notifications service)

## Phase mapping

| Requirement | Phase |
|---|---|
| NOTIF-FOUND-01..08, NOTIF-NF-04 (partial — service ports row + gateway routing) | Phase 1: Notifications Service Foundation |
| NOTIF-DET-01..10, NOTIF-NF-01, NOTIF-NF-02 | Phase 2: Catalog Internal Endpoint + Episode Detector + Cleanup |
| NOTIF-UI-01..08, NOTIF-NF-03 | Phase 3: Frontend — Bell + Dropdown + Toast + Polling |

## Out of Scope (v1.0)

| Feature | Reason |
|---------|--------|
| Web Push API / Service Worker / off-tab push | Self-hosted small-group platform — in-tab polling is enough. Reconsider in v1.1 only if usage data demands. |
| Email or Telegram delivery channels | Scope creep for v1.0. Engine is generic enough to add later as new "channels". |
| WebSocket / SSE real-time delivery | 60-second polling is fine for hourly-detector cadence. |
| Per-type opt-out preferences UI | Defer to v1.1. JSONB payload + future `user_notification_prefs` table can ship later without breaking the engine. |
| Cross-language switch detection (RU→EN, etc.) | Strict combo-match is intentional per design doc Decision #1. |
| Notifications for `anime_list.status = 'planned'` or `dropped` or `paused` | Per design doc Decision #4 — only `watching` to avoid spam. |
| HiAnime / Consumet specific notifications (English players are now via scraper service) | The English-language tab is intentionally hidden in the UI today (see CLAUDE.md). Phase 2's player filter starts with `kodik | animelib` and adds English players when their availability surface is exposed in a stable internal contract — that work belongs in a v1.0.x patch, not v1.0 scope. |
| Notification grouping across anime (e.g. "3 shows have new episodes") | Aggregation is intra-combo only per design doc Decision #5. |
| In-app sound / OS notification permission prompts | Visual-only for v1.0. Sound is a v1.1 preferences item. |

## Score (per project convention)

- **UXΔ:** **+4 (Better)** — direct fix for a constant low-grade friction (manual "did ep N drop yet?" checks). Not +5 because it doesn't unlock anything new, it removes a chore.
- **CDI:** `0.04 × 13` — Spread: backend (new service, gateway, catalog endpoint), frontend (store, 4 new components, locale), DB (2 new tables). Shift: low (additive — no schema mutations on existing tables, no API breakage, gateway gets one new proxy rule). Effort: 13 Fibonacci (notification engine plus type-pluggable + cron + cache + 3-locale frontend across 3 phases is solidly 13, not 8 — bootstrap protection and idempotency invariants alone account for non-trivial test surface).
- **MVQ:** **Griffin 82%/85%** — Griffin (graceful + reliable + visible) is the right shape: noble surface (bell + toast), strong wings (cron + dedupe + bootstrap protection). 82% match — could be a Phoenix if it included recovery flows for stale-link cleanup but the latter is out of v1.0 scope. 85% slop-resistance — the dedupe-key + bootstrap-guard + UPSERT idempotency make accidental duplicates extremely unlikely.
