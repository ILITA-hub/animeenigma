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

## UI Audit Test User (DO NOT DELETE)

Permanent test account for automated UI/UX audits, integration tests, and Playwright e2e tests:

- **Username**: `ui_audit_bot`
- **Public ID**: `ui-audit-bot`
- **Profile URL**: `https://animeenigma.ru/user/ui-audit-bot`
- **API Key**: stored in `docker/.env` as `UI_AUDIT_API_KEY` (not committed)
- **Seeded with**: 8 anime_list entries (mixed statuses), 3 watch_history rows, 3 theme_ratings
- **Password login is enabled** — `audit_bot_test_password_2026` (set 2026-04-07 so audits can use the standard `/api/auth/login` flow with refresh-cookie semantics). Treat this as an automation account, not a human one.

**Permanent infrastructure** — recreating loses seeded state and breaks any e2e tests depending on stable IDs. Re-seeding is idempotent via `scripts/seed-ui-audit-user.sh`.

To use in tests:
```bash
curl -H "Authorization: Bearer $UI_AUDIT_API_KEY" https://animeenigma.ru/api/...
```

To refresh stale data (e.g. before a new audit):
```bash
./scripts/seed-ui-audit-user.sh
```

To rotate the API key (lost the previous one):
```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma \
  -c "UPDATE users SET api_key_hash = NULL WHERE username = 'ui_audit_bot';"
./scripts/seed-ui-audit-user.sh
# New key printed in the banner — save to docker/.env
```

## UI/UX Audit Framework

Use this framework for any future UI/UX audit on AnimeEnigma. Reference: first audit `docs/issues/ui-audit-2026-04-07.md`.

### Methodology

Combined approach (no single technique catches everything):

