# Auto Torrent Population (Watch-Driven First-Party RAW Cache) — Design Spec

**Date:** 2026-06-17
**Milestone:** v4.1 (follow-on to v4.0 Activity Register)
**Status:** Design approved (brainstorm), pending GSD milestone planning
**Owner service:** `services/library` (new `internal/autocache/` subsystem)

---

## 1. Summary

Today the self-hosted **"ae"** (AnimeEnigma first-party) RAW provider only serves what an
admin manually ingested via the library service (BitTorrent → HLS → MinIO). This milestone
adds an **autonomous, watch-driven cache**: the platform predicts which RAW (JP-audio)
episodes its users are about to want and downloads them *ahead of time* into a fixed,
self-evicting storage pool — so when a user hits play on the "ae" provider, the episode is
already there.

The prediction logic in v1 is **predefined/heuristic** (two rules, A & B below). An
**AI-prediction-based** planner is an explicit future TODO.

### Goals
- Auto-populate first-party RAW episodes users are likely to watch next, with **zero admin action**.
- Bound storage to a configurable budget (default **100 GB**) with a deterministic
  **Fresh/Stale** lifecycle and eviction, so the pool is self-managing.
- Surface allocation, usage, preload hit-rate, and a storage-need prediction in Grafana.

### Non-goals (v1)
- SUB and DUB track population (schema reserved; not downloaded). SUB-preferring users are
  *served by the same RAW video* with client-side subtitle overlay — see §4.
- AI-driven prediction (future TODO).
- 2160p+ acquisition / upscaling (future TODO).
- Replacing the existing admin manual-ingest flow (it stays; it just shares the new pool).

---

## 2. Key Decisions (locked during brainstorm)

| # | Decision |
|---|----------|
| D1 | **Build into `services/library`** as a new `internal/autocache/` package — reuse `download_worker`, `encoder_worker`, Jackett/Nyaa search tier, MinIO writer. No new service. |
| D2 | **RAW only in v1.** SUB/DUB branches reserved in schema/DB, not populated. |
| D3 | **A single RAW video object serves both RAW- and SUB-preferring demand** (SUB users get client-side subtitle overlay from existing FE subtitle providers). DUB-preferring demand is ignored. |
| D4 | **Quality = best-seeded release ≤1080p.** (TODO: 2160p+ acquisition + upscaling.) |
| D5 | **Unified metered pool.** Admin-uploaded *and* auto-cached content both live under the new `aeProvider/` layout and share **one budget** (default 100 GB, configurable). |
| D6 | **Admin content is evictable but "more fresh"** than auto content — longer freshness window (default 30d) *and* lower eviction priority (evicted only after all auto-Stale is gone). |
| D7 | **One shared budget**; an admin upload that can't fit even after draining all auto-Stale is **rejected** like any other download (metric fires). |
| D8 | **Active watcher (Logic A)** = list status `watching` **AND** watch progress within the last `active_watcher_days` (default 30). |
| D9 | **Fresh never evicted.** If the entire Stale queue is drained and still short on space → reject the new download. |
| D10 | Config is **DB-backed and admin-editable** (live-tunable, no redeploy). |

---

## 3. Storage Model

### 3.1 Layout
All first-party HLS (admin + auto) lives under one prefix scheme in MinIO:

```
aeProvider/<MALID>/
    RAW/<episode>/playlist.m3u8        ← only track populated in v1
    SUB/<episode>/...                  ← reserved (unused v1)
    DUB/<team-or-provider>/<episode>/… ← reserved (unused v1)
```

`<MALID>` == `shikimori_id` (they are the same number in this codebase — no new mapping).

### 3.2 Storage classes (distinguished by DB column, not by path)
- **admin** — manually ingested by an operator. Longer freshness, lower eviction priority.
- **autocache** — pulled automatically by Logic A/B/backfill.

Both classes are metered against the single budget and both are evictable.

### 3.3 One-time migration of existing admin content
Existing admin episodes at `{shikimori_id}/{ep}/playlist.m3u8` migrate into the new layout:
1. Server-side MinIO copy `{shikimori_id}/{ep}/*` → `aeProvider/{shikimori_id}/RAW/{ep}/*`
   (reuse the existing `minio.Writer.Move()` helper).
2. Update the row's `minio_path` **only after the copy succeeds** (idempotent, per-episode).
3. Delete the old objects.
4. Set `source=admin`, `downloaded_at = created_at`, `last_fetch_at = NULL`.

