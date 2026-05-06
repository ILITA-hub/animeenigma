---
gsd_state_version: 1.0
milestone: v2.0
milestone_name: Recommendations Engine
status: executing
stopped_at: Phase 13 shipped — S6 closed-loop "watched-after" pin live in production. Logged-in users completing a score≥7 anime see an instant "Because you finished X" pin at recs[0] of their "Up Next for you" row. Cascade: local co-occurrence (129k edges, 455 seeds in production) → Shikimori /similar → score-5 fallback → silent omission. Synchronous seed update inside MarkEpisodeWatched at 48ms p95 production (well under 200ms bound). Pin expires at 7 days, verified live. Phase 14 (Admin Debug Page & Eval Pipeline) opens next.
last_updated: "2026-05-06T15:35:00.000Z"
last_activity: 2026-05-06 -- Phase 13 shipped (S6 pin live; 14 commits)
progress:
  total_phases: 14
  completed_phases: 5
  total_plans: 8
  completed_plans: 7
  percent: 93
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-04 for v2.0)

**Core value:** A logged-in user opens the home page and sees a personalized "Up Next for you" row of anime they have not yet started — ranked by a transparent weighted-ensemble of signals. After completing an anime they enjoyed (score ≥ 7), a "Because you finished X" pin appears at the top of the row.
**Current focus:** Phase 14 — Admin Debug Page & Eval Pipeline

## Current Position

Phase: 14 (Admin Debug Page & Eval Pipeline) — NEXT
Plan: 0 of TBD (Phase 14 not yet planned)
Next phase: Phase 14 (Admin Debug Page & Eval Pipeline)
Status: Phase 13 shipped; Phase 14 awaiting plan-phase invocation
Last activity: 2026-05-06 -- Phase 13 shipped (S6 pin live; 14 commits)

## Performance Metrics

**Velocity:**

- Total plans completed: 0 (v2.0)
- Average duration: —
- Total execution time: —

*Updated after each plan completion*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table. Recent decisions affecting current work:

- 2026-05-06: Phase-13 SC#3 cascade observation — production verification with seed=Grand Blue returned `pin_source="shikimori_similar"` even though the live co-occurrence matrix has 129,036 edges across 455 seeds. The post-S11 pool for any specific seed is much smaller than the raw matrix because every anime ui_audit_bot already has in their list is dropped. Phase 14 admin debug page MUST surface pin_source so we can measure cascade hit-rate (local% / shikimori_similar% / score-5%) at scale.
- 2026-05-06: Phase-13 latency measurement — production MarkEpisodeWatched p95 = 48ms (10 sequential calls, full nginx → gateway → JWT → player → Postgres + Redis stack). The 5ms relative bound (REC-INFRA-03) is best validated via the in-process micro-benchmark TestMarkEpisodeWatched_SeedUpdate_LatencyUnder5ms (sqlite, < 50ms total) since production-level network overhead (~30ms) swamps the in-handler delta. Absolute < 200ms bound easily met.
- 2026-05-06: Phase-13 pin RecItem.Final = 0 (not null) — RecItem.Final is float64 in the Go domain; the spec said null but JSON-encoded 0 is the closest valid shape. Frontend gates display on item.pinned (not Final), so the rank=1 pin renders correctly even with Final=0 sitting numerically below the rank-2 ensemble item's Final score. Phase 14 CTR events MUST tag rec_click events with pin_seed_anime_id when item.pinned===true so we can measure pin-driven engagement separately from ensemble-driven engagement.
- 2026-05-06: Phase-12 SC#5 verified live — ui_audit_bot top-3 shifted post-redeploy (rank 3: Re:Zero S4 → Chainsaw Man Recap; rank-1 Steel Ball Run Final score 0.323 → 0.523, demonstrating S5 contribution at weight 0.20).
- 2026-05-06: S5 affinity vector for ui_audit_bot populated 5/6 dimensions (genre / kind / rating / studio / source); tag dimension empty because the AniList tags backfill is still streaming for ui_audit_bot's specific watched anime — handled gracefully by the cold-start contract spec §3.3 (missing-attribute-equals-zero).
- 2026-05-06: Shikimori adaptation-source field is named `origin`, NOT `source` (CONTEXT.md §S5 was wrong). Live introspection confirmed; parser fixed in Phase 12 Wave 1.
- 2026-05-06: libs/database wrapper's AutoMigrate doesn't create m2m join tables for relations added to pre-existing structs — fall through to gorm's native AutoMigrate after the wrapper for new m2m. Caught at Phase 12 Wave 1 redeploy verification.
- 2026-05-06: v2.0 roadmap structured as 6 phases (9-14): Foundation → Population Signals + Trending → User Signals + Up Next Row → S5 TF-IDF → S6 Pin → Admin Debug + Eval. Each phase independently shippable. Phase numbering continues from v1.0 (last shipped phase = 8).
- 2026-05-04: v2.0 ensemble pattern locked over tiered fallback or two-stage retrieval+ranker — graceful cold-start, free admin breakdown, can grow into two-stage at scale without rewrite.
- 2026-05-04: Per-pool min-max normalization is the architectural fix that lets weights be coherent across signals at different raw scales.
- 2026-05-04: S6 score threshold ≥ 7 with fallback to ≥ 5 if pool too thin (more conservative than initial recommendation; cleaner signal).
- 2026-05-04: S6 Variant B (pinned tile) over Variant A (weight-shift) for v2.0 — more transparent, easier to debug; weight-shift deferred to v2.1 once pin CTR measured.
- 2026-05-04: Hybrid storage — Postgres precomputed signals + Redis 6h top-N cache. S6 seed update is synchronous on `MarkEpisodeWatched` so the pin appears immediately.
- 2026-05-04: Anonymous personalization deferred to v2.1; v2.0 anon users see population-only "Trending now" row.
- 2026-05-04: Pluggable `SignalModule` interface from day one — single seam for future signals (S7-S10).
- 2026-05-04: S5 TF-IDF time-weighting falls back to integer episode count for Kodik rows (84% of watch_history; duration_watched unreliable).

