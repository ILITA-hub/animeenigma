# Playback / Health — Grafana Dashboard Merge & Improve

**Date:** 2026-06-05
**Status:** Design — pending implementation
**Author:** AI-assisted (brainstorming session)

## Goal

Merge three existing Grafana dashboards into a single unified **Playback / Health**
dashboard, and improve it on four axes the user called out: better at-a-glance
status, more/better metrics, visual polish & consistency, and less noise / dedup.

Replaced dashboards:

| Old dashboard | uid | File | Domain |
|---|---|---|---|
| Player / Health | `player-health` | `docker/grafana/dashboards/player-health.json` | RU players (Kodik, AnimeLib) liveness |
| Scraper / Health | `scraper-health` | `docker/grafana/dashboards/scraper-health.json` | EN scraper stage probes + fallbacks |
| Scraper / Provider Health | `scraper-provider-health-canary` | `infra/grafana/dashboards/scraper-provider-health.json` | EN playability canary pass/fail |

All three JSON files are **deleted**; one new file replaces them.

> Out of scope: `infra/grafana/dashboards/scraper-providers.json` ("Scraper /
> Provider Management") is a separate management dashboard and is **not** part of
> this merge.

## Key finding: two distinct "provider" worlds

The metrics catalog (verified against live Prometheus on 2026-06-05) contains two
unrelated provider namespaces that the old dashboards blurred:

- **Catalog parsers** — `parser_requests_total{provider,operation,status}` and
  `parser_request_duration_seconds{provider,operation}` (histogram). Live
  providers: `allanime, anime18, animelib, hanime, kodik`. These feed the
  **RU / JP / 18+ players**. **Completely undashboarded today.** They expose
  success/error counts (→ success-rate %) and a latency histogram (→ p95).
- **EN scraper providers** — `provider_health_up{provider,stage}`,
  `provider_enabled{provider}`, `parser_fallback_total{from,to}`,
  `playability_canary_runs_total{provider,server,result,reason,anime_slot}`.
  Live providers: `gogoanime, animepahe, allanime, animefever, miruro,
  nineanime, 18anime`. These feed **OurEnglish**.

The merged dashboard keeps these worlds in separate rows.

### Live-data verification (2026-06-05)

| Metric | Series | Notes |
|---|---|---|
| `player_health_up{player}` | 2 | kodik, animelib |
| `provider_health_up{provider,stage}` | 20 | 7 providers × 5 stages (search/episodes/servers/stream/stream_segment) |
| `parser_requests_total{provider,operation,status}` | 9 | operations: get_adfree_stream/get_episodes/get_stream/get_translations; status: success/error |
| `parser_request_duration_seconds_*` | 9 | histogram present → p95 viable |
| `parser_fallback_total{from,to}` | 3 | |
| `playability_canary_runs_total` | 4 | only `gogoanime/fail` so far; reasons cdn_unreachable, zero_match |
| `provider_enabled{provider}` | 7 | animepahe=0, gogoanime=0, nineanime=0 (degraded), rest=1 |
| `provider_probe_last_tick_timestamp{provider}` | 4 | |
| `parser_zero_match_total{provider,selector}` | 5 | |
| `parser_unplayable_total` | **0 (NO DATA)** | defined but never emitted → **excluded** from dashboard |

## Provisioning context

Grafana provisions dashboards from two file providers (both into the General /
`''` folder — see `docker/grafana/provisioning/dashboards/dashboards.yml`):

- `docker/grafana/dashboards/` → `/var/lib/grafana/dashboards` (provider `default`)
- `infra/grafana/dashboards/` → `/var/lib/grafana/dashboards-infra` (provider `infra-dashboards`)

Volume mounts: `docker/docker-compose.yml` lines ~429–430. Grafana image
`grafana:10.3.3`, schemaVersion 39 for the new file.

The new dashboard lives in **`docker/grafana/dashboards/playback-health.json`**.
Because deletion of a provisioned JSON does not always evict the dashboard from
Grafana's DB, the implementation must verify the three old dashboards are gone
from the live instance after redeploy (provider has `disableDeletion: false`, so
provisioned deletes should propagate; confirm).

## Dashboard specification

- **Title:** `Playback / Health`
- **uid:** `playback-health`
- **tags:** `["playback","health","scraper","players"]`
- **refresh:** `30s`
- **time:** default `now-6h` → `now` (wider than the old `now-1h` so canary +
  trends are visible)
- **schemaVersion:** 39
- **datasource:** Prometheus (match the `${DS_PROMETHEUS}`/default-prometheus
  reference style already used by the existing dashboards — reuse their
  datasource object verbatim to avoid a hardcoded UID drift)
- **layout:** collapsible Grafana rows (each domain is a `row` panel with
  `collapsed` togglable; Rows 1–3 start expanded, the design allows the user to
  collapse any). Collapsed rows stop querying their child panels.

### Template variables

Scoped to the **EN scraper rows (2–3)** only:

- `$provider` — `label_values(provider_health_up, provider)`, multi-value,
  includeAll=true, default `All`.
- `$stage` — `label_values(provider_health_up, stage)`, multi-value,
  includeAll=true, default `All`.

Row 1 (catalog parsers) has a **different** `provider` label value set, so it is
intentionally **not** filtered by `$provider` (documented in panel descriptions).
EN-scraper panels that slice by provider/stage apply
`{provider=~"$provider"}` / `{stage=~"$stage"}` where applicable.

### Row 0 — At-a-glance status strip (not collapsible)

Five `stat` panels, color thresholds, the "is playback broken?" row:

1. **RU Players Up** — `sum(player_health_up)` (green at 2, red below). Unit: short, optionally value-mapped `2 = "All up"`.
2. **EN Providers Enabled** — `sum(provider_enabled)` (shows `4` today). Threshold amber if low.
3. **EN Stage Health** — `sum(provider_health_up) / count(provider_health_up)` as a ratio (0–1, `percentunit`). Green at 1.0, amber below 1.0, red below 0.8.
4. **Canary Last Run** — `time() - scheduler_job_last_success_timestamp{job="scraper_playability_canary"}`. Unit: seconds (`s`), red if stale (> e.g. 2× cadence).
5. **Fallbacks (1h)** — `sum(increase(parser_fallback_total[1h]))`. Amber/red on rising count.

### Row 1 — Player / Parser Health (RU / JP / 18+)

1. **Player Liveness** (`state-timeline`) — `player_health_up`, legend `{{player}}`. (Merged from old Player dash; replaces its two separate Kodik/AnimeLib stat panels.)
2. **Parser Success Rate % by provider** (`timeseries`, percent unit, 0–1) —
   `sum by(provider)(rate(parser_requests_total{status="success"}[5m])) / sum by(provider)(rate(parser_requests_total[5m]))`. Legend `{{provider}}`.
3. **Parser p95 Latency by provider** (`timeseries`, unit `s`) —
   `histogram_quantile(0.95, sum by(le,provider)(rate(parser_request_duration_seconds_bucket[5m])))`. Legend `{{provider}}`.
4. **Player Health-Check Age** (`stat` or `timeseries`) — `time() - player_health_last_check_timestamp`, legend `{{player}}`, unit `s`. (Merged from old Player dash.)

### Row 2 — EN Scraper Provider Health (OurEnglish failover chain)

1. **Provider × Stage Up** (`state-timeline`) —
   `provider_health_up{provider=~"$provider",stage=~"$stage"}`, legend `{{provider}}/{{stage}}`. **The core upgrade**: uses the `provider` label the old Scraper/Health dashboard discarded (it had 5 per-stage stat panels only).
2. **Provider Enabled / Degraded** (`table` or `stat` grid) —
   `provider_enabled{provider=~"$provider"}`, value-mapped `1 = Enabled (green)`, `0 = Degraded (red)`. Makes `SCRAPER_DEGRADED_PROVIDERS` visible.
3. **Provider Fallbacks** (`timeseries`) —
   `sum by(from,to)(increase(parser_fallback_total[1h]))`, legend `{{from}} → {{to}}`. (From old Scraper/Health.)
4. **Probe Last Tick by provider** (`stat`/`timeseries`, unit `s`) —
   `time() - provider_probe_last_tick_timestamp{provider=~"$provider"}`, legend `{{provider}}`. (From old Scraper/Health.)

### Row 3 — EN Playability Canary

(Carried from old Scraper / Provider Health dashboard, queries unchanged except
`$provider` filter where it makes sense.)

1. **Pass / Fail per Provider (24h)** (`barchart`) —
   `sum by(provider,result)(increase(playability_canary_runs_total{provider=~"$provider"}[24h]))`.
2. **Failure Reason Breakdown (24h)** (`barchart`) —
   `sum by(reason)(increase(playability_canary_runs_total{result="fail"}[24h]))`.
3. **Last Canary Run** (`stat`) —
   `scheduler_job_last_success_timestamp{job="scraper_playability_canary"}`, displayed as age/timestamp.
4. **Top Failing (provider, server, reason)** (`table`) —
   `topk(10, sum by(provider,server,reason)(increase(playability_canary_runs_total{result="fail"}[24h])))`.

## Dedup / cut decisions

- Old Player dash: 2 separate Kodik/AnimeLib `stat` panels → collapsed into the
  Row 1 liveness state-timeline + Row 0 "RU Players Up" stat.
- Old Scraper/Health: 5 per-stage `stat` panels → collapsed into one Row 2
  provider×stage state-timeline (recovers the provider dimension).
- `parser_unplayable_total` excluded (no live data).
- Old `now-1h` default range widened to `now-6h` so 24h-window canary panels and
  rate trends render meaningfully.

## Visual polish / consistency rules

- Units set explicitly: ratios → `percentunit`, durations → `s`, counts → `short`.
- Consistent thresholds: up/healthy green, degraded amber, down/failing red.
- Every panel gets a one-line `description` (tooltip) stating what it measures and
  which provider namespace it belongs to.
- Legends use templated `{{label}}` formats, table/timeline placement consistent.
- Datasource object reused from the existing dashboards (no hardcoded foreign UID).

## Testing / verification

This is a provisioned-JSON change (no Go/TS code). Verification steps:

1. `python3 -m json.tool docker/grafana/dashboards/playback-health.json` — valid JSON.
2. Optional dashboard-schema sanity: confirm uid/title/schemaVersion present and
   each `targets[].expr` is non-empty.
3. `make restart-grafana` (config-only; no rebuild needed for a provisioned file —
   confirm whether the mount picks it up without restart; if not, restart).
4. Live smoke (`docs`/admin URL `animeenigma.ru/admin/grafana`, or local
   `localhost:3004`): open **Playback / Health**, confirm all rows render with
   data, `$provider`/`$stage` dropdowns populate and filter Rows 2–3, and the
   three old dashboards no longer appear in the dashboard list.
5. Each PromQL was pre-validated against live Prometheus during design; re-run the
   final exprs via `localhost:9090/prometheus/api/v1/query` to confirm non-empty
   results before declaring done.

## Post-implementation

Run `/animeenigma-after-update` (changelog entry: this is an admin/observability
change — Russian Trump-mode changelog entry, brief, factual). Commit with the
standard co-authors.
