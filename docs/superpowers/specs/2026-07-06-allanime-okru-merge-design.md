# Merge `okru` + `allanime` into one `allanime-okru` provider

**Date:** 2026-07-06
**Status:** Approved (design)
**Scope:** scraper service, catalog service, analytics probe, frontend player

---

## Problem

The scraper roster carries **two provider entries that are really the same AllAnime upstream**:

| | `allanime` provider | `okru` provider |
|---|---|---|
| DB status | **degraded** — out of auto-failover, hacker-mode-only | **enabled**, weight 35 |
| Discovery | AllAnime GraphQL (`api.allanime.day`) — works | *reuses `allanime`'s discovery internally* |
| Stream | **broken** — primary sources decode to `/apivtwo/clock` behind Cloudflare Turnstile, unsolvable from our egress | ok.ru "Ok" sources → `okcdn.ru` HLS — works, clock-free |

`okru` is literally an internal `allanime.Provider` (for discovery) + an ok.ru extractor (for streams). The standalone `allanime` provider can discover but can **not** produce a playable stream from our datacenter egress — its clock/probe stream path is dead code. Keeping it registered (even degraded/hacker-mode) is a trap: selecting it just fails.

This mirrors the existing `animejoy-sibnet` / `animejoy-allvideo` sibling pattern (two providers sharing one upstream discovery), which is the naming precedent for the merge.

## Goal

Collapse the two entries into **one provider** — id `allanime-okru`, label **"AllAnime (OK.ru)"** — that does exactly what today's `okru` does (AllAnime discovery → filter to `Ok` sources → resolve via ok.ru → `okcdn.ru` HLS). Drop AllAnime's dead clock/probe stream machinery. Along the way, **remove the frontend's per-provider coupling** so this rename — and every future EN provider add/rename — needs zero frontend edits.

## Non-goals

