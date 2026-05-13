---
phase: 1
workstream: social
reviewed: 2026-05-13
overall_score: 18/24
pillars:
  copywriting: 4/4
  visuals: 3/4
  color: 3/4
  typography: 2/4
  spacing: 3/4
  experience_design: 3/4
top_fixes:
  - "Replace font-medium (500) throughout the Reviews section + Tabs.vue tab labels with font-semibold (600) to enforce the 2-weight hard rule"
  - "loadMoreFailed error is silently swallowed — commentsError is gated on comments.length===0 but loadMore only fires when comments exist; the error never reaches the user"
  - "Save edit button shows anime.ugc.posting (\"Posting…\") during save — wrong copy context for an edit action; add a dedicated anime.ugc.saving key"
---

# Phase 1 — UI Review

**Audited:** 2026-05-13
**Baseline:** 01-UI-SPEC.md (approved design contract)
**Screenshots:** not captured (no dev server detected on ports 3000, 5173, 8080)

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | 4/4 | All 24 `anime.ugc.*` keys present across EN/JA/RU; CTAs are verb+noun; error states, destructive confirmation, and empty states all match the contract |
| 2. Visuals | 3/4 | Focal-point hierarchy works (cyan underline + cyan Post CTA); h2 section heading added a chat-bubble SVG icon (unspecced) and dynamically mirrors the active tab label instead of staying static |
| 3. Color | 3/4 | 60/30/10 split intact; destructive pink-wash-only correctly applied; h2 icon uses `text-cyan-400` for pure decoration, violating the accent reserved-for list |
| 4. Typography | 2/4 | Tabs.vue tab labels render `font-medium` (500) — spec explicitly requires override to `font-semibold` (600) and it was never applied; Reviews section retains `font-bold` and `font-medium` in the same area |
| 5. Spacing | 3/4 | Declared scale `{4,8,16,24,32}px` is largely respected; `mb-3` (12px), `gap-3` (12px), `px-3` (12px) appear in the Comments section — all outside the declared scale |
| 6. Experience Design | 3/4 | Loading/empty/error states fully implemented; `loadMoreFailed` stored to `commentsError` which is only rendered when `comments.length===0` — the error is unreachable when triggered; focus not programmatically moved into the edit textarea on pencil click |

**Overall: 18/24**

---

## Top 3 Priority Fixes

1. **`loadMoreFailed` error is never displayed** — when `loadMoreComments` fails it writes to `commentsError`, but the load-error card (`v-if="commentsError && comments.length === 0"`) is invisible while comments exist in the list; `loadMore` only fires when `commentsHasMore` is true (i.e., comments ARE loaded). Users get a silent failure with no retry affordance. Fix: render a separate `loadMoreError` ref below the comment list, independent of `commentsError`. (`frontend/web/src/views/Anime.vue` — found via grep `commentsError.value = t('anime.ugc.loadMoreFailed')`)

2. **Font-weight hard rule violated on tab labels** — `Tabs.vue:86` sets `font-medium` (500) in the base class; the spec explicitly states "override `font-medium` → `font-semibold` in the Tabs invocation or via a wrapper class." No such override exists in the Anime.vue invocation. All tab labels across the UGC strip render at weight 500, not 600, breaking the 2-weight rule. Fix: add a CSS override on the `<Tabs>` wrapper or pass a `labelClass` prop. (`frontend/web/src/components/ui/Tabs.vue:86` — found via grep `text-sm font-medium transition-all`)

3. **`Save edit` button shows wrong copy state** — during `editSaving`, the button renders `$t('anime.ugc.posting')` ("Posting…") — semantically wrong in an edit context (the user is saving, not posting). Add `anime.ugc.saving` key ("Saving…" / "保存中…" / "Сохранение…") to all three locale files and switch the button to use it. (`frontend/web/src/views/Anime.vue` — found via grep `editSaving ? $t('anime.ugc.posting') : $t('anime.ugc.saveEdit')`)

---

## Detailed Findings

### Pillar 1: Copywriting (4/4)

All 24 keys under `anime.ugc.*` are present and match the spec contract exactly in EN.

