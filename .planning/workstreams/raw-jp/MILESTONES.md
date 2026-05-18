# Milestones — `raw-jp` workstream

## v0.1 Raw Provider MVP (shipped)

**Status:** ✅ Complete — 4/4 phases shipped
**Started:** 2026-05-18
**Shipped:** 2026-05-18
**Source spec:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Summary:** `milestones/v0.1-SUMMARY.md`
**Known followup:** `docs/issues/README.md` ISS-012 — operator runbook for refreshing AllAnime persisted-query SHAs from a live browser network capture before flipping `VITE_RAW_PROVIDER_ENABLED=true` in production.

**Phases delivered:**
1. AllAnime Parser (backend, catalog service) — `services/catalog/internal/parser/allanime/`
2. Subtitle Aggregator + Extended ID Mapping (backend, catalog + libs) — `services/catalog/internal/parser/opensubtitles/`, `libs/idmapping/kitsu.go`, `services/catalog/internal/service/subs_aggregator.go`
3. RawPlayer.vue + Other Subs Panel (frontend components)
4. Frontend Wiring + Changelog (Anime.vue integration + e2e) — behind `VITE_RAW_PROVIDER_ENABLED`

---

## v0.2 Self-Hosted Library (shipped)

**Status:** ✅ Complete — 6/6 phases shipped
**Started planning:** 2026-05-18 (after v0.1 ship)
**Shipped:** 2026-05-18
**Audit:** `v0.2-MILESTONE-AUDIT.md` — passed (15/15 LIB-* requirements)
**Summary:** `milestones/v0.2-SUMMARY.md`

**Scope:** A new `services/library/` Go microservice on port 8087. Admin-only library manager that finds Nyaa.si / AnimeTosho releases, downloads via embedded BitTorrent (`anacrolix/torrent`), transcodes to HLS via ffmpeg (H.264/AAC, 6s segments), stores in a new MinIO bucket `raw-library`, and exposes a hybrid resolver path that prefers the self-hosted copy over AllAnime when both exist for an anime episode. Library service runs independently of the catalog hot path — its outage degrades gracefully back to AllAnime-only behavior.

**Phases:**
1. Library service scaffold + docker-compose wiring + gateway routing + DB bootstrap
2. Nyaa.si RSS + AnimeTosho JSON-feed search clients
3. Embedded torrent client (`anacrolix/torrent`) + Postgres job queue (`library_jobs` with `FOR UPDATE SKIP LOCKED`) + Prometheus metrics
4. ffmpeg HLS transcoder + MinIO writer
5. `frontend/web/src/views/admin/RawLibrary.vue` admin UI (search → queue → monitor)
6. Hybrid resolver — extend the v0.1 raw resolver to prefer library service when episode is in MinIO

**Goal:** Provide a self-hosted, admin-controlled, stable raw-JP source for the titles that matter — eliminating dependency on AllAnime's persisted-query SHA rotation for the popular catalogue. The library service is a backend concern with a small admin surface; no end-user-visible change until an admin queues a first job and the hybrid resolver redirects playback.

**Acceptance:**
- Admin can search Nyaa + AnimeTosho via the new UI and queue a torrent job.
- Jobs proceed through `queued → downloading → encoding → uploading → done` with progress visible in the UI and emitted as Prometheus metrics.
- MinIO bucket `raw-library` populates with per-shikimori_id HLS segments + playlists.
- After an episode lands in MinIO, the existing `/api/anime/{id}/raw/stream` endpoint returns the MinIO HLS URL (not AllAnime) — verified via log + metric.
- Library service unhealthy → catalog continues to use AllAnime alone (graceful degradation).

**Roadmap:** `milestones/v0.2-ROADMAP.md`
**Requirements:** `milestones/v0.2-REQUIREMENTS.md`

---

## v0.3 Auto-Download for Watched Ongoings (planned)

**Status:** ⏳ Planned
**Trigger:** v0.2 shipped and at least one admin-curated library actively in use.

**Scope:** Watch-tracking → resolve to ongoing anime → SubsPlease/Ohys-Raws RSS poller → auto-queue with admin oversight gate (per-anime opt-in for auto-approve, otherwise jobs land in "pending approval").

**Why deferred:** Needs real watch data to know which ongoings matter to actual users.
