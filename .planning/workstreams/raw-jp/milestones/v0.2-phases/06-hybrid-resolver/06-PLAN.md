---
phase: 06-hybrid-resolver
plan: 01
type: execute
wave: 1
workstream: raw-jp
milestone: v0.2
depends_on: []
files_modified:
  - services/catalog/internal/parser/library/client.go
  - services/catalog/internal/parser/library/client_test.go
  - services/catalog/internal/service/raw_resolver.go
  - services/catalog/internal/service/raw_resolver_test.go
  - services/catalog/internal/handler/internal_cache.go
  - services/catalog/internal/handler/internal_cache_test.go
  - services/catalog/internal/transport/router.go
  - services/catalog/internal/config/config.go
  - services/catalog/cmd/catalog-api/main.go
  - services/library/internal/service/encoder_worker.go
  - services/library/internal/service/encoder_worker_test.go
  - services/library/internal/service/catalog_invalidator.go
  - services/library/internal/service/catalog_invalidator_test.go
  - services/library/internal/metrics/library_metrics.go
  - services/library/internal/config/config.go
  - services/library/cmd/library-api/main.go
  - docker/.env.example
autonomous: true
requirements: [LIB-10]
must_haves:
  truths:
    - "When a library_episodes row exists for (shikimori_id, episode), GET /api/anime/{uuid}/raw/stream?episode=N returns source: \"library\" with a MinIO HLS URL"
    - "When no library row exists, the same request falls back to AllAnime and returns source: \"allanime\""
    - "When the library service is unreachable (stopped / timeout), the request still returns within 2.5s with source: \"allanime\""
    - "After a successful library encode (status=done), the library service POSTs the cache-invalidation webhook to catalog and the affected shikimori_id's catalog cache keys are deleted"
    - "v0.1 e2e raw-player.spec.ts continues to pass — frontend contract unchanged (Source field is additive and ignored by existing types/raw.ts)"
    - "library_cache_invalidation_total{result=\"ok\"|\"fail\"} metric is emitted by the library service after each webhook fire attempt"
  artifacts:
    - path: "services/catalog/internal/parser/library/client.go"
      provides: "Thin HTTP client for library:8089 episodes endpoint with 2s per-request timeout"
      exports: ["NewClient", "Config", "Client", "EpisodeResponse"]
    - path: "services/catalog/internal/parser/library/client_test.go"
      provides: "httptest mocks for 200/404/5xx/timeout paths"
    - path: "services/catalog/internal/service/raw_resolver.go"
      provides: "Extended RawResolver with library *library.Client field + source-decision cache + RawStream.Source"
    - path: "services/catalog/internal/service/raw_resolver_test.go"
      provides: "Table-driven tests covering library hit / 404 / 5xx / timeout / cached library / cached allanime"
    - path: "services/catalog/internal/handler/internal_cache.go"
      provides: "POST /internal/cache/invalidate/raw/{shikimoriId} handler"
      exports: ["InternalCacheHandler", "NewInternalCacheHandler"]
    - path: "services/library/internal/service/catalog_invalidator.go"
      provides: "Best-effort HTTP client that POSTs to catalog's invalidation endpoint + metric"
      exports: ["CatalogInvalidator", "NewCatalogInvalidator", "Config"]
  key_links:
    - from: "services/catalog/internal/service/raw_resolver.go"
      to: "services/catalog/internal/parser/library/client.go"
      via: "RawResolver.library field; GetStream calls library.GetEpisode first"
      pattern: "library\\.GetEpisode"
    - from: "services/catalog/internal/transport/router.go"
      to: "services/catalog/internal/handler/internal_cache.go"
      via: "router mounts POST /internal/cache/invalidate/raw/{shikimoriId} outside /api (no AuthMiddleware) — docker-network-only by virtue of nginx/gateway not proxying /internal/*"
      pattern: "/internal/cache/invalidate/raw/"
    - from: "services/library/internal/service/encoder_worker.go"
      to: "services/library/internal/service/catalog_invalidator.go"
      via: "EncoderPool.processJob calls invalidator.Invalidate(ctx, shikimoriID) after status=done; best-effort, never fails the job"
      pattern: "invalidator\\.Invalidate"
    - from: "services/catalog/cmd/catalog-api/main.go"
      to: "services/catalog/internal/parser/library/client.go"
      via: "main constructs library.NewClient(cfg.Library) and injects into NewRawResolver"
      pattern: "library\\.NewClient"
---

<objective>
Extend the catalog's v0.1 raw resolver (workstream raw-jp, Phase 01) to consult the library service FIRST when resolving a `/raw/stream` request. When a `library_episodes` row exists for `(shikimori_id, episode)`, return the MinIO HLS URL with `source: "library"`. Otherwise fall back to the existing AllAnime path with `source: "allanime"`. Add a cache-invalidation webhook so the library service can bust the catalog's Redis cache after every successful encode.

Purpose: Self-hosted library transparently preferred over AllAnime for shows we've encoded, without any frontend change.

Output: New library HTTP client + extended resolver + new internal cache-invalidation endpoint + library webhook fire + new Prometheus metric + config plumbing in both services + .env.example documentation. Frontend untouched.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/raw-jp/milestones/v0.2-phases/06-hybrid-resolver/06-SPEC.md
@.planning/workstreams/raw-jp/phases/06-hybrid-resolver/06-CONTEXT.md
@.planning/workstreams/raw-jp/milestones/v0.2-REQUIREMENTS.md
@.planning/workstreams/raw-jp/phases/04-ffmpeg-hls-transcoder-minio-writer/04-SUMMARY.md
@services/catalog/internal/service/raw_resolver.go
@services/catalog/internal/handler/raw.go
@services/catalog/internal/transport/router.go
@services/catalog/internal/config/config.go
@services/catalog/cmd/catalog-api/main.go
@services/catalog/internal/parser/allanime/client.go
@services/library/internal/service/encoder_worker.go
@services/library/internal/config/config.go
@services/library/internal/metrics/library_metrics.go
@services/library/cmd/library-api/main.go
@libs/cache/cache.go
@docker/.env.example

