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
- `libs/videoutils/proxy.go` — Backend HLS proxy for CORS. **Auth gate is `allowlisted OR signed`** (`proxy.go:465-470`): catalog's `GetScraperStream` signs scraper-resolved stream/subtitle URLs (`videoutils.SignStreamURL`), and the proxy mints HMAC provenance tokens for the rotating child/segment CDNs — so **scraper CDN hosts are auto-trusted and need NO allowlist entry**. The structured `HLSProxyAllowedDomainsWithProvenance` list (quarterly-review provenance fields) is now a **legacy fallback being phased out**; it still covers non-scraper CDNs + older entries (`jimaku.cc`, `cdnlibs.org` (AniLib), `kwik.cx` (AnimePahe), `fast4speed.rsvp` (AllAnime), `am.vidstream.vip` + `static-cdn-ca1.mofl.pro` (AnimeFever), `pro.ultracloud.cc` + `pru.ultracloud.cc` (Miruro), `my.1anime.site` (9anime), Hanime CDN families `hanime.tv`/`htv-*`/`hydaelyn-*`/`zodiark-*`). Do NOT add new scraper CDNs here — rely on signing.

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

### Design System (Neon Tokyo, shadcn-vue)

Canonical reference: **`frontend/web/src/styles/DESIGN-SYSTEM.md`** (token tiers, usage rules, component inventory). Do NOT duplicate it — read it before touching frontend styling. Bind to semantic tokens; never hardcode colors.

**Lint gate (build-ENFORCED).** `frontend/web/scripts/design-system-lint.sh` runs as a prerequisite of `make lint-frontend` (→ `make lint` / CI) AND `make redeploy-web` (deploy gate). `ERRORS>0 ⇒ exit 1` — these FAIL THE BUILD. It enforces 8 color/token/typography/spacing/primitive rules over `frontend/web/src/**/*.vue` (excludes `*.spec.*` / `__tests__`; RULES 1 & 4 also scan `*.ts`):

