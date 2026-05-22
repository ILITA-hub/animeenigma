# Roadmap: AnimeEnigma `hero-spotlight` workstream

**Workstream:** hero-spotlight (parallel to `notifications`, `raw-jp`, `social`, `ui-ux-audit`)
**Active milestone:** None (v1.0 shipped 2026-05-21)
**Phase numbering:** Workstream-local — restarts at 1 inside each milestone.

## Milestones

- ✅ **v1.0 HeroSpotlightBlock** — Shipped 2026-05-21 (3 phases, 45/45 requirements). See [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md).
- ⏳ **v1.1 Personalization & Polish** — Conditional on 1-2 weeks usage data (slide-order personalization, per-user opt-outs, optional editorial-pick admin form, WebSocket-driven `now_watching`, feature-flag cleanup).

## Workstream-local conventions

- Every GSD command for this workstream MUST pass `--ws hero-spotlight`. Per
  `feedback_workstream_parallelism`, the active marker is local-only and not
  enforced by tooling.
- Metrics per project convention (`.planning/CONVENTIONS.md`): every plan
  scored on UXΔ / CDI / MVQ; no days/hours/sprints.
- Each phase's PLAN.md scored independently; the workstream-level metric is
  in the design doc Section 10.
