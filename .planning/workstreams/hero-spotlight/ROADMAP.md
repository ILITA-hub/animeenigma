# Roadmap: AnimeEnigma `hero-spotlight` workstream

**Workstream:** hero-spotlight (parallel to `notifications`, `raw-jp`, `social`, `ui-ux-audit`)
**Active milestone:** v1.0 HeroSpotlightBlock
**Phase numbering:** Workstream-local — restarts at 1 inside each milestone (`v1.0-phases/01-*`).
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-21-hero-spotlight-block-design.md`
**Requirements:** `REQUIREMENTS.md`

## Milestones

- 🟢 **v1.0 HeroSpotlightBlock** — Active (planning), 3 phases scoped — see below
- ⏳ **v1.1 Personalization & Polish** — Conditional on v1.0 + 1-2 weeks usage data (slide order personalization, per-user opt-outs, optional editorial-pick admin form)

## Goal (v1.0)

The home page opens with a single eye-catching, eligibility-aware spotlight
block above all other content. Anonymous visitors see ≥5 of the eligible
cards (anime of the day, personal trending, latest changelog, platform stats,
random discovery, Telegram news, "now watching"). Logged-in users see up to
all 9 (above + "is it time yet?" + "continue watching with new ep"). The
block auto-cycles every 7s, pauses on hover, and starts on a random eligible
slide each page load. Empty cards never appear. The legacy `trendingRecs`
row at the top of `Home.vue` is removed; its top recommendation surfaces as
the spotlight's `personal_pick` card.

## Vertical-slice phasing rationale

Three phases, each independently demoable and atomically committable:

- **Phase 1 (Backend MVP)** delivers a working `/api/home/spotlight` endpoint
  with the 4 simplest cards: `anime_of_day`, `random_tail`, `latest_news`,
  `platform_stats`. No frontend, no auth-gated cards, no player fan-out. The
  endpoint is curl-testable end-to-end. This phase locks down the aggregator
  pattern (per-card resolvers, 800ms deadlines, Redis day-cache, eligibility
  filter) so Phases 2 and 3 are purely additive — no architectural risk lives
  in the dynamic cards.
- **Phase 2 (Frontend Carousel)** is pure frontend — Vue 3 block + carousel
  state machine + 4 card components + skeleton + a11y + reduced-motion +
  i18n. Mounted in `Home.vue` behind `VITE_HERO_SPOTLIGHT_ENABLED` so it can
  co-exist with the legacy `trendingRecs` row during the transition. End-state:
  user-visible rotating block with 4 cards in production.
- **Phase 3 (Dynamic Cards + Migration)** adds the 5 remaining cards
  (`personal_pick`, `telegram_news`, `now_watching`, `not_time_yet`,
  `continue_watching_new`), including the player-service fan-out and the
  adaptive 1-2-3 layout rule. Removes the legacy `trendingRecs` markup.
  End-state: 9-card spotlight fully replaces the old row.

Splitting the dynamic cards into Phase 3 keeps the most coupling-heavy work
(cross-service HTTP, live SQL, privacy considerations on `now_watching`) out
of the foundational backend and frontend phases. It also creates a natural
verification gate at the end of Phase 2 (the block is live in production
with 4 cards) before adding personalization and live data.

## Phases

### Phase 1: Backend Aggregator + Static Cards

**Goal:** `GET /api/home/spotlight` returns 4 eligible cards (`anime_of_day`,
`random_tail`, `latest_news`, `platform_stats`) in well-shaped JSON. Each
card resolver runs under an 800ms deadline; per-card Redis day-cache works
with `spotlight:*` keys. The endpoint behind the `SPOTLIGHT_ENABLED=true`
feature flag, gateway-routed at `/api/home/spotlight`. Demo: `curl -s
http://localhost:8000/api/home/spotlight | jq` returns 4 cards;
`docker compose exec redis redis-cli KEYS 'spotlight:*'` shows the day-keyed
entries.

**Depends on:** Nothing — additive backend module. Does not touch existing
tables or external services beyond reading them.

**Requirements:** HSB-BE-01..07, HSB-BE-10..13, HSB-NF-01, HSB-NF-03 (partial)

**SPEC:** `phases/01-backend-aggregator/01-SPEC.md` (to be written by gsd-plan-phase)

