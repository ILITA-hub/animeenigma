# Phase 3: Dynamic Cards + Migration - Context

**Gathered:** 2026-05-21
**Status:** Ready for planning
**Mode:** Auto-generated from approved design doc + REQUIREMENTS.md (autonomous mode)

<domain>
## Phase Boundary

Bring the spotlight from 4 cards to 9 by adding the 5 dynamic resolvers
+ 5 Vue card components, exposing 2 internal player-service endpoints for
authenticated user lists, and removing the legacy `trendingRecs` row from
`Home.vue` (HSB-MIG-01).

End-state: 9-card spotlight (4 anon-eligible + 5 dynamic) fully replaces
the legacy row. The `/api/anime/recommended` backend endpoint stays — it
now powers `personal_pick` inside the aggregator.

In scope:
- 5 backend resolvers: `personal_pick`, `telegram_news`, `now_watching`,
  `not_time_yet`, `continue_watching_new`.
- `services/catalog/internal/service/spotlight/client/player_client.go` —
  thin HTTP client to `http://player:8083`.
- Player service exposes 2 internal endpoints (or extends existing):
  - `GET /internal/users/{user_id}/list?status=planned,postponed`
  - `GET /internal/users/{user_id}/list?status=watching`
  - Both internal-only (NOT gateway-proxied), no JWT required (docker
    network trust boundary).
- Optional-auth middleware on the spotlight handler so login-gated cards
  are eligible when a JWT is supplied.
- Adaptive 1-2-3 layout rule for multi-item resolvers (`personal_pick`,
  `latest_news`, `now_watching`, `telegram_news`): N=1 → 1 item; N=2 →
  RANDOM 1 item; N≥3 → top 3 (HSB-BE-30).
- 5 frontend card components:
  - `PersonalPickCard.vue` (1..3 posters; mobile "+ ещё 2 →" link)
  - `NowWatchingCard.vue` (1..3 user-session rows with live dot)
  - `TelegramNewsCard.vue` (1..3 telegram post excerpts)
  - `NotTimeYetCard.vue` (single poster + "Не пришло ли время?" header)
  - `ContinueWatchingNewCard.vue` (single poster + "Продолжить просмотр" +
    "Новая серия ep N!" badge)
- `Home.vue`: REMOVE the entire trendingRecs row (lines ~39-132) plus
  its setup-script state (`trendingRecs`, `trendingProgress`,
  `trendingLoading`, `rowLabelKey`, `reasonI18nKey`, `onRecClick`).
- GORM `idx_watch_progress_updated_at` tag added if missing (HSB-NF-02).
- CLAUDE.md gets a new "Adding a Spotlight Card Type" section under
  Common Tasks (HSB-NF-05).
- The feature flag pair (`SPOTLIGHT_ENABLED` + `VITE_HERO_SPOTLIGHT_ENABLED`)
  REMAINS as kill switches for one release (HSB-MIG-02 — removal deferred
  to a follow-up cleanup commit if everything stays stable, OR Phase 4
  if explicitly scheduled).

Out of scope for this phase (v1.0):
- Slide-order personalization (v1.1).
- Editorial admin-curated card type.
- A/B testing of card sets.
- Persisting "seen" cards per user.
- WebSocket-driven now_watching updates (Phase 1 used 10s Redis polling).

</domain>

<decisions>
## Implementation Decisions

### Optional-auth middleware on the spotlight handler
- New middleware in `services/catalog/internal/transport/router.go` (or
  `internal/middleware/optional_auth.go` if a dedicated package fits):
  validates `Authorization: Bearer <jwt>` if present; on success populates
  `authz.ContextWithClaims`; on failure (invalid/expired) → silently strip
  and continue as anon. NEVER 401s. Wrap the `/home/spotlight` route only.
- Handler reads `authz.UserIDFromContext(ctx)` — returns `(string, bool)`.
  Pass `*string` userID to aggregator (`nil` for anon).
- Tolerance preserved: bare GET (no header) is still treated as anon.

### `personal_pick` resolver
- Anon path: `GET /api/anime/trending?limit=10` against the LOCAL catalog
  service (in-process call to the same package, NOT an HTTP fan-out — use
  the existing service/handler glue). Random 3 of top 10. Redis key
  `spotlight:trending:<YYYY-MM-DD>` TTL 24h.
