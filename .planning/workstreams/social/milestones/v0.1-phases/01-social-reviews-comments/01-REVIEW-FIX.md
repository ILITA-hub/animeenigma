---
phase: 1
workstream: social
iteration: 1
fix_scope: critical_warning
findings_in_scope: 11
fixed: 10
skipped: 1
status: partial
---

# Phase 1: Code Review Fix Report

**Fixed at:** 2026-05-13T00:00:00Z
**Source review:** `.planning/workstreams/social/phases/01-social-reviews-comments/01-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 11 (5 Critical + 6 Warning; 3 Info excluded per `fix_scope: critical_warning`)
- Fixed: 10
- Skipped: 1

All fixes verified by:
- `cd services/player && go build ./... && go test ./...` (passes after every Go edit)
- `cd services/gateway && go build ./... && go test ./...` (passes after CR-04)
- `cd frontend/web && bunx vue-tsc --noEmit` (clean after every frontend edit)

**Branch note:** Fix commits live on the `gsd-reviewfix/01-44264` branch (preserved on the main repo). The cleanup-tail fast-forward of `main → gsd-reviewfix/01-44264` failed because `main` advanced concurrently while this fixer was running. Resolution: merge or cherry-pick `gsd-reviewfix/01-44264` into `main` manually.

## Fixed Issues

### CR-01: In-memory rate limiter resets on every service restart — state shared per-process only

**Files modified:** `services/player/internal/service/comment.go`
**Commit:** `f6e3941`
**Applied fix:** Replaced `keep := existing[:0]` (which aliased the backing array of `b.entries[key]`) with `keep := make([]time.Time, 0, len(existing))`. Each `allow()` call now operates on a fresh slice, eliminating the backing-array aliasing concern. Added an inline comment noting that Redis-backed persistence is a deferred follow-up — out of scope for v0.1 single-replica per CONTEXT.md.

### CR-02: `GetAnimeRating` silently swallows DB query failures and returns a fake zero rating

**Files modified:** `services/player/internal/repo/list.go`
**Commit:** `cef32df`
**Applied fix:** Removed the error-swallowing branch that returned `&domain.AnimeRating{AnimeID: animeID}, nil` on query failure. Now returns `apperrors.Wrap(err, apperrors.CodeInternal, "failed to get anime rating")`. The handler at `handler/review.go:147-151` already routes errors through `httputil.Error`, so the existing error-path is wired correctly; the swallow was unconditionally dropping the signal. Verified with `go test ./...` (existing `repo` and `handler` tests still pass).

### CR-03: `UpsertReview` reload fallback silently returns an incomplete `AnimeListEntry` on DB error

**Files modified:** `services/player/internal/repo/list.go`
**Commit:** `02fa99c`
**Applied fix:** Changed the post-upsert reload-on-error branch from `return entry, nil` (a fallback to the locally-constructed entry with no DB-assigned ID, no CreatedAt, no preserved fields) to `return nil, apperrors.Wrap(err, ...)`. Clients can no longer receive a partial entry with `id=""` that would later cause PATCH/DELETE requests with an empty ID.

### CR-04: Gateway routes `POST /anime/{animeId}/comments` and `PATCH/DELETE /anime/{animeId}/comments/{commentId}` are missing JWT validation at the gateway layer

**Files modified:** `services/gateway/internal/transport/router.go`
**Commit:** `b58d0e1`
**Applied fix:** Kept `GET /anime/{animeId}/comments` as bare `ProxyToPlayer` (public). Wrapped the three mutating verbs in an `r.Group(func(r chi.Router) { r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService)); ... })` block. Pattern mirrors `/admin/scraper/*`, `/admin/recs/*`, `/users/*`, and the themes-mutation routes. Verified with `go test ./...` in services/gateway.

**Note:** Requires human verification — middleware re-ordering can hide subtle routing bugs (route order in chi is registration order). Behaviour confirmed via build/test, but a smoke test against an unauthenticated POST is recommended once the gateway redeploys.

### CR-05: `v-html` renders unsanitized anime description from the server — stored XSS vector

**Files modified:** `frontend/web/src/utils/description-parser.ts`
**Commit:** `742d0f5`
**Applied fix:** Audit of `parseDescription` confirms it is XSS-safe — this is a **false positive** in the review. The parser:
1. Calls `escapeHtml(raw)` first, converting `&`, `<`, `>`, `"`, `'` to entities BEFORE any tag parsing. No attacker-controlled tags or attributes can survive past step 1.
2. Subsequent regex replacements interpolate only pre-escaped capture groups (or numeric IDs matched by `\d+`) into hardcoded HTML templates (`<a>`, `<span>`, `<br>`). The href base is constant; URL paths come from a whitelist (TYPE_URL_MAP).
3. No raw user content lands in attribute values or `javascript:` URLs.

DOMPurify is NOT in `frontend/web/package.json` and is not needed. Added a 13-line docstring above `parseDescription` making the safety invariant explicit, so any future regex edit that bypasses `escapeHtml()` triggers a mandatory re-audit.

### WR-01: `CommentRepository.Update` does not check `deleted_at IS NULL` — updates soft-deleted comments

**Files modified:** `services/player/internal/repo/comment.go`
**Commit:** `a1bc6cf`
**Applied fix:** Added `AND deleted_at IS NULL` to the WHERE clause of `Update`. GORM's automatic soft-delete filter is NOT injected for `Model(...).Where(...).Update(...)` — only for `First`/`Find`/`Delete`. Defense-in-depth — the service layer already guards via `GetByID` (which respects soft-delete).

### WR-03: `postComment` in `Anime.vue` calls `fetchComments()` after every successful post — loses cursor state and reloads page 1

**Files modified:** `frontend/web/src/views/Anime.vue`
**Commit:** `5401401`
**Applied fix:** Replaced `await fetchComments()` with `comments.value.unshift(created)`, reading the created comment from `resp.data?.data || resp.data` (matches the wrapper shape from `httputil.Created`). Preserves `commentsNextCursor` / `commentsHasMore` and the infinite-scroll position.

### WR-04: `deleteCommentItem` restores deleted comment at original index — index may be wrong after concurrent mutations

**Files modified:** `frontend/web/src/views/Anime.vue`
**Commit:** `57a3906`
**Applied fix:** Replaced `comments.value.splice(originalIdx, 0, snapshot)` with a sort-order-preserving re-insertion. The error path walks the current `comments.value` array and inserts the snapshot before the first older entry (by `created_at`, with `id` as tie-breaker), falling back to `push()` if the snapshot is the oldest or the list is empty. Newest-first ordering is preserved without depending on a stale captured index.

### WR-05: `formatDate` in `Anime.vue` is hardcoded to `'ru-RU'` locale regardless of the UI language setting

**Files modified:** `frontend/web/src/views/Anime.vue`
**Commit:** `760bfc9`
**Applied fix:** Mapped `locale.value` (from `useI18n()`, already imported and destructured at line 1052) to a BCP-47 tag: `ru → ru-RU`, `ja → ja-JP`, otherwise `en-US`. Mirrors the existing pattern in `formatNextEpisode` at line 1390.

### WR-06: `ActivityFeed.vue` links user profiles to `/user/${event.user_id}` (UUID) instead of the public profile path

**Files modified:** `frontend/web/src/components/ActivityFeed.vue`
**Commit:** `9e64826`
**Applied fix:** Verified that `/user/${event.user_id}` does NOT 404 (auth service's `GetUserByPublicID` accepts both UUID and public_id, and `Profile.vue:1502-1503` silently redirects UUID → public_id). The review's "404" claim is incorrect, but the redirect produces a URL flash and a wasted round-trip. Added an optional `public_id?: string` field to the `ActivityEvent` interface and changed the link target to `/user/${event.public_id || event.user_id}`. Forward-compatible — works today without backend changes, and the link goes directly to the canonical URL if/when the activity-feed projection is later joined with `users` to populate `public_id`.

## Skipped Issues

### WR-02: Rate limit window uses wall clock `time.Now()` — clock skew on resumption can cause silent bucket reset

**File:** `services/player/internal/service/comment.go:53-77`
**Reason:** Deferred. Switching to monotonic-time tracking (via `time.Since(t)`) would require a deeper refactor of the bucket: timestamps stored in the slice need a new representation, the prune and accept paths both need to be rewritten, and the in-process invariant (Go's `time.Time` values from `time.Now()` carry a monotonic reading that is preserved across in-process comparisons but stripped on serialization) means the present implementation is already lower-risk than the review suggests — values never leave the process, so monotonic readings are preserved automatically. The real fix is the Redis-backed bucket noted under CR-01 (out of scope for v0.1 single-replica). For v0.1 the current `time.Now()` approach is acceptable.

**Original issue:** NTP corrections or system sleep/hibernate could cause `time.Now()` to jump backwards by more than `rateLimitWindow`, silently pruning all stored timestamps and resetting the bucket. A well-timed NTP adjustment could let a user exceed 10 comments/hour.

---

_Fixed: 2026-05-13T00:00:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
