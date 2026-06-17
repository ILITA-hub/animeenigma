# Phase 8: Serving & Fetch Signal - Context

**Gathered:** 2026-06-17
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss) — enriched from the approved design spec

<domain>
## Phase Boundary

When the player resolves the **"ae"** provider for a RAW episode:
- **HIT** — the episode is present in the pool → serve `aeProvider/<mal>/RAW/<ep>/playlist.m3u8`,
  bump the episode's `last_fetch_at`=now + `fetch_count`++ (the "viewed by any user" freshness +
  popularity signal), and count a **preload hit**.
- **MISS** — the episode is absent → fail over to the existing providers with **no regression**,
  count a **preload miss**, and fire a **backfill demand** so it's cached next time.

**Requirements:** SERVE-01 (serve-from-pool + hit), SERVE-02 (bump last_fetch_at/fetch_count on
every ae playback), SERVE-03 (failover-on-miss with no regression + count miss).

**In scope:** the hit/miss instrumentation at the ae-resolution seam; the library-side
fetch-recording path (bump `last_fetch_at`/`fetch_count`); the `library_autocache_serve_total{
result="hit"|"miss"}` counter; and the **backfill-demand intake seam** (a `DemandTracker.Record`
+ internal `POST /internal/library/autocache/demand` endpoint that STORES a wanted
`(mal_id, episode, reason)` item) so the P8 miss path has a real endpoint to call. **The
Planner/consumer that turns demand into downloads is Phase 9 — Phase 8 only RECORDS demand.**

**Out of scope (later phases):** Logic A/B triggers + the download Planner that drains the demand
queue (Phase 9), the evictor (Phase 10), Grafana panels + the serve-hit-rate dashboard (Phase 11
— but Phase 8 MUST emit the `serve_total` counter so P11 has a series to chart).
</domain>

<decisions>
## Implementation Decisions

Authoritative design: `docs/superpowers/specs/2026-06-17-auto-torrent-population-design.md` §5
(hit/miss + fetch signal), §3.4 (`last_fetch_at`/`fetch_count` ledger added in Phase 7), §9
(DemandTracker). Phase 7 already shipped: `library_episodes.last_fetch_at` (`*time.Time`),
`fetch_count` (int64), the `aeProvider/<MALID>/RAW/<ep>/` layout (`autocache.RawPrefix`), and the
`autocache_config` accessor (incl. the master `enabled` switch — when `enabled=false`, do NOT
record demand or bump signals beyond serving what already exists).

Locked points:
- A single RAW object satisfies BOTH raw- and sub-preferring demand (subs are client-side
  overlay) — the serve path does not branch on sub vs raw.
- `shikimori_id` IS the MALID.
- The internal demand/fetch endpoints are **Docker-network-only** (NOT gateway-proxied), mirroring
  the existing `/internal/*` seams (e.g. recs `/internal/recs/recompute-hint`, notifications
  `/internal/notifications`). CLAUDE.md: gateway does NOT proxy `/internal/*`.

### Claude's Discretion
- WHERE the hit/miss decision is observed: the ae provider is resolved in catalog
  `services/catalog/internal/service/raw_resolver.go` via the optional `library.Client`. Decide
  whether (a) catalog reports the resolution outcome to library over an internal call (library
  owns `/metrics` + the ledger), or (b) catalog emits the counter and calls a library
  fetch/demand endpoint. Prefer keeping the `library_autocache_*` metrics + ledger writes in the
  **library** service (it owns `/metrics` per the design and the `library_*` metric namespace),
  with catalog making the internal call on hit (record-fetch) and on miss (record-demand).
- Exact fetch-bump trigger: on ae resolution that returns a playable pool URL (resolution ≈
  playback intent for ae). At-least-once / idempotency of the bump is acceptable (fetch_count is a
  popularity counter, not money). Decide debounce if needed.