- Login path: existing recommendations API in-process (look at
  `services/player/internal/service/recs/` and `/api/anime/recommended` —
  may require fan-out to player since catalog doesn't own recs). If fan-out
  is required, use the new `player_client.go`. Random 3 of top 10. Redis
  key `spotlight:personal:<user_id>:<YYYY-MM-DD>` TTL 24h.
- Adaptive 1-2-3 rule applied at resolver layer.

### `telegram_news` resolver
- Reuses existing `parser/telegram.Client` and existing `news:telegram`
  Redis key. DO NOT add a new prefixed key (HSB-NF-03 keeps the
  `spotlight:` prefix only for NEW keys). Return first 3 posts.
- Adaptive 1-2-3 rule applied.

### `now_watching` resolver
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
- Wait — `watch_progress` is owned by the player service (per CLAUDE.md
  service ports + service architecture). Catalog and player likely share
  the same Postgres database; if so, catalog can SELECT from
  `watch_progress` + `users` + `animes` directly via its shared GORM
  connection. **Verify during plan-phase research** — if catalog cannot
  reach the `watch_progress` table without going through the player
  service, defer to a player_client.go fan-out HTTP call instead.
  Default assumption: shared DB → direct SELECT.
- Drop to 1..3 sessions via adaptive rule. Redis key `spotlight:now_watching`
  (NOT day-keyed — live data). TTL 10s.

### `not_time_yet` resolver (login only)
- New internal player endpoint:
  `GET /internal/users/{user_id}/list?status=planned,postponed`.
- Filter to anime where `airing_started=true` (i.e. anime status is
  `released` or `ongoing` or `current_episode_number > 0`).
- Random pick 1. Redis key `spotlight:not_time_yet:<user_id>` TTL 30s.
- Ineligible if userID is nil (anon) OR list returns 0 matches.

### `continue_watching_new` resolver (login only)
- New internal player endpoint:
  `GET /internal/users/{user_id}/list?status=watching`.
- For each, compare `anime.episodes_aired` vs
  `last_watch_progress.episode_number`. Filter where
  `episodes_aired > last_watched + 1` (NEW episode aired since last view).
- Pick the most-recently-aired item. Redis key
  `spotlight:continue_new:<user_id>` TTL 30s.
- Ineligible if userID is nil OR list returns 0 matches.

### Player internal endpoints
- One endpoint, status-parameter-filtered:
  `GET /internal/users/{user_id}/list?status=planned,postponed,watching`.
- Returns JSON shape that includes per-item: anime id + episodes_aired +
  user's last `watch_progress.episode_number` for that anime (needed by
  `continue_watching_new`). One round-trip per resolver.
- Mounted OUTSIDE `/api` so it's NOT gateway-proxied (existing precedent:
  `services/player/internal/transport/router.go` already mounts other
  `/internal/...` routes with no auth — confirm during plan-phase research).
- No JWT required (docker network trust boundary).
- Add an explicit `denied via gateway` test in the gateway router test
  file as defense-in-depth.

### Adaptive 1-2-3 layout (HSB-BE-30)
- Applied at the resolver layer for `personal_pick`, `latest_news`,
  `now_watching`, `telegram_news`:
  - `len(items) == 0` → resolver returns `(nil, nil)` (ineligible).
  - `len(items) == 1` → return all 1.
  - `len(items) == 2` → return ONE randomly-picked item.
  - `len(items) >= 3` → return top 3.
- `latest_news` already has the 3-item cap from Phase 1 but the N=2
  random-pick rule wasn't applied — Phase 3 retrofits it. Verify the
  change doesn't break Phase 1's 4-card endpoint shape (it shouldn't —
  changelog rarely has exactly 2 entries).

### GORM index for `watch_progress.updated_at` (HSB-NF-02)
- Inspect `services/player/internal/domain/watch.go`. If the field
  doesn't have `gorm:"index:idx_watch_progress_updated_at"`, add it.
- Restart of player triggers AutoMigrate which adds the index. Confirm
  via `\d+ watch_progress` after deploy.

