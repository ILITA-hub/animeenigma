# CLAUDE.md - Project Guidelines for AI Assistants

## Project Overview

AnimeEnigma is a self-hosted anime streaming platform with Shikimori/MAL integration. It uses a Go microservices architecture with a Vue 3 frontend.

**Target deployment**: Small self-hosted groups (no CDN required).

## Architecture Principles

### Video Player Architecture

The platform has 4 video players, each targeting different source APIs:

| Player | Lang | Component | Video Tech | Tracking | JP Subs | Quality |
|--------|------|-----------|-----------|----------|---------|---------|
| **Kodik** | RU | `KodikPlayer.vue` | Kodik iframe | No | No | No (iframe) |
| **AnimeLib** | RU | `AnimeLibPlayer.vue` | HTML5 `<video>` (MP4) or Kodik iframe fallback | Yes | No | Yes (MP4) |
| **HiAnime** | EN | `HiAnimePlayer.vue` | Video.js / HLS.js (switchable) | Yes | Yes | Yes (HLS) |
| **Consumet** | EN | `ConsumetPlayer.vue` | Video.js / HLS.js (switchable) | Yes | Yes | Yes (HLS) |

**Shared components:**
- `SubtitleOverlay.vue` — Custom selectable-text JP subtitle renderer (ASS/SRT/VTT). Used by HiAnime + Consumet. Teleports to fullscreen element, time-synced via `requestAnimationFrame`.
- `subtitle-parser.ts` — Parses ASS (via `ass-compiler`), SRT, VTT into `SubtitleCue[]`
- `libs/videoutils/proxy.go` — Backend HLS proxy for CORS. Allowed domains include streaming CDNs, `jimaku.cc`, `cdnlibs.org` (AnimeLib).

**Known issue:** AnimeLib subtitles are broken — direct MP4 player can't render soft-subs embedded in the video. Kodik iframe fallback works but may not always be available.

### Video Streaming Model

Videos are sourced in three ways:
1. **Kodik iframe** — Frontend embeds Kodik's player iframe (no direct video control)
2. **Backend proxy/restream** — Backend proxies HLS/MP4 streams from external APIs (HiAnime, Consumet, AnimeLib) for CORS
3. **Self-hosted storage** — Admin-uploaded videos stored in MinIO

### On-Demand Catalog Population

The anime catalog is NOT pre-populated. Instead:
1. User searches for anime
2. Backend queries Shikimori GraphQL API
3. Results are mapped by **original Japanese name** as the primary key
4. Anime metadata is stored in PostgreSQL for future lookups
5. Video sources are resolved separately via anime parsers

### External API Integration

Primary data sources:
- **Shikimori** — Anime metadata (titles, descriptions, posters, genres)
- **Kodik** — RU video streaming (iframe embed). Parser: `services/catalog/internal/parser/kodik/`
- **AnimeLib** — RU video streaming (direct MP4 + Kodik fallback). Parser: `services/catalog/internal/parser/animelib/`
- **HiAnime** — EN video streaming (HLS). Parser: `services/catalog/internal/parser/hianime/`
- **Consumet** — EN video streaming (HLS). Parser: `services/catalog/internal/parser/consumet/`
- **Jimaku.cc** — Japanese subtitle files (ASS/SRT/VTT). Used by HiAnime + Consumet players.
- **ARM** (`arm.haglund.dev`) — Anime ID mapping (Shikimori/MAL → AniList). Library: `libs/idmapping/`
- **MAL** (optional) — Additional metadata, ratings sync

## Code Conventions

### Go Services

```
services/{name}/
├── cmd/{name}-api/main.go    # Entry point
├── internal/
│   ├── config/               # Environment config
│   ├── domain/               # Domain models & interfaces
│   ├── handler/              # HTTP handlers
│   ├── service/              # Business logic
│   ├── repo/                 # Database repositories
│   ├── parser/               # External API clients
│   └── transport/            # Router setup
├── migrations/               # SQL migrations
├── Dockerfile
└── go.mod
```

### Naming Conventions

- **Packages**: lowercase, single word (`handler`, `service`, `repo`)
- **Files**: snake_case (`anime_parser.go`, `video_source.go`)
- **Types**: PascalCase (`AnimeService`, `VideoRepository`)
- **Methods**: PascalCase for exported, camelCase for private
- **Variables**: camelCase (`animeID`, `videoURL`)
- **Constants**: PascalCase or ALL_CAPS for env vars

### Error Handling

Use the shared `libs/errors` package:

```go
import "github.com/ILITA-hub/animeenigma/libs/errors"

// Return domain errors
if anime == nil {
    return nil, errors.NotFound("anime not found")
}

// Wrap external errors
if err != nil {
    return nil, errors.Wrap(err, "failed to fetch from shikimori")
}
```

### Database

