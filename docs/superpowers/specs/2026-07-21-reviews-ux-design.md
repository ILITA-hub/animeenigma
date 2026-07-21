# Reviews UX Improvements — Design

**Date:** 2026-07-21 · **Source:** feedback report `2026-07-21T01-16-37_tNeymik_telegram` (@realMiZZeR, TG 4117–4121)
**Scope:** frontend-only (`frontend/web/`). No backend changes — every needed endpoint already exists.

**Metrics:** UXΔ = +3 (Better) · CDI = 0.03 * 8 · MVQ = Sprite 88%/85%

## The four asks

1. A dedicated reviews tab (or filter) in the profile.
2. A better (bigger, more comfortable) review editor.
3. Text formatting in reviews.
4. Long reviews collapsed on the anime page.

## Approach chosen

One coherent feature set built around a **safe mini-markdown subset for reviews**, following the
existing `renderFanfic.ts` pattern (typed token blocks rendered as text nodes — **never `v-html`**;
that stays the entire XSS defense).

### 1. `utils/reviewMarkdown.ts` — parser (dependency-free)

Supported subset (flat, no nesting):
- Blocks: paragraphs (blank-line separated), single `\n` inside a paragraph = line break,
  `- ` / `* ` bullet lists.
- Inline: `**bold**`, `*italic*`, `~~strike~~`, `||spoiler||`.

Output: `ReviewBlock[]`; paragraph/list-item content = `InlineToken[]`
(`{ kind: 'text'|'bold'|'italic'|'strike'|'spoiler', text }`). Existing plain-text reviews parse
as plain paragraphs unchanged. Unit-tested (vitest) including injection attempts.

### 2. `components/anime/ReviewMarkdown.vue` — renderer + collapse

- Renders blocks via interpolation into `<p>/<ul>/<strong>/<em>/<s>` + spoiler `<button>` spans.
- Spoilers: redacted chip until clicked (per-span reveal, `aria-pressed`, i18n label).
- `collapsible` prop: collapsed max-height ≈ 14rem with bottom fade; "Show more / Show less"
  toggle only when content actually overflows (measured via ref; re-checked on content change).
  Used on the anime-page feed and the profile tab; the editor preview is never collapsed.

### 3. `components/anime/ReviewEditor.vue` — editor

- Auto-growing textarea (starts ~8 rows, grows with content up to ~60vh), replaces the tiny
  fixed `rows="4"` box in `Anime.vue`.
- Toolbar: Bold / Italic / Strike / Spoiler / List — wraps the current selection with markers;
  Ctrl/Cmd+B and Ctrl/Cmd+I shortcuts.
- Write ⇄ Preview toggle (preview renders through `ReviewMarkdown`), plus a one-line syntax hint.
- Pure `v-model` wrapper — submit/delete flow in `useAnimeSocial.ts` is untouched.

### 4. Profile → **Reviews tab** (`components/profile/MyReviewsTab.vue`)

- New own-profile-only tab (endpoint is JWT-claims-based): existing `reviewApi.getMyReviews()`
  → `GET /api/users/reviews` (returns entries with preloaded `anime` info).
- Lazy-fetch on first tab open. Client-side filter to `review_text !== ''` (the ask is a list of
  written reviews, not all scores). No tab count badge — lazy fetch means the count is unknown
  until the tab opens.
- Card: `PosterImage` thumb + localized title linking to `/anime/{id}`, score diamond, date,
  status context line, review text via `ReviewMarkdown` (collapsible).
- Kept as a separate component — `Profile.vue` is already 2.4k lines.

### 5. Integration + i18n

- `Anime.vue`: feed `<p>` → `ReviewMarkdown` (collapsible); form textarea → `ReviewEditor`.
- New i18n keys with full **en/ru/ja parity**: tab label, toolbar tooltips, write/preview,
  syntax hint, show more/less, spoiler label, empty-tab state.

## Rejected alternatives

- **"Has review" watchlist filter** instead of a tab — filters are server-side (facets + query
  params through the list endpoints), so it spreads into backend for a worse UX than a real
  reviews list. The reporter offered either; the tab is the first-listed and richer option.
- **markdown-it + DOMPurify** — real dependency + `v-html` surface for user content; the
  house pattern (typed blocks, text nodes) is safer and sufficient for this subset.
- **Public reviews tab on other users' profiles** — needs a new public endpoint; deferred (YAGNI).

## Out of scope

Backend length limits, review search/sort, reactions changes, ActivityFeed changes,
old-client rendering of new markup (plain markers degrade gracefully).