<interfaces>
<!-- Contracts the executor needs. All extracted from the codebase above. -->

From `services/catalog/internal/service/raw_resolver.go` (v0.1 — to be extended):
```go
type RawResolver struct {
    client    *allanime.Client
    animeRepo *repo.AnimeRepository
    cache     *cache.RedisCache
    log       *logger.Logger
    lookups   sync.Map
    // NEW: library *library.Client  — add in Task 2
}

type RawStream struct {
    URL       string        `json:"url"`
    Type      string        `json:"type"`
    Quality   string        `json:"quality,omitempty"`
    Subtitles []RawSubtitle `json:"subtitles,omitempty"`
    ExpiresAt time.Time     `json:"expires_at"`
    // NEW: Source string `json:"source"` — add in Task 2; values "library" | "allanime"
}

func NewRawResolver(client *allanime.Client, animeRepo *repo.AnimeRepository, redisCache *cache.RedisCache, log *logger.Logger) *RawResolver
func (r *RawResolver) GetStream(ctx context.Context, animeID string, episodeNumber int, quality string) (*RawStream, error)
```

From `libs/cache/cache.go`:
```go
func (c *RedisCache) Get(ctx, key, dest) error    // returns ErrNotFound on miss
func (c *RedisCache) Set(ctx, key, value, ttl) error
func (c *RedisCache) Delete(ctx, keys ...string) error
func (c *RedisCache) Invalidate(ctx, pattern string) error  // SCAN + DEL
func (c *RedisCache) Client() *redis.Client       // for raw SCAN if needed
```

From `services/library/internal/service/encoder_worker.go` (Phase 04):
```go
// EncoderPool.processJob currently ends after status=done is written.
// Task 4 will append a webhook fire here when job.ShikimoriID != "".
// invalidator is a new field; nil-safe (worker handles nil).
```

From `services/library/internal/metrics/library_metrics.go`:
```go
// LibraryMetrics already exposes: IncJobsTotal, ObserveEncodeDuration,
// AddUploadBytes, IncEncodeFailures.
// Task 4 adds: IncCacheInvalidation(result string) backed by a new
// counter library_cache_invalidation_total{result="ok"|"fail"}.
```

From `services/auth/internal/transport/router.go` (precedent for `/internal/*`):
```go
// Mounted OUTSIDE /api with no middleware — relies on nginx/gateway
// not proxying /internal/*. The library service reaches catalog via
// http://catalog:8081/internal/... on the docker network.
r.Post("/internal/resolve-api-key", authHandler.ResolveApiKey)
```

From catalog cache key pattern (`raw_resolver.go`):
```
raw:episodes:{animeID}              // existing
raw:mapping:{animeID}               // existing
raw:stream:{animeID}:{ep}:{quality} // existing
raw:source-decision:{animeID}:{ep}  // NEW (Task 2) — value is "library" or "allanime"
```

Note: Source-decision cache is keyed by `animeID` (UUID) per SPEC §"Auto-selected
implementation decisions" — symmetric with the other `raw:*` keys. Invalidation
must translate the inbound `{shikimoriId}` to `animeID` by looking up the anime
row, then DEL the keys for that `animeID`.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Library HTTP client + tests</name>
  <files>services/catalog/internal/parser/library/client.go, services/catalog/internal/parser/library/client_test.go</files>
  <action>
Create a new package `services/catalog/internal/parser/library` mirroring the shape of `services/catalog/internal/parser/allanime` (Config + Client + NewClient + thin domain types — no GraphQL machinery, just JSON over HTTP).

Define exactly these exports (per SPEC §LIB-10 and 06-CONTEXT.md decisions):

- `type Config struct { APIURL string; Timeout time.Duration }` — `APIURL` is the base URL such as `http://library:8089` (NO trailing slash; trim in NewClient if present). `Timeout` defaults to 2*time.Second when zero (SPEC-locked per-request cap).
- `type Client struct { /* unexported fields: cfg Config; httpClient *http.Client */ }`
- `func NewClient(cfg Config) *Client` — constructs `http.Client{Timeout: cfg.Timeout}` so the 2s cap bounds total round-trip (connect + headers + body). Trim trailing slash from `cfg.APIURL`.
- `type EpisodeResponse struct { MinIOURL string ` + "`json:\"minio_url\"`" + `; DurationSec int ` + "`json:\"duration_sec\"`" + `; SizeBytes int64 ` + "`json:\"size_bytes\"`" + ` }`
- `func (c *Client) GetEpisode(ctx context.Context, shikimoriID string, episode int) (*EpisodeResponse, error)`
- `func (c *Client) Ping(ctx context.Context) error` — health-check optimization; not on the request path.

Behavior of `GetEpisode`:
1. URL: `{APIURL}/api/library/episodes/{shikimoriID}/{episode}`. Use `net/url.PathEscape` on the shikimoriID for safety; format the int via `strconv.Itoa`.
2. Build the request with `http.NewRequestWithContext(ctx, GET, url, nil)` so caller ctx cancellation (and the client Timeout) both apply.
3. Reject empty `shikimoriID` and non-positive `episode` with a wrapped error before issuing the request (avoid leaking bad URLs upstream).
4. Status handling:
   - **200**: decode body into `*EpisodeResponse`. **Validate that `MinIOURL != ""`**; if blank, return `(nil, fmt.Errorf("library: empty minio_url in 200 body"))` — defensive against partial server bugs. The library envelope (Phase 04, `httputil.OK`) wraps the payload under `{"success":true,"data":{...}}`; decode into an envelope struct `{ Data *EpisodeResponse }` and return the inner pointer. Re-confirm the envelope shape against `services/library/internal/handler/episodes.go` — if it returns the bare object instead, decode directly. (Reading that file is part of the task.)
   - **404**: return `(nil, nil)` — legitimate empty state (no library row).
   - **5xx**: return `(nil, fmt.Errorf("library: upstream %d", status))`.
   - **other 4xx**: return `(nil, fmt.Errorf("library: unexpected status %d", status))`.
