---
phase: 01-library-service-scaffold
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - go.work
  - services/library/cmd/library-api/main.go
  - services/library/internal/config/config.go
  - services/library/internal/handler/health.go
  - services/library/internal/transport/router.go
  - services/library/internal/domain/.gitkeep
  - services/library/internal/repo/.gitkeep
  - services/library/internal/service/.gitkeep
  - services/library/migrations/.gitkeep
  - services/library/Dockerfile
  - services/library/go.mod
  - services/library/go.sum
  - docker/docker-compose.yml
  - docker/.env.example
  - services/gateway/internal/config/config.go
  - services/gateway/internal/handler/proxy.go
  - services/gateway/internal/transport/router.go
  - Makefile
  - CLAUDE.md
autonomous: true
workstream: raw-jp
milestone: v0.2
requirements:
  - LIB-01
  - LIB-02
  - LIB-NF-04

must_haves:
  truths:
    - "services/library/ compiles standalone (`cd services/library && go build ./...` succeeds)"
    - "Container starts and stays running under `docker compose up -d library`"
    - "`curl http://localhost:8087/health` returns 200 `{\"status\":\"ok\"}`"
    - "`curl http://localhost:8000/api/library/health` returns the same payload via gateway proxy"
    - "`curl http://localhost:8087/metrics` returns Prometheus exposition with `http_requests_total{service=\"library\"}` after first request"
    - "Postgres `library` database is auto-created on first start"
    - "`make health` includes `library:8087` in its check list"
    - "Operator reading CLAUDE.md sees library:8087 in Service Ports + Gateway Routing tables"
  artifacts:
    - path: "services/library/cmd/library-api/main.go"
      provides: "Service entry point mirroring services/themes/cmd/themes-api/main.go"
    - path: "services/library/internal/transport/router.go"
      provides: "chi router with /health, /metrics, standard middleware chain (RequestID, metrics.Middleware, RequestLogger, Recoverer, CORS, RealIP)"
    - path: "services/library/internal/config/config.go"
      provides: "Env loader for LIBRARY_DB_* + SERVER_PORT (default 8087) + JWT"
    - path: "services/library/internal/handler/health.go"
      provides: "GET /health returns {\"status\":\"ok\"} via httputil.OK"
    - path: "services/library/Dockerfile"
      provides: "Multi-stage build mirroring services/themes/Dockerfile; EXPOSE 8087"
    - path: "services/library/go.mod"
      provides: "Module declaration joined to go.work with replace directives for libs/{authz,database,errors,httputil,logger,metrics}"
    - path: "services/library/migrations/.gitkeep"
      provides: "Placeholder for Phase 3/4 SQL migrations"
    - path: "docker/docker-compose.yml"
      provides: "`library` service block on port 8087 + named volumes library_torrents, library_minio_staging; gateway env extended with LIBRARY_SERVICE_URL"
    - path: "services/gateway/internal/transport/router.go"
      provides: "/api/library/* reverse-proxy block mirroring /api/themes/*"
    - path: "Makefile"
      provides: "library:8087 line in `health` target (redeploy-/restart-/logs- already provided by generic %-pattern targets)"
    - path: "CLAUDE.md"
      provides: "library:8087 row in Service Ports + Gateway Routing tables"
    - path: "docker/.env.example"
      provides: "LIBRARY_DB_* env block (LIBRARY_DB_HOST/PORT/USER/PASSWORD/NAME)"
  key_links:
    - from: "services/library/cmd/library-api/main.go"
      to: "libs/database.New()"
      via: "cfg.Database -> auto-creates `library` DB on first run"
      pattern: "database\\.New\\(cfg\\.Database\\)"
    - from: "services/library/internal/transport/router.go"
      to: "libs/metrics.NewCollector(\"library\")"
      via: "metricsCollector.Middleware + /metrics handler"
      pattern: "metrics\\.NewCollector\\(\"library\"\\)"
    - from: "docker/docker-compose.yml gateway block"
      to: "library:8087"
      via: "LIBRARY_SERVICE_URL env + depends_on: [library]"
      pattern: "LIBRARY_SERVICE_URL"
    - from: "services/gateway/internal/transport/router.go"
      to: "services/library:8087 /health endpoint"
      via: "proxyHandler.ProxyToLibrary forwarder + new ProxyToLibrary method in services/gateway/internal/handler/proxy.go"
      pattern: "/api/library"
