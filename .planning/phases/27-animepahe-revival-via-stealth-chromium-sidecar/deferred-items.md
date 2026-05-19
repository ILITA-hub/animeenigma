# Phase 27 — Deferred Items

Items discovered during execution that are OUT OF SCOPE for the
current plan but should be revisited later.

## DEF-001 — Pre-existing failure: `TestOrchestrator_AnimePaheToGogoanimeFailover`

- **Discovered:** Plan 27-02 Task 3 verification pass (2026-05-19)
- **Location:** `services/scraper/internal/service/orchestrator_phase18_test.go:307`
- **Symptom:** `orch.ListEpisodes returned 0 episodes`
- **Verification of pre-existence:** Test fails identically on commit
  `e33be35` (the worktree base, before any Plan 27-02 changes). The
  failure is in the gogoanime provider's ListEpisodes path (the fake
  animepahe is intentionally cache-skipped and never called), so it
  has no relationship to the animepahe-resolver migration this plan
  ships.
- **Hypothesis:** Likely fallout from `1b45e58 fix(scraper/test):
  rewrite TestGetStreamWithGate_AdDecoy_Skipped to avoid parCancel
  race (25-01)` or unrelated drift in gogoanime's test fixtures.
- **Action:** Document only — do not auto-fix in Plan 27-02 (outside
  scope; touching gogoanime would inflate diff and risk masking the
  animepahe migration's signal). Triage as a separate task or roll
  into Phase 28 prep if it persists.
