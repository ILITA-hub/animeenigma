---
phase: 1
workstream: social
plan: 5
type: execute
wave: 3
depends_on: [3]
files_modified:
  - frontend/web/src/api/client.ts
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ja.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/components/ActivityFeed.vue
autonomous: true
requirements:
  - SOCIAL-04
  - SOCIAL-05
  - SOCIAL-06

must_haves:
  truths:
    - "frontend/web/src/api/client.ts exports a commentApi object with exactly four methods: getAnimeComments, createComment, updateComment, deleteComment."
    - "All 24 anime.ugc.* keys from 01-UI-SPEC.md exist in en.json, ja.json, and ru.json. No key is missing in any of the three locale files."
    - "ActivityFeed.vue has an explicit branch for event.type === 'comment' that returns t('activity.comment.posted')."
    - "Each of en.json, ja.json, ru.json contains an activity.comment.posted key with a locale-appropriate string."
    - "bunx vue-tsc --noEmit completes with no errors."
  artifacts:
    - path: "frontend/web/src/api/client.ts"
      provides: "commentApi axios wrapper sibling to reviewApi"
      contains: "commentApi"
    - path: "frontend/web/src/locales/en.json"
      provides: "EN translations for all 24 anime.ugc.* keys + activity.comment.posted"
      contains: "anime.ugc.commentsTab"
    - path: "frontend/web/src/locales/ja.json"
      provides: "JA translations using polite (です/ます) form"
      contains: "anime.ugc.commentsTab"
    - path: "frontend/web/src/locales/ru.json"
      provides: "RU translations using formal Вы form"
      contains: "anime.ugc.commentsTab"
    - path: "frontend/web/src/components/ActivityFeed.vue"
      provides: "Three-line branch in actionText() that returns t('activity.comment.posted') for event.type === 'comment'"
      contains: "event.type === 'comment'"
  key_links:
    - from: "frontend/web/src/api/client.ts (commentApi)"
      to: "gateway routes added in plan 04"
      via: "axios apiClient with baseURL /api; methods hit /anime/:id/comments[/:cid]"
      pattern: "anime/.+/comments"
    - from: "frontend/web/src/components/ActivityFeed.vue"
      to: "frontend/web/src/locales/{en,ja,ru}.json"
      via: "$t('activity.comment.posted')"
      pattern: "activity.comment.posted"
---

<objective>
Land the frontend plumbing that plan 06 will consume: an axios `commentApi` wrapper mirroring `reviewApi`, 24 anime.ugc.* locale keys across en/ja/ru, and a 3-line `comment` branch in `ActivityFeed.vue` so existing review/status events keep working while new comment events render with a locale-correct label.

Purpose: SOCIAL-06 (UI locale strings) + SOCIAL-05 (activity feed renders comment events). Plan 06 implements the Anime.vue tab strip — it depends on commentApi existing and the locale keys being resolvable. Splitting this plan from plan 06 keeps each plan within the 2-3-task budget and lets Plan 06 focus entirely on Anime.vue editing.

Output: zero behavior change to user-visible UI until plan 06 lands; vue-tsc + Playwright tests must pass; ActivityFeed correctly handles type='comment' events.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/social/phases/01-social-reviews-comments/01-UI-SPEC.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-CONTEXT.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-03-SUMMARY.md
@frontend/web/src/api/client.ts
@frontend/web/src/components/ActivityFeed.vue
@frontend/web/src/locales/en.json
@frontend/web/src/locales/ja.json
@frontend/web/src/locales/ru.json

<interfaces>
Existing API client surface (from frontend/web/src/api/client.ts):
- import axios.create() => apiClient with baseURL VITE_API_URL or '/api'
- reviewApi (line 338-357): getAnimeReviews(animeId), getAnimeRating(animeId), getMyReview(animeId), createReview(animeId, payload), deleteReview(animeId), getBatchAnimeRatings(animeIds)
- Pattern: every method returns `apiClient.get(...)` / `apiClient.post(...)` etc. as a thenable Promise<AxiosResponse>

Backend route map (from plan 04):
- GET    /api/anime/:id/comments?cursor=&limit=  -> { comments: Comment[], next_cursor?: string, has_more: boolean }
- POST   /api/anime/:id/comments                 body: { body: string } -> 201 Comment
- PATCH  /api/anime/:id/comments/:commentId      body: { body: string } -> 200 Comment
- DELETE /api/anime/:id/comments/:commentId      -> 204 No Content

From frontend/web/src/components/ActivityFeed.vue:
- actionText() function around line 142-159
- Existing branches for event.type === 'review' / 'status' / 'score'
- Fallback returns the empty string or a placeholder for unknown types
- t() is vue-i18n's translate function imported via useI18n() at top of script setup

