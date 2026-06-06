---
phase: 04-fe-causation-rum
plan: 01
subsystem: analytics
tags: [analytics, collect-handler, activity-register, fe-causation, rum, security]
requires:
  - "services/analytics/internal/domain/event.go register fields (Source/Operation/Target/TargetKind/Requests/DurationMS/Accuracy)"
  - "services/analytics/internal/repo/clickhouse_store.go defaultStr(Source,'be') / defaultStr(Accuracy,'exact')"
provides:
  - "collect handler FE register-field mapping (source/operation/target/target_kind/requests/duration_ms)"
  - "server-side source whitelist {fe, fe_rum} on the public collect beacon"
  - "fe_rum → accuracy=approximate + structural byte-poverty (BytesIn/Out never mapped)"
affects:
  - "FE beacons posted to POST /api/analytics/collect now land as register rows, not source='be' clickstream"
tech-stack:
  added: []
  patterns:
    - "server-side source whitelist (normalize-to-empty) for forged-beacon defense"
    - "byte-poverty by omission (no byte wire fields on the FE wire)"
    - "length-cap (256 runes) public-beacon register strings"
key-files:
  created: []
  modified:
    - "services/analytics/internal/handler/collect.go"
    - "services/analytics/internal/handler/collect_test.go"
decisions:
  - "Map FE attribution onto domain.Event.Source (whitelisted), NOT Origin — matches plan interface contract and the clickhouse_store source default seam."
  - "Forged/empty source normalizes to empty (not rejected-the-whole-event) so the row still lands as an authoritative source='be' default rather than being dropped."
  - "Action only fills the Operation slot when Operation is empty (AR-FE-01 optional semantic label); no EffectKind invented for FE rows."
metrics:
  duration: "~1 session, 2 tasks"
  completed: 2026-06-06
  tasks_completed: 2
  files_modified: 2
requirements: [AR-FE-01, AR-FE-03]
---

# Phase 04 Plan 01: FE Register-Field Mapping in Collect Handler Summary

Extended the analytics `collect` handler so a browser-originated beacon can write an Activity-Register row carrying its own `source`/`operation`/`target`/`target_kind`/`requests`/`duration_ms` — with a server-side `source` whitelist ({fe, fe_rum}) that defeats forged attribution and a structural byte-poverty guarantee for `fe_rum` RUM rows.

## What Was Built

- **`wireEvent` struct** gained 7 snake_case fields (`source`, `operation`, `action`, `target`, `target_kind`, `requests`, `duration_ms`) matching Plan 02's FE wire. Deliberately **no byte fields** — `fe_rum` rows are byte-poor by omission (RESEARCH Pattern 3).
- **Source whitelist** (`whitelistSource`): only `"fe"` / `"fe_rum"` are honored; any other value (forged `"be"`, `"evil"`, or empty) normalizes to empty `Source`, so `clickhouse_store.go:119`'s `defaultStr(e.Source, "be")` keeps backend-recorded rows authoritative and a forged beacon can never inject an attribution origin (threat T-04-01).
- **fe_rum → accuracy** mapping: a whitelisted `fe_rum` source sets `Accuracy="approximate"`; everything else leaves it empty (store defaults to `"exact"`).
- **Byte-poverty (T-04-02):** `BytesIn`/`BytesOut` are never mapped from the FE wire, enforced structurally (no byte wire field exists).
- **Input hardening (T-04-03):** each new register string is length-capped to 256 runes (`capString`) before mapping; existing `LimitReader(256KB)`, clock-skew drop, and per-event `Validate()` skip-bad-keep-rest are reused unchanged.
- **Tests:** `TestCollectMapsFERegisterFields` (AR-FE-01 mapping), `TestFERUMRowCarriesZeroBytes` (AR-FE-03 byte-poverty + accuracy + measures), `TestCollectRejectsForgedSource` (T-04-01 forged `evil`/`be` → empty). All use the existing handwritten `capturingSink` fake; no testify/mock.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Extend wireEvent + map FE register fields with source whitelist | `e63cc08d` | services/analytics/internal/handler/collect.go |
| 2 | Add register-mapping + byte-poverty + forged-source tests | `0d0415e8` | services/analytics/internal/handler/collect_test.go |

## Verification

- `cd services/analytics && go build ./...` — clean.
- `go vet ./internal/handler/` — clean.
- `go test ./internal/handler/ -run 'TestCollectMapsFERegisterFields|TestFERUMRowCarriesZeroBytes|TestCollectRejectsForgedSource' -count=1 -race` — 3/3 PASS.
- `go test ./... -count=1 -race` (full analytics service) — all packages green.
- `grep -n 'fe_rum' .../collect.go` — whitelist path present.
- `grep 'json:"source"'` returns the new field; `grep 'bytes_in|bytes_out'` and `grep 'json:"rows"'` return nothing (byte/rows fields correctly absent).

## TDD Gate Compliance

Both tasks are `tdd="true"`. Plan 04-01 is paired (Task 1 = implementation, Task 2 = the proofs). The two tests assert the exact behavior the implementation provides and pass under `-race`. Note: gate-sequence commits are `feat(04-01)` then `test(04-01)` (implementation-then-proof ordering per the plan's task split) rather than a strict RED-before-GREEN single-feature cycle; the byte-poverty and forged-source assertions are the slop-resistant detail that would have caught a missing whitelist or a stray byte mapping.

## Deviations from Plan

**1. [Rule 3 - Blocking] Missing context file `04-PATTERNS.md`**
- **Found during:** Initial file reads.
- **Issue:** The plan `@`-references `.planning/phases/04-fe-causation-rum/04-PATTERNS.md` in `<context>`, `read_first`, and `<execution_context>`, but the file does not exist in the phase directory (only PLAN/CONTEXT/RESEARCH/VALIDATION files are present).
- **Resolution:** Substituted the equivalent ground truth — read the actual `collect.go`, `collect_test.go`, `domain/event.go` (the `<interfaces>` block transcribes these), `clickhouse_store.go:110-130` (confirmed the `defaultStr(Source,'be')` / `defaultStr(Accuracy,'exact')` seam), and `04-VALIDATION.md` (AR-FE-01/03 proof rows). No PATTERNS-specific instruction was lost; the `<action>` block in the PLAN carried the full mapping + whitelist spec.
- **Files modified:** none (research-only).
- **Commit:** n/a.

Otherwise the plan executed exactly as written.

## Known Stubs

None. The mapping is fully wired end-to-end (wire → whitelist → domain.Event → store defaults). Live ClickHouse phase-gate (FE row joins to BE effects on `trace_id`; RUM rows byte-poor) is a documented manual-only verification (04-VALIDATION.md) requiring a running stack + real browser navigation — out of scope for this backend-ingest plan.

## Threat Flags

None. No new network endpoint, auth path, or schema change was introduced — the change is field-mapping within the existing public `POST /api/analytics/collect` surface, which the plan's threat model already covers (T-04-01/02/03 all mitigated in-plan).

## Self-Check: PASSED

- FOUND: services/analytics/internal/handler/collect.go (modified, commit e63cc08d)
- FOUND: services/analytics/internal/handler/collect_test.go (modified, commit 0d0415e8)
- FOUND: commit e63cc08d in git log
- FOUND: commit 0d0415e8 in git log