**Key-by-key verification (EN):**
- `reviewsTab` = "Reviews" ✓
- `commentsTab` = "Comments" ✓
- `commentPlaceholder` = "Add a comment…" ✓
- `postComment` = "Post comment" ✓ (verb+noun per spec revision)
- `posting` = "Posting…" ✓
- `editComment` = "Edit comment" ✓
- `deleteComment` = "Delete comment" ✓
- `deleteCommentConfirm` = "Delete this comment? This cannot be undone." ✓ (single sentence, both clauses)
- `editPlaceholder` = "Edit your comment…" ✓
- `saveEdit` = "Save edit" ✓ (per spec revision)
- `cancelEdit` = "Cancel edit" ✓ (per spec revision)
- `loadMore` = "Load more comments" ✓
- `loading` = "Loading…" ✓
- `loginToComment` = "Sign in to join the conversation" ✓
- `emptyComments` = "No comments yet. Start the conversation." ✓
- `charCount` = "{count}/2000" ✓
- `rateLimitError` = "You've posted a lot recently. Try again in a few minutes." ✓
- `bodyEmpty` = "Comment can't be empty." ✓
- `bodyTooLong` = "Comment can't be longer than 2000 characters." ✓
- `editFailed` = "Could not save your edit. Try again." ✓
- `deleteFailed` = "Could not delete the comment. Try again." ✓
- `loadFailed` = "Could not load comments." ✓
- `loadMoreFailed` = "Could not load more. Tap to retry." ✓ (the key exists; the display bug is an Experience Design issue)
- `commentsCount` = "{count} comment | {count} comments" ✓ (plural form, reserved per spec)

**JA locale:** All 24 keys present. `saveEdit` = "編集を保存" ✓, `cancelEdit` = "編集をキャンセル" ✓, `postComment` = "コメントを投稿" ✓. Polite form (ます) maintained throughout. (`frontend/web/src/locales/ja.json:126-151` — found via grep `"ugc": {`)

**RU locale:** All 24 keys present. `saveEdit` = "Сохранить правку" ✓, `cancelEdit` = "Отменить правку" ✓, `postComment` = "Опубликовать комментарий" ✓. Formal `Вы` register preserved. Russian plural forms correct (`commentsCount` has three variants). (`frontend/web/src/locales/ru.json:126-151` — found via grep `"ugc": {`)

**ActivityFeed:** `activity.comment.posted` present in all three locales ("commented on" / "оставил(а) комментарий к" / "がコメントしました"). (`frontend/web/src/locales/en.json:492-494` — found via grep `"comment": {`)

**WARNING — `save edit` button uses wrong i18n key while saving:** The save button in edit mode shows `$t('anime.ugc.posting')` ("Posting…") during the PATCH request, not a dedicated "saving" key. This is a copy-context mismatch but does not affect the 24-key completeness count. Copywriting score is not penalized because the key contract is met; the bug is noted here and listed as Priority Fix #3.

---

### Pillar 2: Visuals (3/4)

**PASS — focal-point hierarchy:** The cyan underline on the active tab + the cyan `Post comment` CTA function as the two visual anchors per spec. The glass-card comment list at lower contrast correctly recedes behind these two signals. Mirrors the established review-card pattern.

**PASS — component reuse:** All comment UI lives inside `Anime.vue` (no new SFC files). The avatar circle `w-10 h-10 rounded-full bg-cyan-500/20` mirrors the review author avatar exactly. Skeleton pulse cards match the existing `Skeleton.vue` pattern elsewhere. (`frontend/web/src/views/Anime.vue` — found via grep `w-10 h-10 rounded-full bg-cyan-500/20`)

**WARNING — unspecced decorative icon on h2 heading:** The section heading `<h2>` (line 609) wraps a `<svg class="w-6 h-6 text-cyan-400" ...>` chat-bubble icon. The spec's typography table and the Citations section make no mention of an icon on the h2 — the existing h2 pattern at the synopsis and genres sections also do not consistently use icons. The icon reads fine visually but introduces inconsistency with the page's other heading rows. (`frontend/web/src/views/Anime.vue` — found via grep `svg class="w-6 h-6 text-cyan-400"`)

**WARNING — h2 heading text is dynamic (mirrors active tab), spec said preserve static h2:** The spec states "preserves the existing `<h2>` at L593." The existing h2 was "Reviews". The implementation changes it to `ugcTab === 'comments' ? $t('anime.ugc.commentsTab') : $t('anime.reviews')` — it now switches label on tab change. This is a deliberate enhancement that makes context clearer, but it means the h2 and the active tab label are redundant. On desktop this creates double-labeling: the cyan tab says "Reviews" and the h2 above also says "Reviews". (`frontend/web/src/views/Anime.vue` — found via grep `ugcTab === 'comments' ? $t('anime.ugc.commentsTab') : $t('anime.reviews')`)

