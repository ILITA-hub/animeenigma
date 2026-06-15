# Smart Source Selection — Design Spec

**Date:** 2026-06-15
**Status:** Approved design → ready for implementation planning
**Topic:** Pick the correct/best-working provider in the AnimeEnigma unified player, learn provider reliability per anime, fall back gracefully, and expose granular per-provider options (e.g. Kodik dub team).

---

## Problem

The unified player picks the default provider by taking the **first `active` row in registry array order** (`UnifiedPlayer.vue:383`). That order is hardcoded and has no notion of "better" or "actually working":

- First-party `ae` is first in the registry and is *always* `active` when relevant — even with **zero episodes** in the library — so the default frequently lands on an empty provider for titles we haven't encoded yet.
- Only **scraper** providers report live health; non-scraper providers (`kodik`, `raw`, `ae`, `18anime`, `animelib`, `hanime`) have **no health/reliability signal at all**.
- There is **no learning** about which provider works for which anime, and **no client-side fallback** when a chosen provider fails (only the scraper microservice fails over internally, among scraper providers only).
- Granular provider options the backend already supports are **hidden**: the Kodik dub-team picker (`Combo.team` ↔ `translation_id/title`) exists end-to-end but `SourcePanel` never gets the team list, so the unified player **regressed** vs the legacy `KodikPlayer`.

## Goals

1. **Smart default**: on load, select the best *available and working* provider instead of first-in-array — first-party preferred, health-aware, reliability-ranked, and **never landing on an empty provider**.
2. **Learned reliability**: accumulate real per-`(anime, provider)` success/failure + lag telemetry and compute a **daily** ranking ("provider top") that improves over time.
3. **Graceful fallback**: when a provider fails, fall back to the next-best **client-side**, emit telemetry, and (only when the current best fails) remember the working one for the rest of the day.
4. **Granular options**: restore the hidden per-provider choices — Kodik dub team, scraper servers, sub/dub variants.
5. **Respect the user**: a manually-chosen source is never silently overridden; if it's dead, say so gently.

## Non-Goals

