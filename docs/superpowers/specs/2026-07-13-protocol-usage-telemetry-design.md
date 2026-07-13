# Protocol (h1/h2/h3) Usage Telemetry + Grafana Panels — Design

- **Date:** 2026-07-13
- **Status:** Approved (brainstorming) — ready for implementation plan
- **Branch:** `worktree-aeplayer-failure-alert`
- **Related:** [`2026-07-11-protocol-ladder-design.md`](2026-07-11-protocol-ladder-design.md) (the streamX ladder this instruments), [`2026-07-11-aeplayer-playback-failure-alert-design.md`](2026-07-11-aeplayer-playback-failure-alert-design.md) (the `player_failed` telemetry event this mirrors)

## Goal

Make h1/h2/h3 protocol usage observable. Today the streamX protocol ladder
(`stream3=h3/QUIC · stream2=h2 · stream1=h1.1`) is a purely client-side
mechanism — its tier, measured throughput, downshift trail, and h3-probe verdict
live **only** in the hacker-mode HUD (`debugStats`, which returns `null` when
hacker mode is off) and reach **no** telemetry sink. There is zero protocol
representation in the analytics pipeline or any dashboard.

Add a **per-session-per-tier usage event** to the existing player-telemetry
pipeline and surface it on the `playback-health` Grafana dashboard as (1) a
detail **table** and (2) a **usage-share** panel.

## Scope

**In scope**
- A new `protocol_usage` frontend telemetry event, emitted once per
  (session × protocol tier) — on ladder tier-change and at session end.
- Ingest into the existing `analytics.events` ClickHouse table via a new
  `effect_kind = 'player_protocol'` (no schema change, no new endpoint).
- Two new panels on `docker/grafana/dashboards/playback-health.json`:
  a detail table and a protocol usage-share chart.

**Out of scope (separate follow-up)**
- Capturing the same `debugSnapshot()` data into **user feedback reports**
  (`ReportButton` / `player_diagnostics`). Same source (`ladder.debugSnapshot()`),
  different sink. Tracked as a distinct future item; NOT built here.

## Locked decisions (from brainstorming)

| Decision | Choice | Rationale |
|---|---|---|
| **User identity ("who")** | **Drop the column — fully anonymous** | The player-telemetry pipeline is deliberately anonymous (`playertelemetry.go` sets no `user_id`/`anonymous_id`/`session_id`; audit note guards against identifiable beacon rows). Respects privacy-core masked analytics. |
| **"dropped packages %"** | **Dropped video frames % + segment-timeout count** | Browsers can't see TCP/QUIC packet loss from JS. `getVideoPlaybackQuality()` gives dropped/total frames; ladder tracks timeouts. Two columns. |
| **Row granularity** | **Per session × tier** | A session that ran h2 then upshifted to h3 yields two rows — true per-protocol breakdown. Tier switches are rare (30s cooldown) → low volume. |
| **Storage** | **Reuse `analytics.events`, `effect_kind='player_protocol'`, metrics in `properties` JSON** | Zero schema change, reuses `/api/analytics/player-events`, mirrors `player_failed` exactly, immediately Grafana-queryable via `JSONExtract`. |
| **Panels** | **Detail table + Protocol usage share** | Both requested. |

## Architecture

Three layers, mirroring the existing `playback_failed` path
(`recordPlayerEvent` → `/api/analytics/player-events` → analytics:8092 →
ClickHouse `analytics.events` → Grafana `aenigma-clickhouse`).

### 1. Frontend emission — per session×tier summary

The ladder already computes per-tier segment count (`fragSamples`), timeout
count (`timeoutCount`), measured/needed EWMA, `lastProtocol` (nextHopProtocol),
`trail`, and `lastProbe` — but **`resetTierCounters()` wipes the per-tier
counters on every switch**. Extend the ladder to hand off a residency summary
*before* the reset.

**`frontend/web/src/utils/protocolLadder.ts`**
- Add per-tier residency accumulators: `residencyBytes`, `residencyMs`
  (summed from `reportFragment`'s `FragReport{bytes, ms}` — enables a *true*
  mean speed = `Σbytes·8 / Σseconds`, distinct from the EWMA). `segments` and
  `timeouts` reuse the existing `fragSamples` / `timeoutCount`.
- `onResidencyEnd(cb: (r: TierResidency) => void)` — fires inside `applySwitch`
  immediately before `resetTierCounters()`, with the tier being **left**.
- `currentResidency(): TierResidency | null` — snapshot of the in-progress tier
  for the final flush at session end (returns `null` if 0 segments).
- `TierResidency` = `{ tierId, protocol, segments, avgMbps, neededMbps,
  measuredMbps, timeouts, tierMs, trail, probe }`. Framework-free — the ladder
  only exposes data; it does not import telemetry.
- Residency accumulators reset alongside the existing counters in
  `resetTierCounters()` and on `resetToEntryOnNetworkChange()`.

