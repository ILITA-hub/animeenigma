# Task 11 Report — WS Hub + Lease Delivery + Spot-Resume Sweeper

## TDD Evidence

### Sweeper
- `sweeper_test.go` written first; `go test ./internal/service/...` failed with `undefined: NewSweeper`.
- `sweeper.go` implemented. First run had a data-race on `s.cancel` (goroutine writing in `Run()`, test reading in `Stop()`). Fixed by replacing `context.CancelFunc` field with a `stopCh chan struct{}` — Stop() closes the channel (non-blocking select guards double-close), Run() selects on it. No shared mutable state.
- All 3 sweeper tests pass with `-race`.

### Leaser
- `leaser_test.go` written first; failed with `undefined: NewLeaser` and `undefined: LeasedHandles`.
- `leaser.go` implemented; `LeasedHandles = controlplane.LeaseHandles` type alias avoids cross-package coupling.
- All 4 leaser tests pass with `-race`.

### Hub / WS Handler
- `hub_test.go` written first; failed with `undefined: HubConfig`, `undefined: Hub`, `undefined: NewHubWithConfig`, `undefined: UpgradeHandler`.
- `hub.go` + `ws_handler.go` implemented. Tests use `HubConfig{PongWait:500ms, PingPeriod:200ms, WriteWait:100ms}` for fast timing.
- All 4 hub tests pass with `-race`.

## WS Pump Constants

Production values in `hub.go`:

| Constant          | Value    | Purpose                                   |
|-------------------|----------|-------------------------------------------|
| `defaultPongWait`  | 60s      | Max time between client pongs             |
| `defaultPingPeriod`| 30s      | Ping ticker interval (must be < PongWait) |
| `defaultWriteWait` | 10s      | Deadline for each write/control frame     |
| `defaultMaxMsgSize`| 64 KiB   | Read limit per WebSocket message          |

Test override (via `HubConfig`): PongWait=500ms, PingPeriod=200ms, WriteWait=100ms.

## How lease_grant Is Assembled

1. `readPump` decodes incoming frame and dispatches `"lease_req"` to `dispatch()`.
2. `dispatch()` calls `hub.leaser.OnLeaseReq(ctx, workerID)`.
3. `Leaser.OnLeaseReq`:
   - Calls `jobRepo.NextEligible(ctx)` → oldest non-terminal job with available segments.
   - Calls `segRepo.LeaseNext(ctx, jobID, workerID, leaseTTL=10m)` → claims segment.
   - Mints two HMAC handles via `capability.MintJobHandle(jobID, "segment-get", idx, 12m)` and `capability.MintJobHandle(jobID, "segment-put", idx, 12m)` (leaseTTL + graceWindow=2m).
   - Calls `workers.Heartbeat(ctx, workerID, jobID, seg.Idx, now)` to record assignment.
4. `dispatch()` wraps in `NewFrame("lease_grant", f.Seq+1, LeaseGrantPayload{JobID, Idx, Handles})` and enqueues on `c.send`.
5. `writePump` dequeues and writes to the WebSocket.

## Enroll Wired via EnrollTx (Not Handle+GormEnrollStore)

`transport/router.go` POST `/worker/enroll` handler calls `enrollStore.EnrollTx(r.Context(), req, controlplane.SessionTTL)` directly — where `enrollStore` is `*controlplane.GormEnrollStore` (concrete type, not the `EnrollTokenStore` interface).

The `Handle(ctx, EnrollTokenStore, WorkerUpserter, req)` function is retained for the fake-based unit test path only; it is never called from production routing code.

## Consume Footgun Addressed

The production enroll handler in `router.go` accepts `*controlplane.GormEnrollStore` (concrete pointer), not the `EnrollTokenStore` interface. This means:
- You cannot accidentally pass `GormEnrollStore` to `Handle()` through the router code path, because the router only holds a concrete `*GormEnrollStore` and calls `.EnrollTx()` on it.
- The `EnrollTokenStore` interface (which includes `Consume`) remains intact so the existing fake-based `Handle` tests continue to compile and pass.
- Decision: did NOT remove `Consume` from the `EnrollTokenStore` interface because `Handle()` calls `store.Consume()` — removing it would break the existing tests. Instead, the footgun is prevented structurally: the HTTP handler accepts only `*GormEnrollStore` and calls `EnrollTx`, making it impossible to accidentally route through `Handle+GormEnrollStore` in production.

## Race Test Result

