# Phase 14 Plan: Marketing-surface polish

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Scope: 1 new backend endpoint + 1 new view + 3 locale files. Closes UX-28, UX-29, UX-30.

## Tasks

### Wave 1 — Follower count (UX-28)

- [ ] Add `ListRepository.CountWatchers(ctx, animeID)` to `services/player/internal/repo/list.go`. Query: `SELECT COUNT(*) FROM anime_list WHERE anime_id = ? AND status = 'watching' AND deleted_at IS NULL`. Returns int64.
- [ ] Add `ListService.GetWatchersCount(ctx, animeID)` wrapper.
- [ ] Add `ListHandler.GetWatchersCount` — parses `:animeId` path, returns `{ count: int }`. Public, no auth required.
- [ ] Register route `GET /anime/{animeId}/watchers-count` (player service router).
- [ ] Add gateway proxy: `/api/anime/{animeId}/watchers-count` → player service. Goes in services/gateway/internal/transport/router.go alongside other public proxies. Public, no JWT required.
- [ ] Frontend: `userApi.getWatchersCount(animeId)` in `frontend/web/src/api/client.ts`.
- [ ] In `Anime.vue`, fetch watchers count after anime loads. Render badge near score when `count >= 5`:
  ```vue
  <Badge v-if="watchersCount >= 5" variant="default" class="flex items-center gap-1">
    <span aria-hidden="true">👥</span>
    {{ t('anime.watchersCount', { count: formatCount(watchersCount) }) }}
  </Badge>
  ```
- [ ] Helper `formatCount(n)`: uses `Intl.NumberFormat` with `notation: 'compact'` to render "1.2K".
- [ ] `cd services/player && go test ./...` clean.
- [ ] `make redeploy-player && make redeploy-gateway`.
- [ ] Smoke: `curl http://localhost:8000/api/anime/<some-id>/watchers-count` returns JSON `{ count: 0 }` (or non-zero if seeded).

### Wave 2 — Search placeholder clarity (UX-29)

- [ ] Update `search.placeholder` in en.json: `"Search: title or genre"`.
- [ ] Update `search.placeholder` in ru.json: `"Поиск: название или жанр"`.
- [ ] Update `search.placeholder` in ja.json: `"検索: タイトルまたはジャンル"`.
- [ ] No code changes — `SearchAutocomplete.vue` already binds `$t('search.placeholder')`.

### Wave 3 — About / FAQ view (UX-30)

- [ ] Create `frontend/web/src/views/About.vue`:
  - Header: `{{ t('about.title') }}` + `{{ t('about.subtitle') }}`.
  - 8 FAQ items rendered via `v-for` over a static array of `{ qKey, aKey }`. Each item is:
    ```html
    <details class="border-b border-white/10 py-3 group">
      <summary class="cursor-pointer text-lg font-medium text-white flex items-center justify-between">
        <span>{{ t(item.qKey) }}</span>
        <svg class="w-5 h-5 transition-transform group-open:rotate-180 text-white/40" .../>
      </summary>
      <p class="mt-2 text-white/70">{{ t(item.aKey) }}</p>
    </details>
    ```
  - Wrapper: `max-w-3xl mx-auto px-4 py-12`.
- [ ] Register route `/about` in `frontend/web/src/router/index.ts`. Component: `() => import('@/views/About.vue')`.
- [ ] Add About link to Footer.vue (or to Navbar if no Footer component exists — verify in execution).

### Wave 4 — i18n (en/ru/ja, 18 keys × 3 = 54 entries)

- [ ] In each locale file add:
  ```
  about.title
  about.subtitle
  about.faqs.q1.q, about.faqs.q1.a
  about.faqs.q2.q, about.faqs.q2.a
  ... through q8 ...
  anime.watchersCount (with {count} placeholder)
  ```

  Use the topics outlined in CONTEXT.md. Sample EN strings:
  - `about.title`: "About AnimeEnigma"
  - `about.subtitle`: "A self-hosted anime streaming platform"
  - `q1.q`: "What is AnimeEnigma?" / `q1.a`: "AnimeEnigma is a self-hosted anime streaming platform with Shikimori/MAL integration..."
  - `q2.q`: "Is it free? Are there ads?" / `q2.a`: "Yes, free and ad-free. Self-hosted by a small group..."
  - `q3.q`: "Where do the videos come from?" / `q3.a`: "AnimeEnigma proxies streams from Kodik, AnimeLib, HiAnime, and Consumet..."
  - `q4.q`: "How do recommendations work?" / `q4.a`: "We combine 5 signals: top-list similarity, genre affinity, global trending, rating boost, and seasonal freshness..."
  - `q5.q`: "Can I import my MAL list?" / `q5.a`: "Yes — go to your Profile → Settings → MAL Import..."
  - `q6.q`: "Is there a mobile app?" / `q6.a`: "Not yet — the web is fully mobile-responsive..."
  - `q7.q`: "How do I report a broken player?" / `q7.a`: "Each player has a Report button — your report goes to the admin Telegram..."
  - `q8.q`: "Who runs this site?" / `q8.a`: "A small self-hosted group. Contributions welcome via the project repo..."
  - `anime.watchersCount`: "{count} watching"

  Translate to RU and JA with equivalent meaning.

### Wave 5 — Verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` clean.
- [ ] `bash scripts/i18n-lint.sh` clean.
- [ ] `make redeploy-player && make redeploy-gateway && make redeploy-web` all succeed.
- [ ] grep checks:
  - `CountWatchers` in player repo
  - `watchers-count` in gateway router
  - `getWatchersCount` in client.ts + Anime.vue
  - `about.faqs.q1.q` in en/ru/ja
  - `/about` route in router
- [ ] Manual: curl watchers-count returns 0 or non-zero; load `/about` shows accordion; SearchAutocomplete placeholder reads "Search: title or genre" (EN).

## Files touched

```
services/player/internal/repo/list.go                (CountWatchers method)
services/player/internal/service/list.go             (GetWatchersCount)
services/player/internal/handler/list.go             (GetWatchersCount handler)
services/player/internal/transport/router.go         (public route)
services/gateway/internal/transport/router.go        (proxy route)
frontend/web/src/api/client.ts                       (getWatchersCount)
frontend/web/src/views/Anime.vue                     (watchers badge)
frontend/web/src/views/About.vue                     (new)
frontend/web/src/router/index.ts                     (/about route)
frontend/web/src/components/layout/Footer.vue        (About link, if Footer exists)
frontend/web/src/locales/en.json                     (+18 keys)
frontend/web/src/locales/ru.json                     (+18 keys)
frontend/web/src/locales/ja.json                     (+18 keys)
.planning/workstreams/ui-ux-audit/phases/14-marketing-polish/
  14-CONTEXT.md
  14-PLAN.md
  14-SUMMARY.md       (written at execute end)
  14-VERIFICATION.md  (written at execute end)
```

## Closes

| Req | Surface | Mechanism |
|---|---|---|
| UX-28 | Anime detail | Backend CountWatchers + frontend badge (hidden < 5) |
| UX-29 | All search inputs | Updated search.placeholder i18n (en/ru/ja) |
| UX-30 | /about route | New About.vue with native `<details>` FAQ + Footer link |
