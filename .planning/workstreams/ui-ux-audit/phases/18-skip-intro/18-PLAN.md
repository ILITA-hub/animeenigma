# Phase 18 Plan: Skip-Intro detection (Griffin)

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Scope: 1 new backend endpoint (catalog) + 1 gateway proxy + 1 composable + 2 player updates + 3 locale files. Closes UX-34.

## Tasks

### Wave 1 — Backend (catalog proxy + cache)

- [ ] Create `services/catalog/internal/handler/skip_times.go`:
  - `SkipTimesHandler` struct with `cache` + `http.Client`.
  - `Get(w, r)` method: parses `:malId` + `:episode` path params, checks cache (key `skip-times:{malId}:{ep}`), on miss does HTTP GET to `https://api.aniskip.com/v2/skip-times/{malId}/{ep}?types=op,ed`, caches 7d, returns JSON.
  - Gracefully handle upstream 404 / non-200: return empty `{ found: false, results: [] }`.
- [ ] Register route in `services/catalog/internal/transport/router.go`: `r.Get("/skip-times/{malId}/{episode}", skipTimesHandler.Get)` — public, no auth.
- [ ] Wire `SkipTimesHandler` in `services/catalog/cmd/catalog-api/main.go`.
- [ ] `go test ./...` clean.
- [ ] `make redeploy-catalog`.
- [ ] Smoke: `curl http://localhost:8000/api/skip-times/52614/1` (Frieren MAL ID 52614, ep 1) — returns aniskip JSON or empty `{found:false}` if not in their DB.

### Wave 2 — Gateway proxy

- [ ] In `services/gateway/internal/transport/router.go`, add public proxy route `/skip-times/*` → catalog. Register BEFORE any catch-all admin routes. Public, no JWT.
- [ ] `make redeploy-gateway`.

### Wave 3 — Frontend composable

- [ ] Create `frontend/web/src/composables/useSkipTimes.ts`:
  - Signature: `(malId: Ref<string | null>, episode: Ref<number>)`.
  - Returns `{ opening: Ref<{start, end} | null>, ending: Ref<{start, end} | null>, loading, error }`.
  - Watches inputs; on change, fetches `/api/skip-times/{malId}/{ep}` and parses `results` array, extracting `op` and `ed` entries.
  - Skips fetch when `malId` is null/empty.
- [ ] Add `animeApi.getSkipTimes(malId, ep)` to `frontend/web/src/api/client.ts`.

### Wave 4 — Player integration

- [ ] `frontend/web/src/components/player/HiAnimePlayer.vue`:
  - Import `useSkipTimes`. Pass `anime.mal_id` + `currentEpisode` refs.
  - Compute `showSkipIntro` = `currentTime >= opening?.start && currentTime < opening.end - 1`.
  - Add overlay button (`absolute bottom-20 right-4 px-4 py-2 bg-cyan-500 text-white rounded-lg shadow-lg z-10`) bound to `seekTo(opening.end)`.
  - Same for ending CTA (lower priority): button label `player.skipOutro`.
- [ ] `frontend/web/src/components/player/ConsumetPlayer.vue`: mirror the above changes.

### Wave 5 — i18n (en/ru/ja)

- [ ] Add 2 keys × 3 locales = 6 entries:
  - `player.skipIntro`: EN "Skip Intro" / RU "Пропустить опенинг" / JA "オープニングをスキップ"
  - `player.skipOutro`: EN "Skip Ending" / RU "Пропустить эндинг" / JA "エンディングをスキップ"

### Wave 6 — Verification

- [ ] `cd services/catalog && go test ./...` clean.
- [ ] `cd frontend/web && bunx vue-tsc --noEmit` clean.
- [ ] `bash scripts/i18n-lint.sh` clean.
- [ ] `make redeploy-{catalog,gateway,web}` succeed.
- [ ] grep `useSkipTimes` in HiAnimePlayer + ConsumetPlayer (2+ matches).
- [ ] grep `skip-times` in gateway router + catalog router + client.ts.
- [ ] curl `/api/skip-times/52614/1` returns JSON (200).
- [ ] Manual: load an anime with MAL ID through HiAnime/Consumet player, advance to opening start time, verify button appears, click → seeks past opening.

## Files touched

```
services/catalog/internal/handler/skip_times.go      (new)
services/catalog/internal/transport/router.go        (route)
services/catalog/cmd/catalog-api/main.go             (wire)
services/gateway/internal/transport/router.go        (proxy)
frontend/web/src/composables/useSkipTimes.ts         (new)
frontend/web/src/api/client.ts                       (getSkipTimes)
frontend/web/src/components/player/HiAnimePlayer.vue (overlay)
frontend/web/src/components/player/ConsumetPlayer.vue (overlay)
frontend/web/src/locales/en.json                     (+2)
frontend/web/src/locales/ru.json                     (+2)
frontend/web/src/locales/ja.json                     (+2)
.planning/workstreams/ui-ux-audit/phases/18-skip-intro/
  18-CONTEXT.md
  18-PLAN.md
  18-SUMMARY.md       (written at execute end)
  18-VERIFICATION.md  (written at execute end)
```

## Closes

| Req | Surface | Mechanism |
|---|---|---|
| UX-34 | HiAnime + Consumet players | Backend aniskip proxy + cache, useSkipTimes composable, in-player CTA |
