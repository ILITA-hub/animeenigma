# Raw JP Provider + Multi-Lang Subs + Self-Hosted Library — Design

**Status:** Draft (awaiting user review)
**Date:** 2026-05-18
**Owner:** catalog + new `library` service + frontend
**Workstream:** `raw-jp` (new)

## Problem

The platform has four video providers: Kodik (RU iframe), AnimeLib (RU MP4), HiAnime (EN HLS), Consumet (EN HLS). All four serve dubbed audio. None expose raw Japanese audio as a first-class track, and the subtitle integration is JP-only via Jimaku.

Three gaps follow:

1. **No raw JP audio track.** Users who want to watch with original Japanese audio + their native-language subtitles have no path.
2. **Subtitle catalog is narrow.** Only JP subs (Jimaku) are surfaced; RU/EN/other languages exist on external providers but the platform doesn't aggregate them.
3. **All four providers scrape live external APIs.** No fallback when an upstream rotates endpoints, applies Cloudflare, or shuts down (HiAnime literally went dark in March 2026 — our `aniwatch` container returns 500s today).

The user wants:
- A new "raw" provider serving original Japanese audio with no dub.
- An "Other subs" button that exposes every subtitle file we can find across languages and providers, not just Jimaku JP.
- An admin-controlled path to pre-download seasons from Nyaa.si / AnimeTosho into MinIO so we have a stable, self-hosted source for important titles.

## Goals

- Add a new video parser whose only audio track is raw Japanese, surfacing it under a new "RAW" language tier in the provider chip switcher.
- Aggregate subtitles from at least three providers (Jimaku JP + OpenSubtitles multi-lang + one more) and expose them in a per-language picker plus an "Other subs" panel listing every available track.
- Build an admin-only library manager that finds Nyaa/AnimeTosho releases, downloads them via embedded BitTorrent, transcodes to HLS, and stores in MinIO — then prefers the self-hosted copy over the external scrape when both exist.
- Ship in three milestones so we can deliver value early (v0.1 = streaming) and not block on the heavier library work (v0.2 = manual library, v0.3 = auto-download for watched ongoings).

## Non-Goals

- Replacing the existing HiAnime/Consumet/Kodik/AnimeLib integrations. The dead-HiAnime problem is a separate workstream concern.
- Building a private tracker, indexer, or distribution layer. We consume public sources only.
- DRM, geo-restriction, or licensed-content support. This is a self-hosted small-group platform.
- Per-user upload of subtitle files. Out of scope for v0.x.
- Automatic transcoding presets per device. We always output a single H.264 + AAC ladder.

## Research summary

Streaming-source research and subtitle-source research were both conducted; key findings drive the design.

**Raw JP streaming** — `AllAnime` (`api.allanime.day`) is the only verified live source with explicit `translationType: raw` exposed via a stable GraphQL endpoint. AnimeKai and AnimePahe are second-tier. HiAnime is dead. AllAnime rotates SHA-hashed persisted query IDs every few months — those must be config, not code.

**Subtitles** — Jimaku covers JP (already integrated). OpenSubtitles v1 REST API covers RU/EN/most languages but is keyed by IMDb/TMDB IDs, not AniList — so we extend our existing `libs/idmapping/` to also resolve to IMDb/TMDB. Kage Project / fansubs.ru is the richest RU anime archive but has no API and is Cloudflare-protected; it goes in the "stretch" column.

**animejoy.ru and Substital** were specifically asked about. animejoy.ru is a DLE aggregator with no separate subtitle API — subs are baked inside fansub-team iframe uploads. Substital is a thin Chrome wrapper over the same OpenSubtitles REST endpoint we'd hit directly. Neither offers a new integrable surface.

**Self-hosted library** — `Nyaa.si` is the public index; `AnimeTosho` (feed.animetosho.org) mirrors Nyaa with extra metadata including MAL ID filters. Trusted uploaders for raws: Ohys-Raws, Leopard-Raws, ARC-Raws, SubsPlease. Embedded Go torrent client: `github.com/anacrolix/torrent`.

## Architecture

### Three-milestone breakdown

