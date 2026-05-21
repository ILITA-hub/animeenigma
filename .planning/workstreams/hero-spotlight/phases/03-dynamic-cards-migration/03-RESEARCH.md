# Phase 3: Dynamic Cards + Migration — Research

**Researched:** 2026-05-21
**Domain:** Go microservice composition (5 new resolvers + cross-service player fan-out + optional-auth middleware) + Vue 3 carousel extension (5 new cards + dispatch map + Home.vue migration) + a single GORM index addition
**Confidence:** HIGH (every prescription is anchored to existing Phase 1/2 patterns or to verified files in this repo; no new external dependencies, no new architectural primitives)

## Summary

Phase 3 is **fully additive on top of Phase 1's aggregator and Phase 2's carousel**. The 5 new resolvers slot into the existing `[]spotlight.Resolver` slice that `cmd/catalog-api/main.go:217` already builds, and the 5 new Vue cards extend the existing v-if/v-else-if dispatch chain in `HeroSpotlightBlock.vue:70-89`. No new packages, no new architectural patterns, no new third-party deps. The only "new architecture" pieces are (a) a 50-line `client/player_client.go` HTTP wrapper, (b) an in-router optional-auth middleware on the catalog side (≤15 lines, exact mirror of `services/player/internal/transport/optional_auth.go`), and (c) one `/internal/users/{user_id}/list` endpoint on the player service.

Three findings reshape the CONTEXT.md decisions:

1. **`/api/anime/recommended` does NOT exist** — the existing recs endpoint is `GET /api/users/recs` on the player service (`services/player/internal/handler/recs.go:160` GetRecs). The CONTEXT.md phrasing is a misnomer; design doc §5.4 ("existing recs API") is the correct framing. The `personal_pick` login path therefore **must fan out HTTP via the new `player_client.go`** — there is no in-process catalog code path to recs. The endpoint already supports both anonymous (returns `recs.trending` row) and logged-in (`recs.upNext` row) callers via the OptionalAuthMiddleware on the player side, so the catalog → player call passes the JWT through and gets the right shape.

2. **Catalog and player share the same Postgres database (`animeenigma`)** — every service in `docker/docker-compose.yml` declares `DB_HOST: postgres`, `DB_NAME: animeenigma` [VERIFIED: docker-compose grep]. So the `now_watching` resolver's `DISTINCT ON (wp.user_id)` SQL **can SELECT from `watch_progress`, `users`, and `animes` directly via catalog's shared `*gorm.DB`** — no HTTP fan-out needed. The default CONTEXT.md assumption holds.

3. **The `watch_progress.updated_at` column has NO dedicated index today** — verified by reading `services/player/internal/domain/watch.go:30-52`. `updated_at` has zero GORM tags (line 51: `UpdatedAt time.Time `json:"updated_at"`). The existing composite index `idx_watch_progress_user_anime_ep` is on (user_id, anime_id, episode_number) — useless for the `WHERE wp.updated_at > NOW() - INTERVAL '5 minutes'` predicate. Phase 3 must add `gorm:"index:idx_watch_progress_updated_at"` to the `UpdatedAt` field. Player service's `AutoMigrate` will pick it up on restart. [VERIFIED: read of watch.go]

**Primary recommendation:** Slot the 5 new resolvers into the existing `[]spotlight.Resolver` in `cmd/catalog-api/main.go:217-222` next to the 4 Phase 1 resolvers. Mirror Phase 1's manual `cache.Get` + `errors.Is(err, cache.ErrNotFound)` + `cache.Set` discipline (DELIBERATE DIVERGENCE 1 — Pitfall 5 in 01-RESEARCH.md). Use **direct GORM** for `now_watching` (shared DB) and **HTTP fan-out** for `personal_pick` (login path) + `not_time_yet` + `continue_watching_new`. Add `OptionalAuthMiddleware` to catalog by copying `services/player/internal/transport/optional_auth.go` verbatim and wrapping just the `/home/spotlight` route. The 5 frontend cards each follow Phase 2's typed-`data` prop pattern.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Phase boundary**
- 5 backend resolvers: `personal_pick`, `telegram_news`, `now_watching`, `not_time_yet`, `continue_watching_new`.
- `services/catalog/internal/service/spotlight/client/player_client.go` — thin HTTP client to `http://player:8083`.
- Player service exposes 1 endpoint with `?status=` filter (CONTEXT.md `<decisions>` Player internal endpoints): `GET /internal/users/{user_id}/list?status=planned,postponed,watching`.
- Both internal-only (NOT gateway-proxied), no JWT required (docker network trust boundary).
- Optional-auth middleware on the spotlight handler so login-gated cards are eligible when a JWT is supplied.
- Adaptive 1-2-3 layout rule for multi-item resolvers (`personal_pick`, `latest_news`, `now_watching`, `telegram_news`): N=1 → 1 item; N=2 → RANDOM 1 item; N≥3 → top 3 (HSB-BE-30).
- 5 frontend card components: `PersonalPickCard.vue`, `NowWatchingCard.vue`, `TelegramNewsCard.vue`, `NotTimeYetCard.vue`, `ContinueWatchingNewCard.vue`.
- `Home.vue`: REMOVE the entire `trendingRecs` row (lines ~45-138) plus its setup-script state (`trendingRecs`, `trendingProgress`, `trendingLoading`, `rowLabelKey`, `reasonI18nKey`, `onRecClick`, `dominantSignalKey`, `trendingIds`).
- GORM `idx_watch_progress_updated_at` tag added if missing (HSB-NF-02).
- CLAUDE.md gets a new "Adding a Spotlight Card Type" section under Common Tasks (HSB-NF-05).
- The feature flag pair (`SPOTLIGHT_ENABLED` + `VITE_HERO_SPOTLIGHT_ENABLED`) REMAINS as kill switches for one release (HSB-MIG-02 — removal deferred).

**Optional-auth middleware on catalog**
- Validates `Authorization: Bearer <jwt>` if present; on success populates `authz.ContextWithClaims`; on failure (invalid/expired) → silently strip and continue as anon. NEVER 401s. Wrap the `/home/spotlight` route only.
- Handler reads `authz.ClaimsFromContext(ctx)` — passes `*string` userID to aggregator (`nil` for anon).
- Bare GET (no header) is treated as anon — no panic on missing header.

**`personal_pick` resolver**
- Anon path: in-process catalog `GetTrendingAnime(ctx, 1, 10)` (returns top 10 trending). Random 3 of top 10. Redis key `spotlight:trending:<YYYY-MM-DD>` TTL 24h.
- Login path: **HTTP fan-out** via `player_client.go` to `GET http://player:8083/api/users/recs` with the user's JWT forwarded (player's OptionalAuthMiddleware handles the JWT). Random 3 of top 10 from `envelope.recs`. Redis key `spotlight:personal:<user_id>:<YYYY-MM-DD>` TTL 24h.
- Adaptive 1-2-3 rule applied at resolver layer.

**`telegram_news` resolver**
- Reuses existing `parser/telegram.Client` and existing `news:telegram` Redis key. DO NOT add a new `spotlight:` prefixed key (HSB-NF-03 keeps the `spotlight:` prefix only for NEW keys — design doc §5.3 mandates reuse of `news:telegram`). Return first 3 posts.
- Adaptive 1-2-3 rule applied.

**`now_watching` resolver**
- SQL via shared GORM:
  ```sql
  SELECT DISTINCT ON (wp.user_id)
         u.username, u.public_id, a.id, a.name, a.name_ru, a.poster_url,
         wp.episode_number, wp.updated_at
  FROM watch_progress wp
  JOIN users u  ON u.id = wp.user_id
  JOIN animes a ON a.id = wp.anime_id
  WHERE wp.updated_at > NOW() - INTERVAL '5 minutes'
  ORDER BY wp.user_id, wp.updated_at DESC
  LIMIT 5;
  ```
- Catalog SELECTs from `watch_progress` + `users` + `animes` directly via its shared GORM connection (shared DB confirmed below).
- Drop to 1..3 sessions via adaptive rule. Redis key `spotlight:now_watching` (NOT day-keyed — live data). TTL 10s.

**`not_time_yet` resolver (login only)**
- New player endpoint via `player_client.go`: `GET /internal/users/{user_id}/list?status=planned,postponed`.
- Filter to anime where `airing_started=true` (i.e. anime status is `released` or `ongoing` or `current_episode_number > 0`).
- Random pick 1. Redis key `spotlight:not_time_yet:<user_id>` TTL 30s.
- Ineligible if userID is nil (anon) OR list returns 0 matches.

**`continue_watching_new` resolver (login only)**
- New player endpoint via `player_client.go`: `GET /internal/users/{user_id}/list?status=watching`.
- For each, compare `anime.episodes_aired` vs `last_watch_progress.episode_number`. Filter where `episodes_aired > last_watched + 1` (NEW episode aired since last view).
- Pick the most-recently-aired item. Redis key `spotlight:continue_new:<user_id>` TTL 30s.
- Ineligible if userID is nil OR list returns 0 matches.

**Player internal endpoint**
- One endpoint, status-parameter-filtered: `GET /internal/users/{user_id}/list?status=planned,postponed,watching`.
- Returns JSON shape that includes per-item: anime id + episodes_aired + episodes_count + status + the user's last `watch_progress.episode_number` for that anime (needed by `continue_watching_new`). One round-trip per resolver.
- Mounted OUTSIDE `/api` so it's NOT gateway-proxied. Precedent: `services/catalog/internal/transport/router.go:58-65` mounts `/internal/cache/invalidate/raw/{shikimoriId}` and `/internal/anime/{shikimoriId}/episodes` with no auth middleware. Player service has no `/internal/*` routes today; Phase 3 introduces the convention there.
- No JWT required (docker network trust boundary).
- Add an explicit `denied via gateway` test in the gateway router test file as defense-in-depth.

**Adaptive 1-2-3 layout (HSB-BE-30)**
- Applied at the resolver layer for `personal_pick`, `latest_news`, `now_watching`, `telegram_news`:
  - `len(items) == 0` → resolver returns `(nil, nil)` (ineligible).
  - `len(items) == 1` → return all 1.
  - `len(items) == 2` → return ONE randomly-picked item.
  - `len(items) >= 3` → return top 3.
- `latest_news` already has the 3-item cap from Phase 1 but the N=2 random-pick rule wasn't applied — Phase 3 retrofits it.

**GORM index for `watch_progress.updated_at` (HSB-NF-02)**
- Inspect `services/player/internal/domain/watch.go`. The field is currently bare (`UpdatedAt time.Time `json:"updated_at"``, line 51). Add `gorm:"index:idx_watch_progress_updated_at"`. Restart of player triggers AutoMigrate which adds the index. Confirm via `\d+ watch_progress` after deploy.

**Frontend card components**
- 5 new `.vue` components under `frontend/web/src/components/home/spotlight/cards/`.
- HeroSpotlightBlock.vue's per-type v-if/v-else-if chain (line 70-89; current 4-card chain) is extended to all 9 types.
- i18n: extend `spotlight.*` namespace in en.json + ru.json with the 5 new sub-namespaces. Parity test from Phase 2 must continue to pass.
- All 5 components honor the same UI-SPEC contracts: 2 font weights, `p-4` tablet padding, Tailwind utility-only.

**`trendingRecs` removal in Home.vue (HSB-MIG-01)**
- Delete the entire `<div v-if="trendingRecs.length > 0" ...>` block (lines ~45-128 in current file).
- Delete the related skeleton (lines ~129-138).
- Delete the setup-script state: `trendingRecs`, `trendingProgress`, `trendingLoading`, `rowLabelKey`, `reasonI18nKey`, `onRecClick`, `dominantSignalKey`, `trendingIds`.
- Delete the now-unused imports: `useRecs`, `useAnimeProgress`, `emitRecClick`, `PinSource`, `RecItem` (if no other consumer references them in Home.vue — verify; likely all are scoped to trending).
- The `/api/users/recs` backend endpoint STAYS — it now powers `personal_pick` inside the aggregator.

**Feature flag retention (HSB-MIG-02)**
- Phase 3 does NOT remove the env flags. The dual-flag setup remains as a kill switch for one release post-Phase-3.
- A follow-up cleanup commit (Phase 4 if scheduled, or a manual commit after observing one stable week) removes both flags + their guards.

**Privacy for `now_watching` (HSB-NF-04)**
- Phase 3 default: include all users in `now_watching`. Only public fields (`username`, `public_id`) are exposed — both already publicly visible on user profile pages.
- A user-level opt-out (`users.show_in_now_watching boolean default true`) is DEFERRED to Phase 4+. Phase 3 does NOT add this column.

### Claude's Discretion
- Whether to merge multiple status filters into one player endpoint or to split into two endpoints — **decided: single endpoint with `?status=` filter** (matches REQUIREMENTS.md HSB-BE-24/25 phrasing; reuses existing `ListService.GetUserListPaginated` shape).
- Whether `now_watching` direct DB SELECT or HTTP fan-out — **decided: direct DB** (shared `animeenigma` Postgres confirmed; lower latency, fewer moving parts).
- Whether to add a separate `internal/middleware/optional_auth.go` file or inline the middleware in `router.go`. Inline is fine if it's <15 lines (mirror player's `transport/optional_auth.go` — 35 lines, file is the right size for its own file).
- Whether to extend Phase 1's `aggregator_test.go` or add a new `aggregator_dynamic_test.go` for the 9-card path. **Recommend: extend existing** — the 4-card Phase 1 tests are already loaded with fakes; adding new cases is one block of `it()` blocks.
- Whether to extract the "+ ещё 2 →" link logic into a reusable composable — **inline in PersonalPickCard** for Phase 3; refactor if Phase 4 needs the same pattern.

