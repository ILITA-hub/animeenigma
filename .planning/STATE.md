---
gsd_state_version: 1.0
milestone: v3.0
milestone_name: Universal Anime Scraper
status: planning
stopped_at: "v3.0 milestone started — defining requirements next. Goal: replace dead HiAnime (aniwatch / hianime.to) + broken Consumet (enc-dec.app contract) provider paths with a self-hosted Go scraping service targeting alive EN sources (AnimeKai, AnimePahe, Anitaku/Gogoanime). Kodik + AnimeLib untouched."
last_updated: "2026-05-09T00:00:00.000Z"
last_activity: 2026-05-09
progress:
  total_phases: 0
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-09 — v3.0 milestone started)

**Core value:** A logged-in user opens the home page and sees a personalized "Up Next for you" row of anime they have not yet started — ranked by a transparent weighted-ensemble of signals. After completing an anime they enjoyed (score ≥ 7), a "Because you finished X" pin appears at the top of the row.

**Current focus:** v3.0 Universal Anime Scraper — defining requirements.

## Current Position

Phase: Not started (defining requirements)
Plan: —
Status: Defining requirements
Last activity: 2026-05-09 — Milestone v3.0 started

## Shipped Milestones

| Milestone | Shipped | Phases | Plans |
|-----------|---------|--------|-------|
| v1.0 Smart Watch Picker Overhaul | 2026-05-03 | 1-8 | — |
| v2.0 Recommendations Engine | 2026-05-07 | 9-14 | 8/8 |

## v3.0 Drivers (carried from triage 2026-05-09)

- HiAnime ecosystem dead: `hianime.to` unreachable from this server; `hianime.nz` shows shutdown notice; `aniwatch-api` GitHub repo deleted; `aniwatchtv.to` returns 404. All 4 aniwatch endpoints (search/episodes/servers/sources) time out at 8s upstream.
- Consumet broken: `riimuru/consumet-api:latest` (5 months stale) calls `enc-dec.app` with wrong body shape (`Expected body: text, agent`) → 100% of Zoro stream resolution fails. Other Consumet providers (animepahe, gogoanime) may still work but we don't currently route through them.
- AnimeLib's Kodik-fallback path was just disabled (commit `9347143`, feedback memory `feedback_animelib_no_kodik_fallback.md`). Users with EN-only anime currently have no working player tab other than Kodik.
- Verified alive provider sites (HTTP 200 + real body): AnimeKai (`animekai.to`), AnimePahe (alive mirror), Anitaku/Gogoanime (`anitaku.io`), AniZone (`anizone.to`). Verified dead: hianime.*, aniwatchtv.to, kaido.to, aniwave.to, animekai.bz.

## v1.0 / v2.0 Carryover (preserved across milestone switch)

- **v1.0 Phase 7 follow-up (override-rate re-snapshot)** ran post-deploy; tracked separately from active phases.
- **v2.1 backlog** documented in `.planning/milestones/v2.0-MILESTONE-AUDIT.md`: editable weights UI, S1 neighbor expansion, S6 seed history, per-anime CTR breakdown, session-based attribution, GDPR delete path for rec_events, rec_events rate limit, pin signal_id observability split. Out of v3.0 scope unless explicitly pulled into a phase.

## Session Continuity

Last session: 2026-05-09
Stopped at: v3.0 milestone started; PROJECT.md + STATE.md updated. Next: define REQUIREMENTS.md per `/gsd-new-milestone` workflow.
Resume from: continue `/gsd-new-milestone` at the Research Decision step.