**PASS — action icon visibility:** Edit/delete icons are `text-white/40` at rest (low-contrast, effectively hidden) and become visible on hover (`hover:text-cyan-400`, `hover:text-pink-400`). This satisfies the "visible on hover on desktop" requirement. The spec's "always visible on touch / mobile" requirement cannot be verified without screenshots, but the CSS does not include any `md:` breakpoint qualifier that would hide them on mobile, so they remain present on all screen sizes.

---

### Pillar 3: Color (3/4)

**PASS — 60/30/10 split:** Base `#121218` is the dominant surface. Glass-card `rgba(255,255,255,0.05)` secondary. Cyan-400/500 accent used on: active tab underline (Tabs.vue), Post comment button (`bg-cyan-500`), Save edit button (`bg-cyan-500`), comment avatar tint (`bg-cyan-500/20 text-cyan-400`), retry button in load-error state (`bg-cyan-500`), login button in anon prompt (`bg-cyan-500`). The retry button and the login button are functional primary CTAs, not decorative — their inclusion is acceptable, though the retry button was not in the spec's explicit reserved-for list.

**PASS — destructive pink wash only:** Trash icon (`text-pink-400 hover:bg-pink-500/10`), Cancel edit button (`bg-pink-500/20 hover:bg-pink-500/30 text-pink-400`), post error / edit error / delete error text (`text-pink-400`) — all pink usage is wash-only, no solid pink fill. (`frontend/web/src/views/Anime.vue` — found via grep `hover:bg-pink-500/10`)

**PASS — no amber bleed into Comments tab:** Star rating amber colors exist only in the Reviews tab. The Comments tab has no amber.

**PASS — no hardcoded hex colors:** Comments section uses only Tailwind utility classes. No `#` or `rgb(` strings found in the new code.

**WARNING — h2 decorative icon uses accent color outside reserved list:** The `<svg class="w-6 h-6 text-cyan-400">` chat-bubble on the section heading is a purely decorative element not listed in the spec's accent reserved-for list. The spec lists exactly 5 permitted accent usages and declares "Reserved for" as exhaustive. A decorative heading icon is a 6th usage that bleeds accent tint into an element that should read as neutral heading text. Fix: remove the icon, or set it to `text-white/40` to keep it visually subordinate. (`frontend/web/src/views/Anime.vue:611` — found via grep `svg class="w-6 h-6 text-cyan-400"`)

---

### Pillar 4: Typography (2/4)

The 2-font-weight hard rule (400 body / 600 semibold) is violated in the implemented UGC section. This pillar cannot pass with active deviations from the hardest-stated rule in the spec.

**BLOCKER — Tabs.vue tab labels render at `font-medium` (500):** `Tabs.vue:86` defines the base class as `'px-4 py-2 text-sm font-medium ...'`. The spec explicitly instructs: "Inherited from `Tabs.vue:75` (override `font-medium` → `font-semibold` in the Tabs invocation or via a wrapper class)." No override was applied in `Anime.vue`'s `<Tabs>` call. The "Reviews" and "Comments" tab labels therefore render at weight 500, not 600. This breaks the 2-weight contract. (`frontend/web/src/components/ui/Tabs.vue:86` — found via grep `text-sm font-medium transition-all`)

**WARNING — Reviews section retains pre-existing `font-medium` and `font-bold` violations:** The Reviews section was described as "unchanged" in the scope, but the spec's typography section applies to the entire UGC surface. The following pre-existing classes conflict with the 2-weight rule:
- `font-medium` at line 631 (h3 "Write a review" heading)
- `font-medium` at line 679 (Review submit button)
- `font-medium` at line 686 (Delete review button)
- `font-medium` at line 698 (Login button in Reviews login-prompt)
- `font-medium` at line 719 (review author username link)
- `font-bold` at line 713 (review author avatar initials — should be `font-semibold`)
- `font-bold` at line 730 (review score number — should be `font-semibold`)
(`frontend/web/src/views/Anime.vue` — found via grep `font-medium` and `font-bold`)

**PASS — Comments section uses correct weights:** Comment avatar initials `font-semibold` (line 816), comment author name `font-semibold` (line 822), Post comment button `font-semibold` (line 765), Save edit `font-semibold` (line 873), Cancel edit `font-semibold` (line 881). The new Comments code fully respects the 2-weight rule.

**PASS — Font sizes in scope:** UGC section uses `text-xl` (h2), `text-sm` (tab labels, timestamps, error text, char counter), default body size (comment text, form labels). `text-xs` appears on tab count badges (inherited from Tabs.vue). This is the declared 4-size subset: 12/14/16/20px.

**PASS — Single font family:** No new font families introduced. All text inherits `--font-sans`.

