# Technology Stack

**Analysis Date:** 2026-04-27

## Languages

**Primary:**
- Go 1.22 - Backend microservices, API servers, parsers, video proxy
- TypeScript 5.4.2 - Frontend Vue 3 components, utilities, stores
- Vue 3 3.4.21 - Frontend UI framework

**Secondary:**
- Shell - Deployment scripts, Makefile targets
- SQL - Database migrations and queries via GORM
- YAML - Configuration (docker-compose, Kubernetes manifests, Prometheus/Loki)
- Protocol Buffers 25.0 - Service API contracts (`api/proto/`)

## Runtime

**Environment:**
- Go 1.22 runtime (Alpine Linux in Docker — see `services/*/Dockerfile`)
- Node.js 20.11.0 (runtime support for build tooling, frontend dev)
- Kubernetes 1.29.0 (optional orchestration layer)

**Package Manager:**
- Go: go.work workspace for monorepo (`go.work` — lists all 7 services + 13 libs)
- Frontend: Bun 1.x (not npm/pnpm — enforced per CLAUDE.md)
- Lockfiles: `go.mod`/`go.sum` per service, `bun.lock` for frontend

## Frameworks

**Core Backend:**
- Chi v5.0.12 - HTTP router used across all services (`services/*/go.mod`)
- GORM 1.30.0 - ORM for database operations (`libs/database/go.mod`)
- Redis Go Client v9.5.1 - Caching via `libs/cache` (`libs/cache/go.mod`)
- Gorilla WebSocket v1.5.1 - WebSocket support for rooms service (`services/rooms/go.mod`)

**Frontend:**
- Vite 5.4.21 - Build tool, development server
- Vue Router 4.3.0 - Client-side routing
- Pinia 2.1.7 - State management (auth, player, user lists)
- Tailwind CSS 4.1.18 - Styling (compiled via PostCSS)

**Video Players:**
- Video.js 8.10.0 - HTML5 video player wrapper (HiAnime, Consumet players)
- HLS.js 1.6.15 - HLS stream playback (switchable with Video.js)
- Kodik iframe - Embedded player (no direct control)

**Subtitles:**
- ass-compiler 0.1.16 - Parses ASS subtitle format for JP subtitles
- Custom SubtitleOverlay.vue component (renders selectable text, time-synced)

**API Clients:**
- Hasura GraphQL Client v0.12.1 - Queries Shikimori GraphQL endpoint (`services/catalog/go.mod`)
- goquery v1.9.2 - HTML parsing for video source scrapers
- MinIO Go Client v7.0.67 - S3-compatible object storage (`services/streaming/go.mod`)

**Testing:**
- Playwright 1.58.0 - E2E tests for frontend (`frontend/web/package.json`)
- Testify v1.8.4 - Go test assertions (`services/player/go.mod`, `services/scheduler/go.mod`)

**Build/Dev:**
- Buf 1.29.0 - Protocol Buffer tooling
- Helm 3.14.0 - Kubernetes package manager
- ESLint 8.57.0 - Frontend linting (via `.eslintrc.cjs`)
- Vue TSC 2.0.7 - TypeScript type checking for Vue
- Autoprefixer 10.4.23 - CSS vendor prefixing

## Key Dependencies

**Critical:**
- `golang-jwt/jwt v5.2.0` - JWT token generation/validation (auth service)
- `google/uuid v1.6.0` - UUID generation for entity IDs
- `lib/pq v1.2.0` - PostgreSQL driver (auth service)
- `jackc/pgx v5.7.4` - PostgreSQL connection pooling (GORM driver)
- `redis/go-redis v9.5.1` - Redis client (caching layer)

**Infrastructure:**
- `prometheus/client_golang v1.19.0` - Prometheus metrics export (all services)
- `go.opentelemetry.io/otel` - OpenTelemetry tracing and instrumentation
- `go.uber.org/zap v1.27.0` - Structured logging (all services via `libs/logger`)
- `robfig/cron v3.0.1` - Job scheduling (scheduler service)

**API Integrations:**
- `PuerkitoBio/goquery v1.9.2` - HTML scraping for anime parsers

## Configuration

**Environment:**
- `.env` file in `docker/` directory (not committed — see `.env.example`)
- Service configuration via environment variables:
  - `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` — PostgreSQL
  - `REDIS_HOST`, `REDIS_PORT` — Redis
  - `JWT_SECRET` — Authentication signing key
  - `SHIKIMORI_CLIENT_ID`, `SHIKIMORI_CLIENT_SECRET` — Shikimori API auth
  - `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET` — S3 storage
  - `JIMAKU_API_KEY` — Japanese subtitle service
  - `TELEGRAM_ADMIN_CHAT_ID` — Admin notifications
  - `UI_AUDIT_API_KEY` — API key for `ui_audit_bot` test account

