# AnimeEnigma

[🇷🇺 Русская версия](README.ru.md)

A self-hosted anime streaming platform with MAL/Shikimori integration. Built as a Go microservices monorepo with a Vue 3 frontend.

**Target audience**: Self-hosting for small groups (no CDN required).

## Features

- **Multi-Source Streaming** — 4 video providers: Kodik (RU iframe), AnimeLib (RU MP4), HiAnime (EN HLS), Consumet (EN HLS), plus self-hosted MinIO storage
- **On-Demand Catalog** — Anime data is fetched from Shikimori in real-time when searching
- **Japanese Subtitles** — Selectable-text JP subtitle overlay (ASS/SRT/VTT) via Jimaku.cc
- **OP/ED Ratings** — Rate and browse anime openings and endings
- **Multiplayer Game** — Real-time opening/ending guessing game via WebSocket
- **Progress Tracking** — Watch history, watchlists, playback position sync
- **Browse & Schedule** — Genre filtering, status filtering, airing schedule
- **Multi-Language UI** — English, Russian, and Japanese
- **Error Reporting** — In-player bug reports with diagnostics and Telegram notifications
- **API Key Auth** — API keys for programmatic access (e.g. MAL watchlist import)
- **Authentication** — JWT authorization with role-based access (user/admin)
- **Auto Database Setup** — Databases and tables are created automatically on first run

## Architecture

```
┌─────────────┐                         ┌──────────────┐
│  Frontend   │◄───── REST/WS ─────────►│   Gateway    │
│   (Vue 3)   │                         └──────┬───────┘
└──────┬──────┘                                │
       │              ┌────────┬───────┬───────┼────────┬──────────┐
       │              │        │       │       │        │          │
       │         ┌────▼──┐ ┌──▼───┐ ┌─▼────┐ ┌▼─────┐ ┌▼──────┐ ┌▼───────┐
       │         │ Auth  │ │Catalog│ │Stream│ │Player│ │ Rooms │ │ Themes │
       │         └───────┘ └──┬───┘ └──┬───┘ └──────┘ └───────┘ └────────┘
       │                      │        │
       │         ┌────────────┤        │
       │         │            │        │
       │         ▼            ▼        ▼
       │   ┌──────────┐ ┌─────────┐ ┌──────┐
       │   │Shikimori │ │ Parsers │ │MinIO │
       │   │  API     │ │         │ └──────┘
       │   └──────────┘ └────┬────┘
       │                     │
       │    ┌────────┬───────┼────────┐
       │    ▼        ▼       ▼        ▼
       │ Kodik  AnimeLib  HiAnime  Consumet
       │ (iframe) (MP4)    (HLS)    (HLS)
       └────┘
```

### Video Players

The platform has 4 video players, each targeting a different source:

| Player | Language | Video Tech | Features |
|--------|----------|-----------|----------|
| **Kodik** | RU | Iframe embed | No direct video control |
| **AnimeLib** | RU | HTML5 MP4 (or Kodik fallback) | Quality selection |
| **HiAnime** | EN | HLS via Video.js | JP subs, quality selection, progress tracking |
| **Consumet** | EN | HLS via Video.js | JP subs, quality selection, progress tracking |

Videos are obtained in three ways:

1. **Iframe (Kodik)** — Frontend embeds Kodik player directly
2. **Proxied Stream (HiAnime, Consumet, AnimeLib)** — Backend proxies HLS/MP4 streams for CORS
3. **Self-hosted Storage (MinIO)** — Admin-uploaded videos

### On-Demand Catalog

The anime database is **NOT pre-populated**. Instead:

1. User searches for anime in the frontend
2. Catalog service queries Shikimori GraphQL API
3. Results are matched by **Japanese name** as the primary key
4. Anime metadata is stored in PostgreSQL for future queries
5. Video sources are resolved via parsers (Kodik, AnimeLib, HiAnime, Consumet)

## Quick Start

### Requirements

- Go 1.22+
- Bun 1.x+
- Docker & Docker Compose
- Make

### Development

1. **Start infrastructure:**
   ```bash
   make dev
   ```

2. **Start backend services:**
   ```bash
   # In separate terminals or via docker compose
   cd services/auth && go run ./cmd/auth-api
   cd services/catalog && go run ./cmd/catalog-api
   # ... etc.
   ```

3. **Start frontend:**
   ```bash
   cd frontend/web
   bun install
   bun run dev
   ```

### With Docker Compose

```bash
# Start everything
docker compose -f docker/docker-compose.yml up -d

# View logs
docker compose -f docker/docker-compose.yml logs -f

# Stop
docker compose -f docker/docker-compose.yml down
```

## Project Structure

