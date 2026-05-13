---
id: SOCIAL-reviews-ratings-comments
title: Unify ratings into reviews + add separate Comments feature + tabbed UI
captured_at: 2026-05-13
captured_during: standalone spec request
target_milestone: TBD (post-v3.0 candidate — not yet on ROADMAP.md)
status: backlog (SPEC-ready, awaiting milestone slot)
---

# SOCIAL-reviews-ratings-comments — Specification

**Created:** 2026-05-13
**Ambiguity score:** 0.18 (gate: ≤ 0.20)
**Requirements:** 7 locked
**Mode:** `--auto` (Socratic interview skipped per user instruction "work without stopping for clarifying questions"; key decisions logged below as auto-selected — review and override before promoting to a phase)

> **Phase status:** This is a backlog SPEC, not yet bound to a milestone or phase number.
> To promote: `/gsd-phase add` to slot into a milestone, then move this file to `.planning/phases/<N>-social-reviews-comments/<NN>-SPEC.md` and run `/gsd-discuss-phase <N>`.

## Goal

Make every user score (whether typed on-site or imported from MAL/Shikimori) visible on the anime's public reviews list, and add a separate flat **Comments** stream alongside reviews — with a tab toggle inside the existing Reviews section on the anime detail page to switch between the two streams.

## Background

**Today, three things are true and need to change:**

1. **Reviews and ratings are split.** The `reviews` table (`services/player/internal/domain/watch.go:105`) stores `(user_id, anime_id, score, review_text)` with a unique index on `(user_id, anime_id)`. The `anime_list` table stores a separate `score` field on each watchlist entry. The on-site flow syncs **review → list** (`services/player/internal/service/review.go:~93`) but never **list → review**.

2. **Imported MAL/Shikimori scores are invisible.** `services/player/internal/handler/mal_import.go:277-278` and `services/player/internal/handler/shikimori_import.go:242-243` write the imported MAL `score` into `anime_list.score` only. No `reviews` row is created. Result: a user with thousands of MAL ratings has zero reviews visible on any anime page; the public reviews list under-represents the user base.

3. **No comments feature exists.** The anime detail page (`frontend/web/src/views/Anime.vue:586-705`) renders a single "Reviews" section with no tabs and no comment stream. There is no `comments` table, no comments service, no API surface. The frontend has a `reviewApi` (`frontend/web/src/api/client.ts:338`) and that is the entire UGC surface.

**The user-visible delta this phase delivers:** users with imported ratings show up as star-only reviews on anime pages; the Reviews section becomes a two-tab strip (`Reviews` | `Comments`); users can post free-form comments per anime without a star rating.

## Requirements

1. **List-score → review denormalization.** Any insert/update of `anime_list` with `score > 0` causes a `reviews` row to be upserted for the same `(user_id, anime_id)` pair.
   - **Current:** `ListService.UpdateListEntry` writes only `anime_list`. `ReviewService.CreateOrUpdateReview` writes `reviews` and pushes the score down to `anime_list`. The list→review direction does not exist.
   - **Target:** `ListService.UpdateListEntry` calls a new internal helper (or `ReviewService` method) that upserts `reviews` with `(user_id, anime_id, username, score, review_text='')` when `score > 0`. If a review already exists for that pair and has non-empty `review_text`, leave `review_text` untouched and update only `score`.
   - **Acceptance:** Integration test — POST `/api/users/list` with `score=8` for a user with no existing review creates a `reviews` row with `score=8, review_text=''`. Re-running with `score=9` updates the same row to `score=9` and preserves `review_text`. Setting `score=0` does NOT delete the review.

2. **MAL import populates reviews.** Every MAL list entry imported with `score > 0` produces a `reviews` row in addition to the existing `anime_list` row.
   - **Current:** `mal_import.go:271-318` only writes to `anime_list`. `shikimori_import.go:242` likewise.
   - **Target:** Both import handlers route through the unified list-update path so the new list→review denormalization triggers automatically. No duplicate write logic in the import handlers.
   - **Acceptance:** End-to-end test — import a synthetic MAL XML with 3 entries (scores 7, 9, 0). After import, `reviews` table has 2 new rows (the 0-score entry is skipped), each with `review_text=''` and the correct score; `anime_list` has all 3 rows.

3. **Backfill migration for existing data.** A one-time backfill creates `reviews` rows for every existing `anime_list.score > 0` that has no matching `reviews` row.
   - **Current:** No backfill exists; `reviews` only contains on-site submissions.
   - **Target:** A migration (Go-side, runs once at service startup behind an idempotent guard, OR a one-shot CLI: `cmd/player-tools/backfill-reviews/main.go`) inserts `(user_id, anime_id, username, score, review_text='', source='list')` for every `anime_list` row with `score > 0` and no existing review for that pair.
   - **Acceptance:** Backfill is idempotent — running it twice produces the same `reviews` row count. After backfill on prod-like data, `SELECT COUNT(*) FROM reviews WHERE review_text = ''` is ≥ the count of `anime_list` rows with `score > 0` (modulo pre-existing text reviews where score-only would conflict, in which case the text review wins).