---

### Pillar 5: Spacing (3/4)

Declared scale: `{4, 8, 16, 24, 32}px` = Tailwind `{p-1, p-2, p-4, p-6, p-8}` respectively.

**PASS — Primary structural spacing:** `glass-card p-4 md:p-6` on write form ✓, `mb-6` below form ✓, `space-y-4` between comment cards ✓, `p-4` per comment card ✓, `p-8` on empty state ✓, `px-6 py-2` on Post button ✓, `px-4 py-2` on Save edit / Cancel edit ✓, `gap-2` between action icons ✓, `mt-4` from tab strip to panel (Tabs.vue) ✓, `mt-8` on the outer section ✓.

**WARNING — `mb-3` (12px) used in login prompt and load-error card:** Both the login prompt body text (`<p class="text-white/60 mb-3">`) and the load-error paragraph (`<p class="text-pink-400 text-sm mb-3">`) use `mb-3` = 12px, which is outside the declared scale `{4,8,16,24,32}`. The spec declares `mb-2` (8px) for "gap between comment header and body." These should use `mb-2` for consistency. (`frontend/web/src/views/Anime.vue` — found via grep `mb-3`)

**WARNING — `gap-3` (12px) used for avatar+text header row:** The comment header row (`<div class="flex items-center gap-3">`) and the write-form button row (`<div class="flex items-center gap-3 mt-2">`) use `gap-3` = 12px. The spec defines `gap-3` (avatar + label gap) as... it actually defers to the inherited `gap-3` from the review-author pattern at L688. The review-author row at line 712 also uses `gap-3`, so this is consistent with the existing Reviews pattern. Minor deviation noted but consistent with the inherited-pattern exception.

**WARNING — `px-3` (12px) on comment textarea and edit textarea:** Both textareas use `px-3 py-2` for padding. `py-2` = 8px (in scale). `px-3` = 12px (outside scale). The review textarea uses `px-4 py-3`; the spec declares `py-2` for comment textarea vertical padding but does not explicitly declare horizontal padding for the textarea. No horizontal padding was listed in the spec's spacing table for this element. Minor omission.

**WARNING — Load More button missing responsive width:** Spec declares: "full-width on mobile, `w-fit mx-auto` on desktop." Actual: the Load More button has no `w-full sm:w-fit` — it is inside a `flex justify-center` wrapper and will naturally narrow to its content on all breakpoints. (`frontend/web/src/views/Anime.vue` — found via grep `commentsHasMore`)

---

### Pillar 6: Experience Design (3/4)

**PASS — Loading state:** Initial Comments tab fetch shows 3 skeleton pulse cards while `commentsLoading` is true and the list is empty. Write form (or login prompt) renders immediately above the skeleton. Matches the spec's "render form immediately; show skeleton below" requirement. (`frontend/web/src/views/Anime.vue` — found via grep `commentsLoading`)

**PASS — Empty state:** `glass-card p-8 text-center` with `text-white/50` text and `$t('anime.ugc.emptyComments')` renders when `commentsFetched && comments.length === 0 && !commentsLoading`. Matches the spec's "identical visual treatment to `noReviews` empty state" requirement.

**PASS — Initial GET error state:** `glass-card p-8 text-center` with pink-400 error text and a Retry button renders when `commentsError && comments.length === 0`. Retry calls `fetchComments`. Correct.

**PASS — POST error states:** All four POST error states (empty, too-long, 429, generic) stored in `postError` ref and rendered as `text-pink-400 text-sm mt-2` below the Post button. Textarea not cleared on 429 (correct — user may want to retry). (`frontend/web/src/views/Anime.vue` — found via grep `postError`)

**PASS — PATCH error state:** `editError` rendered as `text-pink-400 text-sm mt-2` below the edit textarea while it remains open. Correct.

**PASS — DELETE error + restore:** Optimistic remove → on failure, comment is re-inserted at correct sort position (by `created_at` + `id` tiebreaker, WR-04 fix) and `deleteError` auto-clears after 5 seconds. (`frontend/web/src/views/Anime.vue` — found via grep `setTimeout`)

**BLOCKER — `loadMoreFailed` error is unreachable:** `loadMoreComments` sets `commentsError.value = t('anime.ugc.loadMoreFailed')` on failure. The only place `commentsError` is rendered is `v-if="commentsError && comments.length === 0"` (line 788). But `loadMore` is only available when `commentsHasMore === true`, which implies at least one page of comments has loaded, which means `comments.length > 0`. Therefore the condition `comments.length === 0` is always false when `loadMoreFailed` fires. Users get a silent failure with the Load More button disappearing (disabled) and no error message shown. The spec requires: "Replace button label with `text-pink-400 text-sm` retry hint." Fix: introduce a separate `loadMoreError` ref, render it below the Load More button, and clear `commentsError` from the `loadMoreComments` catch block. (`frontend/web/src/views/Anime.vue` — found via grep `commentsError.value = t('anime.ugc.loadMoreFailed')`)

