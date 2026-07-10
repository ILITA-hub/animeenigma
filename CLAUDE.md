# CLAUDE.md - Project Guidelines for AI Assistants

## Project Overview

AnimeEnigma: self-hosted anime streaming platform with Shikimori/MAL integration. Go microservices + Vue 3 frontend. **Target deployment**: small self-hosted groups (no CDN).

## Architecture Principles

### Video Player Architecture

> **📖 Canonical, code-verified deep-dive: [`docs/aeplayer-reference.md`](docs/aeplayer-reference.md).** Read it before touching `frontend/web/src/components/player/` or reasoning about playback. The summary below is high-level; that doc is the source of truth.

**There is ONE unified player — `frontend/web/src/components/player/aePlayer/AePlayer.vue`.** The old per-source components (`OurEnglishPlayer.vue`, `RawPlayer.vue`, `AnimeLibPlayer.vue`, `HanimePlayer.vue`) were **deleted** and folded into it. The only separate surface is `KodikPlayer.vue` — the legacy Kodik **iframe**, mounted as a binary "Classic Kodik" fallback toggle on the anime page (and the surface when `VITE_AE_PLAYER_ENABLED=false`).

The "5 players" are now **source families inside one player**, chosen via the in-player **Source** panel. The user selects a **combo** `{audio, lang, provider, server, team}`; the backend **capability feed** (`GET /api/anime/{id}/capabilities`, `(policy,health,content)`-derived, disabled providers omitted) is the single source of truth for what's available.

| Source family | Lang | Group | Video tech | JP subs |
|--------|------|-------|-----------|---------|
| **Kodik** | RU | `ru` | Kodik HLS adapter (the separate `KodikPlayer.vue` is the iframe fallback only) | No |
| **EN scraper chain** | EN | `en` | HTML5 `<video>` + hls.js / MP4 via backend HLS proxy | Yes (SubtitleOverlay) |
| **Raw (AllAnime raw)** | JP | `jp` | HTML5 `<video>` + hls.js / MP4 (`fast4speed.rsvp`) | Yes (Jimaku + others) |
| **Hanime / 18anime** | 18+ | `adult` | HTML5 `<video>` + hls.js | No |
| **ae (self-hosted)** | EN/RU/JP | `firstparty` | HTML5 `<video>` + hls.js (MinIO) | Yes |

> **RAW vs DUB:** the top slider's **RAW** position = `combo.audio:'sub'` (original audio — EN-sub/RU-sub/pure-JP all surface, language filter dropped); **DUB** = `combo.audio:'dub'` (localized; language slider EN/RU only, no JP dub). Subtitles default **OFF** and never auto-enable. Full model in the reference doc.
>
> **EN scraper backend** (`services/scraper/`) fails over **gogoanime → animepahe → allanime-okru → miruro → nineanime** (+ optional `animekai`); see [`docs/scraper-framework.md`](docs/scraper-framework.md). aePlayer is behind `VITE_AE_PLAYER_ENABLED` (defaults on).

**Backend route family** (gateway → catalog → scraper microservice):
- `GET /api/anime/{uuid}/scraper/episodes?prefer=<provider>`
- `GET /api/anime/{uuid}/scraper/servers?episode=<id>&prefer=<provider>`
- `GET /api/anime/{uuid}/scraper/stream?episode=<id>&server=<id>&category=sub|dub&prefer=<provider>`
- `GET /api/anime/_/scraper/health`

**Shared components (reused across players):**
- `SubtitleOverlay.vue` — custom selectable-text JP subtitle renderer (ASS/SRT/VTT); teleports to fullscreen element, time-synced via `requestAnimationFrame`.
- `subtitle-parser.ts` — parses ASS (via `ass-compiler`), SRT, VTT → `SubtitleCue[]`.
- `OtherSubsPanel.vue` — aggregated subtitle picker (Jimaku, OpenSubtitles, etc.).
- `ReportButton.vue` — per-stream user-reportable error path; persists to disk + Telegram admin notification.
- `libs/videoutils/proxy.go` — backend HLS proxy for CORS. **Auth gate = `allowlisted OR signed`**: catalog signs every scraper-resolved stream/subtitle URL (`streamsign` → `videoutils.SignStreamURL`), and the proxy mints HMAC provenance tokens for rotating child/segment CDNs — so **scraper CDN hosts are auto-trusted and need NO allowlist entry**. The static `HLSProxyAllowedDomains` list (flat `[]string`; the provenance/quarterly-review machinery was deleted 2026-07-05) holds ONLY first-party hosts (`stealth-scraper`, `minio`) + catalog paths that don't sign yet: Kodik ad-free (`solodcdn.com`), Hanime CDN families (`hanime.tv`/`htv-*`/`hydaelyn-*`/`zodiark-*`), AniLib (`cdnlibs.org`/`hentaicdn.org`), 18anime mirrors (`mp4upload.com`/`turboviplay.com`/`turbosplayer.com`), subtitles (`jimaku.cc`), and the AUTO-517 stop-gap `mt.nekostream.site`. Do NOT add scraper CDNs here — rely on signing; for a new unsigned catalog path prefer adding `streamsign.Sign` at the source (animejoy endpoints = the pattern).

