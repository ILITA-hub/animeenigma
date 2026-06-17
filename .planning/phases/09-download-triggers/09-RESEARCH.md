# Phase 9: Download Triggers - Research

**Researched:** 2026-06-17
**Domain:** Watch-driven RAW autocache — demand producers (player/scheduler) + library Planner/drainer
**Confidence:** HIGH

## Summary

Phase 9 wires the autonomous RAW-download brain on top of the Phase-7/8 foundation. The working
architecture — **player/scheduler produce demand, the library Planner drains it** — is
**confirmed correct and, in fact, mandatory** because of a hard infrastructure constraint
discovered in this research: the `library` service runs on a **separate Postgres database**
(`LIBRARY_DB_NAME:-library`, `docker/docker-compose.yml:1031`), while `catalog`, `player`,
`notifications`, and `scheduler` all share the `animeenigma` DB. The library Planner therefore
**cannot** join `watch_history × anime_list × animes` to compute "who wants what" — that data is
physically in a different database. All demand-enumeration logic MUST live in a shared-DB service
and reach the library only via the Phase-8 endpoint `POST /internal/library/autocache/demand`
and the `autocache_demand` table (which lives in the library DB). `[VERIFIED: docker-compose.yml grep + hotcombos.go]`

The good news: the exact join Logic A needs **already exists** in
`services/notifications/internal/job/hotcombos.go:46-60` — a DISTINCT join over
`watch_history × anime_list × animes WHERE al.status='watching' AND a.status='ongoing'`. Logic A
is that query plus a JP-audio filter and a "latest aired episode" lookup. Logic B's fire point and
fire-and-forget producer pattern also already exist: `RecsHintProducer`
(`services/player/internal/service/recs_hint.go`) fired from `ListService.MarkEpisodeWatched`
(`list.go:430`) is the exact mirror to copy.

**Primary recommendation:** Build a new `services/library/internal/autocache/` **Planner** (cron
loop draining `autocache_demand`) + two producers: (B) a `DemandProducer` in **player**
fired alongside `recsHint.Hint` in `MarkEpisodeWatched`, and (A) a periodic **scheduler** job that
runs the hotcombos-style join (scheduler shares the `animeenigma` DB and already owns the
`robfig/cron` job harness). Phase 9 must also add the `autocache` value to the `job_source` enum
and an `episode` column to `library_jobs` (neither exists yet — see Pitfalls).

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Logic B "user started ep N → want N+1" | player (shared DB) | — | Fires off a watch-progress event already handled in player; combo + status are in player's request/DB |
| Logic A "ongoing + active JP watchers → want latest aired ep" | scheduler (shared DB) | notifications (alt) | Needs the `watch_history×anime_list×animes` join (animeenigma DB) + a periodic sweep; scheduler owns cron |
| Backfill (ae MISS → want ep) | catalog (shared DB) | — | **Already shipped in Phase 8** (`raw_resolver.GetLibraryStream` MISS → `RecordDemand(backfill)`) |
| Demand intake / dedup row | library (library DB) | — | `autocache_demand` PK `(mal_id,episode)` collapses concurrent demand (Phase 8) |
| Drain demand → download job | library Planner (library DB) | — | Only the library can see the pool (`library_episodes`) + the job queue (`library_jobs`); owns Jackett/torrent/MinIO pipeline |
| RAW/quality/seeder gating | library Planner | — | The `domain.Release` fields (`Quality`, `Seeders`, `Title`) live in the library search tier |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/robfig/cron/v3` | v3.0.1 | Periodic Logic-A sweep + Planner drain loop | Already the scheduler/notifications cron harness `[VERIFIED: scheduler/go.mod:16]` |
| `gorm.io/gorm` + `clause` | existing | demand drain + episode-present + job-exists queries | Project DB convention (CLAUDE.md) |
| stdlib `net/http` | — | player→library `POST /internal/.../demand` producer | Mirrors `recs_hint.go` + Phase-8 `library.Client.RecordDemand` |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golang.org/x/sync/errgroup` | existing | Bounded fan-out if the Planner searches many demands per sweep | Cap concurrency to avoid thundering-herd Jackett searches (detector.go:140 precedent) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| New scheduler cron job for Logic A | Run Logic A inside `notifications` detector | notifications already does the identical join, but its job is notification-shaped (snapshots, parser fan-out). Bolting autocache demand onto it couples two unrelated subsystems. Scheduler is the cleaner home. |
| robfig/cron Planner loop | A bare `time.Ticker` loop in library main.go | Ticker is simpler and the WorkerPool already uses one (`download_worker.go:177`); but the sweep cadence is config-driven (`sweep_interval_min`) and must be re-read live, which a ticker handles fine. Either is acceptable; ticker avoids a new lib dep in `library`. |

