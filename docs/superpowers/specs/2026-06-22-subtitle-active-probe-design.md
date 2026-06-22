# Active subtitle-provider probe + degraded note — design

**Date:** 2026-06-22
**Owner TODO:** `/admin/feedback?id=2026-06-22T04-41-31_claude-code_feedback`
**Status:** approved (brainstorm), pending implementation

## Problem

Subtitle providers (Jimaku especially — known to go down intermittently) have only a
**passive** health signal today. The passive observability **already shipped** to
origin/main (commits `a8460701`, `0cd0c3ea`, `f990792d`, `fe60922e`): `libs/metrics/subtitles.go`
emits `catalog_subtitle_provider_up{provider}`, `catalog_subtitle_resolve_total`, and
`catalog_subtitle_resolve_duration_seconds` from `SubsAggregator.FetchAll`
(`RecordSubtitleResolve`), and `docker/grafana/dashboards/subtitle-health.json` renders
them. The aggregator response already carries `providers_down`, and `OtherSubsPanel.vue`
already shows a passive "some sources didn't respond" note.

The gap: passive metrics are **driven only by real `/subtitles/all` traffic**, so they go
**blind with no traffic** — a Jimaku outage at 4am won't register until someone opens the
subtitle picker. We want an *active* probe that hits a cheap provider endpoint on a fixed
interval so an outage is visible immediately regardless of traffic, **and** a user-facing
note in the subtitle picker when a provider is currently degraded/down (driven by the
probe's last-known verdict, merged with the existing passive `providers_down`).

> Correction note: an earlier draft of this spec was written against a stale base tree and
> wrongly assumed passive observability did not exist yet. It does. This feature is purely
> additive: a new `probe_`-prefixed gauge family, an extension of the existing dashboard,
> and a `provider_health` overlay on the subs response.

## Scope (locked)

In scope:
- Active probe in **catalog** (reuses existing Jimaku + OpenSubtitles clients + API keys).
- New `probe_`-prefixed Prometheus gauges mirroring the video `probe_provider_up` pattern,
  + latency + last-run. Kept DISTINCT from the existing passive `catalog_subtitle_*` family.
- **Extend** the existing `docker/grafana/dashboards/subtitle-health.json` with active-probe
  panels (alongside the passive panels already there).
- FE degraded note in the subtitle picker, driven by the probe's last-known verdict, merged
  with the existing passive `providers_down`.

Out of scope (this iteration):
- Alert rule for "Jimaku down > N min" (deferred; owner chose "Probe + Grafana panel").
- Any change to the existing passive `catalog_subtitle_*` metrics (they stay as-is).

## Decisions (locked)

- **Host:** catalog. The Jimaku/OpenSubtitles clients and API keys already live there;
  analytics has neither. (Owner: "Catalog".)
- **Cadence:** every 5 min (`*/5 * * * *`). ~288 cheap calls/provider/day, well within
  limits; matches the "4am outage visible within minutes" goal. (Owner: "Every 5 min".)
- **FE note merge:** ONE merged note driven by `max(passive providers_down, active probe
  verdict)`; `down` wins over `degraded`. No duplicate warnings. (Owner: "One merged note".)

## Architecture

### 1. Probe (catalog) — `services/catalog/internal/service/subprobe/`

`SubtitleProbe.RunOnce(ctx)`:
- Pings each provider's cheapest **non-quota** reachability endpoint, times it, classifies
  the verdict, stores it in the `HealthStore`, and emits the Prometheus gauges.
- **Jimaku** — new `Client.Ping(ctx) (time.Duration, error)` → `GET /api/entries/search?anilist_id=1`
  with the API key. Any `200` (even an empty array) proves reachability + key validity. No
  download/quota cost.
- **OpenSubtitles** — new `Client.Ping(ctx) (time.Duration, error)` → `GET /infos/formats`
  (static reference data; needs only the `Api-Key` header; does **not** consume the daily
  download quota).
- Classification:
  - `up`     — `200` within the latency budget.
  - `degraded` — `200` but slow (latency > `degradedLatency`), or a transient `429` / `5xx`.
  - `down`   — transport error, auth failure (`401/403`), or other non-2xx.

`HealthStore` (in-memory, RWMutex):
- `map[provider]Health{Status, LatencyMS, CheckedAt}`.
- `Snapshot()` returns the current health, downgrading any entry older than
  `staleAfter` (e.g. 3× the probe interval) to `unknown` — we never report a stale `up`.
- `Record(provider, Health)` is called by the probe.
- A provider that has never been probed is absent from the snapshot (FE treats absent as
  "no signal" → shows nothing, identical to today).

### 2. Trigger path (mirrors `services/scheduler/internal/jobs/probe_trigger.go`)

- Catalog: `POST /internal/subtitle-probe/run` — Docker-network-only, **not** gateway-proxied
  (consistent with the existing `/internal/*` convention). Calls `SubtitleProbe.RunOnce`,
  returns `204` on success.
- Scheduler: new `SubtitleProbeTriggerJob` (clone of `ProbeTriggerJob`) POSTs that endpoint.
  - New cron config `SUBTITLE_PROBE_CRON`, default `*/5 * * * *`.
  - **Reuse** the existing `JobsConfig.CatalogServiceURL` (default `http://catalog:8081`) —
    confirmed present in `services/scheduler/internal/config/config.go`. No new URL var.
  - Registered in `services/scheduler/internal/service/job.go` + wired in
    `cmd/scheduler-api/main.go` exactly like the playback probe (metrics wrapper, last-run).
  - Short request timeout (the probe is two cheap HTTP pings, not a stream sweep) — ~30s.

### 3. Metrics — `libs/metrics/probe.go`

Add (promauto, same file as the video probe gauges):
- `probe_subtitle_provider_up{provider}` — gauge, `1` up / `0.5` degraded / `0` down.
  `unknown`/stale entries are NOT emitted (Reset() each run so stale label series drop).
- `probe_subtitle_latency_seconds{provider}` — gauge, last ping latency in seconds.
- `probe_subtitle_last_run_timestamp` — gauge, unix seconds of the last completed run.

Catalog already exposes `/metrics`; no new scrape target needed.

### 4. FE degraded note

Backend:
- `AggregateResponse` (subs_aggregator) gains:
  ```go
  ProviderHealth []ProviderHealth `json:"provider_health,omitempty"`
  // ProviderHealth: { Provider string; Status string; LatencyMS int }
  ```
- **Injected fresh from `HealthStore.Snapshot()` on every response — NOT part of the
  Redis-cached body.** The aggregator caches tracks for up to 6h; baking health into that
  body would freeze a stale `up`. So: keep the cache exactly as today (tracks only), and
  overlay `ProviderHealth` after the cache get/set, in `FetchAll` (or in the handler
  `respond`), reading the live snapshot each call.
- Merge rule for the response: a provider is reported with the **worse** of its passive
  result (in `ProvidersDown` → treated as `down`) and its active probe status. `down` >
  `degraded` > `up`. (One merged signal — satisfies "one merged note".)

Frontend — `OtherSubsPanel.vue`:
- Read `provider_health` from the response.
- Replace/extend the current `providers_down` note: render a single note listing any
  provider whose merged status is `degraded` or `down`, with wording that distinguishes
  "temporarily degraded" from "down". Keep it visually identical in placement to today's
  warning (`text-warning/80 … border-t`).
- New i18n keys under `player.otherSubs.*` in `en.json`, `ru.json`, `ja.json` (locale
  parity test enforces all three).

### 5. Grafana — extend the existing `subtitle-health` dashboard

`docker/grafana/dashboards/subtitle-health.json` already exists (panels: Per-Provider
Uptime, Resolve Rate by Status, Resolve Latency p50/p95, Tracks Returned, Providers
Currently Down — all passive `catalog_subtitle_*`). Datasource `${DS_PROMETHEUS}`, panels
on a 24-col grid. **Add** active-probe panels below the existing ones:
- "Active Probe — Per-Provider Status" timeseries (`probe_subtitle_provider_up`).
- "Active Probe — Latency" timeseries (`probe_subtitle_latency_seconds`).
- "Active Probe — Last Run" stat (`probe_subtitle_last_run_timestamp`, unit dateTimeFromNow).
- No alert rule this iteration.

## Data flow

```
scheduler (cron */5)
  └─ POST catalog /internal/subtitle-probe/run
       └─ SubtitleProbe.RunOnce
            ├─ jimaku.Ping       → classify → HealthStore.Record + gauges
            └─ opensubtitles.Ping → classify → HealthStore.Record + gauges

user opens subtitle picker
  └─ GET /api/anime/{id}/subtitles/all
       └─ SubsAggregator.FetchAll (tracks from Redis cache or live)
            └─ overlay ProviderHealth = HealthStore.Snapshot() ⊔ ProvidersDown (worse wins)
       └─ OtherSubsPanel renders merged degraded/down note

Prometheus scrapes catalog /metrics → Grafana subtitle-health dashboard
```

## Error handling

- Probe failures never propagate: a panic in one provider's ping must not abort the other
  or the HTTP handler (recover per provider; the run always records a verdict per provider).
- Scheduler trigger: a non-2xx / transport error is returned so the JobService metrics
  wrapper records a failure; a single missed run is tolerated (health carries `CheckedAt`,
  stale → `unknown`).
- Redis outage: the health overlay reads in-memory `HealthStore`, independent of Redis, so
  a Redis blip never hides health.
- Missing API key: `IsConfigured() == false` → probe records `down` (configuration is itself
  a reachability failure for the user) — or skip entirely? **Decision: skip unconfigured
  providers** (absent from snapshot) so a deployment that intentionally runs without a key
  doesn't show a permanent "down" note. Log once at startup.

## Testing

- `subprobe`: table tests for classification (200 fast → up; 200 slow → degraded; 429/5xx →
  degraded; 401/transport → down) using an `httptest.Server` / injected RoundTripper.
- `HealthStore`: record + snapshot + staleness downgrade to `unknown`; concurrent
  record/snapshot under `-race`.
- `jimaku.Ping` / `opensubtitles.Ping`: test against `httptest.Server` (no live API).
- Aggregator overlay: `FetchAll` returns `ProviderHealth` from an injected store; cached
  tracks + fresh health (health not frozen by cache); merge picks the worse status.
- Handler: `/internal/subtitle-probe/run` returns 204 and triggers the probe.
- Scheduler: `SubtitleProbeTriggerJob.Run` posts the right URL; non-2xx → error
  (clone of `probe_trigger_test.go`).
- FE: `OtherSubsPanel.spec.ts` — degraded note renders for a degraded provider; down note
  for down; nothing when all up/absent; merge with `providers_down`.
- Locale parity: new keys present in en/ru/ja.

## Verification gate (the owner's "cheapest next step")

Before declaring done, **prove the gauge flips on a forced failure** end-to-end:
trigger the probe with a deliberately bad Jimaku key (or a blocked host) and confirm
`probe_subtitle_provider_up{provider="jimaku"}` reads `0` on catalog `/metrics`, then
confirm a healthy run flips it back to `1`.

## Files touched (anticipated)

- `libs/metrics/probe.go` — 3 new metrics.
- `services/catalog/internal/parser/jimaku/client.go` — `Ping`.
- `services/catalog/internal/parser/opensubtitles/client.go` — `Ping`.
- `services/catalog/internal/service/subprobe/{probe.go,store.go,*_test.go}` — new.
- `services/catalog/internal/service/subs_aggregator.go` — `ProviderHealth` overlay.
- `services/catalog/internal/handler/` — `/internal/subtitle-probe/run` handler.
- `services/catalog/internal/transport/router.go` — internal route.
- `services/catalog/cmd/catalog-api/main.go` — wire probe + store into aggregator + handler.
- `services/scheduler/internal/jobs/subtitle_probe_trigger.go` (+ test) — new.
- `services/scheduler/internal/config/config.go` — `SUBTITLE_PROBE_CRON` (reuse `CatalogServiceURL`).
- `services/scheduler/internal/service/job.go` + `cmd/scheduler-api/main.go` — register.
- `frontend/web/src/components/player/OtherSubsPanel.vue` (+ `OtherSubsPanel.spec.ts`).
- `frontend/web/src/types/raw.ts` — `ProviderHealth` + `provider_health` on `GroupedSubs`.
- `frontend/web/src/locales/{en,ru,ja}.json` — note keys.
- `docker/grafana/dashboards/subtitle-health.json` — EXTEND with active-probe panels.
- `docker/docker-compose.yml` (scheduler env) + CLAUDE.md env docs — `SUBTITLE_PROBE_CRON`.
