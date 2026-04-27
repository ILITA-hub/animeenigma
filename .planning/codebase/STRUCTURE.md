# Codebase Structure

**Analysis Date:** 2026-04-27

## Directory Layout

```
animeenigma/
├── api/                          # OpenAPI, GraphQL, protobuf schemas
│   ├── events/
│   ├── graphql/
│   ├── openapi/
│   └── proto/
├── bin/                          # Compiled binaries and build output
├── deploy/                       # Kubernetes / deployment configs
│   ├── kustomize/               # Kubernetes manifests (base + overlays)
│   ├── maintenance/
│   └── scripts/
├── docker/                       # Docker Compose for local dev
│   ├── docker-compose.yml
│   ├── docker-compose.prod.yml
│   ├── .env                      # Local secrets (not committed)
│   ├── nginx/                    # Nginx reverse proxy config
│   ├── postgres/                 # PostgreSQL init scripts
│   ├── prometheus/               # Prometheus config
│   ├── grafana/                  # Grafana dashboards
│   ├── loki/                     # Loki log aggregation
│   ├── pgadmin/                  # PostgreSQL admin UI
│   └── promtail/                 # Log shipper to Loki
├── docs/                         # Project documentation
│   └── issues/                   # Issues & incident tracking
├── frontend/                     # Vue 3 single-page application
│   └── web/
│       ├── src/
│       │   ├── api/              # axios client (client.ts)
│       │   ├── components/       # Vue components
│       │   │   ├── anime/        # Anime card, detail, list comps
│       │   │   ├── carousel/     # Carousel component
│       │   │   ├── hero/         # Hero image components
│       │   │   ├── layout/       # Layout wrappers
│       │   │   ├── player/       # 4 video player components + SubtitleOverlay
│       │   │   ├── themes/       # Theme components
│       │   │   └── ui/           # Basic UI (buttons, modals, etc.)
│       │   ├── composables/      # Vue hooks (useAnime, useAuth, etc.)
│       │   ├── locales/          # i18n translations (ru, en)
│       │   ├── router/           # Vue Router (index.ts)
│       │   ├── stores/           # Pinia stores (auth.ts, home.ts, watchlist.ts)
│       │   ├── styles/           # Tailwind CSS + global styles
│       │   ├── types/            # TypeScript interfaces (anime, user, etc.)
│       │   ├── utils/            # Helper functions (diagnostics, subtitle-parser)
│       │   ├── views/            # Page components (Home, Browse, Anime, Profile, etc.)
│       │   ├── App.vue           # Root component
│       │   ├── main.ts           # Entry point
│       │   ├── i18n.ts           # i18n config
│       │   └── vite-env.d.ts     # Vite environment types
│       ├── public/               # Static assets (logo, favicon, changelog.json)
│       ├── index.html            # HTML shell
│       ├── package.json          # Frontend dependencies
│       ├── vite.config.ts        # Vite config
│       ├── tsconfig.json         # TypeScript config
│       └── playwright.config.ts  # E2E test config
├── infra/                        # Infrastructure as Code (Terraform, Ansible, etc.)
├── libs/                         # Shared Go libraries (modules)
│   ├── animeparser/             # Title normalization, video source models
│   ├── authz/                   # Role-based access control
│   ├── cache/                   # Redis wrapper with TTL strategies
│   ├── database/                # GORM PostgreSQL wrapper
│   ├── errors/                  # Domain error types (NotFound, BadRequest, etc.)
│   ├── httputil/                # HTTP middleware, CORS, error responses
│   ├── idmapping/               # ARM client (Shikimori ↔ AniList ID mapping)
│   ├── logger/                  # Structured logging (zap + OpenTelemetry)
│   ├── metrics/                 # Prometheus metrics collection
│   ├── pagination/              # Pagination helpers
│   ├── tracing/                 # OpenTelemetry tracing
│   └── videoutils/              # HLS/MP4 proxy, MinIO client
├── scripts/                      # Helper scripts (db migrations, seed data, etc.)
├── services/                     # Microservices (Go)
│   ├── auth/                    # User registration, login, JWT, API keys
│   │   ├── cmd/auth-api/        # main.go (entry point)
│   │   ├── internal/
│   │   │   ├── config/          # Config loading
│   │   │   ├── domain/          # User struct
│   │   │   ├── handler/         # HTTP handlers (register, login, token, api-key)
│   │   │   ├── repo/            # Database queries
│   │   │   ├── service/         # Business logic
│   │   │   └── transport/       # Router setup
│   │   ├── migrations/          # SQL migrations (auto-created by GORM)
│   │   ├── Dockerfile
│   │   └── go.mod
│   ├── catalog/                 # Anime search, Shikimori API, video source parsers
│   │   ├── cmd/catalog-api/     # main.go (entry point)
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── domain/          # Anime, Video, Genre structs
│   │   │   ├── handler/         # Search, browse, details, admin, news
│   │   │   ├── parser/          # External API clients
│   │   │   │   ├── kodik/       # Kodik embed parser
│   │   │   │   ├── animelib/    # AnimeLib MP4 parser
│   │   │   │   ├── hianime/     # HiAnime HLS parser
│   │   │   │   ├── consumet/    # Consumet HLS parser
│   │   │   │   ├── jimaku/      # Jimaku.cc subtitle fetcher
│   │   │   │   ├── shikimori/   # Shikimori GraphQL client
│   │   │   │   ├── telegram/    # Telegram bot integration
│   │   │   │   ├── jikan/       # Jikan API (alt metadata source)
│   │   │   │   ├── hanime/      # HAnime provider
│   │   │   │   └── aniboom/     # AniBoom provider
│   │   │   ├── repo/            # Anime, video, genre queries
│   │   │   ├── service/         # Search, browse, parser coordination
│   │   │   └── transport/       # Router
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   └── go.mod
│   ├── streaming/               # HLS proxy, MinIO uploads
│   │   ├── cmd/streaming-api/   # main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── handler/         # Proxy, MinIO upload/download
│   │   │   ├── service/         # Stream validation
│   │   │   └── transport/       # Router
│   │   ├── Dockerfile
│   │   └── go.mod
│   ├── player/                  # Watch progress, watchlists, ratings
│   │   ├── cmd/player-api/      # main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── domain/          # WatchProgress, WatchListItem, UserRating
│   │   │   ├── handler/         # Progress, watchlist, rating endpoints + report button
│   │   │   ├── repo/
│   │   │   ├── service/
│   │   │   └── transport/
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   └── go.mod
│   ├── rooms/                   # Game room WebSocket
│   │   ├── cmd/rooms-api/       # main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── domain/          # Room, Player structs
│   │   │   ├── handler/         # WebSocket handler
│   │   │   ├── service/         # Room logic
│   │   │   └── transport/
│   │   ├── Dockerfile
│   │   └── go.mod
│   ├── scheduler/               # Background jobs, cron tasks
│   │   ├── cmd/scheduler-api/   # main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── domain/
│   │   │   ├── handler/
│   │   │   ├── jobs/            # Cron job implementations
│   │   │   ├── repo/
│   │   │   ├── service/
│   │   │   └── transport/
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   └── go.mod
│   ├── themes/                  # Anime OP/ED ratings
│   │   ├── cmd/themes-api/      # main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── domain/          # ThemeRating struct
│   │   │   ├── handler/         # Get/save ratings
│   │   │   ├── parser/          # External OP/ED source parsers
│   │   │   ├── repo/
│   │   │   ├── service/
│   │   │   └── transport/
│   │   ├── Dockerfile
│   │   └── go.mod
│   ├── gateway/                 # API gateway (router + auth proxy)
│   │   ├── cmd/gateway-api/     # main.go
│   │   ├── internal/
│   │   │   ├── config/          # Service URLs config
│   │   │   ├── handler/         # HTTP proxy handler
│   │   │   ├── service/         # ProxyService (routes requests)
│   │   │   └── transport/       # Router
│   │   ├── Dockerfile
│   │   └── go.mod
│   └── maintenance/             # Admin tools (data export, health checks)
│       ├── cmd/
│       ├── Dockerfile
│       └── go.mod
├── .claude/                      # Claude AI integration notes
├── .planning/                    # Generated planning documents
│   └── codebase/                # ARCHITECTURE.md, STRUCTURE.md, etc.
├── .github/                      # GitHub Actions CI/CD
├── .editorconfig                 # Editor config (indentation, line endings)
├── .golangci.yml                 # Go linter config
├── .gitignore
├── .tool-versions                # asdf tool versions (Go, Node, etc.)
├── go.work                       # Go workspace (links all libs + services)
├── Makefile                      # Local dev commands (make dev, make redeploy-*, etc.)
├── CLAUDE.md                     # Project guidelines for AI assistants
├── README.md                     # English project overview
├── README.ru.md                  # Russian project overview
└── LICENSE
```

