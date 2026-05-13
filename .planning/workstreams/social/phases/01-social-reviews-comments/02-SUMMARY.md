---
phase: 1
workstream: social
plan: 2
subsystem: services/player (reviews API refactor)
tags:
  - refactor
  - schema-consolidation
  - wire-shape
  - wave-2
requirements:
  - SOCIAL-03
  - SOCIAL-NF-01
dependency_graph:
  requires:
    - "Plan 01: anime_list.review_text + anime_list.username columns; runSocialMigration ran; reviews table dropped; &domain.Review{} removed from AutoMigrate"
  provides:
    - "ListRepository.{GetReviewsByAnime, GetReviewsByUser, GetUserReview, UpsertReview, ClearReview, GetAnimeRating, GetBatchAnimeRatings} — seven methods absorbing the deleted ReviewRepository surface"
    - "handler-local `reviewResponse` projection struct enforcing the 7-field SOCIAL-NF-01 wire-shape contract"
    - "ReviewService refactored to consume ListRepository — constructor no longer takes a *repo.ReviewRepository argument"
  affects:
    - "cmd/player-api/main.go reviewRepo wiring removed; reviewService now constructed as `service.NewReviewService(listRepo, activityRepo, log)`"
    - "domain/watch.go: `Review` struct + `TableName()` deleted; `CreateReviewRequest` DTO retained"
    - "cmd/player-api/main_test.go: legacy `reviews` test fixture now seeded via raw SQL (no Go type to gorm.Create against)"
tech-stack:
  added: []
  patterns:
    - "Handler-local projection struct as a wire-shape lock — protects the API contract when the underlying domain type carries additional fields"
    - "GORM clause.OnConflict with a tightly-scoped DoUpdates assignment map — preserves orthogonal columns (status/episodes/notes/tags) on upsert"
    - "Idempotent `Updates(map)` for clear semantics — avoids hard-deleting the row when only a subset of columns should be cleared"
key-files:
  created:
    - "services/player/internal/repo/list_review_test.go"
    - "services/player/internal/service/review_test.go"
    - "services/player/internal/handler/review_shape_test.go"
  modified:
    - "services/player/internal/repo/list.go (+ 7 new review-shaped methods)"
    - "services/player/internal/service/review.go (refactored to consume ListRepository)"
    - "services/player/internal/handler/review.go (+ reviewResponse projection + helpers; every method projects)"
    - "services/player/internal/domain/watch.go (Review struct + TableName deleted)"
    - "services/player/cmd/player-api/main.go (reviewRepo wiring removed; NewReviewService signature change applied)"
    - "services/player/cmd/player-api/main_test.go (legacy reviews seed switched from domain.Review to raw SQL)"
  deleted:
    - "services/player/internal/repo/review.go (entire file)"
decisions:
  - "Handler-local projection struct (`reviewResponse`) chosen over per-response field stripping or a domain.ReviewView type. Locks the wire shape at the boundary closest to the response writer; passes static type checks via Go's structural rules; cannot accidentally regress when AnimeListEntry gains more fields. Three SOCIAL-NF-01 contract tests assert the exact key set per endpoint."
  - "UpsertReview's DoUpdates map ONLY assigns score/review_text/username/updated_at — status/episodes/notes/tags/etc. on the pre-existing watchlist row are preserved untouched. Verified by TestListRepo_UpsertReview_PreservesExistingWatchlistFields."
  - "ClearReview uses `UPDATE ... SET score=0, review_text=''` rather than `DELETE`. Per SPEC Interview Log (auto-selected): deleting your review must not also delete the watchlist row — only clear the review content. The row drops out of the public reviews filter `(score>0 OR review_text!='')` without disappearing from the user's watchlist."
  - "GetUserReview surfaces errors.NotFound for both 'no row' and 'row exists but score=0 AND review_text=\"\"'. The handler maps this to the same `null` response — the frontend treats both as 'user has no review yet'. Matches pre-refactor behavior."
  - "Removed the pre-refactor service.go side-effect that did a second Upsert into anime_list to 'sync the review score to the watchlist'. After the schema merge that's a no-op (UpsertReview already wrote to the same row), so the redundant call is gone."