```
Milestone v0.1 — Raw provider MVP (streaming)
  Phase 1  AllAnime parser (services/catalog/internal/parser/allanime/)
  Phase 2  Subtitle aggregator + extended ID mapping
  Phase 3  RawPlayer.vue + Other-subs panel
  Phase 4  Frontend wiring (language chip, provider preference)

Milestone v0.2 — Self-hosted library (manual)
  Phase 5  services/library/ scaffold + docker-compose + gateway routing
  Phase 6  Nyaa/AnimeTosho search clients
  Phase 7  Embedded torrent client + postgres job queue + Grafana metrics
  Phase 8  ffmpeg HLS transcoder (H.264/AAC) + MinIO writer
  Phase 9  RawLibrary.vue admin UI
  Phase 10 Hybrid resolver — prefer MinIO over AllAnime when both exist

Milestone v0.3 — Auto-download for watched ongoings
  Phase 11 Watch-tracking + ongoing-anime resolution
  Phase 12 SubsPlease/Ohys-Raws RSS poller + auto-queue
  Phase 13 Admin oversight gate (per-anime auto-approve, otherwise pending)
```

### Backend: new + extended modules

```
services/catalog/internal/parser/allanime/           # Phase 1 — NEW
  client.go                                          # GraphQL client
  queries.go                                         # Persisted-query SHA hashes (env-overridable)
  episodes.go                                        # episodes/streams/subtitles unpacking
  domains.go                                         # rotating-domain list

services/catalog/internal/parser/opensubtitles/      # Phase 2 — NEW
  client.go                                          # REST v1 client (api.opensubtitles.com)
  search.go                                          # by imdb_id / tmdb_id / query

services/catalog/internal/service/subs_aggregator.go # Phase 2 — NEW
  Fan-out: Jimaku + OpenSubtitles (+ Kage stretch),
  merge results, group by language, dedupe by hash.

libs/idmapping/                                      # Phase 2 — EXTENDED
  Existing: ARM (shikimori/mal → anilist/anidb/kitsu)
  New:      Kitsu mappings table for imdb_id / tmdb_id
  Cache result in catalog DB: anime_ids table (1:N)

services/library/                                    # Phase 5 — NEW SERVICE
  cmd/library-api/main.go
  internal/
    config/         # env + tuning knobs
    domain/         # Job, Episode, Mapping models
    nyaa/           # AnimeTosho JSON + Nyaa RSS clients
    torrent/        # anacrolix/torrent wrapper (download + seed)
    ffmpeg/         # HLS-segment transcoder
    minio/          # bucket writer
    queue/          # postgres-backed job queue
    handler/        # admin API
    service/        # orchestration
    transport/      # router + metrics
  migrations/
    001_library_jobs.sql
    002_library_episodes.sql
    003_library_anime_mappings.sql
  Dockerfile

Gateway routing (services/gateway):                  # Phase 5 — EXTENDED
  /api/admin/library/* → library:8087
```

### Frontend: new + extended components

```
frontend/web/src/components/player/RawPlayer.vue     # Phase 3 — NEW
  - HLS.js + Video.js (mirror HiAnimePlayer.vue)
  - SubtitleOverlay.vue mounted at fullscreen target
  - Subtitle picker UI promoted to primary control
  - "Other subs" trigger button → OtherSubsPanel

frontend/web/src/components/player/OtherSubsPanel.vue # Phase 3 — NEW
  - Modal/sheet listing all subs grouped by language
  - Provider attribution chip per row (Jimaku / OpenSubtitles / Kage)
  - On pick: emit selection up to RawPlayer

frontend/web/src/views/Anime.vue                      # Phase 4 — EXTENDED
  - Add 'raw' to preferred_video_provider type union
  - Add raw chip in provider switcher, third language group "RAW JP"
  - localStorage: preferred_raw_provider (future-proofing for v0.2+)

frontend/web/src/views/admin/RawLibrary.vue           # Phase 9 — NEW
  - Search input (title + optional MAL ID prefill)
  - Result list with filter chips (uploader, quality, size)
  - "Add to queue" action
  - Active downloads dashboard (poll /queue)
  - Library view + episode-to-Shikimori linker
  - Disk-space indicator from /api/admin/library/stats

frontend/web/src/components/player/SubtitleOverlay.vue  # REUSED unchanged
frontend/web/src/utils/subtitle-parser.ts               # REUSED unchanged
```

