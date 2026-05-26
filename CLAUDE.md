# CLAUDE.md - Project Guidelines for AI Assistants

## Project Overview

AnimeEnigma is a self-hosted anime streaming platform with Shikimori/MAL integration. It uses a Go microservices architecture with a Vue 3 frontend.

**Target deployment**: Small self-hosted groups (no CDN required).

## Architecture Principles

### Video Player Architecture

The platform has 5 video players, each targeting different source APIs:

| Player | Lang | Component | Video Tech | Tracking | JP Subs | Quality |
|--------|------|-----------|-----------|----------|---------|---------|
| **Kodik** | RU | `KodikPlayer.vue` | Kodik iframe | No | No | No (iframe) |
| **AniLib** | RU | `AnimeLibPlayer.vue` | HTML5 `<video>` (MP4) | Yes | No | Yes (MP4) |
| **OurEnglish** | EN | `OurEnglishPlayer.vue` | HTML5 `<video>` + hls.js (HLS) or MP4 via backend HLS proxy | Planned | Yes (via SubtitleOverlay) | Yes |
| **Hanime** | 18+ | `HanimePlayer.vue` | HTML5 `<video>` + hls.js | Yes | No | Yes |
| **Raw** | JP | `RawPlayer.vue` | HTML5 `<video>` + hls.js (HLS) or MP4 (AllAnime `fast4speed.rsvp`) | No | Yes (Jimaku + others) | Yes |

> **OurEnglish** (shipped 2026-05 across v3.1 Phases 24–28) replaces the May-2026 removal of HiAnime + Consumet. Single user-facing surface; the backend `services/scraper/` microservice failovers across **gogoanime → animepahe → allanime → animefever → miruro → nineanime** (+ optional `animekai`). The in-player **Source** dropdown lets users pin a specific provider; default is auto. Behind `VITE_OURENGLISH_ENABLED` (defaults on) so it can be dark-shipped via env override if needed.

**Backend route family** (gateway → catalog → scraper microservice):
- `GET /api/anime/{uuid}/scraper/episodes?prefer=<provider>`
- `GET /api/anime/{uuid}/scraper/servers?episode=<id>&prefer=<provider>`
- `GET /api/anime/{uuid}/scraper/stream?episode=<id>&server=<id>&category=sub|dub&prefer=<provider>`
- `GET /api/anime/_/scraper/health`

**Shared components (reused across players):**
- `SubtitleOverlay.vue` — Custom selectable-text JP subtitle renderer (ASS/SRT/VTT). Teleports to fullscreen element, time-synced via `requestAnimationFrame`.
- `subtitle-parser.ts` — Parses ASS (via `ass-compiler`), SRT, VTT into `SubtitleCue[]`
- `OtherSubsPanel.vue` — Aggregated subtitle picker (Jimaku, OpenSubtitles, etc.)
- `ReportButton.vue` — Per-stream user-reportable error path; persists to disk + Telegram admin notification
- `libs/videoutils/proxy.go` — Backend HLS proxy for CORS. Structured `HLSProxyAllowedDomainsWithProvenance` allowlist with quarterly-review provenance fields. Covers streaming CDNs, `jimaku.cc`, `cdnlibs.org` (AniLib), `kwik.cx` (AnimePahe), `fast4speed.rsvp` (AllAnime), `am.vidstream.vip` + `static-cdn-ca1.mofl.pro` (AnimeFever), `pro.ultracloud.cc` + `pru.ultracloud.cc` (Miruro), `my.1anime.site` (9anime), Hanime CDN families (`hanime.tv`, `htv-*`, `hydaelyn-*`, `zodiark-*`).

**Known issue:** AniLib subtitles are broken — direct MP4 player can't render soft-subs embedded in the video.

### Video Streaming Model

