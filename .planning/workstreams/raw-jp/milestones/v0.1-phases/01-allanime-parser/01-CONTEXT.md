# Phase 1: AllAnime Parser — Context

**Gathered:** 2026-05-18
**Status:** Ready for planning
**Mode:** `--auto` (decisions derived from SPEC.md auto-selected implementation choices; ambiguity_score: 0.15)

<spec_lock>
## Locked Requirements (from SPEC.md)

Requirements are LOCKED by `milestones/v0.1-phases/01-allanime-parser/01-SPEC.md`. Downstream agents MUST read SPEC.md before planning. Three requirements covered: RAW-01 (parser module), RAW-02 (catalog endpoints + resolver), RAW-NF-01 (error-handling convention).

Acceptance criteria (verbatim from SPEC.md):
1. `services/catalog/internal/parser/allanime/` exists with `client.go`, `queries.go`, `episodes.go`, `domains.go`.
2. `docker/.env.example` documents three new env vars (`ALLANIME_QUERY_SEARCH_SHA`, `ALLANIME_QUERY_EPISODES_SHA`, `ALLANIME_QUERY_SOURCES_SHA`).
3. `docker/.env` (not committed) has values populated from a live network capture against `allmanga.to`.
4. `services/catalog/internal/domain/anime.go` includes `SourceTypeRaw = "raw"` constant.
5. `GET /api/anime/52082/raw/episodes` returns 200 with non-empty episodes array.
6. `GET /api/anime/52082/raw/stream?episode=1` returns 200 with valid HLS URL.
7. With `ALLANIME_QUERY_SEARCH_SHA=invalid`, the same endpoint returns 503 with body `{"error": "raw provider unavailable"}` (not 500).
8. Stream URL cache hit on second call within 1h verified by Prometheus counter or log line.
9. Unit tests against fixtures pass. Integration tests gated on `INTEGRATION=1` pass against live API.
</spec_lock>

<domain>
## Phase Boundary

A new catalog parser at `services/catalog/internal/parser/allanime/` that queries AllAnime's GraphQL API with `translationType: raw` to surface original Japanese audio streams. Two new HTTP endpoints — `GET /api/anime/{shikimori_id}/raw/episodes` and `GET /api/anime/{shikimori_id}/raw/stream?episode={n}&quality={q}` — expose episodes and HLS stream URLs to the frontend.

Backend-only — no UI, no frontend wiring (deferred to Phase 3/4). No subtitle aggregation (Phase 2). Additive: does not modify existing parsers.

</domain>

<decisions>
## Implementation Decisions

### Parser module layout
- Mirror the structure of `services/catalog/internal/parser/hianime/` (the closest analog: HLS-streaming GraphQL parser).
- Four Go files: `client.go` (Client struct, NewClient, Config), `queries.go` (persisted-query SHA constants + payload builders), `episodes.go` (Search, EpisodesByID, RawStream methods), `domains.go` (rotating-domain helper with first-success caching).
- Domain types: `Config`, `Client`, `SearchResult`, `Episode`, `Stream`, `Subtitle` (signatures locked in SPEC.md RAW-01).

### Domain rotation
- Static list `[allanime.day, allmanga.to, allanime.to]` (env-overridable via `ALLANIME_DOMAINS` comma-separated).
- First-success caching in-memory for process lifetime — store on the `Client` struct, not in Redis/DB.
- Fail-over: 5-minute cooldown on the cached domain before re-checking the full list on the next request.
- No persistent disk cache (domains rotate slowly; a process restart will re-discover in seconds).

### HTTP client
- Standard `net/http` with `Timeout: 10s` per request.
- No third-party GraphQL library — direct `POST` to `/api` with hand-crafted JSON body matching AllAnime's persisted-query shape (`{query, variables, extensions:{persistedQuery:{sha256Hash:...}}}`).
- Headers: `Referer: https://allmanga.to/`, `User-Agent: AnimeEnigma/1.0`. Both env-overridable (`ALLANIME_REFERER`, `ALLANIME_USER_AGENT`).

