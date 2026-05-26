---
phase: 01-backend-foundation
plan: "01.5"
workstream: watch-together
subsystem: services/watch-together
tags: [websocket, lifecycle, auth, capacity, member-presence]
requirements: [WT-FOUND-04, WT-NF-01, WT-NF-02]
status: complete
completed: 2026-05-25
dependency_graph:
  requires: ["01.3 (Hub)", "01.4 (RoomService + REST handlers)"]
  provides: ["GET /api/watch-together/ws upgrade endpoint", "Connection.OnClose lifecycle hook", "Config.AllowAllOrigins"]
  affects: ["01.6 inbound message router (wires Connection.OnMessage)", "01.7 gateway proxy", "01.9 phase smoke"]
tech_stack:
  added: []
  patterns: ["gorilla/websocket upgrader with Origin allowlist", "JWT-in-query-param auth for WS", "multi-tab-aware last-connection cleanup"]
key_files:
  created:
    - services/watch-together/internal/handler/websocket.go
    - services/watch-together/internal/handler/websocket_test.go
  modified:
    - services/watch-together/internal/hub/connection.go
    - services/watch-together/internal/hub/hub.go
    - services/watch-together/internal/config/config.go
    - services/watch-together/internal/transport/router.go
    - services/watch-together/internal/transport/router_test.go
    - services/watch-together/cmd/watch-together-api/main.go
decisions:
  - "Pre-upgrade HTTP 401/400/404 (NOT WS close frames) for token / room-id / room-existence failures — surfaces clearly in browser dev-tools network panel"
  - "Connection.OnClose hook amends the 01.3 hub contract; fires AFTER room-set removal but BEFORE conn.Close so OnClose observers see consistent post-removal state"
  - "Multi-tab cleanup: repo.RemoveMember + member:left broadcast fire ONLY when the user's LAST connection leaves (check Hub.MemberUserIDs post-Unregister)"
  - "Origin allowlist allows missing Origin header (wscat / smoke / curl-based ops tooling) but rejects mismatched browser Origins"
metrics:
  duration: "~45m"
  tasks: 1
  files: 8
  tests_added: 16
---

# Phase 01 Plan 01.5: WebSocket upgrade handler Summary

One-liner: `GET /api/watch-together/ws` ships with JWT-via-query-param auth, pre-upgrade room+capacity gates, snapshot-on-connect, and a multi-tab-aware OnClose path that broadcasts `member:left` + `repo.RemoveMember` only when the user's last tab disconnects.

## What landed

- **`internal/handler/websocket.go`** — `WebSocketHandler` with the full upgrade lifecycle: token validation, room presence check, gorilla upgrade, capacity gate (post-upgrade close-frame path), `repo.AddMember`, `hub.Register`, `room:snapshot` send, `member:joined` fanout. Installs an `OnClose` callback that handles disconnect cleanup with multi-tab awareness.
- **`internal/hub/connection.go`** + **`hub/hub.go`** — added `Connection.OnClose func(*Connection)` field. `Hub.Unregister` fires it AFTER removing the connection from the room set but BEFORE closing the wire, with a panic-safe recover wrapper so a buggy callback can't take down the hub goroutine. This is a small, additive amendment to the 01.3 contract.
- **`internal/config/config.go`** — added `AllowAllOrigins bool` (env `WATCH_TOGETHER_ALLOW_ALL_ORIGINS`) + `getEnvBool` helper. Off by default so production stays Origin-locked to `PublicBaseURL`; flip on for local dev across Vite ports.
- **`internal/transport/router.go`** — route group restructure. `/ws` mounted at depth 1 of the `/api/watch-together` `chi.Route`; `/rooms` wrapped in a nested `chi.Group` that applies `AuthMiddleware`. The WS endpoint sits OUTSIDE the auth middleware because browsers can't set `Authorization: Bearer` on the WS upgrade handshake — the handler does its own JWT validation from `?token=`.
- **`cmd/watch-together-api/main.go`** — boot sequence updated: `uuid.NewString()` for `instanceID`, construct `hub.Hub` with it, construct `WebSocketHandler`, pass into `NewRouter`. On SIGTERM, `wsHub.Close()` runs BEFORE `srv.Shutdown` so live WS connections drain cleanly.
- **`internal/handler/websocket_test.go`** — 16 test cases via real `httptest.Server` + `gorilla/websocket` dialer + JWT minting against the fixture's secret. Covers every acceptance branch in the plan (missing/invalid token, missing/non-existent room, success+snapshot, two-client `member:joined`, capacity full, clean disconnect, abrupt close, multi-tab, Origin allowlist).

## Auth-path divergence from CONTEXT.md (documented for 01.7 / 01.9)

