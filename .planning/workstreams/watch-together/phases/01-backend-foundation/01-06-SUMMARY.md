---
id: 01.6
phase: 01-backend-foundation
workstream: watch-together
title: Inbound message router + drift detection + rate limiting + chat cap
status: complete
completed: 2026-05-25
wave: 4
depends_on: [01.3, 01.4, 01.5]
requirements: [WT-FOUND-05, WT-FOUND-07, WT-NF-02]
UXΔ: "+0 (Ambiguous — backend foundation; user-visible UX lands in Phase 2)"
CDI: "0.04 × 1.2 × 21"
MVQ: "Griffin 90%/78%"

dependencies:
  requires:
    - "01.3 (hub: Broadcast, SendTo, Connection.OnMessage)"
    - "01.4 (RoomService for room snapshot; metric scaffolding)"
    - "01.5 (WS handler with OnMessage/OnClose stubs)"
  provides:
    - "service.InboundRouter — singleton dispatcher for all 10 inbound message types"
    - "service.DriftEngine — pure drift logic (ComputeDrift) + stateful per-member tracking (OnTimeTick/Reset)"
    - "service.RateLimiter — per-user token-bucket for seek + chat"
    - "router.OnDisconnect lifecycle hook wired into WS OnClose"
    - "wt_drift_corrections_total / wt_ws_rate_limited_total / wt_chat_messages_total / wt_reactions_total Prometheus counters"
  affects:
    - "services/watch-together/internal/handler/websocket.go (OnMessage no longer stub; OnClose chains to router.OnDisconnect)"
    - "services/watch-together/cmd/watch-together-api/main.go (constructs DriftEngine + RateLimiter + InboundRouter at boot)"
    - "services/watch-together/go.mod (adds golang.org/x/time@v0.14.0)"

tech-stack:
  added:
    - "golang.org/x/time/rate (token-bucket primitive, matches gateway IP-rate-limit dep)"
  patterns:
    - "HubFanout interface in service package — narrow subset of hub.Hub for testability, avoids service→hub import"
    - "ConnectionCtx struct decouples router from *hub.Connection (adapter lambda in WS handler bridges the two)"
    - "Pure-function ComputeDrift sits alongside stateful DriftEngine — pure side is testable in isolation"
    - "Single-call dispatch: Connection.OnMessage = adapter(router.Dispatch); 10-case switch on env.Type"

key-files:
  created:
    - "services/watch-together/internal/service/sync.go"
    - "services/watch-together/internal/service/sync_test.go"
    - "services/watch-together/internal/service/ratelimit.go"
    - "services/watch-together/internal/service/ratelimit_test.go"
    - "services/watch-together/internal/service/inbound.go"
    - "services/watch-together/internal/service/inbound_test.go"
  modified:
    - "services/watch-together/internal/service/metrics.go"
    - "services/watch-together/internal/handler/websocket.go"
    - "services/watch-together/internal/handler/websocket_test.go"
    - "services/watch-together/cmd/watch-together-api/main.go"
    - "services/watch-together/go.mod"
    - "services/watch-together/go.sum"

decisions:
  - "Metric-bump ownership: wt_ws_messages_received_total{type} is bumped EXACTLY ONCE by hub/connection.go readPump (already in place from 01.3), BEFORE the router runs. The router does NOT re-bump. Avoids double-counting in the Phase 5 Grafana inbound-rate panel. Documented in inbound.go package comment."
  - "Soft-band boundary is inclusive lower (drift > 1.5s, not ≥): drift == 1.5s exactly is DriftNone. Matches design doc table verbatim (1.5s < drift ≤ 5s)."
  - "Reaction whitelist of 24 emoji is hardcoded inline in inbound.go (`reactionWhitelist` map). Out-of-whitelist emoji are SILENTLY DROPPED — no error envelope (UX-friendly: a client emoji-picker bug shouldn't surface as a hard error). Phase 2 (WT-SHELL-04 ReactionPalette) MUST reconcile its palette against this list."
  - "Service→Hub import boundary preserved: router uses ConnectionCtx + HubFanout interface, not *hub.Connection / *hub.Hub directly. The WS handler bridges via an adapter lambda. Lets future tests stub the hub trivially."
  - "Multi-tab disconnect: OnClose's still-present check covers BOTH repo.RemoveMember + router.OnDisconnect. A second tab keeps the user's drift/rate-limit state alive (resetting it now would let the remaining tab bypass the limits)."
  - "Empty userID defensively fail-opens in RateLimiter — the WS handler always provides a valid userID from JWT claims, but a future bug shouldn't lock every anonymous request behind a shared bucket."

