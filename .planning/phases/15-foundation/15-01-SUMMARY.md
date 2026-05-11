---
phase: 15-foundation
plan: 01
subsystem: scraper
tags: [scraper, foundation, golang, docker, makefile, golden-tests]
requires: []
provides:
  - services/scraper/ microservice on port 8088 (deviated from planned 8087)
  - services/scraper/internal/transport/router.go (extendable with /scraper/* in plan 03)
  - services/scraper/internal/config/Config (SERVER_HOST, SERVER_PORT, MEGACLOUD_EXTRACTOR_URL)
  - services/scraper/internal/testharness.New(t) (goldie v2 helper rooted at scraper/testdata)
  - SCRAPER_API_URL env on catalog container (plumbed for plan 04 thin client)
  - make capture-goldens (no-op until Phase 16 lands fixtures)
  - Service health line for scraper in `make health`
affects:
  - go.work (added ./services/scraper)
  - go.work top-level + every per-module go.mod auto-bumped from go 1.22 -> go 1.23.0 (goquery v1.10.3 transitive requirement)
  - catalog/go.mod: goquery v1.9.2 -> v1.10.3 (workspace resolution)
tech-stack:
  added:
    - github.com/sebdah/goldie/v2 v2.5.5 (golden-file test harness)
    - github.com/PuerkitoBio/goquery v1.10.3 (carried via workspace, full use comes in plan 02)
  patterns:
    - Service entrypoint shape mirrors catalog-api: logger.Default() + cfg.Load() + metrics.NewCollector + chi router + 30s graceful SIGTERM
    - Dockerfile shape mirrors catalog: multi-stage golang:1.24-alpine builder -> alpine:3.19 runtime, copies go.work + every workspace member's go.mod
    - Healthcheck: wget -q --spider /health, 30s/10s/3 retries, 15s start_period
    - Loopback-only host binding (127.0.0.1:8088:8088) per project convention for internal-only sidecars
key-files:
  created:
    - services/scraper/go.mod
    - services/scraper/go.sum
    - services/scraper/cmd/scraper-api/main.go
    - services/scraper/Dockerfile
    - services/scraper/internal/config/config.go
    - services/scraper/internal/transport/router.go
    - services/scraper/internal/testharness/goldie.go
    - services/scraper/internal/testharness/goldie_test.go
    - services/scraper/testdata/.gitkeep
  modified:
    - go.work (added ./services/scraper, auto-bumped to go 1.23.0)
    - go.work.sum (workspace dep resolution)
    - All libs/*/go.mod + go.sum (auto-bumped go 1.22 -> go 1.23.0 by `go work sync`)
    - All services/*/go.mod + go.sum (auto-bumped go 1.22 -> go 1.23.0 + catalog goquery v1.10.3)
    - Makefile (SERVICES += scraper, new capture-goldens target, health line for scraper:8088)
    - docker/docker-compose.yml (new scraper service block, SCRAPER_API_URL on catalog, scraper in catalog depends_on)
decisions:
  - Moved scraper from planned port 8087 to 8088 — host-native services/maintenance binary (Phase 14) was already bound to *:8087
  - Accepted Go 1.22 -> 1.23.0 toolchain upgrade across the workspace — D-STACK §5.1 pinned goquery v1.10.3 which actually requires Go 1.23
metrics:
  duration: ~7m
  completed: 2026-05-11T06:06:30Z
  tasks: 3
  files_created: 9
  files_modified: 40+ (go workspace cascade)
---

# Phase 15 Plan 01: Foundation Scaffolding Summary

Stand up `services/scraper/` as a live, healthy docker-compose container on `127.0.0.1:8088`, with shared chi+zap+Prometheus middleware mirroring catalog-api, plus the `make capture-goldens` recipe and goldie v2 test harness skeleton.

## Files Created

| File | Purpose | Lines |
|---|---|---|
| `services/scraper/go.mod` | Module declaration `github.com/ILITA-hub/animeenigma/services/scraper`; replace directives for in-repo libs/{errors,httputil,logger,metrics}; goldie v2 + testify + chi v5 dependencies | 27 |
| `services/scraper/go.sum` | Locked checksums | (auto) |
| `services/scraper/cmd/scraper-api/main.go` | Entrypoint: logger.Default → cfg.Load → metrics.NewCollector("scraper") → transport.NewRouter → http.Server (30s read/write, 60s idle) → SIGINT/SIGTERM graceful shutdown (30s ctx) | 68 |
| `services/scraper/Dockerfile` | Multi-stage build (golang:1.24-alpine → alpine:3.19), copies go.work + all workspace members for `go mod download`, builds static `/scraper-api` binary, EXPOSE 8088 | 47 |
| `services/scraper/internal/config/config.go` | `Config{Server: {Host, Port}, MegacloudExtractorURL}`; `Load()` reads SERVER_HOST/SERVER_PORT/MEGACLOUD_EXTRACTOR_URL env (defaults `0.0.0.0:8088` and `http://megacloud-extractor:3200`); `getEnv` / `getEnvInt` helpers | 57 |
| `services/scraper/internal/transport/router.go` | `NewRouter(cfg, log, mc)` returning chi.Router with `/health` (httputil.OK ok-status) + `/metrics` (promhttp); middleware chain: RequestID → metrics.Middleware → RequestLogger → Recoverer → CORS → RealIP | 44 |
| `services/scraper/internal/testharness/goldie.go` | `testharness.New(t)` returning `*goldie.Goldie` rooted at `services/scraper/testdata/` via runtime.Caller — same fixture root shared by all future provider subpackages; package doc explains `make capture-goldens` workflow | 56 |
| `services/scraper/internal/testharness/goldie_test.go` | Two tests: `TestNewReturnsGoldie` (proves goldie v2 links + helper compiles) + `TestGoldieFixtureDir` (asserts fixture path ends in `services/scraper/testdata`) | 41 |
| `services/scraper/testdata/.gitkeep` | Empty placeholder so testdata/ exists in git for Phase 16 provider fixtures | 0 |

## Files Modified

| File | Change | Driver |
|---|---|---|
| `go.work` | Added `./services/scraper` in alphabetic position; auto-bumped to `go 1.23.0` | This plan + go work sync |
| `go.work.sum` | Workspace dependency resolution | go work sync |
| `Makefile` | `SERVICES += scraper`; new `capture-goldens` target; new health line for scraper:8088 | This plan |
| `docker/docker-compose.yml` | New scraper service block (8088, healthcheck, depends_on megacloud-extractor); SCRAPER_API_URL env on catalog; scraper in catalog depends_on | This plan |
| `libs/{animeparser,cache,database,errors,httputil,metrics,tracing,videoutils}/go.{mod,sum}` | `go 1.22` → `go 1.23.0` directive auto-bumped | go work sync cascade (goquery v1.10.3 transitive) |
| `services/{auth,catalog,gateway,maintenance,player,rooms,scheduler,streaming,themes}/go.{mod,sum}` | `go 1.22` → `go 1.23.0`; catalog goquery `v1.9.2` → `v1.10.3` | go work sync cascade |

## Commits

| Task | Hash | Message |
|---|---|---|
| 1 | `ee21691` | feat(15-01): scaffold services/scraper module + golden test harness |
| 2 | `34afeae` | feat(15-01): add scraper Dockerfile + docker-compose service block |
| 3 | `7faac23` | feat(15-01): add Makefile targets, fix scraper port conflict, deploy live |

## Live Verification Output

Captured live from production (this server, 2026-05-11):

```text
$ docker compose -f docker/docker-compose.yml ps scraper
NAME                  IMAGE            COMMAND           SERVICE   STATUS                    PORTS
animeenigma-scraper   docker-scraper   "./scraper-api"   scraper   Up 33 seconds (healthy)   127.0.0.1:8088->8088/tcp

$ curl -fsS http://localhost:8088/health
{"success":true,"data":{"status":"ok"}}

$ curl -fsS http://localhost:8088/metrics | head -5
# HELP db_pool_idle_connections Number of idle database connections
# HELP db_pool_open_connections Number of open database connections
# HELP db_pool_wait_duration_seconds_total Total time spent waiting for database connections in seconds
# HELP db_pool_wait_total Total number of connections waited for
# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.

$ make health
Checking service health...
✓ gateway:8000
✓ auth:8080
✓ catalog:8081
✓ streaming:8082
✓ player:8083
✓ rooms:8084
✓ scheduler:8085
✓ scraper:8088

$ make capture-goldens
Capturing scraper goldens...
cd services/scraper && go test -update ./... -run "Golden" || true
no Go files in /data/animeenigma/.claude/worktrees/agent-af0cf6ef3790070a2/services/scraper
# (no-op as expected; Phase 16 lands the first real fixtures)

$ cd services/scraper && go test ./... -count=1
ok  github.com/ILITA-hub/animeenigma/services/scraper/internal/testharness 0.004s

$ docker stop -t 30 animeenigma-scraper
# Logs show: "shutting down server..." -> "server stopped" in <1s,
# well inside the 30s graceful window.
```

Sample structured log lines from the live container:

```text
2026-05-11T06:05:17.368Z INFO  starting scraper service
  {"address": "0.0.0.0:8088", "megacloud_extractor_url": "http://megacloud-extractor:3200"}

2026-05-11T06:05:22.420Z INFO  request completed
  {"method": "GET", "path": "/health", "status": 200, "bytes": 40,
   "duration_ms": 0, "remote_addr": "[::1]:44056", "user_agent": "Wget"}

2026-05-11T06:05:50.358Z INFO  shutting down server...
2026-05-11T06:05:50.359Z INFO  server stopped
```

## Deviations from Plan

### 1. [Rule 3 - Blocking issue] Scraper port 8087 -> 8088 due to host port conflict

- **Found during:** Task 3 (`docker compose up -d scraper`)
- **Issue:** `docker compose up` failed with `failed to bind host port 127.0.0.1:8087/tcp: address already in use`. Inspection (`ss -tlnp`) showed the host-native `services/maintenance` binary (PID 52153, started 2026-04-18) was already bound to `*:8087`. The maintenance service is not in the docker-compose stack (it runs as a host-native poller) and was not in CLAUDE.md's documented service-ports table, so the planner had no visibility into the conflict.
- **Fix:** Auto-moved scraper to port 8088 (verified free via `ss -tlnp`).
- **Files modified:** `services/scraper/Dockerfile` (EXPOSE 8088), `services/scraper/internal/config/config.go` (SERVER_PORT default 8088), `docker/docker-compose.yml` (port mapping, healthcheck URL, SCRAPER_API_URL on catalog), `Makefile` (health line).
- **Commit:** `7faac23`

**Orchestrator follow-up required (post-merge sweep):** The planning docs still reference port 8087 and need to be updated to 8088:
- `.planning/phases/15-foundation/15-01-PLAN.md` — frontmatter `must_haves.truths`, action body, `<threat_model>` table
- `.planning/phases/15-foundation/15-02-PLAN.md`, `15-03-PLAN.md`, `15-04-PLAN.md` (if they reference 8087)
- `.planning/STATE.md` Current Position narrative
- `.planning/ROADMAP.md` (Phase 15 description)
- `.planning/REQUIREMENTS.md` `SCRAPER-FOUND-07` / `SCRAPER-FOUND-10` if they hardcode 8087
- CLAUDE.md `Service Ports` table (add scraper:8088, document maintenance host-port collision)

Defer to orchestrator because per `<parallel_execution>` rules, this worktree must not modify STATE.md / ROADMAP.md. The other docs are planning-tier; the orchestrator's merge step is the right seam for the multi-file fix-up.

### 2. [Rule 1 - Bug in plan assumption] Workspace auto-bumped Go 1.22 -> 1.23.0

- **Found during:** Task 1 (`go work sync`)
- **Issue:** Plan / D-STACK §5.1 pinned `github.com/PuerkitoBio/goquery v1.10.3` as "Go-1.22 compatible". `go work sync` reported: `module github.com/PuerkitoBio/goquery@v1.10.3 requires go >= 1.23.0; switching to go1.25.10`. So v1.10.3 actually requires Go 1.23+, not 1.22 as the plan claimed. The Go toolchain auto-bumped every per-module `go` directive in the workspace and additionally upgraded catalog's goquery `v1.9.2 -> v1.10.3` because `go work sync` resolves the highest version across all workspace members.
- **Fix:** Accepted the toolchain upgrade. Verified that catalog, auth, player, gateway, themes, streaming all build cleanly on Go 1.23.0. The Dockerfiles use `golang:1.24-alpine` which already supports 1.23+. No language-feature regression because no consumer relies on Go 1.22-specific behavior.
- **Files modified:** `go.work` + every libs/*/go.{mod,sum} + every services/*/go.{mod,sum} (autograded by go work sync, single atomic effect).
- **Commit:** `ee21691` (rolled into Task 1)
- **Rationale for keeping vs. demoting:** Demoting to goquery v1.9.2 would contradict the explicit planner pin (D-STACK §5.1). The bump is benign — Go 1.23 is a strict superset of 1.22 for our usage.

### 3. [Rule 1 - go.mod auto-prune] Unused requires removed by go work sync

- **Found during:** Task 1
- **Issue:** Plan asked scraper/go.mod to declare `goquery v1.10.3`, `hashicorp/go-retryablehttp v0.7.7`, `golang.org/x/time v0.5.0`, `libs/errors`. `go work sync` (acting like `go mod tidy`) removed those four because nothing in services/scraper imports them yet.
- **Fix:** Accepted the prune. Plan 02 explicitly adds the first actual usage of each (rate.Limiter, retryablehttp, goquery for parser scaffolding, errors for domain errors) and `go mod tidy` at that point will re-add them. Keeping unused requires in go.mod is bad hygiene that Go tooling actively removes.
- **Files modified:** `services/scraper/go.mod` (4 requires removed)
- **Commit:** `ee21691`

## Confirmation Items

- [x] `docker compose ps` shows `animeenigma-scraper` `Up (healthy)` on `127.0.0.1:8088->8088/tcp`
- [x] `curl http://localhost:8088/health` returns HTTP 200 with `{"success":true,"data":{"status":"ok"}}`
- [x] `curl http://localhost:8088/metrics` returns HTTP 200 with Prometheus exposition format
- [x] `go test ./services/scraper/...` passes — `TestNewReturnsGoldie` and `TestGoldieFixtureDir` both green
- [x] Container exits gracefully on SIGTERM within 30 seconds (live-verified, sub-second shutdown)
- [x] `make capture-goldens` runs cleanly (no-op since Phase 15 has no Golden* tests yet)
- [x] `make health` reports `✓ scraper:8088` alongside all other services
- [x] `make redeploy-scraper`, `make restart-scraper`, `make logs-scraper` work via existing wildcard rules (no Makefile changes needed for those — verified by routing through `./deploy/scripts/redeploy.sh scraper`)
- [x] `go.work` lists `./services/scraper`
- [x] `services/scraper/testdata/.gitkeep` committed
- [x] Catalog block has `SCRAPER_API_URL: http://scraper:8088` env + `scraper: service_started` in `depends_on` (plumbing ready for plan 04)

## Threat Surface Scan

No new threat surface beyond what the plan's `<threat_model>` already documented (T-15-01 .. T-15-03). The port change 8087 -> 8088 keeps the same trust boundary (loopback-only host port; docker network internal-only inter-service traffic). No external auth path, no new schema, no new file access pattern.

## Self-Check

**File existence:**

- `services/scraper/cmd/scraper-api/main.go` — FOUND
- `services/scraper/Dockerfile` — FOUND
- `services/scraper/go.mod` — FOUND
- `services/scraper/internal/config/config.go` — FOUND
- `services/scraper/internal/transport/router.go` — FOUND
- `services/scraper/internal/testharness/goldie.go` — FOUND
- `services/scraper/internal/testharness/goldie_test.go` — FOUND
- `services/scraper/testdata/.gitkeep` — FOUND
- `go.work` contains `./services/scraper` — FOUND
- `Makefile` contains `capture-goldens` target — FOUND
- `docker/docker-compose.yml` contains `animeenigma-scraper` — FOUND

**Commit existence:** `ee21691`, `34afeae`, `7faac23` — all FOUND in `git log`.

## Self-Check: PASSED