CONTEXT.md §WebSocket Endpoint specified ROOM_NOT_FOUND as a close-frame error:
> Validates room exists in Redis; rejects with `error: {code: 'ROOM_NOT_FOUND'}` close frame if not

Plan 01.5's `<lifecycle_contract>` deliberately diverged: pre-upgrade HTTP 404 is more debuggable than a successful upgrade followed by an immediate close. The frontend in Phase 2 will see HTTP 404 in the network panel (vs a "what just happened" close event). Implementation matches the plan, not CONTEXT.md.

CAPACITY_FULL stays as the original CONTEXT.md spec: post-upgrade text frame with the error envelope + WS close-control frame. The frontend treats both shapes identically.

Net effect for the gateway in 01.7: the gateway just passes the upstream HTTP response through unchanged. Both 404 and 101+close-frame land at the client correctly.

## Route group restructure

Before 01.5:
```go
r.Route("/api/watch-together", func(r chi.Router) {
    r.Use(AuthMiddleware(cfg.JWT))
    r.Route("/rooms", ...)
    // /ws would land here, BUT the AuthMiddleware would 401 every WS
    // upgrade (no Authorization header on browser WS).
})
```

After 01.5:
```go
r.Route("/api/watch-together", func(r chi.Router) {
    if wsHandler != nil {
        r.Get("/ws", wsHandler.Upgrade) // OUTSIDE the auth group
    }
    if roomHandler != nil {
        r.Group(func(r chi.Router) {
            r.Use(AuthMiddleware(cfg.JWT))
            r.Route("/rooms", ...) // INSIDE the auth group
        })
    }
})
```

Pre-existing `/rooms` tests (`TestCreate_*`, `TestGet_*`, `TestDelete_*` in `rooms_test.go`) still pass — the auth scope around them is unchanged.

## Hub amendment (01.3 → 01.5)

The 01.3 plan called out that 01.5 might need to extend the hub. We added one field:

```go
// In Connection:
OnClose func(*Connection)

// In Hub.Unregister, after removing from room set, before c.Close():
if c.OnClose != nil {
    func() {
        defer func() {
            if rec := recover(); rec != nil {
                h.log.Errorw("watch_together hub OnClose panic", ...)
            }
        }()
        c.OnClose(c)
    }()
}
```

Order matters: the callback observes the post-removal hub state, so `hub.MemberUserIDs(roomID)` inside the callback returns the deduplicated user set EXCLUDING the leaving connection. That lets the WS handler implement multi-tab cleanup with a simple "is my userID still in the list" check.

## Multi-tab semantics

Per plan §<tasks>/Test 10: same `userID` connecting from two tabs counts as 2 connections (capacity gate uses raw connection count) but 1 logical user (member presence). When ONE tab leaves:
- Hub.Unregister removes the connection from the room set.
- OnClose fires, checks `Hub.MemberUserIDs(roomID)`, finds the userID still present (other tab).
- Skips `repo.RemoveMember` and `member:left` broadcast.

When the LAST tab leaves:
- OnClose finds no remaining connections for that userID.
- Calls `repo.RemoveMember`.
- Broadcasts `member:left` to everyone remaining in the room.

This avoids the "user opens 2 tabs, closes 1, everyone sees them leave/rejoin" UX bug.

## Test coverage (16 cases)

| Test | Behavior |
|------|----------|
| `TestWS_MissingToken_Returns401` | No `?token=` → HTTP 401 pre-upgrade |
| `TestWS_InvalidToken_Returns401` | Bad-signature JWT → HTTP 401 pre-upgrade |
| `TestWS_MissingRoom_Returns400` | Valid token, no `?room=` → HTTP 400 pre-upgrade |
| `TestWS_NonExistentRoom_Returns404` | Valid token, unknown room → HTTP 404 pre-upgrade |
| `TestWS_Success_FirstFrameIsRoomSnapshot` | Happy path: 101 Switching Protocols + `room:snapshot` is the FIRST frame |
| `TestWS_TwoClients_FirstSeesMemberJoined` | Two clients: B's snapshot includes A; A receives `member:joined` for B |
| `TestWS_CapacityFull_ThirdConnectionRejected` | MaxMembers=2 + 3rd dial → upgrade then `CAPACITY_FULL` error envelope + close |
| `TestWS_Disconnect_BroadcastsMemberLeftAndRemovesMember` | Clean close: peer sees `member:left`; member removed from Redis |
| `TestWS_AbruptClose_FiresOnCloseCleanup` | TCP-RST: cleanup still fires via OnClose hook |
| `TestWS_MultiTab_OnlyLastTabFiresMemberLeft` | Two tabs same user: only LAST close triggers cleanup |
| `TestWS_OriginAllowlist_RejectsMismatchedBrowserOrigin` | AllowAllOrigins=false + bad Origin → HTTP 403 from upgrader |
| `TestBuildWSOriginCheck` (5 subtests) | Allowlist unit: allow-all override, no-Origin allowed, matching origin, mismatched rejected, malformed rejected |

