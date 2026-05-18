---
id: LIB-search-clients
title: Nyaa.si RSS + AnimeTosho JSON-feed search clients + merger
workstream: raw-jp
milestone: v0.2
phase: 02
created_at: 2026-05-18
status: SPEC-ready
ambiguity_score: 0.15
mode: --auto
---

# Phase 02 (workstream `raw-jp`, milestone v0.2): Nyaa + AnimeTosho Search Clients — Specification

**Workstream:** `raw-jp`
**Milestone:** v0.2 Self-Hosted Library
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Requirements:** LIB-03, LIB-04, LIB-04b
**Depends on:** Phase 1 (service scaffold)
**Mode:** `--auto`

## Goal

Two HTTP parser modules at `services/library/internal/parser/{nyaa,animetosho}/` plus a merger service that fans out to both in parallel and returns a deduped, ranked `Release[]` to the admin UI. Exposed at `GET /api/library/search?q=&mal_id=&limit=`. AnimeTosho is preferred when a MAL ID is provided (because its feed natively filters by MAL ID); Nyaa is the fallback.

## Background

**Today, three things are true and need to change:**

1. **No torrent indexer integration exists.** The catalog has parsers for streaming providers (Kodik, AnimeLib, HiAnime, Consumet, AllAnime) but nothing that reads torrent indexes. The new library service needs at least one source for magnet links + metadata.

2. **Nyaa.si and AnimeTosho are complementary.** Nyaa.si is the larger catalogue but the search is title-keyed; metadata quality varies by uploader. AnimeTosho is a Nyaa mirror that adds normalized metadata + a `mal_id` filter on its JSON feed. When the catalog has a MAL ID for the anime the admin is queuing, AnimeTosho gives more reliable hits.

3. **The library service search endpoint is the foundation of the admin UI in Phase 5.** Both providers feed the same UI table, so they must return a normalized `Release` shape.

**The implementation:**
- `services/library/internal/parser/nyaa/client.go` — RSS client; parses Nyaa's `?page=rss` output.
- `services/library/internal/parser/animetosho/client.go` — JSON feed client at `feed.animetosho.org/json?show=mal&id={mal_id}` (preferred) or `?q={query}` (fallback).
- `services/library/internal/domain/release.go` — shared `Release` type.
- `services/library/internal/service/search.go` — merger + dedupe + rank.
- `services/library/internal/handler/search.go` — `GET /api/library/search`.
- Both parsers extract the infohash from the magnet URI via `github.com/anacrolix/torrent/metainfo` (`ParseMagnetURI`) so the merger can dedupe correctly across providers.

## Requirements

### LIB-03: Nyaa.si RSS client

- **Current:** No `services/library/internal/parser/nyaa/`.
- **Target:**
  - `client.go` with:
    ```go
    type Client struct { /* http client + base url */ }
    func NewClient(cfg Config) *Client
    type Config struct { BaseURL string; HTTPTimeout time.Duration; UserAgent string }
    func (c *Client) Search(ctx context.Context, query string, limit int) ([]Release, error)
    ```
  - Hits `GET {BaseURL}/?page=rss&q={query}&c=1_2&f=0` (`c=1_2` = anime category; `f=0` = no quality filter).
  - Parses RSS via `encoding/xml`. Item fields used: `title`, `link` (magnet — under Nyaa's namespace), `dc:creator` (uploader), `pubDate`, `size` (under Nyaa's namespace).
  - Extracts quality (`1080p` / `720p` / `480p` / `2160p`) from the title via regex (`(?i)\b(2160|1080|720|480)p\b`).
  - Extracts infohash via `metainfo.ParseMagnetURI(magnetLink)`.
- **Acceptance:** Unit tests against an httptest mock with a sample RSS payload assert the parsed `Release` shape. Search request includes the correct query-string parameters. Error path (HTTP 5xx) returns a wrapped error so the merger can downgrade.

### LIB-04: AnimeTosho JSON-feed client

