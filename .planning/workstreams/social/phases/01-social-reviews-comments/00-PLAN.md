---
phase: 1
workstream: social
plan: 0
type: execute
wave: 0
depends_on: []
files_modified:
  - services/player/internal/domain/comment.go
  - services/player/internal/repo/comment.go
  - services/player/internal/repo/comment_test.go
  - services/player/internal/service/comment.go
  - services/player/internal/service/comment_test.go
  - services/player/internal/handler/comment.go
  - services/player/internal/handler/comment_test.go
  - services/player/cmd/player-api/main_test.go
  - frontend/web/e2e/comments.spec.ts
  - scripts/capture-reviews-fixtures.sh
autonomous: true
requirements:
  - SOCIAL-04
  - SOCIAL-05
  - SOCIAL-06
  - SOCIAL-NF-01

must_haves:
  truths:
    - "Every test command listed in 01-VALIDATION.md exists as a real Go test or Playwright spec stub (skipped or failing — RED is fine)."
    - "go test ./services/player/... compiles successfully (no missing symbols, no unresolved imports)."
    - "bunx playwright test --list reports the four comments.spec.ts tests by name."
    - "scripts/capture-reviews-fixtures.sh is executable and prints curl output for six review endpoints."
  artifacts:
    - path: "services/player/internal/domain/comment.go"
      provides: "Comment domain struct + TableName() + Create/Update request DTOs + CommentsListResponse"
      contains: "type Comment struct"
    - path: "services/player/internal/repo/comment.go"
      provides: "CommentRepository skeleton with Create, GetByID, ListByAnime, Update, SoftDelete signatures (bodies may return errors.NotImplemented or stub)"
      contains: "type CommentRepository struct"
    - path: "services/player/internal/repo/comment_test.go"
      provides: "TestCommentRepo_SoftDelete + TestCommentRepo_ListByAnime_Cursor stubs that compile and fail/skip"
      contains: "func TestCommentRepo_SoftDelete"
    - path: "services/player/internal/service/comment.go"
      provides: "CommentService struct skeleton + NewCommentService constructor + 5 method signatures"
      contains: "type CommentService struct"
    - path: "services/player/internal/service/comment_test.go"
      provides: "TestCommentService_RateLimit + TestCommentService_EmitsActivity stubs"
      contains: "func TestCommentService_RateLimit"
    - path: "services/player/internal/handler/comment.go"
      provides: "CommentHandler skeleton with CreateComment/UpdateComment/DeleteComment/ListComments method stubs returning 501"
      contains: "type CommentHandler struct"
    - path: "services/player/internal/handler/comment_test.go"
      provides: "TestCommentHandler_CreateComment_HappyPath + _EmptyBody + TestCommentHandler_UpdateComment_NotOwner stubs"
      contains: "func TestCommentHandler_CreateComment_HappyPath"
    - path: "services/player/cmd/player-api/main_test.go"
      provides: "TestSocialMigration_Idempotent stub"
      contains: "func TestSocialMigration_Idempotent"
    - path: "frontend/web/e2e/comments.spec.ts"
      provides: "Four Playwright test stubs covering SOCIAL-06a/b/c/d"
      contains: "test.describe"
    - path: "scripts/capture-reviews-fixtures.sh"
      provides: "Bash script that curls 6 review endpoints against localhost:8000 and prints concatenated JSON"
      contains: "curl"
  key_links:
    - from: "services/player/internal/handler/comment_test.go"
      to: "services/player/internal/handler/sync_test.go"
      via: "setupSyncTestDB pattern reuse for in-memory SQLite"
      pattern: "setupSyncTestDB|gorm.io/driver/sqlite"
    - from: "frontend/web/e2e/comments.spec.ts"
      to: "frontend/web/e2e/anime.spec.ts"
      via: "Mirror existing anime.spec.ts auth + navigation patterns"
      pattern: "test.describe"
---

<objective>
Scaffold the test files declared as `❌ W0` in 01-VALIDATION.md so subsequent waves can wire RED→GREEN against real test commands. No production logic — every implementation stub returns `errors.NotImplemented(...)` (Go) or `test.skip(...)` (Playwright). The goal is: after this plan, `go test ./services/player/internal/...` compiles, `bunx playwright test --list e2e/comments.spec.ts` enumerates four tests, and `scripts/capture-reviews-fixtures.sh` is executable.

Purpose: satisfy the Nyquist contract — every implementation task in Waves 1-4 has an automated `<verify>` command referencing a real test file that already exists.

