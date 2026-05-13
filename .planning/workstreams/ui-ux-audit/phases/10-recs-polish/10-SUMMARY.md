---
phase: 10
plan: 1
subsystem: ui-ux-audit
tags: [frontend, vue3, i18n, recs, home, css-only]
requires: [phase-3, phase-9]
provides: [trending-row-reason-chip, top-row-giant-numeral]
affects: []
tech-stack:
  added: []
  patterns:
    - dominant-signal-mode-via-reduce-then-sort
    - row-level-reason-chip-not-per-card
    - layered-z-index-giant-numeral-behind-poster
key-files:
  created: []
  modified:
    - frontend/web/src/views/Home.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
decisions:
  - one-chip-per-row-mode-of-top_contributor
  - chip-hidden-when-first-item-pinned-pin_reason-line-owns-that-case
  - numeral-z0-cyan-400-10-decorative-not-affordance
  - numeral-guarded-to-index-lt-10-honest-when-row-grows
  - text-and-poster-lifted-to-relative-z-10-so-content-reads-above-numeral
metrics:
  duration: ~4min
  completed: 2026-05-13
---

# Phase 10 Summary: Recommendations polish — reasoning chip + Top-10 visual

**Completed:** 2026-05-13
**Plan:** 10-PLAN.md
**Outcome:** Two visual polish shipped on `Home.vue` plus 15 locale entries. The personalized/anonymous trending row now carries a single localized "reason category" chip below the row label, derived from the modal `top_contributor` signal across non-pinned items — this is the first frontend consumer of `RecItem.top_contributor`, which closes the visibility gate UA-060 logged against the existing backend field. The "Top Anime" column gains a giant decorative numeral behind each of the first 10 posters à la Netflix's Top-10 row, sitting at `text-cyan-400/10` with `z-0` so the poster + info read above it. No backend changes, no new components, no store changes. Closes UX-19, UX-20. Verifies UA-060.

## Changes shipped

### Reasoning chip (UX-19 / verifies UA-060)

**`frontend/web/src/views/Home.vue`** — three additions:

1. New `dominantSignalKey` computed: filters out pinned items, counts `top_contributor` occurrences across the visible 20 trending recs, returns the modal key (or `null` when nothing qualifies). Pure frontend; no backend call.
2. New `reasonI18nKey` computed: maps the dominant key onto `recs.reason.${k}` (or `null` when there's no dominant signal).
3. Template inserts a `<p class="text-xs text-cyan-400/80 mb-2">` chip between the row `<h2>` label and the existing pin-reason `<p>` line. `v-if="reasonI18nKey && !trendingRecs[0]?.pinned"` so the chip is hidden when the first item is pinned — the existing pin_reason line owns that case to avoid double-rendering reason copy.

**Locked behavior decisions (CONTEXT-driven):**

- **One chip per row, not per-card.** Per-card chips would compete with the rating badge and watchlist-status badge for visual weight on each poster. Per-row keeps the trending row quiet and aligns with the "row label → row content" reading order.
- **`s6_pin` excluded from the mode count.** Pinned items are filtered out of the reduce by the `!r.pinned` guard, so a single pinned card at position 0 never drowns the genuine signal across positions 1-19. The pin already has its own reason line below the chip.
- **Hidden when first item is pinned.** Otherwise the chip and the pin_reason line would stack two reason explanations above the same row. The chip yields to pin_reason.

### Top-10 numeral visual (UX-20)

**`frontend/web/src/views/Home.vue`** — three template touches on the Top column's `v-for="(anime, index) in topAnime"` router-link:

1. Outer `router-link` gains `pl-12 md:pl-16 overflow-hidden`. Left-padding reserves the gutter for the numeral; `overflow-hidden` clips the numeral so it doesn't bleed into adjacent rows when the wrapper is shorter than the giant glyph.
2. New decorative `<span aria-hidden="true" v-if="index < 10">{{ index + 1 }}</span>` immediately inside the wrapper, absolutely positioned at `-left-2 md:-left-4 top-1/2 -translate-y-1/2`, with `text-[80px] md:text-[120px] lg:text-[160px] font-black leading-none text-cyan-400/10 z-0 pointer-events-none select-none`. The `index < 10` guard keeps the row honest — items 11+ render without a numeral (the row is capped at 10 today, but the guard prevents a `11`/`12`/... numeral if the store ever expands the slice).
3. The existing rank badge (the small circular `1/2/3...` chip with gold/silver/bronze gradient) gets `relative z-10`; the `<img>` poster gets `relative z-10`; the info `<div class="flex-1 min-w-0">` gets `relative z-10`. The giant background numeral sits at `z-0`, all foreground content sits at `z-10`, so the cyan glyph reads as decorative depth — not the primary affordance.

**Why the small rank badge AND the giant numeral coexist:** the small badge is the *interactive* rank indicator (`getRankClass(index)` paints it gold/silver/bronze, screen-readers read it). The giant numeral is *atmospheric* — `aria-hidden="true"`, `pointer-events-none`, `select-none`, low opacity. Removing the small badge would have hurt screen-reader navigation; the audit asked for the Netflix-style visual treatment, not a replacement of the existing affordance. Both ship.

### i18n keys (recs.reason.s1 .. recs.reason.s5)

**`frontend/web/src/locales/{en,ru,ja}.json`** — five new entries per locale, nested inside the existing top-level `recs` namespace (NOT to be confused with `admin.recs` which is the Recs Debug panel's own namespace and lives further down each file). Total: 5 × 3 = 15 new entries.

| Key | English | Russian | Japanese |
|---|---|---|---|
| `recs.reason.s1` | Like your top picks | Похоже на ваш топ-список | あなたのトップピックに類似 |
| `recs.reason.s2` | By genres | По жанрам | ジャンルから |
| `recs.reason.s3` | Trending | В тренде | トレンド |
| `recs.reason.s4` | Highly rated | Высокий рейтинг | 高評価 |
| `recs.reason.s5` | Fresh this season | Новинки сезона | 今季の新作 |

`s6_pin` is intentionally absent — the chip skips pinned items entirely and the existing `recs.pinReason.becauseYouFinished` covers that copy.

## Decisions made (deviations from / refinements to plan)

| # | Decision | Rationale |
|---|---|---|
| 1 | Added `overflow-hidden` to the Top-row router-link wrapper (not in PLAN snippet) | The giant numeral at `-left-4 text-[160px]` overflows the wrapper bbox on `lg`. Without `overflow-hidden` the glyph bleeds onto the rank badge of the row below at certain viewport widths. Rule 1 fix (visual bug). |
| 2 | Added `relative z-10` to the existing small rank badge + info `<div>`, not just the poster | The poster was the only stacking-context container originally, but the small rank badge sits to the left of the poster and the info text sits to the right of it. Without lifting both to `z-10`, the numeral peeks through behind the gold gradient on the small badge and behind the title text on lower-DPI displays. Rule 1 fix. |
| 3 | The small rank chip (`getRankClass`-painted) was kept alongside the giant numeral, not replaced | The small badge is an accessible interactive affordance (screen-reader reads "1", "2", "3"; high-contrast gold/silver/bronze gradient). The giant numeral is decorative (`aria-hidden`, `select-none`, `pointer-events-none`, low opacity). Different audiences, different jobs. The plan didn't explicitly call for removal of the small badge, and removing it would have regressed a11y. |

No issues required Rule 4 (architectural change). Three Rule 1 surface refinements were applied inline — all on the Top-row template.

## Self-check: PASSED

- All three commits exist on `main` (verified via `git log --oneline`): `6841d7e` chip, `c7c7a36` numeral, `0fda35b` i18n.
- All artifact files exist at the listed paths.
- `cd frontend/web && bunx vue-tsc --noEmit` clean.
- `bash scripts/i18n-lint.sh` PASS (no missing keys, no syntax errors; pre-existing warnings only).
- All three locale JSONs parse cleanly (`python3 -c "import json; json.load(...)"` for each).
- All plan grep checks return the expected number of matches (`recs.reason.s` → 5 per locale; `reasonI18nKey|dominantSignalKey` → 5 in Home.vue; `index + 1|index < 10` → 3 in Home.vue).
- `make redeploy-web` succeeded; freshly-built bundle contains `"Like your top picks"` and `text-cyan-400/10` styling; `https://animeenigma.ru/` returns 200 OK with `Last-Modified` matching the redeploy.

See `10-VERIFICATION.md` for the full scorecard.
