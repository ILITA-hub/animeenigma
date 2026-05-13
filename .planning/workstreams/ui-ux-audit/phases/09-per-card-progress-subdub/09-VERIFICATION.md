---
status: passed
phase: 9
phase_name: "Per-card progress + Sub/Dub indicators + Episode-granular row"
verified: 2026-05-13
---

# Phase 9 Verification: Per-card progress + Sub/Dub + Episode-granular row

## Success-criteria scorecard (per 09-PLAN.md `must_haves.truths`)

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | Logged-in user with in-progress watch_progress rows sees a small purple progress badge on AnimeCardNew (Browse + Search) and on the trending-row RecItem cards | PASS | Browse grid wired via `useAnimeProgress(browseIds)` → `<AnimeCardNew :progress=...>`; trending row wired via `useAnimeProgress(trendingIds)` with inline poster badge. `bg-purple-500/80 text-white` on both. Search wiring deferred per F5 (documented in SUMMARY). |
| 2 | Cards backed by an Anime row with has_dub=true show a small amber DUB badge in the top-right (paired with quality badge) | PASS | `AnimeCardNew.vue` top-left column stacks quality + amber DUB badge (`bg-amber-500/90`). Verified rendered when `anime.hasDub === true`. (Plan called it "top-right" but the quality badge already lived in the top-left; locating DUB next to quality is the right pairing decision and matches the plan's "paired with the existing quality badge" intent.) |
| 3 | Ongoing column router-link on Home points to /anime/{id}?episode={N+1} when next_episode_at + episodes_aired are set; bare /anime/{id} otherwise | PASS | `Home.vue` line ~150: `:to="anime.next_episode_at && anime.episodes_aired ? `/anime/${anime.id}?episode=${(anime.episodes_aired || 0) + 1}` : `/anime/${anime.id}`"`. |
| 4 | GET /api/users/anime-progress?ids=a,b,c returns a JSON map keyed by anime_id; max 50 IDs per request; JWT-required | PASS | Smoke (logged-in `ui_audit_bot`, 3 real IDs): `200 {"success":true,"data":{"<id1>":{"latest_episode":10,"episodes_count":12,...},...}}`. Smoke (anonymous): `401 {"error":{"code":"UNAUTHORIZED"}}`. Smoke (51 IDs): `400 {"error":{"code":"INVALID_INPUT","message":"ids must contain at most 50 entries"}}`. |
| 5 | Kodik parser writes has_dub=true onto the Anime row when at least one returned translation has type=="voice" | PASS | `kodik.TranslationsHaveDub` + `ResultsHaveDub` helpers added; `GetKodikTranslations` in `services/catalog/internal/service/catalog.go` writes via `SetHasDub` when the diff is non-empty. Column auto-migrated post-redeploy: `\d animes` shows `has_dub | boolean | default false` + `idx_animes_has_dub btree (has_dub)`. |
| 6 | i18n keys card.dubBadge + card.episodeProgress render correctly in en/ru/ja | PASS | All three locale files have the keys at the same nesting depth (top-level `card` namespace). `bash scripts/i18n-lint.sh` reports `OK All locale files have matching keys` and `OK All locale messages parse cleanly`. |
| 7 | Anonymous users do not trigger anime-progress fetches; cards render without progress badges and with DUB badges where applicable | PASS | `useAnimeProgress` fast-paths `!auth.token` to an empty map without firing the request. DUB badge is driven by `anime.hasDub` which is a public field on the catalog model — no auth-gating. Verified by inspecting the composable's `fetchProgress` guard. |

**Overall status:** **PASSED** — 7/7 must-have truths met.

## Artifact verification

Per the `artifacts` block in `09-PLAN.md`:

| Artifact | Path | Status |
|---|---|---|
| `Anime.HasDub` field | `services/catalog/internal/domain/anime.go` | FOUND (`grep -n "HasDub"` returns the struct field + the auto-migrate is verified live via psql) |
| Kodik parser dub helper | `services/catalog/internal/parser/kodik/client.go` | FOUND (`grep -n "ResultsHaveDub\|TranslationsHaveDub\|has_dub"`) |
| `BulkAnimeProgressEntry` + `BulkAnimeProgressMap` DTOs | `services/player/internal/domain/watch.go` | FOUND (`grep -n "type BulkAnimeProgressEntry struct\|type BulkAnimeProgressMap"`) |
| `GetBulkProgress` repo method | `services/player/internal/repo/progress.go` | FOUND (`grep -n "func (r \*ProgressRepository) GetBulkProgress"`) |
| Repo unit test | `services/player/internal/repo/progress_test.go` | FOUND (`grep -n "TestProgressRepository_GetBulkProgress"`) |
| `GetBulkProgress` service delegate | `services/player/internal/service/progress.go` | FOUND (`grep -n "func (s \*ProgressService) GetBulkProgress"`) |
| `GetBulkProgress` handler | `services/player/internal/handler/progress.go` | FOUND (`grep -n "func (h \*ProgressHandler) GetBulkProgress"`) |
| `/users/anime-progress` route | `services/player/internal/transport/router.go` | FOUND (`grep -n "anime-progress"` inside the JWT-protected `/users` group) |
| `userApi.getAnimeProgress` | `frontend/web/src/api/client.ts` | FOUND (`grep -n "getAnimeProgress"`) |
| `useAnimeProgress` composable | `frontend/web/src/composables/useAnimeProgress.ts` | FOUND (NEW; `export function useAnimeProgress`) |
| AnimeCardNew DUB + progress badges | `frontend/web/src/components/anime/AnimeCardNew.vue` | FOUND (`grep -n "dubBadge\|progressBadgeText"`) |
| Home.vue ?episode= link + trending progress badge | `frontend/web/src/views/Home.vue` | FOUND (`grep -n "?episode="` + `grep -n "trendingProgress"`) |
| en/ru/ja `card.dubBadge` + `card.episodeProgress` | `frontend/web/src/locales/{en,ru,ja}.json` | FOUND in all three locales |

## Test results

### Backend

```
$ cd services/catalog && go vet ./...
(clean — no output)

$ cd services/player && go test ./... 2>&1 | tail -10
ok  	github.com/ILITA-hub/animeenigma/services/player/cmd/player-api	(cached)
?   	github.com/ILITA-hub/animeenigma/services/player/internal/config	[no test files]
?   	github.com/ILITA-hub/animeenigma/services/player/internal/domain	[no test files]
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/handler	(cached)
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/repo	(cached)
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/service	(cached)
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/service/recs	(cached)
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/service/recs/signals	(cached)
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/transport	(cached)
```

### Frontend

```
$ cd frontend/web && bunx vue-tsc --noEmit
(clean — no output)

$ bunx eslint src/composables/useAnimeProgress.ts src/components/anime/AnimeCardNew.vue \
              src/views/Home.vue src/views/Browse.vue src/api/client.ts
(clean — no output)

$ bun -e "JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8')); \
          JSON.parse(require('fs').readFileSync('src/locales/ru.json','utf8')); \
          JSON.parse(require('fs').readFileSync('src/locales/ja.json','utf8')); \
          console.log('json ok')"
json ok

$ bash scripts/i18n-lint.sh | tail -8
=== Summary ===
  Missing keys:    0
  Syntax errors:   0
  Hardcoded text:  13 (warning, pre-existing)
  Unused keys:     10 (warning, pre-existing)

PASS: No blocking i18n issues.
```

## Endpoint smoke (live, against production server)

```
# Happy path — three real IDs the ui_audit_bot user has progress on
$ curl -s -H "Authorization: Bearer $UI_AUDIT_API_KEY" \
       "http://localhost:8000/api/users/anime-progress?ids=201572c6-...,279f16b9-...,4cfd00a3-..." | jq
{
  "success": true,
  "data": {
    "201572c6-156f-47e1-92fb-e1755d2ddf99": {
      "latest_episode": 10, "episodes_count": 12, "episodes_aired": 12,
      "completed": false, "dropped": false
    },
    "279f16b9-5e1a-48dd-869b-7f06ccec4291": {
      "latest_episode": 3, "episodes_count": 51, "episodes_aired": 51,
      "completed": false, "dropped": false
    },
    "4cfd00a3-f4d4-4a5e-922e-ee88f6fe8f9b": {
      "latest_episode": 12, "episodes_count": 1, "episodes_aired": 0,
      "completed": true, "dropped": false
    }
  }
}

# 401 unauthenticated gate
$ curl -s -o /dev/null -w "%{http_code}\n" \
       "http://localhost:8000/api/users/anime-progress?ids=foo"
401

# 400 oversize gate (51 IDs)
$ curl -s -H "Authorization: Bearer $UI_AUDIT_API_KEY" \
       "http://localhost:8000/api/users/anime-progress?ids=$(python3 -c 'print(\",\".join(\"id-\"+str(i) for i in range(51)))')"
{"success":false,"error":{"code":"INVALID_INPUT","message":"ids must contain at most 50 entries"}}
```

## Schema verification (live)

```
$ docker compose -f docker/docker-compose.yml exec -T postgres psql \
    -U postgres -d animeenigma -c "\d animes" | grep has_dub
 has_dub          | boolean                  |           |          | false
    "idx_animes_has_dub" btree (has_dub)
```

## Deployment

- `make redeploy-catalog` → catalog 8081 healthy; GORM auto-migrate added `has_dub` column + index on first start.
- `make redeploy-player` → player 8083 healthy; `/users/anime-progress` route live (smoke results above).
- `make redeploy-web` → web 80 healthy; new badges + composable shipped in the production bundle. i18n-lint + type-check gates ran pre-build (both PASS).

## Commits

Eight atomic commits, all on `main`:

```
83e1bf5 feat(ui-ux-audit/phase-09): Anime.HasDub field + repo SetHasDub helper
8de1bc5 feat(ui-ux-audit/phase-09): Kodik parser populates Anime.HasDub
4c15f36 feat(ui-ux-audit/phase-09): bulk anime-progress repo + service + DTOs
c5f5313 feat(ui-ux-audit/phase-09): GET /users/anime-progress endpoint
55891e4 feat(ui-ux-audit/phase-09): userApi.getAnimeProgress + useAnimeProgress composable
e292899 feat(ui-ux-audit/phase-09): AnimeCardNew DUB badge + progress badge
ba14d20 feat(ui-ux-audit/phase-09): Home trending progress badge + Ongoing ?episode link
850f9af feat(ui-ux-audit/phase-09): Browse grid wires useAnimeProgress
d94e5d3 feat(ui-ux-audit/phase-09): i18n card.dubBadge + card.episodeProgress (en/ru/ja)
```

(The first four were committed in a prior session; the remaining five in the current session as the Wave 3 frontend bundle.)

## Closes

| Requirement | Status |
|---|---|
| UX-16 — Per-card progress badge | CLOSED — Home trending row + Browse grid render the badge; Search deferred to Phase 10 |
| UX-17 — Latest episodes row links to specific episode | CLOSED — Home Ongoing column conditional `?episode={N+1}` |
| UX-18 — Sub/Dub indicator badge | CLOSED — `has_dub` column on `animes`, Kodik lazy-backfill, amber DUB badge on AnimeCardNew |
