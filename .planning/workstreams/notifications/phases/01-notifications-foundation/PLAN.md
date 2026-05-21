---
phase: 01-notifications-foundation
plan: 01
type: execute
workstream: notifications
milestone: v1.0
wave: 1
depends_on: []
files_modified:
  # New service
  - services/notifications/cmd/notifications-api/main.go
  - services/notifications/go.mod
  - services/notifications/Dockerfile
  - services/notifications/internal/config/config.go
  - services/notifications/internal/domain/notification.go
  - services/notifications/internal/domain/snapshot.go
  - services/notifications/internal/repo/views.go
  - services/notifications/internal/repo/notification.go
  - services/notifications/internal/repo/snapshot.go
  - services/notifications/internal/repo/indexes.go
  - services/notifications/internal/service/notification.go
  - services/notifications/internal/handler/notification.go
  - services/notifications/internal/handler/internal.go
  - services/notifications/internal/transport/router.go
  - services/notifications/internal/job/doc.go
  # Workspace + compose + gateway
  - go.work
  - docker/docker-compose.yml
  - docker/.env.example
  - services/gateway/internal/config/config.go
  - services/gateway/internal/handler/proxy.go
  - services/gateway/internal/transport/router.go
  # Makefile + docs + seed
  - Makefile
  - CLAUDE.md
  - scripts/seed-notification-for-ui-audit-user.sh
autonomous: true
requirements:
  - NOTIF-FOUND-01
  - NOTIF-FOUND-02
  - NOTIF-FOUND-03
  - NOTIF-FOUND-04
  - NOTIF-FOUND-05
  - NOTIF-FOUND-06
  - NOTIF-FOUND-07
  - NOTIF-FOUND-08
  - NOTIF-NF-04   # partial — service-ports row + gateway-routing row + env-var doc

must_haves:
  truths:
    - "make redeploy-notifications builds clean; the container starts; make health includes 'notifications:8087 - healthy'"
    - "curl http://localhost:8087/health on the host returns 200 {\"status\":\"ok\"}"
    - "curl -H 'Authorization: Bearer $UI_AUDIT_API_KEY' http://localhost:8000/api/notifications returns 200 with an empty list (gateway proxy + JWT auth both work)"
    - "Tables user_notifications and parser_episode_snapshots exist in the shared animeenigma Postgres DB; both partial indexes uk_user_dedupe and idx_user_unread exist on user_notifications"
    - "scripts/seed-notification-for-ui-audit-user.sh inserts one new_episode row; the same curl from above then returns that row with the expected payload shape; POST /api/notifications/{id}/dismiss + re-fetch shows unread_count:0"
    - "Re-running the seed against the same (user_id, dedupe_key) UPSERTs in place — SELECT COUNT(*) FROM user_notifications WHERE user_id=...AND dedupe_key=... stays at 1"
    - "POST /internal/notifications is NOT reachable through the gateway from outside Docker (curl http://localhost:8000/internal/notifications returns 404), but docker compose exec notifications wget -O- localhost:8087/internal/notifications works"
  artifacts:
    - path: "services/notifications/cmd/notifications-api/main.go"
      provides: "Service entrypoint — DB connect, AutoMigrate, EnsureIndexes, router boot on :8087"
      contains: "EnsureIndexes"
    - path: "services/notifications/internal/domain/notification.go"
      provides: "UserNotification GORM model + payload type constants"
      contains: "type UserNotification struct"
    - path: "services/notifications/internal/domain/snapshot.go"
      provides: "ParserEpisodeSnapshot GORM model with uk_combo unique composite index"
      contains: "uniqueIndex:uk_combo"
    - path: "services/notifications/internal/repo/views.go"
      provides: "Read-only GORM views for watch_history, anime_list, animes — NEVER in AutoMigrate"
      contains: "READ-ONLY"
    - path: "services/notifications/internal/repo/indexes.go"
      provides: "EnsureIndexes(ctx) — raw SQL CREATE for the two partial indexes GORM cannot express"
      contains: "uk_user_dedupe"
    - path: "services/notifications/internal/handler/internal.go"
      provides: "POST /internal/notifications UPSERT producer + GET /internal/health"
      contains: "ON CONFLICT"
    - path: "services/notifications/internal/job/doc.go"
      provides: "Placeholder doc.go reserving the job/ package for Phase 2's cron jobs"
      contains: "Phase 2"
    - path: "services/gateway/internal/transport/router.go"
      provides: "/api/notifications/* proxied to notifications:8087 under authMiddleware + userRateLimit"
      contains: "ProxyToNotifications"
    - path: "scripts/seed-notification-for-ui-audit-user.sh"
      provides: "Idempotent UPSERT-via-internal-API seed for ui_audit_bot's new_episode notification"
      contains: "docker compose"
  key_links:
    - from: "services/notifications/cmd/notifications-api/main.go"
      to: "services/notifications/internal/repo/indexes.go::EnsureIndexes"
      via: "main.go calls EnsureIndexes(ctx, db.DB) immediately after AutoMigrate so partial indexes are created on every boot (idempotent CREATE INDEX IF NOT EXISTS)"
      pattern: "EnsureIndexes\\(ctx"
    - from: "services/gateway/internal/transport/router.go"
      to: "services/notifications/internal/transport/router.go"
      via: "Gateway proxies /api/notifications and /api/notifications/* (chi.Route under JWTValidationMiddleware + userRateLimit) to NotificationsService URL"
      pattern: "ProxyToNotifications"
    - from: "services/notifications/internal/handler/notification.go"
      to: "authz.UserIDFromContext"
      via: "All public handlers read user_id from JWT claims via authz.ClaimsFromContext / authz.UserIDFromContext — NOT from a X-User-ID header (themes precedent)"
      pattern: "authz\\.(Claims|UserID)FromContext"
    - from: "services/notifications/internal/repo/notification.go"
      to: "ON CONFLICT (user_id, dedupe_key) WHERE dismissed_at IS NULL DO UPDATE"
      via: "UPSERT in the internal producer path; matches the partial unique index so a dismissed row does not block a fresh re-fire"
      pattern: "ON CONFLICT"
---

<objective>
Stand up a new Go microservice `services/notifications/` on port **8087** with a dedicated CRUD HTTP API and an internal UPSERT producer endpoint, two new tables (`user_notifications` + `parser_episode_snapshots`) created via GORM `AutoMigrate` + a post-migrate `EnsureIndexes` helper for the two partial indexes GORM cannot express, read-only GORM views for the three cross-service tables Phase 2's detector will read (`watch_history`, `anime_list`, `animes`), gateway proxy for `/api/notifications/*` behind the existing JWT middleware, Makefile shortcuts, and a `scripts/seed-notification-for-ui-audit-user.sh` smoke seeder so Phase 3's frontend can develop against the API before Phase 2's detector exists.

**Purpose:** Phase 1 is the infra spine of the v1.0 Notifications Engine. By the end of this phase the engine is a working CRUD surface with a producer endpoint; Phase 2 only adds the cron detector that calls that producer, and Phase 3 only adds the Vue frontend that consumes the public CRUD. No code in Phases 2/3 touches the gateway, docker-compose, or CLAUDE.md again — those are all locked here so subsequent phases are pure additive code with zero infra coordination.

**Output:** A redeployable `notifications` service; two tables + indexes in the shared `animeenigma` DB; a `/api/notifications/*` gateway route; the seed script + manual verification flow documented in this plan's verification matrix; updated CLAUDE.md.

No detector. No cron. No frontend. No catalog endpoint. Those are explicitly Phase 2/3 scope.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/notifications/PROJECT.md
@.planning/workstreams/notifications/REQUIREMENTS.md
@.planning/workstreams/notifications/ROADMAP.md
@docs/superpowers/specs/2026-05-11-notifications-engine-design.md
@CLAUDE.md
@services/themes/Dockerfile
@services/themes/cmd/themes-api/main.go
@services/themes/internal/config/config.go
@services/themes/internal/transport/router.go
@services/themes/internal/handler/rating.go
@services/auth/internal/transport/router.go
@services/gateway/internal/transport/router.go
@services/gateway/internal/handler/proxy.go
@services/gateway/internal/config/config.go
@libs/database/database.go
@libs/authz/jwt.go

<interfaces>
<!-- Key types, conventions, and signatures the executor needs. Extracted from codebase 2026-05-20. -->
<!-- Use these directly — no further codebase exploration required. -->