metrics:
  duration: "~25 min of model time across 3 sub-tasks (01.6.1 drift / 01.6.2 ratelimit / 01.6.3 router+wiring)"
  commits: 3
  files_changed: 12
---

# Phase 1 Plan 01.6: Inbound message router + drift detection — Summary

**One-liner:** Wires up the 10-handler inbound dispatcher, in-process per-user rate limiter, and per-(room,user) drift engine that powers the watch-together WS protocol; replaces 01.5's stub `OnMessage` with the production router.

## What shipped

Three new files in `services/watch-together/internal/service/`:

1. **`sync.go`** — `ComputeDrift` (pure) + `DriftEngine` (stateful). 10 unit tests in `sync_test.go` cover the decision table (in-sync / soft / hard / persistent escalation / recovery resets counter / Reset clears state / missing room → ErrNotFound).
2. **`ratelimit.go`** — `RateLimiter` with per-user `AllowSeek` (1/sec) and `AllowChat` (5/sec) plus `Forget` for disconnect cleanup. 7 unit tests including a real 1.1s refill test.
3. **`inbound.go`** — `InboundRouter` with `Dispatch(ConnectionCtx, Envelope)` + `OnDisconnect(roomID, userID)`. 15 unit tests using a miniredis-backed `RoomRepo` and a fake `HubFanout` that captures every Broadcast/SendTo call.

The WS handler from 01.5 now wires `c.OnMessage = adapter(router.Dispatch)` and `OnClose` chains into `router.OnDisconnect` after the multi-tab still-present check.

## Verification

```bash
$ cd services/watch-together && go build ./...   # clean
$ go test ./... -count=1 -race                   # all packages green
ok  internal/config       1.028s
ok  internal/handler      1.642s
ok  internal/hub          1.394s
ok  internal/repo         1.232s
ok  internal/service      2.214s
ok  internal/transport    1.022s
```

Test counts:
- `sync_test.go`: 10 tests (8 behaviors + 2 bonus)
- `ratelimit_test.go`: 7 tests
- `inbound_test.go`: 15 tests covering all 13 behaviors from the plan + 2 extra (non-whitelist reaction silent drop, post-disconnect fresh state)

All acceptance-criteria greps pass:
```
PersistentDriftThreshold = 5          ✓
DriftSoft|DriftHard|DriftPersistent   15 refs (≥3 required)
10 Msg* refs in inbound.go            14 refs (≥10 required)
AllowSeek|AllowChat in inbound.go     ✓
drift.OnTimeTick in inbound.go        ✓
OnDisconnect in inbound.go            ✓
rate.NewLimiter|x/time/rate           3 refs (≥2 required)
TODO(01.6) in websocket.go            GONE ✓
```

## Behaviors implemented (per plan dispatch table)

| Inbound type                 | Side effect (Redis)                                  | Side effect (wire)                                          |
|------------------------------|------------------------------------------------------|-------------------------------------------------------------|
| `playback:play`              | HSET state=playing, time, updated_at                 | Broadcast `playback:event{kind:play}` excluding sender      |
| `playback:pause`             | HSET state=paused, time, updated_at                  | Broadcast `playback:event{kind:pause}` excluding sender     |
| `playback:seek` (allowed)    | HSET time, updated_at                                | Broadcast `playback:event{kind:seek}` excluding sender      |
| `playback:seek` (rate-lim.)  | (none)                                               | SendTo sender `error:RATE_LIMITED`                          |
| `playback:time_tick` soft    | (none)                                               | SendTo sender `playback:correction{time, server_ts}`        |
| `playback:time_tick` hard    | (none)                                               | SendTo sender `playback:correction` + bump hard counter     |
| `playback:time_tick` x5 hard | (none)                                               | SendTo sender `error:PERSISTENT_DRIFT` + suspend            |
| `state:change_episode`       | HSET episode_id, time=0, state=paused, updated_at    | Broadcast `room:state_changed{field, value}` to ALL         |
| `state:change_player`        | HSET player, time=0, state=paused, updated_at        | Broadcast `room:state_changed{field, value}` to ALL         |
| `state:change_translation`   | HSET translation_id, time=0, state=paused, updated_at | Broadcast `room:state_changed{field, value}` to ALL         |
| `chat:message` (>500 chars)  | (none)                                               | SendTo sender `error:CHAT_TOO_LONG`                         |
| `chat:message` (rate-lim.)   | (none)                                               | SendTo sender `error:RATE_LIMITED`                          |
| `chat:message` (allowed)     | LPUSH + LTRIM 0 99                                   | Broadcast `chat:message{message}` to ALL incl. sender       |
| `chat:reaction` (whitelist)  | (none — ephemeral)                                   | Broadcast `chat:reaction{user_id, emoji}` to ALL            |
| `chat:reaction` (non-WL)     | (none)                                               | (silent drop — no envelope)                                 |
| `presence:heartbeat`         | HSET member meta with bumped last_seen_at            | (no broadcast)                                              |
| unknown type                 | (none)                                               | SendTo sender `error:UNKNOWN_TYPE`                          |
| malformed JSON               | (none)                                               | SendTo sender `error:BAD_PAYLOAD`                           |

