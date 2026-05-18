# Roadmap: AnimeEnigma `raw-jp` workstream

**Workstream:** raw-jp (parallel to root v3.0 Universal Anime Scraper)
**Active milestone:** v0.1 Raw Provider MVP
**Phase numbering:** Workstream-local — starts at 1, independent of root project numbering.
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`

## Milestones

- 🟢 **v0.1 Raw Provider MVP** — Streaming (active) — see [`milestones/v0.1-ROADMAP.md`](milestones/v0.1-ROADMAP.md)
- ⏳ **v0.2 Self-Hosted Library** (planned) — `services/library/` + admin UI + hybrid resolver
- ⏳ **v0.3 Auto-Download Watched Ongoings** (planned) — RSS poller + admin oversight

## Phases (v0.1)

### Phase 1: AllAnime Parser

**Goal:** Implement a new catalog parser at `services/catalog/internal/parser/allanime/` that queries AllAnime's GraphQL API with `translationType: raw` and returns episode lists + HLS stream URLs. Persisted-query SHA hashes live in env config. Rotating-domain support across `[allanime.day, allmanga.to, allanime.to]`.

**Depends on:** Nothing — additive backend module.
**Requirements:** RAW-01, RAW-02, RAW-NF-01
**SPEC:** `milestones/v0.1-phases/01-allanime-parser/01-SPEC.md`

### Phase 2: Subtitle Aggregator + Extended ID Mapping

**Goal:** Add an OpenSubtitles v1 REST parser. Extend `libs/idmapping/` to resolve IMDb/TMDB IDs via Kitsu mappings. Add a `subs_aggregator` catalog service that fans out to Jimaku + OpenSubtitles, merges, dedupes by hash, and groups by language. Expose `GET /api/anime/{id}/subtitles?lang=ru,en,jp,...`.

**Depends on:** Nothing — additive backend modules. Independent of Phase 1.
**Requirements:** RAW-03, RAW-04
**SPEC:** `milestones/v0.1-phases/02-subtitle-aggregator/02-SPEC.md`

### Phase 3: RawPlayer.vue + Other Subs Panel

**Goal:** New `frontend/web/src/components/player/RawPlayer.vue` mirroring the HiAnimePlayer.vue HLS+overlay structure. Subtitle picker promoted to primary control. New `frontend/web/src/components/player/OtherSubsPanel.vue` modal listing every sub track grouped by language with provider attribution.

**Depends on:** Phases 1 + 2 backend endpoints.
**Requirements:** RAW-05, RAW-06
**SPEC:** `milestones/v0.1-phases/03-raw-player-frontend/03-SPEC.md`

### Phase 4: Frontend Wiring + Changelog

**Goal:** Wire the new `'raw'` provider into `frontend/web/src/views/Anime.vue` — add a third "RAW JP" language group alongside RU and EN, extend the `preferred_video_provider` type union, add `preferred_raw_provider` localStorage key. Add changelog entry. Playwright e2e against `ui_audit_bot`.

**Depends on:** Phase 3 components.
**Requirements:** RAW-07, RAW-08, RAW-NF-02
**SPEC:** `milestones/v0.1-phases/04-frontend-wiring/04-SPEC.md`

## Progress

| Phase | Milestone | Plans | Status      | Completed |
|-------|-----------|-------|-------------|-----------|
| 1     | v0.1      | n/a   | Not started | —         |
| 2     | v0.1      | n/a   | Not started | —         |
| 3     | v0.1      | n/a   | Not started | —         |
| 4     | v0.1      | n/a   | Not started | —         |

## Next

Run `/gsd-autonomous --ws raw-jp` to execute v0.1 end-to-end.

Or step-by-step:
- `/gsd-discuss-phase 1 --ws raw-jp`
- `/gsd-plan-phase 1 --ws raw-jp`
- `/gsd-execute-phase 1 --ws raw-jp`
- (repeat for phases 2–4)