Videos are sourced in four ways:
1. **Kodik iframe** — Frontend embeds Kodik's player iframe (no direct video control)
2. **Backend proxy/restream** — Backend proxies MP4/HLS streams from AniLib, AnimePahe (Kwik), AllAnime (`fast4speed.rsvp`), AnimeFever, Miruro, 9anime, and Hanime CDNs through `services/streaming` HLS proxy for CORS + Referer injection
3. **Self-hosted storage** — Admin-uploaded videos stored in MinIO
4. **Stealth-Chromium sidecar** — `services/animepahe-resolver/` (Phase 27) solves DDoS-Guard on `animepahe.pw` so the Go scraper can hit the upstream API; sidecar is internal-only, capped at 500 MB RSS

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
- **AniLib** — RU video streaming (direct MP4). Parser: `services/catalog/internal/parser/animelib/`
- **OurEnglish (`services/scraper/`)** — EN video streaming via failover orchestrator. Providers (in order): `gogoanime` → `animepahe` (Kwik via stealth sidecar) → `allanime` → `animefever` → `miruro` (secure-pipe pure-Go obfuscation) → `nineanime` (MP4-only last-resort) → optional `animekai`. Provider impls live at `services/scraper/internal/providers/{name}/`. Embed extractors at `services/scraper/internal/embeds/`.
- **Hanime** — 18+ video streaming. Parser: `services/catalog/internal/parser/hanime/`
- **AllAnime raw-JP** — Original-audio JP video (Raw player). Library/parser: `services/catalog/internal/parser/allanime/` (+ `services/scraper/internal/providers/allanime/`).
- **Jimaku.cc** — Japanese subtitle files (ASS/SRT/VTT). Consumed by `OurEnglish`, `Raw` players via `SubtitleOverlay.vue`.
- **ARM** (`arm.haglund.dev`) — Anime ID mapping (Shikimori/MAL → AniList). Library: `libs/idmapping/`
- **MAL** (optional) — Additional metadata, ratings sync

## Code Conventions

### Effort & impact metrics — NO days, hours, sprints

**Time-effort units are not used in this project.** Every plan/feature/CHANGELOG entry scored on three dimensional metrics — full spec at `.planning/CONVENTIONS.md`:

- **UXΔ** (UX Delta) — signed `-5..+5` with `Better`/`Worse`/`Ambiguous` label. e.g. `UXΔ = +2 (Better)`
- **CDI** (Coherence Disruption Index) — two numbers: `(Spread × Shift) * Effort_Fib`. e.g. `CDI = 0.02 * 13`. Effort on Fibonacci scale (1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233+). DO NOT pre-multiply.
- **MVQ** (Mythic Vibe Quotient) — creature + match%/slop-resistance%. e.g. `MVQ = Griffin 85%/80%`. Creatures: Phoenix / Griffin / Kraken / Sprite / Basilisk / Dragon.

Sub-agents that produce plans (gsd-planner, gsd-discuss-phase, gsd-roadmapper, etc.) MUST follow this convention. Reject any plan that returns "N days" — re-score and re-submit.

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

Gateway service specific (WV3-T3 — per-user rate limit):
```
RATE_LIMIT_RPS               # per-IP RPS, default 100   (existing)
RATE_LIMIT_BURST             # per-IP burst, default 200 (existing)
USER_RATE_LIMIT_PER_MINUTE   # per-authenticated-user GCRA rate, default 60
USER_RATE_LIMIT_BURST        # per-authenticated-user GCRA burst, default 10
REDIS_ADDR                   # default redis:6379 — gateway now uses Redis for the per-user limiter
NOTIFICATIONS_SERVICE_URL    # default http://notifications:8090 — gateway proxies /api/notifications/* here
```

The per-user limit (`USER_RATE_LIMIT_*`) is layered on top of the per-IP
limit and only applies AFTER auth (anonymous traffic stays per-IP-limited).
A Redis outage fails open (logs WARN, lets the request through) so a Redis
blip cannot 500 every authenticated request. Blocked-request count is
exposed at `/metrics` as `gateway_rate_limit_user_blocked_total` (no labels).

Notifications service specific (workstream notifications, v1.0 Phase 1):
```
CATALOG_URL                  # default http://catalog:8081 — Phase 2 detector calls catalog's /internal/anime/{id}/episodes
```
The notifications service uses the standard `DB_*` + `JWT_SECRET` + `REDIS_HOST`
trio. Internal producer endpoint `POST /internal/notifications` is reachable
only inside the Docker network — the gateway does not proxy `/internal/*`.

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

### Adding a Spotlight Card Type

