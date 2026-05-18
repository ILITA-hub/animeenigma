---
phase: 01-library-service-scaffold
status: complete
workstream: raw-jp
milestone: v0.2
date: 2026-05-18
requirements:
  - LIB-01
  - LIB-02
  - LIB-NF-04
commits:
  - 4c9427c — feat(01): scaffold services/library/ Go module + Dockerfile + go.work
  - da8efcf — feat(01): wire library service into compose + gateway + Makefile + env
  - 80861f1 — docs(01): document library:8089 in CLAUDE.md Service Ports + Gateway Routing
---

# Phase 01: Library Service Scaffold — Summary

Pure infrastructure / scaffolding phase. Stood up a new Go microservice
`services/library/` mirroring the `services/themes/` shape, wired it into
docker-compose + the gateway router, auto-created a dedicated Postgres DB
`library`, and updated operator-facing documentation. No domain logic — that
lands in Phases 2–6.

## What was built

- New `services/library/` Go module joined to `go.work`, compiles standalone.
- Chi router exposing `/health`, `/api/library/health`, and `/metrics` (Prometheus).
- DB connection via `libs/database.New()` auto-creates Postgres `library`.
- New `library` docker-compose service block with two named volumes
  (`library_torrents`, `library_minio_staging`) and 4 CPU / 4 GiB resource limits.
- Gateway routing: `LIBRARY_SERVICE_URL` env + `ProxyToLibrary` handler +
  `service.proxy.go` switch case + `/api/library/*` route block with health +
  wildcard passthrough (forward-compat for Phases 2–5).
- Makefile `SERVICES` includes `library`; `health` target probes 8089.
- `docker/.env.example` documents `LIBRARY_DB_NAME` (other LIBRARY_DB_*
  values inherit shared Postgres credentials).
- `CLAUDE.md` Service Ports table + Gateway Routing list updated.

## Files touched

### NEW

- `services/library/go.mod`, `services/library/go.sum`
- `services/library/cmd/library-api/main.go`
- `services/library/internal/config/config.go`
- `services/library/internal/handler/health.go`
- `services/library/internal/transport/router.go`
- `services/library/internal/domain/.gitkeep`
- `services/library/internal/repo/.gitkeep`
- `services/library/internal/service/.gitkeep`
- `services/library/migrations/.gitkeep`
- `services/library/Dockerfile`
- `.planning/workstreams/raw-jp/phases/01-library-service-scaffold/01-SUMMARY.md` (this file)

### EXTEND

- `go.work` — added `./services/library`
- `docker/docker-compose.yml` — new `library` service block (port 8089),
  two new named volumes, gateway env + depends_on extended
- `docker/.env.example` — new LIBRARY_DB_NAME block
- `services/gateway/internal/config/config.go` — `LibraryService` field +
  `getEnv("LIBRARY_SERVICE_URL", "http://library:8089")` default
- `services/gateway/internal/handler/proxy.go` — `ProxyToLibrary` method
- `services/gateway/internal/service/proxy.go` — `"library"` → `LibraryService`
  case added to `getServiceURL` switch
- `services/gateway/internal/transport/router.go` — `r.Route("/library", ...)`
  block under `/api` with `/health` + wildcard `/*`
- `Makefile` — `SERVICES` list gains `library`; `health` target adds
  `✓ library:8089` check
- `CLAUDE.md` — Service Ports table row + Gateway Routing bullet
- `services/{auth,catalog,gateway,player,rooms,scheduler,scraper,streaming,themes}/Dockerfile` —
  each gained one `COPY services/library/go.mod services/library/go.sum*
  ./services/library/` line because `go.work` requires every workspace
  member's go.mod at build-context time

## Verification results

All eight commands from the plan's `<verification_after>` block pass.

### `cd services/library && go build ./...`
Exit 0, no output. Clean build.

### `make redeploy-library`
```
Container animeenigma-library Started
[INFO] library is running
[INFO] Deployment complete!
```
Container builds, starts, stays running:
```
animeenigma-library Up About a minute 127.0.0.1:8089->8089/tcp
```

