---
phase: 9
slug: recs-foundation
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-05-06
---

# Phase 9 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (Go 1.22 stdlib `testing` with table-driven tests) |
| **Config file** | none — Go uses convention-based discovery |
| **Quick run command** | `cd services/player && go test ./internal/service/recs/...` |
| **Full suite command** | `cd services/player && go test ./...` |
| **Estimated runtime** | ~10 seconds (no network, no DB; pure unit tests) |

---

## Sampling Rate

- **After every task commit:** Run the task's `<automated>` verify block (per-task scoped — already specified in PLAN.md)
- **After every plan wave:** Run `cd services/player && go test ./internal/service/recs/...` (recs package full suite)
- **Before phase verification:** Full player suite + post-deploy schema check (Task 9 rollup)
- **Max feedback latency:** 10 seconds for unit tests; ~15 seconds for the redeploy step in Task 6

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 09-01-01 | 01 | 1 | REC-FOUND-01 | — | N/A | build | `go build ./internal/service/recs/...` | ✅ types.go | ⬜ pending |
| 09-01-02 | 01 | 1 | REC-FOUND-01 | — | N/A | build+grep | `go build ./internal/service/recs/... && grep -c "SignalModule interface" services/player/internal/service/recs/signal.go` | ✅ signal.go | ⬜ pending |
| 09-01-03 | 01 | 1 | REC-FOUND-03 | — | No NaN/Inf/negative on any pool | unit (TDD) | `go test -run "TestMinMaxNormalize" ./internal/service/recs/... -v` | ✅ normalize_test.go | ⬜ pending |
| 09-01-04 | 01 | 1 | REC-FOUND-02 | — | Errors propagate; no silent zeroing | unit (TDD) + grep gate | `go test -run "TestEnsemble" ./internal/service/recs/... -v && (grep -v '^//' services/player/internal/service/recs/ensemble.go \| grep -cE '"s[0-9]+"' \| grep -q '^0$')` | ✅ ensemble_test.go | ⬜ pending |
| 09-01-05 | 01 | 1 | REC-FOUND-04 | T-09-01 mitigate | GORM AutoMigrate is create-only | build+grep | `go build ./internal/domain/... && grep -cE 'func \(Rec(UserSignals\|PopulationSignals\|CompletionCoOccurrence)\) TableName' services/player/internal/domain/recs.go` | ✅ recs.go | ⬜ pending |
| 09-01-06 | 01 | 1 | REC-FOUND-04 | T-09-01 mitigate | FK constraints enforced; idempotent re-deploy | deploy+psql | `go build ./cmd/player-api/... && make redeploy-player && sleep 8 && (psql checks for tables, FK, index)` | ✅ main.go (edited) | ⬜ pending |
| 09-01-07 | 01 | 1 | REC-FOUND-04 | — | N/A | build+grep | `go build ./internal/repo/... && grep -cE 'func \(r \*RecsRepository\) (GetUserSignals\|UpsertUserSignals\|ListPopulationSignals\|UpsertPopulationSignal)' services/player/internal/repo/recs.go` | ✅ recs.go | ⬜ pending |
| 09-01-08 | 01 | 1 | REC-FOUND-01 | — | Errors collected, not short-circuited | unit (TDD) | `go test -run "TestOrchestrator" ./internal/service/recs/... -v` | ✅ precompute_test.go | ⬜ pending |
| 09-01-09 | 01 | 1 | REC-FOUND-01..04 (rollup) | T-09-01 mitigate | All criteria above + framework purity | full suite + deploy + psql + grep | aggregate (see Task 9 `<automated>`) | ✅ rollup | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements:
- Go 1.22 + `go test` already in use across all player-service packages.
- GORM v2 + Postgres already in use; AutoMigrate site at `cmd/player-api/main.go:47-58` already exists.
- Docker Compose Postgres container already running for the project — no new container/test infrastructure needed.

No Wave 0 setup commits required.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| FK constraint creates referential integrity | REC-FOUND-04 | `IF NOT EXISTS` masks would-be errors on already-existing constraints — must visually confirm at least once on first deploy | After Task 6 redeploy: `docker compose ... psql ... -c "\d rec_user_signals"` and confirm `Foreign-key constraints:` section lists `"rec_user_signals_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE` |
| AutoMigrate idempotency on re-deploy | REC-FOUND-04 | Cannot automate "second deploy succeeded" without explicit re-deploy step | Task 9 sub-step 3 explicitly redeploys a second time; tail logs to confirm zero migration errors |

---

## Sampling Continuity Audit

- **9 of 9 tasks have automated `<verify>` blocks.** No 3-consecutive-task sampling gap.
- **No watch-mode flags** anywhere in the plan.
- **No E2E suites** invoked (pure unit tests + one redeploy probe).
- **All commands feedback in <15 seconds** (unit tests <10s; redeploy probe ~12-15s including 8s sleep).

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (none — existing infra sufficient)
- [x] No watch-mode flags
- [x] Feedback latency < 15s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-05-06 (autonomous mode — orchestrator validation per workflow Step 5.5)