Serving stays seamless because catalog/streaming build URLs from the per-row `minio_path`.
**Migration task must audit** `services/catalog/internal/service/raw_resolver.go` and
`services/catalog/internal/parser/library/client.go` for any hardcoded old-prefix
assumptions and update them.

### 3.4 Data model — extend `library_episodes`
Add columns to the existing table (one table = one evictor, no cross-table byte races):

| Column | Type | Notes |
|--------|------|-------|
| `source` | enum(`admin`,`autocache`) | storage class |
| `track` | enum(`raw`,`sub`,`dub`) | always `raw` in v1 |
| `dub_team` | text null | null for raw |
| `mal_id` | text | == shikimori_id (may alias existing column) |
| `downloaded_at` | timestamptz | Fresh rule 1 basis |
| `last_fetch_at` | timestamptz null | Fresh rule 2 basis; bumped on each ae playback |
| `fetch_count` | bigint default 0 | popularity / hit signal |
| `size_bytes` | bigint | already present; authoritative for budget accounting |

Unique key: `(mal_id, track, dub_team, episode_number)`.

### 3.5 Config table `autocache_config` (singleton row, admin-editable)
| Key | Default | Meaning |
|-----|---------|---------|
| `enabled` | true | master kill-switch for the autocache subsystem |
| `budget_bytes` | 100 GiB | total pool cap (admin + auto) |
| `auto_fresh_download_days` | 10 | auto Fresh while < N days since download |
| `auto_fresh_fetch_days` | 3 | auto Fresh while < N days since last fetch |
| `admin_fresh_days` | 30 | admin Fresh while < N days since upload **or** last fetch |
| `active_watcher_days` | 30 | Logic A recency window |
| `quality_cap` | 1080 | max vertical resolution to download |
| `min_seeders` | 3 | minimum seeders to accept a release |
| `sweep_interval_min` | 20 | Logic A planner cadence |

Exposed via `GET/PATCH /api/admin/library/autocache/config` (admin-gated, gateway-routed).

---

## 4. Demand Model & Triggers

**Demand for a RAW episode** = any *active watcher* of that anime whose saved watch-combo
resolves to **JP audio** (i.e. a `raw`- or `sub`-preferring combo). DUB combos create no
demand. A single RAW object satisfies all such demand (subtitles, if wanted, are overlaid
client-side from the existing FE subtitle providers).

The prediction logic in v1 is the two **predefined** rules below.
> **Future TODO:** replace/augment A & B with an **AI-prediction** planner.

### Logic A — ongoing push
A periodic planner sweep (every `sweep_interval_min`, default 20m) walks the set of
**ongoing** anime that have **≥1 active JP-audio watcher** (active per D8). For each, if the
latest aired episode is not yet cached, it queries the Jackett/Nyaa tier for a ≤`quality_cap`
release with ≥`min_seeders` seeders and enqueues a download. "As soon as it's on torrents" =
the next sweep after the release appears.

### Logic B — next-episode pull
When a JP-audio-combo user makes watch progress on episode *N* of a **watching** anime, the
planner ensures *N+1* (if aired) is cached. Fires on the **first progress event for N** to
maximize lead time (a ~1 GB download completes well within a ~20-min episode). The player
emits this over a new internal endpoint:

```
POST /internal/library/autocache/demand   (Docker-network-only; mirrors the recs-hint seam)
body: { mal_id, episode, reason: "next_ep" | "backfill" }
```

### Dedup
Multiple users wanting the same `(mal_id, episode)` collapse to **one** job. Already-present
= instant hit, no job enqueued.

---

## 5. Hit/Miss Path & the "Fetch" Signal

When the player resolves the **ae** provider (catalog `raw_resolver` → library availability):
- **HIT** — ae serves `aeProvider/<mal>/RAW/<ep>/playlist.m3u8`. Streaming/library bumps
  `last_fetch_at` + `fetch_count`. This is simultaneously the **freshness** signal
  ("viewed by any user") and the **preload-hit** counter.
