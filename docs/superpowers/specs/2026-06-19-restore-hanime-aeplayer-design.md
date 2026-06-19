# Restore Hanime provider & wire it into aePlayer

**Date:** 2026-06-19
**Status:** Approved (owner)
**Related:** [[project_retire_all_players_except_aeplayer]], `2026-06-18-player-surface-retirement-scope-design.md`, [[project_smart_source_selection]], [[project_aeplayer_rename_and_deterministic_best]]

## Problem

On 2026-06-18 (Plan B of the player-surface retirement) the standalone `HanimePlayer.vue`
surface was deleted and the `hanime` stream-provider row was flipped to `status=disabled`
(via the `RetireHanimeAnimelib` run-once migration + the `seed.go` row). Hanime was **never**
wired into aePlayer — it has always sat in `UNAVAILABLE_PROVIDERS` in the resolver with a
"deferred: needs slug-based episode key" note, and carries a `staticDisabled` flag in the
provider registry.

The owner wants hanime restored as a **selectable 18+ source inside aePlayer**, alongside the
already-working `18anime` source.

## Current state (verified against origin/main + live system)

All supporting infrastructure is **already intact** — this is a faithful reversal plus one new
frontend adapter, not a rebuild:

- **Backend parser** `services/catalog/internal/parser/hanime/client.go` — `Search` + `GetVideo(slug)`, auth via `HANIME_EMAIL`/`HANIME_PASSWORD`.
- **Credentials configured** — `HANIME_EMAIL` / `HANIME_PASSWORD` present in `docker/.env` and the running catalog container.
- **Catalog handlers + routes** — `GetHanimeEpisodes` / `GetHanimeStream`; routes `GET /api/anime/{id}/hanime/episodes` and `GET /api/anime/{id}/hanime/stream?slug=...` (`router.go:184-185`).
- **HLS-proxy allowlist** — `hanime.tv`, `highwinds-cdn.com`, `htv-*`, `hydaelyn-*`, `zodiark-*` still allowlisted in `libs/videoutils/proxy.go`.
- **Frontend API client** — `hanimeApi.getEpisodes` / `hanimeApi.getStream(animeId, slug)` in `api/client.ts`.
- **Registry entry** — `hanime` exists in `providerRegistry.ts` (`group: 'adult'`, `content: ['hentai']`, `audios: ['dub']`, `langs: ['ru']`) but carries a `staticDisabled` block and is in `CURATED_TIER`.
- **Combo mapping** — `comboMapping.ts` already maps `hanime → 'hanime'` player kind. No change.

The supposed blocker ("episode key must be numeric") is **already solved**: `makeAnime18Adapter`
is slug-keyed (`key: ep.slug`; `resolveStream` uses `String(ep.key)` as the slug). It is the exact
template for the hanime adapter.

What actually keeps hanime off:
1. Roster row `status=disabled` (live DB confirms `hanime|disabled|adult|f`).
2. `'hanime'` in `UNAVAILABLE_PROVIDERS` in `useProviderResolver.ts` (no adapter wired).
3. `staticDisabled` on the registry `hanime` entry.

## Goals

1. Re-enable the `hanime` roster row (fresh **and** existing DBs).
2. Add a slug-keyed `makeHanimeAdapter` and wire it into the resolver dispatch.
3. Un-gate the registry so hanime is selectable — surfaced only for hentai titles (via existing `content: ['hentai']` gating), ranked after `18anime`.
4. Sort the aePlayer provider/source menu so **available sources float to the top**, with capability ranking as the tiebreak.
5. Verify a real hentai title returns episodes/streams **and** plays end-to-end in aePlayer.

## Non-goals (YAGNI)

- Do **not** restore the deleted `HanimePlayer.vue` standalone surface, its WatchTogether tab, or the `Anime.vue` provider tabs. Hanime lives **only** inside aePlayer.
- Do **not** re-enable `animelib` (owner asked for hanime only; it stays `disabled`).
- No new env vars, CDN entries, or HLS-proxy changes — all already present.