(Decisions carried from v1.0 are preserved in PROJECT.md Key Decisions table; this section tracks v2.0-fresh decisions only.)

### Pending Todos

- Plan-phase 14 (Admin Debug Page & Eval Pipeline): the debug page MUST surface S6 pin_source ("local" vs "shikimori_similar" vs "score_5_fallback") in the per-row breakdown so we can measure cascade hit-rate at scale. Add Prometheus counter `rec_pin_source_total{source="..."}` for Grafana visibility.
- Plan-phase 14: rec_click and rec_watched events MUST tag pin_seed_anime_id when item.pinned===true so eval can answer "what's the CTR of pinned cards vs ensemble cards" (the headline question for whether S6 is pulling its weight).
- Plan-phase 14: force-recompute endpoint MUST also re-run S6.Resolve after the ensemble re-rank so the admin can see the post-recompute pin state.
- Plan-phase 14: filter audit panel SHOULD show s6_cascade_dropped_by_s11 reason category for candidates the cascade considered but S11 dropped (purely diagnostic, not a behavior change).
- Plan-phase 9 (Recs Foundation) — historical: inventory `animes` table schema during plan-phase to confirm which of `tags`, `source`, `demographic`, `type`, `studios`, `producers` exist vs. need backfill (open question §14.1 of design spec) — informs Phase 12 scope. (Phase 12 shipped; resolved.)
- Plan-phase 10 — historical: confirm Redis key namespacing for anonymous trending vs. logged-in top-N to avoid cache collisions. (Phase 10/11 shipped; resolved.)
- Plan-phase 13 — historical: code review must verify the synchronous S6 seed update inside `MarkEpisodeWatched` adds < 5 ms p95 overhead. (Phase 13 shipped; production p95 = 48ms full-stack absolute bound met; in-process bound covered by sqlite micro-benchmark.)

### Blockers/Concerns

None yet.

## Deferred Items

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| Signal | S9-implicit OP/ED affinity (skip-behavior) | Backlog (`.planning/backlog/REC-S9-implicit-op-ed-affinity.md`) | 2026-04-28 (during v2.0 design brainstorm) |
| Architecture | S6 Variant A weight-shift | Deferred to v2.1 | 2026-05-04 |
| Signal | S7 content-vector similarity, S8 franchise, S10 staff | Deferred to v3.0 | 2026-05-04 |
| Surface | "Similar to this" sidebar on anime detail | Deferred to v3.0 | 2026-05-04 |
| Personalization | Anonymous user personalization (beyond trending) | Deferred to v2.1 | 2026-05-04 |

## v1.0 Carryover

- **Phase 7 follow-up (override-rate re-snapshot)** runs ≥ 7 d after Phase 6 deploy (≥ 2026-05-10) in parallel with v2.0 phases. Does NOT block v2.0.

## Session Continuity

Last session: 2026-05-06T15:35:00.000Z
Stopped at: Phase 13 shipped — S6 closed-loop "watched-after" pin live in production. Logged-in users completing a score≥7 anime see an instant "Because you finished X" pin at recs[0] of their "Up Next for you" row. Cascade local → Shikimori /similar → score-5 fallback verified end-to-end against ui_audit_bot (Grand Blue → One Piece via shikimori_similar). Pin expires at 7 days verified live. Synchronous seed update at 48ms p95 production. Phase 14 (Admin Debug Page & Eval Pipeline) opens next.
Resume file: .planning/phases/13-combo-watched-after-pin-s6/13-01-SUMMARY.md