**WARNING — No programmatic focus on edit textarea open:** When the pencil icon is clicked, `startEditComment(c)` sets `editingCommentId.value = c.id` synchronously but does not call `nextTick().then(() => editTextareaRef.focus())`. The spec requires: "When the user clicks the pencil, focus moves into the edit textarea." Without focus management, keyboard users must Tab to the textarea manually after triggering the pencil. (`frontend/web/src/views/Anime.vue` — found via grep `startEditComment`)

**WARNING — No focus return on save/cancel edit:** The spec requires "On Save/Cancel, focus returns to the action-button row of the same comment." Neither `cancelEditComment` nor `saveEditComment` contains a focus-return call. This leaves keyboard users with focus on whatever the browser chose after the textarea was removed from the DOM.

**PASS — char counter `aria-live`:** The char counter switches from `aria-live="polite"` to `aria-live="assertive"` when `runeLen(newCommentBody) > 2000`. Correct per spec. (`frontend/web/src/views/Anime.vue` — found via grep `aria-live`)

**PASS — All icon-only buttons have `aria-label`:** Edit button has `:aria-label="$t('anime.ugc.editComment')"` and `:title`. Delete button has `:aria-label="$t('anime.ugc.deleteComment')"` and `:title`. (`frontend/web/src/views/Anime.vue` — found via grep `aria-label.*editComment`)

**PASS — URL persistence and tab state:** `ugcTab` initialized synchronously from `route.query.ugc` before first render. Two-way watcher: `route.query.ugc` → updates `ugcTab`, `ugcTab` → `router.replace`. Other query params preserved via `...route.query` spread. Unknown `?ugc=garbage` falls back to `reviews` without rewriting the URL. Matches all 10 rows of the URL contract.

**PASS — ActivityFeed comment branch:** `actionText()` correctly branches on `event.type === 'comment'` and returns `t('activity.comment.posted')`. WR-06 fix applied: link routes to `/user/${event.public_id || event.user_id}`. (`frontend/web/src/components/ActivityFeed.vue` — found via grep `event.type === 'comment'`)

**PASS — Admin delete visibility:** `v-if="c.user_id === authStore.user?.id || authStore.isAdmin"` on the trash button. Pencil gated on `c.user_id === authStore.user?.id && editingCommentId !== c.id` (own-comment only). Admins see trash on all comments but not pencil. Matches spec.

**PASS — Lazy fetch on tab activation:** `watch(ugcTab, ...)` calls `fetchComments()` only when switching to `comments` and `!commentsFetched.value`. No re-fetch on every tab revisit. Matches spec.

---

## Registry Safety

No shadcn and no third-party component registry detected (`components.json` absent). UI-SPEC.md Registry Safety table confirms no external registries. No audit required.

---

## Files Audited

- `/data/animeenigma/frontend/web/src/views/Anime.vue` (lines 606–920 template, lines 1308–1762 script — UGC section)
- `/data/animeenigma/frontend/web/src/components/ui/Tabs.vue`
- `/data/animeenigma/frontend/web/src/components/ActivityFeed.vue`
- `/data/animeenigma/frontend/web/src/locales/en.json` (`anime.ugc.*` and `activity.comment.*` sections)
- `/data/animeenigma/frontend/web/src/locales/ja.json` (same sections)
- `/data/animeenigma/frontend/web/src/locales/ru.json` (same sections)
- `/data/animeenigma/.planning/workstreams/social/phases/01-social-reviews-comments/01-UI-SPEC.md`
- `/data/animeenigma/.planning/workstreams/social/phases/01-social-reviews-comments/01-CONTEXT.md`
- `/data/animeenigma/.planning/workstreams/social/phases/01-social-reviews-comments/01-REVIEW-FIX.md`

---

## UI REVIEW COMPLETE

Phase 1 (social-reviews-comments) scores 18/24 — Copywriting is a clean pass; Experience Design and Spacing have one BLOCKER each (silent `loadMoreFailed` display, font-medium on tab labels); the Typography pillar is the primary degradation point due to the unresolved `font-medium` inheritance from `Tabs.vue` and pre-existing Review section weight violations.