---

<objective>
Stand up a new Go microservice at `services/library/` on port **8087**, wired into Docker Compose and the gateway router, with a dedicated Postgres database `library` auto-created via `libs/database.New()`. Service responds 200 on `/health` and exposes Prometheus `/metrics`. **Pure scaffolding** — no domain logic, no real migrations, no torrent/ffmpeg/MinIO wiring. All of those are Phases 2–6.

Purpose: Unblock all subsequent v0.2 work (search clients, torrent + job queue, ffmpeg transcoder, admin UI, hybrid resolver) by adding the standard project skeleton in one atomic commit set that an operator can `make redeploy-library` and verify with `make health`.

Output:
- New service tree under `services/library/` joined to the multi-module Go workspace.
- New `library` docker-compose service block + two named volumes (`library_torrents`, `library_minio_staging`).
- New `/api/library/*` proxy route in the gateway.
- `make health` augmented to include `library:8087`.
- `CLAUDE.md` Service Ports + Gateway Routing tables updated.
- `docker/.env.example` documents `LIBRARY_DB_*`.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@.planning/workstreams/raw-jp/milestones/v0.2-ROADMAP.md
@.planning/workstreams/raw-jp/milestones/v0.2-REQUIREMENTS.md
@.planning/workstreams/raw-jp/milestones/v0.2-phases/01-library-scaffold/01-SPEC.md
@.planning/workstreams/raw-jp/phases/01-library-service-scaffold/01-CONTEXT.md

# Template service (mirror its shape exactly):
@services/themes/cmd/themes-api/main.go
@services/themes/internal/config/config.go
@services/themes/internal/transport/router.go
@services/themes/Dockerfile
@services/themes/go.mod

# Files to extend:
@go.work
@docker/docker-compose.yml
@docker/.env.example
@Makefile
@services/gateway/internal/config/config.go
@services/gateway/internal/handler/proxy.go
@services/gateway/internal/transport/router.go

<interfaces>
<!-- Key contracts the executor needs. No codebase exploration required. -->

From `libs/httputil/response.go`:
```go
func OK(w http.ResponseWriter, data interface{})  // writes 200 + raw JSON of `data` (NO `data:` envelope wrap)
```
Implication: `r.Get("/health", ...)` calling `httputil.OK(w, map[string]string{"status": "ok"})` returns the literal body `{"status":"ok"}`. The SPEC's `jq -r .data.status` is incorrect for the chi/httputil convention used elsewhere in this codebase; the correct assertion is `jq -r .status`. The themes service uses the same pattern (see `services/themes/internal/transport/router.go:35-37`).

From `libs/database`:
```go
type Config struct { Host, Port, User, Password, Database, SSLMode string }
func New(cfg Config) (*DB, error)  // auto-creates the DB if it does not exist
```

From `libs/metrics`:
```go
func NewCollector(serviceName string) *Collector
func (c *Collector) Middleware(next http.Handler) http.Handler
func Handler() http.Handler  // returns the Prometheus /metrics handler
func StartDBPoolCollector(*sql.DB, time.Duration)
```

From `libs/logger`:
```go
func Default() *Logger  // structured Zap logger; service_name is set via the SERVICE_NAME env var (see `services/themes/cmd/themes-api/main.go` for example)
```

