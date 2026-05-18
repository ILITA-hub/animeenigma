# Roadmap: AnimeEnigma `raw-jp` workstream

**Workstream:** raw-jp (parallel to root v3.0 Universal Anime Scraper)
**Active milestone:** v0.2 Self-Hosted Library
**Phase numbering:** Workstream-local — restarts at 1 inside each milestone (`v0.1-phases/01-*`, `v0.2-phases/01-*`).
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`

## Milestones

- ✅ **v0.1 Raw Provider MVP** — Shipped 2026-05-18 — see [`milestones/v0.1-SUMMARY.md`](milestones/v0.1-SUMMARY.md)
- 🟢 **v0.2 Self-Hosted Library** (active) — `services/library/` + admin UI + hybrid resolver — see [`milestones/v0.2-ROADMAP.md`](milestones/v0.2-ROADMAP.md)
- ⏳ **v0.3 Auto-Download Watched Ongoings** (planned) — RSS poller + admin oversight

## Active milestone: v0.2 Self-Hosted Library

### Phase 1: Library Service Scaffold

**Goal:** Stand up a new Go microservice at `services/library/` on port 8087 with the standard project layout (`cmd/library-api/main.go`, `internal/{config,domain,handler,repo,service,transport}`, `migrations/`). Wire it into `docker-compose.yml` and the gateway routing (`/api/library/*` → `library:8087`). Bootstrap a dedicated Postgres DB (`library`) via the shared `libs/database` helper. Service responds 200 on `/health` and exposes `/metrics`.

**Depends on:** Nothing — additive backend module.
**Requirements:** LIB-01, LIB-02, LIB-NF-04
**SPEC:** `milestones/v0.2-phases/01-library-scaffold/01-SPEC.md`

### Phase 2: Nyaa + AnimeTosho Search Clients

**Goal:** Two parsers under `services/library/internal/parser/{nyaa,animetosho}/`. `Nyaa.si` RSS client + AnimeTosho JSON-feed client. AnimeTosho preferred when available because it filters by MAL ID; Nyaa is the fallback (RSS, query-string search). Both return a normalized `Release{Title, Magnet, Uploader, Quality, SizeBytes, ...}` slice. Endpoint `GET /api/library/search?q=&mal_id=` returns merged + deduped results.

**Depends on:** Phase 1 service scaffold.
**Requirements:** LIB-03, LIB-04
**SPEC:** `milestones/v0.2-phases/02-nyaa-animetosho-clients/02-SPEC.md`

### Phase 3: Torrent Client + Job Queue + Metrics

**Goal:** Embed `github.com/anacrolix/torrent` behind `services/library/internal/torrent/`. Build a `library_jobs` Postgres-backed queue (state machine: `queued → downloading → encoding → uploading → done|failed|cancelled`) with `FOR UPDATE SKIP LOCKED` for concurrent worker safety. Emit Prometheus metrics on `/metrics` (`library_jobs_total{status}`, `library_download_bytes_total`, `library_active_torrents`, `library_disk_free_bytes`). Surface job enqueue + status endpoints: `POST /api/library/jobs`, `GET /api/library/jobs/{id}`, `GET /api/library/jobs`. Status-only milestone — no encoding yet (Phase 4).

**Depends on:** Phase 1 scaffold; Phase 2 search clients (used by the enqueue handler).
**Requirements:** LIB-05, LIB-06, LIB-NF-01, LIB-NF-02, LIB-NF-03
**SPEC:** `milestones/v0.2-phases/03-torrent-client-job-queue/03-SPEC.md`

### Phase 4: ffmpeg HLS Transcoder + MinIO Writer

**Goal:** Workers progress jobs through the `encoding` and `uploading` states. ffmpeg subprocess wrapper at `services/library/internal/ffmpeg/` transcodes to H.264 + AAC + HLS (6s segments, VOD playlist). MinIO writer at `services/library/internal/minio/` uploads segments + playlist to bucket `raw-library` under `{shikimori_id}/{episode}/playlist.m3u8`. Auto-detect episode number from filename via per-uploader regex patterns stored in `library_filename_patterns`. On successful encode + upload, insert `library_episodes` row.

**Depends on:** Phase 3 job queue.
**Requirements:** LIB-07, LIB-08
**SPEC:** `milestones/v0.2-phases/04-ffmpeg-minio-transcoder/04-SPEC.md`

### Phase 5: RawLibrary.vue Admin UI

**Goal:** New admin-only view `frontend/web/src/views/admin/RawLibrary.vue`. Search input (title + optional MAL ID prefill), result table with provider + uploader + quality + size + "Queue" button, active jobs panel showing progress bars per job, episode-linker dialog for jobs that landed with `shikimori_id=NULL` (admin picks the anime from a search dropdown), and a disk-free / active-torrent stats strip at the top. Gated by `AdminMiddleware` on the backend and by `authStore.isAdmin` in the router.

**Depends on:** Phases 2 (search) + 3 (jobs) + 4 (episodes).
**Requirements:** LIB-09
**SPEC:** `milestones/v0.2-phases/05-rawlibrary-admin-ui/05-SPEC.md`

### Phase 6: Hybrid Resolver

**Goal:** Extend `services/catalog/internal/service/raw_resolver.go` so that `GetStream(animeID, episode, quality)` first checks the library service for a MinIO HLS URL (`GET /api/library/episodes/{shikimori_id}/{episode}`); falls back to AllAnime when absent or when the library service is unhealthy. Add a thin client at `services/catalog/internal/parser/library/`. The catalog continues to expose the same `/api/anime/{id}/raw/stream` shape — frontend code is unchanged.

**Depends on:** Phase 4 (library_episodes table + MinIO state).
**Requirements:** LIB-10
**SPEC:** `milestones/v0.2-phases/06-hybrid-resolver/06-SPEC.md`

## Progress

| Phase | Milestone | Plans | Status      | Completed |
|-------|-----------|-------|-------------|-----------|
| 1     | v0.2      | 0     | Not started | —         |
| 2     | v0.2      | 0     | Not started | —         |
| 3     | v0.2      | 0     | Not started | —         |
| 4     | v0.2      | 0     | Not started | —         |
| 5     | v0.2      | 0     | Not started | —         |
| 6     | v0.2      | 0     | Not started | —         |

## v0.1 phases (shipped — kept for reference)

All four v0.1 phases shipped 2026-05-18. Detail: [`milestones/v0.1-ROADMAP.md`](milestones/v0.1-ROADMAP.md) + [`milestones/v0.1-SUMMARY.md`](milestones/v0.1-SUMMARY.md).

## Next

Run `/gsd-autonomous --ws raw-jp` to execute v0.2 end-to-end.

Or step-by-step:
- `/gsd-discuss-phase 1 --ws raw-jp`
- `/gsd-plan-phase 1 --ws raw-jp`
- `/gsd-execute-phase 1 --ws raw-jp`
- (repeat for phases 2–6)
