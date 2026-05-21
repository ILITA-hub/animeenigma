---
phase: 01-backend-aggregator
plan: 05
workstream: hero-spotlight
subsystem: gateway
tags: [gateway, routing, proxy, spotlight]
requirements: [HSB-BE-06]
dependency_graph:
  requires:
    - services/gateway/internal/handler/proxy.go (existing ProxyToCatalog)
    - services/gateway/internal/transport/router_test.go (existing buildTestGatewayRouter)
  provides:
    - "GET /api/home/spotlight gateway → catalog passthrough (public)"
  affects:
    - services/gateway/internal/transport/router.go (1 chi route line added)
tech_stack:
  added: []
  patterns:
    - "chi public catalog passthrough (precedent: /skip-times/*, /collections/*)"
    - "TDD RED → GREEN with handwritten httptest backend spy (precedent: TestRouter_AdminScraper*)"
key_files:
  created:
    - services/gateway/internal/transport/router_spotlight_test.go
  modified:
    - services/gateway/internal/transport/router.go
decisions:
  - "Route placed alongside /collections/* in the public catalog block (NOT in any JWT-gated r.Group). Spotlight v1.0 Phase 1 is anonymous; later phases will add optional-auth on the catalog side rather than enforced auth at the gateway."
  - "Reused router_test.go's buildTestGatewayRouter helper instead of writing a parallel test harness — catalog/scraper backend spies were already in place and the two new tests slot in alongside the existing TestRouter_AdminScraper* family."
  - "Acceptance check 3 (TestRouter_Spotlight_NotJWTProtected) replaces the impractical structural check 'verify the route is not inside any JWT-protected r.Group' with a behavioural check: send a request with no Authorization header, assert it reaches the catalog backend at 200 (not 401)."
metrics:
  duration_minutes: 4
  tasks_completed: 1
  files_modified: 2
  completed_date: 2026-05-21
---

# Phase 1 Plan 05: Gateway Proxy for `/api/home/spotlight` Summary

**One-liner:** Single chi route line added to the gateway router that proxies the public, JWT-free path `/api/home/spotlight` to the catalog service via `ProxyToCatalog`, locked in by two contract tests that pin route-target and public-access invariants.

## What Changed

### Modified

- **`services/gateway/internal/transport/router.go`** — added 10 lines (one route registration + a 9-line block-doc comment) inside the `r.Route("/api", func(r chi.Router) { ... })` block, directly after the existing public `/collections/*` catalog passthroughs and **before** the JWT-gated `/admin/scraper/*` and `/admin/*` groups. The new line:

  ```go
  // Workstream hero-spotlight, v1.0 Phase 1 (HSB-BE-06) — hero spotlight
  // aggregator. Public, NO JWT (Phase 1 is anonymous; future personalized
  // cards will use optional-auth on the catalog side, not enforced auth
  // here). Mounts at /api/home/spotlight; the catalog proxy path-rewrite
  // is a no-op so the catalog router sees the same path. Registered
  // alongside the other public catalog passthroughs above; /home/* does
  // not collide with /anime/* but the "specific-before-general" placement
  // convention is project-wide.
  r.HandleFunc("/home/spotlight", proxyHandler.ProxyToCatalog)
  ```

### Created

- **`services/gateway/internal/transport/router_spotlight_test.go`** — new test file with 2 tests that pin the HSB-BE-06 contract:
  - `TestRouter_Spotlight_ProxiesToCatalog` — proves the request lands on the catalog backend (not scraper / player / web) at the unchanged path `/api/home/spotlight`. Re-uses `buildTestGatewayRouter` from `router_test.go`, which already spins up parallel httptest spy backends for catalog and scraper.
  - `TestRouter_Spotlight_NotJWTProtected` — proves an anonymous request (no `Authorization` header) reaches the catalog backend at HTTP 200 rather than bouncing off 401 at the gateway. If anyone later nests the route inside a JWT-gated `r.Group`, this test fails loudly.

## Commits

| Commit | Type | Description |
|--------|------|-------------|
| `c5f7143` | test | RED phase — failing tests, both fail with 404 because the route is unregistered |
| `0fe9235` | feat | GREEN phase — 1 chi route line in router.go, both tests flip to PASS |

## Verification

