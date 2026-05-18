# Phase 6: Hybrid Resolver - Context

**Gathered:** 2026-05-18
**Status:** Ready for planning
**Mode:** Auto-generated (SPEC pre-written, ambiguity_score 0.15)

<domain>
## Phase Boundary

Extend the v0.1 catalog raw resolver to consult the library service first when resolving a stream URL. With a `library_episodes` row for `(shikimori_id, episode)`, return the MinIO HLS URL with `source: "library"`. Otherwise fall back to AllAnime. Catalog continues to expose the same `/api/anime/{id}/raw/stream` shape — frontend unchanged.

**Out of scope:** v0.3 auto-download for ongoings, quality preference resolution, re-encode variants, user-facing source indicator, library horizontal scaling.

</domain>

<decisions>
## Implementation Decisions

### Locked from SPEC (`milestones/v0.2-phases/06-hybrid-resolver/06-SPEC.md`)

- **New client:** `services/catalog/internal/parser/library/client.go` calling `library:8089/api/library/episodes/{shikimori_id}/{episode}` (NOTE: port 8089 from Phase 1 deviation, not the SPEC's 8087).
- **Per-request timeout:** 2 seconds. Library is on the same docker network.
- **404 → `(nil, nil)`** — legitimate empty state.
- **5xx / network error / timeout → `(nil, error)` wrapped** — transient.

### Resolver Changes (locked)

- Add `library *library.Client` to `RawResolver`.
- New `GetStream` flow:
  1. Resolve anime row (existing).
  2. Check Redis `raw:source-decision:{animeID}:{episode}`:
     - `"library"` → fetch library URL fresh (don't cache the MinIO URL itself).
     - `"allanime"` → skip to AllAnime path.
  3. If no cache → call `library.GetEpisode`:
     - 200 + non-nil → cache `"library"` for 1h; return MinIO URL with `source: "library"`.
     - 404 → cache `"allanime"` for 1h; fall through.
     - Error → do NOT cache (transient); fall through.
  4. AllAnime path runs with `source: "allanime"` (existing v0.1 behavior).

### Response Shape (locked)

- `RawStream.Source string` with json tag `"source"`. Values: `"library"` | `"allanime"`. Frontend ignores unknown fields (verified compatible).

### Cache Invalidation (locked)

- New endpoint `POST /internal/cache/invalidate/raw/{shikimoriId}` on catalog (gated by existing `Internal` middleware — internal docker network only).
- Handler DELs:
  - `raw:source-decision:{shikimoriID}:*`
  - `raw:stream:{shikimoriID}:*`
  - `raw:episodes:{shikimoriID}`
- Library's encoder worker fires this webhook after every successful `done` (best-effort; log on failure).

### Configuration (locked)

- Catalog: `LIBRARY_API_URL=http://library:8089` (port from Phase 1 deviation), `LIBRARY_API_TIMEOUT=2s`.
- Library: `CATALOG_INTERNAL_API_URL=http://catalog:8081`.

### Cache TTL (locked)

- 1 hour for source-decision. Webhook invalidation reduces practical staleness to seconds.

### Internal middleware reuse (locked)

- Same `Internal` middleware that gates `/internal/resolve-api-key`. No JWT needed; internal-service-to-service trust.

### Ping (locked)

- `Client.Ping(ctx)` used by health-check goroutine (optimization, not correctness). NOT in request path.

### Claude's Discretion (autonomous mode)

- Internal helper signatures.
- Exact Redis SCAN approach for cache invalidation (use SCAN + UNLINK or DEL with pattern via Lua, depending on existing cache lib helpers).
- Whether to inject the webhook URL into the encoder_worker via DI or read at call site (favor DI for testability).
- Test fixture structure for raw_resolver_test.go.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/catalog/internal/service/raw_resolver.go` — v0.1 resolver (only knows AllAnime).
- `services/catalog/internal/parser/allanime/` — existing parser pattern (Client + Config + interface).
- `services/catalog/internal/handler/` — handler shape reference.
- `services/catalog/internal/transport/router.go` — admin/internal middleware mounts.
- `services/catalog/cmd/catalog-api/main.go` — wire library client into resolver.
- `services/catalog/internal/config/config.go` — extend with LibraryConfig.
- `libs/cache/` — Redis cache helpers (key namespace helpers).
- `services/library/internal/service/encoder_worker.go` — Phase 4 worker; extend with webhook fire after `done`.
- `services/library/internal/config/config.go` — extend with CatalogInternalConfig.
- `libs/httputil/` — response envelope (compatible with existing v0.1 catalog endpoints).

### Established Patterns

- HTTP clients: `net/http` + explicit timeout + wrapped errors.
- Tests: httptest mocks + table-driven cases for happy/error paths.
- Cache namespace: `raw:*` keys for v0.1 raw resolver (existing). Reuse pattern for source decision.
- Internal middleware: existing `Internal` middleware on `/internal/*` routes.

### Integration Points

- Catalog: extend `raw_resolver.go`, register internal route, wire library client.
- Library: extend encoder_worker.go with webhook fire, add CatalogInternalConfig.
- `docker/.env.example`: document `LIBRARY_API_URL`, `LIBRARY_API_TIMEOUT`, `CATALOG_INTERNAL_API_URL`.

</code_context>

<specifics>
## Specific Ideas

- SPEC reference at `milestones/v0.2-phases/06-hybrid-resolver/06-SPEC.md` is authoritative.
- The SPEC's `library:8087` port reference is stale — use 8089 (Phase 1 deviation).
- v0.1 e2e `raw-player.spec.ts` must keep passing (frontend contract preservation).
- Live verification: with a `library_episodes` row, `GET /api/anime/{uuid}/raw/stream?episode=N` returns `source: "library"`. Stop library service → falls back within 2.5s.

</specifics>

<deferred>
## Deferred Ideas

- v0.3 auto-download for ongoings.
- Quality preference resolution.
- Per-quality variant ladders.
- User-facing source indicator.
- Library horizontal scaling.

</deferred>
