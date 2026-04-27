<!-- refreshed: 2026-04-27 -->
# Architecture

**Analysis Date:** 2026-04-27

## System Overview

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Vue 3 Frontend (Port 80)                           │
│    `frontend/web/` — Views, Stores, Composables, 4 Video Player Comps      │
└──────────────────────────────┬──────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│            API Gateway (Port 8000) — Route & Auth Proxy                      │
│   `services/gateway/internal/{handler,service,transport,config}`            │
└──────────────┬────────┬────────┬────────┬────────┬────────┬────────┬────────┘
               │        │        │        │        │        │        │
               ▼        ▼        ▼        ▼        ▼        ▼        ▼        ▼
        ┌──────────┬──────────┬───────────┬────────────┬──────────┬──────────┐
        │  Auth    │ Catalog  │ Streaming │   Player   │  Rooms   │Scheduler │
        │ (8080)   │ (8081)   │  (8082)   │  (8083)    │ (8084)   │ (8085)   │
        │ JWT      │ Shikimori│ MinIO HLS │ Progress   │ WebSocket│  Jobs    │
        │ Tokens   │ Search   │  Proxy    │ Watchlist  │ Game     │ Cron     │
        └──────────┴──────────┴───────────┴────────────┴──────────┴──────────┘
               │        │        │
               ▼        ▼        ▼
        ┌──────────────────────────────┐    ┌──────────────┐
        │    PostgreSQL Database       │    │   Redis      │
        │  (Users, Anime, Lists,etc)   │    │  (Caching)   │
        └──────────────────────────────┘    └──────────────┘

        ┌──────────────────────────────┐    ┌──────────────┐
        │   External APIs              │    │  Themes      │
        │ • Shikimori (metadata)       │    │  Service     │
        │ • Kodik (RU embed)           │    │  (8086)      │
        │ • AnimeLib (RU MP4)          │    │              │
        │ • HiAnime (EN HLS)           │    └──────────────┘
        │ • Consumet (EN HLS)          │
        │ • Jimaku.cc (JP subs)        │
        │ • ARM (ID mapping)           │
        └──────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| **Gateway** | Route all API requests, resolve JWT + API keys, rate limiting | `services/gateway/cmd/gateway-api/main.go` |
| **Auth Service** | User registration, login, password reset, JWT generation, API key mgmt | `services/auth/cmd/auth-api/main.go` |
| **Catalog Service** | Anime search/browse, Shikimori integration, video source parsing | `services/catalog/cmd/catalog-api/main.go` |
| **Streaming Service** | HLS proxy for CORS, MinIO integration for user uploads | `services/streaming/cmd/streaming-api/main.go` |
| **Player Service** | Watch progress tracking, watchlists, ratings, user lists | `services/player/cmd/player-api/main.go` |
| **Rooms Service** | Game room WebSocket management, concurrent players | `services/rooms/cmd/rooms-api/main.go` |
| **Scheduler Service** | Background jobs (anime updates, cache refresh, health checks) | `services/scheduler/cmd/scheduler-api/main.go` |
| **Themes Service** | Anime OP/ED ratings, public + protected endpoints | `services/themes/cmd/themes-api/main.go` |
| **Frontend** | Vue 3 single-page app with routing, stores, player selection | `frontend/web/src/` |

## Pattern Overview

**Overall:** Microservices with API Gateway pattern

**Key Characteristics:**
- Independent services each handling a domain (auth, catalog, playback)
- Shared libraries in `libs/` for common concerns (logging, caching, database, errors)
- PostgreSQL for persistence, Redis for caching/session management
- Gateway routes all frontend requests; services do NOT accept direct client connections
- Each service uses standard handler → service → repo → domain layering
- Stateless design — services can be scaled horizontally
- gRPC NOT used — all inter-service communication is HTTP via gateway proxy

## Layers

**Frontend (Vue 3):**
- Purpose: Render UI, call API via axios, manage local state (Pinia stores)
- Location: `frontend/web/src/`
- Contains: Views (pages), Components (UI + players), Stores (auth, home, watchlist), Composables (hooks)
- Depends on: API client (`api/client.ts`), Vue Router, Pinia, i18n, Video.js/HLS.js
- Used by: Browser clients only

