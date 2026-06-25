# Provider Single Source of Truth — Design

**Date:** 2026-06-25
**Status:** Approved (brainstorming) — pending spec review
**Branch:** `worktree-feat+provider-sot`

## Problem

The frontend keeps its own predefined provider brain. `frontend/web/src/components/player/aePlayer/providerRegistry.ts` hardcodes a `PROVIDER_REGISTRY` of 13 providers (id, label, accent hue, group, audio/lang/content, blurb, and a `staticDisabled` flag) plus a `CURATED_TIER` ordering. `useProviderHealth.ts` builds the player's Source list from that registry and only *overlays* live up/down from `/scraper/health`.

Consequences (the observed "data disagreement"):

- The FE **never gates on the DB `policy` column**. It checks `enabled`/`status`/`up` from the health snapshot. So `policy=disabled` in `stream_providers` does **not** guarantee a provider is unusable on the FE. AnimePahe (`policy=disabled`) can still surface; the dashboard (which faithfully mirrors the DB) and the player disagree.
- Provider *set*, *ordering*, and *some disable decisions* live in FE code, not the DB.
- Per-provider brand hues are an FE concern that has nothing to do with a provider's actual usability.

The `stream_providers` table is already the authoritative state (written by probe jobs + the AI recovery operator), and a per-anime, gateway-public, DB-backed endpoint already exists (`GET /api/anime/{id}/capabilities`). The FE simply doesn't treat the backend as the single authority.

## Goal

Make the backend the **single source of truth** for which providers the player may use and how it presents them. The FE becomes a dumb renderer of one anime-specific backend feed. Specifically:

- **`stream_providers` DB = the only authority.** Nothing else decides availability/selectability/order.
- The FE has **no predefined provider knowledge** — `providerRegistry.ts`, `CURATED_TIER`, and `staticDisabled` are deleted.
- **Cosmetics are status-driven, not identity-driven.** A provider's color/treatment comes from its *state*, which the backend derives from `(policy, health)`. No per-provider hue table on the FE.
- **`policy=disabled` ⇒ the provider is not emitted by the backend at all** — it cannot be selected because the FE never learns it exists.

## Non-Goals

- Reworking the scraper orchestrator's failover logic (its `status`-based gate is unchanged).
- Changing how probe jobs / the AI operator *write* the DB (they remain the writers).
- A global provider catalog endpoint — the feed is **anime-specific only** (it must query the library for "does AnimeEnigma have this title," and per-anime caches accumulate there).
- Per-provider brand colors in the player (deliberately removed).

## Backend-Enforced Visibility Rules

These are computed server-side and are the contract the FE obeys verbatim:

| DB state | Emitted by BE? | FE rendering |
|---|---|---|
| `status=disabled` (policy=disabled) | **No — omitted entirely** | Provider does not exist to the FE |
| `status=degraded` (policy=manual) | Yes, `hacker_only=true` | Visible & selectable **only in hacker mode** |
| `status=enabled`, up/recovering, **no episodes for this title** | Yes, `state=no_content` | **Tinted + on-hover tooltip**, not selectable |
| `status=enabled`, `health=recovering` | Yes | `recovering` (lime), selectable |
| `status=enabled`, otherwise | Yes | `active`, selectable |

Notes:
- `health=down` while `status=enabled` (e.g. nineanime, okru today): the provider stays `active`/selectable because `status=enabled` keeps it in the auto chain (and probe health can be wrong — see the gogoanime "up but verified-playing" case). If the user actually probes it and it returns empty servers, the reactive `no_content` rule tints it. This keeps presentation consistent with the user's reactive philosophy.
- `no_content` is **empty-servers only** ("provider has no episodes for this title"). Transient resolve *errors* are not cached — failover continues as today.

## Architecture

### Component 1 — The authoritative anime-specific feed (extend `/capabilities`)

Extend the existing per-anime public endpoint `GET /api/anime/{id}/capabilities` (catalog: `services/catalog/internal/handler/capabilities.go`, `service/capability/`) to be the single feed the FE renders. Server-side it aggregates three inputs and emits a fully-computed provider view:

**Inputs**
1. `stream_providers` DB authority (policy / health / derived `status` via `WireStatus()` / `group` / `preference_weight` / `supports_*` / `reason`). Source: `services/catalog/internal/domain/scraper_provider.go`, seeded in `service/scraperprovider/seed.go`. **`status=disabled` rows are filtered out.**
2. **Library presence** for the first-party `ae` provider — a *cheap* lookup ("does AnimeEnigma have this anime self-hosted in MinIO/library?"). Moves today's FE-side `smartDefault.ts` library check into the backend. Library presence is **not** an expensive scraper resolve, so it is checked live (briefly cached) per request. Absent → `ae` is `no_content` (tinted), present → `active`.
3. The **per-anime `no_content` cache** (Component 2) for expensive scraper providers.