From the gateway (existing patterns to mirror, see `services/gateway/internal/handler/proxy.go:57-60`):
```go
func (h *ProxyHandler) ProxyToThemes(w http.ResponseWriter, r *http.Request) {
    h.proxy(w, r, "themes")
}
```
The `h.proxy(w, r, "themes")` call dispatches via `service.ProxyService.Forward(r, "themes")`, which resolves the upstream URL from `cfg.Services.ThemesService` set in `services/gateway/internal/config/config.go:87`. To add `library`, mirror **three** integration points:
1. `services/gateway/internal/config/config.go` — add `LibraryService string` to `ServiceURLs` + `getEnv("LIBRARY_SERVICE_URL", "http://library:8087")` in `Load()`.
2. `services/gateway/internal/handler/proxy.go` — add `ProxyToLibrary` after `ProxyToThemes` calling `h.proxy(w, r, "library")`.
3. `services/gateway/internal/service/proxy.go` — extend the service URL lookup map / switch to recognize `"library"` and route to `cfg.Services.LibraryService`. (Use the same pattern as the existing `"themes"` case.)

Gateway route block to mirror (`services/gateway/internal/transport/router.go:287-310`):
```go
r.Route("/themes", func(r chi.Router) {
    r.Get("/", proxyHandler.ProxyToThemes)
    r.Get("/{id}", proxyHandler.ProxyToThemes)
    // ...
})
```
For library (Phase 1 scope — health only; Phases 2-5 add the rest):
```go
r.Route("/library", func(r chi.Router) {
    // Public health-check passthrough (admin endpoints are added in later phases)
    r.Get("/health", proxyHandler.ProxyToLibrary)
    // Catch-all wildcard so Phases 2-5 don't need to keep touching this router.
    r.HandleFunc("/*", proxyHandler.ProxyToLibrary)
})
```

Existing themes-service docker-compose block (`docker/docker-compose.yml:541-560`) to mirror:
```yaml
themes:
  build:
    context: ..
    dockerfile: services/themes/Dockerfile
  container_name: animeenigma-themes
  restart: unless-stopped
  environment:
    SERVER_PORT: 8086
    DB_HOST: postgres
    DB_PORT: 5432
    DB_USER: postgres
    DB_PASSWORD: postgres
    DB_NAME: animeenigma   # <-- library will override to `library`
    JWT_SECRET: ${JWT_SECRET:-dev-secret-change-in-production}
  ports:
    - "127.0.0.1:8086:8086"
  depends_on:
    postgres:
      condition: service_healthy
```

Existing `make health` target (`Makefile:436-445`) to extend with one more `curl -sf http://localhost:8087/health` line for library.

Existing generic redeploy/restart/logs targets (`Makefile:275-290`) — these already accept any `%` so `make redeploy-library`, `make restart-library`, `make logs-library` work out of the box once the compose block exists. **Do not add per-service targets**; only update the `SERVICES` variable at `Makefile:9` to include `library` so `make build` and `make clean` pick it up.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Scaffold services/library/ Go module + skeleton + Dockerfile + go.work</name>
  <files>
    services/library/cmd/library-api/main.go,
    services/library/internal/config/config.go,
    services/library/internal/handler/health.go,
    services/library/internal/transport/router.go,
    services/library/internal/domain/.gitkeep,
    services/library/internal/repo/.gitkeep,
    services/library/internal/service/.gitkeep,
    services/library/migrations/.gitkeep,
    services/library/Dockerfile,
    services/library/go.mod,
    services/library/go.sum,
    go.work
  </files>
  <action>
Mirror `services/themes/` shape exactly, stripping out theme-specific clients/handlers. Concretely:

1. **`services/library/go.mod`** — module path `github.com/ILITA-hub/animeenigma/services/library`, go 1.23.0. Required modules: `libs/authz`, `libs/database`, `libs/errors`, `libs/httputil`, `libs/logger`, `libs/metrics`, `github.com/go-chi/chi/v5 v5.0.12`, `gorm.io/gorm v1.30.0` (for the GORM AutoMigrate path used later — keep parity with themes). Add the matching `replace` block pointing to `../../libs/{authz,database,errors,httputil,logger,metrics}`. Run `cd services/library && go mod tidy` to materialize go.sum.

