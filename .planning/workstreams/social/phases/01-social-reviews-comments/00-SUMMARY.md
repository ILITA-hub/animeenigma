---
phase: 1
workstream: social
plan: 0
subsystem: services/player + frontend/web (e2e) + scripts
tags:
  - scaffold
  - tests
  - wave-0
  - nyquist
requirements:
  - SOCIAL-04
  - SOCIAL-05
  - SOCIAL-06
  - SOCIAL-NF-01
  - SOCIAL-NF-02
dependency_graph:
  requires: []
  provides:
    - "services/player/internal/domain/comment.go (Comment + DTOs)"
    - "services/player/internal/repo/comment.go (CommentRepository skeleton)"
    - "services/player/internal/service/comment.go (CommentService skeleton + rateBucket)"
    - "services/player/internal/handler/comment.go (CommentHandler skeleton)"
    - "8 named Go test stubs (all SKIP) ready for plans 01 + 03 to flip to GREEN"
    - "frontend/web/e2e/comments.spec.ts (4 Playwright stubs) ready for plan 06"
    - "scripts/capture-reviews-fixtures.sh ready for SOCIAL-NF-01 manual smoke"
  affects:
    - "services/player/cmd/player-api/main.go (no edits — plan 04 wires the handler/service)"
tech-stack:
  added: []
  patterns:
    - "Repository → Service → Handler layering mirrored from review.go siblings"
    - "In-memory SQLite test DB via setupCommentTestDB() (mirrors sync_test.go:22)"
    - "Claims-into-context handler test injection (mirrors mal_import_test.go:140)"
    - "errors.CodeUnavailable used in stub bodies (no errors.NotImplemented helper exists)"
key-files:
  created:
    - "services/player/internal/domain/comment.go"
    - "services/player/internal/repo/comment.go"
    - "services/player/internal/service/comment.go"
    - "services/player/internal/handler/comment.go"
    - "services/player/internal/repo/comment_test.go"
    - "services/player/internal/service/comment_test.go"
    - "services/player/internal/handler/comment_test.go"
    - "services/player/cmd/player-api/main_test.go"
    - "frontend/web/e2e/comments.spec.ts"
    - "scripts/capture-reviews-fixtures.sh"
  modified: []
decisions:
  - "Stub bodies return errors.New(errors.CodeUnavailable, '...') because libs/errors does not export NotImplemented — closest semantic match is CodeUnavailable (HTTP 503). Plan 03/04 will replace these with real returns."
  - "rateBucket is inline in the comment service (not extracted to libs/ratelimit) — no prior pattern exists in the player service. Plan 03 may revisit if a second consumer appears."
  - "pagination.Cursor is referenced via `var _ = pagination.Cursor{}` so the import is honest today (plan 03 will use the type) and `go vet` stays clean."
  - "main_test.go was force-added past the `**/player-api` .gitignore glob (which targets the compiled binary, not .go files in the package)."
metrics:
  duration_minutes: 7
  completed_date: "2026-05-13"
  tasks_completed: 3
  files_created: 10
  files_modified: 0
  commits: 3
---

# Phase 1 Plan 0: Reviews + Comments Test Scaffolding Summary

**One-liner:** Wave-0 scaffolding for SOCIAL-04..06 — 10 new files (8 Go,
1 TypeScript, 1 shell) that compile and run, with every test stub at SKIP,
so plans 01-06 have real `<verify>` targets to flip RED→GREEN against.

## What Was Built

This plan satisfies the **Nyquist contract** for Phase 1: every
implementation task in plans 01-06 now has an automated `<verify>` command
that references a real test file, real test name, or real script that
already exists in the tree.

### Backend stubs (`services/player/`)

- `internal/domain/comment.go` — `Comment` struct with GORM tags, composite
  indexes `idx_comments_anime_created (anime_id, created_at DESC)` and
  `idx_comments_user_created (user_id, created_at DESC)`, `TableName()`,
  `CreateCommentRequest` (intentionally omits `parent_id` per Pitfall 8),
  `UpdateCommentRequest`, `CommentsListResponse`.
- `internal/repo/comment.go` — `CommentRepository` with five method
  stubs: `Create`, `GetByID`, `ListByAnime`, `Update`, `SoftDelete`.
- `internal/service/comment.go` — `CommentService` + private `rateBucket`
  type with `allow(userID, animeID string) bool` returning `true` today;
  four method stubs: `CreateComment`, `UpdateComment`, `DeleteComment`,
  `ListComments`.
- `internal/handler/comment.go` — `CommentHandler` with four endpoint
  stubs returning `errors.CodeUnavailable` (HTTP 503); `chi.URLParam`
  read for `animeId` / `commentId` so route wiring compiles later.

### Backend test stubs (8 tests, all SKIP)

