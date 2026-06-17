# Roadmap: AnimeEnigma

## Milestones

- ✅ **v1.0 Smart Watch Picker Overhaul** — Phases 1-8 (shipped 2026-05-03) — see `.planning/milestones/v1.0-ROADMAP.md`
- ✅ **v2.0 Recommendations Engine** — Phases 9-14 (shipped 2026-05-07) — see `.planning/milestones/v2.0-ROADMAP.md`
- ✅ **v3.0 Universal Anime Scraper** — Phases 15-20 (shipped 2026-05-18; Phase 20 cutover landed but over-rotated — regression repaired in v3.1 Phase 24) — see `.planning/milestones/v3.0-ROADMAP.md`
- ✅ **v3.1 Scraper Self-Healing** — Phases 21-28 shipped + closed 2026-06-04 (orig 21-23 @2026-05-13 tagged `v3.1`; reopened 24-28 + `18anime` group shipped) — see `.planning/MILESTONES.md` + `.planning/milestones/v3.1-ROADMAP.md`
- ✅ **v4.0 Activity Register (ClickHouse unified event plane)** — Phases 1-6 (shipped 2026-06-08) — see `.planning/milestones/v4.0-ROADMAP.md`
- 🟡 **v4.1 Auto Torrent Population (watch-driven first-party RAW cache)** — Phases 7-11 (started 2026-06-17) — design: `docs/superpowers/specs/2026-06-17-auto-torrent-population-design.md`

## Phases

**Phase Numbering:**
- v4.1 **continues** the v4.0 numbering — v4.0 ended at Phase 6, so v4.1 runs **7–11** (it does NOT reset to 1).
- Decimal phases (e.g., 7.1) are reserved for urgent insertions that must execute between integer phases.

**Milestone goal:** Autonomously predict and pre-download the RAW (JP-audio) episodes users are about to watch into a self-evicting ~100 GB first-party metered pool, so the "ae" provider is already populated when they hit play — with zero admin action. Built into `services/library` (new `internal/autocache/` subsystem) reusing the existing torrent → HLS → MinIO pipeline; no new microservice.

- [ ] **Phase 7: Pool Foundation, Config & Migration** — Unified `aeProvider/<MALID>/RAW/<ep>/` layout, the `library_episodes` accounting ledger (source/track/downloaded_at/last_fetch_at/fetch_count/size_bytes), the admin-editable DB-backed config + master kill-switch, and the one-time migration of existing admin content into the metered pool.
- [ ] **Phase 8: Serving & Fetch Signal** — The "ae" hit/miss serving path against the new pool: present → serve + bump `last_fetch_at`/`fetch_count` + count a preload hit; absent → fail over with no regression + count a miss + fire a backfill demand.
- [ ] **Phase 9: Download Triggers** — Logic A (ongoing push), Logic B (next-episode pull), backfill-on-miss, single-flight dedup, and RAW-only ≤quality_cap / ≥min_seeders release selection — all gated on active JP-audio-combo watchers.
- [ ] **Phase 10: Eviction & Budget** — One evictor over the whole `aeProvider/` pool: Fresh/Stale source-specific classification, source-ranked ordered eviction, reject-when-full, and co-existence with the physical `DiskGuard`.
- [ ] **Phase 11: Observability & Prediction** — The Grafana library dashboard (storage allocation by Fresh/Stale + source, preload hit-rate, eviction/rejection counts, downloads by trigger) plus the daily storage-need prediction-table heuristic job.

## Phase Details

