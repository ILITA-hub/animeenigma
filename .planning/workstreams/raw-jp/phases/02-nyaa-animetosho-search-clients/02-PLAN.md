---
phase: 02-nyaa-animetosho-search-clients
plan: 01
type: execute
wave: 1
workstream: raw-jp
milestone: v0.2
depends_on: [01-library-service-scaffold]
files_modified:
  - services/library/internal/domain/release.go
  - services/library/internal/parser/animetosho/client.go
  - services/library/internal/parser/animetosho/client_test.go
  - services/library/internal/parser/nyaa/client.go
  - services/library/internal/parser/nyaa/client_test.go
  - services/library/internal/service/search.go
  - services/library/internal/service/search_test.go
  - services/library/internal/handler/search.go
  - services/library/internal/transport/router.go
  - services/library/internal/config/config.go
  - services/library/cmd/library-api/main.go
  - services/library/go.mod
  - services/library/go.sum
  - services/gateway/internal/transport/router.go
  - docker/.env.example
autonomous: true
requirements:
  - LIB-03
  - LIB-04
  - LIB-04b
spec: .planning/workstreams/raw-jp/milestones/v0.2-phases/02-nyaa-animetosho-clients/02-SPEC.md
context_doc: .planning/workstreams/raw-jp/phases/02-nyaa-animetosho-search-clients/02-CONTEXT.md

must_haves:
  truths:
    - "GET /api/library/search returns 200 with a JSON envelope wrapping {releases, providers_down}"
    - "When MAL ID is supplied, AnimeTosho hits with that MAL ID rank above Nyaa hits"
    - "Duplicate releases (same InfoHash from both providers) appear exactly once"
    - "When one provider's HTTP call fails or its base URL is unreachable, the endpoint still returns 200 and the failed provider name appears in providers_down"
    - "Direct probe on library:8089 and gateway-proxied probe on gateway:8000 both succeed"
    - "Admin-only gating is enforced on /api/library/search at the gateway (non-admin JWT → 403/401)"
  artifacts:
    - path: "services/library/internal/domain/release.go"
      provides: "Shared Release struct used by both parsers and the merger"
      contains: "type Release struct"
    - path: "services/library/internal/parser/nyaa/client.go"
      provides: "Nyaa.si RSS parser with Search(ctx, query, limit) ([]Release, error)"
      contains: "func (c *Client) Search"
    - path: "services/library/internal/parser/animetosho/client.go"
      provides: "AnimeTosho JSON parser with Search(ctx, SearchParams) ([]Release, error)"
      contains: "type SearchParams struct"
    - path: "services/library/internal/service/search.go"
      provides: "SearchAggregator: parallel fan-out, dedupe by InfoHash, rank AnimeTosho-with-MAL-ID first"
      contains: "errgroup"
    - path: "services/library/internal/handler/search.go"
      provides: "GET /search handler bound under /api/library"
      contains: "func (h *SearchHandler) Search"
  key_links:
    - from: "services/library/internal/handler/search.go"
      to: "services/library/internal/service/search.go"
      via: "SearchAggregator.FetchAll injected via constructor"
      pattern: "service\\.SearchAggregator"
    - from: "services/library/internal/service/search.go"
      to: "services/library/internal/parser/{nyaa,animetosho}"
      via: "Searcher interface, two implementations injected"
      pattern: "errgroup\\.WithContext"
    - from: "services/library/cmd/library-api/main.go"
      to: "router + handler + service + clients"
      via: "constructor wiring in main()"
      pattern: "search\\.NewAggregator|NewSearchHandler"
    - from: "services/gateway/internal/transport/router.go"
      to: "library /api/library/search"
      via: "JWTValidationMiddleware + AdminRoleMiddleware on /api/library/* prefix"
      pattern: "AdminRoleMiddleware|JWTValidationMiddleware"
---

<objective>
Add Nyaa.si RSS and AnimeTosho JSON-feed search clients to the library service,
expose them through a merger that fans out in parallel, dedupes by InfoHash,
and ranks AnimeTosho-with-MAL-ID hits first. Mount the result at
`GET /api/library/search?q=&mal_id=&limit=` and gate the entire `/api/library/*`
prefix behind admin auth at the gateway.

Purpose: gives Phase 5's admin UI a single endpoint to populate the search
table. AnimeTosho is preferred when a MAL ID is known; Nyaa is the fallback.