**Known issue:** HLS codec stall (D-07) — for some HLS sources hls.js loads the master/level playlist but never requests `.ts` fragments (readyState stays 0). Pre-existing; `hls.js` is pinned to `~1.5.20` (1.6.x regressed codecs). Tracked in `aePlayer/MANUAL-REVIEW.md`. (The retired AniLib MP4 player's soft-sub limitation no longer applies — AniLib is not wired into aePlayer.)

### Video Streaming Model

Four sourcing modes:
1. **Kodik iframe** — frontend embeds Kodik's player iframe (no direct video control).
2. **Backend proxy/restream** — `services/streaming` HLS proxy restreams MP4/HLS from AniLib, AnimePahe (Kwik), AllAnime (`fast4speed.rsvp`), AnimeFever, Miruro, 9anime, and Hanime CDNs (CORS + Referer injection).
3. **Self-hosted storage** — admin-uploaded videos in MinIO.
4. **Stealth browser sidecar** — `services/stealth-scraper/` (Camoufox anti-detect Firefox) resolves providers whose DB `engine` is `browser` (gogoanime, nineanime: JS-runtime stream id + Referer-gated rotating CDN); internal-only. _(Earlier `services/animepahe-resolver/` puppeteer sidecar fronting `animepahe.pw` was **retired 2026-06-24** — animepahe disabled, kept in roster for possible revival.)_

### On-Demand Catalog Population

Catalog is NOT pre-populated: 1) user searches → 2) backend queries Shikimori GraphQL → 3) results mapped by **original Japanese name** (primary key) → 4) metadata stored in PostgreSQL for future lookups → 5) video sources resolved separately via anime parsers.

### External API Integration

Primary data sources:
- **Shikimori** — metadata (titles, descriptions, posters, genres).
- **Kodik** — RU iframe embed. Parser: `services/catalog/internal/parser/kodik/`.
- **AniLib** — RU direct MP4. Parser: `services/catalog/internal/parser/animelib/`.
- **OurEnglish (`services/scraper/`)** — EN via failover orchestrator. Order: `gogoanime` → `animepahe` _(disabled 2026-06-24 — resolver sidecar retired; code kept in roster for revival, so it stays in `candidateProviders` but is never registered while disabled)_ → `allanime-okru` _(AllAnime GraphQL discovery + ok.ru stream resolution, clock-free — folded 2026-07-06 from the former standalone `allanime` + `okru` providers; `allanime` survives only as a disabled tombstone row kept in scraper `KnownProviders`)_ → `miruro` (secure-pipe pure-Go obfuscation) → `nineanime` (MP4-only last-resort) → optional `animekai`. _(`animefever` was REMOVED from the binary 2026-07-05 — dead upstream, ad-swapped segments gone for everyone incl. residential; survives only as a disabled `scraper_operated` tombstone row in the catalog DB, kept in scraper `KnownProviders` so the remote-config loader validates.)_ Provider impls: `services/scraper/internal/providers/{name}/`. Embed extractors: `services/scraper/internal/embeds/`.
- **Hanime** — 18+. Parser: `services/catalog/internal/parser/hanime/`.
- **AllAnime raw-JP** — original-audio JP (Raw player). `services/catalog/internal/parser/allanime/` (+ `services/scraper/internal/providers/allanime/`).
- **Jimaku.cc** — JP subtitle files (ASS/SRT/VTT). Consumed by OurEnglish, Raw players via `SubtitleOverlay.vue`.
- **ARM** (`arm.haglund.dev`) — anime ID mapping (Shikimori/MAL → AniList). Lib: `libs/idmapping/`.
- **MAL** (optional) — additional metadata, ratings sync.

## Code Conventions

### Effort & impact metrics — NO days, hours, sprints

