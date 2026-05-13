---
phase: 1
workstream: social
plan: 3
subsystem: services/player (comments CRUD stack)
tags:
  - feature
  - cursor-pagination
  - rate-limit
  - activity-emission
  - wave-3
requirements:
  - SOCIAL-04
  - SOCIAL-05
dependency_graph:
  requires:
    - "Plan 00: Wave-0 scaffolds (8 SKIPPED tests + 4 stub files)"
    - "Plan 01: comments table + idx_comments_anime_created + idx_comments_user_created (live in Postgres + AutoMigrate)"
    - "Plan 02: activity_events test schema pattern (id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))))"
  provides:
    - "services/player/internal/repo/comment.go: CommentRepository.{Create, GetByID, ListByAnime, Update, SoftDelete} — production implementations"
    - "services/player/internal/service/comment.go: CommentService with rateBucket (10/hour/user/anime sliding window) + utf8.RuneCountInString body validation + activity emission (no dedup)"
    - "services/player/internal/handler/comment.go: 4 endpoint methods (CreateComment / UpdateComment / DeleteComment / ListComments) returning 201/200/204 + 400/401/403/429"
    - "7 Go tests flipped from SKIP → PASS; TestSocialMigration_Idempotent remains green from Plan 01"
  affects:
    - "services/player/cmd/player-api/main.go (NOT modified — Plan 04 wires the handler/service/repo trio)"
    - "services/player/internal/transport/router.go (NOT modified — Plan 04 mounts the chi routes)"
tech-stack:
  added: []
  patterns:
    - "Cursor pagination via libs/pagination.Cursor{ID, Timestamp}.Encode + DecodeCursor (consumed verbatim; no hand-rolled base64)"
    - "Sliding-window rate limit (in-memory, per-process) keyed by (userID, animeID) with sync.Mutex-locked prune-and-append (RESEARCH.md Pattern 5)"
    - "UTF-8-aware body length cap via utf8.RuneCountInString (NOT len()) — Cyrillic/Japanese 2000-char comments accepted"
    - "Activity-event emission template adapted from service/review.go:50-86 with the per-day dedup branch stripped (every comment create emits a fresh row)"
    - "SQLite test schema with `lower(hex(randomblob(16)))` id default — same pattern Plan 02 introduced for activity_events"
    - "Owner-or-admin authorization via authz.IsAdmin(ctx) + explicit `existing.UserID != userID && !isAdmin` check returning errors.Forbidden"
key-files:
  created: []
  modified:
    - "services/player/internal/repo/comment.go (Wave-0 stubs → production impl)"
    - "services/player/internal/repo/comment_test.go (Wave-0 SKIPs → real assertions + raw-SQL schema)"
    - "services/player/internal/service/comment.go (Wave-0 stubs → production impl + working rateBucket.allow)"
    - "services/player/internal/service/comment_test.go (Wave-0 SKIPs → real assertions)"
    - "services/player/internal/handler/comment.go (Wave-0 stubs → production impl with auth + bind + service-call + error-map)"
    - "services/player/internal/handler/comment_test.go (Wave-0 SKIPs → real assertions + chi router fixture)"
