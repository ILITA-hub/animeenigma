# HeroSpotlightBlock — Design Spec

**Date:** 2026-05-21
**Status:** Design approved, ready for plan
**Owner:** Frontend (Vue 3) + `services/catalog` (Go)

## 1. Goal

Replace the existing top-of-home `trendingRecs` row with a rotating hero
"spotlight" block that surfaces nine different content types (anime of the day,
personal picks, latest news, platform stats, live "now watching", random
discovery, Telegram channel news, "is it time?" backlog reminders, "continue
watching" with new episodes) in a single auto-cycling carousel.

The block:
- Sits at the top of the home page, above the Ongoing/Top/Announced
  three-column grid.
- Auto-rotates every ~7 seconds, pauses on hover/focus, supports manual
  arrow/dot navigation.
- Picks a random eligible card as the starting slide on each page load.
- Filters out cards with no data (eligibility check) so users never see an
  empty slide.

## 2. Position on Home.vue

```
SystemStatusBanner
SearchAutocomplete + Schedule link
HeroSpotlightBlock                ← NEW (replaces trendingRecs row)
3-column grid (Ongoing | Top | Announced)
CollectionsRow
ContinueWatchingRow
ActivityFeed + LastUpdates (2-col grid)
```

The existing `trendingRecs` block in `Home.vue` (lines ~39-132) is removed
entirely; its #1 recommendation surfaces as the `personal_pick` card inside
the spotlight.

## 3. Carousel UX

- **Auto-cycle:** 7-second slide interval. Pauses on `mouseenter` / `focusin`
  on the block, resumes on `mouseleave` / `focusout`.
- **Manual nav:** Left/right chevrons + dot indicators (one per eligible
  card). Keyboard: `ArrowLeft`/`ArrowRight` when focused, `Tab` to dots.
- **Initial slide:** randomly chosen from the eligible-cards array on
  component mount (re-randomized on every page reload).
- **Reduced motion:** if `prefers-reduced-motion: reduce`, disable auto-cycle
  (only manual nav active); no slide-transition animation.
- **A11y:** `role="region"` `aria-roledescription="carousel"` on the wrapper.
  Each slide has `aria-roledescription="slide"` and `aria-label` describing
  position ("3 of 7"). `aria-live="polite"` on the slide container so screen
  readers announce the active card on change.

## 4. Card inventory (v1)

Nine card types. Each declares an `isEligible(data)` rule; cards returning
`eligible=false` are excluded from the carousel pool entirely.

| # | Type | Eligibility | Cache | Auth |
|---|------|------------|-------|------|
| 1 | `anime_of_day` | Always (catalog non-empty) | day | anon + login |
| 2 | `personal_pick` | recs API returns ≥1 anime | day, per-user | anon (=trending) + login (=personal) |
| 3 | `latest_news` | `changelog.json` has ≥1 entry | day | anon + login |
| 4 | `platform_stats` | Always | day | anon + login |
| 5 | `now_watching` | ≥1 distinct user in `watch_progress` updated in last 5 min | **LIVE** (10s Redis) | anon + login |
| 6 | `random_tail` | catalog has ≥1 anime outside top-100 by score | day | anon + login |
| 7 | `telegram_news` | Telegram parser returned ≥1 item | 30 min (existing) | anon + login |
| 8 | `not_time_yet` | logged-in user has ≥1 anime in Planned/Postponed with airing started | **LIVE** (30s Redis) | login only |
| 9 | `continue_watching_new` | logged-in user has ≥1 anime in Watching with a new episode aired since last `watch_progress` | **LIVE** (30s Redis) | login only |

### 4.1 Card data shapes

```ts
type SpotlightCard =
  | { type: 'anime_of_day';        data: { anime: Anime; reason_i18n_key?: string } }
  | { type: 'personal_pick';       data: { items: Anime[] /* len 1..3 */; reason_i18n_key?: string } }
  | { type: 'latest_news';         data: { entries: ChangelogEntry[] /* len 1..3 */ } }
  | { type: 'platform_stats';      data: { metrics: { key: string; value: number; delta?: number }[] } }
  | { type: 'now_watching';        data: { sessions: { username: string; user_public_id: string; anime: Anime; episode_number: number; updated_at: string }[] /* len 1..3 */ } }
  | { type: 'random_tail';         data: { anime: Anime } }
  | { type: 'telegram_news';       data: { posts: { text: string; link: string; published_at: string }[] /* len 1..3 */ } }
  | { type: 'not_time_yet';        data: { anime: Anime; list_status: 'planned' | 'postponed' } }
  | { type: 'continue_watching_new'; data: { anime: Anime; next_episode: number; last_watched_episode: number; aired_at: string } }
```

### 4.2 Adaptive layout rules

For card types that can carry multiple items (#2, #3, #5, #7):

- **N=0:** card not eligible → excluded.
- **N=1:** "single layout" (one large item).
- **N=2:** "single layout" with **one randomly selected** item shown.
- **N≥3:** "stack layout" with top 3 items (most recent / highest-ranked).

For card types that always carry one item (#1, #4, #6, #8, #9): only the
single layout exists.

### 4.3 Mobile vs desktop layouts (type-specific)

The block is full-width up to `max-w-7xl` (matching other Home rows). Height
~280-360px desktop, ~360-480px mobile (varies by card content).

| Card type | Desktop | Mobile |
|-----------|---------|--------|
| Poster card (single-item: #1, #6, #8, #9) | Poster left + meta right | Stacked vertically (poster top, meta below) |
| Multi-poster (#2 stack=3) | 3 posters in row | 1 poster + "+ ещё 2 →" link to the relevant list/view |
| Multi-text (#3, #5, #7) | 3 items in row | 3 items stacked vertically (compact) |
| Stats (#4) | 3 metrics in row | metrics stacked vertically |

The "+ ещё 2 →" link target per card (mobile only, multi-poster types):
- `personal_pick` → `/browse?sort=trending` (anon) or `/recs` (login)

Multi-text cards (`latest_news`, `now_watching`, `telegram_news`) do NOT use
the "+ ещё 2 →" pattern — on mobile they stack all 3 items vertically, since
text rows stack compactly without overwhelming hero height.

## 5. Backend architecture

### 5.1 Endpoint

```
GET /api/home/spotlight
  Authorization: optional (Bearer JWT)
  Response 200:
    {
      "cards": [ SpotlightCard, … ],         // only eligible cards, server-filtered
      "generated_at": "2026-05-21T12:34:56Z"
    }
  Response 500: { "error": { ... } } — frontend hides the block silently
```

Routed through gateway as `GET /api/home/spotlight → catalog:8081`.

### 5.2 Service ownership

`services/catalog` owns the endpoint. Reasoning: catalog already knows about
animes, news (Telegram parser), and recs; it is the natural aggregator for
content-shaped data. Cards 5/8/9 require player data — catalog fan-outs over
HTTP to `http://player:8083`.

```
services/catalog/internal/
  handler/spotlight.go          ← new handler
  service/spotlight/            ← new package: aggregator + per-card resolvers
    aggregator.go
    cards/
      anime_of_day.go
      personal_pick.go
      latest_news.go
      platform_stats.go
      now_watching.go
      random_tail.go
      telegram_news.go
      not_time_yet.go
      continue_watching_new.go
    client/
      player_client.go          ← HTTP client to player service
      web_client.go             ← HTTP client to web (for changelog.json)
  transport/router.go           ← + r.Get("/home/spotlight", h.Get)
```

### 5.3 Cache strategy

Per-card TTLs in Redis (keys prefixed `spotlight:`):

| Card | Key | TTL | Rationale |
|------|-----|-----|-----------|
| `anime_of_day` | `spotlight:anime_of_day:<YYYY-MM-DD>` | 24h | Deterministic by date |
| `personal_pick` (anon) | `spotlight:trending:<YYYY-MM-DD>` | 24h | Global trending of the day |
| `personal_pick` (login) | `spotlight:personal:<user_id>:<YYYY-MM-DD>` | 24h | Per-user personalization |
| `latest_news` | `spotlight:changelog:<YYYY-MM-DD>` | 24h | Changelog rarely changes mid-day |
| `platform_stats` | `spotlight:stats:<YYYY-MM-DD>` | 24h | Weekly aggregates |
| `random_tail` | `spotlight:random_tail:<YYYY-MM-DD>` | 24h | Deterministic by date |
| `telegram_news` | reuse existing `news:telegram` | 30 min | existing handler |
| `now_watching` | `spotlight:now_watching` | 10s | live but rate-limited |
| `not_time_yet` | `spotlight:not_time_yet:<user_id>` | 30s | live but rate-limited |
| `continue_watching_new` | `spotlight:continue_new:<user_id>` | 30s | live but rate-limited |

Aggregator response itself is NOT cached as a whole (so any expiring sub-key
recomputes only that card). The `cache.GetOrSet` pattern is used per card.

### 5.4 Data sources per card

**1. anime_of_day** — `catalog.GetTopRated(score_min=8.0, limit=200)`, pick
`items[seed_from_date % len]` where `seed_from_date = int(YYYY*100*32 + MM*32 + DD)`.

**2. personal_pick** — anon: `GET /api/anime/trending?limit=10`. Login: existing
recs API. Random 3 from top-10; if recs<3 → fall back to single/stack rule.

**3. latest_news** — `web_client.GetChangelog()` → `http://web:80/changelog.json`
inside docker network. Take first 3 entries (newest).

**4. platform_stats** — three metrics computed from existing DBs:
- `anime_added_7d`: `SELECT count(*) FROM animes WHERE created_at > NOW()-INTERVAL '7d'`
- `episodes_added_7d`: depending on data model (episodes table or sum of
  `episodes_aired - lag(episodes_aired)`). Stub TBD if no episode log exists;
  fallback `null` → metric hidden, card still eligible if ≥1 metric present.
- `active_rooms_7d`: from `rooms` service `count(*) WHERE created_at > NOW()-INTERVAL '7d'`.

If all three metrics fail to compute → card eligible=false.

**5. now_watching** —
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
Then drop to len 1..3 via the adaptive rule. **Privacy:** opt-out via existing
user privacy flag if present; otherwise add `users.show_in_now_watching`
boolean default true (TBD — see Open Questions).

**6. random_tail** — `catalog.GetByScoreRank(rank_min=101, rank_max=2000)`,
pick `items[seed_from_date % len]`. Excludes top-100 to favor discovery.

**7. telegram_news** — reuse existing `catalog/internal/parser/telegram` +
existing `news:telegram` cache. Take first 3 posts.

**8. not_time_yet** — requires login. Player client call:
```
GET /internal/users/{user_id}/list?status=planned,postponed
  → filter to anime where airing_started=true (current_episode_number > 0)
  → random pick 1
```

**9. continue_watching_new** — requires login. Player client call:
```
GET /internal/users/{user_id}/list?status=watching
  → for each, compare anime.episodes_aired vs last watch_progress.episode_number
  → filter where episodes_aired > last_watched + 1 (i.e. a NEW episode aired)
  → most recent newly-aired
```

### 5.5 Fan-out timeout & graceful degradation

Each per-card resolver has a context deadline of **800ms**. If a resolver
times out or errors, that card returns `eligible=false` and a structured log
entry is emitted (`spotlight.card_failed{type, error}`). The aggregator never
fails the whole request because of one bad card — the client always gets at
least the static cards.

Overall request budget: **2 seconds**. If the aggregator itself times out,
catalog returns the last-known-good cached snapshot (best-effort).

## 6. Frontend architecture

### 6.1 Component tree

```
HeroSpotlightBlock.vue              ← carousel state machine, fetches /api/home/spotlight
├── CarouselControls.vue            ← chevrons + dots
└── (one of, per active slide)
    ├── AnimeOfDayCard.vue
    ├── PersonalPickCard.vue
    ├── LatestNewsCard.vue
    ├── PlatformStatsCard.vue
    ├── NowWatchingCard.vue
    ├── RandomTailCard.vue
    ├── TelegramNewsCard.vue
    ├── NotTimeYetCard.vue
    └── ContinueWatchingNewCard.vue
```

All card components live in `frontend/web/src/components/home/spotlight/`.

### 6.2 State machine

`HeroSpotlightBlock.vue`:
- `onMounted` → `GET /api/home/spotlight`
- On 200: `cards.value = response.cards`, `currentIndex.value = randomInt(0, cards.length-1)`, start 7s interval
- On 5xx or empty cards: hide the block (`v-if="cards.length > 0"`)
- `hover`/`focusin` → pause interval; `leave`/`focusout` → resume
- Arrow keys / chevrons → seek prev/next with wrap-around
- Dots → seek to index
- On `prefers-reduced-motion`: skip starting the interval

### 6.3 Loading state

While the fetch is in-flight: render a skeleton placeholder with the same
height as the final block (avoid layout shift). Animated pulse via existing
Tailwind classes.

### 6.4 Error state

If the endpoint fails: the block is hidden entirely (no error banner — the
page is functional without the spotlight). One `console.warn` for debugging.
A retry attempt on the next route navigation (component re-mount).

### 6.5 i18n

All card-specific strings live in `frontend/web/src/locales/{en,ru}.json`
under a new `spotlight.*` namespace. Each card has its own title key (e.g.
`spotlight.animeOfDay.title`, `spotlight.nowWatching.title`).

## 7. Removed code

`Home.vue`:
- Lines ~39-132: the entire `trendingRecs` row markup.
- Lines ~123-132: the trendingLoading skeleton.
- Related setup-script state (`trendingRecs`, `trendingProgress`,
  `trendingLoading`, `rowLabelKey`, `reasonI18nKey`, `onRecClick`) is removed
  if no other consumer depends on it.

`frontend/web/src/components/home/`:
- No existing components are removed; only the inline row inside `Home.vue`.

The `/api/anime/recommended` endpoint stays — it now powers `personal_pick`
inside the aggregator instead of the standalone row.

## 8. Migration & rollout

1. Backend: ship the `/api/home/spotlight` endpoint behind a feature flag
   env `SPOTLIGHT_ENABLED=true` (default true). If false, returns 404 → block
   self-hides.
2. Frontend: ship `HeroSpotlightBlock` gated by `VITE_HERO_SPOTLIGHT_ENABLED`
   (default true). If false, fall back to the original `trendingRecs` row
   (kept around for one release).
3. After one stable week with `SPOTLIGHT_ENABLED=true`, remove the legacy
   `trendingRecs` markup permanently. Drop the feature flag.

Rollback: flip either env var to `false`; service restart restores the
original UX.

## 9. Open questions / TBD

- **Privacy for `now_watching`:** does the project want a user-level opt-out?
  Decision deferred to plan phase; default is to include all users; if any
  user feedback arrives during rollout, add a `users.show_in_now_watching`
  boolean (default true) and respect it in the SQL filter.
- **Platform stats — episodes_added_7d:** depends on whether the codebase
  tracks per-episode add events. To verify during plan phase. Fallback:
  hide the metric if uncomputable; card stays eligible if ≥1 metric present.
- **Server-side card ordering:** for v1, cards are returned in a fixed
  type-order; the client randomizes the starting index. If needed, a
  future iteration can rotate the order server-side per UA/session.

## 10. Metrics — UXΔ / CDI / MVQ

- **UXΔ = +3 (Better)** — single eye-catching top-of-home surface for
  discovery, news, social signals, and personal nudges; replaces a row
  that was mostly redundant with the dedicated "Подобрано для вас" recs flow.
- **CDI = 0.04 × 21** — Spread: 0.20 (Home.vue + catalog + player + web
  fetches); Shift: 0.20 (one row removed, one block added; nine new
  components but they live in a single folder; new aggregator pattern).
  Effort: 21 (one full vertical-stack: backend aggregator + 9 card
  components + cache plumbing + i18n).
- **MVQ = Griffin 80%/75%** — composite of multiple signal types, soaring
  hero-card vibe, well-bounded scope, predictable structure; mild slop risk
  on the privacy/episodes-stats edges.

## 11. Non-goals (v1)

- Personalization of slide ORDER (only of card content).
- Admin-curated "editorial pick" card type.
- Server-driven A/B testing of card sets.
- Persisting "seen" cards per user to avoid repetition.
- Animations beyond the slide cross-fade.
- A "tip / feature highlight" card.