### `curl http://localhost:8089/health` (direct)
```
HTTP 200
{"success":true,"data":{"status":"ok"}}
```

### `curl http://localhost:8000/api/library/health` (gateway proxy)
```
HTTP 200
{"success":true,"data":{"status":"ok"}}
```

### `curl http://localhost:8089/metrics`
Returns Prometheus exposition. After a few health probes:
```
# HELP db_pool_idle_connections Number of idle database connections
# TYPE db_pool_idle_connections gauge
db_pool_idle_connections 1
# HELP http_requests_total ...
http_requests_total{method="GET",path="/api/library/health",service="library",status="200"} 2
http_requests_total{method="GET",path="/health",service="library",status="200"} 1
```
Both `service="library"` label and `http_requests_total` counter present.

### Postgres `library` database
```
$ docker compose exec postgres psql -U postgres -l | grep library
 library     | postgres | UTF8     | libc            | en_US.utf8 | en_US.utf8 |            |           |
```
Auto-created on first connect via `libs/database.New()`. No manual migration.

### `make health`
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
All existing services still healthy — no regression.

### CLAUDE.md library entries
```
| library    | 8089 | /metrics  | Library service (BitTorrent → HLS → MinIO, admin-only) |
- `/api/library/*` → library:8089 (admin-only; routes added incrementally in v0.2 Phases 2–5)
```
Both rows present (Service Ports table + Gateway Routing list).

## Deviations from plan

### 1. **[Rule 4 → reasonable call] Library binds port 8089, not 8087**

**Surfaced at:** Task 2 (`make redeploy-library` first attempt).

**Issue:** Port 8087 is already bound by the host-side `maintenance` bot
binary (`/data/animeenigma/bin/maintenance`, pid 52153, listening on `*:8087`
since 2026-04-17). The SPEC + design doc + plan all locked 8087 as "Library
port — next free port", but that assumption missed the maintenance daemon,
which is a host-native (non-docker) process. `docker-compose up library`
failed with `address already in use` on port 8087.

The maintenance daemon is referenced by other services as
`MAINTENANCE_URL=http://host-gateway:8087` (player + scheduler) — it is a
working production component, not stale infrastructure.

**Fix:** Reassigned the library service to port **8089** (next free port
after `scraper:8088`). Touched all references:
- `services/library/internal/config/config.go` — `SERVER_PORT` default
- `services/library/Dockerfile` — `EXPOSE 8089`
- `docker/docker-compose.yml` — `SERVER_PORT`, `ports`, gateway
  `LIBRARY_SERVICE_URL`
- `services/gateway/internal/config/config.go` — getEnv default
- `Makefile` — `health` target probes 8089
- `CLAUDE.md` — both tables show 8089
- A note in the `library:` compose block documents why 8089, not 8087

**Why this was the smallest blast radius:** moving the maintenance daemon
off port 8087 would have required touching `services/maintenance/`, the
`MAINTENANCE_URL` env in two other services, and an unrelated workstream's
external assumptions. Library has no incoming dependencies yet (Phase 1
scaffold), so re-numbering it is a one-shot change.

**Follow-up:** Future workstream-raw-jp docs (design doc + REQUIREMENTS)
should be cross-referenced and patched to read 8089 — those references are
out of scope for this phase but listed in **Open items** below.

### 2. **[Note — SPEC `jq -r .data.status` is correct, plan instructions were wrong]**

The plan's `<interfaces>` block claimed `httputil.OK(w, ...)` writes a flat
JSON body and that the SPEC's `jq -r .data.status` was incorrect. **The
plan was wrong.** `libs/httputil/response.go:36-48` wraps responses in a
`{success, data, error?, meta?}` envelope. The actual response body for
the library health endpoint is `{"success":true,"data":{"status":"ok"}}`.
So `jq -r .data.status` correctly returns `ok`.

No code change needed — the plan's `grep -q '"status":"ok"'` substring
check happens to match either shape. Documented here so Phase 2+ doesn't
inherit the plan's misreading.

### 3. **[Forward-compat addition] Library router exposes BOTH `/health` AND `/api/library/health`**

**Reason:** The gateway forwards `r.URL.Path` verbatim (no prefix
stripping in `service.proxy.go` for `library`). The themes service does
the same — it registers `r.Route("/api/themes", ...)` so it can serve
`/api/themes/*` paths sent by the gateway. The plan only specified
`/health` for the library, but that means a gateway-proxied
`/api/library/health` would 404 at the library service. Mounted health
on both paths so:
- `/health` covers direct probes (`make health`, docker healthcheck)
- `/api/library/health` covers gateway proxy without path rewriting

No new dependencies, no scope creep. Same pattern Phase 2 will follow for
search/job endpoints.

## Out of scope (per SPEC)

Carried over verbatim — these belong to later v0.2 phases:

- Any business logic (Phases 2–6 add it).
- Real DB migrations (Phase 3/4 add `library_jobs`, `library_episodes`,
  `library_filename_patterns` SQL).
- Torrent client wrapper (`anacrolix/torrent`) — Phase 3.
- ffmpeg subprocess wrapper — Phase 4.
- MinIO writer + bucket bootstrap — Phase 4.
- Admin UI (`RawLibrary.vue`) — Phase 5.
- Hybrid resolver in catalog — Phase 6.
- `library_jobs` Postgres-backed queue with `FOR UPDATE SKIP LOCKED` — Phase 3.
- Disk-free guard, job stall detection, library-specific Prometheus metrics
  — Phase 3 (LIB-NF-01/02/03).
- Grafana dashboard `infra/grafana/dashboards/library.json` — Phase 3.
- Operator runbook entry under `docs/issues/README.md` (`ISS-013`) — Phase 3 or 6.

## Open items

1. **Update upstream workstream docs to reflect port 8089:**
   - `docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md` —
     "Library port: 8087 — Next free port" should now say 8089.
   - `.planning/workstreams/raw-jp/milestones/v0.2-REQUIREMENTS.md` — any
     8087 references.
   - `.planning/workstreams/raw-jp/milestones/v0.2-phases/01-library-scaffold/01-SPEC.md`
     — Acceptance Criteria #4 cites port 8087. (Leave the SPEC as historical
     record but cross-link this SUMMARY.)

2. **Phase 2 first task** should reuse the empty `r.Route("/api/library", ...)`
   subgroup in `services/library/internal/transport/router.go`. The jwtConfig
   parameter is already plumbed in but unused (`_ = jwtConfig`) — Phase 3
   wires it to real `AuthMiddleware + AdminRoleMiddleware` groups.

3. **Dockerfile cross-copy proliferation** — every existing service
   Dockerfile gained one `COPY services/library/go.mod` line. If we add
   another service in the future, the same pattern repeats. Consider
   factoring this into a shared base image or a `go-mods` build target
   so we don't have to touch nine files for each new service. Out of
   scope for raw-jp; tracked as general engineering hygiene.

4. **Maintenance daemon port** — `host-side bin/maintenance` on `:8087`
   is not currently documented in CLAUDE.md's Service Ports table. Worth
   adding a row in a future docs pass so the next person scanning for free
   ports doesn't make the same mistake.

## Pointer to next phase

Phase 02 spec: `.planning/workstreams/raw-jp/milestones/v0.2-phases/02-nyaa-animetosho-clients/02-SPEC.md`
(adds Nyaa + AnimeTosho search clients to `services/library/internal/parser/`,
reuses the empty `r.Route("/api/library", ...)` scaffold from this phase).

## Self-Check: PASSED

- `services/library/cmd/library-api/main.go` — FOUND
- `services/library/internal/config/config.go` — FOUND
- `services/library/internal/handler/health.go` — FOUND
- `services/library/internal/transport/router.go` — FOUND
- `services/library/Dockerfile` — FOUND
- `services/library/go.mod`, `go.sum` — FOUND
- `services/library/migrations/.gitkeep` — FOUND
- Commit `4c9427c` (scaffold) — FOUND in git log
- Commit `da8efcf` (integration) — FOUND in git log
- Commit `80861f1` (CLAUDE.md docs) — FOUND in git log
- All eight `<verification_after>` commands pass with expected output (see above)