### Data flow — raw video playback

```
User opens anime → Anime.vue switches to RAW provider chip
                         ↓
            GET /api/anime/{id}/raw/episodes
                         ↓
       catalog service / raw resolver
            ├─ does library service have shikimori_id?  (Phase 10)
            │      yes → return MinIO HLS URLs
            │      no  → query AllAnime via translationType:raw
            ↓
RawPlayer.vue receives episodes + stream URLs
RawPlayer.vue mounts video, calls GET /api/anime/{id}/subtitles?lang=ru,en,jp
                         ↓
        catalog service / subs aggregator (Phase 2)
            ├─ Jimaku.search_by_anilist(id)
            ├─ OpenSubtitles.search_by_imdb(imdb_id, ep)
            └─ (v0.2+) library service local subs index
                         ↓
        Merge, dedupe, group by language → JSON response
                         ↓
RawPlayer.vue picks default sub by user pref → SubtitleOverlay renders
"Other subs" button opens OtherSubsPanel listing every track
```

### Data flow — admin library download (Phase 7–10)

```
Admin opens RawLibrary.vue → searches "Bocchi the Rock"
                         ↓
        GET /api/admin/library/search?q=...&mal_id=...
                         ↓
        library service / nyaa client
            ├─ AnimeTosho JSON feed (preferred — MAL ID filter)
            └─ Nyaa.si RSS fallback
                         ↓
        Enriched results: filename parse → episode/quality/uploader
                         ↓
Admin clicks "Add to queue" → POST /api/admin/library/queue
                         ↓
        library service:
            1. Insert row in library_jobs (status=queued)
            2. Schedule worker
                         ↓
        Worker pipeline (status transitions):
            queued → downloading (anacrolix/torrent)
            downloading → encoding (ffmpeg HLS transcode)
            encoding → uploading (MinIO PUT segments)
            uploading → done

        Progress + metrics emitted to Prometheus every 5s:
            library_jobs_total{status}
            library_download_bytes_total
            library_encode_duration_seconds
            library_active_torrents
            library_disk_free_bytes
                         ↓
        On done: write to library_episodes
        On done: notify catalog service to invalidate raw-resolver cache
```

### Persistent storage

**Postgres — library service own DB**

```sql
-- 001_library_jobs.sql
CREATE TABLE library_jobs (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  shikimori_id    TEXT,           -- set after admin links episodes
  source          TEXT NOT NULL,  -- 'nyaa' | 'animetosho'
  magnet          TEXT NOT NULL,
  title           TEXT NOT NULL,
  uploader        TEXT,
  quality         TEXT,
  size_bytes      BIGINT,
  status          TEXT NOT NULL,  -- queued|downloading|encoding|uploading|done|failed|cancelled
  progress_pct    REAL DEFAULT 0,
  error_text      TEXT,
  created_at      TIMESTAMPTZ DEFAULT now(),
  updated_at      TIMESTAMPTZ DEFAULT now(),
  completed_at    TIMESTAMPTZ
);
CREATE INDEX idx_library_jobs_status ON library_jobs(status) WHERE status NOT IN ('done', 'cancelled');

-- 002_library_episodes.sql
CREATE TABLE library_episodes (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  shikimori_id    TEXT NOT NULL,
  episode_number  INT NOT NULL,
  job_id          UUID REFERENCES library_jobs(id),
  minio_path      TEXT NOT NULL,  -- {shikimori_id}/{episode}/playlist.m3u8
  duration_sec    INT,
  size_bytes      BIGINT,
  created_at      TIMESTAMPTZ DEFAULT now(),
  UNIQUE(shikimori_id, episode_number)
);

-- 003_library_anime_mappings.sql (auto-detect helper)
CREATE TABLE library_filename_patterns (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uploader        TEXT NOT NULL,
  pattern_regex   TEXT NOT NULL,  -- captures episode_number
  example         TEXT,
  created_at      TIMESTAMPTZ DEFAULT now()
);
```