### Deferred Ideas (OUT OF SCOPE)
- `users.show_in_now_watching` opt-out column → Phase 4+ if user feedback requests.
- Feature flag removal (`SPOTLIGHT_ENABLED`, `VITE_HERO_SPOTLIGHT_ENABLED`) → follow-up cleanup commit, not part of Phase 3.
- WebSocket-driven `now_watching` updates → v1.1.
- Slide-order personalization → v1.1.
- Editorial admin-curated card type → v1.1.
- Per-user avatar rendering in `NowWatchingCard` (Phase 3 uses anime poster + username text only).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| HSB-BE-20 | `personal_pick` resolver — anon: trending; login: recs API. Random 3 of top 10. | Anon: in-process `catalogService.GetTrendingAnime(ctx, 1, 10)` (verified in `services/catalog/internal/service/catalog.go:698`). Login: HTTP fan-out via new `player_client.go` to `GET http://player:8083/api/users/recs` with JWT forwarded. Cache keys per CONTEXT. |
| HSB-BE-21 | `telegram_news` — reuse `parser/telegram` + cache `news:telegram` TTL 30 min. Return first 3 posts. | `parser/telegram.NewClient(channel)` already wired into news handler (`cmd/catalog-api/main.go:137`). New `telegram_news` resolver takes same `*telegram.Client` via constructor + same `news:telegram` Redis key. **DO NOT add a new key.** |
| HSB-BE-22 | `now_watching` SQL — DISTINCT ON (user_id) JOIN users + animes WHERE updated_at > NOW() - INTERVAL '5 minutes' LIMIT 5. 10s Redis TTL. | Shared Postgres confirmed (`docker/docker-compose.yml`: all services share `DB_NAME: animeenigma`). Use `db.WithContext(ctx).Raw(sql).Scan(&rows)` for the Postgres-specific `DISTINCT ON` construct (GORM doesn't have a builder for it). |
| HSB-BE-23 | New `client/player_client.go` — thin HTTP client to `http://player:8083`. | Mirror Phase 1's `client/web_client.go:55-71` constructor pattern (injectable `*http.Client`, 500ms default timeout). Add methods: `GetUserList(ctx, userID, statuses string) ([]ListItem, error)`, `GetUserRecs(ctx, jwt string) (*RecsEnvelope, error)`. |
| HSB-BE-24 | `not_time_yet` resolver (login only) — Player API `?status=planned,postponed`, filter `airing_started=true`, random pick 1. 30s TTL. | New `cards/not_time_yet.go` calls `playerClient.GetUserList(ctx, userID, "planned,postponed")`. Filter where `item.Anime.Status in ("released", "ongoing") || item.Anime.EpisodesAired > 0`. Random pick via `math/rand`. |
| HSB-BE-25 | `continue_watching_new` (login only) — Player API `?status=watching`, filter `episodes_aired > last_watched + 1`, pick most-recently-aired. 30s TTL. | Player endpoint must return `episodes_aired` + `last_watched_episode` per item (extension of standard `AnimeListEntry` shape — add a thin wrapper struct in the internal handler). |
| HSB-BE-26 | Player exposes `/internal/users/{user_id}/list?status=...` — internal-only, no JWT, no gateway proxy. | New file `services/player/internal/handler/internal_list.go`. Mount in `transport/router.go` at the root level BEFORE the `r.Route("/api", ...)` block (mirror catalog's pattern at `transport/router.go:51-65`). Add defense-in-depth test in `services/gateway/internal/transport/router_test.go` that asserts `/internal/users/*` returns 404 via gateway. |
| HSB-BE-30 | Adaptive 1-2-3 layout: N=0 → drop, N=1 → 1, N=2 → random 1, N=3+ → top 3 | Implement inline in each multi-item resolver (`personal_pick`, `latest_news`, `now_watching`, `telegram_news`). Add a tiny shared helper `spotlight.AdaptiveSlice(items []T, rng *rand.Rand) []T` only if it improves readability — preferred per CONTEXT discretion to inline. |
| HSB-FE-24 | `PersonalPickCard.vue` — 1..3 posters with reason chip. Desktop: 3 in row. Mobile: 1 + "+ ещё 2 →" link. | Typed `data: PersonalPickData` prop. `<router-link to="/recs" v-if="loggedIn">` else `<router-link to="/browse?sort=trending">`. The auth check uses the existing `useAuth()` composable or the auth store. Phase 2 Vitest mock pattern (`vi.mock('@/composables/useSpotlight'`) extended for `data` variation. |
| HSB-FE-25 | `NowWatchingCard.vue` — 1..3 user-session rows with green live dot. | Typed `data: NowWatchingData` prop. Use `text-success` (`#00ff9d`) for live dot (UI-SPEC §Color "Reserved for Phase 3"). Rows stack vertically on both mobile and desktop. |
| HSB-FE-26 | `TelegramNewsCard.vue` — 1..3 telegram post excerpts; each links to `t.me/<channel>/<msg_id>` via `data.posts[i].link`. | Typed `data: TelegramNewsData` prop. `<a :href="post.link" rel="noopener noreferrer" target="_blank">`. Excerpt = first ~140 chars of `post.text` (verify telegram parser exposes `Text`). |
| HSB-FE-27 | `NotTimeYetCard.vue` — single poster + meta with header "Не пришло ли время?" + sub-line referencing the list ("Из «Запланировано»"). | Typed `data: NotTimeYetData` prop. i18n key `spotlight.notTimeYet.title` + `.fromPlannedList` / `.fromPostponedList` (computed from `data.list_status`). |
| HSB-FE-28 | `ContinueWatchingNewCard.vue` — single poster + meta with header "Продолжить просмотр" + "Новая серия ep N!" badge. | Typed `data: ContinueWatchingNewData` prop. Badge: `<span class="badge bg-cyan-500/90">{{ t('spotlight.continueWatchingNew.newEpBadge', { n: data.next_episode }) }}</span>`. CTA links to `/anime/{id}?episode={next_episode}`. |
| HSB-MIG-01 | Remove `trendingRecs` markup + script state from Home.vue. `/api/users/recs` endpoint stays. | See file paths and line numbers in Architecture Patterns → Frontend section below. |
| HSB-MIG-02 | Phase 3 does NOT remove the feature flags. | Stays as kill switches for one stable release. No code change required in Phase 3; planner must NOT include flag removal tasks. |
| HSB-NF-02 | `watch_progress(updated_at)` index — add via GORM AutoMigrate. | Add `gorm:"index:idx_watch_progress_updated_at"` to `WatchProgress.UpdatedAt` in `services/player/internal/domain/watch.go:51`. Verify post-deploy: `psql -c "\d+ watch_progress"`. |
| HSB-NF-04 | Privacy — `username` + `public_id` only; no opt-out column in Phase 3. | Verified safe: `username` shows in profile URLs (`/user/<public_id>`). No `email`, `created_at`, or `last_seen_at` exposed. |
| HSB-NF-05 | CLAUDE.md — new "Adding a Spotlight Card Type" section under "Common Tasks". | One-screen reference; see "CLAUDE.md docs scaffold" section below for the literal content sketch. |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Optional-JWT auth on `/home/spotlight` | API / Backend (catalog) | — | Mirrors player's `OptionalAuthMiddleware` precedent — the only tier that owns JWT validation is whichever service receives the request directly; gateway is a public passthrough for `/home/spotlight` |
| `personal_pick` anon (trending) | API / Backend (catalog) | Database / Storage (Postgres) | Same in-process call as `/api/anime/trending` — catalog already owns trending |
| `personal_pick` login (recs) | API / Backend (catalog → player) | API / Backend (player owns recs ensemble) | Recs ensemble (S1..S6 + S11) lives in player service per `services/player/internal/service/recs/`; catalog HTTP-fan-outs because recs is NOT a content-shaped query catalog could do locally |
| `telegram_news` | API / Backend (catalog) | External (`t.me/s/<channel>`) | Catalog already owns the Telegram parser + cache (`services/catalog/internal/parser/telegram/`) |
| `now_watching` SQL | API / Backend (catalog) | Database / Storage (shared Postgres) | Shared DB enables a direct SELECT; no HTTP hop required. Catalog owns the resolver because the aggregator lives in catalog |
| `not_time_yet` + `continue_watching_new` | API / Backend (catalog → player) | API / Backend (player owns `anime_list`) | `anime_list` lives in player service; catalog has no list query interface; must fan out HTTP |
| Player internal list endpoint | API / Backend (player) | — | Standard pattern for service-to-service contracts inside the docker network — no gateway proxy, no JWT |
| Index addition on `watch_progress.updated_at` | Database / Storage (Postgres) | API / Backend (player owns AutoMigrate) | Player service is the only writer of `watch_progress`; only its AutoMigrate creates indexes there |
| 5 new card components | Frontend / Client (Vue 3) | — | Pure SFC work; types/spotlight.ts union extended |
| `trendingRecs` removal | Frontend / Client (Home.vue) | — | Markup + setup-script cleanup; no backend coupling |
| CLAUDE.md doc section | Documentation | — | One-screen reference under Common Tasks |

## Standard Stack

### Core (all already in go.mod / package.json — no new dependencies)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/go-chi/chi/v5` | (already in go.mod) | HTTP router | Used by every service [VERIFIED: codebase grep] |
| `github.com/ILITA-hub/animeenigma/libs/cache` | local | Redis Get/Set + ErrNotFound sentinel | Phase 1 precedent — same `errors.Is(err, cache.ErrNotFound)` discipline (Pitfall 5) [VERIFIED] |
| `github.com/ILITA-hub/animeenigma/libs/logger` | local | Structured logging | `*logger.Logger.Errorw/Warnw/Infow` [VERIFIED: libs/logger/logger.go] |
| `github.com/ILITA-hub/animeenigma/libs/httputil` | local | Response helpers | `httputil.OK`, `httputil.JSON`, `httputil.BadRequest`, `httputil.BearerToken` [VERIFIED] |
| `github.com/ILITA-hub/animeenigma/libs/authz` | local | JWT validation + Context wrappers | `authz.NewJWTManager(cfg.JWT)`, `authz.ContextWithClaims`, `authz.ClaimsFromContext` [VERIFIED: libs/authz/jwt.go:109-181] |
| `gorm.io/gorm` | (already in go.mod) | DB access via `*gorm.DB`. Catalog uses shared connection for `now_watching` raw SQL. | `db.WithContext(ctx).Raw(sql, args...).Scan(&out)` for `DISTINCT ON` [VERIFIED: pattern in services/catalog/internal/repo/*.go] |
| `net/http` stdlib | go 1.24 | `player_client.go` HTTP wrapper | Phase 1 `web_client.go` precedent — `&http.Client{Timeout: 500ms}` [VERIFIED] |
| `math/rand/v2` stdlib | go 1.24 | Adaptive 1-2-3 random pick | Use `rand.IntN(n)` for the N=2 random-pick; use `rand.New(rand.NewPCG(seed,0))` for testable deterministic seeding in resolver unit tests |
| `vue@3.5.x` | (package.json) | SFC framework | Existing Phase 2 cards use Composition API + `<script setup lang="ts">` |
| `@vueuse/core` | (package.json) | `useIntervalFn`, `useMediaQuery` | Already imported by HeroSpotlightBlock.vue — Phase 3 cards may import `formatRelative` for telegram-news dates |
| `vue-i18n` | (package.json) | `useI18n` composable for `t()` | All card SFCs use it; `@intlify/vue-i18n/no-raw-text` ESLint rule blocks hardcoded strings |
| `vitest` | 4.1.6 | Unit + component tests | Co-located `*.spec.ts` files; Phase 2 pattern: `vi.mock('@/composables/useSpotlight', ...)` |
| `@playwright/test` | 1.58.x | e2e tests for the carousel | Phase 2 ships `spotlight.spec.ts` — extend with 9-card scenarios |
| `@axe-core/playwright` | (installed in Phase 2) | a11y assertions | Re-run in Phase 3 against the 9-card variants |

**Version verification (registry-confirmed for new tools):** No new packages are added in Phase 3. All packages are pinned in `services/catalog/go.mod`, `services/player/go.mod`, and `frontend/web/package.json`. Run `go mod tidy` after backend tasks and `bun install` after frontend tasks to ensure go.sum/bun.lockb are clean.

### Supporting (Phase 1 patterns reused without modification)

| Library | Purpose | When to Use |
|---------|---------|-------------|
| Phase 1 `spotlight.Aggregator` | Concurrent fan-out across resolvers | Wire 5 new resolvers into the same `[]spotlight.Resolver` slice |
| Phase 1 `spotlight.DateSeedUTC` / `DateKeyUTC` | Date-keyed Redis keys | `personal_pick` anon uses `spotlight:trending:<DateKeyUTC>` |
| Phase 1 `cache.Cache` + manual `Get/Set` discipline | Empty-pool DOES NOT cache (Pitfall 5) | All 5 new resolvers; copy `cards/latest_news.go:45-71` as the canonical pattern |
| Phase 2 `useSpotlight()` composable | Provides `cards`, `loading`, `error`, `refresh` | No changes — same composable serves all 9 card types |
| Phase 2 i18n key parity Vitest | Asserts en.json + ru.json have identical spotlight.* tree | Extend with the 5 new sub-namespaces |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Direct GORM `Raw().Scan()` for `now_watching` | `db.Model(&WatchProgress{}).Joins(...).Where(...).Find()` | GORM has no native `DISTINCT ON` builder — would require `db.Distinct("user_id").Order(...)` which produces standard SQL DISTINCT (different semantics). Raw SQL is the right tool. |
| Single `/internal/users/{id}/list?status=...` endpoint | Two endpoints `/internal/.../planned-postponed` + `/internal/.../watching` | Status filter parameterization matches existing `ListService.GetUserListPaginated(userID, status, ...)` signature; single endpoint is simpler. |
| In-process call to player's recs service | HTTP fan-out via `player_client.go` | In-process is impossible — player's recs ensemble lives in a different binary. HTTP is the only option. |
| New `internal/middleware/optional_auth.go` package on catalog | Inline middleware in `transport/router.go` | Player's precedent is a dedicated `transport/optional_auth.go` file (35 lines) — match for consistency. |
| `now_watching` HTTP fan-out to player | Direct DB SELECT (decided) | HTTP fan-out adds a hop (5-15ms) + an inter-service contract surface. Direct SELECT is faster and is justified by the shared-DB topology. |
| WebSocket-driven `now_watching` | 10s Redis cache + polling (decided per design doc) | WebSockets would require new infra (Phase 2 didn't ship one); 10s cache is what HSB-BE-22 mandates. |
| Add `users.show_in_now_watching` opt-out | No opt-out (Phase 3 decision) | Per HSB-NF-04 deferred to Phase 4+ if/when user feedback requests. Don't pre-build privacy infra without a request. |

**Installation:**
```bash
# No new packages. Verify lockfiles are clean after Phase 3:
cd services/catalog && go mod tidy
cd services/player && go mod tidy
cd frontend/web && bun install
```

## Architecture Patterns

### System Architecture Diagram

```
                    ┌─────────────────────────┐
   curl/browser ──▶ │ gateway:8000            │  /api/home/spotlight
                    │  (chi router)           │  → ProxyToCatalog
                    │  - RateLimit (per-IP)   │  → /home/spotlight on catalog
                    │  - CORS                 │  (Bearer JWT pass-through)
                    └────────────┬────────────┘
                                 │
                                 ▼
                    ┌──────────────────────────────────────┐
                    │ catalog:8081                         │
                    │  OptionalAuthMiddleware    ◀── NEW   │
                    │   - tries JWT validate               │
                    │   - on success: ContextWithClaims    │
                    │   - on failure/missing: no-op (anon) │
                    │  SpotlightHandler.Get                │
                    │   - reads claims → userID            │
                    │   - log "spotlight.request" with     │
                    │     user=anon|<uid>                  │
                    │   - aggregator.Resolve(ctx, *uid)    │
                    └────────────┬─────────────────────────┘
                                 │ 9 resolvers (fan-out concurrent)
                                 ▼
              ┌────────────────────────────────────────────────────────┐
              │ spotlight.Aggregator                                   │
              │  per-card 800ms ctx + overall 2s budget                │
              │  4 Phase 1 resolvers + 5 NEW resolvers (this phase)    │
              └─────┬─────┬─────┬─────┬─────┬──────┬─────┬─────┬───────┘
       Phase 1 ──> │     │     │     │     │      │     │     │ <── Phase 3 NEW
                   ▼     ▼     ▼     ▼     ▼      ▼     ▼     ▼
                ┌────┐┌────┐┌────┐┌────┐┌──────┐┌────┐┌────┐┌────┐
                │AnOf││Rand││News││Plat││PersnP││TgN ││NowW││NotT││ContW│
                │Day ││Tail││    ││Stat││ ick  ││ews ││atch││Yet ││New  │
                └─┬──┘└─┬──┘└─┬──┘└─┬──┘└─┬────┘└─┬──┘└─┬──┘└─┬──┘└─┬──┘
                  │     │     │     │     │       │     │     │     │
                  │     │     │     │     │ anon  │     │     │ login│ login
                  │     │     │     │     │ + login    │     │     │     │
                  │     │     │     │     │       │     │     │     │
                  ▼     ▼     ▼     ▼     ▼       ▼     ▼     ▼     ▼
            ┌──────────────┐  ┌─────┐  ┌────────────┐ ┌──────────────┐
            │ AnimeRepo    │  │web  │  │ GetTrending│ │ player_client│ ◀── NEW
            │ Search()     │  │:80  │  │ (in-proc)  │ │ GetUserRecs  │
            │ + GORM Count │  │/cha │  │   OR       │ │ GetUserList  │
            │              │  │ngelo│  │ player_clt │ │              │
            └──────┬───────┘  │g.json│ │ .GetRecs   │ └──────┬───────┘
                   │          └─┬───┘  └────────────┘        │
                   │            │                            │
                   ▼            │                            ▼
                Postgres        │                       player:8083
                (animes,        │             /api/users/recs (JWT-forwarded)
                 users,         │             /internal/users/{id}/list  ◀── NEW
                 watch_progress)│             (no JWT, internal-only)
                                ▼
                          Telegram parser
                          + Redis news:telegram

                                 ┌──────────────────────┐
                                 │ Response             │
                                 │ 200 OK               │
                                 │ {                    │
                                 │   "cards": [...],    │ ≤ 9 cards
                                 │   "generated_at": …  │
                                 │ }                    │
                                 └──────────────────────┘
```

### Recommended Project Structure

```
services/catalog/internal/
├── handler/
│   └── spotlight.go                              # extend — read userID from ctx claims
├── service/spotlight/
│   ├── aggregator.go                             # unchanged
│   ├── aggregator_test.go                        # extend — 9-card test cases
│   ├── types.go                                  # extend — 5 new *Data structs
│   ├── seed.go                                   # unchanged
│   ├── adaptive.go                               # NEW (small) — AdaptiveSlice helper, or inline per-resolver
│   ├── cards/
│   │   ├── anime_of_day.go                       # unchanged
│   │   ├── random_tail.go                        # unchanged
│   │   ├── latest_news.go                        # MODIFY — apply adaptive 1-2-3 rule
│   │   ├── platform_stats.go                     # unchanged
│   │   ├── personal_pick.go                      # NEW
│   │   ├── personal_pick_test.go                 # NEW
│   │   ├── telegram_news.go                      # NEW
│   │   ├── telegram_news_test.go                 # NEW
│   │   ├── now_watching.go                       # NEW
│   │   ├── now_watching_test.go                  # NEW
│   │   ├── not_time_yet.go                       # NEW
│   │   ├── not_time_yet_test.go                  # NEW
│   │   ├── continue_watching_new.go              # NEW
│   │   ├── continue_watching_new_test.go         # NEW
│   │   └── fakes_test.go                         # extend — add fakePlayerClient, fakeTelegramClient
│   └── client/
│       ├── web_client.go                         # unchanged
│       ├── player_client.go                      # NEW
│       └── player_client_test.go                 # NEW
└── transport/
    ├── router.go                                 # extend — wrap /home/spotlight with OptionalAuthMiddleware
    └── optional_auth.go                          # NEW — verbatim port of player's pattern

services/catalog/cmd/catalog-api/
└── main.go                                       # extend — wire player_client + 5 new resolvers

services/player/internal/
├── domain/
│   └── watch.go                                  # extend — UpdatedAt gets gorm:"index:idx_watch_progress_updated_at"
├── handler/
│   ├── internal_list.go                          # NEW — handler for /internal/users/{id}/list
│   └── internal_list_test.go                     # NEW
└── transport/
    └── router.go                                 # extend — register /internal/users/{id}/list at root

services/player/cmd/player-api/
└── main.go                                       # extend — register internalListHandler in router

services/gateway/internal/transport/
└── router_test.go                                # extend — assert /internal/users/* is NOT proxied

frontend/web/src/
├── types/spotlight.ts                            # extend — 5 new card variants in the discriminated union
├── components/home/spotlight/
│   ├── HeroSpotlightBlock.vue                    # MODIFY — extend v-if/v-else-if chain to 9 types
│   └── cards/
│       ├── PersonalPickCard.vue                  # NEW
│       ├── PersonalPickCard.spec.ts              # NEW
│       ├── NowWatchingCard.vue                   # NEW
│       ├── NowWatchingCard.spec.ts               # NEW
│       ├── TelegramNewsCard.vue                  # NEW
│       ├── TelegramNewsCard.spec.ts              # NEW
│       ├── NotTimeYetCard.vue                    # NEW
│       ├── NotTimeYetCard.spec.ts                # NEW
│       ├── ContinueWatchingNewCard.vue           # NEW
│       └── ContinueWatchingNewCard.spec.ts       # NEW
├── locales/
│   ├── en.json                                   # extend — 5 new spotlight.* sub-namespaces
│   └── ru.json                                   # extend — 5 new spotlight.* sub-namespaces
├── views/
│   └── Home.vue                                  # MODIFY — remove trendingRecs row + state
└── e2e/
    └── spotlight.spec.ts                         # extend — 9-card scenarios + axe re-run

CLAUDE.md                                         # MODIFY — "Adding a Spotlight Card Type" Common Tasks section
```

### Pattern 1: Optional-auth middleware (catalog side)

**What:** Reads `Authorization: Bearer <jwt>`, validates if present; on success stores claims in context; on any failure (missing, malformed, expired) silently continues as anonymous. Never 401s.

**When to use:** Wrap ONLY the `/home/spotlight` route inside `services/catalog/internal/transport/router.go`. The existing public catalog routes (`/anime/*`, etc.) are deliberately not auth-aware — keep it that way.

**Example:**
```go
// Source: services/player/internal/transport/optional_auth.go (verbatim port)
// NEW file: services/catalog/internal/transport/optional_auth.go

package transport

import (
    "net/http"

    "github.com/ILITA-hub/animeenigma/libs/authz"
    "github.com/ILITA-hub/animeenigma/libs/httputil"
)

// OptionalAuthMiddleware decodes a JWT from the Authorization header IF one
// is present and attaches the resulting Claims to the request context. It
// does NOT reject requests that lack a token or whose token is invalid —
// those simply continue without claims.
//
// Used by /home/spotlight (workstream hero-spotlight, v1.0 Phase 3) to
// unlock login-gated cards (personal_pick=login, not_time_yet,
// continue_watching_new) without rejecting anonymous traffic.
func OptionalAuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
    jwtManager := authz.NewJWTManager(jwtConfig)
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := httputil.BearerToken(r)
            if token != "" {
                if claims, err := jwtManager.ValidateAccessToken(token); err == nil {
                    r = r.WithContext(authz.ContextWithClaims(r.Context(), claims))
                }
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Then in `transport/router.go`, wrap ONLY the spotlight route:
```go
// In NewRouter, replace the current line 76:
//   r.Get("/home/spotlight", spotlightHandler.Get)
// with:
r.Group(func(r chi.Router) {
    r.Use(OptionalAuthMiddleware(cfg.JWT))
    r.Get("/home/spotlight", spotlightHandler.Get)
})
```

The handler then reads:
```go
// In handler/spotlight.go Get, change the userID = nil line:
var userID *string
if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil && claims.UserID != "" {
    uid := claims.UserID
    userID = &uid
    h.log.Infow("spotlight.request", "user", uid)
} else {
    h.log.Infow("spotlight.request", "user", "anon")
}
// ...
resp, err := h.agg.Resolve(ctx, userID)
```

### Pattern 2: Player HTTP client (mirror of `web_client.go`)

**What:** Thin `*http.Client` wrapper exposing two methods: `GetUserRecs(ctx, jwt)` and `GetUserList(ctx, userID, statuses)`. Constructor accepts an injectable `*http.Client` so tests substitute an `httptest.Server`-backed transport.

**When to use:** Inside `personal_pick` (login path), `not_time_yet`, and `continue_watching_new` resolvers.

**Example:**
```go
// NEW file: services/catalog/internal/service/spotlight/client/player_client.go

package client

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "time"
)

const (
    defaultPlayerBaseURL = "http://player:8083"
    defaultPlayerTimeout = 500 * time.Millisecond
)

// PlayerClient is a thin HTTP wrapper around player:8083 used by the
// spotlight aggregator's login-gated resolvers (personal_pick login,
// not_time_yet, continue_watching_new).
type PlayerClient struct {
    baseURL string
    http    *http.Client
}

// NewPlayerClient constructs a PlayerClient. Empty baseURL → "http://player:8083".
// Nil hc → an http.Client with the 500ms default Timeout (snug under the
// 800ms per-card budget).
func NewPlayerClient(baseURL string, hc *http.Client) *PlayerClient {
    if baseURL == "" {
        baseURL = defaultPlayerBaseURL
    }
    if hc == nil {
        hc = &http.Client{Timeout: defaultPlayerTimeout}
    }
    return &PlayerClient{baseURL: baseURL, http: hc}
}

// UserListItem is one entry from /internal/users/{user_id}/list.
// Wider than AnimeListEntry — includes derived fields (episodes_aired
// + last_watched_episode) precomputed by the player handler for
// continue_watching_new.
type UserListItem struct {
    AnimeID              string  `json:"anime_id"`
    Name                 string  `json:"name"`
    NameRU               string  `json:"name_ru,omitempty"`
    NameJP               string  `json:"name_jp,omitempty"`
    PosterURL            string  `json:"poster_url,omitempty"`
    Status               string  `json:"status"`
    EpisodesCount        int     `json:"episodes_count"`
    EpisodesAired        int     `json:"episodes_aired"`
    AnimeStatus          string  `json:"anime_status,omitempty"` // released | ongoing | announced
    LastWatchedEpisode   int     `json:"last_watched_episode"`   // 0 if no progress
    LastWatchedAt        string  `json:"last_watched_at,omitempty"`
    AnimeScore           float64 `json:"anime_score,omitempty"`
}

// GetUserList fetches /internal/users/{userID}/list?status=<csv>.
func (c *PlayerClient) GetUserList(ctx context.Context, userID, statuses string) ([]UserListItem, error) {
    u := fmt.Sprintf("%s/internal/users/%s/list?status=%s",
        c.baseURL, url.PathEscape(userID), url.QueryEscape(statuses))
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
    if err != nil {
        return nil, fmt.Errorf("player client: build request: %w", err)
    }
    req.Header.Set("User-Agent", "AnimeEnigma/1.0 (spotlight)")
    resp, err := c.http.Do(req)
    if err != nil {
        return nil, fmt.Errorf("player client: fetch user list: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
        return nil, fmt.Errorf("player client: unexpected status %d: %s", resp.StatusCode, string(body))
    }
    var out struct {
        Items []UserListItem `json:"items"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return nil, fmt.Errorf("player client: decode: %w", err)
    }
    return out.Items, nil
}

// RecsItemFromPlayer is the subset of player's RecItem the spotlight
// personal_pick resolver consumes — anime payload only, no signals/pin
// chrome. Mirrors player/internal/handler/recs.go RecAnimePayload.
type RecsItemFromPlayer struct {
    Anime struct {
        ID        string  `json:"id"`
        Name      string  `json:"name,omitempty"`
        NameRU    string  `json:"name_ru,omitempty"`
        NameJP    string  `json:"name_jp,omitempty"`
        PosterURL string  `json:"poster_url,omitempty"`
        Score     float64 `json:"score,omitempty"`
    } `json:"anime"`
}

// GetUserRecs fetches /api/users/recs from the player service. The jwt
// argument MUST be the user's Bearer token, forwarded so player's
// OptionalAuthMiddleware classifies the call as a logged-in request.
// Returns up to 10 recs (player's full response is sliced server-side).
func (c *PlayerClient) GetUserRecs(ctx context.Context, jwt string) ([]RecsItemFromPlayer, error) {
    u := c.baseURL + "/api/users/recs"
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
    if err != nil {
        return nil, fmt.Errorf("player client: build recs request: %w", err)
    }
    req.Header.Set("User-Agent", "AnimeEnigma/1.0 (spotlight)")
    if jwt != "" {
        req.Header.Set("Authorization", "Bearer "+jwt)
    }
    resp, err := c.http.Do(req)
    if err != nil {
        return nil, fmt.Errorf("player client: fetch recs: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
        return nil, fmt.Errorf("player client: unexpected status %d: %s", resp.StatusCode, string(body))
    }
    // Player wraps payload in { success, data: { recs, ... } } via httputil.OK
    var env struct {
        Success bool `json:"success"`
        Data    struct {
            Recs []RecsItemFromPlayer `json:"recs"`
        } `json:"data"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
        return nil, fmt.Errorf("player client: decode recs: %w", err)
    }
    return env.Data.Recs, nil
}
```

### Pattern 3: `now_watching` resolver — direct GORM Raw SQL

**What:** Postgres `DISTINCT ON (...)` is not a GORM-native builder, so use `db.Raw(...).Scan(&dest)`.

**When to use:** ONLY `now_watching.go`. Every other resolver uses repo methods or the player client.

**Example:**
```go
// services/catalog/internal/service/spotlight/cards/now_watching.go
package cards

import (
    "context"
    "errors"
    "math/rand/v2"
    "time"

    "gorm.io/gorm"

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

const (
    nowWatchingTTL       = 10 * time.Second
    nowWatchingMaxRows   = 5  // SQL LIMIT
    nowWatchingTopK      = 3  // after adaptive rule
    nowWatchingWindowSec = 5 * 60
)

// NowWatchingRow mirrors the SQL projection — kept package-private since
// it's translated into spotlight.NowWatchingSession before leaving here.
type nowWatchingRow struct {
    Username      string    `gorm:"column:username"`
    PublicID      string    `gorm:"column:public_id"`
    AnimeID       string    `gorm:"column:id"`
    AnimeName     string    `gorm:"column:name"`
    AnimeNameRU   string    `gorm:"column:name_ru"`
    PosterURL     string    `gorm:"column:poster_url"`
    EpisodeNumber int       `gorm:"column:episode_number"`
    UpdatedAt     time.Time `gorm:"column:updated_at"`
}

type NowWatchingResolver struct {
    db    *gorm.DB
    cache cache.Cache
    log   *logger.Logger
}

func NewNowWatchingResolver(db *gorm.DB, c cache.Cache, log *logger.Logger) *NowWatchingResolver {
    return &NowWatchingResolver{db: db, cache: c, log: log}
}

func (r *NowWatchingResolver) Type() string { return "now_watching" }

func (r *NowWatchingResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
    key := "spotlight:now_watching"
    var cached spotlight.NowWatchingData
    if err := r.cache.Get(ctx, key, &cached); err == nil {
        return &spotlight.Card{Type: r.Type(), Data: cached}, nil
    } else if !errors.Is(err, cache.ErrNotFound) {
        r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
    }

    var rows []nowWatchingRow
    sql := `
        SELECT DISTINCT ON (wp.user_id)
               u.username, u.public_id,
               a.id, a.name, a.name_ru, a.poster_url,
               wp.episode_number, wp.updated_at
        FROM watch_progress wp
        JOIN users u  ON u.id = wp.user_id
        JOIN animes a ON a.id = wp.anime_id
        WHERE wp.updated_at > NOW() - INTERVAL '5 minutes'
        ORDER BY wp.user_id, wp.updated_at DESC
        LIMIT ?
    `
    if err := r.db.WithContext(ctx).Raw(sql, nowWatchingMaxRows).Scan(&rows).Error; err != nil {
        return nil, fmt.Errorf("now_watching: db raw: %w", err)
    }

    sessions := make([]spotlight.NowWatchingSession, 0, len(rows))
    for _, row := range rows {
        sessions = append(sessions, spotlight.NowWatchingSession{
            Username:      row.Username,
            UserPublicID:  row.PublicID,
            Anime: spotlight.NowWatchingAnime{
                ID: row.AnimeID, Name: row.AnimeName, NameRU: row.AnimeNameRU,
                PosterURL: row.PosterURL,
            },
            EpisodeNumber: row.EpisodeNumber,
            UpdatedAt:     row.UpdatedAt.UTC().Format(time.RFC3339),
        })
    }

    // Adaptive 1-2-3 rule (HSB-BE-30).
    sessions = adaptiveSlice(sessions, nowWatchingTopK)
    if len(sessions) == 0 {
        return nil, nil // eligible=false; do NOT cache (Pitfall 5)
    }

    data := spotlight.NowWatchingData{Sessions: sessions}
    if err := r.cache.Set(ctx, key, data, nowWatchingTTL); err != nil {
        r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
    }
    return &spotlight.Card{Type: r.Type(), Data: data}, nil
}

// adaptiveSlice — HSB-BE-30. N=0 → empty; N=1 → all 1; N=2 → random 1;
// N≥topK → first topK.
func adaptiveSlice[T any](items []T, topK int) []T {
    switch {
    case len(items) == 0:
        return nil
    case len(items) == 1:
        return items
    case len(items) == 2:
        return []T{items[rand.IntN(2)]}
    default:
        return items[:topK]
    }
}
```

### Pattern 4: `personal_pick` resolver — anon vs login branching

**What:** Branch on `userID *string`. Anon = in-process catalog call to `GetTrendingAnime(ctx, 1, 10)`. Login = HTTP fan-out via `player_client.go`.

**When to use:** `personal_pick.go` only.

**Example:**
```go
// services/catalog/internal/service/spotlight/cards/personal_pick.go
package cards

import (
    "context"
    "errors"
    "fmt"
    "math/rand/v2"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

const personalPickTopK = 3

// trendingFetcher = subset of *service.CatalogService that personal_pick needs.
type trendingFetcher interface {
    GetTrendingAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error)
}

// recsFetcher = subset of *client.PlayerClient that personal_pick needs.
type recsFetcher interface {
    GetUserRecs(ctx context.Context, jwt string) ([]struct {
        Anime struct {
            ID, Name, NameRU, NameJP, PosterURL string
            Score                                float64
        }
    }, error)
}

type PersonalPickResolver struct {
    trending trendingFetcher
    player   recsFetcher
    cache    cache.Cache
    log      *logger.Logger
    // jwtForwarder returns the JWT for the current request — wired by main.go
    // via context value or by a small closure. Phase 3 plan should choose:
    // (a) handler passes JWT through ctx, OR
    // (b) resolver constructor accepts a function. Recommend (a) — see Pitfall 5.
}

// ... full Resolve method per Pitfall 5; see below
```

### Pattern 5: Player internal endpoint (mounted outside `/api`)

**What:** Mount `/internal/users/{user_id}/list` as a sibling to `/api`, NOT inside it. No middleware (no JWT, no rate limit needed at the player layer because the gateway already drops `/internal/*` traffic at the edge).

**When to use:** `services/player/internal/transport/router.go`. Precedent: catalog's `transport/router.go:51-65` does exactly this with `/internal/cache/invalidate/raw/{shikimoriId}` + `/internal/anime/{shikimoriId}/episodes`.

**Example:**
```go
// In services/player/internal/transport/router.go — inside NewRouter, BEFORE the r.Route("/api", ...) block:

// Workstream hero-spotlight, v1.0 Phase 3 (HSB-BE-26).
// Mounted OUTSIDE /api with no AuthMiddleware. The gateway does NOT
// proxy /internal/*, so this is reachable only from within the docker
// network (precedent: services/catalog/internal/transport/router.go
// for /internal/cache/invalidate/raw/{shikimoriId}).
if internalListHandler != nil {
    r.Get("/internal/users/{user_id}/list", internalListHandler.GetUserList)
}
```

The handler:
```go
// services/player/internal/handler/internal_list.go
package handler

import (
    "net/http"
    "strings"

    "github.com/ILITA-hub/animeenigma/libs/httputil"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/player/internal/domain"
    "github.com/ILITA-hub/animeenigma/services/player/internal/service"
    "github.com/go-chi/chi/v5"
)

type InternalListHandler struct {
    listService     *service.ListService
    progressService progressLookup // narrow interface; satisfied by *service.ProgressService
    log             *logger.Logger
}

type progressLookup interface {
    // Returns the user's latest watch_progress.episode_number per anime in the input set.
    LatestEpisodePerAnime(ctx context.Context, userID string, animeIDs []string) (map[string]int, error)
}

func NewInternalListHandler(listSvc *service.ListService, progSvc progressLookup, log *logger.Logger) *InternalListHandler {
    return &InternalListHandler{listService: listSvc, progressService: progSvc, log: log}
}

// GetUserList serves /internal/users/{user_id}/list?status=<csv>.
// No auth — docker network trust boundary.
func (h *InternalListHandler) GetUserList(w http.ResponseWriter, r *http.Request) {
    userID := chi.URLParam(r, "user_id")
    if userID == "" {
        httputil.BadRequest(w, "user_id is required")
        return
    }
    statusParam := strings.TrimSpace(r.URL.Query().Get("status"))
    if statusParam == "" {
        httputil.BadRequest(w, "status query param required")
        return
    }
    statuses := strings.Split(statusParam, ",")
    for i, s := range statuses {
        statuses[i] = strings.TrimSpace(s)
    }

    entries, err := h.listService.GetUserListByStatuses(r.Context(), userID, statuses)
    if err != nil {
        h.log.Errorw("internal_list: get_by_statuses failed", "user_id", userID, "error", err)
        httputil.Error(w, err)
        return
    }
    // Pull last_watched per anime in one call so the resolver doesn't N+1.
    ids := make([]string, 0, len(entries))
    for _, e := range entries {
        ids = append(ids, e.AnimeID)
    }
    progressMap, _ := h.progressService.LatestEpisodePerAnime(r.Context(), userID, ids)

    items := make([]map[string]any, 0, len(entries))
    for _, e := range entries {
        item := map[string]any{
            "anime_id":             e.AnimeID,
            "status":               e.Status,
            "last_watched_episode": progressMap[e.AnimeID],
        }
        if e.Anime != nil {
            item["name"]            = e.Anime.Name
            item["name_ru"]         = e.Anime.NameRU
            item["name_jp"]         = e.Anime.NameJP
            item["poster_url"]      = e.Anime.PosterURL
            item["episodes_count"]  = e.Anime.EpisodesCount
            item["episodes_aired"]  = e.Anime.EpisodesAired
        }
        items = append(items, item)
    }
    httputil.OK(w, map[string]any{"items": items})
}
```

NOTE: `ListService.GetUserListByStatuses` does not exist yet — `service/list.go` has `GetUserList(ctx, userID, status string)` (single status). The plan should either:
- Add a new method `GetUserListByStatuses(ctx, userID, statuses []string)` that delegates to `listRepo.GetByUserAndStatuses(ctx, userID, statuses)` (already exists at `repo/list.go:153`), OR
- Reuse `repo.GetByUserAndStatuses` directly in the handler (skip the service layer for this one read).
- **Recommend the first** — keep handler→service→repo discipline.

### Pattern 6: Frontend card SFC (typed `data` prop, Phase 2 template)

Each new card follows the Phase 2 AnimeOfDayCard.vue template: `<script setup lang="ts">` with `defineProps<{ data: SomeData }>()`, `useI18n()` for `t()`, mobile-first layout, no hardcoded strings.

**Example skeleton (NotTimeYetCard):**
```vue
<template>
  <article class="w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-4 lg:p-6">
    <header class="md:hidden">
      <p class="text-xs font-medium text-pink-400 uppercase tracking-wider mb-1">
        {{ t('spotlight.notTimeYet.title') }}
      </p>
    </header>
    <router-link
      :to="`/anime/${data.anime.id}`"
      class="flex-shrink-0 self-center md:self-start w-32 md:w-40 lg:w-48 group"
    >
      <div class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3]">
        <img :src="data.anime.poster_url || '/placeholder.svg'" :alt="title" loading="lazy"
          class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300" />
      </div>
    </router-link>
    <div class="flex-1 flex flex-col justify-between gap-3 min-w-0">
      <div>
        <p class="hidden md:block text-xs font-medium text-pink-400 uppercase tracking-wider mb-2">
          {{ t('spotlight.notTimeYet.title') }}
        </p>
        <h3 class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2">
          {{ title }}
        </h3>
        <p class="text-sm md:text-base font-medium text-gray-400 mt-2">
          {{ subline }}
        </p>
      </div>
      <router-link :to="`/anime/${data.anime.id}`" class="btn-primary">
        {{ t('spotlight.notTimeYet.cta') }}
      </router-link>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import type { NotTimeYetData } from '@/types/spotlight'

const props = defineProps<{ data: NotTimeYetData }>()
const { t } = useI18n()

const title = computed(() =>
  getLocalizedTitle(props.data.anime.name, props.data.anime.name_ru, props.data.anime.name_jp),
)
const subline = computed(() => {
  if (props.data.list_status === 'planned') return t('spotlight.notTimeYet.fromPlanned')
  return t('spotlight.notTimeYet.fromPostponed')
})
</script>
```

### Pattern 7: Extending HeroSpotlightBlock dispatch chain

Phase 2's chain uses v-if/v-else-if (NOT a lookup map) because `vue-tsc` strict-mode widens the data prop to the union under `<component :is>`. The Phase 3 extension keeps the same pattern, just adds 5 more branches:

```vue
<!-- HeroSpotlightBlock.vue extended (lines 70-89 region) -->
<AnimeOfDayCard
  v-if="active.type === 'anime_of_day'"
  :key="`anime_of_day:${currentIndex}`"
  :data="active.data"
/>
<RandomTailCard
  v-else-if="active.type === 'random_tail'"
  :key="`random_tail:${currentIndex}`"
  :data="active.data"
/>
<LatestNewsCard
  v-else-if="active.type === 'latest_news'"
  :key="`latest_news:${currentIndex}`"
  :data="active.data"
/>
<PlatformStatsCard
  v-else-if="active.type === 'platform_stats'"
  :key="`platform_stats:${currentIndex}`"
  :data="active.data"
/>
<!-- NEW Phase 3 -->
<PersonalPickCard
  v-else-if="active.type === 'personal_pick'"
  :key="`personal_pick:${currentIndex}`"
  :data="active.data"
/>
<NowWatchingCard
  v-else-if="active.type === 'now_watching'"
  :key="`now_watching:${currentIndex}`"
  :data="active.data"
/>
<TelegramNewsCard
  v-else-if="active.type === 'telegram_news'"
  :key="`telegram_news:${currentIndex}`"
  :data="active.data"
/>
<NotTimeYetCard
  v-else-if="active.type === 'not_time_yet'"
  :key="`not_time_yet:${currentIndex}`"
  :data="active.data"
/>
<ContinueWatchingNewCard
  v-else-if="active.type === 'continue_watching_new'"
  :key="`continue_watching_new:${currentIndex}`"
  :data="active.data"
/>
```

Plus the 5 new imports under the existing 4.

### Anti-Patterns to Avoid

- **DO NOT use `cache.GetOrSet`** — it caches the nil/empty return from the resolver, baking a "no data" cache for the full TTL. Use manual `Get` + `errors.Is(err, cache.ErrNotFound)` + `Set` (Phase 1 DELIBERATE DIVERGENCE 1).
- **DO NOT skip the per-resolver context deadline** — Phase 1's aggregator wraps each `Resolve` call in `context.WithTimeout(ctx, 800ms)`. The new resolvers inherit that automatically. Don't add a SECOND timeout inside the resolver — it's redundant and risks races with the HTTP client's own Timeout.
- **DO NOT cache the `now_watching` payload for >10s** — it's a live data card; longer TTL would surface stale "user X is watching ep N" for users who actually finished hours ago.
- **DO NOT use `errgroup` for the player_client list query** — only one HTTP call per resolver, no parallelism needed.
- **DO NOT add a new Redis key prefix for telegram_news** — HSB-NF-03 says `spotlight:` for NEW keys only; `news:telegram` is the EXISTING key and stays reused (HSB-BE-21).
- **DO NOT mount `/internal/users/{id}/list` inside `r.Route("/api", ...)`** — must be at the root level so the gateway's `/api/*` proxy never sees it.
- **DO NOT forward the JWT to player's `/internal/users/...` endpoint** — it's a docker-network-internal endpoint with no auth; sending the JWT leaks unnecessarily.
- **DO forward the JWT to player's `/api/users/recs` endpoint** — that endpoint uses OptionalAuthMiddleware on the player side, so JWT presence determines anon-vs-login behavior.
- **DO NOT remove the feature flags in Phase 3** — HSB-MIG-02 keeps them as kill switches for one release.
- **DO NOT mix `vue-tsc` `<component :is="...">` with the typed data prop** — the `data` prop widens to the union and breaks type-narrowing. Use the v-if/v-else-if chain (Phase 2 precedent).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JWT validation in catalog | Custom Bearer parser + signature check | `libs/authz.NewJWTManager(cfg.JWT).ValidateAccessToken(token)` | Already implemented (`libs/authz/jwt.go:109-130`); also handles `ErrTokenExpired` sentinel for clean error classification |
| Bearer header extraction | `strings.HasPrefix(header, "Bearer ") + strings.TrimPrefix(...)` | `httputil.BearerToken(r)` | Already exists; consistent semantics across services |
| `OptionalAuthMiddleware` from scratch | Custom mux interceptor | Copy `services/player/internal/transport/optional_auth.go` verbatim | 35-line file battle-tested in Phase 10 of the recs workstream |
| Random pick from a slice | `rand.Seed(time.Now()...) + rand.Intn(n)` | `math/rand/v2.IntN(n)` (Go 1.22+) | New `rand/v2` package is auto-seeded, no global state mutation; matches modern Go idiom |
| Aggregating list items by user across services | Replicating `anime_list` table to catalog | `player_client.go` HTTP fan-out | `anime_list` is owned by player service; aggregation creates a multi-writer problem |
| Forwarding JWT through goroutines | Stashing in package globals | `context.WithValue(ctx, jwtKey{}, jwt)` and reading inside the resolver | Standard Go ctx pattern; thread-safe by construction |
| Per-card-type Vue dispatch map | `Record<SpotlightCardType, Component>` lookup | v-if/v-else-if chain | `vue-tsc` widens the data prop under `<component :is>`; chain preserves narrowing (Phase 2 precedent) |
| Vitest mock of `useSpotlight` per spec | New `__mocks__/useSpotlight.ts` | `vi.mock('@/composables/useSpotlight', () => ({ useSpotlight: () => mockState }))` at the top of the spec | Phase 2 HeroSpotlightBlock.spec.ts:51 precedent — works perfectly, no global mock dir needed |
| i18n key validation | Manual file diff | Vitest spec that loads both en.json + ru.json and asserts key parity on the `spotlight.*` subtree | Phase 2 ships `i18n-parity.spec.ts` (or equivalent) — Phase 3 extends with new keys |
| HLS proxy allowlist update | Modifying `libs/videoutils/proxy.go` | Not needed in Phase 3 | No new external CDN hosts; no video work in this phase |

**Key insight:** Phase 3 introduces ZERO new primitive patterns. Every new file either mirrors a Phase 1/2 file or extends an existing one. The planner's job is composition, not invention.

## Common Pitfalls

### Pitfall 1: Forwarding the user's JWT into the spotlight resolver chain

**What goes wrong:** `personal_pick` login path needs the user's Bearer token to call player's `/api/users/recs` (so player's OptionalAuthMiddleware classifies the call as logged-in). But the aggregator's `Resolve(ctx, userID *string)` signature passes only the user ID — no JWT.

**Why it happens:** The Resolver interface was designed for Phase 1, which had no fan-out cards. Phase 3 introduces the first resolver that needs the raw token.

**How to avoid:** Stash the JWT in the request context via the handler, and let the resolver read it via a context value. Example:
```go
// In handler/spotlight.go, after extracting claims:
type spotlightJWTKey struct{}
ctx := r.Context()
if jwt := httputil.BearerToken(r); jwt != "" {
    ctx = context.WithValue(ctx, spotlightJWTKey{}, jwt)
}
// pass ctx into aggregator.Resolve(ctx, userID)
```
Then in the resolver:
```go
if jwt, ok := ctx.Value(spotlightJWTKey{}).(string); ok && jwt != "" {
    return r.player.GetUserRecs(ctx, jwt)
}
```
**Alternative:** Add a `jwt string` field to the `Resolver.Resolve(ctx, userID *string, jwt string)` signature — but that breaks all 4 Phase 1 resolvers and requires a wider blast radius. Context-value passing is the lowest-friction approach.

**Warning signs:** `personal_pick` resolver returns anonymous trending for logged-in users (because the JWT never reached the player call).

### Pitfall 2: Snapshot fallback key collision with `now_watching` short TTL

**What goes wrong:** Phase 1's `Aggregator.Resolve` writes a 24h `spotlight:snapshot:<anon|uid>:<date>` after a successful aggregation. If `now_watching` was eligible during the snapshot write but no longer eligible 10 minutes later, the snapshot still surfaces stale "user X is watching" — for up to 24h.

**Why it happens:** The snapshot is a "last-known-good full response" — it doesn't know that one of its cards has a 10-second TTL.

**How to avoid:** This is mostly a CONTRACTUAL acceptance — the snapshot fallback is only consumed when ZERO cards resolve fresh, which is rare (at minimum the 4 static cards always succeed). When the snapshot DOES fire, the now_watching row inside it is stale by definition — that's the price of partial-success degradation. Add a comment in the resolver acknowledging this. Phase 4 could add per-card snapshot freshness tags if real-world telemetry shows the staleness is user-visible.

**Warning signs:** "Now watching @username — anime title" entry on the spotlight where the username's actual session ended hours ago. Trace via the cache-miss log lines: if `loadSnapshot` returned a non-nil resp and `now_watching` is in it, that's the path.

### Pitfall 3: Anon `personal_pick` Redis key conflict with `random_tail`

**What goes wrong:** CONTEXT.md proposes `spotlight:trending:<YYYY-MM-DD>` for anon `personal_pick`. Phase 1's `random_tail` uses `spotlight:random_tail:<YYYY-MM-DD>` — those are DISTINCT keys. **No conflict.** Documenting here so the planner doesn't second-guess.

**Why it could happen if you weren't careful:** If a future iteration consolidates the trending list cache (e.g. `spotlight:trending:<date>` for both random_tail and personal_pick), the key would conflict. Don't do that — keep them separate.

**How to avoid:** Verify the Redis key list during Plan-Phase by grepping `cards/*.go` for `key := "spotlight:`. The 9 keys MUST be unique.

### Pitfall 4: `personal_pick` for N=2 displays mobile "+ ещё 2 →" link incorrectly

**What goes wrong:** PersonalPickCard's mobile layout shows "1 poster + '+ ещё 2 →' link". If the adaptive rule produces N=2 (it doesn't — see CONTEXT.md `<decisions>` Adaptive 1-2-3 rule), the "ещё 2" text is wrong (should say "ещё 1").

**Why it doesn't actually happen:** The adaptive rule guarantees the resolver returns ONLY 1 or 3 items (N=2 → random pick 1). So the mobile "+ ещё 2 →" link only renders when `data.items.length === 3`. The card template should `v-if="data.items.length > 1"` for the link.

**How to avoid:** Card template:
```vue
<router-link
  v-if="data.items.length > 1"
  :to="targetRoute"
  class="md:hidden text-sm font-medium text-cyan-400"
>
  {{ t('spotlight.personalPick.moreLink', { n: data.items.length - 1 }) }}
</router-link>
```
The `{ n: data.items.length - 1 }` interpolation makes the copy correct regardless of N.

**Warning signs:** Mobile spotlight slide shows "+ ещё 2 →" when only 1 follow-up item exists.

### Pitfall 5: `cache.GetOrSet` would cache empty resolver returns

**What goes wrong:** The convenience helper `cache.GetOrSet(ctx, key, &dest, ttl, fn)` runs `fn`, gets `(nil, nil)`, and writes a zero-value struct to Redis for the full TTL. Next request hits the cache, decodes the zero value, and the resolver's "if data.Anime.ID == ''" guard fires → eligibility=false. The card stays dark all day.

**Why it happens:** `GetOrSet` is designed for resources where empty = "valid, just no data", not for ours where empty = "try again next request".

**How to avoid:** **Manual `Get` + `errors.Is(err, cache.ErrNotFound)` + `Set` discipline.** All 4 Phase 1 resolvers do this. Phase 3 MUST follow. The `cards/fakes_test.go:101` `GetOrSet` panic enforces this at test time — leave that guard in place when extending the fakes.

**Warning signs:** A card disappears mid-day and never returns. Redis key has an empty `{anime: {id: ""}}` JSON value.

### Pitfall 6: Optional-auth middleware panic on malformed Authorization header

**What goes wrong:** A client sends `Authorization: garbage` or `Authorization: Bearer` (empty token). The middleware crashes or 401s.

**Why it shouldn't happen:** `httputil.BearerToken(r)` returns `""` for missing or non-Bearer headers; `jwtManager.ValidateAccessToken("")` returns an error (not a panic). On error the middleware silently strips and continues. The pattern is proven in player's `OptionalAuthMiddleware`.

**How to avoid:** Copy player's `transport/optional_auth.go` verbatim — DO NOT modify it. The 35 lines are a finished primitive.

**Warning signs:** 500 errors or unauthorized panics in catalog when garbage Authorization headers arrive.

### Pitfall 7: `idx_watch_progress_updated_at` already exists with a different name

**What goes wrong:** AutoMigrate ADDS indexes but never DROPS them. If the index already exists with a system-generated name (e.g. `idx_watch_progress_updated_at_idx_n`), the migration creates a SECOND index with the prescribed name. Two indexes on the same column = wasted INSERT/UPDATE cost.

**Why it could happen:** A previous attempt to add the index could have left an artifact.

**How to avoid:** Before adding the tag, run `\d+ watch_progress` to verify no index references `updated_at` exists. If one does with a different name, the plan should DROP the old one first via a manual SQL migration step.

**Verification command:**
```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "\d+ watch_progress" | grep -i updated_at
```

**Warning signs:** `\d+ watch_progress` after the deploy shows two indexes referencing `updated_at`.

### Pitfall 8: `now_watching` SQL slow on first request (cold cache)

**What goes wrong:** Without `idx_watch_progress_updated_at`, the `WHERE wp.updated_at > NOW() - INTERVAL '5 minutes'` predicate seq-scans `watch_progress`. On a table with 100k+ rows the query takes 200-500ms — eating into the 800ms per-card budget.

**Why it happens:** The composite index `idx_watch_progress_user_anime_ep` doesn't cover updated_at as a leading column.

**How to avoid:** The HSB-NF-02 index addition is on the critical path for this resolver to meet its latency budget. Order the plan tasks so the index migration ships BEFORE or with the resolver — never after.

**Warning signs:** First-request latency on `/api/home/spotlight` spikes to >1.5s when `now_watching` is eligible. Prometheus `http_request_duration_seconds{path="/api/home/spotlight"}` p95 above 1500ms.

### Pitfall 9: Player's `/internal/users/{id}/list` 500s on unknown user_id

**What goes wrong:** A bad actor or a buggy resolver calls `/internal/users/INVALID-UUID/list?status=watching`. The handler tries `listService.GetUserListByStatuses(ctx, "INVALID-UUID", ["watching"])`, which queries an empty result set. The current `repo.GetByUserAndStatuses` returns `[]*AnimeListEntry{}, nil` — fine. But if the user_id is malformed (not a UUID), GORM might 500.

**Why it happens:** GORM type validation on `user_id uuid` rejects non-UUID input at the driver level.

**How to avoid:** Validate the user_id parameter at handler entry — match against a UUID regex. If invalid, return 400. Don't 500. Pattern: `services/catalog/internal/handler/internal_episodes.go:62-66` does the same thing with `shikimoriIDPattern`.

**Example UUID regex:**
```go
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
```

**Warning signs:** 500s in player logs when spotlight resolvers fan out for users with deleted accounts (their UUID is still valid format but no longer exists — handler returns 200 with empty items, that's fine; we're protecting against MALFORMED UUIDs, not non-existent ones).

### Pitfall 10: HSB-MIG-02 flag retention — plan must NOT include flag removal

**What goes wrong:** The planner over-interprets "remove the legacy trending row" and also removes the `VITE_HERO_SPOTLIGHT_ENABLED` / `SPOTLIGHT_ENABLED` flags from `.env`, `App.vue`, `HeroSpotlightBlock.vue`, and catalog config. This breaks the kill switch.

**Why it happens:** "Remove" can mean "remove everything related to the old setup". The CONTEXT.md is explicit but easy to miss.

**How to avoid:** Plan tasks MUST NOT include flag removal. Verification grep: after Phase 3 merges, `grep -rn "VITE_HERO_SPOTLIGHT_ENABLED\|SPOTLIGHT_ENABLED" services/ frontend/web/src/ docker/` should still find the flag references.

**Warning signs:** Setting `SPOTLIGHT_ENABLED=false` no longer returns 404; setting `VITE_HERO_SPOTLIGHT_ENABLED=false` no longer hides the block.

### Pitfall 11: `personal_pick` anon path silently breaks when catalog can't reach itself

**What goes wrong:** `personal_pick` anon calls `catalogService.GetTrendingAnime(ctx, 1, 10)`. Trending is itself cache-backed by `cache.KeyTopAnime()`; on cache miss it falls back to a Shikimori API call. Shikimori has rate limits and occasional 5xx — that error propagates up to the resolver and the card drops.

**Why it happens:** Trending is a "live" upstream call when the cache is cold.

**How to avoid:** This is acceptable failure mode — if trending is broken, `personal_pick` anon is dropped, the other 8 cards still render. Don't paper over it. Just ensure the resolver `return nil, fmt.Errorf("personal_pick: trending: %w", err)` so the aggregator logs `spotlight.card_failed{type=personal_pick, error=...}`.

**Warning signs:** Anon spotlight has 4 cards instead of 5 when Shikimori is down. The structured log shows `spotlight.card_failed{type=personal_pick}`.

### Pitfall 12: Removing `useRecs` from Home.vue breaks imports used elsewhere

**What goes wrong:** Home.vue imports `useRecs, type RecItem` (line 441) and `emitRecClick, type PinSource` (line 443). After removing `trendingRecs`, these imports may still be referenced elsewhere in Home.vue, or be cleanly removable. If only the trending row uses them, they can be deleted. If anything else uses them, leave them.

**Why it could happen:** Phase 3 is a surgical removal — only the trending block goes. Search for `useRecs`, `RecItem`, `emitRecClick`, `PinSource` in Home.vue after the markup removal. The grep should show only the now-orphaned import lines.

**How to avoid:** Grep verification step in the plan:
```bash
grep -n "useRecs\|RecItem\|emitRecClick\|PinSource\|useAnimeProgress" /data/animeenigma/frontend/web/src/views/Home.vue
```
If grep returns only the import line + ZERO body references → remove the import. If it returns body references → leave the import, only remove what's safe.

**Warning signs:** `bun run build` fails with "unused import" or "RecItem is not exported" after the changes ship.

### Pitfall 13: i18n parity test misses the new spotlight sub-namespaces

**What goes wrong:** Phase 2's parity test checks the `spotlight.*` subtree. If it iterates only the keys it knows about, the 5 new sub-namespaces could exist in en.json but be missing from ru.json (or vice versa) and the test passes.

**Why it happens:** Tests often hardcode known keys.

**How to avoid:** Make the parity test STRUCTURAL: walk both JSONs starting from `spotlight.*` and assert every leaf key in EN has a matching key in RU and vice versa. Pattern:
```ts
function flattenKeys(obj: Record<string, unknown>, prefix = ''): string[] {
  const result: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    const full = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'object' && v !== null) result.push(...flattenKeys(v as Record<string, unknown>, full))
    else result.push(full)
  }
  return result
}
const enKeys = new Set(flattenKeys(enJson.spotlight, 'spotlight'))
const ruKeys = new Set(flattenKeys(ruJson.spotlight, 'spotlight'))
expect([...enKeys].sort()).toEqual([...ruKeys].sort())
```

**Warning signs:** Spotlight cards render `spotlight.notTimeYet.title` raw key strings in production (no Russian translation found, falls through to key literal).

## Runtime State Inventory

Phase 3 is mostly additive but includes:
- DB schema change (HSB-NF-02 index addition)
- Vue.js component removal in Home.vue (HSB-MIG-01)
- New Redis keys (NOT a state change — keys are written fresh on first request, never migrated)

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | **Postgres**: `watch_progress.updated_at` column gets a new index `idx_watch_progress_updated_at`. No data migration — index is additive. | None — `AutoMigrate` adds the index on next player restart. |
| Stored data | **Redis**: 5 new key families (`spotlight:trending:<date>`, `spotlight:personal:<uid>:<date>`, `spotlight:now_watching`, `spotlight:not_time_yet:<uid>`, `spotlight:continue_new:<uid>`). No migration needed — keys appear fresh on first request. | None — fresh write per request. |
| Stored data | **Redis (reused)**: `news:telegram` (existing, 30min TTL). DO NOT migrate or rename. | None — `telegram_news` resolver reads the existing key. |
| Live service config | None. No n8n / Cloudflare / external service config touched. | None. |
| OS-registered state | None — no systemd/cron/Task Scheduler registrations involve "spotlight" or "hero" terms. | None. |
| Secrets/env vars | Reuses `JWT_SECRET` (catalog already declares it), `DB_HOST/PASSWORD/NAME` (catalog already declares it). No new secrets. | None. |
| Build artifacts | Frontend bundle (Vite) — `bun run build` regenerates `frontend/web/dist/` from sources. No stale artifacts because the changelog/etc. files are static. | Run `make redeploy-web` after frontend changes. |
| Build artifacts | Go binaries — `make redeploy-catalog` and `make redeploy-player` rebuild from source. No cached image staleness because `Dockerfile` uses `COPY` for sources. | Run `make redeploy-catalog && make redeploy-player`. |

**Nothing found in category "Live service config", "OS-registered state", "Secrets/env vars" — verified by searches for `spotlight`, `personal_pick`, `now_watching` across `docker/`, `deploy/`, `infra/`.**

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| PostgreSQL | All resolvers (catalog) + watch_progress reads | ✓ | 16.x (per `docker/docker-compose.yml`) | None — phase blocks if down |
| Redis | All resolvers (caching) | ✓ | 7.x | Cache-miss falls through to compute; `spotlight.cache_get_failed` Warnw on hard failure |
| Player service | personal_pick (login), not_time_yet, continue_watching_new | ✓ | Internal | Resolver fails → card dropped → other 8 cards still render (graceful degradation per HSB-BE-03/05) |
| Telegram parser (t.me/s/...) | telegram_news | ✓ (HTTP fetch) | — | Resolver fails → card dropped → other cards still render |
| Catalog service trending (Shikimori-backed) | personal_pick (anon) | ✓ | Internal | Resolver fails → card dropped |
| `make redeploy-catalog` | Phase 3 ship | ✓ | Internal command | None — blocks deploy if missing |
| `make redeploy-player` | Phase 3 ship | ✓ | Internal command | None |
| `make redeploy-web` | Phase 3 ship | ✓ | Internal command | None |
| `bun` (1.x) | Frontend build/test | ✓ | (system PATH) | None |
| `go` (1.24+) | Backend build/test | ✓ | go1.24.x | None |
| `docker compose` | Test infra | ✓ | (system PATH) | None |
| `bunx playwright` + chromium | e2e tests | ✓ | 1.58.0 + (auto-install) | None |
| `axe-core/playwright` | a11y gate | ✓ (installed Phase 2) | — | None |

**Missing dependencies with no fallback:** None. All Phase 3 prerequisites are already in place from Phase 1 + Phase 2.

**Missing dependencies with fallback:** All external upstream dependencies (Telegram, Shikimori) have graceful degradation via per-card eligibility filter.

## Code Examples

### Adaptive 1-2-3 helper (shared)

```go
// services/catalog/internal/service/spotlight/adaptive.go
package spotlight

import "math/rand/v2"

// AdaptiveSlice applies the HSB-BE-30 layout rule:
//
//   - len(items) == 0 → nil (resolver returns (nil, nil) = ineligible)
//   - len(items) == 1 → all 1
//   - len(items) == 2 → ONE randomly-picked item
//   - len(items) >= topK (3) → items[:topK]
//
// The function is generic over the element type so resolvers can apply
// the rule to []NowWatchingSession, []TelegramPost, []ChangelogEntry,
// and []*domain.Anime uniformly.
func AdaptiveSlice[T any](items []T, topK int) []T {
    if len(items) == 0 {
        return nil
    }
    if len(items) == 1 {
        return items
    }
    if len(items) == 2 {
        return []T{items[rand.IntN(2)]}
    }
    return items[:topK]
}
```

Then in each resolver: `sessions = spotlight.AdaptiveSlice(sessions, 3)`.

### Modify `latest_news` to apply the adaptive rule (retrofit)

```go
// services/catalog/internal/service/spotlight/cards/latest_news.go — line 53 area
entries, err := r.web.GetChangelog(ctx)
if err != nil {
    return nil, fmt.Errorf("latest_news: fetch changelog: %w", err)
}

// Phase 3 retrofit (HSB-BE-30): apply adaptive 1-2-3 rule.
// Phase 1 capped at 3 entries via web_client.maxChangelogEntries; Phase 3
// adds the N=2 → random-1 case. (The N=2 case is extremely rare for the
// changelog so this almost never triggers, but the rule is uniform across
// all multi-item cards per HSB-BE-30.)
entries = spotlight.AdaptiveSlice(entries, 3)
if len(entries) == 0 {
    return nil, nil
}
data := spotlight.LatestNewsData{Entries: entries}
```

### Personal pick resolver (full)

```go
// services/catalog/internal/service/spotlight/cards/personal_pick.go
package cards

import (
    "context"
    "errors"
    "fmt"
    "math/rand/v2"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
)

const personalPickTopK = 3
const personalPickPoolSize = 10

// trendingFetcher narrows the *service.CatalogService surface.
type trendingFetcher interface {
    GetTrendingAnime(ctx context.Context, page, pageSize int) ([]*domain.Anime, int64, error)
}

// PersonalPickResolver — anon path: trending top-10 → random 3.
// Login path: player_client.GetUserRecs() top-10 → random 3.
type PersonalPickResolver struct {
    trending trendingFetcher
    player   *client.PlayerClient
    cache    cache.Cache
    log      *logger.Logger
}

func NewPersonalPickResolver(t trendingFetcher, p *client.PlayerClient, c cache.Cache, log *logger.Logger) *PersonalPickResolver {
    return &PersonalPickResolver{trending: t, player: p, cache: c, log: log}
}

func (r *PersonalPickResolver) Type() string { return "personal_pick" }

// spotlightJWTKey — context key for the request JWT (Pitfall 1).
type spotlightJWTKey struct{}

// ContextWithJWT stashes the request's Bearer JWT into ctx for the
// personal_pick login resolver. Defined here (not in spotlight package)
// because only this resolver needs it. The handler is responsible for
// putting it in; this resolver is responsible for reading it.
func ContextWithJWT(ctx context.Context, jwt string) context.Context {
    return context.WithValue(ctx, spotlightJWTKey{}, jwt)
}

func jwtFromContext(ctx context.Context) string {
    v, _ := ctx.Value(spotlightJWTKey{}).(string)
    return v
}

func (r *PersonalPickResolver) Resolve(ctx context.Context, userID *string) (*spotlight.Card, error) {
    var key string
    if userID == nil {
        key = "spotlight:trending:" + spotlight.DateKeyUTC(time.Now())
    } else {
        key = "spotlight:personal:" + *userID + ":" + spotlight.DateKeyUTC(time.Now())
    }

    var cached spotlight.PersonalPickData
    if err := r.cache.Get(ctx, key, &cached); err == nil {
        return &spotlight.Card{Type: r.Type(), Data: cached}, nil
    } else if !errors.Is(err, cache.ErrNotFound) {
        r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
    }

    var pool []spotlight.PersonalPickItem
    if userID == nil {
        items, _, err := r.trending.GetTrendingAnime(ctx, 1, personalPickPoolSize)
        if err != nil {
            return nil, fmt.Errorf("personal_pick: trending: %w", err)
        }
        for _, a := range items {
            pool = append(pool, toPersonalPickItem(a))
        }
    } else {
        jwt := jwtFromContext(ctx)
        if jwt == "" {
            r.log.Warnw("spotlight.personal_pick_no_jwt", "user_id", *userID)
            return nil, nil // shouldn't happen — handler always sets jwt when userID is non-nil
        }
        recs, err := r.player.GetUserRecs(ctx, jwt)
        if err != nil {
            return nil, fmt.Errorf("personal_pick: recs fan-out: %w", err)
        }
        if len(recs) > personalPickPoolSize {
            recs = recs[:personalPickPoolSize]
        }
        for _, rec := range recs {
            pool = append(pool, spotlight.PersonalPickItem{
                Anime: spotlight.PersonalPickAnime{
                    ID:        rec.Anime.ID,
                    Name:      rec.Anime.Name,
                    NameRU:    rec.Anime.NameRU,
                    NameJP:    rec.Anime.NameJP,
                    PosterURL: rec.Anime.PosterURL,
                    Score:     rec.Anime.Score,
                },
            })
        }
    }

    if len(pool) == 0 {
        return nil, nil
    }

    // Random 3 from the top-10 pool (not just the first 3).
    rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
    picked := pool
    if len(picked) > personalPickTopK {
        picked = picked[:personalPickTopK]
    }

    // Then apply adaptive 1-2-3 rule on the picked set (N=3 ideal, N=1 or 2
    // possible if pool was small).
    picked = spotlight.AdaptiveSlice(picked, personalPickTopK)
    if len(picked) == 0 {
        return nil, nil
    }

    data := spotlight.PersonalPickData{Items: picked}
    if err := r.cache.Set(ctx, key, data, 24*time.Hour); err != nil {
        r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
    }
    return &spotlight.Card{Type: r.Type(), Data: data}, nil
}

func toPersonalPickItem(a *domain.Anime) spotlight.PersonalPickItem {
    return spotlight.PersonalPickItem{
        Anime: spotlight.PersonalPickAnime{
            ID:        a.ID,
            Name:      a.Name,
            NameRU:    a.NameRU,
            NameJP:    a.NameJP,
            PosterURL: a.PosterURL,
            Score:     a.Score,
        },
    }
}
```

### Handler patch for JWT pass-through

```go
// services/catalog/internal/handler/spotlight.go — modified Get():
func (h *SpotlightHandler) Get(w http.ResponseWriter, r *http.Request) {
    if !h.enabled {
        w.WriteHeader(http.StatusNotFound)
        return
    }
    started := time.Now()

    // Phase 3 — read optional claims set by OptionalAuthMiddleware.
    var userID *string
    var jwt string
    if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil && claims.UserID != "" {
        uid := claims.UserID
        userID = &uid
        jwt = httputil.BearerToken(r)
        h.log.Infow("spotlight.request", "user", uid)
    } else {
        h.log.Infow("spotlight.request", "user", "anon")
    }

    ctx, cancel := context.WithTimeout(r.Context(), spotlightCtxTimeout)
    defer cancel()
    if jwt != "" {
        ctx = cards.ContextWithJWT(ctx, jwt)
    }

    resp, err := h.agg.Resolve(ctx, userID)
    if err != nil { /* unchanged */ }
    // ... unchanged
}
```

### Vue card test pattern (PersonalPickCard.spec.ts)

```ts
// frontend/web/src/components/home/spotlight/cards/PersonalPickCard.spec.ts
import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import PersonalPickCard from './PersonalPickCard.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
  }),
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (n?: string, r?: string, j?: string) => n || r || j || '',
}))