5. Transport / decode errors (including `context.DeadlineExceeded`): return wrapped error. Do NOT silently treat timeout as 404.

Behavior of `Ping`:
- GET `{APIURL}/health`, expect 200 within timeout, return wrapped error otherwise. Used by the catalog health checker (out of scope here — wired in Task 5).

`client_test.go` uses `net/http/httptest.NewServer` with table-driven cases:
- **200 happy**: server returns `{"success":true,"data":{"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8","duration_sec":10,"size_bytes":737139}}` → `GetEpisode` returns non-nil with all three fields populated.
- **200 empty minio_url**: response with `minio_url:""` → returns error.
- **404 not found**: server writes 404 → `GetEpisode` returns `(nil, nil)`.
- **500 upstream**: server writes 500 → `GetEpisode` returns `(nil, non-nil)` with message containing "upstream 500".
- **503 upstream**: similar to 500.
- **timeout**: server `time.Sleep(50ms)` before responding; client config `Timeout: 10ms` → `GetEpisode` returns error; assert via `errors.Is(err, context.DeadlineExceeded)` OR substring match on "context deadline exceeded" / "Client.Timeout" (net/http wraps differently across versions).
- **invalid args**: empty shikimoriID, episode=0, episode=-1 → returns error without hitting the test server (assert request count == 0 via an `atomic.Int32` on the handler).
- **Ping happy + Ping 503**: 200 → nil, 503 → error.
- **URL trim**: construct client with `APIURL: server.URL + "/"` (trailing slash) and assert the request path is `/api/library/episodes/X/1` (no double slash).

DO NOT depend on the Phase-04 library service binary at test time — `httptest` is sufficient.

Naming and structure must mirror existing AllAnime client conventions (lowercase package name, exported types only where needed).
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/catalog && go build ./internal/parser/library/... && go vet ./internal/parser/library/... && go test ./internal/parser/library/... -count=1 -race</automated>
  </verify>
  <done>
Package `services/catalog/internal/parser/library` exists. `client.go` exports `NewClient`, `Config`, `Client`, `EpisodeResponse`, `GetEpisode`, `Ping`. All 9+ table-driven test cases pass. `go vet` clean. No external deps beyond `net/http`, `encoding/json`, `context`, `time`, `strconv`, `net/url`, `fmt`, `strings`.

Commit message: `feat(raw-jp 06): catalog library client with 2s timeout + 200/404/5xx/timeout coverage (LIB-10)`
  </done>
</task>

<task type="auto">
  <name>Task 2: Extend RawResolver with library-first branch + RawStream.Source + tests</name>
  <files>services/catalog/internal/service/raw_resolver.go, services/catalog/internal/service/raw_resolver_test.go</files>
  <action>
Extend the v0.1 `RawResolver` in `services/catalog/internal/service/raw_resolver.go` per SPEC §LIB-10 "Resolver changes":

1. **Add field**: `library *library.Client` to `RawResolver` (import `.../parser/library`).
2. **Update constructor signature**: `NewRawResolver(client *allanime.Client, libraryClient *library.Client, animeRepo *repo.AnimeRepository, redisCache *cache.RedisCache, log *logger.Logger) *RawResolver`. Document that `libraryClient` may be nil — if nil, the library branch is skipped entirely (defensive for environments without LIBRARY_API_URL). Update the doc comment.
3. **Extend `RawStream`**: add `Source string `json:"source"`` field. Always populate ("library" | "allanime"). Keep the existing fields and JSON tags unchanged. Existing cached entries from before the field existed will decode with `Source == ""` — treat empty-string Source on a cache hit as "allanime" for backward compatibility (one-line normalization after Get).
4. **Extend `GetStream` flow** (after the existing anime row lookup, BEFORE the AllAnime path):

   ```
   sourceCacheKey := "raw:source-decision:{animeID}:{episode}"
   var sourceDecision string
   _ = r.cache.Get(ctx, sourceCacheKey, &sourceDecision)

   if r.library != nil && anime.ShikimoriID != "" && sourceDecision != "allanime" {
       // Try library when: cache says "library" OR cache is empty (and we haven't decided allanime yet).
       resp, err := r.library.GetEpisode(ctx, anime.ShikimoriID, episodeNumber)
       switch {
       case err == nil && resp != nil:
           // Library hit. Cache the decision (1h) and return MinIO URL.
           _ = r.cache.Set(ctx, sourceCacheKey, "library", time.Hour)
           out := &RawStream{
               URL: resp.MinIOURL,
               Type: "hls",
               Quality: quality,         // pass-through; library single-ladder for v0.2
               Subtitles: nil,           // library service does not return subs (Phase 04)
               ExpiresAt: time.Now().Add(time.Hour),
               Source: "library",
           }
           // No raw:stream:* cache write on library hit — MinIO URLs don't expire and are derived from a stable path.
           // (cache busts on library_episodes update via the webhook anyway)
           if !anime.HasRaw {
               _ = r.animeRepo.SetHasRaw(ctx, anime.ID, true)
           }
           return out, nil
       case err == nil && resp == nil:
           // Library 404. Cache "allanime" for 1h, fall through.
           _ = r.cache.Set(ctx, sourceCacheKey, "allanime", time.Hour)
       default:
           // Library 5xx / timeout / network error. Do NOT cache (transient). Fall through.
           r.log.Warnw("raw: library lookup failed; falling back to allanime",
               "anime_id", animeID, "shikimori_id", anime.ShikimoriID, "episode", episodeNumber, "error", err)
       }
   }
   ```

