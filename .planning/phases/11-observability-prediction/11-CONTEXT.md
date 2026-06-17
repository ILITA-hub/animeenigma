# Phase 11: Observability & Prediction - Context

**Gathered:** 2026-06-17
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped) — enriched from design spec §7 + scout

<domain>
## Phase Boundary

An operator can see, in Grafana, exactly how the pool is allocated, how well preloading is
working, what's being evicted/rejected/downloaded, and whether predicted demand is outrunning the
budget. FINAL phase of v4.1 — it consumes the metrics phases 8-10 already emit and adds the one
remaining backend piece (the daily storage-need prediction).

**Requirements:** OBS-01 (storage alloc/usage by Fresh/Stale + source vs budget), OBS-02 (preload
hit-rate panel), OBS-03 (eviction + rejection counts), OBS-04 (download counts by trigger),
OBS-05 (storage-need prediction table from a daily heuristic vs budget).

**Already emitted (phases 8-10 — DO NOT re-add, just chart):**
- `library_autocache_serve_total{result="hit"|"miss"}` (Phase 8)
- `library_autocache_downloads_total{trigger="A"|"B"|"backfill",result}` (Phase 9)
- `library_autocache_evicted_total{source}`, `library_autocache_rejected_total{reason}` (Phase 10)
- `library_autocache_bytes_used{source,freshness}`, `library_autocache_budget_bytes`,
  `library_autocache_episodes{source,freshness}` (Phase 10, refreshed on the evictor Sweep).

**The one NEW backend piece:** the daily prediction gauge `library_autocache_predicted_bytes{component="ongoing"|"nextep"}`.
</domain>

<decisions>
## Locked Decisions (design spec §7)

- **Grafana dashboard:** extend the EXISTING `infra/grafana/dashboards/library.json` (377 lines, 7
  panels today: job status, torrents, throughput, disk free, enqueue rejects, job-status mix). Add
  an "Autocache Pool" section/row of panels for OBS-01..05. File-provisioned JSON (no API). Keep
  the existing panels intact; APPEND the new ones (unique panel `id`s + `gridPos`).
- **OBS-01 (storage):** stacked usage `library_autocache_bytes_used{source,freshness}` vs
  `library_autocache_budget_bytes` (timeseries or bar gauge); + `library_autocache_episodes{...}`.
- **OBS-02 (preload hit-rate):** `rate(...serve_total{result="hit"}) / rate(...serve_total)` as a
  cache-hit % stat/gauge panel.
- **OBS-03:** `library_autocache_evicted_total{source}` + `library_autocache_rejected_total{reason}`
  (increase over window).
- **OBS-04:** `library_autocache_downloads_total{trigger,result}` (by trigger A/B/backfill).
- **OBS-05 (prediction table):** a Grafana TABLE panel rendering
  `library_autocache_predicted_bytes{component}` (ongoing + nextep + total) compared against
  `library_autocache_budget_bytes`. Backed by the NEW daily job below.

- **Prediction job ownership:** the prediction needs SHARED-DB watcher/combo counts (same data as
  Logic A) → it lives in the **scheduler** service (NOT library — separate DB). A daily scheduler
  cron job computes the §7 v1 HEURISTIC:
  - `predicted_bytes{component="ongoing"}` = (count of ongoing anime with ≥1 active JP-audio
    watcher — reuse the Logic A enumeration) × `avg_raw_ep_size`.
  - `predicted_bytes{component="nextep"}` = (count of DISTINCT anime with an active JP-audio
    watching watcher in the last `active_watcher_days`=30) × `avg_raw_ep_size`.
  - Emit as a `prometheus.GaugeVec` on the scheduler `/metrics` (Grafana scrapes all services, so
    a scheduler-exposed `library_autocache_predicted_bytes{component}` is queryable alongside the
    library-exposed `library_autocache_budget_bytes`). `avg_raw_ep_size` = the same const used in
    Phase 10 (mirror via the scheduler env, default ~1.2 GiB — `AUTOCACHE_AVG_RAW_EP_BYTES`).
  - The heuristic is intentionally COARSE for v1 (spec §7). AI prediction supersedes it (v2 TODO).

### Claude's Discretion
- Daily cadence exact cron (`@daily` / `0 4 * * *`); whether the prediction job reuses the Logic A
  job's join helper or its own query; exact Grafana panel `gridPos`/types; whether to also add a
  per-ongoing-anime breakdown row to the OBS-05 table (the design says "per-ongoing rows + total" —
  if cheap via a labeled gauge it's nicer, but a coarse ongoing/nextep/total table satisfies OBS-05;
  prefer the coarse version for v1 to avoid high-cardinality per-anime gauge labels).
</decisions>

<code_context>
## Existing Code Insights
- `infra/grafana/dashboards/library.json` — the dashboard to extend (file-provisioned; 7 panels;
  Prometheus datasource). Mirror an existing panel's JSON shape for the new ones.
- `services/scheduler/internal/jobs/autocache_logic_a.go` (Phase 9) — the Logic A enumeration +
  the robfig/cron registration + `SchedulerJob{ExecutionsTotal,Duration,LastSuccess}` metrics wrap.
  The prediction job mirrors this (registration, nil-guard, metrics wrap) and can reuse/adapt the
  ongoing-with-active-JP-watcher join for the "ongoing" count + a distinct-anime count for "nextep".
- `services/scheduler/internal/config/config.go` — `AUTOCACHE_ACTIVE_WATCHER_DAYS` mirror added in
  Phase 9; add `AUTOCACHE_AVG_RAW_EP_BYTES` (default ~1.2 GiB) similarly.
- `services/scheduler/internal/service/job.go` — robfig/cron `AddFunc` harness + `GetStatus`.
- How scheduler exposes `/metrics` (the SchedulerJob metrics already register there) — add the new
  `predicted_bytes{component}` GaugeVec to the same registry.

## Pitfalls
- Metric name: expose `library_autocache_predicted_bytes` from scheduler so OBS-05's table can join
  it with the library-exposed `budget_bytes` in one Prometheus query/table.
- Keep gauge cardinality low: `{component}` only (2 series); do NOT label per-anime.
- Grafana JSON: unique panel ids + non-overlapping gridPos; valid datasource uid; don't break the
  existing 7 panels. This is config — vitest/jsdom can't validate it; a JSON-parse + schema sanity
  check is the practical gate (no live Grafana smoke required unless the owner asks).
- This is NOT a Vue frontend phase — no UI-SPEC / DS-lint applies (Grafana JSON only).
</code_context>

<specifics>
## Specific Ideas
- scheduler: `AutocachePredictionJob` (daily cron) → counts (ongoing-with-active-JP-watcher,
  distinct-active-JP-watching-anime) × `avgRawEpBytes` → set `predicted_bytes{component}` gauge.
- scheduler config: `AUTOCACHE_AVG_RAW_EP_BYTES` (default 1288490188 ≈ 1.2 GiB).
- grafana: append an "Autocache Pool" row + panels for OBS-01..05 to library.json (stacked
  bytes_used vs budget; episodes; hit-rate %; evicted/rejected; downloads by trigger; prediction
  table predicted_bytes{component} + budget).
</specifics>

<deferred>
## Deferred Ideas
- AI-prediction model (replaces the coarse heuristic) → v2.
- Per-anime prediction breakdown (high-cardinality) → v2 if needed.
- Live Grafana render smoke → opt-in only (owner request).
</deferred>