Output: 10 new files (8 Go, 1 TypeScript, 1 shell). Zero production code; zero behavior change to the running player service.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/social/phases/01-social-reviews-comments/01-SPEC.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-VALIDATION.md
@services/player/internal/handler/sync_test.go
@services/player/internal/repo/sync_test.go
@services/player/internal/handler/mal_import_test.go
@frontend/web/e2e/anime.spec.ts
@frontend/web/playwright.config.ts
@libs/errors/errors.go
@libs/authz/jwt.go

<interfaces>
<!-- Test infrastructure already established in the player service -->
<!-- See services/player/internal/handler/sync_test.go:22 setupSyncTestDB for the SQLite-in-memory factory -->
<!-- See services/player/internal/handler/mal_import_test.go:140 for the claims-into-context injection pattern -->

From libs/errors/errors.go (used by stub bodies):
- errors.NotImplemented(msg string) error  // returns a domain error
- errors.InvalidInput(msg string) error
- errors.NotFound(msg string) error
- errors.Forbidden(msg string) error
- errors.RateLimited() error

From libs/authz/jwt.go:
- type Claims struct { UserID string; Username string; Role string; ... }
- ClaimsFromContext(ctx context.Context) (*Claims, bool)
- IsAdmin(ctx context.Context) bool
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 0.1: Scaffold Comment domain + repo + service + handler Go files with compiling stub bodies</name>
  <files>services/player/internal/domain/comment.go, services/player/internal/repo/comment.go, services/player/internal/service/comment.go, services/player/internal/handler/comment.go</files>
  <read_first>
    - services/player/internal/domain/watch.go (Review struct shape lines 105-119; AnimeListEntry lines 58-79)
    - services/player/internal/domain/activity.go (gorm.DeletedAt pattern lines 9-21)
    - services/player/internal/repo/review.go (full file — mirror its Repository struct shape)
    - services/player/internal/service/review.go (full file — mirror ReviewService struct shape and constructor)
    - services/player/internal/handler/review.go (full file — mirror ReviewHandler signatures)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Code Examples section, lines 700-825)
    - libs/errors/errors.go (NotImplemented and InvalidInput signatures)
  </read_first>
  <action>
    Create the four Comment files as compiling stubs. NO business logic; every method body returns `errors.NotImplemented("comment <method>")` or zero-value + nil so the test stubs in task 0.2 can compile.

    File 1: `services/player/internal/domain/comment.go` (package `domain`). Declares: `type Comment struct` with GORM tags exactly per 01-RESEARCH.md Code Example 1 (lines 710-720) — fields `ID, UserID, AnimeID, Username, Body, ParentID *string, CreatedAt, UpdatedAt, DeletedAt gorm.DeletedAt`; composite indexes `idx_comments_anime_created` and `idx_comments_user_created` with `sort:desc` on `CreatedAt`; `TableName() string { return "comments" }`. Also declare `type CreateCommentRequest struct { Body string }`, `type UpdateCommentRequest struct { Body string }`, `type CommentsListResponse struct { Comments []*Comment; NextCursor string; HasMore bool }`. `CreateCommentRequest` MUST NOT declare a `ParentID` field (Pitfall 8: mass-assignment guard).

    File 2: `services/player/internal/repo/comment.go` (package `repo`). Declares `type CommentRepository struct { db *gorm.DB }`, constructor `NewCommentRepository(db *gorm.DB) *CommentRepository`. Method stubs (all return zero-value + `errors.NotImplemented("comment repo …")`): `Create(ctx, c *domain.Comment) error`, `GetByID(ctx, id string) (*domain.Comment, error)`, `ListByAnime(ctx, animeID, cursor string, limit int) (comments []*domain.Comment, nextCursor string, err error)`, `Update(ctx, id, body string) error`, `SoftDelete(ctx, id string) error`. Import `libs/errors` and `libs/pagination` (even though pagination is unused in stubs — to keep go imports clean later); if go vet complains about unused imports, use a `var _ = pagination.Cursor{}` line at package level.

    File 3: `services/player/internal/service/comment.go` (package `service`). Declares `type CommentService struct { commentRepo *repo.CommentRepository; activityRepo *repo.ActivityRepository; log *logger.Logger; rateBucket *rateBucket }`. Declare a package-private `type rateBucket struct { mu sync.Mutex; entries map[string][]time.Time }` and a constructor `newRateBucket() *rateBucket { return &rateBucket{entries: map[string][]time.Time{}} }`. Implement `(b *rateBucket) allow(userID, animeID string) bool` returning `true` for now (real implementation in plan 03). Constructor `NewCommentService(commentRepo *repo.CommentRepository, activityRepo *repo.ActivityRepository, log *logger.Logger) *CommentService` wires `rateBucket: newRateBucket()`. Method stubs returning `nil, errors.NotImplemented(...)`: `CreateComment(ctx, userID, username, animeID string, req *domain.CreateCommentRequest) (*domain.Comment, error)`, `UpdateComment(ctx, userID, commentID string, isAdmin bool, req *domain.UpdateCommentRequest) (*domain.Comment, error)`, `DeleteComment(ctx, userID, commentID string, isAdmin bool) error`, `ListComments(ctx, animeID, cursor string, limit int) (*domain.CommentsListResponse, error)`.

    File 4: `services/player/internal/handler/comment.go` (package `handler`). Declares `type CommentHandler struct { commentService *service.CommentService; log *logger.Logger }`, `NewCommentHandler(s *service.CommentService, log *logger.Logger) *CommentHandler`. Four method stubs that each respond with HTTP 501 via `httputil.Error(w, errors.NotImplemented("comment handler …"))`: `CreateComment`, `UpdateComment`, `DeleteComment`, `ListComments`. Use `chi.URLParam(r, "animeId")` and `chi.URLParam(r, "commentId")` so route wiring compiles later.

    All four files MUST compile when `go build ./...` runs from `services/player`. No other files in the player service are touched.
  </action>
  <verify>
    <automated>cd services/player && go build ./...</automated>
  </verify>
  <acceptance_criteria>
    - `cd services/player && go build ./...` exits 0.
    - File `services/player/internal/domain/comment.go` exists and contains the literal regex `func \(Comment\) TableName\(\) string \{ return "comments" \}` (via `grep -E`).
    - File `services/player/internal/domain/comment.go` does NOT contain the identifier `ParentID` inside `CreateCommentRequest` (Pitfall 8). Specifically: `awk '/CreateCommentRequest/,/^\}/' services/player/internal/domain/comment.go | grep -v '^#' | grep -c ParentID` outputs `0`.
    - File `services/player/internal/repo/comment.go` exports `NewCommentRepository`, `Create`, `GetByID`, `ListByAnime`, `Update`, `SoftDelete`. Verify via `grep -E 'func .*CommentRepository.*\b(Create|GetByID|ListByAnime|Update|SoftDelete)\b'`.
    - File `services/player/internal/service/comment.go` exports `NewCommentService`, `CreateComment`, `UpdateComment`, `DeleteComment`, `ListComments`.
    - File `services/player/internal/handler/comment.go` exports `NewCommentHandler`, `CreateComment`, `UpdateComment`, `DeleteComment`, `ListComments`.
    - No file references `&domain.Comment{}` inside `cmd/player-api/main.go` yet (that wiring is plan 04). Verify: `grep -c 'domain.Comment{' services/player/cmd/player-api/main.go` outputs `0`.
  </acceptance_criteria>
  <done>All four Go files exist, compile cleanly, and expose the documented method surface so the Wave-2 implementation plans can fill bodies without changing signatures.</done>