2. **`services/library/cmd/library-api/main.go`** — entry point mirroring `services/themes/cmd/themes-api/main.go` but with these deletions:
   - Remove the `parser/animethemes` import + client init.
   - Remove all repository/service/handler instantiations (Phase 3+ work).
   - The only handler created is the `transport.NewRouter(...)` call with the JWT config, logger, and `metrics.NewCollector("library")`.
   - Keep `database.New(cfg.Database)` + `metrics.StartDBPoolCollector` + graceful-shutdown block verbatim.
   - **Do not** call `db.AutoMigrate(...)` with any model — leave the AutoMigrate call out for Phase 1 (Phase 3 will reintroduce it for `library_jobs`). The library DB itself is still auto-created by `libs/database.New` on first connect.
   - Server timeouts: `ReadTimeout: 15s`, `WriteTimeout: 120s` (matches themes; future torrent endpoints will need the longer write timeout), `IdleTimeout: 60s`.
   - Log line: `log.Infow("starting library service", "address", cfg.Server.Address())`.

3. **`services/library/internal/config/config.go`** — exact copy of `services/themes/internal/config/config.go` with these changes:
   - `SERVER_PORT` default → `8087`.
   - `DB_NAME` default → `"library"` (per locked decision; SPEC Acceptance #7 requires a dedicated `library` DB).
   - **All other DB env vars (`DB_HOST`/`DB_PORT`/`DB_USER`/`DB_PASSWORD`/`DB_SSLMODE`) keep the same names** — the compose block sets them. The `LIBRARY_DB_*` knobs documented in `docker/.env.example` (Task 4) are operator-facing variables that compose translates to `DB_*` for the container. Do NOT rename the in-process env reads.

4. **`services/library/internal/handler/health.go`** — new file. Provide a minimal `HealthHandler` struct (no deps) with `func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) { httputil.OK(w, map[string]string{"status": "ok"}) }`. Mirrors the inline anonymous handler used by themes, but factored into a named handler so Phases 2-5 can extend `/health/extended` (per LIB-09 / SPEC) without touching the router. Constructor: `func NewHealthHandler() *HealthHandler { return &HealthHandler{} }`.

5. **`services/library/internal/transport/router.go`** — chi router with the standard middleware chain (`middleware.RequestID`, `metricsCollector.Middleware`, `httputil.RequestLogger(log)`, `httputil.Recoverer(log)`, `httputil.CORS([]string{"*"})`, `middleware.RealIP`) and these endpoints only:
   - `GET /health` → `healthHandler.Health`
   - `GET /metrics` → `metrics.Handler().ServeHTTP`
   - `r.Route("/api/library", ...)` block with **no children yet** — leave the empty Route so Phase 2 (LIB-03/04/04b) can append handlers without restructuring.
   - Constructor signature: `func NewRouter(healthHandler *handler.HealthHandler, jwtConfig authz.JWTConfig, log *logger.Logger, metricsCollector *metrics.Collector) http.Handler`. Keep the JWT config parameter even though no protected routes exist yet — Phase 3 jobs endpoints will need it.
   - **Do not** import the themes-only middleware helpers (`OptionalAuthMiddleware`, `AdminRoleMiddleware`); those are added later phases. Keep `AuthMiddleware` for forward use? **No** — leave them out entirely for Phase 1. Phase 3 reintroduces auth middleware when it adds protected job endpoints.

6. **Empty package placeholders** — create `.gitkeep` files in:
   - `services/library/internal/domain/.gitkeep`
   - `services/library/internal/repo/.gitkeep`
   - `services/library/internal/service/.gitkeep`
   - `services/library/migrations/.gitkeep`
   These exist so the SPEC "Touches" list is satisfied and Phase 3 has somewhere to add `domain/job.go` etc. without needing `mkdir -p`. Do NOT create `internal/parser/` — that comes in Phase 2 (search clients).

7. **`services/library/Dockerfile`** — copy `services/themes/Dockerfile` verbatim and change:
   - `services/themes` → `services/library` everywhere (COPY paths, `cd` paths, binary name `/library-api`).
   - `EXPOSE 8086` → `EXPOSE 8087`.
   - `CMD ["./themes-api"]` → `CMD ["./library-api"]`.
   - **Important:** also add a `COPY services/library/go.mod services/library/go.sum* ./services/library/` line in the deps stage, AND add `services/library/go.mod` `services/library/go.sum*` to the **existing themes Dockerfile** and every other service's Dockerfile? **No** — only the library Dockerfile needs to copy library's go.mod. Other services already copy library's go.mod is unnecessary because go.work joins workspaces at root build context (each service Dockerfile's deps stage only needs to COPY the go.mod of *its own* deps). Verify this assumption against `services/themes/Dockerfile` lines 8-30 — themes copies every other service's go.mod, which means **we MUST add `COPY services/library/go.mod ./services/library/` to every existing service Dockerfile**. Do this in Task 1 to keep the change atomic. Affected Dockerfiles: `services/{auth,catalog,streaming,player,rooms,scheduler,gateway,themes,maintenance,scraper}/Dockerfile`. Add the new COPY line in the same alphabetical slot the existing themes line occupies.

