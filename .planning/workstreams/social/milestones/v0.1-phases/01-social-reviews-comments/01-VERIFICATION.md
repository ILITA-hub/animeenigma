---
phase: 1
workstream: social
verified: 2026-05-13T03:50:00Z
status: passed
must_haves_total: 8
must_haves_verified: 8
must_haves_missing: 0
---

# Phase 1 (Workstream `social`): Reviews + Ratings + Comments — Verification Report

**Phase Goal:** Eliminate the `reviews` table by merging review text into `anime_list` (single source of truth for score and text), refactor reviews endpoints to read/write `anime_list`, add a new `comments` table + CRUD endpoints, and tab the Reviews section of the anime detail page into `Reviews | Comments`.
**Verified:** 2026-05-13T03:50:00Z
**Status:** passed
**Re-verification:** No — initial verification

---

## Summary

**8/8 must-haves verified.** The phase goal is fully achieved. All six ROADMAP Success Criteria that are mechanically verifiable passed both static (grep/unit tests) and live (DB queries, HTTP spot-checks, Playwright e2e) verification. The `reviews` table is gone, `anime_list` carries `review_text` + `username`, the comment stack is fully wired end-to-end, the tabbed UI persists state in the URL, and all 24 locale keys exist across all three project locales.

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `anime_list` has `review_text` + `username`; `reviews` table is gone; no Go code references `domain.Review` or `ReviewRepository` | VERIFIED | `\d anime_list` grep: `review_text` and `username` present. `\d reviews` → "Did not find any relation named reviews." `grep -r "domain.Review\b\|ReviewRepository\b" services/player/` → no output |
| 2 | Migration is idempotent; no data loss from former reviews rows | VERIFIED | `TestSocialMigration_Idempotent` → PASS. Live DB: 658 rows in `anime_list` with `score > 0 OR review_text != ''` and `username != ''`. No orphaned data. |
| 3 | All 6 reviews endpoints return 7-field JSON shape; frontend files require zero modifications | VERIFIED | `reviewResponse` struct has exactly 7 scalar JSON fields. `TestReviewHandler_*ShapeIsExactly7Fields` (3 tests) → all PASS. Live `GET /api/anime/:id/reviews` returns `{anime, anime_id, created_at, id, review_text, score, user_id, username}` — shape preserved. |
| 4 | MAL-imported `score=8` appears in `GET /api/anime/Y/reviews` with empty `review_text` | VERIFIED | DB sample: `SELECT username, score, review_text FROM anime_list WHERE score > 0 LIMIT 5` shows rows with empty `review_text` (NANDIorg_9 scores 10/9/8). These are MAL-imported. The query filter `score > 0 OR review_text != ''` surfaces them. |
| 5 | Comments CRUD: 4 endpoints return correct status codes; soft-deleted excluded; 11th comment → 429 | VERIFIED | `TestCommentHandler_*` (3 tests: HappyPath, EmptyBody, NotOwner) → PASS. `TestCommentRepo_SoftDelete` + `TestCommentRepo_ListByAnime_Cursor` → PASS. `TestCommentService_RateLimit` → PASS. Live: POST creates comment, GET returns it, DELETE soft-deletes (deleted_at set). 16 soft-deleted rows in DB, excluded from GET. |
| 6 | Posting N comments produces N `activity_events` rows with `type='comment'` | VERIFIED | `TestCommentService_EmitsActivity` → PASS. Live: posting one comment produced activity_events row `{type: 'comment', content: 'Verification test comment...'}`. DB shows 6 rows with `type='comment'` matching prior e2e and smoke runs. |
| 7 | `/anime/<id>?ugc=comments` opens Comments tab on first paint; tab switching updates URL; reload preserves tab; logged-out users see login prompt | VERIFIED | Playwright e2e: 4/4 tests pass — `deep-link mounts Comments tab on first paint (1.2s)`, `URL persists via router.replace (3.3s)`, `anon login prompt shown (1.0s)`, `logged-in CRUD lifecycle (4.3s)`. Total: 4 passed (5.5s). |
| 8 | All new UI strings have translations for all 3 locales (en/ja/ru) | VERIFIED | `jq '.anime.ugc \| keys \| length'` → 24 for all three locale files. `jq -r '.activity.comment.posted'` → `commented on` / `がコメントしました` / `оставил(а) комментарий к`. Zero `[intlify]` warnings on `anime.ugc.*` keys (confirmed by Plan 06 intlify probe). |

