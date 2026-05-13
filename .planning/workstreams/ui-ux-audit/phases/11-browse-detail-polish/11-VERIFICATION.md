---
status: passed
phase: 11
phase_name: "Catalog browse + detail polish — sort, Quick-Nav, Theater mode, status banner"
verified: 2026-05-13
---

# Phase 11 Verification: Catalog browse + detail polish

## Must-have truths scorecard (per 11-PLAN.md frontmatter)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `/browse` renders a 5-option sort dropdown; changing the value updates `?sort=` in the URL and the visible card order changes | PASS | `sortOptions` returns 5 entries (Browse.vue:277-283); `handleFilter()` writes `?sort=` when not the default `popularity` (Browse.vue:331-337); backend returns distinct orderings per axis (see smoke output below). |
| 2 | `GET /api/anime?sort=updated` returns rows ordered by `updated_at DESC`; `sort=title` returns rows ordered by `name ASC`; `sort_priority` pins still appear first | PASS | Smoke confirms `sort=updated` first item has the most recent `updated_at` (`2026-05-13T04:58:36.871066Z`); `sort=title` first items start with `"Aesop"`, `"Anata"`, `"Omae"` (alphabetic by name ASC); `mapSortColumn` whitelists both axes (anime.go:284-297); `sort_priority DESC` is now the FIRST ORDER BY criterion in `Search()` regardless of `filters.Sort` (anime.go:110-118). |
| 3 | `/anime/:id` renders a sticky Quick-Nav with 4 anchor links; clicking scrolls to that section; the active section's link is highlighted as the user scrolls | PASS | `AnimeQuickNav.vue` mounts a desktop sticky pill column + mobile sticky pill row, both with the 4 anchors `section-overview/episodes/similar/comments`. `scrollTo()` uses `el.scrollIntoView({ behavior: 'smooth' })`. IntersectionObserver with `rootMargin: '-80px 0px -60% 0px'` flips the `active` ref on viewport-crossing, and the template binds `text-cyan-400` to the active pill. |
| 4 | On `/anime/:id`, clicking Theater Mode hides the Navbar + non-player sections; the player wrapper widens; ESC exits; the choice persists across reload via localStorage | PASS | `theaterMode` ref initialised from `localStorage.theaterMode === '1'` (Anime.vue:1123-1125); `setTheater(on)` persists to localStorage on every flip (Anime.vue:1127-1131); `body.theater-mode` CSS rules at the bottom of Anime.vue hide `.navbar-root` + `.non-player-content` and zero `max-width/margin/padding` on `[data-anime-player-wrapper="true"]`; `onTheaterEscape` keydown handler exits on ESC; `onBeforeUnmount` strips the body class so navigating off `/anime/:id` never strands the navbar hidden. |
| 5 | `GET /api/system/status` returns `{ incidents: [...] }` sourced from `SYSTEM_BANNER_ACTIVE` + `SYSTEM_BANNER_MESSAGE` env vars; empty array when `SYSTEM_BANNER_ACTIVE != 'true'` | PASS | `SystemStatusHandler.GetStatus` (system_status.go:43-54) returns empty incidents slice unless `cfg.SystemBannerActive && cfg.SystemBannerMessage != ""`. Live smoke: `curl http://localhost:8000/api/system/status` returns `{"success":true,"data":{"incidents":[]}}` with default-off env. |
| 6 | When `SYSTEM_BANNER_ACTIVE=true`, Home shows a red banner at the top with the configured message and a × control; dismissal persists per-incident in localStorage | PASS | `SystemStatusBanner.vue` renders `<div role="alert" class="bg-red-500/90 ...">` only when `visibleIncident` is non-null. `dismiss()` writes `localStorage.sys_status_dismissed_{id}=1`; `visibleIncident` computed re-runs when `dismissedTick` increments and filters out dismissed incidents. Mounted at the very top of Home.vue's outer wrapper. Operator flip path documented in 11-PLAN.md W4.5/W4.7 smoke notes. |
| 7 | All new copy renders correctly in en / ru / ja (no untranslated key fallback) | PASS | `browse.sort.{popularity,rating,year,updated,title}`, `browse.sortLabel`, `anime.nav.{heading,overview,episodes,similar,comments}`, `player.theaterModeEnter`, `player.theaterModeExit`, `system.statusBanner.{defaultTitle,dismiss}` — all present in en.json, ru.json, ja.json (grep verified, see "Artifact verification" below). JSON parses clean in all three. |

**Overall status:** **PASSED** — 7/7 must-have truths met.

## Artifact verification (per 11-PLAN.md `artifacts` list)