describe('PersonalPickCard', () => {
  it('renders 3 posters when items.length === 3', () => {
    const wrapper = mount(PersonalPickCard, {
      global: { stubs: { 'router-link': RouterLinkStub } },
      props: {
        data: {
          items: [
            { anime: { id: 'a', name: 'Anime A' } },
            { anime: { id: 'b', name: 'Anime B' } },
            { anime: { id: 'c', name: 'Anime C' } },
          ],
        },
      },
    })
    expect(wrapper.findAll('img')).toHaveLength(3)
  })

  it('renders single poster with mobile "+ ещё 2 →" link when items.length === 3', () => {
    const wrapper = mount(PersonalPickCard, {
      global: { stubs: { 'router-link': RouterLinkStub } },
      props: {
        data: {
          items: [
            { anime: { id: 'a' } },
            { anime: { id: 'b' } },
            { anime: { id: 'c' } },
          ],
        },
      },
    })
    // The mobile-only link uses the t() output with n=2 (items.length - 1)
    expect(wrapper.html()).toContain('spotlight.personalPick.moreLink::{"n":2}')
  })

  it('does NOT render the "+ ещё" link when items.length === 1', () => {
    const wrapper = mount(PersonalPickCard, {
      global: { stubs: { 'router-link': RouterLinkStub } },
      props: { data: { items: [{ anime: { id: 'a' } }] } },
    })
    expect(wrapper.html()).not.toContain('spotlight.personalPick.moreLink')
  })
})
```

### CLAUDE.md docs scaffold (HSB-NF-05)

```markdown
### Adding a Spotlight Card Type