**MinIO — new bucket `raw-library`**

```
raw-library/
  {shikimori_id}/
    {episode_number}/
      playlist.m3u8
      segment_000.ts
      segment_001.ts
      ...
```

**Catalog DB — extended `anime_ids` table**

```sql
ALTER TABLE animes ADD COLUMN IF NOT EXISTS imdb_id TEXT;
ALTER TABLE animes ADD COLUMN IF NOT EXISTS tmdb_id TEXT;
-- Filled lazily via Kitsu mappings lookup on first OpenSubtitles query.
```

### Tech choices

| Concern | Choice | Why |
|---|---|---|
| Raw streaming source | AllAnime GraphQL with `translationType: raw` | Only verified-live source with explicit raw track |
| Backup raw source | None in v0.1 | YAGNI; revisit if AllAnime instability bites |
| Persisted-query SHA | env config (`ALLANIME_QUERY_*_SHA`) | Rotates every few months upstream; needs hot-swap without redeploy |
| Rotating domains | Static list `[allanime.day, allmanga.to, allanime.to]` with first-success caching | Cheap, matches what scrapers in the wild do |
| Multi-lang subs | Jimaku (JP) + OpenSubtitles v1 REST | Two providers cover ~95% of realistic asks; OpenSubtitles has own API key in `docker/.env` |
| Subtitle key bridging | Extend `libs/idmapping/` with IMDb/TMDB via Kitsu | We already resolve through ARM; one more step gives OpenSubtitles' key set |
| Other-subs UI | Modal panel triggered from player toolbar | Matches existing modal/sheet patterns in the codebase |
| Library service language | Go, separate service `services/library/` | Long-running torrents + ffmpeg should not share a process with HTTP handlers |
| Library port | 8087 | Next free port |
| Torrent client | `github.com/anacrolix/torrent` embedded | Pure-Go, no daemon, supports magnet/DHT/PEX |
| Encoding | ffmpeg subprocess: H.264 + AAC + HLS segmenter (6s segments, VOD playlist) | Maximum browser compatibility; matches user's explicit choice |
| Encoding concurrency | 2 workers default, `LIBRARY_ENCODE_WORKERS` env knob | CPU-bound; tunable per host |
| Job queue | Postgres `library_jobs` table with `FOR UPDATE SKIP LOCKED` | Simple, no extra infrastructure, audit-friendly |
| Auto-download policy | Manual only (v0.2) → auto for watched-ongoing list (v0.3) | User explicit preference |
| Metrics | Prometheus on `/metrics`, Grafana dashboard in `infra/grafana/dashboards/library.json` | Matches existing service convention |

### Configuration

New env vars (additions to `docker/.env`):

```bash
# AllAnime parser — SHA hashes discovered during Phase 1 implementation
# by inspecting the live network traffic to api.allanime.day.
# Treat as opaque rotating tokens, not stable values.
ALLANIME_QUERY_SEARCH_SHA=<set-in-phase-1>
ALLANIME_QUERY_EPISODES_SHA=<set-in-phase-1>
ALLANIME_QUERY_SOURCES_SHA=<set-in-phase-1>

# OpenSubtitles
OPENSUBTITLES_API_KEY=<our-registered-key>
OPENSUBTITLES_USER_AGENT=AnimeEnigma/1.0

# Library service (v0.2)
LIBRARY_DB_HOST=postgres
LIBRARY_DB_NAME=library
LIBRARY_TORRENT_DOWNLOAD_DIR=/data/torrents
LIBRARY_TORRENT_MAX_PEERS=80
LIBRARY_TORRENT_MAX_UPLOAD_RATE_KBPS=1024
LIBRARY_ENCODE_WORKERS=2
LIBRARY_MINIO_BUCKET=raw-library
LIBRARY_DISK_FREE_ALERT_PCT=20
```

Docker-compose additions (v0.2):

