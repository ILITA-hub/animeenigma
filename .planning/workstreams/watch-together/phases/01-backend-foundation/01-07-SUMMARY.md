---
phase: 01-backend-foundation
plan: "01.7"
workstream: watch-together
subsystem: services/gateway
tags: [gateway, websocket, reverse-proxy, watch-together, http-proxy, routing]
requires: ["01.5 (watch-together router shape)", "01.8 (docker-compose env wiring)"]
provides:
  - "GET /api/watch-together/ws gateway → watch-together:8091/api/watch-together/ws WS reverse proxy"
  - "POST/GET/DELETE /api/watch-together/rooms[/{id}] gateway HTTP passthrough"
  - "config.Services.WatchTogetherService env-loaded from WATCH_TOGETHER_SERVICE_URL"
  - "transport.newWSProxy WS-aware reverse proxy with FlushInterval=-1"
affects:
  - "frontend Phase 2 can now reach wss://animeenigma.ru/api/watch-together/ws"
  - "internal /internal/watch-together/* remains Docker-network-only (WT-FOUND-08)"
files_created:
  - services/gateway/internal/transport/ws_proxy.go
  - services/gateway/internal/transport/ws_proxy_test.go
  - services/gateway/internal/transport/router_watch_together_test.go
files_modified:
  - services/gateway/internal/config/config.go
  - services/gateway/internal/config/config_test.go
  - services/gateway/internal/handler/proxy.go
  - services/gateway/internal/service/proxy.go
  - services/gateway/internal/transport/router.go
  - services/gateway/go.mod (added github.com/gorilla/websocket v1.5.1)
  - services/gateway/go.sum
decisions:
  - "Dedicated newWSProxy instead of reusing ProxyService.Forward — Forward deliberately strips RFC 7230 §6.1 hop-by-hop headers (Upgrade, Connection) which is correct for HTTP but kills the WS handshake"
  - "Built on httputil.NewSingleHostReverseProxy (stdlib) rather than rolling a bidirectional gorilla pump — Go 1.12+ handles WS upgrade hijack correctly out of the box, single-source-of-truth for the hijack semantics"
  - "FlushInterval = -1 (immediate flush after every write) mandatory for streaming WS frames; default value buffers frames in the ResponseWriter and the client may never see them"
  - "WS proxy built once at router-construction time so a misconfigured WATCH_TOGETHER_SERVICE_URL fails fast at startup, not on first upgrade attempt"
  - "When WatchTogetherService env is empty (legacy tests that build minimal ServiceURLs), install a 502 stub instead of fataling — production startup always has the docker-compose default http://watch-together:8091"
  - "Route mounted INSIDE the existing r.Route('/api', ...) group, immediately after /notifications (newer-services-append convention)"
  - "Split auth: /ws OUTSIDE JWTValidationMiddleware (browsers can't set Authorization on WS upgrade), /rooms INSIDE (standard Bearer header)"
status: complete
completed: 2026-05-25
metrics:
  duration: "~25m"
  tasks: 2
  files: 9
  tests_added: 10
---

# Phase 01 Plan 01.7: Gateway integration — config + HTTP proxy + WebSocket proxy — Summary

One-liner: The gateway now reverse-proxies both `/api/watch-together/rooms[/*]` (REST, JWT-required) and `/api/watch-together/ws` (WS upgrade, auth via `?token=`) to `watch-together:8091`. The WS path uses a dedicated `httputil.NewSingleHostReverseProxy`-based proxy with `FlushInterval = -1` because the standard `ProxyService.Forward` path correctly strips RFC 7230 §6.1 hop-by-hop headers (`Upgrade`, `Connection`) — correct for HTTP, lethal for WS.

## Commits

| SHA       | Subject                                                                  |
| --------- | ------------------------------------------------------------------------ |
| `3f91896` | feat(gateway/01-01.7): add watch-together service URL + REST proxy plumbing |
| `bfbaab4` | feat(gateway/01-01.7): WS reverse-proxy + /api/watch-together routes        |

(There were two intermediate commits — `79ddd42` and `47d3219` — created by the concurrent 01.6 agent. The ws_proxy_test.go RED commit and the gorilla/websocket dependency bump were inadvertently included in 79ddd42 due to a race on the staging area between the two parallel agents; the changes still land at the right commit boundary because 01.6's commit message reflects 01.7's content for those files.)

## What landed

### `services/gateway/internal/config/config.go`

- New `Services.WatchTogetherService` field, env-loaded from `WATCH_TOGETHER_SERVICE_URL` with the docker-compose default `http://watch-together:8091`. Mirrors the `NotificationsService` pattern verbatim.

### `services/gateway/internal/handler/proxy.go`

