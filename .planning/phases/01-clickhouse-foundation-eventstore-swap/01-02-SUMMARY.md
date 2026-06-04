---
phase: 01-clickhouse-foundation-eventstore-swap
plan: 02
subsystem: analytics
tags: [clickhouse, eventstore, schema, contract-test, gdpr, testcontainers]
requires:
  - "services/analytics/internal/domain/store.go (EventStore interface — the swap seam, unchanged)"
  - "services/analytics/internal/repo/postgres_store.go (shape mirrored)"
provides:
  - "ClickHouseStore implementing domain.EventStore via native PrepareBatch"
  - "EnsureSchema: unified wide-event MergeTree + append-only identities + argMax events_resolved view"
  - "EraseByUserIDCH/EraseByAnonymousIDCH (ALTER..DELETE GDPR erase) + PurgeOlderThanCH no-op (native TTL)"
  - "OpenClickHouse native connection constructor"
  - "runEventStoreContract: backend-agnostic EventStore contract (PG/sqlite + real ClickHouse)"
affects:
  - "01-03 (DI wiring): consumes OpenClickHouse(CHConfig) + repo.NewClickHouseStore(conn) + repo.EnsureSchema(ctx, conn)"
tech-stack:
  added:
    - "github.com/ClickHouse/clickhouse-go/v2 v2.42.0 (native client + batch insert)"
    - "github.com/testcontainers/testcontainers-go/modules/clickhouse v0.40.0 (test-only)"
  patterns:
    - "schema-on-boot idempotent EnsureSchema (CH-native IF NOT EXISTS)"
    - "append-only identity + argMax resolved view (no UPDATE)"
    - "native PrepareBatch/Append/Send columnar insert"
key-files:
  created:
    - "services/analytics/internal/repo/clickhouse_schema.go"
    - "services/analytics/internal/repo/clickhouse_store.go"
    - "services/analytics/internal/repo/store_contract_test.go"
  modified:
    - "services/analytics/go.mod"
    - "services/analytics/go.sum"
    - "services/analytics/internal/repo/postgres_store_test.go (standalone tests folded into contract; newTestDB kept)"
decisions:
  - "Pinned clickhouse-go/v2 v2.42.0 + testcontainers-clickhouse v0.40.0 (NOT the plan's v2.46.0/v0.42.0) because those force a go 1.25.0 directive bump across all 25 workspace modules — out of this plan's services/analytics-only scope"
  - "Measure column named row_count (NOT rows) to avoid native-driver friction (RESEARCH A2)"
  - "go.work.sum churn from go work sync deliberately NOT committed (auto-maintained, self-healing; committing prunes other modules' entries and would collide with parallel agents)"
metrics:
  duration: "~25m"
  completed: "2026-06-04"
  tasks: 3
  files-created: 3
  files-modified: 3
---

# Phase 1 Plan 02: ClickHouse EventStore (schema + store + contract) Summary

Implemented a ClickHouse `EventStore` behind the UNCHANGED `domain.EventStore`
interface — the wide-event `MergeTree` schema (one row per effect), the native-
protocol batch-insert store, append-only identity stitching resolved via an
`argMax` view, a GDPR `ALTER TABLE … DELETE` erase path, and a backend-agnostic
contract suite that runs the identical assertions against both the existing
sqlite/Postgres store and a real ClickHouse via testcontainers.

## What was built

### Task 1 — deps + wide-event schema (AR-STORE-02) — `d1f69579`
- `go get clickhouse-go/v2` into `services/analytics` only (no shared `libs/`
  module, so the 13-Dockerfile checklist does not apply).
