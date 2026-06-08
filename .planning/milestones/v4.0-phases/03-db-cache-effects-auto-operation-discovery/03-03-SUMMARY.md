---
phase: 03-db-cache-effects-auto-operation-discovery
plan: 03
subsystem: libs/tracing/gormtrace (DB-effect plane)
tags: [observability, db-effects, gorm-callbacks, p95-read-gate, D-01, D-02, D-04, D-11, AR-EFFECT-01]
requires:
  - "Plan 03-01 — extended Effect contract (TargetKind/Rows/AnimeID + op PC carrier) + CaptureOperationPCs/UserIDFromContext"
  - "Phase-2 global EffectSink (SetGlobalSink) — the production sink RegisterEffectCallbacks emits to"
provides:
  - "RegisterEffectCallbacks(db, sink, gate) — GORM after-callbacks: db_write (always) + db_read (P95-gated)"
  - "ReadGate interface + snapshotGate in-memory P95 threshold snapshot (atomic, lock-free reads)"
  - "NewReadGate(staticDefaultMS) + SetSnapshot(map) — the plan-05 daily-P95 refresher seam"
affects:
  - "Plan 05 (daily-P95 producer) — REFRESHES the ReadGate snapshot via SetSnapshot off-path"
  - "Plan 06 (boot wiring) — calls RegisterEffectCallbacks in each GORM service main.go (NEVER analytics — D-16/T-03-09)"
tech-stack:
  added: []
  patterns:
    - "GORM Before/After callback seam: per-statement start time on Statement.Settings (sync.Map), read RowsAffected/Table/Context in After"
    - "P95 read gate as an atomic.Value snapshot — pure in-memory, zero sync IO on the hot path (Pitfall 4)"
    - "Sync PC capture (CaptureOperationPCs) + async resolve (Producer side) for db effects too (D-11)"
key-files:
  created:
    - libs/tracing/gormtrace/readgate.go
    - libs/tracing/gormtrace/readgate_test.go
    - libs/tracing/gormtrace/gorm_effect.go
    - libs/tracing/gormtrace/gorm_effect_test.go
  modified: []
decisions:
  - "gorm.go left UNCHANGED — no shared helper was cleaner; the plan's action granted that discretion (frontmatter listed it speculatively). InstrumentGORM stays a one-liner; RegisterEffectCallbacks is a sibling entry point."
  - "Stash the per-statement start time on db.Statement.Settings (the live statement's sync.Map) rather than a package-level map — each query owns its start time with zero cross-goroutine shared mutable state."
  - "Resolve the coarse operation label inline for the gate lookup, then carry the PCs via WithOperationPCs so the Producer re-resolves the FINE label async (D-11) — the gate needs an op key now, the wire wants the precise one later."
metrics:
  duration: ~1 work session
  completed: 2026-06-05
  tasks: 2
  files: 4
---

# Phase 3 Plan 03: GORM DB-Effect Callbacks + P95 Read Gate Summary

