# Roadmap: AnimeEnigma `social` workstream

**Workstream:** social (parallel to root v3.0 Universal Anime Scraper)
**Active milestone:** v0.1 Social: Reviews + Comments
**Phase numbering:** Workstream-local — starts at 1, independent of root project numbering.

## Milestones

- 🟢 **v0.1 Social: Reviews + Comments** — Phase 1 (planning)

## Phases

### Phase 1: Reviews + Ratings + Comments

**Goal:** Eliminate the `reviews` table by merging review text into `anime_list` (single source of truth for score and text), refactor reviews endpoints to read/write `anime_list`, add a new `comments` table + CRUD endpoints, and tab the Reviews section of the anime detail page into `Reviews | Comments`.

**Depends on:** Nothing in this workstream. Touches only `services/player/` and `frontend/web/src/views/Anime.vue` — no overlap with v3.0 scraper work in root milestone.

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
**Plans:** TBD — produced by `/gsd-plan-phase 1 --ws social`
**UI hint:** yes (tabs + comments form on Anime.vue)