- `clickhouse_schema.go`: idempotent `EnsureSchema(ctx, conn)` running
  `CREATE DATABASE IF NOT EXISTS analytics` → the unified `events` `MergeTree`
  (`PARTITION BY toYYYYMM(timestamp)`,
  `ORDER BY (toDate(timestamp), origin, operation, effect_kind, target, timestamp)`,
  `TTL toDateTime(timestamp) + INTERVAL 90 DAY DELETE`) → append-only
  `identities` `MergeTree` → the `argMax` `events_resolved` view. All dims
  `LowCardinality`, counters `Delta+ZSTD`, free-text `ZSTD(1)`; `user_id`/
  `anime_id` Nullable and OUT of the sort key (Pitfall 4).

### Task 2 — ClickHouseStore (AR-STORE-05 reuse) — `058ce37e`
- `ClickHouseStore{ conn driver.Conn }` + `NewClickHouseStore` + the
  compile-time assertion `var _ domain.EventStore = (*ClickHouseStore)(nil)`.
- `OpenClickHouse(CHConfig)` native constructor (LZ4 compression, `MaxOpenConns:10`,
  `MaxIdleConns:5`, `ConnMaxLifetime:1h`, `DialTimeout:5s`, `max_execution_time:60`).
- `InsertBatch`: empty-guard → `PrepareBatch("INSERT INTO events")` →
  `batch.Append(...)` per event in exact column order → `Send()`. Preserves the
  `toModel` semantics (`el_attrs`/`properties` default to `"{}"`, `UserID==""` →
  Nullable NULL). Clickstream rows leave the effect dims/measures at defaults
  (`effect_kind=''`).
- `UpsertIdentity`: empty-guard → plain append into `identities`.
- GDPR: `EraseByUserIDCH` (resolves stitched anon ids, then
  `ALTER TABLE events DELETE WHERE user_id = ?` + `… anonymous_id IN (?)` +
  `ALTER TABLE identities DELETE WHERE user_id = ?`), `EraseByAnonymousIDCH`,
  and `PurgeOlderThanCH` (no-op — native TTL handles retention). All
  parameterized `?` placeholders (T-01-04). `resolve.go` left Postgres-only.

### Task 3 — backend-agnostic contract (AR-STORE-03) — `ac93639e`
- `runEventStoreContract(t, newStore)` driven by a `storeHarness` exposing
  backend-neutral `countEvents` + `resolveUser` hooks.