`go test ./internal/handler/... ./internal/transport/... -count=1 -race` passes in ~1.6s.

## Smoke-test recipe (deferred to 01.9 — full Compose stack required)

The plan's end-to-end smoke (`<acceptance_criteria>` final block) requires the full service to be running with Redis. Recipe for 01.9:

```bash
# 1. Boot
make redeploy-watch-together  # requires Plan 01.8's docker-compose entry

# 2. Mint JWT (use ui_audit_bot's API key for the auth flow)
JWT=$(curl -s -X POST http://localhost:8000/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"ui_audit_bot@animeenigma.test","password":"audit_bot_test_password_2026"}' \
  | jq -r .access_token)

# 3. Create room
ROOM_ID=$(curl -s -X POST http://localhost:8000/api/watch-together/rooms \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"anime_id":"<uuid>","episode_id":"1","player":"animelib","translation_id":"<id>"}' \
  | jq -r .data.room_id)

# 4. Two wscat sessions
wscat -c "ws://localhost:8091/api/watch-together/ws?token=$JWT&room=$ROOM_ID"
# First frame: {"type":"room:snapshot","data":{...}}
# In a second terminal:
wscat -c "ws://localhost:8091/api/watch-together/ws?token=$JWT&room=$ROOM_ID"
# Second frame in first terminal: {"type":"member:joined","data":{"user_id":"...",...}}

# 5. Negative cases
wscat -c "ws://localhost:8091/api/watch-together/ws"               # → HTTP 401
wscat -c "ws://localhost:8091/api/watch-together/ws?token=$JWT"    # → HTTP 400 (missing room)
wscat -c "ws://localhost:8091/api/watch-together/ws?token=$JWT&room=nope"  # → HTTP 404
```

Note that step 4 uses the service port 8091 directly (no gateway WS proxy yet — that's Plan 01.7). After 01.7, the same wscat hits `ws://localhost:8000/api/watch-together/ws?...` through the gateway.

## Decisions

1. **HTTP-status pre-upgrade rejections instead of close frames.** Better debuggability in browser dev-tools; user-facing UX is identical because the frontend treats both shapes the same.
2. **OnClose callback installed on the Connection struct.** Minimal hub-contract amendment, panic-safe via recover. 01.6 reuses the same hook to wire inbound dispatch (`OnMessage` field, also already present).
3. **Multi-tab last-connection-only cleanup.** Eliminates the join/leave noise from tab switches. Implementation is one map iteration on `Hub.MemberUserIDs` post-Unregister.
4. **Origin allowlist allows missing Origin header.** wscat / smoke / curl-based ops tooling never sets Origin; rejecting them would break the plan's acceptance criterion smoke recipe and any future operational debugging.
5. **`writeCloseFrameError` sends BOTH a text frame and a close-control frame.** Text frame so the client's standard envelope decoder picks up the error; close frame so the WS `close` event fires with a clean reason. Belt-and-braces.

## Deferred (handed to 01.6 / 01.7 / 01.9)

- **Inbound message router** (Plan 01.6): wires `Connection.OnMessage = inboundRouter.Dispatch`. Currently stubbed with a TODO comment.
- **Drift detection** (Plan 01.6): consumes `playback:time_tick`, emits `playback:correction`.
- **Gateway proxy** (Plan 01.7): adds `WatchTogetherURL` to gateway config, mounts HTTP + WS proxy at `/api/watch-together/*`.
- **End-to-end smoke** (Plan 01.9): wscat against the full Compose stack.

## Self-Check: PASSED

- `services/watch-together/internal/handler/websocket.go` exists
- `services/watch-together/internal/handler/websocket_test.go` exists
- 16 test cases run via `go test ./internal/handler/... -run WS -count=1 -v` (count via `grep -cE "^=== RUN.*TestWS"`)
- `go build ./...` exits 0
- `go test ./... -count=1 -race` exits 0
- `/ws` route is OUTSIDE the AuthMiddleware-wrapped `chi.Group` (verified via line-order inspection of `router.go`)
- `ValidateAccessToken` referenced in `websocket.go` (1 call site)
- `CAPACITY_FULL` / `ErrCodeCapacityFull` referenced in `websocket.go` (5 occurrences)
- `uuid.NewString` / `instanceID` wired in `main.go` (3 references)
- Commits: `513fc02` (hub OnClose + config) and `c224d99` (WS handler + router + main)
