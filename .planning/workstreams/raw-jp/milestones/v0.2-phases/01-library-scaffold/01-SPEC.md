---
id: LIB-library-scaffold
title: Library service scaffold + docker-compose + gateway routing + DB bootstrap
workstream: raw-jp
milestone: v0.2
phase: 01
created_at: 2026-05-18
status: SPEC-ready
ambiguity_score: 0.15
mode: --auto
---

# Phase 01 (workstream `raw-jp`, milestone v0.2): Library Service Scaffold — Specification

**Workstream:** `raw-jp`
**Milestone:** v0.2 Self-Hosted Library
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Requirements:** LIB-01, LIB-02, LIB-NF-04
**Mode:** `--auto`

## Goal

Stand up a new Go microservice at `services/library/` on port 8087 with the standard project layout, wire it into Docker + the gateway router, and bootstrap a dedicated Postgres database `library`. The service responds 200 on `/health` and exposes Prometheus `/metrics`. No domain logic yet — this is pure scaffolding.

## Background

**Today, three things are true and need to change:**

1. **No raw-library service exists.** All raw-JP playback flows through the catalog's `services/catalog/internal/service/raw_resolver.go` (v0.1) which talks to AllAnime directly. There's no place for self-hosted episodes to live.

2. **Adding a new microservice has an established pattern in this codebase.** Recent additions like `services/themes/`, `services/scheduler/`, `services/scraper/`, and `services/rooms/` all follow the same shape: `cmd/{name}-api/main.go` entry, `internal/{config,domain,handler,repo,service,transport}`, `migrations/`, multi-module `go.mod` joined to the root `go.work`, `Dockerfile`, and a `docker-compose.yml` service block + Makefile targets. The scaffold phase replicates this template.

3. **The gateway is the single entry point for `/api/*` routes.** New services need a proxy route added to `services/gateway/internal/router/routes.go` (or equivalent) that forwards `/api/library/*` to `library:8087`.

**The implementation:**
- Skeleton service that compiles, starts, responds to `/health`, exposes `/metrics`, and connects to the new `library` Postgres database (auto-created via `libs/database.New`).
- One docker-compose service block + two persistent volumes (`library_torrents`, `library_minio_staging`).
- Gateway proxy route registered.
- Makefile targets for redeploy / logs / restart.

## Requirements

### LIB-01: services/library/ scaffold

- **Current:** No `services/library/` directory.
- **Target:**
  - `services/library/cmd/library-api/main.go` — entry point mirroring `services/catalog/cmd/catalog-api/main.go` minus the catalog-specific clients.
  - `services/library/internal/config/config.go` — env loader with placeholder LIBRARY_* knobs.
  - `services/library/internal/domain/` — empty package (filled in Phase 3).
  - `services/library/internal/handler/health.go` — `GET /health` returns `{"status":"ok"}`.
  - `services/library/internal/repo/` — empty package (filled in Phase 3).
  - `services/library/internal/service/` — empty package.
  - `services/library/internal/transport/router.go` — chi router with health + metrics endpoints + the standard middleware chain (RequestID, metrics.Middleware, RequestLogger, Recoverer, CORS).
  - `services/library/migrations/` — empty directory placeholder.
  - `services/library/go.mod` — module declaration joined to `go.work`.
  - `services/library/Dockerfile` — multi-stage build mirroring `services/catalog/Dockerfile`.
- **Acceptance:** `cd services/library && go build ./...` succeeds. `make redeploy-library` builds the container. `curl http://localhost:8087/health` returns 200. `curl http://localhost:8087/metrics` returns Prometheus exposition.

### LIB-02: docker-compose + gateway routing + Makefile

