# CLAUDE.md - Project Guidelines for AI Assistants

## Project Overview

AnimeEnigma is a self-hosted anime streaming platform with Shikimori/MAL integration. It uses a Go microservices architecture with a Vue 3 frontend.

**Target deployment**: Small self-hosted groups (no CDN required).

## Architecture Principles

### Video Streaming Model

Videos are sourced in three ways:
1. **Direct frontend streaming** - Frontend fetches video URLs from external APIs (Aniboom, Kodik) and streams directly
2. **Backend proxy/restream** - Backend proxies video streams from external APIs (for CORS/auth)
3. **Self-hosted storage** - Admin-uploaded videos stored in MinIO

### On-Demand Catalog Population

The anime catalog is NOT pre-populated. Instead:
1. User searches for anime
2. Backend queries Shikimori GraphQL API
3. Results are mapped by **original Japanese name** as the primary key
4. Anime metadata is stored in PostgreSQL for future lookups
5. Video sources are resolved separately via anime parsers

### External API Integration

Primary data sources:
- **Shikimori** - Anime metadata (titles, descriptions, posters, genres)
- **Aniboom/Kodik** - Video streaming sources
- **MAL** (optional) - Additional metadata, ratings sync

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

- Use `libs/database` for connection management
- Write raw SQL for complex queries, use sqlx for scanning
- Migrations in `services/{name}/migrations/` with numbered prefixes (001_, 002_)
- Primary key: UUID strings, not auto-increment

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

```
User -> Frontend -> Catalog Service (get video sources)
                          |
        [Return available sources: aniboom, kodik, minio]
                          |
User -> Frontend -> [Direct stream from aniboom/kodik]
       OR
User -> Frontend -> Streaming Service -> [Proxy stream]
       OR
User -> Frontend -> Streaming Service -> [Serve from MinIO]
```

### Anime Parser Flow

```
Catalog Service -> Anime Parser (libs/animeparser)
                        |
        [Resolve Shikimori ID -> Aniboom/Kodik ID]
                        |
        [Fetch available episodes & qualities]
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

### Aniboom/Kodik

These APIs require integration with third-party anime video aggregators. Implementation in `services/catalog/internal/parser/`.

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
ANIBOOM_API_KEY (if using)
KODIK_API_KEY (if using)
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

```bash
# Create new migration
make migrate-create NAME=add_video_sources

# Run migrations
make migrate-up

# Rollback
make migrate-down
```

## Don't Do

- Don't add CDN-related code (not needed for self-hosted)
- Don't pre-populate the database with anime (on-demand only)
- Don't store video files for external sources (stream directly)
- Don't cache video URLs longer than 1 hour (they expire)
- Don't use ORM magic - write explicit SQL
- Don't add complex abstractions for simple operations

## File Locations

- Shared libraries: `/libs/`
- API contracts: `/api/`
- Service code: `/services/{name}/`
- Frontend: `/frontend/web/`
- Infrastructure: `/docker/`, `/deploy/`, `/infra/`