The home page's `HeroSpotlightBlock` (workstream `hero-spotlight`) renders 9
card types. To add a 10th:

1. **Backend resolver** — new file `services/catalog/internal/service/spotlight/cards/<type>.go`.
   Implement `spotlight.Resolver` (`Type() string`, `Resolve(ctx, userID *string) (*Card, error)`).
   Follow the existing pattern: manual `cache.Get` + `errors.Is(..., cache.ErrNotFound)`
   + `cache.Set` (NEVER `GetOrSet` — see Pitfall 5).
   - Cache key: `spotlight:<type>:<scope>` where scope = `<date>` for daily,
     `<user_id>` for per-user live, or omitted for global live.
   - Return `(nil, nil)` for "no data" (eligibility=false) — do NOT cache empty.
   - Apply the adaptive 1-2-3 rule via `spotlight.AdaptiveSlice(items, 3)` for
     multi-item cards.

2. **Wire it in `cmd/catalog-api/main.go`** — append to the `spotlightResolvers`
   slice at the existing block (around line 217).

3. **Discriminated union** — `services/catalog/internal/service/spotlight/types.go`
   gets a new `<Type>Data struct` and the resolver returns
   `&spotlight.Card{Type: "<type>", Data: data}`.

4. **Frontend type** — `frontend/web/src/types/spotlight.ts` extends the
   `SpotlightCard` union with the new variant.