```
$ go build ./services/gateway/...                                                  # exit 0
$ go test ./services/gateway/internal/transport/... -count=1 -run "TestRouter_Spotlight" -v
=== RUN   TestRouter_Spotlight_ProxiesToCatalog
--- PASS: TestRouter_Spotlight_ProxiesToCatalog (0.00s)
=== RUN   TestRouter_Spotlight_NotJWTProtected
--- PASS: TestRouter_Spotlight_NotJWTProtected (0.00s)
PASS
ok  	github.com/ILITA-hub/animeenigma/services/gateway/internal/transport	0.016s

$ go test ./services/gateway/... -count=1 -short                                   # all pass
?   	github.com/ILITA-hub/animeenigma/services/gateway/cmd/gateway-api	[no test files]
ok  	github.com/ILITA-hub/animeenigma/services/gateway/internal/config	0.004s
ok  	github.com/ILITA-hub/animeenigma/services/gateway/internal/handler	0.006s
ok  	github.com/ILITA-hub/animeenigma/services/gateway/internal/service	0.009s
ok  	github.com/ILITA-hub/animeenigma/services/gateway/internal/transport	0.099s
```

Static placement checks:

```
$ awk '/r\.Route\("\/api"/,/^\t\}\)/' services/gateway/internal/transport/router.go | grep -c "/home/spotlight"
2   # block-doc comment line + the route registration line — both inside /api

$ grep -q '"/home/spotlight", proxyHandler.ProxyToCatalog' services/gateway/internal/transport/router.go && echo "ProxyToCatalog: PASS"
ProxyToCatalog: PASS

$ grep -c "^func TestRouter_Spotlight" services/gateway/internal/transport/router_spotlight_test.go
2
```

## Confirmations

- **Route is inside `r.Route("/api", ...)` block** — Yes. Inserted directly between the existing `r.HandleFunc("/collections/*", proxyHandler.ProxyToCatalog)` line and the `Phase 17 Plan 03: admin scraper routes` `r.Group` block. Brace-nesting confirmed by reading the file slice and by the awk check above.
- **Route is OUTSIDE every JWT-gated `r.Group`** — Yes. The two JWT-gated groups (`/admin/scraper/*` and `/admin/*`) come strictly AFTER our line; our line is in the public, ungated section alongside `/anime`, `/genres`, `/skip-times/*`, `/collections/*`. Pragmatically also proven by `TestRouter_Spotlight_NotJWTProtected` — an Authorization-less request returns 200, not 401.
- **Route uses `ProxyToCatalog`** — Confirmed by grep and by the spy assertion in `TestRouter_Spotlight_ProxiesToCatalog` (catalog backend channel fires; scraper backend channel does not).
- **Existing gateway tests still pass** — Confirmed: `services/gateway/internal/{config,handler,service,transport}` all green under `-count=1 -short`.

## Deviations from Plan

None — plan executed exactly as written. The `<acceptance_criteria>` block in the plan suggested a static check ("verify route is not inside any JWT-protected r.Group" via context-aware brace counting) but also offered the pragmatic substitute "TestRouter_Spotlight_NotJWTProtected proves anonymous access works". Used the pragmatic substitute since it provides a tighter behavioural guarantee with no false positives.

## Done Criteria

- [x] One added line in `services/gateway/internal/transport/router.go` inside the `/api` Route block
- [x] Route uses `ProxyToCatalog` (not any other service)
- [x] Route is NOT inside any JWT-protected `r.Group`
- [x] Two `TestRouter_Spotlight_*` tests pass under the existing gateway test harness
- [x] All other gateway tests still pass
- [x] `cd /data/animeenigma && go build ./services/gateway/...` exits 0
- [x] `cd /data/animeenigma && go test ./services/gateway/... -count=1 -short` exits 0

## TDD Gate Compliance

- **RED** (`c5f7143`): both tests authored and committed in the failing state (404 from chi because the route was unregistered).
- **GREEN** (`0fe9235`): single chi route line added, both tests flip to passing.
- **REFACTOR**: not needed — the change is one line + one comment block; no cleanup pending.

## Self-Check: PASSED

- `services/gateway/internal/transport/router.go` modification: FOUND (contains `/home/spotlight` inside `/api` block).
- `services/gateway/internal/transport/router_spotlight_test.go`: FOUND.
- Commit `c5f7143` (RED test): FOUND in `git log --oneline`.
- Commit `0fe9235` (GREEN impl): FOUND in `git log --oneline`.
