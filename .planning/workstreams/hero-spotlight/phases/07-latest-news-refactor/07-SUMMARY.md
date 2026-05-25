---
phase: 07-latest-news-refactor
plan: 07
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-LN-01, HSB-V11-LN-02, HSB-V11-LN-03, HSB-V11-LN-04]
human_verified: "pending eyeball confirmation on animeenigma.ru after merge + redeploy"
key_files:
  created: []
  modified:
    - frontend/web/src/components/home/spotlight/tokens.ts
    - frontend/web/src/components/home/spotlight/tokens.spec.ts
    - frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue
    - frontend/web/src/components/home/spotlight/cards/LatestNewsCard.spec.ts
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
    - frontend/web/src/locales/__tests__/spotlight-keys.spec.ts
commits:
  - 08cea79 feat(hero-spotlight/07) LatestNewsCard type-icons + relative dates (HSB-V11-LN-01..04)
metrics:
  metric_string: "UXΔ = +3 (Better) · CDI = 0.03 * 8 · MVQ = Sprite 82%/84%"
  completed_date: 2026-05-25
---

# Phase 07 Plan 07: LatestNewsCard refactor Summary

Refactored `LatestNewsCard.vue` from a flat changelog list into a visually
hierarchical surface: gradient-mesh amber backdrop, per-entry type-icons,
type-coded pills, and human relative dates. The fragile sentence-splitter
regex (`splitMessage`/`entryBody`) is gone — title is a clean 60-char
truncation.

## What shipped

### HSB-V11-LN-04 — `cardTokens.latest_news` widening
- Extended `cardTokens.latest_news` with `iconByType` + `labelByType` lookup
  tables (in `tokens.ts`). Type → icon: `feat`/`feature`/`docs` → sparkles,
  `fix`/`infrastructure` → wrench, `perf`/`improvement` → lightning.
- `labelByType` carries `{i18nKey, accent}` badge tokens — `FEAT_BADGE`
  (cyan), `FIX_BADGE` (green), `PERF_BADGE` (amber) — reused across the
  conventional-commit short forms AND the long-form synonyms that ship in
  the live `changelog.json` today (`feature`/`improvement`/`infrastructure`),
  so the pill never silently disappears for real entries.
- `tokens.spec.ts` gained a `cardTokens.latest_news extensions` describe
  block: iconByType keys, labelByType i18nKey-under-`spotlight.latestNews.*`
  + Tailwind accent shape, per-type color-coding, valid-icon-name coverage,
  and explicit long-form-synonym coverage.

### HSB-V11-LN-01 — gradient-mesh backdrop
`LatestNewsCard.vue` wraps content in a single-root `<article>` with
`<SpotlightBackdrop variant="gradient-mesh" accent="amber" />` as the lowest
layer (single-root discipline inherited from Phase 04, so `<Transition
mode="out-in">` never wedges).

### HSB-V11-LN-02 — type-icons + type pills + i18n
- Per-entry `SpotlightIcon` resolved via `iconFor(entry.type)`; type pill
  resolved via `badgeFor(entry.type)` → labeled i18n string.
- Added `spotlight.latestNews.typeFeat` / `typeFix` / `typePerf` to all 3
  locales (en/ru/ja); `spotlight-keys.spec.ts` `latestNewsKeys` expanded to
  assert en+ru parity for the new keys.

### HSB-V11-LN-03 — relative dates + clean title
- `formatEntryDate` uses `Intl.RelativeTimeFormat` with the active locale,
  falling back to an absolute date for entries older than ~30 days.
- `entryTitle(msg)` = first 60 chars + ellipsis if longer. `splitMessage`
  and the `entryBody` element were deleted entirely.

## Merge note (non-standard)
The Phase 07 executor's worktree branched from a **pre-Phase-02** base
(`87c75f8`), so a naive `git merge` produced 3-way conflicts in `tokens.ts`,
`tokens.spec.ts`, and `spotlight-keys.spec.ts` (shared files that phases
02–06 had since evolved). Rather than land a polluted merge commit, the
changes were re-applied as a single clean commit on top of current main
(`642fed5`): conflicts resolved by **unifying** both widenings of
`cardTokens` (Phase 02 `anime_of_day.genreColors` + Phase 07 `latest_news`)
and **keeping** Phase 03's tagline-parity tests alongside the expanded
`latestNewsKeys`. `LatestNewsCard.vue`/`.spec.ts` applied wholesale (no
other phase touched them).

## Verification
- Frontend: 287/287 spotlight Vitest tests pass (15 files); `tsc --noEmit` clean.
- Deployed to https://animeenigma.ru/ via `make redeploy-web`.

## Metrics
`UXΔ = +3 (Better) · CDI = 0.03 * 8 · MVQ = Sprite 82%/84%`

## Deferred
- Backend split of `message` → `title` + `body` (v1.2; resolver schema change).