1. **Static heuristic review** — Nielsen's 10 heuristics applied to screenshots + DOM
2. **Automated a11y scan** — axe-core injected via CDN script tag (loads cleanly under animeenigma.ru's CSP) and `axe.run()` over the document
3. **Per-view interaction probe** — Tab×5 (focus visibility), Esc, scroll, mid-view resize, back-nav, click below fold. Catches interaction bugs static screenshots miss.
4. **Realistic user scenarios** — drive 4-6 end-to-end flows that mirror what real users actually do (search → watch, list management, resume from history, switch player). Catches workflow gaps static review misses.
5. **Cross-view consistency sweep** — compare button styles, modal patterns, loading skeletons, focus rings across all captured views

### Tooling

- **Browser:** Chrome MCP (`mcp__claude-in-chrome__*`)
  - One browser session, no true parallelism — but parallelize JS inspection across multiple tabs in single tool-call batches
  - Use `update_plan` first to register intended domains (one-time consent)
- **axe-core:** load from `https://cdnjs.cloudflare.com/ajax/libs/axe-core/4.10.2/axe.min.js` via injected `<script>` tag in `javascript_tool` (verified to bypass CSP on animeenigma.ru)
- **Auth:** log in as `ui_audit_bot` via `fetch('/api/auth/login', ...)` from inside the page so the refresh cookie gets set properly. Inject the JWT into `localStorage.token` and the user object into `localStorage.user` (matches `frontend/web/src/stores/auth.ts:42-43`)
- **Token discipline (mandatory):**
  - `read_page` → always pass `filter: "interactive"`, `depth: 8`, `max_chars: 20000`
  - `read_console_messages` → always pass `pattern`, `limit: 30`, `clear: true`
  - `read_network_requests` → always pass `urlPattern`, `limit: 30`, `clear: true`
  - Write findings to disk view-by-view, not at end (context survives crashes)

### Severity scale (Nielsen 0-3)

| Level | Meaning | Weight for per-view score |
|---|---|---|
| 0 — Cosmetic | Fix only if extra time | 0 |
| 1 — Minor | Low priority | 1 |
| 2 — Major | High priority | 2 |
| 3 — Catastrophic | Must fix before next release | 3 |

Per-view score = `3*catastrophic + 2*major + 1*minor`. Replaces a raw count which would always make Watch.vue look worst by virtue of being the most complex.

### Citation rules (no hallucinated line numbers)

Every finding cites code as `(file_path — found via grep "anchor string")`. The anchor string MUST be a verbatim string from a `Grep` call performed in the same audit pass.

✅ `frontend/web/src/views/Home.vue — found via grep "Continue Watching"`
❌ `frontend/web/src/views/Home.vue:142` (line number from memory — forbidden)

If a finding can't be anchored to a real grep result, the citation is `(no code anchor found — visual evidence only)`.

### Per-finding template

```markdown
##### [UA-NNN] One-line title — Severity N (label) — category

**View:** Which view + viewport
**Heuristic:** Nielsen #N or category
**Evidence:**
- Concrete observation 1 (DOM query, axe rule, screenshot reference)
- Concrete observation 2
- Cross-reference to seeded data / DB state if relevant

**Why it matters:**
1. User-impact statement
2. Accessibility / SEO / consistency / etc.

**Citations:**
- `path/to/file.vue — found via grep "anchor"`

**Proposed fix:** Concrete steps the implementer can take.
```

### Realistic user scenarios (drive these in addition to per-view static audit)

For AnimeEnigma specifically, the highest-value scenarios are:

**Navigation scenarios:**
- N1: Search for a specific anime → open detail → start watching
- N2: Browse by genre filter → paginate → open detail
- N3: Mobile: switch between Home / Browse / Profile via the hamburger or persistent nav

**List management scenarios:**
- L1: From anime detail, add to watchlist with status "watching"
- L2: From watchlist view, change status (watching → completed)
- L3: View watchlist filtered by status, sort by score

**Watching scenarios:**
- W1: Anonymous: visit Watch view → does the player load without auth?
- W2: Logged in: resume an in-progress episode from watch history
- W3: Switch player or translation mid-episode (Kodik = limited, others = full control)

Each scenario gets its own findings sub-section with friction points, dead ends, missing affordances, and any observed errors.

### Output structure

Single markdown file: `docs/issues/ui-audit-YYYY-MM-DD.md` with sections:
- Header (site, locale, account, methodology, scope, tooling)
- Summary (counts, weighted scores, top quick wins, top high-impact fixes)
- Findings by severity (catastrophic → major → minor → cosmetic)
- Findings by view (same items, regrouped for "fix this view" sessions)
- Realistic-scenario findings (one section per scenario)
- axe-core raw output per view
- Cross-view inconsistencies
- Audit notes (token budget consumed, transient findings filtered, bot detection, truncations)

### Checkpoints (3 mandatory user gates)

1. After dry-run on first view — validate format
2. After all per-view audits complete (before scenarios) — confirm on track
3. Before commit/push — final review for redaction, accuracy, completeness

### Don't do

- Don't cite line numbers from memory — only from grep calls in the same pass
- Don't mark a finding as "real" if it only reproduces once (transient — must repro on 2+ navigations)
- Don't bypass bot detection — abort the audit and report which view tripped it
- Don't auto-invoke `animeenigma-after-update` after the audit — wait for explicit user "ship it"
- Don't create accounts in production for the audit; reuse `ui_audit_bot`

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

### Pinning anime to the top of listings (sort_priority)

The `Anime` model has a `SortPriority int` field (default 0). Browse/search results are ordered by `sort_priority DESC, score DESC`, so any anime with `sort_priority > 0` appears before others.

**To pin an anime to the top of a listing (e.g. announcements):**

```sql
-- Via direct DB (replace shikimori_id with target anime)
UPDATE animes SET sort_priority = 1 WHERE shikimori_id = '57466';

-- To unpin
UPDATE animes SET sort_priority = 0 WHERE shikimori_id = '57466';
```

Higher values = higher position. Use `sort_priority = 1` for a single pin; use `2, 1` etc. to control relative ordering among pinned items.

This affects ALL browse queries (announcements, ongoing, released, search results).

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

## After-Update Skill (MUST USE)

After completing any implementation work (features, bug fixes, refactoring), **always invoke** `/animeenigma-after-update` before ending the conversation. This skill:

1. Lints and builds the affected code
2. Redeploys changed services via `make redeploy-<service>`
3. Runs health checks
4. Updates `frontend/web/public/changelog.json` with user-facing changelog entries (informative + enthusiastic tone with emojis) — this is what `LastUpdates.vue` loads in the Changelog tab
5. Commits all changes with co-authors and pushes to remote

**Do not skip this step.** It ensures every implementation is deployed, verified, documented for users, and pushed.

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
