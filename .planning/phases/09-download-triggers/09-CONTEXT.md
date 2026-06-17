# Phase 9: Download Triggers - Context

**Gathered:** 2026-06-17
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped) — enriched from the design spec + 09-RESEARCH.md (HIGH confidence)

<domain>
## Phase Boundary

Autonomously enqueue the right RAW downloads — **Logic A** (ongoing push: newly-aired episodes
of watched ongoings), **Logic B** (next-episode pull ahead of an active watcher), and
**backfill-on-miss** — with single-flight dedup and RAW-only ≤1080p / ≥3-seeders gating.

**Requirements:** TRIG-01 (Logic A), TRIG-02 (Logic B), TRIG-03 (backfill drain), TRIG-04
(single-flight dedup), TRIG-05 (RAW ≤quality_cap / ≥min_seeders selection; DUB never triggers).

**Out of scope (later phases):** the evictor / budget enforcement (Phase 10 — the Planner may
enqueue past budget in this phase; Phase 10 adds the pre-admit eviction/reject gate), Grafana
panels (Phase 11 — but Phase 9 MUST emit `library_autocache_downloads_total{trigger,result}` so
P11 can chart it).
</domain>

<decisions>
## Locked Decisions (from 09-RESEARCH.md + design spec — confidence HIGH)

**LOAD-BEARING CONSTRAINT:** the `library` service runs on a SEPARATE Postgres DB
(`LIBRARY_DB_NAME`, default `library`); `catalog`/`player`/`notifications`/`scheduler` share the
`animeenigma` DB. The library Planner therefore CANNOT join `watch_history × anime_list × animes`.
**All "who wants what" enumeration lives in shared-DB producers; the library only ever sees
`autocache_demand` rows** (shipped Phase 8) via the `POST /internal/library/autocache/demand`
endpoint. This is the architecture: **producers (player + scheduler) → demand → library Planner
drains → downloads.**

1. **JP-audio determination** = combo `Player ∈ {ae, raw}` OR `Language == "ja"` (NOT kodik/animelib=ru,
   english=en). Combo type is `(Player, Language, WatchType)` (`player/internal/domain/preference.go`).
2. **Logic B producer (TRIG-02):** fire a `next_ep` demand for episode **N+1** from
   `player UpdateProgress` (`progress.go` — the heartbeat / "first progress event for N", per spec
   §4 which says fire on first progress of N for max lead time — NOT from MarkEpisodeWatched which
   is too late), WHEN the resolved combo is JP-audio AND the anime is status=watching. Fire-and-
   forget, non-blocking, drop-on-failure — mirror the existing `RecsHintProducer` pattern. Reuse
   the catalog→library demand client OR add a player→library demand call to the Phase-8 endpoint.
3. **Logic A producer (TRIG-01):** a NEW **scheduler** cron job (scheduler shares the DB + owns the
   `robfig/cron` harness). Every `sweep_interval_min` (default 20; read from autocache_config or a
   scheduler env mirror), enumerate ongoing anime (`animes` ongoing status + `episodes_aired`) that
   have ≥1 **active JP-audio watcher** (status=watching AND watch progress within
   `active_watcher_days`=30 — D8), and fire a demand for the latest-aired episode of each. The join
   already exists in `notifications/internal/job/hotcombos.go` — adapt it (add the D8 recency
   predicate + JP-audio filter + `episodes_aired` join). "As soon as on torrents" = the library
   Planner retries the search each loop until a release appears; the producer just re-asserts demand.
4. **Library Planner / drainer (TRIG-03/04/05):** a periodic loop in the library service that drains
   `autocache_demand` (both `next_ep` and `backfill` reasons), honoring `enabled` + `sweep_interval_min`
   from `autocache_config`. For each `(mal_id, episode)`:
   - **Dedup (TRIG-04, single-flight):** skip if the episode is already present in the pool
     (`EpisodeRepository.GetByShikimoriEpisode` row exists) OR a non-terminal `library_jobs` row
     already targets that `(mal_id, episode)`.
   - **Search (TRIG-05):** `TieredSearcher.FetchAll` (Jackett→Nyaa→AnimeTosho, seeder-ranked),
     filter to RAW + ≤`quality_cap`(1080) + ≥`min_seeders`(3). RAW has NO release-type field in
     `domain.Release` → infer via the heuristic below.
   - **Enqueue:** `jobRepo.Create` a `library_jobs` row with `source=autocache`,
     `episode=<intended ep>`, then the existing WorkerPool→EncoderPool→MinIO pipeline runs; the
     downloaded file's real episode is filename-detected post-download (existing behavior).
   - On enqueue/success, delete/mark the demand row; on no-release-found, LEAVE the demand row so
     the next loop retries (Logic A "as soon as on torrents"). Bound retries / rate-limit searches
     to avoid a thundering herd.
5. **Two schema migrations (required):** migration 008 — add `autocache` to the `job_source` enum
   (copy `004_jackett_source.sql` idempotent pattern). Migration 009 — add `library_jobs.episode INT`
   (nullable; the intended episode from the demand) for single-flight dedup + OBS-04 trigger
   attribution. Mirror both in the GORM `Job` domain struct.