4. **Review provenance via `source` column.** Reviews track where the score came from so the UI can label rating-only entries.
   - **Current:** `reviews` schema has no provenance column.
   - **Target:** Add `source varchar(16) not null default 'website'` column to `reviews`. Allowed values: `website` (typed on-site, with or without text), `list` (synced from list edit), `mal` (came from a MAL import), `shikimori` (came from a Shikimori import). Import handlers and the list→review sync pass the correct value through. `source` is informational only — never affects aggregate rating math.
   - **Acceptance:** `SELECT DISTINCT source FROM reviews` after a backfill + a fresh MAL import returns exactly `{website, list, mal}` (or a subset reflecting which paths ran). The aggregated rating endpoint `/api/anime/:id/rating` returns the same `average_score` and `total_reviews` regardless of `source` distribution.

5. **Comments table and CRUD API.** A new `comments` table and four endpoints on the player service surface a per-anime comment stream.
   - **Current:** No `comments` table; no comments endpoints; no comments handler/service/repo in `services/player/internal/`.
   - **Target:** New domain type `Comment` (id uuid PK, user_id uuid, anime_id uuid, username varchar(32), body text, parent_id uuid nullable [reserved, NULL in v1], deleted_at timestamp nullable [soft delete], created_at, updated_at). Indexes on `(anime_id, created_at DESC)` and `(user_id, created_at DESC)`. Routes on the player service:
     - `GET    /api/anime/:id/comments?cursor=&limit=50` — paginated, newest first, excludes soft-deleted, public
     - `POST   /api/anime/:id/comments` — auth required, body 1–2000 chars, rate-limited
     - `PATCH  /api/anime/:id/comments/:cid` — auth required, owner only, body 1–2000 chars
     - `DELETE /api/anime/:id/comments/:cid` — auth required, owner OR admin, soft delete
   - **Acceptance:** Each endpoint has a unit test verifying happy-path + 1 failure case (e.g. PATCH by non-owner returns 403, POST with empty body returns 400, GET returns paginated newest-first results excluding soft-deleted rows). Schema migration runs via GORM `AutoMigrate(&domain.Comment{})` on player service startup.

6. **Activity event for new comments.** Posting a comment emits an `activity_events` row so the user's followers see it in their feed, mirroring how reviews emit activity events today.
   - **Current:** `ReviewService.CreateOrUpdateReview` emits a `review` activity event (deduped per day) in `services/player/internal/service/review.go`. Comments emit nothing because they don't exist.
   - **Target:** `CommentService.Create` emits an `activity_events` row with `type='comment'`, the content preview (first 300 runes, "…" suffix if truncated), `anime_id`, `user_id`, `username`. No dedup — multiple comments per day each emit their own event (different from reviews, which dedup per day).
   - **Acceptance:** Posting two comments on different anime by the same user on the same day produces two `activity_events` rows. Posting three comments on the same anime by the same user produces three `activity_events` rows.

7. **Tabbed UI on Anime detail page.** The current "Reviews" section becomes a two-tab strip with `Reviews` and `Comments`; tab selection is URL-persisted.
   - **Current:** `frontend/web/src/views/Anime.vue:586-705` renders a single "Reviews" section. No tabs, no comments UI.
   - **Target:** Refactor the section into a `<UgcTabs>` component (or inline tabs — pick whatever matches existing tab patterns in the project, e.g. the player tab strip). Two tabs: `Reviews ({count})` and `Comments ({count})`. URL state via query param `?ugc=reviews` (default) or `?ugc=comments`. Each tab renders its respective stream. Comments tab has a textarea input (auth-gated), a "Post" button, a paginated comment list ("Load more" button below the last item), and edit/delete actions on the user's own comments.
   - **Acceptance:** Playwright e2e — load `/anime/<id>?ugc=comments` and the Comments tab is the active tab on first paint; switching tabs updates the URL query param; refreshing the page preserves the active tab; a logged-in user can post, edit, and delete their own comment; a logged-out user sees the comment list + a login prompt instead of the textarea.

## Boundaries

**In scope:**
- Schema additions: `reviews.source` column, new `comments` table
- One-time backfill: `anime_list.score > 0` → `reviews` rows
- List-update → review-upsert sync (closes the existing one-way sync into a two-way sync)
- MAL/Shikimori imports flow through the unified list-update path so denormalization triggers automatically
- Comments domain + repo + service + handler + routes in the player service
- Comments emit `activity_events`
- Anime detail page tabs: `Reviews` | `Comments`, URL-persisted via `?ugc=...`
- Rating-only reviews (empty `review_text`) get a small "via MAL" / "via Shikimori" / "from your list" provenance badge in the reviews tab
- Locale keys for new UI strings (3 locales — consistent with project i18n setup)