- No attempt to revive AllAnime's clock path (needs a residential proxy — separately deferred).
- No change to the `embeds/okru.go` ok.ru host extractor (it extracts the ok.ru *host*; that's not the provider identity — it stays named `okru`).
- No generalization of the frontend's stable single-id adapter branches (`kodik`, `ae`, `18anime`, `hanime`, `animejoy-*`). Only the churny EN-scraper set becomes feed-driven.

---

## Decisions (locked)

1. **Fold the package.** One Go package `allanimeokru` (dir `services/scraper/internal/providers/allanimeokru/`) containing AllAnime GraphQL discovery **+** ok.ru resolution. Both old dirs (`okru/`, `allanime/`) deleted. `Name()` returns `"allanime-okru"`.
2. **Chip label** = `"AllAnime (OK.ru)"` (capability-feed `display_name`).
3. **Tombstone** the old `allanime` DB row (status `disabled`, kept in `KnownProviders` — the animefever codeless-tombstone precedent), rather than deleting it.
4. **Frontend goes fully feed-driven** for EN-chain membership — the two hardcoded provider-id sets are deleted, replaced by a `family === "ourenglish"` check sourced from the capability feed.

---

## Design

### A. The package fold (scraper)

New package `allanimeokru` (Go pkg name `allanimeokru` — no hyphen; the hyphen lives only in `Name()`'s return value and the DB/wire id).

**Keep — discovery half (from `allanime/`):**
`FindID`, `ListEpisodes`, `fetchSources`, `EpisodeSourceURLs`, the GraphQL/APQ plumbing (`doGraphQL`, `buildSearchVariables`/`buildEpisodesVariables`/`buildSourcesVariables`, `buildExtensions`, SHA constants), `decrypt.go`, `dto.go`, `queries.go`, `cache.go`, `decodeSourceURL`, `splitEpisodeID`, `materializeEpisodes`, `categoriesFor`/`fetchShowDetail`.

**Keep — resolution half (from `okru/`):**
`isOk` filtering, ok.ru `ListServers` (Ok sources across sub+dub), ok.ru `GetStream` (Ok source → `embeds.OkruExtractor`), the 5-stage health snapshot.

**Drop — dead stream machinery (from `allanime/`):**
`classify` + `sourceprobe` usage, `orderCandidates`, `resolveSourceURL`, `streamType`, `streamGateBudget`/`maxStreamProbes`, `translationTypeFor` (only if unused after fold — `EpisodeSourceURLs` uses it, so keep), and AllAnime's own embed-skipping `GetStream`/`ListServers`/`materializeServers`. This path cannot produce a playable source from our egress; removing it **is** the "cleanup allanime."

**Collapse the cross-package seam:** `allanime.EpisodeSourceURLs` / `allanime.NamedSource` were exported only so `okru` could call across packages. After the fold they are same-package, so they may be unexported (`episodeSourceURLs` / `namedSource`) — implementation detail, minimize diff.

**Provider shape after fold:** one `Provider` struct holding the GraphQL cache/http + the ok.ru `extractor`, with `Name()="allanime-okru"`, `FindID`/`ListEpisodes` (discovery), `ListServers`/`GetStream` (Ok-only), `HealthCheck`. Essentially "today's `okru` with the discovery code inlined." Meaningfully less code than the two packages combined.

**Tests:** `allanime/client_test.go`, `allanime/queries_test.go`, `allanime/testdata/`, and `okru/client_test.go` move into `allanimeokru/`; the cross-package `allanime` import in the okru test collapses to same-package.

**Adapted-from comments:** `nineanime/{cache,client,doc}.go` reference `providers/allanime/` as their "base template" — update those path strings to `providers/allanimeokru/` (comments only).

### B. Registration + config (scraper)

- `cmd/scraper-api/main.go`:
  - Delete the `allAnimeProvider` construction + `registerByStatus(allAnimeProvider)` and the `allanime` import.
  - Point the surviving construction at `allanimeokru`; fold the discovery HTTP client's per-host RPS (`api.allanime.day`, `allmanga.to`) into the merged provider's `BaseHTTPClient` (`WithProvider("allanime-okru")`).
  - `candidateProviders` is **derived** from `p.Name()` (accumulator refactor), so `allanime-okru` flows in automatically and the removed `allanime` simply stops being appended — no hand-maintained count to update. The residual wiring-invariant (`got != want`) stays correct because it counts registered providers.
- `internal/config/providers.go` `KnownProviders`: remove `"okru"`, add `"allanime-okru"`, **keep `"allanime"`** (tombstone — the remote loader hard-fails on any `scraper_operated` name it doesn't recognize, and the tombstoned DB row is `scraper_operated`).

### C. DB roster + migration (catalog)

- `scraperprovider/seed.go` `defaultProviders`:
  - Rename the `okru` entry → `Name: "allanime-okru"` (keep `StatusEnabled`, weight 35, sub+dub); refresh `Reason`/`Description`.
  - Flip the `allanime` entry → `StatusDisabled` (tombstone); update `Reason`/`Description` to "folded into allanime-okru".
  - Update the `knownProviders` gate set (`"okru": true` → `"allanime-okru": true`; keep `"allanime": true`).
- `scraperprovider/migrate.go` — new guarded migration `AllanimeOkruMerge` (guard key `allanime_okru_merge`), wired into the same runner as `AllAnimeDegrade` / `AnimefeverDisable`. Two `UPDATE`s only (no source-pin migration — see below):
  1. `UPDATE stream_providers SET name='allanime-okru', reason=…, description=… WHERE name='okru'` (preserves status=enabled/weight/engine).
  2. `UPDATE stream_providers SET status='disabled', reason=… WHERE name='allanime'` (degraded → tombstone).
  - Idempotent + guard-gated (skip if guard row present), matching the existing migration idiom.
- `sourceranking/writer.go` `knownProviders`: replace `"okru"` → `"allanime-okru"`, keep `"allanime"`. This is a **Redis-only** allowlist for the public `srcfix:{animeID}` override key (24h TTL) — there is **no source-pin table**, so any stale `okru`/`allanime` pin self-expires within 24h; only the allowlist needs the new id so future pins can select it. (The `SYNC: keep in step with frontend providerRegistry.ts CURATED_TIER` comment there is already stale — no such FE registry exists — so leave the comment or trim it, don't chase it.)
- `capability/rank.go` `displayName`: remove `"okru"`, add `"allanime-okru": "AllAnime (OK.ru)"`, keep `"allanime"` (still a known tombstone name that can surface in admin roster views; the map only title-cases labels, so a stale entry is harmless either way).

### D. Playability probe (analytics)

- `analytics/internal/config/config.go` `PROBE_PROVIDERS` default: replace `okru` → `allanime-okru`; drop `allanime` (tombstoned — nothing to probe). Update `config_test.go` assertion.

### E. Frontend — backend-driven, zero per-provider coupling

Both hardcoded sets encode the same fact — *"is this an EN-scraper-chain provider?"* — which the capability feed already answers: the backend emits every EN provider under `family: "ourenglish"` (`catalog .../capability/service.go`).

- **Delete** `SCRAPER_IDS` (`useProviderResolver.ts`) and `EN_SCRAPER_IDS` (`comboMapping.ts`).
- **Propagate `family`** from `SourceFamily.family` onto `ProviderRow` in `useProviderFeed.rowsFromReport` (the feed already carries it).
- **Derive `familyOf(providerId)`** — a reactive `Map<provider, family>` built from the loaded `CapabilityReport` — and inject it into `ResolverDeps`. Single provider→family source, 100% backend-sourced.
- Resolver EN branch: `SCRAPER_IDS.has(provider)` → `familyOf(provider) === 'ourenglish'` → `scraperAdapter`.
- Combo persistence: `EN_SCRAPER_IDS.has(provider)` → `family === 'ourenglish'` → `'english'`.
- Stable single-id branches (`kodik`/`ae`/`18anime`/`hanime`/`animejoy-*`) untouched.
- Update `comboMapping.spec.ts` + `useProviderResolver.spec.ts` to inject a `familyOf` stub instead of asserting on the deleted sets.

**Edge cases:** stream resolution runs *after* the feed loads (a row was selected), so `familyOf` is populated. Degraded EN providers stay in the feed (marked `hacker_only`) under `family:"ourenglish"`, so hacker-mode picks still route. A provider absent from the feed (fully disabled) is unroutable — same `NotAvailableError` as today.

**Result:** the frontend no longer enumerates any EN provider id. `okru`→`allanime-okru`, and any future EN provider add/remove, needs no frontend change.

---

## Back-compat

- **Saved combos** persist only the coarse `player:'english'` (via `providerToLegacyPlayer`), not the granular provider id — unaffected by the rename; the resolver re-picks a live EN provider from the feed on restore.
- **Watch-Together room tokens** carry the granular id opaquely but rooms are ephemeral (15-min sliding TTL) — a room spanning the deploy is acceptable collateral, no persistent state to migrate.
- **Admin source-pins** (`srcfix`) are Redis keys with a 24h TTL, not persisted rows — a stale `okru`/`allanime` pin self-expires within a day. Only the writer allowlist gains the new id (step C); nothing to migrate.

## Deploy-order hazard

The scraper remote-config loader **hard-fails** on a `scraper_operated` roster name absent from `KnownProviders`. Because `"okru"` is removed from `KnownProviders`, the scraper must not boot against an un-migrated DB (which still says `okru`).

**Deploy order: catalog first** (runs `AllanimeOkruMerge` → roster shows `allanime-okru`), verify the roster, **then scraper.** Same class of hazard flagged in the original okru ship. Deploy from a clean `origin/main` worktree with `docker/.env`.

## Testing / verification

- `cd services/scraper && go build ./... && go vet ./... && go test ./... -count=1`
- `cd services/catalog && go test ./internal/service/scraperprovider/... ./internal/service/capability/... ./internal/service/sourceranking/... -count=1`
- `cd services/analytics && go test ./internal/config/... -count=1`
- Frontend: `bunx vitest run src/composables/aePlayer/comboMapping.spec.ts src/composables/aePlayer/useProviderResolver.spec.ts && bunx tsc --noEmit` + `/frontend-verify`.
- Live (post-deploy, catalog→scraper): `prefer=allanime-okru` → episodes → servers (OK.ru sub+dub) → stream 200 (`*.okcdn.ru/*.m3u8`, valid `#EXTM3U`). Capability feed shows chip "AllAnime (OK.ru)", no `okru`/`allanime` active rows. Probe surfaces `allanime-okru`.

## File-touch summary

**scraper:** `internal/providers/allanimeokru/*` (new, folded), delete `internal/providers/{okru,allanime}/`, `cmd/scraper-api/main.go`, `internal/config/providers.go`, `internal/providers/nineanime/*` (comment paths).
**catalog:** `internal/service/scraperprovider/{seed.go,migrate.go}`, `internal/service/sourceranking/writer.go`, `internal/service/capability/rank.go`.
**analytics:** `internal/config/{config.go,config_test.go}`.
**frontend:** `composables/aePlayer/{useProviderResolver.ts,comboMapping.ts,useProviderFeed.ts}`, `types/capabilities.ts`/`aePlayer.ts` (add `family` to `ProviderRow`), the two `.spec.ts`.
**docs:** `CLAUDE.md` (scraper failover chain + source-family table: `okru`→`allanime-okru`), `docs/scraper-framework.md`.
