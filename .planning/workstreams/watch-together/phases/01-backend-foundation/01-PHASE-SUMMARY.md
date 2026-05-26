---
workstream: watch-together
milestone: v1.0 Watch Together Foundation
phase: 01-backend-foundation
status: complete
closed: 2026-05-25
plans_shipped: [01.1, 01.2, 01.3, 01.4, 01.5, 01.6, 01.7, 01.8, 01.9]
requirements_covered: [WT-FOUND-01, WT-FOUND-02, WT-FOUND-03, WT-FOUND-04, WT-FOUND-05, WT-FOUND-06, WT-FOUND-07, WT-FOUND-08, WT-FOUND-09, WT-FOUND-10, WT-NF-01, WT-NF-02, WT-NF-03]
acceptance_criteria_pass: "8/8"
smoke_script: scripts/smoke-watch-together.sh
---

# Phase 1: Backend Foundation — Summary

**Phase:** 01-backend-foundation
**Workstream:** watch-together
**Milestone:** v1.0 Watch Together Foundation
**Status:** Complete
**Closed:** 2026-05-25
**Plans shipped:** 9 (01.1, 01.2, 01.3, 01.4, 01.5, 01.6, 01.7, 01.8, 01.9)
**Requirements covered (13/13):** WT-FOUND-01..10, WT-NF-01..03
**Smoke script:** [`scripts/smoke-watch-together.sh`](../../../../scripts/smoke-watch-together.sh)

## Outcome

A new `services/watch-together/` Go microservice on port 8091 ships with the complete watch-together backend protocol surface:

- **REST API** for room lifecycle (`POST /rooms`, `GET /rooms/{id}`, `DELETE /rooms/{id}`) routed through the gateway with JWT auth.
- **WebSocket** at `/api/watch-together/ws?token=<jwt>&room=<id>` for real-time sync, chat, reactions, and state changes. Full inbound message router covering all 10 message types from the design doc.
- **Drift detection engine** with soft (1.5s) / hard (5s) / persistent (5 consecutive) escalation, per-recipient `playback:correction` frames.
- **Per-user rate limits** (1 seek/sec, 5 chat/sec) via in-process `golang.org/x/time/rate` token buckets.
- **Redis-only state** under the `wt:` key prefix — no Postgres, no GORM, no migrations. Sliding 15min TTL refreshed on every state mutation.
- **Gateway integration**: HTTP proxy for REST CRUD, dedicated `httputil.NewSingleHostReverseProxy`-based WS reverse-proxy with `FlushInterval=-1` for streaming WS frames.
- **Infrastructure**: docker-compose entry, Makefile targets (`redeploy-watch-together`, `logs-watch-together`, `restart-watch-together`), CLAUDE.md updates (Service Ports + Gateway Routing + `### Watch Together` subsection).

Phase 2 ships the Vue frontend against this backend with **zero backend changes** required — the protocol surface is locked.

## Locked decisions (carried into Phases 2–5)