- **Current:** No `library` service block in `docker-compose.yml`. No `/api/library/*` route in gateway. No Makefile targets for library.
- **Target:**
  - `docker/docker-compose.yml` — new `library` service block (build context = root, dockerfile = `services/library/Dockerfile`), `ports: ["8087:8087"]`, `env_file: [./.env]`, `volumes: [library_torrents:/data/torrents, library_minio_staging:/tmp/encode]`, `depends_on: [postgres]`, deploy resource limits `cpus: '4.0', memory: '4G'`. Two new named volumes at the bottom: `library_torrents` and `library_minio_staging`.
  - `services/gateway/internal/router/routes.go` (or the project's equivalent) — new reverse-proxy route block for `/api/library/*` → `library:8087`. Mirror the existing `/api/themes/*` proxy.
  - `Makefile` — three new targets: `redeploy-library` (build + restart), `restart-library` (no rebuild), `logs-library` (follow logs). Add `library:8087` to the `health` target's check list.
- **Acceptance:** `make redeploy-library` builds and starts the container. `make health` includes `library:8087 - healthy`. `curl http://localhost:8000/api/library/health` returns the same 200 payload as the direct `localhost:8087` probe (gateway forwards correctly).

### LIB-NF-04: Operator documentation

- **Current:** `CLAUDE.md` "Service Ports" + "Gateway Routing" sections don't mention library.
- **Target:**
  - `CLAUDE.md` — extend the "Service Ports" table with `library | 8087 | /metrics | Library service (BitTorrent → HLS → MinIO, admin-only)`.
  - `CLAUDE.md` — extend the "Gateway Routing" list with `/api/library/* → library:8087`.
  - `docker/.env.example` — new `# workstream raw-jp / v0.2 — library service` block documenting `LIBRARY_DB_HOST`, `LIBRARY_DB_PORT`, `LIBRARY_DB_USER`, `LIBRARY_DB_PASSWORD`, `LIBRARY_DB_NAME` (default `library`). Phase 3 will add the torrent + encoder envs; this phase only documents the DB + scaffold knobs.
- **Acceptance:** `grep -E "library:8087" CLAUDE.md` finds two hits (Service Ports + Gateway Routing). Operator can read `docker/.env.example` to find every env var with a default that doesn't require changing for a default deploy.

## Acceptance Criteria

1. `services/library/cmd/library-api/main.go` exists; `go build ./...` from `services/library/` succeeds.
2. `docker/docker-compose.yml` has a `library` service block with the documented ports + volumes + depends_on + resource limits.
3. `docker compose -f docker/docker-compose.yml up -d library` (or `make redeploy-library`) starts the container and it stays running.
4. `curl -s http://localhost:8087/health | jq -r .data.status` → `ok`.
5. `curl -s http://localhost:8000/api/library/health | jq -r .data.status` → `ok` (gateway proxy).
6. `curl -s http://localhost:8087/metrics | grep "^http_requests_total" | wc -l` → `> 0` after the first request.
7. Postgres `library` database exists (`docker compose exec postgres psql -U postgres -l | grep library`).
8. `make health` output includes `✓ library:8087`.
9. `CLAUDE.md` and `docker/.env.example` updated per LIB-NF-04.

## Auto-selected implementation decisions

- **Skeleton template source:** Mirror `services/themes/` shape — smallest existing service, cleanest reference for what the bare minimum looks like.
- **DB schema migration tool:** GORM `AutoMigrate` via `libs/database.New()` (consistent with the rest of the project; no separate migration runner).
- **Logging:** `libs/logger` Default logger with `service_name: "library"`.
- **Metrics:** `libs/metrics.NewCollector("library")` in `transport.NewRouter`.
- **Gateway routing implementation:** Add the `/api/library/*` route block in the same file as `/api/themes/*`. Use the existing reverse-proxy helper (do not roll a new one).
- **Volume names:** `library_torrents` (downloaded torrent data, transient between container restarts; not bind-mounted to host by default), `library_minio_staging` (ffmpeg encode temp dir, also transient).
- **Resource limits:** 4 CPU / 4 GiB RAM — matches the design doc's example.

## Touches

- **New:** `services/library/cmd/library-api/main.go`
- **New:** `services/library/internal/config/config.go`
- **New:** `services/library/internal/handler/health.go`
- **New:** `services/library/internal/transport/router.go`
- **New:** `services/library/Dockerfile`
- **New:** `services/library/go.mod`, `services/library/go.sum`
- **New:** `services/library/migrations/.gitkeep` (placeholder)
- **Extend:** `go.work` (add `./services/library`)
- **Extend:** `docker/docker-compose.yml`
- **Extend:** `services/gateway/internal/router/routes.go` (or project equivalent)
- **Extend:** `Makefile`
- **Extend:** `CLAUDE.md` (Service Ports + Gateway Routing tables)
- **Extend:** `docker/.env.example`

## Out of Scope (for this phase)

- Any business logic (Phases 2-6 add it).
- Real DB migrations (Phase 3 / 4 add `library_jobs`, `library_episodes`, `library_filename_patterns`).
- Torrent client / ffmpeg / MinIO wiring.
- Admin UI.
- Hybrid resolver.

## Citations to design doc

- Architecture → "services/library/ (NEW)" service block including the sub-package tree.
- Architecture → docker-compose service block snippet for the library service.
- Tech-choices → "Library port: 8087 — Next free port".
- Tech-choices → "Library service language: Go, separate service".