All 24 locale keys (from 01-UI-SPEC.md Copywriting Contract, table at lines 207-231):
- anime.ugc.reviewsTab, commentsTab, commentPlaceholder, postComment, posting, editComment, deleteComment, deleteCommentConfirm, editPlaceholder, saveEdit, cancelEdit, loadMore, loading, loginToComment, emptyComments, charCount, rateLimitError, bodyEmpty, bodyTooLong, editFailed, deleteFailed, loadFailed, loadMoreFailed, commentsCount

Plus one new activity key: activity.comment.posted
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 5.1: Add commentApi to frontend/web/src/api/client.ts</name>
  <files>frontend/web/src/api/client.ts</files>
  <read_first>
    - frontend/web/src/api/client.ts (full file — focus on reviewApi at lines 338-357 for the pattern)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Code Example 4, lines 829-849)
  </read_first>
  <action>
    Add a `commentApi` export immediately after the `reviewApi` block (currently ending around line 357). The shape mirrors reviewApi exactly. Four methods:

    - getAnimeComments(animeId: string, params?: { cursor?: string; limit?: number }) => apiClient.get(`/anime/${animeId}/comments`, { params })
    - createComment(animeId: string, body: string) => apiClient.post(`/anime/${animeId}/comments`, { body })
    - updateComment(animeId: string, commentId: string, body: string) => apiClient.patch(`/anime/${animeId}/comments/${commentId}`, { body })
    - deleteComment(animeId: string, commentId: string) => apiClient.delete(`/anime/${animeId}/comments/${commentId}`)

    Use TypeScript types — explicit `string` typing on parameters. Use template literals for URL paths (matches the existing reviewApi convention). Do NOT introduce a new TypeScript interface for Comment in this plan (plan 06 will define it locally in Anime.vue alongside the Review interface; client.ts already uses untyped axios responses for the other *Api blocks).

    Place the export between reviewApi and the next existing export (likely adminApi or similar — confirm by reading the file). Keep a single blank line above and below the block per file convention.
  </action>
  <verify>
    <automated>cd frontend/web && bunx tsc --noEmit && grep -c 'commentApi' src/api/client.ts && grep -c "anime/\${animeId}/comments" src/api/client.ts</automated>
  </verify>
  <acceptance_criteria>
    - `cd frontend/web && bunx tsc --noEmit` exits 0.
    - `grep -c 'export const commentApi' frontend/web/src/api/client.ts` outputs `1`.
    - `grep -c 'getAnimeComments' frontend/web/src/api/client.ts` outputs ≥ 1.
    - `grep -c 'createComment' frontend/web/src/api/client.ts` outputs ≥ 1.
    - `grep -c 'updateComment' frontend/web/src/api/client.ts` outputs ≥ 1.
    - `grep -c 'deleteComment' frontend/web/src/api/client.ts` outputs ≥ 1.
    - `grep -E 'anime/\$\{animeId\}/comments/\$\{commentId\}' frontend/web/src/api/client.ts | wc -l` outputs `2` (updateComment + deleteComment use the commentId path).
  </acceptance_criteria>
  <done>commentApi is exported and type-checks cleanly. Plan 06 imports it from `@/api/client`.</done>
</task>

