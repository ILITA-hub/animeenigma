---
id: RAW-subtitle-aggregator
title: Subtitle aggregator (Jimaku + OpenSubtitles) + extended ID mapping
workstream: raw-jp
milestone: v0.1
phase: 02
created_at: 2026-05-18
status: SPEC-ready
ambiguity_score: 0.15
mode: --auto
---

# Phase 02 (workstream `raw-jp`, milestone v0.1): Subtitle Aggregator + Extended ID Mapping — Specification

**Workstream:** `raw-jp`
**Milestone:** v0.1 Raw Provider MVP
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Requirements:** RAW-03, RAW-04, RAW-NF-01
**Mode:** `--auto`

## Goal

Aggregate subtitles from multiple providers (Jimaku for JP, OpenSubtitles for RU/EN/everything else), exposed via two new catalog endpoints: one with a language filter for the player's default-pick logic, one without (returns every track) for the "Other subs" panel. Extend `libs/idmapping/` to resolve IMDb/TMDB IDs via Kitsu mappings — required because OpenSubtitles uses IMDb/TMDB keying, not AniList/MAL.

## Background

**Today, two things are true and need to change:**

1. **Subtitle coverage is JP-only via Jimaku.** `services/catalog/internal/parser/jimaku/client.go` is the only subtitle parser. It searches by AniList ID (resolved from Shikimori via `libs/idmapping/` ARM client). The frontend's `SubtitleOverlay.vue` renders ASS/SRT/VTT and supports selectable text for vocab lookup — that infrastructure is solid and should be reused, not replaced.

2. **ID mapping is shallow.** `libs/idmapping/` currently calls ARM (`arm.haglund.dev/api/v2`) which returns `{anilist_id, anidb_id, kitsu_id, thetvdb_id}`. It does **not** return IMDb or TMDB IDs. OpenSubtitles' REST API v1 (`api.opensubtitles.com/api/v1/subtitles`) keys subtitle search by `imdb_id` or `tmdb_id` (or query string), so we must extend the resolver.

**The implementation:**
- New parser `services/catalog/internal/parser/opensubtitles/` calls the OpenSubtitles v1 REST endpoint, auth via `OPENSUBTITLES_API_KEY` env var.
- New aggregator service `services/catalog/internal/service/subs_aggregator.go` fans out to Jimaku + OpenSubtitles in parallel, dedupes by content hash (or URL when hash unavailable), groups by ISO language code.
- `libs/idmapping/` extended with a Kitsu mappings lookup that derives IMDb/TMDB from the existing Kitsu ID (Kitsu API exposes `mappings` for IMDb and TMDB on its anime endpoint).
- Catalog DB `animes` table gains `imdb_id` + `tmdb_id` columns. Populated lazily on first OpenSubtitles query.

## Requirements

### RAW-03: OpenSubtitles parser + aggregator + endpoints

- **Current:** No OpenSubtitles parser. `services/catalog/internal/parser/jimaku/client.go` is the only subtitle source. No aggregator service. The subtitle URL list is exposed inline on the catalog stream-response (e.g. `hianime.Stream.Subtitles []Subtitle`).
- **Target:**
  - `services/catalog/internal/parser/opensubtitles/client.go` — REST v1 client.
    - `func (c *Client) Search(ctx, params SearchParams) ([]SubtitleEntry, error)` with `SearchParams{IMDbID, TMDBID, Query, Languages, SeasonNumber, EpisodeNumber}`.
    - Returns `[]SubtitleEntry{ID, FileID, Language, Release, DownloadCount, Format, DownloadURL}`.
    - Auth via `Api-Key` header from `OPENSUBTITLES_API_KEY` env.
    - User-Agent from `OPENSUBTITLES_USER_AGENT` (default `AnimeEnigma/1.0`).
  - `services/catalog/internal/service/subs_aggregator.go` — fan-out service.
    - `func (s *Aggregator) FetchAll(ctx, shikimoriID, episode int, langs []string) ([]AggregatedSubtitle, error)`.
    - Resolves Shikimori → {anilist, imdb, tmdb} via `libs/idmapping/`.
    - Goroutines: Jimaku.SearchByAnilistID, OpenSubtitles.Search.
    - Merges results, dedupes by `(language, sha256(content))` when content hash known; falls back to `(language, url)` when not.
    - Groups by ISO 639-1 language code.
  - `services/catalog/internal/handler/subtitles.go`:
    - `GET /api/anime/{shikimori_id}/subtitles?lang=ru,en,jp&episode=1`
    - `GET /api/anime/{shikimori_id}/subtitles/all?episode=1`
    - Both return `{"languages": {"ja": [...tracks], "en": [...tracks], ...}, "episode": 1}`.
- **Acceptance:** For Shikimori ID 52082 (Bocchi the Rock), episode 1, requesting `lang=ja,en,ru` returns ≥1 track in `ja` (from Jimaku) and ideally tracks in `en` from OpenSubtitles. The `/all` endpoint returns every track including languages not requested.

### RAW-04: Extended ID mapping (IMDb/TMDB via Kitsu)

