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

## Shipped milestones

- ✅ **v0.1 Social: Reviews + Comments** — shipped 2026-05-13. Phase 1 delivered: dropped the `reviews` table (merged into `anime_list`), refactored 6 review endpoints with byte-identical wire shape, added `comments` table + 4 CRUD endpoints (1-2000 chars, soft-delete, cursor pagination, 10/hr rate limit), `type='comment'` activity events, Reviews|Comments tabs on Anime.vue with URL persistence, 24 locale keys × 3 locales. 4 Playwright e2e tests green. See [`milestones/v0.1-ROADMAP.md`](milestones/v0.1-ROADMAP.md) and [`MILESTONES.md`](MILESTONES.md).

## Active milestone

None. Run `/gsd-new-milestone --ws social` to start the next social-workstream milestone.

## Validated requirements (carried from v0.1)

- ✓ SOCIAL-01..06, NF-01/02 — Reviews + Comments stack — v0.1

## Active requirements (for next milestone)

None yet — capture during `/gsd-new-milestone`.

## Context

After v0.1: ~25 files touched, ~40 commits, ~24 tasks across 7 plans. Tech stack unchanged (Go + Vue 3 + Tailwind + GORM + Chi + Postgres). Known carry-overs documented in `MILESTONES.md` under "Known deferred items".

---

*Workstream root: `.planning/workstreams/social/`*
*Switch to this workstream: `gsd-sdk query workstream.set social` or pass `--ws social` on every GSD command.*
*Last updated: 2026-05-13 after v0.1 milestone.*