decisions:
  - "setupCommentTestDB used AutoMigrate(&domain.Comment{}) in Wave 0. That call FAILS on SQLite with 'near \"(\": syntax error' because the Comment struct's GORM tags carry Postgres-only `default:gen_random_uuid()` and `default:now()`. Replaced with raw CREATE TABLE in all three test files (repo + service + handler), mirroring the activity_events trick from Plan 02 (id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16))))). The Wave-0 scaffold only compiled because every test was SKIPPED — the broken AutoMigrate was never executed."
  - "Validation centralized in a `validateBody` helper called from both CreateComment and UpdateComment — the plan's behavior block listed the same trim + non-empty + 2000-rune cap rules twice, so a single helper keeps them in lockstep. The literal acceptance criterion `grep -c 'utf8.RuneCountInString' ... outputs ≥ 2` still passes (count is 2: one in the helper, one in the doc comment) and the substantive intent (both endpoints reject the same way) is satisfied."
  - "logger.NewNop() was specified in the plan but doesn't exist in libs/logger. Used `logger.New(logger.Config{Level: \"error\", ...})` instead — same pattern Plan 02 used in service/review_test.go. Produces near-silent output at the test level; activity-emit failures (which the service treats as non-fatal) wouldn't spam the test output anyway because they don't happen in green-path tests."
  - "Cursor strict-less-than tuple comparison emitted as `created_at < ? OR (created_at = ? AND id < ?)` to handle the (extremely unlikely on Postgres but possible on SQLite's 1-second timestamp resolution) tie on created_at. Plan's <behavior> matches verbatim."
  - "Admin override is allowed for both UPDATE and DELETE at the service layer per CONTEXT.md (frontend hides the pencil for admins on non-owned comments; backend allows admin-edit for tooling parity). TestCommentHandler_UpdateComment_NotOwner asserts both directions: non-admin bob → 403 + DB row unchanged; admin bob → 200 + DB row updated to 'admin-edit'."
  - "rateBucket is constructed inside NewCommentService (NOT as a package-level singleton) so each test gets a fresh bucket; without this, TestCommentService_RateLimit would 429 itself across runs. Acceptable for v0.1 single-replica per CONTEXT.md."
  - "Comments list response uses `domain.CommentsListResponse{Comments, NextCursor, HasMore}` directly (no projection struct). Comments have only 9 fields total in the domain.Comment struct and none are sensitive — unlike reviews (where the 7-field projection in Plan 02 had to filter out 10+ AnimeListEntry fields). When Plan 04 wires the gateway it can choose to project if SOCIAL-NF-01-style wire-shape locking is desired."
metrics:
  duration_minutes: 14
  completed_date: "2026-05-13"
  tasks_completed: 3
  files_created: 0
  files_modified: 6
  commits: 3
---

# Phase 1 Plan 03 (Workstream `social`): Comments CRUD Stack Summary

**One-liner:** CommentRepository / CommentService / CommentHandler stubs
replaced with production implementations driving SOCIAL-04 (CRUD + cursor
pagination + 1-2000 rune body + 10/hour rate limit + soft delete) and
SOCIAL-05 (one activity event per comment, no per-day dedup). All 7
previously-SKIPPED Go tests now PASS; TestSocialMigration_Idempotent
remains green from Plan 01. Plan 04 wires the chi router so these
handlers become live HTTP endpoints.

## What Was Built

### Repository layer — `services/player/internal/repo/comment.go`

Five methods, all backed by GORM against the `comments` table created
by Plan 01's AutoMigrate:

| Method | Behavior |
|---|---|
| `Create(ctx, c)` | Inserts via `db.Create(c)`. Wraps any error as `errors.CodeInternal`. Production Postgres assigns ID via `gen_random_uuid()`; SQLite tests assign via `lower(hex(randomblob(16)))` or explicit `c.ID = newUUIDHex(t)`. |
| `GetByID(ctx, id)` | `WHERE id = ?` + GORM's auto-injected `AND deleted_at IS NULL`. Maps `gorm.ErrRecordNotFound` → `errors.NotFound("comment")`. |
| `ListByAnime(ctx, animeID, cursor, limit)` | Newest-first via `Order("created_at DESC, id DESC")`. If cursor non-empty, decode via `pagination.DecodeCursor` and filter `(created_at < cur.Timestamp OR (created_at = cur.Timestamp AND id < cur.ID))`. Query `Limit(limit+1)`; when `len > limit`, drop the extra and emit `pagination.Cursor{ID, Timestamp}.Encode()` for the last visible row. Invalid cursor → `errors.InvalidInput`. |
| `Update(ctx, id, body)` | `UPDATE comments SET body = ? WHERE id = ?`. Zero rows affected → `errors.NotFound("comment")`. |
| `SoftDelete(ctx, id)` | `db.Where("id = ?", id).Delete(&domain.Comment{})` — GORM converts to `UPDATE deleted_at = NOW()` because of the `gorm.DeletedAt` tag. Idempotent on missing rows. |

### Service layer — `services/player/internal/service/comment.go`

Four public methods + a working `rateBucket`:

