---
gsd_state_version: 1.0
workstream: notifications
milestone: v1.0
milestone_name: "Notifications Engine"
created: 2026-05-20
status: planning
last_updated: "2026-05-21"
last_activity: "2026-05-21 — Phase 2 SHIPPED. 5 atomic commits merged. Catalog has new /internal/anime/{id}/episodes (5min Redis cache, kodik+animelib only; English deferred to v1.0.x). NewEpisodeDetectorJob (hourly cron ±5min jitter) with bootstrap protection, dedupe-key UPSERT, failure isolation, idempotency, aggregation. Daily cleanup at 03:30. Admin /internal/detector/run-once + /internal/cleanup/run-once for deterministic verify. 6 Prometheus metrics. 5 unit tests including Test_Detector_BootstrapProtection. 7/7 SC PASS. All 10 services healthy. Two auto-fixes in verification (catalog envelope unwrap, Postgres INTERVAL syntax). Phase 3 (frontend) unblocked — real notifications now materialize hourly."
progress:
  total_phases: 3
  completed_phases: 2
  total_plans: 2
  completed_plans: 2
  percent: 67
---

# Project State — `notifications` workstream

## Project Reference

See: `PROJECT.md` (workstream-local) and `/data/animeenigma/.planning/PROJECT.md` (parent project).

**Core value:** A user with ongoing shows in `anime_list.status = 'watching'` sees a bell badge + a toast (if foregrounded) within ~1 hour of a new episode landing on the same player/translation they were already watching with. Click → straight to the right episode. Engine generic enough that adding a second concrete notification type in v1.1 needs only a payload schema + a frontend renderer component.

**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-11-notifications-engine-design.md`

## Current Position

Phase: Phase 2 (Detector + Catalog Endpoint) — COMPLETE ✓
Plan: `phases/02-detector-and-catalog-endpoint/02-PLAN.md` (executed)
Status: Phases 1 + 2 shipped; Phase 3 (frontend bell + dropdown + toast) ready to plan
Last activity: 2026-05-21 — Phase 2 merged to main; 7/7 SC pass; 02-SUMMARY.md written

## Resume / Continuity

**Resume file:** none — Phases 1 + 2 done, awaiting Phase 3 plan generation.
**Next command:** `/gsd-plan-phase 3 --ws notifications` then `/gsd-ui-phase 3 --ws notifications` (frontend phase — UI-SPEC generation) then `/gsd-execute-phase 3 --ws notifications`.

## Workstream Phase Map

| Phase | Name                                                                          | Requirements                                            | Status |
|-------|-------------------------------------------------------------------------------|---------------------------------------------------------|--------|
| 1     | Notifications Service Foundation                                              | NOTIF-FOUND-01..08, NOTIF-NF-04 (partial)               | **complete ✓** |
| 2     | Catalog Internal Endpoint + Episode Detector + Cleanup                        | NOTIF-DET-01..10, NOTIF-NF-01, NOTIF-NF-02              | **complete ✓** |
| 3     | Frontend — Bell + Dropdown + Toast + Pinia Store + Renderer Registry + i18n   | NOTIF-UI-01..08, NOTIF-NF-03                            | pending |

## Operator Next Steps

1. **Review** `phases/01-notifications-foundation/PLAN.md` — check the 5 decisions (D-01..D-05), 7 risks, and 6 verification commands.
2. `/gsd-execute-phase 1 --ws notifications` — execute Phase 1 (atomic commits + state tracking).
3. `/gsd-plan-phase 2 --ws notifications` — plan Phase 2 (detector + catalog endpoint) once Phase 1 ships.
4. `/gsd-plan-phase 3 --ws notifications` — plan Phase 3 (frontend) once Phase 2 ships.
5. `/gsd-audit-milestone --ws notifications` after Phase 3 ships.
6. `/gsd-complete-milestone v1.0 --ws notifications` to archive.