### Persisted-query SHA hashes
- Stored in env vars: `ALLANIME_QUERY_SEARCH_SHA`, `ALLANIME_QUERY_EPISODES_SHA`, `ALLANIME_QUERY_SOURCES_SHA`.
- Documented in `docker/.env.example` with placeholder hashes + a comment pointing at the AllAnime web client as the source (capture from devtools).
- Real values populated in `docker/.env` (gitignored) by the implementer.
- Never hardcoded in Go source.

### Error handling
- All AllAnime parser failures wrap with `libs/errors` helpers — never bare `fmt.Errorf`.
- All-domains-timeout → `errors.Unavailable("allanime: all domains unreachable", err)`.
- GraphQL 4xx response (likely stale SHA) → `errors.Unavailable("allanime: query rejected (likely stale SHA)", err)`.
- Empty `data.shows.edges` for a search → `errors.NotFound("allanime: no match for query")`.
- HTTP 5xx upstream → `errors.Unavailable` (NEVER propagated as 500).
- Handler maps `errors.Unavailable` → HTTP 503 with body `{"error": "raw provider unavailable"}`.

### Catalog endpoints
- Two new routes registered in `services/catalog/internal/transport/router.go`:
  - `GET /api/anime/{shikimori_id}/raw/episodes`
  - `GET /api/anime/{shikimori_id}/raw/stream?episode={n}&quality={q}` (quality optional)
- Backed by `services/catalog/internal/handler/raw.go` (mirrors `hianime.go` handler shape).
- Resolution flow: shikimori_id → fetch Anime row from DB (lookup or populate via Shikimori) → use `russian` or `japanese` title to call AllAnime `Search` → take first hit → call `EpisodesByID` / `RawStream`.

### Raw resolver service
- New file `services/catalog/internal/service/raw_resolver.go`.
- Wraps the AllAnime client + cache.
- Standard 1-hour cache pattern: `cache.Set(ctx, "raw:stream:"+animeID+":"+episode, stream, time.Hour)` (URLs expire upstream).
- Episode lists cached longer (6h) since they change less frequently: `"raw:episodes:"+animeID`.

### Quality selection
- Return all qualities from the upstream response; let the frontend pick.
- No server-side quality preference / no auto-selection of "best" quality.

### Subtitle extraction
- Best-effort — if AllAnime returns subtitle URLs in the stream response, surface them in `Stream.Subtitles`.
- Do NOT block stream resolution on subtitle parsing failures (log a warning, return the stream with empty subtitle list).
- Phase 2's subtitle aggregator is the primary source of subtitles; AllAnime's embedded subs are a bonus.

### Domain extensions
- `services/catalog/internal/domain/anime.go`: add `SourceTypeRaw = "raw"` constant alongside existing `SourceTypeKodik`, `SourceTypeAnimelib`, `SourceTypeHianime`, `SourceTypeConsumet`.

### Config loading
- `services/catalog/internal/config/config.go`: add `AllAnime AllAnimeConfig` substruct with `Domains []string`, `QuerySearchSHA string`, `QueryEpisodesSHA string`, `QuerySourcesSHA string`, `HTTPTimeout time.Duration`, `Referer string`, `UserAgent string`.
- Env vars loaded via existing `envconfig` pattern.

### Testing
- Unit tests in `services/catalog/internal/parser/allanime/*_test.go` against recorded GraphQL fixtures (JSON files in `testdata/`).
- Integration tests gated on `INTEGRATION=1` env var, hit `api.allanime.day` for Bocchi the Rock (MAL 52082), expect ≥10 episodes + valid HLS URL.
- Mock external API in unit tests via `httptest.Server`.

### Claude's Discretion
- Exact fixture data files in `testdata/` — choose representative responses from a live capture.
- Internal logger usage — follow `libs/logger` conventions used by sibling parsers.
- Metric/counter names for cache hit observability — use the existing convention (`raw_stream_cache_total{hit="true"|"false"}`).
- Whether the all-domain probe is parallel or serial — implementer's call (serial is simpler; parallel is faster).

</decisions>

<code_context>
## Existing Code Insights