</task>

<task type="auto">
  <name>Task 0.2: Scaffold Go test stubs for comment repo + service + handler + migration idempotency</name>
  <files>services/player/internal/repo/comment_test.go, services/player/internal/service/comment_test.go, services/player/internal/handler/comment_test.go, services/player/cmd/player-api/main_test.go</files>
  <read_first>
    - services/player/internal/handler/sync_test.go (full file — copy `setupSyncTestDB` pattern, lines 1-100)
    - services/player/internal/repo/sync_test.go (full file — `setupTestDB` pattern)
    - services/player/internal/handler/mal_import_test.go (lines 130-180 — claims-into-context injection for handler tests)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-VALIDATION.md (per-task table — extract exact test names)
    - services/player/internal/domain/comment.go (from task 0.1 — for struct shape used by tests)
  </read_first>
  <action>
    Create four Go test files. Every test function uses `t.Skip("Wave 0 scaffold — implementation in plan 0X")` so the test compiles, runs, and visibly reports "skipped" rather than "passed" or "failed". The point is: the test binary builds and the named tests exist for later RED→GREEN wiring.

    File 1: `services/player/internal/repo/comment_test.go` (package `repo`). Declares helper `setupCommentTestDB(t *testing.T) *gorm.DB` that mirrors `setupTestDB` from `sync_test.go` and `AutoMigrate(&domain.Comment{}, &domain.AnimeInfo{})` against in-memory SQLite. Two test functions: `TestCommentRepo_SoftDelete(t *testing.T)` and `TestCommentRepo_ListByAnime_Cursor(t *testing.T)`. Both bodies: `t.Skip("Wave 0 scaffold — implementation in plan 03")`. SQLite caveat: do NOT rely on `gen_random_uuid()`; tests will set IDs explicitly via `uuid.NewString()` — note this in a top-of-file comment.

    File 2: `services/player/internal/service/comment_test.go` (package `service`). Tests: `TestCommentService_RateLimit(t *testing.T)` and `TestCommentService_EmitsActivity(t *testing.T)`. Both bodies: `t.Skip("Wave 0 scaffold — implementation in plan 03")`.

    File 3: `services/player/internal/handler/comment_test.go` (package `handler`). Tests: `TestCommentHandler_CreateComment_HappyPath(t *testing.T)`, `TestCommentHandler_CreateComment_EmptyBody(t *testing.T)`, `TestCommentHandler_UpdateComment_NotOwner(t *testing.T)`. Bodies: `t.Skip("Wave 0 scaffold — implementation in plan 03")`. Include the claims-injection helper extracted from `mal_import_test.go:140` (copy + rename — local to this file) so plan 03 can populate it.

    File 4: `services/player/cmd/player-api/main_test.go` (package `main`). Test: `TestSocialMigration_Idempotent(t *testing.T)`. Body: `t.Skip("Wave 0 scaffold — implementation in plan 01")`. This file MUST live in `cmd/player-api/` (same dir as `main.go`) so it has package access to any unexported migration helper plan 01 may extract.

    Every file: imports must be minimal but include `testing`. Where SQLite-in-memory is set up, import `gorm.io/driver/sqlite` (already in `go.mod` per RESEARCH.md). Verify with `cd services/player && go vet ./...`.
  </action>
  <verify>
    <automated>cd services/player && go test ./internal/repo/ ./internal/service/ ./internal/handler/ ./cmd/player-api/ -run 'TestCommentRepo_SoftDelete|TestCommentRepo_ListByAnime_Cursor|TestCommentService_RateLimit|TestCommentService_EmitsActivity|TestCommentHandler_CreateComment_HappyPath|TestCommentHandler_CreateComment_EmptyBody|TestCommentHandler_UpdateComment_NotOwner|TestSocialMigration_Idempotent' -v 2>&1 | grep -E '\-\-\- SKIP|^=== RUN' | wc -l</automated>
  </verify>
  <acceptance_criteria>
    - `cd services/player && go test ./internal/repo/ ./internal/service/ ./internal/handler/ ./cmd/player-api/ -run 'TestComment|TestSocial' -v` reports at least 8 SKIP outcomes (one per stub test) and exits 0.
    - All four test files exist; running `grep -l 't.Skip("Wave 0 scaffold' services/player/internal/repo/comment_test.go services/player/internal/service/comment_test.go services/player/internal/handler/comment_test.go services/player/cmd/player-api/main_test.go | wc -l` outputs `4`.
    - `go vet ./...` from `services/player` exits 0.
  </acceptance_criteria>
  <done>Eight named tests are discoverable by `go test -list .` in their respective packages; every one currently reports SKIP. Plans 01 and 03 will replace `t.Skip` with real assertions.</done>
