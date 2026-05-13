# Phase 1: Reviews + Ratings + Comments - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-accept (autonomous; SPEC ambiguity 0.15 — all key decisions pre-locked in `01-SPEC.md`)

<domain>
## Phase Boundary

Drop the `reviews` table; merge `review_text` and `username` into `anime_list` so a single row per `(user_id, anime_id)` carries both score and (optional) review text. Refactor the six reviews endpoints to read/write `anime_list` with identical JSON response shape. Add a new `comments` table + CRUD endpoints (GET / POST / PATCH / DELETE) with body 1–2000 chars, 10/hour/user/anime rate limit, soft delete, cursor pagination 50/page newest-first. Emit `activity_events` rows of `type='comment'` on creation (no per-day dedup). On the anime detail page, split the current Reviews section into a two-tab strip (`Reviews ({count})` | `Comments ({count})`), URL-persisted via `?ugc=reviews|comments`. Touches `services/player/` and `frontend/web/src/views/Anime.vue` only.

</domain>

<decisions>
## Implementation Decisions

### Schema (storage layer)

- `anime_list` gains `review_text text NOT NULL DEFAULT ''` and `username varchar(32) NOT NULL DEFAULT ''`. Both non-null with empty default so existing rows pre-migration remain valid.
- The `reviews` table is dropped (not renamed, not archived).
- The `comments` table has columns: `id uuid PK`, `user_id uuid`, `anime_id uuid`, `username varchar(32)`, `body text`, `parent_id uuid nullable` (reserved, always NULL in v0.1), `deleted_at timestamp nullable` (soft delete), `created_at`, `updated_at`. Indexes on `(anime_id, created_at DESC)` and `(user_id, created_at DESC)`.
- `username` denormalized onto `anime_list` and `comments` rows (mirrors the existing `reviews.username` pattern; avoids JOIN on every reviews-list / comments-list render).

### Migration

- One-shot bootstrap function called once at player service startup. Idempotency guard = "does `reviews` table still exist?" — if no, skip.
- Order of operations:
  1. `AutoMigrate(&AnimeListEntry{})` adds the new columns.
  2. For every `reviews` row, upsert into `anime_list` on `(user_id, anime_id)`: copy `score` (only if existing `anime_list.score = 0`), copy `review_text`, copy `username`. If no `anime_list` row exists, create one with `status='completed'`.
  3. Backfill `username` for any remaining `anime_list` rows by JOINing `users`.
  4. `AutoMigrate(&Comment{})` creates the comments table.
  5. `DROP TABLE reviews;`
