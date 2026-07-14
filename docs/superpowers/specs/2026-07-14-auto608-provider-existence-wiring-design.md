# AUTO-608 — Provider Existence Wiring (DB roster = universal key)

**Date:** 2026-07-14 · **Status:** Approved by owner · **Scope:** scraper, catalog, analytics, notifications, frontend, Grafana

**Metrics:** UXΔ = +1 (Better) · CDI = 0.12 * 34 · MVQ = Kraken 90%/85%

## Problem

The catalog Postgres table `stream_providers` (PK: `name`, the canonical provider id) is the
single source of truth for provider **state** (policy/health/status/traits) — but not for provider
**existence**. A 2026-07-14 audit (this spec's parent session) found ~10 hand-maintained
compile-time rosters across five services + the FE that must each be edited before a new DB row
does anything. Worst of them: `services/scraper/internal/config/providers.go` `KnownProviders`
fails **closed against the entire DB config** — one unknown `scraper_operated` row makes
`LoadProvidersRemote` error out and the scraper silently fall back to the bundled all-enabled
default, ignoring every DB policy/health edit.

## End state

`stream_providers.name` is the universal provider key. Adding/enabling a row makes the provider
exist everywhere automatically. The only hand-written artifact per provider is its actual code
(Go constructor / capability family builder / FE resolver adapter), registered in **one registry
per seam, keyed by DB name**. A row with no code at a seam becomes an observable **"unwired"**
state (Prometheus gauge + dashboard column) — never a config-load failure, never silently
dropped data.

## Design

### 1. Scraper — fail-open + constructor registry

- `providers_remote.go:103`: delete the unknown-name hard error. Unknown `scraper_operated` row
  → `log.Warnw` + accept the row into `ProvidersConfig` (traits and policy still apply).
- `cmd/scraper-api/main.go`: replace the literal per-provider constructor block with
  `var providerConstructors = map[string]func(...) provider` (gogoanime, animepahe,
  allanime-okru, miruro, nineanime, animekai, 18anime). Boot iterates the DB roster ordered by
  `preference_weight`, registers rows that have constructors, and exports
  `scraper_provider_unwired{provider}=1` for rows that don't.
- The compile-time `candidateProviders` order survives **only** as the catalog-unreachable
  fallback (unchanged semantics).
- `KnownProviders` stops gating remote rows entirely; it survives solely as the name list the
  bundled offline-default config is built from (delete it if the fallback builds without it).

### 2. Catalog — roster-driven capability assembly

- `service/capability/service.go` `buildFamilies`: replace the fixed call list with iteration
  over registered (`status <> 'disabled'`) non-EN DB rows, dispatched through
  `var familyBuilders = map[string]familyBuilderFunc` (`kodik-noads`→kodikFamily,
  `animelib`→animelibFamily, `hanime`→hanimeFamily, `animejoy-sibnet`/`animejoy-allvideo`→leg
  builders, `ae`→aeFamily, `18anime`→dbRowFamily-adult). **Default for an unknown row = the
  existing generic `dbRowFamily(row)`** — a brand-new row appears in `/capabilities` with
  trait-derived variants instead of vanishing. `kodik-iframe` is explicitly skipped (it is the
  Classic Kodik iframe surface, not an aePlayer capability).
- Family emit order: `preference_weight DESC, name ASC` from the rows (replaces the fixed slice
  order at service.go:147).
- EN family (`BuildENFamily`) unchanged — already DB-driven.
- New column `display_name` (`gorm:"size:64"`, seeded with today's pretty names, operator-
  editable). `rank.go`'s `displayName` map and the per-builder display-string literals read it;
  fallback = title-cased `name`.

### 3. Analytics — roster-fed whitelist + probe targets

- `handler/playertelemetry_whitelist.go`: replace the compile-time `knownProviders` map with a
  roster client fetching `GET {CATALOG_URL}/internal/scraper/providers` (envelope-decoded —
  `data.providers`, see ISS-032), 60s TTL cache, **last-good fallback** on fetch failure, and
  the embedded seed snapshot as the cold-start fallback. Synthetic ids (`offline`) stay
  code-side — they are player surfaces, not roster providers. The same roster feeds
  `service/playability.go` `inRoster`.
- Probe targets (`cmd/analytics-api/main.go` `build` map + `PROBE_PROVIDERS`): membership now
  comes from the DB probe-plan (`/internal/providers/probe-plan`, already DB-driven); the
  per-provider resolver `build` map stays as the irreducible code registry. `PROBE_PROVIDERS`
  env is demoted to an **optional filter** (unset ⇒ no filtering; the default provider list in
  `config.go:117` is deleted). A row scheduled by the plan but lacking a resolver → probe
  `provider_unwired` gauge, not silence.

### 4. Legacy player-name gates (namespace bridge)

`validPlayers` (`episodes_validate.go:60`), the anime-level `switch`
(`anime_level_episodes.go:66`), the `hotcombos.go:63` SQL `IN`-list, and FE
`comboMapping.providerToLegacyPlayer` all key on the legacy `watch_history.player` namespace
(`english`/`ourenglish`/`aeplayer`/`kodik`/…), not provider ids.

- Add columns to `stream_providers`:
  - `player_key` (`size:32`) — the legacy `watch_history.player` value this provider maps to
    (e.g. gogoanime→`english`, kodik-noads→`kodik`, ae→`ae`, hanime/18anime→`hanime`,
    animejoy-*→their own names). Seeded; empty ⇒ provider has no legacy-player identity.
  - `anime_level` (bool) — provider resolves episodes at anime level (english/ae/animejoy legs
    today). Drives `anime_level_episodes.go` and the hotcombos eligibility.
- `validPlayers` / anime-level gate: derive the allowed sets from the roster (TTL-cached
  in-process; catalog owns the DB so this is a repo read, not HTTP). Keep `aeplayer` +
  `ourenglish` as static compat aliases where they exist today.
- `hotcombos.go`: the literal `IN (...)` becomes
  `wh.player IN (SELECT player_key FROM stream_providers WHERE anime_level AND status <> 'disabled')`
  — verified: notifications' GORM connection is the shared `animeenigma` database, which holds
  `stream_providers`, so the subselect works directly (no HTTP hop).
- Capability feed exposes `player_key` per provider cap so the FE mapping is feed-driven.
- **Out of scope:** migrating stored `watch_history.player` data to provider ids. Compat is
  preserved; full namespace unification is a future effort.

### 5. Frontend

- Delete `UNAVAILABLE_PROVIDERS` (`useProviderResolver.ts:601`) — feed omission is the truth;
  if the DB re-activates animelib the FE must follow.
- `comboMapping.providerToLegacyPlayer`: read `player_key` from the capability feed (via
  `groupOfProvider`-style report lookup); the hardcoded switch survives only as fallback for
  feed-less contexts (offline synthetic).
- `playbackFailure.ts:38` + `AePlayer.vue:911`: `'ae'` literal → `group === 'firstparty'`
  (report already in scope) so a second first-party provider trips the ae_failed alert +
  `is_first_party` flag.
- `useOverrideTracker.ts`: emit the raw provider id instead of the 3-value legacy enum with
  `?? 'kodik'` default.
- Resolver `getAdapter` stays id-keyed (irreducible: FE code must exist per video tech). Safe
  because SourcePanel only offers feed rows; add a dev-console warn on unknown-id resolution
  attempts.

### 6. Observability — the "unwired" surface

- Each seam exports `provider_unwired{provider}` (0/1) via `libs/metrics` (⚠ use the shared
  registration helpers, NOT plain promauto — the auto-registration trap).
- Playback-health dashboard roster table gains a "Wired" column (join on the gauge).
- New warning-severity Grafana rule "Provider row unwired" → maintenance webhook, `for: 30m`
  (a row existing before its code deploys mid-rollout must not page instantly).

### Decisions (owner-approved 2026-07-14)

1. **kodik alias:** public/capability id stays `kodik`; the `kodik-noads`→`kodik` alias is
   owned in ONE documented catalog domain const (today's `capability/playability.go`
   `providerAliases` promoted to the domain package). No renames — saved combos, URLs, and
   deep links keep working.
2. **`player_key` + `anime_level` columns:** approved (see §4).
3. **Phasing:** P1 = §1–§3 + §6 (existence core) → P2 = §4 (player-gate namespace) →
   P3 = §5 FE + dashboard column. One worktree, three landings, each independently
   deployable + verifiable.

### Deliberately unchanged

- `intrinsicGroups` / `GroupOf` / `scraperOperatedNames` — security defense: group and
  scraper-operated are name-derived, never DB-trusted (18+/EN separation). A new scraper
  provider needs Go code anyway; touching the intrinsic maps rides the same commit as the
  constructor.
- Streaming per-CDN Referer/UA host rules (genuinely code-shaped).
- Gateway path routes (`/kodik/*`, `/animelib/*`) — path contracts, not roster.
- Browse filters (`useBrowseFilters.ts`) — map to catalog boolean columns, a different surface.
- AdminFeedback `PLAYER_TYPES` — historical report-file category tags.
- Grafana pinned legends — cosmetic follow-up.
- The per-provider "Kodik Player Unavailable" alert — separate liveness surface.

## Error handling

- Roster fetch failure (analytics): serve last-good; cold start with catalog down → embedded
  seed snapshot; never drop telemetry rows because the roster was unreachable.
- Unknown row at a seam: register nothing, export `provider_unwired`, log once per refresh
  cycle (not per request).
- Scraper remote-config: a malformed row (empty name) is still rejected row-wise; only the
  membership check is removed.

## Testing

- Unit: each registry's "row without code" path (skip + gauge, no error); fail-open remote
  loader with an unknown provider name; `dbRowFamily` default dispatch for a synthetic row;
  hotcombos subselect vs the literal list (golden equivalence on today's roster);
  comboMapping feed-driven `player_key` path incl. offline fallback; playbackFailure
  firstparty-group classification.
- Integration (testcontainers): seed + one synthetic extra row → `/capabilities` contains it
  (generic family), scraper remote load succeeds, analytics whitelist admits it.
- Live verify after each phase deploy: `curl /api/anime/{id}/capabilities`, scraper boot logs
  (`providers: N`), analytics whitelist metric, Grafana Wired column.