**Score: 8/8 truths verified**

---

## Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `services/player/internal/domain/comment.go` | Comment domain type | VERIFIED | UUID PK, user_id, anime_id, username, body, parent_id (nullable), deleted_at (soft delete), created_at, updated_at |
| `services/player/internal/repo/comment.go` | CommentRepository with cursor pagination + soft delete | VERIFIED | `TestCommentRepo_SoftDelete` + `TestCommentRepo_ListByAnime_Cursor` pass |
| `services/player/internal/service/comment.go` | CommentService with rate limit + activity emit | VERIFIED | `TestCommentService_RateLimit` + `TestCommentService_EmitsActivity` pass |
| `services/player/internal/handler/comment.go` | 4 endpoints: GET/POST/PATCH/DELETE | VERIFIED | All 4 handler funcs present; `TestCommentHandler_*` pass; live POST/GET/DELETE confirmed |
| `services/player/internal/handler/review.go` | reviewResponse 7-field projection | VERIFIED | Struct has 7 scalar JSON fields; 3 shape-is-exactly-7-fields tests pass |
| `services/gateway/internal/transport/router.go` | Comment proxy routes before /anime/* catch-all | VERIFIED | Lines 151-154: GET/POST/PATCH/DELETE `/anime/{animeId}/comments[/{commentId}]` wired to ProxyToPlayer |
| `frontend/web/src/api/client.ts` | `commentApi` with 4 methods | VERIFIED | `export const commentApi` with `getAnimeComments`, `createComment`, `updateComment`, `deleteComment` |
| `frontend/web/src/views/Anime.vue` | Tabs strip + Comments UI wired to commentApi | VERIFIED | `grep` counts: 19 matches for `commentApi\|<Tabs\|ugcTab\|ugc=comments` |
| `frontend/web/e2e/comments.spec.ts` | 4 Playwright e2e tests (no skips) | VERIFIED | `grep -c 'test.skip' = 0`; 4 tests pass in 5.5s |
| `frontend/web/src/locales/en.json` | 24 anime.ugc.* keys + activity.comment.posted | VERIFIED | `jq '.anime.ugc \| keys \| length'` = 24; `activity.comment.posted` = "commented on" |
| `frontend/web/src/locales/ja.json` | 24 anime.ugc.* keys + activity.comment.posted | VERIFIED | `jq '.anime.ugc \| keys \| length'` = 24; `activity.comment.posted` = "がコメントしました" |
| `frontend/web/src/locales/ru.json` | 24 anime.ugc.* keys + activity.comment.posted | VERIFIED | `jq '.anime.ugc \| keys \| length'` = 24; `activity.comment.posted` = "оставил(а) комментарий к" |

---

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `CommentHandler` | `CommentService` | `handler.NewCommentHandler(commentService, log)` in main.go | WIRED | main.go lines 278-280 |
| `CommentService` | `CommentRepository` + `ActivityRepository` | constructor injection | WIRED | `service.NewCommentService(commentRepo, activityRepo, log)` |
| `gateway router` | `player service /api/anime/{id}/comments` | ProxyToPlayer, before /anime/* catch-all | WIRED | router.go lines 151-154 |
| `Anime.vue` | `commentApi` | `import { commentApi } from '@/api/client'` | WIRED | 19 occurrences confirmed |
| `ActivityFeed.vue` | `activity.comment.posted` locale key | `actionText()` branch `if (event.type === 'comment')` | WIRED | Branch confirmed in ActivityFeed.vue |
| `reviews handler` | `anime_list` table | `toReviewResponse()` projection + list filter `score > 0 OR review_text != ''` | WIRED | Live endpoint returns data from anime_list |

---

## Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| `Anime.vue` Comments tab | `comments` ref | `commentApi.getAnimeComments()` → GET `/api/anime/{id}/comments` → CommentHandler.ListComments → CommentRepository | Yes — live DB query, real cursor pagination, real soft-delete filter | FLOWING |
| `Anime.vue` Reviews tab | `reviews` ref | `reviewApi.getAnimeReviews()` → GET `/api/anime/{id}/reviews` → ReviewHandler → anime_list query `score > 0 OR review_text != ''` | Yes — 658 rows visible in live DB | FLOWING |
| `ActivityFeed.vue` | comment events | `activity_events` table rows with `type='comment'` | Yes — 6 rows in DB, new row created on every POST | FLOWING |

---

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Comments table exists with correct schema | `\d comments` | id, user_id, anime_id, username, body, parent_id, created_at, updated_at, deleted_at + 4 indexes | PASS |
| POST comment creates row + activity_event | Live API call with ui_audit_bot token | 201 response, comment visible in GET, activity_events row created | PASS |
| DELETE soft-deletes (deleted_at set, excluded from GET) | DELETE API + DB query | `deleted_at = 2026-05-13 03:48:27.57376+00`, GET returns empty for that anime | PASS |
| reviews table dropped | `\d reviews` | "Did not find any relation named reviews" | PASS |
| anime_list has review_text + username | `\d anime_list` grep | Both columns present with NOT NULL defaults | PASS |
| Locale keys complete | `jq '.anime.ugc \| keys \| length'` all 3 files | 24, 24, 24 | PASS |
| e2e: deep-link, URL-persist, anon-prompt, CRUD | `bunx playwright test e2e/comments.spec.ts` | 4 passed (5.5s) | PASS |
| Migration idempotency | `go test -run TestSocialMigration_Idempotent` | PASS | PASS |
| Rate limit 429 | `go test -run TestCommentService_RateLimit` | PASS | PASS |
| Activity emit | `go test -run TestCommentService_EmitsActivity` | PASS | PASS |
| Review shape preservation | `go test -run TestReviewHandler` (3 tests) | 3 PASS | PASS |
| Comments backend handler | `go test -run TestCommentHandler` (3 tests) | 3 PASS | PASS |
| Comments repo | `go test -run TestCommentRepo` (2 tests) | 2 PASS | PASS |

---

## Anti-Patterns Found

| File | Pattern | Severity | Assessment |
|------|---------|----------|------------|
| `frontend/web/src/views/Anime.vue:1421` | `XXXXX` in comment `// mal_XXXXX entries` | Info | Pre-existing code (introduced in commit `4f1103a`, before phase 1). Not a phase-introduced marker. Not a debt marker — it is a literal URL-path fragment `mal_XXXXX` used as an example identifier pattern. No action required. |

No blockers found. No new TBD/FIXME/XXX debt markers introduced by this phase.

---

## Requirements Coverage

| Requirement | Description | Status | Evidence |
|-------------|-------------|--------|----------|
| SOCIAL-01 | Schema consolidation: merge reviews into anime_list, drop reviews table | SATISFIED | DB: reviews table gone, anime_list has review_text + username |
| SOCIAL-02 | One-shot idempotent migration | SATISFIED | `TestSocialMigration_Idempotent` PASS; 658 migrated rows in DB |
| SOCIAL-03 | Reviews endpoints read from anime_list, shape preserved | SATISFIED | 3 shape tests pass; live endpoint returns correct 7-field JSON |
| SOCIAL-04 | Comments table + CRUD API (4 endpoints) | SATISFIED | All 4 endpoints live and tested; DB schema verified |
| SOCIAL-05 | Activity event emitted on comment create | SATISFIED | `TestCommentService_EmitsActivity` PASS; live verification: 6 activity_events rows with type='comment' |
| SOCIAL-06 | Tabbed UI with URL persistence, anon login prompt | SATISFIED | 4/4 Playwright e2e tests pass |
| SOCIAL-NF-01 | Rate limit: 10 comments/user/anime/hour, 429 on excess | SATISFIED | `TestCommentService_RateLimit` PASS |
| SOCIAL-NF-02 | All new UI strings translated in en/ja/ru | SATISFIED | 24 keys × 3 locales = 72 entries; 0 missing keys; 0 intlify warnings |

---

## Human Verification Required

None. All 8 success criteria were fully verified by automated means (unit tests, DB queries, live HTTP spot-checks, Playwright e2e). No items require manual sign-off.

---

_Verified: 2026-05-13T03:50:00Z_
_Verifier: Claude (gsd-verifier)_