**Time-effort units are not used.** Every plan/feature/CHANGELOG entry is scored on three dimensional metrics (full spec `.planning/CONVENTIONS.md`):
- **UXΔ** (UX Delta) — signed `-5..+5` + `Better`/`Worse`/`Ambiguous`. e.g. `UXΔ = +2 (Better)`.
- **CDI** (Coherence Disruption Index) — two numbers: `(Spread × Shift) * Effort_Fib`. e.g. `CDI = 0.02 * 13`. Effort on Fibonacci (1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233+). DO NOT pre-multiply.
- **MVQ** (Mythic Vibe Quotient) — creature + match%/slop-resistance%. e.g. `MVQ = Griffin 85%/80%`. Creatures: Phoenix / Griffin / Kraken / Sprite / Basilisk / Dragon.

Plan-producing sub-agents (gsd-planner, gsd-discuss-phase, gsd-roadmapper, etc.) MUST follow this. Reject any plan that returns "N days" — re-score and re-submit.

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
- **Packages**: lowercase, single word (`handler`, `service`, `repo`).
- **Files**: snake_case (`anime_parser.go`, `video_source.go`).
- **Types**: PascalCase (`AnimeService`). **Methods**: PascalCase exported / camelCase private. **Variables**: camelCase (`animeID`, `videoURL`). **Constants**: PascalCase or ALL_CAPS for env vars.

### Error Handling
Use the shared `libs/errors` package (`github.com/ILITA-hub/animeenigma/libs/errors`):
- Domain errors: `return nil, errors.NotFound("anime not found")`.
- Wrap external errors: `return nil, errors.Wrap(err, "failed to fetch from shikimori")`.

### Database
- `libs/database` + GORM for connections; DB & tables auto-created on service startup.
- GORM query methods for most ops; raw SQL for complex queries.
- Primary key: UUID strings with `gen_random_uuid()` default.

```go
type User struct {
    ID        string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    Username  string         `gorm:"size:32;uniqueIndex" json:"username"`
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
db, err := database.New(cfg.Database)  // auto-creates DB if absent
db.AutoMigrate(&domain.User{})         // creates table if absent
```

### Caching
`libs/cache` with TTL strategies — anime details 6h (`cache.KeyAnime(id)`, `cache.TTLAnimeDetails`); search results 15min (`cache.KeySearchResults(query, page)`, `cache.TTLSearchResults`); external video URLs 1h, they expire (`"video:"+animeID`, `time.Hour`).

### Logging
Structured via `libs/logger`: `log.Infow("fetching anime from shikimori", "query", query, "page", page)`; `log.Errorw("failed to proxy video stream", "anime_id", animeID, "source", "aniboom", "error", err)`.

### Design System (Neon Tokyo, shadcn-vue)

Canonical reference: **`frontend/web/src/styles/DESIGN-SYSTEM.md`** (token tiers, usage rules, component inventory). Do NOT duplicate it — read it before touching frontend styling. Bind to semantic tokens; never hardcode colors.

**FE pre-flight (`/frontend-verify`).** Before finishing ANY `frontend/web/` change, run **`/frontend-verify`** (DS-lint, i18n en/ru/ja parity, real `bun run build`, lucide/TS2614/Tailwind-v4-cascade traps, opt-in Chrome smoke). A `PostToolUse` hook (`.claude/hooks/ds-lint-postedit.sh`) also runs the DS gate automatically on every `frontend/web/src/**/*.{vue,ts}` edit, so violations surface at edit time, not just at build/deploy.

