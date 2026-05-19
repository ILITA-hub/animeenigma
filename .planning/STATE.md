---
gsd_state_version: 1.0
milestone: v3.1
milestone_name: Scraper Self-Healing
status: REOPENED — Phase 24 ready to plan
stopped_at: "v3.1 reopened 2026-05-19 after (a) commit fe3b487 (2026-05-18) folded an untracked deletion of EnglishPlayer.vue + e2e specs + parsers into a docs commit, undoing SCRAPER-HEAL-08's user-facing surface; (b) commit 9bb8a85..49b32f2 (2026-05-18 cleanup pass) finished the rip by removing the EN tab from Anime.vue, narrowing type unions to exclude 'english', and stripping i18n keys; (c) original 2026-05-13 milestone audit BLK-INT-01 (rotated hls3 hosts not in allowlist) was never closed; (d) operator scope-expanded with provider-expansion ask (AllAnime lift, fresh 2026 candidates, AnimeKai recovery). Three new phases scoped: 24 EN Reconnect, 25 Audit Findings Resolution, 26 Provider Expansion."
last_updated: "2026-05-19T00:00:00.000Z"
last_activity: 2026-05-19 — Milestone v3.1 REOPENED — three new phases scoped, planning to begin
progress:
  total_phases: 12
  completed_phases: 8
  total_plans: 36
  completed_plans: 29
  percent: 67
---

# Project State

## Project Reference

See: .planning/PROJECT.md (last updated 2026-05-09 — v3.0 milestone started; v3.1 inherits the same project context).

**Core value:** A logged-in user opens the home page and sees a personalized "Up Next for you" row of anime they have not yet started — ranked by a transparent weighted-ensemble of signals. After completing an anime they enjoyed (score ≥ 7), a "Because you finished X" pin appears at the top of the row. v3.1's contribution: when the user actually presses Play on an English-source anime, the player surfaces real video instead of upstream ad-decoy garbage.

**Current focus:** Phase 24 — EN Reconnect (v3.1 reopened)

## Current Position

Phase: 24 — EN Reconnect (v3.1 reopened, ready to plan)
Plan: —
Status: Planning — `/gsd-plan-phase --phase 24` is the next operator action
Last activity: 2026-05-19 — v3.1 reopened with three new phases (24 EN Reconnect, 25 Audit Findings Resolution, 26 Provider Expansion). See `.planning/milestones/v3.1-REOPENING.md` for the rewrite trace.

## Shipped Milestones

| Milestone | Shipped | Phases | Plans |
|-----------|---------|--------|-------|
| v1.0 Smart Watch Picker Overhaul | 2026-05-03 | 1-8 | — |
| v2.0 Recommendations Engine | 2026-05-07 | 9-14 | 8/8 |

## In-Flight Milestones

| Milestone | Phases | Status |
|-----------|--------|--------|
| v3.0 Universal Anime Scraper | 15-20 | Phases 15-19 SHIPPED 2026-05-11..12; Phase 20 cutover SHIPPED 2026-05-18 (over-rotated — see v3.1 Phase 24) |
| v3.1 Scraper Self-Healing | 21-26 | REOPENED 2026-05-19 — Phases 21-23 SHIPPED 2026-05-13 but regression undid 21's user-facing surface; Phases 24-26 newly scoped |

## v3.1 Phase Map

| Phase | Name | Requirements | Status |
|---|---|---|---|
| 21 | Playability Foundation | SCRAPER-HEAL-01..08 | SHIPPED 2026-05-13 — user-facing surface (SCRAPER-HEAL-08 EnglishPlayer.vue) regressed 2026-05-18; restored in Phase 24 |
| 22 | Provider Robustness | SCRAPER-HEAL-09..11 | SHIPPED 2026-05-13 — hls3 host rotation (BLK-INT-01) still open; addressed in Phase 25 |
| 23 | Self-Maintenance Loop | SCRAPER-HEAL-12..16 | SHIPPED 2026-05-13 — W-INT-02 (cacheStream symbol), W-INT-03 (silent-200) addressed in Phase 25 |
| 24 | EN Reconnect | SCRAPER-HEAL-17..20 | PLANNING — restore EnglishPlayer.vue, EN tab, type unions; provider verification per "test each provider" gate |
| 25 | Audit Findings Resolution | SCRAPER-HEAL-21..24 | PLANNING — BLK-INT-01 hls3 allowlist auto-discovery, W-INT-01 probe race test, W-INT-02 cacheStream symbol, W-INT-03 silent-200 |
| 26 | Provider Expansion | SCRAPER-HEAL-25..28 | PLANNING — AllAnime parser lift to scraper, fresh 2026 EN-source survival research, AnimeKai recovery (carried from v3.0 Phase 19) |