## Directory Purposes

**api/:**
- OpenAPI schema definitions for API contract
- GraphQL schema for potential future queries
- Protobuf definitions (future: gRPC, if needed)

**bin/:**
- Compiled Go binaries after `make build`
- Not committed to git

**deploy/kustomize/:**
- Kubernetes manifests organized by component
- `base/` contains all resource definitions
- Includes ConfigMaps, Secrets, Services, Deployments, Ingress rules, monitoring stack (Prometheus, Grafana, Loki)

**docker/:**
- `docker-compose.yml` — Development environment (all services + postgres + redis + nginx + monitoring)
- `.env` — Local secrets (JIMAKU_API_KEY, UI_AUDIT_API_KEY, etc. — not committed)
- `nginx/` — Reverse proxy config (routes :80 → frontend, :8000 → gateway)
- Infrastructure service configs (postgres, prometheus, grafana, loki, promtail)

**docs/:**
- Project documentation
- `issues/` — Issue tracking + UI audit reports (docs/issues/ui-audit-YYYY-MM-DD.md)

**frontend/web/:**
- Complete Vue 3 SPA
- `src/` contains all application code
- `public/` serves static assets (logo, changelog.json for LastUpdates.vue)
- `vite.config.ts` handles build, dev server, compression

**infra/:**
- Infrastructure-as-Code (future: Terraform, Ansible playbooks)