5. **Vue component** — new file
   `frontend/web/src/components/home/spotlight/cards/<Type>Card.vue` with a
   typed `data: <Type>Data` prop. Follow the existing AnimeOfDayCard layout.

6. **Register in HeroSpotlightBlock.vue** — extend the v-if/v-else-if chain in
   the slide template (search for `v-if="active.type === 'anime_of_day'"`).
   Add a matching import.

7. **i18n keys** — add a new `spotlight.<typeNamespace>.*` sub-namespace to
   BOTH `frontend/web/src/locales/en.json` and `.../ru.json`. The Vitest
   parity test will fail if either side is missing keys.

8. **Tests** — backend resolver unit test (`<type>_test.go` with handwritten
   fakes, no testify/mock); frontend component test
   (`<Type>Card.spec.ts` with `vi.mock('@/composables/useSpotlight', ...)`).

9. **e2e** — extend `frontend/web/e2e/spotlight.spec.ts` with a scenario that
   ensures the new card type renders without errors (use a mocked
   `/api/home/spotlight` payload).

Done. The aggregator's fan-out picks up the new resolver automatically; the
frontend dispatch chain renders it when `card.type === '<type>'`.
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Top-of-home `trendingRecs` row | `HeroSpotlightBlock` carousel with 9 card types | Phase 1/2/3 of hero-spotlight workstream (2026-05-21) | One row → one carousel; multiple content surfaces in one slot |
| Per-card narrow Vue render | Same after Phase 3 — 9 type-specific cards in single carousel | n/a | n/a |
| In-process recs computation in catalog | Recs ensemble in player service, fan-out HTTP | Workstream recs (Phase 10-14, 2026 Q1) | Catalog → player HTTP hop; standardizes recs ownership |
| `cache.GetOrSet` for resolvers | Manual `Get` / `errors.Is(...ErrNotFound)` / `Set` | Phase 1 of hero-spotlight | Prevents nil-caching for 24h |
| `errgroup` for fan-out | `sync.WaitGroup` + buffered chan | Phase 1 of hero-spotlight | "Drop on error" semantics — never fail-fast |
| `<component :is>` dispatch | v-if/v-else-if chain | Phase 2 of hero-spotlight | Preserves `vue-tsc` type narrowing on `data` prop |