**Heavy/complex FE changes → prototype first (`design-prototyping` skill).** For a redesign, multi-component rework, or any visually-heavy UI change, do NOT edit `.vue` first. Invoke **`design-prototyping`** (`.claude/skills/design-prototyping/`): it stands up a localhost design sandbox (token-faithful self-contained HTML, served over the owner's `-L 3000:localhost:58363` SSH tunnel) so the owner reviews + iterates `vN` in-browser and approves the direction BEFORE any Vue is written. One-line copy/token tweaks and pure-logic fixes skip it. After approval: port to Vue → `/frontend-verify` → `/animeenigma-after-update`.

**Lint gate (build-ENFORCED).** `frontend/web/scripts/design-system-lint.sh` gates `make lint-frontend` (→ `make lint` / CI) AND `make redeploy-web` — `ERRORS>0 ⇒ exit 1` **FAILS THE BUILD.** It enforces **8 rules** (off-palette color classes · non-allowlisted hex · deprecated `var()` aliases · off-scale font weights · bare native form controls · arbitrary spacing · raw `rgba()`/`hsl()` · inline-style color) plus governance-only structural rules. **Full regexes, exemptions & gotchas: [`docs/design-system-lint-rules.md`](docs/design-system-lint-rules.md)** (or the script itself + `--selftest`).

- **EXEMPT brand/provider hues** (NOT off-palette — never "fix" these): `cyan pink orange rose indigo teal lime` (Neon-Tokyo + per-provider accents: Kodik cyan, AniLib orange, Hanime pink, Raw rose).
- **Escape-hatch:** migrate to a token first; only if none fits, add a justified `path:value:reason` line to `design-system-allowlist.txt` (Rules 2/7/8) or `design-system-spacing-allowlist.txt` (Rule 6) — **never disable the gate.** `--accent` = shadcn hover surface; use `--brand-cyan` for brand cyan.

**In-browser (Chrome) smoke is OPT-IN, not mandatory (DS-NF-06, revised 2026-06-11 to save tokens).** Do one only when the owner asks. Small fixes: skip silently. Non-small visual changes: ASK the owner instead of auto-running. Caveat: jsdom/vitest CANNOT catch Tailwind-v4 cascade bugs (unlayered custom classes beat utilities) — flag this when offering the checkup on cascade-sensitive styling.

## Key Flows

- **Search:** User → Frontend → Gateway → Catalog → [check local DB first; if miss, query Shikimori; store results; return].
- **Kodik playback:** User → Frontend → Catalog (Kodik parser) → Kodik API → embed URL with params → `KodikPlayer.vue` iframe.
- **AnimeLib playback:** User → Frontend → Catalog (AnimeLib parser) → AnimeLib hapi API → MP4 URLs + qualities (or Kodik iframe URL) → `AnimeLibPlayer.vue` → backend proxy → MP4 stream (OR Kodik iframe fallback).
- **Anime parser:** Catalog → parser (`services/catalog/internal/parser/{name}/`) → resolve Shikimori ID → provider-specific ID → fetch episodes/translations/qualities → cache video URLs 1h.

## External APIs

### Shikimori GraphQL
Endpoint `https://shikimori.one/api/graphql`:
```graphql
query SearchAnime($search: String!, $limit: Int) {
  animes(search: $search, limit: $limit) {
    id name russian japanese poster { originalUrl } genres { id name russian } episodes score
  }
}
```

### Video Source Providers
Each parser in `services/catalog/internal/parser/{name}/`:
- **Kodik** — RU iframe embed; returns embed URLs with translation/episode params; no direct video control.
- **AnimeLib** — RU direct MP4 via hapi API (episode data, multi-quality MP4 URLs, translation info); falls back to Kodik iframe when direct URLs unavailable.

## Testing
- Unit: `go test ./...` · Integration: `go test -tags=integration ./...`.
- testcontainers for DB tests; mock external APIs (don't hit them in tests).

## Environment Variables

Required for all services: `DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME`, `REDIS_HOST, REDIS_PORT`, `JWT_SECRET`. Secrets live in `docker/.env` (host-only, git-ignored).

Per-service vars + defaults (egress recorder, Catalog, Streaming, Gateway rate-limits, Notifications, Recs, Scheduler crons): **[`docs/environment-variables.md`](docs/environment-variables.md)**.

## Feedback Triage Statuses (/admin/feedback)

User feedback / error reports = JSON files on the `docker_player_reports` volume; triage statuses in the sidecar `_status.json` (re-read by the player service on every request — external writes apply instantly). Statuses: `new | in_progress | ai_done | resolved | not_relevant`.

**AI agents are PRE-AUTHORIZED by the project owner (2026-06-10)** to set `new`, `in_progress`, `ai_done`, and `not_relevant` autonomously — that is the whole point of `ai_done` ("AI believes this is done, awaiting human verification"). **Only `resolved` is human-only** (owner promotes `ai_done` → `resolved` in the UI after verifying). Use the guarded helper (it enforces the human-only rule): `bin/feedback-status <report-id> <status> [updated_by]` (refuses `resolved`).

## UI Audit Test User (DO NOT DELETE)
Permanent `ui_audit_bot` account (API key, password login, seed data) for UI audits + integration/e2e tests. **DO NOT DELETE / recreate.** Details, seed contents, key-rotation: [`docs/ui-audit-test-user.md`](docs/ui-audit-test-user.md).

## UI/UX Audit Framework
Methodology, severity scale, citation rules, per-finding template, realistic-scenario list: [`docs/ui-audit-framework.md`](docs/ui-audit-framework.md). Reference first audit: `docs/issues/ui-audit-2026-04-07.md`.

## Common Tasks

**Add a new anime parser:** 1) `services/catalog/internal/parser/{name}/client.go` 2) implement `AnimeParser` interface 3) add to parser registry at service init 4) config for API keys.