<task type="auto">
  <name>Task 5.2: Add 24 anime.ugc.* keys + activity.comment.posted to en.json, ja.json, ru.json</name>
  <files>frontend/web/src/locales/en.json, frontend/web/src/locales/ja.json, frontend/web/src/locales/ru.json</files>
  <read_first>
    - frontend/web/src/locales/en.json (find the existing `anime` namespace, ~line 40; existing keys like anime.noReviews, anime.loginToReview — they set the tone for the new keys)
    - frontend/web/src/locales/ja.json (same — for JA tone reference)
    - frontend/web/src/locales/ru.json (same — for RU tone, use formal Вы)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-UI-SPEC.md (Copywriting Contract section, lines 202-241 — exact EN copy + tone rules for JA/RU)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Code Example 5 — activity.comment.posted "commented on")
  </read_first>
  <action>
    Add the following nested JSON structure inside the existing `anime` namespace in all three locale files, alongside existing keys like noReviews / loginToReview:

    ```
    "ugc": {
      "reviewsTab": ...,
      "commentsTab": ...,
      "commentPlaceholder": ...,
      "postComment": ...,
      "posting": ...,
      "editComment": ...,
      "deleteComment": ...,
      "deleteCommentConfirm": ...,
      "editPlaceholder": ...,
      "saveEdit": ...,
      "cancelEdit": ...,
      "loadMore": ...,
      "loading": ...,
      "loginToComment": ...,
      "emptyComments": ...,
      "charCount": "{count}/2000",
      "rateLimitError": ...,
      "bodyEmpty": ...,
      "bodyTooLong": ...,
      "editFailed": ...,
      "deleteFailed": ...,
      "loadFailed": ...,
      "loadMoreFailed": ...,
      "commentsCount": "{count, plural, one {# comment} other {# comments}}"
    }
    ```

    For EN: use the exact strings from the UI-SPEC Copywriting Contract table (column EN). Examples: `reviewsTab: "Reviews"`, `commentsTab: "Comments"`, `postComment: "Post comment"`, `saveEdit: "Save edit"`, `cancelEdit: "Cancel edit"`, `emptyComments: "No comments yet. Start the conversation."`, `loginToComment: "Sign in to join the conversation"`, `deleteCommentConfirm: "Delete this comment? This cannot be undone."`. Copy the strings from the UI-SPEC verbatim — they were already checker-approved.

    For JA: where the UI-SPEC table has an explicit JA value (postComment "コメントを投稿", saveEdit "編集を保存", cancelEdit "編集をキャンセル"), use it verbatim. For everything else (marked "(translate)"), translate from EN using polite (です/ます) form. Match the energy of existing anime.* keys (e.g. anime.noReviews JA equivalent). Examples: reviewsTab → "レビュー", commentsTab → "コメント", commentPlaceholder → "コメントを追加…", emptyComments → "まだコメントがありません。最初にコメントしてみよう。" etc.

    For RU: where the UI-SPEC table has an explicit RU value (postComment "Опубликовать комментарий", saveEdit "Сохранить правку", cancelEdit "Отменить правку"), use it verbatim. For everything else, translate using formal Вы form. Match existing keys like anime.loginToReview "Войдите, чтобы оставить отзыв". Examples: reviewsTab → "Отзывы", commentsTab → "Комментарии", commentPlaceholder → "Добавить комментарий…", emptyComments → "Пока нет комментариев. Начните обсуждение.", loginToComment → "Войдите, чтобы присоединиться к обсуждению." etc.

    Leave `charCount` and `commentsCount` literal — they use ICU placeholders that don't need translation per the UI-SPEC tone rules.

    Additionally, find the existing `activity` namespace (~line 427 in en.json) and add inside it a `comment` sub-object:
    ```
    "comment": { "posted": "commented on" }
    ```
    For JA: `"comment": { "posted": "がコメントしました" }`.
    For RU: `"comment": { "posted": "оставил(а) комментарий к" }`.

    Do NOT touch any existing keys. JSON syntactic validity is mandatory — run `jq . < file.json > /dev/null` after each edit.
  </action>
  <verify>
    <automated>jq . frontend/web/src/locales/en.json > /dev/null && jq . frontend/web/src/locales/ja.json > /dev/null && jq . frontend/web/src/locales/ru.json > /dev/null && for f in en ja ru; do c=$(jq '.anime.ugc | keys | length' frontend/web/src/locales/$f.json); echo "$f: $c keys"; test "$c" -ge 24; done && for f in en ja ru; do jq -e '.activity.comment.posted' frontend/web/src/locales/$f.json > /dev/null; done</automated>
  </verify>
  <acceptance_criteria>
    - `jq . frontend/web/src/locales/{en,ja,ru}.json > /dev/null` exits 0 for all three files (valid JSON).
    - `jq '.anime.ugc | keys | length' frontend/web/src/locales/en.json` outputs `24`. Same for ja.json and ru.json.
    - Each of the 24 keys is non-empty: `for k in reviewsTab commentsTab commentPlaceholder postComment posting editComment deleteComment deleteCommentConfirm editPlaceholder saveEdit cancelEdit loadMore loading loginToComment emptyComments charCount rateLimitError bodyEmpty bodyTooLong editFailed deleteFailed loadFailed loadMoreFailed commentsCount; do for f in en ja ru; do v=$(jq -r ".anime.ugc.$k // empty" frontend/web/src/locales/$f.json); test -n "$v" || echo "MISSING: $f.$k"; done; done` outputs nothing.
    - `jq -r '.activity.comment.posted' frontend/web/src/locales/en.json` outputs `commented on` (or equivalent non-empty string per locale).
    - EN postComment is exactly "Post comment". RU postComment is exactly "Опубликовать комментарий". JA postComment is exactly "コメントを投稿". EN saveEdit is exactly "Save edit". EN cancelEdit is exactly "Cancel edit". Verify via `jq -r '.anime.ugc.postComment' frontend/web/src/locales/en.json` etc.
    - `cd frontend/web && bunx tsc --noEmit` exits 0 (locales aren't TS-checked but ensure no upstream regression).
  </acceptance_criteria>
  <done>All 72 locale entries (24 keys × 3 locales) + 3 activity.comment.posted entries are present, syntactically valid JSON, and ready for plan 06 to consume.</done>
</task>

<task type="auto">
  <name>Task 5.3: Add comment branch to ActivityFeed.vue actionText() function</name>
  <files>frontend/web/src/components/ActivityFeed.vue</files>
  <read_first>
    - frontend/web/src/components/ActivityFeed.vue (full file — focus on actionText() at lines 142-159 and the TS event interface at lines 94-107)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Pitfall 6 + Code Example 5)
    - frontend/web/src/locales/en.json (confirm activity.comment.posted exists per task 5.2)
  </read_first>
  <action>
    Edit `frontend/web/src/components/ActivityFeed.vue`. Locate the `actionText()` function (likely lines 142-159 — `function actionText(event: ActivityEvent): string { ... }` or `const actionText = (event) => { ... }`).

    Add a new branch BEFORE the existing review/status branches (so the comment case short-circuits cleanly). The branch is a single `if`:

    ```
    if (event.type === 'comment') {
      return t('activity.comment.posted')
    }
    ```

    Do NOT touch the existing review/status/score branches — they keep working unchanged for non-comment events.

    If the TS `ActivityEvent` interface (lines 94-107) declares `type` as a string-literal union, ensure 'comment' is one of the allowed values; if it's just `string`, no change needed.

    Run the dev server briefly (or rely on `bunx vue-tsc --noEmit`) to confirm no missing-key warning would fire when a comment event is rendered. Visual verification of the rendered output is deferred to a manual smoke after plan 06 ships.
  </action>
  <verify>
    <automated>cd frontend/web && bunx vue-tsc --noEmit && grep -c "event.type === 'comment'" src/components/ActivityFeed.vue && grep -c "activity.comment.posted" src/components/ActivityFeed.vue</automated>
  </verify>
  <acceptance_criteria>
    - `cd frontend/web && bunx vue-tsc --noEmit` exits 0.
    - `grep -c "event.type === 'comment'" frontend/web/src/components/ActivityFeed.vue` outputs `1`.
    - `grep -c "activity.comment.posted" frontend/web/src/components/ActivityFeed.vue` outputs `1`.
    - The new `if` branch sits inside the `actionText` function — verify by piping the function body through awk: `awk '/function actionText|const actionText/,/^}/' frontend/web/src/components/ActivityFeed.vue | grep -c "event.type === 'comment'"` outputs `1`.
    - `cd frontend/web && bunx playwright test --list e2e/comments.spec.ts 2>&1 | grep -c 'deep-link'` still outputs ≥ 1 (no regression in the e2e suite manifest).
  </acceptance_criteria>
  <done>ActivityFeed correctly renders comment events with the new locale key. No [intlify] warnings on dev console when a comment event lands in the feed.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Frontend → gateway | Existing axios interceptors handle auth + token refresh; commentApi inherits them automatically. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-1-V13 | API & Web Service | commentApi axios wrapper | mitigate | Reuses the existing apiClient — same baseURL, interceptors, auth handling. No new auth surface. |
