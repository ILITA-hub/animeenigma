---
phase: 10-continue-watching-new-refactor
plan: 10
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-CWN-01, HSB-V11-CWN-02, HSB-V11-CWN-03]
human_verified: "pending eyeball confirmation on animeenigma.ru after merge + redeploy"
key_files:
  created: []
  modified:
    - frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.vue
    - frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.spec.ts
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
    - frontend/web/src/locales/__tests__/spotlight-keys.spec.ts
commits:
  - 840f197 feat(spotlight/10) add continueWatchingNew lastWatched/newEpisodeLine/resumeCtaWithEp i18n keys (en/ru/ja) (HSB-V11-CWN-03)
  - 89ccfc0 feat(spotlight/10) refactor ContinueWatchingNewCard to hero-ribbon + deep-link (HSB-V11-CWN-01/02)
  - 5bf1fa6 test(spotlight/10) cover ContinueWatchingNewCard hero-ribbon + deep-link refactor (HSB-V11-CWN-01/02)
  - 3d829bf chore(spotlight/10) merge worktree
metrics:
  metric_string: "UXΔ = +4 (Better) · CDI = 0.04 * 8 · MVQ = Phoenix 86%/82%"
  completed_date: 2026-05-25
---

# Phase 10 Plan 10: ContinueWatchingNewCard refactor Summary

Final card of v1.1-polish. Transformed the tiny "New episode N!" corner badge
into a hero treatment: a full-width gradient ribbon across the poster top, a
two-row episode meta with clear hierarchy, and a deep-link CTA that jumps
straight into the new episode. Frontend-only — no backend change.

## What shipped

### HSB-V11-CWN-01 — backdrop + hero ribbon
- Single-root `<article>` with `SpotlightBackdrop variant="poster-blur"` +
  `from-purple-500/30` overlay.
- Hero ribbon `inset-x-0 top-0` across the poster: purple→fuchsia gradient,
  play icon + "New episode {n}!" (`newEpisodeBadge`). Replaces the old corner
  badge for emphasis.

### HSB-V11-CWN-02 — two-row episode hierarchy + deep-link CTA
- Subdued "Watched up to ep {n}" (`lastWatched`, gray) over accent "New
  episode {n}" (`newEpisodeLine`, purple, larger) — the visual hierarchy makes
  "what's new" the focal point.
- CTA: `.cta-hero[data-accent="purple"]` → **`/anime/{id}?episode={n}`**.

### HSB-V11-CWN-03 — i18n (3 locales)
- Added `lastWatched`, `newEpisodeLine`, `resumeCtaWithEp` to en/ru/ja.
- Reused existing `title`, `newEpisodeBadge`, `resumeCta`. Normalized
  `newEpisodeBadge` wording (had leftover "ep" artifacts) without renaming the
  key.

## Deviations (all sound)
1. **Deep-link CTA correction (pre-flight by orchestrator):** PLAN used
   `/anime/{id}/watch?episode={n}`, but `/anime/:id/watch` is only a redirect
   alias to `/anime/:id`. The honored contract — read by `Anime.vue`'s
   `queryEpisode` computed and used by the sibling `ContinueWatchingRow.vue` —
   is `/anime/{id}?episode={n}`. Used the canonical form (no redirect hop); no
   watch-view change needed. Spec asserts `?episode={n}` AND that the href does
   NOT contain `/watch?episode=`.
2. **Ribbon weight `font-semibold` not `font-bold`** — the UI-SPEC contract (and
   the card spec's `not.toContain('font-bold')` assertion) permits only
   `font-medium`/`font-semibold`. Consistent with sibling cards.
3. **Parity spec updated** — it enumerates `continueWatchingNew` keys explicitly,
   so the new keys were added to the list + a cross-locale `{n}` interpolation
   guard.

## Verification
- Frontend: 325/325 spotlight + util + parity Vitest (18 files); `tsc --noEmit` clean.
- Deployed via `make redeploy-web`.

## Metrics
`UXΔ = +4 (Better) · CDI = 0.04 * 8 · MVQ = Phoenix 86%/82%`

## Deferred
- Thumbnail of the new episode's first frame (needs episode-thumbnail backend
  data — v1.2).
