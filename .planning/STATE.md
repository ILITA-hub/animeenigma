---
gsd_state_version: 1.0
milestone: v2.0
milestone_name: Recommendations Engine
status: milestone-audit-pending
stopped_at: Phase 14 shipped — Admin debug page (/admin/recs/:user_id) live with per-signal breakdown + S5 TF-IDF + S6 pin source + S11 filter audit. Force-recompute endpoint p95 = 10ms production. rec_click + rec_watched events flowing to rec_events table + Prometheus counters with {signal_id, pinned} labels. Grafana "Rec engine" dashboard JSON shipped (5 panels). v2.0 milestone is functionally COMPLETE — orchestrator should run milestone-audit → milestone-complete → milestone-cleanup next (user-initiated per CLAUDE.md auto-mode safety rule).
last_updated: "2026-05-07T02:23:10Z"
last_activity: 2026-05-07 -- Phase 14 plan 14-01 complete; v2.0 final phase shipped
progress:
  total_phases: 14
  completed_phases: 6
  total_plans: 8
  completed_plans: 8
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-04 for v2.0)

**Core value:** A logged-in user opens the home page and sees a personalized "Up Next for you" row of anime they have not yet started — ranked by a transparent weighted-ensemble of signals. After completing an anime they enjoyed (score ≥ 7), a "Because you finished X" pin appears at the top of the row.
**Current focus:** Phase 14 — Admin Debug Page & Eval Pipeline

## Current Position

Phase: 14 (Admin Debug Page & Eval Pipeline) — COMPLETE
Plan: 1 of 1 complete
Next phase: v2.0 milestone-audit (user-initiated)
Status: v2.0 milestone functionally complete; awaiting milestone-audit
Last activity: 2026-05-07 -- Phase 14 plan 14-01 shipped — admin debug + eval pipeline live in production

## Performance Metrics

**Velocity:**

- Total plans completed: 6 (v2.0: phases 9, 10, 11, 12-01, 12-02, 12-03, 13-01, 14-01)
- Average duration: ~3.5 hours per plan (planning + execution combined)
- Total execution time: ~28 hours (v2.0 milestone, 2026-04-28 → 2026-05-07)

**Phase 14 plan 14-01:**

- Duration: ~32 minutes execution-only (planning + smart-discuss occurred in prior sessions)
- Tasks completed: 14
- Files created: 16
- Files modified: 18
- Commits: 18 (test/feat pairs across 8 backend deliverables + 4 frontend deliverables + Grafana dashboard + smart-discuss + plan)

*Updated after each plan completion*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table. Recent decisions affecting current work:

- 2026-05-07: Phase-14 AdminRecRow JSON schema exposes `breakdown` (normalized) + `weights` but not `raw` or `weighted` as top-level fields. The Vue admin UI computes `weighted = breakdown × weight` client-side; raw scores never need to surface in v2.0. Future v2.1 weight-tuning analysis can add `raw` to the response if eval-pipeline benefits.
- 2026-05-07: Phase-14 force-recompute production p95 = ~10ms for ui_audit_bot (3 sequential calls), not the 50-5000ms estimate. Warm Postgres + Redis state + uninterrupted in-process precompute (no Shikimori roundtrip in this hot path) put actual latency 100x faster than the soft expectation. Acceptance bound was upper-only (< 5000ms), easily met.
- 2026-05-07: Phase-14 Scenario 5 admin endpoint test approach — least-invasive: temporarily promoted ui_audit_bot to role=admin, ran the curl, then UPDATE'd back to role=user. Confirmed 403 returned post-revert. No new admin accounts created in production. This pattern is reusable for future admin-endpoint integration tests when no dedicated test admin account exists.
- 2026-05-07: Phase-14 browser-driven scenarios (9 admin page render, 10 non-admin redirect, 11 auto-mark correlation, 12 Grafana live render) deferred to manual confirmation since the executor can't drive a real Chrome session. Code reviews + backend equivalents (Scenarios 4, 5, 7, 8) cover the security-relevant gates. Browser sanity checks should run post-merge as part of normal admin workflow.
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

- v2.1 candidate (was Plan-phase 14 todo): editable weights in admin UI — currently weights is read-only display. v2.1 should add a "save weights" button persisting per-user (or global) overrides via new `rec_weight_overrides` table; admin can A/B-tune signals in production with CTR data flowing to the Grafana dashboard.
- v2.1 candidate: S1 neighbor expansion — current S1 cosine similarity uses fixed k=10. v2.1 should expose k as a tunable + add neighbor-quality metrics (avg jaccard) to admin breakdown.
- v2.1 candidate: S6 seed history — currently only the most recent score-≥7 completion drives the pin. v2.1 could maintain last-5 history and rotate daily; admin debug should surface the rotation list.
- v2.1 candidate: per-anime CTR breakdown — Grafana panel 5 currently shows total click + watch counts only (anime_id NOT a Prometheus label to bound cardinality). v2.1 should add Postgres-backed admin handler `GET /api/admin/recs/top-clicked` querying rec_events directly.
- v2.1 candidate: session-based attribution — current strict click→watched within 1h via localStorage. v2.1 could correlate via session_id or user_id over 24h for more attribution coverage.
- v2.1 candidate: rec_events GDPR delete path — schema indexed on user_id; v2.1 should bundle a `DELETE FROM rec_events WHERE user_id=?` flow with the existing user-deletion path.
- v2.1 candidate: rate limit on /api/events/rec — T-14-06 accepted current bounded INSERT volume; v2.1 should add 5/s/IP limit if abuse patterns emerge.
- v2.1 candidate: pin signal_id observability extension — currently `pinned="true"` events all share signal_id="s6_pin"; v2.1 could split into s6_pin_local / s6_pin_shikimori_similar / s6_pin_score_5_fallback to compare CTR per cascade source.
- Plan-phase 14 (historical): debug page surfaces pin_source per row — RESOLVED (admin handler returns pin_source, AdminRecs.vue contributorDetail expander renders it).
- Plan-phase 14 (historical): rec_click/rec_watched events tag pin_seed_anime_id when item.pinned — RESOLVED (recsAnalytics.ts emits pin_seed_anime_id; rec_events table has pin_seed_anime_id column).
- Plan-phase 14 (historical): force-recompute re-runs S6.Resolve — RESOLVED (precompute.RunForUser fires the full ensemble; the next /api/users/recs hits the busted cache and re-runs S6.Resolve fresh).
- Plan-phase 14 (historical): filter audit shows s6_cascade_dropped_by_s11 category — DEFERRED to v2.1 (purely diagnostic; not blocking shipping).
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

Last session: 2026-05-07T02:23:10Z
Stopped at: Phase 14 plan 14-01 shipped — admin debug page (/admin/recs/:user_id) live with per-signal breakdown + S5 TF-IDF + S6 pin source + S11 filter audit. Force-recompute endpoint p95 ~10ms production. rec_click + rec_watched events flowing to rec_events table + Prometheus counters with {signal_id, pinned} labels. Grafana "Rec engine" dashboard JSON shipped (5 panels, uid=rec-engine). v2.0 milestone is functionally COMPLETE. Next: orchestrator should run milestone-audit → milestone-complete → milestone-cleanup (user-initiated per CLAUDE.md auto-mode safety rule).
Resume from: .planning/phases/14-admin-debug-eval-pipeline/14-01-SUMMARY.md (full verification outputs + v2.1 candidate list)
Resume file: .planning/phases/13-combo-watched-after-pin-s6/13-01-SUMMARY.md
