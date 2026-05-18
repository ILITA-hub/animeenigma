---
phase: 06-hybrid-resolver
status: complete
workstream: raw-jp
milestone: v0.2
date: 2026-05-18
requirements: [LIB-10]
commits:
  - 575a430 feat(raw-jp 06): catalog library client with 2s timeout + 200/404/5xx/timeout coverage
  - 88fb7a9 feat(raw-jp 06): hybrid resolver branch + RawStream.Source + table-driven coverage
  - 5be82bd feat(raw-jp 06): internal cache-invalidation handler + router mount
  - 375b733 feat(raw-jp 06): library catalog-invalidator + webhook fire from encoder + library_cache_invalidation_total metric
  - 1b2e5ff feat(raw-jp 06): config + main wiring for hybrid resolver + webhook
---

# Phase 06 (workstream raw-jp / v0.2): Hybrid Resolver Summary

**One-liner:** Catalog's `/raw/stream` now prefers the self-hosted library (MinIO HLS, `source:"library"`) over AllAnime, falling back transparently with `source:"allanime"` on library miss / outage / timeout — and a library-side webhook busts catalog cache the moment a new encode lands.

## What Was Built

End-to-end hybrid resolver path with cache invalidation, integrated across two services:

1. **`services/catalog/internal/parser/library`** — new thin HTTP client. `GetEpisode(ctx, shikimoriID, episode)` returns `(*EpisodeResponse, error)` with the documented semantics: 200 → non-nil payload (validates `minio_url != ""`), 404 → `(nil, nil)`, 5xx/timeout/transport error → wrapped error never silently downgraded. Per-request timeout defaults to 2s. `Ping(ctx)` is exposed for an external health-check goroutine (not on the request path).

2. **`services/catalog/internal/service/raw_resolver.go`** — extended with:
   - New optional `library *library.Client` field (nil-safe).
   - `RawStream.Source string` JSON field with values `"library" | "allanime"`. Older v0.1 cached entries that lack the field decode with `Source == ""` and are normalized to `"allanime"` on read.
   - Library-first branch in `GetStream`: reads `raw:source-decision:{animeID}:{episode}` cache; on cache miss or "library", calls library and (200 → cache "library" + return MinIO URL with `source:"library"`; 404 → cache "allanime" + fall through; 5xx/timeout → no cache write, fall through). AllAnime path otherwise unchanged.
   - Lifted cache-key constants (`CacheKeySourceDecision`, `CacheKeyStream`, `CacheKeyEpisodes`) so the invalidation handler builds patterns from a single source of truth.

3. **`services/catalog/internal/handler/internal_cache.go`** — `POST /internal/cache/invalidate/raw/{shikimoriId}` mounted **outside `/api`** (no AuthMiddleware) on the catalog router. Strict `^[a-zA-Z0-9_-]+$` regex on the path param, looks up animeID via `GetByShikimoriID`, deletes the three raw:* families (SCAN+DEL for the `*` patterns, exact DEL for `raw:episodes:`). Idempotent on unknown shikimori_id (200 + `found:false`).

4. **`services/library/internal/service/catalog_invalidator.go`** — best-effort webhook client. `Invalidate(ctx, shikimoriID)` POSTs to `{base}/internal/cache/invalidate/raw/{shikimoriID}` (PathEscape on the ID, default 3s timeout). Never returns an error to caller; never fails the encoder. Empty `CatalogInternalAPIURL` yields a no-op invalidator (1h TTL preserves correctness). 2xx → `IncCacheInvalidation("ok")`; non-2xx / transport error / timeout → `IncCacheInvalidation("fail")`.

5. **`services/library/internal/service/encoder_worker.go`** — `EncoderPool` gains a nil-safe `invalidator CatalogInvalidator` field; `processJob` fires the webhook AFTER `status=done` is committed, only when `ShikimoriID != ""`.

6. **`services/library/internal/metrics/library_metrics.go`** — new `library_cache_invalidation_total{result="ok"|"fail"}` counter + `IncCacheInvalidation` method + the `ForTest` test seam.

7. **Config + main wiring** — `LibraryConfig{APIURL, Timeout}` on catalog (defaults `http://library:8089` + 2s); `CatalogInternalConfig{APIURL, Timeout}` on library (defaults `http://catalog:8081` + 3s). Both services build, vet, and unit-test cleanly with `-race`.

## Files Touched

**New (8):**
- `services/catalog/internal/parser/library/client.go` (170 lines)
- `services/catalog/internal/parser/library/client_test.go` (220 lines, 9 cases)
- `services/catalog/internal/service/raw_resolver_test.go` (~480 lines, 10 cases)
- `services/catalog/internal/handler/internal_cache.go` (135 lines)
- `services/catalog/internal/handler/internal_cache_test.go` (~190 lines, 5 cases)
- `services/catalog/internal/parser/allanime/testseam.go` (15 lines — one-line `SetHTTPClientForTest`)
- `services/library/internal/service/catalog_invalidator.go` (155 lines)
- `services/library/internal/service/catalog_invalidator_test.go` (~190 lines, 6 cases)