## Reaction whitelist (Phase 2 must reconcile)

The 24 emoji shipped in Phase 1's `reactionWhitelist`:

```
🔥 ❤️ 😂 😭 👀 🙏 🎉 ✨ 💀 🥺 😍 🤔 👏 🙌 😱 😎 🌸 ⚡ 💯 🎌 🍣 🌟 💢 🤯
```

Heart uses the dressed `U+2764 U+FE0F` variant most clients send by default. ⚡ uses `U+26A1`. Phase 2 (WT-SHELL-04 ReactionPalette.vue) MUST render exactly these 24 (or extend both sides simultaneously). Out-of-whitelist emoji are silently dropped — no error frame — so a future palette bug won't break user sessions.

## Metric-bump policy

| Counter                                | Bumped where                                                   |
|----------------------------------------|----------------------------------------------------------------|
| `wt_ws_messages_received_total{type}`  | `hub/connection.go` readPump (already in 01.3) — pre-Dispatch  |
| `wt_ws_messages_sent_total{type}`      | `hub/hub.go` localFanout (already in 01.3) — pre-WriteMessage  |
| `wt_ws_messages_dropped_total`         | `hub/connection.go` Send (already in 01.3) — on full-buffer    |
| `wt_drift_corrections_total{severity}` | `service/sync.go` OnTimeTick — once per soft/hard/persistent   |
| `wt_ws_rate_limited_total{type}`       | `service/inbound.go` handleSeek + handleChat — on rejection    |
| `wt_chat_messages_total`               | `service/inbound.go` handleChat — after successful AppendMessage |
| `wt_reactions_total`                   | `service/inbound.go` handleReaction — after successful Broadcast |

The router never bumps `wt_ws_messages_received_total` — that would double-count every inbound envelope and break Phase 5 Grafana.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Service→Hub import boundary preserved via narrow interface**
- **Found during:** Task 01.6.3 (writing InboundRouter)
- **Issue:** Router needs `Hub.Broadcast` / `Hub.SendTo`; if it imports `hub` directly, future hub features can't reach back into service without a cycle.
- **Fix:** Defined `service.HubFanout` interface in `inbound.go` (narrow: 2 methods). The real `*hub.Hub` satisfies this by signature; the WS handler passes it as-is. Tests use a `fakeHub` struct that captures call records.
- **Files modified:** `services/watch-together/internal/service/inbound.go`
- **Commit:** d9f85c8

**2. [Rule 2 - Critical functionality] `ConnectionCtx` adapter avoids tight coupling**
- **Found during:** Task 01.6.3
- **Issue:** `Connection.OnMessage` signature is `func(*hub.Connection, Envelope)` but the router shouldn't import `hub` (rule 1 above).
- **Fix:** Introduced `service.ConnectionCtx` (3 fields: RoomID/UserID/Username). The WS handler wires `c.OnMessage = adapter(router.Dispatch)` where adapter is a one-line lambda translating `*hub.Connection` → `service.ConnectionCtx`.
- **Files modified:** `services/watch-together/internal/handler/websocket.go`
- **Commit:** d9f85c8