- Use `libs/database` with GORM for connection management
- Database and tables are auto-created on service startup
- Use GORM query methods for most operations
- For complex queries, use GORM's raw SQL capabilities
- Primary key: UUID strings with `gen_random_uuid()` default

```go
// Example: Define model with GORM tags
type User struct {
    ID        string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    Username  string         `gorm:"size:32;uniqueIndex" json:"username"`
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Example: Auto-migrate in main.go
db, err := database.New(cfg.Database)  // Auto-creates DB if not exists
db.AutoMigrate(&domain.User{})         // Creates table if not exists
```

### Caching

Use `libs/cache` with appropriate TTL strategies:

```go
// Anime details - cache 6 hours
cache.Set(ctx, cache.KeyAnime(id), anime, cache.TTLAnimeDetails)

// Search results - cache 15 minutes
cache.Set(ctx, cache.KeySearchResults(query, page), results, cache.TTLSearchResults)

// External video URLs - cache 1 hour (they expire)
cache.Set(ctx, "video:"+animeID, videoURL, time.Hour)
```

### Logging

Use structured logging via `libs/logger`:

```go
log.Infow("fetching anime from shikimori",
    "query", query,
    "page", page,
)

log.Errorw("failed to proxy video stream",
    "anime_id", animeID,
    "source", "aniboom",
    "error", err,
)
```

## Key Flows

### Search Flow

```
User -> Frontend -> Gateway -> Catalog Service
                                    |
                    [Check local DB first]
                                    |
                    [If not found, query Shikimori]
                                    |
                    [Store results, return to user]
```

### Video Playback Flow

**Kodik (iframe):**
```
User -> Frontend -> Catalog (Kodik parser) -> Kodik API
                          |
        [Return embed URL with params]
                          |
User -> Frontend -> KodikPlayer.vue [iframe src=embed URL]
```

**AnimeLib (MP4 proxy or Kodik fallback):**
```
User -> Frontend -> Catalog (AnimeLib parser) -> AnimeLib hapi API
                          |
        [Return MP4 URLs + qualities, or Kodik iframe URL]
                          |
User -> Frontend -> AnimeLibPlayer.vue -> Backend proxy -> MP4 stream
       OR
User -> Frontend -> AnimeLibPlayer.vue [Kodik iframe fallback]
```

**HiAnime / Consumet (HLS proxy):**
```
User -> Frontend -> Catalog (HiAnime/Consumet parser) -> External API
                          |
        [Return HLS m3u8 URLs + VTT subtitle URLs]
                          |
User -> Frontend -> Player.vue -> Backend HLS proxy -> m3u8 stream
                          |
        [Optional: Jimaku.cc JP subs via ARM AniList ID lookup]
```

### Anime Parser Flow

```
Catalog Service -> Anime Parser (services/catalog/internal/parser/{name}/)
                        |
        [Resolve Shikimori ID -> provider-specific ID]
                        |
        [Fetch available episodes, translations & qualities]
                        |
        [Cache video URLs for 1 hour]
```

## External APIs

### Shikimori GraphQL

Endpoint: `https://shikimori.one/api/graphql`

```graphql
query SearchAnime($search: String!, $limit: Int) {
  animes(search: $search, limit: $limit) {
    id
    name
    russian
    japanese
    poster { originalUrl }
    genres { id name russian }
    episodes
    score
  }
}
```

### Video Source Providers

Each provider has a parser in `services/catalog/internal/parser/{name}/`:

- **Kodik** — RU iframe embed. Returns embed URLs with translation/episode params. No direct video control.
- **AnimeLib** — RU direct MP4. Uses AnimeLib's hapi API for episode data, MP4 URLs at multiple qualities, and translation info. Falls back to Kodik iframe when direct URLs unavailable.
- **HiAnime** — EN HLS streaming. Returns m3u8 URLs and VTT subtitle tracks. Proxied through backend for CORS.
- **Consumet** — EN HLS streaming. Returns m3u8 URLs and VTT subtitle tracks. Proxied through backend for CORS.

## Testing

- Unit tests: `go test ./...`
- Integration tests: `go test -tags=integration ./...`
- Use testcontainers for database tests
- Mock external APIs, don't hit them in tests

## Environment Variables

Required for all services:
```
DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
REDIS_HOST, REDIS_PORT
JWT_SECRET
```

Catalog service specific:
```
SHIKIMORI_CLIENT_ID
SHIKIMORI_CLIENT_SECRET
KODIK_API_KEY (if using)
JIMAKU_API_KEY (if using JP subtitles)
```

Streaming service specific:
```
MINIO_ENDPOINT
MINIO_ACCESS_KEY
MINIO_SECRET_KEY
MINIO_BUCKET
```

## Common Tasks

### Adding a new anime parser

