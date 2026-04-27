# External Integrations

**Analysis Date:** 2026-04-27

## APIs & External Services

### Anime Metadata & Search

**Shikimori (Russian anime database):**
- What it's used for: Primary anime metadata source (titles, descriptions, posters, genres, scores, episode counts)
- SDK/Client: `hasura/go-graphql-client v0.12.1` — queries Shikimori GraphQL endpoint
- Parser: `services/catalog/internal/parser/shikimori/client.go`
- Endpoint: `https://shikimori.one/api/graphql`
- Auth: OAuth2 (Client ID/Secret) — env vars `SHIKIMORI_CLIENT_ID`, `SHIKIMORI_CLIENT_SECRET`
- Rate limiting: Built-in via GraphQL, caching layer (`libs/cache`) with TTL 6 hours for anime details
- GraphQL queries: Search by title (Japanese name primary key), fetch full anime metadata, genres

**Jikan (MyAnimeList GraphQL wrapper):**
- Parser: `services/catalog/internal/parser/jikan/client.go`
- Endpoint: `https://graphql.anilist.co/` (via Jikan)
- Auth: None required
- Purpose: Alternative metadata source, supplementary data
- Rate limiting: Cached via Redis

**Aniwatch API:**
- Docker image: `ghcr.io/ghoshritesh12/aniwatch:v2.19.0` (bundled in docker-compose)
- Port: 3100 (→ internal 4000)
- Purpose: Anime data provider, optional fallback

### Video Source Parsers

Each parser in `services/catalog/internal/parser/{name}/` implements the `AnimeParser` interface:

**Kodik (Russian video streaming):**
- Parser: `services/catalog/internal/parser/kodik/client.go`
- Endpoints: `https://kodik-api.com` (API), `https://raw.githubusercontent.com/nb557/plugins/...` (token source)
- Auth: API key-based token (`KODIK_API_KEY` — env var, optional)
- Output: Embed URLs with translation/episode params (iframe player)
- Subtitle support: No (iframe only)
- Quality control: No (iframe only)
- Caching: Video URLs cached 1 hour

**AnimeLib (Russian video streaming with MP4):**
- Parser: `services/catalog/internal/parser/animelib/client.go`
- API: AnimeLib hapi API (internal reverse engineering)
- Output: MP4 URLs at multiple qualities, OR Kodik iframe fallback
- Video tech: HTML5 `<video>` tag for MP4, Kodik iframe fallback
- Subtitle support: External ASS/VTT files captured from API `subtitles` field
- Quality control: Yes (multiple MP4 bitrates available)
- Caching: 1 hour

**HiAnime (English video streaming with HLS):**
- Parser: `services/catalog/internal/parser/hianime/client.go`
- Output: HLS m3u8 URLs, VTT subtitle tracks
- Video tech: Video.js / HLS.js (switchable)
- Subtitle support: Yes (VTT tracks + JP subtitles via Jimaku.cc via ARM mapping)
- Quality control: Yes (HLS adaptive bitrate)
- Caching: 1 hour

**Consumet (English video streaming with HLS):**
- Parser: `services/catalog/internal/parser/consumet/client.go`
- Docker image: `riimuru/consumet-api:latest` (bundled in docker-compose)
- Port: 3101 (→ internal 3000)
- Output: HLS m3u8 URLs, VTT subtitle tracks
- Video tech: Video.js / HLS.js (switchable)
- Subtitle support: Yes (VTT tracks + JP subtitles via Jimaku.cc via ARM mapping)
- Quality control: Yes (HLS adaptive bitrate)
- Caching: 1 hour

**AniBoom (Video scraper):**
- Parser: `services/catalog/internal/parser/aniboom/client.go`
- Purpose: Scraper for additional streaming sources
- Tech: HTML parsing via goquery

**HANime (Adult anime streaming):**
- Parser: `services/catalog/internal/parser/hanime/client.go`
- Purpose: Specialized source for adult content

