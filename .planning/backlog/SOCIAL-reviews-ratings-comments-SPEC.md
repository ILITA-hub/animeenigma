---
id: SOCIAL-reviews-ratings-comments
title: Merge reviews into anime_list (drop sync) + add separate Comments feature + tabbed UI
captured_at: 2026-05-13
captured_during: standalone spec request
target_milestone: TBD (post-v3.0 candidate — not yet on ROADMAP.md)
status: backlog (SPEC-ready, awaiting milestone slot)
---

# SOCIAL-reviews-ratings-comments — Specification

**Created:** 2026-05-13 (v2 — simplified per user feedback "sync усложнит систему, хотелось бы упростить")
**Ambiguity score:** 0.15 (gate: ≤ 0.20)
**Requirements:** 6 locked
**Mode:** `--auto` (Socratic interview skipped per user instruction "work without stopping for clarifying questions"; key decisions logged below as auto-selected — review and override before promoting to a phase)

> **Phase status:** This is a backlog SPEC, not yet bound to a milestone or phase number.
> To promote: `/gsd-phase add` to slot into a milestone, then move this file to `.planning/phases/<N>-social-reviews-comments/<NN>-SPEC.md` and run `/gsd-discuss-phase <N>`.

## Goal

Eliminate the `reviews` table by merging review text into `anime_list` (one row per `(user, anime)`, single source of truth for both score and text), then add a separate flat **Comments** stream alongside reviews with a tab toggle inside the Reviews section on the anime detail page.

## Background

**Today, three things are true and need to change:**

1. **Two tables hold one concept.** The `reviews` table (`services/player/internal/domain/watch.go:105`) stores `(user_id, anime_id, score, review_text, username)` with a unique index on `(user_id, anime_id)`. The `anime_list` table (same file, line 58) stores `(user_id, anime_id, score, notes, …)` with the same unique index. Two tables, same primary-key shape, same `score` column, different text fields — but functionally the same row from the user's perspective.

2. **One-way sync hides imported ratings from the public reviews list.** The on-site flow writes both: `ReviewService.CreateOrUpdateReview` writes `reviews` and pushes the score down into `anime_list.score` (`services/player/internal/service/review.go:~93`). The reverse direction does not exist. MAL imports (`handler/mal_import.go:277-278`) and Shikimori imports (`handler/shikimori_import.go:242`) write only `anime_list.score`. Result: thousands of imported ratings are invisible on the public reviews list of every anime.

3. **No comments feature exists.** The anime detail page (`frontend/web/src/views/Anime.vue:586-705`) renders a single "Reviews" section with no tabs and no comment stream. There is no `comments` table, no comments service, no API. The frontend has a `reviewApi` (`frontend/web/src/api/client.ts:338`) and that is the entire UGC surface.

**The simpler design:**
- Drop the `reviews` table entirely. Move `review_text` into `anime_list` next to `notes`. A user has at most one "review" per anime by virtue of having at most one list entry per anime. Score and text live on the same row — no sync logic anywhere, no two-write hazards, no provenance column.
- "The reviews list for anime X" becomes `SELECT * FROM anime_list WHERE anime_id = X AND (score > 0 OR review_text != '')`.
- Imports automatically appear in the reviews list because they already write to `anime_list`.
- Comments are a genuinely different concept (many-per-user, no score, free-form) so they get their own table.

## Requirements

1. **Schema consolidation.** Move `review_text` from `reviews` into `anime_list` and remove the `reviews` table.
   - **Current:** Two tables (`reviews`, `anime_list`) each carry a `score` column for the same `(user_id, anime_id)` pair plus dedicated text columns (`review_text`, `notes`).
   - **Target:** `anime_list` carries `score int`, `notes text` (private, existing), `review_text text default ''` (public, new), and `username varchar(32)` (denormalized from `users` for join-free renders, mirrors the current `reviews.username` pattern). The `reviews` table is dropped. The unique index on `(user_id, anime_id)` already enforces "at most one review per user per anime."
   - **Acceptance:** `\d anime_list` shows `review_text` and `username` columns. `\d reviews` returns "Did not find any relation named reviews." All existing `anime_list` rows have a non-null (possibly empty) `review_text` and a populated `username`.