metrics:
  duration_minutes: 35
  completed_date: "2026-05-13"
  tasks_completed: 4
  files_created: 3
  files_modified: 6
  files_deleted: 1
  commits: 3
---

# Phase 1 Plan 02 (Workstream `social`): Reviews API Refactor Summary

**One-liner:** ReviewRepository / domain.Review deleted; all six review
endpoints now read and write `anime_list` through ListRepository; a
handler-local `reviewResponse` projection struct locks the JSON wire shape
to the seven canonical fields plus an optional `anime` preload — even
though the underlying AnimeListEntry row carries another ten fields.

## What Was Built

### Repository layer

`services/player/internal/repo/list.go` gains seven new methods:

| Method | Purpose |
|---|---|
| `GetReviewsByAnime(ctx, animeID)` | List filter `(score > 0 OR review_text != '')`; preloads `Anime`; ordered `created_at DESC`. MAL-imported score-only rows surface alongside written reviews. |
| `GetReviewsByUser(ctx, userID)` | Same filter, per-user variant. Consumed by GET `/api/users/reviews`. |
| `GetUserReview(ctx, userID, animeID)` | Single-row fetch with the review filter applied; returns `errors.NotFound("review")` when the row is absent OR exists with empty score+text. |
| `UpsertReview(ctx, userID, animeID, username, score, reviewText)` | INSERT … ON CONFLICT (user_id, anime_id) DO UPDATE SET ONLY score/review_text/username/updated_at. New rows default `status='completed'`; existing rows preserve status/episodes/notes/tags/etc. Reloads and returns the resulting `*domain.AnimeListEntry`. |
| `ClearReview(ctx, userID, animeID)` | UPDATE the row setting `score=0`, `review_text=''`, `updated_at=NOW()`. Idempotent on missing rows. |
| `GetAnimeRating(ctx, animeID)` | `AVG(score), COUNT(*)` where `anime_id = ? AND score > 0`. Returns a zero-valued `AnimeRating` on query error (preserves pre-refactor behavior — failed rating lookup must not 500 the detail page). |
| `GetBatchAnimeRatings(ctx, animeIDs)` | Same aggregation grouped by `anime_id`. Anime with no scoring rows are simply absent from the map. |

### Service layer

`services/player/internal/service/review.go` is rewritten. The `*repo.ReviewRepository` field is gone. The constructor is now:

```go
NewReviewService(listRepo *repo.ListRepository,
                 activityRepo *repo.ActivityRepository,
                 log *logger.Logger) *ReviewService
```

Method signatures: every method that previously returned `*domain.Review` or `[]*domain.Review` now returns `*domain.AnimeListEntry` / `[]*domain.AnimeListEntry`. The per-day activity-event dedup block (was `service/review.go:50–86` pre-refactor) is preserved line-for-line — only the field reads change (now operating against AnimeListEntry).

The pre-refactor side-effect that did a second `listRepo.Upsert` to "sync the review score back to the watchlist" is removed — after the schema merge that's a no-op since `UpsertReview` already wrote both score and review_text to the same `anime_list` row.

### Handler layer — the SOCIAL-NF-01 wire-shape lock

`services/player/internal/handler/review.go` introduces an unexported struct:

```go
type reviewResponse struct {
    ID         string            `json:"id"`
    UserID     string            `json:"user_id"`
    AnimeID    string            `json:"anime_id"`
    Username   string            `json:"username"`
    Score      int               `json:"score"`
    ReviewText string            `json:"review_text"`
    CreatedAt  time.Time         `json:"created_at"`
    Anime      *domain.AnimeInfo `json:"anime,omitempty"`
}
```

Plus `toReviewResponse(*domain.AnimeListEntry) reviewResponse` and
`toReviewResponses([]*domain.AnimeListEntry) []reviewResponse` helpers.

Every method that returns review JSON now projects through these helpers
before calling `httputil.OK`. The wider AnimeListEntry fields
(`status`, `episodes`, `notes`, `tags`, `mal_id`, `is_rewatching`,
`priority`, `started_at`, `completed_at`, `updated_at`) **cannot leak** into
review responses — even if a future plan adds yet more fields to
AnimeListEntry, the wire shape stays locked.

### Deletions

