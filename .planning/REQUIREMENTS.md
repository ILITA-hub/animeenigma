# Requirements: AnimeEnigma — v4.1 Auto Torrent Population

**Defined:** 2026-06-17
**Core Value:** A logged-in user hits play on the first-party "ae" provider and the RAW (JP-audio) episode is already there — pre-downloaded by the platform's prediction of what they're about to watch, served from a self-managing storage pool with zero admin action.
**Design spec:** `docs/superpowers/specs/2026-06-17-auto-torrent-population-design.md`

## v1 Requirements

Requirements for milestone v4.1. Each maps to exactly one roadmap phase.

### Storage Pool & Config (POOL)

- [ ] **POOL-01**: First-party HLS is stored under `aeProvider/<MALID>/RAW/<episode>/playlist.m3u8` (RAW track only in v1; SUB/DUB branches reserved but unused).
- [ ] **POOL-02**: Existing admin-ingested episodes are migrated one-time from `{shikimori_id}/{ep}/` into the new `aeProvider/<MALID>/RAW/<ep>/` layout without interrupting playback (copy → repoint `minio_path` → delete old).
- [ ] **POOL-03**: `library_episodes` carries `source` (admin|autocache), `track`, `downloaded_at`, `last_fetch_at`, `fetch_count`, and `size_bytes` so one evictor can classify and account for every pool object.
- [ ] **POOL-04**: An admin can view and edit autocache config (budget, freshness windows, active-watcher window, quality cap, min seeders, sweep interval) live via `GET/PATCH /api/admin/library/autocache/config` with no redeploy.
- [ ] **POOL-05**: A master `enabled` switch turns all autocache downloading and eviction on/off.

### Download Triggers (TRIG)

- [ ] **TRIG-01**: For each ongoing anime with ≥1 active JP-audio-combo watcher (list status `watching` AND watch progress within `active_watcher_days`), the system downloads each newly-aired episode once a ≤`quality_cap` release with ≥`min_seeders` appears on the torrent indexers (Logic A — ongoing push).
- [ ] **TRIG-02**: When an active JP-audio-combo user begins watching episode N of a watching anime, the system ensures episode N+1 (if aired) is downloaded ahead of time (Logic B — next-episode pull).
- [ ] **TRIG-03**: A cache miss on the "ae" provider enqueues a backfill download of that episode so subsequent requests hit.
- [ ] **TRIG-04**: Concurrent demand for the same `(mal_id, episode)` collapses to a single download job; an already-present episode enqueues nothing.
- [ ] **TRIG-05**: Only RAW releases at or below `quality_cap` (1080p) and at or above `min_seeders` are selected; DUB-preferring demand never triggers a download.

### Eviction & Budget (EVICT)

- [ ] **EVICT-01**: Total bytes of the `aeProvider/` pool (admin + auto combined) are bounded by a configurable budget (default 100 GB).
- [ ] **EVICT-02**: Each episode is classified Fresh or Stale by source-specific windows — auto: `<auto_fresh_download_days` since download OR `<auto_fresh_fetch_days` since last fetch; admin: `<admin_fresh_days` since upload OR last fetch.
- [ ] **EVICT-03**: When space is needed, only Stale episodes are evicted, in order: auto-never-fetched → auto-fetched → admin-never-fetched → admin-fetched (oldest-first within each group); Fresh episodes are never evicted.
- [ ] **EVICT-04**: If draining the entire Stale queue still cannot fit a new download (including an admin upload), the download is rejected and `library_autocache_rejected_total{reason="budget_full"}` is incremented.
- [ ] **EVICT-05**: The logical budget co-exists with the existing physical-disk `DiskGuard`; both must pass before a download proceeds.

### Serving & Fetch Signal (SERVE)

- [ ] **SERVE-01**: When the "ae" provider is resolved and the episode is present in the pool, it is served from `aeProvider/` and counted as a preload hit.
- [ ] **SERVE-02**: Each "ae" playback updates the episode's `last_fetch_at` and `fetch_count` (the "viewed by any user" freshness + popularity signal).
- [ ] **SERVE-03**: When the episode is absent, the player fails over to existing providers with no regression, and the event is counted as a preload miss.