2. **One-shot migration with text + username copy.** A migration copies `reviews.review_text` and `reviews.username` into the matching `anime_list` row before dropping the `reviews` table.
   - **Current:** No migration exists. `reviews` rows would be orphaned if the table were dropped naively.
   - **Target:** Migration (Go-side, registered with the existing GORM AutoMigrate path in `cmd/player-api/main.go`, gated by a one-time idempotency check like a marker row in a `schema_migrations` table or a check for the `reviews` table's existence) performs:
     1. `ALTER TABLE anime_list ADD COLUMN review_text text NOT NULL DEFAULT '';`
     2. `ALTER TABLE anime_list ADD COLUMN username varchar(32) NOT NULL DEFAULT '';`
     3. For every `reviews` row, upsert into `anime_list` matching `(user_id, anime_id)`: set `score = reviews.score` (only if `anime_list.score = 0` or unset), set `review_text = reviews.review_text`, set `username = reviews.username`. If no `anime_list` row exists for that pair, create one with `status='completed'` (consistent with someone who wrote a review of it).
     4. Backfill `username` for any remaining `anime_list` rows by joining against `users`.
     5. `DROP TABLE reviews;`
   - **Acceptance:** After migration on a database with N `reviews` rows: every one of those (user_id, anime_id) pairs has a row in `anime_list` with `review_text` and `username` populated. `reviews` table no longer exists. Running the migration twice is a no-op (idempotency check passes immediately on second run).

3. **Reviews endpoints read from `anime_list`.** All reviews API endpoints query `anime_list` instead of `reviews`, with the response shape preserved so the frontend keeps working unchanged.
   - **Current:** `GET /api/anime/:id/reviews`, `GET /api/anime/:id/rating`, `GET /api/anime/:id/reviews/me`, `POST /api/anime/:id/reviews`, `DELETE /api/anime/:id/reviews`, `POST /api/anime/ratings/batch` — all backed by `ReviewRepository` queries on the `reviews` table.
   - **Target:** Same routes and request/response shapes, backed by queries on `anime_list`. The list filter is `WHERE anime_id = ? AND (score > 0 OR review_text != '')` for the public reviews list. `POST /api/anime/:id/reviews` becomes an `UPSERT` into `anime_list` setting `score` and `review_text` (and `status='completed'` if no row existed). `DELETE` clears both `score` and `review_text` but leaves the list entry; the row drops out of the reviews list because the filter no longer matches.
   - **Acceptance:** Frontend `reviewApi.getAnimeReviews(animeId)`, `reviewApi.getAnimeRating(animeId)`, `reviewApi.getMyReview(animeId)`, `reviewApi.createReview(...)`, `reviewApi.deleteReview(...)`, `reviewApi.getBatchRatings([...])` all return responses with the exact same JSON shape as before the change. Existing frontend code paths (Anime.vue, Home.vue, Browse.vue, ActivityFeed.vue, useSiteRatings.ts) require zero modifications. Integration test fixtures pass without rewrites.

4. **Comments table and CRUD API.** A new `comments` table and four endpoints on the player service surface a per-anime comment stream — kept separate because comments are many-per-user, score-less, and free-form.
   - **Current:** No `comments` table; no comments endpoints; no comments handler/service/repo in `services/player/internal/`.
   - **Target:** New domain type `Comment` (`id uuid PK`, `user_id uuid`, `anime_id uuid`, `username varchar(32)`, `body text`, `parent_id uuid nullable` [reserved, NULL in v1], `deleted_at timestamp nullable` [soft delete], `created_at`, `updated_at`). Indexes on `(anime_id, created_at DESC)` and `(user_id, created_at DESC)`. Routes on the player service:
     - `GET    /api/anime/:id/comments?cursor=&limit=50` — paginated, newest first, excludes soft-deleted, public
     - `POST   /api/anime/:id/comments` — auth required, body 1–2000 chars, rate-limited
     - `PATCH  /api/anime/:id/comments/:cid` — auth required, owner only, body 1–2000 chars
     - `DELETE /api/anime/:id/comments/:cid` — auth required, owner OR admin, soft delete
   - **Acceptance:** Each endpoint has a unit test verifying happy-path + 1 failure case (PATCH by non-owner returns 403, POST with empty body returns 400, GET returns paginated newest-first results excluding soft-deleted rows). Schema migration runs via GORM `AutoMigrate(&domain.Comment{})` on player service startup.

5. **Activity event for new comments.** Posting a comment emits an `activity_events` row so the user's followers see it in their feed, mirroring how reviews already emit activity events.
   - **Current:** `ReviewService.CreateOrUpdateReview` emits a `review` activity event (deduped per day). Comments emit nothing because they don't exist.
   - **Target:** `CommentService.Create` emits an `activity_events` row with `type='comment'`, content preview (first 300 runes, "…" suffix if truncated), `anime_id`, `user_id`, `username`. No dedup — multiple comments per day each emit their own event (different from reviews, which dedup per day).
   - **Acceptance:** Posting three comments on the same anime by the same user produces three `activity_events` rows. Posting two comments on different anime by the same user on the same day also produces two events.

6. **Tabbed UI on Anime detail page.** The current "Reviews" section becomes a two-tab strip with `Reviews` and `Comments`; tab selection is URL-persisted.
   - **Current:** `frontend/web/src/views/Anime.vue:586-705` renders a single Reviews section. No tabs, no comments UI.
   - **Target:** Refactor the section into a two-tab strip (inline or as a `<UgcTabs>` component — match whichever pattern already exists in the codebase for tabs). Tabs: `Reviews ({count})` and `Comments ({count})`. URL state via query param `?ugc=reviews` (default) or `?ugc=comments`. Each tab renders its respective stream. Comments tab includes a textarea input (auth-gated), a "Post" button, a paginated comment list ("Load more" button below the last item), and edit/delete actions on the user's own comments.
   - **Acceptance:** Playwright e2e — load `/anime/<id>?ugc=comments` and the Comments tab is the active tab on first paint. Switching tabs updates the URL query param. Refreshing the page preserves the active tab. A logged-in user can post, edit, and delete their own comment. A logged-out user sees the comment list + a login prompt instead of the textarea.

## Boundaries

**In scope:**
- Schema change: add `review_text`, `username` columns to `anime_list`; drop `reviews` table
- One-shot migration that copies `reviews.review_text` + `reviews.username` into `anime_list` and creates missing rows
- Refactor of `ReviewService` / `ReviewRepository` to read and write `anime_list` instead of `reviews`
- API response-shape preservation so the frontend needs zero changes for the schema swap
- New `Comment` domain + repo + service + handler + routes in the player service
- Comments emit `activity_events` (no dedup)
- Anime detail page tabs: `Reviews` | `Comments`, URL-persisted via `?ugc=...`
- Locale keys for new UI strings (3 locales)

**Out of scope:**
- **Bidirectional sync between separate tables.** Eliminated by design — one table, one row, no sync. Was in v1 of this spec; removed per user feedback.
- **Source/provenance column on reviews.** Removed by design — `anime_list` already has `mal_id` which already signals "imported from MAL," and Shikimori provenance can be inferred from import history if ever needed. No new column.
- **Comment replies / threading.** `comments.parent_id` exists in the schema but stays NULL in v1; no reply UI, no nesting. Reason: scope creep — flat comments are sufficient for v1 and threading needs separate UX work.
- **Voting / likes** on reviews or comments. Reason: separate engagement feature.
- **Markdown / rich text** in comments. Plain text only, with newlines preserved (`whitespace-pre-wrap`). Reason: XSS surface + sanitization complexity not worth it for v1.
- **Mentions / notifications.** No `@username` parsing, no notification fan-out on comment creation. Reason: requires the notification engine (see `memory/project_notifications_engine.md`).
- **Comment moderation queue.** Admin can soft-delete any comment via the existing DELETE endpoint; no review-queue UI, no flagging, no auto-mod. Reason: minimal moderation is sufficient until there's a real problem.
- **Real-time updates** for comments (WebSocket / SSE / polling). The list refetches on tab activation and after the user posts; that's it.
- **Cross-sync comments → anime_list.notes** or vice versa. Comments are independent.
- **Pagination of reviews.** Reviews stay un-paginated (matching current behavior); only comments are paginated.
- **Recommendations engine integration.** Comments do not feed the rec engine. Reviews already feed it via `anime_list.score`, and that doesn't change.
- **Migration rollback.** Forward-only. Backup runs in `make redeploy-player` as usual. Reason: rollback would require restoring the `reviews` table from backup anyway, which is the standard ops procedure for any GORM AutoMigrate change.
- **Phase placement in ROADMAP.md.** This spec is a backlog candidate — slotting it into a milestone is a separate decision.

## Constraints

- **Anime_list schema additions:** `review_text text NOT NULL DEFAULT ''`, `username varchar(32) NOT NULL DEFAULT ''`. Both nullable-safe-by-default so existing rows pre-migration remain valid.
- **Comment body:** 1–2000 characters (UTF-8), validated server-side. Reject empty / whitespace-only bodies with 400.
- **Comments rate limit:** 10 comments per user per anime per hour at the handler layer (reuse existing gateway rate-limit infra if compatible, else a per-user in-memory bucket). Returns 429 on excess.
- **Comments pagination:** 50 per page via cursor (created_at + id), newest first. Reuses an existing cursor pattern in the player service if available, else introduces one.
- **GORM AutoMigrate:** Schema changes go through the standard project pattern — update the domain struct and register with `db.AutoMigrate(...)` in `cmd/player-api/main.go`. The data-copy + table-drop step is a one-shot bootstrap function called once at service startup, gated by an idempotency check (e.g. "does `reviews` table still exist?").
- **No-downtime rollout strategy:** The migration runs on player service startup. During the brief window where some replicas have the new code and some have the old: old replicas writing `reviews` is acceptable for ≤ 1 deploy cycle because the migration runs again on next startup. Recommended: deploy with single replica or drain old replicas before rollout.
- **Response shape preservation:** All review endpoints return JSON identical to today's shape (same field names, same types, same omissions). The frontend treats `reviews` as opaque rows; this constraint guarantees zero frontend churn for the schema swap part. New `username` field on `anime_list` rows in *other* endpoints (e.g. `/api/users/list`) is additive and ignored by existing consumers.
- **Player service deploy only:** Schema changes ship via `make redeploy-player`. Auth, gateway, catalog, etc., are untouched.

## Acceptance Criteria

- [ ] **Single-source schema:** After deploy, `anime_list` has `review_text` and `username` columns; `reviews` table does not exist; no Go code references `domain.Review` or `ReviewRepository`.
- [ ] **Migration idempotency:** Running the player service startup migration twice produces no data changes on the second run.
- [ ] **Migration completeness:** For every former row in `reviews`, there is now an `anime_list` row with matching `(user_id, anime_id)`, identical `score`, identical `review_text`, identical `username`. No data loss.
- [ ] **API shape preservation:** All six review endpoints (`getAnimeReviews`, `getAnimeRating`, `getMyReview`, `createReview`, `deleteReview`, `getBatchRatings`) return responses with identical JSON shape to pre-migration. Verified by golden-file diff against a snapshot taken before the migration.
- [ ] **Frontend unchanged for reviews:** Anime.vue, Home.vue, Browse.vue, ActivityFeed.vue, useSiteRatings.ts compile and render correctly with zero modifications related to the reviews schema swap (modifications are only for the new comments tab).
- [ ] **Imported ratings visible:** A user with a MAL import containing `score=8` on an anime appears in that anime's public reviews list with score 8 and empty `review_text`, with their username. This is a side effect of req 3 — no extra code needed.
- [ ] **Comments CRUD:** All four endpoints (`GET`/`POST`/`PATCH`/`DELETE` on `/api/anime/:id/comments[/:cid]`) return correct status codes for happy-path and one failure path each. Verified by unit tests.
- [ ] **Soft delete:** Deleted comments do not appear in `GET /api/anime/:id/comments` responses; the row still exists with `deleted_at` populated.
- [ ] **Rate limit:** The 11th comment by the same user on the same anime within an hour returns HTTP 429.
- [ ] **Activity events:** Posting N comments produces N activity events with `type='comment'`.
- [ ] **Tabs UI:** `/anime/<id>?ugc=comments` opens the Comments tab on first paint; switching tabs updates the URL; reload preserves the tab.
- [ ] **Anonymous Comments tab:** A logged-out user sees the comment list + a login prompt instead of the textarea.
- [ ] **Locales:** All new UI strings have translations for the three project locales; no missing-key warnings in dev console.

## Ambiguity Report

| Dimension          | Score | Min  | Status | Notes                                                                  |
|--------------------|-------|------|--------|------------------------------------------------------------------------|
| Goal Clarity       | 0.88  | 0.75 | ✓      | Single goal sentence + 6 concrete requirements                         |
| Boundary Clarity   | 0.88  | 0.70 | ✓      | 12-item out-of-scope list with reasoning (incl. explicit sync removal) |
| Constraint Clarity | 0.78  | 0.65 | ✓      | Schema, rate limit, response-shape, no-downtime strategy all locked    |
| Acceptance Criteria| 0.82  | 0.70 | ✓      | 13 pass/fail checkboxes                                                |
| **Ambiguity**      | 0.15  | ≤0.20| ✓      | Below gate (improved from v1's 0.18 by removing sync complexity)       |

Status: ✓ = met minimum, ⚠ = below minimum (planner treats as assumption)

## Interview Log

`--auto` mode — Socratic interview skipped per user instruction; key decisions logged below. **Review and override before promoting to a phase.**

| Round | Perspective       | Question (implicit)                                                | Auto-selected decision                                                                                       |
|-------|-------------------|--------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------|
| 1     | Researcher        | What's the actual gap between ratings and reviews?                 | Two tables, both holding `(user_id, anime_id, score)` with overlapping uniqueness. Different text columns.   |
| 1     | Researcher        | Does a comments concept exist anywhere?                            | No. Greenfield feature.                                                                                       |
| 2     | Simplifier        | v1 spec proposed bidirectional sync. Is there a simpler design?    | **Yes — merge `reviews` into `anime_list`.** One row per (user, anime); same unique-index shape; no sync.   |
| 2     | Simplifier        | Minimum viable comments shape?                                     | Flat list. `parent_id` reserved in schema but always NULL in v1.                                              |
| 3     | Boundary Keeper   | What's tempting to scope-creep that we should exclude?             | Replies, votes, markdown, mentions, moderation queue, real-time updates, source/provenance column.            |
| 3     | Boundary Keeper   | What does "done" look like at the UI level?                        | Two tabs in Anime.vue, URL-persisted via `?ugc=`, count badges.                                              |
| 4     | Failure Analyst   | What's the worst-case data hazard?                                 | Losing review text during migration. Mitigation: per-row copy with explicit field mapping, idempotent guard. |
| 4     | Failure Analyst   | What could silently break the frontend?                            | Changing the JSON response shape of review endpoints. Mitigation: response-shape preservation constraint.    |
| 5     | Seed Closer       | Where does `username` live in the merged model?                    | Denormalized into `anime_list` (matches the current `reviews.username` pattern). Backfilled from `users`.    |
| 5     | Seed Closer       | Comment rate-limit threshold?                                      | 10 per user per anime per hour. Tunable post-launch.                                                          |
| 6     | Seed Closer       | What happens to old `reviewRepo`/`reviewService` code?             | Refactored in place to query `anime_list`. Public API and method names preserved so handler code is stable.  |

**Decisions most likely to need user override before phase promotion:**
1. Rate-limit threshold (10/hour) — pure guess; user may have a stronger opinion.
2. Whether `DELETE /api/anime/:id/reviews` clears both score AND text (current behavior implication), or only the text. Auto-selected: clear both, matching today's "deleting your review" semantics.
3. Whether to keep `username` denormalized on `anime_list` rows or always JOIN to `users`. Auto-selected: denormalize (matches existing reviews pattern, avoids JOIN on every reviews-list render).
4. No-downtime rollout strategy — auto-selected single-replica or drained rollout. May need a more sophisticated approach (dual-write window) if the prod deployment has multiple player replicas.

---

*Backlog item — not yet on ROADMAP.md.*
*Spec created: 2026-05-13 (auto mode, v2 simplified)*
*Next step when promoted:* `/gsd-phase add` to slot into a milestone → move this file to `.planning/phases/<N>-social-reviews-comments/<NN>-SPEC.md` → `/gsd-discuss-phase <N>`.
