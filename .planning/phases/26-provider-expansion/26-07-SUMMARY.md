# Plan 26-07 — Dropdown Polish (Wave 5)

**Status:** SKIPPED
**Requirement:** SCRAPER-HEAL-28
**Wave:** 5 (gated)
**Date:** 2026-05-19

## What was done

NOTHING. Per 26-07-PLAN.md's `<task type="checkpoint:gate">` Task 0,
this plan SKIPS when `frontend/web/src/components/player/EnglishPlayer.vue`
is missing.

```
$ ls frontend/web/src/components/player/EnglishPlayer.vue
ls: cannot access ... : No such file or directory
```

## Why

`EnglishPlayer.vue` was removed during the May 2026 EN-tab cleanup and is
owned by **Phase 24 SCRAPER-HEAL-17**. Phase 24 is currently
HARD-GATE-BLOCKED on broken upstream EN providers — gogoanime + animepahe
+ animekai all failed Wave 0 verification.

Phase 26 Wave 1 (26-01, AllAnime lift) **is the recovery path**: AllAnime
is now live, delivering 28 Frieren episodes via `api.allanime.day`. That
gives Phase 24 a working provider to ship `EnglishPlayer.vue` against.

## What 26-07 will do when unblocked

Once Phase 24 restores EnglishPlayer.vue:

1. Add `capitalizeProvider()` branches for `allanime` (and any 26-03
   survivor picks).
2. Add `player.scraperProvider.*` i18n labels in en/ru/ja.
3. Playwright e2e verifying dropdown renders with multiple providers.
4. Milestone-closing changelog entry.

## Files modified

NONE.

## Files created (planning artifacts only)

- `.planning/phases/26-provider-expansion/26-07-VERIFICATION.md`
- `.planning/phases/26-provider-expansion/26-07-SUMMARY.md`

## Re-invocation

`/gsd-execute-phase 26 --plan 26-07` once EnglishPlayer.vue exists.