1. **No off-palette Tailwind color classes** — `(text|bg|border|ring|from|to|via|fill|stroke|placeholder|divide|outline|decoration|shadow)-(red|amber|yellow|emerald|green|blue|sky|purple|violet|gray|slate|zinc)-(50…975)` (shade group now includes `925|950|975`). Migrate to a semantic token (`text-destructive`, `bg-warning`, `text-success`, `text-info`, `text-muted-foreground`, …). **EXEMPT brand/provider identity hues** (deliberately absent from the set, NOT forbidden): `cyan`, `pink`, `orange`, `rose`, `indigo`, `teal`, `lime` (Neon-Tokyo brand + per-provider accents — Kodik cyan, AniLib orange, Hanime pink, Raw rose). Also scans `*.ts` (class-strings leak via `cva` variant files), **excluding `*-variants.ts`** (the canonical semantic-variant defs).
2. **No hardcoded hex outside the allowlist** — any raw `#[0-9a-fA-F]{3,8}` in a `.vue` not listed (per `(file,hex)`) in `frontend/web/scripts/design-system-allowlist.txt`. `.vue` only — `.ts` hex is intentional brand/provider color data (e.g. `providerRegistry.ts`).
3. **No deprecated-alias `var()` usages** — `var(--ink)` / `var(--accent)` / `var(--pink)` / `var(--violet)` / `var(--f-display)` / `var(--f-ui)` / `var(--f-mono)` / `var(--f-jp)`. Migrate to the canonical token (`--brand-violet`, `--font-display`, `--font-sans`, `--font-mono`, `--font-jp`). (Survivors `--ink-2`, `--ink-4`, `--accent-soft`, `--accent-line`, `--accent-glow`, `--pink-soft` are kept.)
4. **No off-scale font weights** — `font-(bold|extrabold|black|light|thin)`; DS allows only `font-medium` / `font-semibold`. Scans `*.vue` + `*.ts`. (Promoted 2026-06-15 from governance-only to build-enforced.)
5. **No bare native form controls** — `<select>`, `<input type="date">`, `<input type="checkbox">`, `<input type="radio">`; use the `Select` / `DatePicker` / `Checkbox` / `Switch` / `RadioGroup` primitives. Exempts `components/player/` (reka portals break in fullscreen, so player pickers stay native) and `type="datetime-local"`. **Per-site escape hatch:** a `bespoke-keep` comment within 6 lines above the control exempts it (justify inline — e.g. per-provider-accent checkbox, sr-only segmented-toggle radio, rich card-style radio the flat `RadioGroup` can't model). This is the *only* enforceable slice of "reuse primitives" — controls that map to a native element are greppable; div-composition primitives (Card/Badge/Dialog/Tabs/etc.) stay governance-only below.
6. **No arbitrary spacing values** — `(p|px|py|pt|pr|pb|pl|m|mx|my|mt|mr|mb|ml|gap|gap-x|gap-y|space-x|space-y)-[<n>px|rem|em]` bypass the 4px token scale; use a token (`px-[10px]` → `px-2.5`; `1px` → `-px`). **Sizing props (`w/h/min-*/max-*/size`) are OUT OF SCOPE** (no token scale for arbitrary pixel dims); `calc()`/`var()` arbitrary values allowed. Off-grid odd-pixel survivors (`3/5/7/9/11px` on dense player menus + `Stepper`) allowlisted per-`(file,class)` in `frontend/web/scripts/design-system-spacing-allowlist.txt`. (Added 2026-06-17.)
7. **No raw `rgba()`/`hsl()` color literals in `.vue`** — matches both comma form `rgba(0,0,0,.5)` and modern space/slash form `rgb(0 0 0 / .5)`; bind to an alpha token (`--white-a4/a8/a20/a30`, `--cyan-a08/a20/a40/a60`, `--black-a40/a60/a80`, `--scrim-bg-soft/strong`, or a semantic `*-soft`). `rgb*(var(--…))` forms are exempt; `.vue` only (`.ts` color data stays intentional). Identity/decorative literals (gacha rarity hues, decorative gradient ramps) allowlisted per-`(file,value)` in `design-system-allowlist.txt`. Tokens are a curated snap scale — see `docs/superpowers/specs/2026-06-17-ds-rules-hardening-rgba-inline-style-design.md`. (Added 2026-06-17.)
8. **No static color inside inline `style`** — `style="…"` / `:style="'…'"` carrying `#hex` / `rgb()` / `hsl()`; use a class or token. Dynamic bindings (`:style="{ width: pct }"`) and `px`/layout values are NOT flagged (color is the DS concern, layout isn't). (Added 2026-06-17.)

**Escape-hatch:** prefer migrating to a token; only when no token reproduces the value, add a justified line to the matching allowlist — `path:hex:reason` / `path:value:reason` in `design-system-allowlist.txt` (hex Rule 2; rgba/hsl Rule 7; inline-style color Rule 8) or `path:class:reason` in `design-system-spacing-allowlist.txt` (spacing, Rule 6) — never disable the gate. Prove the fail-path with `bash frontend/web/scripts/design-system-lint.sh --selftest`. Since 05-04, `--accent` is the shadcn hover surface — use `--brand-cyan` for brand cyan.

**Structural rules (GOVERNANCE-ONLY — human/AI-followed, NOT build-enforced;** a grep can't AST-distinguish them):

- Reuse `@/components/ui` primitives before building new (Button/Card/Badge/Input/Select/Dialog/Tabs/DropdownMenu/Tooltip/Popover/Switch/Checkbox).
- Only `font-medium` / `font-semibold` weights.
- Padding scale (card `p-4 md:p-6 lg:p-8`).
- `cva` variants for component variation.

**In-browser (Chrome) smoke is OPT-IN, not mandatory (DS-NF-06, revised 2026-06-11 to save tokens).** Do a Chrome smoke only when the owner asks for one. For small fixes, skip it silently. For non-small visual changes, ASK the owner whether they want a Chrome checkup instead of running it automatically. Caveat that still stands: jsdom/vitest CANNOT catch Tailwind-v4 cascade bugs (unlayered custom classes beat utilities) — so when a change touches cascade-sensitive styling, say so when offering the checkup.

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

BE egress recorder (catalog/scraper/streaming — Activity Register v4.0 Phase 2):
```
ANALYTICS_INTERNAL_URL   # default http://analytics:8092 — catalog/scraper/streaming
                         # ship recorded outbound egress effects (host/provider/bytes,
                         # one aggregated row per HLS watch session) to analytics
                         # POST /internal/effects over the Docker network. Non-secret
                         # service-discovery URL; the producer is non-blocking +
                         # drop-on-full so an analytics outage never affects requests.
                         # /internal/effects is NOT gateway-proxied (Docker-network-only).
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
USER_RATE_LIMIT_PER_MINUTE   # per-authenticated-user GCRA rate, default 240 (was 60; resized 2026-06-12 — profile-page tab prefetch tripped it)
USER_RATE_LIMIT_BURST        # per-authenticated-user GCRA burst, default 40 (was 10)
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

Recs service specific (extracted from player, spec 2026-06-11):
```
CATALOG_URL          # default http://catalog:8081 — S6 combo-pin Shikimori /similar fallback
```
Standard `DB_*` + `JWT_SECRET` + `REDIS_HOST` trio. Internal endpoint
`POST /internal/recs/recompute-hint` (Docker-network-only) receives
fire-and-forget watch-activity hints from player; player config:
`RECS_INTERNAL_URL` (default http://recs:8094), `RECS_HINT_ENABLED` (default true).
Gateway: `RECS_SERVICE_URL` (default http://recs:8094).

## Feedback Triage Statuses (/admin/feedback)

User feedback / error reports live as JSON files on the `docker_player_reports`
volume; triage statuses in the sidecar `_status.json` (re-read by the player
service on every request — external writes apply instantly). Statuses:
`new | in_progress | ai_done | resolved | not_relevant`.

**AI agents are PRE-AUTHORIZED by the project owner (2026-06-10) to set
`new`, `in_progress`, `ai_done`, and `not_relevant` autonomously** — that is
the whole point of `ai_done`: "AI believes this is done, awaiting human
verification". **Only `resolved` is human-only** (the owner promotes
`ai_done` → `resolved` in the UI after verifying). Use the guarded helper —
it enforces the human-only rule itself:

```bash
bin/feedback-status <report-id> <status> [updated_by]   # refuses "resolved"
```

## UI Audit Test User (DO NOT DELETE)

Permanent `ui_audit_bot` account (API key, password login, seed data) for UI audits + integration/e2e tests. **DO NOT DELETE / recreate.** Full details, seed contents, and key-rotation steps: [`docs/ui-audit-test-user.md`](docs/ui-audit-test-user.md).

## UI/UX Audit Framework

Methodology, severity scale, citation rules, per-finding template, and realistic-scenario list for any UI/UX audit live in [`docs/ui-audit-framework.md`](docs/ui-audit-framework.md). Reference first audit: `docs/issues/ui-audit-2026-04-07.md`.

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
> **Design + quality contract** (shell anatomy, accent triad, badge/CTA rules, mobile, review process): [`docs/spotlight-card-guidelines.md`](docs/spotlight-card-guidelines.md) — read alongside this recipe.


1. **Backend resolver** — Create `services/catalog/internal/service/spotlight/cards/{new_type}.go` implementing the `spotlight.Resolver` interface (`Type()` + `Resolve(ctx, userID *string) (*spotlight.Card, error)`). Mirror `featured.go`'s pattern (the «Рекомендуем сегодня» card): manual `cache.Get`/`cache.Set` with `errors.Is(err, cache.ErrNotFound)`, return `(nil, nil)` for ineligible, `(nil, err)` for failure, `(*Card, nil)` for success. Multi-item resolvers MUST apply `spotlight.AdaptiveSlice` (the 1-2-3 layout rule). Login-only resolvers return `(nil, nil)` when `userID == nil`. Always carry the `spotlight:` Redis key prefix for new keys (HSB-NF-03). Add a co-located `_test.go` with handwritten fakes — no testify/mock.

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
4. Updates the changelog with user-facing entries in **Russian Trump-mode** — bombastic, self-aggrandizing, ALL-CAPS emphasis on key words, signature closers ("Поверьте мне." / "Никто другой так не делает!" / "ВЕЛИКОЛЕПНО."), emojis kept, factual claims preserved. Entries are prepended to `frontend/web/changelog.full.json` (full-history source of truth); the served file `frontend/web/public/changelog.json` is **generated** from it (latest 30 entries only) via `frontend/web/scripts/changelog-trim.mjs` — it's fetched whole on every page load, so we ship only the newest entries. This is what `LastUpdates.vue` loads in the Changelog tab. Full style spec + examples live in `.claude/commands/animeenigma-after-update.md` step 4; the 2026-05-19 group in `changelog.full.json` is the gold-standard reference.
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
| recs       | 8094 | /metrics  | Recommendation engine (extracted from player, spec 2026-06-11) |
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
- `/api/users/recs`, `/api/events/rec` → recs:8094 (optional JWT); `/api/admin/recs/*` → recs:8094 (admin). Internal `/internal/recs/recompute-hint` NOT exposed — Docker-network-only; player fires it on watch activity
- `/api/watch-together/*` → watch-together:8091 (JWT required for HTTP; WS uses `?token=` query param since browsers can't set custom headers on WS upgrade)

### Watch Together

Ephemeral private friend rooms (2-10 members) for synchronized anime watching across all 5 players. Single Go microservice `services/watch-together/` (port 8091), Redis-only state under `wt:` prefix, 15min sliding TTL + 5min grace. Gateway-routed REST (`/api/watch-together/rooms`) + WS (`/ws?token=&room=`). Full architecture, env vars, locked decisions, dependency audit, and the daily Kodik-canary runbook: [`docs/watch-together-reference.md`](docs/watch-together-reference.md).

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

Admin is **path-based** at `animeenigma.org/admin` — there is no `admin.*` subdomain.
Grafana/Prometheus/pgAdmin are reached via the gateway `/admin/*` route:

- `https://animeenigma.org/admin/grafana` - Grafana dashboards
- `https://animeenigma.org/admin/prometheus` - Prometheus raw metrics
- `https://animeenigma.org/admin/pgadmin` - PostgreSQL admin
- `https://animeenigma.org/admin/k8s` - Kubernetes dashboard

## File Locations

- Shared libraries: `/libs/`
- API contracts: `/api/`
- Service code: `/services/{name}/`
- Frontend: `/frontend/web/`
- Infrastructure: `/docker/`, `/deploy/`, `/infra/`
- Kubernetes manifests: `/deploy/kustomize/`