- **Current:** No `services/library/internal/parser/animetosho/`.
- **Target:**
  - `client.go` with:
    ```go
    type Client struct { /* http client + base url */ }
    func NewClient(cfg Config) *Client
    type Config struct { BaseURL string; HTTPTimeout time.Duration; UserAgent string }
    type SearchParams struct { MALID int; Query string; Limit int }
    func (c *Client) Search(ctx context.Context, p SearchParams) ([]Release, error)
    ```
  - When `p.MALID > 0`: hits `GET {BaseURL}/json?show=mal&id={mal_id}` — preferred path.
  - When `p.MALID == 0`: hits `GET {BaseURL}/json?q={query}` — fallback path.
  - Parses JSON array items into `Release` (fields: `title`, `link` → magnet, `nyaa_subcat`, `info_hash`, `total_size`, `timestamp`, `quality` extracted from title, `uploader` extracted from title — AnimeTosho doesn't expose uploader directly so we regex it from the `[Uploader]` prefix in title).
- **Acceptance:** Unit tests against httptest mocks for both MAL-ID-path and query-path. Returns empty slice on `[]` response. Wraps non-200 in a typed error.

### LIB-04b: Merger + endpoint

- **Current:** No search endpoint on the library service.
- **Target:**
  - `services/library/internal/domain/release.go` with the shared `Release` type:
    ```go
    type Release struct {
        Title      string    `json:"title"`
        Magnet     string    `json:"magnet"`
        InfoHash   string    `json:"info_hash"`
        Uploader   string    `json:"uploader,omitempty"`
        Quality    string    `json:"quality,omitempty"`
        SizeBytes  int64     `json:"size_bytes,omitempty"`
        Source     string    `json:"source"`     // "nyaa" or "animetosho"
        MALID      int       `json:"mal_id,omitempty"`
        FoundAt    time.Time `json:"found_at"`
    }
    ```
  - `services/library/internal/service/search.go`:
    - `Searcher` interface: `Search(ctx, SearchParams) ([]domain.Release, error)`.
    - `SearchAggregator` wraps the two clients + a logger.
    - `FetchAll(ctx, params) ([]Release, []string error)` — second return value is the list of provider names that failed (empty when both succeeded).
    - Runs both clients via `errgroup.Group{SetLimit(2)}`, fail-soft per provider.
    - Dedupes by `(InfoHash)` — keeps the AnimeTosho hit when both providers return the same infohash (richer metadata).
    - Ranks AnimeTosho-with-MAL-ID hits to the top, then everything else by `FoundAt DESC`.
  - `services/library/internal/handler/search.go`:
    - `GET /api/library/search?q=&mal_id=&limit=` (default limit 50, cap 200).
    - Returns `{"releases": [...], "providers_down": ["nyaa"|"animetosho"|...]}`.
    - Admin-only (gated by the gateway's existing AdminMiddleware once Phase 1 wires it; in v0.2 the library routes are admin-only end-to-end).
- **Acceptance:**
  1. `curl 'http://localhost:8087/api/library/search?q=bocchi+the+rock&mal_id=52082'` returns ≥1 result from each provider (assuming upstream reachable).
  2. AnimeTosho hits sort to the top when MAL ID is provided.
  3. Duplicate infohash returns once.
  4. One provider down → still 200 with `providers_down: ["..."]`.

## Acceptance Criteria

1. `services/library/internal/parser/nyaa/{client.go,client_test.go}` exists. `go test ./internal/parser/nyaa/...` passes.
2. `services/library/internal/parser/animetosho/{client.go,client_test.go}` exists. `go test ./internal/parser/animetosho/...` passes.
3. `services/library/internal/domain/release.go` defines the shared `Release` type.
4. `services/library/internal/service/search.go` runs both clients in parallel + dedupes + ranks.
5. `services/library/internal/handler/search.go` mounts `GET /api/library/search`.
6. Live smoke: `curl '/api/library/search?q=frieren&mal_id=52991'` returns 200 with a non-empty `releases` array (assuming `feed.animetosho.org` reachable from the deploy environment).
7. With one provider's base URL set to an unreachable host, the same request returns 200 with `providers_down` populated.
8. `go build ./...` from `services/library/` clean. `go vet` clean.

## Auto-selected implementation decisions

- **HTTP client:** Standard `net/http` with 15s timeout per request (longer than catalog parsers because torrent indexers are slower and more variable than streaming APIs).
- **User-Agent:** `AnimeEnigma/1.0 (library service)` — both providers tolerate generic agents.
- **Rate-limiting:** None at the parser layer; the admin UI's search input is debounced + manual, so accidental flooding is unlikely. If needed, add later via `golang.org/x/time/rate`.
- **Quality regex:** Single regex `(?i)\b(2160|1080|720|480)p\b` captures the most common values; 4K, 360p, etc. surface as empty `Quality`.
- **Uploader regex (AnimeTosho):** Match leading `[Group]` bracket; if absent, leave empty.
- **Dedupe key:** `InfoHash` only (NOT magnet URI — magnet strings can vary in tracker list and dn parameter for the same content).
- **Provider name on `Release.Source`:** lowercase strings `"nyaa"` / `"animetosho"` — matches the Phase 5 chip label convention.
- **Search endpoint auth:** Admin-only — `services/gateway/internal/router/routes.go` AdminMiddleware on the entire `/api/library/*` prefix is the simplest gate; v0.3 may carve out user-facing endpoints later.
- **Anacrolix dependency:** `github.com/anacrolix/torrent` is already required in Phase 3 — adding the import in Phase 2 (for `metainfo.ParseMagnetURI`) is acceptable double-importing; we'll pin the version in Phase 3.

## Touches

- **New:** `services/library/internal/parser/nyaa/{client.go,client_test.go}`
- **New:** `services/library/internal/parser/animetosho/{client.go,client_test.go}`
- **New:** `services/library/internal/domain/release.go`
- **New:** `services/library/internal/service/search.go`
- **New:** `services/library/internal/handler/search.go`
- **Extend:** `services/library/internal/transport/router.go` (register route)
- **Extend:** `services/library/internal/config/config.go` (new `NyaaConfig`, `AnimeToshoConfig`)
- **Extend:** `services/library/cmd/library-api/main.go` (wire clients + service + handler)
- **Extend:** `services/library/go.mod` (add `github.com/anacrolix/torrent`)
- **Extend:** `docker/.env.example` (document `NYAA_BASE_URL`, `ANIMETOSHO_BASE_URL`, `LIBRARY_SEARCH_TIMEOUT`)

## Out of Scope (for this phase)

- Job enqueueing / torrent downloads (Phase 3).
- Encoding / MinIO (Phase 4).
- Admin UI (Phase 5).
- Caching the search response (not worth the complexity; admin searches are ad-hoc).

## Citations to design doc

- Architecture → "library service / nyaa client" + "AnimeTosho JSON feed (preferred — MAL ID filter)".
- Architecture → "Nyaa.si RSS fallback".
- Tech-choices → "Trusted uploaders for raws: Ohys-Raws, Leopard-Raws, ARC-Raws, SubsPlease" (informs the quality regex but no per-uploader logic here).
