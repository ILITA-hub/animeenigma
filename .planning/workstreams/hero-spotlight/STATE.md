---
gsd_state_version: 1.0
workstream: hero-spotlight
milestone: v1.1-polish
milestone_name: HeroSpotlightVisualPolish
status: planned
last_updated: "2026-05-21"
last_activity: "2026-05-21 — v1.1-polish milestone planned. 10 phases (01 foundation + 9 card refactors), 30 requirements. Direction: cinematic backdrops + distinct templates. Driven by Phase 03 UAT findings ('EACH card looks poor'). Phase 01 also bundles the rapid-click transition blank-card bug fix."
progress:
  total_phases: 10
  completed_phases: 0
  total_plans: 10
  completed_plans: 0
  percent: 0
---

# Project State — `hero-spotlight` workstream

## Project Reference

- **Parent:** `/data/animeenigma/.planning/PROJECT.md`
- **Workstream PROJECT:** `PROJECT.md`
- **Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-21-hero-spotlight-block-design.md`
- **Source UAT:** `milestones/v1.0-phases/03-dynamic-cards-migration/03-UAT.md`
- **Source proposal:** `milestones/v1.1-polish/REFACTOR-PROPOSAL.md`
- **Requirements:** `REQUIREMENTS.md`
- **Roadmap:** `ROADMAP.md`

## Current Position

**Status:** v1.1-polish planned, awaiting execution.
**Current Phase:** none in progress
**Next Phase to Execute:** Phase 01 — Foundation
**Last Activity:** 2026-05-21

## Progress

**Phases Complete:** 0 / 10
**Plans Complete:** 0 / 10 (one PLAN.md per phase; all written)

## Phase Breakdown

| # | Phase | Status | Files Touched | Blocked By |
|---|-------|--------|---------------|-----------|
| 01 | Foundation | Planned | tokens, SpotlightBackdrop, SpotlightIcon, CTA classes, transition lock, CarouselControls | — |
| 02 | AnimeOfDayCard refactor | Planned | cards/AnimeOfDayCard.vue | 01 |
| 03 | RandomTailCard refactor | Planned | cards/RandomTailCard.vue | 01 |
| 04 | PersonalPickCard refactor | Planned | cards/PersonalPickCard.vue | 01 |
| 05 | NowWatchingCard refactor | Planned | cards/NowWatchingCard.vue | 01 |
| 06 | TelegramNewsCard refactor | Planned | cards/TelegramNewsCard.vue + backend pass-through | 01 |
| 07 | LatestNewsCard refactor | Planned | cards/LatestNewsCard.vue | 01 |
| 08 | PlatformStatsCard refactor | Planned | cards/PlatformStatsCard.vue + backend extension | 01 |
| 09 | NotTimeYetCard refactor | Planned | cards/NotTimeYetCard.vue + backend pass-through | 01 |
| 10 | ContinueWatchingNewCard refactor | Planned | cards/ContinueWatchingNewCard.vue | 01 |

## Session Continuity

**Stopped At:** All 10 plans written and committed. v1.0 files archived under `milestones/v1.0-*` (no changes). Ready to run autonomous mode.
**Resume File:** None
**Next Command:** `/gsd-autonomous --ws hero-spotlight`

## Open Questions (carried from v1.0)

- **Privacy for `now_watching`:** opt-out flag deferred to v1.2 if user
  demand emerges. v1.1 default: all users visible (unchanged from v1.0).