Output: two parser packages, one Release domain type, one aggregator service,
one HTTP handler, gateway admin-gate wiring, config + env documentation, and
unit tests against httptest mocks.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@.planning/workstreams/raw-jp/milestones/v0.2-REQUIREMENTS.md
@.planning/workstreams/raw-jp/milestones/v0.2-phases/02-nyaa-animetosho-clients/02-SPEC.md
@.planning/workstreams/raw-jp/phases/02-nyaa-animetosho-search-clients/02-CONTEXT.md
@.planning/workstreams/raw-jp/phases/01-library-service-scaffold/01-SUMMARY.md
@services/library/cmd/library-api/main.go
@services/library/internal/transport/router.go
@services/library/internal/config/config.go
@services/library/internal/handler/health.go
@services/library/go.mod
@services/catalog/internal/parser/allanime/client.go
@libs/httputil/response.go
@libs/errors/errors.go
@libs/logger/logger.go

<interfaces>
Library service is already on port 8089 (NOT 8087 — host-side maintenance bot
owns 8087; see 01-SUMMARY.md "Deviations from plan #1"). All curl-based smoke
tests must use 8089 for direct probes and 8000 for gateway-proxied probes.

Key extant patterns (extracted, do not re-read):

From libs/httputil/response.go:
  func OK(w http.ResponseWriter, data interface{})            // 200, wraps in {success:true,data:...}
  func BadRequest(w http.ResponseWriter, message string)       // 400
  func Error(w http.ResponseWriter, err error)                 // maps AppError → status

From libs/errors/errors.go:
  func Wrap(err error, code ErrorCode, message string) *AppError
  func ExternalAPI(service string, err error) *AppError       // use for upstream RSS/JSON failures

From libs/logger/logger.go:
  type Logger struct { *zap.SugaredLogger }
  func Default() *Logger
  // Methods used downstream: Infow, Warnw, Errorw, Debugw

From services/catalog/internal/parser/allanime/client.go (shape to mirror):
  type Config struct { HTTPTimeout time.Duration; UserAgent string; ... }
  type Client struct { cfg Config; httpClient *http.Client }
  func NewClient(cfg Config) *Client

Gateway routing today (services/gateway/internal/transport/router.go, around line 317):
  r.Route("/library", func(r chi.Router) {
      r.Get("/health", proxyHandler.ProxyToLibrary)
      r.HandleFunc("/*", proxyHandler.ProxyToLibrary)
  })
This currently passes through with NO auth — Task 4 adds the admin gate around
all non-/health library routes (mirror the /streaming admin subgroup pattern
at lines 332-336).

Library router today (services/library/internal/transport/router.go):
  r.Route("/api/library", func(r chi.Router) {
      _ = jwtConfig
      r.Get("/health", healthHandler.Health)
  })
Phase 2 registers `r.Get("/search", searchHandler.Search)` inside this same
block so the full path is /api/library/search.

Provider URLs and defaults (locked by SPEC):
  Nyaa:        BaseURL=https://nyaa.si           path=/?page=rss&q={q}&c=1_2&f=0
  AnimeTosho:  BaseURL=https://feed.animetosho.org
               MAL-path:    /json?show=mal&id={mal_id}
               query-path:  /json?q={query}

User-Agent: "AnimeEnigma/1.0 (library service)"  (both clients)
HTTP timeout: 15s per request (both clients)
Quality regex: (?i)\b(2160|1080|720|480)p\b
Uploader regex (AnimeTosho only — Nyaa uses <dc:creator>): leading `\[([^\]]+)\]` from title; empty when absent.
Dedupe key: InfoHash only.
Provider names on Release.Source: lowercase "nyaa" / "animetosho".
Default limit: 50. Cap: 200. (limit<=0 → 50. limit>200 → 200.)

Anacrolix dependency:
  go get github.com/anacrolix/torrent
  import "github.com/anacrolix/torrent/metainfo"
  m, err := metainfo.ParseMagnetURI(magnet)   // m.InfoHash.HexString()
This will be pinned again in Phase 3; double-import is acceptable per SPEC.
</interfaces>
</context>

<goal>
By the end of this plan an admin in Phase 5 can call
`GET /api/library/search?q=frieren&mal_id=52991` through the gateway and
receive a JSON-enveloped `{releases:[...], providers_down:[...]}` payload
containing deduped, ranked releases sourced from both Nyaa.si and AnimeTosho,
with single-provider failure surviving as a soft-degraded response. The
admin gate lives at the gateway; the library service itself trusts what
the gateway forwards.
</goal>

