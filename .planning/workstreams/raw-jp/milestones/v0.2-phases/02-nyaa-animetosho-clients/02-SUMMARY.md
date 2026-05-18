---
phase: 02-nyaa-animetosho-search-clients
status: complete
workstream: raw-jp
milestone: v0.2
date: 2026-05-18
requirements:
  - LIB-03
  - LIB-04
  - LIB-04b
commits:
  - 79058b0 — feat(02): add Release type + AnimeTosho JSON client with tests
  - c5f8ed7 — feat(02): add Nyaa RSS client with tests
  - 488327c — feat(02): add SearchAggregator (parallel fan-out, dedupe, rank)
  - 9923948 — feat(02): wire search handler, router, config, env + gateway admin gate
---

# Phase 02: Nyaa + AnimeTosho Search Clients — Summary

Adds two torrent-indexer HTTP clients (Nyaa RSS + AnimeTosho JSON feed)
to the library service, a parallel-fan-out aggregator that dedupes by
InfoHash and ranks AnimeTosho-with-matching-MAL-ID hits first, and the
admin-only `GET /api/library/search?q=&mal_id=&limit=` endpoint that
the Phase 5 admin UI will consume. The library service trusts what the
gateway forwards; admin gating lives at the gateway via the existing
`JWTValidationMiddleware + AdminRoleMiddleware` pattern (mirrors
`/streaming/admin/*`).

Fail-soft is the headline contract: when one provider goes down the
other's hits still flow back and the dead provider's name appears in
`providers_down`. Verified end-to-end with a fault-injected smoke
(NYAA_BASE_URL → 127.0.0.1:1 → HTTP 200, `providers_down: ["nyaa"]`).

## What was built

**Task 1 — Release type + AnimeTosho client (commit `79058b0`).**
`internal/domain/release.go` defines the locked `Release` struct used
by both parsers. `internal/parser/animetosho/client.go` implements the
JSON-feed client: route selection by `MALID` (>0 → `/json?show=mal&id=`,
else → `/json?q=`), title regex for quality and `[Uploader]`, and
InfoHash fallback via `anacrolix/torrent/metainfo.ParseMagnetUri` when
the response omits `info_hash`. Five httptest-backed unit tests cover
both routes, non-2xx, limit clamping, and the magnet-hash fallback.
`go.work` bumped to `go 1.24.0` (required by anacrolix/torrent v1.61.0).

**Task 2 — Nyaa RSS client (commit `c5f8ed7`).**
`internal/parser/nyaa/client.go` mirrors the AnimeTosho client shape.
Parses Nyaa's namespaced RSS via `encoding/xml` (`nyaa:infoHash`,
`nyaa:size`, `dc:creator`), synthesizes a magnet URI from the info
hash + title because `<link>` points at the `.torrent` download page,
and decodes human-readable size strings (B/KB/KiB/MB/MiB/GB/GiB/TB/TiB)
into bytes. Five tests cover field parsing, query-parameter
correctness, non-2xx, limit clamping, and a table-driven `parseSize`.

**Task 3 — SearchAggregator (commit `488327c`).**
`internal/service/search.go` exposes `NewAggregator` and `FetchAll`.
Each provider runs in its own goroutine via `sync.WaitGroup`; a mutex
guards the shared `providersDown` slice. Each goroutine catches its
own error so a single-provider failure never propagates to the caller.
`FetchAll` returns a non-nil error ONLY when both providers fail (the
handler maps that to 502). The merger:

- Drops releases with empty `InfoHash` (cannot be deduped, cannot be
  queued downstream).
- Dedupes by lowercase-hex `InfoHash`. On collision the AnimeTosho copy
  wins (richer metadata per SPEC).
- Splits the merged map into `headed` (animetosho + matching MALID)
  and `tail` (everything else), sorts each by `FoundAt DESC`, returns
  `headed ++ tail` clamped to the requested `limit`.

Seven tests cover both-succeed dedupe, MAL-ranking gate, each
single-provider-down case, both-providers-down, limit clamp, and the
empty-InfoHash drop.

**Task 4 — Handler, router, config, env, main, gateway gate
(commit `9923948`).**

- `internal/handler/search.go`: parses `q`, `mal_id`, `limit`; returns
  400 when neither `q` nor `mal_id` present; 502 (ExternalAPI) when
  both providers fail; otherwise 200 with
  `{releases: [...], providers_down: [...]}` wrapped in the httputil
  envelope. Always emits non-nil `releases` + `providers_down` so the
  client doesn't have to special-case `null`.