**libs/:**
- Shared Go modules, each a standalone importable package
- Managed by `go.work` for local development (import across libs without `go get`)
- Deployed to git and imported by services via semantic versioning (future: push to registry)

**scripts/:**
- `seed-ui-audit-user.sh` — Create/re-seed ui_audit_bot test account
- Database migration helpers
- Backup scripts

**services/:**
- 9 independent microservices, each with its own Dockerfile, go.mod, migrations
- Standard structure: `cmd/{service}-api/main.go` → `internal/{config,domain,handler,service,repo,transport}` → `Dockerfile` + `go.mod`
- All share same Go version (1.22) and dependency patterns

## Key File Locations

**Entry Points:**

| Service | Entry Point | Port |
|---------|-------------|------|
| Auth | `services/auth/cmd/auth-api/main.go` | 8080 |
| Catalog | `services/catalog/cmd/catalog-api/main.go` | 8081 |
| Streaming | `services/streaming/cmd/streaming-api/main.go` | 8082 |
| Player | `services/player/cmd/player-api/main.go` | 8083 |
| Rooms | `services/rooms/cmd/rooms-api/main.go` | 8084 |
| Scheduler | `services/scheduler/cmd/scheduler-api/main.go` | 8085 |
| Themes | `services/themes/cmd/themes-api/main.go` | 8086 |
| Gateway | `services/gateway/cmd/gateway-api/main.go` | 8000 |
| Frontend | `frontend/web/src/main.ts` | 80 (via nginx) |

**Configuration:**
- Service configs: `services/{name}/internal/config/config.go` (env var parsing)
- Frontend config: `frontend/web/src/i18n.ts` (i18n setup), `frontend/web/vite.config.ts` (build)
- Docker: `docker/docker-compose.yml` (dev), `docker/docker-compose.prod.yml` (prod)
- Kubernetes: `deploy/kustomize/base/` (manifests)

**Core Logic:**
- Catalog search: `services/catalog/internal/service/catalog.go` (SearchAnime, GetTrendingAnime, etc.)
- Auth: `services/auth/internal/service/auth.go` (RegisterUser, Login, GenerateToken, etc.)
- Video parsers: `services/catalog/internal/parser/{kodik,animelib,hianime,consumet}/` (each implements AnimeParser interface)
- Watch progress: `services/player/internal/service/` (UpdateWatchProgress, GetWatchHistory, etc.)
- Frontend stores: `frontend/web/src/stores/{auth,home,watchlist}.ts` (reactive state)

**Testing:**
- Backend: `services/{name}/cmd/{name}-api/main_test.go` or `internal/*/test.go` files
- Frontend: `frontend/web/playwright.config.ts` + `frontend/web/tests/` (e2e only, no unit tests)

## Naming Conventions

