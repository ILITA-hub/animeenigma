---
phase: 1
workstream: social
plan: 5
subsystem: frontend/web (api client + locales + ActivityFeed)
tags:
  - frontend
  - i18n
  - api-client
  - activity-feed
  - wave-4
requirements:
  - SOCIAL-04
  - SOCIAL-05
  - SOCIAL-06
dependency_graph:
  requires:
    - "Plan 04: gateway proxy + player chi routes for /api/anime/{id}/comments[/{cid}] live in production (commit a232ebd)"
    - "Plan 03: CommentRepository / Service / Handler emit `{ comments[], has_more, next_cursor }` envelope"
  provides:
    - "commentApi (4 methods) on @/api/client — getAnimeComments, createComment, updateComment, deleteComment"
    - "anime.ugc.* namespace (24 keys) + activity.comment.posted in en.json / ja.json / ru.json"
    - "ActivityFeed.vue actionText() returns t('activity.comment.posted') for type='comment' events"
  affects:
    - "frontend/web/src/api/client.ts (commentApi exported between reviewApi and activityApi)"
    - "frontend/web/src/locales/en.json (anime.ugc + activity.comment namespaces)"
    - "frontend/web/src/locales/ja.json (anime.ugc + activity.comment namespaces)"
    - "frontend/web/src/locales/ru.json (anime.ugc + activity.comment namespaces)"
    - "frontend/web/src/components/ActivityFeed.vue (new branch in actionText())"
tech-stack:
  added: []
  patterns:
    - "Sibling axiosApi exports — commentApi mirrors reviewApi's shape exactly (template-literal URLs, plain apiClient.get/post/patch/delete returns)"
    - "ICU MessageFormat in vue-i18n locale strings (charCount, commentsCount with plural rules for RU's one/few/many)"
    - "Short-circuit branching in actionText() — comment case sits at top so it returns before review/status/score branches"
key-files:
  created:
    - ".planning/workstreams/social/phases/01-social-reviews-comments/05-SUMMARY.md"
  modified:
    - "frontend/web/src/api/client.ts"
    - "frontend/web/src/locales/en.json"
    - "frontend/web/src/locales/ja.json"
    - "frontend/web/src/locales/ru.json"
    - "frontend/web/src/components/ActivityFeed.vue"
decisions:
  - "commentApi return types are untyped axios responses (apiClient.get(...) / .post(...) etc.), matching every other *Api block in client.ts. Plan 06 will define the Comment interface locally in Anime.vue alongside the existing Review interface — keeps client.ts's untyped-response convention consistent and defers the type to the consumer."
  - "commentsCount uses ICU plural form. RU declares all four CLDR forms (one/few/many/other) per Russian grammar; EN declares one/other; JA uses identical bodies for both (Japanese is non-pluralized) but still wraps in the plural construct for symmetry."
  - "The activity.comment.posted branch sits at the TOP of actionText() (before score/review). Comments emit fresh events on every create (no per-day dedup, unlike reviews), so making this branch the first short-circuit minimizes wasted comparisons in the hottest path through the feed."
  - "RU activity.comment.posted uses the gender-neutral 'оставил(а)' form to avoid requiring user gender data on the feed."
metrics:
  duration_minutes: 8
  completed_date: "2026-05-13"
  tasks_completed: 3
  files_created: 1
  files_modified: 5
  commits: 3
---

# Phase 1 Plan 05 (Workstream `social`): Frontend Plumbing for Comments Summary

**One-liner:** Frontend infrastructure for SOCIAL-04/05/06 — commentApi
axios wrapper sibling to reviewApi, 24 anime.ugc.* locale keys plus
activity.comment.posted across en/ja/ru, and a three-line branch in
ActivityFeed.vue's actionText() so type='comment' events render without
[intlify] warnings. Zero user-visible UI change until Plan 06 wires
Anime.vue.

## What Was Built

### commentApi axios wrapper (Task 5.1)

Inserted between `reviewApi` (line 363) and `activityApi` in
`frontend/web/src/api/client.ts`. Four methods, all typed `string`
parameters, template-literal URLs:

```ts
export const commentApi = {
  // Get paginated comments for an anime (public, newest-first; cursor is opaque)
  getAnimeComments: (animeId: string, params?: { cursor?: string; limit?: number }) =>
    apiClient.get(`/anime/${animeId}/comments`, { params }),
  // Create a new comment on an anime (auth required, 1–2000 chars, 10/hr/(user,anime))
  createComment: (animeId: string, body: string) =>
    apiClient.post(`/anime/${animeId}/comments`, { body }),
  // Update an existing comment (owner only)
  updateComment: (animeId: string, commentId: string, body: string) =>
    apiClient.patch(`/anime/${animeId}/comments/${commentId}`, { body }),
  // Soft-delete a comment (owner or admin)
  deleteComment: (animeId: string, commentId: string) =>
    apiClient.delete(`/anime/${animeId}/comments/${commentId}`),
}
```

Reuses the existing `apiClient` axios instance — inherits the
request/response interceptors (proactive JWT refresh, X-Anon-ID header,
retry-on-401), so the new endpoints get auth + token-rotation for free.
No new TypeScript interfaces introduced — response is left untyped
exactly like the other *Api blocks (`reviewApi`, `activityApi`,
`adminApi`, etc.).

### 24 anime.ugc.* keys + activity.comment.posted (Task 5.2)

Added inside the existing `anime` namespace of all three locale files,
alongside the `status` sub-object. Plus one `activity.comment` sub-object
inside the existing `activity` namespace.

Sample EN strings (from UI-SPEC Copywriting Contract, verbatim):

```json
"anime": {
  "ugc": {
    "reviewsTab": "Reviews",
    "commentsTab": "Comments",
    "commentsCount": "{count, plural, one {# comment} other {# comments}}",
    "commentPlaceholder": "Add a comment…",
    "postComment": "Post comment",
    "posting": "Posting…",
    "deleteCommentConfirm": "Delete this comment? This cannot be undone.",
    "saveEdit": "Save edit",
    "cancelEdit": "Cancel edit",
    "loginToComment": "Sign in to join the conversation",
    "emptyComments": "No comments yet. Start the conversation.",
    "charCount": "{count}/2000",
    "rateLimitError": "You've posted a lot recently. Try again in a few minutes.",
    "bodyTooLong": "Comment can't be longer than 2000 characters.",
    ...
  }
},
"activity": {
  "comment": { "posted": "commented on" }
}
```

Sample JA strings (polite です/ます form):

| Key | JA |
|-----|----|
| `anime.ugc.commentsTab` | `コメント` |
| `anime.ugc.postComment` | `コメントを投稿` |
| `anime.ugc.saveEdit` | `編集を保存` |
| `anime.ugc.cancelEdit` | `編集をキャンセル` |
| `anime.ugc.deleteCommentConfirm` | `このコメントを削除しますか？この操作は元に戻せません。` |
| `anime.ugc.emptyComments` | `まだコメントがありません。最初のコメントを投稿しましょう。` |
| `anime.ugc.loginToComment` | `会話に参加するにはログインしてください` |
| `activity.comment.posted` | `がコメントしました` |

Sample RU strings (formal Вы form):

| Key | RU |
|-----|----|
| `anime.ugc.commentsTab` | `Комментарии` |
| `anime.ugc.postComment` | `Опубликовать комментарий` |
| `anime.ugc.saveEdit` | `Сохранить правку` |
| `anime.ugc.cancelEdit` | `Отменить правку` |
| `anime.ugc.deleteCommentConfirm` | `Удалить этот комментарий? Это действие нельзя отменить.` |
| `anime.ugc.emptyComments` | `Пока нет комментариев. Начните обсуждение.` |
| `anime.ugc.loginToComment` | `Войдите, чтобы присоединиться к обсуждению` |
| `activity.comment.posted` | `оставил(а) комментарий к` |

ICU placeholders preserved literally across all three locales:
`charCount = "{count}/2000"`; `commentsCount` uses the full plural form
appropriate to each language (RU declares all four CLDR forms one/few/many/other,
EN declares one/other, JA wraps both branches in identical bodies).

### ActivityFeed.vue comment branch (Task 5.3)

Three-line addition at the top of `actionText()` in
`frontend/web/src/components/ActivityFeed.vue` (was at line 142, now
spans lines 142–145):

