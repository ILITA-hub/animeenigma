---
phase: 1
workstream: social
plan: 3
type: execute
wave: 2
depends_on: [1, 2]
files_modified:
  - services/player/internal/repo/comment.go
  - services/player/internal/repo/comment_test.go
  - services/player/internal/service/comment.go
  - services/player/internal/service/comment_test.go
  - services/player/internal/handler/comment.go
  - services/player/internal/handler/comment_test.go
autonomous: true
requirements:
  - SOCIAL-04
  - SOCIAL-05

must_haves:
  truths:
    - "CommentRepository implements Create, GetByID, ListByAnime (cursor-paginated), Update (body), SoftDelete — all backed by GORM against the comments table."
    - "ListByAnime excludes rows where deleted_at IS NOT NULL (soft-delete filter — gorm.DeletedAt index handles this automatically)."
    - "ListByAnime returns up to `limit` rows + a next_cursor string when more rows exist; cursor decodes via libs/pagination.DecodeCursor and round-trips."
    - "CommentService.CreateComment rejects empty / whitespace-only bodies with errors.InvalidInput (→ HTTP 400)."
    - "CommentService.CreateComment rejects bodies longer than 2000 UTF-8 runes (NOT bytes) with errors.InvalidInput."
    - "CommentService.CreateComment enforces 10/hour/user/anime via an in-memory bucket; the 11th call within an hour returns errors.RateLimited (→ HTTP 429)."
    - "CommentService.CreateComment emits exactly one activity_events row per successful comment with type='comment' and content = body truncated to 300 runes + '…' if longer."
    - "CommentService.UpdateComment allows owner OR admin; non-owner non-admin returns errors.Forbidden (→ HTTP 403)."
    - "CommentService.DeleteComment is soft-delete: sets deleted_at, row remains, subsequent ListByAnime excludes it."
    - "CommentHandler endpoints use chi.URLParam(r, \"animeId\") and chi.URLParam(r, \"commentId\"); auth-required endpoints read claims via authz.ClaimsFromContext."
    - "CreateCommentRequest does NOT expose parent_id; comments created in v0.1 always have ParentID = nil (Pitfall 8)."
  artifacts:
    - path: "services/player/internal/repo/comment.go"
      provides: "Production CommentRepository implementation using libs/pagination.Cursor for ListByAnime"
      contains: "pagination.DecodeCursor"
    - path: "services/player/internal/service/comment.go"
      provides: "Production CommentService with rateBucket (per-user-anime sliding hour window), validation, activity emission"
      contains: "utf8.RuneCountInString"
    - path: "services/player/internal/handler/comment.go"
      provides: "Four production handler methods returning 201/200/204 happy-path and 400/401/403/429 failure-path"
      contains: "authz.ClaimsFromContext"
  key_links:
    - from: "services/player/internal/service/comment.go"
      to: "services/player/internal/repo/activity.go"
      via: "activityRepo.Create(ctx, *domain.ActivityEvent{Type: 'comment', ...})"
      pattern: "activityRepo\\.Create"
    - from: "services/player/internal/service/comment.go (rateBucket)"
      to: "errors.RateLimited"
      via: "rateBucket.allow returns false -> errors.RateLimited() -> HTTP 429 via libs/errors mapping"
      pattern: "errors.RateLimited"
    - from: "services/player/internal/repo/comment.go (ListByAnime)"
      to: "libs/pagination/cursor.go"
      via: "pagination.Cursor{ID, Timestamp}.Encode() + pagination.DecodeCursor"
      pattern: "pagination.Cursor"
---