**Extended (8):**
- `services/catalog/internal/service/raw_resolver.go` (library-first branch + `Source` field + cache-key constants)
- `services/catalog/internal/transport/router.go` (new `InternalCacheHandler` parameter + `/internal/...` route mount)
- `services/catalog/cmd/catalog-api/main.go` (library client + internal handler wire-up)
- `services/catalog/internal/config/config.go` (new `LibraryConfig`)
- `services/library/internal/service/encoder_worker.go` (invalidator field + post-done fire + `NewEncoderPool` signature)
- `services/library/internal/service/encoder_worker_test.go` (+4 new tests; trailing nil added to existing call sites)
- `services/library/internal/metrics/library_metrics.go` (new counter + method + test seam)
- `services/library/internal/config/config.go` (new `CatalogInternalConfig`)
- `services/library/cmd/library-api/main.go` (catalog invalidator wire-up)
- `docker/.env.example` (Phase 06 block documenting all four env vars)

## Verification Results

### Unit tests (all pass with `-race`)
```
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/library  1.079s
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/service          1.473s
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/handler          1.041s
ok  github.com/ILITA-hub/animeenigma/services/library/internal/service          1.652s
ok  github.com/ILITA-hub/animeenigma/services/library/internal/metrics          1.020s
```

### Build + vet
```
$ go build ./services/catalog/... ./services/library/...     # clean
$ go vet   ./services/catalog/... ./services/library/...     # clean
```

### Deployment health
```
$ make redeploy-catalog && make redeploy-library && make health
✓ gateway:8000  ✓ auth:8080  ✓ catalog:8081  ✓ streaming:8082  ✓ player:8083
✓ rooms:8084  ✓ scheduler:8085  ✓ scraper:8088  ✓ library:8089
```

### Live smoke (against the deployed stack)

**1. Library hit → `source: "library"`** (used Lycoris Recoil UUID + shikimori_id 50709 + a synthetic library_episodes row at `fake-smoke-test/50709/1/`):
```
$ curl -s "http://localhost:8000/api/anime/bbed5c38-329c-43b6-93c9-1d51ec056c80/raw/stream?episode=1"
{"success":true,"data":{"url":"http://minio:9000/raw-library/fake-smoke-test/50709/1/playlist.m3u8","type":"hls","expires_at":"2026-05-18T09:51:31.672810632Z","source":"library"}}
```
✅ Pass — `data.source == "library"`, `data.url` is a MinIO HLS playlist URL.

**2. Library miss → falls through to AllAnime** (episode=2 of same anime; no library_episodes row):
```
$ curl -s "http://localhost:8000/api/anime/bbed5c38-329c-43b6-93c9-1d51ec056c80/raw/stream?episode=2"
{"success":false,"error":{"code":"UNAVAILABLE","message":"raw provider unavailable"}}

$ docker exec animeenigma-redis redis-cli GET "raw:source-decision:bbed5c38-329c-43b6-93c9-1d51ec056c80:2"
"allanime"
```
✅ Pass — library returned 404 → resolver cached `"allanime"` and fell through. AllAnime then 503'd because of ISS-012 (stale persisted-query SHAs at v0.2 ship time — documented in `docs/issues/issues.json`). The fall-through PATH is the test; AllAnime success is not in scope.

**3. Library stopped → falls back within 2.5s**:
```
$ docker exec animeenigma-redis redis-cli DEL "raw:source-decision:bbed5c38-329c-43b6-93c9-1d51ec056c80:3"
$ docker stop animeenigma-library
$ START=$(date +%s.%N); curl -s ".../raw/stream?episode=3" >/dev/null; END=$(date +%s.%N)
ELAPSED: 0.016s
$ docker exec animeenigma-redis redis-cli GET "raw:source-decision:bbed5c38-329c-43b6-93c9-1d51ec056c80:3"
(nil)
```
✅ Pass — library-down round trip < 2.5s (actually ~16 ms because the docker DNS resolution failed instantly; well under the 2s per-request cap which only kicks in if a TCP connect succeeds). Cache key NOT set (transient failure correctly bypasses caching).

**4. Webhook end-to-end (catalog invalidation endpoint)**:
```
$ docker exec animeenigma-redis redis-cli SET "raw:source-decision:bbed5c38-329c-43b6-93c9-1d51ec056c80:1" "library"
OK
$ docker exec animeenigma-library wget -qO- --post-data="" "http://catalog:8081/internal/cache/invalidate/raw/50709"
{"success":true,"data":{"found":true,"status":"ok"}}
$ docker exec animeenigma-redis redis-cli GET "raw:source-decision:bbed5c38-329c-43b6-93c9-1d51ec056c80:1"
(nil)
```
Catalog log:
```
INFO  raw: cache invalidated  {"shikimori_id": "50709", "anime_id": "bbed5c38-329c-43b6-93c9-1d51ec056c80"}
INFO  request completed {"method":"POST","path":"/internal/cache/invalidate/raw/50709","status":200}
```
✅ Pass — library → catalog network reachability verified, endpoint deletes the source-decision key, catalog logs the invalidation with both IDs.