### Phase 7: Pool Foundation, Config & Migration
**Goal**: Every first-party RAW object (admin + auto) lives under one metered layout with a per-row accounting ledger, governed by live-editable config and a master switch — and all pre-existing admin content has been migrated into that pool without interrupting playback.
**Depends on**: Nothing (first v4.1 phase; brownfield extension of `services/library`)
**Requirements**: POOL-01, POOL-02, POOL-03, POOL-04, POOL-05
**Success Criteria** (what must be TRUE):
  1. A newly-ingested RAW episode is written to and resolvable from `aeProvider/<MALID>/RAW/<episode>/playlist.m3u8`; the SUB/DUB branches exist in the schema but are never written. (POOL-01)
  2. Every previously admin-ingested episode now resolves from the new `aeProvider/<MALID>/RAW/<ep>/` layout with its `minio_path` repointed (copy → repoint → delete), and playback of those episodes never broke during or after the migration. (POOL-02)
  3. Querying `library_episodes` returns `source`, `track`, `downloaded_at`, `last_fetch_at`, `fetch_count`, and `size_bytes` for every pool object, so a single accountant can classify and sum the pool. (POOL-03)
  4. An admin can `GET` the autocache config and `PATCH` any field (budget, freshness windows, active-watcher window, quality cap, min seeders, sweep interval) and the new value takes effect with no redeploy. (POOL-04)
  5. Flipping the master `enabled` switch off halts all autocache downloading and eviction; flipping it on resumes them — observable in behavior, not just stored. (POOL-05)
**Plans**: 3 plans (3 waves)
  - [x] 07-01-PLAN.md — Schema + ledger columns (migration 005), extended Episode model, aeProvider layout helper, write-site repoints (POOL-01, POOL-03)
  - [x] 07-02-PLAN.md — autocache_config singleton table (migration 006), Get/Patch accessor, admin GET/PATCH /api/library/autocache/config, master enabled switch (POOL-04, POOL-05)
  - [x] 07-03-PLAN.md — One-time admin-content migration (Move→repoint), repo helpers, boot wiring, catalog ae-resolver audit (POOL-02)

### Phase 8: Serving & Fetch Signal
**Goal**: When the player resolves the "ae" provider, a present episode serves from the new pool and records the "viewed by any user" fetch signal; an absent episode fails over cleanly and self-heals for next time.
**Depends on**: Phase 7 (pool layout + `last_fetch_at`/`fetch_count` ledger must exist before the serve path can write them)
**Requirements**: SERVE-01, SERVE-02, SERVE-03
**Success Criteria** (what must be TRUE):
  1. When the episode is present in the pool, the "ae" provider serves `aeProvider/<mal>/RAW/<ep>/playlist.m3u8` and the playback is counted as a preload hit. (SERVE-01)
  2. Each "ae" playback updates that episode's `last_fetch_at` to now and increments `fetch_count` — the shared freshness + popularity signal consumed later by eviction. (SERVE-02)
  3. When the episode is absent, the player fails over to the existing providers with no regression versus today, the event is counted as a preload miss, and a backfill demand is emitted for that episode. (SERVE-03)
**Plans**: 3 plans (3 waves)
  - [x] 08-01-PLAN.md — Library data + metrics: migration 007 autocache_demand, AutocacheDemand model + DemandRepository.Record, EpisodeRepository.BumpFetch, library_autocache_serve_total counter (SERVE-01/02/03)
  - [x] 08-02-PLAN.md — Library /internal/library/autocache/{fetch,demand} handlers + enabled-gating + router mount + main.go DI (apply 007) (SERVE-01/02/03)
  - [x] 08-03-PLAN.md — Catalog fire-and-forget hit/miss: client RecordFetch/RecordDemand + GetLibraryStream wiring, failover unchanged (SERVE-01/02/03)

### Phase 9: Download Triggers
**Goal**: The platform autonomously enqueues the right RAW downloads — pushing newly-aired episodes of watched ongoings, pulling the next episode ahead of an active watcher, and backfilling on a miss — without ever duplicating work or downloading the wrong track.
**Depends on**: Phase 7 (pool + config + ledger), Phase 8 (the backfill demand seam fired on a serve miss)
**Requirements**: TRIG-01, TRIG-02, TRIG-03, TRIG-04, TRIG-05
**Success Criteria** (what must be TRUE):
  1. For an ongoing anime with ≥1 active JP-audio-combo watcher (list status `watching` AND progress within `active_watcher_days`), the system downloads each newly-aired episode the next planner sweep after a ≤`quality_cap` / ≥`min_seeders` release appears on the indexers. (TRIG-01)
  2. When an active JP-audio-combo user begins watching episode N, episode N+1 (if aired) is downloaded ahead of time. (TRIG-02)
  3. A cache miss on the "ae" provider results in a backfill download of that episode, so a subsequent request for it hits. (TRIG-03)
  4. Concurrent demand for the same `(mal_id, episode)` collapses to a single download job, and an already-present episode enqueues no job. (TRIG-04)
  5. Only RAW releases at or below `quality_cap` (1080p) and at or above `min_seeders` are selected; DUB-preferring demand triggers no download. (TRIG-05)
