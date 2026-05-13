---
phase: 1
workstream: social
plan: 6
type: execute
wave: 4
depends_on: [4, 5]
files_modified:
  - frontend/web/src/views/Anime.vue
  - frontend/web/e2e/comments.spec.ts
autonomous: false
requirements:
  - SOCIAL-06

must_haves:
  truths:
    - "The current Reviews section in Anime.vue (lines 590-715) is wrapped in a <Tabs> strip with two tabs: Reviews ({reviews.length}) and Comments ({commentsTotal}). The existing review content is preserved verbatim inside the Reviews tab slot."
    - "Default tab is Reviews. /anime/<id> with no ?ugc= query param shows Reviews on first paint."
    - "/anime/<id>?ugc=comments shows Comments tab on first paint (synchronous initial state, no flicker)."
    - "Clicking a tab calls router.replace (NOT router.push) and updates the URL ?ugc= value while preserving other query params."
    - "Browser back button leaves the page entirely — does NOT cycle through tabs (consequence of router.replace)."
    - "Comments tab logged-in: glass-card with rows=3 textarea, char counter <count>/2000 in text-white/40 (or text-pink-400 when >2000), Post comment button (cyan-500 bg, text-black, font-semibold). Disabled when trimmed body empty OR rune count > 2000. Posting empty/whitespace body shows inline anime.ugc.bodyEmpty. Posting too-long shows anime.ugc.bodyTooLong. Posting on 429 shows anime.ugc.rateLimitError. Successful post clears the textarea and refetches first page."
    - "Comments tab logged-out: glass-card with text anime.ugc.loginToComment + a router-link to /auth styled like the existing review login button."
    - "Comment list: newest first, 50/page, paginated via Load more button (anime.ugc.loadMore) shown only when has_more === true. Each comment card glass-card p-4 space-y-2: avatar (cyan-500/20 tint, 2-char username slice fallback ??), router-link to /user/:user_id with author username, formatDate(created_at) relative timestamp, whitespace-pre-wrap body. Own-comment cards show edit pencil + delete trash icons. Admin sees trash on every comment but not pencil."
    - "Edit mode: clicking pencil swaps body <p> for an inline textarea + Save edit (cyan-500 bg, px-4) + Cancel edit (pink-500/20 wash, px-4). Save calls PATCH; Cancel reverts. On PATCH error, anime.ugc.editFailed appears under the textarea and the textarea stays open."
    - "Delete: clicking trash triggers window.confirm(t('anime.ugc.deleteCommentConfirm')); on confirm, optimistic remove + DELETE call; on error, restore card + anime.ugc.deleteFailed toast."
    - "All four Playwright tests (deep-link, URL persists, anon login prompt, logged-in CRUD) PASS — converted from SKIP."
  artifacts:
    - path: "frontend/web/src/views/Anime.vue"
      provides: "Reviews section wrapped in Tabs; Comments tab content (form + list + edit-mode + load-more + login-prompt + empty-state); reactive ugcTab ref synced bidirectionally with route.query.ugc; commentApi calls wired in"
      contains: "anime.ugc.commentsTab"
    - path: "frontend/web/e2e/comments.spec.ts"
      provides: "Four passing Playwright tests replacing the Wave-0 SKIP stubs"
      contains: "await page.goto"
  key_links:
    - from: "Anime.vue (Comments tab)"
      to: "commentApi from @/api/client"
      via: "import { commentApi } from '@/api/client' and use in fetchComments / postComment / editComment / deleteComment"
      pattern: "commentApi\\.(getAnimeComments|createComment|updateComment|deleteComment)"
    - from: "Anime.vue (ugcTab ref)"
      to: "vue-router route.query.ugc"
      via: "two watchers — route.query.ugc → ugcTab.value (back/forward + deep links); ugcTab → router.replace (click handlers)"
      pattern: "router\\.replace.*ugc"
    - from: "Anime.vue (Tabs component)"
      to: "components/ui/Tabs.vue"
      via: "<Tabs v-model='ugcTab' :tabs='...' variant='underline'>"
      pattern: "variant=\"underline\""
