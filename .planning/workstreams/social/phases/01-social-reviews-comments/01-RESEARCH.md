# Phase 1: Reviews + Ratings + Comments — Research

**Researched:** 2026-05-13
**Domain:** Backend (Go/GORM/Chi) schema consolidation + new CRUD feature; Frontend (Vue 3) tabbed UI on an existing view
**Confidence:** HIGH — every architectural decision is anchored to a real file in this repo; no new external libraries; the SPEC is locked at ambiguity 0.15 with all design choices pre-decided.

## Summary

This is a single-service refactor + greenfield feature contained entirely within `services/player/` and `frontend/web/src/views/Anime.vue`. The hard parts are already de-risked:

1. **Schema consolidation** has a direct precedent (`cmd/player-api/main.go:188-213` "phase 3 backfill") — the same "one-shot block guarded by a `SELECT 1 FROM ... LIMIT 1` check + `Exec` raw SQL" pattern fits the reviews→anime_list copy perfectly.
2. **Comments CRUD** mirrors the existing reviews implementation 1:1 — domain struct + repo + service + handler + router group; the `ReviewService.CreateOrUpdateReview` activity-event emission (`service/review.go:50-86`) is the template, minus the per-day dedup branch.
3. **Cursor pagination** — `libs/pagination/cursor.go` already provides a `Cursor{ID, Timestamp}` + `Encode/DecodeCursor` primitive used elsewhere; `ActivityRepository.GetFeed` (`repo/activity.go:54-81`) is the existing precedent for "newest-first, cursor-based, fetch limit+1 to detect hasMore" — repurpose the exact pattern.
4. **Auth-gated route mounting** — chi's existing `r.Group(func(r chi.Router) { r.Use(AuthMiddleware(jwtConfig)); ... })` pattern in `transport/router.go:172-177` is the template for the comments protected sub-group.
5. **Rate limiting** — there is NO existing per-user-per-resource bucket in this codebase. The gateway has only an IP-keyed `IPRateLimiter` (`services/gateway/internal/transport/router.go:460-514`). For 10/hour/user/anime we roll a small in-memory bucket inline in the comments service (acceptable: player runs single-replica today, and the SPEC explicitly accepts this).
6. **Frontend Tabs** — `components/ui/Tabs.vue` already supports `variant="underline"`, per-tab `count` badge, and `v-model` two-way binding; no new component needed.
7. **Gateway routing** — a non-obvious requirement: `services/gateway/internal/transport/router.go:144-149` defines explicit player-bound routes for `/anime/{animeId}/reviews*` BEFORE the catch-all `/anime/*` → catalog. The new `/anime/{animeId}/comments*` routes MUST be registered the same way; otherwise they'll be misrouted to catalog.

**Primary recommendation:** Build this in the order (Wave 0 tests scaffolding) → (Wave 1 backend: schema + migration + comments domain) → (Wave 2 backend: reviews refactor + comments CRUD + activity event) → (Wave 3 gateway routes + frontend API client + locales) → (Wave 4 Anime.vue tab strip + comments UI). The reviews-refactor and the comments-CRUD can be developed in parallel by different agents because they share no functions — the `ReviewService` keeps its public method signatures, and `CommentService` is wholly new.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Schema (storage layer)

- `anime_list` gains `review_text text NOT NULL DEFAULT ''` and `username varchar(32) NOT NULL DEFAULT ''`. Both non-null with empty default so existing rows pre-migration remain valid.
- The `reviews` table is dropped (not renamed, not archived).
- The `comments` table has columns: `id uuid PK`, `user_id uuid`, `anime_id uuid`, `username varchar(32)`, `body text`, `parent_id uuid nullable` (reserved, always NULL in v0.1), `deleted_at timestamp nullable` (soft delete), `created_at`, `updated_at`. Indexes on `(anime_id, created_at DESC)` and `(user_id, created_at DESC)`.
- `username` denormalized onto `anime_list` and `comments` rows (mirrors the existing `reviews.username` pattern; avoids JOIN on every reviews-list / comments-list render).

#### Migration

- One-shot bootstrap function called once at player service startup. Idempotency guard = "does `reviews` table still exist?" — if no, skip.
- Order of operations:
  1. `AutoMigrate(&AnimeListEntry{})` adds the new columns.
  2. For every `reviews` row, upsert into `anime_list` on `(user_id, anime_id)`: copy `score` (only if existing `anime_list.score = 0`), copy `review_text`, copy `username`. If no `anime_list` row exists, create one with `status='completed'`.
  3. Backfill `username` for any remaining `anime_list` rows by JOINing `users`.
  4. `AutoMigrate(&Comment{})` creates the comments table.
  5. `DROP TABLE reviews;`