- Endpoint shapes under `/internal/library/autocache/{fetch,demand}` and the `DemandTracker`
  storage (in-memory vs a `autocache_demand` table). A durable table is preferable so Phase 9's
  Planner can drain it across restarts — but keep the schema minimal (`mal_id, episode, reason,
  requested_at`, unique on `(mal_id,episode)` for dedup).
</decisions>

<code_context>
## Existing Code Insights

- `services/catalog/internal/service/raw_resolver.go` — the ae provider resolution. `library
  *library.Client` is the optional seam (`NewRawResolver(..., libraryClient, ...)`). `GetEpisodes`
  / the stream resolution decide library-present vs AllAnime-fallback. **This is where HIT vs MISS
  is observable.** (Phase 7 audit confirmed it builds URLs only from `minio_url`/`minio_path`.)
- `services/catalog/internal/parser/library/client.go` — catalog→library HTTP client; add the
  fetch/demand internal calls here (or a sibling).
- `services/library/internal/handler/` + `internal/transport/router.go` — where the new internal
  `/internal/library/autocache/{fetch,demand}` handlers mount. Internal routes are Docker-network
  only (NOT under the gateway-proxied `/api/library/*`). Check how library currently distinguishes
  internal vs gateway routes (if it doesn't yet, add an `/internal/` prefix not proxied by gateway).
- `services/library/internal/repo/episode.go` — Phase 7 added `UpdateMinioPath`; add a
  `BumpFetch(ctx, malID, ep)` (set `last_fetch_at=now()`, `fetch_count=fetch_count+1`) here.
- `services/library/internal/metrics/library_metrics.go` — existing Prometheus metrics; add
  `library_autocache_serve_total{result}` counter (and keep it cheap/no high-cardinality labels).
- `services/library/internal/config/config.go` + `services/catalog` config — add the internal URL
  wiring (`LIBRARY_INTERNAL_URL` / reuse existing catalog→library base URL; player/catalog config).
- Reference seams for the internal producer pattern: recs `POST /internal/recs/recompute-hint`
  (fire-and-forget, non-blocking, drop-on-failure) and notifications `POST /internal/notifications`.
  The fetch/demand calls from catalog MUST be non-blocking + best-effort (never fail a playback
  resolution because the library internal call errored).

## Player / frontend
- The frontend "ae" provider already fails over to AllAnime-raw/others on unavailability (today's
  behavior). SERVE-03 "no regression" means: do not change the failover UX; only ADD the
  miss-count + backfill-demand emission on the backend resolution path. Confirm the OurEnglish/Raw
  player failover path is untouched.
</code_context>

<specifics>
## Specific Ideas

- `repo.EpisodeRepository.BumpFetch(ctx, malID, ep)` — `last_fetch_at=now()`, `fetch_count++`,
  scoped to `(shikimori_id, episode_number, source/track)`; no-op (no error) if the row is absent.
- `autocache_demand` table (minimal) + `DemandRepository.Record(mal_id, ep, reason)` upsert
  (`ON CONFLICT (mal_id, episode) DO NOTHING` / refresh `requested_at`). Reason enum: `next_ep`,
  `backfill` (Phase 9 adds the producers; Phase 8 only writes `backfill`).
- Internal handlers: `POST /internal/library/autocache/fetch` {mal_id, episode} → BumpFetch +
  `serve_total{result="hit"}`++; `POST /internal/library/autocache/demand` {mal_id, episode,
  reason} → DemandRepository.Record + (when reason=backfill) `serve_total{result="miss"}`++.
  Honor the `enabled` switch (when off: still serve, but skip demand recording).
- Catalog ae-resolution: on present → fire fetch call (non-blocking); on absent → fire demand
  (reason=backfill) call (non-blocking) + ensure failover unchanged.
</specifics>

<deferred>
## Deferred Ideas

- Logic A/B demand producers + the download Planner that drains `autocache_demand` → Phase 9.
- Evictor / budget enforcement → Phase 10.
- Grafana serve-hit-rate panel (consumes `library_autocache_serve_total`) + prediction → Phase 11.
- Fetch-event aggregation/analytics beyond the counter + ledger bump → not in this milestone.
</deferred>