### Frontend card components
- 5 new `.vue` components under
  `frontend/web/src/components/home/spotlight/cards/`:
  - `PersonalPickCard.vue` — 1..3 posters with reason chip. Desktop: 3 in
    row. Mobile single + "+ ещё 2 →" link to `/browse?sort=trending`
    (anon) or `/recs` (login). Uses the existing routing logic to switch.
  - `NowWatchingCard.vue` — 1..3 rows: `@username → anime title · ep N`.
    Green dot for "live". Avatar/poster optional.
  - `TelegramNewsCard.vue` — 1..3 telegram post excerpts. Each links to
    `t.me/<channel>/<msg_id>` (use `data.posts[i].link`).
  - `NotTimeYetCard.vue` — single poster + meta with header `t('spotlight.notTimeYet.title')`
    + sub-line referencing the list ("Из «Запланировано»").
  - `ContinueWatchingNewCard.vue` — single poster + meta with header
    `t('spotlight.continueWatchingNew.title')` + "Новая серия ep N!" badge.
- HeroSpotlightBlock.vue's `cardComponent` lookup map (Phase 2 has 4
  entries) is extended to all 9 types.
- i18n: extend `spotlight.*` namespace in en.json + ru.json with the 5
  new sub-namespaces. Parity test from Phase 2 must continue to pass.
- All 5 components honor the same UI-SPEC contracts: 2 font weights,
  `p-4` tablet padding, Tailwind utility-only.

### `trendingRecs` removal in Home.vue (HSB-MIG-01)
- Delete the entire `<div v-if="trendingRecs.length > 0" ...>` block
  (lines ~39-132 in current file).
- Delete the related skeleton and setup-script state:
  `trendingRecs`, `trendingProgress`, `trendingLoading`, `rowLabelKey`,
  `reasonI18nKey`, `onRecClick`.
- Verify no other consumer in `Home.vue` references these names; if any,
  leave the related code alone and trim only the markup. (Likely all
  are scoped to the trending row.)
- The `/api/anime/recommended` backend endpoint STAYS — it now powers
  `personal_pick` inside the aggregator.

### Feature flag retention (HSB-MIG-02)
- Phase 3 does NOT remove the env flags. The dual-flag setup remains as
  a kill switch for one release post-Phase-3.
- A follow-up cleanup commit (Phase 4 if scheduled, or a manual commit
  after observing one stable week) removes both flags + their guards.
- Phase 3's plan does NOT include the flag removal.

### Privacy for `now_watching` (HSB-NF-04)
- Phase 3 default: include all users in `now_watching`. Only public
  fields (`username`, `public_id`) are exposed — both already publicly
  visible on user profile pages.
- A user-level opt-out (`users.show_in_now_watching boolean default true`)
  is DEFERRED to Phase 4+ if/when user feedback requests it. Phase 3
  does NOT add this column.

### CLAUDE.md documentation (HSB-NF-05)
- Add a new "Adding a Spotlight Card Type" section under "Common Tasks"
  (one screen) explaining:
  - Where to add the resolver
    (`services/catalog/internal/service/spotlight/cards/`)
  - Where to add the Vue component
    (`frontend/web/src/components/home/spotlight/cards/`)
  - Where to register the type in HeroSpotlightBlock.vue's cardComponent map
  - Where to add i18n keys (`spotlight.{newType}.*` in en.json + ru.json)
  - Where to extend the TypeScript discriminated union (`types/spotlight.ts`)

### Testing strategy
- Backend resolver unit tests mirror Phase 1 pattern (handwritten struct
  fakes, no testify/mock).
- Player internal endpoint test: handler-level unit test with a fake repo.
- Gateway-not-routing test: assert `/internal/users/...` is NOT proxied.
- Catalog client unit test: HTTP mock against fake player.
- Aggregator concurrency tests STAY at 4 resolvers in Phase 1's test file;
  add new tests for the 9-card path.
- Frontend Vitest specs per new card; e2e spec extended.

### Claude's Discretion
- Whether to merge multiple status filters into one player endpoint or
  to split into two endpoints — single endpoint with `?status=` filter
  is simpler and matches REQUIREMENTS.md's HSB-BE-24/25 phrasing.