6. **RAW / quality / seeder heuristic (TRIG-05):** RAW = known RAW-uploader allowlist
   (Ohys-Raws / SubsPlease / Erai-raws — already in `library_filename_patterns`) AND/OR negative
   token filter (reject `dub|dual-audio|hardsub`), AND `quality ≤ 1080p`, AND `seeders ≥ 3`. DUB
   demand never reaches the library (producers only fire for JP-audio combos), so the heuristic is a
   safety net, not the primary gate.
7. **Metric:** emit `library_autocache_downloads_total{trigger="A"|"B"|"backfill", result}` from the
   Planner (trigger derived from the demand `reason`: next_ep→B, backfill→backfill; Logic-A-originated
   next-latest demands → A — see note). Low cardinality.

### Trigger attribution note
Logic A and Logic B both produce demand; A re-asserts the latest-aired ep, B asserts N+1. To
attribute `downloads_total{trigger}`, either (a) extend the demand `reason` enum with `ongoing`
(Logic A) distinct from `next_ep` (Logic B), or (b) keep reason={next_ep,backfill} and let Logic A
also use `next_ep`. **Recommend (a): add `ongoing` to the reason enum** so OBS-04 can distinguish
A vs B. Confirm in planning; if simpler, fold A into next_ep and label the metric by producer.

### Claude's Discretion
Exact loop cadence/backoff, search rate-limiting, demand-row lifecycle (delete-on-enqueue vs
mark-claimed), whether the player Logic-B call reuses the catalog→library client or a new player
client, and the precise scheduler env wiring — all at Claude's discretion guided by RESEARCH.md.
</decisions>

<code_context>
## Existing Code Insights (from 09-RESEARCH.md — file:line evidence)

- **Phase 8 demand seam (drain source):** `autocache_demand` table + `DemandRepository.Record`
  upsert + `POST /internal/library/autocache/demand`. The Planner reads this table directly (same
  library DB).
- **Player combo:** `services/player/internal/domain/preference.go` (combo = Player/Language/WatchType);
  `services/player/internal/service/resolver.go` + `preference.go` resolve the per-anime combo;
  `services/player/internal/service/progress.go` `UpdateProgress` (Logic B fire point);
  `list.go:430` `MarkEpisodeWatched` fires `RecsHintProducer` (the fire-and-forget mirror to copy).
- **Logic A join:** `services/notifications/internal/job/hotcombos.go:46-60` (ongoing × active-combo
  join to adapt); `services/catalog/internal/repo/anime.go` (`episodes_aired`, ongoing status,
  `next_episode_at`).
- **Scheduler harness:** `services/scheduler/internal/service/job.go:43` (robfig/cron registration);
  `services/scheduler/internal/jobs/{read_threshold.go,shikimori.go}` (cron-job pattern to mirror).
- **Library Planner reuse:** `services/library/internal/service/jackett_tier.go:61` `TieredSearcher.FetchAll`
  (seeder-ranked); `services/library/internal/service/search.go`; `download_worker.go` (WorkerPool);
  `services/library/internal/repo/episode.go:48` `GetByShikimoriEpisode` (present-check);
  `services/library/internal/handler/jobs.go:158` → `jobRepo.Create` (enqueue path);
  `services/library/internal/domain/job.go` (`JobSource` enum — add `autocache`; add `Episode`).
- **Migration pattern:** `services/library/migrations/004_jackett_source.sql` (enum add, idempotent)
  → copy for 008/009. Register in `migrations.go` + apply in `main.go` (the Phase 7/8 3-touch flow).
- **Metric pattern:** `services/library/internal/metrics/library_metrics.go` (CounterVec — clone the
  `serve_total{result}` shape from Phase 8 for `downloads_total{trigger,result}`).

## Pitfalls (from research)
- Episode number is NOT in the magnet — it's filename-detected post-download (`encoder_worker.go:255`).
  Dedup/enqueue key on the INTENDED episode from the demand row (`library_jobs.episode`); the final
  placement self-corrects via filename detection. Accept rare mislabel for v1.
- catalog/player `WriteTimeout` (~30s) — keep producer internal calls fast/non-blocking.
- Thundering herd: Logic A re-asserts demand every sweep; the Planner must rate-limit Jackett
  searches (the aggregated query is ~20s) and not search the same un-findable ep on every tick.
- Scheduler cron registration must follow the existing job-registration pattern.
</code_context>

<specifics>
## Specific Ideas
- library: migrations 008 (`autocache` job_source) + 009 (`library_jobs.episode`); `Planner` service
  (drain loop) reusing `TieredSearcher.FetchAll` + `GetByShikimoriEpisode` + `jobRepo.Create`;
  `downloads_total{trigger,result}` counter; RAW/quality/seeder filter; single-flight via in-flight
  job check; demand reason enum gains `ongoing` (Logic A).
- player: Logic B producer in `UpdateProgress` — resolve combo, if JP-audio + watching, fire
  `next_ep` demand(N+1) to library internal (fire-and-forget, drop-on-full like the Phase-8 producer
  + the RecsHintProducer).
- scheduler: new cron job (Logic A) adapting the hotcombos join + D8 recency + JP-audio + episodes_aired
  → fire demand(`ongoing`, latest-aired ep) per ongoing anime with ≥1 active JP-audio watcher.
</specifics>

<deferred>
## Deferred Ideas
- Evictor / budget pre-admit gate on enqueue → Phase 10 (Planner may over-enqueue here; P10 gates it).
- Grafana download/trigger panels → Phase 11 (but emit downloads_total now).
- AI-prediction producers → v2.
</deferred>