**`frontend/web/src/components/player/aePlayer/AePlayer.vue`** (emission glue)
- Snapshot `getVideoPlaybackQuality()` `{droppedVideoFrames, totalVideoFrames}`
  at each tier's **start**; at residency end compute the **delta** →
  `dropped_frames_pct = 100 · droppedΔ / max(totalΔ, 1)`. (Delta, not
  cumulative, so each tier row reflects only its own residency.)
- Generate one ephemeral, non-identifying `sess` id per player mount (groups a
  session's tier rows in the dashboard; NOT a user id).
- Subscribe to `ladder.onResidencyEnd` → assemble the `protocol_usage` event
  (ladder summary + dropped-frame delta + combo/anime/episode/sess) → emit via
  `recordPlayerEvent`.
- Flush `ladder.currentResidency()` on `pagehide`, component unmount, and source
  teardown. Skip any residency with `segments === 0`.

**`frontend/web/src/utils/playerTelemetry.ts`**
- Add `'protocol_usage'` to the `PlayerEvent['kind']` union. All protocol metrics
  ride in `detail` (merged verbatim into `properties` by the handler), so no new
  wire fields are needed. Existing batching / rate caps apply unchanged.

`protocol_usage` event shape:
```ts
recordPlayerEvent({
  kind: 'protocol_usage',
  provider,            // combo provider (gogoanime, ae, …) — must be whitelisted
  anime_id, episode, audio, lang,
  detail: {
    schema_version: 1,
    protocol: 'h2',           // nextHopProtocol (ground truth)
    tier: 'h2', tier_index: 1, tier_count: 3,   // which origin the ladder aimed at
    segments: 214,
    avg_mbps: 3.2,            // Σbytes·8 / Σseconds (true mean)
    needed_mbps: 2.1,         // required bitrate EWMA (the ×1.2 headroom target)
    dropped_frames_pct: 0.4,
    seg_timeouts: 0,
    tier_ms: 184000,
    trail: '',               // "h3→h2 (first-frag projected 17s)" if it switched
    probe: 'h3 2.1 Mbps @03:24 — rejected (<1.1× h2)',  // h3 adoption signal
    anime_name: 'Naruto',
    combo: 'sub·en·gogoanime',
    sess: 's_9f3a',          // ephemeral, non-identifying session correlator
  },
})
```

> **Why `protocol` and `tier` are both captured:** `tier` is which origin the
> ladder pointed at; `protocol` is the browser's actual negotiated
> `nextHopProtocol`. They can disagree (e.g. aimed at `stream3` but Alt-Svc not
> yet learned → silently rides h2). Keeping both makes that visible.

### 2. Backend ingest — zero schema change

**`services/analytics/internal/handler/playertelemetry.go`**
- One new `switch` case:
  ```go
  case "protocol_usage":
      effectKind = "player_protocol"
  ```
- The existing `detail` → `propMap` merge stores every metric in the
  `events.properties` String column. The provider whitelist gate
  (`whitelistProvider`) still applies — `protocol_usage` carries a real provider
  id, so it passes. No new endpoint, wire struct, table, DDL, or migration.

Resulting `events` row: `event_type='player'`, `effect_kind='player_protocol'`,
`target=<provider>`, `target_kind='provider'`, `anime_id`, `source='fe'`,
`properties=<the detail JSON>`.

### 3. Grafana — two new panels on `playback-health.json`

Datasource `grafana-clickhouse-datasource` / uid `aenigma-clickhouse`
(db `analytics`), following the existing `events` panels' `$__timeFilter`
idiom. New row **"Protocol Ladder (h1/h2/h3)"**.

**Panel A — table: "Protocol usage (per session × tier)"** (`type: "table"`)
```sql
SELECT timestamp AS "Time",
  JSONExtractString(properties,'protocol')                                 AS "Protocol",
  coalesce(nullIf(JSONExtractString(properties,'anime_name'),''), anime_id) AS "Anime",
  JSONExtractString(properties,'combo')                                    AS "Combo",
  JSONExtractUInt(properties,'segments')                                   AS "Segments",
  round(JSONExtractFloat(properties,'avg_mbps'),2)                         AS "Avg Mbps",
  round(JSONExtractFloat(properties,'dropped_frames_pct'),2)               AS "Dropped frames %",
  JSONExtractUInt(properties,'seg_timeouts')                               AS "Seg timeouts",
  round(JSONExtractFloat(properties,'needed_mbps'),2)                      AS "Needed Mbps",
  JSONExtractString(properties,'trail')                                    AS "Ladder trail",
  JSONExtractString(properties,'probe')                                    AS "h3 probe",
  target                                                                   AS "Provider"
FROM events
WHERE effect_kind = 'player_protocol' AND $__timeFilter(timestamp)
ORDER BY timestamp DESC
LIMIT 500
```
Field overrides: color-threshold "Dropped frames %" (green→amber→red) and
"Seg timeouts" (0 green, >0 amber/red); left-align text columns.

**Panel B — usage share: "Protocol usage share"** (`type: "piechart"`, donut)
Segment-volume-weighted share of each protocol over the dashboard window:
```sql
SELECT JSONExtractString(properties,'protocol') AS protocol,
       sum(JSONExtractUInt(properties,'segments')) AS segments
FROM events
WHERE effect_kind = 'player_protocol' AND $__timeFilter(timestamp)
GROUP BY protocol
ORDER BY segments DESC
```
Legend shows value + percent. (Share is weighted by segments = delivered-volume;
`tier_ms` or session count are alternatives if a time/session weighting is later
preferred.)

Both panels honor the dashboard time picker and `refresh:30s`.

## Data flow

```
AePlayer mount → ladder residency accumulates (segments, bytes, ms, timeouts)
  ├─ ladder tier switch → onResidencyEnd(leftTier) → emit protocol_usage → reset
  └─ pagehide / unmount / source teardown → flush currentResidency() → emit
        │ (skip if segments == 0)
        ▼
recordPlayerEvent({kind:'protocol_usage', detail}) → batched beacon
  → POST /api/analytics/player-events → gateway ProxyToAnalytics → analytics:8092
  → playertelemetry.go: kind→effect_kind 'player_protocol', detail merged→properties
  → batcher → ClickHouse analytics.events
  → Grafana playback-health (aenigma-clickhouse): table + usage-share panels
```

## Edge cases

- **Hacker mode off** — irrelevant here: emission reads the ladder's own
  accumulators + `debugSnapshot()`-equivalent fields directly, never the
  hacker-gated `debugStats`. Works regardless of HUD state.
- **Single-tier / dev** (`isMultiTier() === false`) — ladder does no tracking;
  `currentResidency()`/`onResidencyEnd` never fire; no events. Correct (no
  protocol choice to report).
- **0-segment tiers** (instant switch, MP4/native-HLS source that never feeds
  `reportFragment`) — skipped, no row.
- **Provider not whitelisted** — dropped at the handler gate (same as all player
  telemetry). A protocol event for a genuine provider is always whitelisted.
- **Volume** — ≤ a handful of rows per session (switches are 30s-cooldown-gated,
  plus one final flush); well within FE `RATE_PER_MIN`/`SESSION_CAP` and the
  analytics drop-on-full backpressure.
- **Privacy** — no user id, no signed stream URLs; `properties` carries combo,
  anime name, CDN-agnostic protocol metrics, and an ephemeral `sess`. Admin-only
  dashboard. Consistent with masked-analytics.

## Testing

- **`protocolLadder.spec.ts`** — residency accumulation (bytes/ms/segments/
  timeouts); `onResidencyEnd` fires with the correct summary **before** counters
  reset; `currentResidency()` returns null at 0 segments; accumulators clear on
  network-change reset.
- **AePlayer emission** — dropped-frame delta math; skip-empty-tier; flush on
  pagehide/unmount; one event per residency.
- **`playertelemetry_test.go`** — `kind:'protocol_usage'` → `effect_kind
  'player_protocol'`; all `detail` keys land in `properties`; whitelist still
  enforced.
- **Dashboard JSON** — `jq` validity; `make restart-grafana` then log-grep for
  provisioning errors; eyeball both panels render.

## Files (anchors)

**Frontend**
- `frontend/web/src/utils/protocolLadder.ts` — residency accumulators +
  `onResidencyEnd` / `currentResidency` / `TierResidency` (+ `protocolLadder.spec.ts`)
- `frontend/web/src/components/player/aePlayer/AePlayer.vue` — dropped-frame
  delta, `sess` id, subscribe + emit, pagehide/unmount flush
- `frontend/web/src/utils/playerTelemetry.ts` — `'protocol_usage'` kind

**Backend**
- `services/analytics/internal/handler/playertelemetry.go` — `protocol_usage`
  → `player_protocol` case (+ `playertelemetry_test.go`)

**Grafana**
- `docker/grafana/dashboards/playback-health.json` — new row + table panel +
  usage-share panel

## Deploy

- FE → `/frontend-verify`, then `make redeploy-web`
- analytics → `make redeploy-analytics` (reruns idempotent `EnsureSchema`; no
  DDL change here anyway)
- Grafana → `make restart-grafana` (dashboard JSON auto-provisioned from the
  mounted dir; **no** force-recreate — the ClickHouse datasource/plugin already
  exist)
- Order: analytics before web (so the handler accepts the new kind before the
  FE starts emitting it), Grafana any time.

## Scoring (per `.planning/CONVENTIONS.md`)

- **UXΔ = 0 (Ambiguous)** — no end-user-visible change; pure operator
  observability. (Second-order UX win: h3-adoption/regression finally
  measurable, feeding future ladder tuning.)
- **CDI = 0.03 × 21** — additive across three layers (one FE module extension,
  one FE glue site, one BE case, one dashboard file); low spread/shift, moderate
  effort.
- **MVQ = Griffin 85% / 80%** — reuses the established telemetry + dashboard
  patterns end to end; low slop risk.

## Open / follow-up

- **Feedback-report `debugSnapshot()` capture** — the deferred sibling item
  (protocol + full hacker-mode bundle into `ReportButton` / `player_diagnostics`).
  Separate spec.
- Optional later: time-weighted (`tier_ms`) or session-count share weighting;
  a protocol-over-time state-timeline panel.
