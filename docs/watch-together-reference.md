# Watch Together — Reference

> Extracted from `CLAUDE.md` (2026-06-03) to keep the root guidelines under the context-size budget. This is the canonical inline reference for the Watch Together subsystem. See also the design doc and workstream linked at the bottom.

Watch Together — ephemeral private friend rooms (2-10 members) for synchronized anime watching across all 5 players. The Watch Together v1.0 milestone shipped 2026-05 across 5 phases (backend foundation → frontend shell → player sync → state switching → polish). State is Redis-only with sliding 15min TTL + 5min last-disconnect grace.

**Architecture:**
- Single Go microservice `services/watch-together/` (port 8091) — no Postgres, no migrations, Redis-only state under the `wt:` key prefix.
- REST for room lifecycle (POST/GET/DELETE `/rooms`), WebSocket at `/ws?token=&room=` for real-time sync/chat/reactions/state-changes.
- All 10 inbound + 10 outbound message types defined in `services/watch-together/internal/domain/ws_message.go` (protocol_version `"1.0"`, forward-compat field on every snapshot).
- Drift detection engine with soft (>1.5s) / hard (>5s) / persistent (5 consecutive) thresholds; per-recipient `playback:correction` envelopes.
- In-process per-user rate limits (1 seek/s, 5 chat/s) via `golang.org/x/time/rate` token buckets. v2 horizontal-scale will need a Redis-backed limiter (deferred).
- State validation (episode/player/translation switches): synchronous call to catalog's `/internal/anime/{id}/episodes/validate` with 3s timeout + 5s positive-result cache. Permissive for ourenglish/hanime/raw (v1.1 will tighten).
- 5min grace timer (`internal/service/grace.go`): last-member-disconnect starts a `time.AfterFunc`; returning member cancels; timer fire broadcasts `room:closed` + deletes Redis keys.

**HTTP + WS surface (gateway-routed):**
- `POST   /api/watch-together/rooms` — create room (JWT required)
- `GET    /api/watch-together/rooms/{id}` — full RoomSnapshot or 410 Gone (JWT required)
- `DELETE /api/watch-together/rooms/{id}` — host-only force-close (broadcasts `room:closed` then deletes)
- `WS     /api/watch-together/ws?token=<jwt>&room=<id>` — bidirectional sync channel

**Frontend:**
- Route `/watch/room/:roomId` → `WatchTogetherView.vue` (chunk ~6.6 kB gz, lazy)
- Composable `useWatchTogetherRoom(roomId)` owns WS lifecycle + reconnect backoff + snapshot replay + 9 emit methods + 10 subscribe methods (`onPlaybackEvent`, `onStateChanged`, `onRoomClosed`, `onAuthExpired`, `onError`, …)
- Player sync via `usePlayerSyncBridge(videoRef, room)` for HTML5 players (AnimeLib/OurEnglish/Hanime/Raw); Kodik via `kodik_player_api` postMessage RPC adapter with boot-time smoke probe + daily Playwright canary at `frontend/web/e2e/kodik-rpc-probe.spec.ts`.
- Reaction whitelist: 24 emoji declared in both backend (`internal/service/inbound.go:reactionWhitelist`) AND frontend (`@/types/watch-together.REACTION_WHITELIST`); MUST update both sides in lock-step.

**Env vars (set in `docker/.env`):**

| Var | Default | Purpose |
|-----|---------|---------|
| `WATCH_TOGETHER_PORT` | 8091 | Service listen port |
| `WATCH_TOGETHER_REDIS_ADDR` | redis:6379 | Redis backend |
| `WATCH_TOGETHER_JWT_SECRET` | `${JWT_SECRET}` | JWT validation; same secret as auth service |
| `WATCH_TOGETHER_MAX_MEMBERS` | 10 | Per-room capacity cap |
| `WATCH_TOGETHER_ROOM_TTL` | 15m | Sliding TTL on `wt:room:*` keys |
| `WATCH_TOGETHER_GRACE_PERIOD` | 5m | Last-disconnect grace before delete |
| `WATCH_TOGETHER_PUBLIC_BASE_URL` | https://animeenigma.ru | Used to construct invite URLs |
| `WATCH_TOGETHER_ALLOW_ALL_ORIGINS` | false | Dev override for WS origin allowlist |
| `WATCH_TOGETHER_CATALOG_URL` | http://catalog:8081 | Catalog HTTP back-channel for state validation |

The gateway side reads `WATCH_TOGETHER_SERVICE_URL` (default `http://watch-together:8091`) — separate var so the gateway can be redeployed without touching the watch-together service's own env.