<!-- libs/database — single-connection pattern; one *DB per service -->
type Config struct { Host, Port, User, Password, Database, SSLMode ... }
func New(cfg Config) (*DB, error)                  // auto-creates DB if missing
func (db *DB) AutoMigrate(models ...interface{}) error
func (db *DB) Close() error
func (db *DB) Health() error
// db.DB is the underlying *gorm.DB — pass it into repos.

<!-- libs/authz — JWT claims pulled from context, NOT from X-User-ID header -->
type Claims struct { UserID string `json:"uid"`; Role string; ... }
func ClaimsFromContext(ctx) (*Claims, bool)
func UserIDFromContext(ctx) string                  // returns "" if no claims
func IsAdmin(ctx) bool

<!-- libs/httputil — standard response helpers used by every service -->
func OK(w, body)         // 200 + JSON
func BadRequest(w, msg)  // 400
func Unauthorized(w)     // 401
func Forbidden(w)        // 403
func NotFound(w, msg)    // 404
func Error(w, err)       // domain-error-aware
func Bind(r, &dst) error // JSON body parse
func BearerToken(r) string
func RequestLogger(log) middleware
func Recoverer(log) middleware
func CORS([]string) middleware

<!-- libs/metrics — Prometheus collector boilerplate -->
func NewCollector(serviceName string) *Collector   // labels every metric with service=...
func (*Collector) Middleware(next) Handler         // increments http_requests_total + http_request_duration_seconds
func Handler() http.Handler                        // serves /metrics
func StartDBPoolCollector(sqlDB, every time.Duration)

<!-- libs/logger -->
log := logger.Default()
log.Infow("msg", "key", value)
log.Errorw("msg", "error", err)
log.Fatalw("msg", "error", err)

<!-- libs/authz JWT middleware pattern (copy from services/themes/internal/transport/router.go)
     AuthMiddleware(cfg) validates token + populates ctx with claims;
     handlers then call authz.ClaimsFromContext(r.Context()). -->

<!-- Gateway proxy handler pattern (services/gateway/internal/handler/proxy.go) -->
type ProxyHandler struct{ proxyService *service.ProxyService; log *logger.Logger }
func (h *ProxyHandler) ProxyToThemes(w, r) { h.proxy(w, r, "themes") }
// Add: func (h *ProxyHandler) ProxyToNotifications(w, r) { h.proxy(w, r, "notifications") }
// proxyService internally maps the "notifications" string to cfg.Services.NotificationsService

<!-- Gateway ServiceURLs (services/gateway/internal/config/config.go around line 95) -->
Services: ServiceURLs{
    ThemesService:    getEnv("THEMES_SERVICE_URL", "http://themes:8086"),
    LibraryService:   getEnv("LIBRARY_SERVICE_URL", "http://library:8089"),
    // ADD: NotificationsService: getEnv("NOTIFICATIONS_SERVICE_URL", "http://notifications:8087"),
}

<!-- Gateway route block to add (mirror /themes block in services/gateway/internal/transport/router.go ~line 311) -->
r.Route("/notifications", func(r chi.Router) {
    r.Group(func(r chi.Router) {
        r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
        r.Use(userRateLimit)
        r.Get("/",                proxyHandler.ProxyToNotifications)
        r.Get("/unread-count",    proxyHandler.ProxyToNotifications)
        r.Post("/mark-all-read",  proxyHandler.ProxyToNotifications)
        r.Post("/{id}/read",      proxyHandler.ProxyToNotifications)
        r.Post("/{id}/dismiss",   proxyHandler.ProxyToNotifications)
        r.Post("/{id}/click",     proxyHandler.ProxyToNotifications)
    })
})

<!-- UserNotification GORM model — design doc §Data Model -->
type UserNotification struct {
    ID          string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    UserID      string         `gorm:"type:uuid;not null;index" json:"user_id"`
    Type        string         `gorm:"size:32;not null;index" json:"type"`
    DedupeKey   string         `gorm:"size:255;not null" json:"dedupe_key"`
    Payload     datatypes.JSON `gorm:"type:jsonb;not null" json:"payload"`
    ReadAt      *time.Time     `json:"read_at"`
    DismissedAt *time.Time     `gorm:"index" json:"dismissed_at"`
    ClickedAt   *time.Time     `json:"clicked_at"`
    CreatedAt   time.Time      `gorm:"index" json:"created_at"`
    UpdatedAt   time.Time      `json:"updated_at"`
}
// + the two partial indexes are created via raw SQL by repo.EnsureIndexes, NOT GORM tags.

<!-- ParserEpisodeSnapshot — composite uk_combo expressible in pure GORM tags -->
type ParserEpisodeSnapshot struct {
    ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    AnimeID       string    `gorm:"type:uuid;not null;uniqueIndex:uk_combo,priority:1"`
    Player        string    `gorm:"size:20;not null;uniqueIndex:uk_combo,priority:2"`
    Language      string    `gorm:"size:5;not null;uniqueIndex:uk_combo,priority:3"`
    WatchType     string    `gorm:"size:5;not null;uniqueIndex:uk_combo,priority:4"`
    TranslationID string    `gorm:"size:50;not null;uniqueIndex:uk_combo,priority:5"`
    LatestEpisode int       `gorm:"not null"`
    CheckedAt     time.Time
    UpdatedAt     time.Time
}

<!-- Read-only views — match existing schemas; never AutoMigrated -->
// services/notifications/internal/repo/views.go
// READ-ONLY VIEW — owned by player service. Do NOT include in AutoMigrate.
type WatchHistoryView struct {
    UserID, AnimeID, Player, Language, WatchType, TranslationID string
    EpisodeNumber int
    // ... mirror services/player/internal/domain/watch_history.go fields used by Phase 2's detector
}
// (same for AnimeListView matching player's anime_list, AnimeView matching catalog's animes)

<!-- Internal middleware pattern (services/auth/internal/transport/router.go line 47-48):
     "Internal endpoints (only reachable within Docker network)"
     r.Post("/internal/resolve-api-key", authHandler.ResolveApiKey)
     Note: there is NO middleware wrapper — internal-ness is enforced by the gateway
     simply not proxying anything under /internal. Mirror that here. -->
</interfaces>
</context>

<decisions>

### D-01: Single shared Postgres DB (`animeenigma`), NOT a dedicated `notifications` DB

**Conflict in source artifacts:**
- `REQUIREMENTS.md` NOTIF-FOUND-01 says "Dedicated Postgres DB `notifications` auto-created via `libs/database.New()`"
- `PROJECT.md` Decision #7 (locked at design time) says "Cross-service reads of `watch_history`, `anime_list`, `animes` via shared Postgres — standard project pattern"
- Design doc §Architecture shows ALL five tables (`user_notifications`, `parser_episode_snapshots`, `watch_history`, `anime_list`, `animes`) inside one "Shared Postgres" box

**Codebase evidence (single-DB pattern is established):**
- `services/{auth,catalog,player,scheduler,themes}/internal/config/config.go` all default `DB_NAME=animeenigma`
- `services/themes/internal/repo/theme.go:50` does `LEFT JOIN animes ON ...` from the themes service — same `*gorm.DB`, no second connection
- `services/player/internal/service/recs/signals/s1_score_cluster.go:83` does `db.Table("anime_list")` and `db.Table("animes")` from player — same handle
- The only outlier is `services/library/` (`DB_NAME=library`) but library does NO cross-service joins; it only invokes catalog over HTTP

**Resolution:** Treat REQUIREMENTS.md's wording as a writeup slip. The locked decision (PROJECT.md #7 + design-doc architecture) wins. The notifications service uses `DB_NAME=animeenigma` in `docker-compose.yml` and `internal/config/config.go`. The two new tables live alongside `watch_history`/`anime_list`/`animes`. Read-only GORM views in `repo/views.go` then work via the same `*gorm.DB` — no second handle, no cross-DB plumbing.

**Phase 2 follow-up:** This decision also resolves NOTIF-DET-02's "second cross-DB read connection (or ?search_path swap) — pick whichever is the established project pattern" — the answer is "neither needed, single handle suffices". This will be noted in the Phase 2 plan.

**Migration ergonomics:** A future operator who wants notifications on a separate physical DB can flip `DB_NAME=notifications` in `docker-compose.yml` — `libs/database.New()` will auto-create the DB and AutoMigrate the two service-owned tables. The read-only views would then need to point at a second connection, but that's a v1.1 concern with a clean migration path.

### D-02: `job/doc.go` placeholder ONLY — option (a)

**Choice:** Ship `services/notifications/internal/job/doc.go` with a `// Package job ...` doc comment reserving the package for Phase 2's cron jobs. No `scheduler.go`, no `cron.Cron` instance.