- **Current:** `libs/idmapping/client.go` calls ARM and returns `{anilist_id, anidb_id, kitsu_id, thetvdb_id}`. No IMDb or TMDB.
- **Target:**
  - New `libs/idmapping/kitsu.go`:
    - `func (c *Client) KitsuMappings(ctx, kitsuID int) (ExtraIDs, error)` calling `https://kitsu.io/api/edge/anime/{kitsuID}?include=mappings`.
    - Returns `ExtraIDs{IMDbID, TMDBID}` extracted from the `mappings` array (external sites `imdb`, `themoviedb/movie` or `themoviedb/tv`).
  - Schema: `ALTER TABLE animes ADD COLUMN IF NOT EXISTS imdb_id TEXT;` and `tmdb_id TEXT;` (via GORM AutoMigrate path in `services/catalog/cmd/catalog-api/main.go`).
  - On first OpenSubtitles query for an anime, if `animes.imdb_id` is NULL, call the Kitsu mappings endpoint and persist the result.
  - Cache the mapping result indefinitely (IDs don't change).
- **Acceptance:** Querying subtitles for an anime with `animes.imdb_id IS NULL` triggers a Kitsu lookup. After the request, `SELECT imdb_id FROM animes WHERE shikimori_id = ?` returns a non-null value (when Kitsu has the mapping). Second query is served from the cached DB column without re-hitting Kitsu.

### RAW-NF-01: Graceful degradation

- **Current:** Subtitle errors propagate to the player as failed stream responses.
- **Target:**
  - Jimaku 401/403 (missing or revoked API key) → log error, return OpenSubtitles-only results.
  - OpenSubtitles 429 (rate limit) → log warning, return Jimaku-only results.
  - OpenSubtitles 5xx → log error, return Jimaku-only results.
  - Both providers down → return `200 OK` with empty `languages` object and a header `X-Subtitle-Providers-Down: jimaku,opensubtitles` for observability.
  - Kitsu mapping fails → continue without IMDb/TMDB; OpenSubtitles falls back to query-string search.
- **Acceptance:** With `OPENSUBTITLES_API_KEY=invalid`, the `/subtitles` endpoint returns 200 with Jimaku-only results, not 5xx. With both providers' API keys revoked, the endpoint returns 200 with empty result, not 5xx.

## Acceptance Criteria

1. `services/catalog/internal/parser/opensubtitles/client.go` exists with the `Search` method and unit tests against recorded HTTP fixtures.
2. `services/catalog/internal/service/subs_aggregator.go` exists and parallel-fans-out Jimaku + OpenSubtitles via goroutines.
3. `services/catalog/internal/handler/subtitles.go` registers two new routes.
4. `libs/idmapping/kitsu.go` exists and `libs/idmapping/client.go` is extended with `KitsuMappings`.
5. `animes` table has `imdb_id` + `tmdb_id` columns after the next service start (GORM AutoMigrate).
6. `curl http://localhost:8081/api/anime/52082/subtitles?lang=ja,en` returns 200 with ≥1 result.
7. `curl http://localhost:8081/api/anime/52082/subtitles/all` returns 200 with ≥2 distinct languages represented.
8. With `OPENSUBTITLES_API_KEY=invalid`, the same endpoint still returns 200 (Jimaku-only).
9. After the first query, `SELECT imdb_id FROM animes WHERE shikimori_id = '52082'` returns non-null.

## Auto-selected implementation decisions

- **Concurrency:** `errgroup.Group` with `SetLimit(2)` for the two providers. No per-provider retry inside the goroutine — fail-fast and downgrade.
- **Dedupe key:** `(language, sha256(content))` when content hash is known (Jimaku exposes it); `(language, url)` otherwise. Strict string match, no fuzzy matching.
- **Language code normalization:** ISO 639-1 two-letter codes everywhere (`ja`, `en`, `ru`). Convert Jimaku's `japanese` and OpenSubtitles' `jpn` to `ja` at the parser boundary.
- **Episode-aware query:** OpenSubtitles search includes `season_number=1` for non-movie anime by default. Movies (anime type = `movie`) omit episode/season.
- **Rate-limit detection:** HTTP 429 OR response body containing `"Reached download limit"`. Either triggers Jimaku-only fallback.
- **Cache:** Subtitle list cached for 6 hours per `(shikimori_id, episode, language_set)` key in Redis. Subtitle file content not cached (downloaded on demand by the frontend).

## Touches

- **New:** `services/catalog/internal/parser/opensubtitles/client.go`
- **New:** `services/catalog/internal/service/subs_aggregator.go`
- **New:** `services/catalog/internal/handler/subtitles.go`
- **New:** `libs/idmapping/kitsu.go`
- **Extend:** `libs/idmapping/client.go` — add `Resolve()` that returns the full `ExtraIDs` (anilist + anidb + kitsu + thetvdb + imdb + tmdb)
- **Extend:** `services/catalog/internal/transport/router.go` (register routes)
- **Extend:** `services/catalog/internal/domain/anime.go` — add `IMDbID *string` and `TMDBID *string` fields to `Anime` struct with GORM tags
- **Extend:** `services/catalog/internal/config/config.go` — load `OPENSUBTITLES_API_KEY`, `OPENSUBTITLES_USER_AGENT`
- **Extend:** `docker/.env.example` and `docker/.env`
- **Extend:** `services/catalog/internal/cache/keys.go` — add `KeySubtitles(shikimoriID, episode int, langs []string)`

## Out of Scope (for this phase)

- Frontend integration — Phase 3.
- Kage Project / fansubs.ru integration — deferred (Cloudflare-protected).
- Subtitle quality scoring or auto-pick — deferred to v0.4+.
- User-uploaded subtitles — out of scope for all v0.x.

## Citations to design doc

- Architecture → "services/catalog/internal/parser/opensubtitles/ (NEW)"
- Architecture → "services/catalog/internal/service/subs_aggregator.go (NEW)"
- Architecture → "libs/idmapping/ (EXTENDED)"
- Tech-choices table → "Multi-lang subs: Jimaku (JP) + OpenSubtitles v1 REST"
- Tech-choices table → "Subtitle key bridging: Extend libs/idmapping/ with IMDb/TMDB via Kitsu"
- Error-handling table → OpenSubtitles 429, Jimaku 401 rows