**Gateway (HTTP Router + Proxy):**
- Purpose: Accept all client requests, route to appropriate service, handle cross-cutting auth
- Location: `services/gateway/internal/`
- Contains: Router (chi/v5), ProxyService, RequestID middleware, metrics collection
- Depends on: libs/logger, libs/metrics, libs/httputil, auth service (for JWT resolution)
- Used by: Frontend, public clients via API keys

**Application Services (8 microservices):**
- Purpose: Domain-specific business logic (auth, catalog, playback, etc.)
- Location: `services/{auth,catalog,streaming,player,rooms,scheduler,themes}/internal/`
- Standard structure: config → domain → handler (HTTP) → service (logic) → repo (DB) → transport (router)
- Depends on: libs/database (GORM), libs/cache (Redis), libs/logger, libs/errors, external APIs
- Used by: Gateway proxy, internal job schedulers

**Shared Libraries:**
- Purpose: Cross-cutting concerns shared by all services
- Location: `libs/{cache,database,logger,errors,metrics,httputil,idmapping,videoutils,...}/`
- Each library is a standalone Go module (reusable via go.work)

**Data Layer (PostgreSQL):**
- Purpose: Persistent storage for users, anime, watchlists, ratings, video sources
- Auto-migrated via GORM on service startup
- No raw SQL scripts in migrations/ — uses GORM AutoMigrate()

**Cache Layer (Redis):**
- Purpose: Session tokens, anime metadata cache, search results, video URL cache (1-hour TTL)
- Used by: All services via libs/cache with TTL strategies per data type

**External Integrations:**
- Shikimori GraphQL API (anime metadata)
- Video source providers: Kodik (embed), AnimeLib (MP4), HiAnime (HLS), Consumet (HLS)
- Jimaku.cc (JP subtitles)
- ARM (anime ID mapping for subtitle resolution)

## Data Flow

### Primary Request Path (Search → Watch)

1. **Frontend Request** → User enters search query in Browse.vue (`frontend/web/src/views/Browse.vue`)
2. **Gateway Proxy** → axios calls `GET /api/anime?q=query` → Gateway (port 8000)
3. **Auth Verification** → Gateway decodes JWT from Authorization header, caches user context
4. **Catalog Service** → Gateway proxies to `GET /api/anime?q=query` on catalog:8081
5. **Handler Processing** → CatalogHandler.SearchAnime parses filters (`services/catalog/internal/handler/catalog.go`)
6. **Service Logic** → CatalogService.SearchAnime checks cache, calls Shikimori API if miss (`services/catalog/internal/service/catalog.go`)
7. **Repository** → CatalogRepo stores anime metadata in PostgreSQL for future lookups (`services/catalog/internal/repo/`)
8. **Cache Store** → Results cached in Redis with TTL (search results: 15 min)
9. **Response** → JSON array of anime + pagination metadata returned to frontend

**State Management:** Search filters (query, page, genre) stored in Pinia `home.ts` store; results cached in memory + Redis.

### Video Playback Flow (4 Player Types)

#### **Kodik (RU Embed):**
1. User clicks play on anime detail
2. Frontend calls Catalog API: `GET /api/anime/{id}/sources` → returns Kodik embed URL
3. Frontend selects `KodikPlayer.vue` based on source type
4. KodikPlayer.vue renders iframe with Kodik embed URL
5. **No tracking** — iframe prevents video event access; user can adjust translation/episode via Kodik's own UI

**Files:** `frontend/web/src/components/player/KodikPlayer.vue`, `services/catalog/internal/parser/kodik/`

#### **AnimeLib (RU MP4 or Kodik Fallback):**
1. User clicks play on anime detail
2. Frontend calls Catalog API: `GET /api/anime/{id}/sources?provider=animelib` 
3. AnimeLib parser queries hapi API for episode list + MP4 URLs + translations
4. **If direct MP4 URLs available:** Return quality array + subtitle URLs
5. **If unavailable:** Fall back to Kodik iframe URL
6. Frontend selects `AnimeLibPlayer.vue`
7. **If MP4:** HTML5 `<video>` element plays MP4; SubtitleOverlay renders ASS/SRT/VTT subs
8. **If Kodik fallback:** Same as Kodik flow above

**Files:** `frontend/web/src/components/player/AnimeLibPlayer.vue`, `services/catalog/internal/parser/animelib/`, `frontend/web/src/components/player/SubtitleOverlay.vue`