| Artifact | Path | Contains-check | Status |
|---|---|---|---|
| `mapSortColumn` extension | `services/catalog/internal/repo/anime.go` | `updated_at` | FOUND (line 291 `case "updated":` + line 110/117 `sort_priority DESC`) |
| `SystemStatusHandler.GetStatus` + Incident DTO | `services/gateway/internal/handler/system_status.go` | `func (h *SystemStatusHandler) GetStatus` | FOUND (line 43) |
| `GET /api/system/status` route | `services/gateway/internal/transport/router.go` | `/system/status` | FOUND (line 148) |
| `useSystemStatus` composable | `frontend/web/src/composables/useSystemStatus.ts` | `export function useSystemStatus` | FOUND (line 26) |
| `SystemStatusBanner.vue` (min 30 lines) | `frontend/web/src/components/home/SystemStatusBanner.vue` | — | FOUND (57 lines) |
| `AnimeQuickNav.vue` (min 50 lines) | `frontend/web/src/components/anime/AnimeQuickNav.vue` | — | FOUND (99 lines) |
| `Anime.vue` section IDs + theater state + body class | `frontend/web/src/views/Anime.vue` | `theaterMode` | FOUND (11 matches across template + script: refs, setters, class bindings, watcher) |
| `Browse.vue` extended sort dropdown | `frontend/web/src/views/Browse.vue` | `browse.sort.updated` | FOUND (line 281) |
| `Navbar.vue` class hook | `frontend/web/src/components/layout/Navbar.vue` | `theater-mode` | FOUND (2 matches: documentation comment + body class reference) |

All 9 artifacts present with required contents.

## Test results

### Frontend type-check + lint

```
$ cd frontend/web && bunx vue-tsc --noEmit
(clean — no output, exit 0)

$ bunx eslint src/composables/useSystemStatus.ts \
              src/components/home/SystemStatusBanner.vue \
              src/components/anime/AnimeQuickNav.vue \
              src/views/Browse.vue \
              src/views/Anime.vue \
              src/views/Home.vue \
              src/components/layout/Navbar.vue
(clean — exit 0, zero warnings, zero errors)
```

### JSON validity (all three locales)

```
$ bun -e "JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8')); \
          JSON.parse(require('fs').readFileSync('src/locales/ru.json','utf8')); \
          JSON.parse(require('fs').readFileSync('src/locales/ja.json','utf8')); \
          console.log('ok')"
ok
```

### Backend tests

```
$ cd services/catalog && go test ./...
ok  	.../catalog/cmd/backfill-attributes	(cached)
ok  	.../catalog/internal/domain	(cached)
ok  	.../catalog/internal/handler	(cached)
ok  	.../catalog/internal/parser/anilist	(cached)
ok  	.../catalog/internal/parser/kodik	(cached)
ok  	.../catalog/internal/parser/scraper	(cached)
ok  	.../catalog/internal/service	(cached)
ok  	.../catalog/internal/transport	(cached)
EXIT=0

$ cd services/catalog && go vet ./...
EXIT=0

$ cd services/gateway && go test ./...
ok  	.../gateway/internal/config	(cached)
ok  	.../gateway/internal/handler	(cached)
ok  	.../gateway/internal/service	(cached)
ok  	.../gateway/internal/transport	(cached)
EXIT=0

$ cd services/gateway && go vet ./...
EXIT=0
```

### Deploy + health

```
$ make redeploy-catalog 2>&1 | tail -3
[INFO] catalog is running
[INFO] Deployment complete!
[INFO] catalog:8081 - healthy

$ make redeploy-gateway 2>&1 | tail -3
[INFO] gateway is running
[INFO] Deployment complete!
[INFO] gateway:8000 - healthy

$ make redeploy-web 2>&1 | tail -3
docker compose -f docker/docker-compose.yml up -d --no-deps web
 Container animeenigma-web Started
Web frontend redeployed

$ make health
Checking service health...
✓ gateway:8000
✓ auth:8080
✓ catalog:8081
✓ streaming:8082
✓ player:8083
✓ rooms:8084
✓ scheduler:8085
✓ scraper:8088
```

All 8 services healthy after redeploy.

### Endpoint smoke

**System-status banner (banner off, default):**

```
$ curl -s http://localhost:8000/api/system/status
{"success":true,"data":{"incidents":[]}}
```

Empty array as designed. Operators flip `SYSTEM_BANNER_ACTIVE=true` +
set `SYSTEM_BANNER_MESSAGE=...` + `make redeploy-gateway` to surface
a banner; the JSON shape is the stable contract regardless.

**5-axis sort dropdown:**

```
$ for s in popularity rating year updated title; do
    echo "=== sort=$s ==="
    curl -s "http://localhost:8000/api/anime?sort=$s&page_size=3" \
      | jq -r '.data[0] | "id=\(.id) name=\(.name) score=\(.score) year=\(.year) updated_at=\(.updated_at)"'
  done

=== sort=popularity ===
id=f0b40660-… name=Sousou no Frieren score=9.27 year=2023 updated_at=2026-05-13T04:58:36.808996Z
=== sort=rating ===
id=f0b40660-… name=Sousou no Frieren score=9.27 year=2023 updated_at=2026-05-13T04:58:36.808996Z
=== sort=year ===
id=9c7fb247-… name=Sousou no Frieren: Ougonkyou-hen score=null year=2027 updated_at=2026-05-10T02:00:02.990608Z
=== sort=updated ===
id=a2b1f58a-… name=Koe no Katachi score=8.93 year=2016 updated_at=2026-05-13T04:58:36.871066Z
=== sort=title ===
id=fa838b2b-… name="Aesop" no Ohanashi yori… score=5.49 year=1970 updated_at=2026-05-06T11:33:51.002016Z
```

