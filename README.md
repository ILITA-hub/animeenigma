# AnimeEnigma

[🇷🇺 Русская версия](README.ru.md)

A self-hosted anime streaming platform with MAL/Shikimori integration, built as a Go microservices monorepo with a Vue 3 frontend.

**Target**: Self-hosted for small groups (no CDN required).

## Features

- 🎬 **Hybrid Video Streaming** - Stream from external APIs (Kodik, Aniboom) or self-hosted MinIO
- 🔍 **On-demand Catalog** - Anime data fetched from Shikimori in real-time when users search
- 🎮 **Multiplayer Game** - Anime opening/ending guessing game with real-time WebSocket rooms
- 📊 **Progress Tracking** - Watch history, anime lists, and playback position sync
- 🔐 **Authentication** - JWT-based auth with role-based access control (user/admin)

## Architecture

```
┌─────────────┐                         ┌──────────────┐
│   Frontend  │◄───── REST/GraphQL ────►│   Gateway    │
│   (Vue 3)   │                         └──────┬───────┘
└──────┬──────┘                                │
       │                     ┌─────────────────┼─────────────────┐
       │                     │                 │                 │
       │               ┌─────▼─────┐     ┌─────▼─────┐     ┌─────▼─────┐
       │               │   Auth    │     │  Catalog  │     │ Streaming │
       │               └───────────┘     │(Shikimori)│     │  (Proxy)  │
       │                                 └─────┬─────┘     └─────┬─────┘
       │                                       │                 │
       │    ┌──────────────────────────────────┘                 │
       │    │                                                    │
       │    ▼                                                    ▼
       │ ┌──────────┐   ┌──────────┐                    ┌──────────────┐
       │ │ Shikimori│   │  Kodik   │                    │    MinIO     │
       │ │   API    │   │   API    │                    │  (uploads)   │
       │ └──────────┘   └────┬─────┘                    └──────────────┘
       │                     │
       └─────── iframe ──────┘ (direct playback)
              OR
       └─── proxy stream ────► Streaming Service ────► Aniboom API
```

### Video Streaming Flow

Videos are sourced in three ways:

1. **Iframe Embed (Kodik)** - Frontend embeds Kodik player directly
2. **Proxied Stream (Aniboom)** - Backend proxies HLS streams for CORS bypass
3. **Self-hosted (MinIO)** - Admin-uploaded videos served from MinIO storage

### On-Demand Catalog

The anime catalog is **NOT pre-populated**. Instead:

1. User searches for anime in the frontend
2. Catalog service queries Shikimori GraphQL API
3. Results are mapped by **Japanese title** as primary key
4. Anime metadata is stored in PostgreSQL for future lookups
5. Video sources are resolved via Kodik/Aniboom by title matching

## Quick Start

### Prerequisites

- Go 1.22+
- Node.js 20+
- Docker & Docker Compose
- Make

### Development

1. **Start infrastructure:**
   ```bash
   make dev
   ```

2. **Start backend services:**
   ```bash
   # In separate terminals or use docker compose
   cd services/auth && go run ./cmd/auth-api
   cd services/catalog && go run ./cmd/catalog-api
   # ... etc
   ```

3. **Start frontend:**
   ```bash
   cd frontend/web
   npm install
   npm run dev
   ```

### Using Docker Compose

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
│   ├── player/         # Watch progress & lists
│   ├── rooms/          # Game rooms & WebSocket
│   ├── scheduler/      # Background jobs
│   └── gateway/        # API gateway
│
├── frontend/
│   └── web/            # Vue 3 SPA
│
├── libs/               # Shared Go libraries
│   ├── logger/
│   ├── errors/
│   ├── cache/
│   ├── database/
│   ├── authz/
│   ├── httputil/
│   ├── pagination/
│   ├── animeparser/
│   ├── videoutils/
│   └── tracing/
│
├── api/                # API contracts
│   ├── openapi/
│   ├── proto/
│   ├── graphql/
│   └── events/
│
├── docker/             # Docker Compose for local dev
├── deploy/             # Kubernetes configs
│   └── kustomize/
├── infra/              # Helm charts
│   └── helm/
└── scripts/            # Build & utility scripts
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| Gateway | 8000 | API gateway, rate limiting, routing |
| Auth | 8080 | Authentication & user management |
| Catalog | 8081 | Anime catalog, Shikimori integration |
| Streaming | 8082 | Video streaming/proxy |
| Player | 8083 | Watch progress & anime lists |
| Rooms | 8084 | Game rooms & WebSocket |
| Scheduler | 8085 | Background jobs |
| Frontend | 3000 | Vue 3 SPA |

## Configuration

Services are configured via environment variables. See each service's `internal/config/config.go` for available options.

### Core Services

| Variable | Description | Default |
|----------|-------------|---------|
| `JWT_SECRET` | JWT signing secret | - |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | PostgreSQL connection | localhost:5432 |
| `REDIS_HOST`, `REDIS_PORT` | Redis connection | localhost:6379 |
| `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY` | MinIO storage | localhost:9000 |

### Video Providers

| Variable | Description | Required |
|----------|-------------|----------|
| `KODIK_API_KEY` | Kodik API key for video search | For Kodik support |
| `KODIK_BASE_URL` | Kodik API base URL | `https://kodikapi.com` |
| `ANIBOOM_BASE_URL` | Aniboom API base URL | For Aniboom support |
| `SHIKIMORI_CLIENT_ID` | Shikimori OAuth client ID | Optional |
| `SHIKIMORI_CLIENT_SECRET` | Shikimori OAuth secret | Optional |

### Example `.env`

```env
# Database
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

# Auth
JWT_SECRET=your-super-secret-key

# Video Providers
KODIK_API_KEY=your-kodik-api-key
# ANIBOOM_BASE_URL=https://api.aniboom.one
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

- OpenAPI specs: `api/openapi/`
- GraphQL schema: `api/graphql/schema.graphql`
- Proto definitions: `api/proto/`

## Legacy Setup

The original NestJS backend and Express rooms backend are preserved in `services/backend/` and `services/roomsBackend/` for reference during migration.

## License

MIT