- **MISS** — ae reports unavailable; the player fails over to AllAnime-raw / other providers
  (today's behavior) **and** fires a `reason: "backfill"` demand so the episode is cached for
  next time. Even un-predicted views improve future hit-rate.

---

## 6. Eviction Algorithm

Runs (a) before admitting a new download when the budget would be exceeded, and (b) on a
periodic sweep. Operates **only** on `aeProvider/` content (admin manual-ingest legacy paths
that haven't migrated yet are out of scope until migration completes).

**Freshness classification** (per row, evaluated at eviction time):
- `autocache` Fresh ⟺ `now − downloaded_at < auto_fresh_download_days` **OR**
  `now − last_fetch_at < auto_fresh_fetch_days`.
- `admin` Fresh ⟺ `now − downloaded_at < admin_fresh_days` **OR**
  `now − last_fetch_at < admin_fresh_days`.
- Otherwise **Stale**.

**Eviction order (only Stale rows are eligible; Fresh is never deleted):**
1. `autocache` · never-fetched (`last_fetch_at IS NULL`) → oldest `downloaded_at` first
2. `autocache` · fetched → oldest `last_fetch_at` first
3. `admin` · never-fetched → oldest `downloaded_at` first
4. `admin` · fetched → oldest `last_fetch_at` first

Delete from the top of this queue until enough room is freed. **If the queue is exhausted and
still short → reject the new download** and increment `autocache_rejected_total{reason="budget_full"}`.

This co-exists with the existing physical-disk `DiskGuard` (free-% safety net): both must
pass. The 100 GB budget is a *logical* quota on the pool; `DiskGuard` protects the host disk.

---

## 7. Observability (update `infra/grafana/dashboards/library.json`)

New Prometheus metrics on the library `/metrics` endpoint:

**Storage allocation & usage**
- `library_autocache_bytes_used{source,freshness}` — stacked usage
- `library_autocache_budget_bytes` — the cap
- `library_autocache_episodes{source,freshness}`

**Preload hit (cache-hit style)**
- `library_autocache_serve_total{result="hit"|"miss"}` → hit-rate % panel

**Eviction / rejection**
- `library_autocache_evicted_total{source}`
- `library_autocache_rejected_total{reason="budget_full"}`

**Downloads**
- `library_autocache_downloads_total{trigger="A"|"B"|"backfill",result}`

**Prediction table** — a daily backend job computes
`library_autocache_predicted_bytes{component="ongoing"|"nextep"}` from a **v1 heuristic**:
- *ongoing* = Σ over watched-ongoing anime (with ≥1 active JP-audio watcher) of
  `avg_raw_ep_size` (one episode of headroom per ongoing).
- *nextep* = (distinct JP-combo watching-anime active in last 30d) × `avg_raw_ep_size`.

Grafana renders a **table** panel: per-ongoing rows + a total, compared against
`budget_bytes`, so the operator can see whether demand is outrunning the pool.
> The heuristic is intentionally coarse for v1; AI-prediction supersedes it later.

---

## 8. Future TODOs (out of v1 scope, captured here)
- **AI-prediction-driven prefetch** — replace/augment Logic A & B with a learned model of
  per-user next-watch probability.
- **2160p+ acquisition + upscaling** — raise `quality_cap` and/or add an upscale stage.
- **DUB/team population** — populate `DUB/<team>/<ep>` from release-group-tagged torrents.
- **Burned-SUB as a distinct track** — only if a hardsubbed raw is ever needed separately
  from the JP video + client overlay model.

---

## 9. Component Inventory (all in `services/library/internal/autocache/`)

| Component | Responsibility |
|-----------|----------------|
| **DemandTracker** | Live set of wanted `(mal_id, episode, reason)` from Logic A/B/backfill; dedup. |
| **Planner** | Wanted-but-absent → download jobs; quality ≤cap, ≥min_seeders; budget pre-check; tags `library_jobs.source=autocache`. |
| **Evictor** | Enforces §6; Fresh/Stale classification + ordered deletion within `aeProvider/`. |
| **Accountant** | Authoritative bytes-used / Fresh / Stale; publishes §7 gauges. |
| **Migrator** | One-time §3.3 admin-content path migration. |
| **PredictionJob** | Daily §7 storage-need heuristic gauge. |

Reused as-is: `download_worker`, `encoder_worker`, Jackett/Nyaa search tier, `minio.Writer`,
`DiskGuard`, `library_jobs` pipeline.

---

## 10. Open Risks / Notes for Planning
- **Combo enumeration for Logic A** needs an internal way to list "active JP-audio watchers
  per anime" — likely a new internal catalog/player endpoint; confirm data ownership in phase
  planning.
- **Migration ordering** — autocache must not start evicting until the §3.3 migration has
  imported admin rows into the metered pool, else admin content (still on old paths) is
  invisible to the Accountant and the budget math is wrong.
- **`avg_raw_ep_size`** for predictions is bootstrapped from observed downloads; before any
  downloads exist, fall back to a configured constant (~1.2 GB).
