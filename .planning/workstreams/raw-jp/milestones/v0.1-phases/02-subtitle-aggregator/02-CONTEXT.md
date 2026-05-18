# Phase 2: Subtitle Aggregator + Extended ID Mapping — Context

**Gathered:** 2026-05-18
**Status:** Ready for planning
**Mode:** `--auto` (decisions auto-derived from SPEC.md; ambiguity_score: 0.15)

<spec_lock>
## Locked Requirements (from SPEC.md)

`milestones/v0.1-phases/02-subtitle-aggregator/02-SPEC.md` locks: RAW-03 (OpenSubtitles parser + aggregator + endpoints), RAW-04 (IMDb/TMDB ID mapping via Kitsu), RAW-NF-01 (graceful degradation on provider failures).

Critical acceptance points: `/api/anime/{id}/subtitles?lang=...&episode=N` returns 200 with merged Jimaku+OpenSubtitles results; `/all` variant returns every language; revoked OpenSubtitles key still returns 200 (Jimaku-only); after the first call, `animes.imdb_id` is populated.
</spec_lock>

<domain>
## Phase Boundary

A backend-only subtitle aggregation layer: a new OpenSubtitles REST parser, an extended `libs/idmapping/` that resolves IMDb/TMDB IDs from Kitsu, a fan-out aggregator service that merges Jimaku + OpenSubtitles results, and two new catalog HTTP endpoints (`/subtitles?lang=...` and `/subtitles/all`).

No frontend changes. No new video sources. Subtitle file content remains downloaded on demand by the frontend; the aggregator only returns metadata + URLs.

</domain>

<decisions>
## Implementation Decisions

### OpenSubtitles parser (`services/catalog/internal/parser/opensubtitles/`)
- Single file `client.go` (small surface — just one Search method).
- REST v1 against `https://api.opensubtitles.com/api/v1`.
- Auth: `Api-Key` header from `OPENSUBTITLES_API_KEY` env.
- Required headers: `User-Agent` (from env, default `AnimeEnigma/1.0`), `Content-Type: application/json`, `Accept: application/json`.
- `SearchParams{IMDbID, TMDBID, Query, Languages, SeasonNumber, EpisodeNumber}`.
- Returns `[]SubtitleEntry{ID, FileID, Language (normalized to ISO 639-1), Release, DownloadCount, Format, DownloadURL}`.
- HTTP 401/403 → `ErrUnauthorized` (typed). 429 → `ErrRateLimited`. 5xx → wrapped `errors.AppError(Unavailable)`.
- 10s HTTP timeout.

### Kitsu mapping (`libs/idmapping/kitsu.go`)
- Independent file in the same package — extends `Client` with `KitsuMappings(ctx, kitsuID)`.
- Calls `https://kitsu.io/api/edge/anime/{kitsuID}?include=mappings`.
- Walks the `included` JSON:API resource array for `external_site` values `imdb`, `themoviedb/movie`, `themoviedb/tv`, and reads `external_id` into the result.
- Returns `ExtraIDs{IMDbID *string, TMDBID *string}` — both nullable because not every anime has either mapping.
- HTTP 404 → `(nil, nil)` (no mapping is a legitimate state, not an error).
- 10s timeout.

### Domain extension
- Add `IMDbID *string` and `TMDBID *string` to `domain.Anime` with `gorm:"size:50;index"` tags (nullable, indexed for future cross-reference lookups).

### Repo helper
- New `AnimeRepository.UpdateExternalIDs(ctx, animeID string, imdb, tmdb *string) error` — updates both columns in one query. Only writes non-nil values.