</task>

<task type="auto">
  <name>Task 0.3: Scaffold Playwright comments.spec.ts + reviews-fixture capture script</name>
  <files>frontend/web/e2e/comments.spec.ts, scripts/capture-reviews-fixtures.sh</files>
  <read_first>
    - frontend/web/e2e/anime.spec.ts (full file — auth and navigation patterns)
    - frontend/web/e2e/profile.spec.ts (full file — login flow with API key / password)
    - frontend/web/playwright.config.ts (baseURL, project layout)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-VALIDATION.md (test names: deep-link, URL persists, anon login prompt, logged-in CRUD)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-SPEC.md (Acceptance Criteria — exact behavior of the four scenarios)
    - CLAUDE.md (UI Audit Test User section — `ui_audit_bot` credentials, login flow)
  </read_first>
  <action>
    File 1: `frontend/web/e2e/comments.spec.ts`. Use `import { test, expect } from '@playwright/test'`. Single `test.describe('Anime comments tab', ...)` block with four `test(...)` cases using the EXACT name keywords listed in 01-VALIDATION.md so `-g` filters match:
    - `test('deep-link to ?ugc=comments mounts Comments tab on first paint', ...)`
    - `test('URL persists across tab clicks via router.replace', ...)`
    - `test('anon login prompt shown to logged-out users on Comments tab', ...)`
    - `test('logged-in CRUD — post, edit, delete own comment', ...)`
    Every test body: `test.skip(true, 'Wave 0 scaffold — implementation in plan 06')`. Add a `test.beforeAll` block that declares the target anime ID as a constant `const ANIME_ID = '<a known seeded anime id from ui_audit_bot seed>'` — leave as `'TBD'` with a comment for plan 06 to replace. Imports only `@playwright/test`. No external module imports.

    File 2: `scripts/capture-reviews-fixtures.sh`. Bash script (`#!/usr/bin/env bash`, `set -euo pipefail`). Reads `${API_BASE:-http://localhost:8000}` and `${ANIME_ID:-?}` from env. Curls six endpoints exactly: `GET /api/anime/$ANIME_ID/reviews`, `GET /api/anime/$ANIME_ID/rating`, `GET /api/anime/$ANIME_ID/reviews/me` (needs `Authorization: Bearer $UI_AUDIT_API_KEY`), `POST /api/anime/ratings/batch -d '{"anime_ids":["'$ANIME_ID'"]}'`, plus prints a stub note for `POST /api/anime/$ANIME_ID/reviews` and `DELETE /api/anime/$ANIME_ID/reviews` (mutating — do NOT call from a fixture script, just print "SKIP: mutating endpoint shape already covered by handler test"). Output: one JSON blob per endpoint, prefixed with a `# === <endpoint> ===` separator. `chmod +x` after writing.

    Both files are self-contained and have ZERO impact on the running services or the build pipeline.
  </action>
  <verify>
    <automated>test -f frontend/web/e2e/comments.spec.ts && test -x scripts/capture-reviews-fixtures.sh && cd frontend/web && bunx playwright test --list e2e/comments.spec.ts 2>&1 | grep -E 'deep-link|URL persists|anon login prompt|logged-in CRUD' | wc -l</automated>
  </verify>
  <acceptance_criteria>
    - `bunx playwright test --list e2e/comments.spec.ts` (run from `frontend/web/`) outputs exactly 4 test names that contain the substrings `deep-link`, `URL persists`, `anon login prompt`, `logged-in CRUD` (one each).
    - `scripts/capture-reviews-fixtures.sh` is executable (`test -x` returns 0).
    - `bash -n scripts/capture-reviews-fixtures.sh` (syntax check) exits 0.
    - `comments.spec.ts` contains no `await page.` calls (it is pure skip-stubs at this point — verify via `grep -c 'await page' frontend/web/e2e/comments.spec.ts` outputs `0`).
  </acceptance_criteria>
  <done>Playwright recognizes four named tests in `comments.spec.ts` (all skipped); the reviews-fixture script is executable and validates as bash. Plan 06 will fill the Playwright bodies; SOCIAL-NF-01 manual smoke uses the capture script before/after plan 04 deploys.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| (none new) | Wave 0 is pure scaffolding. No new code paths execute against untrusted input. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-1-W0-01 | Tampering | Stub Comment handlers respond 501 to any client | accept | Stubs return `errors.NotImplemented` via `httputil.Error`; no DB write path active until plan 03; no exposure surface created. |
| T-1-W0-02 | Information disclosure | `scripts/capture-reviews-fixtures.sh` may log API key to stdout | mitigate | Script reads `UI_AUDIT_API_KEY` from env only; never echoes the key value; ops uses local-only fixture files in `tmp/` (gitignored). |
</threat_model>

<verification>
- `cd services/player && go build ./...` exits 0
- `cd services/player && go test -run 'TestComment|TestSocial' -v ./...` shows ≥ 8 SKIP outcomes, exit 0
- `cd frontend/web && bunx playwright test --list e2e/comments.spec.ts` lists 4 tests
- `bash -n scripts/capture-reviews-fixtures.sh` exits 0
- Player service running state is unchanged — no `make redeploy-player` required because nothing is wired into main.go yet.
</verification>

<success_criteria>
After this plan: every implementation task in Plans 01-06 has a real Go test or Playwright spec to convert from SKIP → assertion. No production behavior changed. CI gates introduced by future waves will be RED→GREEN cycles, not green-on-arrival.
</success_criteria>

<output>
After completion, create `.planning/workstreams/social/phases/01-social-reviews-comments/01-00-SUMMARY.md` documenting the 10 files added, their package surface, and the Nyquist contract handoff to Plan 01.
</output>