**`library_cache_invalidation_total`:** The counter is registered but only becomes visible on `/metrics` after the first call to `IncCacheInvalidation`. The unit tests `TestCatalogInvalidator_Happy200`, `_Non2xx_Fail`, `_Timeout_Bounded` exhaustively cover both label values; a live counter increment would require a full encode pipeline run (out of smoke scope this phase — Phase 04 covered the encode → done transition end-to-end). The metric IS present in the registered counter map at process start (the test-seam `GetCacheInvalidationForTest("ok")` confirms it during unit tests).

**5. v0.1 e2e (`raw-player.spec.ts`)**:
```
$ BASE_URL=http://localhost:3003 bunx playwright test e2e/raw-player.spec.ts --project=chromium
1 skipped
```
The test self-skips via its `rawJpVisible` early-return when the RAW JP chip isn't present (current state because of ISS-012 AllAnime degradation, not a regression from this phase). The frontend contract is preserved — the new `source` field is additive and ignored by existing types/raw.ts.

**6. AllAnime degradation regression check (ISS-012)**:
The catalog's `/raw/stream` returns the same 503 + `{"code":"UNAVAILABLE"}` shape on AllAnime failure as v0.1. No regression. The Phase 06 changes are strictly additive on the success path; the failure-path shape is identical to v0.1.

### Cleanup

- Removed synthetic `library_episodes` row for `shikimori_id='50709'` post-smoke.
- Cleared all `raw:source-decision:bbed5c38-...:*` keys.

## Deviations from Plan

**None — plan executed essentially as written.** Three minor implementation choices worth documenting:

1. **`SetHTTPClientForTest` lives in `testseam.go`, not `*_test.go`** (Rule 3). Cross-package tests can't see `*_test.go` files, so the test-seam had to be in a regular file. Documented as test-only in the doc comment per project convention (mirrors `services/library/internal/metrics.GetJobsTotalForTest`).

2. **Resolver tests use real Redis (DB 14) + sqlite + skip-on-unreachable** instead of miniredis. Miniredis is not vendored in the catalog service; following the existing precedent from `libs/cache/cache_setnx_test.go`, real Redis is used with `t.Skipf` if unreachable. The catalog handler tests use a separate DB 13 to avoid cross-test pollution.

3. **No new compose-level env declarations** (per plan §"DO NOT touch docker/docker-compose.yml"). All four Phase-06 env vars have safe in-cluster defaults; documented in `.env.example` for operator visibility. Listed as an open item in case future phases want explicit compose declarations.

## Out of Scope (per SPEC §"Out of Scope")

- v0.3 auto-download for ongoings.
- Quality preference resolution.
- Re-encode / per-quality variant ladders (single 1080p-cap HLS today).
- User-facing `source` indicator chip on the player.
- Library service horizontal scaling.

## Open Items

- **Optional compose env declarations** for `LIBRARY_API_URL`, `LIBRARY_API_TIMEOUT`, `CATALOG_INTERNAL_API_URL`, `CATALOG_INTERNAL_API_TIMEOUT`. Defaults already work in-cluster; one-line follow-up if operator visibility is wanted in `docker/docker-compose.yml`.
- **ISS-012 (AllAnime SHA staleness)** continues to block the fallback path's success case. Phase 06 surface-tested the fallback PATH (library 404 → resolver tries AllAnime → returns same v0.1 503 shape). When ISS-012 is resolved (operator refresh of `ALLANIME_QUERY_*_SHA` env vars), the fall-through case will return live HLS URLs with `source:"allanime"` without any code change required here.
- **End-to-end encoder webhook smoke** (real torrent → encode → done → webhook fire → `library_cache_invalidation_total` increment) was not exercised in this phase's smoke because Phase 04 already validated the encode path end-to-end. The Phase 06 unit tests exhaustively cover the invalidator side; the integration is the simple `if p.invalidator != nil && job.ShikimoriID != ""` call site added to `processJob`.

## Self-Check: PASSED

All claimed files exist:
- `services/catalog/internal/parser/library/client.go` — FOUND
- `services/catalog/internal/parser/library/client_test.go` — FOUND
- `services/catalog/internal/service/raw_resolver_test.go` — FOUND
- `services/catalog/internal/handler/internal_cache.go` — FOUND
- `services/catalog/internal/handler/internal_cache_test.go` — FOUND
- `services/library/internal/service/catalog_invalidator.go` — FOUND
- `services/library/internal/service/catalog_invalidator_test.go` — FOUND
- `.planning/workstreams/raw-jp/phases/06-hybrid-resolver/06-SUMMARY.md` — FOUND (this file)

All claimed commits exist in `git log --oneline -10`:
- 575a430 ✓ 88fb7a9 ✓ 5be82bd ✓ 375b733 ✓ 1b2e5ff ✓
