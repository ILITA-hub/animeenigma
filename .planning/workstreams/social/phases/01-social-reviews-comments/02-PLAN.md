---
phase: 1
workstream: social
plan: 2
type: execute
wave: 2
depends_on: [1]
files_modified:
  - services/player/internal/repo/review.go
  - services/player/internal/repo/list.go
  - services/player/internal/service/review.go
  - services/player/internal/handler/review.go
  - services/player/internal/domain/watch.go
  - services/player/cmd/player-api/main.go
autonomous: false
requirements:
  - SOCIAL-03
  - SOCIAL-NF-01

must_haves:
  truths:
    - "No Go source file in services/player/ contains the identifier `ReviewRepository` after this plan lands."
    - "No Go source file in services/player/ contains the identifier `domain.Review` after this plan lands."
    - "All six review endpoints (GET reviews, GET rating, GET reviews/me, POST reviews, DELETE reviews, POST ratings/batch) read/write `anime_list` exclusively."
    - "Each of the six endpoints returns a JSON shape byte-identical to its pre-migration shape on the seven canonical Review fields (id, user_id, anime_id, username, score, review_text, created_at) plus the existing `anime` preload. No extra anime_list fields (notes, status, episodes, tags, mal_id, is_rewatching, priority, started_at, completed_at, updated_at) leak into review responses."
    - "GET /api/anime/:id/reviews returns rows where `score > 0 OR review_text != ''` — so MAL-imported score=8 rows appear."
    - "POST /api/anime/:id/reviews on a user with existing list entry preserves status / episodes / notes / tags fields; only updates score + review_text + username."
    - "DELETE /api/anime/:id/reviews clears BOTH score and review_text on the row (the row stays; it just drops out of the public reviews filter)."
  artifacts:
    - path: "services/player/internal/repo/review.go"
      provides: "DELETED — file removed entirely"
      contains: "(file does not exist)"
    - path: "services/player/internal/repo/list.go"
      provides: "Five new methods on ListRepository — GetReviewsByAnime, GetUserReview, UpsertReview, ClearReview, GetAnimeRating, GetBatchAnimeRatings"
      contains: "func (r *ListRepository) GetReviewsByAnime"
    - path: "services/player/internal/service/review.go"
      provides: "ReviewService refactored — constructor takes (*ListRepository, *ActivityRepository, *logger.Logger); same six public method signatures; same activity-event dedup logic for reviews"
      contains: "NewReviewService(listRepo *repo.ListRepository"
    - path: "services/player/internal/handler/review.go"
      provides: "Unchanged method names; internal projection struct reviewResponse with exactly 7 fields; all responses converted via toReviewResponse(...)"
      contains: "reviewResponse"
    - path: "services/player/internal/domain/watch.go"
      provides: "Review struct DELETED — CreateReviewRequest kept (still consumed by handler)"
      contains: "type CreateReviewRequest struct"
    - path: "services/player/cmd/player-api/main.go"
      provides: "reviewRepo wiring removed; reviewService constructed from listRepo; AutoMigrate no longer references &domain.Review{}"
      contains: "service.NewReviewService(listRepo"
  key_links:
    - from: "services/player/internal/handler/review.go"
      to: "services/player/internal/repo/list.go"
      via: "ReviewService -> ListRepository (via service layer)"
      pattern: "ListRepository"
    - from: "services/player/internal/handler/review.go (reviewResponse struct)"
      to: "JSON wire shape contract"
      via: "handler projection — only 7 canonical fields exported"
      pattern: "reviewResponse"
    - from: "services/player/cmd/player-api/main.go"
      to: "ReviewService"
      via: "NewReviewService(listRepo, activityRepo, log)"
      pattern: "NewReviewService\\(listRepo"
---

<objective>
Refactor the reviews API so it reads and writes `anime_list` instead of the (now-dropped) `reviews` table, while keeping the wire shape of every review endpoint byte-identical to pre-migration. Delete `ReviewRepository` and `domain.Review`; project handler responses to a 7-field `reviewResponse` struct to prevent leaking the wider `AnimeListEntry` JSON shape.