```
animeenigma/
├── services/           # Go microservices
│   ├── auth/           # Authentication service
│   ├── catalog/        # Anime catalog with Shikimori integration
│   ├── streaming/      # Video streaming/proxy service
│   ├── player/         # Watch progress, watchlists, error reports
│   ├── rooms/          # Game rooms and WebSocket
│   ├── scheduler/      # Background tasks
│   ├── themes/         # OP/ED ratings
│   └── gateway/        # API gateway
│
├── frontend/
│   └── web/            # Vue 3 SPA
│
├── libs/               # Shared Go libraries
│   ├── logger/         # Structured logging
│   ├── errors/         # Error handling
│   ├── cache/          # Redis caching
│   ├── database/       # PostgreSQL with GORM (auto-init)
│   ├── authz/          # JWT authentication
│   ├── httputil/       # HTTP utilities
│   ├── pagination/     # Pagination
│   ├── animeparser/    # Video source parser interface
│   ├── idmapping/      # Anime ID mapping (MAL/Shikimori → AniList via ARM)
│   ├── videoutils/     # Video handling, HLS proxy, MinIO
│   ├── metrics/        # Prometheus metrics
│   └── tracing/        # OpenTelemetry
│
├── api/                # API contracts
│   ├── openapi/        # OpenAPI specifications
│   ├── proto/          # Protobuf definitions
│   ├── graphql/        # GraphQL schema
│   └── events/         # CloudEvents for async messaging
│
├── docker/             # Docker Compose for local development
├── deploy/             # Kubernetes configurations
│   └── kustomize/
├── infra/              # Helm charts
│   └── helm/
└── scripts/            # Build scripts and utilities
```

## Services

| Service   | Port | Description                              |
|-----------|------|------------------------------------------|
| Gateway   | 8000 | API gateway, rate limiting, routing       |
| Auth      | 8080 | Authentication, JWT, API keys             |
| Catalog   | 8081 | Anime catalog, Shikimori, video parsers   |
| Streaming | 8082 | Video streaming/proxy, MinIO              |
| Player    | 8083 | Watch progress, watchlists, error reports |
| Rooms     | 8084 | Game rooms and WebSocket                  |
| Scheduler | 8085 | Background tasks                          |
| Themes    | 8086 | OP/ED ratings                             |
| Frontend  | 80   | Vue 3 SPA (nginx)                         |

## Configuration

Services are configured via environment variables. See `internal/config/config.go` of each service.

### Core Services

| Variable                                              | Description      | Default        |
|-------------------------------------------------------|------------------|----------------|
| `JWT_SECRET`                                          | JWT signing key  | -              |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | PostgreSQL       | localhost:5432 |
| `REDIS_HOST`, `REDIS_PORT`                            | Redis            | localhost:6379 |
| `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY` | MinIO storage    | localhost:9000 |

### Video & External Services

| Variable              | Description                          | Required                  |
|-----------------------|--------------------------------------|---------------------------|
| `KODIK_API_KEY`       | Kodik API key for RU video search    | For Kodik support         |
| `SHIKIMORI_CLIENT_ID` | Shikimori OAuth client ID            | Optional                  |
| `SHIKIMORI_CLIENT_SECRET` | Shikimori OAuth secret           | Optional                  |
| `JIMAKU_API_KEY`      | Jimaku.cc API key for JP subtitles   | For JP subtitle support   |
| `TELEGRAM_ADMIN_CHAT_ID` | Telegram chat ID for admin notifications | For error report alerts |

HiAnime and Consumet are configured via their sidecar containers in docker-compose. AnimeLib requires no API key.

### Example `.env`

```env
# Database (auto-created if not exists)
DB_HOST=localhost
DB_PORT=5432
DB_USER=animeenigma
DB_PASSWORD=secret
DB_NAME=animeenigma

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# MinIO (for admin uploads)
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=animeenigma

# Authentication
JWT_SECRET=your-super-secret-key

# Video providers
KODIK_API_KEY=your-kodik-api-key

# Optional
JIMAKU_API_KEY=your-jimaku-api-key
TELEGRAM_ADMIN_CHAT_ID=your-telegram-chat-id
```

## Development

```bash
# Run all Go tests
make test

# Lint Go code
make lint

# Build all services
make build

# Generate API code
make generate

# Build Docker images
make docker-build

# Redeploy a single service after code changes
make redeploy-catalog

# Follow service logs
make logs-catalog

# Check health of all services
make health
```

## Deployment

### Kubernetes with Kustomize

```bash
# Deploy to dev
kubectl apply -k deploy/kustomize/overlays/dev

# Deploy to prod
kubectl apply -k deploy/kustomize/overlays/prod
```

### Helm

```bash
cd infra/helm
helm install animeenigma ./gateway -f gateway/values.yaml
```

## API Documentation

- OpenAPI specifications: `api/openapi/`
- GraphQL schema: `api/graphql/schema.graphql`
- Proto definitions: `api/proto/`

## Multiplayer Game

AnimeEnigma includes an opening/ending guessing game:

1. Create a room with settings (number of rounds, time, mode)
2. Invite friends via link
3. Opening/ending video plays
4. Players enter the anime title
5. Points are awarded for speed and accuracy
6. Global and session leaderboards

## Monitoring

Each service exposes Prometheus metrics at `/metrics`:

- `http_requests_total` - Request counter with labels
- `http_request_duration_seconds` - Latency histogram (p50/p95/p99)
- `http_response_size_bytes` - Response size histogram

Grafana dashboard and Prometheus are available via the admin-nginx container when deployed.

## License

MIT