<tasks>

<task type="auto">
  <name>Task 1: Release domain type + AnimeTosho JSON client + tests</name>
  <files>
    services/library/internal/domain/release.go,
    services/library/internal/parser/animetosho/client.go,
    services/library/internal/parser/animetosho/client_test.go,
    services/library/go.mod,
    services/library/go.sum
  </files>
  <action>
    Create the shared `Release` struct in package `domain` exactly as locked
    in CONTEXT.md (Title, Magnet, InfoHash, Uploader, Quality, SizeBytes,
    Source, MALID, FoundAt) with the JSON tags spelled in the SPEC. Add the
    anacrolix dependency: `cd services/library && go get github.com/anacrolix/torrent@latest`
    (let go.mod resolve to the latest tag — Phase 3 will pin).

    Implement the AnimeTosho client at `internal/parser/animetosho/client.go`
    following the allanime parser shape (per D-01 in CONTEXT.md). Required
    public surface:

      type Config struct { BaseURL string; HTTPTimeout time.Duration; UserAgent string }
      type Client struct { cfg Config; httpClient *http.Client }
      type SearchParams struct { MALID int; Query string; Limit int }
      func NewClient(cfg Config) *Client
      func (c *Client) Search(ctx context.Context, p SearchParams) ([]domain.Release, error)

    Defaults inside NewClient: BaseURL "https://feed.animetosho.org", timeout 15s,
    UA "AnimeEnigma/1.0 (library service)". Route selection: `p.MALID > 0` →
    `/json?show=mal&id={mal_id}`; else → `/json?q={url.QueryEscape(p.Query)}`.
    JSON response shape (relevant fields, parse with `encoding/json`):
      title (string), link (string, magnet:?), info_hash (string), total_size (int64),
      timestamp (int64 unix seconds), nyaa_subcat (string, ignored by Phase 2).
    For each entry:
      - InfoHash: prefer the response's `info_hash`; if empty, derive via
        `metainfo.ParseMagnetURI(link)` and `m.InfoHash.HexString()`.
      - Quality: regex `(?i)\b(2160|1080|720|480)p\b` on Title; empty when no match.
      - Uploader: regex `^\[([^\]]+)\]` on Title; empty when no match.
      - FoundAt: `time.Unix(timestamp, 0).UTC()`.
      - MALID: `p.MALID` (will be zero on the query-path; that is correct).
      - Source: literal `"animetosho"`.
      - SizeBytes: `total_size`.
    Honor `p.Limit` by trimming the returned slice (clamp 1..200, default 50
    when <=0). Wrap non-2xx HTTP responses with `errors.ExternalAPI("animetosho", ...)`
    so the merger can fail-soft. Wrap JSON decode errors with `errors.Wrap`.

    Tests at `client_test.go` using `httptest.NewServer`:
      - TestSearch_MALPath: serves a fixture with two entries when r.URL.RawQuery
        contains `show=mal&id=52991`. Asserts both releases parsed, MALID propagated,
        Quality + Uploader extracted, InfoHash hex-lowercase, Source="animetosho".
      - TestSearch_QueryPath: when MALID==0, assert r.URL.Query().Get("q")=="frieren".
        Empty response `[]` returns an empty slice, no error.
      - TestSearch_Non200: returns 500 → Search returns a wrapped error.
      - TestSearch_LimitClamp: returns 10 entries, p.Limit=3 → exactly 3 results;
        p.Limit=0 → default 50 (no clamp visible because <50 entries).
      - TestSearch_InfoHashFromMagnet: fixture omits info_hash, only `link` →
        parser falls back to ParseMagnetURI.

    No mutation of public packages outside `internal/`. No real network calls
    in tests.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/library &amp;&amp; go build ./... &amp;&amp; go vet ./internal/parser/animetosho/... &amp;&amp; go test ./internal/parser/animetosho/... -count=1</automated>
  </verify>
  <done>
    `services/library/internal/domain/release.go` exists with the locked
    Release struct. The animetosho package builds and all tests above pass.
    `go.mod` lists `github.com/anacrolix/torrent` in require. Commit:
    `feat(library/02): add Release type + AnimeTosho JSON client with tests`.
  </done>
</task>