8. **`go.work`** — add `./services/library` to the `use ( ... )` block. Run `go work sync` after editing.

Verification commands (run inside the working tree):
```bash
cd services/library && go mod tidy && go build ./...
cd /data/animeenigma && go work sync && go build ./services/library/...
```
Both must succeed with zero errors.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/library && go build ./... && cd /data/animeenigma && go work sync</automated>
  </verify>
  <done>
    - `services/library/` directory exists with the full file layout listed above.
    - `go build ./...` succeeds inside `services/library/`.
    - `go.work` lists `./services/library` and `go work sync` is idempotent.
    - Every existing service Dockerfile in `services/{auth,catalog,streaming,player,rooms,scheduler,gateway,themes,maintenance,scraper}/Dockerfile` has the `COPY services/library/go.mod ./services/library/` line added.
  </done>
</task>

<task type="auto">
  <name>Task 2: Wire library into docker-compose + gateway routing + Makefile + .env.example</name>
  <files>
    docker/docker-compose.yml,
    docker/.env.example,
    services/gateway/internal/config/config.go,
    services/gateway/internal/handler/proxy.go,
    services/gateway/internal/service/proxy.go,
    services/gateway/internal/transport/router.go,
    Makefile
  </files>
  <action>
Three integration points, all driven by mirroring the existing themes service path:

1. **`docker/docker-compose.yml`** — three edits:

   a. **New `library` service block** (insert after the `themes:` block at line ~541):
   ```yaml
     library:
       build:
         context: ..
         dockerfile: services/library/Dockerfile
       container_name: animeenigma-library
       restart: unless-stopped
       environment:
         SERVER_PORT: 8087
         DB_HOST: postgres
         DB_PORT: 5432
         DB_USER: postgres
         DB_PASSWORD: postgres
         DB_NAME: ${LIBRARY_DB_NAME:-library}
         JWT_SECRET: ${JWT_SECRET:-dev-secret-change-in-production}
       ports:
         - "127.0.0.1:8087:8087"
       volumes:
         - library_torrents:/data/torrents
         - library_minio_staging:/tmp/encode
       depends_on:
         postgres:
           condition: service_healthy
       deploy:
         resources:
           limits:
             cpus: '4.0'
             memory: 4G
   ```
   Notes: Port bound to `127.0.0.1` matching every other service. `deploy.resources.limits` is read by Swarm and as an advisory by Docker Compose v2 — keep it for documentation + future K8s parity (D-CONTEXT: 4 CPU / 4 GiB RAM). Volumes use the two named volumes added in (c) below.

   b. **Extend `gateway` env block** (around line ~344, in the `gateway:` service):
   - Add `LIBRARY_SERVICE_URL: http://library:8087` after the `THEMES_SERVICE_URL` line.
   - Add `- library` to the gateway's `depends_on:` list (after `- themes`).

   c. **Extend top-level `volumes:` block** (around line ~590):
   - Add `library_torrents:` and `library_minio_staging:` entries (no driver/options — default local driver is correct; both are transient by spec).