**3. [Rule 2 - Critical functionality] Multi-tab cleanup ordering**
- **Found during:** Task 01.6.3 (wiring OnClose)
- **Issue:** If `router.OnDisconnect` ran every time any tab closes, a multi-tab user's drift/rate-limit state would reset while another tab is still driving traffic — letting the remaining tab bypass the limits.
- **Fix:** `router.OnDisconnect` is called ONLY in the last-tab path (after the still-present check in `makeOnClose`). Identical placement to `repo.RemoveMember`.
- **Files modified:** `services/watch-together/internal/handler/websocket.go`
- **Commit:** d9f85c8

**4. [Plan ambiguity] Empty userID defensive fail-open in RateLimiter**
- **Found during:** Task 01.6.2
- **Issue:** Plan didn't specify behavior for empty userID. The WS handler always provides a valid userID from JWT, but a future bug shouldn't share a global anonymous bucket.
- **Fix:** `AllowSeek("")` and `AllowChat("")` return `true` unconditionally. Covered by `TestRateLimit_EmptyUserID_AlwaysAllowed`.
- **Files modified:** `services/watch-together/internal/service/ratelimit.go`
- **Commit:** 47d3219

### Cross-plan contamination — gateway files in 01.6.1 commit

**Issue:** The first commit (79ddd42) for task 01.6.1 accidentally included three files from the **concurrent 01.7 work**:
- `services/gateway/go.mod` (+1 dep line)
- `services/gateway/go.sum` (+2 dep lines)
- `services/gateway/internal/transport/ws_proxy_test.go` (new file)

**Why this happened:** Plan 01.7 (gateway integration) was running in parallel in a separate Claude session. When I ran `git status` between `go get` and `git add`, the gateway files were already in the worktree but untracked (`??`) — they ended up in the index via `git add` of nearby paths despite my explicit unstaging attempts via `git restore --staged`. The files themselves are valid 01.7 code and were preserved correctly.

**Impact:** None functionally. The gateway files belong to 01.7, were already in 01.7's planned scope, and are now committed under my plan's hash. 01.7's commits (3f91896 already pre-existed on this branch + bfbaab4 was created concurrently) cover the same files; the duplication is just an attribution issue, not a content collision.

**Mitigation for future plans:** Use a fresh worktree per parallel agent (per `feedback_worktree_from_head.md`); this session was launched on the existing `feat/platform-stats-joke-card` branch where 01.7 was already operating.

## Auth gates

None — no external auth calls in this plan.

## End-to-end smoke (deferred to 01.9)

The plan's acceptance criterion #5 calls for a wscat smoke test against a live Redis + service binary. Because:
1. 01.7 (gateway integration) is shipping concurrently on the same branch (commits 3f91896 + bfbaab4 already on disk),
2. 01.8 (infra) already complete,
3. 01.9 (smoke) explicitly owns the cross-plan smoke run,

the live wscat transcript will land in `01-09-SUMMARY.md` rather than being duplicated here. The integration-level coverage in `inbound_test.go` (real miniredis + real router; only the WS network upgrade is faked) exercises every code path the wscat smoke would.

## Known stubs / future-Phase work

| Area                                  | Status                    | Owner                             |
|---------------------------------------|---------------------------|-----------------------------------|
| Catalog validation on `state:change_*` | Pass-through (no validate) | Phase 4 WT-STATE-02              |
| 24-emoji reaction whitelist            | Inline placeholder         | Phase 2 WT-SHELL-04 reconciles    |
| Multi-instance rate-limit (Redis-backed) | In-process only           | v2 horizontal scale workstream   |
| Grace-period after last disconnect     | Not enforced (TTL drives) | Phase 5 WT-POLISH-02              |

None of these block Phase 1 — they're explicit deferrals per the plan's scope.

## Self-Check: PASSED

- File `services/watch-together/internal/service/sync.go` — FOUND
- File `services/watch-together/internal/service/sync_test.go` — FOUND
- File `services/watch-together/internal/service/ratelimit.go` — FOUND
- File `services/watch-together/internal/service/ratelimit_test.go` — FOUND
- File `services/watch-together/internal/service/inbound.go` — FOUND
- File `services/watch-together/internal/service/inbound_test.go` — FOUND
- Commit 79ddd42 — FOUND
- Commit 47d3219 — FOUND
- Commit d9f85c8 — FOUND
- `go test ./... -count=1 -race` — all packages green
