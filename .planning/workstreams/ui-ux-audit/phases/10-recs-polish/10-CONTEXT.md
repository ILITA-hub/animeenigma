# Phase 10: Recommendations polish — reasoning chip + Top-10 visual - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, frontend-only polish on personalized rec row + Top row)

<domain>
## Phase Boundary

Two visual polish items on Home.vue's personalized/trending row and Top row:

- **UX-19** — Reasoning chip on every personalized rec card. The backend already surfaces `top_contributor` (s1-s5 signal IDs) on each `RecItem`. Render a small chip BELOW the row label showing a localized "reason category" derived from the signal ID. Closes the personalization-explanation gap with Crunchyroll. Tier E #10. Also verifies UA-060 top_contributor visibility.
- **UX-20** — Numbered Top-10 visual treatment on the "Топ аниме" row in Home.vue (currently a 3-column grid at line ~206). Apply a giant-numeral-behind-poster pattern à la Netflix: a `1`, `2`, `3` numeral rendered behind/beside each card at large size with reduced opacity. Tier E #14.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**UX-19 — Reasoning chip:**
- The backend `top_contributor` field carries signal IDs `s1`-`s5` (and the literal `s6_pin` for pinned items). True "Because you watched X" (with anime name) would require backend rework to surface seed-anime — out of scope for v0.1. Instead, render localized reason CATEGORIES:
  - `s1` → "Похоже на ваш топ-список" / "Like your top picks" / "あなたのトップピックに類似"
  - `s2` → "По жанрам" / "By genres" / "ジャンルから"
  - `s3` → "В тренде" / "Trending" / "トレンド"
  - `s4` → "Высокий рейтинг" / "Highly rated" / "高評価"
  - `s5` → "Новинки сезона" / "Fresh this season" / "今季の新作"
  - `s6_pin` → handled separately by existing pin_reason — skip the chip for pinned items.
- Chip rendering: small `text-xs text-cyan-400/80` badge below row label, ABOVE the card grid. ONE chip per row showing the dominant signal (mode of `top_contributor` across items). Not per-card — per-row keeps the visual quiet and avoids per-card noise.
- i18n keys: `recs.reason.s1` through `recs.reason.s5` in en/ru/ja.
- The chip is hidden when the row is the anonymous trending row (no personalization → no signal).

**UX-20 — Top-10 numbered visual:**
- Modify the Top row template (Home.vue line ~225-280 — the `topAnime` v-for loop) to render the rank index as a giant numeral element BEHIND the poster.
- Numeral styling: `text-[120px] md:text-[160px] font-black leading-none text-cyan-400/10 absolute -left-4 top-1/2 -translate-y-1/2 z-0 pointer-events-none select-none`. Behind the poster (z-0; poster gets z-10). Use `index + 1` (1-indexed). Visible only on items 1-10 (skip 11+).
- Layout: the current Top row uses a 3-column grid (`grid-cols-1 md:grid-cols-2 lg:grid-cols-3`). To accommodate the numeral, wrap each item in a relative container and shift the poster right with `pl-12 md:pl-16`. The numeral occupies the gutter.
- Mobile: numeral scales to `text-[80px]` to fit. Acceptable visual on 375px-wide viewports.
- Applies ONLY to first 10 items. If `topAnime.length > 10`, items 11+ render without numeral (keeps the row honest).
- No backend change. Pure CSS / template.

### Locked from ROADMAP

- Both items are visual polish on Home.vue. No new components extracted (the row remains inline in Home.vue per existing pattern).
- "Because you watched X" with literal anime name is deferred (requires backend seed-tracking).
- Numbered numeral only applies to the dedicated Top row, NOT the trending row (which is personalized order, not ranked-by-score).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `frontend/web/src/composables/useRecs.ts` already exposes `top_contributor` on each `RecItem`. No backend or composable change.
- `frontend/web/src/views/Home.vue` — trending row block at lines 32-88; Top row block starts around line 206; both use Tailwind directly.
- Existing i18n nesting under `recs.*` and `home.*` in the locale files — extend with `recs.reason.*` keys.

### Established Patterns

- Reason copy localization mirrors Phase 3's UX-09 pin-reason approach (`pin_reason_key` + i18n) — reuse the convention for consistency.
- Numbered numeral pattern (giant-numeral-behind-poster) is established in Netflix's Top-10 row globally — no design system precedent in AnimeEnigma but the Tailwind primitives suffice.

### Integration Points

- No router changes. No backend changes. No new tables. No new endpoints.
- Both visual changes ship on the same redeploy (`make redeploy-web`).

</code_context>

<specifics>
## Specific Ideas

- The reasoning chip is a SINGLE chip per row, not per-card. Per-card chips would visually compete with the rating badge and watchlist-status badge. Per-row chip is cleaner.
- For the dominant-signal computation: `Object.entries(_.countBy(recs, 'top_contributor')).sort((a, b) => b[1] - a[1])[0][0]`. Pure frontend, no backend call. Lodash is already a dep — confirm during execution (or use inline reduce).
- The Top-10 numeral uses `cyan-400/10` — very subtle. The poster + content remain visually dominant. Numeral is decorative depth, not the primary affordance.

</specifics>

<deferred>
## Deferred Ideas

- True "Because you watched [Anime Name]" with backend seed-tracking — separate backend feature, deferred.
- Numbered numeral on `/browse?sort=popularity` Top-10 — Phase 11 introduces sort dropdown; could carry numeral pattern there too. Deferred to Phase 11 polish.
- Per-card reason chip — design noise, deferred.

</deferred>