```yaml
library:
  build: { context: ., dockerfile: services/library/Dockerfile }
  ports: ["8087:8087"]
  env_file: [./.env]
  volumes:
    - library_torrents:/data/torrents   # downloaded torrent files (transient)
    - library_minio_staging:/tmp/encode # ffmpeg working dir
  depends_on: [postgres, minio]
  deploy:
    resources:
      limits: { cpus: '4.0', memory: '4G' }
```

## Error handling

| Failure | Behavior |
|---|---|
| AllAnime GraphQL 4xx (stale persisted-query SHA) | Log warning, return empty episode list, surface "raw provider unavailable" in UI; admin updates env and restarts catalog |
| AllAnime all domains timeout | Same as above |
| OpenSubtitles 429 (rate-limit) | Return Jimaku-only result, log warning, retry-after backoff in client |
| Jimaku 401 | Log error, return OpenSubtitles-only result |
| AnimeTosho timeout | Fall back to Nyaa RSS |
| Torrent stalled (no peers for >30 min) | Mark job `failed`, surface to admin UI with retry button |
| ffmpeg non-zero exit | Mark job `failed`, capture stderr tail in `error_text`, retain partial download for debugging |
| MinIO PUT failure | Retry 3× with exponential backoff, then mark job `failed` |
| Disk free < 20% | Refuse new jobs with HTTP 507, emit alert metric |
| Episode auto-detection failure | Job completes with `shikimori_id=NULL`, admin links episodes manually in UI |

## Testing

| Layer | Approach |
|---|---|
| AllAnime parser | Mock GraphQL responses (recorded from real API) → assert episode/stream extraction; integration test gated on `INTEGRATION=1` actually hits the live API |
| Subs aggregator | Mock each provider client → assert merge/dedupe/grouping; one integration test against the UI audit user's profile |
| ID mapping | Unit test ARM→Kitsu→IMDb chain with fixtures; cache assertion |
| Library torrent client | Unit test wrapping anacrolix/torrent with a fake tracker fixture; integration test downloads a known-small public-domain torrent |
| ffmpeg worker | Unit test command construction; integration test transcodes a 30s MP4 to HLS, asserts playlist + segments produced |
| Job queue | Unit test state machine transitions; concurrent-worker test with `SKIP LOCKED` |
| RawPlayer.vue | Playwright e2e: open known AllAnime-backed anime as `ui_audit_bot`, assert raw video loads, subtitle picker populates, "Other subs" panel opens |
| RawLibrary.vue | Playwright e2e (admin role): search returns results, queue a no-op torrent (fixture), assert UI reflects state transitions |

## Rollout

**v0.1 (streaming-only)** — additive, no migration. Behind feature flag `RAW_PROVIDER_ENABLED` until live testing on `ui_audit_bot` passes. Provider chip is hidden when disabled. Changelog entry in `frontend/web/public/changelog.json`.

**v0.2 (manual library)** — admin-only surface, no user-visible change until an admin queues a first job. Hybrid resolver gates on library service health; if library is unhealthy, catalog falls back to AllAnime alone.

**v0.3 (auto-ongoing)** — per-anime opt-in by admin; no global enable. Default behavior unchanged from v0.2.

## Open questions for plan-phase

These belong in the plan phase, not the spec:

- Exact filename-parse heuristics per uploader (Ohys-Raws, SubsPlease, Erai-raws variants).
- Multi-season torrent handling: do we treat `Bocchi.the.Rock.S1+S2.Complete` as one or two library entries?
- MinIO storage cap + cleanup policy when disk is tight.
- OpenSubtitles free-tier daily download quota — what limit do we apply per request, and do we cache aggressively?
- Resume strategy for interrupted torrents across service restarts.
- Whether the library service exposes a public read-only stats page for transparency.

## Future work (out of scope here)

- v0.4+: subtitle quality scoring + auto-pick best track per language
- v0.4+: AnimeKai / AnimePahe as secondary raw sources behind a feature flag
- v0.5+: HiAnime → AllAnime migration (replace dead `services/catalog/internal/parser/hianime/` integration with sub/dub via AllAnime)
- v0.5+: Kage Project / fansubs.ru integration for richer RU subs
- v0.6+: User-uploaded subtitles
