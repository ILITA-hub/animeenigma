# 26-07 Verification — Dropdown Polish (Wave 5)

**Date:** 2026-05-19
**SCRAPER-HEAL-28**
**Status:** SKIPPED — gated on Phase 24 (`EnglishPlayer.vue` restoration).

## Execution gate check

```
$ ls frontend/web/src/components/player/EnglishPlayer.vue
ls: cannot access 'frontend/web/src/components/player/EnglishPlayer.vue': No such file or directory

$ ls frontend/web/src/components/player/
AnimeLibPlayer.vue
HanimePlayer.vue
KodikPlayer.vue
OtherSubsPanel.vue
RawPlayer.vue
ReportButton.vue
SubtitleOverlay.vue
```

`EnglishPlayer.vue` is **MISSING**. Per 26-07-PLAN.md's `<task type="checkpoint:gate">`
Task 0, this plan SKIPS when the file is absent. The component was
removed alongside HiAnime/Consumet in the May 2026 EN-tab cleanup and
its restoration is owned by Phase 24 SCRAPER-HEAL-17.

## Cross-phase status

- **Phase 24 status:** HARD-GATE-BLOCKED per
  `.planning/phases/24-en-reconnect/24-00-SUMMARY.md` — three EN providers
  (gogoanime, animepahe, animekai) all failed Wave 0 verification.
- **Phase 26 Wave 1 (26-01) recovery path:** **DELIVERED**. AllAnime is
  now the third live `domain.Provider` in the scraper failover pool. Its
  end-to-end smoke against Frieren returned 28 episodes from
  `api.allanime.day`.
- **Implication for Phase 24:** AllAnime alone unblocks Phase 24 if the
  operator decides EnglishPlayer.vue can ship with one fully-live
  provider. Per `.planning/phases/24-en-reconnect/24-CONTEXT.md`, that's
  the documented "minimum lovable" outcome.

## No code changes made

This plan touches no source files. It does NOT modify
`EnglishPlayer.vue`, locales, `useWatchPreferences.ts`, or the changelog.
Doing so without Phase 24's component restoration in place would
introduce dangling i18n keys + unreferenced capitalizeProvider branches.

## When this plan runs

Re-invoke `/gsd-execute-phase 26 --plan 26-07` AFTER:

1. Phase 24 ships, OR Phase 24 is unblocked-on-AllAnime and a minimal
   `EnglishPlayer.vue` is restored.
2. The file `frontend/web/src/components/player/EnglishPlayer.vue` exists
   on disk.

The plan's Tasks 1–6 then execute the dropdown polish: `capitalizeProvider`
branches for `allanime` (and any survivor picks from 26-03's Decision
Gate), `player.scraperProvider.*` i18n keys, Playwright e2e verification,
and the milestone-closing changelog entry.

## Acceptance criteria

- [x] `EnglishPlayer.vue` existence check ran (`ls` returned ENOENT).
- [x] No source files modified.
- [x] SKIPPED status recorded in this VERIFICATION.md.
- [x] Cross-reference to Phase 24's HARD-GATE-BLOCKED state documented.
- [x] Cross-reference to Phase 26 Wave 1's AllAnime delivery documented
      as the recovery path.

## SCRAPER-HEAL-28 disposition

**Status:** BLOCKED-ON-PHASE-24. To be tracked in
`.planning/milestones/v3.1-REQUIREMENTS.md` as a remaining open
requirement when v3.1 ships otherwise.