**Plans**: 4 plans (3 waves)
  - [x] 09-01-PLAN.md — Library schema + repo + metric foundation: migrations 008 (autocache source) / 009 (library_jobs.episode) / 010 (ongoing reason), Job.Episode + DemandReasonOngoing, HasActiveForEpisode + Drain/Delete, downloads_total counter (TRIG-03/04/05)
  - [x] 09-02-PLAN.md — Library Planner drain loop (config-gated ticker, present + in-flight dedup, RAW/quality/seeder filter, source=autocache enqueue) + migrations applied + Planner DI in main.go (TRIG-03/04/05)
  - [x] 09-03-PLAN.md — Player Logic B: fire-and-forget DemandProducer + UpdateProgress next_ep(N+1) fire for JP-audio active watchers + config/DI (TRIG-02)
  - [x] 09-04-PLAN.md — Scheduler Logic A: cron job adapting the hotcombos join (JP-audio + D8 recency + episodes_aired) firing ongoing demand per ongoing anime + cron registration + config/DI (TRIG-01)

### Phase 10: Eviction & Budget
**Goal**: The whole first-party pool stays self-managing under one budget — Fresh content is protected, Stale content is evicted in a fair source-ranked order, and an unfittable download is cleanly rejected rather than blowing the budget.
**Depends on**: Phase 7 (migration must be complete so admin rows are in the metered pool and visible to the accountant — otherwise budget math is wrong), Phase 8 (fetch signal feeds Fresh/Stale classification), Phase 9 (downloads are what consume budget and trigger pre-admit eviction)
**Requirements**: EVICT-01, EVICT-02, EVICT-03, EVICT-04, EVICT-05
**Success Criteria** (what must be TRUE):
  1. The total bytes of the `aeProvider/` pool (admin + auto combined) stay bounded by the configurable budget (default 100 GB). (EVICT-01)
  2. Each episode is correctly classified Fresh or Stale by its source-specific windows — auto: `<auto_fresh_download_days` since download OR `<auto_fresh_fetch_days` since last fetch; admin: `<admin_fresh_days` since upload OR last fetch. (EVICT-02)
  3. When space is needed, only Stale episodes are evicted, in the order auto-never-fetched → auto-fetched → admin-never-fetched → admin-fetched (oldest-first within each group), and a Fresh episode is never deleted. (EVICT-03)
  4. If draining the entire Stale queue still cannot fit a new download (including an admin upload), the download is rejected and `library_autocache_rejected_total{reason="budget_full"}` increments. (EVICT-04)
  5. A download proceeds only when BOTH the logical budget and the existing physical-disk `DiskGuard` pass. (EVICT-05)
**Plans**: 3 plans (3 waves)
  - [x] 10-01-PLAN.md — Primitives: repo SumPoolBytes/ListStaleEvictionCandidates/DeleteByID + minio DeletePrefix + 2 eviction/rejection counters + 3 byte/episode gauges (EVICT-01/02/03/04)
  - [x] 10-02-PLAN.md — autocache.Evictor: EnsureRoom budget arithmetic + 4-tier ordered Stale eviction + Classify + periodic Sweep + Accountant gauge publishing, one-mutex serialized (EVICT-01/02/03/04)
  - [x] 10-03-PLAN.md — Pre-admit wiring: Planner enqueue gate (leave-demand + backoff on reject) + admin upload gate (HTTP 507) layered on DiskGuard + main.go construct/Start/inject/Stop (EVICT-04/05)