**Files:**
- Go: `snake_case.go` (e.g., `anime_parser.go`, `watch_progress.go`)
- Vue: `PascalCase.vue` (e.g., `KodikPlayer.vue`, `SubtitleOverlay.vue`)
- TypeScript: `camelCase.ts` (e.g., `subtitle-parser.ts`, `diagnostics.ts`)
- Bash: `snake_case.sh` (e.g., `seed-ui-audit-user.sh`)

**Directories:**
- Go packages: `lowercase` (e.g., `handler`, `service`, `repo`)
- Vue components: Group by domain (e.g., `components/anime/`, `components/player/`)
- Routes: Kebab-case paths (e.g., `/anime`, `/user/:id`, `/game/:roomId`)

**Variables / Functions:**
- Go exported: `PascalCase` (e.g., `SearchAnime()`, `AnimeService`)
- Go private: `camelCase` (e.g., `parseFilters()`, `animeID`)
- TypeScript: `camelCase` (e.g., `activeCues`, `updateBaseFontSize()`)
- Constants: `ALL_CAPS` (e.g., `TTLAnimeDetails`, `SEARCH_PAGE_SIZE`)

**Types:**
- Go structs: `PascalCase` (e.g., `type Anime struct`, `type VideoSource struct`)
- Vue props: `camelCase` (e.g., `videoElement`, `subtitleUrl`)
- TypeScript interfaces: `PascalCase` (e.g., `SubtitleCue`, `AnimeFilterOptions`)

## Where to Add New Code

**New Feature (e.g., anime recommendations):**
- **Backend logic:** `services/catalog/internal/service/catalog.go` (new function `GetRecommendations()`)
- **Database model:** `services/catalog/internal/domain/recommendation.go` (struct with GORM tags)
- **Repository:** `services/catalog/internal/repo/recommendation.go` (DB queries)
- **HTTP handler:** `services/catalog/internal/handler/catalog.go` (new method `GetRecommendations(w, r)`)
- **Router:** `services/catalog/internal/transport/router.go` (add route `GET /api/anime/:id/recommendations`)
- **Frontend:** `frontend/web/src/views/Anime.vue` (new section in detail page) + `frontend/web/src/api/client.ts` (axios call)

**New Video Source Provider (e.g., "MyAnimeSource"):**
- Create `services/catalog/internal/parser/myanimesource/` directory
- Implement `AnimeParser` interface in `client.go`
- Add config env vars (e.g., `MYANIMESOURCE_API_KEY`)
- Register in `services/catalog/cmd/catalog-api/main.go` parser registry
- Create `services/catalog/internal/handler/` handler if admin endpoints needed
- Frontend: No changes (player selection auto-detects source type)

**New Component/Module (e.g., user recommendations, activity feed):**
- **Backend:** New service in `services/{feature}/` with standard structure
- **Database:** Migration via GORM AutoMigrate in main.go
- **Frontend:** New Vue components in `frontend/web/src/components/{feature}/` + new view in `frontend/web/src/views/{Feature}.vue` if top-level page
- **Store:** If needs state, add to Pinia in `frontend/web/src/stores/{feature}.ts`
- **Route:** Add to `frontend/web/src/router/index.ts`

**Utilities / Helpers:**
- Go: `libs/{category}/{utility}.go` (e.g., `libs/cache/key_generator.go`)
- Vue: `frontend/web/src/utils/{utility}.ts` (e.g., `subtitle-parser.ts`, `diagnostics.ts`)
- Composables: `frontend/web/src/composables/use{Feature}.ts` (e.g., `useAnime.ts`, `useAuth.ts`)

**Tests:**
- Go: Unit tests alongside source (`anime_test.go` next to `anime.go`); integration tests in dedicated `*_integration_test.go`
- Vue: E2E tests in `frontend/web/tests/` (Playwright only, no unit tests currently)

## Special Directories

**docker/:**
- Purpose: Local development environment (docker-compose)
- Generated: No — committed to repo
- Committed: Yes, except `.env` (contains secrets)

**bin/:**
- Purpose: Compiled binaries output
- Generated: Yes (by `make build`)
- Committed: No (in .gitignore)

**node_modules/ (frontend/web):**
- Purpose: npm/bun dependencies
- Generated: Yes (by `bun install`)
- Committed: No (in .gitignore)

**.planning/codebase/:**
- Purpose: Auto-generated codebase analysis documents (ARCHITECTURE.md, STRUCTURE.md, etc.)
- Generated: Yes (by `/gsd-map-codebase` orchestrator)
- Committed: Yes (reference material)

---

*Structure analysis: 2026-04-27*