## Design

### Backend (catalog) — re-enable the roster row

Three coordinated edits so a fresh DB and the existing live DB both converge on `enabled`
(the seed is insert-if-absent, so it cannot update an already-present disabled row):

1. **`services/catalog/internal/service/scraperprovider/seed.go`** — flip the `hanime` row
   `StatusDisabled → StatusEnabled`; refresh `Reason`/`Description` to reflect "restored into
   aePlayer (2026-06-19)". Keep `group: adult`, `scraper_operated: false`, `QualityCeiling:
   "1080p"`. (For a fresh DB, seed now inserts it enabled.)

2. **`services/catalog/internal/service/scraperprovider/migrate.go`** — remove `"hanime"` from
   the `RetireHanimeAnimelib` name filter so it only disables `animelib`. This prevents a fresh
   DB from seeding hanime enabled and then immediately re-disabling it. The existing guard key
   stays (already applied on live DBs); only the affected name set narrows. Update the doc
   comment + `migrate_test.go` accordingly.

3. **New guarded migration `ReEnableHanime(db)`** in `migrate.go` — mirrors the
   `RetireHanimeAnimelib` pattern (run-once via the `catalog_migration_guards` ledger, new guard
   key e.g. `reenable_hanime`). Flips the **existing** live `hanime` row `status` back to
   `enabled` exactly once. This is what fixes production. Wire it into the catalog boot sequence
   in `cmd/catalog-api/main.go` right after the existing retire/backfill migrations. Co-locate a
   test in `migrate_test.go` (idempotent: second run is a no-op; respects an operator who later
   re-disables — guard prevents re-flip).

   Operator-override note: the guard means once `ReEnableHanime` has run, a future operator
   `status=disabled` edit is preserved (the migration won't re-flip it), matching the
   DB-authoritative roster philosophy.

`animelib` is untouched and stays disabled.

### Frontend (aePlayer) — add the adapter

4. **`frontend/web/src/composables/aePlayer/useProviderResolver.ts`**
   - Import `hanimeApi` from `@/api/client`.
   - Add `hanimeApi?: typeof hanimeApi` to `ResolverDeps`.
   - Add `makeHanimeAdapter(api)` — a clone of `makeAnime18Adapter`:
     - `listEpisodes(animeId)` → `hanimeApi.getEpisodes(animeId)`, map each episode to
       `{ key: ep.slug, label: <number/name>, number: <number> }` (slug-keyed). Handle the
       `{success,data}` envelope (`response.data?.data || response.data`).
     - `resolveStream(animeId, ep)` → `slug = String(ep.key)`; `hanimeApi.getStream(animeId, slug)`;
       throw `NotAvailableError('hanime', 'returned no stream URL')` when empty; wrap the URL via
       the same `buildProxyUrl(url, referer ?? '', type)` path the 18anime adapter uses
       (`type = is_hls ? 'hls' : 'mp4'`). Adapt field names to the hanime stream payload
       (`HanimeStream.Sources[].URL` — pick the best quality; the backend response shape is
       resolved during planning against `domain.HanimeStream`).
   - Add a dispatch case `if (provider === 'hanime') { … return makeHanimeAdapter(deps.hanimeApi) }`
     with the same missing-dep guard as the others.
   - **Remove `'hanime'` from `UNAVAILABLE_PROVIDERS`.**
   - Pass `hanimeApi` into the default `makeResolver({ scraperApi, rawApi, anime18Api, kodikApi, aeApi, hanimeApi })` (line ~540).
   - Update the stale top-of-file doc comment (drop the "deferred" hanime note; document the new adapter like 18anime).

5. **`frontend/web/src/components/player/aePlayer/providerRegistry.ts`** — remove the
   `staticDisabled` block from the `hanime` entry so it becomes selectable. Keep `group: 'adult'`,
   `content: ['hentai']` (auto-appears only for hentai titles) and its existing `CURATED_TIER`
   position (after `18anime`).

6. **Provider-menu sort — available on top** (`SourcePanel.vue`, `sortedRows` computed,
   ~line 231). Today `sortedRows` orders purely by `rankedIds` position, so in the full list
   (hacker mode / expanded) a `disabled`/`irrelevant`/`down` row can sit above an `active` one.
   Change `sortedRows` to a stable two-key sort:
   - **Primary key:** availability bucket derived from `ProviderRow.state` (`ChipState`):
     available states first (`active`, then `degraded`), then the rest (`wip`, `down`,
     `disabled`, `irrelevant`) — define an explicit rank map so ordering is deterministic.
   - **Secondary key:** the existing `rankedIds` position (capability ranking; unranked →
     `MAX_SAFE_INTEGER`, which already falls back to curated/registry order upstream via
     `rankedProviderIds`).
   This floats every available provider above every unavailable one while preserving the
   capability ranking within each bucket. `activeRows`/`activeCount`/`topRow`/`visibleRows`
   already filter on `state === 'active'`, so the collapsed default view is unaffected; the
   change is visible in the full/expanded list. No registry or backend change for this item.

7. **Adapter + sort unit tests** — extend `useProviderResolver.spec.ts` with hanime cases
   (listEpisodes maps slug→key; resolveStream calls `getStream` with the slug and wraps via the
   proxy; missing-dep + empty-URL throw `NotAvailableError`). Extend `SourcePanel.spec.ts` (or add
   a focused spec) to assert available rows sort above unavailable rows with ranking preserved
   inside a bucket.

## Data flow (hanime, after restore)

```
aePlayer SourcePanel  ── user picks "Hanime" (shown only for hentai titles) ──▶
  useProviderResolver.getAdapter('hanime') ─▶ makeHanimeAdapter(hanimeApi)
    listEpisodes → GET /api/anime/{id}/hanime/episodes      (catalog → hanime parser)
    resolveStream(slug) → GET /api/anime/{id}/hanime/stream?slug=…
      → stream URL wrapped in buildProxyUrl(url, referer, hls|mp4)
      → streaming HLS proxy (CORS + Referer; hanime CDNs already allowlisted)
      → <video> / hls.js in aePlayer