**Add a new video source:** 1) add type to `domain.SourceType` 2) proxy handler in streaming service if needed 3) update frontend player.

**Pin anime to top of listings (`sort_priority`):** `Anime.SortPriority int` (default 0); browse/search ordered `sort_priority DESC, score DESC`, so any anime with `sort_priority > 0` appears first.
```sql
UPDATE animes SET sort_priority = 1 WHERE shikimori_id = '57466';  -- pin
UPDATE animes SET sort_priority = 0 WHERE shikimori_id = '57466';  -- unpin
```
Higher = higher position (`1` for a single pin; `2, 1` etc. for relative order among pinned). Affects ALL browse queries (announcements, ongoing, released, search).

**Database migrations:** tables auto-created via GORM `AutoMigrate()` on startup. Schema changes: 1) update domain struct fields/tags 2) restart (new columns added automatically) 3) destructive changes (dropping columns) need manual SQL or table recreate. GORM only CREATES new tables/columns — it does NOT modify/drop existing ones (protects data).

**Add a Spotlight Card Type:** `HeroSpotlightBlock` is a 9-card rotating carousel; a 10th touches 5 anchors (BE resolver + Data type + DI, FE SFC + dispatch/i18n/types) (~50 lines). Full 5-step recipe + verify commands + design/quality contract: **[`docs/spotlight-card-guidelines.md`](docs/spotlight-card-guidelines.md)**.

## Git & Deploy Workflow

Full guide: [`docs/git-workflow.md`](docs/git-workflow.md). Flow:
**worktree off fresh `origin/main` → code + verify → pull-rebase-push to `main` → `/animeenigma-after-update` (run AFTER the push, to catch the latest) → `git worktree remove` + `prune` (only once after-update is green).**