<task type="auto">
  <name>Task 2: Nyaa RSS client + tests</name>
  <files>
    services/library/internal/parser/nyaa/client.go,
    services/library/internal/parser/nyaa/client_test.go
  </files>
  <action>
    Implement `services/library/internal/parser/nyaa/client.go` mirroring the
    AnimeTosho client shape. Required public surface:

      type Config struct { BaseURL string; HTTPTimeout time.Duration; UserAgent string }
      type Client struct { cfg Config; httpClient *http.Client }
      func NewClient(cfg Config) *Client
      func (c *Client) Search(ctx context.Context, query string, limit int) ([]domain.Release, error)

    Defaults: BaseURL "https://nyaa.si", timeout 15s, UA "AnimeEnigma/1.0 (library service)".
    URL: `{BaseURL}/?page=rss&q={url.QueryEscape(query)}&c=1_2&f=0`.

    Parse the RSS via `encoding/xml`. Define internal types using xml struct
    tags including Nyaa's custom namespaces. Reference Nyaa item fields used:
      - <title> → Release.Title
      - <link> → typically a /download/...torrent URL; the magnet is at
        Nyaa's namespaced <nyaa:infoHash> + we synthesize a magnet URI as
        `magnet:?xt=urn:btih:{infoHash}&dn={url.QueryEscape(title)}`.
        Also store the raw InfoHash directly (lowercase hex).
      - <dc:creator> → Release.Uploader
      - <pubDate> → parse RFC1123Z (`time.Parse(time.RFC1123Z, ...)`); fallback to RFC1123.
      - <nyaa:size> → human-readable like "1.4 GiB"; parse via a small helper
        `parseSize(s string) int64` that accepts B/KB/KiB/MB/MiB/GB/GiB/TB/TiB
        case-insensitively. Unknown formats return 0 (do not error).
      - Quality: same regex on Title.
      - Source: literal `"nyaa"`.
      - MALID: 0 (Nyaa does not expose MAL IDs).

    Honor limit (clamp 1..200, default 50 when <=0). Wrap non-2xx with
    `errors.ExternalAPI("nyaa", ...)`. Wrap XML decode errors with `errors.Wrap`.

    Tests using `httptest.NewServer`:
      - TestSearch_RSSParsesFields: fixture with 2 items (one 1080p, one 720p,
        each with a different uploader and nyaa:infoHash). Assert Title,
        Uploader (from dc:creator), Quality, InfoHash (lowercase hex), SizeBytes
        > 0, FoundAt non-zero, Source="nyaa", and the synthesized magnet URI
        starts with `magnet:?xt=urn:btih:`.
      - TestSearch_QueryParameters: assert r.URL.Query() has q=q, c=1_2, f=0,
        page=rss.
      - TestSearch_Non200: 503 → wrapped error.
      - TestSearch_LimitClamp: 10 items, limit=3 → 3 returned.
      - TestParseSize: table-driven over "1.4 GiB", "700 MiB", "512 MB",
        "1024", "bogus" → bytes / 0.

    Do not depend on the animetosho package — both clients consume only `domain`.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/library &amp;&amp; go vet ./internal/parser/nyaa/... &amp;&amp; go test ./internal/parser/nyaa/... -count=1</automated>
  </verify>
  <done>
    `services/library/internal/parser/nyaa/client.go` and its `client_test.go`
    exist; all tests above pass; full module still builds clean. Commit:
    `feat(library/02): add Nyaa RSS client with tests`.
  </done>
</task>