Purpose: SOCIAL-03 (API contract preserved) + SOCIAL-NF-01 (frontend untouched for schema swap). Without the handler-local projection, every review endpoint would silently start emitting 12 extra fields and break consumers that rely on shape equality.

Output: zero behavioral change visible to the frontend; six endpoints functionally backed by the consolidated table; ReviewRepository / domain.Review gone from the codebase.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/social/phases/01-social-reviews-comments/01-SPEC.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-01-SUMMARY.md
@services/player/internal/repo/review.go
@services/player/internal/repo/list.go
@services/player/internal/service/review.go
@services/player/internal/handler/review.go
@services/player/internal/domain/watch.go
@services/player/cmd/player-api/main.go
@libs/errors/errors.go
@libs/httputil/response.go

<interfaces>
<!-- Public surface that MUST be preserved (frontend consumes these) -->

From frontend/web/src/api/client.ts (reviewApi):
- getAnimeReviews(animeId)         -> GET  /api/anime/:id/reviews    -> [{id, user_id, anime_id, username, score, review_text, created_at, anime?}]
- getAnimeRating(animeId)          -> GET  /api/anime/:id/rating     -> {anime_id, average_score, total_reviews}
- getMyReview(animeId)             -> GET  /api/anime/:id/reviews/me -> {id, user_id, anime_id, username, score, review_text, created_at, anime?} | null
- createReview(animeId, payload)   -> POST /api/anime/:id/reviews    -> {id, user_id, anime_id, username, score, review_text, created_at}
- deleteReview(animeId)            -> DELETE /api/anime/:id/reviews  -> 204 No Content
- getBatchAnimeRatings(animeIds)   -> POST /api/anime/ratings/batch  -> {<anime_id>: {anime_id, average_score, total_reviews}}

Frontend Review TypeScript interface (Anime.vue:816-824):
- id: string; user_id: string; anime_id: string; username: string; score: number; review_text: string; created_at: string; anime?: AnimeInfo

