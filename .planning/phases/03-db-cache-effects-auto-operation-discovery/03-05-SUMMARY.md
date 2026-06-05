---
phase: 03-db-cache-effects-auto-operation-discovery
plan: 05
subsystem: db-read-gate-feedback-loop (analytics CH→Redis→gormtrace ReadGate)
tags: [observability, db-effects, p95-read-gate, daily-feedback-loop, D-03, AR-EFFECT-01]
requires:
  - "Plan 03-03 — ReadGate interface + snapshotGate + SetSnapshot(map) refresh seam"
  - "Phase-1 ClickHouse events table (effect_kind/operation/target/duration_ms)"
  - "Phase-2 /internal/effects ingestion (db_read rows already flow into events)"
provides:
  - "analytics service.ReadThresholdService: ComputeReadThresholds (quantile(0.95) per op|table) + PublishReadThresholds (read_thresholds Redis hash, 48h TTL)"
  - "analytics POST /internal/read-thresholds/recompute (Docker-network only)"
  - "libs/tracing/gormtrace.ThresholdRefresher: ticker HGETALL read_thresholds -> gate.SetSnapshot (off-path, never per-query)"
  - "scheduler jobs.ReadThresholdJob: daily cron POST to analytics recompute"
affects:
  - "Plan 06 (boot wiring) — each GORM service constructs a ThresholdRefresher over its Redis client + ReadGate and Start()s it"
tech-stack:
  added: []
  patterns:
    - "ClickHouse quantile(0.95)(duration_ms) GROUP BY (operation,target) HAVING count()>=20 → compact Redis hash → in-memory atomic snapshot"
    - "HashReader (HGetAll) interface keeps libs/tracing free of go-redis; services pass a thin adapter at boot"
    - "Producer (analytics, has CH+Redis) computes+publishes; trigger (scheduler, has neither) fires it over /internal HTTP"
key-files:
  created:
    - services/analytics/internal/service/read_threshold.go
    - services/analytics/internal/service/read_threshold_test.go
    - services/analytics/internal/handler/read_threshold.go
    - services/analytics/internal/handler/read_threshold_test.go
    - libs/tracing/gormtrace/threshold_refresh.go
    - libs/tracing/gormtrace/threshold_refresh_test.go
    - services/scheduler/internal/jobs/read_threshold.go
  modified:
    - services/analytics/internal/repo/clickhouse_store.go
    - services/analytics/internal/transport/router.go
    - services/analytics/internal/config/config.go
    - services/analytics/cmd/analytics-api/main.go
    - services/analytics/cmd/analytics-api/adapters.go
    - services/scheduler/internal/service/job.go
    - services/scheduler/internal/config/config.go
    - services/scheduler/cmd/scheduler-api/main.go
decisions:
  - "P95 query lives in repo.QueryReadThresholdP95 (clickhouse_store.go where the driver.Conn lives); the service orchestrates compute+publish. The service file carries the SQL in a doc comment so the source assertion + the structural intent both hold."
  - "Distribution channel: scheduler triggers analytics via /internal HTTP (analytics is the only CH-connected service; scheduler has no CH conn). Analytics does the Redis-direct publish — the plan's preferred Redis path, with the scheduler as a pure daily tick. No new gateway route (T-03-15)."
  - "ThresholdRefresher depends on a narrow HashReader (HGetAll) interface, NOT *redis.Client, so the shared libs/tracing module gains ZERO new dependencies (go.mod/go.sum unchanged across all three modules)."
  - "Empty compute map = no-op publish (never blanks the live hash); the hash itself carries a 48h TTL so one missed daily run is tolerated (T-03-17)."
  - "analytics Redis client is NON-fatal at boot — a Redis outage disables the recompute endpoint (nil handler) but never takes analytics ingestion down."
metrics:
  duration: ~1 work session
  completed: 2026-06-05
  tasks: 2
  files: 15
---

# Phase 3 Plan 05: Daily db_read P95 Feedback Loop Summary

Closed the D-03 loop that makes the plan-03 GORM `ReadGate` dynamic: a daily job computes per-`(operation, target)` `db_read` P95 from the ClickHouse `events` register and publishes a compact `read_thresholds` Redis hash; each GORM service runs a `ThresholdRefresher` ticker that snapshots that hash into its in-memory `ReadGate` (`SetSnapshot`) — off the query hot path, never a per-SELECT lookup (Pitfall 4). Cold-start (empty hash) falls back to the static 50ms default; a poisoned/oversized hash is defensively bounded. `UXΔ = +1 (Better)` · `CDI = 0.06 * 8` · `MVQ = Griffin 84%/83%`.

## What Was Built

**Task 1 — analytics compute + publish (`cf856046`):**
- `repo.QueryReadThresholdP95(ctx, conn, lookbackDays, minSamples)` — runs `SELECT operation, target, quantile(0.95)(duration_ms) FROM events WHERE effect_kind='db_read' AND timestamp >= now() - INTERVAL ? DAY GROUP BY operation, target HAVING count() >= ?` (parameterized; injection-safe). Lives in `clickhouse_store.go` where the `driver.Conn` lives.
- `service.ReadThresholdService` — `ComputeReadThresholds` maps rows to `"operation|target" -> p95_ms` (matching the plan-03 ReadGate snapshot key); `PublishReadThresholds` writes the `read_thresholds` Redis hash with a 48h TTL (empty map = no-op so a transient empty compute never blanks live thresholds); `Recompute` = compute+publish with a nil-conn/nil-writer no-op guard for degraded boots.
- `handler.ReadThresholdHandler` + `POST /internal/read-thresholds/recompute` — Docker-network only, never gateway-proxied (T-03-15), 60s server-side timeout on the CH query.
- analytics gains a non-fatal Redis client (config + main.go); `redisThresholdWriter` adapter does a `DEL`+`HSET`+`EXPIRE` in a `TxPipeline` so a stale field can't survive a recompute. D-16: this path READS the events table and records NO effect.