**Output — per-provider view** (extends `domain.ProviderCap` in `services/catalog/internal/domain/capability.go`):

```jsonc
{
  "provider": "gogoanime",
  "label": "Gogoanime",          // BE-owned display name (existing DisplayName source)
  "group": "en",                 // en | ru | adult | jp | firstparty (from DB column)
  "state": "active",             // active | recovering | degraded | no_content
  "selectable": true,
  "hacker_only": false,          // true only for degraded
  "order": 85,                   // preference_weight; FE sorts desc
  "audios": ["sub", "dub"],      // from supports_sub/dub/raw
  "health": "up",                // raw signal, carried for nuance/debug
  "reason": ""                   // BE "why" for non-active states → tooltip text
}
```

Providers are grouped by `group` (the existing family grouping). `state`, `selectable`, and `hacker_only` are computed entirely server-side per the visibility table above.

### Component 2 — The reactive `no_content` cache

- **Populate (reactive):** the existing `/scraper/servers` resolve **is** the user-side probe. When it returns an empty server list for `(anime, provider)`, the backend records `no_content` for that pair. The scraper orchestrator (`services/scraper/internal/service/orchestrator.go`) already distinguishes empty-servers from errors; on empty it signals catalog to cache the pair.
- **Store:** Redis, key e.g. `provcontent:{anime_uuid}:{provider}`, **TTL = 1 hour**. Catalog owns the cache (it owns the feed).
- **Read:** the feed (Component 1) reflects cached `no_content` as the tinted state on subsequent loads, so a provider that came back empty stays tinted for up to an hour without re-probing.
- **Purge:** see Component 3.

### Component 3 — Notifications purge hook

