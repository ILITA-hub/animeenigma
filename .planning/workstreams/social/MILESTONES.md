# Milestones — `social` workstream

## v0.1 Social: Reviews + Comments (shipped 2026-05-13)

**Phases:** 1 (Reviews + Ratings + Comments) — 7 plans across 6 waves.
**Audit:** `milestones/v0.1-MILESTONE-AUDIT.md` — status: passed, 8/8 requirements satisfied, 14/14 integrations wired, 8/8 E2E flows complete.

### Delivered

- Dropped the `reviews` table; merged review text + username into `anime_list` as a single row per `(user_id, anime_id)`. One-shot idempotent migration copies all existing reviews into `anime_list` and drops the old table on player service startup.
- Refactored the 6 review endpoints to read/write `anime_list`. Byte-identical JSON wire shape preserved via a handler-local 7-field `reviewResponse` projection struct (`TestReviewHandler_*ShapeIsExactly7Fields` x3 enforces). Frontend `reviewApi` untouched.
- Added a `comments` table + 4 CRUD endpoints (GET / POST / PATCH / DELETE) with body 1–2000 UTF-8 runes, soft delete, cursor pagination 50/page newest-first, and a 10/hour/user/anime rate limit returning 429.
- Posting a comment emits an `activity_events` row with `type='comment'` (no per-day dedup, unlike reviews) so followers see new comments in their feed.
- Tabbed the Reviews section on `frontend/web/src/views/Anime.vue` into `Reviews | Comments` using the existing `Tabs.vue` (variant `underline`). URL persistence via `?ugc=reviews|comments` with `router.replace`. Anonymous users see a comment list + login CTA in place of the textarea.
- 24 `anime.ugc.*` locale keys + 1 `activity.comment.posted` key shipped across en/ja/ru. ActivityFeed.vue branch renders comment events with the new locale string. Zero `[intlify]` warnings.
- Gateway routes `/anime/{animeId}/comments*` BEFORE the `/anime/*` catch-all to catalog (RESEARCH.md Pitfall 1 mitigated). JWT defense-in-depth applied on the three mutation routes.
- 4 Playwright e2e tests cover the four interaction contracts (deep-link, URL persistence, anon login prompt, logged-in CRUD lifecycle) — all green in 5.5s.

### Stats

- **Phases:** 1
- **Plans:** 7 (Wave 0 test scaffolding → Wave 5 Anime.vue tabs + e2e + after-update)
- **Tasks:** ~24
- **Commits:** ~40 across the milestone (feat / test / fix / refactor / docs)
- **Files touched:** ~25 (player domain/repo/service/handler + gateway router + frontend Anime.vue + ActivityFeed.vue + client.ts + 3 locales + e2e spec)
- **Test coverage added:** 16 new repo + 4 service + 3 handler unit tests + 1 migration idempotency test + 4 Playwright e2e tests

### Known deferred items

- **WR-02 (rate-limiter NTP clock-correction):** Current `time.Now()` approach acceptable for single-replica deploy; real fix is a Redis-backed bucket. Deferred to a future milestone.
- **SOCIAL-NF-01 letter vs spirit:** Golden-file pre-image was never captured before migration deployed. Functional contract enforced by 3 unit-level shape tests; the literal "golden-file diff" audit trail is permanently unavailable.
- **Pre-existing API-key auth bug:** `JWTValidationMiddleware` 401s `ak_` bearer tokens on `/anime/{animeId}/*` route family (works on `/api/users/*`). Out of scope for this milestone; affects only automated tests, real users use JWT cookies.
- **UI review (advisory, 18/24):** 2 blockers + 9 warnings in `phases/01-social-reviews-comments/01-UI-REVIEW.md` — `loadMoreFailed` error message unreachable, `Tabs.vue` base class bakes in `font-medium` violating the 2-weight rule, "Save edit" button reuses "Posting…" copy. Worth a small follow-up patch.

### Code review

- 5 Critical + 6 Warning + 3 Info findings on the standard-depth review.
- Fix pass applied 10 of 11 in-scope findings (5 CR + 5 WR). WR-02 deferred with rationale.

---

*Archive references:*
- `milestones/v0.1-ROADMAP.md` — full milestone roadmap
- `milestones/v0.1-REQUIREMENTS.md` — final requirements state (all 8 marked Done)
- `milestones/v0.1-MILESTONE-AUDIT.md` — pre-close audit