<task type="auto">
  <name>Task 3: SearchAggregator merger + tests</name>
  <files>
    services/library/internal/service/search.go,
    services/library/internal/service/search_test.go
  </files>
  <action>
    Implement the merger at `services/library/internal/service/search.go`.
    Public surface:

      type SearchParams struct { Query string; MALID int; Limit int }
      type Result struct {
          Releases      []domain.Release
          ProvidersDown []string
      }
      // Searcher is the local provider abstraction; nyaa.Client and
      // animetosho.Client satisfy two separate adapters defined here, so
      // the service package does not import the parser packages with hard
      // type coupling beyond the constructor.
      type NyaaSearcher       interface { Search(ctx context.Context, q string, limit int) ([]domain.Release, error) }
      type AnimeToshoSearcher interface {
          Search(ctx context.Context, p animetoshoParams) ([]domain.Release, error)
      }
      type SearchAggregator struct { /* nyaa, animetosho, log */ }
      func NewAggregator(nyaa NyaaSearcher, at AnimeToshoSearcher, log *logger.Logger) *SearchAggregator
      func (a *SearchAggregator) FetchAll(ctx context.Context, p SearchParams) (Result, error)

    Use `golang.org/x/sync/errgroup` with `g.SetLimit(2)`. Fan out:
      - Goroutine A: nyaa.Search(ctx, p.Query, p.Limit)
      - Goroutine B: animetosho.Search(ctx, {MALID:p.MALID, Query:p.Query, Limit:p.Limit})
    Each goroutine MUST recover its own error and NOT propagate it through
    errgroup (we want both to run to completion regardless). On error, log
    via `log.Warnw("library search provider failed", "provider", ..., "error", err)`
    and append the provider name to a local []string. `errgroup.Wait()` always
    returns nil from FetchAll — the function only returns a non-nil error if
    BOTH providers fail (return that combined error so the handler can 502).

    Merge / dedupe / rank:
      - Build a map[string]domain.Release keyed by `strings.ToLower(InfoHash)`.
        Skip entries with empty InfoHash (they cannot be deduped or queued
        downstream; log Debugw and drop).
      - On collision, prefer the entry whose Source=="animetosho" (richer
        metadata per SPEC). If both sides are the same source, keep the
        first occurrence.
      - Rank: split into two slices, `headed` = entries where
        Source=="animetosho" AND MALID>0 AND MALID==p.MALID (only when
        p.MALID>0). The rest go to `tail`. Sort `headed` by FoundAt DESC,
        then `tail` by FoundAt DESC. Final slice = headed ++ tail.
      - Clamp final to p.Limit (default 50, cap 200).
    Return `Result{Releases: final, ProvidersDown: down}`.

    Tests at `search_test.go` using inline fakes (struct types satisfying
    NyaaSearcher / AnimeToshoSearcher with fields driving the returns):
      - TestFetchAll_BothSucceed_Dedupes: same InfoHash from both → kept once,
        and the kept copy has Source=="animetosho".
      - TestFetchAll_RanksAnimeToshoWithMatchingMAL: p.MALID=52991, mixed
        results from both providers, AnimeTosho hits with MALID=52991 come first.
      - TestFetchAll_NyaaDown: nyaa returns an error; ProvidersDown==["nyaa"];
        AnimeTosho's releases survive; Result.Releases is non-empty.
      - TestFetchAll_AnimeToshoDown: symmetrical.
      - TestFetchAll_BothDown: both error → FetchAll returns a non-nil error
        AND ProvidersDown is the both-providers slice in some order.
      - TestFetchAll_LimitClamp: 60 entries combined, p.Limit=5 → 5 returned.
      - TestFetchAll_EmptyInfoHashSkipped: a release with InfoHash=="" is
        dropped from the merged slice.

    Add `golang.org/x/sync` to go.mod (it's already an indirect dep — promote
    to direct via `go get golang.org/x/sync`).
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/library &amp;&amp; go vet ./internal/service/... &amp;&amp; go test ./internal/service/... -count=1</automated>
  </verify>
  <done>
    `internal/service/search.go` exposes `NewAggregator` + `FetchAll`. All
    seven tests pass. `go build ./...` from `services/library/` clean.
    Commit: `feat(library/02): add SearchAggregator (parallel fan-out, dedupe, rank)`.
  </done>
</task>