From services/player/internal/repo/list.go (existing methods to consult / extend — DO NOT break):
- Upsert(ctx, entry *domain.AnimeListEntry) error  (uses clause.OnConflict on user_id + anime_id)
- GetByUserAndAnime(ctx, userID, animeID string) (*domain.AnimeListEntry, error)
- GetByUser(...), GetByAnime(...)  (existing watchlist queries — DO NOT touch)
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 2.1: Extend ListRepository with review-shaped queries AND refactor ReviewService to consume them</name>
  <files>services/player/internal/repo/list.go, services/player/internal/service/review.go</files>
  <behavior>
    Repository methods (added to ListRepository):
    - `GetReviewsByAnime(ctx, animeID)` returns rows from `anime_list WHERE anime_id = ? AND (score > 0 OR review_text != '') AND deleted_at IS NULL` (note: anime_list has no DeletedAt in current schema — verify; if absent, omit that clause). Order: `created_at DESC`. Preloads Anime.
    - `GetUserReview(ctx, userID, animeID)` returns the single anime_list row for that pair, errors.NotFound if no row OR if both score=0 AND review_text=''. Preloads Anime.
    - `UpsertReview(ctx, userID, animeID, username string, score int, reviewText string)` — UPSERT into anime_list. If no row exists, create with `status='completed'`, the given score + review_text + username, leaving notes/tags/episodes empty. If row exists, UPDATE only score + review_text + username + updated_at (preserve status, episodes, notes, tags, etc.). Returns the resulting `*domain.AnimeListEntry`.
    - `ClearReview(ctx, userID, animeID)` — UPDATE anime_list SET score=0, review_text='', updated_at=NOW() WHERE user_id=? AND anime_id=?. Returns nil on no-match (idempotent).
    - `GetAnimeRating(ctx, animeID)` returns `(*domain.AnimeRating, error)` with average + count from `SELECT AVG(score), COUNT(*) FROM anime_list WHERE anime_id = ? AND score > 0`. Identical contract to the old `ReviewRepository.GetAnimeRating`.
    - `GetBatchAnimeRatings(ctx, animeIDs []string)` returns `map[string]*domain.AnimeRating` for the supplied anime IDs, querying anime_list grouped by anime_id with `WHERE anime_id IN (?) AND score > 0`. Identical contract to `ReviewRepository.GetBatchAnimeRatings`.

    Service refactor (services/player/internal/service/review.go):
    - `NewReviewService(listRepo *repo.ListRepository, activityRepo *repo.ActivityRepository, log *logger.Logger) *ReviewService` — note: removes the `reviewRepo` parameter entirely.
    - `CreateOrUpdateReview(ctx, userID, username string, req *domain.CreateReviewRequest) (*domain.AnimeListEntry, error)` — return type changes from `*domain.Review` to `*domain.AnimeListEntry`. Validates `score >= 0 && score <= 10` (matches existing rule). Calls `listRepo.UpsertReview(...)`. On success, emits activity event with per-day dedup logic IDENTICAL to current `service/review.go:50-86` (dedup is for reviews; comments use a different code path in plan 03). For the activity event, populate `OldValue`/`NewValue` from the prior score/review_text if available (parity with existing behavior).
    - `GetAnimeReviews(ctx, animeID)` returns `[]*domain.AnimeListEntry, error` (was `[]*domain.Review`) via `listRepo.GetReviewsByAnime`.
    - `GetUserReview(ctx, userID, animeID)` returns `*domain.AnimeListEntry, error` via `listRepo.GetUserReview`. errors.NotFound surface unchanged.
    - `GetAnimeRating(ctx, animeID)` returns `*domain.AnimeRating, error` via `listRepo.GetAnimeRating`.
    - `GetBatchAnimeRatings(ctx, animeIDs)` returns `map[string]*domain.AnimeRating, error` via `listRepo.GetBatchAnimeRatings`.
    - `DeleteReview(ctx, userID, animeID)` calls `listRepo.ClearReview`. Returns nil on no-row (idempotent matching current behavior).
  </behavior>
  <read_first>
    - services/player/internal/repo/list.go (full file — existing Upsert pattern, GetByUserAndAnime)
    - services/player/internal/repo/review.go (full file — methods being absorbed)
    - services/player/internal/service/review.go (full file — preserve all activity dedup logic verbatim)
    - services/player/internal/repo/activity.go (GetTodayByUserAnimeType signature for the dedup check)
    - services/player/internal/domain/watch.go (AnimeListEntry, AnimeRating shape)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Pitfall 2 — score-preservation note: live POST always sets the new score; CASE-WHEN is only for the migration)
  </read_first>
  <action>
    PART A — Add the six methods declared in `<behavior>` to `ListRepository` in `services/player/internal/repo/list.go`. Use GORM idioms throughout — `r.db.WithContext(ctx).Where(...).Find(...).Error`. For `UpsertReview` use `clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "anime_id"}}, DoUpdates: clause.Assignments(map[string]interface{}{ ... })}` per existing `Upsert` precedent (list.go:54-77). The DoUpdates map sets only `score`, `review_text`, `username`, `updated_at` — explicitly NOT `status`, `episodes`, `notes`, `tags`, etc. (preserves the existing watchlist row).

    PART B — Rewrite `services/player/internal/service/review.go` in place. The `ReviewService` struct now has: `listRepo *repo.ListRepository`, `activityRepo *repo.ActivityRepository`, `log *logger.Logger`. Remove `reviewRepo *repo.ReviewRepository` from the struct entirely. Every method body changes its repository target from `s.reviewRepo.*` to `s.listRepo.*`. Method signatures change return types as documented in `<behavior>` (from `*domain.Review` to `*domain.AnimeListEntry`). The activity-emission block (lines 50-86 of the original) stays line-for-line identical — only the field reads change (`existingReview.Score` → `existing.Score` where `existing *domain.AnimeListEntry`).

    PART C — Tests. Add repo tests in `services/player/internal/repo/list_review_test.go` (or append to existing repo test file if conventional — check `services/player/internal/repo/recs_test.go` for the in-memory-DB pattern):
    - `TestListRepo_GetReviewsByAnime_IncludesScoreOnlyRows` — seed two anime_list rows for the same anime: one with score=8 review_text='', one with score=0 review_text='great'; assert both appear in result.
    - `TestListRepo_UpsertReview_PreservesExistingWatchlistFields` — seed an anime_list row with status='watching', episodes=5, notes='foo'; call UpsertReview with score=9 reviewText='cool' username='bob'; reload row; assert status still 'watching', episodes still 5, notes still 'foo', score now 9, review_text now 'cool', username now 'bob'.
    - `TestListRepo_ClearReview_PreservesRow` — UpsertReview then ClearReview; reload row; assert row STILL EXISTS with score=0 review_text=''.
    - `TestListRepo_GetAnimeRating_ExcludesZeroScores` — seed 3 rows for anime X with scores 7, 0, 9; assert avg=8 (only the non-zero rows) and count=2.

    Add service tests in `services/player/internal/service/review_test.go` (or `review_refactor_test.go` if a `review_test.go` already exists — check):
    - `TestReviewService_CreateOrUpdateReview_EmitsActivityOnce` — create new review → assert 1 activity event with type='review'.
    - `TestReviewService_CreateOrUpdateReview_DedupsWithinSameDay` — create then update same day → assert still 1 activity event (dedup preserved).
    - `TestReviewService_DeleteReview_ClearsScoreAndText` — assert the anime_list row's score becomes 0 and review_text becomes '' but the row STILL EXISTS.

    Use the `setupTestDB` helper from `services/player/internal/repo/sync_test.go` for the SQLite fixture (per Wave 0 convention).

    Note: the SERVICE PACKAGE must build standalone (`cd services/player && go build ./internal/service/...` exits 0). The full `go build ./...` may fail at `main.go` until task 2.3 rewires it; that is expected.
  </action>
  <verify>
    <automated>cd services/player && go build ./internal/service/... && go build ./internal/repo/... && go test ./internal/repo/ -run 'TestListRepo_GetReviewsByAnime_IncludesScoreOnlyRows|TestListRepo_UpsertReview_PreservesExistingWatchlistFields|TestListRepo_ClearReview_PreservesRow|TestListRepo_GetAnimeRating_ExcludesZeroScores' -v -count=1 && go test ./internal/service/ -run 'TestReviewService' -v -count=1 && grep -c 'reviewRepo' services/player/internal/service/review.go</automated>
  </verify>
  <acceptance_criteria>
    - All four new repo tests pass (`--- PASS`).
    - All three new `TestReviewService_*` tests pass.
    - `grep -E 'func \(r \*ListRepository\) (GetReviewsByAnime|GetUserReview|UpsertReview|ClearReview|GetAnimeRating|GetBatchAnimeRatings)' services/player/internal/repo/list.go | wc -l` outputs `6`.
    - `grep -c 'reviewRepo' services/player/internal/service/review.go` outputs `0` (no leftover references to the old repo).
    - `grep -c 'listRepo' services/player/internal/service/review.go` outputs ≥ 6 (one per refactored method).
    - `cd services/player && go build ./internal/service/...` exits 0 (service package compiles independently).
    - `cd services/player && go build ./internal/repo/...` exits 0.
    - `cd services/player && go test ./internal/repo/...` exits 0 (no regressions in existing list/sync/recs tests).
    - The full `go build ./...` may fail on `cmd/player-api/main.go` until task 2.3 rewires it; this is expected.
  </acceptance_criteria>
  <done>ListRepository owns all the queries previously in ReviewRepository, all backed by the unified `anime_list` schema, all tested. ReviewService is fully ported to ListRepository while preserving its public surface and activity-event behavior. The service + repo packages compile independently; main.go still needs task 2.3 to compile end-to-end.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2.2: Add reviewResponse projection to handler/review.go to preserve wire shape</name>
  <files>services/player/internal/handler/review.go</files>
  <behavior>
    - `reviewResponse` is an unexported struct in `handler/review.go` with exactly 7 JSON-tagged fields: `id`, `user_id`, `anime_id`, `username`, `score`, `review_text`, `created_at`. Plus `anime *AnimeInfoResponse` (or `*domain.AnimeInfo` if its JSON tags don't leak unwanted fields — verify).
    - Helper `toReviewResponse(*domain.AnimeListEntry) reviewResponse` projects an entry into the wire shape. Helper `toReviewResponses([]*domain.AnimeListEntry) []reviewResponse` for lists.
    - All five review handler methods (`GetAnimeReviews`, `GetAnimeRating`, `GetUserReview`, `CreateOrUpdateReview`, `DeleteReview`) — plus the existing `GetBatchAnimeRatings` (already wire-shape-correct) — call the projection before writing JSON. `DeleteReview` continues to write 204 No Content (no body — projection not needed).
    - `GetAnimeReviews` returns `[]reviewResponse` (always an array, never null, even on empty result — match existing semantics).
    - `CreateOrUpdateReview` returns a single `reviewResponse`.
    - `GetUserReview` returns `reviewResponse` on hit; 404 via `httputil.Error(w, errors.NotFound(...))` on miss.
  </behavior>
  <read_first>
    - services/player/internal/handler/review.go (full file)
    - services/player/internal/domain/watch.go (AnimeListEntry json tags, AnimeInfo)
    - libs/httputil/response.go (OK, Created, NoContent, Error helpers)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Pitfall 1 — handler projection is the recommended mitigation)
  </read_first>
  <action>
    Edit `services/player/internal/handler/review.go`:
    1. Add the `reviewResponse` struct + helpers near the top of the file (after imports, before the `ReviewHandler` struct).
    2. Update each handler method to call the projection. The `Get*` methods change their `*domain.Review` interface accesses to `*domain.AnimeListEntry` (the service layer returns the new type after task 2.1), then immediately project to `reviewResponse`.
    3. `GetAnimeReviews` body: `entries, err := h.reviewService.GetAnimeReviews(ctx, animeID); if err != nil { httputil.Error(w, err); return }; httputil.OK(w, toReviewResponses(entries))`. Empty slice case: `if entries == nil { entries = []*domain.AnimeListEntry{} }` so JSON encodes as `[]` not `null`.

    Write a test `services/player/internal/handler/review_shape_test.go`:
    - `TestReviewHandler_GetAnimeReviews_ShapeIsExactly7Fields` — seed one anime_list row with all extra fields populated (status='watching', episodes=12, notes='abc', tags='action,drama'); call the handler; parse the JSON response body into a `map[string]json.RawMessage`; iterate the first element's keys; assert the set is exactly `{"id","user_id","anime_id","username","score","review_text","created_at","anime"}` (the `anime` key is the preload; the others are the 7 canonical scalars). Specifically assert keys `notes`, `status`, `episodes`, `tags`, `mal_id` do NOT appear.
    - `TestReviewHandler_CreateOrUpdateReview_ShapeIsExactly7Fields` — POST a review; assert the response body has exactly the same key set as above.

    These tests prove SOCIAL-NF-01 (frontend shape preservation).
  </action>
  <verify>
    <automated>cd services/player && go test ./internal/handler/ -run 'TestReviewHandler_.*ShapeIsExactly7Fields' -v -count=1</automated>
  </verify>
  <acceptance_criteria>
    - Both `*ShapeIsExactly7Fields` tests pass.
    - `grep -c 'reviewResponse' services/player/internal/handler/review.go` outputs ≥ 5 (struct decl + 4 method projections at minimum).
    - The test assertion explicitly checks that `notes`, `status`, `episodes`, `tags`, `mal_id` do NOT appear in the JSON keys. Verify: `grep -E '"notes"|"status"|"episodes"|"tags"|"mal_id"' services/player/internal/handler/review_shape_test.go` finds these strings inside the negative assertion (`NotContains`).
    - `cd services/player && go build ./internal/handler/...` exits 0 (service package compiles; main.go may still be broken pending task 2.3).
  </acceptance_criteria>
  <done>Wire shape is locked. Even if AnimeListEntry adds fields later, the reviews endpoint will continue to expose exactly 7 scalars + 1 preload.</done>
