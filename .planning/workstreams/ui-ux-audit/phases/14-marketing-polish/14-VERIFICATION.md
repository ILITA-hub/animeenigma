---
status: passed
phase: 14
phase_name: "Marketing-surface polish"
verified: 2026-05-13
---

# Phase 14 Verification: Marketing-surface polish

## Must-have truths scorecard (per 14-PLAN.md)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `ListRepository.CountWatchers(ctx, animeID)` returns int64 from `anime_list WHERE anime_id=? AND status='watching'` (soft-deletes excluded via GORM) | PASS | `services/player/internal/repo/list.go` lines 139-148: `Model(&domain.AnimeListEntry{}).Where("anime_id = ? AND status = ?", animeID, "watching").Count(&count)`. GORM's gorm.DeletedAt on the model auto-applies `deleted_at IS NULL`. |
| 2 | `ListService.GetWatchersCount` wrapper exists | PASS | `services/player/internal/service/list.go` lines 413-417: single-line wrapper `return s.listRepo.CountWatchers(ctx, animeID)`. |
| 3 | `ListHandler.GetWatchersCount` returns `{ count: int }` JSON, public, no auth | PASS | `services/player/internal/handler/list.go` lines 318-333: chi.URLParam → service → `httputil.OK(w, map[string]int64{"count": count})`. No `authz.ClaimsFromContext` call. |
| 4 | Player route `GET /anime/{animeId}/watchers-count` registered under public `/anime/{animeId}` group | PASS | `services/player/internal/transport/router.go` line 192 (inside the public-routes block, BEFORE the AuthMiddleware-protected group at line 195): `r.Get("/watchers-count", listHandler.GetWatchersCount)`. |
| 5 | Gateway proxies `/api/anime/{animeId}/watchers-count` to player, registered BEFORE `/anime/*` → catalog catch-all | PASS | `services/gateway/internal/transport/router.go` line 162: `r.Get("/anime/{animeId}/watchers-count", proxyHandler.ProxyToPlayer)`. Catalog catch-all is at line 180 (`r.HandleFunc("/anime/*", proxyHandler.ProxyToCatalog)`), so chi's order-sensitive routing hits player first. |
| 6 | `animeApi.getWatchersCount` in `client.ts` | PASS | `frontend/web/src/api/client.ts` lines 265-266: `getWatchersCount: (animeId: string) => apiClient.get<{count:number}\|{data:{count:number}}>('/anime/${animeId}/watchers-count')`. |
| 7 | `Anime.vue` renders `<Badge v-if="watchersCount >= 5">` near the score rail using `anime.watchersCount` i18n key + `formatCount` | PASS | `frontend/web/src/views/Anime.vue` lines 104-107: `<Badge v-if="watchersCount >= 5" variant="default" class="flex items-center gap-1"><span aria-hidden="true">👥</span><span>{{ $t('anime.watchersCount', { count: formatCount(watchersCount) }) }}</span></Badge>`. |
| 8 | `formatCount(n)` uses `Intl.NumberFormat` with `notation: 'compact'` | PASS | `views/Anime.vue` lines 1508-1515: `new Intl.NumberFormat(locale.value, { notation: 'compact', maximumFractionDigits: 1 }).format(n)` with try/catch fallback to `n.toString()`. |
| 9 | `fetchWatchersCount` called after anime loads, errors swallowed | PASS | `views/Anime.vue` lines 1523-1532 (fn body) + line 2085 (`void fetchWatchersCount()` after `await fetchReviews()`). Reset in `loadAnimeData` at line 1988 (`watchersCount.value = 0`). |
| 10 | `search.placeholder` updated in en.json | PASS | `frontend/web/src/locales/en.json` line 247: `"placeholder": "Search: title or genre"`. |
| 11 | `search.placeholder` updated in ru.json | PASS | `frontend/web/src/locales/ru.json` line 247: `"placeholder": "Поиск: название или жанр"`. |
| 12 | `search.placeholder` updated in ja.json | PASS | `frontend/web/src/locales/ja.json` line 247: `"placeholder": "検索: タイトルまたはジャンル"`. |
| 13 | New `About.vue` at `frontend/web/src/views/About.vue` with 8 FAQ `<details>` items + header | PASS | `frontend/web/src/views/About.vue` — 73 lines, wrapper `max-w-3xl mx-auto px-4 py-12`, `t('about.title')` + `t('about.subtitle')` header, `v-for="item in faqs"` with `<details>` + rotating chevron. Hides `summary::-webkit-details-marker`. |
| 14 | Router `/about` registered with `titleKey: 'about.title'` | PASS | `frontend/web/src/router/index.ts` lines 86-92: `{ path: '/about', name: 'about', component: () => import('@/views/About.vue'), meta: { titleKey: 'about.title' } }`. |
| 15 | Navbar drawer carries About link (no Footer.vue exists) | PASS | `frontend/web/src/components/layout/Navbar.vue` lines 253-263 (mobile drawer block): `<router-link to="/about" ...>{{ $t('nav.about') }}</router-link>`. Confirmed `ls frontend/web/src/components/layout/` shows only `Navbar.vue` + `index.ts`. |
| 16 | i18n: 18 new keys × 3 locales = 54 entries; JSON parses clean; lint passes | PASS | `bash scripts/i18n-lint.sh`: `Missing keys: 0`, `Syntax errors: 0`. All 54 new entries present: `nav.about`, `anime.watchersCount`, `about.title`, `about.subtitle`, `about.faqs.q{1..8}.{q,a}` in each of en/ru/ja. `python3 json.load` validates all three files. |
| 17 | All five planned commits exist in git log, plus one Rule 3 fix commit | PASS | `git log --oneline -10`: `deb2d0e` (Wave 1 backend), `efb9d12` (Wave 1 frontend), `a73e8de` (Wave 2), `76cfad3` (Wave 3), `d691581` (Wave 4), `5dd6f72` (Dockerfile fix for streamprobe). |