**Plans:** 6 plans
- [x] 01-01-PLAN.md — Scaffold spotlight package (types, seed helpers, Resolver interface, Aggregator skeleton)
- [x] 01-02-PLAN.md — 4 card resolvers (anime_of_day, random_tail, latest_news, platform_stats) + web client for changelog.json
- [x] 01-03-PLAN.md — Concurrent Aggregator (per-card 800ms, overall 2s, eligibility filter, snapshot fallback)
- [x] 01-04-PLAN.md — Catalog handler + SPOTLIGHT_ENABLED config flag + chi route + main.go DI wiring
- [x] 01-05-PLAN.md — Gateway /api/home/spotlight → catalog proxy
- [x] 01-06-PLAN.md — docker/.env.example + smoke script + human-verify checkpoint

**Touches:**
- `services/catalog/internal/handler/spotlight.go` (new)
- `services/catalog/internal/service/spotlight/{aggregator,types}.go` (new)
- `services/catalog/internal/service/spotlight/cards/{anime_of_day,random_tail,latest_news,platform_stats}.go` (new)
- `services/catalog/internal/service/spotlight/client/web_client.go` (new — fetches changelog from web container)
- `services/catalog/internal/transport/router.go` (extend — add `r.Get("/home/spotlight", h.Get)`)
- `services/catalog/cmd/catalog-api/main.go` (extend — wire spotlight handler)
- `services/catalog/internal/config/config.go` (extend — `SpotlightEnabled bool`)
- `services/gateway/internal/transport/router.go` (extend — `/api/home/spotlight` proxy)
- `docker/.env.example` (new `SPOTLIGHT_ENABLED=true`)

**Success criteria:**
1. `make redeploy-catalog && make redeploy-gateway` builds clean. `make health` green.
2. `curl -s http://localhost:8000/api/home/spotlight | jq '.cards | length'` returns `4`.
3. `jq '.cards[].type' < /tmp/resp.json` includes `anime_of_day`, `random_tail`, `latest_news`, `platform_stats` (order may vary; only eligible cards present).
4. `docker compose exec redis redis-cli KEYS 'spotlight:*'` shows 4 day-keyed entries.
5. Second curl within 1 second returns identical `generated_at` (cache hit) AND p95 latency from `/metrics` is < 100ms.
6. With `SPOTLIGHT_ENABLED=false` + redeploy, curl returns `404`.
7. If `web:80/changelog.json` is intentionally broken (e.g. `docker compose stop web`), `latest_news` is dropped (`.cards | length == 3`); the other 3 cards still return; a `spotlight.card_failed{type="latest_news"}` log line is present.

### Phase 2: Frontend HeroSpotlightBlock + Carousel

**Goal:** `HeroSpotlightBlock.vue` mounted at the top of `Home.vue` (above
Ongoing/Top/Announced), renders the 4 Phase-1 cards in a 7-second
auto-cycling carousel with manual nav. Initial slide random per page load.
Pauses on hover. Respects `prefers-reduced-motion`. Mobile-responsive
type-specific layouts. Behind `VITE_HERO_SPOTLIGHT_ENABLED=true`; legacy
`trendingRecs` row co-exists. End-state: user-visible rotating block in
production with 4 cards.

**Depends on:** Phase 1 (consumes `/api/home/spotlight`).

**Requirements:** HSB-FE-01..09, HSB-FE-20..23, HSB-FE-40

**SPEC:** `phases/02-frontend-carousel/02-SPEC.md` (to be written by gsd-plan-phase)

**Touches:**
- `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` (new)
- `frontend/web/src/components/home/spotlight/CarouselControls.vue` (new)
- `frontend/web/src/components/home/spotlight/cards/{AnimeOfDayCard,LatestNewsCard,PlatformStatsCard,RandomTailCard}.vue` (new)
- `frontend/web/src/composables/useSpotlight.ts` (new — fetch + state)
- `frontend/web/src/views/Home.vue` (extend — mount `<HeroSpotlightBlock />`)
- `frontend/web/src/locales/{en,ru}.json` (extend — `spotlight.*` namespace)
- `frontend/web/.env.development` + `frontend/web/.env.production.example` (new `VITE_HERO_SPOTLIGHT_ENABLED=true`)
- `frontend/web/tests/e2e/spotlight.spec.ts` (new — Playwright)