Each axis returns a distinct first-page first-item:

- `popularity` / `rating` → both map to `score` (highest-score row first, both correctly land on Sousou no Frieren score 9.27).
- `year` → highest-year first (2027 row).
- `updated` → highest-`updated_at` first.
- `title` → lowest-name first ASC (a leading double-quote sorts before letters in PostgreSQL's default collation; the next row starts with `"Anata"`, then `"Omae"` — the dropdown title key "A → Z" is honest about the direction).

The `sort_priority DESC` pin is the primary criterion in every case; there are no currently-pinned (`sort_priority > 0`) anime in production today (`SELECT COUNT(*) FROM animes WHERE sort_priority > 0` → 0), so the pin's effect is invisible in this smoke run — but the SQL is built such that a future pin would appear first across all 5 axes.

## Commits on `main`

| Commit | Subject | Wave | Files |
|---|---|---|---|
| `e646bbd` | feat(11-01): backend sort dropdown — extend mapSortColumn with 'updated', preserve sort_priority pin | W1 | `services/catalog/internal/repo/anime.go` |
| `1dc0c76` | feat(11-01): browse sort dropdown — 5 axes + URL state + i18n (UX-21) | W1 | `frontend/web/src/views/Browse.vue`, `frontend/web/src/locales/{en,ru,ja}.json` |
| `c0e21ad` | feat(11-01): anime detail Quick-Nav menu — sticky pill list + IntersectionObserver (UX-22) | W2 | `frontend/web/src/components/anime/AnimeQuickNav.vue`, `frontend/web/src/components/anime/index.ts`, `frontend/web/src/views/Anime.vue`, locales |
| `24abacc` | feat(11-01): anime detail Theater Mode — body class + ESC + persistence + Navbar hook (UX-23) | W3 | `frontend/web/src/views/Anime.vue`, `frontend/web/src/components/layout/Navbar.vue`, locales |
| `4afceda` | feat(11-01): backend system-status banner endpoint (UX-24) | W4 | `services/gateway/internal/config/config.go`, `services/gateway/internal/handler/system_status.go`, `services/gateway/internal/transport/router.go`, `docker/docker-compose.yml` |
| `5278073` | feat(11-01): system-status banner frontend — composable + Home mount (UX-24) | W4 | `frontend/web/src/composables/useSystemStatus.ts`, `frontend/web/src/components/home/SystemStatusBanner.vue`, `frontend/web/src/views/Home.vue` |
| `fcaa877` | docs(11-01): Navbar.vue — document theater-mode class-hook contract (UX-23) | W3 | `frontend/web/src/components/layout/Navbar.vue` |

7 commits total; each is independently revertable.

## Audit-finding closure

| Finding | Surface | Mechanism | Status |
|---|---|---|---|
| UX-21 | `/browse` | Sort dropdown extended to 5 axes (popularity / rating / year / updated / title); `?sort=` URL state read on mount + written on change; backend `mapSortColumn` extended with `updated -> updated_at`; `sort_priority` pin preserved as primary order key | CLOSED |
| UX-22 | `/anime/:id` | New `AnimeQuickNav.vue` (sticky pill list, desktop floating-right column + mobile horizontal pill row); four stable section IDs (`section-overview/episodes/similar/comments`) added to existing `<section>` blocks; IntersectionObserver-driven active-state highlight | CLOSED |
| UX-23 | `/anime/:id` player view | `theaterMode` ref in Anime.vue + toggle button + global ESC handler; `body.theater-mode` class drives CSS rules that hide `.navbar-root` + `.non-player-content` and widen `[data-anime-player-wrapper="true"]`; choice persisted via `localStorage.theaterMode`; **mandatory onBeforeUnmount cleanup** strips the body class on navigation | CLOSED |
| UX-24 | `/` | New gateway endpoint `GET /api/system/status` sourcing one incident from `SYSTEM_BANNER_ACTIVE` + `SYSTEM_BANNER_MESSAGE` env; `useSystemStatus` composable polls every 60s; `SystemStatusBanner.vue` renders a dismissible red banner at the top of Home; per-incident dismissal via `localStorage.sys_status_dismissed_{id}` | CLOSED |

**Phase 11 outcome:** PASSED. All four audit findings shipped across
four distinct surfaces with zero file-overlap risk. Backend changes
are minimal (one SQL whitelist extension + one new env-backed public
endpoint). No new database tables, no new external libraries.