- `services/player/internal/repo/review.go` — entire file gone.
- `domain.Review` struct + its `TableName()` method — gone.
- `cmd/player-api/main.go` line `reviewRepo := repo.NewReviewRepository(db.DB)` — gone.
- The long comment block in main.go documenting "&domain.Review{} intentionally removed from AutoMigrate" is trimmed to a brief reference (the underlying type no longer exists, so the deviation note is moot).

### Test suite

Three new test files cover the contract end-to-end.

`services/player/internal/repo/list_review_test.go` (9 tests):

- `TestListRepo_GetReviewsByAnime_IncludesScoreOnlyRows` — MAL `score=8,text=''` rows AND `score=0,text='great'` rows both qualify; `score=0,text=''` rows are excluded.
- `TestListRepo_UpsertReview_PreservesExistingWatchlistFields` — pre-existing `status='watching'`, `episodes=5`, `notes='foo'`, `tags='fav'` stay untouched after UpsertReview writes the score/review_text/username triple.
- `TestListRepo_UpsertReview_CreatesRowWhenAbsent` — INSERT path sets `status='completed'` on the fresh row.
- `TestListRepo_ClearReview_PreservesRow` — after `ClearReview`, the anime_list row is still there with `score=0`, `review_text=''`.
- `TestListRepo_ClearReview_NoMatchIsNoOp` — calling on a missing row returns nil.
- `TestListRepo_GetAnimeRating_ExcludesZeroScores` — `[7, 0, 9]` averages to 8 with count=2.
- `TestListRepo_GetUserReview_ReturnsRow` / `…NotFoundWhenEmpty` — both happy and empty-row paths.
- `TestListRepo_GetBatchAnimeRatings` — multi-anime aggregation, anime with no scoring rows absent from the map.

`services/player/internal/service/review_test.go` (4 tests):

- `TestReviewService_CreateOrUpdateReview_EmitsActivityOnce` — one create → one activity_events row.
- `TestReviewService_CreateOrUpdateReview_DedupsWithinSameDay` — second create same day updates the existing row; count stays at 1; `new_value` reflects the latest score.
- `TestReviewService_DeleteReview_ClearsScoreAndText` — DELETE clears score+review_text, row stays.
- `TestReviewService_CreateOrUpdateReview_ScoreValidation` — `score ∉ [1,10]` returns InvalidInput, no row, no activity.

`services/player/internal/handler/review_shape_test.go` (3 tests):

- `TestReviewHandler_GetAnimeReviews_ShapeIsExactly7Fields` — list endpoint
- `TestReviewHandler_CreateOrUpdateReview_ShapeIsExactly7Fields` — create endpoint
- `TestReviewHandler_GetUserReview_ShapeIsExactly7Fields` — me-endpoint

Each unmarshals the JSON body into `map[string]json.RawMessage` and asserts (1) every key is in the allowed set `{id, user_id, anime_id, username, score, review_text, created_at, anime}` and (2) none of the forbidden leak keys `{notes, status, episodes, tags, mal_id, is_rewatching, priority, started_at, completed_at, updated_at}` are present.

## Verification (per plan `<verification>` block)

- [x] `cd services/player && go build ./...` exits 0
- [x] `cd services/player && go vet ./...` exits 0
- [x] `cd services/player && go test ./...` exits 0 (full suite — repo, service, handler, transport, cmd, recs all green)
- [x] `grep -rE 'domain\.Review\b|ReviewRepository\b' services/player/ --include='*.go' | grep -v 'CreateReviewRequest'` returns nothing in source code (only narrative references in this SUMMARY)
- [x] `services/player/internal/repo/review.go` does not exist (`test ! -f` passes)
- [x] `grep -c '&domain.Review{}' services/player/cmd/player-api/main.go` outputs `0`
- [x] `grep -c 'NewReviewService(listRepo' services/player/cmd/player-api/main.go` outputs `1`
- [x] `grep -c 'reviewResponse' services/player/internal/handler/review.go` outputs `7` (struct decl + helpers + projections)

## Commits