- **`CreateComment(ctx, userID, username, animeID, req)`** — trim + non-empty + ≤2000-rune body via the centralized `validateBody` helper → `rateBucket.allow` gate (10/hour/(userID, animeID)) returning `errors.RateLimited` on excess → `commentRepo.Create` → emit exactly one `activity_events` row with `type='comment'` and `content` = first 300 runes (+ "…" suffix when truncated). Activity-emit failure is non-fatal (logged, returns the saved comment).
- **`UpdateComment(ctx, userID, commentID, isAdmin, req)`** — same body validation, then `GetByID` → owner-or-admin check (`existing.UserID != userID && !isAdmin` → `errors.Forbidden`) → `Update(body)` → reload via `GetByID` to return the canonical row.
- **`DeleteComment(ctx, userID, commentID, isAdmin)`** — `GetByID` → owner-or-admin check → `SoftDelete`.
- **`ListComments(ctx, animeID, cursor, limit)`** — clamps limit to `[1, 100]` (default 50), delegates to `commentRepo.ListByAnime`, wraps the (comments, nextCursor) tuple in `CommentsListResponse{Comments, NextCursor, HasMore: nextCursor != ""}`.

**`rateBucket` (RESEARCH.md Pattern 5):** `map[string][]time.Time` keyed by `userID + "|" + animeID`, `sync.Mutex`-locked. On every `allow()` call: prune timestamps older than `time.Hour`, return false if ≥ 10 remain, else append `now` and return true. In-place prune via `keep := existing[:0]` to avoid extra allocations. Constructed fresh inside `NewCommentService` so each test gets isolated state.

### Handler layer — `services/player/internal/handler/comment.go`

Four endpoint methods mirroring `handler/review.go` for the auth + bind + service-call + error-map idiom:

| Method | Status | Notes |
|---|---|---|
| `CreateComment` | 201 / 400 / 401 / 429 / 500 | `chi.URLParam("animeId")` → `ClaimsFromContext` → `httputil.Bind(&CreateCommentRequest)` → service → `httputil.Created`. |
| `UpdateComment` | 200 / 400 / 401 / 403 / 404 / 500 | Both `animeId` + `commentId` extracted. Passes `authz.IsAdmin(ctx)` to the service. `httputil.OK`. |
| `DeleteComment` | 204 / 401 / 403 / 404 / 500 | Same auth pattern. `httputil.NoContent`. |
| `ListComments` | 200 / 400 / 500 | Public. Parses `?cursor=` + `?limit=` via `strconv.Atoi`; service applies default/cap. |

All error returns flow through `httputil.Error` which auto-maps via `libs/errors` — `InvalidInput` → 400, `Unauthorized` → 401, `Forbidden` → 403, `NotFound` → 404, `RateLimited` → 429, anything else → 500.

### Test changes

Seven Go test bodies converted from `t.Skip("Wave 0 scaffold …")` to real assertions. Critical fixture change: the Wave-0 `setupCommentTestDB` helpers in all three test files used `db.AutoMigrate(&domain.Comment{}, &domain.AnimeInfo{})`, which produces this SQLite error:

```
near "(": syntax error
CREATE TABLE `comments` (`id` uuid DEFAULT gen_random_uuid(), …, `created_at` datetime NOT NULL DEFAULT now(), …)
```

The Wave-0 scaffold only compiled because every test was SKIPPED. The fix is to create the schema via raw SQL, mirroring the production Postgres shape but using `lower(hex(randomblob(16)))` as the id default (same pattern Plan 02 adopted for `activity_events` in `service/review_test.go`). All three test files now construct their schemas this way.

Test outcomes:

| Test | File | Status | What it asserts |
|---|---|---|---|
| `TestCommentRepo_SoftDelete` | repo/comment_test.go | PASS | SoftDelete sets deleted_at non-NULL; ListByAnime omits the row; GetByID returns errors.NotFound; second SoftDelete is a no-op; SoftDelete on non-existent id is a no-op. |
| `TestCommentRepo_ListByAnime_Cursor` | repo/comment_test.go | PASS | 5 seeded rows, first page (limit=3) returns rows [4,3,2] newest-first with a non-empty cursor pointing at row 2; cursor round-trips through `pagination.DecodeCursor` (asserts ID + Timestamp); second page returns rows [1,0] with empty cursor; invalid cursor → `errors.InvalidInput`. |
| `TestCommentService_RateLimit` | service/comment_test.go | PASS | 10 successful creates for (u1, animeA), 11th → `CodeRateLimited`. Different user on same anime OK; same user on different anime OK. |
| `TestCommentService_EmitsActivity` | service/comment_test.go | PASS | First create → 1 activity row with `type='comment'`, `content='hello world'`, `username='alice'`. Second same-day same-(user, anime) create → 2 rows (no dedup; divergence from reviews). 350-char body → preview is exactly 300 'a's + "…" (rune count 301). |
| `TestCommentHandler_CreateComment_HappyPath` | handler/comment_test.go | PASS | POST `/api/anime/a1/comments/` with `{"body":"hello"}` + claims → 201, body fields `{user_id, anime_id, username, body, id (non-empty)}`. |
| `TestCommentHandler_CreateComment_EmptyBody` | handler/comment_test.go | PASS | POST with `{"body":"   "}` + claims → 400, error envelope `{code: "INVALID_INPUT", message contains "cannot be empty"}`. |
| `TestCommentHandler_UpdateComment_NotOwner` | handler/comment_test.go | PASS | Seed comment owned by alice; PATCH from bob (RoleUser) → 403 (FORBIDDEN) + DB body untouched. PATCH from bob (RoleAdmin) → 200 + DB body = "admin-edit". |

Plus `TestSocialMigration_Idempotent` (Plan 01) — still PASS.

## Verification

Per the plan's `<verification>` block:

| Check | Result |
|---|---|
| `cd services/player && go build ./...` exits 0 | PASS |
| `cd services/player && go test ./... -count=1` exits 0 | PASS (all packages green) |
| `cd services/player && go test ./internal/... -race -count=1` exits 0 | PASS |
| `grep -rE 'errors.NotImplemented' services/player/internal/{repo,service,handler}/comment*.go` returns nothing | PASS (zero matches) |
| `grep -rE 't.Skip\("Wave 0 scaffold' services/player/internal/{repo,service,handler}/comment*_test.go` returns nothing | PASS (zero matches) |

Per-task acceptance criteria:

**Task 3.1 (Repo) — all 5 green:**
- Both repo tests PASS.
- `grep -c 't.Skip' internal/repo/comment_test.go` → `0`.
- `grep -c 'errors.NotImplemented' internal/repo/comment.go` → `0`.
- `grep -c 'pagination.Cursor' internal/repo/comment.go` → `1` (≥ 1).
- `grep -c 'pagination.DecodeCursor' internal/repo/comment.go` → `1` (≥ 1).
- `go build ./...` → 0.

**Task 3.2 (Service) — all 7 green:**
- Both service tests PASS.
- `grep -c 't.Skip' internal/service/comment_test.go` → `0`.
- `grep -c 'errors.NotImplemented' internal/service/comment.go` → `0`.
- `grep -c 'utf8.RuneCountInString' internal/service/comment.go` → `2` (≥ 2).
- `grep -c 'errors.RateLimited' internal/service/comment.go` → `1` (≥ 1).
- `grep -E 'Type:\s*"comment"' internal/service/comment.go` → matches.
- `go build ./...` → 0.

**Task 3.3 (Handler) — all 7 green:**
- All three handler tests PASS.
- `grep -c 't.Skip' internal/handler/comment_test.go` → `0`.
- `grep -c 'errors.NotImplemented' internal/handler/comment.go` → `0`.
- `grep -c 'authz.ClaimsFromContext' internal/handler/comment.go` → `3` (≥ 3).
- `grep -c 'authz.IsAdmin' internal/handler/comment.go` → `2` (≥ 2).
- `grep -c 'chi.URLParam' internal/handler/comment.go` → `6` (≥ 4).
- `go build ./...` → 0; full `go test ./...` → 0.

## Commits

| Task | Commit | Message |
|------|--------|---------|
| 3.1 | `8f8f1a4` | `feat(1-3): implement CommentRepository with cursor pagination + soft delete` |
| 3.2 | `2a3655a` | `feat(1-3): implement CommentService with validation + rate-limit + activity emission` |
| 3.3 | `30b4c34` | `feat(1-3): implement CommentHandler endpoints with 201/200/204 happy paths + 400/401/403/429 failures` |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] Wave-0 `setupCommentTestDB` AutoMigrate fails on SQLite**