| Test | Maps to |
|------|---------|
| `TestCommentRepo_SoftDelete` | SOCIAL-04d / 01-Comment-04 |
| `TestCommentRepo_ListByAnime_Cursor` | SOCIAL-04e / 01-Comment-05 |
| `TestCommentService_RateLimit` | SOCIAL-04f / 01-Comment-06 |
| `TestCommentService_EmitsActivity` | SOCIAL-05 / 01-Activity-01 |
| `TestCommentHandler_CreateComment_HappyPath` | SOCIAL-04a / 01-Comment-01 |
| `TestCommentHandler_CreateComment_EmptyBody` | SOCIAL-04b / 01-Comment-02 |
| `TestCommentHandler_UpdateComment_NotOwner` | SOCIAL-04c / 01-Comment-03 |
| `TestSocialMigration_Idempotent` | SOCIAL-NF-02 / 01-Migrate-01 |

Helpers shipped:
- `setupCommentTestDB(t *testing.T) *gorm.DB` (in-memory SQLite +
  AutoMigrate) — mirrors `handler/sync_test.go:22`.
- `withCommentClaims(r, userID, username, role) *http.Request` —
  mirrors `mal_import_test.go:140`.

### Frontend test stubs

- `frontend/web/e2e/comments.spec.ts` — `describe('Anime comments tab')`
  with 4 tests whose names match the exact `-g` filters in
  `01-VALIDATION.md`: `deep-link`, `URL persists`, `anon login prompt`,
  `logged-in CRUD`. `ANIME_ID = 'TBD'` placeholder + plan-06 to-do
  comment. Pure `test.skip(true, ...)` bodies — zero `await page.` calls.

### Manual smoke tooling

- `scripts/capture-reviews-fixtures.sh` — bash script (`set -euo pipefail`)
  that curls four read-side review endpoints and prints `# SKIP` markers
  for the two mutating endpoints. Reads `UI_AUDIT_API_KEY` from env (never
  echoed). Executable; `bash -n` clean. Used pre/post plan 04 deploy for
  the SOCIAL-NF-01 golden-file shape diff.

## Verification

Final overall checks (all green):

```
$ cd services/player && go build ./...
$ cd services/player && go test -run 'TestComment|TestSocial' -v ./...
--- SKIP: TestSocialMigration_Idempotent (0.00s)
--- SKIP: TestCommentHandler_CreateComment_HappyPath (0.00s)
--- SKIP: TestCommentHandler_CreateComment_EmptyBody (0.00s)
--- SKIP: TestCommentHandler_UpdateComment_NotOwner (0.00s)
--- SKIP: TestCommentRepo_SoftDelete (0.00s)
--- SKIP: TestCommentRepo_ListByAnime_Cursor (0.00s)
--- SKIP: TestCommentService_RateLimit (0.00s)
--- SKIP: TestCommentService_EmitsActivity (0.00s)
ok  github.com/ILITA-hub/animeenigma/services/player/...
$ cd services/player && go vet ./...    # clean
$ cd frontend/web && bunx playwright test --list e2e/comments.spec.ts
# enumerates 4 distinct tests × 3 browser projects (chromium, firefox, Mobile Chrome) = 12 lines
$ bash -n scripts/capture-reviews-fixtures.sh    # syntax OK
$ test -x scripts/capture-reviews-fixtures.sh    # executable
```

Per-task acceptance criteria:

- **Task 0.1** — all 7 criteria green (build clean, `TableName()` regex
  match, no `ParentID` in `CreateCommentRequest`, all five repo methods
  exported, all five service methods exported, all five handler methods
  exported, no `domain.Comment{` in `main.go` yet).
- **Task 0.2** — verify command emits ≥ 8 SKIP lines (16 lines total:
  one `=== RUN` and one `--- SKIP` per test = 8 × 2); `t.Skip("Wave 0
  scaffold` marker present in all 4 test files (4 of 4); `go vet ./...`
  clean.
- **Task 0.3** — Playwright `--list` enumerates 4 distinct named tests
  (12 lines when multiplied by browser projects); script is `+x`;
  `bash -n` clean; no `await page.` in the spec.

## Commits

| Task | Commit | Message |
|------|--------|---------|
| 0.1 | `5dd47f2` | `feat(1-0): scaffold Comment domain + repo + service + handler stubs` |
| 0.2 | `c21a4fe` | `test(1-0): scaffold comment + migration test stubs (8 skipped tests)` |
| 0.3 | `5c2ba55` | `test(1-0): scaffold comments.spec.ts + capture-reviews-fixtures.sh` |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `libs/errors` exports no `NotImplemented` helper.**
- **Found during:** Task 0.1 (writing repo stub bodies).
- **Issue:** Plan asked stubs to call `errors.NotImplemented("...")`. The
  shared `libs/errors` package exports `NotFound`, `InvalidInput`,
  `Forbidden`, `RateLimited`, `Internal`, `Unauthorized`,
  `ServiceUnavailable`, `ExternalAPI`, `VideoNotReady` — but no
  `NotImplemented` and no `CodeNotImplemented` enum value.
