# Project: AnimeEnigma — `notifications` workstream

**Parent project:** AnimeEnigma (see `/data/animeenigma/.planning/PROJECT.md`)
**Workstream:** notifications
**Created:** 2026-05-20
**Lifecycle:** Independent of v3.x scraper work. Runs in parallel.
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-11-notifications-engine-design.md` (Approved for planning, 2026-05-11)

## Scope of this workstream

Build a **generic in-app notification engine** for AnimeEnigma, with one fully-baked first concrete type:

> **Когда вышла новая серия онгоинга, который ты смотришь — на том же плеере и в том же переводе, что ты использовал — это должно быть понятно и удобно.**
>
> (When a new episode of an ongoing show you are watching is available — on the same player and translation you were using — you should notice it without effort.)

The engine itself is **type-pluggable from day one**: adding a future type (comments, friend activity, admin announcements) means a new payload schema + a new frontend renderer component. No backend schema changes, no bell/toast/store changes. The new-episode type is the proof-of-concept that exercises every layer.

## Core value

A user with 3-8 ongoing shows in their `anime_list.status = 'watching'` should never have to manually check whether ep N+1 dropped. Within an hour of availability (Kodik / AnimeLib / English players via scraper), the bell shows an unread badge, a toast slides in if the tab is foregrounded, and one click takes them straight to the right episode on the right player and translation they were already using. No cross-language surprises. No spam from shows they dropped.

## Out of scope for this workstream (v1.0)

| Excluded | Reason |
|---|---|
| Web Push API / Service Worker / off-tab push | Self-hosted small-group platform — in-tab polling is acceptable for the audience and cheaper to ship and operate. Reconsider in v1.1 if usage data demands. |
| Email or Telegram delivery | Same: scope creep for v1.0. Engine is generic enough to add later as new "channels". |
| WebSocket / SSE real-time | 60-second polling is fine for hourly-detector cadence. Real-time delivery is wasted granularity on top of an hourly detector. |
| Per-type opt-out UI (notification preferences) | Defer to v1.1. The payload column is JSONB, so per-type metadata + a per-user prefs table can ship later without breaking the engine. |
| Cross-language switch detection (e.g. RU→EN) | Strict combo-match is intentional — predictable, no surprise translations. |
| Notification dependence on `watch_history` rows that lack `translation_id` | Pre-translation-tracking history is opaque. Document the floor: notifications only fire for combos the user has actually watched with a recorded translation. |

## Active milestone

🟢 **v1.0 Notifications Engine** — Three phases, vertical-slice decomposition:

| Phase | Layer | Independently demoable end-state |
|---|---|---|
| 1 | Notifications Service Foundation | New `services/notifications/` microservice on port 8090 (8087 was already taken by host-native maintenance binary — see Phase 1 SUMMARY D-PORT), two tables (`user_notifications` + `parser_episode_snapshots`), CRUD HTTP API, gateway routing. Manual `INSERT` into the table makes a notification appear via `GET /api/notifications`. |
| 2 | Detector + Catalog Endpoint | Catalog gains `GET /internal/anime/{shikimori_id}/episodes?...` with 5-10min cache. New hourly cron detector populates real notifications. Bootstrap protection guarantees no first-run spam. Daily cleanup prunes dismissed > 30 days. |
| 3 | Frontend — Bell, Dropdown, Toast, Polling | Vue 3 / Pinia store with 60s polling (paused on `document.hidden`). `NotificationBell` + dropdown + toast + `NewEpisodeCard` renderer + type-pluggable registry + i18n in 3 locales. Click → router push to the right player / translation / episode. |

Each phase is independently demoable and atomically committable. Phase 1 ships with seeded test rows; Phase 2 makes detection real; Phase 3 makes the UX visible to users.

## Planned milestones (post-v1.0)

- **v1.1 Type Expansion** — Add a second concrete type (likely social `new_comment_on_anime` or admin `system_announcement`) purely to validate that the renderer-registry pattern needs no engine changes. Add minimal per-user preferences (mute-type toggles).
- **v1.2 Real-time delivery** (conditional) — Only if usage data shows polling cost matters or hourly latency is felt. SSE preferred over WebSocket given polling-replacement is the only use case.

## Active requirements (v1.0)

See `REQUIREMENTS.md` for NOTIF-FOUND-*, NOTIF-DET-*, NOTIF-UI-* IDs.

## Context (v1.0 surface area)

**Backend touches:**
- New: `services/notifications/` (new microservice, port 8090 — design doc says 8087, but that port is taken; see Phase 1 SUMMARY D-PORT)
- Modified: `services/catalog/internal/handler/*` + `services/catalog/internal/parser/*` (one new internal endpoint, 4-parser router under the hood)
- Modified: `services/gateway/internal/router/routes.go` (one new `/api/notifications/*` proxy)
- Modified: `docker/docker-compose.yml` (one new service block)
- Modified: `docker/.env.example` (new `NOTIFICATIONS_SERVICE_URL` etc.)
- Modified: `Makefile` (`make redeploy-notifications`, `make logs-notifications`)
- Modified: `CLAUDE.md` (Service Ports table + Gateway Routing table)
- New: `go.work` extended for `./services/notifications`

**Frontend touches:**
- New: `frontend/web/src/stores/notifications.ts`
- New: `frontend/web/src/components/NotificationBell.vue`
- New: `frontend/web/src/components/NotificationDropdown.vue`
- New: `frontend/web/src/components/NotificationToast.vue`
- New: `frontend/web/src/components/notifications/NewEpisodeCard.vue`
- New: `frontend/web/src/lib/notification-renderers.ts`
- New: `frontend/web/src/api/notifications.ts`
- Modified: `frontend/web/src/App.vue` (mount toast + start polling)
- Modified: `frontend/web/src/components/Header.vue` or equivalent (mount bell)
- Modified: `frontend/web/src/locales/{en,ru,ja}.json` (notification copy in 3 locales)

**Operational touches:**
- New: Grafana dashboard panel for `notifications_*` metrics (deferred to phase summary follow-up if needed; nothing user-facing).
- New: ISS-NNN entry in `docs/issues/README.md` if anything subtle surfaces during integration (placeholder slot).

**Database touches** (notifications service, dedicated `notifications` DB):
- New: `user_notifications` (id, user_id, type, dedupe_key, payload jsonb, read_at, dismissed_at, clicked_at, created_at, updated_at)
- New: `parser_episode_snapshots` (id, anime_id, player, language, watch_type, translation_id, latest_episode, checked_at, updated_at)
- Read-only models in notifications service for `watch_history`, `anime_list`, `animes` (no migrations — those tables are owned by player / catalog).

## Decisions locked at design time (do not relitigate)

| # | Decision | Locked rationale |
|---|---|---|
| 1 | Strict combo match `(player, language, watch_type, translation_id)` | User wants predictable behavior; no surprise translations. |
| 2 | Toast + bell + dropdown (not toast-only) | Build the real engine once. Toast-only would be a dead end for type #2. |
| 3 | Hourly cron detector | Acceptable latency for ongoing anime; manageable parser load with worker-pool cap. |
| 4 | Only `anime_list.status = 'watching'` | Avoid spam from one-off sample-watches. |
| 5 | Aggregate consecutive episodes into 1 notification (click → first unwatched) | User-friendly batching with clear default action. |
| 6 | New `services/notifications/` microservice (not bolted onto player) | Player service is already growing the rec engine. Keep concerns split. |
| 7 | Cross-service reads of `watch_history`, `anime_list`, `animes` via shared Postgres | Standard project pattern; no need to invent service-to-service queries for read-only lookups. |
| 8 | Catalog calls go via HTTP (not direct parser imports) | Catalog already owns the 4-parser router. Don't duplicate that wiring inside the notifications service. |
| 9 | Kodik IS included | Kodik exposes `last_episode` per translation in its search API. The iframe-playback limitation does not affect availability discovery. |

---

*Workstream root: `.planning/workstreams/notifications/`*