2. **`docker/.env.example`** — append a new block at the end of the file (or after the existing per-service env blocks):
   ```
   # =============================================================================
   # Library Service (workstream raw-jp / v0.2) — port 8087
   # =============================================================================
   # The library service uses its own Postgres database, auto-created on first
   # start via libs/database.New(). The default is `library` and rarely needs
   # changing; override only if you need an isolated test database.
   LIBRARY_DB_NAME=library
   # All other LIBRARY_DB_* values inherit DB_HOST / DB_PORT / DB_USER /
   # DB_PASSWORD from the shared Postgres credentials above. Phase 3 of the
   # raw-jp workstream adds LIBRARY_TORRENT_* and LIBRARY_ENCODE_* knobs; this
   # phase only documents the DB connection.
   ```

3. **Gateway integration** (three coordinated edits per the `<interfaces>` block above):

   a. **`services/gateway/internal/config/config.go`**:
   - Add `LibraryService string` field to `ServiceURLs` (after `ThemesService` to keep alphabetical-ish ordering).
   - Add `LibraryService: getEnv("LIBRARY_SERVICE_URL", "http://library:8087"),` in `Load()`.

   b. **`services/gateway/internal/handler/proxy.go`**:
   - Add a new method right after `ProxyToThemes` (line ~60):
     ```go
     // ProxyToLibrary proxies requests to the library service (workstream raw-jp / v0.2).
     func (h *ProxyHandler) ProxyToLibrary(w http.ResponseWriter, r *http.Request) {
         h.proxy(w, r, "library")
     }
     ```

   c. **`services/gateway/internal/service/proxy.go`** — extend the service URL lookup (whatever shape it has: switch statement, map, or chain of conditionals — mirror the `"themes"` case verbatim with `"library"` → `cfg.Services.LibraryService`). Read the file first to determine the exact pattern.

   d. **`services/gateway/internal/transport/router.go`** — add a new `r.Route("/library", ...)` block alongside the `/themes` block (around line ~310):
   ```go
   // Library service routes (workstream raw-jp / v0.2). Public health passthrough
   // + wildcard so Phases 2-5 can add handlers without touching the gateway router.
   // Admin endpoints (POST/DELETE /jobs, etc.) are added in later phases; the
   // gateway gates them with JWTValidationMiddleware + AdminRoleMiddleware then.
   r.Route("/library", func(r chi.Router) {
       r.Get("/health", proxyHandler.ProxyToLibrary)
       r.HandleFunc("/*", proxyHandler.ProxyToLibrary)
   })
   ```
   The wildcard handler is intentional Phase-1 forward-compat — Phase 5 will add admin auth groups inside this block; for now everything passes through and the library service itself enforces nothing (only `/health` exists).

4. **`Makefile`** — two edits:
   - Line 9 `SERVICES := auth catalog streaming player rooms scheduler gateway themes scraper` → append ` library` at the end so `make build` and `make clean` pick it up.
   - In the `health:` target (line ~436), insert one new line in alphabetical-ish service order (after `scraper:8088`):
     ```
     @curl -sf http://localhost:8087/health > /dev/null && echo "✓ library:8087" || echo "✗ library:8087"
     ```
   - **Do not** add `redeploy-library`, `restart-library`, `logs-library` as explicit targets — the existing generic `redeploy-%`, `restart-%`, `logs-%` pattern targets (Makefile:275-290) already match. SPEC LIB-02 "three new targets" requirement is satisfied by the pattern targets; document the resolution in the commit message.