1. Create client in `services/catalog/internal/parser/{name}/client.go`
2. Implement `AnimeParser` interface
3. Add to parser registry in service initialization
4. Update config for API keys

### Adding a new video source

1. Add source type to `domain.SourceType`
2. Implement proxy handler in streaming service if needed
3. Update frontend player to handle new source

### Database migrations

Tables are auto-created via GORM's `AutoMigrate()` on service startup. For schema changes:

1. Update the domain model struct with new fields/tags
2. Restart the service - new columns are added automatically
3. For destructive changes (dropping columns), use manual SQL or recreate the table

**Note**: GORM only creates new tables/columns, it does NOT modify or drop existing columns to protect data.

## Local Development Commands

Use `make` for all local development operations. Run `make help` to see all available targets.

| Command | Description |
|---------|-------------|
| `make dev` | Start full development environment |
| `make dev-down` | Stop development environment |
| `make redeploy-<service>` | Rebuild and restart a service (after code changes) |
| `make restart-<service>` | Restart without rebuilding (after config changes) |
| `make logs-<service>` | Follow service logs |
| `make health` | Check health of all services |
| `make ps` | Show running containers |

**Frontend Note**: Use `bun` (not npm/pnpm) for frontend development:
```bash
cd frontend/web
bun install          # Install dependencies
bun run dev          # Development server
bun run build        # Production build
bun run test:e2e     # Run e2e tests

# For Playwright, use bunx (not npx):
bunx playwright test                           # Run all e2e tests
bunx playwright test hianime-integration       # Run specific test file
bunx playwright test --reporter=list           # With list reporter

# For all CLI tools, use bunx (not npx):
bunx eslint src/                               # Run ESLint
bunx tsc --noEmit                              # Type-check
```

Examples:
```bash
# After modifying gateway code
make redeploy-gateway

# After changing docker-compose.yml env vars (no code changes)
make restart-grafana

# Debug catalog service
make logs-catalog

# Check all services are healthy
make health
```

## Don't Do

- Don't add CDN-related code (not needed for self-hosted)
- Don't pre-populate the database with anime (on-demand only)
- Don't store video files for external sources (stream directly)
- Don't cache video URLs longer than 1 hour (they expire)
- Don't fight GORM - use its conventions for simple queries, raw SQL for complex ones
- Don't add complex abstractions for simple operations

## Service Ports

| Service    | Port | Metrics   | Description                    |
|------------|------|-----------|--------------------------------|
| gateway    | 8000 | /metrics  | API gateway, rate limiting     |
| auth       | 8080 | /metrics  | Authentication, JWT            |
| catalog    | 8081 | /metrics  | Anime catalog, Shikimori API   |
| streaming  | 8082 | /metrics  | Video streaming, MinIO         |
| player     | 8083 | /metrics  | Watch progress, watchlists     |
| rooms      | 8084 | /metrics  | Game rooms, WebSocket          |
| scheduler  | 8085 | /metrics  | Background jobs                |
| themes     | 8086 | /metrics  | Anime OP/ED ratings            |
| web        | 80   | -         | Vue 3 frontend (nginx)         |

### Gateway Routing

All API requests go through the gateway service:

- `/api/auth/*` → auth:8080
- `/api/anime/*` → catalog:8081
- `/api/genres` → catalog:8081
- `/api/kodik/*` → catalog:8081
- `/api/admin/*` → catalog:8081 (protected)
- `/api/streaming/*` → streaming:8082
- `/api/users/*` → player:8083
- `/api/rooms/*` → rooms:8084
- `/api/game/*` → rooms:8084
- `/api/themes/*` → themes:8086 (public + protected + admin)

### Monitoring Endpoints

Each service exposes Prometheus metrics at `/metrics`:

```bash
# Check gateway metrics
curl http://localhost:8000/metrics

# Check catalog latency percentiles
curl http://localhost:8081/metrics | grep http_request_duration_seconds
```

Available metrics:
- `http_requests_total` - Counter with labels: service, method, path, status
- `http_request_duration_seconds` - Histogram for p50/p95/p99 latencies
- `http_response_size_bytes` - Response size histogram

### Admin URLs (Kubernetes)

When deployed to Kubernetes, admin interfaces are available at:

- `https://admin.animeenigma.ru/grafana` - Grafana dashboards
- `https://admin.animeenigma.ru/prometheus` - Prometheus raw metrics
- `https://admin.animeenigma.ru/pgadmin` - PostgreSQL admin
- `https://admin.animeenigma.ru/k8s` - Kubernetes dashboard

## File Locations

- Shared libraries: `/libs/`
- API contracts: `/api/`
- Service code: `/services/{name}/`
- Frontend: `/frontend/web/`
- Infrastructure: `/docker/`, `/deploy/`, `/infra/`
- Kubernetes manifests: `/deploy/kustomize/`
