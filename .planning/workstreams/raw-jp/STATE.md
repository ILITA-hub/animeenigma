---
workstream: raw-jp
milestone: v0.2
created: 2026-05-18
status: v0.2-shipped
last_updated: 2026-05-18
last_activity: 2026-05-18 — v0.2 Self-Hosted Library shipped (6/6 phases) + milestone audit passed + archived
progress:
  total_phases: 6
  completed_phases: 6
  total_plans: 6
  completed_plans: 6
  percent: 100
---

# Project State — `raw-jp` workstream

## Project Reference

See: `PROJECT.md` (workstream-local) and `/data/animeenigma/.planning/PROJECT.md` (parent project).

**Core value:** New "RAW JP" video provider serving original Japanese audio + an "Other subs" multi-language subtitle aggregator panel (v0.1) + self-hosted library that the catalog prefers when present and falls back from gracefully (v0.2).

**Current focus:** v0.2 shipped — awaiting v0.3 planning.

## Current Position

**Status:** v0.2 Self-Hosted Library complete; all 6 phases shipped + audited.
**Active milestone:** None (between milestones).
**Next:** Start v0.3 with `/gsd-new-milestone --ws raw-jp` when ready.

## Source artifacts (v0.2, archived)

- **Milestone summary:** `milestones/v0.2-SUMMARY.md`
- **Roadmap (frozen):** `milestones/v0.2-ROADMAP.md`
- **Requirements (frozen):** `milestones/v0.2-REQUIREMENTS.md`
- **Audit:** `v0.2-MILESTONE-AUDIT.md`
- **Per-phase SPECs (reference):** `milestones/v0.2-phases/`

## Source artifacts (v0.1, archived)

- **Milestone summary:** `milestones/v0.1-SUMMARY.md`
- **Roadmap (frozen):** `milestones/v0.1-ROADMAP.md`
- **Requirements (frozen):** `milestones/v0.1-REQUIREMENTS.md`
- **Followup:** `docs/issues/README.md` § ISS-012 — operator runbook for AllAnime persisted-query SHA refresh.

## Source artifacts

- **Design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
- **Workstream root:** `.planning/workstreams/raw-jp/`

## Progress (v0.2 shipped)

| Phase | Title                                            | Status |
|-------|--------------------------------------------------|--------|
| 1     | Library Service Scaffold                         | Done ✓ |
| 2     | Nyaa + AnimeTosho Search Clients                 | Done ✓ |
| 3     | Torrent Client + Job Queue + Metrics             | Done ✓ |
| 4     | ffmpeg HLS Transcoder + MinIO Writer             | Done ✓ |
| 5     | RawLibrary.vue Admin UI                          | Done ✓ |
| 6     | Hybrid Resolver                                  | Done ✓ |

## v0.1 (shipped)

| Phase | Title                                            | Status |
|-------|--------------------------------------------------|--------|
| 1     | AllAnime Parser                                  | Done ✓ |
| 2     | Subtitle Aggregator + Extended ID Mapping        | Done ✓ |
| 3     | RawPlayer.vue + Other Subs Panel                 | Done ✓ |
| 4     | Frontend Wiring + Changelog                      | Done ✓ |

## Known followups (non-blocking)

- Port deviation 8087 → 8089 in upstream design doc + REQUIREMENTS (docs sweep).
- `ui_audit_bot` lacks admin role required for the new admin gate (Phase 5 e2e uses temp role promotion).
- v0.2 explicit followups deferred to v0.2.1 / v0.3: bulk-queue UI, storage cleanup, per-anime quality profiles, user-facing `source` chip.

## Resume / start next milestone

```
/gsd-new-milestone --ws raw-jp
```

## Session Continuity

**Stopped At:** N/A — v0.2 milestone complete.
**Resume File:** None
