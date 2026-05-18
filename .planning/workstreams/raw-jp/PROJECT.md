# Project: AnimeEnigma — `raw-jp` workstream

**Parent project:** AnimeEnigma (see `/data/animeenigma/.planning/PROJECT.md`)
**Workstream:** raw-jp
**Created:** 2026-05-18
**Lifecycle:** Independent of v3.0 Universal Anime Scraper. Runs in parallel.
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`

## Scope of this workstream

Add a new **raw Japanese** video provider, an **"Other subs"** multi-language subtitle aggregator, and (in later milestones) an admin-controlled self-hosted library backed by Nyaa/AnimeTosho torrents transcoded to HLS in MinIO.

The new provider serves a single audio track — original Japanese, no dub. RU and EN coverage comes from the subtitle layer, not the video layer.

## Out of scope for this workstream

- HiAnime / Consumet / Kodik / AnimeLib changes — separate workstream concern. The dead-HiAnime problem will be addressed in its own track.
- DRM / geo-restriction / licensed-content support — self-hosted small-group platform.
- Per-user upload of subtitle files — out of scope for all v0.x milestones.
- Private trackers, indexers, or distribution layers — we consume public sources only.

## Active milestone

🟢 **v0.2 Self-Hosted Library** — `services/library/` Go microservice on port 8087: Nyaa.si + AnimeTosho search → embedded BitTorrent (`anacrolix/torrent`) → ffmpeg HLS transcode → MinIO storage → hybrid resolver that prefers the self-hosted copy over AllAnime. Six phases. Planning artifacts ready under `milestones/v0.2-*`.

Run `/gsd-autonomous --ws raw-jp` to drive v0.2 end-to-end.

## Planned milestones

- ✅ **v0.1 Raw Provider MVP** (shipped 2026-05-18) — Streaming-only foundation: AllAnime parser, multi-language subtitle aggregator, `RawPlayer.vue` + "Other subs" panel + chip wiring (behind `VITE_RAW_PROVIDER_ENABLED`). Followup ISS-012 for the AllAnime persisted-query SHA refresh runbook. See `milestones/v0.1-SUMMARY.md`.
- 🟢 **v0.2 Self-Hosted Library** (active) — `services/library/` with `anacrolix/torrent` + ffmpeg HLS + MinIO + `RawLibrary.vue` admin + hybrid resolver. See `milestones/v0.2-ROADMAP.md` + `milestones/v0.2-REQUIREMENTS.md` + per-phase SPECs.
- ⏳ **v0.3 Auto-Download Watched Ongoings** (planned) — SubsPlease/Ohys-Raws RSS poller, per-anime opt-in, admin oversight gate.

## Active requirements (v0.2)

See `milestones/v0.2-REQUIREMENTS.md` for LIB-01..10 + LIB-NF-01..04.

## Shipped requirements (v0.1, carried)

- RAW-01..08, RAW-NF-01, RAW-NF-02 — all delivered. Detail: `milestones/v0.1-SUMMARY.md`.

## Context

Touches in v0.2: a brand-new `services/library/` Go service + Dockerfile, `docker/docker-compose.yml` (library block + new volumes), `services/gateway/internal/router/routes.go` (proxy for `/api/library/*`), `services/catalog/internal/service/raw_resolver.go` (hybrid resolver extension), `services/catalog/internal/parser/library/` (new client), `frontend/web/src/views/admin/RawLibrary.vue`, `frontend/web/src/router/index.ts`, `frontend/web/src/api/client.ts` (`adminLibraryApi`), `frontend/web/src/types/library.ts`, `infra/grafana/dashboards/library.json`, `Makefile`, `CLAUDE.md`. Zero overlap with v3.0 scraper microservice.

---

*Workstream root: `.planning/workstreams/raw-jp/`*