- Sub-tests: `InsertBatch_persists`, `InsertBatch_empty_noop`,
  `UpsertIdentity_latest_wins` (asserts the resolved view's `argMax` latest-wins),
  `UpsertIdentity_empty_noop`.
- `TestPostgresStore_Contract` over in-memory sqlite (Docker-free).
- `TestClickHouseStore_Contract` boots `clickhouse/clickhouse-server:24.3` via
  testcontainers, runs `EnsureSchema`, runs the identical body; skipped under
  `-short`. Truncates between sub-test harness builds for count isolation.

## For 01-03 to wire

- **Connection constructor:** `repo.OpenClickHouse(repo.CHConfig{Addr, Database, Username, Password}) (driver.Conn, error)` — `Addr` is `host:port` for the native protocol (e.g. `"clickhouse:9000"`).
- **Schema bootstrap:** call `repo.EnsureSchema(ctx, conn)` right after opening, mirroring the `repo.AutoMigrateAll`+`repo.EnsureView` call site in `main.go`.
- **Store:** `repo.NewClickHouseStore(conn)` returns a `domain.EventStore` to hand to `ingest.New(...)` unchanged.
- **Measure column** is `row_count`, NOT `rows` (kept consistent in DDL + any future effect-row inserts).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Pinned older CH dep versions to avoid a workspace-wide go directive bump**
- **Found during:** Task 1
- **Issue:** The plan/RESEARCH pinned `clickhouse-go/v2 v2.46.0` (requires go >= 1.24.1) and `testcontainers-go/modules/clickhouse v0.42.0` (requires go >= 1.25.0). With these, `go get` + `go work sync` bumped the `go` directive from `1.24.0` to `1.25.0` across ALL 25 workspace modules and re-pinned transitive deps workspace-wide — far outside this plan's `services/analytics`-only scope and a collision risk for parallel worktree agents.
- **Fix:** Pinned `clickhouse-go/v2 v2.42.0` and `testcontainers-go/modules/clickhouse v0.40.0` — the newest versions that require only go 1.24.0 — keeping the analytics + workspace go directive at `1.24.0`. Both remain official ClickHouse / testcontainers org packages (legitimacy unchanged per threat T-01-SC), so this is a version pin, not a package substitution; no human-verify checkpoint required.
- **Files modified:** `services/analytics/go.mod`, `services/analytics/go.sum`
- **Commit:** `d1f69579` (Task 1), `ac93639e` (testcontainers dep, Task 3)

**2. [Rule 3 - Blocking] Did not commit `go work sync` go.work.sum / cross-module churn**
- **Found during:** Task 1
- **Issue:** `go work sync` (and even a scoped analytics `go build`) rewrites `go.work.sum` and every module's `go.sum` for workspace consistency, pruning checksum entries belonging to modules outside the current build graph. Committing that churn touches 24 unrelated modules and risks colliding with parallel agents.
- **Fix:** Reverted all non-`services/analytics` module files to HEAD and did NOT stage `go.work.sum`. `go.work.sum` is auto-maintained and self-healing — go re-derives any missing entries on demand; the analytics module builds and tests pass with `go.work.sum` at HEAD (verified via `-mod=readonly`). Only `services/analytics/{go.mod,go.sum}` + the new repo files are committed.
- **Files modified:** none beyond analytics (reverted libs/* and other services/*)
- **Commit:** scope enforced across `d1f69579`, `058ce37e`, `ac93639e`

**3. [Rule 2 - Critical] Strengthened the contract's empty/identity coverage**
- **Found during:** Task 3
- **Issue:** The plan's named sub-tests covered persist / empty / latest-wins; `InsertBatch_empty_noop` only asserted no error, and `UpsertIdentity` empty-guard had no explicit test.
- **Fix:** `InsertBatch_empty_noop` now also asserts 0 persisted rows; added `UpsertIdentity_empty_noop` exercising both empty-anon and empty-user guards. Both backends run them.
- **Files modified:** `services/analytics/internal/repo/store_contract_test.go`
- **Commit:** `ac93639e`

## Threat Surface

No new trust boundaries beyond the plan's `<threat_model>`. T-01-04 (parameterized
inserts + erase WHERE clauses) and T-01-05 (PII / 90-day TTL / GDPR erase parity)
are satisfied as designed; no new network endpoints or auth paths introduced.

## Known Stubs

`PurgeOlderThanCH` intentionally returns `(0, nil)` — ClickHouse retention is
handled declaratively by the events table's native `TTL … DELETE`, so the Go
purge cron is retired for the CH backend (RESEARCH §Don't Hand-Roll). This is a
deliberate no-op, not unfinished work; documented in the function comment.

## TDD Gate Compliance

Plan `type: execute` (not `tdd`). Implementation-first with a backend-agnostic
contract suite added in Task 3; the CH contract was verified green against a real
container before completion.

## Verification Results

- `go build ./...` — OK
- `go test ./internal/repo/... -short -count=1` — PASS (PG/sqlite contract, Docker-free)
- `go test ./internal/repo/... -run TestClickHouseStore_Contract -count=1` — PASS (4/4 sub-tests against `clickhouse/clickhouse-server:24.3` via testcontainers; Docker present)
- `go test ./... -short` (whole service) — PASS
- `go vet ./internal/repo/...` — OK
- `domain.EventStore` interface, `ingest/batcher.go`, `observ/metrics.go` — byte-for-byte unchanged vs base (AR-STORE-05 reuse confirmed)

## Self-Check: PASSED

- Files: clickhouse_schema.go FOUND, clickhouse_store.go FOUND, store_contract_test.go FOUND
- Commits: d1f69579 FOUND, 058ce37e FOUND, ac93639e FOUND