- Whether `now_watching` direct DB SELECT or HTTP fan-out — decided based
  on plan-phase research finding. Direct DB is preferred (lower latency).
- Whether to add a separate `internal/middleware/optional_auth.go` file
  or inline the middleware in `router.go`. Inline is fine if it's <15
  lines.
- Whether to extend Phase 1's `aggregator_test.go` or add a new
  `aggregator_dynamic_test.go` for the 9-card path. Either acceptable.
- Whether to extract the "+ ещё 2 →" link logic into a reusable
  composable (only if Phase 4 needs the same pattern; otherwise inline
  in PersonalPickCard).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Phase 1 spotlight infrastructure** — aggregator, Card types, Resolver
  interface, manual cache discipline. All 5 new resolvers slot in via the
  same constructor pattern. The aggregator constructor takes a variadic
  list of `Resolver` — wiring is purely additive.
- **`services/catalog/internal/parser/telegram.Client`** — already used by
  the news handler; reuse the same client + cache for `telegram_news`.
- **`services/player/internal/service/list.go`** + `repo/list.go` — owns
  `anime_list` table queries. The new internal endpoint reuses these
  service methods (don't reinvent).
- **`services/player/internal/service/recs/`** — owns recommendations
  logic. If `personal_pick` login path needs it, fan out to player.
- **`libs/authz`** — JWT validation utilities. The new optional-auth
  middleware uses `jwtManager.ValidateAccessToken`.

### Established Patterns
- **Internal player endpoints** — confirm existing precedent in
  `services/player/internal/transport/router.go` for `/internal/*` routes
  with no auth middleware. Mirror precisely.
- **`watch_progress` table** — owned by player service domain but may be
  shared-DB-readable from catalog. Verify in plan-phase research.
- **Card component shape** — Phase 2's 4 cards establish the template:
  typed `data` prop, `t('spotlight.*')` strings, `font-medium/semibold`
  only, `p-4` tablet, `aria-*` attributes as documented in UI-SPEC.md.

### Integration Points
- `services/catalog/cmd/catalog-api/main.go` — extend the aggregator
  constructor with the 5 new resolvers + the player_client dependency.
- `services/catalog/internal/transport/router.go` — wrap `/home/spotlight`
  with the new optional-auth middleware.
- `services/player/internal/transport/router.go` — register the new
  `/internal/users/{id}/list` route.
- `services/gateway/internal/transport/router.go` — NO new route here;
  but write a defense-in-depth test that asserts `/internal/*` is NOT
  routed.
- `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` —
  extend the `cardComponent` lookup map to all 9 types.
- `frontend/web/src/views/Home.vue` — REMOVE trendingRecs block (HSB-MIG-01).
- `frontend/web/src/locales/{en,ru}.json` — extend `spotlight.*` namespace.
- `CLAUDE.md` — add "Common Tasks" section.

</code_context>

<specifics>
## Specific Ideas

- **Adaptive 1-2-3 rule** is non-negotiable per HSB-BE-30; bake into all
  4 multi-item resolvers AND verify the same Card payload shape so
  HeroSpotlightBlock doesn't break.
- **Adaptive rule retrofit to `latest_news`** — Phase 1 had 3-item cap
  but not the N=2 random-pick. Phase 3 retrofits.
- **`watch_progress` direct-SELECT preferred** — single Postgres DB.
  Verified during plan-phase research.
- **The two flags STAY** — kill switches for one release. Removal is a
  follow-up commit, NOT part of this phase.
- **The `/api/anime/recommended` endpoint STAYS** — it powers
  `personal_pick` login path.

</specifics>

<deferred>
## Deferred Ideas

- `users.show_in_now_watching` opt-out column → Phase 4+ if user feedback
  requests.
- Feature flag removal (`SPOTLIGHT_ENABLED`, `VITE_HERO_SPOTLIGHT_ENABLED`)
  → follow-up cleanup commit, not part of Phase 3.
- WebSocket-driven `now_watching` updates → v1.1.
- Slide-order personalization → v1.1.
- Editorial admin-curated card type → v1.1.

</deferred>
</content>
</invoke>