#### **HiAnime / Consumet (EN HLS):**
1. User clicks play on anime detail
2. Frontend calls Catalog API: `GET /api/anime/{id}/sources?provider=hianime`
3. Parser queries external API (HiAnime or Consumet) for episode list + m3u8 URLs + VTT subtitle tracks
4. **Subtitle Resolution:** Parser calls ARM API to map Shikimori ID → AniList ID, then Jimaku.cc for JP ASS files
5. Frontend selects `HiAnimePlayer.vue` or `ConsumetPlayer.vue`
6. Video.js or HLS.js loads m3u8 from backend HLS proxy (CORS bypass)
7. Backend HLS proxy (`libs/videoutils/proxy.go`) validates domain whitelist, forwards request to external CDN
8. SubtitleOverlay renders VTT (English) and/or ASS (Japanese) subtitles with time sync via requestAnimationFrame
9. ReportButton component allows user to report broken streams

**Files:** `frontend/web/src/components/player/HiAnimePlayer.vue`, `frontend/web/src/components/player/ConsumetPlayer.vue`, `services/catalog/internal/parser/{hianime,consumet}/`, `services/streaming/internal/handler/` (HLS proxy), `libs/videoutils/proxy.go`

#### **Common Subtitle Handling:**
- SubtitleOverlay.vue listens to `<video>` element's currentTime via requestAnimationFrame
- Parses ASS via ass-compiler (lazy-loaded), SRT/VTT inline
- Renders visible cues as selectable-text overlay, time-synced
- Teleports to fullscreen element during fullscreen playback

**Files:** `frontend/web/src/components/player/SubtitleOverlay.vue`, `frontend/web/src/utils/subtitle-parser.ts`

### Anime Parser Flow (Internal)

```
Catalog Service
    ↓
Parser Registry (config-driven init)
    ↓
Anime Parser (e.g., HiAnimeParser, KodikParser)
    ├─ Resolve Shikimori ID → external provider's ID
    ├─ Fetch episode list
    ├─ Fetch available translations + qualities
    └─ Cache video URLs (1 hour TTL)
    ↓
Return VideoSource array to frontend
```

Example: `services/catalog/internal/parser/hianime/` client queries HiAnime API, extracts m3u8 URLs and episode data, caches in Redis.

**Files:** `services/catalog/internal/parser/{kodik,animelib,hianime,consumet,jimaku,telegram,etc}/`

## Key Abstractions

**VideoSource:**
- Purpose: Represents one available stream (provider + URL + translation + quality + episode)
- Examples: `libs/animeparser/video_source.go`
- Pattern: Returned by parsers, cached in Redis, consumed by frontend player selection

**Anime (Domain Model):**
- Purpose: Anime metadata (title, poster, genres, episodes, Shikimori ID)
- Examples: `services/catalog/internal/domain/`, `services/player/internal/domain/`
- Pattern: Mapped from Shikimori API, stored in PostgreSQL, cached in Redis

**AnimeParser Interface:**
- Purpose: Standardizes how external video sources are queried
- Examples: Each parser in `services/catalog/internal/parser/*/` implements the interface
- Pattern: Config-driven registration, called on-demand by CatalogService

**User & Auth:**
- Purpose: Represent authenticated users, API keys, JWT tokens
- Examples: `services/auth/internal/domain/`
- Pattern: JWT issued by auth service, resolved at gateway, cached in Redis

**User List / Watchlist:**
- Purpose: Anime status tracking (watching, completed, on-hold, etc.)
- Examples: `services/player/internal/domain/`
- Pattern: Per-user, stored in PostgreSQL, updated via Player Service API

## Entry Points

**Backend Services:**
- `services/auth/cmd/auth-api/main.go` — Listens on :8080, initializes DB, loads config, starts router
- `services/catalog/cmd/catalog-api/main.go` — Listens on :8081, initializes parsers, sets up cache
- `services/gateway/cmd/gateway-api/main.go` — Listens on :8000, routes to all upstream services
- `services/streaming/cmd/streaming-api/main.go` — Listens on :8082, handles HLS/MinIO proxying
- `services/player/cmd/player-api/main.go` — Listens on :8083, manages user lists + watch progress
- `services/rooms/cmd/rooms-api/main.go` — Listens on :8084, WebSocket for game rooms
- `services/scheduler/cmd/scheduler-api/main.go` — Listens on :8085, background jobs only
- `services/themes/cmd/themes-api/main.go` — Listens on :8086, OP/ED ratings