---

<objective>
The user-visible deliverable. Wrap the existing Reviews section in Anime.vue with a two-tab strip, build the full Comments UI per the UI-SPEC, and convert all four Playwright e2e stubs from SKIP to PASS. After this plan ships, a real user opening `/anime/<id>?ugc=comments` can post, edit, and delete a comment.

Purpose: SOCIAL-06. This is the only plan touching Anime.vue — kept separate from plan 05 (API + locales) so each plan stays within budget and the Anime.vue diff is reviewable.

Output: the Reviews section visually unchanged when Reviews tab is active; new Comments tab with full CRUD UI behind it; URL-persisted state; four green Playwright tests.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/social/phases/01-social-reviews-comments/01-UI-SPEC.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-CONTEXT.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-05-SUMMARY.md
@frontend/web/src/views/Anime.vue
@frontend/web/src/components/ui/Tabs.vue
@frontend/web/src/api/client.ts
@frontend/web/src/locales/en.json
@frontend/web/src/styles/main.css
@frontend/web/e2e/comments.spec.ts
@frontend/web/e2e/anime.spec.ts
@CLAUDE.md

<interfaces>
From frontend/web/src/api/client.ts (added in plan 05):
- commentApi.getAnimeComments(animeId, { cursor?, limit? }) -> Promise<AxiosResponse<{ comments: Comment[], next_cursor?: string, has_more: boolean }>>
- commentApi.createComment(animeId, body) -> Promise<AxiosResponse<Comment>>
- commentApi.updateComment(animeId, commentId, body) -> Promise<AxiosResponse<Comment>>
- commentApi.deleteComment(animeId, commentId) -> Promise<AxiosResponse<void>>

From frontend/web/src/components/ui/Tabs.vue (existing — DO NOT modify):
- Props: tabs: Array<{ value: string; label: string; count?: number }>, modelValue: string, variant: 'default' | 'pills' | 'underline'
- Emits: update:modelValue (use v-model)
- Slot: per-tab named slot keyed by tab.value (slot name = tab value)

From frontend/web/src/views/Anime.vue (existing, ~1500 lines):
- useRoute / useRouter already imported (lines 832-833)
- reviewApi, animeApi, userApi already imported (line 791)
- formatDate() helper available (~line 1068) — produces relative timestamps
- Review TS interface (lines 816-824)
- authStore via useAuthStore (search for the import)
- Review section to wrap: lines 590-715