## v3.1 Drivers (from PoC 2026-05-13)

- VibePlayer (HD-1, the default first server returned by gogoanime) serves ad-decoy m3u8 manifests whose entire variant playlist points at TikTok's ad CDN (`p16-ad-sg.ibyteimg.com`). Real headless Chromium gets the same poison — confirmed IP-level, not fingerprint. Production EnglishPlayer plays *something* (manifest parses, duration loads) but never any actual video frame.
- StreamHG (`otakuhg.site` → `premilkyway.com`) and Earnvids (`otakuvid.online` → `dramiyos-cdn.com`) work perfectly — Go regex on packed JS extracts a valid signed `.m3u8`, HLS proxy returns 200, real `.ts` segments. They were never tried because VibePlayer is sorted first.
- Both StreamHG and Earnvids ALSO expose a secondary `hls3` URL family at rotated CDNs (`managementadvisory.sbs`, `exoplanethunting.space`) for use when `hls2` signed-URL TTL expires — currently unused.
- v3.0 Phase 17 observability infrastructure (metrics, health gauges, admin endpoint) ships v3.1's metrics without new infrastructure work.

## v3.0 Carryover (resumable, not abandoned)

- **v3.0 Phase 20 Cutover** — SHIPPED 2026-05-18 across plans 20-02..20-05 but **over-rotated**: in addition to the targeted HiAnimePlayer.vue + ConsumetPlayer.vue + parser/hianime + parser/consumet deletions, the cutover swept up EnglishPlayer.vue, the EN tab markup in Anime.vue, the i18n keys (`tabEnglish`, `tabDebugSuffix`, `sourceUnhealthy`, etc.), and the multi-source dropdown infrastructure that v3.1 Phase 21 had shipped. The Phase 20 success criteria are met (no HiAnime/Consumet residue) but the *unintended* over-deletion regressed v3.1 Phase 21's user-facing surface. v3.1 Phase 24 (this milestone reopening) restores the regression cleanly.
- **AnimeKai (Phase 19) gated R&D** carried as `SCRAPER_ANIMEKAI_ENABLED=false`. Picked up by v3.1 Phase 26.

## v1.0 / v2.0 Carryover (preserved across milestone switches)

- **v1.0 Phase 7 follow-up (override-rate re-snapshot)** ran post-deploy; tracked separately from active phases.
- **v2.1 backlog** documented in `.planning/milestones/v2.0-MILESTONE-AUDIT.md`: editable weights UI, S1 neighbor expansion, S6 seed history, per-anime CTR breakdown, session-based attribution, GDPR delete path for rec_events, rec_events rate limit, pin signal_id observability split. Out of v3.1 scope unless explicitly pulled into a phase.

## Session Continuity

Last session: 2026-05-19T00:00:00.000Z
Stopped at: v3.1 reopened with three new phases scoped (24 EN Reconnect, 25 Audit Findings Resolution, 26 Provider Expansion). Top-level planning docs (STATE, MILESTONES, ROADMAP) updated; v3.1-REQUIREMENTS.md rewritten from the misnamed v3.0-archive; per-phase CONTEXT.md created for each new phase; v3.1-MILESTONE-AUDIT.md annotated as superseded. No code changes yet — that begins at `/gsd-plan-phase --phase 24`.
Resume from: `/gsd-plan-phase --phase 24` (Phase 24 EN Reconnect — the user-visible regression restore is the lowest-risk fastest-win starting point).

## Operator Next Steps

- Plan Phase 24 with `/gsd-plan-phase --phase 24`. Phase 24 CONTEXT.md is ready at `.planning/milestones/v3.1-phases/24-en-reconnect/24-CONTEXT.md`.
- Phase 24 has a hard "test each provider" gate (Phase 0 from the standalone plan superseded into this milestone) — gogoanime + animepahe + animekai-fall-through must all be verified end-to-end against Frieren (MAL 52991) before any frontend code touches.