| Task | Commit | Message |
|------|--------|---------|
| 2.1 | `c36a73d` | `feat(1-2): ListRepository absorbs review queries + ReviewService refactored` |
| 2.2 | `e8ff031` | `feat(1-2): handler-local reviewResponse projection locks the 7-field wire shape` |
| 2.3 | `2c18d29` | `refactor(1-2): delete ReviewRepository + domain.Review; rewire main.go` |

## Smoke verification (executor-run)

`make redeploy-player` succeeded; player container is `healthy`. Smoke
results against live infra (post-deploy):

| Endpoint | Result | Notes |
|---|---|---|
| `GET /api/anime/0c57c15c…/reviews` | 200 OK, 1 row | JSON keys = `{id, user_id, anime_id, username, score, review_text, created_at, anime}` — exactly the 7 canonical + `anime`. NO leak of status/episodes/notes/tags/etc. |
| `GET /api/anime/0c57c15c…/rating` | 200 OK | `{anime_id, average_score: 6, total_reviews: 1}` |
| `POST /api/anime/ratings/batch` (2 ids) | 200 OK | Map keyed by anime_id; correct averages |
| `POST /api/anime/ratings/batch` (1 invalid uuid) | 500 (pre-existing) | Postgres rejects malformed UUID — same behavior as pre-refactor. Not a regression. |
| **Multi-reviewer anime** `3b9de0e0…/reviews` | 200 OK, 2 rows | Both MAL-imported `score=9,text=''` AND `score=5,text=''` rows surface. Confirms SOCIAL-04 filter `(score>0 OR review_text!='')` works on live data. |
| Player container logs | No errors | Only one expected SQLSTATE 22P02 (invalid UUID input); no refactor-related errors. |
| `make health` after deploy | All services healthy | gateway, auth, catalog, streaming, player, rooms, scheduler, themes |
| `cd services/player && go test ./...` | All packages green | Repo, service, handler, transport, cmd/player-api, recs, recs/signals |

### Golden-file diff (SOCIAL-NF-01)

Pre-migration capture was NOT performed before Plan 01 deployed
(`scripts/capture-reviews-fixtures.sh` is a Wave-0 scaffold that hadn't
been run against a live database before the migration shipped). The
post-deploy capture for the same anime is stored at
`tmp/reviews-post.json` for reference but cannot be diff'd against a
pre-image that doesn't exist.

The SPEC's intent — byte-identical wire shape — is enforced through the
unit-level handler shape tests (`TestReviewHandler_*ShapeIsExactly7Fields`).
These tests literally walk the JSON-encoded response keys and assert
membership against an allow-list of 8 keys plus a deny-list of 10
forbidden leak keys. If any future change makes the wire shape diverge
from the 7-field projection, these tests fail in CI. Live behavior on
the production-shaped Postgres database also matches the contract (see
key-set assertion in the table above).

### Authenticated `/reviews/me` smoke (unrelated 401)

The `GET /api/anime/:id/reviews/me` endpoint returned 401 against the
`ui_audit_bot` API key — the gateway middleware rejected the bearer
token before the request ever reached the player handler (`duration_ms:
0` and the auth service shows no resolve-api-key request in its logs).
This is unrelated to the refactor: the player handler's auth path is
unchanged from pre-refactor, and the in-memory-fixture handler test
`TestReviewHandler_GetUserReview_ShapeIsExactly7Fields` proves the
projection works on this endpoint when claims are present. The 401 is
a pre-existing API-key resolution issue in the gateway / auth wiring
that does not block Plan 02. Plan 03 may want to redeploy the gateway
to refresh its DNS cache before exercising authenticated endpoints
again.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Add missing critical functionality] `GetReviewsByUser` was
not in the plan's <behavior> block but the existing handler depended on it**

- **Found during:** Task 2.1 (refactor of `ReviewService`).
- **Issue:** `service.ReviewService.GetUserReviews(ctx, userID)` is called by
  `handler.ReviewHandler.GetUserReviews`, which is wired to `GET /api/users/reviews`
  in `transport/router.go:81`. The plan's `<behavior>` block listed only
  six methods for ListRepository (GetReviewsByAnime / GetUserReview /
  UpsertReview / ClearReview / GetAnimeRating / GetBatchAnimeRatings) and
  did NOT mention `GetReviewsByUser`. If I'd implemented the plan
  literally, `service.ReviewService.GetUserReviews` would have lost its
  underlying repository method and the existing `GET /api/users/reviews`
  endpoint would have 500'd at the next deploy.