### Reusable assets
- `libs/errors/errors.go` — `Wrap`, `NotFound`, `Unavailable` (need to confirm last exists; if not, add it during planning).
- `libs/cache` — Redis cache with `Set`, `Get`, key helpers; use `time.Hour` TTL for stream URLs.
- `libs/logger` — `Infow`, `Errorw` structured logging used by all sibling parsers.
- `services/catalog/internal/parser/hianime/` — closest existing analog (GraphQL-ish HTTP client returning HLS). Mirror file layout.
- `services/catalog/internal/transport/router.go` — chi router; register new routes alongside existing `/api/anime/{id}/...` patterns.
- `services/catalog/internal/handler/anime.go` and per-provider handlers (`hianime.go`, `consumet.go`) — shape of provider-specific handlers.

### Established patterns
- Parser layer: pure HTTP client, no DB access, no caching (caching lives in the service layer).
- Service layer: orchestrates parser + cache + DB lookups; returns domain types.
- Handler layer: parses URL params + query string, calls service, maps domain errors to HTTP statuses.
- Config: `envconfig`-style struct per service, loaded once at startup.
- Error wrapping: `libs/errors.Wrap(err, "operation context")` everywhere.

### Integration points
- Router registration: `services/catalog/internal/transport/router.go` — add two new route lines.
- Service wiring: `services/catalog/cmd/catalog-api/main.go` — instantiate AllAnime client, raw resolver service, raw handler; inject into router.
- Config: `services/catalog/internal/config/config.go` + `docker/.env.example` + `docker/.env`.
- Domain constant: `services/catalog/internal/domain/anime.go` — single-line addition.

</code_context>

<specifics>
## Specific Ideas

- Bocchi the Rock (Shikimori/MAL 52082) is the canonical "known-good" test anime for AllAnime — used in acceptance criteria #5/#6.
- Reference implementations to study for the persisted-query GraphQL pattern:
  - `justfoolingaround/animdl` (Python)
  - `sdaqo/anipy-cli` (Python)
  - `pystardust/ani-cli` (shell)
- The HiAnime parser's `Type string // "sub", "dub", "raw"` field already includes "raw" — AllAnime fills the gap that HiAnime's `raw` branch was meant to cover (HiAnime container has been returning HTTP 500 since the March 2026 takedown).

</specifics>

<deferred>
## Deferred Ideas

- Frontend integration (RawPlayer.vue, provider chip in Anime.vue) — Phase 3 + Phase 4.
- Subtitle aggregation (OpenSubtitles, "Other subs" panel) — Phase 2.
- Self-hosted MinIO library fallback — v0.2 milestone.
- Replacing the dead HiAnime parser — separate workstream (not raw-jp's concern).
- SHA-hash auto-discovery (scraping AllAnime web client at startup to refresh hashes) — explicitly NOT in v0.1; manual env-var update is acceptable for a small-group self-hosted deployment.

</deferred>

<canonical_refs>
## Canonical References

Downstream agents (researcher, planner, executor) MUST read these before acting:

- `.planning/workstreams/raw-jp/milestones/v0.1-phases/01-allanime-parser/01-SPEC.md` — Locked requirements; MUST READ.
- `docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md` — Source design doc (tech-choices table, error-handling table, env-var examples).
- `.planning/workstreams/raw-jp/milestones/v0.1-REQUIREMENTS.md` — Workstream-level acceptance criteria for RAW-01, RAW-02, RAW-NF-01.
- `.planning/workstreams/raw-jp/PROJECT.md` — Workstream vision + out-of-scope list.
- `services/catalog/internal/parser/hianime/` — Closest existing analog; mirror file layout.
- `services/catalog/internal/parser/consumet/` — Secondary analog (HLS + handler shape).
- `services/catalog/internal/handler/anime.go` — Handler conventions; map domain errors to HTTP status codes.
- `services/catalog/internal/transport/router.go` — Where new routes register.
- `services/catalog/internal/config/config.go` — Where new env vars load.
- `services/catalog/internal/domain/anime.go` — Where `SourceTypeRaw` constant goes.
- `libs/errors/` — Wrap / NotFound / Unavailable helpers.
- `libs/cache/` — Cache helpers + TTL conventions.
- `.planning/codebase/CONVENTIONS.md` and `.planning/codebase/STACK.md` — Project-wide patterns.

</canonical_refs>