<task type="auto">
  <name>Task 4: HTTP handler, router wiring, config, env, main.go, gateway admin gate</name>
  <files>
    services/library/internal/handler/search.go,
    services/library/internal/transport/router.go,
    services/library/internal/config/config.go,
    services/library/cmd/library-api/main.go,
    services/gateway/internal/transport/router.go,
    docker/.env.example
  </files>
  <action>
    Create `services/library/internal/handler/search.go`:

      type SearchHandler struct { agg *service.SearchAggregator; log *logger.Logger }
      func NewSearchHandler(agg *service.SearchAggregator, log *logger.Logger) *SearchHandler
      func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request)

    Parse query params: `q` (string, optional but at least one of {q, mal_id}
    must be present — empty → `httputil.BadRequest("q or mal_id required")`),
    `mal_id` (int, default 0; on parse error → BadRequest), `limit` (int,
    default 50, parse error tolerated → 50, clamped 1..200 inside aggregator).
    Call `agg.FetchAll(r.Context(), service.SearchParams{Query: q, MALID: malID, Limit: limit})`.
    On nil error: `httputil.OK(w, map[string]any{"releases": result.Releases, "providers_down": result.ProvidersDown})`.
    On non-nil error (both providers down): log Errorw, then `httputil.Error(w, errors.ExternalAPI("library_search", err))`
    which surfaces as 502/503 via the AppError → HTTP mapping.

    Update `services/library/internal/transport/router.go`: extend the
    `NewRouter` signature to accept the new `*handler.SearchHandler`. Inside
    the existing `r.Route("/api/library", ...)` block, add
    `r.Get("/search", searchHandler.Search)`. Keep the existing `_ = jwtConfig`
    sentinel — auth is still enforced at the gateway only in v0.2.

    Extend `services/library/internal/config/config.go` with three new sub-configs
    on the top-level `Config`:

      type NyaaConfig         struct { BaseURL string; HTTPTimeout time.Duration; UserAgent string }
      type AnimeToshoConfig   struct { BaseURL string; HTTPTimeout time.Duration; UserAgent string }
      type LibrarySearchConfig struct { DefaultLimit int; MaxLimit int }  // currently informational; aggregator uses constants

    Read env in `Load()`:
      NYAA_BASE_URL          (default "https://nyaa.si")
      ANIMETOSHO_BASE_URL    (default "https://feed.animetosho.org")
      LIBRARY_SEARCH_TIMEOUT (default 15s)  — applied to both clients
      LIBRARY_SEARCH_UA      (default "AnimeEnigma/1.0 (library service)")
      LIBRARY_SEARCH_DEFAULT_LIMIT (default 50)
      LIBRARY_SEARCH_MAX_LIMIT     (default 200)

    Update `services/library/cmd/library-api/main.go`: after the DB init,
    construct (in order): nyaa.NewClient(cfg.Nyaa-as-nyaa.Config), animetosho.NewClient(...),
    service.NewAggregator(...), handler.NewSearchHandler(...). Pass the new
    handler into `transport.NewRouter`. Keep all existing logging unchanged
    so deployment diff stays small.

    Gateway admin gate at `services/gateway/internal/transport/router.go`
    (around lines 317–320): wrap non-/health library routes in a JWT + admin
    subgroup, mirroring the existing `/streaming/admin/*` pattern (lines
    332-336). Concretely:

      r.Route("/library", func(r chi.Router) {
          r.Get("/health", proxyHandler.ProxyToLibrary)              // public
          r.Group(func(r chi.Router) {
              r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
              r.Use(authz.AdminRoleMiddleware)                       // import if not already imported here
              r.HandleFunc("/*", proxyHandler.ProxyToLibrary)         // /search and everything else
          })
      })

    If `authz.AdminRoleMiddleware` is not already imported in this file, add
    `"github.com/ILITA-hub/animeenigma/libs/authz"` (it is already used at
    line 334-ish for JWT — verify, then add only what is missing). Do NOT
    change any other route. Make absolutely sure `/api/library/health`
    remains public (the docker healthcheck depends on it).

    Update `docker/.env.example` — append a new commented block after the
    existing external-API block (around line 47), with these keys and their
    defaults documented:
      NYAA_BASE_URL=https://nyaa.si
      ANIMETOSHO_BASE_URL=https://feed.animetosho.org
      LIBRARY_SEARCH_TIMEOUT=15s
      LIBRARY_SEARCH_UA=AnimeEnigma/1.0 (library service)
      LIBRARY_SEARCH_DEFAULT_LIMIT=50
      LIBRARY_SEARCH_MAX_LIMIT=200
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/library &amp;&amp; go build ./... &amp;&amp; go vet ./... &amp;&amp; cd /data/animeenigma/services/gateway &amp;&amp; go build ./... &amp;&amp; go vet ./...</automated>
  </verify>
  <done>
    Both library and gateway services compile clean. `make redeploy-library`
    and `make redeploy-gateway` succeed. `curl http://localhost:8089/health`
    still returns 200. `curl http://localhost:8000/api/library/health` still
    returns 200. `curl -i http://localhost:8000/api/library/search?q=test`
    without a JWT returns 401 (not 200, not 502). With a valid admin JWT,
    same call returns 200 with a JSON envelope. Commit:
    `feat(library/02): wire search handler, router, config, env + gateway admin gate`.
  </done>
</task>