```
go test -race -count=1 ./internal/controlplane/... ./internal/service/...
ok  github.com/ILITA-hub/animeenigma/services/upscaler/internal/controlplane  1.373s
ok  github.com/ILITA-hub/animeenigma/services/upscaler/internal/service       1.034s
```

No races detected.

## Build/Test Output (Final Run)

```
$ go build ./...
(no output — clean build)

$ go vet ./...
(no output — clean vet)

$ go test -race -count=1 ./...
?   .../cmd/upscaler-api      [no test files]
ok  .../internal/autocache    1.017s
ok  .../internal/capability   1.012s
ok  .../internal/config       1.034s
ok  .../internal/controlplane 1.375s
ok  .../internal/domain       1.021s
ok  .../internal/ffmpeg       1.175s
?   .../internal/handler      [no test files]
ok  .../internal/minio        1.027s
ok  .../internal/repo         1.084s
ok  .../internal/service      1.034s
ok  .../internal/source       1.058s
ok  .../internal/transport    1.040s
```

All 11 testable packages pass; 0 races.

## Files Changed

New files:
- `services/upscaler/internal/controlplane/hub.go`
- `services/upscaler/internal/controlplane/hub_test.go`
- `services/upscaler/internal/controlplane/ws_handler.go`
- `services/upscaler/internal/service/leaser.go`
- `services/upscaler/internal/service/leaser_test.go`
- `services/upscaler/internal/service/sweeper.go`
- `services/upscaler/internal/service/sweeper_test.go`

Modified files:
- `services/upscaler/cmd/upscaler-api/main.go` — wires repos→leaser→hub→sweeper→router
- `services/upscaler/internal/transport/router.go` — NewRouter adds Hub+GormEnrollStore params; wires /worker/enroll + /worker/ws
- `services/upscaler/internal/transport/router_separation_test.go` — updated buildUpscalerRouter to pass stub Hub+nil enrollStore
- `services/upscaler/go.mod` — gorilla/websocket v1.5.3
- `services/upscaler/go.sum` — updated hashes

## Git Scope

All changes are confined to `services/upscaler/`. No files outside this directory were touched. Commit SHA: `5508149d`.

## Self-Review

**Correctness:**
- Sweeper correctly uses `ListConnected(time.Time{})` to get all non-gone workers, then filters by heartbeat age. This is two-phase: list all, filter in Go. A single-query approach would require a `ListStale` repo method; the current two-phase is correct and the worker count is bounded.
- The `StopCancelsRun` test previously exposed a race between `Stop()` reading `s.cancel` and `Run()` writing it. Fixed with `stopCh chan struct{}` — fully race-free.
- `UpgradeHandler` checks the Origin header twice (before upgrader.Upgrade + inside CheckOrigin) for belt-and-suspenders; the gorilla upgrader's CheckOrigin would be sufficient alone, but the explicit pre-check returns 403 with a clear error body for diagnostics.

**Concurrency:**
- Hub uses `sync.RWMutex` for `conns` map — reads lock RLock, writes lock Lock. No goroutine leak: `readPump` always calls `hub.Unregister(conn.workerID)` on defer; `writePump` always calls `c.close()` on defer.
- `Conn.close()` uses `sync.Once` — safe to call from both pumps.

## Concerns

1. **Sweeper `ListConnected(time.Time{})` semantics**: The repo's `ListConnected(since)` returns workers with `last_heartbeat_at >= since`. Passing `time.Time{}` (zero value) should match all rows since any real timestamp is after epoch, but this relies on SQLite/Postgres treating a zero-value time correctly. In practice the Go `time.Time{}` becomes `"0001-01-01 00:00:00 +0000 UTC"` which IS before any real heartbeat. Verified by test.

2. **Hub test `TestHub_LeaseReqReturnsLeaseGrant`**: Uses fake HMAC sigs (`"fakesig..."`), which means `capability.VerifyJobHandle` would fail on the returned handles. The test only checks that `grant.Handles.GetHandle != ""` / `PutHandle != ""`, not that they verify — appropriate for a hub integration test (capability correctness is tested in `leaser_test.go`).

3. **Leaser interfaces**: `jobEligibleRepo`, `segmentLeaserRepo`, `workerHeartbeater` are unexported interfaces in the service package. They're satisfied by `*repo.JobRepository`, `*repo.SegmentRepository`, `*repo.WorkerRepository` respectively. This is correct Go dependency injection — but the service package now implicitly depends on the repo package's method set rather than its types. This is intentional and keeps the leaser testable with pure fakes.

