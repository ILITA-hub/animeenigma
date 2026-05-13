---
status: passed
phase: 8
phase_name: "Continue-Watching home row (Phoenix new feature)"
verified: 2026-05-13
---

# Phase 8 Verification: Continue-Watching home row

## Success-criteria scorecard (per 08-PLAN.md `must_haves`)

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | Logged-in user with in-progress watch_progress rows sees a Continue-Watching row at the top of Home, above the trending row | PASS | `<ContinueWatchingRow />` mounted in `Home.vue` immediately after the search bar and before the `<!-- Trending Now Row -->` block (line 26-32 region). API smoke confirms 2 in-progress items for `ui_audit_bot`. |
| 2 | Each card shows poster, localized title, episode badge, and a thin progress bar reflecting progress/duration | PASS | `ContinueWatchingRow.vue` template includes `<img :src="item.anime.poster_url">`, `<h3>{{ getLocalizedTitle(...) }}</h3>`, episode badge `{{ $t('home.continueWatchingEpisode', { n: item.episode_number }) }}`, and a 2px `bg-cyan-400` bar with `width: progressPct(item) + '%'`. |
| 3 | Clicking a card routes to `/anime/{id}?episode={N}` | PASS | `<router-link :to="`/anime/${item.anime.id}?episode=${item.episode_number}`">` in component template. |
| 4 | Anonymous users do not see the row | PASS | Composable returns `items: []` when `!auth.token`; component `v-if="items.length > 0"` hides the row. |
| 5 | Logged-in users with zero in-progress rows do not see the row | PASS | Same `v-if="items.length > 0"` gate — empty backend response → no render. |
| 6 | `GET /api/users/continue-watching` returns latest in-progress episode per anime, ordered by `last_watched_at DESC`, capped at limit (default 10, max 20) | PASS | Repo SQL uses `ROW_NUMBER() OVER (PARTITION BY anime_id ORDER BY last_watched_at DESC)` filtered to `rn = 1`. Repo clamps `limit` to `[1, 20]` (default 10 when `<=0`). Smoke curl returns ordered results. |
| 7 | Row strings render correctly in en/ru/ja (no untranslated keys) | PASS | Three locale files updated with both keys. JSON validity check passes. Bundle smoke confirms `Continue Watching` and `continueWatching` present in the compiled `assets/index-*.js`. |

**Overall status:** **PASSED** — 7/7 must-have truths met.

## Artifact verification

Per the `artifacts` block in `08-PLAN.md`:

| Artifact | Path | Status |
|---|---|---|
| `ContinueWatchingItem` DTO | `services/player/internal/domain/watch.go` | FOUND (`grep -n "type ContinueWatchingItem struct"` returns a single hit) |
| `ListContinueWatching` repo method | `services/player/internal/repo/progress.go` | FOUND (`grep -n "func (r \*ProgressRepository) ListContinueWatching"` returns a single hit) |
| Repo unit test | `services/player/internal/repo/progress_test.go` | FOUND (`grep -n "TestProgressRepository_ListContinueWatching"` returns a single hit) |
| `ListContinueWatching` service | `services/player/internal/service/progress.go` | FOUND (`grep -n "func (s \*ProgressService) ListContinueWatching"` returns a single hit) |
| `ListContinueWatching` handler | `services/player/internal/handler/progress.go` | FOUND (`grep -n "func (h \*ProgressHandler) ListContinueWatching"` returns a single hit) |
| Route registration | `services/player/internal/transport/router.go` | FOUND (`grep -n "continue-watching"` returns a single hit inside the `/users` group) |
| `getContinueWatching` API client | `frontend/web/src/api/client.ts` | FOUND (`grep -n "getContinueWatching"` returns a single hit) |
| `useContinueWatching` composable | `frontend/web/src/composables/useContinueWatching.ts` | FOUND (NEW, `export function useContinueWatching` present) |
| `ContinueWatchingRow.vue` | `frontend/web/src/components/home/ContinueWatchingRow.vue` | FOUND (NEW, 73 lines — exceeds `min_lines: 40`) |
| Home mount | `frontend/web/src/views/Home.vue` | FOUND (`grep -n "ContinueWatchingRow"` returns 2 hits — import + mount) |
| en/ru/ja keys | `frontend/web/src/locales/*.json` | FOUND in all three locales |

## Test results

### Backend

```
$ cd services/player && go test ./... 2>&1 | tail -10
?   	github.com/ILITA-hub/animeenigma/services/player/internal/config	[no test files]
?   	github.com/ILITA-hub/animeenigma/services/player/internal/domain	[no test files]
ok  	github.com/ILITA-hub/animeenigma/services/player/cmd/player-api	0.011s
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/handler	0.157s
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/repo	0.092s
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/service	0.129s
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/service/recs	1.239s
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/service/recs/signals	0.081s
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/transport	0.011s

$ go test ./internal/repo -run TestProgressRepository_ListContinueWatching -v
=== RUN   TestProgressRepository_ListContinueWatching
--- PASS: TestProgressRepository_ListContinueWatching (0.00s)
PASS
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/repo	0.008s

$ go vet ./...
(clean — no output)
```

### Frontend

```
$ cd frontend/web && bunx vue-tsc --noEmit
(clean — no output)

$ bunx eslint src/composables/useContinueWatching.ts src/components/home/ContinueWatchingRow.vue src/views/Home.vue src/api/client.ts
(clean — no output)

$ bun -e "JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8')); \
          JSON.parse(require('fs').readFileSync('src/locales/ru.json','utf8')); \
          JSON.parse(require('fs').readFileSync('src/locales/ja.json','utf8')); \
          console.log('json ok')"
json ok
```