5. **AllAnime path**: existing logic unchanged EXCEPT the returned `RawStream` now sets `Source: "allanime"`. Also: when reading from the existing `raw:stream:*` cache hit at the top of GetStream, normalize `cached.Source == "" → "allanime"` before returning.
6. **Cache key constants**: add `raw:source-decision:` as a `const` near the top of the file for grep-ability.
7. **Imports**: add `"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/library"`.

**New test file** `services/catalog/internal/service/raw_resolver_test.go` (do NOT replace any existing tests in the directory — this is a NEW file). Use `httptest.NewServer` to fake BOTH the library service AND the AllAnime upstream. Reuse the existing pattern from `services/catalog/internal/parser/allanime/client_test.go` (read it for the AllAnime fake-server shape).

Use a real `cache.RedisCache` against `miniredis` (the `github.com/alicebob/miniredis/v2` package is already in the workspace — confirm via `grep -r "miniredis" go.mod` in the catalog service; if absent, use a tiny in-memory `cache.Cache` stub that satisfies the interface used by RawResolver — Get/Set only). Use a real `repo.AnimeRepository` against `sqlmock` OR a small fake that satisfies a minimal interface — favor the minimal-fake approach to avoid a new test dep. (The resolver only calls `GetByID` and `SetHasRaw`.)

Table-driven cases covering SPEC §LIB-10 acceptance:
- **library_hit_no_cache**: library returns 200 → response.Source="library", URL is MinIO URL, source-decision cache key set to "library".
- **library_404_no_cache**: library returns 404 → falls to AllAnime → response.Source="allanime", source-decision cache key set to "allanime".
- **library_5xx_no_cache**: library returns 500 → falls to AllAnime → response.Source="allanime", source-decision cache key NOT set (transient).
- **library_timeout_no_cache**: library handler sleeps past client timeout → falls to AllAnime → response.Source="allanime", source-decision cache key NOT set, total wall time < 2.5s (assert via `time.Now()` delta).
- **cached_library**: pre-seed `raw:source-decision:{id}:{ep}` = "library" → library is hit (fresh MinIO URL fetch — confirmed via fake-server request count == 1) → response.Source="library".
- **cached_allanime**: pre-seed `raw:source-decision:{id}:{ep}` = "allanime" → library is NOT hit (fake-server request count == 0) → response.Source="allanime".
- **nil_library_client**: construct resolver with `libraryClient=nil` → falls straight to AllAnime, no panic, response.Source="allanime".
- **empty_shikimori_id**: anime row has `ShikimoriID == ""` → library skipped, response.Source="allanime".
- **backward_compat_old_cached_stream**: pre-populate `raw:stream:{id}:{ep}:{q}` with a `RawStream` that lacks `Source` (omit the field at encode time) → cache hit returns response with `Source == "allanime"` (normalized).
- **library_hit_sets_has_raw**: anime row has `HasRaw=false` → library hit triggers SetHasRaw (verify via fake repo).

Each case has an `expectedSource string`, `expectedLibraryCalls int`, `expectedAllAnimeCalls int`, `preSeedKey string`, `preSeedVal string`, `libraryHandler http.HandlerFunc`, `allanimeHandler http.HandlerFunc`.

The constructor signature change cascades to **catalog-api/main.go**; that's covered in Task 5. No other call sites of NewRawResolver exist (confirmed by `grep -rn "NewRawResolver" services/`).

DO NOT change any existing AllAnime caching, deduplication via `lookups sync.Map`, or `resolveShowID` logic. Those remain bit-for-bit identical.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/catalog && go build ./... && go vet ./... && go test ./internal/service/... -count=1 -run "RawResolver|raw_resolver" -race</automated>
  </verify>
  <done>
`raw_resolver.go` has a new `library *library.Client` field, `RawStream.Source` field with json tag `"source"`, library-first branch in GetStream, source-decision cache key constants. New `raw_resolver_test.go` has all 10 table-driven cases passing. `go vet` clean. The full `services/catalog` package still builds (compile-time signature break in main.go is fixed in Task 5 — for this task, the test build is sufficient evidence the service-layer changes are correct).

Commit message: `feat(raw-jp 06): hybrid resolver branch + RawStream.Source + table-driven coverage (LIB-10)`
  </done>
</task>

<task type="auto">
  <name>Task 3: Internal cache-invalidation handler + router wire-up + tests</name>
  <files>services/catalog/internal/handler/internal_cache.go, services/catalog/internal/handler/internal_cache_test.go, services/catalog/internal/transport/router.go</files>
  <action>
Add the catalog endpoint that the library service calls to bust cache after a successful encode (SPEC §LIB-10 "New cache invalidation endpoint").

**`internal_cache.go`** — new file:
- `type InternalCacheHandler struct { cache *cache.RedisCache; animeRepo *repo.AnimeRepository; log *logger.Logger }`
- `func NewInternalCacheHandler(c *cache.RedisCache, animeRepo *repo.AnimeRepository, log *logger.Logger) *InternalCacheHandler`
- `func (h *InternalCacheHandler) InvalidateRaw(w http.ResponseWriter, r *http.Request)` — handler for `POST /internal/cache/invalidate/raw/{shikimoriId}`.