| Locale-drift / missing keys | Information disclosure (UX failure) | i18n missing-key warnings | mitigate | Acceptance criteria assert all 24 keys exist in all 3 locales via jq before merge. Manual dev-console grep for [intlify] in plan 06 checkpoint catches any drift introduced by Anime.vue references. |
| Stored XSS via ActivityFeed.vue | Tampering | t() interpolation | accept | Vue's `{{ }}` and t() return interpolation auto-escape; comment body is rendered separately in Anime.vue (plan 06) with `whitespace-pre-wrap` and `{{ }}` — no v-html anywhere. |
</threat_model>

<verification>
- `cd frontend/web && bunx tsc --noEmit` exits 0
- `cd frontend/web && bunx vue-tsc --noEmit` exits 0
- `jq . frontend/web/src/locales/{en,ja,ru}.json` exits 0 for all three
- `jq '.anime.ugc | keys | length' frontend/web/src/locales/en.json` outputs 24
- ActivityFeed.vue contains the comment branch via grep
- Frontend dev build (`bun run build` if quick) succeeds
</verification>

<success_criteria>
Frontend plumbing for SOCIAL-04/05/06 is in place. Plan 06 imports `commentApi` from `@/api/client`, references `$t('anime.ugc.*')` keys, and ActivityFeed renders comment events without missing-key warnings.
</success_criteria>

<output>
After completion, create `.planning/workstreams/social/phases/01-social-reviews-comments/01-05-SUMMARY.md` documenting: the commentApi public surface, sample EN/JA/RU strings for the most-used keys (commentsTab, postComment, deleteCommentConfirm), and the exact line in ActivityFeed.vue where the comment branch was inserted.
</output>