### Subs aggregator (`services/catalog/internal/service/subs_aggregator.go`)
- New `SubsAggregator` struct wrapping: `*jimaku.Client`, `*opensubtitles.Client`, `*idmapping.Client`, `*repo.AnimeRepository`, `*cache.RedisCache`, `*logger.Logger`.
- `FetchAll(ctx, animeID string, episode int, langs []string) (*AggregateResponse, error)` — public entry.
- Internally resolves the Anime row, then in parallel via `errgroup.Group{SetLimit(2)}`:
  - Jimaku: SearchByAnilistID → GetFiles(entryID, episode) — only when `anime.AniListID` is set or resolvable.
  - OpenSubtitles: ensure `anime.IMDbID`/`TMDBID` is populated (via Kitsu lookup if NULL), then Search with `(imdb_id, episode_number, season_number=1)`.
- Fail-soft per provider (log + skip on that provider's failure, don't abort the whole request).
- Output: `AggregateResponse{Languages map[string][]SubtitleTrack, Episode int, ProvidersDown []string}` — `ProvidersDown` populated for observability.
- Dedupe within each language group by `(lang, url)` (we don't get content-hash from either provider, so URL is the only stable key for v0.1).
- ISO 639-1 normalization at the boundary: `japanese|jpn|ja` → `ja`, `english|eng|en` → `en`, `russian|rus|ru` → `ru`, and so on.
- Cache the merged response for 6h per `(animeID, episode, langs-canonical-sorted)` key.

### Lazy IMDb/TMDB backfill
- Helper `(s *SubsAggregator) ensureExternalIDs(ctx, anime) error` runs only when `anime.IMDbID == nil || anime.TMDBID == nil`.
- Calls `idmapping.Client.KitsuMappings(ctx, kitsuID)` where `kitsuID` comes from `idmapping.Client.ResolveByShikimoriID(anime.ShikimoriID).Kitsu`.
- Persists results to the anime row via `animeRepo.UpdateExternalIDs`.
- Failures are non-fatal — OpenSubtitles falls back to query-string search (using the romanized name) when no IMDb ID.

### Handler (`services/catalog/internal/handler/subtitles.go`)
- New `SubtitlesHandler` struct wrapping `*service.SubsAggregator` + logger.
- `Get` — `GET /api/anime/{animeId}/subtitles?lang=ja,en,ru&episode=N`.
  - `lang` comma-separated, ISO 639-1, defaults to `ja,en,ru`.
  - `episode` required, positive integer.
  - Returns aggregated response.
- `GetAll` — `GET /api/anime/{animeId}/subtitles/all?episode=N`.
  - No language filter — every track in every language.
- Sets `X-Subtitle-Providers-Down: <csv>` response header when one or both providers failed for observability (per SPEC's RAW-NF-01).

### Config + env
- `OpenSubtitlesConfig{APIKey, UserAgent, Timeout}` substruct in `config.Config`.
- Env vars: `OPENSUBTITLES_API_KEY`, `OPENSUBTITLES_USER_AGENT` (default `AnimeEnigma/1.0`), `OPENSUBTITLES_TIMEOUT` (default 10s).
- Documented in `docker/.env.example`.

### Router wiring
- New routes in `services/catalog/internal/transport/router.go`:
  - `r.Get("/{animeId}/subtitles", subtitlesHandler.Get)`
  - `r.Get("/{animeId}/subtitles/all", subtitlesHandler.GetAll)`
- `NewRouter` signature gains `subtitlesHandler *handler.SubtitlesHandler` parameter.

### main.go wiring
- Instantiate `opensubtitles.NewClient(cfg.OpenSubtitles)`, `idmapping.NewClient()` (already a free function — reuse).
- Build `subsAggregator := service.NewSubsAggregator(jimakuClient, openSubsClient, idMappingClient, animeRepo, redisCache, log)`.
- Build `subtitlesHandler := handler.NewSubtitlesHandler(subsAggregator, log)`.
- Pass to `transport.NewRouter`.

### Claude's Discretion
- Exact JSON shape of OpenSubtitles' `attributes` parsing — implementer maps to flat `SubtitleEntry`.
- Whether to expose a per-provider error breakdown — for v0.1 the comma-separated `X-Subtitle-Providers-Down` header is enough.
- Movie vs TV detection — use `anime.Kind == "movie"` to skip episode/season fields.
- Whether to expose `episode` as path or query — query (matches SPEC).
- Whether to log every dedupe collision — too noisy; only log per-provider failures.

</decisions>

<code_context>
## Existing Code Insights

### Reusable assets
- `services/catalog/internal/parser/jimaku/client.go` — existing JP subtitle parser; `SearchByAnilistID` + `GetFiles(entryID, episode)`.
- `libs/idmapping/client.go` — ARM client with `ResolveByShikimoriID` returning `{AniList, MAL, AniDB, Kitsu, LiveChart, IMDB}` — note: IMDB string already present in MappingResult but populated only when ARM has it (Kitsu mapping is more reliable for IMDb).
- `libs/errors` — `Wrap(err, code, msg)`, `ServiceUnavailable`, `RateLimited` — used by handlers and service layer.
- `libs/cache` — Redis cache.
- `libs/logger` — structured logging.
- `services/catalog/internal/repo/anime.go` — repository with `UpdateAniListID`, `SetHasRaw`, etc.

### Established patterns
- Parsers are pure HTTP clients in `services/catalog/internal/parser/{name}/client.go`.
- Service layer in `services/catalog/internal/service/` orchestrates parsers + cache + DB.
- Handler layer in `services/catalog/internal/handler/` parses URL params and delegates to the service.
- Router in `services/catalog/internal/transport/router.go` is the single registration point.
- Domain models in `services/catalog/internal/domain/anime.go` are the GORM-tagged source of truth.

### Integration points
- Router: insert subtitles routes near existing `/jimaku/subtitles` route.
- Main.go: instantiate after `jimakuClient` and `idMappingClient` (which the existing `CatalogService` already uses).
- `idMappingClient` is currently scoped inside `CatalogService` — need to surface it to main scope or instantiate independently.

</code_context>

<specifics>
## Specific Ideas

- Bocchi the Rock (Shikimori/MAL 52082) is the canonical test anime — SPEC acceptance #6 / #7.
- OpenSubtitles v1 docs: `https://opensubtitles.stoplight.io/docs/opensubtitles-api`.
- Kitsu mappings: GET `/anime/{id}?include=mappings` returns JSON:API with `included` array of resources; each has `attributes.external_site` and `attributes.external_id`.
- IMDb IDs are strings prefixed `tt` (e.g. `tt15302498` for Bocchi). TMDB IDs are integers but we store them as strings for consistency.

</specifics>

<deferred>
## Deferred Ideas

- Kage Project / fansubs.ru integration — Cloudflare-protected, too much friction for v0.1.
- Subtitle content download/caching server-side — frontend handles the actual fetch.
- Subtitle quality scoring / auto-pick best track — v0.4+.
- User-uploaded subtitles — out of scope for all v0.x.
- Per-provider error breakdown in response body — `X-Subtitle-Providers-Down` header is sufficient.
- Fuzzy dedupe across languages (same .srt translated to multiple languages by upload) — not worth the complexity for v0.1.

</deferred>

<canonical_refs>
## Canonical References

- `.planning/workstreams/raw-jp/milestones/v0.1-phases/02-subtitle-aggregator/02-SPEC.md` — Locked requirements; MUST READ.
- `docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md` — Design doc (architecture, error-handling table).
- `.planning/workstreams/raw-jp/milestones/v0.1-REQUIREMENTS.md` — RAW-03, RAW-04, RAW-NF-01.
- `services/catalog/internal/parser/jimaku/client.go` — existing JP subtitle parser to reuse.
- `libs/idmapping/client.go` — ARM client to extend.
- `services/catalog/internal/transport/router.go` — register new routes.
- `services/catalog/internal/repo/anime.go` — extend with `UpdateExternalIDs`.

</canonical_refs>