**Locked decisions (across all 5 phases):**
- Port 8091, Redis-only, `wt:` key prefix, 15min sliding TTL, 5min grace, capacity 10 members.
- WS auth: `?token=` query param (browsers can't set `Authorization: Bearer` on WS upgrade).
- Pre-upgrade rejections use HTTP 401/400/404 (NOT close frames) for debuggability.
- DELETE `/rooms` broadcasts `room:closed` BEFORE deleting Redis keys (Plan 05.1 closed the original 01.4 TODO).
- 24-emoji reaction whitelist — must be reconciled across backend `internal/service/inbound.go:reactionWhitelist` and frontend `@/types/watch-together.REACTION_WHITELIST`.
- Permissive episode validation for ourenglish/hanime/raw (v1.1 will tighten via scraper round-trip; v1.0 trusts user selection).
- Window test hook `__wtTestRoom` is exposed via `VITE_TEST_HOOK` in dev/test builds only — NEVER ship in production builds.

**References:**
- Design doc: [`docs/superpowers/specs/2026-05-25-watch-together-design.md`](superpowers/specs/2026-05-25-watch-together-design.md)
- Workstream: [`.planning/workstreams/watch-together/`](../.planning/workstreams/watch-together/) (v1.0 milestone, 5 phases)
- Phase summaries: [Phase 1](../.planning/workstreams/watch-together/phases/01-backend-foundation/01-PHASE-SUMMARY.md) · [Phase 2](../.planning/workstreams/watch-together/phases/02-frontend-shell/02-PHASE-SUMMARY.md) · [Phase 3](../.planning/workstreams/watch-together/phases/03-player-sync/03-PHASE-SUMMARY.md) · [Phase 4](../.planning/workstreams/watch-together/phases/04-state-switching/04-PHASE-SUMMARY.md) · [Phase 5](../.planning/workstreams/watch-together/phases/05-polish/05-PHASE-SUMMARY.md)
- Grafana dashboard: `infra/grafana/dashboards/watch-together.json` (auto-provisioned; UID `watch-together`)
- Kodik RPC reference: `reference_kodik_inbound_postmessage_api.md` (user memory)

**Dependency audit (WT-NF-05):**

Backend (`services/watch-together/go.mod` direct requires — verified 2026-05-26 against go.mod):
- `github.com/gorilla/websocket` — WS lib
- `github.com/redis/go-redis/v9` — Redis client
- `github.com/go-chi/chi/v5` — HTTP router
- `github.com/golang-jwt/jwt/v5` — JWT validation (direct, NOT via a `libs/jwt` wrapper)
- `golang.org/x/time/rate` — token buckets
- `github.com/google/uuid` — instance/room IDs
- `github.com/prometheus/client_golang` + `client_model` — metrics (already used project-wide)
- `github.com/alicebob/miniredis/v2` — test-only Redis fake
- Project libs reused: `libs/authz`, `libs/cache`, `libs/errors`, `libs/httputil`, `libs/logger`, `libs/metrics`

Every direct require is either a project-default already in use elsewhere in `services/` (`chi`, `prometheus`, `uuid`, `go-redis`, `golang-jwt`) or a thin, well-known utility (`gorilla/websocket`, `golang.org/x/time`, `miniredis`). No license-incompatible deps; all licenses are MIT/BSD/Apache-compatible with the project (MIT).

Frontend: ZERO new npm dependencies introduced by Watch Together across all 5 phases. Verified via `git log --since='2026-05-20' frontend/web/package.json` — the only diff in that window is `@axe-core/playwright` from the unrelated hero-spotlight workstream. All UI uses pre-existing vue 3 + pinia + vue-i18n + vue-router + tailwind + `@vueuse/core` + `hls.js` + `ass-compiler`.

Audited 2026-05-26 (close of v1.0).

**Daily Kodik canary CI:**

The Kodik postMessage RPC is undocumented; the bundle could change at any time. A Playwright spec at `frontend/web/e2e/kodik-rpc-probe.spec.ts` runs daily via GitHub Actions (`.github/workflows/watch-together-kodik-canary.yml`) and alerts via Telegram on failure. If you see this alert: confirm `kodik_player_api` still works in browser DevTools by sending `{key:'kodik_player_api',value:{method:'get_time'}}` to a Kodik iframe; if the dispatcher really has changed, all rooms using Kodik will fall back to the "Kodik sync unavailable" banner mode — single-user playback still works.