**Overall status:** PASSED — 17/17 must-have truths met.

## Artifact verification (per 14-PLAN.md "Files touched")

| Artifact | Path | Contains-check | Status |
|---|---|---|---|
| CountWatchers repo method | `services/player/internal/repo/list.go` | `func (r *ListRepository) CountWatchers` | FOUND (1 match) |
| GetWatchersCount service | `services/player/internal/service/list.go` | `func (s *ListService) GetWatchersCount` | FOUND (1 match) |
| GetWatchersCount handler | `services/player/internal/handler/list.go` | `func (h *ListHandler) GetWatchersCount` | FOUND (1 match) |
| Player public route | `services/player/internal/transport/router.go` | `r.Get("/watchers-count", listHandler.GetWatchersCount)` | FOUND (1 match) |
| Gateway proxy route | `services/gateway/internal/transport/router.go` | `/anime/{animeId}/watchers-count` | FOUND (1 match) |
| API client | `frontend/web/src/api/client.ts` | `getWatchersCount` | FOUND (1 match) |
| Anime.vue badge + fetch | `frontend/web/src/views/Anime.vue` | `watchersCount`, `formatCount`, `fetchWatchersCount`, `anime.watchersCount` | FOUND (8 matches — badge, ref, helper, fetcher, reset, call, 2 i18n) |
| About.vue (new) | `frontend/web/src/views/About.vue` | `<details`, `t('about.faqs.q1.q')` (via faqs[]) | FOUND (new file, 73 lines) |
| Router `/about` | `frontend/web/src/router/index.ts` | `path: '/about'`, `name: 'about'` | FOUND |
| Navbar About link | `frontend/web/src/components/layout/Navbar.vue` | `to="/about"`, `$t('nav.about')` | FOUND |
| en.json keys | `frontend/web/src/locales/en.json` | `about.faqs.q1.q`, `anime.watchersCount`, `nav.about` | FOUND (18 new keys) |
| ru.json keys | `frontend/web/src/locales/ru.json` | same | FOUND (18 new keys) |
| ja.json keys | `frontend/web/src/locales/ja.json` | same | FOUND (18 new keys) |

## Test results

### Backend — `go test ./...` in player service

```
$ cd services/player && go test ./...
ok  github.com/ILITA-hub/animeenigma/services/player/cmd/player-api
ok  github.com/ILITA-hub/animeenigma/services/player/internal/handler
ok  github.com/ILITA-hub/animeenigma/services/player/internal/repo
ok  github.com/ILITA-hub/animeenigma/services/player/internal/service
ok  github.com/ILITA-hub/animeenigma/services/player/internal/service/recs
ok  github.com/ILITA-hub/animeenigma/services/player/internal/service/recs/signals
ok  github.com/ILITA-hub/animeenigma/services/player/internal/transport
```

