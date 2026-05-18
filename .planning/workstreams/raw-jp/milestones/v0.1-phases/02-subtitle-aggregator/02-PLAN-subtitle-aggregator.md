# Plan 02: Subtitle Aggregator + Extended ID Mapping

**Phase:** 2 — Subtitle Aggregator + Extended ID Mapping (workstream `raw-jp`, milestone v0.1)
**Created:** 2026-05-18
**Status:** Ready for execution

## Tasks

1. **idmapping/kitsu.go** — `KitsuMappings(ctx, kitsuID int) (ExtraIDs, error)` resolving IMDb/TMDB via Kitsu JSON:API.

2. **opensubtitles parser** — `services/catalog/internal/parser/opensubtitles/client.go` + `client_test.go` against httptest mocks.

3. **Domain extension** — `Anime.IMDbID *string`, `Anime.TMDBID *string`.

4. **Repo helper** — `AnimeRepository.UpdateExternalIDs(ctx, animeID, imdb, tmdb)`.

5. **Subs aggregator service** — `services/catalog/internal/service/subs_aggregator.go` with `FetchAll`, errgroup fan-out, dedupe, ISO 639-1 normalization, lazy IMDb/TMDB backfill.

6. **Handler** — `services/catalog/internal/handler/subtitles.go` with `Get` and `GetAll`.

7. **Config** — `OpenSubtitlesConfig` substruct + env loaders.

8. **Router** — register routes, extend `NewRouter` signature.

9. **main.go** — instantiate OpenSubtitles client, aggregator, handler; pass to router. Add `Anime.IMDbID`/`TMDBID` columns via AutoMigrate.

10. **docker/.env.example** — document `OPENSUBTITLES_*` env vars.

11. **Build + unit tests** — `go test ./internal/parser/opensubtitles/...` green, full `go build ./...` clean, `go vet ./...` clean.

## Out of scope

- Frontend integration (Phase 3+).
- Live OpenSubtitles smoke test (requires real API key in user's env).