</task>

<task type="auto">
  <name>Task 2.3: Delete ReviewRepository + domain.Review; rewire main.go; restore build</name>
  <files>services/player/internal/repo/review.go, services/player/internal/domain/watch.go, services/player/cmd/player-api/main.go</files>
  <read_first>
    - services/player/internal/repo/review.go (entire file — confirm no surviving consumers outside the four files we control)
    - services/player/cmd/player-api/main.go (lines 50-260 — AutoMigrate block, reviewRepo wiring, NewReviewService call site)
    - services/player/internal/domain/watch.go (lines 105-119 — Review struct)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Pitfall 9 — refactor + migration must ship together)
  </read_first>
  <action>
    Three deletions + one rewire:

    1. `rm services/player/internal/repo/review.go` (delete the file entirely — it's been replaced by the ListRepository methods).

    2. In `services/player/internal/domain/watch.go`, delete the `Review` struct + its `TableName()` method (lines 105-119 in the original; current line numbers may differ after task 1.1's additions — locate by `grep -n 'type Review struct'`). DO NOT delete `CreateReviewRequest` — it's still the POST body shape.

    3. In `services/player/cmd/player-api/main.go`:
       - Inside the `db.AutoMigrate(...)` block, REMOVE the line `&domain.Review{},` (the reviews table no longer exists per plan 01's migration; AutoMigrate-ing the struct would attempt to recreate the table on every boot).
       - REMOVE the line `reviewRepo := repo.NewReviewRepository(db.DB)` (currently line ~219).
       - CHANGE the line `reviewService := service.NewReviewService(reviewRepo, listRepo, activityRepo, log)` to `reviewService := service.NewReviewService(listRepo, activityRepo, log)` (drop the `reviewRepo` arg).
       - The `reviewHandler := handler.NewReviewHandler(reviewService, log)` line stays unchanged.

    4. Run `cd services/player && go build ./...` to confirm the build is now end-to-end clean. If any test or non-test file still references `domain.Review` or `ReviewRepository`, the compiler will name the file; fix by updating that file's consumer (most likely an old test stub) to use `*domain.AnimeListEntry`.
  </action>
  <verify>
    <automated>cd services/player && go build ./... && ! test -f services/player/internal/repo/review.go && (grep -rE 'domain\.Review\b|ReviewRepository' services/player/ --include='*.go' | grep -v _test.go | grep -v 'CreateReviewRequest\|review_text' | wc -l | awk '{exit ($1==0)?0:1}')</automated>
  </verify>
  <acceptance_criteria>
    - `services/player/internal/repo/review.go` does not exist. Verify: `test ! -f services/player/internal/repo/review.go`.
    - `grep -rE 'domain\.Review\b' services/player/ --include='*.go' | grep -v 'CreateReviewRequest'` returns nothing. (`CreateReviewRequest` is grep-excluded because it's still the live POST DTO and shares the prefix.)
    - `grep -rE '\bReviewRepository\b' services/player/ --include='*.go'` returns nothing.
    - `grep -c '&domain.Review{}' services/player/cmd/player-api/main.go` outputs `0`.
    - `grep -c 'NewReviewService(listRepo' services/player/cmd/player-api/main.go` outputs `1`.
    - `cd services/player && go build ./...` exits 0.
    - `cd services/player && go vet ./...` exits 0.
    - `cd services/player && go test ./...` exits 0 (full package test suite — including the new tests from tasks 2.1-2.2).
  </acceptance_criteria>
  <done>The codebase is shape-equivalent to "reviews never existed as a separate table." Build + vet + tests are green end-to-end.</done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Checkpoint 2.4: Deploy + golden-file diff of six review endpoints</name>
  <what-built>
    All six review endpoints now backed by `anime_list` through the refactored ReviewService. Handler projection ensures the response shape is byte-identical (7 scalars + 1 preload).
  </what-built>
  <action>Manual verification gate — implementer pauses execution and the human runs the steps in &lt;how-to-verify&gt; below, then types the resume signal. No automated work in this task.</action>
  <how-to-verify>
    1. BEFORE deploy: capture pre-refactor JSON for a known anime + the `ui_audit_bot` user. From `scripts/capture-reviews-fixtures.sh` (created in Wave 0):
       `ANIME_ID=<id_of_anime_with_known_review_data> bash scripts/capture-reviews-fixtures.sh > tmp/reviews-pre.json`
       NOTE: this step had to be performed BEFORE plan 02 deployed. If skipped, fall back to comparing the handler shape test in task 2.2 (which is unit-test-equivalent to the golden file).
    2. Run `make redeploy-player`.
    3. AFTER deploy: rerun the capture script: `ANIME_ID=<same id> bash scripts/capture-reviews-fixtures.sh > tmp/reviews-post.json`.
    4. Diff: `diff tmp/reviews-pre.json tmp/reviews-post.json | head -50` — MUST be empty (no shape changes) OR show only `created_at` / `updated_at` reordering / floating-point representation drift (tolerable). If new keys appear like `notes`, `status`, `episodes` — STOP — task 2.2 projection is broken.
    5. Functional smoke: from a logged-in browser session on `/anime/<id>`, post a review → confirm it appears in the list → delete it → confirm it disappears. The frontend should run with ZERO code changes.
    6. Confirm MAL-imported visibility: pick a user who has an MAL-imported `score=8` row but no review_text on a given anime. Verify they appear in the `GET /api/anime/<id>/reviews` response with score=8 review_text=''.
  </how-to-verify>
  <resume-signal>Type "approved" if diff is empty (or only contains tolerable drift), live POST/DELETE work end-to-end, and the MAL-imported user appears. If the diff shows new keys leaking, describe the keys and the planner will produce a projection-fix revision.</resume-signal>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client → POST /api/anime/:id/reviews | Authenticated body; same JWT enforcement as before. |
| handler → service → repo | Internal — typed through Go structs. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-1-V13 | API & Web Service | reviewResponse projection | mitigate | Handler-local struct projects exactly 7 fields; AnimeListEntry's other fields (notes, status, etc.) cannot leak via the public reviews API. Unit-tested in TestReviewHandler_*ShapeIsExactly7Fields. |
| T-1-V11 | Business logic | UpsertReview preserves watchlist fields | mitigate | `clause.OnConflict.DoUpdates` only assigns score/review_text/username/updated_at; status/episodes/notes/tags/etc. unaffected. Unit-tested in TestListRepo_UpsertReview_PreservesExistingWatchlistFields. |
| T-1-V4 | Access control | POST/DELETE reviews | accept | Existing JWT middleware unchanged; no auth surface modification in this plan. |
</threat_model>

<verification>
- `cd services/player && go build ./...` exits 0
- `cd services/player && go test ./...` exits 0 (all new + existing tests pass)
- `grep -rE 'domain\.Review\b|ReviewRepository' services/player/ --include='*.go' | grep -v CreateReviewRequest` returns nothing
- `services/player/internal/repo/review.go` does not exist
- Post-deploy golden-file diff of six review endpoints is empty (or only tolerable drift)
</verification>

<success_criteria>
SOCIAL-03 satisfied: all six review endpoints work, frontend unchanged, no `domain.Review` / `ReviewRepository` residue. SOCIAL-NF-01 satisfied: shape preservation enforced both at unit-test level (TestReviewHandler_*ShapeIsExactly7Fields) and at integration level (golden-file diff).
</success_criteria>

<output>
After completion, create `.planning/workstreams/social/phases/01-social-reviews-comments/01-02-SUMMARY.md` documenting: the deleted artifacts (file path + struct names), the new ListRepository method surface, the reviewResponse projection contract, and the golden-file diff outcome from checkpoint 2.4.
</output>