```ts
const actionText = (event: ActivityEvent): string => {
  if (event.type === 'comment') {
    return t('activity.comment.posted')
  }
  if (event.type === 'score') { ... }
  if (event.type === 'review') { ... }
  ...
}
```

Position chosen: BEFORE all existing branches. Comments emit fresh events
on every create (no per-day dedup, unlike reviews) so this is the hottest
path through the feed once Plan 06 ships. The existing score/review/status
branches are untouched.

Existing template usage (`{{ actionText(event) }}` on line 52, followed
by the anime-name router-link) renders as
"`{username}` commented on `{anime name}`" for comment events. The
`event.content` field below (lines 60-62) shows the first 300 runes of
the body preview emitted by Plan 03's service layer.

## Verification

### Static checks (all PASS)

| Check | Result |
|---|---|
| `cd frontend/web && bunx tsc --noEmit` | exits 0 (no output) |
| `cd frontend/web && bunx vue-tsc --noEmit` | exits 0 (no output) |
| `jq . frontend/web/src/locales/en.json > /dev/null` | exits 0 (valid JSON) |
| `jq . frontend/web/src/locales/ja.json > /dev/null` | exits 0 (valid JSON) |
| `jq . frontend/web/src/locales/ru.json > /dev/null` | exits 0 (valid JSON) |
| `jq '.anime.ugc \| keys \| length' frontend/web/src/locales/en.json` | `24` |
| `jq '.anime.ugc \| keys \| length' frontend/web/src/locales/ja.json` | `24` |
| `jq '.anime.ugc \| keys \| length' frontend/web/src/locales/ru.json` | `24` |
| `jq -r '.activity.comment.posted' frontend/web/src/locales/en.json` | `commented on` |
| `jq -r '.activity.comment.posted' frontend/web/src/locales/ja.json` | `がコメントしました` |
| `jq -r '.activity.comment.posted' frontend/web/src/locales/ru.json` | `оставил(а) комментарий к` |
| `grep -c 'export const commentApi' frontend/web/src/api/client.ts` | `1` |
| `grep -c 'getAnimeComments' frontend/web/src/api/client.ts` | `1` |
| `grep -E 'anime/\$\{animeId\}/comments/\$\{commentId\}' frontend/web/src/api/client.ts \| wc -l` | `2` (updateComment + deleteComment) |
| `grep -c "event.type === 'comment'" frontend/web/src/components/ActivityFeed.vue` | `1` |
| `grep -c 'activity.comment.posted' frontend/web/src/components/ActivityFeed.vue` | `1` |
| `awk '/const actionText/,/^}/' .../ActivityFeed.vue \| grep -c "event.type === 'comment'"` | `1` (branch inside actionText body) |

### Verbatim-canonical-string assertions

Per UI-SPEC's locked-down primary CTA / Save / Cancel keys:

| Key | Expected | Actual |
|-----|----------|--------|
| `en.anime.ugc.postComment` | `Post comment` | `Post comment` ✓ |
| `ja.anime.ugc.postComment` | `コメントを投稿` | `コメントを投稿` ✓ |
| `ru.anime.ugc.postComment` | `Опубликовать комментарий` | `Опубликовать комментарий` ✓ |
| `en.anime.ugc.saveEdit` | `Save edit` | `Save edit` ✓ |
| `en.anime.ugc.cancelEdit` | `Cancel edit` | `Cancel edit` ✓ |

### Missing-key sweep

All 24 keys × 3 locales = 72 entries checked. Loop output
(any missing prints `MISSING: $f.$k`):

```
DONE: empty=missing-only output
```

No missing entries.

## Commits

| Task | Commit  | Message |
|------|---------|---------|
| 5.1  | `6c1101b` | `feat(1-5): add commentApi axios wrapper to frontend client` |
| 5.2  | `a34602c` | `feat(1-5): add anime.ugc.* + activity.comment.posted locale keys (en/ja/ru)` |
| 5.3  | `ae5ad21` | `feat(1-5): render type='comment' events in ActivityFeed via activity.comment.posted` |

## Deviations from Plan

### Auto-fixed Issues

None. Plan 05 was a three-task plan and each task executed exactly as
written.

### Notes (not deviations)