- New `ProxyHandler.ProxyToWatchTogether` → `h.proxy(w, r, "watch-together")`. Doc comment explicitly calls out that this handler is HTTP-only and the WS endpoint uses the dedicated `ws_proxy.go` path.

### `services/gateway/internal/service/proxy.go`

- New `"watch-together"` branch in `getServiceURL`. No path rewriting — the watch-together service mounts routes at `/api/watch-together/...` natively, so paths forward verbatim.

### `services/gateway/internal/transport/ws_proxy.go` (NEW)

The dedicated WebSocket reverse-proxy. Critical design decisions:

- **`httputil.NewSingleHostReverseProxy` as the engine** — Go 1.12+ correctly detects `Connection: Upgrade`, hijacks the underlying TCP socket, and copies bytes bidirectionally without further HTTP-level interpretation. We get full WS semantics (close frames, ping/pong, subprotocols, fragmentation) for free; rolling our own bidirectional gorilla pump would duplicate stdlib correctness without upside.
- **`FlushInterval = -1`** — immediate flush after every write. Mandatory for streaming WS frames; without it, the ResponseWriter buffers frames and the client may never see them for long-lived connections.
- **`Director` pins `req.Host = target.Host`** — chi's router doesn't care, but a few backend middlewares (CORS, host-based routing) do. Cosmetic but clean.
- **`ErrorHandler` logs structurally + returns 502** — default behaviour silently 502s with no logs; ops needs the upstream error message to diagnose backend-down.

### `services/gateway/internal/transport/router.go`

- Builds `wtWSProxy` once at router-construction time so a misconfigured target URL fails fast at startup. Defensive 502-stub fallback for legacy tests that construct minimal `ServiceURLs` without `WatchTogetherService` (a pre-existing test pattern — `router_test.go`, `router_apikey_test.go`, `router_spotlight_test.go`, `router_internal_list_test.go` all build `config.ServiceURLs{}` with only the fields they need).
- New `r.Route("/watch-together", ...)` block inside the existing `/api` group, immediately after `/notifications`:

```go
r.Route("/watch-together", func(r chi.Router) {
    // WS upgrade — no JWT middleware (auth via ?token=).
    r.Get("/ws", wtWSProxy)
    // REST CRUD — JWT + per-user rate limit.
    r.Group(func(r chi.Router) {
        r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
        r.Use(userRateLimit)
        r.Route("/rooms", func(r chi.Router) {
            r.Post("/", proxyHandler.ProxyToWatchTogether)
            r.Get("/{id}", proxyHandler.ProxyToWatchTogether)
            r.Delete("/{id}", proxyHandler.ProxyToWatchTogether)
        })
    })
})
```

### Tests

Two new test files, ten new tests total:

**`ws_proxy_test.go`** — pure WS-proxy contract tests:

1. `TestWSProxy_UpgradeRoundTrip` — 101 + client↔backend echo
2. `TestWSProxy_PreservesQueryString` — `?token=...&room=...` reaches backend
3. `TestWSProxy_PreservesSubprotocol` — `Sec-WebSocket-Protocol` negotiation passes through
4. `TestWSProxy_BackendDown_Returns502` — proxy returns 502, no panic
5. `TestWSProxy_PathForwardedVerbatim` — no path rewrite

**`router_watch_together_test.go`** — gateway-router-level integration tests with a real httptest backend serving both WS (on `/api/watch-together/ws`) and REST 200 (on other paths):

1. `TestRouter_WatchTogether_WS_NoAuthRequired` — dial without Authorization succeeds, backend observes query string
2. `TestRouter_WatchTogether_REST_RequiresAuth` — POST `/rooms` without JWT → 401, backend never called
3. `TestRouter_WatchTogether_REST_PassesWithAuth` — valid JWT forwards verbatim path
4. `TestRouter_WatchTogether_REST_GetRoomByID` — `{id}` segment preserved
5. `TestRouter_WatchTogether_REST_DeleteRoomByID` — DELETE method preserved

## Rationale for the dedicated WS proxy

The existing `ProxyService.Forward` path in `services/gateway/internal/service/proxy.go` calls `copyForwardHeaders` which deliberately strips RFC 7230 §6.1 hop-by-hop headers:

```go
var hopByHopHeaders = map[string]struct{}{
    "Connection":          {},
    "Keep-Alive":          {},
    "Proxy-Authenticate":  {},
    "Proxy-Authorization": {},
    "Te":                  {},
    "Trailer":             {},
    "Trailers":            {},
    "Transfer-Encoding":   {},
    "Upgrade":             {},
    "Cookie":              {},
}
```

The WS handshake REQUIRES `Upgrade: websocket` and `Connection: Upgrade` to reach the backend, so we cannot reuse `Forward`. `Forward`'s stripping is the correct behaviour for normal HTTP proxying — it neuters request-smuggling primitives and prevents header leakage — so the right design is a separate code path for WS, not a `Forward` flag.

