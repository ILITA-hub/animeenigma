---
workstream: raw-jp
milestone: v0.2
created: 2026-05-18
status: ready-for-autonomous
last_updated: 2026-05-18
last_activity: 2026-05-18 — Phase 3 (Torrent Client + Job Queue + Metrics) complete; queue + worker pool + admin REST live, Grafana dashboard auto-loaded
progress:
  total_phases: 6
  completed_phases: 3
  total_plans: 3
  completed_plans: 3
  percent: 50
---

# Project State — `raw-jp` workstream

## Project Reference

See: `PROJECT.md` (workstream-local) and `/data/animeenigma/.planning/PROJECT.md` (parent project).

**Core value:** New "RAW JP" video provider serving original Japanese audio + an "Other subs" multi-language subtitle aggregator panel.

**Current focus:** v0.1 Raw Provider MVP — ready for autonomous execution.

## Current Position

**Status:** Phases 1-3 shipped; Phase 4 (ffmpeg HLS Transcoder + MinIO Writer) is next.
**Active milestone:** v0.2 Self-Hosted Library — six phases planned
**Current phase:** Phase 4 — ffmpeg HLS Transcoder + MinIO Writer
**Last activity:** 2026-05-18 — Phase 3 shipped. Library service can now POST a magnet, claim it via FOR UPDATE SKIP LOCKED, drive it queued → downloading via embedded anacrolix/torrent, emit six Prometheus metrics, auto-load the Grafana dashboard at uid="library", and stop at status='encoding'. Phase 4 picks up at the encoding boundary.

## Source artifacts (v0.2)

- **Active milestone roadmap:** `.planning/workstreams/raw-jp/milestones/v0.2-ROADMAP.md`
- **Requirements:** `.planning/workstreams/raw-jp/milestones/v0.2-REQUIREMENTS.md`
- **Per-phase SPECs:**
  - Phase 1: `milestones/v0.2-phases/01-library-scaffold/01-SPEC.md`
  - Phase 2: `milestones/v0.2-phases/02-nyaa-animetosho-clients/02-SPEC.md`
  - Phase 3: `milestones/v0.2-phases/03-torrent-client-job-queue/03-SPEC.md`
  - Phase 4: `milestones/v0.2-phases/04-ffmpeg-minio-transcoder/04-SPEC.md`
  - Phase 5: `milestones/v0.2-phases/05-rawlibrary-admin-ui/05-SPEC.md`
  - Phase 6: `milestones/v0.2-phases/06-hybrid-resolver/06-SPEC.md`

## Source artifacts (v0.1, archived)

- **Milestone summary:** `milestones/v0.1-SUMMARY.md`
- **Roadmap (frozen):** `milestones/v0.1-ROADMAP.md`
- **Requirements (frozen):** `milestones/v0.1-REQUIREMENTS.md`
- **Followup:** `docs/issues/README.md` § ISS-012 — operator runbook for AllAnime persisted-query SHA refresh.

## Source artifacts

- **Design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
- **Workstream root:** `.planning/workstreams/raw-jp/`
- **Active milestone:** `.planning/workstreams/raw-jp/milestones/v0.1-ROADMAP.md`
- **Requirements:** `.planning/workstreams/raw-jp/milestones/v0.1-REQUIREMENTS.md`
- **Per-phase SPECs:**
  - Phase 1: `milestones/v0.1-phases/01-allanime-parser/01-SPEC.md`
  - Phase 2: `milestones/v0.1-phases/02-subtitle-aggregator/02-SPEC.md`
  - Phase 3: `milestones/v0.1-phases/03-raw-player-frontend/03-SPEC.md`
  - Phase 4: `milestones/v0.1-phases/04-frontend-wiring/04-SPEC.md`

## Progress (v0.2 active)

| Phase | Title                                            | Status      |
|-------|--------------------------------------------------|-------------|
| 1     | Library Service Scaffold                         | Done ✓      |
| 2     | Nyaa + AnimeTosho Search Clients                 | Done ✓      |
| 3     | Torrent Client + Job Queue + Metrics             | Done ✓      |
| 4     | ffmpeg HLS Transcoder + MinIO Writer             | Not started |
| 5     | RawLibrary.vue Admin UI                          | Not started |
| 6     | Hybrid Resolver                                  | Not started |

## v0.1 (shipped)

| Phase | Title                                            | Status |
|-------|--------------------------------------------------|--------|
| 1     | AllAnime Parser                                  | Done ✓ |
| 2     | Subtitle Aggregator + Extended ID Mapping        | Done ✓ |
| 3     | RawPlayer.vue + Other Subs Panel                 | Done ✓ |
| 4     | Frontend Wiring + Changelog                      | Done ✓ |

## Wave structure (for v0.2 autonomous execution)

| Wave | Phases | Parallelizable |
|------|--------|----------------|
| 1    | 1      | n/a — scaffold blocks everything |
| 2    | 2      | could overlap with 3, kept serial for simpler review |
| 3    | 3      | n/a — depends on scaffold |
| 4    | 4      | n/a — depends on 3 |
| 5    | 5, 6   | yes — UI work and hybrid resolver have zero file overlap |

## Resume / start

```
/gsd-autonomous --ws raw-jp
```

The autonomous workflow discovers phases from `milestones/v0.2-ROADMAP.md`, runs discuss→plan→execute per phase, and only pauses on grey-area decisions, blockers, or validation requests.

Step-by-step alternative:

```
/gsd-discuss-phase 1 --ws raw-jp
/gsd-plan-phase 1 --ws raw-jp
/gsd-execute-phase 1 --ws raw-jp
# repeat for phases 2, 3, 4, 5, 6
```

## Session Continuity

**Stopped At:** N/A
**Resume File:** None