**Deprecated/outdated:** No deprecations introduced by this phase. `useRecs` composable remains in use for other surfaces (account profile recs panel) — only its consumption from Home.vue is removed.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Backend framework | Go standard `testing` + `httptest.NewServer` (Phase 1 precedent) |
| Frontend framework | Vitest 4.1.6 (jsdom env) + Vue Test Utils 2.4.10 |
| E2E framework | Playwright 1.58.x + @axe-core/playwright |
| Backend config file | `services/catalog/go.mod`, `services/player/go.mod` |
| Frontend config file | `frontend/web/vitest.config.ts`, `frontend/web/playwright.config.ts` |
| Quick run (backend) | `cd services/catalog && go test ./internal/service/spotlight/...` |
| Quick run (frontend) | `cd frontend/web && bunx vitest run src/components/home/spotlight/` |
| Full suite (backend) | `cd services/catalog && go test ./... && cd ../player && go test ./...` |
| Full suite (frontend) | `cd frontend/web && bunx tsc --noEmit && bunx eslint src/ && bunx vitest run && bunx playwright test spotlight` |
| Phase gate | All four full-suite commands return exit 0 |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| HSB-BE-20 | `personal_pick` anon returns up to 3 random from top-10 trending | Resolver unit | `go test ./services/catalog/internal/service/spotlight/cards/ -run PersonalPick` | ❌ Wave 0 |
| HSB-BE-20 | `personal_pick` login HTTP-fan-outs to player with forwarded JWT | Resolver unit + httptest | `go test ./services/catalog/internal/service/spotlight/cards/ -run PersonalPick_Login` | ❌ Wave 0 |
| HSB-BE-21 | `telegram_news` reuses `news:telegram` cache (no new key) | Resolver unit (assert Redis key) | `go test ./services/catalog/internal/service/spotlight/cards/ -run TelegramNews` | ❌ Wave 0 |
| HSB-BE-22 | `now_watching` SQL — DISTINCT ON returns most-recent per user | Resolver unit (testcontainers or fake repo) | `go test ./services/catalog/internal/service/spotlight/cards/ -run NowWatching` | ❌ Wave 0 |
| HSB-BE-23 | `player_client.GetUserList` + `GetUserRecs` produce correct shapes | Unit (httptest.NewServer) | `go test ./services/catalog/internal/service/spotlight/client/ -run PlayerClient` | ❌ Wave 0 |
| HSB-BE-24 | `not_time_yet` filters airing_started + random pick 1 | Resolver unit | `go test ./services/catalog/internal/service/spotlight/cards/ -run NotTimeYet` | ❌ Wave 0 |
| HSB-BE-25 | `continue_watching_new` filters `episodes_aired > last_watched+1` | Resolver unit | `go test ./services/catalog/internal/service/spotlight/cards/ -run ContinueWatchingNew` | ❌ Wave 0 |
| HSB-BE-26 | Player `/internal/users/{id}/list` 200 with items shape | Handler unit | `go test ./services/player/internal/handler/ -run InternalList` | ❌ Wave 0 |
| HSB-BE-26 | Gateway does NOT proxy `/internal/users/*` (defense-in-depth) | Router test | `go test ./services/gateway/internal/transport/ -run InternalRoutesNotProxied` | ❌ Wave 0 |
| HSB-BE-30 | Adaptive rule: N=1 returns 1, N=2 returns 1 random, N=3 returns 3 | Helper unit | `go test ./services/catalog/internal/service/spotlight/ -run AdaptiveSlice` | ❌ Wave 0 |
| HSB-FE-24 | PersonalPickCard renders 1..3 posters + mobile "+ ещё N →" link | Vitest component | `bunx vitest run src/components/home/spotlight/cards/PersonalPickCard.spec.ts` | ❌ Wave 0 |
| HSB-FE-25 | NowWatchingCard renders 1..3 user-session rows with green live dot | Vitest component | `bunx vitest run src/components/home/spotlight/cards/NowWatchingCard.spec.ts` | ❌ Wave 0 |
| HSB-FE-26 | TelegramNewsCard renders 1..3 posts; links open in new tab | Vitest component | `bunx vitest run src/components/home/spotlight/cards/TelegramNewsCard.spec.ts` | ❌ Wave 0 |
| HSB-FE-27 | NotTimeYetCard renders header + sub-line per `list_status` | Vitest component | `bunx vitest run src/components/home/spotlight/cards/NotTimeYetCard.spec.ts` | ❌ Wave 0 |
| HSB-FE-28 | ContinueWatchingNewCard renders "Новая серия ep N!" badge | Vitest component | `bunx vitest run src/components/home/spotlight/cards/ContinueWatchingNewCard.spec.ts` | ❌ Wave 0 |
| HSB-FE-* | HeroSpotlightBlock dispatch chain covers all 9 types | Vitest extend (HeroSpotlightBlock.spec.ts) | `bunx vitest run src/components/home/spotlight/HeroSpotlightBlock.spec.ts` | Extends existing 354-line spec |
| HSB-MIG-01 | Home.vue has no `trendingRecs` references after merge | Grep test (shell) | `grep -c 'trendingRecs' frontend/web/src/views/Home.vue` returns 0 | Wave 0 (shell command, no test file) |
| HSB-NF-02 | `idx_watch_progress_updated_at` index exists in Postgres | Integration (shell) | `docker compose exec -T postgres psql -U postgres -d animeenigma -c "\d+ watch_progress" | grep -c idx_watch_progress_updated_at` returns ≥1 | Wave 0 |
| HSB-NF-04 | now_watching response contains only `username` + `public_id` (no email) | Resolver unit | Part of HSB-BE-22 test | Wave 0 |
| HSB-NF-05 | CLAUDE.md has "Adding a Spotlight Card Type" section | Grep test | `grep -c 'Adding a Spotlight Card Type' CLAUDE.md` returns 1 | Wave 0 |
| End-to-end | 9-card spotlight renders without console errors | Playwright | `bunx playwright test spotlight-full` | ❌ Wave 0 (new file) |
| End-to-end | axe-core a11y on each of the 5 new cards | Playwright + axe | Same Playwright run with `injectAxe()` then `checkA11y()` per slide | ❌ Wave 0 |
| End-to-end | Logged-in spotlight surfaces personal_pick / continue_new / not_time_yet | Playwright with auth as `ui_audit_bot` | `bunx playwright test spotlight-full --grep loggedIn` | ❌ Wave 0 |
| End-to-end | Anon spotlight does NOT include `not_time_yet` or `continue_watching_new` | Playwright | `bunx playwright test spotlight-full --grep anon` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `bunx tsc --noEmit && bunx eslint src/components/home/spotlight src/views/Home.vue` (frontend) + `go build ./...` (backend) — fast (<15s).
- **Per wave merge:** Full Vitest suite on `src/components/home/spotlight/` + `go test ./services/catalog/internal/service/spotlight/... && go test ./services/player/internal/handler/internal_list_test.go`.
- **Phase gate:** Full project test suite + 9-card Playwright e2e + axe-core run → all green before `/gsd-verify-work`.