> **GOLDEN RULE: never make direct changes in `/data/animeenigma` (the `main` base tree) — do all work in worktrees. The only exception is `.env`/secrets files** (git-ignored + host-only, so they don't cause divergence). The base tree is a read-only mirror of `origin/main`, fast-forwarded every 10 min by a cron (`/usr/local/bin/animeenigma-git-autosync.sh`, source `infra/host/`); committing or editing code there makes it dirty/diverged, which **pauses the ff-only auto-sync** (logs `skip: DIVERGED`) and tangles your diff with other agents' WIP.

## Local Development Commands

Use `make` for all local ops (`make help` for all targets):

| Command | Description |
|---------|-------------|
| `make dev` / `make dev-down` | Start / stop dev environment |
| `make redeploy-<service>` | Rebuild + restart a service (after code changes) |
| `make restart-<service>` | Restart without rebuild (after config changes) |
| `make logs-<service>` | Follow service logs |
| `make health` | Health-check all services |
| `make ps` | Show running containers |

**Frontend**: use `bun` (not npm/pnpm); `bunx` for CLI tools (not npx):
```bash
cd frontend/web
bun install                 # deps
bun run dev | build         # dev server | prod build
bun run test:e2e            # e2e
bunx playwright test [name] [--reporter=list]   # e2e (bunx, not npx)
bunx eslint src/            # lint
bunx tsc --noEmit           # type-check
```
Examples: `make redeploy-gateway` (after gateway code), `make restart-grafana` (after compose env, no code), `make logs-catalog`, `make health`.

## After-Update Skill (MUST USE)

After ANY implementation work (features, bug fixes, refactoring), **always invoke** `/animeenigma-after-update` before ending the conversation. It:
1. Runs `/simplify` over the changed code to fold in behavior-preserving quality cleanups (skipped for docs/changelog-only changes).
2. Lints and builds the affected code.
3. Redeploys changed services (`make redeploy-<service>`).
4. Runs health checks.
5. Updates the changelog in **Russian Trump-mode** — bombastic, self-aggrandizing, ALL-CAPS emphasis on key words, signature closers ("Поверьте мне." / "Никто другой так не делает!" / "ВЕЛИКОЛЕПНО."), emojis kept, factual claims preserved. Entries prepend to `frontend/web/changelog.full.json` (full-history source of truth); the served `frontend/web/public/changelog.json` is **generated** from it (latest 30 entries) via `frontend/web/scripts/changelog-trim.mjs` (fetched whole on every page load). `LastUpdates.vue` loads it in the Changelog tab. Full style spec + examples: `.claude/commands/animeenigma-after-update.md` step 5; gold-standard ref = 2026-05-19 group in `changelog.full.json`.
6. Commits all changes with co-authors and pushes.

**Do not skip this step.** It ensures every implementation is deployed, verified, documented for users, and pushed.

## Don't Do
- No CDN code (not needed self-hosted) · No DB pre-population (on-demand only) · No storing external video files (stream directly) · No caching video URLs >1h (they expire) · Don't fight GORM (conventions for simple queries, raw SQL for complex) · No complex abstractions for simple operations.

## Service Ports

| Service | Port | Metrics | Description |
|---------|------|---------|-------------|
| gateway | 8000 | /metrics | API gateway, rate limiting |
| auth | 8080 | /metrics | Authentication, JWT |
| catalog | 8081 | /metrics | Anime catalog, Shikimori API |
| streaming | 8082 | /metrics | Video streaming, MinIO |
| player | 8083 | /metrics | Watch progress, watchlists |
| rooms | 8084 | /metrics | Game rooms, WebSocket |
| scheduler | 8085 | /metrics | Background jobs |
| themes | 8086 | /metrics | Anime OP/ED ratings |
| library | 8089 | /metrics | Library service (BitTorrent → HLS → MinIO, admin-only) |
| notifications | 8090 | /metrics | Generic notification engine (new episodes, future types) |
| watch-together | 8091 | /metrics | Co-watch service (Redis-only; rooms + sync + chat) |
| recs | 8094 | /metrics | Recommendation engine (extracted from player, spec 2026-06-11) |
| governor | 8100 | /metrics | Degradation governor (pressure level via Redis `ae:degradation:level`; spec 2026-07-10) |
| web | 80 | - | Vue 3 frontend (nginx) |

### Gateway Routing
All API requests go through the gateway:
- `/api/auth/*`→auth:8080 · `/api/anime/*`, `/api/genres`, `/api/kodik/*`→catalog:8081 · `/api/admin/*`→catalog:8081 (protected) · `/api/streaming/*`→streaming:8082 · `/api/users/*`→player:8083 · `/api/rooms/*`, `/api/game/*`→rooms:8084 · `/api/themes/*`→themes:8086 (public+protected+admin) · `/api/library/*`→library:8089 (admin-only; routes added incrementally in v0.2 Phases 2–5)
- `/api/notifications/*`→notifications:8090 (JWT required; internal `/internal/notifications` NOT exposed — Docker-network-only)
- `/api/users/recs`, `/api/events/rec`→recs:8094 (optional JWT); `/api/admin/recs/*`→recs:8094 (admin); internal `/internal/recs/recompute-hint` NOT exposed — Docker-network-only, player fires it on watch activity
- `/api/watch-together/*`→watch-together:8091 (JWT for HTTP; WS uses `?token=` query param since browsers can't set custom headers on WS upgrade)

### Watch Together
Ephemeral private friend rooms (2-10 members) for synchronized watching across all 5 players. Single Go microservice `services/watch-together/` (port 8091), Redis-only state under `wt:` prefix, 15min sliding TTL + 5min grace. Gateway-routed REST (`/api/watch-together/rooms`) + WS (`/ws?token=&room=`). Full architecture, env vars, locked decisions, dependency audit, daily Kodik-canary runbook: [`docs/watch-together-reference.md`](docs/watch-together-reference.md).

### Monitoring Endpoints
Each service exposes Prometheus `/metrics`:
```bash
curl http://localhost:8000/metrics
curl http://localhost:8081/metrics | grep http_request_duration_seconds
```
Metrics: `http_requests_total` (labels: service/method/path/status), `http_request_duration_seconds` (p50/p95/p99 histogram), `http_response_size_bytes` (size histogram).

### Admin URLs (Kubernetes)
Admin is **path-based** at `animeenigma.org/admin` (no `admin.*` subdomain). Grafana/Prometheus/pgAdmin via gateway `/admin/*`: `https://animeenigma.org/admin/{grafana | prometheus | pgadmin | k8s}` (Grafana dashboards / Prometheus raw metrics / PostgreSQL admin / Kubernetes dashboard).

## File Locations
Shared libs `/libs/` · API contracts `/api/` · Services `/services/{name}/` · Frontend `/frontend/web/` · Infra `/docker/`, `/deploy/`, `/infra/` · K8s manifests `/deploy/kustomize/`.