**Installation:**
```bash
# NONE for library Planner core (stdlib + existing gorm).
# IF the Planner uses robfig/cron, add to services/library/go.mod (already in scheduler):
#   github.com/robfig/cron/v3 v3.0.1
# Recommendation: use a time.Ticker in library (no new dep) for the drain loop; reserve
# robfig/cron for the scheduler-side Logic A job (already present there).
```

**Version verification:** `robfig/cron/v3 v3.0.1` confirmed present in `services/scheduler/go.mod:16`
`[VERIFIED: go.mod grep]`. No new external package is required for Phase 9 — all primitives
(gorm upsert, net/http producer, ticker loop, errgroup) are already in the affected modules.

## Package Legitimacy Audit

> Phase 9 installs **no new external packages**. The only candidate (`robfig/cron/v3`) is already
> a direct dependency of `services/scheduler` and `services/notifications`.

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| github.com/robfig/cron/v3 | Go modules | mature (v3.0.1, 2020) | very high | github.com/robfig/cron | n/a (Go, pre-vendored) | Already a dep — Approved |

**Packages removed due to slopcheck [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

*No new install → no slopcheck gate required for this phase. If a planner adds an external lib,
re-run the legitimacy gate before approving it.*

## Architecture Patterns

### System Architecture Diagram

```
                 ┌──────────── SHARED DB: animeenigma ────────────┐
                 │                                                 │
 user watches ep N                                                │
      │                                                           │
      ▼                                                           │
 player: ListService.MarkEpisodeWatched (list.go:320)             │
      │  combo = (Player, Language, WatchType) from req           │
      │  JP-audio? (Player∈{ae,raw} OR Language=ja)               │
      │  status=watching?                                         │
      ├─► recsHint.Hint(...)        ── (existing, untouched)      │
      └─► [NEW] DemandProducer.Want(mal_id, N+1, "next_ep") ──┐   │
                                                              │   │
 scheduler cron (every sweep_interval_min) [NEW Logic A]      │   │
      │  DISTINCT join watch_history×anime_list×animes        │   │
      │  WHERE status='watching' AND a.status='ongoing'       │   │
      │  AND JP-audio combo  (hotcombos.go pattern)           │   │
      │  → per ongoing anime: latest aired ep (EpisodesAired) │   │
      └─► [NEW] DemandProducer.Want(mal_id, latestEp, "A") ───┤   │
                                                              │   │
 catalog raw_resolver MISS [EXISTING Phase 8]                 │   │
      └─► library.Client.RecordDemand(mal,ep,"backfill") ─────┤   │
                                                              │   │
                 └──────────────────────────────────────────┼───┘
                                                             ▼
                            POST /internal/library/autocache/demand  (Docker-network-only)
                                                             │
                 ┌──────────── LIBRARY DB: library ──────────┼───┐
                 │                                           ▼   │
                 │   DemandRepository.Record  → autocache_demand │
                 │     PK(mal_id,episode)  (dedup, Phase 8)      │
                 │                           ▲                   │
                 │   [NEW] Planner loop (every sweep_interval_min)│
                 │     1. read autocache_config (enabled? cadence)│
                 │     2. drain autocache_demand rows            │
                 │     3. DEDUP:                                 │
                 │        - GetByShikimoriEpisode present? skip  │
                 │        - non-terminal library_jobs row? skip  │
                 │     4. TieredSearcher.FetchAll (Jackett→Nyaa) │
                 │        filter RAW + ≤quality_cap + ≥min_seeders│
                 │     5. jobRepo.Create(source=autocache,       │
                 │            episode=N, magnet, shikimori_id)   │
                 │     6. delete autocache_demand row            │
                 │                  │                            │
                 │                  ▼ (existing pipeline)        │
                 │   WorkerPool → EncoderPool → MinIO            │
                 │     detector.DetectEpisode(filename)          │
                 │     → library_episodes row (aeProvider/…/RAW) │
                 └───────────────────────────────────────────────┘
```

### Recommended Project Structure
```
services/library/internal/autocache/
├── layout.go               # EXISTS — RawPrefix(malID, ep)
├── migrator.go             # EXISTS — Phase 7 pool migration
├── planner.go              # NEW — drain autocache_demand → jobs (TRIG-01..05 drain side)
└── planner_test.go         # NEW

services/player/internal/service/
├── recs_hint.go            # EXISTS — copy this pattern verbatim
├── autocache_demand.go     # NEW — DemandProducer (player → library POST), Logic B
└── list.go                 # MODIFY — fire DemandProducer.Want next to recsHint.Hint (line 430)

services/scheduler/internal/jobs/
├── autocache_logic_a.go    # NEW — hotcombos-style join + per-ongoing latest-ep demand
└── autocache_logic_a_test.go  # NEW
services/scheduler/internal/service/job.go  # MODIFY — register the cron job

services/library/migrations/
├── 008_autocache_job_source.sql   # NEW — ALTER TYPE job_source ADD VALUE 'autocache'
└── 009_library_jobs_episode.sql    # NEW — ADD COLUMN episode INT (for (mal,ep) single-flight)
```

### Pattern 1: Fire-and-forget producer (Logic B) — copy `recs_hint.go`
**What:** A buffered-channel + single-worker goroutine producer that POSTs to a library
`/internal/*` endpoint, drop-on-full, 3s timeout, nil-safe, Start()/Stop() lifecycle.
**When to use:** Player's Logic B `next_ep` demand on watch progress.
**Example:**
```go
// Source: services/player/internal/service/recs_hint.go:80 (Hint) — mirror exactly,
// swapping the URL to LIBRARY_SERVICE_URL + /internal/library/autocache/demand and the
// payload to {mal_id, episode, reason:"next_ep"}.
func (p *RecsHintProducer) Hint(userID, animeID string) {
    if p == nil || !p.enabled { return }
    msg := recsHintMsg{UserID: userID, AnimeID: animeID}
    select {
    case p.ch <- msg:
    default:
        p.log.Warnw("recs hint channel full; dropping recompute hint", ...)
    }
}
```
Note: catalog already has a synchronous `library.Client.RecordDemand(ctx, malID, episode, reason)`
(`services/catalog/internal/parser/library/client.go`, Phase 8) — but player should use the
**async fire-and-forget** shape (recs_hint) so a slow library never blocks `MarkEpisodeWatched`.

### Pattern 2: Logic A enumeration — copy `hotcombos.go` join
**What:** A single DISTINCT join answering "active watchers per ongoing anime + their combos."
**Example:**
```sql
-- Source: services/notifications/internal/job/hotcombos.go:46 — ADD a JP-audio filter
-- and join the aired-episode count. The tables (watch_history, anime_list, animes) are
-- ALL in the animeenigma DB, so this runs from scheduler with no HTTP.
SELECT DISTINCT a.shikimori_id, a.episodes_aired
FROM watch_history wh
JOIN anime_list al ON al.user_id = wh.user_id AND al.anime_id = wh.anime_id
JOIN animes a       ON a.id = wh.anime_id
WHERE al.status = 'watching'
  AND a.status  = 'ongoing'
  AND (wh.player IN ('ae','raw') OR wh.language = 'ja')   -- JP-audio (D3 demand)
  AND al.updated_at > now() - (interval '1 day' * :active_watcher_days)  -- D8 recency
```
The "active watcher" recency window (D8 = `active_watcher_days`) is the part hotcombos does NOT
yet enforce — Phase 9 adds the `updated_at`/`last_watched_at` recency predicate.

### Pattern 3: Planner drain loop — copy `download_worker.go` ticker + `claim` shape
**What:** A ctx-aware ticker loop that re-reads config each tick (live `enabled`/cadence), drains
demand rows, dedups, searches, enqueues.
**Example:**
```go
// Source: services/library/internal/service/download_worker.go:139 (sleep) + main.go:243 (Start)
for {
    if !p.sleep(ctx, cadence) { return }        // ctx-aware
    cfg, err := configRepo.Get(ctx)             // re-read live: enabled + sweep_interval_min
    if err != nil || !cfg.Enabled { continue }  // master switch (POOL-05)
    rows := demandRepo.Drain(ctx, batchLimit)   // NEW repo method (oldest requested_at first)
    for _, d := range rows { p.plan(ctx, d, cfg) }
}
```

### Anti-Patterns to Avoid
- **Joining pool data across the DB boundary:** The Planner must NOT try to read `watch_history`
  from the library DB — it is not there. Demand only arrives via `autocache_demand`.
- **Enqueueing without the present-check:** TRIG-04 requires "already-present enqueues nothing."
  Always `GetByShikimoriEpisode` first.
- **Searching on every demand row every sweep:** A demand row for an anime with no torrent yet
  (Logic A "as soon as on torrents") will sit forever. Searching it every 20min is a
  thundering-herd risk. Add backoff (see Pitfalls).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| `(mal,ep)` demand dedup | A custom in-memory set | `autocache_demand` PK `(mal_id,episode)` + `DemandRepository.Record` ON CONFLICT (`repo/demand.go:40`) | Already durable + restart-safe (Phase 8) |
| Torrent search + seeder ranking | A new Jackett client | `TieredSearcher.FetchAll` (`jackett_tier.go:61`) → Jackett primary, Nyaa+AnimeTosho fallback, already seeder-ranked DESC | Phase 7 dead-swarm fix lives here |
| Download → HLS → MinIO pipeline | Anything | `jobRepo.Create` → `WorkerPool` → `EncoderPool` (existing) | The whole point of building into `library` (D1) |
| Episode-number extraction | Regex in the Planner | `detector.DetectEpisode(filename, uploader)` post-download (`encoder_worker.go:255`) | Per-uploader regex catalogue (`library_filename_patterns`, migration 003) |
| Async player→library producer | New plumbing | Copy `recs_hint.go` `RecsHintProducer` verbatim | Proven drop-on-full + nil-safe + shutdown-ordered |
| Logic A active-watcher join | New SQL from scratch | Extend `hotcombos.go:46` DISTINCT join | Identical trust surface, one query |

**Key insight:** Phase 9 is almost entirely **wiring existing seams** — the producers (recs-hint
pattern), the join (hotcombos), the dedup table (Phase 8), and the download pipeline (Phases 3-4)
all exist. The genuinely new code is the Planner drain loop + the two thin demand producers + two
small migrations.

## Runtime State Inventory

> Phase 9 is additive (new producers + Planner), not a rename/migration. The relevant "state"
> question is schema/enum gaps that block the new code.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | `autocache_demand` (library DB) populated by Phase-8 backfill MISSes already flowing | Planner must drain these on first deploy (backfill demand predates Logic A/B) |
| Live service config | `autocache_config` singleton row (library DB): `enabled`, `quality_cap=1080`, `min_seeders=3`, `active_watcher_days=30`, `sweep_interval_min=20` — all live-editable (`autocache_config.go:20`) | Planner + Logic A read these live each sweep (no redeploy) |
| Schema gap (job_source) | `job_source` enum = {nyaa, animetosho, manual, jackett} — **`autocache` NOT present** (`001`+`004` migrations) | **NEW migration 008**: `ALTER TYPE job_source ADD VALUE IF NOT EXISTS 'autocache'` (copy `004_jackett_source.sql`) + extend `domain.JobSource` const + `allowedSources` map in `jobs.go:148` |
| Schema gap (episode col) | `library_jobs` has **no `episode` column** (`001_library_jobs.sql`) — episode is detected post-download from the filename | **NEW migration 009**: `ADD COLUMN episode INT` on `library_jobs` so the Planner can single-flight on `(shikimori_id, episode)` for in-flight jobs (TRIG-04) without waiting for filename detection |
| Env vars | player needs `LIBRARY_SERVICE_URL` (already at compose `:467`) for the demand POST; an `AUTOCACHE_DEMAND_ENABLED` toggle mirrors `RECS_HINT_ENABLED` | Add player env + scheduler cron expr env |
| Build artifacts | None — pure Go additions | none |

## Common Pitfalls

### Pitfall 1: Library can't see watch data (cross-DB boundary)
**What goes wrong:** A naive Planner tries to compute "active JP watchers" itself and finds the
tables don't exist in its database.
**Why it happens:** `library` is on `LIBRARY_DB_NAME:-library`; the watch tables are on
`animeenigma`. `[VERIFIED: docker-compose.yml:1031 vs 528]`
**How to avoid:** Keep ALL enumeration in shared-DB producers (scheduler/player). The library only
ever drains `autocache_demand`. This is the load-bearing reason the working architecture is correct.
**Warning signs:** Any `gorm` query in `services/library/` referencing `watch_history`,
`anime_list`, or `animes`.

### Pitfall 2: Episode number unknown at enqueue (filename-detected post-download)
**What goes wrong:** The Planner wants to enqueue "episode N" but the magnet doesn't carry an
episode number; `detector.DetectEpisode` only runs after the torrent payload exists
(`encoder_worker.go:255`).
**Why it happens:** Torrents are filename-addressed; a release may be a single ep or a season pack.
**How to avoid:** (a) Persist the *intended* episode on the new `library_jobs.episode` column at
enqueue so single-flight dedup + the A/B/backfill trigger metric (OBS-04) work before detection.
(b) For RAW single-episode groups (Ohys-Raws, SubsPlease, Erai-raws — the seeded patterns in
`003_library_filename_patterns.sql`), the Planner's search query should target the specific
episode (e.g. `"<title>" <NN>`) so the top hit is that episode, not a batch. (c) Accept that a
season-pack match may populate multiple episodes — the encoder writes whatever it detects; the
demand row for N is satisfied as long as the resulting `library_episodes` row for N appears.
**Warning signs:** Demand rows never clearing because the detected episode ≠ the requested episode.

### Pitfall 3: RAW vs SUB/DUB is not a structured field
**What goes wrong:** `domain.Release` has `Title`, `Quality`, `Seeders` — but **no release-type
field**. TRIG-05 "RAW only" must be inferred from the title/uploader. `[VERIFIED: release.go:28]`
**Why it happens:** Torrent indexers don't tag audio language structurally.
**How to avoid (heuristic, recommend):**
  - **Prefer known RAW uploaders** — `Ohys-Raws`, `Leopard-Raws`, `ARC-Raws` are raw-only groups
    (and are already in the filename-pattern catalogue). `SubsPlease`/`Erai-raws` carry soft subs
    but are JP-audio (RAW video) — acceptable under D3 (one RAW video serves SUB demand via overlay).
  - **Negative filter on title tokens** that imply non-JP audio or hardsub: reject titles matching
    `(?i)\b(dub|dual[ .-]?audio|multi[ .-]?audio|eng[ .-]?dub|hardsub)\b` when the goal is a pure JP
    track. (Dual-audio is borderline — it contains JP audio so it is *usable*, but is larger; rank
    it below single-audio raws.)
  - **Quality gate:** parse the resolution token with the existing
    `qualityRegex = (?i)\b(2160|1080|720|480)p\b` (`animetosho/client.go:78`); accept only
    `≤ quality_cap`. The `Release.Quality` string is already normalized to e.g. `"1080p"`.
  - **Seeder gate:** `Release.Seeders >= min_seeders` (only Jackett populates Seeders; on the
    Nyaa/AnimeTosho fallback Seeders==0, so either require the Jackett tier for autocache or treat
    0-seeder fallback hits as ineligible — recommend: autocache enqueues only Jackett-sourced
    releases so the seeder gate is meaningful).
**Realistic limitation:** This is a best-effort heuristic; there is no perfect RAW classifier from
torrent metadata. Document the chosen token-list + uploader allowlist as a tunable.

### Pitfall 4: Thundering-herd searches for not-yet-released episodes (Logic A)
**What goes wrong:** Logic A demands "latest aired ep" but the torrent may not exist yet ("as soon
as on torrents"). A naive Planner re-searches that demand every 20-min sweep forever.
**Why it happens:** Demand rows are durable; absence of a torrent is not absence of demand.
**How to avoid:** Add a per-row backoff. Options: (a) a `last_searched_at` / `attempts` column on
`autocache_demand` and skip rows searched within an exponentially growing window; (b) cap searches
per sweep via `errgroup.SetLimit` (detector.go:145 caps at 5). Recommend (a)+(b). Also TTL stale
demand (e.g. a Logic-A demand for an ep that aired >N days ago and still has no torrent gets
dropped).
**Warning signs:** Jackett rate-limit errors; `autocache_demand` row count growing unbounded.

### Pitfall 5: catalog WriteTimeout=30s vs library WriteTimeout=120s
**What goes wrong:** A synchronous demand/search call routed through catalog could be cut at 30s
(`catalog-api/main.go:410`). `[VERIFIED]`
**Why it happens:** catalog's 30s WriteTimeout is tight for any slow downstream (project memory:
"catalog WriteTimeout=30s cuts pool warm-up").
**How to avoid:** Phase 9 producers do NOT route demand through catalog request paths — Logic B is
fire-and-forget from player (async, returns immediately) and Logic A runs in scheduler (a cron job,
no HTTP request timeout). The library Planner runs its own ticker loop (library WriteTimeout=120s
doesn't even apply — it's not serving a request). Keep it that way; never make a user-facing
request synchronously wait on a Jackett search or a download.
**Warning signs:** Any new demand fire placed inside a catalog HTTP handler's response path.

### Pitfall 6: Demand-row lifecycle vs job lifecycle drift
**What goes wrong:** The Planner enqueues a job then deletes the demand row; the job later fails
(stall, no peers — `download_worker.go:220`). The episode is now neither present nor wanted, so it
silently never downloads.
**Why it happens:** Two separate lifecycles (demand row in library DB, job row in library DB) with
no link if the demand row is deleted on *enqueue*.
**How to avoid:** Either (a) delete the demand row only when the `library_episodes` row appears
(on encoder success), not on enqueue — re-derive "in flight" from a non-terminal `library_jobs`
row to avoid re-enqueue; or (b) keep the demand row and let the single-flight dedup (present-check
+ non-terminal-job-check) naturally re-attempt after a failed job goes terminal. Recommend (b):
demand rows are cheap and the dedup already prevents duplicates; clear them on confirmed presence.
**Warning signs:** Episodes that "should" be cached but aren't, with no pending job.

## Code Examples

### Episode-present dedup (TRIG-04 "already present → nothing")
```go
// Source: services/library/internal/repo/episode.go:48
// Returns liberrors.NotFound when absent — the Planner's present-check.
ep, err := episodeRepo.GetByShikimoriEpisode(ctx, malID, episode)
if err == nil && ep != nil {
    // already in the pool — drop the demand, enqueue nothing (TRIG-04)
    _ = demandRepo.Delete(ctx, malID, episode) // NEW thin repo method
    continue
}
```

### Tiered search + RAW/quality/seeder filter (TRIG-05)
```go
// Source: services/library/internal/service/jackett_tier.go:61 (FetchAll)
res, err := tiered.FetchAll(ctx, service.SearchParams{Query: title, MALID: malID, Limit: 50})
// res.Releases is seeder-ranked DESC (Jackett). Filter for autocache:
for _, r := range res.Releases {
    if !isRaw(r.Title, r.Uploader) { continue }            // uploader allowlist + token negative-filter
    if resOf(r.Quality) > cfg.QualityCap { continue }      // ≤1080p via qualityRegex parse
    if r.Seeders < cfg.MinSeeders { continue }             // ≥3 seeders (Jackett-only signal)
    return r, true                                         // best-seeded eligible RAW release
}
```

### Enqueue with source=autocache (after migrations 008/009)
```go
// Source: services/library/internal/handler/jobs.go:210 (Create) — Planner calls jobRepo.Create directly
job := &domain.Job{
    Source:      domain.JobSourceAutocache,  // NEW enum value (migration 008)
    Magnet:      release.Magnet,
    Title:       release.Title,
    Quality:     release.Quality,
    SizeBytes:   release.SizeBytes,
    ShikimoriID: malID,                       // == mal_id
    Episode:     episode,                     // NEW column (migration 009) — single-flight key
    Status:      domain.JobStatusQueued,
}
err := jobRepo.Create(ctx, job)
```

### In-flight single-flight check (TRIG-04 concurrent collapse)
```go
// NEW JobRepository method, mirrors List() filter shape (repo/job.go:77).
// Skip enqueue if a non-terminal job already exists for (shikimori_id, episode).
exists, _ := jobRepo.HasActiveForEpisode(ctx, malID, episode) // status NOT IN (done,failed,cancelled)
if exists { _ = demandRepo.Delete(ctx, malID, episode); continue }
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Admin-only manual library ingest | Watch-driven autocache (this milestone) | v4.1 | Planner auto-enqueues; admin path (`jobs.go:158`) stays |
| In-memory demand queue | Durable `autocache_demand` table | Phase 8 | Survives restarts; Planner drains across reboots |
| `job_source` ∈ {nyaa,animetosho,manual,jackett} | + `autocache` | **Phase 9 (NEW)** | OBS-04 trigger attribution + admin-UI provenance |

**Deprecated/outdated:**
- The user's premise "source enum now includes `autocache`" is **not yet true** — Phase 9 must add
  it (migration 008). `[VERIFIED: 001/004 migrations]`

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | "JP-audio combo" == `Player ∈ {ae, raw}` OR `Language == "ja"`; Kodik/AniLib(ru)/English(en) = not JP-audio | Q1 / Logic B+A filter | If the real signal is `WatchType`/translation-based, the wrong watchers trigger demand. Confirm with the actual `watch_history` distribution — `ValidLanguages={ru,en,ja}` + `ValidPlayers` include `ae`/`raw` strongly support this. `[ASSUMED]` |
| A2 | Logic B should fire from `MarkEpisodeWatched` (list.go:430) alongside `recsHint.Hint`, not from `UpdateProgress` | Q1 | Spec §4 says "first progress event for N to maximize lead time" — that argues for `UpdateProgress` (progress.go:29, the heartbeat) for earlier lead time. `MarkEpisodeWatched` fires only when N is *completed* (too late for "ahead of N"). **Recommend firing N+1 demand from the first heartbeat of episode N in `UpdateProgress`** — but `UpdateProgress` lacks a recsHint precedent, so verify the request carries combo + that it's idempotent enough not to spam. `[ASSUMED]` |
| A3 | `animes.episodes_aired` is the authoritative "latest aired episode" for Logic A | Q2 | `EpisodesAired` exists (`domain/anime.go:32`) but may lag Shikimori sync; if 0/stale, Logic A targets the wrong ep. Cross-check against `next_episode_at`. `[ASSUMED]` |
| A4 | Autocache should enqueue only Jackett-sourced releases (so the `min_seeders` gate is meaningful — Nyaa/AnimeTosho leave Seeders=0) | Q4 | If Jackett is disabled (`JACKETT_API_KEY` empty), autocache silently never enqueues. Document the dependency; consider a fallback seeder source. `[ASSUMED]` |
| A5 | RAW classification heuristic (uploader allowlist + dub/dual-audio negative token filter) is acceptable accuracy for v1 | Q4 | False negatives (skip a valid raw) or false positives (download a dub) are possible; impact is wasted budget or a wrong-audio episode. Tunable mitigates. `[ASSUMED]` |

## Open Questions

1. **Logic B fire point: `UpdateProgress` (heartbeat) vs `MarkEpisodeWatched` (completion)?**
   - What we know: spec §4 wants the demand on the *first progress event for N* for max lead time.
     `recsHint.Hint` lives in `MarkEpisodeWatched` (completion). `UpdateProgress` (progress.go:29)
     is the heartbeat and already has the combo (`req.Player`).
   - What's unclear: whether to add a "first-heartbeat-for-this-episode" guard so N+1 demand fires
     once early, not on every heartbeat.
   - Recommendation: fire from `UpdateProgress` with a once-per-(user,anime,episode) guard (or just
     rely on `autocache_demand` PK dedup to absorb repeats — cheapest). Confirm in discuss-phase.

2. **Does Logic A belong in scheduler or notifications?**
   - What we know: notifications already runs the exact join (hotcombos.go) on its own cron; the
     scheduler owns the generic cron-job harness and a clean `jobs/` dir.
   - What's unclear: whether to reuse notifications' collector (DRY) or duplicate the join in
     scheduler (decoupled).
   - Recommendation: **scheduler** — new `jobs/autocache_logic_a.go`, copy the hotcombos SQL +
     JP-audio + recency + aired-ep join. Keeps notification logic and autocache logic independent.
     Data-join cost: one DISTINCT join over indexed columns, run every `sweep_interval_min` (~20m)
     — negligible.

3. **`library_jobs.episode` column — add now or derive from a `(shikimori_id, title)` convention?**
   - What we know: no episode column today; episode is filename-detected post-download.
   - Recommendation: **add the column** (migration 009). Single-flight dedup (TRIG-04) and the
     per-trigger download metric (OBS-04) both need the intended episode known at enqueue.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Postgres `animeenigma` DB | Logic A join (scheduler), Logic B (player) | ✓ | shared | — |
| Postgres `library` DB | Planner drain, `autocache_demand`, `library_jobs` | ✓ | separate (`LIBRARY_DB_NAME`) | — |
| Jackett indexer | RAW/seeder search (`JACKETT_API_KEY`) | conditional | — | Nyaa+AnimeTosho (no Seeders → can't gate min_seeders) |
| `robfig/cron/v3` | scheduler Logic A job | ✓ | v3.0.1 | time.Ticker |
| `LIBRARY_SERVICE_URL` (player) | Logic B demand POST | ✓ | compose:467 | — |

**Missing dependencies with no fallback:** none (all infra present).
**Missing dependencies with fallback:** Jackett — when `JACKETT_API_KEY` is empty, the seeder gate
is unenforceable; recommend autocache no-ops (or warns) rather than enqueueing peerless torrents.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + httptest + GORM/SQLite seams |
| Config file | none (per-package `*_test.go`) |
| Quick run command | `cd services/library && go test ./internal/autocache/... ./internal/service/... -count=1` |
| Full suite command | `cd services/library && go test ./... -count=1 && cd ../player && go test ./internal/service/... -count=1 && cd ../scheduler && go test ./internal/jobs/... -count=1` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TRIG-01 | Logic A: ongoing+active JP watcher → demand latest ep | unit | `go test ./internal/jobs -run LogicA` | ❌ Wave 0 |
| TRIG-02 | Logic B: watch ep N → demand N+1 | unit | `go test ./internal/service -run Demand` (player) | ❌ Wave 0 |
| TRIG-03 | ae MISS → backfill demand | unit | **EXISTS** (`raw_resolver_test.go`, Phase 8) | ✅ |
| TRIG-04 | concurrent (mal,ep) collapses to one job; present → nothing | unit | `go test ./internal/autocache -run Dedup` | ❌ Wave 0 |
| TRIG-05 | RAW ≤1080p ≥3-seeders only; DUB never triggers | unit | `go test ./internal/autocache -run Filter` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/autocache/... -count=1` (library) / relevant producer pkg
- **Per wave merge:** full library + player + scheduler suites
- **Phase gate:** all three module suites green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `services/library/internal/autocache/planner_test.go` — drain + dedup + filter (TRIG-04/05)
- [ ] `services/player/internal/service/autocache_demand_test.go` — Logic B producer (TRIG-02), copy `recs_hint_test.go`
- [ ] `services/scheduler/internal/jobs/autocache_logic_a_test.go` — join + JP-filter (TRIG-01)
- [ ] SQLite test seams for the new `library_jobs.episode` + `HasActiveForEpisode` query

## Security Domain

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Producers are Docker-network-only `/internal/*` (Phase 8 precedent); no user auth on demand |
| V4 Access Control | yes | `/internal/library/autocache/demand` must stay gateway-unreachable (Phase 8 mounted it outside `/api`, verified no gateway route) |
| V5 Input Validation | yes | `mal_id`/`episode` validated server-side (Phase 8 handler already does `mal_id!="" && episode>0`); `reason` forced server-side |
| V6 Cryptography | no | n/a |

### Known Threat Patterns for {Go internal producers + torrent search}
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Spoofed `reason` to force a download trigger | Tampering | Phase 8 already forces `reason` server-side on the backfill path; Logic A/B set reason server-side in the producer, never from a client |
| Unauthenticated demand flood via `/internal/*` | DoS / EoP | Endpoint is Docker-network-only (no gateway proxy — verified Phase 8); demand PK dedup + Planner backoff bound the blast radius |
| Malicious magnet from a search result | Tampering | Magnet parsed (`metainfo.ParseMagnetUri`, jobs.go:181) before enqueue; same path as admin |
| Budget exhaustion via demand spam | DoS | Phase-10 evictor + `budget_bytes` cap (out of Phase 9 scope but co-designed); Planner respects `enabled` master switch |

## Sources

### Primary (HIGH confidence)
- `docs/superpowers/specs/2026-06-17-auto-torrent-population-design.md` §4 (Demand Model & Triggers), §9 (Component Inventory), §10 (Open Risks) — the locked design
- `services/notifications/internal/job/hotcombos.go:46-60` — the Logic A join (NOTIF-DET-02)
- `services/notifications/internal/job/detector.go:140-145` — bounded fan-out / errgroup precedent
- `services/player/internal/service/recs_hint.go` — fire-and-forget producer to copy
- `services/player/internal/service/list.go:320-433` — `MarkEpisodeWatched` (recsHint.Hint fire point)
- `services/player/internal/service/progress.go:29` — `UpdateProgress` heartbeat (alt Logic B point)
- `services/player/internal/domain/preference.go:14-31` — `ValidPlayers`/`ValidLanguages` (JP-audio determination)
- `services/library/internal/repo/demand.go:40` — `DemandRepository.Record` ON CONFLICT dedup
- `services/library/internal/repo/episode.go:48` — `GetByShikimoriEpisode` present-check
- `services/library/internal/service/jackett_tier.go:61` — `TieredSearcher.FetchAll`
- `services/library/internal/service/search.go:93` — `SearchAggregator.FetchAll` fallback
- `services/library/internal/domain/release.go:28-44` — `Release` fields (Quality/Seeders/Title; NO release-type)
- `services/library/internal/parser/animetosho/client.go:78` — `qualityRegex` resolution parse
- `services/library/internal/domain/job.go` + `001`/`004` migrations — `job_source` enum (no `autocache`), no `episode` col
- `services/library/internal/service/encoder_worker.go:255,295,317` — `DetectEpisode` + `RawPrefix` + `library_episodes` write
- `services/library/cmd/library-api/main.go:117-149,234-283` — migration applies + worker startup pattern
- `docker/docker-compose.yml:528,1031` — shared `animeenigma` DB vs separate `library` DB (the load-bearing constraint)
- `services/catalog/cmd/catalog-api/main.go:410` — catalog WriteTimeout=30s hazard
- `services/scheduler/internal/service/job.go:43-58` — `robfig/cron` `AddFunc` registration pattern

### Secondary (MEDIUM confidence)
- `.planning/REQUIREMENTS.md` TRIG-01..05 — requirement text
- `.planning/phases/08-serving-fetch-signal/08-02-SUMMARY.md`, `08-03-SUMMARY.md` — Phase 8 endpoints/producer wiring

### Tertiary (LOW confidence)
- none — all claims traced to in-repo files or the design spec

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all primitives already in-repo (cron, gorm, net/http, existing seams)
- Architecture (producers vs Planner + cross-DB ownership): HIGH — verified by compose DB config + hotcombos join
- Pitfalls: HIGH — each grounded in a specific file:line (schema gaps, episode detection, RAW heuristic, timeouts)
- JP-audio determination (A1) + Logic B fire point (A2): MEDIUM — strong code evidence, needs one discuss-phase confirmation

**Research date:** 2026-06-17
**Valid until:** 2026-07-17 (stable internal codebase; re-verify if Phases 7/8 are amended)
