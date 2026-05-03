# Phase 4 — Plan 01 — Summary

**Completed (code):** 2026-05-03
**Status:** ✓ Implementation complete; ⏳ Wave 2 batch deploy + production verification pending

## One-liner

When a logged-in user opens an anime, `Anime.vue` fetches their server-side
`watch_progress`, runs the result through a small state machine, and renders
exactly one of four banners above the player area: **"You finished ep N"**
(watching), **"You finished this anime"** with rewatch / mark-complete / find-
similar actions (finished), **"Episode N+1 — not yet available [{when}]"**
(not-yet-aired), or **"Episode N+1 is airing now"** (currently-airing). The
state machine also feeds the player's `:initial-episode` so the player mounts
on N+1 (watching) or N (finished / awaiting next ep) without each player
component duplicating that decision. Anonymous users keep the existing
localStorage path — D-01 in Phase 7 will add the parallel anon state machine.

## What changed

| Layer | File | Change |
|---|---|---|
| Frontend composable | `frontend/web/src/composables/useResumeStateMachine.ts` | NEW — `kind`, `lastWatched`, `startEpisode`, `finishedEpisode`, `loaded` reactive outputs derived from anime metadata + server `watch_progress` |
| Frontend API | `frontend/web/src/api/client.ts` | `getProgress(animeId)` shared with Phase 5 |
| Frontend view | `frontend/web/src/views/Anime.vue` | Wires `useResumeStateMachine`; renders 4-state banner above player; `:initial-episode` switched from raw `lastEpisode` to `resumeStartEpisode` (override-aware); rewatch click sets a manual override that wins over the state machine |
| Frontend i18n | `frontend/web/src/locales/{en,ru}.json` | New `anime.resume.{justFinished,youFinishedThis,rewatch,markCompleteInList,findSimilar,notYetAvailable,notYetAvailableEta,currentlyAiring}` keys |

The four player components themselves are unchanged for Phase 4 — they
already accept `:initial-episode` and the state-machine output flows through
that single prop. No per-player divergence (success criterion 4).

## Test results

`bunx tsc --noEmit` clean. ESLint: 0 errors, 3 pre-existing warnings in
untouched files. No frontend unit-test framework is set up in the repo;
Playwright e2e covers the user-facing flows (will be exercised against the
deployed Wave 2 environment).

## Success criteria status

| SC | Status | Evidence |
|---|---|---|
| 1. last < total → start ep N+1, breadcrumb "you finished N" | ✓ Code complete | `useResumeStateMachine.kind === 'watching'` → `startEpisode = last+1`; banner uses `finishedEpisode` |
| 2. last == total → "you finished this" surface w/ Rewatch / Mark complete / Find similar | ✓ Code complete | `kind === 'finished'` branch in `Anime.vue` banner; rewatch uses `resumeOverrideEpisode` |
| 3. last < total but next ep not aired → "not yet available [ETA]" / "currently airing" | ✓ Code complete | `kind === 'not-yet-aired'` uses `nextEpisodeAt` + `formatNextEpisode`; `currently-airing` triggers when ETA passed |
| 4. Same logic across Kodik, AnimeLib, HiAnime, Consumet | ✓ Code complete | State machine lives in `Anime.vue`; all 4 players consume the single `:initial-episode` prop |
| 5. Copy ships in EN + RU | ✓ Code complete | Both locales updated with the same 8 keys |

## What's next

1. **Right now:** Commit Phase 4 (single commit: composable + view + i18n).
2. **Wave 2 batch deploy:** Phase 5 already committed; `/animeenigma-after-update` ships both at once.
3. **Production verification:** `ui_audit_bot` has 8 anime_list entries (mixed statuses) seeded — open one in each state (watching, completed, ongoing) and verify the banner renders correctly + the player mounts on the right episode.
4. **D-01 follow-up (Phase 7):** Extend the state machine to read localStorage for anonymous users so they get the same banners.