<objective>
Land the comments CRUD stack — repo, service, handler — backed by the comments table created in plan 01. Drive every test from Wave 0 from SKIP to PASS. Emit one activity event per successful comment create. Rate-limit 10/hour/user/anime via an in-memory bucket scoped to a single CommentService instance (so tests don't leak between cases).

Purpose: SOCIAL-04 (CRUD + cursor pagination + 1-2000 char body + 10/hour rate limit + soft delete) and SOCIAL-05 (activity event per comment, no per-day dedup).

Output: three production-ready Go files (repo/service/handler for comments); seven test functions converted from SKIP to PASS. Service is NOT yet wired into main.go or chi router (plan 04 does the wiring) — handlers exist but are unreachable from the live HTTP server until plan 04 lands.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/social/phases/01-social-reviews-comments/01-SPEC.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-CONTEXT.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-VALIDATION.md
@services/player/internal/domain/comment.go
@services/player/internal/repo/comment.go
@services/player/internal/service/comment.go
@services/player/internal/handler/comment.go
@services/player/internal/repo/activity.go
@services/player/internal/service/review.go
@services/player/internal/handler/review.go
@libs/pagination/cursor.go
@libs/errors/errors.go
@libs/httputil/response.go
@libs/authz/jwt.go

<interfaces>
<!-- Library surfaces consumed by this plan -->

From libs/pagination/cursor.go:
- type Cursor struct { ID string; Timestamp time.Time }
- (c Cursor) Encode() string                  // base64 JSON
- DecodeCursor(s string) (Cursor, error)      // returns errors.InvalidInput on parse failure

From libs/errors/errors.go (status-code mapping):
- errors.InvalidInput(msg)  -> 400
- errors.Unauthorized()     -> 401
- errors.Forbidden(msg)     -> 403
- errors.NotFound(msg)      -> 404
- errors.RateLimited()      -> 429
- errors.Wrap(err, code, msg)

From libs/httputil/response.go:
- httputil.OK(w, body)              -> 200
- httputil.Created(w, body)         -> 201
- httputil.NoContent(w)             -> 204
- httputil.BadRequest(w, msg)       -> 400
- httputil.Unauthorized(w)          -> 401
- httputil.Forbidden(w, msg)        -> 403
- httputil.Error(w, err)            -> mapped via libs/errors
- httputil.Bind(r, &dto) error      -> JSON body decode

From libs/authz/jwt.go:
- type Claims struct { UserID string; Username string; Role string; ... }
- ClaimsFromContext(ctx) (*Claims, bool)
- IsAdmin(ctx) bool

From services/player/internal/repo/activity.go:
- (r *ActivityRepository) Create(ctx, event *domain.ActivityEvent) error

From services/player/internal/domain/activity.go:
- type ActivityEvent struct { ID, UserID, Username, AnimeID, Type, Content, OldValue, NewValue string; CreatedAt time.Time; DeletedAt gorm.DeletedAt; ... }
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 3.1: Implement CommentRepository (Create, GetByID, ListByAnime cursor-paginated, Update, SoftDelete)</name>
  <files>services/player/internal/repo/comment.go, services/player/internal/repo/comment_test.go</files>
  <behavior>
    - `Create(ctx, c *domain.Comment) error` inserts a new row; relies on GORM `gen_random_uuid()` default for ID (or sets one if nil). Sets `CreatedAt = NOW()`, `UpdatedAt = NOW()`.
    - `GetByID(ctx, id string) (*domain.Comment, error)` — returns the row; errors.NotFound on no match; gorm.DeletedAt filter applies automatically so soft-deleted rows return NotFound.
    - `ListByAnime(ctx, animeID, cursorStr string, limit int) (comments []*domain.Comment, nextCursor string, err error)` — newest-first via `Order("created_at DESC, id DESC")`. If `cursorStr != ""`, decode via `pagination.DecodeCursor`; filter `(created_at < cur.Timestamp OR (created_at = cur.Timestamp AND id < cur.ID))`. Query `Limit(limit + 1)`; if len > limit, drop the last element and emit `pagination.Cursor{ID: last.ID, Timestamp: last.CreatedAt}.Encode()` as nextCursor. If len <= limit, nextCursor is "". gorm.DeletedAt soft-delete filter applies automatically — no manual WHERE deleted_at IS NULL needed.
    - `Update(ctx, id, body string) error` — UPDATE comments SET body=?, updated_at=NOW() WHERE id=? AND deleted_at IS NULL. Returns errors.NotFound on 0 rows affected.
    - `SoftDelete(ctx, id string) error` — `r.db.WithContext(ctx).Delete(&domain.Comment{}, "id = ?", id).Error` — GORM converts to UPDATE deleted_at because of gorm.DeletedAt tag. Returns nil even if no row matches (idempotent).
  </behavior>
  <read_first>
    - services/player/internal/repo/comment.go (Wave 0 stub)
    - services/player/internal/repo/comment_test.go (Wave 0 SKIP stubs)
    - services/player/internal/repo/activity.go (lines 30-90 — cursor pattern precedent)
    - libs/pagination/cursor.go (Cursor struct + Encode + DecodeCursor)
    - services/player/internal/repo/sync_test.go (setupTestDB SQLite-in-memory helper)
    - services/player/internal/domain/comment.go (struct + indexes)
  </read_first>
  <action>
    Replace the Wave 0 stub bodies with the real implementations described in `<behavior>`. Reuse `libs/pagination.Cursor` verbatim — do NOT hand-roll base64 encoding. Use GORM's `Where(...)`, `Order(...)`, `Limit(...)` chain; no raw SQL needed.

    Convert the two test stubs from SKIP to PASS:

    `TestCommentRepo_SoftDelete`:
    - Setup: setupTestDB; AutoMigrate(&domain.Comment{}).
    - Seed: insert two comments for the same anime, distinct user_ids, distinct created_at (1 second apart).
    - Call `repo.SoftDelete(ctx, comment1.ID)`. Assert no error.
    - Reload via GORM raw `Unscoped().First(&c, "id = ?", comment1.ID)` and assert `c.DeletedAt.Valid` is true.
    - Call `repo.ListByAnime(ctx, animeID, "", 50)`. Assert returned slice contains ONLY comment2 (length 1).
    - Call `repo.GetByID(ctx, comment1.ID)`. Assert error is errors.NotFound (use `errors.Is` or string match on the code).

    `TestCommentRepo_ListByAnime_Cursor`:
    - Setup: setupTestDB; AutoMigrate(&domain.Comment{}).
    - Seed: insert 5 comments for the same anime with explicit, monotonically increasing created_at values (e.g. now-5s, now-4s, …, now-1s). Set `c.ID = uuid.NewString()` explicitly because SQLite doesn't support gen_random_uuid().
    - First call: `repo.ListByAnime(ctx, animeID, "", 3)`. Assert len(result)==3, results are in newest-first order (created_at of [0] > [1] > [2]), nextCursor != "".
    - Second call: `repo.ListByAnime(ctx, animeID, firstCallNextCursor, 3)`. Assert len(result)==2 (the remaining two), nextCursor == "".
    - Decode the firstCallNextCursor and assert it round-trips: `pagination.DecodeCursor(firstCallNextCursor)` returns the ID + timestamp of the third result from the first call.
  </action>
  <verify>
    <automated>cd services/player && go test ./internal/repo/ -run 'TestCommentRepo_SoftDelete|TestCommentRepo_ListByAnime_Cursor' -v -count=1</automated>
  </verify>
  <acceptance_criteria>
    - Both tests pass with `--- PASS`.
    - `grep -c 't.Skip' services/player/internal/repo/comment_test.go` outputs `0` (no skips remain).
    - `grep -c 'errors.NotImplemented' services/player/internal/repo/comment.go` outputs `0` (no stub bodies remain).
    - `grep -c 'pagination.Cursor' services/player/internal/repo/comment.go` outputs ≥ 1 (the Encode call) and `grep -c 'pagination.DecodeCursor' services/player/internal/repo/comment.go` outputs ≥ 1.
    - `cd services/player && go build ./...` exits 0.
  </acceptance_criteria>
  <done>CommentRepository is production-ready and round-trips correctly through cursor pagination. Soft-delete is idempotent and properly excludes rows from list queries.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3.2: Implement CommentService with validation + rate-limit + activity emission</name>
  <files>services/player/internal/service/comment.go, services/player/internal/service/comment_test.go</files>
  <behavior>
    - `CreateComment(ctx, userID, username, animeID string, req *domain.CreateCommentRequest) (*domain.Comment, error)`:
      - Trim req.Body. If empty, return `errors.InvalidInput("comment body cannot be empty")`.
      - `utf8.RuneCountInString(body)`. If > 2000, return `errors.InvalidInput("comment body cannot exceed 2000 characters")`.
      - `if !s.rateBucket.allow(userID, animeID) { return nil, errors.RateLimited() }`.
      - Build `*domain.Comment{UserID, AnimeID, Username, Body}`; call `commentRepo.Create(ctx, c)`. Wrap any error with `errors.Wrap(err, errors.CodeInternal, "failed to save comment")`.
      - Build content preview: rune-truncate to 300 + `"…"` if truncation occurred.
      - Call `activityRepo.Create(ctx, &domain.ActivityEvent{UserID, Username, AnimeID, Type: "comment", Content: preview})`. NO dedup branch — every successful create emits one event. Activity-emit failure is non-fatal (log Errorw, return the saved comment anyway).
    - `UpdateComment(ctx, userID, commentID string, isAdmin bool, req *domain.UpdateCommentRequest) (*domain.Comment, error)`:
      - Same body validation as Create (trim + empty check + 2000 rune cap).
      - Fetch comment via `commentRepo.GetByID(ctx, commentID)`. Bubble NotFound.
      - Authorization: if `existing.UserID != userID && !isAdmin`, return `errors.Forbidden("not the comment owner")`.
      - Note per CONTEXT.md: admins do NOT edit other users' comments — but for parity with backend tooling, admin-edit is allowed by the service. Frontend enforces "admins don't see the pencil." Backend allows it for tooling parity.
      - Call `commentRepo.Update(ctx, commentID, body)`. Reload via `commentRepo.GetByID` and return.
    - `DeleteComment(ctx, userID, commentID string, isAdmin bool) error`:
      - Fetch comment; bubble NotFound.
      - Authorization: if `existing.UserID != userID && !isAdmin`, return `errors.Forbidden("not the comment owner")`.
      - Call `commentRepo.SoftDelete(ctx, commentID)`. Return its error.
    - `ListComments(ctx, animeID, cursor string, limit int) (*domain.CommentsListResponse, error)`:
      - Validate `1 <= limit <= 100`; default to 50 if 0; cap at 100.
      - Call `commentRepo.ListByAnime`. Build response with `Comments`, `NextCursor`, `HasMore: nextCursor != ""`.
    - `rateBucket.allow(userID, animeID)` — sliding-window: prune timestamps older than 1 hour from the (userID, animeID) bucket; if remaining count >= 10, return false; else append now + return true. Mutex-locked. Bucket map and mutex are instance-scoped — `NewCommentService` constructs fresh `entries: map[string][]time.Time{}`.
  </behavior>
  <read_first>
    - services/player/internal/service/comment.go (Wave 0 stub)
    - services/player/internal/service/comment_test.go (Wave 0 SKIP stubs)
    - services/player/internal/service/review.go (lines 30-102 — activity emit pattern; dedup branch which we strip)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Pattern 4 + Pattern 5; Code Example 3)
    - libs/errors/errors.go (NotImplemented, InvalidInput, Forbidden, RateLimited)
  </read_first>
  <action>
    Replace the Wave 0 stubs with the real implementations described in `<behavior>`. The `rateBucket` follows RESEARCH.md Pattern 5 verbatim (lines 482-512).

    Convert two test stubs from SKIP to PASS:

    `TestCommentService_RateLimit`:
    - Setup: setupTestDB; AutoMigrate(&domain.Comment{}, &domain.ActivityEvent{}). Construct `commentRepo`, `activityRepo`, `service := NewCommentService(commentRepo, activityRepo, logger.NewNop())`.
    - Loop: for i := 0; i < 10; i++ — call `service.CreateComment(ctx, "u1", "alice", "a1", &domain.CreateCommentRequest{Body: "hi"})`. Assert no error each time.
    - 11th call: assert err matches `errors.RateLimited()` (use `errors.Is` or check the error code field).
    - Verify isolation: call with a different userID ("u2", "bob") — assert succeeds (rate bucket is per-(user,anime), not global).
    - Verify isolation: call ("u1", "alice") on a different animeID — assert succeeds.

    `TestCommentService_EmitsActivity`:
    - Setup as above.
    - Call `service.CreateComment(...)` once with body "hello world".
    - Query `activityRepo` for events with `Type: "comment"` and `UserID: u1`. Assert exactly 1 row exists with `Content = "hello world"`.
    - Call `service.CreateComment(...)` a second time same user same anime, body "second comment".
    - Query again. Assert exactly 2 rows (no dedup; this is the divergence from reviews).
    - Test content-preview truncation: post a comment with body = 350 'a's. Query activity. Assert the row's `Content` has exactly 301 runes (300 'a's + the '…' rune) and ends with `…`.
  </action>
  <verify>
    <automated>cd services/player && go test ./internal/service/ -run 'TestCommentService_RateLimit|TestCommentService_EmitsActivity' -v -count=1</automated>
  </verify>
  <acceptance_criteria>
    - Both tests pass with `--- PASS`.
    - `grep -c 't.Skip' services/player/internal/service/comment_test.go` outputs `0`.
    - `grep -c 'errors.NotImplemented' services/player/internal/service/comment.go` outputs `0`.
    - `grep -c 'utf8.RuneCountInString' services/player/internal/service/comment.go` outputs ≥ 2 (one in CreateComment, one in UpdateComment).
    - `grep -c 'errors.RateLimited' services/player/internal/service/comment.go` outputs ≥ 1.
    - `grep -E 'Type:\s*"comment"' services/player/internal/service/comment.go` exits 0.
    - `cd services/player && go build ./...` exits 0.
  </acceptance_criteria>
  <done>CommentService enforces all validation rules, rate-limits per-user-anime correctly, and emits exactly one activity event per successful create with the correct content preview.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3.3: Implement CommentHandler with happy-path 201 + 400 / 403 / 429 failure paths</name>
  <files>services/player/internal/handler/comment.go, services/player/internal/handler/comment_test.go</files>
  <behavior>
    - `CreateComment(w, r)`: extract animeID via chi.URLParam; extract claims via authz.ClaimsFromContext (if missing → httputil.Unauthorized + return). Bind body via httputil.Bind into `domain.CreateCommentRequest`. Call `commentService.CreateComment(ctx, claims.UserID, claims.Username, animeID, &req)`. On error → httputil.Error (auto-maps to 400/429/500). On success → httputil.Created(w, comment).
    - `UpdateComment(w, r)`: extract animeID + commentID via chi.URLParam. Extract claims. Bind body into `domain.UpdateCommentRequest`. Call `commentService.UpdateComment(ctx, claims.UserID, commentID, authz.IsAdmin(ctx), &req)`. httputil.Error on failure; httputil.OK on success.
    - `DeleteComment(w, r)`: extract animeID + commentID via chi.URLParam. Extract claims. Call `commentService.DeleteComment(ctx, claims.UserID, commentID, authz.IsAdmin(ctx))`. httputil.Error on failure; httputil.NoContent on success.
    - `ListComments(w, r)`: extract animeID via chi.URLParam. Parse `cursor` and `limit` from `r.URL.Query()`. Parse limit as int; default 50 if missing or non-numeric. Cap at 100. Call `commentService.ListComments`. httputil.OK on success.
  </behavior>
  <read_first>
    - services/player/internal/handler/comment.go (Wave 0 stub)
    - services/player/internal/handler/comment_test.go (Wave 0 SKIP stubs)
    - services/player/internal/handler/review.go (full file — handler pattern, claims access, error mapping)
    - services/player/internal/handler/mal_import_test.go (lines 130-180 — claims-into-context injection helper for tests)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Code Example 2)
    - libs/authz/jwt.go (ClaimsFromContext, IsAdmin)
  </read_first>
  <action>
    Replace the Wave 0 stub bodies with real implementations described in `<behavior>`. Mirror `handler/review.go` for the auth + bind + service-call + error-map pattern.

    Convert three test stubs from SKIP to PASS:

    `TestCommentHandler_CreateComment_HappyPath`:
    - Setup: setupTestDB + AutoMigrate; construct repo, service, handler. Build a chi router or use `httptest.NewRecorder` + manual `r = mux.SetURLVars(r, map[string]string{"animeId": "a1"})` (chi's test helper).
    - Build request: POST /api/anime/a1/comments with body `{"body":"hello"}`, content-type application/json.
    - Inject claims into context (use the helper extracted to comment_test.go from mal_import_test.go pattern).
    - Call `handler.CreateComment(w, r)`. Assert status code 201. Decode body into `domain.Comment`; assert `Body=="hello"`, `UserID==<claims.UserID>`, `AnimeID=="a1"`, `Username==<claims.Username>`, `ID` non-empty.

    `TestCommentHandler_CreateComment_EmptyBody`:
    - Same setup. POST with body `{"body":"   "}` (whitespace only). Inject claims. Assert status code 400. Decode error response; assert error message contains `cannot be empty` (case-insensitive substring match).

    `TestCommentHandler_UpdateComment_NotOwner`:
    - Setup: seed one comment owned by user "alice". Inject claims for user "bob" (non-admin). PATCH /api/anime/a1/comments/<id> with `{"body":"hacked"}`. Assert status code 403. Verify the comment body in the DB is unchanged (still the original).
    - Repeat with bob as admin (Role: "admin"). Assert status code 200 (admin override works at the service layer — frontend hides the pencil for admins, but the backend allows it). The comment body is updated to "hacked".
  </action>
  <verify>
    <automated>cd services/player && go test ./internal/handler/ -run 'TestCommentHandler_CreateComment_HappyPath|TestCommentHandler_CreateComment_EmptyBody|TestCommentHandler_UpdateComment_NotOwner' -v -count=1</automated>
  </verify>
  <acceptance_criteria>
    - All three tests pass with `--- PASS`.
    - `grep -c 't.Skip' services/player/internal/handler/comment_test.go` outputs `0`.
    - `grep -c 'errors.NotImplemented' services/player/internal/handler/comment.go` outputs `0`.
    - `grep -c 'authz.ClaimsFromContext' services/player/internal/handler/comment.go` outputs ≥ 3 (one per protected method: Create/Update/Delete).
    - `grep -c 'authz.IsAdmin' services/player/internal/handler/comment.go` outputs ≥ 2 (Update + Delete).
    - `grep -c 'chi.URLParam' services/player/internal/handler/comment.go` outputs ≥ 4 (animeId in all 4 methods + commentId in Update/Delete).
    - `cd services/player && go build ./...` exits 0.
    - `cd services/player && go test ./...` exits 0 (full suite — including plan 02's tests if they're already merged).
  </acceptance_criteria>
  <done>All four comment endpoints have working handler implementations with correct status-code mapping; happy-path + empty-body + non-owner cases pass at the handler layer.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client → POST/PATCH/DELETE comments | Authenticated body with user-generated comment text |
| client → GET comments?cursor=... | Cursor parameter is opaque user-controlled input |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-1-V5 | Input validation | CommentService.CreateComment / UpdateComment | mitigate | utf8.RuneCountInString rejects > 2000-rune bodies (NOT bytes — would falsely reject Cyrillic / Japanese); trim + empty check rejects whitespace-only bodies. Unit-tested in TestCommentHandler_CreateComment_EmptyBody. |
| T-1-V5 (cursor) | Tampering | repo.CommentRepository.ListByAnime | mitigate | pagination.DecodeCursor validates base64 + JSON; GORM `Where("created_at < ? ...", ...)` parameterizes — no string interpolation; invalid cursor returns errors.InvalidInput. |
| T-1-V4 | Access control | CommentService.UpdateComment / DeleteComment | mitigate | Explicit owner-or-admin check: `existing.UserID != userID && !isAdmin` → errors.Forbidden. Unit-tested in TestCommentHandler_UpdateComment_NotOwner. |
| T-1-V11 | Business logic / DoS | CommentService.rateBucket | mitigate | 10/hour/user/anime sliding window; per-instance map (not global) so test isolation is preserved. Acceptable for v0.1 single-replica per CONTEXT.md. Unit-tested in TestCommentService_RateLimit. |
| Mass-assignment | Tampering / EoP | domain.CreateCommentRequest | mitigate | DTO omits `parent_id` entirely; httputil.Bind cannot populate fields not declared on the struct. Comments always have ParentID=nil in v0.1. |
| Stored XSS | Tampering | comment.Body display | accept | Plain text only per SPEC; Vue `{{ }}` interpolation auto-escapes in plan 06; backend stores as-is. |
| Soft-delete bypass | Tampering | gorm.DeletedAt filter | mitigate | All queries flow through CommentRepository; no handler issues raw SQL. gorm.DeletedAt auto-injects `WHERE deleted_at IS NULL`. |
</threat_model>

<verification>
- `cd services/player && go build ./...` exits 0
- `cd services/player && go test ./...` exits 0 — including all seven new comment-suite tests
- `grep -rE 'errors.NotImplemented' services/player/internal/{repo,service,handler}/comment*.go` finds nothing
- `grep -rE 't.Skip\("Wave 0 scaffold' services/player/internal/{repo,service,handler}/comment*_test.go` finds nothing
</verification>

<success_criteria>
SOCIAL-04 (CRUD + cursor + body limits + rate limit + soft delete) satisfied at the unit-test level. SOCIAL-05 (activity event per comment with truncated preview) satisfied at the unit-test level. Plan 04 will wire these handlers into the chi router and gateway proxy so they become live HTTP endpoints.
</success_criteria>

<output>
After completion, create `.planning/workstreams/social/phases/01-social-reviews-comments/01-03-SUMMARY.md` documenting the production CommentRepository / CommentService / CommentHandler surface, the rateBucket implementation choices (per-instance map, sliding 1-hour window), and the seven tests now passing. Note the wiring gap (main.go + router.go) that plan 04 closes.
</output>
