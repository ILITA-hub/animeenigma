# Roadmap: AnimeEnigma `social` workstream

**Workstream:** social (parallel to root v3.0 Universal Anime Scraper)
**Active milestone:** v0.1 Social: Reviews + Comments
**Phase numbering:** Workstream-local — starts at 1, independent of root project numbering.

## Milestones

- 🟢 **v0.1 Social: Reviews + Comments** — Phase 1 (planning)

## Phases

### Phase 1: Reviews + Ratings + Comments

**Goal:** Eliminate the `reviews` table by merging review text into `anime_list` (single source of truth for score and text), refactor reviews endpoints to read/write `anime_list`, add a new `comments` table + CRUD endpoints, and tab the Reviews section of the anime detail page into `Reviews | Comments`.

**Depends on:** Nothing in this workstream. Touches only `services/player/`, `services/gateway/internal/transport/router.go`, and `frontend/web/src/views/Anime.vue` (+ `ActivityFeed.vue`, `client.ts`, three locale files) — no overlap with v3.0 scraper work in root milestone.

**Requirements:** SOCIAL-01, SOCIAL-02, SOCIAL-03, SOCIAL-04, SOCIAL-05, SOCIAL-06, SOCIAL-NF-01, SOCIAL-NF-02

**Success Criteria** (what must be TRUE — distilled from `01-SPEC.md` Acceptance Criteria):

  1. `\d anime_list` shows `review_text` and `username` columns; `\d reviews` returns "Did not find any relation"; no Go code references `domain.Review` or `ReviewRepository`.
  2. Migration is idempotent — running it twice produces no data changes on the second run. Every former row in `reviews` has a matching `anime_list` row with identical `(score, review_text, username)`. No data loss.
  3. All six reviews endpoints return JSON identical in shape to pre-migration. Verified by golden-file diff. Existing frontend files (`Anime.vue`, `Home.vue`, `Browse.vue`, `ActivityFeed.vue`, `useSiteRatings.ts`) compile and render unchanged for the schema swap.
  4. A MAL-imported `score=8` for user X on anime Y appears in `GET /api/anime/Y/reviews` as a review by user X with score 8 and empty `review_text` (side-effect of req 3 — no extra code).
  5. Comments CRUD (4 endpoints) returns correct status codes for happy-path + one failure case each. Soft-deleted comments excluded from list responses. 11th comment in an hour returns 429.
  6. Posting N comments produces N rows in `activity_events` with `type='comment'`.
  7. `/anime/<id>?ugc=comments` opens the Comments tab on first paint; switching tabs updates the URL; reload preserves tab. Logged-out users see comment list + login prompt instead of textarea.
  8. All new UI strings have translations for the three project locales.

**SPEC:** `phases/01-social-reviews-comments/01-SPEC.md` (ambiguity 0.15, 6 requirements + 2 NF, --auto mode)

**Plans:** 7 plans across 5 waves (Wave 0 scaffolding + Waves 1-4 implementation)

Plans:
- [ ] 00-PLAN.md — Wave 0 test scaffolding (Go test stubs + Playwright spec + reviews-fixture script)
- [ ] 01-PLAN.md — Schema extension (AnimeListEntry + Comment domain) + one-shot idempotent migration
- [ ] 02-PLAN.md — Reviews API refactor to ListRepository + handler projection (shape preservation) + delete ReviewRepository / domain.Review
- [ ] 03-PLAN.md — Comments backend stack: CommentRepository (cursor pagination + soft delete) + CommentService (validation, rate limit, activity emit) + CommentHandler (4 endpoints)
- [ ] 04-PLAN.md — Router wiring: main.go comment pipeline + player chi routes + gateway proxy routes (before /anime/* catch-all)
- [ ] 05-PLAN.md — Frontend plumbing: commentApi in client.ts + 24 anime.ugc.* locale keys in en/ja/ru + ActivityFeed comment branch
- [ ] 06-PLAN.md — Anime.vue tab strip + Comments UI (form, list, edit, delete, load-more, login prompt, empty state) + Playwright e2e tests

**Wave structure:**

| Wave | Plans | Parallelizable | Autonomous |
|------|-------|----------------|------------|
| 0    | 00    | n/a            | yes        |
| 1    | 01    | n/a            | no (checkpoint after deploy) |
| 2    | 02, 03 | yes (no file overlap) | 02: no / 03: yes |
| 3    | 04, 05 | yes (no file overlap) | 04: no / 05: yes |
| 4    | 06    | n/a            | no (checkpoint after deploy) |

**UI hint:** yes (tabs + comments form on Anime.vue)