The home page's `HeroSpotlightBlock` (workstream `hero-spotlight`) is a 9-card rotating carousel. To add a 10th card type, touch the following 5 anchors (all from the same package boundaries, ~50 lines total):

1. **Backend resolver** — Create `services/catalog/internal/service/spotlight/cards/{new_type}.go` implementing the `spotlight.Resolver` interface (`Type()` + `Resolve(ctx, userID *string) (*spotlight.Card, error)`). Mirror `anime_of_day.go`'s pattern: manual `cache.Get`/`cache.Set` with `errors.Is(err, cache.ErrNotFound)`, return `(nil, nil)` for ineligible, `(nil, err)` for failure, `(*Card, nil)` for success. Multi-item resolvers MUST apply `spotlight.AdaptiveSlice` (the 1-2-3 layout rule). Login-only resolvers return `(nil, nil)` when `userID == nil`. Always carry the `spotlight:` Redis key prefix for new keys (HSB-NF-03). Add a co-located `_test.go` with handwritten fakes — no testify/mock.

2. **Backend Data type** — Add the JSON-shaped `{NewType}Data` struct to `services/catalog/internal/service/spotlight/types.go` extending the Card union. Update `types_test.go` with a round-trip marshal/unmarshal test.

3. **Backend DI** — Add a `cards.New{NewType}Resolver(...)` call to the `spotlightResolvers` slice in `services/catalog/cmd/catalog-api/main.go`. Stable order matters for tie-breaking display order.

4. **Frontend SFC** — Create `frontend/web/src/components/home/spotlight/cards/{NewType}Card.vue` accepting a typed `data` prop (use the new TypeScript variant from step 5). Honor the UI-SPEC contract: ONLY `font-medium` / `font-semibold` weights, `p-4 md:p-6 lg:p-8` padding, Tailwind utility-only, `min-h-[400px] md:min-h-[340px] lg:min-h-[320px]` height. Add `target="_blank"` + `rel="noopener noreferrer"` on any external anchor. Co-locate a `.spec.ts` with at least 5 Vitest assertions.

5. **Frontend dispatch + i18n + types** —
   - Extend the `SpotlightCard` discriminated union in `frontend/web/src/types/spotlight.ts` with the new `{ type: '{new_type}', data: {NewType}Data }` variant.
   - Add a `v-else-if="active.type === '{new_type}'"` branch to the dispatch chain in `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` (DO NOT switch to `<component :is>` — keep the typed chain so vue-tsc narrows props).
   - Add a new `spotlight.{newType}.*` sub-namespace to BOTH `frontend/web/src/locales/en.json` and `frontend/web/src/locales/ru.json`. The parity test at `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` will fail if any key is added to one file but not the other.