### Phase 11: Observability & Prediction
**Goal**: An operator can see, in Grafana, exactly how the pool is allocated, how well preloading is working, what's being evicted/rejected/downloaded, and whether predicted demand is outrunning the budget.
**Depends on**: Phase 7 (config/budget gauge), Phase 8 (serve hit/miss counters), Phase 9 (download counters by trigger), Phase 10 (eviction/rejection counters + Fresh/Stale + byte accounting). Counters/gauges are emitted by each owning phase's code; this phase builds the dashboard and the daily prediction job on top of them.
**Requirements**: OBS-01, OBS-02, OBS-03, OBS-04, OBS-05
**Success Criteria** (what must be TRUE):
  1. Grafana shows pool storage allocation and usage split by Fresh/Stale and by source (admin/auto) against the budget cap. (OBS-01)
  2. Grafana shows preload hit-rate (hit vs miss) as a cache-hit-style panel. (OBS-02)
  3. Grafana shows eviction counts by source and budget-full rejection counts. (OBS-03)
  4. Grafana shows autocache download counts by trigger (A / B / backfill) and result. (OBS-04)
  5. Grafana renders a storage-need prediction table from a daily heuristic (ongoing + next-episode components) compared against the budget. (OBS-05)
**Plans**: 2 plans (2 waves)
  - [x] 11-01-PLAN.md — Scheduler daily prediction job: AutocachePredictedBytes GaugeVec + config (cron + avg-ep-bytes int64 env) + AutocachePredictionJob (two DISTINCT-join counts x avgRawEpBytes) + cron registration + DI (OBS-05 backend)
  - [x] 11-02-PLAN.md — Grafana: append 6 Autocache Pool panels (ids 8..13) to library.json — storage vs budget, episodes, preload hit-rate %, eviction/rejection, downloads by trigger, prediction table (OBS-01..05)

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

Deferred out of v4.1 (tracked in `.planning/REQUIREMENTS.md` § v2):
- **PRED-01**: AI-prediction-driven prefetch — learned per-user next-watch probability model replacing/augmenting the predefined Logic A/B heuristics.
- **ACQ-01**: 2160p+ acquisition and/or an upscaling stage (raise `quality_cap` beyond 1080p).
- **TRACK-01**: Populate the `SUB/<ep>` track (e.g. hardsubbed raws) where JP-video + client-overlay is insufficient.
- **TRACK-02**: Populate the `DUB/<team-or-provider>/<ep>` track from release-group-tagged torrents.

Deferred out of v4.0 (archived in `.planning/milestones/v4.0-REQUIREMENTS.md` § v2 / Deferred):
- **AR-V2-01**: `AggregatingMergeTree` pre-aggregated rollups (1С-style accumulation registers) beyond what the dashboards need.
- **AR-V2-02**: Pyroscope continuous profiling (cost-by-function) integration.

Prior-milestone reserved ideas still on the shelf (unnumbered until committed):
- VibePlayer Recovery via WARP egress (revive VibePlayer by routing scraper egress through Cloudflare WARP; separate spec when there is appetite).
- MinIO Hot Archival (rip popular HLS streams to MinIO; serve from there to decouple from upstream availability; separate spec).

## Progress

| Phase | Milestone | Plans | Status | Completed |
|-------|-----------|-------|--------|-----------|
| 1-8 | v1.0 | 18/18 | ✅ Complete | 2026-04-27 → 2026-05-03 |
| 9-14 | v2.0 | 8/8 | ✅ Complete | 2026-05-06 → 2026-05-07 |
| 15-20 | v3.0 | — | ✅ Complete | 2026-05-11 → 2026-05-18 |
| 21-28 | v3.1 | — | ✅ Complete | 2026-05-13 → 2026-06-04 |
| 1-6 | v4.0 | 23/23 | ✅ Complete | 2026-06-05 → 2026-06-08 |
| 7. Pool Foundation, Config & Migration | v4.1 | 3/3 | Complete    | 2026-06-17 |
| 8. Serving & Fetch Signal | v4.1 | 3/3 | Complete    | 2026-06-17 |
| 9. Download Triggers | v4.1 | 4/4 | Complete    | 2026-06-17 |
| 10. Eviction & Budget | v4.1 | 3/3 | Complete    | 2026-06-17 |
| 11. Observability & Prediction | v4.1 | 2/2 | Complete    | 2026-06-17 |
</content>
</invoke>
