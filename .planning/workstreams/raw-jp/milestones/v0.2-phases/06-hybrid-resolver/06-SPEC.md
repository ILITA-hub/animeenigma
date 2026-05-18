---
id: LIB-hybrid-resolver
title: Hybrid resolver — prefer library MinIO over AllAnime when both exist
workstream: raw-jp
milestone: v0.2
phase: 06
created_at: 2026-05-18
status: SPEC-ready
ambiguity_score: 0.15
mode: --auto
---

# Phase 06 (workstream `raw-jp`, milestone v0.2): Hybrid Resolver — Specification

**Workstream:** `raw-jp`
**Milestone:** v0.2 Self-Hosted Library
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Requirements:** LIB-10
**Depends on:** Phase 4 (library_episodes table + MinIO state)
**Mode:** `--auto`

## Goal

Extend the v0.1 catalog raw resolver to consult the library service FIRST when resolving a stream URL. When a `library_episodes` row exists for the shikimori_id + episode, return the MinIO HLS URL with `source: "library"`. Otherwise fall back to the existing AllAnime path. Catalog continues to expose the same `/api/anime/{id}/raw/stream` endpoint shape — the frontend is unchanged.

## Background

**Today, three things are true and need to change:**

1. **The catalog's raw resolver only knows about AllAnime.** v0.1 Phase 1 hard-wired `services/catalog/internal/service/raw_resolver.go` to AllAnime. After v0.2 ships, we have a second, often-preferred source — the self-hosted library — but the resolver doesn't see it.

2. **The frontend MUST stay unchanged.** RawPlayer.vue calls `/api/anime/{id}/raw/stream?episode=N&quality=Q` and expects an HLS URL. Whether that URL points at AllAnime or MinIO is a backend concern.

3. **Failure modes need to be graceful.** Library service down → catalog must continue to use AllAnime. Library service slow → catalog can't hang on a 30s library lookup before falling back. These are operational requirements for v0.2.

**The implementation:**
- New thin client `services/catalog/internal/parser/library/client.go` — `GetEpisode(ctx, shikimoriID, episode)` calling `GET library:8087/api/library/episodes/{shikimori_id}/{episode}`. 2-second timeout per request. 404 → return `(nil, nil)`; 5xx / timeout → return wrapped error.
- Extend `raw_resolver.go` GetStream to call the library client first; on hit, return the MinIO URL. On miss / unhealthy, fall back to AllAnime.
- Cache the per-`(animeID, episode)` source decision in Redis for 1h. Cache busted via a new internal endpoint `POST /internal/cache/invalidate/raw/{shikimori_id}` that the library service hits after every `done` job.
- Add a `source` field on the stream response envelope (`"library"` | `"allanime"`); existing frontend ignores unknown fields.

## Requirements

### LIB-10: Library-first raw resolver