### Wave 0 Gaps

- [ ] `services/catalog/internal/service/spotlight/cards/personal_pick_test.go` — covers HSB-BE-20
- [ ] `services/catalog/internal/service/spotlight/cards/telegram_news_test.go` — covers HSB-BE-21
- [ ] `services/catalog/internal/service/spotlight/cards/now_watching_test.go` — covers HSB-BE-22, HSB-NF-04
- [ ] `services/catalog/internal/service/spotlight/cards/not_time_yet_test.go` — covers HSB-BE-24
- [ ] `services/catalog/internal/service/spotlight/cards/continue_watching_new_test.go` — covers HSB-BE-25
- [ ] `services/catalog/internal/service/spotlight/client/player_client_test.go` — covers HSB-BE-23
- [ ] `services/catalog/internal/service/spotlight/adaptive_test.go` — covers HSB-BE-30
- [ ] `services/catalog/internal/transport/optional_auth_test.go` — covers optional-auth middleware
- [ ] `services/player/internal/handler/internal_list_test.go` — covers HSB-BE-26
- [ ] Extend `services/gateway/internal/transport/router_test.go` — defense-in-depth for `/internal/*`
- [ ] Extend `services/catalog/internal/service/spotlight/aggregator_test.go` — 9-card path tests
- [ ] `frontend/web/src/components/home/spotlight/cards/{PersonalPickCard,NowWatchingCard,TelegramNewsCard,NotTimeYetCard,ContinueWatchingNewCard}.spec.ts` — covers HSB-FE-24..28
- [ ] Extend `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.spec.ts` — extend dispatch test for 9 types
- [ ] Frontend i18n parity spec extension (if not auto-tested by Phase 2 spec)
- [ ] `frontend/web/e2e/spotlight.spec.ts` — extend with 9-card scenarios + axe-core re-run + login/anon variants