**Rationale:**
- Phase 1 has zero behavioral need for cron — shipping an empty `cron.Cron` adds a goroutine + a `robfig/cron/v3` dependency in `go.mod` that does nothing, which violates the "Phase 1 = pure infra, no detector logic" carve-out from ROADMAP.md.
- A `doc.go` reserves the directory shape so Phase 2's plan diff is "add detector.go, hotcombos.go, cleanup.go, scheduler.go" rather than "create the job/ dir + add four files" — keeps the Phase 2 review tight.
- Option (c) "skip the directory entirely" was rejected because the ROADMAP touches-list explicitly lists `internal/{config,domain,handler,repo,service,job,transport}/` — shipping the directory makes the Phase 1 / Phase 2 boundary visible in PR diffs.
- Option (b) "stub cron.Cron" was rejected because it introduces a runtime resource (a goroutine + ticker scheduled via `c.Start()`) with no consumers; an unused cron is a subtle leak signal in `make logs-notifications`.

### D-03: `user_id` from JWT claims via `authz.UserIDFromContext`, NOT from `X-User-ID` header

**Conflict in source artifacts:** REQUIREMENTS.md NOTIF-FOUND-04 says "read user_id from the gateway-forwarded `X-User-ID` header (project convention — see `services/themes/internal/handler/`)".

**Codebase evidence:** Grep confirms `services/themes/internal/handler/rating.go:30` reads `authz.ClaimsFromContext(r.Context())` — no `X-User-ID` header is read anywhere in `services/themes/internal/handler/`. The actual project convention is: the **gateway** validates the JWT (`JWTValidationMiddleware`), the **downstream service** ALSO validates the JWT (`AuthMiddleware` in `transport/router.go`) and reads claims from context. This double-validation is by design — services are independently authenticatable.

**Resolution:** Follow the established themes-precedent pattern. Notifications service mounts `AuthMiddleware(cfg.JWT)` on `/api/notifications/*` and handlers call `authz.UserIDFromContext(r.Context())`. The `X-User-ID` header is not referenced anywhere.

### D-04: `gorm.io/datatypes` for the JSONB `payload` column

**New dependency.** Not currently used anywhere in the monorepo (`grep -r "gorm.io/datatypes" services/ libs/ --include=go.mod` returns nothing). Required for the `datatypes.JSON` field type the design doc specifies.