- **Do NOT change the watch-combo resolution logic** (`/preferences/resolve`, strict-fallback tiers, pinned translations). It is reused, not modified.
- **Do NOT use Redis as the telemetry accumulation store.** Telemetry accumulates in **ClickHouse**; the daily-computed artifact and the same-day override are the *only* Redis usage.
- No CDN work (self-hosted target). No pre-population. No backend "resolve-best" unification (rejected — collapses the granular UI and fights the don't-override rule).

---

## Architecture

### Layering — the linchpin

The new reliability ranking is an **inner provider-selection layer beneath the existing watch-combo**, never a replacement:

```
WatchCombo  (UNTOUCHED — /preferences/resolve)      outer: player + language + watch_type + team(translation_id)
      └── Provider reliability layer  (NEW)          inner: which gogoanime/allanime/kodik/ae… + which server
```

`/preferences/resolve` keeps resolving the coarse combo (its strict-fallback tiers + pinned translations are untouched). The new layer only decides the **granular provider/server underneath** and validates the stream.

**Watch-combo integration (decided):** the unified player will be **wired into `/preferences/resolve`** (today only the legacy per-player components use it) so old + new players share one combo system. This requires:
- **Additive** extension of the `WatchCombo.player` enum (`'kodik'|'animelib'|'hanime'|'english'`) with `'raw'` and `'ae'`. Additive only — the backend resolver ignores unknown fields and tiers are data, so **no resolver-logic change**.
- A unified-group → `WatchCombo.player` mapping (e.g. EN scraper chain → `english`; `kodik` → `kodik`; `raw` → `raw`; `ae` → `ae`; `18anime`/`hanime` → adult).
- The granular provider id (gogoanime/allanime/…) lives **beneath** the coarse `player` value and is owned by the unified player's reliability layer.

### Three independent loops

```
PLAYBACK (per watch)                 LEARNING (once/day)                 DECISION (per load)
─────────────────────                ───────────────────                ───────────────────
player resolves a stream  ─emit─►  analytics.events (ClickHouse)       player fetches source payload
  player_resolve {provider,             │                               from catalog:
   anime_id, outcome,            scheduler daily cron ─►               { firstParty{available,count},
   reached_playback,             analytics /internal/                    dailyTop[], curatedTier[],
   latency_ms, error_kind}        player-ranking/recompute               override? }
  player_stall {provider,         → aggregate ClickHouse                       │
   anime_id, stall_ms, ...}       → publish to Redis (48h)             decision flow (§ Decision)
                                          │                                    │
  on failure → client-side ───────────────┘                            picks granular provider,
  fallback + emit + (if best       (Redis = computed artifact only,     validated via watch-combo
  was broken) write srcfix          NOT raw accumulation)               ("OK pass")
  override (24h) ◄──────────────────────────────────────────────────────────┘
```

---

## Component 1 — Telemetry (ClickHouse)

New public FE beacon `POST /api/analytics/player-events` (mirrors `services/analytics/internal/handler/clienterror.go`; sent via `sendBeacon`/`fetch keepalive`, with dedup + rate-cap like `feErrorLog.ts`). Lands in the existing `analytics.events` wide table via new `effect_kind` values, with the provider carried as `target` (`target_kind='provider'`) and `anime_id` populated — **no schema rewrite** (optionally a few nullable columns for stall measures).

Two event kinds:

- **`player_resolve`** — emitted on:
  - **success**: fired *once* when playback passes the first frame (`currentTime > 0`) → the "played past 0:00" signal.
  - **failure**: `NotAvailableError`, no-servers, empty episode list, or hls.js fatal.
  - Fields: `provider, anime_id, episode, outcome('ok'|'fail'), reached_playback(bool), latency_ms(resolve time), error_kind, audio, lang, player(coarse)`.

- **`player_stall`** — the new lag signal: while playing, when the playhead advances but buffered-ahead depletes (a `waiting`/stall fires on the `<video>` / hls.js). Fields: `provider, anime_id, episode, stall_count, stall_ms(total), position_pct, level_bitrate`.
  - Lag has three possible causes — bad user internet, slow provider/CDN, or our own bandwidth limit — and we **cannot** cleanly separate them. We collect it **raw** and use it only as a **soft** ranking input for now; the ambiguity is documented and revisited later.

Because the unified resolver routes **every** provider (Kodik/Raw/ae/18anime/EN-chain) through one path, this single beacon yields uniform reliability + lag telemetry across all providers at once — filling the "non-scraper providers have no signal" gap with one mechanism.

The existing scraper Prometheus metrics (`parser_requests_total`, `parser_fallback_total`, `parser_request_duration_seconds`) remain as a **secondary corroborating signal**.

## Component 2 — Widen the "Playback / Health" Grafana dashboard

The dashboard exists (`docker/grafana/dashboards/playback-health.json`) but is **Prometheus probe-only** (scraper canary health, parser success). Add **ClickHouse-backed panels** (ClickHouse is already a registered Grafana datasource, UID `aenigma-clickhouse`) for *real user* data:

- Reached-playback rate by provider
- Real resolve success % by provider (contrast with probe success)
- p95 real resolve latency by provider
- **Stall rate / p95 stall-ms by provider** (new lag view)
- Top-failing `(provider, anime_id)` from real watches

## Component 3 — Daily ranking job

Clone of the proven `read_threshold` recompute pattern (`services/analytics/internal/service/read_threshold.go` + scheduler cron):

- **scheduler** daily cron (~`0 4 * * *`, sibling of `top_anime.go`) → `analytics POST /internal/player-ranking/recompute`.
- **analytics** queries ClickHouse over a trailing window (7d, recency-weighted), `HAVING count() >= N` **min-sample gate** (cold titles defer to curated tier).
- **Score** per `(anime_id, provider)` and global per-provider:
  - **primary**: reached-playback rate (did real users actually watch past 0:00).
  - **soft adjustments**: resolve success rate, p95 resolve latency, stall rate.
- **Publish** the computed top to **Redis** (48h TTL, exactly like `read_thresholds`). This honors "don't use Redis to *accumulate*" — ClickHouse accumulates; Redis holds only the **daily-computed artifact** (explicitly approved) + the same-day override.
- **Catalog** reads the published ranking and folds it into the source payload the player already fetches (one read, no extra round-trip).

## Component 4 — Source payload (catalog)

Catalog serves a single per-anime payload consumed at player load:

```jsonc
{
  "firstParty": { "available": true, "episodeCount": 12 },  // from catalog's library parser client
  "dailyTop":   [ { "provider": "allanime", "score": 0.93 }, ... ],  // global ∪ per-anime
  "curatedTier":[ "ae", "allanime", "gogoanime", "kodik", ... ],     // static prior
  "override":   "miruro"   // Redis srcfix:{animeId} if present, else null
}
```

`firstParty` is sourced from the existing **library parser client** (`services/catalog/internal/parser/library/client.go`) — see Component 6.

## Component 5 — Decision flow at load (in watch-combo terms)

1. **Saved combo wins** — the user's *manually selected* WatchCombo (`/preferences`, untouched). If its provider/server is now dead → toast *"the source you watched last time isn't available right now"* → fall through.
2. **Redis same-day override** (`srcfix:{animeId}`) — consulted next, adopted **only if it produces a watch-combo OK pass** (validates through `/preferences/resolve` *and* resolves a playable stream).
3. **Daily reliability top** — client receives it and picks via watch-combo logic. **First-party `ae` is positioned on top**, *unless* the title isn't in our library (Component 6).
4. Within the chosen rank, walk the list and pick the first provider that **actually has episodes** for this anime; emit telemetry on the way.

All candidates are validated through the existing watch-combo resolver — the new layer never crosses language/type boundaries the resolver forbids.

## Component 6 — Seamless first-party availability

Catalog's **library parser client** (`services/catalog/internal/parser/library/client.go`) already knows first-party episode availability. We surface `firstParty: { available, episodeCount }` **inside the source payload** (Component 4), so the client positions `ae` on top **only when `available`** and silently drops it otherwise — **no extra round-trip, no empty-provider flash, no wasted resolve**. The empty-`listEpisodes` → auto-fallback path (Component 7) stays as a belt-and-suspenders safety net. This also fixes today's bug where the default lands on an empty `ae`.

## Component 7 — Client-side fallback (cautious, staged)

Two distinct failure moments, handled differently:

- **Resolve-time** (provider returns nothing *before* playback): **silent automatic** fallback down the ranked list — nothing was playing, so no disruption. Emit `player_resolve` fail. If the fallback reaches playback **and the original best was the one that failed**, write the Redis same-day override `srcfix:{animeId}` (24h) — the "only save best-working when current best fails" rule.
- **Playback-time** (hls.js fatal mid-watch): **staged**.
  - **Stage 1**: a one-tap *"This source dropped — switch to X?"* suggestion (no surprise swap).
  - **Stage 2**: opt-in silent auto-advance, enabled once telemetry shows it's safe.

Every failure feeds the next day's ranking.

## Component 8 — Granular options (Kodik dub team & friends)

Restore the affordance the unified player dropped:

- The Kodik adapter already fetches translations and `Combo.team` is plumbed end-to-end (`useProviderResolver.ts:392`, `SourcePanel.vue:89`). We just **populate the Team chips** `SourcePanel` already renders, mapping each team to its `translation_id/translation_title`.
- The same mechanism exposes scraper **servers** and **sub/dub variants**.
- Result: parity with the legacy `KodikPlayer` team picker, inside the unified player.

---

## Staging

**Stage 1 — no telemetry dependency (immediate value):**
- Curated tier + live scraper health + **seamless first-party availability** (kills the empty-`ae` default).
- Saved-combo respect + "last source unavailable" toast.
- Silent resolve-time fallback.
- **Kodik team picker** restored.
- Wire unified player into `/preferences/resolve` (+ additive `WatchCombo.player` enum values, group→player mapping).

**Stage 2 — after telemetry accrues:**
- `player-events` beacon (`player_resolve` + `player_stall`).
- ClickHouse panels on the Playback dashboard.
- Daily ranking job (scheduler → analytics recompute → Redis publish).
- Ranking-aware default + Redis same-day override.
- Playback-time fallback suggestion (Stage 2 of Component 7).

---

## Files / seams touched (indicative)

- **Frontend:** `components/player/unified/UnifiedPlayer.vue` (decision flow, fallback), `providerRegistry.ts` (curated tier), `composables/unifiedPlayer/useProviderResolver.ts` (team/server exposure, outcome emit), `useProviderHealth.ts` (ranking merge), `SourcePanel.vue` (team chips), `composables/useWatchPreferences.ts` + `useWatchTracking.ts` (wire resolve), new `utils/playerTelemetry.ts` (beacon), `types/preference.ts` (enum), `types/unifiedPlayer.ts`.
- **catalog:** source-payload endpoint (ranking + firstParty), `parser/library/client.go` availability read, Redis override read.
- **analytics:** `player-events` ingest handler, ClickHouse write, `/internal/player-ranking/recompute` aggregation (+ Redis publish), `events` effect_kind additions.
- **scheduler:** new `jobs/player_ranking.go` daily cron.
- **Grafana:** `docker/grafana/dashboards/playback-health.json` new ClickHouse panels.

## Open questions (resolve during planning)

- Exact ranking score weights (reached-playback vs latency vs stall) and the min-sample threshold `N`.
- Whether `player_stall` measures need dedicated nullable columns or fit existing `events` measures.
- `WatchCombo.player` adult mapping for `18anime` vs `hanime` (both currently `hanime`?).
- Trailing window length + recency decay for the daily aggregation.

---

## Effort & Impact (per `.planning/CONVENTIONS.md`)

- **UXΔ = +3 (Better)** — correct default (no empty-`ae`), restored Kodik team picker, fewer dead-source dead-ends, gentle handling of dead saved sources.
- **CDI (Stage 1) = 0.04 * 13** — moderate spread (frontend + catalog), moderate shift (wiring unified player into the existing resolver), contained effort.
- **CDI (Stage 2) = 0.06 * 21** — wider spread (frontend + analytics + scheduler + catalog + Grafana), new telemetry + aggregation pipeline.
- **MVQ = Griffin 85%/80%** — composes proven existing patterns (`read_threshold` recompute, watch-combo resolver, ClickHouse events) rather than inventing; slop-resistant because each loop is independently testable.