- **Found during:** Task 3.1 (running the first real repo test).
- **Issue:** The Wave-0 scaffold helper in `repo/comment_test.go`:
  ```go
  if err := db.AutoMigrate(&domain.Comment{}, &domain.AnimeInfo{}); err != nil {
      t.Fatalf(...)
  }
  ```
  fails on SQLite with `near "(": syntax error`, because `domain.Comment`'s GORM tags carry the Postgres-only defaults `default:gen_random_uuid()` and `default:now()`. The Wave-0 SUMMARY documents that AutoMigrate "does NOT execute the gen_random_uuid() default — the column simply has no default on SQLite," but this is incorrect — GORM emits the literal `DEFAULT gen_random_uuid()` clause into the CREATE TABLE statement, which SQLite refuses to parse. The scaffold only compiled because every test was `t.Skip(...)`-gated; the broken AutoMigrate was never reached.
- **Fix:** Replace AutoMigrate with raw SQL in all three test files (`repo/comment_test.go`, `service/comment_test.go`, `handler/comment_test.go`), using `lower(hex(randomblob(16)))` as the id default — same pattern Plan 02 adopted for `activity_events` in `service/review_test.go`. Each test file now creates its own schema with the columns and indexes that match the production Postgres shape.
- **Files modified:** all three `comment*_test.go` files (the setup helpers — production code unaffected).
- **Commits:** bundled into 3.1 / 3.2 / 3.3 each.

**2. [Rule 3 — Blocking] `logger.NewNop()` does not exist**

- **Found during:** Task 3.2 (writing service test boilerplate).
- **Issue:** The plan's <action> block says to construct the service via `service := NewCommentService(commentRepo, activityRepo, logger.NewNop())`. There is no `NewNop` constructor in `libs/logger` — only `New(cfg Config)` and `Default()`. Same gap Plan 02 papered over by using `logger.New(logger.Config{Level: "error", ...})`.
- **Fix:** Use the Plan-02-style `logger.New(logger.Config{Level: "error", Development: false, Encoding: "json"})` in all three test files. Errors get logged at error-level only (which is fine — green-path tests don't trigger activity-emit failures, so the test output stays clean).
- **Files modified:** all three test files.
- **Commits:** bundled into each task.

### Notes (not deviations)

- `httputil.Bind` returns `errors.InvalidInput("invalid request body")` on JSON parse failure, which the empty-body test would NOT trigger (the body `{"body":"   "}` is valid JSON; the service-layer trim+empty check fires). The handler test asserts on the service-layer error message ("cannot be empty"), not the bind-layer one. The plan's behavior block matches this.
- `httputil.OK` / `httputil.Created` / `httputil.Error` wrap responses in an envelope `{"success": true|false, "data": ...}` or `{"success": false, "error": {code, message}}`. The handler test fixtures decode through this envelope before asserting on individual fields. This is the project-wide convention (mirrors `handler/review_shape_test.go`).
- The plan asked for the SUMMARY to be created at `.planning/workstreams/social/phases/01-social-reviews-comments/01-03-SUMMARY.md`. Per the executor objective, the target path is `03-SUMMARY.md` (matching Plans 00/01/02 naming). Going with `03-SUMMARY.md`.
- Background activity from other workstreams (continue-watching `feat(08)` commits) landed on `main` between my task commits. None of them touch comment files; my 3 commits are intact at `8f8f1a4`, `2a3655a`, `30b4c34`.
- The validation acceptance criterion `grep -c 'utf8.RuneCountInString' services/player/internal/service/comment.go` outputs `2`. The plan said "≥ 2 (one in CreateComment, one in UpdateComment)" — I centralized both calls into a `validateBody` helper called by both endpoints. The grep still hits 2 occurrences (one in the helper body + one in the doc comment for the constant), so the literal acceptance check passes and the substantive intent (both endpoints reject the same way) is satisfied.

## Handoff to Plan 04

Plan 04 (wiring + gateway proxy) can now assume:

- `repo.NewCommentRepository(db)` returns a fully working `*CommentRepository`.
- `service.NewCommentService(commentRepo, activityRepo, log)` returns a fully working `*CommentService` with a fresh in-memory rate bucket.
- `handler.NewCommentHandler(svc, log)` returns a fully working `*CommentHandler` whose four methods are ready to mount on chi.
- The route shape to mount (per RESEARCH.md Pattern 3, adapted for comments):
  ```go
  // Inside r.Route("/anime/{animeId}", func(r chi.Router) { ... })
  r.Get("/comments", commentHandler.ListComments) // public

  r.Group(func(r chi.Router) {
      r.Use(AuthMiddleware(jwtConfig))
      r.Post("/comments", commentHandler.CreateComment)
      r.Patch("/comments/{commentId}", commentHandler.UpdateComment)
      r.Delete("/comments/{commentId}", commentHandler.DeleteComment)
  })
  ```
- The gateway proxy in `services/gateway/internal/transport/router.go` must add explicit `/api/anime/{id}/comments*` entries BEFORE the `/anime/*` catch-all that routes to catalog (RESEARCH.md anti-pattern: "Don't bypass the gateway").

## Known Stubs

None. The three production files are fully implemented. The handlers exist
but are not yet reachable from the live HTTP server — Plan 04 closes that
wiring gap. This is the documented Wave-3 deliverable per the plan's
`<objective>`: "Service is NOT yet wired into main.go or chi router (plan
04 does the wiring) — handlers exist but are unreachable from the live
HTTP server until plan 04 lands."