Verify the full stack with: `cd services/catalog && go test ./internal/service/spotlight/... -count=1 -race` and `cd frontend/web && bunx vitest run src/components/home/spotlight/ src/locales/__tests__/spotlight-keys.spec.ts && bunx tsc --noEmit`. The Phase 3 end-to-end Playwright spec `frontend/web/e2e/spotlight.spec.ts` is the canonical regression test for the full carousel.

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
bunx playwright test player                    # Run a specific spec by name
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
| library    | 8089 | /metrics  | Library service (BitTorrent → HLS → MinIO, admin-only) |
| notifications | 8090 | /metrics | Generic notification engine (new episodes, future types) |
| watch-together | 8091 | /metrics | Co-watch service (Redis-only; rooms + sync + chat) |
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
- `/api/library/*` → library:8089 (admin-only; routes added incrementally in v0.2 Phases 2–5)
- `/api/notifications/*` → notifications:8090 (JWT required; internal `/internal/notifications` NOT exposed — Docker-network-only)
- `/api/watch-together/*` → watch-together:8091 (JWT required for HTTP; WS uses `?token=` query param since browsers can't set custom headers on WS upgrade)

### Watch Together

Watch Together — ephemeral private friend rooms (2-10 members) for synchronized anime watching across all 5 players. The Watch Together v1.0 milestone shipped 2026-05 across 5 phases (backend foundation → frontend shell → player sync → state switching → polish). State is Redis-only with sliding 15min TTL + 5min last-disconnect grace.

**Architecture:**
- Single Go microservice `services/watch-together/` (port 8091) — no Postgres, no migrations, Redis-only state under the `wt:` key prefix.
- REST for room lifecycle (POST/GET/DELETE `/rooms`), WebSocket at `/ws?token=&room=` for real-time sync/chat/reactions/state-changes.
- All 10 inbound + 10 outbound message types defined in `services/watch-together/internal/domain/ws_message.go` (protocol_version `"1.0"`, forward-compat field on every snapshot).
- Drift detection engine with soft (>1.5s) / hard (>5s) / persistent (5 consecutive) thresholds; per-recipient `playback:correction` envelopes.
- In-process per-user rate limits (1 seek/s, 5 chat/s) via `golang.org/x/time/rate` token buckets. v2 horizontal-scale will need a Redis-backed limiter (deferred).
- State validation (episode/player/translation switches): synchronous call to catalog's `/internal/anime/{id}/episodes/validate` with 3s timeout + 5s positive-result cache. Permissive for ourenglish/hanime/raw (v1.1 will tighten).
- 5min grace timer (`internal/service/grace.go`): last-member-disconnect starts a `time.AfterFunc`; returning member cancels; timer fire broadcasts `room:closed` + deletes Redis keys.

**HTTP + WS surface (gateway-routed):**
- `POST   /api/watch-together/rooms` — create room (JWT required)
- `GET    /api/watch-together/rooms/{id}` — full RoomSnapshot or 410 Gone (JWT required)
- `DELETE /api/watch-together/rooms/{id}` — host-only force-close (broadcasts `room:closed` then deletes)
- `WS     /api/watch-together/ws?token=<jwt>&room=<id>` — bidirectional sync channel

**Frontend:**
- Route `/watch/room/:roomId` → `WatchTogetherView.vue` (chunk ~6.6 kB gz, lazy)
- Composable `useWatchTogetherRoom(roomId)` owns WS lifecycle + reconnect backoff + snapshot replay + 9 emit methods + 10 subscribe methods (`onPlaybackEvent`, `onStateChanged`, `onRoomClosed`, `onAuthExpired`, `onError`, …)
- Player sync via `usePlayerSyncBridge(videoRef, room)` for HTML5 players (AnimeLib/OurEnglish/Hanime/Raw); Kodik via `kodik_player_api` postMessage RPC adapter with boot-time smoke probe + daily Playwright canary at `frontend/web/e2e/kodik-rpc-probe.spec.ts`.
- Reaction whitelist: 24 emoji declared in both backend (`internal/service/inbound.go:reactionWhitelist`) AND frontend (`@/types/watch-together.REACTION_WHITELIST`); MUST update both sides in lock-step.

**Env vars (set in `docker/.env`):**

| Var | Default | Purpose |
|-----|---------|---------|
| `WATCH_TOGETHER_PORT` | 8091 | Service listen port |
| `WATCH_TOGETHER_REDIS_ADDR` | redis:6379 | Redis backend |
| `WATCH_TOGETHER_JWT_SECRET` | `${JWT_SECRET}` | JWT validation; same secret as auth service |
| `WATCH_TOGETHER_MAX_MEMBERS` | 10 | Per-room capacity cap |
| `WATCH_TOGETHER_ROOM_TTL` | 15m | Sliding TTL on `wt:room:*` keys |
| `WATCH_TOGETHER_GRACE_PERIOD` | 5m | Last-disconnect grace before delete |
| `WATCH_TOGETHER_PUBLIC_BASE_URL` | https://animeenigma.ru | Used to construct invite URLs |
| `WATCH_TOGETHER_ALLOW_ALL_ORIGINS` | false | Dev override for WS origin allowlist |
| `WATCH_TOGETHER_CATALOG_URL` | http://catalog:8081 | Catalog HTTP back-channel for state validation |

The gateway side reads `WATCH_TOGETHER_SERVICE_URL` (default `http://watch-together:8091`) — separate var so the gateway can be redeployed without touching the watch-together service's own env.

**Locked decisions (across all 5 phases):**
- Port 8091, Redis-only, `wt:` key prefix, 15min sliding TTL, 5min grace, capacity 10 members.
- WS auth: `?token=` query param (browsers can't set `Authorization: Bearer` on WS upgrade).
- Pre-upgrade rejections use HTTP 401/400/404 (NOT close frames) for debuggability.
- DELETE `/rooms` broadcasts `room:closed` BEFORE deleting Redis keys (Plan 05.1 closed the original 01.4 TODO).
- 24-emoji reaction whitelist — must be reconciled across backend `internal/service/inbound.go:reactionWhitelist` and frontend `@/types/watch-together.REACTION_WHITELIST`.
- Permissive episode validation for ourenglish/hanime/raw (v1.1 will tighten via scraper round-trip; v1.0 trusts user selection).
- Window test hook `__wtTestRoom` is exposed via `VITE_TEST_HOOK` in dev/test builds only — NEVER ship in production builds.

**References:**
- Design doc: [`docs/superpowers/specs/2026-05-25-watch-together-design.md`](docs/superpowers/specs/2026-05-25-watch-together-design.md)
- Workstream: [`.planning/workstreams/watch-together/`](.planning/workstreams/watch-together/) (v1.0 milestone, 5 phases)
- Phase summaries: [Phase 1](.planning/workstreams/watch-together/phases/01-backend-foundation/01-PHASE-SUMMARY.md) · [Phase 2](.planning/workstreams/watch-together/phases/02-frontend-shell/02-PHASE-SUMMARY.md) · [Phase 3](.planning/workstreams/watch-together/phases/03-player-sync/03-PHASE-SUMMARY.md) · [Phase 4](.planning/workstreams/watch-together/phases/04-state-switching/04-PHASE-SUMMARY.md) · [Phase 5](.planning/workstreams/watch-together/phases/05-polish/05-PHASE-SUMMARY.md)
- Grafana dashboard: `infra/grafana/dashboards/watch-together.json` (auto-provisioned; UID `watch-together`)
- Kodik RPC reference: `reference_kodik_inbound_postmessage_api.md` (user memory)

**Dependency audit (WT-NF-05):**

Backend (`services/watch-together/go.mod` direct requires — verified 2026-05-26 against go.mod):
- `github.com/gorilla/websocket` — WS lib
- `github.com/redis/go-redis/v9` — Redis client
- `github.com/go-chi/chi/v5` — HTTP router
- `github.com/golang-jwt/jwt/v5` — JWT validation (direct, NOT via a `libs/jwt` wrapper)
- `golang.org/x/time/rate` — token buckets
- `github.com/google/uuid` — instance/room IDs
- `github.com/prometheus/client_golang` + `client_model` — metrics (already used project-wide)
- `github.com/alicebob/miniredis/v2` — test-only Redis fake
- Project libs reused: `libs/authz`, `libs/cache`, `libs/errors`, `libs/httputil`, `libs/logger`, `libs/metrics`

Every direct require is either a project-default already in use elsewhere in `services/` (`chi`, `prometheus`, `uuid`, `go-redis`, `golang-jwt`) or a thin, well-known utility (`gorilla/websocket`, `golang.org/x/time`, `miniredis`). No license-incompatible deps; all licenses are MIT/BSD/Apache-compatible with the project (MIT).

Frontend: ZERO new npm dependencies introduced by Watch Together across all 5 phases. Verified via `git log --since='2026-05-20' frontend/web/package.json` — the only diff in that window is `@axe-core/playwright` from the unrelated hero-spotlight workstream. All UI uses pre-existing vue 3 + pinia + vue-i18n + vue-router + tailwind + `@vueuse/core` + `hls.js` + `ass-compiler`.

Audited 2026-05-26 (close of v1.0).

**Daily Kodik canary CI:**

The Kodik postMessage RPC is undocumented; the bundle could change at any time. A Playwright spec at `frontend/web/e2e/kodik-rpc-probe.spec.ts` runs daily via GitHub Actions (`.github/workflows/watch-together-kodik-canary.yml`) and alerts via Telegram on failure. If you see this alert: confirm `kodik_player_api` still works in browser DevTools by sending `{key:'kodik_player_api',value:{method:'get_time'}}` to a Kodik iframe; if the dispatcher really has changed, all rooms using Kodik will fall back to the "Kodik sync unavailable" banner mode — single-user playback still works.

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
