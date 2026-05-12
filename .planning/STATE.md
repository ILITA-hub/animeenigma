---
gsd_state_version: 1.0
milestone: v3.0
milestone_name: Universal Anime Scraper
status: phase_16_wave_1_paused_usage_exhaustion
stopped_at: "Phase 16 (AnimePahe + EnglishPlayer) wave 1 paused on Anthropic usage exhaustion (resets 12:30pm Europe/Berlin). Discuss + UI-SPEC + RESEARCH + 6-plan plan are all committed. Wave 1 dispatched 3 parallel executor agents (16-01, 16-02, 16-04); all 3 ran out mid-execution. Partial work preserved in locked worktrees: 16-01 zero commits, 16-02 has RED test scaffold only, 16-04 has 2 commits (scraperApi + ReportButton diagnostics). Wave 2 (16-03 AnimePahe Provider), Wave 3 (16-05 boot wiring), Wave 4 (16-06 EnglishPlayer.vue + Anime.vue) untouched. Phase 15 fully complete and verified (passed 6/6). Resume after 12:30pm Berlin with /clear then /gsd-autonomous --from 16."
last_updated: "2026-05-12T05:30:00.000Z"
last_activity: 2026-05-12
progress:
  total_phases: 6
  completed_phases: 1
  total_plans: 10
  completed_plans: 4
  percent: 17
---


# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-09 — v3.0 milestone started)

**Core value:** A logged-in user opens the home page and sees a personalized "Up Next for you" row of anime they have not yet started — ranked by a transparent weighted-ensemble of signals. After completing an anime they enjoyed (score ≥ 7), a "Because you finished X" pin appears at the top of the row.

**Current focus:** Phase 16 — animepahe-and-new-englishplayer

## Current Position

Phase: 16 (animepahe-and-new-englishplayer) — EXECUTING
Plan: 1 of 6
Status: Executing Phase 16
Last activity: 2026-05-11 -- Phase 16 execution started

## Shipped Milestones

| Milestone | Shipped | Phases | Plans |
|-----------|---------|--------|-------|
| v1.0 Smart Watch Picker Overhaul | 2026-05-03 | 1-8 | — |
| v2.0 Recommendations Engine | 2026-05-07 | 9-14 | 8/8 |

## v3.0 Phase Map

| Phase | Name | Requirements |
|---|---|---|
| 15 | Foundation | SCRAPER-FOUND-01..10, SCRAPER-NF-01, SCRAPER-NF-03 |
| 16 | AnimePahe + new EnglishPlayer | SCRAPER-PAHE-01..05, SCRAPER-UI-01..04, SCRAPER-NF-02, SCRAPER-NF-05 |
| 17 | Observability | SCRAPER-OBS-01..05, SCRAPER-NF-04 |
| 18 | 9anime | SCRAPER-9ANI-01..06 |
| 19 | AnimeKai (gated) | SCRAPER-KAI-01..07 |
| 20 | Cutover | SCRAPER-CUT-01..07 (gated on ≥ 7 days clean prod traffic) |

## v3.0 Drivers (carried from triage 2026-05-09)

- HiAnime ecosystem dead: `hianime.to` unreachable from this server; `hianime.nz` shows shutdown notice; `aniwatch-api` GitHub repo deleted; `aniwatchtv.to` returns 404. All 4 aniwatch endpoints (search/episodes/servers/sources) time out at 8s upstream.
- Consumet broken: `riimuru/consumet-api:latest` (5 months stale) calls `enc-dec.app` with wrong body shape (`Expected body: text, agent`) → 100% of Zoro stream resolution fails. Other Consumet providers (animepahe, gogoanime) may still work but we don't currently route through them.
- AnimeLib's Kodik-fallback path was just disabled (commit `9347143`, feedback memory `feedback_animelib_no_kodik_fallback.md`). Users with EN-only anime currently have no working player tab other than Kodik.
- Verified alive provider sites (HTTP 200 + real body): AnimeKai (`animekai.to`), AnimePahe (alive mirror), Anitaku/Gogoanime (`anitaku.io`), AniZone (`anizone.to`). Verified dead: hianime.*, aniwatchtv.to, kaido.to, aniwave.to, animekai.bz.

## v1.0 / v2.0 Carryover (preserved across milestone switch)

- **v1.0 Phase 7 follow-up (override-rate re-snapshot)** ran post-deploy; tracked separately from active phases.
- **v2.1 backlog** documented in `.planning/milestones/v2.0-MILESTONE-AUDIT.md`: editable weights UI, S1 neighbor expansion, S6 seed history, per-anime CTR breakdown, session-based attribution, GDPR delete path for rec_events, rec_events rate limit, pin signal_id observability split. Out of v3.0 scope unless explicitly pulled into a phase.

## Session Continuity

Last session: 2026-05-11
Stopped at: Phase 15 (Foundation) — discuss + plan done, 4 PLAN.md files committed, execution paused on Anthropic usage exhaustion (resets 7:30am Europe/Berlin).
Resume from: `/gsd-autonomous --from 15` after usage reset. The autonomous workflow will detect existing CONTEXT.md + PLAN.md and skip straight to gsd-execute-phase. Each plan is autonomous-flagged so the executor can run them back-to-back without further user input.