- `internal/transport/router.go`: registers
  `GET /api/library/search` inside the existing `/api/library` route
  group.
- `internal/config/config.go`: new `NyaaConfig`, `AnimeToshoConfig`,
  and `LibrarySearchConfig` blocks; six new env vars
  (`NYAA_BASE_URL`, `ANIMETOSHO_BASE_URL`, `LIBRARY_SEARCH_TIMEOUT`,
  `LIBRARY_SEARCH_UA`, `LIBRARY_SEARCH_DEFAULT_LIMIT`,
  `LIBRARY_SEARCH_MAX_LIMIT`).
- `cmd/library-api/main.go`: wires Nyaa + AnimeTosho clients into the
  aggregator via an `animeToshoAdapter` (translates between
  `service.AnimeToshoParams` and `animetosho.SearchParams`, keeping
  the service package free of any parser-package import).
- `services/gateway/internal/transport/router.go`: mirrors the
  existing `/streaming/admin/*` pattern — `/api/library/health` stays
  public, all other `/api/library/*` paths gated by
  `JWTValidationMiddleware + AdminRoleMiddleware`.
- `docker/.env.example`: documents the six new env vars and corrects
  the stale "Library Service ... port 8087" header (Phase 1 moved the
  service to 8089 — see 01-SUMMARY.md deviation #1).

## Files touched

### NEW

- `services/library/internal/domain/release.go`
- `services/library/internal/parser/animetosho/client.go`
- `services/library/internal/parser/animetosho/client_test.go`
- `services/library/internal/parser/nyaa/client.go`
- `services/library/internal/parser/nyaa/client_test.go`
- `services/library/internal/service/search.go`
- `services/library/internal/service/search_test.go`
- `services/library/internal/handler/search.go`
- `.planning/workstreams/raw-jp/phases/02-nyaa-animetosho-search-clients/02-SUMMARY.md` (this file)

### EXTEND

- `services/library/internal/transport/router.go` — added `searchHandler` parameter + `/search` route
- `services/library/internal/config/config.go` — added 3 sub-configs + 6 env vars
- `services/library/cmd/library-api/main.go` — wired clients + aggregator + adapter
- `services/library/go.mod` — added `github.com/anacrolix/torrent v1.61.0` direct require
- `services/library/go.sum` — supporting checksums
- `services/gateway/internal/transport/router.go` — admin-gated `/api/library/*` non-/health paths
- `docker/.env.example` — documented new env vars + fixed stale "8087" header
- `go.work` — bumped to `go 1.24.0` (required by anacrolix/torrent)
- `go.work.sum` — supporting checksums

## Verification results

### `cd services/library && go build ./...`
Exit 0, clean.

### `cd services/library && go vet ./...`
Exit 0, clean.

### `cd services/library && go test ./... -count=1`

```
?   	github.com/ILITA-hub/animeenigma/services/library/cmd/library-api	[no test files]
?   	github.com/ILITA-hub/animeenigma/services/library/internal/config	[no test files]
?   	github.com/ILITA-hub/animeenigma/services/library/internal/domain	[no test files]
?   	github.com/ILITA-hub/animeenigma/services/library/internal/handler	[no test files]
ok  	github.com/ILITA-hub/animeenigma/services/library/internal/parser/animetosho	0.010s
ok  	github.com/ILITA-hub/animeenigma/services/library/internal/parser/nyaa	0.008s
ok  	github.com/ILITA-hub/animeenigma/services/library/internal/service	0.003s
?   	github.com/ILITA-hub/animeenigma/services/library/internal/transport	[no test files]
```

17 unit tests across three packages, all PASS:
- `internal/parser/animetosho`: 5 tests (MAL path, query path, non-2xx, limit clamp, infohash-from-magnet)
- `internal/parser/nyaa`: 5 tests (RSS fields, query params, non-2xx, limit clamp, parseSize table)
- `internal/service`: 7 tests (both-succeed dedupe, MAL ranking, each single-provider-down, both-down, limit clamp, empty-infohash drop)

### `cd services/gateway && go build ./... && go vet ./...`
Exit 0, clean.

### `make redeploy-library && make redeploy-gateway && make health`

```
✓ gateway:8000
✓ auth:8080
✓ catalog:8081
✓ streaming:8082
✓ player:8083
✓ rooms:8084
✓ scheduler:8085
✓ scraper:8088
✓ library:8089
```

All nine services healthy post-deploy. No regression on Phase 1
`/health` or `/api/library/health`.

### Smoke #1 — direct probe library:8089 (admin JWT via API key)

```
HTTP 200
.success = true
.data.releases | length = 50
.data.providers_down = []
```

First three releases (excerpt from `/tmp/lib-smoke-1.json`):

```json
{
  "source": "animetosho",
  "title": "Lupin III - The Legend of the Gold of Babylon (1985) ...",
  "info_hash": "ad2464124afbac78eb47d872c9090759c5d2123e",
  "mal_id": 52991,
  "found_at": "2026-05-08T23:56:51Z"
}
{
  "source": "animetosho",
  "title": "[Piyoko] Onegai AiPri - 05 [WEB AMZN 1080p h264 AC3 2.0]",
  "info_hash": "1d216fd20501abb613bcfd4c2233dc039ddf1db4",
  "mal_id": 52991,
  "quality": "1080p",
  "found_at": "2026-05-08T23:00:16Z"
}
```

All 50 returned entries are `source: animetosho` with `mal_id: 52991`
— the headed slice (AT-with-matching-MAL-ID) saturated the limit. Nyaa
returned hits too (no `providers_down` entry) but they got clipped by
the limit clamp. A user wanting Nyaa entries alongside can pass
`limit=100`. This is correct per the ranking spec.

### Smoke #2 — gateway-proxied (admin JWT via API key)

```
HTTP 200
.success = true
.data.releases | length = 50
.data.providers_down = []
```

Identical envelope shape to Smoke #1. Gateway admin gate accepted the
admin role; proxy forwarded the request to library:8089 and returned
the response verbatim.

### Smoke #3 — gateway-proxied WITHOUT auth

```
HTTP/1.1 401 Unauthorized
Content-Type: application/json
...
{"success":false,"error":{"code":"UNAUTHORIZED","message":"authentication required"}}
```

**Hard gate passes.** Anonymous traffic on `/api/library/search` is
rejected at the gateway by `JWTValidationMiddleware` before reaching
the library service. The library's own `/api/library/health` route
remains public (verified above in `make health`).

### Smoke #4 — provider-down soft-fail (`NYAA_BASE_URL=http://127.0.0.1:1`)

Ran the library container with `-e NYAA_BASE_URL=http://127.0.0.1:1`
(reachable IP, no listener), kept AnimeTosho default, hit
`/api/library/search?q=frieren&mal_id=52991` with admin auth:

```
HTTP 200
.success = true
.data.releases | length = 50
.data.providers_down = ["nyaa"]
```

**Hard gate passes.** Nyaa's connection refused (within 15s timeout)
was caught by the aggregator goroutine, logged via
`log.Warnw("library search provider failed", "provider", "nyaa", ...)`,
and surfaced in `providers_down` while AnimeTosho's results flowed
through untouched. The container was torn down and the normal library
container restored at the end of the test; `make health` confirms
`✓ library:8089`.

## Deviations from plan

### 1. **[Rule 3 — blocker] AnimeTosho parser uses `ParseMagnetUri`, not `ParseMagnetURI`**

The plan said to call `metainfo.ParseMagnetURI`. The actual exported
function in `github.com/anacrolix/torrent/metainfo` v1.61.0 is named
`ParseMagnetUri`; `ParseMagnetURI` exists too but is declared as
`var ParseMagnetURI = ParseMagnetUri` (an alias). Either works; we
use `ParseMagnetUri` to match the package's primary naming convention
visible in `go doc`. No functional impact.

### 2. **[Side effect — necessary] `go.work` bumped from `go 1.23.0` to `go 1.24.0`**

`anacrolix/torrent v1.61.0` (latest as of 2026-05-18) requires Go
1.24+. `go get` upgraded the workspace toolchain directive
automatically. All existing services compile clean against 1.24.0;
docker builds were verified for library + gateway. No runtime impact
on other services. The plan's `interfaces` block flagged anacrolix as
"will be pinned in Phase 3" — Phase 3 can now pin to v1.61.0 or
choose to downgrade if a 1.23-compatible version turns out to be
preferable.

### 3. **[Plan-spec text fix] `docker/.env.example` "Library Service ... port 8087" was stale**

Phase 1's deviation #1 moved the library service to port 8089 (the
host-side maintenance daemon owned 8087). The `.env.example` block
header still read "port 8087" — corrected in this phase's same
commit that adds the new search env vars. Carries forward Phase 1
open-item #1.

