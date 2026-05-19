# Plan 26-01 — AllAnime Scraper Provider Lift

**Status:** COMPLETED
**Requirement:** SCRAPER-HEAL-25 (backend)
**Wave:** 1 (autonomous)
**Date:** 2026-05-19

## What shipped

Lifted AllAnime from `services/catalog/internal/parser/allanime/` into
`services/scraper/internal/providers/allanime/` as the third live EN-sub
`domain.Provider`. Per CONTEXT.md D1, this is copy-with-adaptation — NOT a
move. The catalog-side parser keeps serving workstream raw-jp unchanged.

## Files created (new package)

- `services/scraper/internal/providers/allanime/doc.go` — Package doc +
  lift rationale + upstream contract notes.
- `services/scraper/internal/providers/allanime/queries.go` — Apollo APQ
  GraphQL queries + computed SHAs (lifted verbatim from catalog parser).
- `services/scraper/internal/providers/allanime/decrypt.go` — AES-256-CTR
  `tobeparsed` blob decryption (lifted verbatim — algorithm is upstream-
  defined and consumer-agnostic).
- `services/scraper/internal/providers/allanime/dto.go` — Scraper-side
  DTOs (searchShowsResponse, showResponse, episodeEnvelope, sourceURL).
  Adapted from catalog DTOs — drops raw-jp-only fields.
- `services/scraper/internal/providers/allanime/cache.go` — `cacheLayer`
  with 4 key families (show ID, episodes, servers, stream) + TTLs
  matching gogoanime's pattern.
- `services/scraper/internal/providers/allanime/client.go` — `Provider`
  struct + `New(Deps)` constructor + all 6 `domain.Provider` methods.
  Top-of-file Lift Decision Log documents what was lifted vs adapted.
- `services/scraper/internal/providers/allanime/client_test.go` — 16
  unit tests including compile-time interface assertion. No network
  calls — every test uses httptest.NewServer.
- `services/scraper/internal/providers/allanime/testdata/show_frieren.json`
- `services/scraper/internal/providers/allanime/testdata/episodes_frieren.json`
- `services/scraper/internal/providers/allanime/testdata/servers_frieren_ep1.json`

## Files modified

- `services/scraper/internal/config/config.go` — Added `AllAnimeConfig`
  type + `Config.AllAnime` field + `SCRAPER_ALLANIME_BASE_URL` env binding.
- `services/scraper/cmd/scraper-api/main.go` — Imported allanime package,
  constructed provider between animepahe and the animekai gate, updated
  Phase 19 wiring invariant's `candidateProviders` slice to include
  `"allanime"`.

## Verification

See `26-01-VERIFICATION.md`. Highlights:

- `go test ./internal/providers/allanime/... -race -count=2` → all pass.
- `make redeploy-scraper` → exit 0, container healthy.
- `curl http://localhost:8088/scraper/health` → `allanime` present in
  providers map with all 5 canonical stages.
- Live end-to-end smoke: AllAnime resolved Frieren (UUID
  `f0b40660-6627-4a59-8dcf-7ec8596b3623`) → show ID `ReHMC7TQnch3C6z8j`
  → 28 episodes returned.

## Decisions taken in flight

1. `splitEpisodeID` uses `:` separator (not `/` like the catalog parser)
   to avoid path-collision footguns in orchestrator URL routing.
2. Stage health is seeded with `Up=true` (optimistic) — opposite of
   AnimeKai's escape-hatch stub which seeds `Up=false`. AllAnime is a
   real working provider, not an escape hatch.
3. Phase 19 invariant in main.go was the wiring footgun's tripwire — it
   counted "want 0 enabled providers" because allanime wasn't in
   candidateProviders. Fixed by appending "allanime" to that slice.

## Not done

Nothing — plan completed per acceptance criteria.
