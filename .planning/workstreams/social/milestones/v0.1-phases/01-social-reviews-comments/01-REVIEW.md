---
phase: 01-social-reviews-comments
reviewed: 2026-05-13T00:00:00Z
depth: standard
files_reviewed: 23
files_reviewed_list:
  - services/player/internal/domain/comment.go
  - services/player/internal/domain/watch.go
  - services/player/internal/repo/comment.go
  - services/player/internal/repo/comment_test.go
  - services/player/internal/repo/list.go
  - services/player/internal/service/comment.go
  - services/player/internal/service/comment_test.go
  - services/player/internal/service/review.go
  - services/player/internal/handler/comment.go
  - services/player/internal/handler/comment_test.go
  - services/player/internal/handler/review.go
  - services/player/internal/transport/router.go
  - services/player/cmd/player-api/main.go
  - services/player/cmd/player-api/main_test.go
  - services/gateway/internal/transport/router.go
  - frontend/web/src/api/client.ts
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ja.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/components/ActivityFeed.vue
  - frontend/web/src/views/Anime.vue
  - frontend/web/e2e/comments.spec.ts
  - scripts/capture-reviews-fixtures.sh
findings:
  critical: 5
  warning: 6
  info: 3
  total: 14
status: issues_found
---

# Phase 01: Code Review Report

**Reviewed:** 2026-05-13T00:00:00Z
**Depth:** standard
**Files Reviewed:** 23
**Status:** issues_found

## Summary

This phase ships the social reviews + comments feature: schema merge of the legacy `reviews` table into `anime_list`, a new `comments` table with CRUD endpoints, cursor pagination, rate limiting, soft delete, and UGC tab UI in `Anime.vue`. The overall architecture is sound and the critical security surfaces (owner check, auth gating, mass-assignment defense on `parent_id`) are in place. However, five blocker-level issues were found that represent incorrect behavior or security risks, and six warnings that degrade robustness or create subtle bugs under realistic conditions.

---

## Critical Issues

### CR-01: In-memory rate limiter resets on every service restart — state shared per-process only

**File:** `services/player/internal/service/comment.go:41-76`

**Issue:** The rate limiter is a plain in-memory `sync.Mutex`-guarded map inside `rateBucket`. Every service restart (rolling redeploy, crash, OOM kill) resets all per-user, per-anime counters to zero. A user blocked at 10 comments/hour can bypass the limit immediately by triggering a restart (or simply waiting through the next deploy window). On a self-hosted instance where deploys happen frequently, this degrades the limit to a soft suggestion rather than a hard gate. For a first phase this is a known documented trade-off ("acceptable for v0.1 single-replica per CONTEXT.md"), but the CONTEXT.md reference is incorrect — no such justification appears in the reviewed files — making this an undocumented behavioral gap rather than an explicit design choice.

More critically, the in-memory bucket is also NOT shared across goroutines correctly under load: the `keep = existing[:0]` slice re-use pattern (line 64) aliases the underlying array. If two goroutines call `allow` concurrently for the same key, the slice returned by `b.entries[key]` at line 62 can be modified by one goroutine's compaction loop while the other goroutine reads it, even though the Mutex is held — both operations happen inside the lock, so the race is not with the lock but with the aliased backing array when the same slice header is re-stored. The safer fix is to allocate a fresh slice each call.

**Fix:**
```go
// Instead of aliasing:
keep := existing[:0]
// Allocate a fresh slice to avoid backing-array aliasing on re-entry:
keep := make([]time.Time, 0, len(existing))
for _, t := range existing {
    if t.After(cutoff) {
        keep = append(keep, t)
    }
}
```

For the persistence gap, persist rate-limit counters in Redis (the player service already depends on Redis as of Phase 10).

---

### CR-02: `GetAnimeRating` silently swallows DB query failures and returns a fake zero rating

**File:** `services/player/internal/repo/list.go:318-338`

**Issue:** When the `Raw(...)` query fails (e.g., DB connection timeout, table lock), the function returns a zeroed `AnimeRating` struct with `AverageScore = 0` and `TotalReviews = 0` — indistinguishable from a legitimate "no ratings yet" response. The error is discarded entirely with the comment "a failed rating lookup should not blow up the anime detail page." This is incorrect behavior: an anime with thousands of ratings will silently display `0.0 (0 reviews)` to every user during any DB hiccup. The comment's goal (don't blow up the page) is valid, but the implementation loses the error signal. Callers cannot distinguish "really no ratings" from "DB is down."

