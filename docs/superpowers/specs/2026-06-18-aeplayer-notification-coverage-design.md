# aePlayer new-episode notification coverage (anime-level detection + scraper sub/dub)

**Date:** 2026-06-18
**Status:** Design — phased milestone, approved for planning (Phase 1 first)
**Builds on:** `2026-06-18-notification-deeplink-aeplayer-provider-design.md` (deep-link provider/team), which shipped `2560cf52`.

## Problem

New-episode notifications fire only for combos with a non-empty `translation_id`
(`services/notifications/internal/job/hotcombos.go:59` —
`WHERE ... AND wh.translation_id != ''`). But **aePlayer persists
`translation_id: ''` for every provider** — `comboMapping.ts::comboToWatchCombo`
puts the team in `translation_title` and leaves `translation_id` empty
(confirmed: `useWatchTracking.ts` sends `c.translation_id`, AePlayer feeds it
`comboToWatchCombo(state.combo)`). So **every aePlayer watch is dropped from
notification detection**, and as the legacy per-player components retire in
favour of aePlayer, the feature trends toward total silence.

The legacy detector also only knows how to answer "latest available episode"
for `kodik`/`animelib` and *requires* a `translation_id`
(`internal_episodes.go:75`, `episodes_lookup.go` switch) — so even if a combo
got through the filter, the lookup would 400.

## Goal

aePlayer watchers receive new-episode notifications, with the detector resolving
"latest available episode" at the **anime level** (team-agnostic) for combos
whose `translation_id` is empty, across the player families
`english`, `ae`, `raw`, `kodik`, `animelib`. EN `english` detection is split by
**sub vs dub** using real per-category data surfaced from the scraper providers.

**Non-goals:** `hanime`/`18anime` (no episode-list capability, 18+ — see Decisions);
changing the legacy translation-specific path for `translation_id != ''` combos;
schema changes (the existing snapshot key already accommodates empty `translation_id`).

## Key facts (verified 2026-06-18)

- Snapshot table keys on `(anime_id, player, language, watch_type, translation_id)`
  (`domain/snapshot.go`). An empty `translation_id` naturally **collapses all
  teams into one row per `(anime, player, watch_type, language)`** — exactly the
  anime-level granularity we want. **No migration.**
- The detector's episode-check is `GET /internal/anime/{shikimoriId}/episodes?player=&translation_id=&watch_type=&language=`
  → `EpisodesLookupService.LatestAvailable(...)` → `latest_available_episode`.
- **gogoanime's `ListEpisodes` already fetches BOTH `/category/<slug>` (sub) and
  `/category/<slug>-dub`** and merges them (the `SubDubMerge` test), encoding
  category in the episode slug — then `domain.Episode` (`ID, Number, Title,
  IsFiller`) **discards the category**. The per-category signal exists upstream
  and is thrown away. Audio category is otherwise only known at the `Server`
  level (`domain.Server.Type`), post per-episode `GetServers` (too expensive for
  a periodic detector).
- Per-family anime-level latest-episode sources (all confirmed present):
  - `english` → `scraper.Client.GetEpisodes(malID, title, altTitles, prefer="")` → episode list.
  - `ae` → `RawResolver.GetLibraryEpisodes(animeID)` → `EpisodesResponse`.
  - `raw` → `RawResolver.GetEpisodes(animeID)` → `EpisodesResponse`.
  - `kodik` → `kodik.Client.GetTranslations(shikimoriID)` → `[]Translation{EpisodesCount}`; max across teams.
  - `animelib` → `animelib.Client.GetEpisodes(animelibID)` → `[]Episode{Number}`; max parsed number (animelibID via `animeRepo`, as `lookupAnimeLib` already does).
- `EpisodesLookupService` already depends on `catalogService *CatalogService`
  (which owns the scraper client + raw resolver) — the new resolvers reach the
  scraper/raw paths through it (or via newly injected narrow deps; plan decides).

## Decisions

- **hanime/18anime excluded.** No episode-list API (only `GetVideo(slug)`); 18+
  ongoing-episode notifications are low value. No regression — they get none today.
- **english split by sub/dub via real provider data**, not a coarse shared count.
  Phase 1 ships english **sub** using today's merged episode count (the merged
  list is sub-complete; sub is the reliable floor). Phase 2 adds the scraper
  category surface so **dub** gets its own accurate count. A provider that cannot
  determine dub reports sub-only → that anime simply yields no dub notification
  (graceful, never a false dub claim).
- **Anime-level semantics.** For empty-`translation_id` combos the notification
  means "a new episode of this anime is available on player-family X (in this
  category)". The deep-link carries `provider=<player>` + empty `team`; aePlayer
  pins `kodik`/`ae`/`raw`/`animelib` directly and auto-picks the best source for
  the coarse `english` (per the shipped `pickInitialProvider`).

## Architecture

### A. Scraper — surface per-episode category (Phase 2)

