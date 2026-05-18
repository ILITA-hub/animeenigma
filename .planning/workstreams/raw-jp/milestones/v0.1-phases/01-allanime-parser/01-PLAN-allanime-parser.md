# Plan 01: AllAnime Parser

**Phase:** 1 — AllAnime Parser (workstream `raw-jp`, milestone v0.1)
**Created:** 2026-05-18
**Status:** Ready for execution

> Reads `01-SPEC.md` for locked requirements and `01-CONTEXT.md` for implementation decisions.

## Tasks (atomic; one commit each)

1. **Domain constant**
   - Add `SourceTypeRaw SourceType = "raw"` to `services/catalog/internal/domain/anime.go`.
   - Add `HasRaw bool \`gorm:"default:false;index;column:has_raw"\` json:"has_raw"` on `Anime` mirroring `HasHiAnime` / `HasConsumet`.

2. **Parser module skeleton** (`services/catalog/internal/parser/allanime/`)
   - `client.go` — Client struct, NewClient, Config struct, public type signatures (SearchResult, Episode, Stream, Subtitle).
   - `domains.go` — Rotating-domain helper with first-success caching (in-memory, 5-min cooldown).
   - `queries.go` — Persisted-query SHA constants (from env) + JSON payload builders.
   - `episodes.go` — Search, EpisodesByID, RawStream methods.

3. **Parser unit tests** (`services/catalog/internal/parser/allanime/client_test.go`)
   - Mock AllAnime upstream via `httptest.Server`.
   - Verify Search returns parsed results.
   - Verify EpisodesByID returns ordered episode list.
   - Verify RawStream returns HLS URL + subtitles.
   - Verify domain rotation: first domain 5xx, second returns 200.
   - Verify GraphQL 4xx → wrapped error.

4. **Config wiring**
   - Add `AllAnimeConfig` substruct to `services/catalog/internal/config/config.go` with `Domains []string`, three SHA fields, `HTTPTimeout`, `Referer`, `UserAgent`.
   - Load from env: `ALLANIME_DOMAINS`, `ALLANIME_QUERY_SEARCH_SHA`, `ALLANIME_QUERY_EPISODES_SHA`, `ALLANIME_QUERY_SOURCES_SHA`, `ALLANIME_HTTP_TIMEOUT`, `ALLANIME_REFERER`, `ALLANIME_USER_AGENT`.
   - Document in `docker/.env.example` with realistic defaults.

5. **Raw resolver service** (`services/catalog/internal/service/raw_resolver.go`)
   - `RawResolver` struct wrapping the AllAnime client + cache + animeRepo + logger.
   - `GetEpisodes(ctx, animeID)` — Resolve anime → search AllAnime → cache 6h.
   - `GetStream(ctx, animeID, episode, quality)` — Resolve anime → search → episodes → stream → cache 1h.
   - On all failures, wrap with `errors.ServiceUnavailable(...)` so the handler maps to 503.
   - Use `metrics.ObserveParser("allanime", ...)` and increment `metrics.EpisodeStreamRequestsTotal.WithLabelValues("raw")`.
   - Backfill `anime.HasRaw = true` on first successful resolution.

6. **Raw handler** (`services/catalog/internal/handler/raw.go`)
   - `RawHandler` struct wrapping `*service.RawResolver` + logger.
   - `GetEpisodes` — `GET /api/anime/{animeId}/raw/episodes` → JSON `{episodes:[], available:bool, source:"allanime"}`.
   - `GetStream` — `GET /api/anime/{animeId}/raw/stream?episode={n}&quality={q}` → JSON `{url, subtitles, quality, expires_at, type}`.
   - Map `errors.AppError` correctly via `httputil.Error` (already handles Unavailable → 503).

7. **Router registration**
   - Update `services/catalog/internal/transport/router.go` `NewRouter` signature: accept `rawHandler *handler.RawHandler`.
   - Register two new routes under `/api/anime/{animeId}/raw/*`.

8. **Service wiring in main.go**
   - Instantiate AllAnime `Config` from `cfg.AllAnime` substruct.
   - Build `allanime.NewClient(cfg)`.
   - Build `service.NewRawResolver(allanimeClient, animeRepo, redisCache, log)`.
   - Build `handler.NewRawHandler(rawResolver, log)`.
   - Pass to `transport.NewRouter`.

9. **docker/.env populate**
   - Add the three persisted-query SHA hashes to `docker/.env` (not committed). Use the public SHAs from `ani-cli` / `animdl` as starting point — the implementer's network-capture step is documented in the SPEC but I'll use the published hashes from upstream reference projects which are stable enough to verify with.

10. **End-to-end smoke test**
   - `make redeploy-catalog`.
   - `curl http://localhost:8000/api/anime/{uuid}/raw/episodes` returns 200 + episodes.
   - `curl http://localhost:8000/api/anime/{uuid}/raw/stream?episode=1` returns 200 + HLS URL.
   - `curl` with bad SHA (mutate env) returns 503 + `{"error":"..."}` body.

## Out of scope (deferred)

- Frontend integration (Phase 3, 4).
- Subtitle aggregation (Phase 2).
- Production-grade fixture corpus (smoke test on live API is enough for v0.1).

## Risks / Open Questions

- **SHA staleness:** Persisted-query hashes rotate. If `ani-cli`-published values are stale, smoke test will fail with 503. Mitigation: env-overridable, redeploy with new value.
- **Domain block:** If all three domains are blocked from this network, smoke test fails. Mitigation: log the rotation attempts so the operator sees which to whitelist.
- **Bocchi the Rock (MAL 52082) coverage:** SPEC's canonical test anime — assumed to be on AllAnime. If not, fall back to a different known-good title at smoke-test time.