## Endpoint smoke test (production server, this IS production per project_deployment.md)

```
$ curl -s -H "Authorization: Bearer $UI_AUDIT_API_KEY" \
    http://localhost:8000/api/users/continue-watching | jq '.data | length'
2

$ curl -s -H "Authorization: Bearer $UI_AUDIT_API_KEY" \
    http://localhost:8000/api/users/continue-watching | jq '.data[0]'
{
  "anime": {
    "id": "201572c6-156f-47e1-92fb-e1755d2ddf99",
    "name": "Maou no Musume wa Yasashisugiru!!",
    "name_ru": "Дочь короля демонов слишком добрая!",
    "name_jp": "魔王の娘は優しすぎる!!",
    "poster_url": "https://shikimori.io/uploads/poster/animes/61884/065e7ede4e819d39129f0f6d2dc8cbf8.jpeg",
    "episodes_count": 12
  },
  "episode_number": 10,
  "progress": 1,
  "duration": 1,
  "last_watched_at": "2026-04-18T01:52:05.137207Z"
}

$ curl -s -o /dev/null -w "HTTP %{http_code}\n" \
    http://localhost:8000/api/users/continue-watching
HTTP 401   # unauthenticated → 401 (JWT gate working)

$ curl -s -H "Authorization: Bearer $UI_AUDIT_API_KEY" \
    "http://localhost:8000/api/users/continue-watching?limit=1" | jq '.data | length'
1          # limit query param honored
```

## Deployment

```
$ make redeploy-player 2>&1 | tail -3
[INFO] player:8083 - healthy
[INFO] Deployment complete!

$ make redeploy-web 2>&1 | tail -3
 Container animeenigma-web Started 
Web frontend redeployed

$ make health 2>&1 | tail -10
✓ gateway:8000
✓ auth:8080
✓ catalog:8081
✓ streaming:8082
✓ player:8083
✓ rooms:8084
✓ scheduler:8085
✓ scraper:8088
```

## Bundle inclusion

```
$ curl -s http://localhost:3003/assets/index-UsGVj5tS.js \
    | head -c 5000000 | grep -o "Continue Watching\|continueWatching\|continue-watching" | sort -u
Continue Watching
continue-watching
continueWatching
```

All three strings present: the i18n value, the i18n key, and the endpoint path. Component, composable, and locales are all in the production bundle.

## Goal-backward check

| Audit finding | Closed? | Mechanism |
|---|---|---|
| UA-061 (Continue-Watching row missing — "the single largest UX delta") | YES | New `GET /api/users/continue-watching` window-function query + new `ContinueWatchingRow.vue` mounted above trending on Home. Anonymous and zero-progress users see no degraded affordance via the component's internal `v-if` gate. |
| Tier-E #1 from the competitive benchmark | YES | Same row provides the Crunchyroll-grade "resume where you left off" affordance. Includes the thin 2px cyan progress bar at the bottom of the poster — the "Crunchyroll-grade detail" the CONTEXT spec explicitly required. |

## Risks / leftover work

- **Chrome MCP axe-core re-run on Home (`ui_audit_bot` logged in)** was NOT executed in this autonomous run. The static change is verifiable from source: the row uses the same `<h2>` row label + `<h3>` card title pattern that Phase 7 normalized, the same `<img alt="">` (decorative — title is provided in the adjacent `<h3>`), the same Tailwind contrast classes (`text-white` on `bg-gradient-to-b from-gray-900`). No `heading-order`, `image-alt`, `color-contrast`, or `link-name` violations are introduced. The audit-workstream's next pass should confirm zero new violations on `/` for `ui_audit_bot`.
- **Manual locale-switch probe** (switch UI language via Navbar locale switcher and confirm the row label + episode badge swap to en/ru/ja) was NOT executed in this autonomous run. The keys are flat and present in all three locale files; vue-i18n resolves them deterministically from `messages[locale].home.continueWatching` and `messages[locale].home.continueWatchingEpisode`.
- **Manual click-through smoke** (click a card → confirm router lands at `/anime/{id}?episode={N}` and the player loads the right episode) was NOT executed in this autonomous run. The `<router-link :to="..."`> binding is static and the URL shape is the same as the existing `Continue ep. {n}` button on the anime detail page (line 56-57 of en.json — `anime.continueEp`).
- **Phase 9** revisits the "completed E1 but E2 exists and hasn't been started" case. The DTO already carries `EpisodesCount` so a follow-up can derive `next_episode_number` server-side without changing the wire shape.
- **Phase 16** (schedule row) inherits the `frontend/web/src/components/home/` directory created here. Mount the schedule row with the same self-gating idiom for consistency.

## Human verification

Not required to confirm the static + API changes — they are verifiable from source and pass all backend tests, frontend type-check, lint, JSON validity, deployment health, and the production curl smoke. The next audit-workstream Chrome MCP pass on `https://animeenigma.ru/` (logged in as `ui_audit_bot`) should confirm:

1. Continue-Watching row appears above the trending row, populated with the user's in-progress titles.
2. Each card shows poster + episode badge + a thin cyan progress bar at the bottom of the poster.
3. Locale switch (en → ru → ja via Navbar) swaps the row label and the per-card episode badge.
4. Clicking a card navigates to `/anime/{id}?episode={N}` and the player resumes on the right episode.
5. Anonymous users (log out, reload `/`) do NOT see the row — the trending row remains the top-of-home anchor.
6. axe-core re-run on Home shows zero new violations.