- `domain.Episode` gains `HasSub bool` and `HasDub bool` (JSON `has_sub`/`has_dub`).
  Default `HasSub=true, HasDub=false` when a provider cannot distinguish (safe:
  sub is the common case, dub is opt-in).
- Each provider's `ListEpisodes` populates the flags from data it already
  retrieves or can cheaply retrieve. **gogoanime first** (it already fetches the
  dub category page — tag merged episodes instead of discarding). Other providers
  populate where feasible, else default sub-only.
- `/scraper/episodes` response carries the flags. The catalog-side consumer
  derives `latest_sub = max(Number where HasSub)` and `latest_dub = max(Number
  where HasDub)`.

### B. Catalog — anime-level latest-episode resolver (Phases 1 & 3)

Extend `EpisodesLookupService.LatestAvailable`: when `translation_id == ""`,
dispatch to a per-player anime-level resolver instead of requiring an id.
Keep the existing translation-specific branch unchanged for `translation_id != ""`.

- Phase 1: `english` (sub = current merged count), `ae`, `raw`.
- Phase 2: `english` dub (consumes the new `has_dub` flags; `watch_type` selects
  sub vs dub max).
- Phase 3: `kodik` (max `EpisodesCount` over `GetTranslations`), `animelib`
  (max `Number` over `GetEpisodes`, animelibID via `animeRepo`).

Each resolver returns the same `EpisodesLookupResult{LatestAvailableEpisode,
TranslationTitle, CheckedAt}` and uses the existing 5-min Redis cache (key already
includes player + (empty) translation_id + watch_type).

### C. Detector plumbing (each phase enables its players)

- `internal_episodes.go`: allow the anime-level players; **drop the
  `translation_id required` 400** for them (still required for legacy
  kodik/animelib translation-specific combos — branch on empty id, not on player).
- `hotcombos.go`: filter becomes
  `WHERE ... AND (wh.translation_id != '' OR wh.player IN (<enabled players>))`.
  The `IN` list grows per phase (Phase 1: `english`,`ae`,`raw`; Phase 3 adds
  `kodik`,`animelib`). This keeps unsupported empty-id players (hanime) excluded
  so the detector never logs spurious parser-failures for them.

## Data flow (anime-level english/dub example)

```
detector → hotcombos (english/dub/en, translation_id='') admitted
        → LatestEpisode → catalog GET /internal/.../episodes?player=english&watch_type=dub
        → EpisodesLookupService: english resolver → scraper GetEpisodes (w/ has_dub flags)
        → latest_dub = max(Number where HasDub)
        → snapshot compare → fire notification (provider=english, team='', episode=N)
        → deep-link /anime/{id}/watch?provider=english&team=&episode=N
        → aePlayer mounts, auto-picks best EN source, lands on episode N
```

## Error handling

- Resolver upstream failure → `CodeUnavailable` (detector logs a parser-failure
  metric, skips the combo, retries next run) — same contract as today.
- "Anime not on this source" (e.g. `EpisodesResponse.Available=false`, empty
  scraper list) → `CodeNotFound` → detector skips silently (no notification, no
  failure metric), matching the existing not-found semantics.
- Provider can't determine dub → `latest_dub` absent → dub combo yields no
  notification that run (never a false positive).

## Testing

- **Scraper (Phase 2):** gogoanime `ListEpisodes` tags `HasSub`/`HasDub` from the
  sub + dub category pages (extend the existing `SubDubMerge` fixture pair, whose
  dub golden is currently a 404 — add a real dub golden). `domain.Episode`
  round-trip includes the flags. Default sub-only for a provider with no dub data.
- **Catalog (Phases 1/3):** `episodes_lookup` table tests per resolver — happy
  path max-episode, empty→NotFound, upstream-error→Unavailable, cache hit. Handler
  test: anime-level player accepted without `translation_id`; legacy
  kodik/animelib still require it.
- **Notifications (each phase):** `hotcombos` admits the enabled players with
  empty `translation_id` and still excludes the disabled ones (hanime); existing
  kodik/animelib-with-id behaviour unchanged.

## Phasing (one milestone, shipped incrementally)

- **Phase 1 — `english` (sub) + `ae` + `raw`.** No scraper changes. Delivers the
  core "aePlayer watchers get notifications" for the common path. Files:
  `episodes_lookup.go`, `internal_episodes.go`, `hotcombos.go` (+ DI wiring, tests).
- **Phase 2 — scraper sub/dub surface → `english` dub.** `domain.Episode` flags +
  gogoanime population (then other providers best-effort) + catalog consumes
  `has_dub`. Files: `services/scraper/internal/domain/provider.go`, provider
  `ListEpisodes`, scraper handler, catalog scraper client + `episodes_lookup`.
- **Phase 3 — `kodik` + `animelib` any-team.** New parser helpers
  (`kodik.LatestEpisodeAnyTranslation`, `animelib.LatestEpisodeAnyTeam`) +
  resolver cases + filter/handler entries.

Each phase is independently shippable and leaves the system correct.