## Threat Flags

None new. The plan's threat model identified six threats, all addressed:

- **T-1-V5 (Input validation — body)**: mitigated via `utf8.RuneCountInString` in `validateBody`, unit-tested by `TestCommentHandler_CreateComment_EmptyBody`.
- **T-1-V5 (cursor tampering)**: mitigated. `pagination.DecodeCursor` validates base64 + JSON; GORM `Where("created_at < ? OR …", …)` parameterizes — no string interpolation. Invalid cursor returns `errors.InvalidInput` (asserted in `TestCommentRepo_ListByAnime_Cursor`).
- **T-1-V4 (Access control)**: mitigated by explicit `existing.UserID != userID && !isAdmin` check in both UpdateComment and DeleteComment; unit-tested in `TestCommentHandler_UpdateComment_NotOwner`.
- **T-1-V11 (DoS / business logic — rate limit)**: mitigated. 10/hour/(user, anime) sliding window; per-instance bucket; unit-tested in `TestCommentService_RateLimit`.
- **Mass-assignment**: mitigated. `domain.CreateCommentRequest` DTO does not declare `parent_id`; `httputil.Bind` cannot populate fields not on the struct. The service builds `*domain.Comment{UserID, AnimeID, Username, Body}` with `ParentID` left nil.
- **Soft-delete bypass**: mitigated. All queries flow through CommentRepository; no handler issues raw SQL. `gorm.DeletedAt` auto-injects `WHERE deleted_at IS NULL`; `TestCommentRepo_SoftDelete` asserts ListByAnime omits the deleted row.

## Self-Check: PASSED

**Files verified to exist (modified):**

- `services/player/internal/repo/comment.go` — FOUND (production impl; 0 occurrences of `CodeUnavailable`)
- `services/player/internal/repo/comment_test.go` — FOUND (2 PASS tests, 0 SKIPs)
- `services/player/internal/service/comment.go` — FOUND (production impl; 0 occurrences of `CodeUnavailable`)
- `services/player/internal/service/comment_test.go` — FOUND (2 PASS tests, 0 SKIPs)
- `services/player/internal/handler/comment.go` — FOUND (production impl; 0 occurrences of `CodeUnavailable`)
- `services/player/internal/handler/comment_test.go` — FOUND (3 PASS tests, 0 SKIPs)

**Commits verified in `git log --all`:**

- `8f8f1a4` — FOUND (Task 3.1 — CommentRepository)
- `2a3655a` — FOUND (Task 3.2 — CommentService)
- `30b4c34` — FOUND (Task 3.3 — CommentHandler)

**Full suite check:**

- `cd services/player && go build ./...` — exits 0
- `cd services/player && go test ./... -count=1` — all packages PASS
- `cd services/player && go test ./internal/... -race -count=1` — all packages PASS
- All 8 previously-skipped tests in the social phase now PASS; 0 SKIPs, 0 FAILs remaining for `Test(Comment|Social)*` patterns.