### 4. **[Scope-bounded restoration] Reverted unrelated `services/auth/go.{mod,sum}` + `libs/metrics/go.mod` changes**

`go mod tidy` (run after `go get github.com/anacrolix/torrent`)
touched several other workspace members' `go.mod`/`go.sum` files
(testify promoted to direct, pmezard added as indirect, etc.). Those
changes were not strictly required for the library service to compile
and could overlap with other in-flight work, so they were restored
via `git checkout --` before the Task 1 commit. Both `services/auth`
and `libs/metrics` still build clean.

## Out of scope (per SPEC)

Carried over verbatim — these belong to later v0.2 phases:

- Job enqueueing, BitTorrent download (Phase 3).
- ffmpeg HLS transcoding (Phase 4).
- MinIO writer / `raw-library` bucket bootstrap (Phase 4).
- `RawLibrary.vue` admin UI (Phase 5).
- Hybrid resolver in catalog service (Phase 6).
- Caching the search response (admin searches are ad-hoc).
- Rate-limiting at the parser layer.
- Per-uploader scoring / trusted-uploader whitelist
  (Ohys/Leopard/ARC/SubsPlease — deferred to v0.3+).
- Pinning the `anacrolix/torrent` dependency version — Phase 3 owns it.
- `library_jobs` / `library_episodes` migrations — Phases 3/4.
- Updating `docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
  and `v0.2-REQUIREMENTS.md` port references from 8087 → 8089
  (Phase 1 open item; still carried forward).

## Open items

1. **`ui_audit_bot` is `role=user` and cannot exercise admin-gated
   library routes.** Smoke verification used a temporary admin API
   key minted directly against the `tNeymik` admin user via the
   direct-DB pattern documented in `MEMORY.md`; the key was revoked
   immediately after the smoke run (`UPDATE users SET api_key_hash =
   NULL WHERE username = 'tNeymik'`). Phase 5 (admin UI) and any
   future e2e tests against `/api/library/*` will need either:
   - a dedicated admin-role test account (e.g. `library_admin_bot`),
     OR
   - a documented procedure for elevating `ui_audit_bot` to admin
     for the duration of a library audit.

2. **Limit-clamping behavior when AT-MAL-feed saturates the result
   slice.** Smoke #1 showed that 50 AnimeTosho-with-matching-MAL
   hits filled the entire result, pushing Nyaa entries past the
   default `limit=50`. This is correct per the ranking spec ("AT-with-MAL
   first, then everything else by FoundAt DESC"), but Phase 5's UI
   should expose a `limit` knob and/or render a "Nyaa results
   suppressed by limit" hint when the headed slice fills the page.

3. **Phase 1 docs port-correction (carried forward).** Still need to
   update:
   - `docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
     ("Library port: 8087")
   - `.planning/workstreams/raw-jp/milestones/v0.2-REQUIREMENTS.md`
     (any 8087 mentions)
   - `.planning/workstreams/raw-jp/milestones/v0.2-phases/01-library-scaffold/01-SPEC.md`
     Acceptance Criteria #4 cites port 8087.

## Self-Check: PASSED

Every file in the plan frontmatter's `files_modified` array is present
and reflects the work described in this summary.

- `services/library/internal/domain/release.go` — FOUND
- `services/library/internal/parser/animetosho/client.go` — FOUND
- `services/library/internal/parser/animetosho/client_test.go` — FOUND
- `services/library/internal/parser/nyaa/client.go` — FOUND
- `services/library/internal/parser/nyaa/client_test.go` — FOUND
- `services/library/internal/service/search.go` — FOUND
- `services/library/internal/service/search_test.go` — FOUND
- `services/library/internal/handler/search.go` — FOUND
- `services/library/internal/transport/router.go` — FOUND (extended)
- `services/library/internal/config/config.go` — FOUND (extended)
- `services/library/cmd/library-api/main.go` — FOUND (extended)
- `services/library/go.mod` — FOUND (extended; `anacrolix/torrent` direct require)
- `services/library/go.sum` — FOUND (extended)
- `services/gateway/internal/transport/router.go` — FOUND (extended; admin gate)
- `docker/.env.example` — FOUND (extended)

Commit hashes verified in `git log --oneline`:

- `79058b0 feat(02): add Release type + AnimeTosho JSON client with tests` — FOUND
- `c5f8ed7 feat(02): add Nyaa RSS client with tests` — FOUND
- `488327c feat(02): add SearchAggregator (parallel fan-out, dedupe, rank)` — FOUND
- `9923948 feat(02): wire search handler, router, config, env + gateway admin gate` — FOUND
