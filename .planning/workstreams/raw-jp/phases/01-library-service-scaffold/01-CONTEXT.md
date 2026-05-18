# Phase 1: Library Service Scaffold - Context

**Gathered:** 2026-05-18
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase; full SPEC pre-written)

<domain>
## Phase Boundary

Stand up a new Go microservice at `services/library/` on port 8087 with the standard project layout (`cmd/library-api/main.go`, `internal/{config,domain,handler,repo,service,transport}`, `migrations/`). Wire it into `docker-compose.yml` and the gateway routing (`/api/library/*` → `library:8087`). Bootstrap a dedicated Postgres DB (`library`) via the shared `libs/database` helper. Service responds 200 on `/health` and exposes `/metrics`.

**Out of scope:** Any domain logic, real migrations, torrent client, ffmpeg, MinIO, admin UI, or hybrid resolver. These are Phases 2–6.

</domain>

<decisions>
## Implementation Decisions

### Locked from SPEC (`milestones/v0.2-phases/01-library-scaffold/01-SPEC.md`)

- **Skeleton template source:** Mirror `services/themes/` shape — smallest, cleanest reference service.
- **DB schema migration tool:** GORM `AutoMigrate` via `libs/database.New()` (consistent with the rest of the project).
- **Logging:** `libs/logger` Default logger with `service_name: "library"`.
- **Metrics:** `libs/metrics.NewCollector("library")` in `transport.NewRouter`.
- **Gateway routing:** Add `/api/library/*` block in the same file as `/api/themes/*`. Reuse the existing reverse-proxy helper.
- **Volume names:** `library_torrents` and `library_minio_staging` (both transient by default).
- **Resource limits:** 4 CPU / 4 GiB RAM (matches design doc example).

### Claude's Discretion (autonomous mode)

All remaining implementation choices are at Claude's discretion — pure infrastructure phase. The SPEC's acceptance criteria are concrete (file exists, build succeeds, health endpoint returns 200, Postgres DB exists, gateway proxy works).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `libs/database` — shared Postgres connection helper with auto-DB-create on first run.
- `libs/logger` — structured Zap-based logger with service_name field.
- `libs/metrics` — Prometheus collector with HTTP middleware (`metrics.Middleware`).
- `libs/errors` — domain error wrapping.
- Reference service: `services/themes/` — minimal shape (chi router, health, metrics, DB).

### Established Patterns

- Multi-module Go workspace: each service has its own `go.mod`, joined via `go.work`.
- Dockerfile: multi-stage build (deps → build → runtime), root build context.
- docker-compose: env_file: `./.env`, named volumes, depends_on Postgres.
- Gateway routing: reverse-proxy block per backend, lives in `services/gateway/internal/router/routes.go` (or equivalent).
- Service ports table in `CLAUDE.md` is the canonical list — extend it for every new service.

### Integration Points

- `go.work` — add `./services/library`.
- `docker/docker-compose.yml` — new `library` service block + two named volumes.
- `services/gateway/internal/router/routes.go` — new `/api/library/*` proxy.
- `Makefile` — `redeploy-library`, `restart-library`, `logs-library`; `health` target check.
- `CLAUDE.md` — Service Ports + Gateway Routing tables.
- `docker/.env.example` — `LIBRARY_DB_*` env block.

</code_context>

<specifics>
## Specific Ideas

SPEC reference at `milestones/v0.2-phases/01-library-scaffold/01-SPEC.md` is authoritative for acceptance criteria (LIB-01, LIB-02, LIB-NF-04). Touches list and out-of-scope items in the SPEC apply.

</specifics>

<deferred>
## Deferred Ideas

None — phase scope is pre-defined and tightly scoped by the SPEC.

</deferred>
