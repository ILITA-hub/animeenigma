---
phase: 03
slug: db-cache-effects-auto-operation-discovery
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-06-05
---

# Phase 03 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (standard) for libs/services; live ClickHouse/Redis/Tempo smoke for integration |
| **Config file** | none — uses repo go.work + `make` targets |
| **Quick run command** | `go test ./libs/tracing/... ./libs/cache/...` |
| **Full suite command** | `go test ./...` (per-service) + live-stack smoke (see Manual-Only) |
| **Estimated runtime** | ~30–90 seconds for unit; live smoke ~minutes |

---

## Sampling Rate

- **After every task commit:** Run `go test ./<touched-package>/...`
- **After every plan wave:** Run `go test ./libs/tracing/... ./libs/cache/...` plus the touched services
- **Before `/gsd:verify-work`:** Full unit suite green + the 4 live success-criteria smokes pass
- **Max feedback latency:** 90 seconds (unit)

---

## Per-Task Verification Map

> Populated by the planner against actual task IDs. The 4 success criteria map to these
> end-to-end assertions (AR-EFFECT-01..04):

| Criterion | Requirement | Validation | Test Type |
|-----------|-------------|------------|-----------|
| DB write → one `db_write` row w/ `table`,`op`,`row_count`; high-volume read → no row | AR-EFFECT-01 | Exercise a write op + a trivial read; query ClickHouse `events` for the write row, assert zero rows for the read | integration (live CH) |
| Cache miss-then-hit → two classified `cache` rows by key-class | AR-EFFECT-02 | Hit a cached read twice; assert summed `cache` rows with `result=miss` and `result=hit` for the key_class | integration (live CH) |
| `operation` auto-derived, no manual catalog (`catalog.UpdateAnimeInfo`) | AR-EFFECT-03 | Assert an effect row's `operation` is a real `*/internal/service/*` symbol with no code naming it | unit (resolver) + integration |
| Tempo span-metrics + service graph → per-op RED metric + service graph in Prometheus | AR-EFFECT-04 | Query a per-operation request/error/duration metric in Prometheus; view service graph in Grafana | manual (live stack) |

---

## Wave 0 Requirements

- [ ] Unit-test scaffolding for the `runtime.Callers` service-frame resolver (table-driven over synthetic frames) — covers AR-EFFECT-03 deterministically without a live stack
- [ ] Unit-test scaffolding for the cache aggregator flush (deterministic clock, like the HLS reaper test) — covers AR-EFFECT-02 counting logic
- [ ] Existing `go test` infrastructure covers the rest

*Existing infrastructure (go test + live docker stack via `make dev`) covers all phase requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Per-operation RED metric present in Prometheus | AR-EFFECT-04 | Requires live Tempo metrics_generator → Prometheus remote-write | Enable config, generate traffic, query `traces_spanmetrics_*` in Prometheus |
| Service graph renders in Grafana | AR-EFFECT-04 | Visual, requires live datasource | Open Grafana → service graph panel after traffic |
| End-to-end effect rows in live ClickHouse | AR-EFFECT-01/02/03 | Requires full docker stack + analytics sink | Run `make dev`, exercise ops, query `events` table |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (resolver + ReadGate + GORM hook + cache aggregator + threshold refresher TDD scaffolds in-plan)
- [x] No watch-mode flags
- [x] Feedback latency < 90s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-06-05
