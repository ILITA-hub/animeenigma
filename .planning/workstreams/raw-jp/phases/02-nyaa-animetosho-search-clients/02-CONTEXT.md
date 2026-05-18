# Phase 2: Nyaa + AnimeTosho Search Clients - Context

**Gathered:** 2026-05-18
**Status:** Ready for planning
**Mode:** Auto-generated (SPEC pre-written, ambiguity_score 0.15)

<domain>
## Phase Boundary

Two HTTP parser modules at `services/library/internal/parser/{nyaa,animetosho}/` plus a merger service that fans out to both in parallel and returns a deduped, ranked `Release[]` to the admin UI. Exposed at `GET /api/library/search?q=&mal_id=&limit=`. AnimeTosho is preferred when a MAL ID is provided; Nyaa is the fallback.

**Out of scope:** Job enqueueing, torrent downloads, encoding, MinIO, admin UI, response caching.

</domain>

<decisions>
## Implementation Decisions

### Locked from SPEC (`milestones/v0.2-phases/02-nyaa-animetosho-clients/02-SPEC.md`)

- **HTTP client:** `net/http` with 15s timeout per request.
- **User-Agent:** `AnimeEnigma/1.0 (library service)`.
- **Rate-limiting:** None at parser layer.
- **Quality regex:** `(?i)\b(2160|1080|720|480)p\b`.
- **Uploader regex (AnimeTosho):** Leading `[Group]` bracket; empty if absent.
- **Dedupe key:** `InfoHash` only (not magnet URI).
- **Provider names:** lowercase `"nyaa"` / `"animetosho"`.
- **Auth:** Admin-only via gateway AdminMiddleware on `/api/library/*` prefix.
- **Infohash extraction:** `github.com/anacrolix/torrent/metainfo.ParseMagnetURI` (dependency double-imported, will be pinned in Phase 3).

### Module Layout (locked)

- `services/library/internal/parser/nyaa/{client.go,client_test.go}`
- `services/library/internal/parser/animetosho/{client.go,client_test.go}`
- `services/library/internal/domain/release.go` — shared Release type
- `services/library/internal/service/search.go` — merger + dedupe + rank
- `services/library/internal/handler/search.go` — HTTP handler

### Endpoint Contract (locked)

- `GET /api/library/search?q=&mal_id=&limit=` (default limit 50, cap 200)
- Response: `{"releases": [...], "providers_down": ["..."]}`
- 200 even when one provider down, with provider name in `providers_down`

### Release Type (locked)

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

### Claude's Discretion (autonomous mode)

- Internal helper function names
- Exact `errgroup` configuration (will use `SetLimit(2)`)
- Test fixture content shape (use realistic-looking samples)
- Whether to add a Search interface in domain vs service package (favor service-local)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/library/` — Phase 1 scaffold already in place with chi router, config, transport, logger, metrics.
- `services/library/internal/config/config.go` — extend with `NyaaConfig` and `AnimeToshoConfig` blocks.
- `services/library/cmd/library-api/main.go` — wire new clients + service + handler.
- `libs/httputil` — response envelope helpers.
- `libs/logger` — structured logging.
- `libs/errors` — error wrapping.
- Existing parser pattern in catalog: `services/catalog/internal/parser/{kodik,animelib,hianime,consumet,allanime}/` — follow shape (Client struct, NewClient(cfg), Search method).

### Established Patterns

- HTTP clients use `net/http` with explicit timeout.
- Tests use `httptest` with mock servers + fixture payloads.
- RSS parsing: `encoding/xml` with namespaced custom tags (handled via xml tag with namespace prefix).
- Error wrapping via `libs/errors.Wrap`.
- Service injection: main.go composes clients + services, transport.NewRouter takes them as deps.

### Integration Points

- `services/library/internal/transport/router.go` — register `GET /api/library/search` (path `/search` since router is mounted at root).
- `services/library/internal/config/config.go` — extend with Nyaa + AnimeTosho + LibrarySearch sub-configs.
- `docker/.env.example` — document `NYAA_BASE_URL`, `ANIMETOSHO_BASE_URL`, `LIBRARY_SEARCH_TIMEOUT`.

</code_context>

<specifics>
## Specific Ideas

- SPEC reference at `milestones/v0.2-phases/02-nyaa-animetosho-clients/02-SPEC.md` is authoritative.
- Nyaa RSS URL: `{BaseURL}/?page=rss&q={query}&c=1_2&f=0` (default `BaseURL: https://nyaa.si`).
- AnimeTosho JSON URL: `{BaseURL}/json?show=mal&id={mal_id}` (MAL path) or `{BaseURL}/json?q={query}` (fallback). Default `BaseURL: https://feed.animetosho.org`.

</specifics>

<deferred>
## Deferred Ideas

- Per-uploader scoring or trusted-uploader whitelist (Ohys, Leopard, ARC, SubsPlease) — not a v0.2 must-have.
- Caching the search response — not worth complexity; admin searches are ad-hoc.
- Rate-limiting at parser layer — if needed later, add via `golang.org/x/time/rate`.

</deferred>