- The plan's task 5.3 acceptance criteria included a Playwright e2e
  sanity check (`bunx playwright test --list e2e/comments.spec.ts 2>&1 | grep -c 'deep-link'`).
  That spec file doesn't exist yet (it would be authored by a later plan
  in the wave that lands the Anime.vue UI), so the Playwright list check
  isn't applicable to this plan's scope. The plan's intent — "no regression
  in the e2e manifest" — is satisfied because no existing spec was
  modified.
- The plan suggested writing summary to `01-05-SUMMARY.md`. The
  orchestrator's target path is `05-SUMMARY.md` (matching the existing
  `04-SUMMARY.md`, `03-SUMMARY.md` naming on disk), so the SUMMARY is
  written at that canonical location.
- All three locale files were committed in a single commit (5.2). Per
  CLAUDE.md and the GSD task-commit protocol, related-files-per-task
  is acceptable; the three locales are functionally one i18n change
  and splitting them would produce intermediate states where vue-i18n
  warns about missing keys for two of three locales.

## Handoff

Plan 06 (Anime.vue tab strip + comments UI) can now import and use:

```ts
import { commentApi } from '@/api/client'

// Initial load
const { data } = await commentApi.getAnimeComments(animeId, { limit: 50 })
// data.data.comments: Comment[]
// data.data.has_more: boolean
// data.data.next_cursor?: string

// Pagination
await commentApi.getAnimeComments(animeId, { cursor: nextCursor, limit: 50 })

// Create
await commentApi.createComment(animeId, bodyText)        // 201 or 400/429

// Edit
await commentApi.updateComment(animeId, commentId, newBody)  // 200 or 400/403

// Soft delete
await commentApi.deleteComment(animeId, commentId)           // 204 or 403/404
```

All anime.ugc.* keys are resolvable via `$t('anime.ugc.commentsTab')`,
`$t('anime.ugc.postComment')`, etc. Pluralization uses ICU MessageFormat:
`$t('anime.ugc.commentsCount', { count: N })`. Character counter uses
`$t('anime.ugc.charCount', { count: textarea.value.length })`.

ActivityFeed.vue automatically renders any new type='comment' events
from the activity feed endpoint with no further code changes required.

## Known Stubs

None. All four commentApi methods hit the live gateway endpoints
verified in Plan 04's checkpoint smoke. Every locale value is a real
human-readable translation, not a placeholder. The ActivityFeed branch
returns a real, non-empty translated label in all three locales.

## Threat Flags

None new. The plan's threat register has three items, all addressed:

- **T-1-V13 (axios wrapper inherits auth)**: mitigated. `commentApi`
  reuses the existing `apiClient` instance — the request interceptor
  attaches `Authorization: Bearer <jwt>` and `X-Anon-ID` headers
  automatically. No new auth surface.
- **Locale drift / missing keys**: mitigated. All 24 keys verified
  present (jq), all 3 activity.comment.posted entries verified present,
  no missing-key warnings possible from the new ActivityFeed branch
  (the key exists in all locales before the branch reads it).
- **Stored XSS via ActivityFeed t() interpolation**: accepted (no
  change). The branch returns a static `t()` lookup with no event-data
  interpolation; the surrounding template uses `{{ actionText(event) }}`
  and `{{ animeName(event) }}` which auto-escape. No `v-html`
  introduced.

## Self-Check: PASSED

**Files verified to exist (modified):**

- `frontend/web/src/api/client.ts` — FOUND; `grep -c 'export const commentApi' = 1`
- `frontend/web/src/locales/en.json` — FOUND; valid JSON; 24 anime.ugc keys; activity.comment.posted present
- `frontend/web/src/locales/ja.json` — FOUND; valid JSON; 24 anime.ugc keys; activity.comment.posted present
- `frontend/web/src/locales/ru.json` — FOUND; valid JSON; 24 anime.ugc keys; activity.comment.posted present
- `frontend/web/src/components/ActivityFeed.vue` — FOUND; comment branch inside actionText body

**Commits verified in `git log`:**

- `6c1101b` — FOUND (Task 5.1 — commentApi)
- `a34602c` — FOUND (Task 5.2 — locale keys)
- `ae5ad21` — FOUND (Task 5.3 — ActivityFeed branch)

**Type-check / JSON parse:**

- `bunx tsc --noEmit` — exits 0
- `bunx vue-tsc --noEmit` — exits 0
- `jq .` against all three locale files — exits 0