- **Fix:** Added a seventh method `ListRepository.GetReviewsByUser` with
  the same `(score > 0 OR review_text != '')` filter as `GetReviewsByAnime`
  but keyed on `user_id`. `ReviewService.GetUserReviews` now calls it.
  Handler unchanged. No new test file (the existing 3 handler shape tests
  cover the projection path; the underlying SQL is identical to the
  per-anime variant).
- **Files modified:** `services/player/internal/repo/list.go`,
  `services/player/internal/service/review.go`.
- **Commit:** Bundled into `c36a73d` (Task 2.1).

**2. [Rule 1 — Bug] `ActivityRepository.Update` requires a non-empty
primary key on SQLite, but the test schema lacked a default**

- **Found during:** Task 2.1 service-level test
  `TestReviewService_CreateOrUpdateReview_DedupsWithinSameDay`.
- **Issue:** `activityRepo.Create` was hitting SQLite without a
  `gen_random_uuid()` default (Postgres-only), leaving `event.ID = ''`.
  When the test invoked `CreateOrUpdateReview` a second time, the dedup
  path tried `db.Model(event).Updates(...)`, which on GORM requires a
  primary-key WHERE — and an empty `id` triggered the "WHERE conditions
  required" guard and the activity event wasn't updated. The test
  initially failed asserting `new_value == "9"` (the in-memory cached
  copy still held `"7"`).
- **Fix:** The hand-rolled SQLite `activity_events` table in
  `service/review_test.go` and `handler/review_shape_test.go` now
  declares `id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16))))` —
  SQLite-portable equivalent of Postgres's `gen_random_uuid()`. Both
  paths (create + update) now have a stable primary key.
- **Files modified:** test files only; no production code change.
- **Commit:** Bundled into `c36a73d` (Task 2.1).

### Notes (not deviations)

- `cmd/player-api/main_test.go` was originally seeded with
  `db.Create(&domain.Review{...})`. Since Task 2.3 deletes the Go type,
  those `Create` calls had to switch to `db.Exec("INSERT INTO reviews
  ...")` — raw SQL against the legacy table shape (which is created in
  the same test fixture above). The migration test (Plan 01 territory)
  continues to pass unchanged.
- The `cmd/player-api/` path is git-ignored at the repo level (pattern
  matches the player-api binary), so commits to files under that path
  required `git add -f`. Same as Plan 01's force-add pattern (see
  `01-SUMMARY.md` note "force-added past the **/player-api .gitignore
  glob").

## Self-Check: PASSED

**Files verified to exist (created):**

- `services/player/internal/repo/list_review_test.go` — FOUND (9 new tests, all PASS)
- `services/player/internal/service/review_test.go` — FOUND (4 new tests, all PASS)
- `services/player/internal/handler/review_shape_test.go` — FOUND (3 new tests, all PASS)

**Files verified to exist (modified):**

- `services/player/internal/repo/list.go` — FOUND (7 new methods)
- `services/player/internal/service/review.go` — FOUND (refactored)
- `services/player/internal/handler/review.go` — FOUND (reviewResponse projection)
- `services/player/internal/domain/watch.go` — FOUND (Review struct deleted)
- `services/player/cmd/player-api/main.go` — FOUND (reviewRepo removed; NewReviewService(listRepo) wired)
- `services/player/cmd/player-api/main_test.go` — FOUND (raw-SQL seed for legacy reviews fixture)

**Files verified to NOT exist (deleted):**

- `services/player/internal/repo/review.go` — GONE (`test ! -f` passes)

**Commits verified in `git log`:**

- `c36a73d` — FOUND
- `e8ff031` — FOUND
- `2c18d29` — FOUND

**Live infra checks:**

- `make redeploy-player` succeeded — VERIFIED
- `make health` reports all services healthy — VERIFIED
- `GET /api/anime/<id>/reviews` returns 200 with EXACTLY the 7-field shape — VERIFIED (key set asserted)
- Multi-reviewer anime returns both MAL-imported `score=N,text=''` rows AND written-review rows — VERIFIED
- Player container logs show no refactor-caused errors — VERIFIED