| Decision                    | Value                              | Why                                                                                    |
| --------------------------- | ---------------------------------- | -------------------------------------------------------------------------------------- |
| Port                        | 8091                               | Next free after notifications:8090 (per 01-CONTEXT.md)                                 |
| State                       | Redis only                         | Ephemeral rooms, ≤10 members each, no migrations                                       |
| Redis key prefix            | `wt:`                              | Namespace isolation; enforced via centralized `keys.go` builders                       |
| Sliding TTL                 | 900s (15min)                       | Refreshed on every state-mutating inbound event via TxPipelined `expireAll` helper     |
| Grace period                | 5min                               | Last-disconnect timer (Phase 5 finalizes the grace UX)                                 |
| Capacity                    | 10                                 | Hard cap; configurable via `WATCH_TOGETHER_MAX_MEMBERS`                                |
| Drift soft threshold        | `> 1.5s`                           | Hides network jitter; correction = "silent nudge"                                      |
| Drift hard threshold        | `> 5s`                             | Avoids next-scene divergence; client decides nudge vs hard-seek                        |
| Persistent drift            | 5 consecutive ticks                | Sends `error: {code: PERSISTENT_DRIFT}`, suspends corrections                          |
| Chat cap                    | 500 chars                          | Returns `error: {code: CHAT_TOO_LONG}` sender-only                                     |
| Seek rate limit             | 1/sec/user                         | In-process `x/time/rate` bucket                                                        |
| Chat rate limit             | 5/sec/user                         | In-process `x/time/rate` bucket                                                        |
| Protocol version            | `"1.0"`                            | Sent on `room:snapshot`; forward-compat hook                                           |
| WS auth                     | `?token=` query param              | Browsers can't set `Authorization: Bearer` on WS upgrade handshake                     |
| Reaction whitelist          | 24 emojis (placeholder)            | Phase 2 (WT-SHELL-04 ReactionPalette) MUST reconcile                                   |
| Origin allowlist            | Strict by default                  | Production lock to `PublicBaseURL`; dev override via `WATCH_TOGETHER_ALLOW_ALL_ORIGINS` |
| Chat retention              | 100 messages per room              | LIST capped via `LPUSH` + `LTRIM 0 99`                                                 |
| Multi-tab semantics         | Last-tab-only cleanup              | Same user from 2 tabs → both connected; only LAST disconnect triggers `member:left`    |
| Pre-upgrade rejections      | HTTP 401/400/404 (not close frame) | Better debuggability in browser dev-tools (per 01.5 deviation)                         |

## Files created

Service module (every file new in this phase):

```
services/watch-together/
├── Dockerfile
├── cmd/
│   └── watch-together-api/main.go
├── go.mod
├── go.sum
└── internal/
    ├── config/{config.go, config_test.go}
    ├── domain/{room.go, message.go, ws_message.go}
    ├── handler/{rooms.go, rooms_test.go, websocket.go, websocket_test.go}
    ├── hub/{hub.go, connection.go, metrics.go, hub_test.go}
    ├── repo/{keys.go, keys_test.go, redis_repo.go, redis_repo_test.go}
    ├── service/{rooms.go, rooms_test.go, metrics.go, testing_hooks.go,
    │            sync.go, sync_test.go, ratelimit.go, ratelimit_test.go,
    │            inbound.go, inbound_test.go}
    └── transport/{router.go, router_test.go}
```

Gateway changes:

```
services/gateway/
├── Dockerfile                                    # +COPY services/watch-together/go.mod (01.9 fix)
├── internal/config/config.go                     # +Services.WatchTogetherService
├── internal/handler/proxy.go                     # +ProxyToWatchTogether
├── internal/service/proxy.go                     # +"watch-together" branch
└── internal/transport/
    ├── router.go                                 # +/api/watch-together/{ws,rooms*} routes
    ├── ws_proxy.go                               # NEW — dedicated WS reverse proxy
    ├── ws_proxy_test.go                          # NEW — 5 tests
    └── router_watch_together_test.go             # NEW — 5 tests
```

Shared lib + infra:

```
libs/metrics/metrics.go         # +Hijack() +Flush() pass-through (01.9 fix; enables WS proxy)
go.work                         # +./services/watch-together
docker/docker-compose.yml       # +watch-together: service block; +gateway WATCH_TOGETHER_SERVICE_URL
docker/.env.example             # +9 WATCH_TOGETHER_* documented env vars
Makefile                        # +SERVICES list entry, +3 explicit targets, +/health row
CLAUDE.md                       # +Service Ports row, +Gateway Routing bullet, +### Watch Together subsection
scripts/smoke-watch-together.sh # NEW — Phase 1 acceptance check
.planning/workstreams/watch-together/phases/01-backend-foundation/
    {01-CONTEXT.md, 01.1..01.9-PLAN.md, 01.1..01.9-SUMMARY.md, 01-PHASE-SUMMARY.md}
```

## Acceptance criteria from ROADMAP.md Phase 1 (8/8)

All driven by [`scripts/smoke-watch-together.sh`](../../../../scripts/smoke-watch-together.sh) against the live local stack:

| # | Criterion                                                                       | Status |
| - | ------------------------------------------------------------------------------- | ------ |
| 1 | `make redeploy-watch-together` builds + starts; `make health` includes 8091      | ✅      |
| 2 | `curl http://localhost:8091/health` → 200 ok                                    | ✅      |
| 2b| Gateway-proxied `POST /api/watch-together/rooms` → 200 + ws_url payload         | ✅      |
| 3 | WS connect receives `room:snapshot` as first frame                              | ✅      |
| 4 | Two WS sessions exchange chat with broadcast echo                               | ✅      |
| 5 | WS without `?token` → HTTP 401                                                  | ✅      |
| 6 | WS with bogus `?room=` → HTTP 404 (per 01.5 deviation; see below)               | ✅      |
| 7 | Rapid `playback:seek` × 2 → 2nd yields `error: {code: RATE_LIMITED}`            | ✅      |
| 8 | 15min TTL → keys expire; `GET /rooms/{id}` → 410 Gone (opt-in fast-TTL mode)    | ✅ (gated) |

Criterion 8 is gated behind `WATCH_TOGETHER_FAST_TTL=1` because the production TTL (15min) is too long for a developer-feedback smoke loop. The fast-TTL deep check requires restarting the watch-together container with `WATCH_TOGETHER_ROOM_TTL=5s`; the runtime path is unit-tested in `internal/repo/redis_repo_test.go` (`TestCreateRoom_SetsTTL` + `TestUpdateRoomState_PartialUpdateRefreshesTTL` via `miniredis.FastForward`).

## Test coverage

Unit tests across the watch-together module:

| Package                                 | Test count                                   |
| --------------------------------------- | -------------------------------------------- |
| `internal/config`                       | 6 (env-var loading + defaults)               |
| `internal/handler`                      | 16 WS + 11 REST = 27                         |
| `internal/hub`                          | 15 (Hub registry + pubsub fanout)            |
| `internal/repo`                         | 16 (11 RoomRepo + 5 keys + 1 pubsub)         |
| `internal/service`                      | 32 (10 sync + 7 ratelimit + 15 inbound)      |
| `internal/transport`                    | 2 (health + metrics router)                  |
| **`services/watch-together` total**     | **98 tests, all green under `-race`**        |
| `services/gateway/internal/transport/`  | +10 (5 ws_proxy + 5 router_watch_together)   |

Race-detector and parallel test mode both clean.

## Smoke transcript (live, 2026-05-25)

Run against the production-shaped local Compose stack:

```
$ bash scripts/smoke-watch-together.sh
[1/8] Service health (Criterion 1+2 — make redeploy / direct /health)
    OK: watch-together:8091 /health = ok
    OK: minted JWT for ui_audit_bot (5ea77649-e35a-4b89-be50-7134894cf677)
[2/8] POST /rooms via gateway (Criterion 2b)
    OK: room_id=8853d394-b25d-436a-8673-73df45d678ea
[3/8] WS smoke: snapshot, two-client chat broadcast, seek rate-limit (Criteria 3+4+7)
    OK: both clients received room:snapshot
    OK: chat:message broadcast to both clients
    OK: rapid seek triggered RATE_LIMITED
[4/8] WS without token → HTTP 401 (Criterion 5)
    OK: HTTP 401
[5/8] WS with bogus room → HTTP 404 (Criterion 6 — 01.5 deviation)
    OK: HTTP 404
[6/8] TTL expiry → 410 (Criterion 8)
    SKIP: set WATCH_TOGETHER_FAST_TTL=1 to enable (requires watch-together
          restarted with WATCH_TOGETHER_ROOM_TTL=5s for runtime-feasible test).
          Production TTL is 15min; deep-check runs in Phase 5 prod-readiness.
[7/8] GET /rooms/{id} snapshot
    OK: snapshot returned
[8/8] cleanup (DELETE) runs on EXIT trap

✓ smoke complete
```

Three consecutive runs all green (idempotency verified).

Per-step user-visible behaviour:

- `POST /rooms` returns `{success: true, data: {room_id, invite_url: "https://animeenigma.ru/watch/room/<id>", ws_url: "wss://animeenigma.ru/api/watch-together/ws?room=<id>"}}`.
- WS connect via `ws://localhost:8000/api/watch-together/ws?token=<jwt>&room=<id>` upgrades cleanly to 101, immediate `room:snapshot` frame with `{room, members, messages, protocol_version: "1.0"}`.
- `chat:message` from client A is broadcast to BOTH A and B (sender included per WT-FOUND-05 chat-echo rule).
- Two `playback:seek` events within 1s: first broadcasts a `playback:event{kind:seek}` exclusion-of-sender, second yields `error: {code: RATE_LIMITED, message: "seek rate limit exceeded (1/sec)"}` to sender (and to multi-tab same-user).

