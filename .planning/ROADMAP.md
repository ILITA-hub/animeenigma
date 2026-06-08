# Roadmap: AnimeEnigma

## Milestones

- ✅ **v1.0 Smart Watch Picker Overhaul** — Phases 1-8 (shipped 2026-05-03) — see `.planning/milestones/v1.0-ROADMAP.md`
- ✅ **v2.0 Recommendations Engine** — Phases 9-14 (shipped 2026-05-07) — see `.planning/milestones/v2.0-ROADMAP.md`
- ✅ **v3.0 Universal Anime Scraper** — Phases 15-20 (shipped 2026-05-18; Phase 20 cutover landed but over-rotated — regression repaired in v3.1 Phase 24) — see `.planning/milestones/v3.0-ROADMAP.md`
- ✅ **v3.1 Scraper Self-Healing** — Phases 21-28 shipped + closed 2026-06-04 (orig 21-23 @2026-05-13 tagged `v3.1`; reopened 24-28 + `18anime` group shipped) — see `.planning/MILESTONES.md` + `.planning/milestones/v3.1-ROADMAP.md`
- ✅ **v4.0 Activity Register (ClickHouse unified event plane)** — Phases 1-6 (shipped 2026-06-08) — see `.planning/milestones/v4.0-ROADMAP.md`

## Phases

<details>
<summary>✅ v4.0 Activity Register (Phases 1-6) — SHIPPED 2026-06-08</summary>

Full phase detail archived in `.planning/milestones/v4.0-ROADMAP.md`. Audit: `.planning/milestones/v4.0-MILESTONE-AUDIT.md` (16/16 requirements satisfied; FE→BE trace_id causation join live-verified).

- [x] Phase 1: ClickHouse Foundation + EventStore Swap (3/3 plans) — completed 2026-06-05
- [x] Phase 2: BE Egress Recorder (4/4 plans) — completed 2026-06-05
- [x] Phase 3: DB/Cache Effects + Auto Operation Discovery (6/6 plans) — completed 2026-06-06
- [x] Phase 4: FE Causation + RUM (4/4 plans) — completed 2026-06-06 (FE→BE join gap closed 2026-06-08)
- [x] Phase 5: Reports & Dashboards (3/3 plans) — completed 2026-06-06
- [x] Phase 6: Consolidation → Topology A (3/3 plans) — completed 2026-06-08 (Tempo + Loki retired; ClickHouse single trace/log/event plane)

**Deferred (tracked):** Phase 1 & 6 visual Grafana render checks (`human_needed`, user-deferred); see `06-HUMAN-UAT.md`.

</details>

## Backlog / Reserved Future

Deferred out of v4.0 (archived in `.planning/milestones/v4.0-REQUIREMENTS.md` § v2 / Deferred):
- **AR-V2-01**: `AggregatingMergeTree` pre-aggregated rollups (1С-style accumulation registers) beyond what the dashboards need.
- **AR-V2-02**: Pyroscope continuous profiling (cost-by-function) integration — touched as an optional spike in Phase 5's design notes; promoted to its own deferred item.

Prior-milestone reserved ideas still on the shelf (unnumbered until committed):
- VibePlayer Recovery via WARP egress (revive VibePlayer by routing scraper egress through Cloudflare WARP; separate spec when there is appetite).
- MinIO Hot Archival (rip popular HLS streams to MinIO; serve from there to decouple from upstream availability; separate spec).

Start the next cycle with `/gsd-new-milestone`.

## Progress

| Phase | Milestone | Plans | Status | Completed |
|-------|-----------|-------|--------|-----------|
| 1-8 | v1.0 | 18/18 | ✅ Complete | 2026-04-27 → 2026-05-03 |
| 9-14 | v2.0 | 8/8 | ✅ Complete | 2026-05-06 → 2026-05-07 |
| 15-20 | v3.0 | — | ✅ Complete | 2026-05-11 → 2026-05-18 |
| 21-28 | v3.1 | — | ✅ Complete | 2026-05-13 → 2026-06-04 |
| 1-6 | v4.0 | 23/23 | ✅ Complete | 2026-06-05 → 2026-06-08 |
