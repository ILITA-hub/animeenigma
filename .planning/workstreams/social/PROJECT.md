# Project: AnimeEnigma — `social` workstream

**Parent project:** AnimeEnigma (see `/data/animeenigma/.planning/PROJECT.md`)
**Workstream:** social
**Created:** 2026-05-13
**Lifecycle:** Independent of v3.0 Universal Anime Scraper. Runs in parallel.

## Scope of this workstream

Social / UGC features on the anime detail page — public-facing user-generated content that lives alongside the watchlist/list-entry surface. This workstream exists separately from the main project ROADMAP because the work has zero overlap with v3.0 (scraper microservice / providers / catalog) — it touches the `player` service and the `Anime.vue` view only.

## Out of scope for this workstream

- Anything in `services/scraper/`, `services/catalog/internal/parser/`, or the EnglishPlayer surface — that's the v3.0 milestone in the root `.planning/` tree.
- Notifications engine, mentions, activity-feed surface beyond emitting events (see project-level memory `project_notifications_engine.md` for the broader feature).
- Reactions / votes / threading — see workstream Out of Scope in REQUIREMENTS.md.

## Active milestone

**v0.1 Social: Reviews + Comments** — see `ROADMAP.md` in this directory.

---

*Workstream root: `.planning/workstreams/social/`*
*Switch to this workstream: `gsd-sdk query workstream.set social` or pass `--ws social` on every GSD command.*