**Task 2 — refresher + scheduler trigger (RED `8bc9ae88`, GREEN `0e50c225`, scheduler `acd220df`):**
- `gormtrace.ThresholdRefresher` — holds a `HashReader` (`HGetAll`) interface (keeps libs/tracing free of go-redis), a `*snapshotGate`, and an interval. `Start(ctx)` does one immediate refresh then ticks; `refreshOnce` parses `op|table->p95` defensively (skips empty/malformed/oversized/negative/NaN fields, bounds field count — T-03-14) and calls `gate.SetSnapshot`. A read error leaves the prior snapshot intact (never poisons, never kills the ticker). `Stop()` is idempotent.
- `scheduler.jobs.ReadThresholdJob` — daily cron POSTs analytics' recompute endpoint (scheduler has no ClickHouse). Registered in `JobService.Start` wrapped in the existing `SchedulerJobExecutions/Duration/LastSuccess` metrics (label `read_threshold_recompute`), with a nil-job guard. Config: `READ_THRESHOLD_CRON` (default `0 5 * * *`) + `ANALYTICS_INTERNAL_URL`.

## Deviations from Plan

None of substance — plan executed as written.

- **Recompute endpoint chosen over scheduler-side Redis-direct (documented discretion).** The plan offered both ("by calling analytics' `/internal` recompute endpoint ... or ... simply ensures the daily tick fires the analytics path; prefer the simplest wiring"). Since the scheduler has no ClickHouse connection and analytics is the only CH-connected service, the simplest correct wiring is: analytics owns the Redis-direct publish (the plan's *preferred* channel), and the scheduler is a pure daily HTTP tick. This satisfies both "Redis-direct publish preferred" and "scheduler fires the analytics path" without giving the scheduler a CH driver.
- **[Rule 2 — security] Recompute endpoint kept `/internal/*` only with the existing router boundary** — no gateway route added (T-03-15 mitigation), mirroring `/internal/effects`.

## TDD Gate Compliance

Task 2's library (`ThresholdRefresher`) carried a `<behavior>` block → full RED/GREEN:
- RED: `8bc9ae88` `test(03-05): add failing ThresholdRefresher tests` — compile-fail on undefined `NewThresholdRefresher` (verified).
- GREEN: `0e50c225` `feat(03-05): ThresholdRefresher ticker ...` — all five behaviors pass with `-race`. REFACTOR: none needed.

Task 1 (analytics) is a `type="auto"` task without a `<behavior>` block; tests were written alongside (service publish path + handler recompute path) and land in the same `feat` commit, all green.

## Verification

- `cd libs/tracing/gormtrace && go test -race -count=1 ./...` — green (refresher: one-tick-feeds-gate, empty-hash-static-default, read-error-keeps-snapshot, malformed-skipped, Start/Stop-clean + the existing ReadGate/callback suite).
- `cd services/analytics && go build ./...` + `go test ./internal/service/... ./internal/handler/...` — green (publish: op|table fields + 48h TTL + empty-no-op + error-propagation; recompute handler: 204 success / 500 on error / nil-conn no-op).
- `cd services/scheduler && go build ./... && go vet ./internal/... && go test ./internal/...` — green.
- Source assertions: `quantile(0.95)` + `effect_kind = 'db_read'` present in both repo and service files; `read_thresholds` hash name present; `operation|target` join key present; `HAVING count() >= ?` present; `SetSnapshot` called only by the refresher (sole caller); `read_threshold_recompute` metric label wrapped in `job.go`.
- go.mod/go.sum UNCHANGED across analytics, scheduler, and libs/tracing (zero new packages — T-03-SC; clickhouse-go + robfig/cron + go-redis all pre-present).

## Threat Flags

None new. T-03-14 (poisoned/oversized hash) mitigated — refresher bounds field count (`maxThresholdFields=10000`), value length (`maxFieldValueLen=32`), and rejects negative/NaN/Inf/absurd values; the gate consults an in-memory snapshot only (no sync read on the callback). T-03-15 (recompute exposed via gateway) mitigated — endpoint is `/internal/*` only, never gateway-proxied. T-03-16 (analytics self-amplification) — this path only READS events, records nothing (D-16). T-03-17 (missed run blanks table) mitigated — 48h hash TTL + empty-compute no-op + static 50ms default bound any flood. T-03-SC — zero new packages.

## Known Stubs

None. The `read_thresholds` hash is empty until the first successful daily recompute populates it from a full day of `db_read` history — that is correct cold-start behavior (the gate serves the static 50ms default meanwhile), not a stub. Plan 06 wires each GORM service's `ThresholdRefresher.Start()`; until then the refresher exists but is unstarted by design (this plan's scope is the producer + refresher + trigger, not the per-service boot wiring).

## Self-Check: PASSED

- FOUND: services/analytics/internal/service/read_threshold.go
- FOUND: services/analytics/internal/service/read_threshold_test.go
- FOUND: services/analytics/internal/handler/read_threshold.go
- FOUND: services/analytics/internal/handler/read_threshold_test.go
- FOUND: libs/tracing/gormtrace/threshold_refresh.go
- FOUND: libs/tracing/gormtrace/threshold_refresh_test.go
- FOUND: services/scheduler/internal/jobs/read_threshold.go
- FOUND commit: cf856046 (Task 1 feat)
- FOUND commit: 8bc9ae88 (Task 2 RED)
- FOUND commit: 0e50c225 (Task 2 GREEN)
- FOUND commit: acd220df (Task 2 scheduler wiring)