All packages green. No new tests added (the new code is a single COUNT
query + a thin wrapper + a public handler; existing handler/router test
patterns already cover the wiring shape, and the smoke test below
exercises the full path against the live database).

### Backend — `go build ./...` in gateway service

Clean — no output, exit 0.

### Frontend type-check

```
$ cd frontend/web && bunx vue-tsc --noEmit
(clean — no output, exit 0)
```

### i18n-lint

```
=== Checking for missing translation keys ===
  OK All locale files have matching keys

=== Checking message-format syntax in locale files ===
  OK All locale messages parse cleanly

=== Summary ===
  Missing keys:    0
  Syntax errors:   0
  Hardcoded text:  20 (warning — all pre-existing in HanimePlayer.vue etc.)
  Unused keys:     16 (warning — all pre-existing)

PASS: No blocking i18n issues.
```

### Redeploys

```
$ make redeploy-player
[INFO] player is running
[INFO] player:8083 - healthy

$ make redeploy-gateway
[INFO] gateway is running
[INFO] gateway:8000 - healthy

$ make redeploy-web
Web frontend redeployed
```

All three services deployed cleanly after the Rule 3 Dockerfile fix (see
"Deviations" in SUMMARY.md). Pre-fix, `make redeploy-player` failed at
`go mod download` due to a missing `libs/streamprobe` COPY directive — a
pre-existing infra bug introduced by an earlier workstream commit
(abe5199 / 6aeac90).

### Smoke test against the live endpoint

```
$ curl -s http://localhost:8000/api/anime/8e913af8-580b-4ae6-b6ee-eb4e9ca71e1f/watchers-count
{"success":true,"data":{"count":0}}

$ ANIME_WITH_WATCHERS=8dd0c714-2760-44c8-83d1-dbe090d6cd9f   # from psql: anime_list WHERE status='watching' GROUP BY anime_id ORDER BY count DESC LIMIT 1 → 3 watchers
$ curl -s "http://localhost:8000/api/anime/8dd0c714-2760-44c8-83d1-dbe090d6cd9f/watchers-count"
{"success":true,"data":{"count":3}}
```

Endpoint reachable through the gateway proxy, response shape matches the
handler contract (`{ count: int }` wrapped by httputil.OK), and counts
match the underlying `anime_list` table.

### Grep checks (per 14-PLAN.md Wave 5 checklist)

```
$ grep -n "CountWatchers" services/player/internal/repo/list.go services/player/internal/service/list.go services/player/internal/handler/list.go
services/player/internal/service/list.go:417: return s.listRepo.CountWatchers(ctx, animeID)
services/player/internal/repo/list.go:139:    // CountWatchers returns the number of anime_list rows where ...
services/player/internal/repo/list.go:144:    func (r *ListRepository) CountWatchers(ctx context.Context, animeID string) (int64, error)

$ grep -n "watchers-count" services/gateway/internal/transport/router.go
162: r.Get("/anime/{animeId}/watchers-count", proxyHandler.ProxyToPlayer)

$ grep -n "getWatchersCount" frontend/web/src/api/client.ts frontend/web/src/views/Anime.vue
frontend/web/src/api/client.ts:265: getWatchersCount: (animeId: string) =>
frontend/web/src/views/Anime.vue:1526: const response = await animeApi.getWatchersCount(anime.value.id)

$ grep -n "about.faqs.q1.q\|\"q1\"" frontend/web/src/locales/{en,ru,ja}.json
en.json:716: "q1": { ... }
ru.json:716: "q1": { ... }
ja.json:716: "q1": { ... }

$ grep -n "/about\|name: 'about'" frontend/web/src/router/index.ts
87: path: '/about',
88: name: 'about',
```

All five required grep targets resolve.

## Summary

Phase 14 closed UX-28, UX-29, UX-30 in 6 commits (5 feature + 1 Rule 3
fix). 14 files touched (12 modified + 1 new view + 1 new SUMMARY/
VERIFICATION pair). Backend changes are surgical (one COUNT query + one
public route + one gateway proxy line). Frontend changes are additive
(one badge, one composable formatter, one new view, one drawer link, 54
i18n entries). No new dependencies, no schema changes, no auth changes.

Verification confirmed against the live system: endpoint reachable,
returns expected shape, counts match the database. Type-check clean,
i18n-lint clean, all three planned redeploys succeed and report healthy.

Status: **PASSED**.