Handler behavior:
1. Read `shikimoriId` from chi URL param. Validate non-empty + alphanumeric (anti-injection: reject anything that fails `regexp.MustCompile(^[a-zA-Z0-9_-]+$)`). On invalid → 400.
2. Look up the anime row by `shikimoriID` via `animeRepo.GetByShikimoriID(ctx, shikimoriID)` (confirm method name by grepping `services/catalog/internal/repo/anime*.go`; use whatever it's actually called — likely `GetByShikimoriID` or `FindByShikimoriID`). If the row doesn't exist: 200 with `{"status":"ok","invalidated":0}` (idempotent — the encoder may finish before the anime row exists in catalog DB).
3. Build the three pattern lists and delete:
   - `raw:source-decision:{animeID}:*` — via `cache.Invalidate(ctx, pattern)` (SCAN + DEL — already implemented in libs/cache).
   - `raw:stream:{animeID}:*` — via `cache.Invalidate(ctx, pattern)`.
   - `raw:episodes:{animeID}` — exact key via `cache.Delete(ctx, key)`.
   Sum keys deleted (Invalidate doesn't return a count today — read its impl in libs/cache/cache.go and either (a) add a count-returning sibling, or (b) accept the lack of count and return `{"status":"ok"}`). Favor (b) — cleanest, no libs change. Document the decision inline.
4. Log at info level: `r.log.Infow("raw: cache invalidated", "shikimori_id", shikimoriID, "anime_id", anime.ID)`.
5. Return `httputil.OK(w, map[string]any{"status":"ok"})`.
6. On internal repo error: 500 via `httputil.Error(w, err)`.

**Router wire-up** in `services/catalog/internal/transport/router.go`:
- Add `internalCacheHandler *handler.InternalCacheHandler` to the `NewRouter(...)` parameter list (positioned alongside the other handlers; suggested placement: right after `subtitlesHandler`).
- Mount **outside** `/api`, with NO middleware beyond the global ones already in place:
  ```go
  // Internal endpoints (only reachable within Docker network — gateway/nginx
  // does NOT proxy /internal/*). Mirrors services/auth/internal/transport/router.go.
  r.Post("/internal/cache/invalidate/raw/{shikimoriId}", internalCacheHandler.InvalidateRaw)
  ```

**Tests** in `internal_cache_test.go`:
- Spin up a chi router with the route + a `miniredis`-backed `cache.RedisCache` (if miniredis present) or use the resolver-test fake. Either way, pre-seed cache keys:
  - `raw:source-decision:{animeID}:1` = "library"
  - `raw:source-decision:{animeID}:2` = "allanime"
  - `raw:stream:{animeID}:1:` = `{...}`
  - `raw:episodes:{animeID}` = `{...}`
  - And one unrelated key `raw:source-decision:OTHER:1` = "library" — must survive.
- Cases:
  - **happy**: POST with a valid shikimoriID where the anime row exists → 200; all 4 keys for `{animeID}` deleted; the OTHER key still present.
  - **unknown shikimori_id**: anime row absent → 200 idempotent (no cache reads beyond the lookup).
  - **bad shikimori_id format**: `../etc/passwd` or `id;DROP` → 400, no DB read attempted.
  - **method not allowed**: GET → 405 (chi default).
  - **internal repo error**: fake repo returns a non-not-found error → 500.

Use a minimal `AnimeRepoLike` interface with just `GetByShikimoriID` exposed via the handler's struct field (favor a small interface for testability — define it in `internal_cache.go` and let the production `*repo.AnimeRepository` satisfy it). If `GetByShikimoriID` doesn't exist on the repo, add it (read `services/catalog/internal/repo/anime*.go` first; only add if missing).

Also update `main.go`'s `NewRouter` call site — covered in Task 5 to avoid double-touching main.go in two tasks.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/catalog && go build ./internal/handler/... ./internal/transport/... && go vet ./internal/handler/... ./internal/transport/... && go test ./internal/handler/... -count=1 -run "InternalCache" -race</automated>
  </verify>
  <done>
`internal_cache.go` + `internal_cache_test.go` exist. Router registers `POST /internal/cache/invalidate/raw/{shikimoriId}` outside `/api`. All 5 test cases pass. `go vet` clean. Route precedence: chi does not collide with any existing route (the closest is `/internal/resolve-api-key` on the auth service — different binary, different port, no conflict).

Commit message: `feat(raw-jp 06): internal cache-invalidation handler + router mount (LIB-10)`
  </done>
</task>

<task type="auto">
  <name>Task 4: Library encoder_worker webhook fire + library_cache_invalidation_total metric + tests</name>
  <files>services/library/internal/service/catalog_invalidator.go, services/library/internal/service/catalog_invalidator_test.go, services/library/internal/service/encoder_worker.go, services/library/internal/service/encoder_worker_test.go, services/library/internal/metrics/library_metrics.go</files>
  <action>
Add a best-effort webhook fire from the library encoder worker so the catalog cache is busted within seconds of a new episode landing.

**`catalog_invalidator.go`** — new file:
- `type CatalogInvalidator interface { Invalidate(ctx context.Context, shikimoriID string) }` — fire-and-forget semantics: returns nothing.
- `type InvalidatorConfig struct { CatalogInternalAPIURL string; Timeout time.Duration }` — `CatalogInternalAPIURL` is the base such as `http://catalog:8081`; `Timeout` defaults to 3s when zero.
- `type HTTPCatalogInvalidator struct { /* cfg + httpClient + metrics + log */ }` (struct name distinct from the interface so test fakes can implement the interface).
- `func NewCatalogInvalidator(cfg InvalidatorConfig, metrics InvalidationMetrics, log *logger.Logger) CatalogInvalidator`.
- Declare a metrics interface local to this package: `type InvalidationMetrics interface { IncCacheInvalidation(result string) }` — satisfied by `*metrics.LibraryMetrics` (after the new method is added below).

Behavior of `Invalidate`:
1. Build URL: `{base}/internal/cache/invalidate/raw/{shikimoriID}` (PathEscape the ID).
2. POST with empty body and a context derived from the caller plus the configured Timeout (`context.WithTimeout`).
3. Read + discard the response body to allow connection reuse (`io.Copy(io.Discard, resp.Body)`).
4. On 2xx: log info, call `metrics.IncCacheInvalidation("ok")`.
5. On non-2xx or transport error: log warn (not error — webhook is best-effort), call `metrics.IncCacheInvalidation("fail")`.
6. NEVER return an error to the caller; NEVER block the encoder beyond Timeout.

If `cfg.CatalogInternalAPIURL == ""` the constructor returns a no-op invalidator (`type noopInvalidator struct{}` with `Invalidate` empty) and logs once at startup that webhooks are disabled.

**`library_metrics.go`** — extend:
- New field on `LibraryMetrics`: `cacheInvalidationTotal *prometheus.CounterVec` registered with name `library_cache_invalidation_total`, help `"Cache-invalidation webhook fires from library to catalog, labeled by result"`, label `[]string{"result"}`.
- New method `func (m *LibraryMetrics) IncCacheInvalidation(result string) { m.cacheInvalidationTotal.WithLabelValues(result).Inc() }`.
- Add a corresponding mirror in `library_metrics_test.go` (if the existing test file probes each metric — read it first to follow precedent).

**`encoder_worker.go`** — extend:
- Add field `invalidator CatalogInvalidator` to `EncoderPool` (nil-safe).
- Extend `NewEncoderPool` signature to accept the invalidator at the end of the parameter list (additive — least disruptive). Document nil-safety in the doc comment.
- In `processJob`, AFTER the successful `status=done` transition and the success-log block at the very end, add:
  ```go
  if p.invalidator != nil && job.ShikimoriID != "" {
      // Best-effort: never fails the job, runs synchronously within the
      // invalidator's own 3s timeout (caller ctx is the long-lived worker ctx).
      p.invalidator.Invalidate(ctx, job.ShikimoriID)
  }
  ```
- DO NOT change any of the failure paths.

**`encoder_worker_test.go`** — extend:
- Add a fake `CatalogInvalidator` recording (shikimoriID, callCount).
- New test cases:
  - **invalidator_fires_on_done_with_shikimori**: happy path completes → fake invalidator called exactly once with the expected shikimoriID.
  - **invalidator_skipped_on_empty_shikimori**: no-shikimori job → fake invalidator NOT called.
  - **invalidator_skipped_on_failure**: ffmpeg fails → fake invalidator NOT called.
  - **nil_invalidator_safe**: pool constructed with `invalidator=nil`, happy path completes without panic.

**`catalog_invalidator_test.go`** — new file:
- Using `httptest.NewServer`:
  - **happy 200**: server accepts POST and returns 200 → `IncCacheInvalidation("ok")` called once via a fake metrics recorder.
  - **non-2xx 500**: server returns 500 → `IncCacheInvalidation("fail")` called once.
  - **timeout**: server sleeps past invalidator timeout → `IncCacheInvalidation("fail")`, total wall time < 2× timeout.
  - **empty url is noop**: construct with `CatalogInternalAPIURL=""` → calling Invalidate is a no-op (no metric increment).
  - **PathEscape**: shikimoriID with a slash (defensive — shouldn't happen but assert anyway): server receives the request at `/internal/cache/invalidate/raw/57466%2Fx` (encoded slash).
  - **method is POST**: server records the request method and asserts it's POST.

All tests must not depend on the live library or catalog services.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/library && go build ./... && go vet ./... && go test ./internal/service/... ./internal/metrics/... -count=1 -race</automated>
  </verify>
  <done>
`catalog_invalidator.go` + `catalog_invalidator_test.go` exist. `library_metrics.go` exports the new `cacheInvalidationTotal` counter + `IncCacheInvalidation` method. `encoder_worker.go` fires the webhook after successful `done` (only when ShikimoriID != ""). All new tests pass. Existing encoder_worker tests still pass. `go vet` clean.

Commit message: `feat(raw-jp 06): library catalog-invalidator + webhook fire from encoder + library_cache_invalidation_total metric (LIB-10)`
  </done>
</task>

<task type="auto">
  <name>Task 5: Config extension + main wiring for both services + .env.example</name>
  <files>services/catalog/internal/config/config.go, services/catalog/cmd/catalog-api/main.go, services/library/internal/config/config.go, services/library/cmd/library-api/main.go, docker/.env.example</files>
  <action>
Wire the new client + handler + invalidator through both services' config + main entry points and document every env var.

**`services/catalog/internal/config/config.go`**:
- Add type `LibraryConfig struct { APIURL string; Timeout time.Duration }` near the other sub-configs (after `AllAnimeConfig` / `OpenSubtitlesConfig`).
- Add field `Library LibraryConfig` on `Config`.
- In `Load()`:
  ```go
  Library: LibraryConfig{
      APIURL:  getEnv("LIBRARY_API_URL", "http://library:8089"),
      Timeout: getEnvDuration("LIBRARY_API_TIMEOUT", 2*time.Second),
  },
  ```

**`services/catalog/cmd/catalog-api/main.go`**:
- Add import: `"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/library"`.
- Construct the library client right after `allanimeClient` is built:
  ```go
  libraryClient := library.NewClient(library.Config{
      APIURL:  cfg.Library.APIURL,
      Timeout: cfg.Library.Timeout,
  })
  ```
- Update the `NewRawResolver` call to pass `libraryClient` (per Task 2's new signature):
  ```go
  rawResolver := service.NewRawResolver(allanimeClient, libraryClient, animeRepo, redisCache, log)
  ```
- Construct the internal cache handler:
  ```go
  internalCacheHandler := handler.NewInternalCacheHandler(redisCache, animeRepo, log)
  ```
- Update the `transport.NewRouter(...)` call to pass `internalCacheHandler` in the position chosen in Task 3.

**`services/library/internal/config/config.go`**:
- Add type `CatalogInternalConfig struct { APIURL string; Timeout time.Duration }`.
- Add field `CatalogInternal CatalogInternalConfig` on `Config`.
- In `Load()`:
  ```go
  CatalogInternal: CatalogInternalConfig{
      APIURL:  getEnv("CATALOG_INTERNAL_API_URL", "http://catalog:8081"),
      Timeout: getEnvDuration("CATALOG_INTERNAL_API_TIMEOUT", 3*time.Second),
  },
  ```

**`services/library/cmd/library-api/main.go`**:
- Construct the catalog invalidator after the metrics object is built and BEFORE the EncoderPool is constructed:
  ```go
  catalogInvalidator := service.NewCatalogInvalidator(
      service.InvalidatorConfig{
          CatalogInternalAPIURL: cfg.CatalogInternal.APIURL,
          Timeout:               cfg.CatalogInternal.Timeout,
      },
      libraryMetrics, // *metrics.LibraryMetrics satisfies InvalidationMetrics
      log,
  )
  ```
- Pass `catalogInvalidator` as the new trailing parameter to `service.NewEncoderPool(...)`.

**`docker/.env.example`** — add a labeled block immediately after the existing Phase-04 block:
```
# ── Phase 06 (workstream raw-jp / v0.2): Hybrid Resolver ──────────────
#
# The catalog's raw resolver calls the library service FIRST for every
# /api/anime/{uuid}/raw/stream request. When library_episodes has a row
# for (shikimori_id, episode), the catalog returns the MinIO HLS URL
# with source:"library". Otherwise it falls back to AllAnime. Per-request
# library timeout is 2s — the library is on the docker network, so any
# longer wait means it's actually down.
#
# Catalog → library (the hybrid lookup):
#   LIBRARY_API_URL=http://library:8089
#   LIBRARY_API_TIMEOUT=2s
#
# Library → catalog (cache-invalidation webhook after every successful
# encode). The catalog mounts POST /internal/cache/invalidate/raw/{id}
# OUTSIDE /api — reachable only from within the docker network because
# nginx/gateway does not proxy /internal/*.
#   CATALOG_INTERNAL_API_URL=http://catalog:8081
#   CATALOG_INTERNAL_API_TIMEOUT=3s
```

Verify the build of BOTH services compiles cleanly. The Task-2 signature break in catalog-api/main.go is finally closed here.

DO NOT touch `docker/docker-compose.yml` — the env vars all have safe in-cluster defaults; compose only needs explicit declarations if the operator wants to override the defaults, which is not required for v0.2 ship.

(If the live deployment needs explicit compose env entries for visibility, that's a one-line follow-up — record it as an "open item" in the eventual SUMMARY rather than expanding scope here.)
  </action>
  <verify>
    <automated>cd /data/animeenigma && go build ./services/catalog/... ./services/library/... && go vet ./services/catalog/... ./services/library/...</automated>
  </verify>
  <done>
Both `services/catalog` and `services/library` build cleanly. `go vet` clean across both. `docker/.env.example` has the Phase-06 block with all four env vars documented. The full integration is now compile-time wired end-to-end.

Commit message: `feat(raw-jp 06): config + main wiring for hybrid resolver + webhook (LIB-10)`
  </done>
</task>

<task type="auto">
  <name>Task 6: Live smoke + SUMMARY.md</name>
  <files>.planning/workstreams/raw-jp/phases/06-hybrid-resolver/06-SUMMARY.md, docker/.env.example</files>
  <action>
Redeploy both services, smoke-test every acceptance criterion from SPEC §LIB-10 against the live stack, and write the phase SUMMARY.

**Redeploy**:
```
make redeploy-catalog
make redeploy-library
make health
```
All services healthy. If health fails on either, diagnose via `make logs-<svc>` and fix before proceeding.

**Live smokes** (record verbatim output + verdict for each in the SUMMARY):

1. **Library hit → source: "library"**. Pick a shikimori_id with an existing `library_episodes` row (re-create the Phase-04 smoke synthetic encoding job if cleaned up: insert a fresh `library_jobs` row at `status='encoding'` with `shikimori_id='57466'`, `uploader='Ohys-Raws'`, a source file matching the infohash; wait for `status='done'`; confirm the row in `library_episodes`). Then:
   ```
   curl -s "http://localhost:8000/api/anime/{ANIME_UUID}/raw/stream?episode=1" | jq .
   ```
   Expect 200, `data.source == "library"`, `data.url` starts with `http://minio:9000/raw-library/57466/1/playlist.m3u8`.

2. **Library miss → source: "allanime"**. Pick an anime with NO `library_episodes` row (use any anime currently working under v0.1 raw-player.spec.ts):
   ```
   curl -s "http://localhost:8000/api/anime/{OTHER_ANIME_UUID}/raw/stream?episode=1" | jq .
   ```
   Expect 200, `data.source == "allanime"`, `data.url` is an AllAnime HLS URL.

3. **Library stopped → falls back within 2.5s**. Stop the library container:
   ```
   docker stop animeenigma-library
   time curl -s "http://localhost:8000/api/anime/{ANIME_UUID}/raw/stream?episode=2" | jq .
   ```
   Episode 2 is chosen so the source-decision cache doesn't short-circuit. Expect: response in < 2.5s (record `real` time), `data.source == "allanime"`. Bust the cache before re-running:
   ```
   docker exec animeenigma-redis redis-cli DEL raw:source-decision:{ANIME_UUID}:2
   ```
   Restart the library container after: `docker start animeenigma-library`.

4. **Webhook fires on done**. With library back up, trigger a fresh encoding job (insert another job at `status='encoding'` with a different episode number and an existing shikimori_id):
   - Watch library logs: `make logs-library` — expect an info log `raw: cache invalidated` (from catalog) AND `cache invalidation ok` (from library) around the time `status=done` is written.
   - Inspect metric: `curl -s http://localhost:8089/metrics | grep library_cache_invalidation_total` — counter `{result="ok"}` should have incremented.
   - Cache verification: `docker exec animeenigma-redis redis-cli KEYS "raw:source-decision:*"` — keys for the affected animeID are gone immediately after the job completes.

5. **v0.1 e2e still passes**. From `frontend/web`:
   ```
   bunx playwright test e2e/raw-player.spec.ts --reporter=list
   ```
   Expect all assertions PASS (the test does not check the new `source` field — proves frontend contract preserved).

6. **No regression on v0.1 ISS-012 path**. If AllAnime persisted-query SHAs are currently stale (per `docs/issues/issues.json`), confirm that fallback still degrades gracefully and the catalog returns the same shape as v0.1 — `source: "allanime"`, may return 503 or empty list depending on the v0.1 behavior. Document the observed behavior in the SUMMARY.

**Cleanup** any temporary library_jobs / library_episodes rows used for smoke testing, mirroring the cleanup section in Phase-04 SUMMARY.

**Write `06-SUMMARY.md`** following the Phase-04 template exactly (frontmatter with status/workstream/milestone/date/requirements/commits + sections: "What was built" / "Files touched" / "Verification results" / "Deviations from plan" / "Out of scope" / "Open items" / "Self-Check"). Include all 6 commit hashes from Tasks 1-5 (Task 6 itself does not commit code; the docs commit goes last).

Required SUMMARY sections (mirrors Phase-04 fidelity):
- All 6 acceptance points from SPEC §LIB-10 marked verified with verbatim curl/log output.
- Files touched (new vs extended) with line counts where useful.
- Deviations from this PLAN with rule attribution (Rule 1 / 2 / 3 from the execute-plan workflow).
- Open items carried forward (e.g. compose env-block follow-up if env vars were left implicit).

Final commit (docs only):
```
git add .planning/workstreams/raw-jp/phases/06-hybrid-resolver/06-SUMMARY.md docker/.env.example
git commit -m "docs(raw-jp 06): SUMMARY + .env.example final touches (LIB-10)"
```
  </action>
  <verify>
    <automated>test -f /data/animeenigma/.planning/workstreams/raw-jp/phases/06-hybrid-resolver/06-SUMMARY.md && grep -q "^status: complete" /data/animeenigma/.planning/workstreams/raw-jp/phases/06-hybrid-resolver/06-SUMMARY.md && grep -q "source.*library" /data/animeenigma/.planning/workstreams/raw-jp/phases/06-hybrid-resolver/06-SUMMARY.md && grep -q "source.*allanime" /data/animeenigma/.planning/workstreams/raw-jp/phases/06-hybrid-resolver/06-SUMMARY.md && grep -q "library_cache_invalidation_total" /data/animeenigma/.planning/workstreams/raw-jp/phases/06-hybrid-resolver/06-SUMMARY.md</automated>
  </verify>
  <done>
Both services redeployed and healthy. All 6 acceptance criteria from SPEC §LIB-10 demonstrated end-to-end and documented verbatim in `06-SUMMARY.md`. `library_cache_invalidation_total{result="ok"}` increments observed on `/metrics`. v0.1 e2e `raw-player.spec.ts` passes. SUMMARY frontmatter has `status: complete`, all six task commit hashes, and includes the requirements/milestone/workstream fields. No smoke artifacts left in the DB or MinIO.

Commit message: `docs(raw-jp 06): SUMMARY + .env.example final touches (LIB-10)`
  </done>
</task>

</tasks>

<verification>
Phase-level checks (run after all tasks complete, summarized in 06-SUMMARY.md):

1. **Build + vet across both services**:
   ```
   cd /data/animeenigma && go build ./services/catalog/... ./services/library/... && go vet ./services/catalog/... ./services/library/...
   ```
2. **Unit tests across new packages**:
   ```
   cd /data/animeenigma/services/catalog && go test ./internal/parser/library/... ./internal/service/... ./internal/handler/... -count=1 -race
   cd /data/animeenigma/services/library && go test ./internal/service/... ./internal/metrics/... -count=1 -race
   ```
3. **Live integration** (per Task 6): library hit → `source:"library"`; library miss → `source:"allanime"`; library down → fallback in < 2.5s; webhook fires + Redis cache busted on encoder `done`.
4. **Frontend contract preserved**: `bunx playwright test e2e/raw-player.spec.ts` passes.
5. **Metric exposed**: `curl -s http://localhost:8089/metrics | grep ^library_cache_invalidation_total` returns at least one labeled line.
</verification>

<success_criteria>
The phase is complete when every must-have truth holds against the deployed stack:

- A library-resolved episode returns `source: "library"` with the MinIO URL in the catalog response.
- A non-library episode returns `source: "allanime"` from the same endpoint.
- Stopping the library service does NOT increase /raw/stream latency beyond 2.5s.
- A new encode triggers an `library_cache_invalidation_total{result="ok"}` increment AND the affected catalog cache keys are deleted within ~1s of `status=done`.
- The v0.1 `raw-player.spec.ts` Playwright suite passes unchanged.
- The catalog AND library services build, vet, and unit-test cleanly with `-race`.
- `06-SUMMARY.md` exists with frontmatter `status: complete`, references all six commit hashes, and documents every smoke result verbatim.
</success_criteria>

<output>
After completion, write `.planning/workstreams/raw-jp/phases/06-hybrid-resolver/06-SUMMARY.md` following the Phase-04 SUMMARY template exactly (same frontmatter shape, same section order, same verbatim-evidence discipline). Then run `make redeploy-catalog && make redeploy-library && make health` once more to confirm post-merge health, and stop.
</output>