<task type="auto">
  <name>Task 5: Live smoke + provider-down soft-fail verification + summary</name>
  <files>
    .planning/workstreams/raw-jp/phases/02-nyaa-animetosho-search-clients/02-SUMMARY.md
  </files>
  <action>
    Redeploy the library + gateway (idempotent):
      make redeploy-library
      make redeploy-gateway
      make health

    Obtain an admin JWT — the ui_audit_bot account from CLAUDE.md is the
    canonical automation user. Use either the saved `UI_AUDIT_API_KEY` from
    `docker/.env` OR a fresh `/api/auth/login` against `ui_audit_bot` with
    the documented password to mint a JWT (whichever is faster from the host).
    Bot account is admin-equivalent for library routes if its role is
    set; if not, mint a JWT for any user whose role is admin via the
    existing auth flow. (Implementation note: this is a verification-only
    step; if neither account in `docker/.env` is admin, document that in
    the summary as an open item — the gate is correct, the test user is
    the gap.)

    Smoke tests (BEST-EFFORT — capture exit codes and bodies):

      # 1. Direct hit, MAL-ID-aware, expect 200 + non-empty releases
      curl -s -o /tmp/lib-smoke-1.json -w 'HTTP %{http_code}\n' \
        -H "Authorization: Bearer ${ADMIN_JWT}" \
        'http://localhost:8089/api/library/search?q=frieren&mal_id=52991'
      jq '.data.releases | length, .data.providers_down' /tmp/lib-smoke-1.json

      # 2. Gateway-proxied (admin-gated), same URL via 8000
      curl -s -o /tmp/lib-smoke-2.json -w 'HTTP %{http_code}\n' \
        -H "Authorization: Bearer ${ADMIN_JWT}" \
        'http://localhost:8000/api/library/search?q=frieren&mal_id=52991'

      # 3. Gateway without auth — must be 401/403
      curl -s -o /dev/null -w 'HTTP %{http_code}\n' \
        'http://localhost:8000/api/library/search?q=frieren&mal_id=52991'

      # 4. Provider-down soft-fail. Temporarily override NYAA_BASE_URL to an
      #    unreachable host (e.g. http://127.0.0.1:1 ) via the compose env,
      #    redeploy library, re-hit endpoint, expect 200 + providers_down=["nyaa"]:
      #    (revert immediately after capture)
      docker compose -f docker/docker-compose.yml exec -T -e NYAA_BASE_URL=http://127.0.0.1:1 library /bin/true || true
      # The cleaner approach: edit docker-compose.yml env: NYAA_BASE_URL=http://127.0.0.1:1
      # then `make redeploy-library`, hit endpoint, capture, revert, redeploy again.

    Acceptable outcomes (capture all in summary):
      - Smoke #1 and #2: ideally 200 + non-empty releases. If upstream
        unreachable from the deploy host, both providers may end up in
        providers_down — still HTTP 200, still valid. Record the actual
        body. This is the BEST-EFFORT note in CONTEXT.md.
      - Smoke #3: must be 401 or 403. Anything else (200/200-empty-array)
        is a regression and blocks the phase.
      - Smoke #4: must be HTTP 200 with `providers_down` containing "nyaa".

    Write `02-SUMMARY.md` using `~/.claude/get-shit-done/templates/summary.md`
    as the skeleton. Required sections:
      - frontmatter: phase, status, workstream, milestone, date, requirements
        (LIB-03, LIB-04, LIB-04b), commits (paste five short SHAs).
      - What was built (one paragraph per task).
      - Files touched (NEW / EXTEND lists, exhaustive).
      - Verification results — paste the four smoke command outputs verbatim
        plus the unit-test pass output (`go test ./... -count=1 | tail`).
      - Deviations from plan (anything that drifted, including UA strings
        from upstream WAF nudges, retry decisions, etc.).
      - Out of scope (carry over from SPEC).
      - Open items.
      - Self-Check block listing every file in `files_modified` from the
        plan frontmatter, each marked FOUND.

    Commit: `docs(library/02): summary + verification artifacts`.
  </action>
  <verify>
    <automated>test -s /data/animeenigma/.planning/workstreams/raw-jp/phases/02-nyaa-animetosho-search-clients/02-SUMMARY.md &amp;&amp; cd /data/animeenigma/services/library &amp;&amp; go test ./... -count=1</automated>
  </verify>
  <done>
    All four smoke captures recorded in `02-SUMMARY.md`. Smoke #3 returned
    401/403 (regression gate). Smoke #4 returned 200 with providers_down
    populated. All unit tests across the library module pass. Commit
    pushed (do NOT push remote here — `animeenigma-after-update` handles
    push; just `git commit`). Summary's Self-Check block lists every file
    in the plan frontmatter as FOUND.
  </done>