### Subtitle Services

**Jimaku.cc (Japanese subtitle files):**
- What it's used for: ASS/SRT/VTT Japanese subtitle files for anime episodes
- SDK/Client: Direct HTTP requests in `services/catalog/internal/parser/jimaku/client.go`
- Endpoint: `https://jimaku.cc/api/...` (internal reverse engineering)
- Auth: API key (`JIMAKU_API_KEY` — env var)
- Output: ASS/SRT/VTT files, parsed by `frontend/web/src/utils/subtitle-parser.ts`
- Frontend component: `frontend/web/src/components/SubtitleOverlay.vue` (custom selectable-text renderer)
- Used by: HiAnime + Consumet players (with ARM AniList ID lookup)
- Caching: Subtitle URLs cached 1 hour

### Anime ID Mapping

**ARM (anime-relations-mapping) API:**
- What it's used for: Map anime IDs between Shikimori/MAL → AniList for JP subtitle lookup
- SDK/Client: `libs/idmapping/` package
- Endpoint: `https://arm.haglund.dev/api/v2`
- Auth: None required
- Query: `GET /ids?source=myanimelist&id={id}` (Shikimori IDs = MAL IDs)
- Response: AniList ID for the same anime
- Used by: Catalog service → HiAnime/Consumet → Jimaku.cc subtitle fetching
- Caching: ID mappings cached via Redis

### External Video Extraction

**Megacloud Extractor (Browser automation):**
- Docker build: `docker/megacloud-extractor/Dockerfile`
- Port: 3200
- Purpose: Extract video URLs from Megacloud (requires browser automation)
- Used by: Aniwatch parser (depends on megacloud-extractor for video extraction)

## Data Storage

**Primary Database:**
- **PostgreSQL 16** (docker-compose service)
- Connection: `postgres:5432` (localhost in development, Docker network in production)
- Credentials: `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` (env vars)
- Client: GORM (via `libs/database/go.mod`)
- Auto-migration: Tables created on service startup via GORM's `AutoMigrate()`
- Schema files: None (schema defined as Go struct tags in domain models)
- Served by: auth, catalog, player, themes, scheduler services

**Object Storage:**
- **MinIO (S3-compatible)**
- Endpoint: `127.0.0.1:9000` (development), env var `MINIO_ENDPOINT` (production)
- Credentials: `MINIO_ROOT_USER=minioadmin`, `MINIO_ROOT_PASSWORD=minioadmin` (dev default)
- SDK: `minio/minio-go/v7 v7.0.67`
- Bucket: `MINIO_BUCKET` (env var)
- Purpose: Store user-uploaded videos, backups
- Used by: streaming service (`services/streaming/internal/handler/upload.go`)
- Production: May use AWS S3 instead (same SDK)

**Cache Layer:**
- **Redis 7** (docker-compose service)
- Connection: `redis:6379` (localhost in development)
- Credentials: None (development default, auth optional in production)
- Client: `redis/go-redis/v9 v9.5.1`
- Wrapper: `libs/cache/` (provides `Cache` interface with TTL management)
- TTL Strategies:
  - Anime details: 6 hours (`cache.TTLAnimeDetails`)
  - Search results: 15 minutes (`cache.TTLSearchResults`)
  - Video URLs: 1 hour (expire when external URLs become stale)
  - ID mappings: 24 hours (ARM API results)
- Used by: All services (auth tokens, API responses, frequently accessed data)

**Optional Search Index:**
- **Meilisearch 1.6** (docker-compose service)
- Port: 7700
- Purpose: Full-text search on anime catalog (optional enhancement)
- Currently: Not heavily integrated, present but not required

**Optional Message Broker:**
- **NATS with JetStream** (docker-compose service)
- Ports: 4222 (NATS), 8222 (HTTP monitoring)
- Purpose: Event-driven architecture, inter-service messaging (optional)
- Status: Bundled but not core to current architecture

