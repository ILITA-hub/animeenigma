# Phase 21 — Deferred Items

Items discovered during Phase 21 execution that are out of scope for the
current plan and have been deferred. SCOPE BOUNDARY: an executor only
auto-fixes issues DIRECTLY caused by the current task's changes; pre-existing
failures and unrelated linting noise are logged here instead.

## Pre-existing test failures (not introduced by Phase 21)

### TestOrchestrator_AnimePaheToGogoanimeFailover

- **Discovered during:** Plan 21-02 execution (Task 2 GREEN gate verification)
- **File:** `services/scraper/internal/service/orchestrator_phase18_test.go:307`
- **Symptom:** `orch.ListEpisodes returned 0 episodes`
- **Confirmed pre-existing:** Reproduced on `main` HEAD prior to 21-02 changes
  (via `git stash`). Failure exists in the wave-1 baseline; not caused by the
  metrics counter additions or the `writeSuccess` signature change in 21-02.
- **Scope:** belongs to `services/scraper/internal/service/` — outside
  Plan 21-02's allowed files (`libs/metrics/`, `services/scraper/internal/handler/`).
- **Action:** Leave to whichever future plan owns orchestrator fixtures.
  Plan 21-03 (gogoanime server-priority + gate) touches the same package and
  may incidentally repair this test fixture; if not, a dedicated `fix(scraper)`
  task is warranted.