**Success criteria:**
1. `cd frontend/web && bunx tsc --noEmit && bunx eslint src/` pass.
2. `bun run build` succeeds; deployed via `make redeploy-web`.
3. Visiting `https://animeenigma.ru/` (logged-out) shows the block at top, above Ongoing/Top/Announced.
4. Block cycles every 7s; hovering pauses; arrow keys (Left/Right) seek.
5. F5 reload — initial slide is different ~75% of the time across 10 reloads (cards.length=4 → 75% probability of NOT picking the same one).
6. DevTools → emulate `prefers-reduced-motion: reduce` — auto-cycle stops; manual nav still works.
7. Mobile viewport 375×667: poster cards stack vertically; carousel still works.
8. `bunx playwright test spotlight` passes (load, autoplay, pause-on-hover, keyboard nav, reduced-motion).
9. axe-core (run via `read_console_messages` in browser audit) reports zero violations on the block.
10. With `VITE_HERO_SPOTLIGHT_ENABLED=false` + rebuild, block is absent; legacy `trendingRecs` row visible.

### Phase 3: Dynamic Cards + Migration

**Goal:** Add 5 remaining cards (`personal_pick`, `telegram_news`,
`now_watching`, `not_time_yet`, `continue_watching_new`). Player service
exposes 2 internal endpoints for cards 8/9. Adaptive 1-2-3 layout rule live.
Remove `trendingRecs` markup from `Home.vue`. End-state: 9-card spotlight
fully replaces the legacy row.

**Depends on:** Phase 1 + Phase 2 in production.

**Requirements:** HSB-BE-20..26, HSB-BE-30, HSB-FE-24..28, HSB-MIG-01..02, HSB-NF-02, HSB-NF-04, HSB-NF-05

**SPEC:** `phases/03-dynamic-cards-migration/03-SPEC.md` (to be written by gsd-plan-phase)

**Touches:**
- `services/catalog/internal/service/spotlight/cards/{personal_pick,telegram_news,now_watching,not_time_yet,continue_watching_new}.go` (new)
- `services/catalog/internal/service/spotlight/client/player_client.go` (new — HTTP to player:8083)
- `services/player/internal/handler/internal_list.go` (extend or new — `/internal/users/{id}/list?status=...`)
- `services/player/internal/transport/router.go` (extend — internal route registration)
- `services/player/internal/domain/watch.go` (extend — `idx_watch_progress_updated_at` GORM tag if missing)
- `frontend/web/src/components/home/spotlight/cards/{PersonalPickCard,NowWatchingCard,TelegramNewsCard,NotTimeYetCard,ContinueWatchingNewCard}.vue` (new)
- `frontend/web/src/views/Home.vue` (modify — remove `trendingRecs` block + related state)
- `frontend/web/src/locales/{en,ru}.json` (extend — new card i18n keys)
- `CLAUDE.md` (extend — new "Common Tasks" section on adding a card type)

**Success criteria:**
1. `curl -H "Authorization: Bearer $UI_AUDIT_API_KEY" http://localhost:8000/api/home/spotlight | jq '.cards | length'` returns up to `9` (depending on data availability).
2. Anonymous curl: `.cards[].type` does NOT include `not_time_yet` or `continue_watching_new`.
3. `now_watching` card response: when ≥3 distinct users have `watch_progress.updated_at > NOW() - INTERVAL '5 minutes'` returns 3 sessions; when exactly 2, returns 1 random; when 1, returns 1; when 0, card absent from response (eligibility filter).
4. Telegram card uses existing `news:telegram` Redis key (verify with `redis-cli GET news:telegram` before AND after curl — same value).
5. `/api/home/spotlight` p95 latency from `/metrics` < 1500ms cold, < 400ms cached.
6. `Home.vue` after merge: `grep -c "trendingRecs" frontend/web/src/views/Home.vue` returns `0`. The `/api/anime/recommended` endpoint is still queryable directly.
7. `bunx playwright test spotlight-full` passes (9-card scenarios with mocked API).
8. `make redeploy-catalog && make redeploy-player && make redeploy-web` clean. `make health` green.
9. Login as `ui_audit_bot`: spotlight shows ≥1 of `personal_pick`, `continue_watching_new`, `not_time_yet` (the seed data should make at least one eligible).
10. `CLAUDE.md` has a new "Adding a Spotlight Card Type" section under Common Tasks.

## Workstream-local conventions

- Every GSD command for this workstream MUST pass `--ws hero-spotlight`. Per
  `feedback_workstream_parallelism`, the active marker is local-only and not
  enforced by tooling.
- Metrics per project convention (`.planning/CONVENTIONS.md`): every plan
  scored on UXΔ / CDI / MVQ; no days/hours/sprints.
- Each phase's PLAN.md scored independently; the workstream-level metric is
  in the design doc Section 10.
