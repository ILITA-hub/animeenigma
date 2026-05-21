# Roadmap: AnimeEnigma `notifications` workstream

**Workstream:** notifications (parallel to root `v3.x` scraper work, parallel to other workstreams `raw-jp`, `social`, `ui-ux-audit`)
**Active milestone:** v1.0 Notifications Engine
**Phase numbering:** Workstream-local — restarts at 1 inside each milestone (`v1.0-phases/01-*`, future `v1.1-phases/01-*`).
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-11-notifications-engine-design.md`
**Requirements:** `REQUIREMENTS.md`

## Milestones

- 🟢 **v1.0 Notifications Engine** — Active (planning), 3 phases scoped — see below
- ⏳ **v1.1 Type Expansion + Preferences** — Conditional on v1.0 + 1-2 weeks usage data
- ⏳ **v1.2 Real-Time Delivery** — Conditional on v1.0 polling data showing pain

## Goal (v1.0)

A self-hosted small-group AnimeEnigma user with 3-8 ongoing shows in `anime_list.status = 'watching'` notices when a new episode drops without ever having to refresh the anime page. Within ~1 hour of upstream availability on the player and translation they were already using, a red badge appears on the header bell. If the tab is foregrounded, a toast slides in. One click on either takes them to the right episode on the right player with the right translation. No surprise translations. No spam from shows they dropped. No first-run notification storm.

The engine is **type-pluggable from day one**: the second concrete type in v1.1 (likely `new_comment_on_anime` or `system_announcement`) will be additive only — new payload schema + new renderer component, zero changes to bell, dropdown, toast, store, gateway, or table schema. v1.0 explicitly delivers this generic surface, not a narrow new-episode toast.

## Vertical-slice phasing rationale

Three phases, each independently demoable and atomically committable:

- **Phase 1 (Foundation)** ends with a working CRUD API behind the gateway. Manual insert via `/internal/notifications` proves the read path, dismiss/click telemetry, and dedupe behavior. The Phase 3 frontend can be developed against this even before the Phase 2 detector exists. Phase 1 is the only phase that touches the gateway and adds a service to docker-compose — once shipped, Phases 2 and 3 are pure backend-and-frontend extensions with no infra coordination cost.
- **Phase 2 (Detection)** is where the subtle correctness work lives: bootstrap protection (zero notifications on first run), dedupe-key UPSERT semantics, failure isolation (parser hiccup doesn't kill the run), idempotency (re-running the detector with no upstream change is a no-op). Catalog gains exactly one new internal endpoint with a 5-minute Redis cache. Daily cleanup also lives here. End-state: real notifications materialize hourly.
- **Phase 3 (UX)** is pure frontend — Pinia store + 4 components + type-pluggable registry + 3-locale i18n + App.vue mount. Bell on header, toast on app root, dropdown anchored to bell, polling lifecycle paused on `document.hidden`. End-state: user-visible behavior is complete.

Splitting the detector into its own phase prevents the most error-prone work from being co-mingled with infra setup or UI polish. It also creates a natural verification gate at the end of Phase 2 (manual `make seed-notification-fake-snapshot && make run-detector-once`) before any user sees anything.

## Phases

### Phase 1: Notifications Service Foundation

**Goal:** Stand up `services/notifications/` on port 8090 (8087 was claimed by the host-native maintenance binary — see Phase 1 SUMMARY D-PORT) with two new tables (`user_notifications` + `parser_episode_snapshots`), the full CRUD HTTP API (list / unread-count / read / mark-all-read / dismiss / click), the internal `/internal/notifications` UPSERT producer endpoint, gateway proxy for `/api/notifications/*` under JWT auth, and read-only cross-service GORM views for `watch_history` / `anime_list` / `animes` (which Phase 2 reads). End-state: `curl -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications` returns `{"notifications":[],"unread_count":0,"total":0}`. After `docker compose exec notifications` POSTing a seed row, the same curl returns the seed; `POST /api/notifications/:id/dismiss` removes it from the unread query but keeps it in `status=all`. No detector, no cron, no frontend.

**Depends on:** Nothing — additive backend module. Does not touch existing tables.
**Requirements:** NOTIF-FOUND-01, NOTIF-FOUND-02, NOTIF-FOUND-03, NOTIF-FOUND-04, NOTIF-FOUND-05, NOTIF-FOUND-06, NOTIF-FOUND-07, NOTIF-FOUND-08, NOTIF-NF-04 (partial — service-ports row + gateway-routing row)
**SPEC:** `phases/01-notifications-foundation/01-SPEC.md` (to be written by gsd-plan-phase)
**Touches:**
- `services/notifications/cmd/notifications-api/main.go` (new)
- `services/notifications/internal/{config,domain,handler,repo,service,job,transport}/` (new — `job/` is scaffolded empty in Phase 1, populated in Phase 2)
- `services/notifications/Dockerfile` (new)
- `services/notifications/go.mod` (new — joined to `go.work`; require + replace for every `libs/*` used)
- `go.work` (extend)
- `docker/docker-compose.yml` (new notifications service block + depends_on postgres/redis)
- `docker/.env.example` (new `NOTIFICATIONS_*` env vars, `NOTIFICATIONS_SERVICE_URL` for gateway)
- `services/gateway/internal/config/config.go` (new `NotificationsURL` field)
- `services/gateway/internal/router/routes.go` (new `/api/notifications/*` proxy under `authMiddleware`)
- `Makefile` (`make redeploy-notifications`, `make logs-notifications`, `make restart-notifications`)
- `CLAUDE.md` (Service Ports row + Gateway Routing row)
- `scripts/seed-notification-for-ui-audit-user.sh` (new)

**Success criteria:**
1. `make redeploy-notifications` builds and starts the container clean; `make health` includes `notifications:8090 - healthy`.
2. `curl http://localhost:8090/health` → 200 `{"status":"ok"}` directly; `curl http://localhost:8000/api/notifications -H "Authorization: Bearer $UI_AUDIT_API_KEY"` → 200 with empty list (gateway proxy + auth both work).
3. Both new tables exist in the dedicated `notifications` Postgres DB (`\dt user_notifications parser_episode_snapshots` shows them). The two partial indexes on `user_notifications` exist (`\d user_notifications` shows `uk_user_dedupe` and `idx_user_unread`).
4. Running `scripts/seed-notification-for-ui-audit-user.sh` then `curl -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications` returns the seed row with the expected `new_episode` payload shape. `POST /api/notifications/{id}/dismiss` then re-fetch shows `unread_count: 0`.
5. `POST /internal/notifications` is NOT reachable from outside the Docker network (curl from the host hits a 404 or no-route from the gateway; only `docker compose exec notifications wget -O- localhost:8090/internal/notifications` works).
6. Re-running the seed script with the same `(user_id, dedupe_key)` UPSERTs (no duplicate row created — verified by `SELECT COUNT(*)`).

### Phase 2: Catalog Internal Endpoint + Episode Detector + Cleanup

**Goal:** Make notifications real. Catalog gains `GET /internal/anime/{shikimori_id}/episodes?player=...` with a 5-minute Redis cache. The notifications service gets the hourly `NewEpisodeDetectorJob` (jitter ±5min) and the daily `30 3 * * *` cleanup cron. End-state: after seeding `watch_history` for `ui_audit_bot` to ep 5 of an anime and inserting a `parser_episode_snapshots` row with `latest_episode = 6`, running the detector once (admin endpoint or `make` shortcut) produces exactly one `user_notifications` row with the expected payload, dedupe key, and watch_url. Bootstrap protection verified: starting from truly empty tables, the first detector run with the same seed produces zero `user_notifications` rows (it populates the snapshot only).

**Depends on:** Phase 1 (service exists, tables exist, internal endpoint exists, read-only views exist).
**Requirements:** NOTIF-DET-01, NOTIF-DET-02, NOTIF-DET-03, NOTIF-DET-04, NOTIF-DET-05, NOTIF-DET-06, NOTIF-DET-07, NOTIF-DET-08, NOTIF-DET-09, NOTIF-DET-10, NOTIF-NF-01, NOTIF-NF-02
**SPEC:** `phases/02-detector-and-catalog-endpoint/02-SPEC.md` (to be written by gsd-plan-phase)
**Touches:**
- `services/catalog/internal/handler/internal_episodes.go` (new)
- `services/catalog/internal/transport/router.go` (route registration — under internal middleware)
- `services/catalog/internal/parser/{kodik,animelib}/client.go` (extend with `LatestEpisode(shikimoriID, translationID, watchType) (int, error)` if not already exposed — likely needs a thin wrapper around existing search/episodes methods)
- `services/notifications/internal/job/hotcombos.go` (new — SQL collector)
- `services/notifications/internal/job/detector.go` (new — orchestrator + worker pool + diff + UPSERT)
- `services/notifications/internal/job/cleanup.go` (new — daily DELETE)
- `services/notifications/internal/job/scheduler.go` (new — `cron.Cron` instance wiring)
- `services/notifications/internal/repo/snapshot.go` (new — bulk read + bulk UPSERT)
- `services/notifications/internal/service/catalog_client.go` (new — thin HTTP client for catalog `/internal/anime/{id}/episodes`)
- `services/notifications/internal/handler/admin.go` (new — `POST /admin/detector/run-once` for manual triggering during verification; admin-only, internal middleware)
- `services/notifications/cmd/notifications-api/main.go` (wire cron + worker pool on boot)
- `Makefile` (`make run-detector-once` shortcut hitting the admin endpoint via `docker compose exec`)

**Success criteria:**
1. `curl 'http://catalog:8081/internal/anime/57466/episodes?player=animelib&translation_id=9999&watch_type=dub&language=ru'` from inside the network returns `{"latest_available_episode": N, "checked_at": "..."}`. Repeat within 5 minutes is served from Redis (verified by parser-log absence).
2. Starting from completely empty `parser_episode_snapshots` and `user_notifications`, manually triggering the detector once via `make run-detector-once`:
   - Populates `parser_episode_snapshots` with one row per active hot combo.
   - Inserts ZERO rows in `user_notifications` (bootstrap protection).
3. Seeding `ui_audit_bot`'s `watch_history` to ep 5 of Frieren on Kodik / AniLibria / ru / dub, manually setting that combo's `parser_episode_snapshots.latest_episode = 5` (so the detector's next observation of "6 available" is a real diff), then running the detector with the parser mocked to return `latest_episode = 6`, produces EXACTLY one row in `user_notifications` for `ui_audit_bot` with `dedupe_key = "new_episode:{anime_id}:kodik:ru:dub:{translation_id}"`, `payload.first_unwatched_episode = 6`, `payload.latest_available_episode = 6`, and a non-null `payload.watch_url`.
4. Re-running the detector against the same upstream state produces NO new notification rows (idempotency); the existing row's `updated_at` may bump but `read_at` is preserved as NULL.
5. Re-running the detector with the parser now returning `latest_episode = 8` UPSERTs the existing notification (does NOT insert a new one); `payload.latest_available_episode` becomes 8 while `payload.first_unwatched_episode` stays 6; `read_at` is reset to NULL so the UX re-fires.
6. After manually `UPDATE`ing one notification's `dismissed_at` to `NOW() - INTERVAL '31 days'`, triggering the cleanup cron deletes that row but leaves a row with `dismissed_at = NOW() - INTERVAL '29 days'` untouched.
7. The 6 NOTIF-NF-01 metrics are visible at `http://localhost:8090/metrics` after one detector run.

### Phase 3: Frontend — Bell + Dropdown + Toast + Pinia Store + Renderer Registry + i18n

**Goal:** Make the engine visible. Pinia store with 60s polling (paused on hidden tab). NotificationBell on the header. NotificationDropdown anchored to the bell. NotificationToast at the app root. NewEpisodeCard renderer wired via a type-pluggable registry. i18n keys added in en/ru/ja. End-state: with the Phase 2 detector seeded against `ui_audit_bot`, the user logs in, sees the red badge on the bell within 60 seconds (or immediately on visibility regain), clicks the bell, sees the NewEpisodeCard with the right anime + episode range + translation source, clicks the card, lands on the watch page at the right episode. Foregrounded toast appears once per session per notification.

**Depends on:** Phase 1 (API), Phase 2 (real notifications to render). Phase 3 can technically be built against Phase 1's manual seeds in parallel with Phase 2, but the end-state verification requires Phase 2.
**Requirements:** NOTIF-UI-01, NOTIF-UI-02, NOTIF-UI-03, NOTIF-UI-04, NOTIF-UI-05, NOTIF-UI-06, NOTIF-UI-07, NOTIF-UI-08, NOTIF-NF-03
**SPEC:** `phases/03-frontend-bell-dropdown-toast/03-SPEC.md` (to be written by gsd-plan-phase)
**Touches:**
- `frontend/web/src/stores/notifications.ts` (new)
- `frontend/web/src/api/notifications.ts` (new — typed client over the 6 public routes)
- `frontend/web/src/components/NotificationBell.vue` (new)
- `frontend/web/src/components/NotificationDropdown.vue` (new)
- `frontend/web/src/components/NotificationToast.vue` (new)
- `frontend/web/src/components/notifications/NewEpisodeCard.vue` (new)
- `frontend/web/src/components/notifications/UnknownNotificationCard.vue` (new — graceful fallback for forward-compat)
- `frontend/web/src/lib/notification-renderers.ts` (new — `renderers: Record<string, Component>`)
- `frontend/web/src/lib/relativeTime.ts` (new IF not already present)
- `frontend/web/src/types/notification.ts` (new — `UserNotification` + `NewEpisodePayload`)
- `frontend/web/src/locales/en.json` (new `notifications.*` keys per NOTIF-UI-07)
- `frontend/web/src/locales/ru.json` (same)
- `frontend/web/src/locales/ja.json` (same)
- `frontend/web/src/App.vue` (mount toast + start polling on auth state)
- `frontend/web/src/components/<existing-header-component>.vue` (mount bell — exact file located during planning)
- `frontend/web/tests/e2e/notifications.spec.ts` (new — Playwright test driving the end-to-end UX)

**Success criteria:**
1. Logged-out user: bell is NOT rendered. No polling fires (verified via Network tab — no `/api/notifications` requests).
2. Logged-in user with zero notifications: bell renders with no badge. `GET /api/notifications?status=unread` fires once on mount, then every 60s while the tab is visible. Tab-hide pauses polling within ~1s.
3. Logged-in `ui_audit_bot` with one seeded `new_episode` notification: badge shows "1" within 60s of mount. Foregrounded toast slides in once. Toast auto-hides after 8s, or on dismiss × click, or on click (which routes to the watch URL). After dismiss/click, the toast does NOT reappear in the same session even if the notification is still unread (verified via `shownToastIds`).
4. Clicking the bell opens the dropdown. Clicking the NewEpisodeCard fires `POST /:id/click`, `router.push(watch_url)`, and the dropdown closes. Mark-all-read button fires `POST /mark-all-read` and updates the badge to zero.
5. The toast does NOT appear when the user is already on the matching anime's watch page (route param `animeId === payload.anime_id`).
6. Unknown notification types render via `UnknownNotificationCard` in the dropdown and are suppressed in the toast (verified by inserting a hand-crafted `type: "future_type"` row).
7. All 3 locales render the card and dropdown without falling back to translation keys (verified by switching language and visually inspecting).
8. Logout calls `stopPolling()` and clears state — no further `/api/notifications` requests fire.

## Next

After Phase 1 plan approval and execution, continue with:

```
/gsd-plan-phase 2 --ws notifications   # Detector + catalog endpoint
/gsd-plan-phase 3 --ws notifications   # Frontend
```

When v1.0 ships:

```
/gsd-audit-milestone --ws notifications
/gsd-complete-milestone v1.0 --ws notifications
/gsd-new-milestone --ws notifications  # if v1.1 is in scope
```