### Observability (OBS)

- [ ] **OBS-01**: Grafana shows pool storage allocation and usage split by Fresh/Stale and by source (admin/auto), against the budget cap.
- [ ] **OBS-02**: Grafana shows preload hit-rate (hit vs miss) as a cache-hit-style panel.
- [ ] **OBS-03**: Grafana shows eviction counts (by source) and budget-full rejection counts.
- [ ] **OBS-04**: Grafana shows autocache download counts by trigger (A / B / backfill) and result.
- [ ] **OBS-05**: Grafana shows a storage-need prediction table from a daily heuristic (ongoing + next-episode components) compared to the budget.

## v2 Requirements

Deferred to a future milestone. Tracked but not in this roadmap.

### Prediction (PRED)

- **PRED-01**: AI-prediction-driven prefetch replaces/augments the predefined Logic A & B with a learned per-user next-watch probability model.

### Acquisition (ACQ)

- **ACQ-01**: Acquire 2160p+ releases and/or add an upscaling stage (raise `quality_cap` beyond 1080p).

### Tracks (TRACK)

- **TRACK-01**: Populate the `SUB/<ep>` track (e.g. hardsubbed raws) as a distinct object where the JP-video + client-overlay model is insufficient.
- **TRACK-02**: Populate the `DUB/<team-or-provider>/<ep>` track from release-group-tagged torrents.

## Out of Scope

Explicitly excluded for v4.1 to prevent scope creep.

| Feature | Reason |
|---------|--------|
| SUB track population | SUB-preferring demand is served by the **same** RAW JP video with client-side subtitle overlay from existing FE subtitle providers — no separate object needed in v1. |
| DUB track population | Per-DUB-team torrents are rare/messy and DUB-preferring users are served by Kodik/AniLib/EN providers; DUB demand creates no autocache download. |
| AI-based prediction | v1 uses predefined Logic A/B heuristics; the learned model is v2 (PRED-01). |
| 2160p+ / upscaling | Quality capped at 1080p to keep per-episode size and budget predictable; v2 (ACQ-01). |
| New microservice for the cache brain | Built into `services/library` to reuse the existing torrent→HLS→MinIO pipeline; a separate service would split byte-accounting from the evictor. |
| Replacing the physical-disk `DiskGuard` | The logical budget is layered on top of it; the host-disk safety net stays. |

## Traceability

Which phases cover which requirements. **Filled by the roadmapper.**

| Requirement | Phase | Status |
|-------------|-------|--------|
| POOL-01 | TBD | Pending |
| POOL-02 | TBD | Pending |
| POOL-03 | TBD | Pending |
| POOL-04 | TBD | Pending |
| POOL-05 | TBD | Pending |
| TRIG-01 | TBD | Pending |
| TRIG-02 | TBD | Pending |
| TRIG-03 | TBD | Pending |
| TRIG-04 | TBD | Pending |
| TRIG-05 | TBD | Pending |
| EVICT-01 | TBD | Pending |
| EVICT-02 | TBD | Pending |
| EVICT-03 | TBD | Pending |
| EVICT-04 | TBD | Pending |
| EVICT-05 | TBD | Pending |
| SERVE-01 | TBD | Pending |
| SERVE-02 | TBD | Pending |
| SERVE-03 | TBD | Pending |
| OBS-01 | TBD | Pending |
| OBS-02 | TBD | Pending |
| OBS-03 | TBD | Pending |
| OBS-04 | TBD | Pending |
| OBS-05 | TBD | Pending |

**Coverage:**
- v1 requirements: 23 total
- Mapped to phases: 0 (pending roadmap)
- Unmapped: 23 ⚠️

---
*Requirements defined: 2026-06-17*
*Last updated: 2026-06-17 at milestone v4.1 start*