```

## Verification (owner chose: wire + verify live playback)

1. **Backend** — against a running catalog, request `/api/anime/{hentai-uuid}/hanime/episodes`
   and `/hanime/stream?slug=…` for a known hentai title; confirm real episodes + a stream URL
   are returned (proves creds + parser + CDN path live).
2. **Build gates** — `cd services/catalog && go test ./internal/service/scraperprovider/... -race`;
   `cd frontend/web && bunx vitest run` (resolver + SourcePanel specs) + `bunx vue-tsc --noEmit` +
   `i18n-lint.sh` + `design-system-lint.sh`.
3. **Frontend playback** — after deploy, a Chrome smoke on a hentai title in aePlayer: confirm
   the `Hanime` source appears in the menu (and sorts among the available rows), selects, and the
   video actually plays through the HLS proxy. Per the opt-in policy this is offered at verify
   time, not assumed.

## Risks / notes

- **Stale shared tree.** Local `/data/animeenigma` is behind origin/main and churned by other
  agents — all work happens in a clean origin/main worktree; deploy by building from a clean
  worktree (copy `docker/.env`; node_modules symlinked for the FE gates). [[feedback_deploy_from_clean_worktree]]
- aePlayer is `useI18n`-free (template `$t` only) — no composable i18n added here, so the
  `AePlayer.room.spec` i18n-mock hazard does not apply. [[project_wt_inplayer_button_wired]]
- The change touches DS-lint-scanned `.vue`/`.ts` files but adds no colors/spacing/weights;
  `vue-tsc` type-checks the new adapter.
- Hanime upstream is auth-gated; if a future credential expiry breaks it, the roster row can be
  flipped back to `degraded`/`disabled` by an operator without code changes (DB-authoritative).
- No changelog/i18n-key additions expected (no new user-facing strings — registry `name` and
  existing chip states already localized). Confirm during planning.
```
