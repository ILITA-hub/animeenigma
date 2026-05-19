# 26-02 Verification — has_english column + Browse Filter

**Date:** 2026-05-19
**SCRAPER-HEAL-25** — Browse filter activation per CONTEXT.md D5.

## Backend

### Auto-migration of has_english column

```
$ docker compose exec -T postgres psql -U postgres -d animeenigma -c '\d animes' | grep -i has_
 has_video        | boolean                  |           |          | false
 has_dub          | boolean                  |           |          | false
 has_kodik        | boolean                  |           |          | false
 has_consumet     | boolean                  |           |          | false
 has_animelib     | boolean                  |           |          | false
 has_hianime      | boolean                  |           |          | false
 has_raw          | boolean                  |           |          | false
 has_english      | boolean                  |           |          | false
    "idx_animes_has_english" btree (has_english)
```

Column added with `default false` and indexed.

### Lazy backfill confirmed

```
$ curl -s 'http://localhost:8000/api/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623/scraper/episodes?provider=allanime' | head -c 100
{"success":true,"data":{"episodes":[{"id":"ReHMC7TQnch3C6z8j:1","number":1,"title":"Episode 1",...

$ docker compose exec -T postgres psql -U postgres -d animeenigma \
    -c "SELECT shikimori_id, has_english FROM animes WHERE shikimori_id='52991';"
 shikimori_id | has_english
--------------+-------------
 52991        | t
```

After a single curl, has_english flipped to `t` for Frieren.

### Filter narrows listing

```
$ curl -s 'http://localhost:8000/api/anime?providers=english&page_size=5' | jq '.data | length'
1

$ curl -s 'http://localhost:8000/api/anime?providers=english&page_size=5' | jq '.data[0].has_english'
true
```

Filter correctly returns only anime where has_english=true.

## Frontend

### Type-check + build

```
$ cd frontend/web && bunx tsc --noEmit
(exit 0)

$ cd frontend/web && bunx eslint src/composables/useBrowseFilters.ts src/components/browse/BrowseSidebar.vue
(exit 0)

$ bun run build
... gzip outputs ...
(exit 0)
```

### Locale files

```
$ grep '"english"' frontend/web/src/locales/en.json
"english": "English"

$ grep '"english"' frontend/web/src/locales/ru.json
"english": "Английский"

$ grep '"english"' frontend/web/src/locales/ja.json
"english": "英語"
```

All three locales have the key with appropriate translations.

## Acceptance criteria

- [x] HasEnglish field on Anime domain model
- [x] has_english column auto-migrated by GORM with `default false` + index
- [x] SetHasEnglish method on AnimeRepository
- [x] `english` entry in repo filter's colsByKey map
- [x] `english` in handler-level providers query-param whitelist
- [x] Opportunistic SetHasEnglish call from scraper-episodes resolver
      (success-only, gated on non-empty episodes payload, fire-and-forget)
- [x] Provider union type widened: `'kodik' | 'animelib' | 'english'`
- [x] PROVIDER_VALUES array includes 'english'
- [x] BrowseSidebar providerOptions has english row with emerald accent
- [x] en.json / ru.json / ja.json have browse.filters.provider.english
- [x] bun run build exits 0; bunx tsc --noEmit exits 0
- [x] make redeploy-catalog exits 0; column migrated; service healthy
- [x] make redeploy-web exits 0
- [x] After one curl to scraper episodes endpoint, has_english=true in DB
- [x] /api/anime?providers=english returns the now-tagged anime