**Alternative considered:** Use `json.RawMessage` or `[]byte` with a custom `gorm:"type:jsonb"` tag. Rejected because `datatypes.JSON` is GORM's canonical JSONB helper and provides `Value()`/`Scan()` plus `JSONQuery` helpers that Phase 2's `jsonb_set(...)` UPSERT will lean on. The dependency cost is one `go get` + one require/replace pair (no replace needed — it's an external module, not a local `libs/*`).

### D-05: Internal endpoint security model — gateway-non-routing, not middleware

**Codebase evidence:** `services/auth/internal/transport/router.go:47-48` registers `POST /internal/resolve-api-key` directly on the root chi router with NO middleware. Internal-ness is enforced by the **gateway never proxying anything under `/internal`** — combined with the Docker network being non-routable from outside, this is the project's "internal" pattern.

**Resolution:** Mirror that. `/internal/notifications` is registered on the notifications service's root router with no auth middleware. The gateway's new `/api/notifications/*` block does NOT include any rule that proxies to `/internal/*`. Task verification (success criterion 5) confirms `curl http://localhost:8000/internal/notifications` returns 404 from the host.

</decisions>

<touch_list>

**Service code (new):**
- `services/notifications/cmd/notifications-api/main.go` — entrypoint: config load, `database.New`, `db.AutoMigrate(&UserNotification{}, &ParserEpisodeSnapshot{})`, `repo.EnsureIndexes(ctx, db.DB)`, wire repos/services/handlers, mount router on `:8087`, graceful shutdown.
- `services/notifications/internal/config/config.go` — mirror `services/themes/internal/config/config.go` with `SERVER_PORT=8087`, `DB_NAME=animeenigma`, `JWT_*`.
- `services/notifications/internal/domain/notification.go` — `UserNotification` GORM struct (see interfaces block) + `NotificationType` constants (only `TypeNewEpisode = "new_episode"` in v1.0) + `NewEpisodePayload` Go struct (mirrors design-doc payload JSON).
- `services/notifications/internal/domain/snapshot.go` — `ParserEpisodeSnapshot` struct with `uniqueIndex:uk_combo,priority:N` tags.
- `services/notifications/internal/repo/views.go` — `WatchHistoryView`, `AnimeListView`, `AnimeView` with READ-ONLY comments + `TableName()` overrides mapping to the existing physical tables. No `gen_random_uuid()` defaults, no `AutoMigrate`.
- `services/notifications/internal/repo/notification.go` — CRUD: `List(userID, status, limit, offset)`, `Get(userID, id)`, `UnreadCount(userID)`, `MarkRead(userID, id)`, `MarkAllRead(userID)`, `Dismiss(userID, id)`, `Click(userID, id)`, plus `Upsert(userID, type, dedupeKey, payload)` (the producer path).
- `services/notifications/internal/repo/snapshot.go` — Phase 1 ships only the type definition and a `// TODO(phase 2): bulk load / bulk upsert` placeholder; the table just needs to exist.
- `services/notifications/internal/repo/indexes.go` — `func EnsureIndexes(ctx context.Context, db *gorm.DB) error` runs two `CREATE UNIQUE INDEX IF NOT EXISTS uk_user_dedupe ON user_notifications (user_id, dedupe_key) WHERE dismissed_at IS NULL` + the analogous `idx_user_unread` statement via `db.WithContext(ctx).Exec(...)`. Idempotent.
- `services/notifications/internal/service/notification.go` — thin orchestration layer wrapping the repo: input validation, payload-JSON marshal, dedupe-key construction helpers (`NewEpisodeDedupeKey(animeID, player, language, watchType, translationID) string`).
- `services/notifications/internal/handler/notification.go` — 6 public handlers + JSON shapes (`ListResponse{Notifications, UnreadCount, Total}`, etc.). All handlers extract `userID := authz.UserIDFromContext(r.Context())`.
- `services/notifications/internal/handler/internal.go` — `POST /internal/notifications` + `GET /internal/health`. The POST decodes `{user_id, type, dedupe_key, payload}` and calls `service.Upsert(...)`. The GET returns 200 OK.
- `services/notifications/internal/transport/router.go` — mirror `services/themes/internal/transport/router.go`: chi router with `RequestID`, `metricsCollector.Middleware`, `RequestLogger`, `Recoverer`, `CORS`. `GET /health`, `GET /metrics`, then `POST /internal/notifications` + `GET /internal/health` (root-level, no middleware), then `r.Route("/api/notifications", ...)` wrapping all 6 public routes under `AuthMiddleware(jwtConfig)`.
- `services/notifications/internal/job/doc.go` — single-file package placeholder. Body: `// Package job reserves the cron-job scaffold for Phase 2 of the v1.0 Notifications Engine workstream...`
- `services/notifications/Dockerfile` — verbatim copy of `services/themes/Dockerfile` with `themes` → `notifications` substitution AND a new `COPY services/notifications/go.mod ...` line in the deps stage AND `EXPOSE 8087` AND `CMD ["./notifications-api"]`.
- `services/notifications/go.mod` — module `github.com/ILITA-hub/animeenigma/services/notifications`, `go 1.24.0`, requires: `libs/{authz, database, errors, httputil, logger, metrics}`, `github.com/go-chi/chi/v5 v5.0.12`, `gorm.io/gorm v1.30.0`, `gorm.io/datatypes` (latest). `replace` block for every `libs/*` pointing at `../../libs/{name}`. (`scripts/seed-ui-audit-user.sh` test-user pattern — also referenced in CLAUDE.md.)

**Workspace + compose + env:**
- `go.work` — add `./services/notifications` to the existing `use(...)` block.
- `docker/docker-compose.yml` — new `notifications:` service block (mirror `themes:` block): build context `..`, dockerfile `services/notifications/Dockerfile`, `ports: ["8087:8087"]`, env `SERVER_PORT=8087`, `DB_*` pointing at `postgres:5432`/`animeenigma`, `JWT_SECRET`, `REDIS_HOST=redis` (for future use; harmless to set now). `depends_on: [postgres, redis]`. Also add `NOTIFICATIONS_SERVICE_URL: http://notifications:8087` to the gateway block's `environment:`.
- `docker/.env.example` — append `NOTIFICATIONS_SERVICE_URL=http://notifications:8087` (and any `NOTIFICATIONS_*` vars introduced).

**Gateway:**
- `services/gateway/internal/config/config.go` — add `NotificationsService string` to `ServiceURLs`, populate via `getEnv("NOTIFICATIONS_SERVICE_URL", "http://notifications:8087")` (mirror the `ThemesService` line).
- `services/gateway/internal/handler/proxy.go` — add `func (h *ProxyHandler) ProxyToNotifications(w, r) { h.proxy(w, r, "notifications") }` (mirror `ProxyToThemes`).
- `services/gateway/internal/service/proxy.go` — extend the internal service-name → URL map to include `"notifications": cfg.Services.NotificationsService`. (Verify exact map-extension shape during execution; this is where the proxy service resolves the service-name parameter from the handler.)
- `services/gateway/internal/transport/router.go` — add `r.Route("/notifications", ...)` block per the interfaces snippet (JWT + userRateLimit on every route).

**Tooling + docs:**
- `Makefile` — the existing wildcard `redeploy-%:` target (line 281) already handles `make redeploy-notifications`. Same for `restart-%` and `logs-%`. **No edit required** — confirm by inspection. (Triple-check the wildcard rule is unconditional; if it's gated by a hardcoded service list, add `notifications` to that list.)
- `CLAUDE.md` — add `| notifications | 8087 | /metrics | Generic notification engine (new episodes, future types) |` row to the Service Ports table; add `- /api/notifications/*` → `notifications:8087` (JWT required) line under Gateway Routing; add `NOTIFICATIONS_SERVICE_URL` to the gateway env-var documentation; add a small note `Notifications service specific:` block with `CATALOG_URL` (used by Phase 2 — declared in env now for forward-compat).
- `scripts/seed-notification-for-ui-audit-user.sh` — bash + `set -euo pipefail` (mirror `scripts/seed-ui-audit-user.sh` style). Reads `UI_AUDIT_USER_ID` via SQL lookup `SELECT id FROM users WHERE username='ui_audit_bot'`. Calls `docker compose -f docker/docker-compose.yml exec -T notifications wget -qO- --post-data='{...}' --header='Content-Type: application/json' http://localhost:8087/internal/notifications` with a sample `new_episode` payload (anime: Frieren, ep N/M, AniLibria, ru/dub, fake animeID UUID, watch_url). Idempotent because of the dedupe-key UPSERT.

</touch_list>

<tasks>

<task type="auto">
  <name>Task 1: Service scaffold + domain + Dockerfile + go.work + go.mod</name>
  <files>services/notifications/go.mod, services/notifications/Dockerfile, services/notifications/internal/config/config.go, services/notifications/internal/domain/notification.go, services/notifications/internal/domain/snapshot.go, services/notifications/internal/job/doc.go, go.work</files>
  <action>
Create the bare bones of the new microservice — module file, Dockerfile, config, domain models, job-package placeholder, workspace registration — so a `cd services/notifications && go build ./...` is green even before any handlers exist. (D-01: DB_NAME defaults to "animeenigma" — single shared DB. D-02: `job/doc.go` placeholder only, no cron.Cron. D-04: `gorm.io/datatypes` is the JSONB helper, fetched in this task.)

Mirror `services/themes/` exactly for shape: same Dockerfile multi-stage layout (builder/runtime), same `COPY libs/*/go.mod` discipline (add the new `services/notifications/go.mod` line; remove no existing lines), `EXPOSE 8087`, `CMD ["./notifications-api"]`.

`go.mod`: module path `github.com/ILITA-hub/animeenigma/services/notifications`. `go 1.24.0`. Require `libs/{authz, database, errors, httputil, logger, metrics}`, `github.com/go-chi/chi/v5 v5.0.12`, `gorm.io/gorm v1.30.0`, `gorm.io/datatypes`. Add `replace` block pointing every `libs/*` at `../../libs/{name}`. Run `cd services/notifications && go get gorm.io/datatypes && go mod tidy` to populate `go.sum`.

`config.go`: copy `services/themes/internal/config/config.go` verbatim except `SERVER_PORT` default = `8087`, drop the themes-specific bits. Keep `DB_NAME` default = `"animeenigma"` (NOT `"notifications"` — see D-01).

`domain/notification.go`: `UserNotification` struct per interfaces block (tags exactly as written; `datatypes.JSON` for Payload). Define `NotificationType` string-typed constants — only `TypeNewEpisode = "new_episode"` in v1.0. Define `NewEpisodePayload` Go struct mirroring the design-doc payload JSON (with json tags). Add a `TableName() string { return "user_notifications" }` only if needed to override the default GORM-pluralized name (it would be `user_notifications` either way — verify, omit override if redundant).

`domain/snapshot.go`: `ParserEpisodeSnapshot` struct per interfaces block. Verify the `uniqueIndex:uk_combo,priority:N` tag syntax works in `gorm.io/gorm v1.30.0` (it does — composite-index priority is standard GORM tag syntax since v1.20).

`job/doc.go`: single file. Body: `// Package job reserves the cron-job scaffold for Phase 2 of the v1.0 Notifications Engine workstream. // Phase 1 ships this as an empty package on purpose; Phase 2 adds detector.go, hotcombos.go, cleanup.go, and scheduler.go (which wires a robfig/cron/v3 instance). // Reference: .planning/workstreams/notifications/ROADMAP.md` then `package job`.

`go.work`: append `./services/notifications` to the `use(...)` block alphabetically between `./services/maintenance` and `./services/player` (or wherever alphabetical order dictates). Run `cd /data/animeenigma && go work sync`.
  </action>
  <verify>
    <automated>cd /data/animeenigma && go work sync &amp;&amp; cd services/notifications &amp;&amp; go build ./...</automated>
  </verify>
  <done>`go build ./...` exits 0 inside services/notifications. `go.work` lists notifications in the `use(...)` block. `services/notifications/Dockerfile` is byte-equivalent to themes Dockerfile in structure (diff only on service name + port + the new go.mod COPY line).</done>
</task>

<task type="auto">
  <name>Task 2: Repo layer (indexes + notification CRUD + read-only views + snapshot stub) and service layer</name>
  <files>services/notifications/internal/repo/indexes.go, services/notifications/internal/repo/notification.go, services/notifications/internal/repo/snapshot.go, services/notifications/internal/repo/views.go, services/notifications/internal/service/notification.go</files>
  <action>
Build the data-access + business-logic layers as a single coherent commit so they can be wired by Task 3's handlers without forward-referencing.

`indexes.go`: exports `func EnsureIndexes(ctx context.Context, db *gorm.DB) error`. Body runs exactly two `db.WithContext(ctx).Exec(...)` statements:
  - `CREATE UNIQUE INDEX IF NOT EXISTS uk_user_dedupe ON user_notifications (user_id, dedupe_key) WHERE dismissed_at IS NULL`
  - `CREATE INDEX IF NOT EXISTS idx_user_unread ON user_notifications (user_id, created_at DESC) WHERE dismissed_at IS NULL`

  Both use `IF NOT EXISTS` so re-boot is idempotent. Return wrapped error via `libs/errors.Wrap` on failure. Document at the top: `// EnsureIndexes creates partial indexes on user_notifications that GORM AutoMigrate cannot express. // MUST be called immediately after db.AutoMigrate(&UserNotification{}). Safe to call on every boot.`

`notification.go`: `type NotificationRepository struct { db *gorm.DB }` + `NewNotificationRepository(db) *NotificationRepository`. Methods:
  - `List(ctx, userID, status string, limit, offset int) (rows []domain.UserNotification, unreadCount int64, total int64, err error)` — when `status=="unread"` filter `read_at IS NULL AND dismissed_at IS NULL`; when `status=="all"` filter `dismissed_at IS NULL`. Default limit 20, cap at 100. ORDER BY `created_at DESC`. Returns total + unread_count via separate `Count()` queries (one transaction).
  - `Get(ctx, userID, id string) (*domain.UserNotification, error)` — returns `errors.NotFound` if not found OR owned by another user (404, not 403, to avoid id-enumeration leak per design-doc API table).
  - `UnreadCount(ctx, userID string) (int64, error)`
  - `MarkRead(ctx, userID, id string) error` — UPDATE ... SET read_at = NOW() WHERE id=? AND user_id=? AND read_at IS NULL. NotFound if 0 rows.
  - `MarkAllRead(ctx, userID string) error` — bulk UPDATE WHERE user_id=? AND read_at IS NULL AND dismissed_at IS NULL.
  - `Dismiss(ctx, userID, id string) error` — UPDATE ... SET dismissed_at = NOW().
  - `Click(ctx, userID, id string) error` — UPDATE ... SET clicked_at = NOW() WHERE clicked_at IS NULL.
  - `Upsert(ctx, userID, ntype, dedupeKey string, payload []byte) (*domain.UserNotification, error)` — uses GORM `Clauses(clause.OnConflict{Columns: []clause.Column{{Name:"user_id"},{Name:"dedupe_key"}}, TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "dismissed_at IS NULL"}}}, DoUpdates: clause.Assignments(map[string]interface{}{"payload": ..., "updated_at": gorm.Expr("NOW()"), "read_at": nil})})` to express the partial-index-aware UPSERT. Return the resulting row (use `RETURNING *` clause or re-fetch by `(user_id, dedupe_key)`).

`snapshot.go`: `type SnapshotRepository struct { db *gorm.DB }` + `NewSnapshotRepository(db) *SnapshotRepository`. v1.0 Phase 1 only needs the type + constructor; add a comment `// TODO(phase 2 detector): BulkLoad(combos) / BulkUpsert(snaps) — see services/notifications/internal/job/`. No methods needed yet — the table just has to exist so AutoMigrate runs against it.

`views.go`: three read-only structs `WatchHistoryView`, `AnimeListView`, `AnimeView`. Each prefixed with a multiline comment: `// READ-ONLY VIEW — table owned by <player|catalog> service. // DO NOT add to db.AutoMigrate in cmd/notifications-api/main.go. // Phase 2's detector reads from these to compute hot combos.` Each defines a `TableName() string` returning the existing physical table name (`watch_history`, `anime_list`, `animes`). Fields are MINIMAL — only the columns Phase 2's hot-combo + per-user-max-watched queries touch (UserID, AnimeID, Player, Language, WatchType, TranslationID, EpisodeNumber for WatchHistoryView; UserID, AnimeID, Status for AnimeListView; ID, ShikimoriID, Status, Name, NameRU, PosterURL for AnimeView). Cross-reference the source-of-truth structs in `services/player/internal/domain/watch_history.go` and `services/catalog/internal/domain/anime.go` for exact field names + types — copy the GORM tags verbatim for the fields included.

`service/notification.go`: `type NotificationService struct { repo *repo.NotificationRepository; log *logger.Logger }`. Wraps repo calls. Adds:
  - `NewEpisodeDedupeKey(animeID, player, language, watchType, translationID string) string` returning `fmt.Sprintf("new_episode:%s:%s:%s:%s:%s", ...)` per design-doc Dedupe Key spec.
  - `Upsert(ctx, req UpsertRequest) (*domain.UserNotification, error)` — validates `req.Type` is in the allowed-types set (only `"new_episode"` in v1.0), validates payload is valid JSON, marshals, calls `repo.Upsert`.
  - Other methods delegate 1:1 to the repo.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/notifications &amp;&amp; go build ./...</automated>
  </verify>
  <done>All five files compile clean as part of `go build ./...`. The read-only view structs reference real fields from player/catalog source structs (grep-verified). The Upsert clause uses GORM's `clause.OnConflict` with `TargetWhere` for the partial-index match.</done>
</task>

<task type="auto">
  <name>Task 3: HTTP handlers + transport router + main.go + boot wiring</name>
  <files>services/notifications/internal/handler/notification.go, services/notifications/internal/handler/internal.go, services/notifications/internal/transport/router.go, services/notifications/cmd/notifications-api/main.go</files>
  <action>
Wire the service into a runnable HTTP server.

`handler/notification.go`: `type NotificationHandler struct { svc *service.NotificationService; log *logger.Logger }`. Six handlers + JSON DTOs:
  - `List(w, r)` — parses `status` (default `unread`), `limit` (default 20, max 100), `offset` (default 0). Extracts `userID := authz.UserIDFromContext(r.Context())`; if empty → `httputil.Unauthorized(w)`. Calls `svc.List(...)`. Returns `{notifications: [...], unread_count: N, total: M}`.
  - `UnreadCount(w, r)` — returns `{unread_count: N}`.
  - `MarkRead(w, r)` — extracts `id := chi.URLParam(r, "id")`. Returns 200 OK on success, 404 on not-found.
  - `MarkAllRead(w, r)` — returns 200 OK + `{updated: N}`.
  - `Dismiss(w, r)` — same shape as MarkRead.
  - `Click(w, r)` — same shape; body is empty.

  All extract userID via `authz.UserIDFromContext(r.Context())`. **Do NOT read X-User-ID** — that header is not the project convention (D-03).

`handler/internal.go`: `type InternalHandler struct { svc *service.NotificationService; log *logger.Logger }`. Two handlers:
  - `CreateNotification(w, r)` (POST /internal/notifications) — decode `{user_id, type, dedupe_key, payload}` (payload is raw `json.RawMessage`). Calls `svc.Upsert(...)`. Returns the row.
  - `Health(w, r)` (GET /internal/health) — returns `{status: "ok"}`.

  No auth middleware. Security is by gateway-non-routing (D-05).

`transport/router.go`: mirror `services/themes/internal/transport/router.go` structure. Order:
  1. chi.NewRouter() + middleware: `RequestID`, `metricsCollector.Middleware`, `RequestLogger(log)`, `Recoverer(log)`, `CORS(["*"])`, `RealIP`.
  2. `GET /health` → 200 `{"status":"ok"}`.
  3. `GET /metrics` → `metrics.Handler()`.
  4. **Internal routes at root** (no middleware): `POST /internal/notifications` → `internalHandler.CreateNotification`. `GET /internal/health` → `internalHandler.Health`.
  5. `r.Route("/api/notifications", ...)`:
     - `r.Use(AuthMiddleware(jwtConfig))` (copy AuthMiddleware verbatim from themes router — JWT validation, populates ctx with claims).
     - `r.Get("/", h.List)`
     - `r.Get("/unread-count", h.UnreadCount)`
     - `r.Post("/mark-all-read", h.MarkAllRead)`
     - `r.Post("/{id}/read", h.MarkRead)`
     - `r.Post("/{id}/dismiss", h.Dismiss)`
     - `r.Post("/{id}/click", h.Click)`

`cmd/notifications-api/main.go`: copy `services/themes/cmd/themes-api/main.go` and replace the themes-specific guts. Sequence:
  1. `logger.Default()` + defer Sync.
  2. `config.Load()`.
  3. `database.New(cfg.Database)` + defer Close + `metrics.StartDBPoolCollector`.
  4. `db.AutoMigrate(&domain.UserNotification{}, &domain.ParserEpisodeSnapshot{})` — do NOT pass any of the read-only views.
  5. `repo.EnsureIndexes(context.Background(), db.DB)` — fatal on error.
  6. Construct repos → service → handlers (both public + internal).
  7. `metricsCollector := metrics.NewCollector("notifications")`.
  8. `router := transport.NewRouter(notifHandler, internalHandler, cfg.JWT, log, metricsCollector)`.
  9. `srv := &http.Server{Addr: cfg.Server.Address(), Handler: router, ReadTimeout: 15s, WriteTimeout: 30s, IdleTimeout: 60s}`.
  10. `go srv.ListenAndServe()` + graceful shutdown on SIGINT/SIGTERM.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/notifications &amp;&amp; go build ./... &amp;&amp; go vet ./...</automated>
  </verify>
  <done>`go build` + `go vet` are both green. main.go references EnsureIndexes after AutoMigrate. The router registers exactly the 6 public + 2 internal + 2 ops (`/health`, `/metrics`) endpoints. AuthMiddleware is applied only to `/api/notifications/*`, not to internal routes.</done>
</task>

<task type="auto">
  <name>Task 4: Gateway integration — config field + proxy method + proxy-service map + route block</name>
  <files>services/gateway/internal/config/config.go, services/gateway/internal/handler/proxy.go, services/gateway/internal/service/proxy.go, services/gateway/internal/transport/router.go</files>
  <action>
Add `/api/notifications/*` proxy in the gateway behind JWT + the existing user rate limit. Internal routes are NEVER exposed.

`config/config.go`: in `ServiceURLs` struct add `NotificationsService string`. In `Load()` (or the equivalent constructor near the existing `ThemesService: getEnv(...)` line ~line 96), add `NotificationsService: getEnv("NOTIFICATIONS_SERVICE_URL", "http://notifications:8087"),`. Update any unit-test fixture in `config_test.go` if it asserts on the URL list.

`handler/proxy.go`: append a method right after `ProxyToThemes`:
```
// ProxyToNotifications proxies requests to the notifications service
// (v1.0 Notifications Engine workstream, Phase 1 — see
// .planning/workstreams/notifications/ROADMAP.md).
func (h *ProxyHandler) ProxyToNotifications(w http.ResponseWriter, r *http.Request) {
    h.proxy(w, r, "notifications")
}
```

`service/proxy.go`: locate the function that resolves the service-name string into a URL (likely a switch or map keyed off the string passed by the handler). Add the `"notifications"` case pointing at `cfg.Services.NotificationsService`. **Verify the exact extension shape during execution** — the resolver may be a `map[string]string` populated at constructor time, or a switch in `proxy()`. Match whatever the existing code does for `"themes"` and `"library"`.

`transport/router.go`: insert a new `r.Route("/notifications", ...)` block immediately after the existing `/themes` route block (around line 337):

```
// Notifications service routes (v1.0 Notifications Engine, Phase 1)
r.Route("/notifications", func(r chi.Router) {
    r.Group(func(r chi.Router) {
        r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
        r.Use(userRateLimit)
        r.Get("/",                proxyHandler.ProxyToNotifications)
        r.Get("/unread-count",    proxyHandler.ProxyToNotifications)
        r.Post("/mark-all-read",  proxyHandler.ProxyToNotifications)
        r.Post("/{id}/read",      proxyHandler.ProxyToNotifications)
        r.Post("/{id}/dismiss",   proxyHandler.ProxyToNotifications)
        r.Post("/{id}/click",     proxyHandler.ProxyToNotifications)
    })
})
```

**Critical:** Do NOT add any route under `/internal` — internal endpoints are reachable only via Docker network on `notifications:8087` directly.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/gateway &amp;&amp; go build ./... &amp;&amp; go test ./internal/transport/... -run TestRouter -count=1</automated>
  </verify>
  <done>Gateway builds clean. The transport router_test (if present) still passes. Grep `grep -c "ProxyToNotifications" services/gateway/internal/` returns ≥ 2 (handler def + route registration).</done>
</task>

<task type="auto">
  <name>Task 5: Docker-compose + env + CLAUDE.md + seed script</name>
  <files>docker/docker-compose.yml, docker/.env.example, CLAUDE.md, scripts/seed-notification-for-ui-audit-user.sh, Makefile</files>
  <action>
Wire the new service into the runtime + document the ports/env + ship the smoke seeder.

`docker/docker-compose.yml`:
  1. Add `NOTIFICATIONS_SERVICE_URL: http://notifications:8087` to the gateway service's `environment:` block (alphabetically next to the existing `THEMES_SERVICE_URL` line).
  2. Add a complete new `notifications:` service block (mirror `themes:` block exactly). Use:
     ```
     notifications:
       build:
         context: ..
         dockerfile: services/notifications/Dockerfile
       container_name: animeenigma-notifications
       restart: unless-stopped
       environment:
         SERVER_PORT: 8087
         DB_HOST: postgres
         DB_PORT: 5432
         DB_USER: postgres
         DB_PASSWORD: postgres
         DB_NAME: animeenigma
         JWT_SECRET: ${JWT_SECRET:-dev-secret-change-in-production}
         REDIS_HOST: redis
         CATALOG_URL: http://catalog:8081
       ports:
         - "8087:8087"
       depends_on:
         - postgres
         - redis
       healthcheck:
         test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8087/health"]
         interval: 30s
         timeout: 5s
         retries: 3
     ```
     Verify the healthcheck shape matches what `make health` consumes (look at themes/library blocks — mirror the closest one).
  3. Add `notifications` to the gateway service's `depends_on:` list (under `condition: service_started`) so the gateway boots after notifications is up.

`docker/.env.example`: append `NOTIFICATIONS_SERVICE_URL=http://notifications:8087` near the other `*_SERVICE_URL` entries.

`CLAUDE.md`:
  1. **Service Ports table** — insert a new row between `themes` and `library`:
     `| notifications | 8087 | /metrics | Generic notification engine (new episodes, future types) |`
  2. **Gateway Routing section** — add a line between the themes and library entries:
     `- /api/notifications/*` → `notifications:8087` (JWT required, internal `/internal/notifications` NOT exposed)
  3. **Environment Variables section** — under a new sub-block `Notifications service specific:` add:
     ```
     CATALOG_URL                  # default http://catalog:8081 — Phase 2 detector calls catalog's /internal/anime/{id}/episodes
     ```
     and under the gateway section append `NOTIFICATIONS_SERVICE_URL # default http://notifications:8087`.

`scripts/seed-notification-for-ui-audit-user.sh`: bash script with `set -euo pipefail`, `cd "$(dirname "$0")/.."`. Steps:
  1. Resolve `ui_audit_bot`'s UUID via `docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -tAc "SELECT id FROM users WHERE username='ui_audit_bot'"` → strip whitespace.
  2. Build a JSON body for `POST /internal/notifications`:
     ```
     {
       "user_id": "<UUID>",
       "type": "new_episode",
       "dedupe_key": "new_episode:seed-anime-uuid:animelib:ru:dub:9999",
       "payload": {
         "anime_id": "seed-anime-uuid",
         "shikimori_id": "57466",
         "anime_title": "Frieren (seed)",
         "anime_poster_url": "https://shikimori.one/path/poster.jpg",
         "first_unwatched_episode": 14,
         "latest_available_episode": 16,
         "player": "animelib",
         "language": "ru",
         "watch_type": "dub",
         "translation_id": "9999",
         "translation_title": "AniLibria",
         "watch_url": "/anime/seed-anime-uuid/watch?player=animelib&episode=14&translation=9999"
       }
     }
     ```
  3. POST via `docker compose -f docker/docker-compose.yml exec -T notifications wget -qO- --post-data="$BODY" --header='Content-Type: application/json' http://localhost:8087/internal/notifications`.
  4. Echo `Seeded notification for ui_audit_bot. Re-running this script will UPSERT (no duplicate).`.

`Makefile`: inspect the `redeploy-%` wildcard target (line 281) — if it shells `docker compose -f docker/docker-compose.yml up -d --build $*` unconditionally, no edit needed. If it has a hardcoded service whitelist, add `notifications`. Same check for `restart-%` (line ~280s) and `logs-%`. Verify by running `make help` after the docker-compose edit and confirming `make redeploy-notifications` is implicitly available.
  </action>
  <verify>
    <automated>docker compose -f docker/docker-compose.yml config --services | grep -c '^notifications$' &amp;&amp; grep -c 'notifications | 8087' CLAUDE.md &amp;&amp; bash -n scripts/seed-notification-for-ui-audit-user.sh</automated>
  </verify>
  <done>`docker compose config --services` lists `notifications`. CLAUDE.md grep finds the new row. The seed script has no bash syntax errors (`bash -n`). The `.env.example` contains `NOTIFICATIONS_SERVICE_URL`.</done>
</task>

<task type="auto">
  <name>Task 6: End-to-end smoke — redeploy, verify health, run seed, verify CRUD, verify internal isolation</name>
  <files>(no source edits — verification-only task)</files>
  <action>
Run the full success-criteria gauntlet against the live containers. **This task is the gate** — if any sub-step fails, fix the regression and re-run.

Steps (each must pass before moving to the next):
  1. `make redeploy-gateway` (picks up the new env + route block).
  2. `make redeploy-notifications` (builds + starts the new service).
  3. Wait ~5s for warmup, then `make health` — must include a healthy `notifications:8087` line.
  4. `curl -s http://localhost:8087/health | jq` → `{"status":"ok"}`.
  5. `curl -s -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications | jq` → `{"notifications":[],"unread_count":0,"total":0}`.
  6. Verify both tables exist + both partial indexes exist:
     ```
     docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "\d user_notifications" | grep -E "uk_user_dedupe|idx_user_unread"
     docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "\dt user_notifications parser_episode_snapshots"
     ```
     Both should show 2 matching lines.
  7. Run `./scripts/seed-notification-for-ui-audit-user.sh` → confirms HTTP 200 from the wget.
  8. Re-curl the public list — must show 1 row with the expected `new_episode` payload.
  9. `NOTIF_ID=$(curl -s -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications | jq -r '.notifications[0].id')` then `curl -X POST -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications/$NOTIF_ID/dismiss`.
  10. Re-curl unread-count → `{"unread_count":0}`.
  11. Re-run the seed script → no error, fresh row created (dismissed → new) per design-doc rule.
  12. Re-run the seed script AGAIN immediately (no dismiss in between) → SELECT COUNT(*) FROM user_notifications WHERE user_id=... AND dedupe_key=... AND dismissed_at IS NULL stays at 1 (UPSERT, not INSERT).
  13. **Internal-isolation check** — `curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8000/internal/notifications` must return `404`. `docker compose -f docker/docker-compose.yml exec -T notifications wget -qO- --post-data='...' --header='Content-Type: application/json' http://localhost:8087/internal/notifications` must return 200.

Document the actual outputs in the phase SUMMARY at the end. If something deviates, fix-forward in the same task (no separate gap-closure plan needed for trivial bugs surfaced here).
  </action>
  <verify>
    <automated>set -euo pipefail; curl -fsS http://localhost:8087/health > /dev/null; curl -fsS -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications > /dev/null; ./scripts/seed-notification-for-ui-audit-user.sh; UNREAD=$(curl -fsS -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications/unread-count | jq -r .unread_count); test "$UNREAD" -ge 1; CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8000/internal/notifications); test "$CODE" = "404"</automated>
  </verify>
  <done>All 13 sub-steps pass. The notifications service is live on 8087, the gateway proxies `/api/notifications/*` under JWT auth, both tables + partial indexes exist, the seed script + dismiss + re-seed cycle behaves per spec, internal routes are unreachable from the host. SUMMARY.md captures actual curl outputs.</done>
</task>

</tasks>

<verification>

## Verification matrix — ROADMAP.md Phase 1 success criteria → exact runner commands

(The reviewer runs these. The Task 6 automated gauntlet covers the same surface end-to-end, but this matrix is for human verification post-merge.)

### SC1: `make redeploy-notifications` builds clean; `make health` shows healthy

```bash
make redeploy-notifications
sleep 5
make health | grep -i "notifications" | grep -i "healthy"
# Expect: a line like "notifications:8087 ... healthy" (or whatever shape make health emits)
```

### SC2: Health + gateway-proxy CRUD both work

```bash
curl -fsS http://localhost:8087/health | jq
# Expect: {"status":"ok"}

curl -fsS -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications | jq
# Expect: {"notifications":[],"unread_count":0,"total":0}  (if seeded already, list is non-empty but valid JSON)
```

### SC3: Both tables + both partial indexes exist in `animeenigma` Postgres DB

```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma \
  -c "\dt user_notifications parser_episode_snapshots"
# Expect: exactly 2 rows, both in 'public' schema

docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma \
  -c "\d user_notifications" | grep -E "uk_user_dedupe|idx_user_unread"
# Expect: 2 matching lines, both showing "WHERE (dismissed_at IS NULL)"
```

### SC4: Seed → list → dismiss → list cycle

```bash
./scripts/seed-notification-for-ui-audit-user.sh
# Expect: prints success line, no errors

curl -fsS -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications | jq '.notifications[0].payload'
# Expect: full new_episode payload (anime_title, first_unwatched_episode, latest_available_episode, player, etc.)

NOTIF_ID=$(curl -fsS -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications | jq -r '.notifications[0].id')
curl -fsS -X POST -H "Authorization: Bearer $UI_AUDIT_API_KEY" "http://localhost:8000/api/notifications/$NOTIF_ID/dismiss"
curl -fsS -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/notifications/unread-count
# Expect: {"unread_count":0}
```

### SC5: Internal route NOT reachable via gateway, IS reachable via Docker network

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8000/internal/notifications
# Expect: 404

docker compose -f docker/docker-compose.yml exec -T notifications \
  wget -qO- http://localhost:8087/internal/health
# Expect: {"status":"ok"}
```

### SC6: UPSERT idempotency on same (user_id, dedupe_key)

```bash
./scripts/seed-notification-for-ui-audit-user.sh
./scripts/seed-notification-for-ui-audit-user.sh
./scripts/seed-notification-for-ui-audit-user.sh

docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -tAc "
  SELECT COUNT(*) FROM user_notifications
  WHERE user_id = (SELECT id FROM users WHERE username='ui_audit_bot')
    AND dedupe_key = 'new_episode:seed-anime-uuid:animelib:ru:dub:9999'
    AND dismissed_at IS NULL
"
# Expect: 1
```

</verification>

<threat_model>

## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Internet → gateway:8000 | Anonymous + authenticated user traffic; rate-limited per-IP and per-user. |
| Gateway → notifications:8087 (`/api/*`) | JWT-validated by gateway, re-validated by notifications service `AuthMiddleware`. |
| Docker network → notifications:8087 (`/internal/*`) | Trusted producers only (Phase 2 detector). NO public exposure. |
| Notifications service → Postgres | Shared `animeenigma` DB, single connection, read-write on the two service-owned tables + read-only on the three views. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-01-01 | S (Spoofing) | `/api/notifications/*` | mitigate | Gateway `JWTValidationMiddleware` + service-side `AuthMiddleware` double-validate the JWT; `userID` is pulled from claims, never from a request header or body parameter. |
| T-01-02 | S | `/internal/notifications` | mitigate | Gateway never proxies `/internal/*`. Service binds 8087 on a `127.0.0.1:8087:8087` mapping (verify the compose port shape and lock to localhost-only on the host; the internal Docker-network address remains free). Reachable only from inside the Docker network. |
| T-01-03 | T (Tampering) | UPSERT path `(user_id, dedupe_key)` | mitigate | Unique partial index `uk_user_dedupe` is enforced by Postgres — a producer cannot create duplicate active rows even by racing. |
| T-01-04 | R (Repudiation) | `read_at`, `dismissed_at`, `clicked_at` | accept | Telemetry-only; no business-critical audit needed in v1.0. These are user-visible state, not auth events. |
| T-01-05 | I (Information Disclosure) | `GET /api/notifications/{id}` (via list query) | mitigate | All queries filter on `user_id = ?` from claims; cross-user reads return 404 (NOT 403) to avoid existence-leak via id enumeration. |
| T-01-06 | D (Denial of Service) | `/api/notifications/*` polling at 60s by frontend | mitigate | Gateway already enforces per-user GCRA rate limit (60/min, burst 10) — see CLAUDE.md `USER_RATE_LIMIT_*`. A foregrounded tab polling once per minute is well inside that envelope. The notifications service exposes `/metrics` so `notifications_active_unread_gauge` (Phase 2 NF-01) can detect anomalous client behavior. |
| T-01-07 | D | `/internal/notifications` flood from a buggy detector | accept | Single-instance detector by design (NOTIF-DET-03); a runaway loop would manifest as a `payload`-equal UPSERT no-op storm — slow but not destructive. Phase 2 will add the `notifications_created_total{type,producer}` counter to surface this. |
| T-01-08 | E (Elevation of Privilege) | Bearer token on `/api/notifications/*` belonging to user A used to mark user B's notification as read | mitigate | All UPDATE statements include `WHERE user_id = ?` from claims; a mismatched id returns NotFound. |

</threat_model>

<risks_and_mitigations>

### R1: AutoMigrate + partial indexes race on first boot of multi-replica deployment

**Risk:** If two notifications containers boot simultaneously, both may attempt `AutoMigrate` + `EnsureIndexes` at the same time. Postgres serializes DDL transactions, but a partial-index `CREATE UNIQUE INDEX IF NOT EXISTS` is technically not atomic with the table create (table CREATE first, then index).

**Mitigation:** `IF NOT EXISTS` on both index statements makes them idempotent. Even if both replicas race, the second one's `CREATE INDEX IF NOT EXISTS` is a no-op. AutoMigrate itself is safe because GORM checks `Migrator().HasTable()` before creating. **Accept** the tiny window of duplicate work — outcome is correct.

### R2: Port 8087 collision

**Risk:** Some other process on the host might bind 8087.

**Mitigation:** Verified `grep -rn ':8087' docker/docker-compose.yml services/` returns no current bindings. Documenting the port in CLAUDE.md Service Ports table makes future collisions easy to catch in code review.

### R3: GORM AutoMigrate doesn't handle `jsonb NOT NULL DEFAULT '{}'` cleanly

**Risk:** GORM's `AutoMigrate` for a `datatypes.JSON` field tagged `not null` may emit `ALTER TABLE ... ADD COLUMN payload jsonb NOT NULL` without a default, which Postgres rejects if rows exist.

**Mitigation:** First-time table creation has no existing rows, so this is a no-op risk on a fresh DB. For idempotent re-boots, `AutoMigrate`'s `HasColumn` check skips re-add. If we ever need to add a NOT NULL column to an existing populated table, that requires a manual `ALTER` — but that's Phase N+ scope and not Phase 1's concern.

### R4: gateway `proxy.go` service-resolver may be a switch (not a map)

**Risk:** Task 4's assumption that `service/proxy.go` resolves `"notifications"` → URL via a map may be wrong; it could be a hardcoded switch with no extension point.

**Mitigation:** Task 4's action explicitly says "Verify the exact extension shape during execution — match whatever the existing code does for `themes` and `library`." If it's a switch, the executor adds a new `case "notifications":` arm. Either shape is a 1-line edit.

### R5: `Makefile` `redeploy-%` wildcard may be gated by a whitelist

**Risk:** If `redeploy-%` is implemented as `redeploy-%: docker compose ... up -d --build $*` it's fully wildcarded; if it's `redeploy-%: $(if $(filter $*,gateway catalog ...),docker compose ...)` it's whitelisted.

**Mitigation:** Task 5 explicitly inspects the rule before declaring "no Makefile change needed." If whitelisted, the executor adds `notifications` to the whitelist. Same for `restart-%` and `logs-%`.

### R6: `chi` URL-param routing precedence between `/api/notifications/mark-all-read` and `/api/notifications/{id}/...`

**Risk:** chi-go-chi is strict about literal-vs-param route precedence; declaring `Post("/{id}/dismiss", ...)` before `Post("/mark-all-read", ...)` could shadow the literal route at the `/mark-all-read` URL (parsing `mark-all-read` as `{id}` with no trailing `/dismiss`).

**Mitigation:** Two-fold. (1) The literal `mark-all-read` is at depth 1, while `{id}/dismiss` is at depth 2 — chi differentiates by depth, so this is not actually a collision. (2) To be safe, Task 3's router registration order puts literal routes BEFORE param routes: `/`, `/unread-count`, `/mark-all-read` first, then `/{id}/read`, `/{id}/dismiss`, `/{id}/click`. Task 6 SC4 implicitly verifies by exercising both.

### R7: `gorm.io/datatypes` version compatibility with `gorm.io/gorm v1.30.0`

**Risk:** `datatypes` is versioned semi-independently of `gorm`. A mismatched version could fail to compile.

**Mitigation:** Task 1's `go get gorm.io/datatypes` resolves the latest compatible version automatically via the module graph. If `go build` fails in Task 1's verify step, the executor pins to a known-good version (`gorm.io/datatypes@v1.2.x` is the standard pairing with gorm v1.30.x).

</risks_and_mitigations>

<rollback>

If Phase 1 ships and is broken, the clean rollback is fully additive — nothing else in the codebase depends on the new service yet:

1. **Stop + remove the container:**
   ```bash
   docker compose -f docker/docker-compose.yml stop notifications
   docker compose -f docker/docker-compose.yml rm -f notifications
   ```

2. **Revert the gateway proxy:** revert the `r.Route("/notifications", ...)` block in `services/gateway/internal/transport/router.go`, the `NotificationsService` field in `services/gateway/internal/config/config.go`, the `ProxyToNotifications` method in `services/gateway/internal/handler/proxy.go`, and the service-resolver map/switch arm in `services/gateway/internal/service/proxy.go`. `make redeploy-gateway` to ship.

3. **Revert docker-compose:** delete the `notifications:` service block, the `NOTIFICATIONS_SERVICE_URL` line in the gateway's `environment:`, and the `notifications` entry in the gateway's `depends_on:`. Delete `NOTIFICATIONS_SERVICE_URL` from `docker/.env.example`.

4. **Revert CLAUDE.md edits:** drop the Service Ports row, the Gateway Routing line, and the env-var notes.

5. **Optional — drop the tables:**
   ```bash
   docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "
     DROP TABLE IF EXISTS user_notifications CASCADE;
     DROP TABLE IF EXISTS parser_episode_snapshots CASCADE;
   "
   ```
   (Safe — no other service writes to these tables. The read-only views in `repo/views.go` don't drop the source tables.)

6. **Optional — remove `services/notifications/` from `go.work`** and the directory itself.

Total rollback time: under 5 minutes. No data migration needed, no consumer impact, no frontend touched.

</rollback>

<score>

Per project convention (`.planning/CONVENTIONS.md` — no days/hours/sprints):

- **UXΔ:** **0 (Ambiguous)** — Phase 1 ships zero user-visible change. The bell/toast/dropdown all arrive in Phase 3. This phase only enables future UXΔ; on its own it is invisible to end users (and that's correct — vertical-slice phasing intentionally puts the infrastructure spine before the UI).

- **CDI:** `0.05 × 5` —
  - **Spread:** 0.05 — narrow (one new service + one gateway block + one CLAUDE.md row + one seed script; no existing-table mutations, no frontend, no cross-service refactor).
  - **Shift:** low (pure-additive — one new docker-compose service, one new gateway route, two new tables in a shared DB; no schema mutations on existing tables, no API breakage, no Vue components touched). Shift folded into the 0.05 spread number.
  - **Effort_Fib:** **5** — six tasks, mostly mechanical cloning of `services/themes/`. The partial-index helper + UPSERT clause + read-only-view design are the only non-trivial bits, and all three are spec'd in the design doc. 5 (not 3) because the cross-DB-pattern decision (D-01) required real codebase investigation and the gateway proxy-service map shape is unverified-until-execution (R4).

- **MVQ:** **Sprite 78%/90%** —
  - **Sprite** (small, deft, gets out of the way) is the right shape for Phase 1: a quiet scaffold that nothing else depends on yet. Not a Griffin yet — that's Phase 3 (visible UX layered on top). Not a Kraken — no sprawling tentacles, just a clean isolated module.
  - **78% match** — could be a stronger Sprite if it didn't have the cross-cutting CLAUDE.md + docker-compose touches; those drag it slightly toward Griffin territory.
  - **90% slop-resistance** — extremely high. The dedupe-key partial unique index, UPSERT semantics, double-JWT-validation, internal-isolation-by-gateway-non-routing, and read-only views all make accidental data corruption or auth bypass mechanically very hard. The only weak surface is the manual `go.work` + `Dockerfile` `COPY libs/*` editing — easy to miss a libs/ when adding a new one (mitigated by the "Adding new libs/ module" MEMORY.md checklist, which we explicitly follow in Task 1).

</score>

<definition_of_done>

- [ ] **SC1** — `make redeploy-notifications` builds + starts clean; `make health` shows the new service healthy.
- [ ] **SC2** — `curl /health` on 8087 and `curl /api/notifications` through 8000 (with `UI_AUDIT_API_KEY`) both return 200 + expected JSON.
- [ ] **SC3** — Both tables exist in the shared `animeenigma` DB; both partial indexes (`uk_user_dedupe`, `idx_user_unread`) exist with the `WHERE dismissed_at IS NULL` predicate.
- [ ] **SC4** — Seed script inserts; list shows the row; dismiss removes it from unread; re-seed re-fires (new row, since previous was dismissed).
- [ ] **SC5** — `/internal/notifications` returns 404 through the gateway from the host; same path returns 200 from inside the Docker network via `docker compose exec`.
- [ ] **SC6** — Re-running the seed without dismissing in between leaves the active-row count at exactly 1 (UPSERT, not INSERT).
- [ ] CLAUDE.md updated with Service Ports row + Gateway Routing line + env-var documentation.
- [ ] `scripts/seed-notification-for-ui-audit-user.sh` is executable (`chmod +x`) and idempotent.
- [ ] `go build ./...` is green across `services/notifications/` and `services/gateway/`.
- [ ] Phase 1 SUMMARY.md captures the actual curl outputs from Task 6 SC1-SC6.
- [ ] Phase 1 commits include co-authors per project convention; `make redeploy-gateway` + `make redeploy-notifications` shipped at least once after the final commit; user explicitly says "ship it" (or invokes `/animeenigma-after-update`) before the phase is closed.

</definition_of_done>

<success_criteria>
This plan succeeds when all 6 ROADMAP.md Phase 1 success criteria pass via the verification matrix above (also re-asserted in Task 6's automated gate). Phase 1 outcome: a notifications microservice on port 8087 with full CRUD HTTP API, an internal UPSERT producer endpoint, two database tables with the correct partial indexes, read-only cross-service views ready for Phase 2's detector, gateway routing under JWT, and a working smoke seeder. No detector. No cron. No frontend.
</success_criteria>

<output>
After completion, create `.planning/workstreams/notifications/phases/01-notifications-foundation/01-SUMMARY.md` capturing:
- Each task's actual outcome (commit SHA, files touched, surprises)
- Task 6 SC1-SC6 actual curl outputs (verbatim — for the Phase 3 frontend developer to crib from)
- Any deferred decisions or open questions surfaced for Phase 2
- Cross-references to the design doc, REQUIREMENTS.md NOTIF-FOUND-* IDs, and CLAUDE.md edits
- The literal text of the new CLAUDE.md row + the new docker-compose service block (so a future operator can quickly re-derive what was added)
</output>