**Build:**
- `Makefile` — Targets: `make dev`, `make redeploy-<service>`, `make logs-<service>`, etc.
- Service Dockerfiles: multi-stage build pattern (builder + runtime)
  - All services inherit Go builder from `golang:1.22-alpine`
  - Frontend uses nginx (`frontend/web/Dockerfile` or via docker-compose)
- `.editorconfig` — Enforces tabs for Go/Makefile, spaces for YAML/JSON/Vue/TS

## Platform Requirements

**Development:**
- Go 1.22 (via `.tool-versions`)
- Node.js 20.11.0 (for frontend build tooling)
- Docker & Docker Compose (for local services: PostgreSQL, Redis, MinIO, Prometheus, Grafana, etc.)
- Bun package manager (not npm/pnpm) for `frontend/web/` development

**Production:**
- Kubernetes 1.29.0 (orchestration, see `/deploy/kustomize/`)
- Docker images pushed to container registry
- PostgreSQL 16 (database)
- Redis 7 (caching)
- MinIO (S3-compatible object storage for uploaded videos)
- Grafana 10.3.3 (metrics dashboards)
- Prometheus 2.50.1 (metrics scraping)
- Loki 2.9.4 (log aggregation)
- NATS 2.10 with JetStream (message broker, optional)
- Meilisearch 1.6 (search index, optional)
- Nginx (reverse proxy, serving frontend static assets)

## Shared Libraries

Located in `libs/`:
- `database` — GORM connection management, auto-migration
- `cache` — Redis wrapper with TTL strategies
- `errors` — Domain error types (NotFound, Unauthorized, etc.)
- `logger` — Structured logging via zap, trace context support
- `authz` — JWT validation, role-based access control
- `httputil` — HTTP middleware (panic recovery, request logging, error response formatting)
- `metrics` — Prometheus instrumentation
- `idmapping` — ARM API client for anime ID mapping (Shikimori ↔ AniList ↔ MAL)
- `tracing` — OpenTelemetry setup
- `pagination` — Cursor-based pagination utilities
- `animeparser` — Video source client interface, anime ID resolver
- `videoutils` — HLS/MP4 proxy handler, CORS support

## Service Ports

| Service | Port | Purpose |
|---------|------|---------|
| gateway | 8000 | API gateway, rate limiting |
| auth | 8080 | Authentication, JWT, API key resolution |
| catalog | 8081 | Anime metadata, video source parsers, Shikimori integration |
| streaming | 8082 | Video proxy, MinIO storage |
| player | 8083 | Watch progress, watchlist, user lists |
| rooms | 8084 | Game rooms, WebSocket, real-time features |
| scheduler | 8085 | Background jobs, cron tasks |
| themes | 8086 | Anime OP/ED theme ratings |
| frontend | 80 | Vue 3 SPA (nginx reverse proxy) |

## External Docker Services

Defined in `docker/docker-compose.yml`:

| Service | Image | Purpose | Port |
|---------|-------|---------|------|
| postgres | postgres:16-alpine | Main database | 5432 |
| redis | redis:7-alpine | Session + request caching | 6379 |
| minio | minio/minio:latest | S3-compatible object storage (videos) | 9000, 9001 |
| nats | nats:2.10-alpine | Message broker with JetStream | 4222, 8222 |
| meilisearch | getmeili/meilisearch:v1.6 | Full-text search index | 7700 |
| aniwatch | ghcr.io/ghoshritesh12/aniwatch:v2.19.0 | Aniwatch API for anime data | 3100 (→ 4000) |
| consumet | riimuru/consumet-api:latest | Consumet API for streaming sources | 3101 (→ 3000) |
| megacloud-extractor | Local build | Browser automation for video extraction | 3200 |
| prometheus | prom/prometheus:v2.50.1 | Metrics scraping | 9090 |
| loki | grafana/loki:2.9.4 | Log aggregation | 3102 (→ 3100) |
| promtail | grafana/promtail:2.9.4 | Log shipper to Loki | — |
| grafana | grafana/grafana:10.3.3 | Dashboard UI | 3004 (→ 3000) |
| backup | Custom Docker build | PostgreSQL backups to S3 | — |

---

*Stack analysis: 2026-04-27*
