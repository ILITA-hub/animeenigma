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

🟢 **v0.1 Raw Provider MVP** — Streaming-only foundation. AllAnime parser, multi-language subtitle aggregator, new `RawPlayer.vue`, "Other subs" panel, provider chip wiring.

Run `/gsd-autonomous --ws raw-jp` to drive v0.1 end-to-end.

## Planned milestones

- 🟢 **v0.1 Raw Provider MVP** (active) — Streaming. AllAnime + subs + player + wiring.
- ⏳ **v0.2 Self-Hosted Library** (planned) — `services/library/` with anacrolix/torrent + ffmpeg HLS + MinIO + RawLibrary.vue admin + hybrid resolver.
- ⏳ **v0.3 Auto-Download Watched Ongoings** (planned) — SubsPlease/Ohys-Raws RSS poller, per-anime opt-in, admin oversight gate.

## Active requirements (v0.1)

See `milestones/v0.1-REQUIREMENTS.md`.

## Validated requirements (carried)

None yet — first milestone.

## Context

Touches: `services/catalog/internal/parser/{allanime,opensubtitles}/`, `services/catalog/internal/service/`, `libs/idmapping/`, `frontend/web/src/components/player/{RawPlayer,OtherSubsPanel}.vue`, `frontend/web/src/views/Anime.vue`, `docker/.env` (new env vars), `frontend/web/public/changelog.json`. Zero overlap with v3.0 scraper microservice.

---

*Workstream root: `.planning/workstreams/raw-jp/`*
