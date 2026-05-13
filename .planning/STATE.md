---
gsd_state_version: 1.0
milestone: v3.1
milestone_name: Scraper Self-Healing
status: executing
stopped_at: v3.1 milestone artifacts complete — `.planning/milestones/v3.1-REQUIREMENTS.md`, `v3.1-ROADMAP.md`, `phases/21..23-*/CONTEXT.md`, ROADMAP.md Phase Details blocks for Phase 21/22/23, `.claude/maintenance-prompt.md` Patterns 6/7. PoC artifacts at `/tmp/extractor-poc/` (ephemeral, not committed).
last_updated: "2026-05-13T07:13:33.963Z"
last_activity: 2026-05-13
progress:
  total_phases: 9
  completed_phases: 7
  total_plans: 33
  completed_plans: 27
  percent: 82
---

# Project State

## Project Reference

See: .planning/PROJECT.md (last updated 2026-05-09 — v3.0 milestone started; v3.1 inherits the same project context).

**Core value:** A logged-in user opens the home page and sees a personalized "Up Next for you" row of anime they have not yet started — ranked by a transparent weighted-ensemble of signals. After completing an anime they enjoyed (score ≥ 7), a "Because you finished X" pin appears at the top of the row. v3.1's contribution: when the user actually presses Play on an English-source anime, the player surfaces real video instead of upstream ad-decoy garbage.

**Current focus:** Phase 23 — Self-Maintenance Loop

## Current Position

Phase: 23 (Self-Maintenance Loop) — EXECUTING
Plan: 2 of 3
Status: Ready to execute
Last activity: 2026-05-13

## Shipped Milestones

| Milestone | Shipped | Phases | Plans |
|-----------|---------|--------|-------|
| v1.0 Smart Watch Picker Overhaul | 2026-05-03 | 1-8 | — |
| v2.0 Recommendations Engine | 2026-05-07 | 9-14 | 8/8 |

## In-Flight Milestones

| Milestone | Phases | Status |
|-----------|--------|--------|
| v3.0 Universal Anime Scraper | 15-20 | Phase 15-19 SHIPPED 2026-05-11..12; Phase 20 PAUSED 2026-05-13 (1/5 plans done) — resumes after v3.1 Phase 21 + 7-day clean soak |
| v3.1 Scraper Self-Healing | 21-23 | EXECUTING — Phase 21 ready to plan |

## v3.1 Phase Map

| Phase | Name | Requirements |
|---|---|---|
| 21 | Playability Foundation | SCRAPER-HEAL-01..08 |
| 22 | Provider Robustness | SCRAPER-HEAL-09..11 |
| 23 | Self-Maintenance Loop | SCRAPER-HEAL-12..16 |

## v3.1 Drivers (from PoC 2026-05-13)

- VibePlayer (HD-1, the default first server returned by gogoanime) serves ad-decoy m3u8 manifests whose entire variant playlist points at TikTok's ad CDN (`p16-ad-sg.ibyteimg.com`). Real headless Chromium gets the same poison — confirmed IP-level, not fingerprint. Production EnglishPlayer plays *something* (manifest parses, duration loads) but never any actual video frame.
- StreamHG (`otakuhg.site` → `premilkyway.com`) and Earnvids (`otakuvid.online` → `dramiyos-cdn.com`) work perfectly — Go regex on packed JS extracts a valid signed `.m3u8`, HLS proxy returns 200, real `.ts` segments. They were never tried because VibePlayer is sorted first.
- Both StreamHG and Earnvids ALSO expose a secondary `hls3` URL family at rotated CDNs (`managementadvisory.sbs`, `exoplanethunting.space`) for use when `hls2` signed-URL TTL expires — currently unused.
- v3.0 Phase 17 observability infrastructure (metrics, health gauges, admin endpoint) ships v3.1's metrics without new infrastructure work.

## v3.0 Carryover (resumable, not abandoned)

- **v3.0 Phase 20 Cutover** — Plan 20-01 (pre-flight guardrail) complete. Plans 20-02 through 20-05 paused. The Cutover PR's gate ("≥ 7 days clean prod traffic on EnglishPlayer") is structurally unsatisfiable until v3.1 Phase 21 ships. After Phase 21 production deploy, soak clock starts; if 7 days pass cleanly, Phase 20 resumes from 20-02. If new regressions appear (caught by v3.1 Phase 23 canary), soak clock resets.
- **AnimeKai (Phase 19) gated R&D** carried as `SCRAPER_ANIMEKAI_ENABLED=false`. Independent of v3.1.

## v1.0 / v2.0 Carryover (preserved across milestone switches)

- **v1.0 Phase 7 follow-up (override-rate re-snapshot)** ran post-deploy; tracked separately from active phases.
- **v2.1 backlog** documented in `.planning/milestones/v2.0-MILESTONE-AUDIT.md`: editable weights UI, S1 neighbor expansion, S6 seed history, per-anime CTR breakdown, session-based attribution, GDPR delete path for rec_events, rec_events rate limit, pin signal_id observability split. Out of v3.1 scope unless explicitly pulled into a phase.

## Session Continuity

Last session: 2026-05-13T07:13:29.790Z
Stopped at: v3.1 milestone artifacts complete — `.planning/milestones/v3.1-REQUIREMENTS.md`, `v3.1-ROADMAP.md`, `phases/21..23-*/CONTEXT.md`, ROADMAP.md Phase Details blocks for Phase 21/22/23, `.claude/maintenance-prompt.md` Patterns 6/7. PoC artifacts at `/tmp/extractor-poc/` (ephemeral, not committed).
Resume from: `/gsd-plan-phase --phase 21` to draft Phase 21's 4 plans across 2 waves, then `/gsd-execute-phase --phase 21`. After Phase 21 production deploy, monitor canary signals (none yet — Phase 23 ships them) by hand for 24-48h, then proceed to Phase 22. Phase 23 last. v3.0 Phase 20 resumes only after v3.1 ships AND 7-day clean soak completes.