- **Current:** `services/catalog/internal/service/raw_resolver.go` calls only AllAnime.
- **Target:**
  - New client `services/catalog/internal/parser/library/client.go`:
    ```go
    type Client struct { /* http client + base url + 2s timeout */ }
    func NewClient(cfg Config) *Client
    type Config struct { APIURL string; Timeout time.Duration }
    type EpisodeResponse struct {
        MinIOURL    string `json:"minio_url"`
        DurationSec int    `json:"duration_sec"`
        SizeBytes   int64  `json:"size_bytes"`
    }
    func (c *Client) GetEpisode(ctx context.Context, shikimoriID string, episode int) (*EpisodeResponse, error)
    func (c *Client) Ping(ctx context.Context) error  // for the health-check short-circuit
    ```
    - 404 → `(nil, nil)` (no library episode is a legitimate state).
    - 5xx / network error / timeout → `(nil, error)` wrapped.
    - Per-request timeout: 2 seconds (the library is on the same docker network; longer means it's actually down).
  - Resolver changes in `services/catalog/internal/service/raw_resolver.go`:
    - Add `library *library.Client` field on `RawResolver`.
    - Modify `GetStream(ctx, animeID, episode, quality)`:
      1. Resolve anime row from DB (existing).
      2. If `anime.ShikimoriID != ""`, check Redis cache `raw:source-decision:{animeID}:{episode}` for a cached source. If `"library"`, fetch the library URL fresh (cached MinIO URLs can stale-but-not-expire); if `"allanime"`, skip directly to the AllAnime path.
      3. If no cache, call `library.GetEpisode(ctx, anime.ShikimoriID, episode)`:
         - 200 + non-nil → cache `"library"` for 1h, set `source: "library"`, return MinIO URL in the response.
         - 404 → cache `"allanime"` for 1h, fall through to the AllAnime path.
         - Error (timeout/5xx) → do NOT cache (transient); fall through to AllAnime.
      4. Existing AllAnime path runs with `source: "allanime"`.
    - Response shape extension: `RawStream.Source string` (json `"source"`). Frontend reads it for analytics later; no current consumer.
  - New cache invalidation endpoint in catalog:
    - `services/catalog/internal/handler/internal_cache.go` — `POST /internal/cache/invalidate/raw/{shikimoriId}`. Internal-only middleware: same `Internal` middleware that gates `/internal/resolve-api-key` (allow only from internal docker network IPs / known service host headers).
    - Handler deletes the matching cache keys: `DEL raw:source-decision:{shikimoriID}:*`, `DEL raw:stream:{shikimoriID}:*`, `DEL raw:episodes:{shikimoriID}`.
  - Library service hits the invalidation endpoint:
    - `services/library/internal/service/encoder_worker.go` — after a successful encode + upload + `library_episodes` insert, POST to `http://catalog:8081/internal/cache/invalidate/raw/{shikimori_id}`. Best-effort (log on failure; don't fail the job).
- **Acceptance:**
  1. With a `library_episodes` row for `(shikimori_id, episode)`, `GET /api/anime/{uuid}/raw/stream?episode=N` returns 200 with `source: "library"` and `url` = MinIO HLS URL.
  2. Without a row, the same request returns 200 with `source: "allanime"` (or graceful-degradation per v0.1 ISS-012 when SHA is stale).
  3. Library service stopped → request returns within 2.5s with `source: "allanime"`.
  4. After a new `library_episodes` insert, the catalog's Redis cache for the affected shikimori_id is busted (verified via `redis-cli` SCAN).
  5. Existing v0.1 e2e (`raw-player.spec.ts`) still passes — the frontend contract is preserved.

## Acceptance Criteria

1. `services/catalog/internal/parser/library/{client.go,client_test.go}` exists with the documented interface; unit tests cover 200/404/5xx/timeout paths against httptest mocks.
2. `services/catalog/internal/service/raw_resolver.go` extended with the library-first branch.
3. `services/catalog/internal/service/raw_resolver_test.go` (new) table-driven tests cover: library hit, library miss, library 5xx, library timeout, cached `"library"`, cached `"allanime"`.
4. `services/catalog/internal/handler/internal_cache.go` mounted at `POST /internal/cache/invalidate/raw/{shikimoriId}`.
5. `services/library/internal/service/encoder_worker.go` POSTs the invalidation after every successful `done`.
6. `RawStream.Source` populated in the response payload (`"library"` or `"allanime"`).
7. Live smoke against a library-populated anime: response has `source: "library"`. Stop the library service: response falls back to `source: "allanime"` within 2.5s.
8. `go build ./services/catalog/... ./services/library/...` clean. `go vet` clean.
9. v0.1's `raw-player.spec.ts` still passes (frontend contract unchanged).

## Auto-selected implementation decisions

- **Source-decision cache TTL:** 1 hour. Balances "don't double-hit two backends per stream resolve" against "admin-curated changes should show within reasonable time"; the webhook invalidation reduces the practical staleness window to seconds.
- **Cache key shape:** `raw:source-decision:{animeID}:{episode}` — keyed by animeID (UUID) not shikimoriID for symmetry with other catalog cache keys (`raw:stream:{animeID}:...` etc).
- **Invalidation endpoint auth:** Re-use the existing `Internal` middleware pattern (private to the docker network). No JWT needed — internal-service-to-service trust model matches `/internal/resolve-api-key`.
- **Webhook failure handling:** Library service logs at warn level and continues. The 1h TTL ensures eventual consistency; the webhook is just a fast path.
- **MinIO URL form:** Library service returns a full URL (`http://minio:9000/raw-library/...`) that the streaming service's existing HLS proxy can handle. No new domain allowlist needed because the proxy already trusts MinIO (existing v3.0 self-hosted videos).
- **Per-request timeout:** 2 seconds. Library is on the same docker network; 2s is generous. Hard cap prevents the frontend from hanging.
- **`Ping()` use:** Not in the request path (every stream resolve would be one extra round-trip). Used by the catalog's health-check goroutine to short-circuit calls when the library is known-down — optimization, not correctness.
- **Frontend changes:** None. RawPlayer.vue + types/raw.ts already treat unknown response fields as ignorable. Future v0.3+ might surface `source` to the user but it's out of scope.

## Touches

- **New:** `services/catalog/internal/parser/library/{client.go,client_test.go}`
- **New:** `services/catalog/internal/service/raw_resolver_test.go`
- **New:** `services/catalog/internal/handler/internal_cache.go`
- **Extend:** `services/catalog/internal/service/raw_resolver.go` (library-first branch + `Source` field)
- **Extend:** `services/catalog/internal/transport/router.go` (register internal route under the existing internal middleware mux)
- **Extend:** `services/catalog/cmd/catalog-api/main.go` (instantiate library client, inject into resolver)
- **Extend:** `services/catalog/internal/config/config.go` (new `LibraryConfig{APIURL, Timeout}`)
- **Extend:** `services/library/internal/service/encoder_worker.go` (fire invalidation webhook after `done`)
- **Extend:** `services/library/internal/config/config.go` (new `CatalogInternalConfig{APIURL}` for the webhook target)
- **Extend:** `docker/.env.example` (`LIBRARY_API_URL`, `LIBRARY_API_TIMEOUT`, `CATALOG_INTERNAL_API_URL`)

## Out of Scope (for this phase)

- v0.3 auto-download for ongoings.
- Quality preference resolution (the resolver returns the library URL as-is; quality selection lives in the player).
- Re-encoding or per-quality variant ladders (single 1080p-cap HLS today).
- User-facing `source` indicator chip on the player.
- Library service horizontal scaling (single replica; the catalog only knows one base URL).

## Citations to design doc

- Architecture → "Hybrid resolver — prefer MinIO over AllAnime when both exist".
- Data flow → "does library service have shikimori_id?  yes → return MinIO HLS URLs / no → query AllAnime via translationType:raw".
- Rollout → "v0.2 (manual library) — hybrid resolver gates on library service health; if library is unhealthy, catalog falls back to AllAnime alone".
- Error-handling → all-fail-soft behavior expected for library service outages.