**Fix:**
```go
func (r *ListRepository) GetAnimeRating(ctx context.Context, animeID string) (*domain.AnimeRating, error) {
    // ...
    err := r.db.WithContext(ctx).Raw(...).Scan(&result).Error
    if err != nil {
        // Return the error so callers can handle (e.g., serve stale cache or
        // omit the rating widget). Do NOT silently zero-out a real rating.
        return nil, errors.Wrap(err, errors.CodeInternal, "failed to get anime rating")
    }
    return &domain.AnimeRating{...}, nil
}
```

The handler already has an error path via `httputil.Error`; use it.

---

### CR-03: `UpsertReview` reload fallback silently returns an incomplete `AnimeListEntry` on DB error

**File:** `services/player/internal/repo/list.go:292-299`

**Issue:** After the `ON CONFLICT` upsert succeeds, the code reloads the canonical row with a second query. If that second query fails, it falls back to returning the locally-constructed `entry` — but that `entry` does not have `ID` populated (the PK is set by Postgres `gen_random_uuid()`), does not have `CreatedAt` from the DB, and does not have the preserved pre-existing fields (status, episodes, notes, etc.) for the conflict-update path. The handler calls `toReviewResponse(entry)`, which returns a response with an empty `id` field. Clients that cache or use this `id` to perform a PATCH or DELETE will send a request with an empty ID.

```go
// Fall back to the constructed entry if reload fails
return entry, nil // entry.ID == "" on Postgres
```

**Fix:**
```go
var fresh domain.AnimeListEntry
if err := r.db.WithContext(ctx).
    Where("user_id = ? AND anime_id = ?", userID, animeID).
    First(&fresh).Error; err != nil {
    // Propagate the error instead of returning a partial entry with no ID
    return nil, errors.Wrap(err, errors.CodeInternal, "failed to reload review after upsert")
}
return &fresh, nil
```

---

### CR-04: Gateway routes `POST /anime/{animeId}/comments` and `PATCH/DELETE /anime/{animeId}/comments/{commentId}` are missing JWT validation at the gateway layer

**File:** `services/gateway/internal/transport/router.go:152-155`

**Issue:** The gateway registers all four comment routes as bare `ProxyToPlayer` with no middleware:

```go
r.Get("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
r.Post("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
r.Patch("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
r.Delete("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
```

The public GET is correctly unauthenticated. However, POST/PATCH/DELETE also have no `JWTValidationMiddleware` at the gateway — auth is enforced only downstream in the player's `AuthMiddleware`. The player enforces auth correctly, so the request will ultimately be rejected. But the gateway is meant to be a defense-in-depth layer that validates JWTs before proxying to services, which is explicitly noted in the router for every other protected route. An attacker can send arbitrary unauthenticated requests directly to the POST/PATCH/DELETE endpoints and the gateway will proxy them all to the player service, increasing the attack surface of the player service and negating the gateway's rate-limit-before-auth protection order. Compare with review routes: `r.Post("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)` — same gap exists there too, but that predates this phase.

**Fix:**
```go
// In gateway/internal/transport/router.go, wrap mutating comment routes:
r.Group(func(r chi.Router) {
    r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
    r.Post("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
    r.Patch("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
    r.Delete("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
})
r.Get("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer) // public
```

---

### CR-05: `v-html` renders unsanitized anime description from the server — stored XSS vector

**File:** `frontend/web/src/views/Anime.vue:279`

**Issue:** The anime synopsis is rendered via `v-html`:

```vue
<p ... v-html="parsedDescription" />
```

`parsedDescription` is the output of `parseDescription(anime.value.description)`. The description comes from Shikimori's API and is stored in the catalog service's DB. If `parseDescription` does not strictly strip all HTML tags and only emits safe subsets, any HTML/JS in a description (e.g., a `<script>` tag or `onerror` attribute injected via Shikimori or a compromised catalog DB row) will execute in users' browsers. This is a stored XSS vector affecting all visitors to any anime page. The `parseDescription` function is not in scope for this review, but the pattern of feeding server-controlled HTML into `v-html` without visible sanitization must be flagged.

**Fix:** Audit `parseDescription` to ensure it DOMPurify-sanitizes or strictly allowlists output before emitting. If it does text-only formatting (newline → `<br>`), replace `v-html` with a computed that returns an array of `<p>` elements or use `whitespace-pre-wrap` on a plain text binding:

```vue
<!-- Safe alternative if only newlines need formatting -->
<p class="... whitespace-pre-wrap">{{ anime.description }}</p>
```

---

## Warnings

### WR-01: `CommentRepository.Update` does not check `deleted_at IS NULL` — updates soft-deleted comments

**File:** `services/player/internal/repo/comment.go:97-109`

**Issue:** The `Update` method uses `Where("id = ?", id)` without GORM's soft-delete filter:

```go
res := r.db.WithContext(ctx).
    Model(&domain.Comment{}).
    Where("id = ?", id).
    Update("body", body)
```

GORM's automatic `WHERE deleted_at IS NULL` clause is only injected when the model is used with `Find`/`First`/`Delete` — NOT with `Model(&T{}).Where(...).Update(...)` when `T` is not scoped via `db.Model` with the actual type that carries `gorm.DeletedAt`. Verify: a soft-deleted comment row can have its `body` updated via PATCH if the caller knows the comment's UUID (e.g., an admin who obtained it before deletion). The service checks `GetByID` first (which respects soft-delete), so this is a defense-in-depth gap rather than a direct user-reachable path — but it's still a correctness bug.

**Fix:**
```go
res := r.db.WithContext(ctx).
    Where("id = ? AND deleted_at IS NULL", id).
    Model(&domain.Comment{}).
    Update("body", body)
```

---

### WR-02: Rate limit window uses wall clock `time.Now()` — clock skew on resumption can cause silent bucket reset

**File:** `services/player/internal/service/comment.go:53-77`

**Issue:** The rate limiter prunes entries with `t.After(cutoff)` where `cutoff = now.Add(-rateLimitWindow)`. On systems with NTP corrections or after a system sleep/hibernate, `time.Now()` can jump backwards. If it jumps back by more than `rateLimitWindow` (1 hour), ALL stored timestamps are in the "future" relative to the new `cutoff`, so they are all pruned and the bucket resets. A well-timed NTP adjustment could let a user post significantly more than 10 comments in an hour. Use monotonic time (via `time.Since` instead of comparing `time.Time` values directly) or at minimum document the limitation.

**Fix:** Use `time.Duration` deltas with `time.Since(t) < rateLimitWindow` instead of absolute `time.Time` comparisons. `time.Time` values from `time.Now()` include monotonic readings, but stored values (serialized and deserialized, or compared across goroutines) strip it. The current in-memory slice approach does preserve the monotonic clock since values are only stored and compared in-process without serialization, so the real risk is lower — but the design is fragile. Document this assumption or switch to a Redis sorted-set TTL approach.

---

### WR-03: `postComment` in `Anime.vue` calls `fetchComments()` after every successful post — loses cursor state and reloads page 1

**File:** `frontend/web/src/views/Anime.vue:1617-1621`

**Issue:** After a successful `createComment` call, the frontend calls `fetchComments()` (which fetches page 1 with no cursor). This resets `commentsNextCursor`, `commentsHasMore`, and replaces `comments.value` entirely. If the user had scrolled through several pages of comments before posting, they lose their position and are snapped back to page 1. Additionally, the new comment is guaranteed to appear at the top of the list (newest-first ordering), so the user will see it — but any comments that were on pages 2+ that they had already loaded are now gone from the rendered list.

**Fix:** Instead of refetching all comments, prepend the newly returned comment object (from the API response) to `comments.value`:

```typescript
const resp = await commentApi.createComment(anime.value.id, trimmed)
const created: Comment = resp.data?.data || resp.data
if (created?.id) {
  comments.value.unshift(created)
  commentsFetched.value = true
}
newCommentBody.value = ''
```

---

### WR-04: `deleteCommentItem` restores deleted comment at original index — index may be wrong after concurrent mutations

**File:** `frontend/web/src/views/Anime.vue:1691-1708`

**Issue:** The optimistic delete pattern captures `originalIdx` before removing the comment, then on failure calls `comments.value.splice(originalIdx, 0, snapshot)`. If between the optimistic removal and the error recovery the user (or a concurrent `loadMoreComments` call) mutates the `comments` array (e.g., `loadMoreComments` appends new entries), the `originalIdx` is now stale and the comment is restored at the wrong position. This is a minor correctness issue in a race condition but produces visible list corruption.

**Fix:** Restore by re-inserting at index 0 (newest-first), since the comment was at the top if it was recently added, or re-fetch the first page to get a consistent state:

```typescript
// Simple: restore at position 0 rather than using a stale index
comments.value.unshift(snapshot)
```

---

### WR-05: `formatDate` in `Anime.vue` is hardcoded to `'ru-RU'` locale regardless of the UI language setting

**File:** `frontend/web/src/views/Anime.vue:1363-1370`

**Issue:**
```typescript
const formatDate = (dateStr: string) => {
  const date = new Date(dateStr)
  return date.toLocaleDateString('ru-RU', { ... })  // always Russian
}
```

This function formats review and comment creation dates. English and Japanese users will see dates formatted in Russian conventions (e.g., "13 мая 2026 г.") on review/comment cards. The `locale` ref from `useI18n()` is available in scope but not used.

**Fix:**
```typescript
const formatDate = (dateStr: string) => {
  const date = new Date(dateStr)
  const loc = locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US'
  return date.toLocaleDateString(loc, { day: 'numeric', month: 'long', year: 'numeric' })
}
```

---

### WR-06: `ActivityFeed.vue` links user profiles to `/user/${event.user_id}` (UUID) instead of the public profile path

**File:** `frontend/web/src/components/ActivityFeed.vue:46-50`

**Issue:**
```vue
<router-link :to="`/user/${event.user_id}`" ...>
  {{ event.username }}
</router-link>
```

`event.user_id` is a UUID (e.g., `a3b4c5d6-...`). The public profile route uses `public_id` (a user-chosen slug like `johndoe`), not the internal UUID. Clicking a username in the activity feed navigates to `/user/<uuid>`, which will 404 or show "user not found" for every user. The `ActivityEvent` interface in the component does not include a `public_id` field, so the backend likely does not return it.

**Fix:** The `ActivityEvent` domain model and the activity feed API response need to include `public_id`. Alternatively, the activity feed endpoint can resolve `public_id` server-side before returning events. Update the interface and the link target:

```vue
<router-link :to="`/user/${event.public_id || event.user_id}`" ...>
```

And add `public_id` to the backend `ActivityEvent` projection.

---

## Info

### IN-01: Japanese locale `recs` section is untranslated — falls back to English

**File:** `frontend/web/src/locales/ja.json:43-49`

**Issue:** The `recs` block in the Japanese locale file contains four English strings:

```json
"recs": {
  "trending": "Trending now",
  "upNext": "Up Next for you",
  "empty": "No recommendations yet",
  "pinBadge": "PINNED",
  ...
}
```

The Russian locale has proper translations. The Japanese locale also has untranslated `admin.recs` keys (lines 632-664), but those are admin-only. The `recs` section affects the homepage for Japanese-locale users.

**Fix:** Translate the `recs` keys to Japanese, matching the structure in `ru.json`.

---

### IN-02: `capture-reviews-fixtures.sh` uses `--fail-with-body` which is only available in curl 7.76+

**File:** `scripts/capture-reviews-fixtures.sh:52-53`

**Issue:**
```bash
curl -sS --max-time 10 "$@" || echo "{\"error\":\"curl failed for ${label}\"}"
```

Wait — actually the script comments on line 52 mention `--fail-with-body` but the actual invocation does not use it (the flag was described in the comment, not in the actual curl call). The script falls through correctly via the `|| echo` fallback. This is a comment/documentation accuracy issue: the comment says `--fail-with-body to print the body on non-2xx but still exit non-zero` but the actual flag is absent from the curl invocation. Not a bug, but misleading.

**Fix:** Either add `--fail-with-body` to the curl call (requires curl >= 7.76) with a version check, or remove the misleading comment.

---

### IN-03: `commentBodyMaxRunes = 2000` validation not applied to `UpdateCommentRequest` body in tests

**File:** `services/player/internal/handler/comment_test.go` — no test for over-length PATCH body

**Issue:** `TestCommentHandler_CreateComment_EmptyBody` covers the 400 path for POST, but there is no test asserting that a PATCH with a body > 2000 runes also returns 400. The `UpdateComment` handler calls `validateBody` from the service, so the logic is correct — but the test gap means a future refactor could drop the validation from the update path without a failing test.

**Fix:** Add a `TestCommentHandler_UpdateComment_BodyTooLong` test that patches an existing comment with a 2001-rune body and asserts HTTP 400 + `INVALID_INPUT` error code.

---

_Reviewed: 2026-05-13T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