Added the GORM after-callback set that fact-rows DB writes as `db_write` effects (always, carrying table/op/RowsAffected) and DB reads as `db_read` effects only when a SELECT exceeds its own per-`(operation, table)` P95 — built on a pure in-memory `ReadGate` snapshot with a static 50ms cold-start default and an atomic refresh seam for the plan-05 daily-P95 producer. AR-EFFECT-01: the write path is deterministic, the read path is the P95-gated sparse capture, and the callbacks issue ZERO DB queries so RowsAffected is never zeroed (gorm #7044).

## What Was Built

**Task 1 (TDD) — `readgate.go` + tests:**
- `ReadGate` interface — `ShouldRecord(operation, table string, durationMS int) bool`, the hot-path read decision.
- `snapshotGate` — holds `map[string]float64` keyed `operation+"|"+table -> p95_ms` behind an `atomic.Value` for lock-free reads; `SetSnapshot(map)` swaps the whole map atomically (the plan-05 refresher seam). `ShouldRecord` returns `durMS > p95` when the key is present, else `durMS > staticDefaultMS`.
- `NewReadGate(staticDefaultMS int)` — exposes the cold-start threshold (A1, defaults to 50) so it is tunable without a redeploy of the gate logic. NO Redis/HTTP/ClickHouse import — the gate is consulted on every SELECT, so it must be a pure in-memory lookup (Pitfall 4 / T-03-07).

**Task 2 (TDD) — `gorm_effect.go` + tests:**
- `RegisterEffectCallbacks(db, sink, gate) error` — registers a Before-callback on Create/Update/Delete/Query that stamps a start time on `db.Statement.Settings`, plus After-callbacks:
  - Create/Update/Delete → ALWAYS one `EffectKind:"db_write"`, `Target:<table>`, `TargetKind:"table"`, `Rows:RowsAffected`, `Requests:1`, `UserID` from the private ctx, op PCs carried for async resolve (D-01).
  - Query → `EffectKind:"db_read"` ONLY when `gate.ShouldRecord(op, table, durMS)` is true (D-02/D-04). A6: when a SELECT's `RowsAffected==0`, the row count falls back to the reflect-len of `Statement.Dest`.
- CRITICAL invariant proven by test + grep: the after-callback bodies issue ZERO DB queries (no `db.Find`/`First`/`Raw`/`Scan`), so the Query callback never self-amplifies and RowsAffected is never zeroed (Pitfall 1 / gorm #7044 / T-03-06).

## Deviations from Plan

None of substance — plan executed as written.

- **`gorm.go` left unchanged (non-deviation).** The plan frontmatter `files_modified` listed `gorm.go`, but the Task 2 action says "keep `InstrumentGORM` unchanged. Update `gorm.go` only if a shared helper is cleaner." No shared helper improved clarity, so `gorm.go` stays untouched and `RegisterEffectCallbacks` lives in its own file. This is the action's documented discretion, not a divergence.

## TDD Gate Compliance

- Task 1 — RED: `2f3165dc` `test(03-03): add failing ReadGate snapshot tests` (compile-fail on undefined `NewReadGate` — verified). GREEN: `3f860202` `feat(03-03): ReadGate in-memory P95 threshold snapshot`. REFACTOR: none needed.
- Task 2 — RED: `f6308b1f` `test(03-03): add failing GORM db-effect callback tests` (compile-fail on undefined `RegisterEffectCallbacks` — verified). GREEN: `66193484` `feat(03-03): GORM db-effect after-callbacks`. A post-GREEN tidy (inline the `db_write` literal to satisfy the source-assertion grep + avoid an unused helper) was folded into the same GREEN commit before it landed; tests stayed green throughout.

## Verification

- `cd libs/tracing/gormtrace && go build ./... && go vet ./... && go test -race -count=1 ./...` — green (ReadGate + write-always + read-gated + RowsAffected-non-zero + EffectKind-always-explicit).
- `cd libs/tracing && go test -race -count=1 ./...` — parent package still green (consumed Effect contract intact).
- Source assertions: `grep -c 'EffectKind: *"db_write"'` = 1 and `grep -c 'EffectKind: *"db_read"'` = 1 (kinds set explicitly, never defaulted to egress). `grep -E 'redis|net/http|clickhouse' readgate.go` returns nothing (gate is pure in-memory). No `db.Find`/`First`/`Raw`/`Scan` in the after-callback bodies (no DB query — Pitfall 1).
- Behavior: a Create yields one `db_write` with `Rows>=1`; an Update over 3 rows yields `db_write` with `Rows=3` (RowsAffected not zeroed); a gated-true SELECT yields one `db_read` with non-zero Rows; a gated-false SELECT yields zero effects; missing-key reads use the 50ms static default; `-race` passes with concurrent `SetSnapshot`+`ShouldRecord`.

## Threat Flags

None. No new network endpoints, auth paths, or schema changes at trust boundaries. T-03-06 (callback recursion / RowsAffected zeroing) mitigated — zero DB queries in callbacks, proven by grep + test. T-03-07 (per-query sync threshold lookup) mitigated — ReadGate is an atomic in-memory snapshot. T-03-08 (user_id leak) mitigated — UserID sourced only from `UserIDFromContext` (private ctx), db effects post over the Docker network. T-03-09 (analytics self-amplification) is a plan-06 wiring constraint, documented in the `RegisterEffectCallbacks` doc comment. T-03-SC (package installs) — none; `gorm.io/driver/sqlite v1.6.0` was already a direct test dependency of `libs/tracing` (verified in go.mod, not newly added).

## Known Stubs

None. The `ReadGate` snapshot starts empty and serves the static cold-start default until the plan-05 daily-P95 refresher calls `SetSnapshot` — this is the intended extension seam (an empty snapshot is correct cold-start behavior, not a stub).

## Self-Check: PASSED

- FOUND: libs/tracing/gormtrace/readgate.go
- FOUND: libs/tracing/gormtrace/readgate_test.go
- FOUND: libs/tracing/gormtrace/gorm_effect.go
- FOUND: libs/tracing/gormtrace/gorm_effect_test.go
- FOUND commit: 2f3165dc (test/RED Task 1)
- FOUND commit: 3f860202 (feat/GREEN Task 1)
- FOUND commit: f6308b1f (test/RED Task 2)
- FOUND commit: 66193484 (feat/GREEN Task 2)