## Authentication & Identity

**JWT (JSON Web Tokens):**
- Issuer: `services/auth/`
- Signing key: `JWT_SECRET` (env var)
- Library: `github.com/golang-jwt/jwt/v5`
- Token format: Standard JWT with claims (user ID, roles, exp)
- Validation: All services validate via `libs/authz/` middleware
- Refresh mechanism: Cookies (browser-based, refresh_token HTTP-only cookie)
- Scope: User session, API authentication

**API Keys:**
- Prefix: `ak_` (48 hex characters, e.g., `ak_<32-hex-random>`)
- Storage: SHA-256 hashed in `users.api_key_hash` column
- Validation: Gateway service (`services/gateway/`) resolves `ak_` tokens → mints JWT for downstream
- Purpose: Non-interactive authentication (scheduled jobs, CLI tools, integration testing)
- Test user: `ui_audit_bot` (permanent account with API key for automated audits)
- Endpoints:
  - `POST /api/auth/api-key` — Generate new key (protected)
  - `DELETE /api/auth/api-key` — Revoke key (protected)
  - `GET /api/auth/api-key` — Get current key hash (protected)
  - `POST /internal/resolve-api-key` — Gateway internal endpoint to resolve key → user

**OAuth2 (Shikimori):**
- Provider: Shikimori.one
- Client credentials: `SHIKIMORI_CLIENT_ID`, `SHIKIMORI_CLIENT_SECRET` (env vars)
- Purpose: Authorize catalog service to query Shikimori API
- Flow: Client credentials grant (service-to-service, not user OAuth)

## Monitoring & Observability

**Error Tracking & Reporting:**
- Frontend error reporting: `ReportButton` component in all 4 video players
- Backend storage: `services/player/internal/handler/report.go` — saves JSON reports to disk
- Volume: `player_reports:/data/reports/` (persistent Docker volume)
- Notification: Telegram bot (admin chat)
- Capture: Console logs, network requests, page HTML via `frontend/web/src/utils/diagnostics.ts`

**Metrics (Prometheus):**
- Exporter: All services expose `/metrics` endpoint
- Scraper: `prometheus:9090` (docker-compose service)
- Retention: 15 days
- Metrics captured:
  - `http_requests_total` — request count (service, method, path, status)
  - `http_request_duration_seconds` — latency histogram (p50/p95/p99)
  - `http_response_size_bytes` — response size histogram
  - Custom: `search_requests_total` (by source, `libs/metrics/`)
- Dashboard: Grafana (`grafana:3000` → reverse proxied at `/admin/grafana`)

**Logs (Loki + Promtail):**
- Log aggregator: `loki:3100` (docker-compose service)
- Log shipper: `promtail:2.9.4` — reads Docker container logs, ships to Loki
- Config: `docker/loki/loki-config.yml`, `docker/promtail/config.yml`
- Retention: Loki default (configurable)
- Query: Via Grafana Explore tab (Loki data source)

**Tracing (OpenTelemetry):**
- Library: `go.opentelemetry.io/otel`, `otlptrace/otlptracegrpc` exporter
- Setup: `libs/tracing/` package
- Export target: Not specified in current config (OTLP receiver endpoint via env var, optional)
- Span context: Propagated across service boundaries (W3C Trace Context)

**Admin Notifications:**
- **Telegram Bot**
- Recipient: Admin chat ID (`TELEGRAM_ADMIN_CHAT_ID` env var)
- Triggered by: Error reports from frontend (via `ReportButton`)
- Implementation: `services/player/internal/handler/report.go`

## CI/CD & Deployment

**Hosting:**
- Primary: Docker Compose (development), Kubernetes (production)
- Kubernetes manifests: `/deploy/kustomize/`
- Helm charts: Optional, kubectl 1.29.0 required