**Out of scope:**
- **Comment replies / threading.** `comments.parent_id` exists in the schema but stays NULL in v1; no reply UI, no nesting. Reason: scope creep — flat comments are sufficient for v1 and threading needs separate UX work.
- **Voting / likes** on reviews or comments. Reason: separate engagement-engine feature, deserves its own phase.
- **Markdown / rich text** in comments. Plain text only, with newlines preserved (`whitespace-pre-wrap`). Reason: XSS surface + sanitization complexity not worth it for v1.
- **Mentions / notifications.** No `@username` parsing, no notification fan-out on comment creation. Reason: requires a notification engine (see `memory/project_notifications_engine.md` — that's a separate backlog initiative).
- **Comment moderation queue.** Admin can soft-delete any comment via the existing DELETE endpoint, but there's no review-queue UI, no flagging, no auto-mod. Reason: minimal moderation is sufficient until there's a real problem.
- **Real-time updates** (WebSocket / SSE / polling for new comments). The list refetches on tab activation and after the user posts; that's it. Reason: complexity not justified for v1 traffic.
- **Cross-sync comments → anime_list.notes** or vice versa. Comments are independent. Reason: they serve different purposes (notes are private; comments are public).
- **Pagination of reviews.** Reviews stay un-paginated (matching current behavior). Only comments are paginated. Reason: review counts per anime are small; pagination is premature.
- **Recommendations engine integration.** Comments do not feed the rec engine. Reviews already feed it via `anime_list.score`. Reason: comments are free-form and noisy; not a useful signal without NLP work.
- **Phase placement in ROADMAP.md.** This spec is a backlog candidate — slotting it into a milestone (v3.1, v4.0, or as a Phase 21 carry-over) is a separate decision by the user.

## Constraints

- **Comment body:** 1–2000 characters (UTF-8), validated server-side. Reject empty / whitespace-only bodies with 400.
- **Comments rate limit:** 10 comments per user per anime per hour, enforced at the handler layer (use the existing gateway rate-limit infrastructure if reusable, else a per-user in-memory bucket). Returns 429 on excess.
- **Comments pagination:** 50 per page via cursor (created_at + id), newest first. Reuses the cursor pattern from any existing cursor-paginated endpoint in the player service; otherwise introduces one.
- **GORM AutoMigrate:** Use the standard pattern — add the field/table to the domain struct, register with `db.AutoMigrate(...)` in `cmd/player-api/main.go`. No manual SQL migration files (matches existing project convention per `CLAUDE.md` Database section).
- **Backfill performance:** Backfill must process a snapshot of `anime_list` in batches of 1000 rows; total runtime budget ≤ 5 minutes on the production dataset (current `anime_list` row count + future capacity). Log progress every 1000 rows.
- **No breaking changes to existing endpoints:** `reviewApi.getAnimeReviews`, `reviewApi.getAnimeRating`, `reviewApi.createReview`, etc., keep their current request/response shapes. The `source` field is additive in the response. Existing frontend code paths continue to work without modification.
- **Player service deploy:** Schema changes ship via `make redeploy-player` only — no other service rebuilds required. (Auth service is unaffected; gateway routing already wildcards `/api/users/*` and `/api/anime/:id/...` to player + catalog respectively.)

## Acceptance Criteria

- [ ] **Sync path:** Setting a non-zero score on an `anime_list` row (via on-site list edit, MAL import, or Shikimori import) produces a corresponding `reviews` row with the same score, same user, same anime, and empty `review_text` (unless one already exists with text).
- [ ] **Backfill idempotency:** Running the backfill twice on the same database produces identical row counts in `reviews`.
- [ ] **Source labeling:** Every row in `reviews` has a non-null `source` value from the allowed enum. Existing rows default to `'website'`.
- [ ] **Aggregate parity:** `GET /api/anime/:id/rating` returns the same `average_score` and `total_reviews` regardless of `source` distribution (i.e. rating-only reviews are first-class citizens in the aggregate).
- [ ] **Comments CRUD:** All four endpoints (`GET`/`POST`/`PATCH`/`DELETE` on `/api/anime/:id/comments[/:cid]`) return correct status codes for happy-path and one failure path each. Verified by unit tests in `services/player/internal/handler/comment_test.go`.
- [ ] **Soft delete:** Deleted comments do not appear in `GET /api/anime/:id/comments` responses and cannot be retrieved by ID; the row still exists in the database with `deleted_at` populated.
- [ ] **Rate limit:** The 11th comment by the same user on the same anime within an hour returns HTTP 429.
- [ ] **Activity events:** Posting a comment inserts an `activity_events` row with `type='comment'` and the truncated body preview. Posting N comments produces N activity events.
- [ ] **Tabs UI:** `/anime/<id>?ugc=comments` opens the Comments tab on first paint; switching tabs updates the URL; reload preserves the tab.
- [ ] **Anonymous Comments tab:** A logged-out user can read comments but the textarea is replaced with a login prompt.
- [ ] **Provenance badge:** A rating-only review (empty `review_text`, `source='mal'`) renders with a "via MAL" badge next to the star count in the Reviews tab.
- [ ] **No regression:** Existing reviewApi calls, the AnimeRating aggregate, and the home-page batch ratings (`reviewApi.getBatchRatings`) continue to return correct data for all existing test fixtures.
- [ ] **Locales:** All new UI strings have translations for the three project locales; missing-key warnings do not appear in dev console.

## Ambiguity Report

| Dimension          | Score | Min  | Status | Notes                                                                |
|--------------------|-------|------|--------|----------------------------------------------------------------------|
| Goal Clarity       | 0.85  | 0.75 | ✓      | Three concrete pieces + acceptance per requirement                   |
| Boundary Clarity   | 0.85  | 0.70 | ✓      | Explicit 9-item out-of-scope list with reasoning                     |
| Constraint Clarity | 0.75  | 0.65 | ✓      | Body size, rate limit, pagination, AutoMigrate, deploy scope locked  |
| Acceptance Criteria| 0.80  | 0.70 | ✓      | 13 pass/fail checkboxes                                              |
| **Ambiguity**      | 0.18  | ≤0.20| ✓      | Below gate                                                           |

Status: ✓ = met minimum, ⚠ = below minimum (planner treats as assumption)

## Interview Log

`--auto` mode — Socratic interview skipped per user instruction ("work without stopping for clarifying questions"). The following decisions were auto-selected by Claude based on codebase scouting + project conventions. **Review and override before promoting to a phase.**

| Round | Perspective       | Question (implicit)                                          | Auto-selected decision                                                                                       |
|-------|-------------------|--------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------|
| 1     | Researcher        | What's the sync gap between ratings and reviews today?       | review→list works (`review.go:~93`); list→review is the missing direction. Both MAL/Shikimori imports only touch `anime_list`. |
| 1     | Researcher        | Does a comments concept exist anywhere?                      | No. No table, no service, no API, no UI. Greenfield feature.                                                 |
| 2     | Simplifier        | Minimum viable denormalization shape?                        | Empty `review_text` for rating-only entries (reuses existing renderer that already gates body on `v-if="review.review_text"`). |
| 2     | Simplifier        | Minimum viable comments shape?                               | Flat list. `parent_id` reserved in schema for future threading but always NULL in v1. No votes, no markdown. |
| 3     | Boundary Keeper   | What's tempting to scope-creep that we should exclude?       | Replies/threading, votes, markdown, mentions, moderation queue, real-time updates — all deferred (see Out of scope). |
| 3     | Boundary Keeper   | What does "done" look like at the UI level?                  | Two tabs in Anime.vue, URL-persisted via `?ugc=`, count badges, provenance badges on rating-only reviews.    |
| 4     | Failure Analyst   | What's the worst-case data hazard?                           | Backfill running twice and double-inserting. Mitigation: idempotency check on `(user_id, anime_id)` before insert. |
| 4     | Failure Analyst   | What aggregate could silently break?                         | `GET /api/anime/:id/rating` includes rating-only rows once they're real reviews — this is desired behavior, not a bug. Existing UI label says "(N reviews)" — N now correctly counts rating-only entries too. |
| 5     | Seed Closer       | Source enum values?                                          | `website`, `list`, `mal`, `shikimori`. Default `website` for backfill of pre-existing reviews.               |
| 5     | Seed Closer       | Comment rate-limit threshold?                                | 10 per user per anime per hour. Conservative; can be tuned post-launch.                                      |

**Decisions most likely to need user override before phase promotion:**
1. Rate-limit threshold (10/hour) — pure guess; user may have a stronger opinion.
2. Whether rating-only reviews should display the user's name with a "via MAL" badge, or stay anonymized to "imported rating." Auto-selected: show name + badge.
3. Whether comments need email/Telegram notifications to the anime's original reviewers / earlier commenters. Auto-selected: no (deferred to notifications engine).
4. Whether to keep `reviews.source` column at all, or derive provenance from a join against import history. Auto-selected: dedicated column (cheap, queryable, no join).

---

*Backlog item — not yet on ROADMAP.md.*
*Spec created: 2026-05-13 (auto mode)*
*Next step when promoted:* `/gsd-phase add` to slot into a milestone → move this file to `.planning/phases/<N>-social-reviews-comments/<NN>-SPEC.md` → `/gsd-discuss-phase <N>`.