</task>

</tasks>

<validation_strategy>
**Unit layer (Tasks 1–3, deterministic):**
- `go test ./internal/parser/nyaa/... -count=1`
- `go test ./internal/parser/animetosho/... -count=1`
- `go test ./internal/service/... -count=1`
- `go vet ./...` from the library module — must be clean.

**Wiring layer (Task 4, deterministic):**
- `go build ./...` from both `services/library/` and `services/gateway/`.
- `make redeploy-library && make redeploy-gateway && make health` — all
  existing checks still pass plus `✓ library:8089`.
- Direct `/health` probe and gateway-proxied `/api/library/health` both
  return 200 (no regression on Phase 1 endpoints).

**End-to-end layer (Task 5, best-effort + regression gate):**
- Admin JWT + `/api/library/search?q=&mal_id=` → 200 with envelope.
- Same URL without JWT → 401/403 (HARD GATE — anything else fails the phase).
- One provider's `*_BASE_URL` pointed at `127.0.0.1:1` → 200 + providers_down
  contains that provider (HARD GATE).
- Upstream-reachability flakiness for releases-non-empty is tolerated;
  documented in summary, does not block.
</validation_strategy>

<out_of_scope>
- Job enqueueing, BitTorrent download (Phase 3).
- ffmpeg HLS transcoding (Phase 4).
- MinIO writer / `raw-library` bucket bootstrap (Phase 4).
- `RawLibrary.vue` admin UI (Phase 5).
- Hybrid resolver in catalog service (Phase 6).
- Caching the search response (deferred — admin searches are ad-hoc).
- Rate-limiting at the parser layer (deferred to `golang.org/x/time/rate` if needed).
- Per-uploader scoring / trusted-uploader whitelist (Ohys, Leopard, ARC, SubsPlease) — deferred to v0.3+.
- Pinning the anacrolix/torrent dependency version — Phase 3 owns this.
- `library_jobs` / `library_episodes` migrations — Phases 3/4.
- Updating `docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
  and v0.2-REQUIREMENTS.md port references from 8087 → 8089 (Phase 1
  open item; carry forward).
</out_of_scope>

<success_criteria>
1. `services/library/internal/parser/nyaa/{client.go,client_test.go}` exists; `go test ./internal/parser/nyaa/...` passes.
2. `services/library/internal/parser/animetosho/{client.go,client_test.go}` exists; `go test ./internal/parser/animetosho/...` passes.
3. `services/library/internal/domain/release.go` defines the locked Release struct.
4. `services/library/internal/service/search.go` runs both clients via `errgroup.SetLimit(2)`, fail-soft, dedupes by InfoHash, ranks AnimeTosho-with-matching-MAL-ID first.
5. `services/library/internal/handler/search.go` mounts `GET /search` under the existing `/api/library` route group.
6. `services/library/internal/config/config.go` reads `NYAA_BASE_URL`, `ANIMETOSHO_BASE_URL`, `LIBRARY_SEARCH_TIMEOUT`, and the limit/UA env vars; `docker/.env.example` documents them.
7. `services/gateway/internal/transport/router.go` gates all `/api/library/*` non-/health routes behind JWT + AdminRoleMiddleware.
8. `cd services/library && go build ./... && go vet ./...` clean.
9. Smoke: `curl 'http://localhost:8089/api/library/search?q=frieren&mal_id=52991'` returns HTTP 200 with a JSON envelope wrapping `{releases, providers_down}`. Same via `http://localhost:8000/...` with admin JWT.
10. Provider-down test (one base URL → `127.0.0.1:1`): HTTP 200 + `providers_down` populated.
11. Unauthenticated gateway hit on `/api/library/search` returns 401/403.
12. All five commits land on the workstream branch with the messages above; `02-SUMMARY.md` exists with the Self-Check block listing every modified file as FOUND.
</success_criteria>

<output>
After completion, the executor MUST create
`.planning/workstreams/raw-jp/phases/02-nyaa-animetosho-search-clients/02-SUMMARY.md`
(Task 5). The summary must include a Self-Check block listing every file in
the frontmatter `files_modified` array, each annotated FOUND, plus the four
smoke-test transcripts and the final `go test ./... -count=1` output.
</output>