**Build System:**
- Local dev: `Makefile` (targets: `make dev`, `make redeploy-<service>`, `make logs-<service>`)
- Docker: Multi-stage Dockerfiles (Go builder + Alpine runtime)
- Container registry: Not specified (likely Docker Hub or private registry)

**No explicit CI platform configured** (GitHub Actions, GitLab CI, etc. not evident in repo). Deployment is manual or via orchestration tools.

## Webhooks & Callbacks

**Incoming (from external services to AnimeEnigma):**
- None explicitly configured

**Outgoing (from AnimeEnigma to external services):**
- Telegram notifications (error reports)
- S3/MinIO video uploads (backup service)

## Internal Service-to-Service Communication

**HTTP (default):**
- All services communicate via HTTP/REST
- Router: `go-chi/chi/v5` (Chi v5.0.12 — consistent across all services)
- Gateway pattern: Central `gateway` service (port 8000) routes incoming requests
- Gateway routing:
  - `/api/auth/*` → auth:8080
  - `/api/anime/*`, `/api/genres`, `/api/admin/*` → catalog:8081
  - `/api/kodik/*` → catalog:8081
  - `/api/streaming/*` → streaming:8082
  - `/api/users/*` → player:8083
  - `/api/rooms/*`, `/api/game/*` → rooms:8084
  - `/api/themes/*` → themes:8086

**WebSocket:**
- Used by: rooms service for real-time game rooms + chat
- Library: `gorilla/websocket v1.5.1` (`services/rooms/go.mod`)
- Connection: Persistent, authenticated via JWT or session cookie

**gRPC (optional):**
- Protocol Buffers defined in `api/proto/` (`.proto` files), but not currently used for inter-service communication
- Proto files present: `api/proto/catalog.proto`, `api/proto/common.proto`, `api/proto/streaming.proto`
- Status: Defined but not activated (HTTP/REST is current transport)

## Environment Configuration

**Required env vars (all services):**
```
DB_HOST=postgres
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=animeenigma
REDIS_HOST=redis
REDIS_PORT=6379
JWT_SECRET=<random-32-char-secret>
```

**Catalog service specific:**
```
SHIKIMORI_CLIENT_ID=<from shikimori.one>
SHIKIMORI_CLIENT_SECRET=<from shikimori.one>
JIMAKU_API_KEY=<optional, for JP subtitles>
KODIK_API_KEY=<optional, for Kodik parser>
```

**Streaming service specific:**
```
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=videos
```

**Admin/Notifications:**
```
TELEGRAM_ADMIN_CHAT_ID=<Telegram user ID>
GRAFANA_ADMIN_PASSWORD=<dashboard admin password>
GRAFANA_WEBHOOK_PASS=<for alerting webhooks>
```

**Testing:**
```
UI_AUDIT_API_KEY=<for ui_audit_bot permanent test user>
```

**Secrets location:**
- Development: `docker/.env` (not committed)
- Example template: `docker/.env.example`
- Production: Kubernetes Secrets or managed secret store (not shown in repo)

## Video Streaming Infrastructure

**Frontend Proxy (Backend HLS/MP4 proxy):**
- Purpose: CORS workaround for external HLS/MP4 streams
- Implementation: `libs/videoutils/proxy.go` (streaming service)
- Allowed domains: Streaming CDNs, `jimaku.cc`, `cdnlibs.org` (AnimeLib), others
- Used by: HiAnime, Consumet, AnimeLib players
- Caching: Transparent pass-through (no content caching, stream forwarded directly)

**Player Routing:**
1. **Kodik** (iframe) — No proxy needed, user browser embeds iframe directly
2. **AnimeLib** (MP4 or Kodik fallback) — MP4 via backend proxy, Kodik iframe fallback if unavailable
3. **HiAnime** (HLS) — m3u8 + TS segments via backend HLS proxy
4. **Consumet** (HLS) — m3u8 + TS segments via backend HLS proxy

---

*Integration audit: 2026-04-27*