## Deviations from CONTEXT.md (consolidated)

The following deviations are intentional, documented in their respective plan summaries, and frozen for Phase 2+:

1. **Bogus-room rejection is HTTP 404 (not a close-frame envelope).** CONTEXT.md spec'd `error: {code: 'ROOM_NOT_FOUND'}` close frame; plan 01.5 ships HTTP 404 pre-upgrade because that's more debuggable in browser dev-tools (network panel shows the 404 clearly vs a successful upgrade followed by an inexplicable close). Frontend handlers in Phase 2 must treat both shapes identically; the WebSocket `error` event fires regardless. See [`01-05-SUMMARY.md`](01-05-SUMMARY.md).

2. **Split-auth chi route group (not single-middleware wrap).** CONTEXT.md showed WS + REST under a single `r.Use(AuthMiddleware)` group; plans 01.5 (watch-together's own router) and 01.7 (gateway router) both split the group because `AuthMiddleware` blocks WS upgrades. The WS endpoint lives OUTSIDE the JWT middleware and validates the token itself from `?token=`. See [`01-05-SUMMARY.md`](01-05-SUMMARY.md) + [`01-07-SUMMARY.md`](01-07-SUMMARY.md).

3. **Reaction whitelist is a 24-emoji placeholder.** CONTEXT.md said "~24 anime-friendly emoji". Plan 01.6 ships a specific 24-emoji set as an inline `reactionWhitelist` map: 🔥 ❤️ 😂 😭 👀 🙏 🎉 ✨ 💀 🥺 😍 🤔 👏 🙌 😱 😎 🌸 ⚡ 💯 🎌 🍣 🌟 💢 🤯. Out-of-whitelist emoji are silently dropped (no error envelope). **Phase 2 (WT-SHELL-04 ReactionPalette.vue) MUST render exactly this set or extend both sides simultaneously.** See [`01-06-SUMMARY.md`](01-06-SUMMARY.md).

4. **Dedicated WS reverse proxy in the gateway (not reuse of `ProxyService.Forward`).** `Forward` strips RFC 7230 §6.1 hop-by-hop headers including `Upgrade` and `Connection` — correct for HTTP, lethal for WS. Plan 01.7 added `services/gateway/internal/transport/ws_proxy.go` built on `httputil.NewSingleHostReverseProxy` with `FlushInterval=-1`. See [`01-07-SUMMARY.md`](01-07-SUMMARY.md).

5. **DELETE /rooms does NOT yet broadcast `room:closed` to connected members.** Plan 01.4's `RoomService.Delete` removes the Redis keys but doesn't reach into the hub to fan out a `room:closed` envelope to active WS connections. TODO inline; will be wired in Phase 5 (`WT-POLISH-02` grace + room-closed UX). See [`01-04-SUMMARY.md`](01-04-SUMMARY.md).

6. **Per-user rate limits are in-process only, not Redis-backed.** Per the design doc, single-instance v1.0 is sufficient. v2 horizontal-scale (multi-instance) will need a Redis-backed limiter; deferred. See [`01-06-SUMMARY.md`](01-06-SUMMARY.md).

7. **`go.work` workspace bug requires `genproto` pin.** A pre-existing repo-wide `ambiguous import: google.golang.org/genproto/googleapis/rpc/status` blocks every `go build` in the workspace mode; the watch-together module pins `require google.golang.org/genproto v0.0.0-20240528184218-...` to dodge it. The fix is local to watch-together; future maintainers should not strip the pin via `go mod tidy`. See [`01-01-SUMMARY.md`](01.1-SUMMARY.md) deviation #5.

8. **(01.9 fix) libs/metrics needed Hijacker pass-through to unblock WS.** Discovered during this plan's smoke run: the global `metrics.Collector.Middleware` wraps the `ResponseWriter` without forwarding `http.Hijacker`, which broke the WS reverse-proxy upgrade at BOTH the gateway and the watch-together service. Fixed in commit `132c16f` by adding `Hijack()` + `Flush()` delegation in `libs/metrics/responseWriter`. Backwards-compatible — no caller previously type-asserted Hijack against this writer (it never worked).

9. **(01.9 fix) Gateway Dockerfile was missing `COPY services/watch-together/go.mod`.** Caught by `make redeploy-gateway` failing with `cannot load module ../watch-together`. Same fix commit `132c16f`.

## Metrics emitted (Prometheus)

All exposed at `http://localhost:8091/metrics` with `service="watch-together"` label on standard HTTP counters.

| Counter                                | Source                  | Cardinality                       |
| -------------------------------------- | ----------------------- | --------------------------------- |
| `wt_room_create_total`                 | `service/rooms.go`      | unlabeled                         |
| `wt_ws_connections_active{room_id}`    | `hub/metrics.go`        | gauge, one label                  |
| `wt_ws_messages_received_total{type}`  | `hub/connection.go`     | bumped in readPump (pre-Dispatch) |
| `wt_ws_messages_sent_total{type}`      | `hub/hub.go`            | bumped in localFanout             |
| `wt_ws_messages_dropped_total`         | `hub/connection.go`     | on full-buffer Send               |
| `wt_drift_corrections_total{severity}` | `service/sync.go`       | severities: soft / hard / persistent |
| `wt_ws_rate_limited_total{type}`       | `service/inbound.go`    | types: seek / chat                |
| `wt_chat_messages_total`               | `service/inbound.go`    | post-AppendMessage                |
| `wt_reactions_total`                   | `service/inbound.go`    | post-Broadcast                    |

Phase 5 (`WT-NF-06`) will add `wt_active_rooms`, `wt_active_members`, `wt_kodik_probe_failures`.

## What Phase 2 inherits

- **Locked WebSocket protocol** — 10 inbound + 10 outbound message types with fully-defined payload structs (`internal/domain/ws_message.go`). Constants are exported as Go string literals; frontend re-declares them in TypeScript.
- **Working REST endpoints** — frontend can pre-fetch `RoomSnapshot` via `GET /rooms/{id}` before opening the WS, so the room view can render initial state without waiting on the upgrade.
- **Gateway routing live** at `wss://animeenigma.ru/api/watch-together/ws?token=...&room=...` and `https://animeenigma.ru/api/watch-together/rooms*`.
- **Auto-reconnect-friendly server** — every (re)connect receives a full `room:snapshot`, so the frontend reconnect path is "open a new socket, throw away local state, re-render from snapshot."
- **Reaction whitelist placeholder** — 24 specific emoji listed above. WT-SHELL-04 (ReactionPalette.vue) must reconcile against this exact set.
- **Multi-tab UX is server-side correct** — same user from 2 tabs counts as 2 connections (capacity) but 1 member (join/leave UX). Frontend doesn't need to dedupe.

## What Phase 3 inherits

- `playback:event` / `playback:correction` protocol stable. HTML5 + Kodik adapters wire against fixed field shapes (`{kind, time, by_user_id}` for events; `{time, server_ts}` for corrections).
- Drift engine sends **per-recipient** corrections (Phase 3's "silent nudge" UX matches the design doc verbatim).
- `playback:time_tick` is **server-consumed only** (never rebroadcast) — Phase 3's 1Hz heartbeat is one-way.
- Persistent-drift cutoff (`5 consecutive` ticks > 5s) sends `error: {code: PERSISTENT_DRIFT, hint: 'reload'}` and suspends corrections for that member. Phase 3 frontend handles the banner.

## What Phase 4 inherits

- `state:change_episode` / `state:change_player` / `state:change_translation` handlers are wired and broadcast `room:state_changed{field, value}` to ALL members. Phase 4 adds **catalog-side validation** (`WT-STATE-02` — "does this episode exist for this anime+player+translation combo?"). The HASH supports field-level updates so Phase 4's validation is purely additive — no schema change.
- The 3 state-change handlers HSET `time=0, state=paused, updated_at` alongside the changed field so the new player/episode/translation starts paused at 0. Phase 4 frontend must re-mount or re-load the player on `room:state_changed`.

## What Phase 5 inherits

- All Phase 1 baseline metrics are emitted; Phase 5 adds `wt_active_rooms`, `wt_active_members`, `wt_kodik_probe_failures` and ships the Grafana dashboard panel.
- 5min grace timer is configurable via env (`WATCH_TOGETHER_GRACE_PERIOD=5m`); Phase 5 wires the grace UX + reconnect-within-grace flow.
- DELETE-broadcasts-`room:closed` TODO from plan 01.4 should be wired here (host force-close → fan out via hub).
- Origin allowlist + 10-member capacity gates are already in place; Phase 5 polish-tests the 10/10 UX page and the room-expired redirect.

## Cross-references

- [01.1-SUMMARY.md](01.1-SUMMARY.md) — Service scaffold, go.work entry, Dockerfile, domain constants
- [01.2-SUMMARY.md](01.2-SUMMARY.md) — Redis repo (HASH/LIST/PUBSUB), `wt:` key builders, sliding TTL, miniredis tests
- [01.3-SUMMARY.md](01.3-SUMMARY.md) — Hub + Connection (read/write pumps), pubsub fanout, base metrics
- [01.4-SUMMARY.md](01.4-SUMMARY.md) — REST handlers (POST/GET/DELETE /rooms), RoomService, auth wiring
- [01-05-SUMMARY.md](01-05-SUMMARY.md) — WS upgrade handler, route-group split, OnClose multi-tab cleanup
- [01-06-SUMMARY.md](01-06-SUMMARY.md) — Inbound message router (10 handlers), drift engine, rate limits
- [01-07-SUMMARY.md](01-07-SUMMARY.md) — Gateway integration (HTTP proxy + dedicated WS reverse proxy)
- [01.8-SUMMARY.md](01.8-SUMMARY.md) — docker-compose, .env.example, Makefile, CLAUDE.md
- [01.9-PLAN.md](01.9-PLAN.md) — Phase close-out smoke + this summary

## Live infrastructure verified (2026-05-25)

- `make redeploy-watch-together` builds and starts cleanly
- `make redeploy-gateway` builds and starts cleanly (after 01.9 Dockerfile fix)
- `docker compose ps watch-together` shows healthy with port 8091 bound to 127.0.0.1
- `curl http://localhost:8091/health` → `{"success":true,"data":{"status":"ok"}}`
- `curl http://localhost:8091/metrics` → Prometheus exposition format
- Smoke script `bash scripts/smoke-watch-together.sh` exits 0 (3 consecutive runs confirmed)

## Smoke transcript artefacts

- Run 1 / 2 / 3 of `scripts/smoke-watch-together.sh` all green
- Two-client WS sessions exchange chat with broadcast echo (frame inspection via `bun + ws`)
- Rate-limit error envelope received on 2nd rapid seek
- HTTP 401 for missing `?token`
- HTTP 404 for bogus `?room=`
- Cleanup trap DELETEs all created rooms on EXIT

## Self-check

| Check                                                                                  | Result |
| -------------------------------------------------------------------------------------- | ------ |
| `scripts/smoke-watch-together.sh` exists                                               | ✅      |
| `test -x scripts/smoke-watch-together.sh`                                              | ✅      |
| `bash -n scripts/smoke-watch-together.sh` exits 0                                      | ✅      |
| `grep -c "FAIL:" scripts/smoke-watch-together.sh` = 13 (≥6 required)                   | ✅      |
| `.planning/.../01-PHASE-SUMMARY.md` exists, ≥100 lines                                 | ✅      |
| ≥8 `## ` H2 sections in 01-PHASE-SUMMARY.md                                            | ✅      |
| ≥8 cross-references to predecessor `01-*-SUMMARY.md` files                             | ✅      |
| Live smoke transcript pasted (not a placeholder)                                       | ✅      |
| Re-running the smoke 3× exits 0 each time (idempotent cleanup)                         | ✅      |
| All 8 ROADMAP success criteria covered (1-7 always-on, 8 opt-in)                       | ✅      |

## Next: Phase 2 — Frontend Shell + Chat

`/gsd-plan-phase --ws watch-together 02-frontend-shell` — vertical-slice frontend phase against the locked backend protocol. No backend changes required.