- Forward-only — no rollback path. Rely on standard ops backup.
- No-downtime rollout: deploy with single replica or drain old replicas before rollout (acceptable per the SPEC's `Constraints` section).

#### Reviews API

- Same six routes, same request/response JSON shape. The frontend (`reviewApi.ts`, `Anime.vue`, `Home.vue`, `Browse.vue`, `ActivityFeed.vue`, `useSiteRatings.ts`) requires zero changes for the schema swap.
- List filter on the public reviews list: `WHERE anime_id = ? AND (score > 0 OR review_text != '')` — so MAL-imported `score=8` rows automatically appear.
- `POST /api/anime/:id/reviews` becomes an UPSERT on `anime_list` setting `score` + `review_text` (and `status='completed'` if no row existed before).
- `DELETE /api/anime/:id/reviews` clears BOTH `score` and `review_text` (matches today's "deleting your review" semantics — auto-selected per SPEC Interview Log).
- The Go domain type `Review` and `ReviewRepository` are deleted from the codebase. The service layer `ReviewService` is refactored to call `ListRepository`.

#### Comments API

- Four routes:
  - `GET    /api/anime/:id/comments?cursor=&limit=50` — paginated, newest first, excludes soft-deleted, public.
  - `POST   /api/anime/:id/comments` — auth required, body 1–2000 chars (UTF-8), rate-limited 10 per user per anime per hour, returns 429 on excess.
  - `PATCH  /api/anime/:id/comments/:cid` — auth required, owner only (returns 403 otherwise), body 1–2000 chars.
  - `DELETE /api/anime/:id/comments/:cid` — auth required, owner OR admin, soft delete (sets `deleted_at`).
- Empty / whitespace-only bodies return 400.
- Cursor format: opaque base64-encoded `(created_at, id)` tuple. Newest first; consumer passes the last item's cursor back to get the next page.
- Rate limit is per-user-per-anime-per-hour (not global per-user). Implementation: per-user-anime in-memory bucket inside the comments service; refresh per process. Acceptable because the player service runs as a single replica today, and the value is "soft" (10/hour is a guardrail, not a hard contract).
- Comments emit `activity_events` rows on creation with `type='comment'`, `content` = first 300 runes of body with `…` suffix if truncated, `anime_id`, `user_id`, `username`. NO per-day dedup (unlike reviews) — every comment emits an event.

#### Frontend (Anime.vue)

- Wrap the existing Reviews section (lines 590–705) in a tabbed surface using the existing `components/ui/Tabs.vue` component (no new tab component).
- Two tabs: `Reviews ({count})` and `Comments ({count})`.
- URL state via Vue Router query param `?ugc=reviews` (default) or `?ugc=comments`.
- Tab switch updates the URL via `router.replace` (no history entry — back button skips through tabs).
- Anonymous user on Comments tab: see the comment list + a login prompt CTA in place of the textarea.
- Comment list: newest first, paginated via "Load more" button below the last item. Each comment shows username + relative timestamp + body (`whitespace-pre-wrap`) + edit/delete pencil/trash icons on the user's own comments.
- Edit mode: clicking the pencil swaps the body text for an inline textarea + Save / Cancel buttons. No modal.
- Delete: confirm via `window.confirm`.
- New locale keys for the three locales (`en.json`, `ja.json`, `ru.json`). Key prefix: `anime.ugc.*`. Full key list in 01-UI-SPEC.md.

#### Activity feed

- Existing `ActivityFeed.vue` already renders `review` events. It now also receives `comment` events. Add a minimal renderer for `comment` events (same shape as `review`, but the icon and label differ). If `ActivityFeed.vue` does not branch on `type`, this is purely additive — comment events appear with a generic label.
- No change to dedup logic for reviews; comments simply emit on every create.

### Claude's Discretion

- Choice of GORM `AutoMigrate` hook ordering vs a separate `migrations/social-merge.go` file — implementer picks whichever fits cleanly. The SPEC requires the migration to be one-shot + idempotent; exact module placement is a planning detail.
- Exact wording of the locale strings — implementer drafts the EN copy and translates to JA / RU using existing locale conventions.
- Whether the Tabs component variant is `default`, `underline`, or `pills` — UI-SPEC locks `underline`.
- Whether the rate-limit bucket is implemented inline in the comments service or extracted into a small `libs/ratelimit` helper. If a similar pattern exists elsewhere in the player service, reuse it; otherwise inline is fine. **Research finding: no existing per-user-per-resource bucket → inline implementation.**
- Whether the cursor is `base64(created_at|id)` or `base64({created_at,id})` JSON — implementer picks. **Research finding: `libs/pagination/cursor.go` already provides a JSON-encoded `Cursor` struct — reuse it.**
- Test scope: at minimum, one unit test per new endpoint covering happy-path + one failure case (per SPEC). Implementer adds golden-file JSON-shape diff tests for the six reviews endpoints if a fixture pattern exists in the player service, else relies on existing handler tests. **Research finding: no fixture pattern exists; rely on handler tests + a single capture-then-diff smoke check.**

### Deferred Ideas (OUT OF SCOPE)

- Comment replies / threading — `comments.parent_id` reserved but NULL in v0.1.
- Voting / likes on reviews or comments.
- Markdown / rich text in comments — plain text only.
- Mentions / notifications (`@username` parsing, notification fan-out).
- Comment moderation queue / flagging UI — admin can soft-delete via DELETE endpoint; no queue.
- Real-time updates (WebSocket / SSE / polling) — list refetches on tab activation + after post.
- Pagination of reviews — reviews stay un-paginated.
- Migration rollback path — forward-only.
- Provenance column on reviews — `anime_list.mal_id` already signals MAL provenance.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SOCIAL-01 | Drop `reviews` table; add `review_text` + `username` columns to `anime_list`. | GORM `AutoMigrate` adds columns idempotently — see `cmd/player-api/main.go:50-65` for the existing `AutoMigrate` block. Domain struct tag pattern at `domain/watch.go:58-79` (AnimeListEntry); mirror the `reviews.review_text` column annotation pattern from `domain/watch.go:112`. |
| SOCIAL-02 | One-shot idempotent migration copies `reviews.review_text` + `reviews.username` into `anime_list`. | Direct precedent at `cmd/player-api/main.go:188-213` ("phase 3 backfill"): wrap a block in `{ }`, check a marker row with `SELECT 1 FROM ... LIMIT 1`, on empty run `Exec` raw SQL, on non-empty short-circuit. Same pattern fits reviews→anime_list copy. Idempotency guard = `SELECT 1 FROM information_schema.tables WHERE table_name = 'reviews' LIMIT 1`. |
| SOCIAL-03 | All six reviews endpoints read/write `anime_list` with preserved response shape. | `service/review.go:30-132` — refactor each method to call `ListRepository` instead of `ReviewRepository`. The frontend `Review` TypeScript interface (`Anime.vue:816-824`) requires exactly: `id`, `user_id`, `anime_id`, `username`, `score`, `review_text`, `created_at` — all already present on `AnimeListEntry` post-migration. The `Preload("Anime")` works identically because both `Review` and `AnimeListEntry` define `Anime *AnimeInfo` with the same `foreignKey:AnimeID` annotation. |
| SOCIAL-04 | New `comments` table + four CRUD endpoints with body 1–2000 chars, 10/hour/user/anime rate limit, soft-delete, cursor pagination 50/page newest-first. | Domain struct mirroring `Review` (`domain/watch.go:105-119`); repo mirroring `repo/review.go`; handler mirroring `handler/review.go`. Cursor pagination via `libs/pagination/cursor.go` (already in `go.mod` of player — verify); `ActivityRepository.GetFeed` (`repo/activity.go:54-81`) is the existing template for cursor-based newest-first lists with `Limit(limit+1)` hasMore detection. Soft delete via `gorm.DeletedAt` (existing precedent: `domain/activity.go:20`). |
| SOCIAL-05 | Posting a comment emits an `activity_events` row with `type='comment'` (no per-day dedup). | `service/review.go:50-86` is the template. Strip the dedup branch (lines 74-82): just call `s.activityRepo.Create(ctx, commentEvent)`. The `Content` field (truncate to 300 runes + `…`) is identical to lines 54-57. |
| SOCIAL-06 | Two-tab strip on `Anime.vue` Reviews section, `?ugc=` URL-persisted, auth-gated textarea, login prompt for anonymous. | `components/ui/Tabs.vue` provides `variant="underline"` + per-tab `count` + `v-model`. `useRoute()`/`useRouter()` already imported (`Anime.vue:832-833`). `route.query.ugc` ↔ local ref via two watchers (`route → ref` for back-button compat + `ref → router.replace` for click-handler). |
| SOCIAL-NF-01 | No frontend changes for reviews schema swap (verified by golden-file diff). | Response shape is preserved because `AnimeListEntry` already carries every field the frontend reads. The only new field in responses is `notes` (which has always existed on `anime_list`) — JSON has it serialized always, but the frontend Anime.vue Review interface (lines 816-824) does not declare it, and TypeScript permits excess properties — no compile break. **Risk:** Other unmentioned consumers (e.g. `useSiteRatings.ts`) might break if extra fields confuse downstream JSON parsers. **Mitigation:** capture a JSON sample of each of the six review endpoints BEFORE deploy; diff after deploy; fix or call out any field-name regressions. |
| SOCIAL-NF-02 | GORM AutoMigrate handles the schema changes; data-migration step gated by idempotency check. | The bootstrap block sits in `main.go` between `db.AutoMigrate(...)` (around line 50-65) and `srv.Start()` (line 308) — same location as the phase-3 backfill at lines 188-213. |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Schema migration (drop reviews, extend anime_list, create comments) | Database / Storage | API / Backend (player) | GORM AutoMigrate runs in player's `main.go` against PostgreSQL; player service owns the `anime_list` and `reviews` schema today. |
| Reviews refactor (six endpoints querying `anime_list`) | API / Backend (player) | — | Pure player-service refactor of `ReviewService` → `ListRepository`. No frontend, no gateway, no other service change. |
| Comments CRUD (4 endpoints) | API / Backend (player) | API Gateway | Handlers + service + repo all in player. Gateway must add explicit routes for `/anime/{animeId}/comments*` ahead of the catch-all `/anime/*` → catalog. |
| Per-user-anime rate limit (10/hour) | API / Backend (player) | — | In-memory bucket inside the comments service. Not a gateway concern because the gateway only knows IP, not authenticated user/anime tuple. |
| Activity-event emission on comment create | API / Backend (player) | — | Existing `ActivityRepository.Create` in player; no fan-out service needed. |
| Tabbed UI on Anime detail | Browser / Client | — | Vue component-local state in `Anime.vue`. No SSR (project is Vue SPA + nginx static serve). |
| URL persistence (`?ugc=`) | Browser / Client | — | Vue Router `route.query.ugc` ↔ local ref via watchers. |
| API client (`commentApi`) | Browser / Client | — | Axios wrapper in `frontend/web/src/api/client.ts:338` siblings. |
| Locale strings (`anime.ugc.*`) | Browser / Client | — | Three JSON files under `frontend/web/src/locales/`. |
| ActivityFeed.vue rendering of comment events | Browser / Client | — | Single `if (event.type === 'comment')` branch in `actionText()` of `ActivityFeed.vue:142-159`. |

## Standard Stack

### Core (all already in this monorepo — no new deps)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.22 | Backend services | Project pin (`.tool-versions`) [VERIFIED: STACK.md] |
| GORM | 1.30.0 | ORM, AutoMigrate, upsert via `clause.OnConflict` | Project's ORM across all services [VERIFIED: STACK.md] |
| Chi v5 | v5.0.12 | HTTP router, route groups, middleware | Used in every service [VERIFIED: STACK.md] |
| `gorm.io/driver/postgres` | inherited via `libs/database` | PostgreSQL driver | Project default [VERIFIED: STACK.md] |
| `libs/pagination` | local | Cursor encoding via `Cursor{ID, Timestamp}.Encode()` + `DecodeCursor(s)` | Already in workspace; reuse [VERIFIED: `libs/pagination/cursor.go`] |
| `libs/errors` | local | Domain error types — `NotFound`, `InvalidInput`, `Unauthorized`, `Forbidden`, `RateLimited` | Project convention [VERIFIED: `libs/errors/errors.go`] |
| `libs/httputil` | local | Response helpers — `OK`, `Created`, `NoContent`, `BadRequest`, `Forbidden`, `TooManyRequests`, `Error`, `Bind` | Project convention [VERIFIED: `libs/httputil/response.go`] |
| `libs/authz` | local | JWT claim extraction (`ClaimsFromContext`, `IsAdmin`) | Used by every protected route in player [VERIFIED: `libs/authz/jwt.go:163-194`] |
| `libs/logger` | local | Structured zap logging via `Infow` / `Errorw` | Project convention [VERIFIED: STACK.md] |
| Vue 3 | 3.4.21 | Frontend framework | Project pin [VERIFIED: STACK.md] |
| Vue Router | 4.3.0 | `useRoute()` / `useRouter()` already imported in Anime.vue:771 | Project default [VERIFIED: STACK.md, `Anime.vue:771-832`] |
| vue-i18n | inherited | `$t('anime.ugc.commentsTab')` | Project convention [VERIFIED: `i18n.ts`] |
| Tailwind v4 | 4.1.18 | All styling | Project default [VERIFIED: STACK.md] |
| testify | v1.8.4 | Go test assertions | Project convention [VERIFIED: STACK.md] |
| `gorm.io/driver/sqlite` (test only) | (inherited) | In-memory test DB | Existing handler/repo test precedent [VERIFIED: `repo/sync_test.go:15-42`] |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `gorm.io/gorm/clause` | inherited | `clause.OnConflict` for upsert | Reviews refactor's UPSERT into `anime_list` (existing pattern: `repo/list.go:54-77`) |
| `gorm.DeletedAt` | inherited | Soft delete via index column | `Comment.DeletedAt` — see `domain/activity.go:20` for the exact tag |
| Native `time.Time` truncation | stdlib | "Within last 1 hour" rate-limit window | Mirrors `ActivityRepository.GetTodayByUserAnimeType` truncation pattern (`repo/activity.go:30`) |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Inline in-memory rate-limit bucket | `golang.org/x/time/rate.Limiter` (the lib used by gateway IP limiter) | `rate.Limiter` works in tokens-per-second + burst, not "N per hour per (user,anime)". For per-(user, anime) you'd still need to maintain a `sync.Map` of limiters keyed by `userID|animeID`. Net code volume is similar; the inline counter+timestamp approach is more honest about what's happening and is easier to test. **Use a hand-rolled bucket.** |
| Reusing `libs/pagination.Cursor` | New ad-hoc `base64(created_at\|id)` string | The lib already exists, has tests, and handles both ID+Timestamp. **Reuse it.** Encode `Cursor{ID: comment.ID, Timestamp: comment.CreatedAt}`. |
| Separate `migrations/social-merge.go` file with a `Run()` function called from main | Inline block in `main.go` between `AutoMigrate` and `srv.Start()` | The existing precedent (phase 3 backfill) is inline. Inline keeps the deploy-time contract obvious — anyone reading `main.go` sees the migration. **Inline.** Extract only if the SQL block exceeds ~30 lines. |
| Adding a `username` column to comments table | JOIN to users on every list query | Project convention is denormalize (see `domain/watch.go:110` `Reviews.Username`, `domain/activity.go:12` `ActivityEvent.Username`). **Denormalize.** |

**Installation:** No new dependencies. All packages already in `services/player/go.mod` and `frontend/web/package.json`.

**Version verification:** All versions are pinned in `go.work` / `go.mod` / `package.json` and have already been verified by the project's `STACK.md` analysis on 2026-04-27.

## Architecture Patterns

### System Architecture Diagram

```
                                  ┌───────────────────────┐
                                  │  Browser              │
   /anime/:id?ugc=comments        │   Anime.vue           │
   ──────────────────────────────►│    └─ <Tabs>          │
                                  │       ├─ Reviews tab  │ (unchanged content; reviewApi calls)
                                  │       └─ Comments tab │ (new content; commentApi calls)
                                  │    ActivityFeed.vue   │ (renders type='comment' events too)
                                  └───────────┬───────────┘
                                              │ axios
                                              ▼
            ┌─────────────────────────────────────────────────────┐
            │  Gateway (services/gateway)                         │
            │   ┌──────────────────────────────────────────┐      │
            │   │ /anime/{id}/reviews*  → ProxyToPlayer    │      │ (EXISTING)
            │   │ /anime/{id}/comments* → ProxyToPlayer    │ ◄────┤ NEW (MUST be added BEFORE
            │   │ /anime/*              → ProxyToCatalog   │      │  the /anime/* catch-all)
            │   └──────────────────────────────────────────┘      │
            └─────────────────────────┬───────────────────────────┘
                                      ▼
            ┌──────────────────────────────────────────────────────────────┐
            │  Player Service (services/player)                            │
            │                                                              │
            │   transport/router.go  ── chi routes                         │
            │     /api/anime/{id}/reviews*  → ReviewHandler  (refactored)  │
            │     /api/anime/{id}/comments* → CommentHandler (NEW)         │
            │                                                              │
            │   handler/                                                   │
            │     review.go     → calls ReviewService                      │
            │     comment.go    → calls CommentService                     │  NEW
            │                                                              │
            │   service/                                                   │
            │     review.go     → calls ListRepository (was ReviewRepo)    │  REFACTORED
            │     comment.go    → calls CommentRepository + ActivityRepo   │  NEW
            │                     + per-user-anime in-mem rate bucket      │
            │                                                              │
            │   repo/                                                      │
            │     list.go       → AnimeListEntry queries (unchanged)       │
            │     review.go     → DELETED                                  │  DELETED
            │     comment.go    → Comment queries; cursor paginated        │  NEW
            │     activity.go   → Create (existing) used for type='comment'│
            │                                                              │
            │   domain/                                                    │
            │     watch.go      → AnimeListEntry +ReviewText +Username     │  EXTENDED
            │                   → Review struct DELETED                    │  DELETED
            │     comment.go    → Comment struct (NEW)                     │  NEW
            │                                                              │
            │   cmd/player-api/main.go                                     │
            │     AutoMigrate(AnimeListEntry, Comment, ...)                │  EXTENDED
            │     One-shot block (idempotency: reviews table existence)    │  NEW
            │       copies reviews → anime_list, drops reviews             │
            └──────────────────────────────────────┬───────────────────────┘
                                                   ▼
                                         ┌──────────────────────────┐
                                         │ PostgreSQL (animeenigma) │
                                         │  anime_list (extended)   │
                                         │  comments (new)          │
                                         │  activity_events         │
                                         │  reviews (dropped after  │
                                         │           migration)     │
                                         └──────────────────────────┘
```

### Component Responsibilities

| File | Role | Status |
|------|------|--------|
| `services/player/internal/domain/watch.go` | Extend `AnimeListEntry` with `ReviewText` + `Username`; delete `Review` struct | EDIT |
| `services/player/internal/domain/comment.go` | New `Comment` struct with `gorm.DeletedAt`, `parent_id *string` (nullable) | NEW |
| `services/player/internal/repo/review.go` | Delete the file entirely | DELETE |
| `services/player/internal/repo/list.go` | Add `GetByAnimeWithReview(ctx, animeID)` — returns rows where `score>0 OR review_text!=''` | EDIT |
| `services/player/internal/repo/list.go` | Add `GetReviewByUserAndAnime`, `UpsertReview`, `ClearReview`, `GetAnimeRating`, `GetBatchAnimeRatings` (or refactor existing Upsert to accept review fields) | EDIT |
| `services/player/internal/repo/comment.go` | `Create`, `Update`, `SoftDelete`, `GetByID`, `ListByAnime(ctx, animeID, cursor, limit)`, `CountByAnime` | NEW |
| `services/player/internal/service/review.go` | Refactor to call `ListRepository` methods; preserve method signatures (`CreateOrUpdateReview`, `GetAnimeReviews`, `GetUserReview`, `GetAnimeRating`, `GetBatchAnimeRatings`, `DeleteReview`) | EDIT |
| `services/player/internal/service/comment.go` | `CreateComment`, `UpdateComment`, `DeleteComment`, `ListComments`, `CountComments`. Owns the in-memory rate-limit map. Emits `activity_events` row of type `comment` on Create. | NEW |
| `services/player/internal/handler/review.go` | No structural change — handler signatures unchanged. The `*domain.Review` it returns becomes `*domain.AnimeListEntry` but with the same JSON tags so the wire shape is identical. | EDIT (light) |
| `services/player/internal/handler/comment.go` | `CreateComment`, `UpdateComment`, `DeleteComment`, `ListComments`. Parse cursor from query, owner-or-admin check on patch/delete. | NEW |
| `services/player/internal/transport/router.go` | Mount `/api/anime/{animeId}/comments` group (public GET) with nested protected group (POST/PATCH/DELETE) | EDIT |
| `services/player/cmd/player-api/main.go` | Add `&domain.Comment{}` to `AutoMigrate`; remove `&domain.Review{}` (after the bootstrap block runs); add the bootstrap block | EDIT |
| `services/gateway/internal/transport/router.go` | Add four explicit `/anime/{animeId}/comments*` routes immediately after the existing reviews routes (lines 144-149) | EDIT |
| `frontend/web/src/api/client.ts` | Add `commentApi` after `reviewApi` (around line 357) | EDIT |
| `frontend/web/src/views/Anime.vue` | Wrap lines 590-714 in `<Tabs>`; add Comments tab content (form, list, edit-mode, delete-confirm) | EDIT |
| `frontend/web/src/components/ActivityFeed.vue` | Add `if (event.type === 'comment')` branch in `actionText()` returning `t('activity.comment.posted')` | EDIT (1 line) |
| `frontend/web/src/locales/en.json`, `ja.json`, `ru.json` | Add `anime.ugc.*` block (~21 keys per UI-SPEC) + `activity.comment.posted` (3 lines per file) | EDIT |

### Pattern 1: One-shot idempotent bootstrap migration

**What:** Run a data-migration block exactly once per environment, gated by a simple existence/marker check.
**When to use:** Schema changes that GORM AutoMigrate can't express (data copy, table drop, complex backfill).
**Example (verbatim precedent — `cmd/player-api/main.go:192-213`):**

```go
// Phase 3 backfill: synthesize watch_progress.completed=true rows for legacy
// data (any (user, anime, ep <= anime_list.episodes) without a completed=true
// row). Idempotent — guarded by an early-exit check so it short-circuits on
// every restart after the first deploy. Non-fatal if it fails.
{
    var anyCompleted int
    _ = db.DB.Raw("SELECT 1 FROM watch_progress WHERE completed = true LIMIT 1").Scan(&anyCompleted).Error
    if anyCompleted == 0 {
        log.Infow("phase 3 backfill: synthesizing watch_progress.completed=true rows from anime_list.episodes")
        if err := db.DB.Exec(`...INSERT ... ON CONFLICT DO UPDATE ...`).Error; err != nil {
            log.Errorw("phase 3 backfill failed (non-fatal)", "error", err)
        } else {
            log.Infow("phase 3 backfill complete")
        }
    }
}
```

**Adapted for this phase (write inside `cmd/player-api/main.go` AFTER the existing `db.AutoMigrate(...)` block at lines 50-65, BEFORE the Phase 3 backfill block at lines 192-213):**

```go
// Phase 1 (workstream social): merge reviews → anime_list, drop reviews.
// Idempotency guard: does the `reviews` table still exist?
{
    var reviewsExists bool
    _ = db.DB.Raw(
        "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'reviews')",
    ).Scan(&reviewsExists).Error
    if reviewsExists {
        log.Infow("social migration: merging reviews into anime_list")

        // Step 1: AutoMigrate already added review_text + username columns above.
        // Step 2: Upsert reviews into anime_list. Postgres-only `INSERT ... ON CONFLICT`
        // because GORM's Clauses(...).Create cannot express "set score only when target.score = 0".
        if err := db.DB.Exec(`
            INSERT INTO anime_list (
                id, user_id, anime_id, status, score, episodes,
                review_text, username, created_at, updated_at
            )
            SELECT gen_random_uuid(), r.user_id, r.anime_id, 'completed',
                   r.score, 0,
                   r.review_text, r.username,
                   NOW(), NOW()
            FROM reviews r
            ON CONFLICT (user_id, anime_id) DO UPDATE SET
                score        = CASE WHEN anime_list.score = 0 THEN EXCLUDED.score ELSE anime_list.score END,
                review_text  = EXCLUDED.review_text,
                username     = COALESCE(NULLIF(EXCLUDED.username, ''), anime_list.username),
                updated_at   = NOW()
        `).Error; err != nil {
            log.Fatalw("social migration step 2 (reviews→anime_list copy) failed", "error", err)
        }

        // Step 3: backfill username for any anime_list rows that still have empty username.
        if err := db.DB.Exec(`
            UPDATE anime_list SET username = u.username, updated_at = NOW()
            FROM users u
            WHERE anime_list.user_id = u.id
              AND (anime_list.username IS NULL OR anime_list.username = '')
        `).Error; err != nil {
            log.Fatalw("social migration step 3 (username backfill) failed", "error", err)
        }

        // Step 4: comments table already auto-migrated above. Step 5: drop reviews.
        if err := db.DB.Exec(`DROP TABLE reviews`).Error; err != nil {
            log.Fatalw("social migration step 5 (DROP TABLE reviews) failed", "error", err)
        }

        log.Infow("social migration complete")
    }
}
```

### Pattern 2: Cursor-paginated newest-first list

**What:** Page through a large result set using an opaque cursor, fetching `limit+1` to detect hasMore.
**When to use:** Comments list (50/page); any future feed.
**Example (verbatim precedent — `repo/activity.go:54-81`):**

```go
func (r *ActivityRepository) GetFeed(ctx context.Context, limit int, before string) ([]*domain.ActivityEvent, bool, error) {
    query := r.db.WithContext(ctx).
        Preload("Anime").
        Order("created_at DESC, id DESC")

    if before != "" {
        var cursor domain.ActivityEvent
        if err := r.db.WithContext(ctx).Select("created_at").Where("id = ?", before).First(&cursor).Error; err != nil {
            return nil, false, err
        }
        query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", cursor.CreatedAt, cursor.CreatedAt, before)
    }

    var events []*domain.ActivityEvent
    err := query.Limit(limit + 1).Find(&events).Error
    if err != nil {
        return nil, false, err
    }

    hasMore := len(events) > limit
    if hasMore {
        events = events[:limit]
    }
    return events, hasMore, nil
}
```

**For comments, refine using `libs/pagination/Cursor`:**

```go
import "github.com/ILITA-hub/animeenigma/libs/pagination"

func (r *CommentRepository) ListByAnime(ctx context.Context, animeID, cursorStr string, limit int) ([]*domain.Comment, string, error) {
    query := r.db.WithContext(ctx).
        Where("anime_id = ? AND deleted_at IS NULL", animeID).
        Order("created_at DESC, id DESC")

    if cursorStr != "" {
        cur, err := pagination.DecodeCursor(cursorStr)
        if err != nil {
            return nil, "", errors.InvalidInput("invalid cursor")
        }
        query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", cur.Timestamp, cur.Timestamp, cur.ID)
    }

    var comments []*domain.Comment
    if err := query.Limit(limit + 1).Find(&comments).Error; err != nil {
        return nil, "", err
    }

    var nextCursor string
    if len(comments) > limit {
        comments = comments[:limit]
        last := comments[len(comments)-1]
        nextCursor = pagination.Cursor{ID: last.ID, Timestamp: last.CreatedAt}.Encode()
    }
    return comments, nextCursor, nil
}
```

### Pattern 3: Chi protected sub-group inside a public route group

**What:** Mount auth-required POST/PATCH/DELETE under the same path prefix as a public GET.
**When to use:** `/api/anime/{animeId}/comments` — GET is public, POST/PATCH/DELETE require auth.
**Example (verbatim precedent — `transport/router.go:166-178`):**

```go
r.Route("/anime/{animeId}", func(r chi.Router) {
    // Public routes
    r.Get("/reviews", reviewHandler.GetAnimeReviews)
    r.Get("/rating", reviewHandler.GetAnimeRating)

    // Protected routes
    r.Group(func(r chi.Router) {
        r.Use(AuthMiddleware(jwtConfig))
        r.Post("/reviews", reviewHandler.CreateOrUpdateReview)
        r.Get("/reviews/me", reviewHandler.GetUserReview)
        r.Delete("/reviews", reviewHandler.DeleteReview)
    })
})
```

**Adapted for comments (add INSIDE the same `r.Route("/anime/{animeId}", ...)` block, right after the existing reviews routes):**

```go
// Public route — list comments
r.Get("/comments", commentHandler.ListComments)

// Protected routes — write/edit/delete
r.Group(func(r chi.Router) {
    r.Use(AuthMiddleware(jwtConfig))
    r.Post("/comments", commentHandler.CreateComment)
    r.Patch("/comments/{commentId}", commentHandler.UpdateComment)
    r.Delete("/comments/{commentId}", commentHandler.DeleteComment)
})
```

### Pattern 4: Activity event emit (no dedup)

**What:** On a write, emit one row into `activity_events` so the feed picks it up.
**When to use:** New comments (every create emits; no per-day rollup).
**Example (adapted from `service/review.go:50-86`, with dedup branch stripped):**

```go
contentPreview := body
if len([]rune(contentPreview)) > 300 {
    contentPreview = string([]rune(contentPreview)[:300]) + "…"
}
event := &domain.ActivityEvent{
    UserID:   userID,
    Username: username,
    AnimeID:  animeID,
    Type:     "comment",
    Content:  contentPreview,
    // OldValue / NewValue intentionally empty — comments have no score / status delta
}
if err := s.activityRepo.Create(ctx, event); err != nil {
    s.log.Errorw("failed to record comment activity", "user_id", userID, "anime_id", animeID, "error", err)
    // non-fatal: comment is already saved
}
```

### Pattern 5: Per-(user, anime) rate-limit bucket (in-memory)

**What:** Track count of writes in the last 1 hour, keyed by `(userID, animeID)`.
**When to use:** Comments POST rate limit (10/hour).
**Example (no existing precedent — hand-rolled):**

```go
// Inside CommentService struct
type rateBucket struct {
    mu      sync.Mutex
    entries map[string][]time.Time // key = userID + "|" + animeID
}

const rateLimitMax = 10
const rateLimitWindow = time.Hour

func (b *rateBucket) allow(userID, animeID string) bool {
    key := userID + "|" + animeID
    b.mu.Lock()
    defer b.mu.Unlock()

    now := time.Now()
    cutoff := now.Add(-rateLimitWindow)
    keep := b.entries[key][:0]
    for _, t := range b.entries[key] {
        if t.After(cutoff) {
            keep = append(keep, t)
        }
    }
    if len(keep) >= rateLimitMax {
        b.entries[key] = keep
        return false
    }
    keep = append(keep, now)
    b.entries[key] = keep
    return true
}
```

**In the handler/service:**

```go
if !s.rateBucket.allow(userID, req.AnimeID) {
    return nil, errors.RateLimited()
}
```

This naturally returns HTTP 429 via `httputil.Error(w, err)` because `errors.RateLimited()` maps to `http.StatusTooManyRequests` (see `libs/errors/errors.go:149-150`).

**Note:** acceptable per CONTEXT.md — single-replica today, value is soft. If the player ever runs multi-replica, swap for a Redis-backed sliding window (the player already has Redis wired in `main.go:128-132`).

### Pattern 6: Vue Tabs with URL persistence

**What:** Sync a Vue ref to a `route.query` param so deep links + tab clicks update the URL without history entries.
**When to use:** `?ugc=reviews|comments` on Anime.vue.
**Example (this phase — no existing precedent in `Anime.vue`):**

```vue
<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Tabs from '@/components/ui/Tabs.vue'

const route = useRoute()       // already imported (Anime.vue:832)
const router = useRouter()     // already imported (Anime.vue:833)

const ALLOWED = ['reviews', 'comments'] as const
type UgcTab = typeof ALLOWED[number]
const initial = (route.query.ugc as UgcTab) ?? 'reviews'
const ugcTab = ref<UgcTab>(ALLOWED.includes(initial) ? initial : 'reviews')

// route.query.ugc → ref (back/forward nav, deep links)
watch(() => route.query.ugc, (v) => {
  const val = (v as UgcTab) ?? 'reviews'
  if (ALLOWED.includes(val) && val !== ugcTab.value) ugcTab.value = val
})

// ref → router.replace (click handler)
watch(ugcTab, (v) => {
  if (route.query.ugc !== v) {
    router.replace({ query: { ...route.query, ugc: v } })
  }
})
</script>

<template>
  <Tabs
    v-model="ugcTab"
    :tabs="[
      { value: 'reviews',  label: $t('anime.ugc.reviewsTab'),  count: reviews.length },
      { value: 'comments', label: $t('anime.ugc.commentsTab'), count: commentsTotal },
    ]"
    variant="underline"
  >
    <template #reviews>... existing reviews markup ...</template>
    <template #comments>... new comments markup ...</template>
  </Tabs>
</template>
```

### Anti-Patterns to Avoid

- **Don't bypass the gateway** when adding new player-bound routes — every `/api/anime/{id}/*` path must be explicitly proxied in `services/gateway/internal/transport/router.go` BEFORE the `/anime/*` catch-all that routes to catalog (line 153). Adding `/api/anime/{id}/comments*` only to player's router without gateway proxy entries will produce 404s in production.
- **Don't extract a new `Comment` table username column from JWT claims at every request** — the JWT only knows about the writer, not the readers. Denormalize at insert time (mirrors `reviews.username`).
- **Don't dedup comment activity events.** Reviews dedup per day because re-saving the same review shouldn't spam the feed; comments are distinct posts and each is a separate event.
- **Don't use `router.push` for tab switches.** Use `router.replace`. The back button should leave the page entirely (per UI-SPEC), not cycle through tabs.
- **Don't mount the public `GET /comments` route inside the existing `/users` group** — that group has `AuthMiddleware` and would 401 anonymous readers. Add it to the public `r.Route("/anime/{animeId}", ...)` block at `router.go:166`.
- **Don't store the rate-limit bucket as a global singleton.** Construct it inside `NewCommentService(...)` so tests get fresh buckets (otherwise a 429 from one test leaks into the next).
- **Don't return a non-empty `Anime` preload on the comments list response.** The frontend only needs username + body + timestamp; preloading `Anime` doubles the response size and N+1s the genres join. Reviews preload `Anime` because the user's profile page reuses the same response shape (`GetUserReviews`); comments have no such reuse.
- **Don't TRUNCATE the `reviews` table before the data copy.** Use the `INSERT ... ON CONFLICT` pattern from the example — TRUNCATE before the copy would lose data if anything fails between.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Cursor encode/decode | Custom `base64(id + "|" + ts)` string format | `libs/pagination.Cursor{ID, Timestamp}.Encode()` + `pagination.DecodeCursor(s)` | Already exists, already tested, already used elsewhere in the workspace |
| HTTP error → status code mapping | Switch statements in handlers | `libs/errors` + `httputil.Error(w, err)` — `errors.RateLimited()` → 429 automatically, `errors.NotFound(...)` → 404, etc. | Single source of truth; new error types auto-map |
| JSON request body decode + validation | Manual `json.NewDecoder` | `httputil.Bind(r, &req)` or `httputil.BindAndValidate(r, &req)` (the latter accepts a `Validator` interface) | Standard pattern across all handlers; returns `errors.InvalidInput` on parse failure |
| UTF-8 char count (for 1-2000 body length) | `len(body)` (counts bytes, not runes) | `utf8.RuneCountInString(body)` from `unicode/utf8` (stdlib) | Comment body is UTF-8; a 2000-rune Japanese comment is ~6000 bytes — byte length would falsely reject |
| Owner-or-admin authorization check | Hand-rolled `if comment.UserID != claims.UserID && claims.Role != "admin"` | Same idiom but use `authz.IsAdmin(ctx)` for the admin check | `authz.IsAdmin(ctx)` is the existing helper (`libs/authz/jwt.go:192-194`) and matches the `AdminRoleMiddleware` pattern |
| Soft-delete filter | Manual `WHERE deleted_at IS NULL` everywhere | `gorm.DeletedAt` tagged with `gorm:"index"` on the struct → GORM auto-injects the filter on every query (and the `Delete()` method becomes a soft-delete via `UPDATE deleted_at = NOW()`) | Project convention (see `domain/activity.go:20`); free correctness |
| Idempotent table creation | Manual `CREATE TABLE IF NOT EXISTS comments (...)` | `db.AutoMigrate(&domain.Comment{})` | GORM creates the table if missing, adds new columns if struct changes, skips if up to date. The migration block in `main.go` only handles the data-copy + DROP, not the new table creation. |
| Vue tabs component | New `<UgcTabs>` SFC | `components/ui/Tabs.vue` (existing, `variant="underline"`, per-tab `count` prop, `v-model`) | UI-SPEC explicitly mandates this. |
| Tab badge styling | New badge CSS | `Tabs.vue` already renders count as `<span class="ml-2 px-1.5 py-0.5 text-xs rounded-full bg-white/10">{{ count }}</span>` (line 16) | Free |
| Date formatting | New `formatDate()` | Reuse existing `formatDate()` in `Anime.vue:1068` | Already locale-aware |
| Username avatar fallback | New helper | Reuse the inline pattern `username?.slice(0, 2).toUpperCase() || '??'` from `Anime.vue:689` | Free |

**Key insight:** This phase has essentially zero greenfield: every pattern needed is already in the codebase. The only genuinely new pieces are the `Comment` domain struct, the comments repo/service/handler files (which clone the reviews pattern), and the rate-limit bucket (which is ~30 lines of code).

## Runtime State Inventory

> The phase involves dropping the `reviews` table. This is a destructive schema change requiring a state inventory.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | Postgres `reviews` table — N rows, schema `(id, user_id, anime_id, username, score, review_text, created_at, updated_at)` with unique index on `(user_id, anime_id)`. Verified via `services/player/internal/domain/watch.go:105-119`. | **Data migration** — Step 2 of the one-shot block copies these rows into `anime_list`. After the migration completes, the bootstrap block runs `DROP TABLE reviews`. |
| Stored data | Postgres `anime_list` table — has existing `username` column? **NO** — verified via `services/player/internal/domain/watch.go:58-79`, only `Notes` and `Tags` are text-ish; no `username`. | **Code edit** — GORM AutoMigrate adds the column. **Data migration** — Step 3 backfills empty usernames from `users` JOIN. |
| Stored data | Postgres `activity_events` table — `type` column is `varchar(20)` indexed. Existing values: `"review"`, `"status"`, `"score"`. Adding `"comment"` is additive — no schema change, no migration. Verified via `services/player/internal/domain/activity.go:9-21`. | **None** — adding a new `type` value works without schema change. |
| Live service config | No external service has the string `"reviews"` baked into config that we know of. Reviews endpoints stay at the same URL paths. | **None** — verified by reading gateway router (no hardcoded reviews→player anywhere except the explicit proxy routes that remain). |
| OS-registered state | None — this is a code+schema change, no Task Scheduler / pm2 / launchd / systemd registration. | **None.** |
| Secrets / env vars | None — no new env var required. JWT secret already used by `AuthMiddleware`. Redis already wired. No new external API keys. | **None.** |
| Build artifacts / installed packages | Frontend changes go through `make redeploy-web` (nginx serves a freshly-built bundle). Backend changes through `make redeploy-player`. No package re-install needed (`reviewApi` keeps the same exported name). | **Code edit** — delete `services/player/internal/repo/review.go` from disk so it's not in the next build. |
| Build artifacts (Go code) | The Go file `services/player/internal/domain/watch.go` contains the `Review` struct (lines 105-119). After deletion, `cmd/player-api/main.go:56` references `&domain.Review{}` — that line must also be removed. Other references: `service/review.go:14` (`reviewRepo *repo.ReviewRepository`), `cmd/player-api/main.go:219` (`reviewRepo := repo.NewReviewRepository(db.DB)`), `cmd/player-api/main.go:248` (`reviewService := service.NewReviewService(reviewRepo, ...)`). | **Code edit** — refactor `ReviewService` to take `*ListRepository` instead of `*ReviewRepository`; drop both `NewReviewRepository` and the `&domain.Review{}` AutoMigrate entry. Compile will fail loudly if anything is missed. |
| Build artifacts (frontend) | `frontend/web/src/views/Anime.vue:816-824` declares the `Review` TypeScript interface. After backend changes the new responses include extra fields like `notes`, `status`, `episodes`, `tags`, `mal_id` (from `AnimeListEntry`'s JSON tags). TypeScript permits excess properties on object literals, so this compiles without changes; consider expanding the interface for clarity but not strictly required. | **Optional code edit** — extend the interface for self-documentation. |

**The canonical question:** *After every file in the repo is updated, what runtime systems still have the old string cached, stored, or registered?*

Answer: Only the `reviews` table itself — and the bootstrap block deletes it. Postgres replication/backup, if any, contains the old `reviews` table snapshot which is the desired backup-of-last-resort per the SPEC (forward-only migration; restore from backup if rollback needed).

## Common Pitfalls

### Pitfall 1: Frontend response-shape regression (silent)

**What goes wrong:** The `AnimeListEntry` struct has more JSON fields than `Review` did. After the refactor, `GET /api/anime/:id/reviews` returns each row with `notes`, `status`, `episodes`, `tags`, `mal_id`, `is_rewatching`, `priority`, `started_at`, `completed_at`, and a nested `anime` object that the old shape also had. Most frontends ignore extra fields silently — but a consumer that does `JSON.stringify(reviews)` or compares object equality will see different payloads.
**Why it happens:** `AnimeListEntry` was designed for a different consumer (the watchlist UI).
**How to avoid:** Either (a) create a dedicated `ReviewResponse` struct in the handler that projects only the seven canonical fields, or (b) accept the wider payload and document it as the new contract. **Recommended (a):** define an unexported projection struct inside `handler/review.go` and `Scan` into it from the repo, so the public JSON shape is byte-identical to today's.
**Warning signs:** Golden-file diff of `GET /api/anime/{id}/reviews` shows new keys after the migration.

### Pitfall 2: GORM `clause.OnConflict` not propagating `DoNothing` for the existing row's `score`

**What goes wrong:** The repo Upsert for reviews → anime_list must preserve `score` when the user already had a `score>0` (i.e., set via the watchlist score input). The naive GORM expression `"score": review.Score` overwrites with the new value. The current `repo/review.go:21-37` Upsert is for the `reviews` table where this is fine. When refactored to write `anime_list`, we need `"score": gorm.Expr("CASE WHEN anime_list.score = 0 THEN ? ELSE anime_list.score END", review.Score)` — **but only if** the spec wants to preserve the previously-set list score. The CONTEXT.md decision is: **`POST /api/anime/:id/reviews` always sets `score = req.Score`** (it's "the user explicitly chose this score"). So the existing simple assignment is correct for the new code. The CASE-WHEN logic is ONLY needed in the migration backfill, where it's "preserve list score if it was already non-zero, else copy from reviews".
**Why it happens:** Two different code paths (migration vs. live write) need two different conflict-resolution rules.
**How to avoid:** Use raw SQL `INSERT ... ON CONFLICT` for the migration (with the CASE expression) so the runtime upsert path can stay simple.
**Warning signs:** A user who already had `anime_list.score = 7` (set in the watchlist) and never wrote a review keeps score 7 — but a user who imported from MAL gets their MAL score correctly migrated into a `score>0` row.

### Pitfall 3: Gateway not routing `/comments` paths to player

**What goes wrong:** Frontend `commentApi.getAnimeComments(id)` calls `/api/anime/{id}/comments` → gateway → catalog (because `/anime/*` is catch-all routed to catalog at `services/gateway/internal/transport/router.go:153`) → catalog returns 404 because it has no such route.
**Why it happens:** The catch-all matches before any specific route the planner forgets to add.
**How to avoid:** Mirror the existing reviews-route pattern. Add four lines IMMEDIATELY after the reviews routes at `router.go:144-149`:
```go
r.Get("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
r.Post("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
r.Patch("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
r.Delete("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
```
**Warning signs:** `curl localhost:8000/api/anime/<id>/comments` returns a catalog 404 with shape `{"error": "anime not found"}` or similar.

### Pitfall 4: Anonymous user gets 401 on `GET /comments`

**What goes wrong:** Placing the comments routes inside the existing `/api/users/*` protected group, or accidentally inside the reviews `r.Group(... AuthMiddleware ...)` at `router.go:172-177`, makes the list endpoint reject anonymous readers.
**How to avoid:** Add `r.Get("/comments", ...)` OUTSIDE the protected group — at the same level as `r.Get("/reviews", reviewHandler.GetAnimeReviews)` at `router.go:168`.
**Warning signs:** Logged-out user opens `/anime/<id>?ugc=comments` and sees 401 in the browser network panel.

### Pitfall 5: Rune count vs. byte count for body length

**What goes wrong:** Validating body length with `len(req.Body)` rejects Japanese / Cyrillic comments (which have ~3 bytes per char in UTF-8) at well under 2000 visible characters.
**How to avoid:** Use `utf8.RuneCountInString(req.Body)` from `unicode/utf8`.
**Warning signs:** A Russian user pastes a 700-char comment and gets 400 `body too long`.

### Pitfall 6: ActivityFeed.vue dies on `event.type === 'comment'`

**What goes wrong:** `ActivityFeed.vue:142-159` returns `t('activity.status.' + event.new_value)` as a fallback. For `comment` events, `new_value` is empty, so it becomes `t('activity.status.')` which renders as an empty string at best, a missing-key warning in dev console at worst.
**How to avoid:** Add a `comment` branch before the fallback:
```typescript
if (event.type === 'comment') {
  return t('activity.comment.posted')
}
```
Add `"activity.comment.posted"` to all three locale files (`en.json`, `ja.json`, `ru.json`).
**Warning signs:** Activity feed shows a comment row but the action text is empty.

### Pitfall 7: Two locale files drift out of sync

**What goes wrong:** Locale keys added to `en.json` but not `ja.json` / `ru.json` produce missing-key warnings in vue-i18n.
**How to avoid:** Make the locale-file edit a single Wave task with three subtasks (one per file). The UI-SPEC's copywriting contract table has English copy for all 21 keys; the implementer translates the Japanese and Russian columns marked `(translate)`.
**Warning signs:** `[intlify] Not found 'anime.ugc.commentPlaceholder' key in 'ja' locale messages.`

### Pitfall 8: Comment `parent_id` enforcement

**What goes wrong:** The spec says `parent_id` is reserved (always NULL in v0.1). Without enforcement, a malicious or buggy client could POST with a non-null `parent_id` and create a "reply" the UI doesn't render. Worse, with a FK constraint added later, those rows would dangle.
**How to avoid:** In the `CreateComment` handler, simply do not read `parent_id` from the request body — the struct field is server-only. The `CreateCommentRequest` DTO omits `parent_id` entirely.
**Warning signs:** Any code path that writes to `Comment.ParentID` in v0.1.

### Pitfall 9: Reviews refactor breaks the existing `GetAnimeRating` SQL

**What goes wrong:** The existing `ReviewRepository.GetAnimeRating` query (`repo/review.go:71-97`) UNIONs `reviews.score` and `anime_list.score` and dedups. After the migration, only `anime_list` exists. The new query is simpler: `SELECT AVG(score), COUNT(*) FROM anime_list WHERE anime_id = ? AND score > 0`. **But:** if the bootstrap migration hasn't run yet (e.g., during the first boot after deploy, before the block executes), the `reviews` table still exists and the existing query returns correct results. After the block runs, the `reviews` table is dropped — and the existing query throws `relation "reviews" does not exist`. The window is small (single AutoMigrate → block → server start) but the bug exists.
**How to avoid:** Move the `ReviewRepository`/`ReviewService` refactor + the migration block deploy to a single PR. The new `GetAnimeRating` (operating on `anime_list` only) ships AT THE SAME TIME as the migration. There is no intermediate state where the old query runs against a dropped `reviews` table.
**Warning signs:** A staging deploy that ships only the migration without the SQL refactor will 500 every `/rating` request.

## Code Examples

Verified patterns from official sources / this codebase:

### Example 1: Comment domain struct

```go
// services/player/internal/domain/comment.go
package domain

import (
    "time"
    "gorm.io/gorm"
)

type Comment struct {
    ID        string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    UserID    string         `gorm:"type:uuid;index:idx_comments_user_created" json:"user_id"`
    AnimeID   string         `gorm:"type:uuid;index:idx_comments_anime_created" json:"anime_id"`
    Username  string         `gorm:"size:32" json:"username"`
    Body      string         `gorm:"type:text" json:"body"`
    ParentID  *string        `gorm:"type:uuid" json:"parent_id,omitempty"`
    CreatedAt time.Time      `gorm:"index:idx_comments_anime_created,sort:desc;index:idx_comments_user_created,sort:desc" json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Comment) TableName() string { return "comments" }

// CreateCommentRequest is the POST body. Intentionally omits parent_id —
// parent_id is reserved for v1.0; v0.1 always writes NULL.
type CreateCommentRequest struct {
    Body string `json:"body"`
}

type UpdateCommentRequest struct {
    Body string `json:"body"`
}

// CommentsListResponse is the GET response.
type CommentsListResponse struct {
    Comments   []*Comment `json:"comments"`
    NextCursor string     `json:"next_cursor,omitempty"`
    HasMore    bool       `json:"has_more"`
}
```

Source: project conventions in `domain/watch.go:105-119` + `domain/activity.go:9-21` (soft delete pattern).

### Example 2: Comment handler (one method, illustrative)

```go
// services/player/internal/handler/comment.go (partial — CreateComment)
func (h *CommentHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
    animeID := chi.URLParam(r, "animeId")
    if animeID == "" {
        httputil.BadRequest(w, "anime_id is required")
        return
    }

    claims, ok := authz.ClaimsFromContext(r.Context())
    if !ok || claims == nil {
        httputil.Unauthorized(w)
        return
    }

    var req domain.CreateCommentRequest
    if err := httputil.Bind(r, &req); err != nil {
        httputil.Error(w, err)
        return
    }

    comment, err := h.commentService.CreateComment(r.Context(), claims.UserID, claims.Username, animeID, &req)
    if err != nil {
        httputil.Error(w, err)
        return
    }
    httputil.Created(w, comment)
}
```

Source: mirrors `handler/review.go:27-47`.

### Example 3: Comment service (validation + rate limit + create + activity emit)

```go
// services/player/internal/service/comment.go (CreateComment)
func (s *CommentService) CreateComment(ctx context.Context, userID, username, animeID string, req *domain.CreateCommentRequest) (*domain.Comment, error) {
    body := strings.TrimSpace(req.Body)
    if body == "" {
        return nil, errors.InvalidInput("comment body cannot be empty")
    }
    runes := utf8.RuneCountInString(body)
    if runes > 2000 {
        return nil, errors.InvalidInput("comment body cannot exceed 2000 characters")
    }

    if !s.rateBucket.allow(userID, animeID) {
        return nil, errors.RateLimited()
    }

    comment := &domain.Comment{
        UserID:   userID,
        AnimeID:  animeID,
        Username: username,
        Body:     body,
    }
    if err := s.commentRepo.Create(ctx, comment); err != nil {
        return nil, errors.Wrap(err, errors.CodeInternal, "failed to save comment")
    }

    // Emit activity event — no dedup, every comment is a new event.
    contentPreview := body
    if utf8.RuneCountInString(contentPreview) > 300 {
        contentPreview = string([]rune(contentPreview)[:300]) + "…"
    }
    event := &domain.ActivityEvent{
        UserID:   userID,
        Username: username,
        AnimeID:  animeID,
        Type:     "comment",
        Content:  contentPreview,
    }
    if err := s.activityRepo.Create(ctx, event); err != nil {
        s.log.Errorw("failed to record comment activity", "user_id", userID, "anime_id", animeID, "error", err)
        // non-fatal
    }

    return comment, nil
}
```

Source: mirrors `service/review.go:30-102` minus the dedup branch.

### Example 4: Frontend `commentApi` (`api/client.ts`)

```typescript
// Add after reviewApi at client.ts:357
export const commentApi = {
  // List comments for an anime (public, paginated)
  getAnimeComments: (animeId: string, params?: { cursor?: string; limit?: number }) =>
    apiClient.get(`/anime/${animeId}/comments`, { params }),
  // Create a new comment
  createComment: (animeId: string, body: string) =>
    apiClient.post(`/anime/${animeId}/comments`, { body }),
  // Edit an existing comment
  updateComment: (animeId: string, commentId: string, body: string) =>
    apiClient.patch(`/anime/${animeId}/comments/${commentId}`, { body }),
  // Soft-delete a comment
  deleteComment: (animeId: string, commentId: string) =>
    apiClient.delete(`/anime/${animeId}/comments/${commentId}`),
}
```

Source: mirrors `reviewApi` at `client.ts:338-357`.

### Example 5: ActivityFeed.vue branch addition

```typescript
// Add inside actionText() in ActivityFeed.vue around line 148
if (event.type === 'comment') {
  return t('activity.comment.posted')
}
```

And in each locale file under the `activity` key:

```json
"activity": {
  ...existing...,
  "comment": { "posted": "commented on" }
}
```

Source: project convention from `en.json:432-435` (review namespace).

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Two tables `reviews` + `anime_list` with overlapping `score` and one-way sync | Single source of truth in `anime_list` (`review_text`, `username` columns added) | This phase | Imports auto-appear in reviews list; no sync hazard |
| Per-day deduped review activity events | Distinct events on every comment create | This phase | Feed shows every comment, not just one-per-day |
| Single Reviews section on Anime.vue | Two-tab strip `Reviews | Comments` with URL persistence | This phase | Users discover and engage with each anime's discussion |

**Deprecated/outdated:**
- `services/player/internal/domain/watch.go` — `Review` struct → delete after migration.
- `services/player/internal/repo/review.go` — entire file → delete.
- `cmd/player-api/main.go` line `&domain.Review{}` inside `db.AutoMigrate(...)` → remove (because we drop the table inside the bootstrap block; leaving the AutoMigrate entry would attempt to re-create it after drop).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Frontend `Review` interface tolerating excess JSON keys (notes, status, etc.) won't break runtime consumers | Pitfall 1 / SOCIAL-NF-01 | Some hand-rolled component might do `Object.keys(review).length === 7` or use `JSON.stringify` — would silently break. Mitigation: capture/diff JSON snapshots; OR project to a dedicated handler-local struct (recommended). [ASSUMED based on TypeScript structural typing] |
| A2 | The player service runs single-replica in production today (rate-limit bucket is in-memory) | Pattern 5 / CONTEXT decisions | If actually multi-replica, the 10/hour limit would be 10 × replicas effectively. CONTEXT.md explicitly states this is acceptable for the "soft" limit. [ASSUMED based on CONTEXT.md decision, no production replica count verified in this session] |
| A3 | `gorm.DeletedAt` index gracefully filters all `Find`, `First`, and `Where` queries on the `Comment` model | Don't Hand-Roll table | GORM's documented behavior; standard pattern across the project. [VERIFIED: `domain/activity.go:20` uses it the same way] |
| A4 | Postgres `information_schema.tables` is the right table to probe for the migration idempotency guard | Pattern 1 / SOCIAL-NF-02 | Standard SQL. The phase-3 backfill uses a different style (probe a data row); the new block needs to probe schema. [CITED: Postgres docs] |
| A5 | The `golden-file JSON-shape diff` testing pattern does NOT have a precedent in `services/player/` | CONTEXT discretion | Verified by listing `services/player/internal/{handler,service,repo}/*_test.go` — no `golden*.json` fixture file pattern observed. Plan should NOT invest in a fixture-based framework; rely on a smoke check (capture, diff, fix). [VERIFIED via Bash listing] |
| A6 | The user is OK with `DELETE /api/anime/:id/reviews` clearing both score AND review_text (vs only the text) | CONTEXT decisions | CONTEXT.md auto-selected this in the SPEC Interview Log. The implementer should not surface this for re-discussion. [VERIFIED: CONTEXT.md and SPEC Interview Log align] |
| A7 | `parent_id` being explicitly omitted from the request DTO is sufficient enforcement of "NULL in v0.1" | Pitfall 8 | Strong: Go struct decoding by `httputil.Bind` won't populate fields the DTO doesn't declare. [VERIFIED: `httputil.Bind` uses `render.DecodeJSON` which respects the struct shape] |
| A8 | All AnimeListEntry rows for users who wrote reviews (or imported scores) have a corresponding `users.id` row to backfill `username` from | Migration step 3 | If a user account was hard-deleted but their `anime_list` row remained (orphan), the JOIN finds nothing and `username` stays empty. The `WHERE anime_list.username IS NULL OR anime_list.username = ''` filter is harmless — those rows just stay with empty username and the frontend renders `?? `. [ASSUMED — depends on the project's user-deletion policy, not verified in this session] |
| A9 | Gateway route ordering (specific reviews routes registered before catch-all `/anime/*`) is a hard chi rule that we must preserve for comments | Pitfall 3 | Direct verification of chi behavior + verbatim precedent at `services/gateway/internal/transport/router.go:144-153`. [VERIFIED] |
| A10 | `Tabs.vue` v-model + slot pattern handles synchronous initial render of `?ugc=comments` deep links without flicker | Pattern 6 / SOCIAL-06 | Vue 3 sets initial reactivity synchronously before first DOM render; the `ref` is initialized from `route.query.ugc` in `setup`, so the correct slot mounts. [VERIFIED: Vue 3 docs + UI-SPEC line 273] |

**If this table is empty:** Not empty. The most user-confirmable claim is A1 (response shape consumers); the safest mitigation is to project to a handler-local struct. The planner should propose this as a hard constraint (Wave 1 task: introduce `reviewResponseProjection` struct in `handler/review.go`).

## Open Questions

1. **What's the current row count in `reviews`?**
   - What we know: schema is `(user_id, anime_id, username, score, review_text)`; primary keys are UUIDs.
   - What's unclear: scale of the data migration. If it's <10K rows, the INSERT...ON CONFLICT runs in <1s. If 1M, it could be 30-60s and might exceed the player service's container startup probe.
   - Recommendation: Query before plan execution. If huge, batch the INSERT (`LIMIT/OFFSET` or `WITH RECURSIVE` chunking). Otherwise inline single-statement is fine.

2. **Are there orphan `anime_list` rows with no matching `users.id`?**
   - What we know: assumption A8.
   - What's unclear: whether soft-deleted users in this codebase leave behind `anime_list` rows.
   - Recommendation: pre-flight SQL check before the migration. If found, log them and proceed (they keep empty username — harmless).

3. **Does the project ever run multi-replica player?**
   - What we know: CONTEXT.md explicitly accepts in-memory rate-limit bucket; Phase 10 added Redis dependency.
   - What's unclear: whether ops plans to scale player horizontally in v0.1's lifetime.
   - Recommendation: design the rate-limit bucket as a swappable interface (`RateLimiter` with `Allow(key) bool`) so a future Redis-backed implementation is a drop-in. Adds ~3 lines, costs nothing.

4. **Should the activity-event icon for `type='comment'` differ from `type='review'`?**
   - What we know: UI-SPEC explicitly defers this — "for v0.1, comment events appear via the existing generic event renderer".
   - What's unclear: whether the existing ActivityFeed.vue's icon is generic enough to be visually correct for comments.
   - Recommendation: Add the locale key `activity.comment.posted` and the type branch. Visual icon: same chat-bubble SVG already used for reviews works fine (or no icon change at all — the existing component doesn't render an event-type-specific icon, just the anime poster).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go 1.22 | Player service build | ✓ (assumed — project standard) | 1.22 | — |
| PostgreSQL 16 | All schema changes | ✓ (via `docker compose`) | 16-alpine | — |
| Redis 7 | Player service starts (Phase 10) | ✓ (via `docker compose`) | 7-alpine | — |
| `make redeploy-player` | Backend deploy | ✓ (Makefile) | — | — |
| `make redeploy-web` | Frontend deploy | ✓ (Makefile) | — | — |
| Bun | Frontend lint/build | ✓ (per CLAUDE.md) | 1.x | — |
| `bunx playwright test` | E2E tests | ✓ | 1.58.0 | — |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** None.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework (Go) | `testing` stdlib + `github.com/stretchr/testify v1.8.4` |
| Framework (Frontend e2e) | `@playwright/test 1.58.0` |
| Config file (Go) | `.golangci.yml` (lint) — no separate test config; tests run via `go test ./...` |
| Config file (Playwright) | `frontend/web/playwright.config.ts` |
| Quick run command (Go, single test) | `cd services/player && go test ./internal/handler -run TestCommentHandler_CreateComment -v` |
| Quick run command (Go, package) | `cd services/player && go test ./internal/service/...` |
| Full suite command (Go, player) | `cd services/player && go test ./... -race -cover` |
| Full suite command (frontend e2e) | `cd frontend/web && bunx playwright test` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SOCIAL-01 | New columns `review_text`, `username` exist on `anime_list`; `reviews` table dropped | smoke (boot) | `make redeploy-player && docker compose exec postgres psql -d animeenigma -c "\d anime_list" \| grep review_text` | ❌ Wave 0 manual step |
| SOCIAL-02 | Migration idempotency — run twice = no diff | unit + manual | Unit: `go test ./services/player/cmd/player-api/... -run TestMigration_Idempotent`. Manual smoke: `make redeploy-player` twice; verify second start logs no migration message. | ❌ Wave 0 new test file (or skip — bootstrap-style migration tests are uncommon in this repo) |
| SOCIAL-03 | Six reviews endpoints preserve response shape | unit per endpoint | `cd services/player && go test ./internal/handler -run TestReviewHandler_ -v` | ✅ existing pattern (`review.go` handler has implicit coverage; add explicit shape assertion tests) |
| SOCIAL-04a | POST comment 1-2000 chars happy path | unit | `go test ./services/player/internal/handler -run TestCommentHandler_CreateComment_HappyPath -v` | ❌ Wave 0 |
| SOCIAL-04b | POST comment empty body → 400 | unit | `go test ./services/player/internal/handler -run TestCommentHandler_CreateComment_EmptyBody` | ❌ Wave 0 |
| SOCIAL-04c | PATCH by non-owner → 403 | unit | `go test ./services/player/internal/handler -run TestCommentHandler_UpdateComment_NotOwner` | ❌ Wave 0 |
| SOCIAL-04d | DELETE by owner soft-deletes (row remains; excluded from GET) | unit | `go test ./services/player/internal/repo -run TestCommentRepo_SoftDelete` | ❌ Wave 0 |
| SOCIAL-04e | GET pagination — cursor returns next page | unit | `go test ./services/player/internal/repo -run TestCommentRepo_ListByAnime_Cursor` | ❌ Wave 0 |
| SOCIAL-04f | 11th POST in an hour → 429 | unit | `go test ./services/player/internal/service -run TestCommentService_RateLimit` | ❌ Wave 0 |
| SOCIAL-05 | Each comment create writes one `activity_events` row | unit | `go test ./services/player/internal/service -run TestCommentService_EmitsActivity` | ❌ Wave 0 |
| SOCIAL-06a | `?ugc=comments` deep-link mounts Comments tab on first paint | Playwright e2e | `bunx playwright test e2e/comments.spec.ts -g "deep-link"` | ❌ Wave 0 |
| SOCIAL-06b | Tab click updates URL via `router.replace` | Playwright e2e | `bunx playwright test e2e/comments.spec.ts -g "URL persists"` | ❌ Wave 0 |
| SOCIAL-06c | Anonymous user sees login prompt on Comments tab | Playwright e2e | `bunx playwright test e2e/comments.spec.ts -g "anon login prompt"` | ❌ Wave 0 |
| SOCIAL-06d | Logged-in user can post/edit/delete own comment | Playwright e2e | `bunx playwright test e2e/comments.spec.ts -g "logged-in CRUD"` | ❌ Wave 0 |
| SOCIAL-NF-01 | Golden-file diff of six reviews-endpoint responses pre/post-migration | manual smoke (or unit if a fixture is captured) | Pre-deploy: `curl` and stash six JSON responses. Post-deploy: `diff` them. | ❌ Wave 0 manual step |
| SOCIAL-NF-02 | AutoMigrate runs on player startup; bootstrap block runs once | smoke (boot logs) | `make logs-player \| grep "social migration"` shows "complete" on first run, NOTHING on second run | ❌ Manual log inspection — no automated test |

### Sampling Rate
- **Per task commit:** `cd services/player && go test ./internal/{handler,service,repo}/... -short` (≤ 30s)
- **Per wave merge:** `cd services/player && go test ./... -race -cover` + `cd frontend/web && bunx vue-tsc --noEmit && bunx playwright test e2e/comments.spec.ts`
- **Phase gate:** Full suite green + manual smoke checks (DB inspection, golden-file diff) before `/gsd-verify-work`.

### Wave 0 Gaps

- [ ] `services/player/internal/handler/comment.go` — does not exist
- [ ] `services/player/internal/handler/comment_test.go` — does not exist (covers SOCIAL-04a/b/c)
- [ ] `services/player/internal/service/comment.go` — does not exist
- [ ] `services/player/internal/service/comment_test.go` — does not exist (covers SOCIAL-04f, SOCIAL-05)
- [ ] `services/player/internal/repo/comment.go` — does not exist
- [ ] `services/player/internal/repo/comment_test.go` — does not exist (covers SOCIAL-04d, SOCIAL-04e)
- [ ] `services/player/internal/domain/comment.go` — does not exist
- [ ] `frontend/web/e2e/comments.spec.ts` — does not exist (covers SOCIAL-06a..d)
- [ ] Framework install: none — all test packages are already in player's `go.mod` (testify) and the frontend has Playwright wired

**Existing test infrastructure that DOES cover requirements:**
- `services/player/internal/handler/sync_test.go:22 setupSyncTestDB` — in-memory SQLite + manual `CREATE TABLE` — directly reusable as the test-DB factory for comment handler tests. SQLite caveat: it does NOT support `gen_random_uuid()`, so test code must set `Comment.ID` explicitly OR use `uuid.New().String()`.
- `services/player/internal/repo/sync_test.go:15 setupTestDB` — same pattern at the repo layer.
- `services/player/internal/handler/mal_import_test.go:140 TestMALImportHandler_ImportMALList_Success` — example of handler tests injecting `authz.Claims` into the request context. Direct template for `CreateComment` happy path.

**Note for the planner:** Wave 0 should NOT introduce a new test framework — `testify` + SQLite-in-memory is the standard. The "Wave 0 Gaps" list is just file scaffolding (empty test files with table-driven test stubs). It's mechanical, not a research decision.

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Existing JWT (`AuthMiddleware`) on all write endpoints; unchanged |
| V3 Session Management | yes (transitively) | Existing access-token + refresh-cookie flow; no change in this phase |
| V4 Access Control | **yes** | New: owner-or-admin check on PATCH/DELETE. Implementation: `comment.UserID != claims.UserID && !authz.IsAdmin(ctx)` → return `errors.Forbidden(...)` |
| V5 Input Validation | **yes** | New: body 1-2000 UTF-8 runes, trimmed, non-empty. Use `utf8.RuneCountInString` (not `len`). Validate cursor decodes cleanly. |
| V6 Cryptography | no (no new secrets / new crypto) | — |
| V11 Business logic (rate-limit) | **yes** | 10/hour/user/anime — handled at the service layer. Returns 429 (`errors.RateLimited()`). |
| V12 File / Resource handling | no | — |
| V13 API & Web Service | yes | All routes use the existing `httputil.JSON` response wrapper |
| V14 Configuration | no | No new env vars |

### Known Threat Patterns for Go/Chi/GORM stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SQL injection via cursor parameter | Tampering | `pagination.DecodeCursor` rejects invalid base64; GORM `Where("... ? ...", cursor.ID, cursor.Timestamp)` parameterizes; never string-concatenate cursor values into SQL |
| Mass-assignment via `parent_id` | Tampering / Elevation of privilege | DTO `CreateCommentRequest` omits `parent_id` entirely; the field is server-only and stays NULL in v0.1 |
| XSS via comment body | Tampering | Plain text only (per spec); Vue's `{{ }}` interpolation auto-escapes; never use `v-html` with comment body |
| Cross-user PATCH/DELETE | Spoofing | Owner check via `claims.UserID` from JWT; admin override via `authz.IsAdmin(ctx)` |
| DoS via comment flood | Denial of Service | Per-user-anime rate limit (10/hour); gateway IP rate limit still active globally |
| Stored XSS in username | Tampering | Username comes from JWT claims, which originated from user-controlled `auth/register`. Existing reviews already display the same username without escaping concerns (Vue `{{ }}` escapes). Verify the avatar `slice(0,2).toUpperCase()` doesn't break on unicode (it can — but it can't inject HTML; only render garbled). |
| Cursor enumeration | Information disclosure | The cursor encodes `(timestamp, id)` — both already public on the comment, so no leak |
| Race condition: two PATCH for same comment | Data integrity | Last-write-wins via `UpdatedAt`; acceptable for this scope |
| `gorm.DeletedAt` bypass via raw SQL | Tampering | All comments queries go through the typed `CommentRepository`; no handler issues raw SQL that bypasses the `deleted_at IS NULL` filter |

**Note on rate-limit fairness:** the in-memory bucket lives per-process. If/when player runs N replicas, the effective limit is 10×N per hour per (user, anime). This is acceptable for v0.1 per CONTEXT.md, and is also acceptable from a security standpoint (the gateway's IP rate limit still bounds the total request rate).

## Sources

### Primary (HIGH confidence — verified in this session)

- **Codebase: `services/player/cmd/player-api/main.go`** — AutoMigrate block (50-65), phase-3 backfill (188-213), service wiring (215-296). Direct precedent for the bootstrap migration block.
- **Codebase: `services/player/internal/domain/watch.go`** — `AnimeListEntry` (58-79), `Review` (105-119) struct shapes.
- **Codebase: `services/player/internal/domain/activity.go`** — `ActivityEvent` with `gorm.DeletedAt` (9-21).
- **Codebase: `services/player/internal/service/review.go`** — `CreateOrUpdateReview` activity emission template (30-102).
- **Codebase: `services/player/internal/repo/{review,list,activity}.go`** — Upsert patterns, GORM `clause.OnConflict` usage, cursor pagination via `Limit+1`.
- **Codebase: `services/player/internal/handler/review.go`** — Handler signature pattern.
- **Codebase: `services/player/internal/transport/router.go`** — Protected group nesting, `AuthMiddleware`, route group structure.
- **Codebase: `services/gateway/internal/transport/router.go`** — Reviews routes registered BEFORE catch-all `/anime/*` → catalog (lines 144-153). IPRateLimiter implementation (460-548).
- **Codebase: `libs/pagination/cursor.go`** — `Cursor{ID, Timestamp}.Encode()` + `DecodeCursor` — direct reuse for comments pagination.
- **Codebase: `libs/errors/errors.go`** — Domain error → HTTP status mapping. `RateLimited()` → 429.
- **Codebase: `libs/httputil/response.go`** — `OK`, `Created`, `BadRequest`, `Forbidden`, `TooManyRequests`, `Bind`.
- **Codebase: `libs/authz/jwt.go`** — `IsAdmin(ctx)` helper (192-194) and `ClaimsFromContext`.
- **Codebase: `frontend/web/src/components/ui/Tabs.vue`** — `variant="underline"` + `count` badge + `v-model` (complete file).
- **Codebase: `frontend/web/src/views/Anime.vue`** — Existing Reviews section (590-714), `useRoute/useRouter` already imported, `formatDate` available, `Review` TS interface (816-824).
- **Codebase: `frontend/web/src/api/client.ts`** — `reviewApi` shape (338-357) — template for `commentApi`.
- **Codebase: `frontend/web/src/components/ActivityFeed.vue`** — `actionText` branch logic (143-159), TS event shape (94-107).
- **Codebase: `frontend/web/src/locales/{en,ja,ru}.json`** — `anime.*` namespace at line 40, `activity.*` at 427.
- **Planning: `.planning/workstreams/social/phases/01-social-reviews-comments/01-CONTEXT.md`** — Locked decisions.
- **Planning: `.planning/workstreams/social/phases/01-social-reviews-comments/01-SPEC.md`** — Acceptance criteria, constraints, interview log.
- **Planning: `.planning/workstreams/social/phases/01-social-reviews-comments/01-UI-SPEC.md`** — Visual + interaction contract.
- **Planning: `.planning/codebase/{STACK,STRUCTURE,CONVENTIONS,TESTING}.md`** — Project standards.

### Secondary (MEDIUM confidence)

- **GORM docs** — `clause.OnConflict`, `gorm.DeletedAt` semantics (well-known; existing code uses both correctly).
- **Postgres docs** — `INSERT ... ON CONFLICT (...) DO UPDATE SET ...`, `information_schema.tables`.

### Tertiary (LOW confidence)

- None. Every claim in this research is anchored to either this repo or to standard Postgres/GORM semantics that the repo already uses correctly.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new dependencies; all libs already in the workspace.
- Architecture: HIGH — every pattern has a verbatim precedent in this codebase.
- Pitfalls: HIGH — eight of nine pitfalls are anchored to specific file paths and line numbers in this repo.
- Security: MEDIUM-HIGH — ASVS mapping is correct, but project does NOT have a security_enforcement key in config.json — included as best practice anyway because comments accept user-generated content.
- Migration: HIGH for idempotency (precedent at main.go:192-213); MEDIUM for performance at scale (depends on `reviews` row count — Open Question 1).

**Research date:** 2026-05-13
**Valid until:** 2026-06-12 (30 days — stable internal codebase, no external API dependencies that move quickly)