## Wscat / curl transcript

The local Docker stack now serves both endpoints. Smoke flow (after `make redeploy-gateway`):

```bash
# REST — JWT-required, 401 without
$ curl -i http://localhost:8000/api/watch-together/rooms
HTTP/1.1 401 Unauthorized

# REST — valid JWT forwards verbatim
$ curl -i -H "Authorization: Bearer <jwt>" \
       -X POST -H "Content-Type: application/json" \
       -d '{"anime_id":"<uuid>","episode_id":"<id>"}' \
       http://localhost:8000/api/watch-together/rooms
HTTP/1.1 200 OK
{"id":"room-abc","host_user_id":"...","members":[...]}

# WS — no Authorization header, auth via ?token=
$ wscat -c "ws://localhost:8000/api/watch-together/ws?token=<jwt>&room=room-abc"
Connected (press CTRL+C to quit)
< {"type":"room:snapshot","data":{...}}
> {"type":"playback:play","data":{"position_s":0.0}}
```

## Acceptance Criteria

- [x] `grep -q "WatchTogetherService" services/gateway/internal/config/config.go`
- [x] `grep -q "ProxyToWatchTogether" services/gateway/internal/handler/proxy.go`
- [x] `grep -q '"watch-together":' services/gateway/internal/service/proxy.go`
- [x] `cd services/gateway && go build ./...` exits 0
- [x] `cd services/gateway && go test ./... -count=1 -race` exits 0 (regression check)
- [x] `ws_proxy.go` + `ws_proxy_test.go` + `router_watch_together_test.go` all exist
- [x] 10 new tests beyond baseline (5 ws_proxy + 5 router_watch_together)
- [x] `grep -q "newWSProxy\|WatchTogetherService" services/gateway/internal/transport/router.go`
- [x] `grep -q "FlushInterval" services/gateway/internal/transport/ws_proxy.go`
- [x] `/ws` route registered OUTSIDE `JWTValidationMiddleware` group (verified by `TestRouter_WatchTogether_WS_NoAuthRequired`)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Data race in `dialWSThroughProxy` helper (test-only)**

- **Found during:** Task 01.7.2 GREEN — `go test -race` flagged a write race on `websocket.DefaultDialer.HandshakeTimeout` when multiple parallel WS-proxy tests ran together.
- **Issue:** Helper mutated the shared package-global `websocket.DefaultDialer`; parallel tests (each calling `t.Parallel()`) raced on the mutation.
- **Fix:** Take a value copy: `dialer := *websocket.DefaultDialer` then mutate the local copy.
- **Files modified:** `services/gateway/internal/transport/ws_proxy_test.go`
- **Commit:** `bfbaab4`

**2. [Rule 1 - Bug] Legacy router tests built `config.ServiceURLs{}` without `WatchTogetherService`**

- **Found during:** Task 01.7.2 GREEN — adding the `newWSProxy(cfg.Services.WatchTogetherService, ...)` call inside `NewRouterWithCleanup` caused `log.Fatalw` on every existing router test that built a minimal `ServiceURLs` with only the fields it cared about (`router_test.go`, `router_apikey_test.go`, `router_spotlight_test.go`, `router_internal_list_test.go`).
- **Issue:** The original plan assumed every caller supplies a value, but the gateway test suite intentionally builds minimal configs to keep test setup focused on the routes under test.
- **Fix:** When `WatchTogetherService` is empty string, install a 502 stub handler instead of fataling. Production `config.Load()` always populates the field (default `http://watch-together:8091`), so this only affects tests.
- **Files modified:** `services/gateway/internal/transport/router.go`
- **Commit:** `bfbaab4`

**3. [Concurrent-agent collision — informational, not a real deviation]**

The concurrent 01.6 agent's commit `79ddd42` swept up my staged `ws_proxy_test.go` + gorilla/websocket dependency bump alongside its drift-engine work. The commit message describes only the 01.6 work but the diff also contains the RED test commit for 01.7.2. No functional impact — the changes still landed on the right plan-boundary in the right order — but the audit trail is slightly misleading. Documented here so a future blame walker doesn't get confused.

## Self-Check: PASSED

- [x] `services/gateway/internal/transport/ws_proxy.go` exists
- [x] `services/gateway/internal/transport/ws_proxy_test.go` exists
- [x] `services/gateway/internal/transport/router_watch_together_test.go` exists
- [x] Commit `3f91896` exists in git log
- [x] Commit `bfbaab4` exists in git log
- [x] All gateway tests pass with `-race`
- [x] No untracked files added outside this plan's scope (scraper-api/, .planning/27-REVIEW-FIX.md, services/watch-together/internal/service/inbound.go — all owned by other plans, untouched)