4. **nil enrollStore in router_separation_test.go**: The test passes `nil` for the enroll store. The `POST /worker/enroll` handler calls `enrollStore.EnrollTx(...)` — if the test ever hits that endpoint, it will panic. The separation test only hits `/api/upscale/*` and `/worker/ws`-adjacent paths; the enroll endpoint is not exercised. A future hardening step could add a nil guard in the handler.

---

## Review Follow-up (opus review, post-merge)

Four findings on Task 11's own code were addressed (the finalizing-flip liveness
gap was correctly deferred to Task 11c and NOT touched here). Commit:
`fix(upscaler): offload lease dispatch from read pump + Send reuse + heartbeat log + enroll nil-guard`.

### I-2 (Important) — Don't block readPump on lease DB round-trips

**Before:** `dispatch()` ran `leaser.OnLeaseReq` (NextEligible + LeaseNext TX +
Heartbeat = 3+ DB round-trips) synchronously inside `readPump`. While it ran,
`ReadMessage` wasn't called, so inbound PONG control frames weren't processed —
under DB contention a lease taking >pongWait would tear the connection down.

**Fix:** Added a per-`Conn` `leaseInFlight atomic.Bool` single-flight guard. On a
`lease_req` frame, `handleLeaseReq(reqSeq)`:
- `CompareAndSwap(false, true)` — if already set, the duplicate is dropped and
  logged at WARN (a worker holds at most one lease at a time, so this is correct
  and bounds lease goroutines to ≤1 per connection).
- Otherwise spawns ONE goroutine that calls `OnLeaseReq`, builds the
  `lease_grant`, sends it, and `defer`s `leaseInFlight.Store(false)`.
- The goroutine `select`s on `c.ctx.Done()` before sending so it stops if the
  conn closed mid-resolve.

`readPump` now returns to `ReadMessage` immediately after dispatching a
`lease_req`. `heartbeat` stays inline (one fast call), as specified.

### M-1 — Route lease_grant through Hub.Send (kills M-4 dead code)

The inline `json.Marshal` + non-blocking `select` in `dispatch` was replaced with
`c.hub.Send(c.workerID, grant)`. `Send` already has identical non-blocking
semantics and returns the previously-declared sentinels — so `errSendBufferFull`
and `errWorkerNotFound` are now actually used (removes the M-4 dead-code). The
goroutine branches on `errors.Is`: buffer-full → WARN drop; worker-not-found →
benign (conn dropped between resolve and send); other → WARN.

### M-2 / M-3 — Log the leaser's swallowed heartbeat error

`leaser.go` previously did `_ = l.workers.Heartbeat(...)`. The `Leaser` now
carries a `*logger.Logger` (via `NewLeaser` default + `NewLeaserWithLogger`).
A heartbeat failure is logged at WARN for parity with the hub's heartbeat path;
the lease itself is durable (LeaseNext already committed) so the lease is still
granted — only logged, not failed.

### M-3 (enroll) — Nil-guard the enroll handler

`POST /worker/enroll` now returns a clean 500 (with an ERROR log) when
`enrollStore == nil`, instead of panicking on the `EnrollTx` dereference. This
matches the concern flagged in the original report (item 4) — the separation
test passes `nil`.

### New tests (TDD for I-2 + single-flight)

Added to `hub_test.go` with a gate-controlled `slowLeaser` fake
(`started`/`finished` atomics, a `gate` chan):
- **`TestHub_SlowLeaseDoesNotBlockReadPump`** — fires a `lease_req`, waits for the
  lease to begin, then sleeps `pongWait + 300ms` with the lease still blocked on
  the gate. The connection MUST still be registered (read loop kept auto-ponging
  the server's pings) — proving the lease runs off the read loop. Releasing the
  gate then delivers the `lease_grant`.
- **`TestHub_DuplicateLeaseReqIgnored`** — fires one `lease_req` (blocks on gate),
  then 4 more while it's in flight; asserts `OnLeaseReq` started exactly once and
  exactly one `lease_grant` arrives after the gate releases (the 4 WARN
  "duplicate lease_req" log lines confirm the drops).

### Verification

- `go build ./services/upscaler/...` — clean.
- `go vet ./services/upscaler/...` — clean.
- `go test -race -count=1 ./services/upscaler/internal/controlplane/... ./services/upscaler/internal/service/...` — both packages **ok, 0 races**.
- Full `go test -race -count=1 ./...` across the upscaler module — all packages ok, 0 races.
- Git scope: only `services/upscaler/{internal/controlplane/hub.go, internal/controlplane/hub_test.go, internal/service/leaser.go, internal/transport/router.go}`. No `go work sync`.