- Forward-only — no rollback path. Rely on standard ops backup.
- No-downtime rollout: deploy with single replica or drain old replicas before rollout (acceptable per the SPEC's `Constraints` section).

### Reviews API

- Same six routes, same request / response JSON shape. The frontend (`reviewApi.ts`, `Anime.vue`, `Home.vue`, `Browse.vue`, `ActivityFeed.vue`, `useSiteRatings.ts`) requires zero changes for the schema swap.
- List filter on the public reviews list: `WHERE anime_id = ? AND (score > 0 OR review_text != '')` — so MAL-imported `score=8` rows automatically appear.
- `POST /api/anime/:id/reviews` becomes an UPSERT on `anime_list` setting `score` + `review_text` (and `status='completed'` if no row existed before).
- `DELETE /api/anime/:id/reviews` clears BOTH `score` and `review_text` (matches today's "deleting your review" semantics — auto-selected per SPEC Interview Log).
- The Go domain type `Review` and `ReviewRepository` are deleted from the codebase. The service layer `ReviewService` is refactored to call `ListRepository`.

### Comments API

- Four routes:
  - `GET    /api/anime/:id/comments?cursor=&limit=50` — paginated, newest first, excludes soft-deleted, public.
  - `POST   /api/anime/:id/comments` — auth required, body 1–2000 chars (UTF-8), rate-limited 10 per user per anime per hour, returns 429 on excess.
  - `PATCH  /api/anime/:id/comments/:cid` — auth required, owner only (returns 403 otherwise), body 1–2000 chars.
  - `DELETE /api/anime/:id/comments/:cid` — auth required, owner OR admin, soft delete (sets `deleted_at`).
- Empty / whitespace-only bodies return 400.
- Cursor format: opaque base64-encoded `(created_at, id)` tuple. Newest first; consumer passes the last item's cursor back to get the next page.
- Rate limit is per-user-per-anime-per-hour (not global per-user). Implementation: per-user-anime in-memory bucket inside the comments service; refresh per process. Acceptable because the player service runs as a single replica today, and the value is "soft" (10/hour is a guardrail, not a hard contract).
- Comments emit `activity_events` rows on creation with `type='comment'`, `content` = first 300 runes of body with `…` suffix if truncated, `anime_id`, `user_id`, `username`. NO per-day dedup (unlike reviews) — every comment emits an event.

### Frontend (Anime.vue)

- Wrap the existing Reviews section (lines 590–705) in a tabbed surface using the existing `components/ui/Tabs.vue` component (no new tab component).
- Two tabs:
  - `Reviews ({count})` — count from `reviews.length`, current existing content unchanged.
  - `Comments ({count})` — count from `commentsTotal`, new content (textarea + Post button + list + Load more + edit/delete actions on own comments).
- URL state via Vue Router query param `?ugc=reviews` (default) or `?ugc=comments`. Default value is "reviews" — first paint with no query renders Reviews.
- Tab switch updates the URL via `router.replace` (no history entry — back button skips through tabs).
- Anonymous user on Comments tab: see the comment list + a login prompt CTA in place of the textarea (mirrors today's behavior on the review form).
- Comment list: newest first, paginated via "Load more" button below the last item. Each comment shows username + relative timestamp + body (`whitespace-pre-wrap`) + edit / delete pencil/trash icons on the user's own comments.
- Edit mode: clicking the pencil swaps the body text for an inline textarea + Save / Cancel buttons. No modal.
- Delete: confirm via `window.confirm` (matches existing review delete UX; no separate dialog).
- New locale keys for the three locales (`en.json`, `ja.json`, `ru.json`). Key prefix: `anime.ugc.*` (e.g. `anime.ugc.reviewsTab`, `anime.ugc.commentsTab`, `anime.ugc.postComment`, `anime.ugc.editComment`, `anime.ugc.deleteCommentConfirm`, `anime.ugc.loadMore`, `anime.ugc.loginToComment`, `anime.ugc.emptyComments`, `anime.ugc.commentPlaceholder`).

### Activity feed

- Existing `ActivityFeed.vue` already renders `review` events. It now also receives `comment` events. Add a minimal renderer for `comment` events (same shape as `review`, but the icon and label differ). If `ActivityFeed.vue` does not branch on `type`, this is purely additive — comment events appear with a generic label.
- No change to dedup logic for reviews; comments simply emit on every create.

### Claude's Discretion

- Choice of GORM `AutoMigrate` hook ordering vs a separate `migrations/social-merge.go` file — implementer picks whichever fits cleanly. The SPEC requires the migration to be one-shot + idempotent; exact module placement is a planning detail.
- Exact wording of the locale strings — implementer drafts the EN copy and translates to JA / RU using existing locale conventions in the file.
- Whether the Tabs component variant is `default`, `underline`, or `pills` — implementer chooses based on visual fit with the existing Anime.vue header. `underline` is the typical UGC tab style.
- Whether the rate-limit bucket is implemented inline in the comments service or extracted into a small `libs/ratelimit` helper. If a similar pattern exists elsewhere in the player service, reuse it; otherwise inline is fine.
- Whether the cursor is `base64(created_at|id)` or `base64({created_at,id})` JSON — implementer picks. Stays opaque to the frontend.
- Test scope: at minimum, one unit test per new endpoint covering happy-path + one failure case (per SPEC). Implementer adds golden-file JSON-shape diff tests for the six reviews endpoints if a fixture pattern exists in the player service, else relies on existing handler tests.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `services/player/internal/domain/watch.go:58-79` — `AnimeListEntry` struct + `TableName()`; extend in-place with `ReviewText` and `Username` fields.
- `services/player/internal/domain/watch.go:105-119` — `Review` struct; delete after migration runs.
- `services/player/internal/domain/activity.go:9-18` — `ActivityEvent`; reuse for `type='comment'`.
- `services/player/internal/repo/review.go` — refactor in place to query `anime_list`. Public API and method names preserved so handler code is stable.
- `services/player/internal/service/review.go:20-90` — `ReviewService` already calls `ActivityRepository.UpdateOrCreate` for review activity events; mirror the pattern for comments (without the per-day dedup).
- `services/player/internal/handler/report.go` — example of POST + Telegram side-effect pattern (not directly reused, but a reference for handler structure with body validation + auth check + service call).
- `services/player/internal/transport/router.go:165-176` — anime-reviews route group; add a sibling `comments` group inline.
- `frontend/web/src/components/ui/Tabs.vue` — existing tab component with `tabs[]`, `modelValue`, `variant`, `count` per-tab built-in.
- `frontend/web/src/views/Anime.vue:590-705` — current Reviews section to wrap in the tab strip.
- `frontend/web/src/api/client.ts:338` — `reviewApi`; add a sibling `commentApi` with `getAnimeComments`, `createComment`, `updateComment`, `deleteComment`.

### Established Patterns
- GORM `AutoMigrate(&domain.X{})` in `cmd/player-api/main.go` for schema setup. Migration is one-shot in main before `srv.Start()`.
- Repository → Service → Handler layering per `services/player/internal/{repo,service,handler}/`.
- JSON response shape: lowercase snake_case keys via GORM struct tags. Time fields ISO 8601 via `time.Time` default Go encoding.
- Auth-required routes nested under the `r.Group(...)` with `chi.Use(authMiddleware)` (see `transport/router.go`).
- Frontend i18n via `vue-i18n`'s `$t('namespace.key')`. Locale JSON files mirrored across `en.json`, `ja.json`, `ru.json`.
- Glass-card class pattern (`glass-card p-4`) used for content cards across `Anime.vue`.
- Activity events deduped per-day in `service/review.go:50-86` for reviews. Comments will skip the dedup branch and call `activityRepo.Create` directly.

### Integration Points
- Migration: `cmd/player-api/main.go` runs `AutoMigrate(&AnimeListEntry{}, &Comment{}, ...)` then the bootstrap copy function.
- Router: `services/player/internal/transport/router.go` — add `r.Route("/comments", ...)` siblings to the existing reviews routes.
- API client: `frontend/web/src/api/client.ts` — add `commentApi` next to `reviewApi`.
- Anime detail view: `frontend/web/src/views/Anime.vue` — wrap the existing Reviews section in `<Tabs>`, mount Comments tab content, wire `route.query.ugc` ↔ active tab via watcher + `router.replace`.
- Activity feed: `frontend/web/src/components/ActivityFeed.vue` — receives `type='comment'` events automatically via the existing feed query.

</code_context>

<specifics>
## Specific Ideas

- Tab badge counts use the same pill style as `Tabs.vue`'s built-in count prop (`bg-white/10 rounded-full ml-2 px-1.5 py-0.5 text-xs`).
- Reviews tab is the default tab (matches today's behavior — Reviews is what users see now).
- URL persistence uses `router.replace({ query: { ...route.query, ugc: 'comments' }})` so back/forward navigation doesn't accumulate intermediate tab states.
- Comment edit window: unlimited (no time-bound restriction). Users can edit / delete their own comments any time. Admins can delete any comment at any time.
- Soft-deleted comments excluded from GET responses; the row stays in DB for ops / audit.
- Activity event content preview = first 300 runes of comment body with `…` suffix if truncated (matches the SPEC's req 5 spec).

</specifics>

<deferred>
## Deferred Ideas

- Comment replies / threading — `comments.parent_id` reserved but NULL in v0.1. Out of scope.
- Voting / likes on reviews or comments — separate engagement feature.
- Markdown / rich text in comments — plain text only.
- Mentions / notifications (`@username` parsing, notification fan-out) — requires the notification engine. See `memory/project_notifications_engine.md`.
- Comment moderation queue / flagging UI — admin can soft-delete via DELETE endpoint; no queue.
- Real-time updates (WebSocket / SSE / polling) — list refetches on tab activation + after post; that's it.
- Pagination of reviews — reviews stay un-paginated.
- Migration rollback path — forward-only.

</deferred>