- **Fix:** Used `errors.New(errors.CodeUnavailable, "comment <subsystem>
  <method>: not implemented")` instead. `CodeUnavailable` maps to HTTP
  503 which is the standard "endpoint exists but is not ready" response;
  preserves error-handling semantics for the future real implementation.
- **Files modified:** all four Task-0.1 .go files.
- **Commit:** `5dd47f2`.

**2. [Rule 3 - Blocking] `services/player/cmd/player-api/` matches the
`**/player-api` .gitignore glob.**
- **Found during:** Task 0.2 commit attempt.
- **Issue:** Plan placed `main_test.go` in `cmd/player-api/` (correct —
  same package as `main.go`). But `git add` refused with "ignored by
  .gitignore" because `**/player-api` matches the compiled-binary glob
  (intended to ignore the binary, not the source dir).
- **Fix:** Force-added the test file with `git add -f`. The existing
  `main.go` is already tracked the same way — this is consistent with
  the existing convention.
- **Files modified:** `services/player/cmd/player-api/main_test.go`.
- **Commit:** `c21a4fe`.

### Notes (not deviations)

- The plan's verify command for Task 0.3 (`bunx playwright test --list
  ... | grep -E 'deep-link|URL persists|anon login prompt|logged-in
  CRUD' | wc -l`) returns **12** in this environment (4 tests × 3
  Playwright projects), not 4. The substantive acceptance — "4 distinct
  named tests exist matching those patterns" — is met (verified
  separately with `sort -u`). No code change needed; this is a verify-
  command artifact specific to the project's multi-browser config.
- The unused-import handling for `libs/pagination` in `repo/comment.go`
  uses `var _ = pagination.Cursor{}` (as the plan suggested as a
  fallback). Plan 03 will start using the type and the sentinel can be
  removed.

## Handoff to Plan 01

Plan 01 (Schema + Migration) can now reference:

- `services/player/cmd/player-api/main_test.go::TestSocialMigration_Idempotent` —
  flip the `t.Skip` to a two-pass migration assertion (first run copies
  `reviews` → `anime_list` and drops `reviews`; second run is a no-op).
- `services/player/internal/domain/comment.go::Comment` — the table the
  AutoMigrate block will add.
- `scripts/capture-reviews-fixtures.sh` — capture `reviews-pre.json`
  before redeploy.

## Handoff to Plan 03

Plan 03 (Comment CRUD + Activity) can now reference 7 named Go tests
already in the tree (all SKIP). Replace `t.Skip(...)` with real
assertions; the helpers and imports are already wired.

## Handoff to Plan 06

Plan 06 (Frontend Tabs) can now reference
`frontend/web/e2e/comments.spec.ts` with 4 named tests matching the
exact `-g` keywords from `01-VALIDATION.md`. Replace `ANIME_ID = 'TBD'`
with a seeded `ui_audit_bot` anime id, mirror the auth pattern from
`anime.spec.ts`, and remove the `test.skip(true, ...)` lines.

## Known Stubs

None that block plan completion. The 10 files added are all explicit,
documented Wave-0 scaffolds with `Wave 0 scaffold — implementation in
plan 0X` markers pointing the next agent at the correct follow-up. The
service is unaffected; nothing in `main.go` references the new packages
yet (per Task 0.1 acceptance criteria).

## Threat Flags

None. Wave 0 is pure scaffolding — no new code paths execute against
untrusted input. The two threats in the plan's `<threat_model>` are
both accept/mitigate dispositions that this scaffold honors:

- T-1-W0-01 (Tampering — 501/503 stub handlers): mitigated. Handlers
  return `errors.CodeUnavailable` via `httputil.Error`; no DB write
  path active. They are not wired into `cmd/player-api/main.go` yet, so
  even the 503 surface does not exist on the running service.
- T-1-W0-02 (Info disclosure — API key in capture script): mitigated.
  Script reads `UI_AUDIT_API_KEY` from env only, never echoes the value,
  and skips the authenticated endpoint with a comment when the var is
  unset (rather than sending an empty Bearer token).

## Self-Check: PASSED

**Files verified to exist:**

- `services/player/internal/domain/comment.go` — FOUND
- `services/player/internal/repo/comment.go` — FOUND
- `services/player/internal/service/comment.go` — FOUND
- `services/player/internal/handler/comment.go` — FOUND
- `services/player/internal/repo/comment_test.go` — FOUND
- `services/player/internal/service/comment_test.go` — FOUND
- `services/player/internal/handler/comment_test.go` — FOUND
- `services/player/cmd/player-api/main_test.go` — FOUND
- `frontend/web/e2e/comments.spec.ts` — FOUND
- `scripts/capture-reviews-fixtures.sh` — FOUND (and executable)

**Commits verified in `git log --all`:**

- `5dd47f2` — FOUND
- `c21a4fe` — FOUND
- `5c2ba55` — FOUND
