# Phase 28 Deferred Items

Out-of-scope discoveries during execution; recorded per executor's scope-boundary rules.

## From Plan 28-02 execution (worktree-agent-a161967d4e0368dec)

### TestOrchestrator_AnimePaheToGogoanimeFailover failure (pre-existing)

- **File:** `services/scraper/internal/service/orchestrator_phase18_test.go:307`
- **Symptom:** `orch.ListEpisodes returned 0 episodes`
- **Cause:** Pre-existing — reproduces on the worktree base commit
  (dc7f89f) before any 28-02 changes were applied (verified via
  `git stash && go test`).
- **In-scope work:** Plan 28-02's changes (animefever package, main.go
  registration, proxy allowlist) do NOT touch
  `services/scraper/internal/service/`. Out of scope per
  executor deviation rules SCOPE BOUNDARY.
- **Disposition:** Should be triaged by a follow-up plan or the
  orchestrator's post-merge build gate. Not blocking Plan 28-02.

