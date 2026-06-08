---
phase: 03-db-cache-effects-auto-operation-discovery
plan: 01
subsystem: libs/tracing (activity-register effect plane)
tags: [observability, attribution, runtime-callers, effect-wire-contract, D-08, D-09, D-11, AR-EFFECT-03]
requires:
  - "Phase-2 Effect/Producer/RoundTrip egress recorder (libs/tracing)"
  - "analytics /internal/effects receive side (already carries target_kind/anime_id/row_count)"
provides:
  - "Operation resolver (CaptureOperationPCs sync + Operation.Resolve async) — the PRIMARY operation attribution for ALL effect kinds"
  - "Extended Effect contract: TargetKind, AnimeID, Rows + an op PC carrier"
  - "wireProducerEffect row_count + anime_id; post() honors e.TargetKind"
  - "Egress retrofitted to stack-frame-primary operation (D-08)"
affects:
  - "Plan 03 (DB effects) — consumes Effect.TargetKind/Rows/AnimeID + CaptureOperationPCs in GORM callbacks"
  - "Plan 04 (cache effects) — consumes Effect.TargetKind/key_class rows"
tech-stack:
  added: []
  patterns:
    - "RESEARCH Pattern 2: sync runtime.Callers PC capture + async runtime.CallersFrames resolve (D-11)"
    - "Never-empty fallback chain: service frame -> baggage op -> origin name (D-09)"
key-files:
  created:
    - libs/tracing/attribution.go
    - libs/tracing/attribution_test.go
  modified:
    - libs/tracing/effect.go
    - libs/tracing/producer.go
    - libs/tracing/client.go
decisions:
  - "Derive the service-name label from the path segment ABOVE internal/service (e.g. player.SaveProgress) rather than the literal 'service' package — sharper operation labels, matches the catalog.UpdateAnimeInfo example."
  - "Egress carries operation PCs via Effect.WithOperationPCs and resolves on the async Producer side (post()), keeping CallersFrames off the request hot path even for egress (D-11)."
metrics:
  duration: ~1 work session
  completed: 2026-06-05
  tasks: 2
  files: 5
---

# Phase 3 Plan 01: Unified Effect Contract + runtime.Callers Operation Resolver Summary

Built the `runtime.Callers` operation resolver that becomes the PRIMARY attribution for every effect kind, and extended the Phase-2 effect wire contract so DB and cache effects can flow through the existing async Producer — retrofitting egress to stack-frame-primary operation (D-08) along the way.

## What Was Built

**Task 1 (TDD) — `attribution.go` + tests:**
- `CaptureOperationPCs(ctx) Operation` — a SYNC, hot-path-safe capture that does only `runtime.Callers(3, ...)` into a fixed `[32]uintptr` (skip Callers, this fn, the immediate hook caller). No symbol resolution (D-11).
- `(o Operation) Resolve() string` — ASYNC: walks `runtime.CallersFrames`, returns the first frame whose function path contains `/internal/service/` (normalized to `<pkg>.<Func>`), else the baggage operation (`ReadBaggage`), else an `originName` shaped `goroutine(<name>)`/`scheduled_job(<name>)`, defaulting to `goroutine(unknown)`. Never returns `""`.
- `normalizeServiceFrame` — strips the module path prefix to a compact `pkg.Func` / `pkg.Type.Method` label, derives the service name from the segment above `internal/service`, and drops pointer-receiver parens.
- The resolver only READS ctx — it never seeds `user_id` (or anything) into baggage (T-03-01).

**Task 2 — extended contract + egress retrofit:**
- `Effect` gains `TargetKind` (`host`|`table`|`key_class`), `AnimeID`, `Rows` (GORM RowsAffected), plus an unexported `op Operation` carrier + `WithOperationPCs` / `resolvedOperation` helpers.
- `wireProducerEffect` gains `RowCount int json:"row_count,omitempty"` and `AnimeID string json:"anime_id,omitempty"` — matching the analytics receive side exactly (Pitfall 5: tag is `row_count`, never `rows`).
- `post()` stops hard-coding `TargetKind: "host"` for every row — it honors `e.TargetKind` (host fallback only when empty), maps `RowCount: e.Rows` + `AnimeID`, and resolves the fine operation on the async goroutine via `e.resolvedOperation()`.
- `client.go RoundTrip` is now stack-frame-PRIMARY (D-08): it captures PCs with `CaptureOperationPCs(ctx)` at record time and threads them onto the Effect; baggage operation becomes the fallback inside `Resolve`. The T-02-PII guard is intact (`TestNoUserIDOnOutboundWire` still green).

## Deviations from Plan

None of substance — plan executed as written. One test-expectation refinement during the GREEN phase: the plan's `normalizeServiceFrame` examples (`catalog.UpdateAnimeInfo`) derive the *service name* from the path. My implementation correctly yields `player.SaveProgress` for a bare `.../player/internal/service.SaveProgress` frame (service-name-derived), which is sharper than the literal `service.SaveProgress`; I aligned the one test case to match the plan's documented `catalog.<Func>` shape. This is a test assertion alignment, not a code deviation.

## TDD Gate Compliance

- RED gate: `b7c61850` `test(03-01): add failing tests ...` (compile-fail on undefined symbols — verified failing before implementation).
- GREEN gate: `e0adc1a6` `feat(03-01): runtime.Callers operation resolver`.
- REFACTOR: none needed (code clean on first green).

## Verification

- `cd libs/tracing && go build ./... && go vet ./... && go test -race -count=1 ./...` — all green (resolver + existing egress/baggage/recording suites).
- `go test -run TestOperationResolve -race` — passes (service-frame primary + baggage + origin fallback, never empty).
- Source assertions: `runtime.Callers(` appears at exactly one call site (CaptureOperationPCs); `runtime.CallersFrames(` at exactly one (Resolve); `row_count`/`anime_id` present in producer.go; `e.TargetKind` honored (no bare `TargetKind: "host"` hardcode); `CaptureOperationPCs` wired in client.go.
- `TestNoUserIDOnOutboundWire` green post-retrofit — PII guard preserved.

## Threat Flags

None. No new network endpoints, auth paths, or schema changes at trust boundaries. The `operation` symbol path lands only in ClickHouse/Grafana over the Docker network (T-03-02 accepted); the resolver never touches baggage (T-03-01 mitigated); CallersFrames runs async only (T-03-03 mitigated). No package installs (T-03-SC).

## Known Stubs

None. The Effect contract carries `TargetKind`/`Rows`/`AnimeID` ready for plans 03 (DB) and 04 (cache) to populate; these are intentional extension points consumed by downstream wave plans, not stubs.

## Self-Check: PASSED

- FOUND: libs/tracing/attribution.go
- FOUND: libs/tracing/attribution_test.go
- FOUND: libs/tracing/effect.go (modified)
- FOUND: libs/tracing/producer.go (modified)
- FOUND: libs/tracing/client.go (modified)
- FOUND commit: b7c61850 (test/RED)
- FOUND commit: e0adc1a6 (feat/Task 1)
- FOUND commit: e2e6e9ef (feat/Task 2)