Acceptance for this task: see `<verify>` below. End state: `make redeploy-library` builds and starts the container; `curl http://localhost:8087/health` and `curl http://localhost:8000/api/library/health` both return `{"status":"ok"}`; `make health` shows `✓ library:8087`; Postgres `library` database exists.
  </action>
  <verify>
    <automated>cd /data/animeenigma && docker compose -f docker/docker-compose.yml config --quiet && go build ./services/gateway/... && make redeploy-library && sleep 5 && curl -sf http://localhost:8087/health | grep -q '"status":"ok"' && curl -sf http://localhost:8000/api/library/health | grep -q '"status":"ok"' && curl -sf http://localhost:8087/metrics | grep -q '^# HELP' && docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -l | grep -q '^ library' && make health 2>&1 | grep -q '✓ library:8087'</automated>
  </verify>
  <done>
    - `docker compose -f docker/docker-compose.yml config --quiet` exits 0 (compose file is valid).
    - `make redeploy-library` builds and starts the container; it stays running (`docker ps | grep animeenigma-library` shows Up status).
    - `curl -s http://localhost:8087/health` returns `{"status":"ok"}` (note: SPEC's `.data.status` jq path is wrong — the actual httputil.OK envelope is flat; verify against `.status` instead).
    - `curl -s http://localhost:8000/api/library/health` returns the same payload via the gateway.
    - `curl -s http://localhost:8087/metrics` returns Prometheus exposition; after at least one prior request, `grep '^http_requests_total' | wc -l` is > 0.
    - `docker compose exec postgres psql -U postgres -l` lists a `library` database.
    - `make health` output contains `✓ library:8087`.
  </done>
</task>

<task type="auto">
  <name>Task 3: Documentation — CLAUDE.md Service Ports + Gateway Routing</name>
  <files>CLAUDE.md</files>
  <action>
Two edits to `CLAUDE.md`:

1. **Service Ports table** — insert a new row in the table (alphabetical-ish; between `scheduler` and `themes`):
   ```
   | library    | 8087 | /metrics  | Library service (BitTorrent → HLS → MinIO, admin-only) |
   ```
   Adjust column widths if needed to keep the table readable.

2. **Gateway Routing list** — insert one new bullet, after the `/api/themes/*` line:
   ```
   - `/api/library/*` → library:8087 (admin-only; routes added incrementally in v0.2 Phases 2–5)
   ```

**Do not** add the library service to any other section of CLAUDE.md (no need to mention it in the "Video Player Architecture" or "External API Integration" sections — those are user-facing capabilities, not infrastructure inventory). The full v0.2 architecture documentation lands when the milestone ships; this phase only updates the two operator-facing tables required by SPEC LIB-NF-04.

After editing, run `grep -E "library:8087" CLAUDE.md | wc -l` and confirm the output is exactly 2.
  </action>
  <verify>
    <automated>grep -E "library:8087" /data/animeenigma/CLAUDE.md | wc -l | grep -qx '2' && grep -q '/api/library/\*' /data/animeenigma/CLAUDE.md</automated>
  </verify>
  <done>
    - `grep -E "library:8087" CLAUDE.md | wc -l` returns `2` (Service Ports row + Gateway Routing bullet).
    - `grep '/api/library/\*' CLAUDE.md` finds the gateway routing entry.
    - File is well-formed markdown (no broken tables).
  </done>
</task>

</tasks>

<verification>
End-to-end manual smoke (run after all three tasks committed):

```bash
# Build + start
make redeploy-library
sleep 5

# Direct health
curl -s http://localhost:8087/health
# expected: {"status":"ok"}

# Gateway-proxied health
curl -s http://localhost:8000/api/library/health
# expected: {"status":"ok"}

# Metrics endpoint present and emitting after a request
curl -s http://localhost:8087/health > /dev/null
curl -s http://localhost:8087/metrics | grep '^http_requests_total' | head -3
# expected: at least one http_requests_total{...} line with service="library"

# DB exists
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -l | grep library
# expected: one line showing the `library` database owned by postgres

# Make health includes library
make health
# expected: line "✓ library:8087" present

# Other services still healthy (regression check — no compose-block breakage)
make health
# expected: all existing services still show ✓
```

If any step fails, the executor must fix in-place and re-verify before committing — do not commit a broken scaffold.
</verification>

<success_criteria>
All nine SPEC acceptance criteria pass:

1. `services/library/cmd/library-api/main.go` exists and `cd services/library && go build ./...` succeeds.
2. `docker/docker-compose.yml` has a `library` service block with ports `["127.0.0.1:8087:8087"]`, volumes `[library_torrents, library_minio_staging]`, `depends_on: postgres`, and resource limits 4 CPU / 4 GiB.
3. `make redeploy-library` (or `docker compose up -d library`) starts the container and it stays running.
4. `curl -s http://localhost:8087/health` returns `{"status":"ok"}`. **(Adjusted from SPEC's `jq -r .data.status` — the codebase's `httputil.OK` envelope is flat; see Task 1 step 4 note.)**
5. `curl -s http://localhost:8000/api/library/health` returns the same `{"status":"ok"}` via gateway proxy.
6. `curl -s http://localhost:8087/metrics | grep '^http_requests_total' | wc -l` is > 0 after the first request.
7. `docker compose exec postgres psql -U postgres -l` lists the `library` database.
8. `make health` output contains `✓ library:8087`.
9. `CLAUDE.md` and `docker/.env.example` are updated per LIB-NF-04 (`grep -E "library:8087" CLAUDE.md | wc -l` → 2; `grep LIBRARY_DB_NAME docker/.env.example` → 1).

Out-of-scope items (NOT verified here, picked up by Phases 2-6):
- No torrent client, ffmpeg, MinIO writer, or admin UI.
- No real Postgres migrations (only `.gitkeep` placeholder).
- No `library_jobs` / `library_episodes` / `library_filename_patterns` tables.
- No hybrid resolver (catalog still falls through to AllAnime exclusively).
- No `/api/library/search`, `/api/library/jobs`, or `/api/library/episodes/...` endpoints.
</success_criteria>

<out_of_scope>
Per SPEC and CONTEXT — explicitly **NOT** part of this phase:

- Any business logic (Phases 2-6 add it).
- Real DB migrations (Phase 3 / 4 add `library_jobs`, `library_episodes`, `library_filename_patterns` SQL).
- Torrent client wrapper (`anacrolix/torrent`) — Phase 3.
- ffmpeg subprocess wrapper — Phase 4.
- MinIO writer + bucket bootstrap — Phase 4.
- Admin UI (`RawLibrary.vue`) — Phase 5.
- Hybrid resolver in catalog — Phase 6.
- `library_jobs` Postgres-backed queue with `FOR UPDATE SKIP LOCKED` — Phase 3.
- Disk-free guard, job stall detection, library-specific Prometheus metrics — Phase 3 (LIB-NF-01/02/03).
- Grafana dashboard `infra/grafana/dashboards/library.json` — Phase 3.
- Operator runbook entry under `docs/issues/README.md` (`ISS-013`) — Phase 3 or 6 (whichever phase first exposes operator-visible failure modes).

If any of the above show up in the executor's diff for this phase, that's scope creep and the change should be rolled back into a Phase 2+ branch.
</out_of_scope>

<output>
After completion, create `.planning/workstreams/raw-jp/phases/01-library-service-scaffold/01-SUMMARY.md` documenting:
- Final file tree under `services/library/`.
- Any deviations from the SPEC (notably the `.data.status` → `.status` JSON envelope correction).
- The Dockerfile go.mod-COPY proliferation (every existing service's Dockerfile gained one line — verify in summary that this was applied and didn't break other services' builds).
- Confirmation that all three integration points (gateway config + handler + service.proxy + router) are wired.
- Pointers to the next phase's SPEC (`v0.2-phases/02-nyaa-animetosho-clients/02-SPEC.md`).
</output>