Framework install: not needed — Vitest, Playwright, axe-core/playwright all installed in Phase 2.

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Reuse `libs/authz.JWTManager.ValidateAccessToken` for optional-auth — do NOT roll custom JWT verification |
| V3 Session Management | yes (existing) | Phase 3 doesn't introduce new sessions; uses existing JWT flow from auth service |
| V4 Access Control | yes | `/internal/users/{id}/list` is gateway-non-proxied (defense-in-depth via router test); player handler validates `user_id` is a UUID before passing to GORM (Pitfall 9) |
| V5 Input Validation | yes | `?status=` parameter is a comma-separated allowlist; player handler MUST validate against a fixed set (`{planned, postponed, watching, completed, dropped, on_hold}`) and reject unknown values 400 |
| V6 Cryptography | n/a — no new crypto introduced | Reuse `libs/authz` HMAC-SHA256 for JWT |
| V7 Error Handling & Logging | yes | Never log JWT contents (Pitfall 6 from 01-RESEARCH.md); never log user emails; log only `user=<uid|anon>` |
| V8 Data Protection | yes | `now_watching` SQL exposes only `username` + `public_id` per HSB-NF-04. Other PII (email, last_seen_at) MUST NOT be in the projection. |
| V13 API & Web Service | yes | `/internal/*` endpoints are NOT proxied by gateway (verify via router test); `/api/home/spotlight` IS gateway-proxied with rate limit + CORS via the standard middleware chain |
| V14 Configuration | yes (light) | `SPOTLIGHT_ENABLED` feature flag check at handler entry; `VITE_HERO_SPOTLIGHT_ENABLED` build-time flag at frontend mount |

### Known Threat Patterns for Go + Vue 3 + Chi + GORM stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SQL injection in `now_watching` raw SQL | Tampering | The `LIMIT ?` parameter is the only user-influenced input — and it's hardcoded to `5`. The other parts of the SQL are static. Safe by construction. |
| SQL injection via `user_id` URL param on player internal | Tampering | Validate against UUID regex BEFORE passing to GORM (Pitfall 9). |
| Open-redirect via `t.me/<channel>/<msg_id>` link | Tampering | `data.posts[i].link` is constructed server-side from the Telegram parser; user input never reaches it. Frontend renders with `rel="noopener noreferrer" target="_blank"` to harden against tab-hijack. |
| JWT replay via leaked Authorization header | Spoofing | Already mitigated by JWT short TTL (15 min access token per `libs/authz.DefaultJWTConfig`). Spotlight does not extend the token lifetime. |
| Internal endpoint exposed via gateway misconfig | Information disclosure | Defense-in-depth: gateway router test asserts `/internal/users/*` returns 404 via gateway. Player handler ALSO has no JWT requirement on `/internal/*` — but it's not reachable from outside the docker network. |
| `now_watching` leaks active users' watch sessions | Information disclosure | HSB-NF-04 documents the public-by-default decision; users already make username + public_id publicly visible on profile pages. Opt-out deferred to Phase 4+ if requested. |
| XSS via `telegram_news` post text | Information disclosure (low) | `posts[i].text` is rendered via Vue text interpolation `{{ post.text }}` which escapes HTML by default. Never use `v-html` for this content. |
| CSRF on player's `/internal/users/{id}/list` | Spoofing | Not applicable — endpoint is GET and unreachable from outside docker network. |

## Sources

### Primary (HIGH confidence)
- `services/catalog/internal/service/spotlight/aggregator.go` — Phase 1 aggregator (verified read 2026-05-21)
- `services/catalog/internal/service/spotlight/types.go` — Phase 1 type system
- `services/catalog/internal/service/spotlight/cards/{anime_of_day,random_tail,latest_news,platform_stats}.go` — Phase 1 resolver canonical pattern
- `services/catalog/internal/service/spotlight/cards/fakes_test.go` — handwritten fake pattern
- `services/catalog/internal/service/spotlight/client/web_client.go` — HTTP client pattern (used as template for player_client.go)
- `services/catalog/internal/handler/spotlight.go` — handler pattern with feature-flag short-circuit
- `services/catalog/internal/transport/router.go` — catalog router; sample `/internal/*` route placement (lines 51-65)
- `services/player/internal/transport/router.go` — player router
- `services/player/internal/transport/optional_auth.go` — verbatim source for catalog's new middleware
- `services/player/internal/handler/recs.go` — `/api/users/recs` shape (verified envelope returns `{success, data:{recs:[...]}}`)
- `services/player/internal/handler/list.go` — existing list handler signatures
- `services/player/internal/service/list.go` — `GetUserList`, `GetUserListPaginated`, `GetUserStatuses` signatures
- `services/player/internal/repo/list.go:153` — `GetByUserAndStatuses(ctx, userID, statuses []string)` method
- `services/player/internal/domain/watch.go` — `WatchProgress` struct, `idx_watch_progress_updated_at` MISSING (verified)
- `libs/authz/jwt.go` — `JWTManager`, `ContextWithClaims`, `ClaimsFromContext`, `UserIDFromContext`
- `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` — dispatch chain location for extension (lines 70-89)
- `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.spec.ts` — Vitest mock pattern reference
- `frontend/web/src/types/spotlight.ts` — discriminated union extension point
- `frontend/web/src/views/Home.vue` — trendingRecs row markup (lines 45-138) + setup-script state (lines 470-522)
- `frontend/web/src/locales/en.json:985-1018` — Phase 2 spotlight i18n tree
- `docker/docker-compose.yml` — shared DB confirmation (all services declare `DB_NAME: animeenigma`)
- `services/catalog/cmd/catalog-api/main.go:209-224` — Phase 1 spotlight wiring (extension point)
- `services/catalog/internal/handler/internal_episodes.go` — internal endpoint precedent with UUID validation
- `services/gateway/internal/transport/router.go:224-232` — Phase 1 gateway proxy entry; `/internal/*` not proxied
- `.planning/workstreams/hero-spotlight/phases/01-backend-aggregator/01-RESEARCH.md` — Phase 1 patterns
- `.planning/workstreams/hero-spotlight/phases/02-frontend-carousel/02-UI-SPEC.md` — UI design tokens
- `.planning/workstreams/hero-spotlight/phases/02-frontend-carousel/02-04-SUMMARY.md` — HeroSpotlightBlock keystone deliverable
- `docs/superpowers/specs/2026-05-21-hero-spotlight-block-design.md` — workstream design doc §4 card inventory, §5.4 data sources

### Secondary (MEDIUM confidence)
- `services/catalog/internal/service/catalog.go:696-740` — `GetTrendingAnime` signature (returns `([]*domain.Anime, int64, error)`)
- `services/catalog/internal/parser/telegram/client.go:24-47` — `NewClient`, `NewsItem` struct shape
- `services/catalog/internal/handler/news.go:15` — `newsRedisKey = "news:telegram"` confirms reuse contract
- `services/player/internal/handler/recs.go:154-247` — `GetRecs` branches on JWT presence

### Tertiary (LOW confidence — flag for verification during plan-phase)
- The exact JSON shape returned by `/api/users/recs` envelope was inferred from the test file (`recs_test.go`) — plan-phase should curl the LIVE endpoint with `UI_AUDIT_API_KEY` to confirm:
  ```bash
  curl -s -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/users/recs | jq '.data.recs[0]'
  ```
  Confirm the response wraps in `{success, data:{recs, ...}}` (matches `httputil.OK` convention).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `/api/users/recs` envelope is `{success: true, data: {recs: [...], ...}}` (player wraps via `httputil.OK`) | Pattern 2 + Pitfall 1 | Low. If the shape is `{recs, ...}` flat, the player_client decode path needs to drop the outer `.Data` indirection. One-line fix. **Mitigation:** plan-phase curls the endpoint. |
| A2 | The frontend currently calls `/api/users/recs` (NOT `/api/anime/recommended`) for the trending row | CONTEXT corrections + Summary finding 1 | Very low. Confirmed by reading `frontend/web/src/composables/useRecs.ts:74` — `apiClient.get('/users/recs')`. |
| A3 | `users.username` and `users.public_id` are public (visible on `/user/<public_id>` profile pages) per CLAUDE.md test user section | HSB-NF-04 + Security Domain | Very low. CLAUDE.md explicitly references the public profile URL `https://animeenigma.ru/user/ui-audit-bot` (public_id) and the UI Audit framework references usernames in audit reports. |
| A4 | The `users` table is in the same `animeenigma` Postgres database, addressable from catalog via shared GORM | Pattern 3 + Architecture Diagram | Low. The `auth` service writes the users table and shares `DB_HOST: postgres`, `DB_NAME: animeenigma`. `services/auth/internal/repo/user.go` would use `&domain.User{}` GORM model. **Plan-phase should verify** the table exists and is readable via catalog's DB connection by running:
   ```bash
   docker compose exec -T postgres psql -U postgres -d animeenigma -c "\d users"
   ``` |
| A5 | The `watch_progress` table is in the same `animeenigma` Postgres database, addressable from catalog | Pattern 3 + Architecture Diagram | Low. Same shared-DB topology as A4. |
| A6 | Adding a GORM index tag triggers `AutoMigrate` to CREATE the index on next service restart (without dropping existing indexes) | Pitfall 7 | Low — verified by reading `libs/database/database.go` AutoMigrate semantics; CLAUDE.md "Database migrations" section explicitly states "GORM only creates new tables/columns, it does NOT modify or drop existing columns to protect data". Indexes follow the same idempotent ADD-only contract. |
| A7 | `math/rand/v2` package available in Go 1.24 | Standard Stack + Adaptive helper | Very low. Go 1.22 introduced `math/rand/v2`; project's go.mod is on 1.24. **Verified:** `go.mod`'s `go 1.24` directive (or whatever the current minor is). |
| A8 | The Phase 2 `HeroSpotlightBlock.spec.ts` random-init test (12 it() blocks) does NOT need to be extended for Phase 3 — only NEW `it()` blocks for 9-card variants need adding | Wave 0 gaps | Low. The existing 12 tests are type-agnostic; they test the state machine, not specific card types. Adding 5 new card-type render-coverage cases is additive. |
| A9 | Telegram channel name comes from `cfg.Telegram.NewsChannel` (the existing config field) | telegram_news resolver | Very low. Verified at `services/catalog/cmd/catalog-api/main.go:137` — `telegram.NewClient(cfg.Telegram.NewsChannel)`. Phase 3 resolver receives same `*telegram.Client` via constructor. |
| A10 | The OptionalAuthMiddleware can co-exist with chi's `Route(...)` Group pattern | Pattern 1 | Very low. Player's router does exactly this (player router lines 133-148). |

**If this table proves stale during plan-phase research:** A4 + A5 (shared-DB topology) are the highest-risk assumptions — both should be confirmed with one shell command before sealing the plan. All other assumptions are very low risk.

## Open Questions

1. **Where should `personal_pick`'s JWT-pass-through helper live — `cards` package or `spotlight` root package?**
   - What we know: The helper (`ContextWithJWT` / `jwtFromContext`) is only used by `personal_pick.go`.
   - What's unclear: If a future card also needs the JWT (e.g., a v1.1 admin-only card), would placing the helper in `cards` create circular imports?
   - Recommendation: Place in `cards/personal_pick.go` as unexported package-level helpers initially. If v1.1 needs broader reuse, refactor up to `spotlight` package at that time.

2. **Should `not_time_yet` and `continue_watching_new` share a player_client list-fetch call, or each make their own?**
   - What we know: They use different `?status=` values (`planned,postponed` vs `watching`).
   - What's unclear: An aggregated single call `?status=planned,postponed,watching` is the simplest, but it returns more data than each resolver needs.
   - Recommendation: Two separate calls. Per-card 800ms budget is independent. Combining would couple the two resolvers' failure modes (one slow list query takes both cards down).

3. **Where should `ListService.GetUserListByStatuses` live — service or directly in handler?**
   - What we know: `repo.GetByUserAndStatuses` already exists (line 153). The service layer currently exposes `GetUserList(ctx, userID, status string)` with a single status string.
   - What's unclear: Is it worth a new service method for a single internal handler call?
   - Recommendation: Add the service method. Keep handler→service→repo discipline consistent across the codebase. Tiny method, clean test.

4. **Should the `LISTITEM` JSON shape include the full anime payload or a minimal projection?**
   - What we know: `not_time_yet` needs `anime.status` + `episodes_aired` + `current_episode_number`. `continue_watching_new` needs `episodes_aired` + `last_watched_episode`. Frontend cards need `name/name_ru/poster_url/id`.
   - What's unclear: Whether `episode_duration`, `genres`, `description` should be included (frontend doesn't render them in the spotlight card).
   - Recommendation: Minimal projection — drop everything not consumed by the two card types. Saves bandwidth on what's already a per-resolver call. Plan should formalize this with a struct in `player_client.go` (the `UserListItem` struct in Pattern 2 above).

5. **Should the 5 new resolvers be wired in main.go in a single block or split into "anon-eligible" and "login-only" groups?**
   - What we know: The Aggregator iterates resolvers uniformly — order doesn't matter for correctness.
   - What's unclear: Whether visual grouping in main.go helps future maintenance.
   - Recommendation: Single block, but add an inline comment grouping them (4 lines: `// anon-eligible:` + 3 resolvers, then `// login-only (returns (nil,nil) when userID==nil):` + 2 resolvers). Phase 3 plan can use whichever feels clearest.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every package is already in go.mod / package.json; no new external dependencies.
- Architecture: HIGH — every pattern is an extension of Phase 1/2; nothing architecturally new.
- Backend resolvers: HIGH — pattern proven across 4 Phase 1 resolvers + Phase 1 PATTERNS doc.
- Frontend cards: HIGH — pattern proven across 4 Phase 2 cards + UI-SPEC.md.
- Player internal endpoint: HIGH — precedent in catalog's `/internal/cache/invalidate/raw` + `/internal/anime/{shikimoriId}/episodes`.
- Optional-auth middleware: HIGH — verbatim port of player's 35-line battle-tested file.
- Adaptive 1-2-3 rule: HIGH — pure function, exhaustively testable.
- GORM index addition: HIGH — AutoMigrate semantics confirmed in CLAUDE.md.
- Pitfalls: HIGH — 13 enumerated with concrete code-anchored evidence + warning signs.

**Research date:** 2026-05-21
**Valid until:** 2026-06-04 (14 days — stable patterns, no fast-moving deps)
