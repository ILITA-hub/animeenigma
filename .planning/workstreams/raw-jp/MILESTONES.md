# Milestones — `raw-jp` workstream

## v0.1 Raw Provider MVP (in progress)

**Status:** 🟢 Active — 0/4 phases complete
**Started:** 2026-05-18
**Source spec:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`

**Phases:**
1. AllAnime Parser (backend, catalog service)
2. Subtitle Aggregator + Extended ID Mapping (backend, catalog + libs)
3. RawPlayer.vue + Other Subs Panel (frontend components)
4. Frontend Wiring + Changelog (Anime.vue integration + e2e)

**Goal:** Ship a working RAW JP audio player with multi-language subtitle aggregation behind a feature flag `RAW_PROVIDER_ENABLED`. Enable for `ui_audit_bot` first; flip globally after one week of validation.

**Acceptance:**
- User can watch any AllAnime-covered anime in raw JP audio.
- Subs panel shows ≥3 language groups (RU/EN/JP) when available.
- "Other subs" panel surfaces ≥1 OpenSubtitles result per major anime in the catalog.
- Playwright e2e against `ui_audit_bot` covers the load → subtitle pick → "Other subs" → switch track happy path.

---

## v0.2 Self-Hosted Library (planned)

**Status:** ⏳ Planned
**Trigger:** v0.1 shipped and validated for ≥1 week.

**Scope:** `services/library/` with anacrolix/torrent, ffmpeg HLS transcoder (H.264/AAC), MinIO writer, Grafana metrics, `RawLibrary.vue` admin UI, hybrid resolver (prefer MinIO over AllAnime when both exist).

**Why deferred:** Bigger lift than v0.1, benefits from real usage data on which titles AllAnime fails on.

---

## v0.3 Auto-Download for Watched Ongoings (planned)

**Status:** ⏳ Planned
**Trigger:** v0.2 shipped and at least one admin-curated library actively in use.

**Scope:** Watch-tracking → resolve to ongoing anime → SubsPlease/Ohys-Raws RSS poller → auto-queue with admin oversight gate (per-anime opt-in for auto-approve, otherwise jobs land in "pending approval").

**Why deferred:** Needs real watch data to know which ongoings matter to actual users.