- The notifications new-episode detector (`services/notifications/`, which already calls catalog's `/internal/anime/{id}/episodes`) gains a hook: when it discovers **new episodes** for an anime, it calls a new catalog internal endpoint to purge that anime's `no_content` cache.
- New internal (Docker-network-only, **not** gateway-proxied) endpoint on catalog: `POST /internal/providers/cache/purge` with `{ anime_id, reason }`.
- On purge, catalog deletes `provcontent:{anime_uuid}:*` and emits the resurrection metric/event (Component 4).
- Rationale: a title that gains episodes must re-light its providers instead of staying tinted for the rest of the hour.

### Component 4 — "Anime resurrections" dashboard

- Every `no_content` purge caused by the notifications probe finding new content emits:
  - A Prometheus counter `anime_content_resurrection_total` (label: `trigger` = `new_episodes` | `library_add`; **no `anime_id` label** — cardinality).
  - An analytics event (catalog → `ANALYTICS_INTERNAL_URL` `/internal/effects`, or a dedicated `analytics.events` row) carrying `anime_id`, `name`, `trigger`, `timestamp` for the list panel.
- A **new Grafana dashboard** `docker/grafana/dashboards/anime-resurrections.json`:
  - Timeseries (Prometheus): resurrection rate over time.
  - Table (ClickHouse datasource, `format: 1`): recently resurrected / newly-available anime (name, trigger, when).
  - Reachable at `/admin/grafana/d/anime-resurrections`.

## Data Flow

```
Player opens anime
   │
   ▼
GET /api/anime/{id}/capabilities   ◄── single authority
   │   catalog aggregates:
   │     • stream_providers (omit disabled, derive state/selectable/hacker_only/order)
   │     • library presence  → ae state
   │     • Redis no_content cache → tinted providers
   ▼
FE SourcePanel renders verbatim (color by state, section by group, order by `order`,
   degraded shown only in hacker mode). No registry, no FE availability logic.
   │
   ▼  user (or prefetch) selects/probes a provider
GET /api/anime/{id}/scraper/servers?...=  ── empty? ──► catalog caches
   │                                                     provcontent:{id}:{prov}=1 (TTL 1h)
   │                                                     → next feed load tints it
   ▼
(meanwhile) notifications detector finds new episodes for {id}
   │
   ▼
POST /internal/providers/cache/purge {id, reason:new_episodes}
   │   catalog: DEL provcontent:{id}:*  + anime_content_resurrection_total++  + analytics event
   ▼
Grafana "anime-resurrections" dashboard
```

## Affected Code (inventory)

**Delete / gut (FE):**
- `frontend/web/src/components/player/aePlayer/providerRegistry.ts` — deleted (registry, `CURATED_TIER`, `staticDisabled`).
- `frontend/web/src/composables/aePlayer/useProviderHealth.ts` — rewritten to consume the feed (no registry merge).
- `frontend/web/src/composables/aePlayer/smartDefault.ts` — library check removed (now backend).
- `frontend/web/src/components/player/aePlayer/SourcePanel.vue`, `ProviderChip.vue` — render by backend `state`/`group`/`order`; color from DS semantic tokens keyed on `state`, not per-provider hue.
- `frontend/web/src/types/aePlayer.ts` — `ProviderDef` shrinks/retires; new feed type added.

**Backend:**
- `services/catalog/internal/handler/capabilities.go`, `domain/capability.go`, `service/capability/*` — extend feed (state/selectable/hacker_only/order, omit disabled, library presence, no_content overlay).
- `services/catalog/internal/handler/` — new `POST /internal/providers/cache/purge` (Docker-only).
- catalog Redis client — `provcontent:` keys, 1h TTL.
- `services/scraper/internal/service/orchestrator.go` — on empty servers, signal catalog to cache no_content.
- `services/notifications/` — detector calls the purge endpoint on new-episode discovery.
- Metrics: catalog `anime_content_resurrection_total` + analytics event.
- `docker/grafana/dashboards/anime-resurrections.json` — new dashboard.

## Error Handling

- **Feed degradation:** if the no_content cache (Redis) is unreachable, fail open — treat providers as non-tinted (selectable). A Redis blip must never hide working providers. (Mirrors the gateway rate-limiter fail-open convention.)
- **Library check failure:** if the library lookup errors/times out, `ae` falls back to `no_content` (tinted) rather than crashing the feed.
- **Purge endpoint:** best-effort; a failed purge logs WARN and does not fail the notifications detector. The 1h TTL is the backstop.
- **Empty-vs-error:** only empty-servers caches `no_content`. Resolve errors/timeouts are transient and never cached.

## Testing

- **Catalog feed (Go):** table tests over `(policy, health, no_content?, library_present?)` → expected `{state, selectable, hacker_only, emitted?}`. Assert `status=disabled` is omitted; `degraded` ⇒ `hacker_only`; empty no_content ⇒ tinted; library absent ⇒ `ae` tinted. Handwritten fakes (no testify/mock), per project convention.
- **No_content cache (Go):** populate on empty servers, read-back tints, TTL semantics, purge clears.
- **Purge hook (Go):** notifications detector → purge endpoint → cache cleared + metric incremented (fake Redis + metric registry).
- **FE (Vitest):** SourcePanel renders feed verbatim — disabled never appears, degraded hidden unless hacker mode, no_content tinted+unselectable, ordering by `order`, color by `state`. Delete registry-coupled tests; add feed-driven tests.
- **i18n:** any new tooltip/blurb strings added to `en.json` + `ru.json` + `ja.json` (3-locale gate).
- **Dashboard:** JSON validates; CH panel uses `format: 1`.
- `/frontend-verify` (DS-lint + i18n parity + real `bun run build`) before finishing FE.

## Phasing

One coherent feature, four sequenced phases:

1. **Phase 1 — Authoritative feed + FE dumb-render.** Extend `/capabilities` (state/selectable/hacker_only/order, omit disabled, library presence). Delete the FE registry; render from the feed. *This alone closes the original "data disagreement."*
2. **Phase 2 — Reactive `no_content` cache.** Empty-servers → Redis (1h TTL) → tinted state in the feed.
3. **Phase 3 — Notifications purge hook.** New-episode discovery purges the anime's cache (internal endpoint).
4. **Phase 4 — Resurrection metrics + Grafana dashboard.**

## Effort & Impact (per `.planning/CONVENTIONS.md`)

- **UXΔ = +3 (Better)** — the player and the ops dashboard finally agree; disabled providers truly vanish, degraded is honestly hacker-gated, dead-for-this-title providers are explained, not silently failed.
- **CDI = 0.06 × 34** — moderate spread (FE player + catalog + scraper + notifications + Grafana) × moderate shift (FE loses its provider brain; one feed becomes load-bearing) × Effort_Fib 34 (four phases, cross-service). Not pre-multiplied.
- **MVQ = Griffin 85% / 80%** — disciplined consolidation onto an existing endpoint; slop-resistance from clear per-state contract + table-driven tests.

## Open Questions Resolved

- **Library = first-party `ae`/self-hosted MinIO content.** Absent ⇒ `ae` tinted (`no_content`), not omitted.
- **Resurrection event = purge-due-to-probe (new episodes) + library-add.** Counted on the dashboard; `anime_id` lives in the analytics event, not the Prometheus label.
- **Endpoint = extend `/capabilities`** (anime-specific), not a new endpoint and not a global catalog.
- **No-content detection = reactive (FE-side probe via existing `/scraper/servers`) → cached server-side 1h → purged by notifications rediscovery.**