Reference for Vue+router pattern (from 01-RESEARCH.md Pattern 6):
- Two watchers — route.query.ugc → ugcTab (deep link + back/forward) and ugcTab → router.replace (click)
- const ALLOWED = ['reviews', 'comments'] as const; default to 'reviews' for any unknown value
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 6.1: Wrap Reviews section in Tabs strip; add Comments tab UI (form + list + edit-mode + load-more + login-prompt + empty-state)</name>
  <files>frontend/web/src/views/Anime.vue</files>
  <read_first>
    - frontend/web/src/views/Anime.vue (lines 580-720 — the current Reviews section + immediate neighbors)
    - frontend/web/src/views/Anime.vue (lines 760-870 — the script setup imports, useRoute/useRouter, Review interface)
    - frontend/web/src/views/Anime.vue (lines 1050-1100 — formatDate helper, used for timestamp rendering)
    - frontend/web/src/components/ui/Tabs.vue (full file — props, slot naming, count badge style)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-UI-SPEC.md (Component Inventory + Interaction Contract — the source of truth for visuals)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Pattern 6 — Vue Tabs URL persistence)
  </read_first>
  <action>
    Edit `frontend/web/src/views/Anime.vue` in three phases:

    Phase A — Script imports + reactive state:
    1. Add `Tabs` to the imports: `import Tabs from '@/components/ui/Tabs.vue'`.
    2. Add `commentApi` to the existing `@/api/client` import line.
    3. Declare a TS interface `Comment` near the Review interface (line 816-824 area):
       ```
       interface Comment { id: string; user_id: string; anime_id: string; username: string; body: string; created_at: string; updated_at: string }
       ```
    4. Declare reactive state (inside the existing script setup, near other refs):
       - `const ALLOWED = ['reviews', 'comments'] as const`
       - `type UgcTab = typeof ALLOWED[number]`
       - `const initialTab = (route.query.ugc as UgcTab) ?? 'reviews'`
       - `const ugcTab = ref<UgcTab>(ALLOWED.includes(initialTab) ? initialTab : 'reviews')`
       - `const comments = ref<Comment[]>([])`
       - `const commentsTotal = ref(0)` — though the SPEC says the count badge displays total, the cursor pagination doesn't return total. Resolution: commentsTotal = `comments.value.length + (hasMore ? '+' : '')`. For simplicity, the tab badge shows just `comments.value.length` and on initial load issues a separate call OR derives from has_more. CONTEXT.md commentsTotal in tab badge — interpret as "what's loaded so far"; refresh on tab activation.
       - `const commentsHasMore = ref(false)`, `const commentsNextCursor = ref('')`, `const commentsLoading = ref(false)`, `const commentsError = ref('')`
       - `const newCommentBody = ref('')`, `const posting = ref(false)`, `const postError = ref('')`
       - `const editingCommentId = ref<string | null>(null)`, `const editingBody = ref('')`, `const editError = ref('')`
    5. Add two watchers (per RESEARCH.md Pattern 6):
       - `watch(() => route.query.ugc, (v) => { const val = (v as UgcTab) ?? 'reviews'; if (ALLOWED.includes(val) && val !== ugcTab.value) ugcTab.value = val })`
       - `watch(ugcTab, (v) => { if (route.query.ugc !== v) { router.replace({ query: { ...route.query, ugc: v } }) }; if (v === 'comments' && comments.value.length === 0 && !commentsLoading.value) fetchComments() })`
    6. Implement async helpers (declared as const arrow fns inside script setup):
       - `fetchComments()`: set commentsLoading; call `commentApi.getAnimeComments(anime.value.id, { limit: 50 })`; populate comments / commentsNextCursor / commentsHasMore. On error: set commentsError = t('anime.ugc.loadFailed').
       - `loadMoreComments()`: same but pass `cursor: commentsNextCursor.value`; APPEND results to comments (don't replace); update cursor + has_more.
       - `postComment()`: validate trimmed body non-empty + <= 2000 runes (use `[...body].length` for rune count in JS). Set posting=true, postError=''. Call `commentApi.createComment`. On success: prepend new comment to comments.value, clear newCommentBody, decrement loading. On 429: postError = t('anime.ugc.rateLimitError'). On 400: postError = bodyEmpty or bodyTooLong (parse error body).
       - `startEditComment(c)`: set editingCommentId = c.id; editingBody = c.body.
       - `saveEditComment()`: validate; call `commentApi.updateComment`. On success: replace comment in array; clear editing state. On error: editError = t('anime.ugc.editFailed').
       - `cancelEditComment()`: clear editingCommentId + editingBody + editError.
       - `deleteCommentItem(c)`: window.confirm(t('anime.ugc.deleteCommentConfirm')) gate; optimistic remove; call `commentApi.deleteComment`; on error: re-insert at original index + show inline error.

    Phase B — Template wrap:
    1. Find the existing `<section ... class="mt-8">` that contains the Reviews block (around line 591). DO NOT delete the existing `<h2>` header above the tabs strip — leave it in place per UI-SPEC (the h2 sits ABOVE the tab strip, lines 70 of UI-SPEC).
    2. Wrap the existing review-section CONTENT (the write-form, login prompt, list — lines 595-715) in `<Tabs v-model="ugcTab" :tabs="[{ value: 'reviews', label: $t('anime.ugc.reviewsTab'), count: reviews.length }, { value: 'comments', label: $t('anime.ugc.commentsTab'), count: comments.length }]" variant="underline">`.
    3. Inside `<Tabs>`, wrap the existing reviews content in `<template #reviews> ... </template>`.
    4. Add a new `<template #comments> ... </template>` slot containing the Comments UI per UI-SPEC §Visuals + §Interaction Contract. Required elements:
       - Logged-in: glass-card with `<textarea v-model="newCommentBody" rows="3" :placeholder="$t('anime.ugc.commentPlaceholder')" class="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-cyan-500 transition-colors resize-none">`. Char counter `<div class="text-sm text-right" :class="[...].length > 2000 ? 'text-pink-400' : 'text-white/40']">{{ $t('anime.ugc.charCount', { count: [...newCommentBody].length }) }}</div>`. Post button `<button @click="postComment" :disabled="..." class="bg-cyan-500 hover:bg-cyan-400 text-black font-semibold rounded-lg px-6 py-2">{{ posting ? $t('anime.ugc.posting') : $t('anime.ugc.postComment') }}</button>`. Inline `<p v-if="postError" class="text-pink-400 text-sm mt-2">{{ postError }}</p>`.
       - Logged-out: glass-card with `<p class="text-white/60 mb-3">{{ $t('anime.ugc.loginToComment') }}</p>` + `<router-link to="/auth" class="...">{{ $t('nav.login') }}</router-link>` styled identical to the existing review login button at L671-676.
       - Comment list: `<div v-if="commentsLoading && comments.length === 0">` skeleton (reuse the existing Skeleton.vue if imported). `<div v-else-if="comments.length === 0" class="glass-card p-8 text-center"><p class="text-white/50">{{ $t('anime.ugc.emptyComments') }}</p></div>` for empty state. Otherwise iterate `<article v-for="c in comments" :key="c.id" class="glass-card p-4 space-y-2">` containing avatar + author router-link to /user/{c.user_id} + formatDate(c.created_at) + body (`<p class="whitespace-pre-wrap text-white/70">{{ c.body }}</p>` when NOT editing; `<textarea v-model="editingBody">` when editing) + own-comment-only edit/delete icon buttons.
       - Edit mode controls: Save edit (`px-4 py-2`, cyan-500 bg) + Cancel edit (`px-4 py-2`, pink-500/20 wash). Use the exact px-4 vs px-6 contrast from UI-SPEC §Spacing.
       - Load more: `<button v-if="commentsHasMore" @click="loadMoreComments" :disabled="commentsLoading" class="block mx-auto w-fit text-white/70 hover:text-white px-4 py-2 rounded-lg glass-card">{{ commentsLoading ? $t('anime.ugc.loading') : $t('anime.ugc.loadMore') }}</button>`.
       - Admin trash visibility: condition `v-if="c.user_id === authStore.user?.id || authStore.isAdmin"` for trash; `v-if="c.user_id === authStore.user?.id"` (NOT admin) for pencil. Per UI-SPEC: admins see trash on every comment, never the pencil.

    Phase C — Cleanup:
    1. Confirm reviews behavior is unchanged: the existing review fetch on mount (around line 1199) still runs; the reviews-tab content renders exactly as before.
    2. Confirm `posting`, `commentsLoading`, etc. are exposed in script setup (no template-only refs).
    3. Run `bunx vue-tsc --noEmit` to confirm no type errors.

    Visual constraints (DO NOT violate — checker-enforced):
    - Font weights: 400 + 600 only. Use `font-semibold` (600) for tab labels, author names, button text. No `font-medium` (500).
    - Spacing: only `{4, 8, 16, 24, 32}` px (Tailwind 1/2/3/4/5/6/8). NO custom paddings like p-3 (12px) or p-5 (20px).
    - Colors: cyan-500 for primary actions; pink-500/20 wash + pink-400 text for destructive; no green/amber/purple in this section (except amber stars already in Reviews tab — UNTOUCHED).
  </action>
  <verify>
    <automated>cd frontend/web && bunx vue-tsc --noEmit && grep -c 'commentApi' src/views/Anime.vue && grep -c "anime.ugc.commentsTab" src/views/Anime.vue && grep -c 'variant="underline"' src/views/Anime.vue && grep -c 'router.replace' src/views/Anime.vue</automated>
  </verify>
  <acceptance_criteria>
    - `cd frontend/web && bunx vue-tsc --noEmit` exits 0.
    - `grep -c 'commentApi' frontend/web/src/views/Anime.vue` outputs ≥ 4 (one per API method invocation: getAnimeComments, createComment, updateComment, deleteComment).
    - `grep -c 'anime.ugc.commentsTab' frontend/web/src/views/Anime.vue` outputs ≥ 1.
    - `grep -c '<Tabs' frontend/web/src/views/Anime.vue` outputs ≥ 1.
    - `grep -c 'variant="underline"' frontend/web/src/views/Anime.vue` outputs ≥ 1.
    - `grep -c 'router.replace' frontend/web/src/views/Anime.vue` outputs ≥ 1 (the ugcTab watcher).
    - `grep -c 'anime.ugc.deleteCommentConfirm' frontend/web/src/views/Anime.vue` outputs ≥ 1 (in the window.confirm call).
    - `grep -c 'whitespace-pre-wrap' frontend/web/src/views/Anime.vue` outputs ≥ 1 (comment body rendering — and the existing one for reviews if present).
    - `grep -c 'authStore.isAdmin' frontend/web/src/views/Anime.vue` outputs ≥ 1 (admin trash visibility).
    - No `router.push` was added for tab switches: `grep "router.push.*ugc" frontend/web/src/views/Anime.vue | wc -l` outputs `0`.
    - Build smoke: `cd frontend/web && bun run build 2>&1 | tail -20` shows no Vue compile errors. (Optional if build is slow; vue-tsc covers most issues.)
  </acceptance_criteria>
  <done>Anime.vue renders both tabs, URL-persists tab state, fully implements the Comments UI per UI-SPEC, and type-checks cleanly.</done>
</task>

<task type="auto">
  <name>Task 6.2: Convert four Playwright e2e tests from SKIP to PASS</name>
  <files>frontend/web/e2e/comments.spec.ts</files>
  <read_first>
    - frontend/web/e2e/comments.spec.ts (Wave 0 stubs)
    - frontend/web/e2e/anime.spec.ts (full file — navigation + selector patterns for Anime.vue)
    - frontend/web/e2e/profile.spec.ts (login flow with API key or password using ui_audit_bot)
    - frontend/web/playwright.config.ts (baseURL)
    - CLAUDE.md (UI Audit Test User — username `ui_audit_bot`, API key in docker/.env as UI_AUDIT_API_KEY, password `audit_bot_test_password_2026`)
  </read_first>
  <action>
    Replace each `test.skip(true, ...)` body with a real Playwright test. Remove the `test.skip(...)` line; keep the test name verbatim so the `-g` filters from 01-VALIDATION.md still match.

    Common setup (extract into a helper at top of the file):
    - `async function loginAsAuditBot(page: Page) { ... }` — POST /api/auth/login from inside the page via page.evaluate fetch (sets refresh cookie correctly). Then `await page.evaluate((data) => { localStorage.setItem('token', data.token); localStorage.setItem('user', JSON.stringify(data.user)); }, loginResp);`. Reuse the existing pattern from `e2e/profile.spec.ts` if there's already a helper there.
    - `const ANIME_ID` — pick a stable seeded anime from postgres; e.g. query `SELECT id FROM animes LIMIT 1` once and hard-code the UUID; alternatively, accept it via `process.env.E2E_ANIME_ID` with a default. Document the chosen ID at the top of the file in a comment.

    Test 1: `test('deep-link to ?ugc=comments mounts Comments tab on first paint', ...)`:
    - `await page.goto(\`/anime/${ANIME_ID}?ugc=comments\`)`.
    - Wait for the page to be visible: `await page.waitForSelector('[role="tab"]')`.
    - Assert the active tab has aria-selected='true' and contains the localized "Comments" label: `await expect(page.getByRole('tab', { name: /Comments/i, selected: true })).toBeVisible()`.
    - Assert the Reviews tab is NOT selected: `await expect(page.getByRole('tab', { name: /Reviews/i, selected: false })).toBeVisible()`.

    Test 2: `test('URL persists across tab clicks via router.replace', ...)`:
    - `await page.goto(\`/anime/${ANIME_ID}\`)` (no query param — Reviews tab default).
    - Assert URL has no ?ugc= initially.
    - Click the Comments tab: `await page.getByRole('tab', { name: /Comments/i }).click()`.
    - Assert URL now contains `ugc=comments`: `await expect(page).toHaveURL(/ugc=comments/)`.
    - Click the Reviews tab.
    - Assert URL now contains `ugc=reviews`.
    - History check: `await page.goBack()`. Assert the page navigates AWAY from this anime entirely (back to whatever was before — confirms router.replace, NOT router.push, was used).

    Test 3: `test('anon login prompt shown to logged-out users on Comments tab', ...)`:
    - Open a fresh context (no auth state): `const context = await browser.newContext()`.
    - Navigate to `/anime/${ANIME_ID}?ugc=comments`.
    - Assert NO textarea is visible inside the Comments panel: `await expect(page.locator('[role="tabpanel"] textarea')).toHaveCount(0)`.
    - Assert the login prompt is visible: `await expect(page.getByText(/Sign in to join the conversation|Войдите|サインイン/i)).toBeVisible()` (the regex covers all three locales).

    Test 4: `test('logged-in CRUD — post, edit, delete own comment', ...)`:
    - Log in as ui_audit_bot via the helper.
    - Navigate to `/anime/${ANIME_ID}?ugc=comments`.
    - Wait for the textarea to be visible.
    - Post: type `'e2e comment ' + Date.now()` into the textarea, click Post comment, wait for the comment card to appear in the list with that body text.
    - Edit: click the edit pencil on the new comment (`button[aria-label*="Edit" i]` inside the new comment card); type `' edited'` appended; click Save edit; wait for the body text to update.
    - Delete: stub the window.confirm with `page.on('dialog', d => d.accept())`; click the delete trash; wait for the card to disappear.
    - Cleanup: if any test step fails mid-way, the next run should not see stale data. Acceptable risk: tests may leave stray comments if interrupted; document this in a comment.

    All four tests must NOT depend on each other — each opens its own page / context.

    Test fixtures: pick a stable anime ID. If none is known, add a beforeAll that issues a Shikimori search or queries `/api/anime` for the first listed anime; persist it across the four tests.
  </action>
  <verify>
    <automated>cd frontend/web && bunx playwright test e2e/comments.spec.ts --reporter=list 2>&1 | tee /tmp/comments-test.log | grep -E '4 passed|passed \(' | head -5</automated>
  </verify>
  <acceptance_criteria>
    - `cd frontend/web && bunx playwright test e2e/comments.spec.ts` exits 0 with `4 passed` (or `4 tests passed`).
    - `grep -c 'test.skip' frontend/web/e2e/comments.spec.ts` outputs `0` (no skips remain).
    - `grep -c 'await page.goto' frontend/web/e2e/comments.spec.ts` outputs ≥ 4 (each test navigates at least once).
    - `grep -c 'router.replace\|toHaveURL.*ugc' frontend/web/e2e/comments.spec.ts` outputs ≥ 1 (URL persistence assertion).
    - `grep -c 'ui_audit_bot\|loginAsAuditBot' frontend/web/e2e/comments.spec.ts` outputs ≥ 1 (auth setup for the CRUD test).
    - The four test name keywords (`deep-link`, `URL persists`, `anon login prompt`, `logged-in CRUD`) each appear in exactly one test name.
  </acceptance_criteria>
  <done>Four green Playwright tests cover the four SOCIAL-06 acceptance behaviors. Cypress / manual smoke is no longer required for those specific paths.</done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Checkpoint 6.3: Deploy + visual + functional smoke against the live app</name>
  <what-built>
    Live two-tab strip on `/anime/<any-id>` with full Comments CRUD UI, URL persistence, locale support, admin moderation.
  </what-built>
  <action>Manual verification gate — implementer pauses execution and the human runs the steps in &lt;how-to-verify&gt; below, then types the resume signal. No automated work in this task.</action>
  <how-to-verify>
    1. Deploy the frontend: `make redeploy-web` (or `bun run build && make restart-web` if redeploy-web bundles to nginx).
    2. Open `/anime/<any-id>` in a browser logged in as `ui_audit_bot`. Confirm:
       - The Reviews section now shows TWO tabs at the top: `Reviews ({n})` and `Comments ({n})`.
       - Reviews tab is active by default; existing review content is visible and unchanged.
       - Tab badge styling: pill style, white/10 bg, rounded-full, text-xs.
       - Tab variant: underline (cyan-400 bottom border on active tab).
       - Click Comments tab: URL becomes `?ugc=comments`; tab content swaps to the new Comments UI.
       - Click Reviews tab: URL becomes `?ugc=reviews`; content swaps back to existing reviews.
       - Browser back button leaves the page (does NOT toggle between tabs).
    3. Functional Comments smoke (logged in as ui_audit_bot):
       - Post a comment "checkpoint 6.3 test" → appears at top of list. Tab badge increments.
       - Click pencil on that comment → textarea appears inline; type " edited" → click Save edit → body updates inline.
       - Click trash → window.confirm appears with "Delete this comment? This cannot be undone." → click OK → comment disappears.
    4. Functional anon smoke (private browser / new incognito window):
       - Open `/anime/<id>?ugc=comments`.
       - Confirm: comment list visible (incl. any seeded comments). Below it: NO textarea; INSTEAD a glass-card with "Sign in to join the conversation" + a Login button linking to /auth.
    5. Locale switch — switch i18n to JA, then to RU. Confirm:
       - Tab labels change ("Comments" → "コメント" / "Комментарии").
       - Empty state, login prompt, button labels all switch.
       - Browser dev console shows NO `[intlify] Not found` warnings (search console).
    6. Activity feed: navigate to your profile / activity feed page; confirm the comment posted in step 3 appears with the new locale label ("commented on …" / "がコメントしました …" / "оставил(а) комментарий к …").
    7. Admin view: log in as an admin user; navigate to `/anime/<id>?ugc=comments`. Confirm:
       - You see a trash icon on EVERY comment (not just your own).
       - You DO NOT see a pencil icon on others' comments (only on your own).
       - Clicking trash on someone else's comment + confirming actually soft-deletes it.
    8. Rate limit visual: post 11 comments quickly; on the 11th, confirm an inline "You've posted a lot recently. Try again in a few minutes." appears below the Post comment button (not a generic 500 error).
    9. Length validation: paste 2001 characters into the textarea; confirm char counter shows in pink, Post button disabled, on click an inline "Comment can't be longer than 2000 characters." appears.
    10. Final cleanup: delete the test comments created during this checkpoint.
  </how-to-verify>
  <resume-signal>Type "approved" if all 10 verification steps pass with no console errors and no missing-key warnings. If any step fails (visual, functional, locale), describe the symptom and the planner will produce a revision targeting the specific failure mode.</resume-signal>
</task>


<task type="checkpoint:human-action" gate="blocking">
  <name>Task 6.4: Invoke /animeenigma-after-update to lint, build, redeploy, update changelog, commit, and push</name>
  <files>frontend/web/public/changelog.json (modified by the skill; not edited directly here)</files>
  <action>After checkpoint 6.3 is approved, the executor MUST invoke the `/animeenigma-after-update` slash skill. The skill is interactive — do NOT attempt to script its steps or run them piecemeal. The skill itself handles: lint + build of touched code, `make redeploy-<service>` for each changed service (web + player), health checks, appending a user-facing changelog entry to `frontend/web/public/changelog.json` (dated 2026-05-13, mentioning Reviews + Comments tabs, written in the informative + enthusiastic tone with emojis per the project convention), and the commit + push (with the mandatory co-authors per project memory). Do not pre-stage files, do not pre-commit anything — the skill owns the commit. Pause here until the user types the resume signal confirming the skill has finished cleanly.</action>
  <how-to-verify>
    1. Run `/animeenigma-after-update` and let it complete end-to-end.
    2. After it finishes, confirm:
       - `frontend/web/public/changelog.json` contains a new top-level entry with `date: "2026-05-13"` (or the current date if different) whose summary text mentions both "Reviews" and "Comments" (or their localized equivalents). Verify: `jq '.[0]' frontend/web/public/changelog.json` shows the new entry.
       - The latest commit was authored with the three project co-authors. Verify: `git log -1 --format=%B` contains all three `Co-Authored-By:` lines (Claude Opus 4.6, 0neymik0, NANDIorg) exactly as in MEMORY.md.
       - `git log -1 --format=%s` shows a non-empty commit subject (the skill's auto-generated message).
       - The commit was pushed: `git status` shows the local branch is in sync with origin (no "ahead by" message).
       - `make health` reports all services healthy.
  </how-to-verify>
  <resume-signal>Type "shipped" once the skill has completed, the changelog entry is visible, the commit has the three co-authors, and the push succeeded. If the skill fails partway, describe the failure mode and the planner will produce a recovery revision.</resume-signal>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| User-submitted comment text → DOM rendering | Vue `{{ }}` interpolation auto-escapes; no v-html used. |
| Auth state in localStorage → axios Authorization header | Existing apiClient interceptor handles; commentApi inherits. |
| window.confirm → DELETE | Native browser dialog; no XSS risk. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| Stored XSS | Tampering | comment.body rendered in `<p class="whitespace-pre-wrap">{{ c.body }}</p>` | mitigate | Vue interpolation auto-escapes HTML; whitespace-pre-wrap is a CSS rule, not v-html. Plain text only per SPEC. |
| Tab-state tampering via URL | Tampering | route.query.ugc | mitigate | Server-side trust unaffected (just UI state); client filter `ALLOWED.includes(val)` rejects arbitrary strings; unknown values fall back to 'reviews'. |
| Optimistic delete leaves UI inconsistent on backend failure | Data integrity | deleteCommentItem catch block | mitigate | On DELETE error, re-insert the comment at original index + show inline error; UI converges with backend within one render cycle. |
| Admin sees pencil on other users' comments (UX bug) | Authorization (UI-only) | v-if condition | mitigate | Pencil visibility: `v-if="c.user_id === authStore.user?.id"` (own-only). Trash: `v-if="c.user_id === authStore.user?.id || authStore.isAdmin"` (own or admin). Checkpoint 6.3 step 7 verifies. |
| Locale-key drift causes [intlify] warnings | Information disclosure (UX) | template t() calls | mitigate | Plan 05 ensures all 24 keys exist in all 3 locales; checkpoint 6.3 step 5 grep-checks dev console for [intlify]. |
</threat_model>

<verification>
- `cd frontend/web && bunx vue-tsc --noEmit` exits 0
- `cd frontend/web && bunx playwright test e2e/comments.spec.ts` exits 0 — all 4 tests pass
- `cd frontend/web && bun run build` succeeds
- Live deploy: clicking through both tabs, posting/editing/deleting a comment works; URL updates; no console errors
- Three-locale check: switch locale to each of en/ja/ru and confirm no missing-key warnings
</verification>

<success_criteria>
SOCIAL-06 fully shipped. A real user can: open the anime detail page, switch between Reviews and Comments tabs (URL-persisted), post a comment, edit it, delete it, and see their action in the activity feed with a locale-correct label.
</success_criteria>

<output>
After completion, create `.planning/workstreams/social/phases/01-social-reviews-comments/01-06-SUMMARY.md` documenting: the Anime.vue line ranges that changed, the four Playwright test outcomes, screenshots (or links to them) of each tab in each locale, and the checkpoint 6.3 sign-off.
</output>