**Frontend:**
- `frontend/web/src/main.ts` — Creates Vue app, mounts to #app, initializes Pinia + Router + i18n
- `frontend/web/src/App.vue` — Root component, renders layout + router-view
- `frontend/web/src/router/index.ts` — Route definitions (Home, Browse, Anime detail, Profile, etc.)

## Architectural Constraints

- **Threading:** Go services use net/http's default goroutine-per-request model; frontend is single-threaded event loop (Vue + requestAnimationFrame for subtitle sync)
- **Global state:** Pinia stores in frontend (auth, home, watchlist); cache singleton in Go via libs/cache (Redis client pool); database singleton via libs/database (GORM connection pool)
- **Circular imports:** Services use `go.work` to manage dependency relationships; no service imports another service directly (only via HTTP proxy)
- **Video player selection:** Determined by available source types; Kodik forces iframe (no video control), others use HTML5 + SubtitleOverlay
- **Database auto-migration:** GORM AutoMigrate() on startup creates tables if absent; schema changes require code change + restart (no manual migrations in production)
- **Cross-service communication:** HTTP via gateway only; no shared database access between services (each service owns its schema)

## Anti-Patterns

### Circular Service Dependency

**What happens:** Service A imports service B, Service B imports service A → Go compilation fails
**Why it's wrong:** Breaks modularity, makes services hard to scale independently
**Do this instead:** All inter-service calls go through gateway proxy (HTTP). Service code NEVER imports() another service directly. See `services/gateway/internal/service/proxy.go` for the pattern — services only call each other via HTTP.

### Direct Database Access from Frontend

**What happens:** Frontend queries PostgreSQL directly (no auth, no cache invalidation)
**Why it's wrong:** Breaks security, bypasses authorization, creates n+1 query problems
**Do this instead:** Frontend only calls HTTP APIs via gateway. Services handle all database access (`services/*/internal/repo/`) with caching via Redis.

### Storing Video URLs for External Sources

**What happens:** Parse video URL once, cache forever in database
**Why it's wrong:** External video CDN URLs expire (typically within hours); cached URLs become 404s
**Do this instead:** Cache video URLs in Redis with 1-hour TTL (see `libs/cache` constants). On cache miss, re-query the external API.

### Mixing Authentication Levels

**What happens:** Some endpoints require JWT, others accept API keys, some are public → inconsistent auth logic in each handler
**Why it's wrong:** Creates security gaps, makes auditing hard
**Do this instead:** Gateway resolves JWT + API key into a unified User context; attach to request context. Services check authz via `libs/authz` package with role-based rules.

## Error Handling

**Strategy:** Centralized error types in `libs/errors/`; domain errors wrap with context; handlers convert to HTTP status codes

**Patterns:**
- Domain error: `return nil, errors.NotFound("anime not found")` → HTTP 404
- External API error: `if err != nil { return nil, errors.Wrap(err, "failed to fetch from shikimori") }` → HTTP 502
- Validation error: `return nil, errors.BadRequest("invalid query")` → HTTP 400
- Authorization error: `return nil, errors.Unauthorized("token expired")` → HTTP 401 (gateway handles JWT refresh)

Errors logged via `libs/logger` with structured fields (anime_id, source, error message).

## Cross-Cutting Concerns

**Logging:** Structured logging via `libs/logger` (zap + OpenTelemetry). All handlers log request start/end with status, latency, error details.

**Validation:** Handlers validate input (query length, page bounds, enum values); business logic assumes valid input.

**Authentication:** JWT tokens issued by auth service, resolved at gateway, cached in Redis. API keys (ak_ prefix) hashed as SHA-256 in database, resolved by auth service on gateway request.

**Authorization:** Role-based access control via `libs/authz` (admin, user, public). Checked in handlers or middleware.

**Metrics:** Prometheus metrics collected via `libs/metrics` (request count, latency histogram, DB pool stats). Exposed at `/metrics` on each service.

**Caching:** Redis via `libs/cache` with TTL strategies:
- Anime metadata: 6 hours
- Search results: 15 minutes
- Video URLs: 1 hour
- Session tokens: Until expiry (JWT)

---

*Architecture analysis: 2026-04-27*
