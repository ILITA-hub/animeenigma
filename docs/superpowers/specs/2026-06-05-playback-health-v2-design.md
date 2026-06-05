# Playback / Health Dashboard — v2 Refinement

**Date:** 2026-06-05
**Status:** Design — approved, pending implementation
**Supersedes parts of:** `2026-06-05-playback-health-grafana-merge-design.md` (the v1 merge that produced the current `docker/grafana/dashboards/playback-health.json`)

## Goal

Second refinement pass on the unified **Playback / Health** dashboard, driven by a
live walkthrough. Folds the player-liveness concept into a single unified
**Provider Health** view, merges the standalone Scraper/Provider Management
dashboard in, makes fallback behavior readable, consolidates the canary row, and
fixes several correctness/display bugs found against live Prometheus.

## Findings that motivated the changes (verified live 2026-06-05)

- **AniLib in "RU Players":** `player_health_up{player="animelib"}` is emitted by
  catalog's `PlayerHealthChecker` (`health_checker.go`), which still probes
  AnimeLib every 5 min even though the AniLib *player* is hidden in the frontend
  (`VITE_ANIMELIB_ENABLED` off). Orphaned probe.
- **"56 years" canary age:** `time() - scheduler_job_last_success_timestamp{...}`
  returns ~`time()` (≈56y) when the gauge is `0` (canary never *succeeded* yet).
  Currently healthy (~4h) but the latent display bug remains.
- **Top Failing vs Pass/Fail "disagree":** both read `playability_canary_runs_total`
  and currently both evaluate to `0` — `increase([24h])` decays to ~0 between
  once-daily canary runs. The barchart renders a labeled zero bar (looks like
  data); the topk table of all-zeros renders blank. There has also never been a
  `result="pass"` sample, so "Pass/Fail per Provider" only ever shows fails.
- **Bracket windows not dropdown-controlled:** panels hardcode `[1h]`/`[24h]`
  windows that ignore the top time picker (`now-6h`). Misleading.
- **18anime pollutes EN aggregates:** `provider_enabled`/`provider_health_up`
  include `18anime` (the 18+ provider, not part of the EN failover chain),
  inflating "EN Providers Enabled" and "EN Stage Health".
- **Provider Fallbacks panel:** NOT empty — rich data (`gogoanime → allanime` etc.)
  — but the from→to stacked bars are unreadable. Empty-`to` rows (`miruro →`)
  mean "chain exhausted".

## Decisions (from the walkthrough)

1. **Unify into "Provider Health".** Rename the row
   "EN Scraper Provider Health (OurEnglish failover chain)" → **"Provider Health"**.
   Remove the "Player Liveness" panel and retire the `player_health_*` probes.
   Re-emit the **Kodik** probe as a provider so it appears in Provider Health.
2. **Canary row:** keep **only** the Top Failing tuple table (add columns as
   useful); delete the Pass/Fail and Failure-Reason barcharts. Fix the
   `increase()`-decay so the table populates.
3. **Merge** the Scraper/Provider Management dashboard's table in, then **delete**
   `infra/grafana/dashboards/scraper-providers.json`.
4. **Fallback transparency:** replace the from→to bars with **two indicators** —
   "Fallbacked from" (`by(from)`) and "Stopped fallbacking at" (`by(to)`).

## Backend change — catalog

`services/catalog/internal/service/health_checker.go` + `cmd/catalog-api/main.go`:

- **Drop the AnimeLib probe** (`checkAnimeLib`, `animelibClient`, `playerAnimeLib`).
  `NewPlayerHealthChecker` loses its `animelibClient` parameter; `main.go` stops
  passing `catalogService.AnimeLibClient()`.
- **Re-emit the Kodik probe as a provider** (metrics already in shared
  `libs/metrics/provider.go`):
  - `metrics.ProviderHealthUp.WithLabelValues("kodik", "liveness").Set(1|0)`
  - `metrics.ProviderProbeLastTick.WithLabelValues("kodik").SetToCurrentTime()`
  - once at startup: `metrics.ProviderEnabled.WithLabelValues("kodik").Set(1)` and
    `metrics.ProviderInfo.WithLabelValues("kodik", "RU iframe player",
    "Kodik RU iframe — liveness via Naruto search probe").Set(1)` so Kodik is a
    first-class row in the merged management table.
- **Stop emitting `player_health_*`** (`PlayerHealthUp`/`PlayerHealthLastCheck`/
  `PlayerHealthCheckDuration` calls removed). `libs/metrics/player_health.go` left
  in place but unused (no consumers after this change).
- Keep the checker type/loop, the 5-min interval, and the existing `checkKodik`
  reachability logic unchanged. → `make redeploy-catalog`.

> Hanime + Raw remain visible via the **Parser Performance** row
> (`parser_requests_total`); only Kodik gets a dedicated liveness probe as
> requested.

## Dashboard change — `docker/grafana/dashboards/playback-health.json`

**Global**
- Default time range → **`now-24h`** (so the daily canary + `$__range` panels
  render under the shared dropdown).
- Every windowed count/canary/fallback panel → **`[$__range]`** (the `[5m]`
  smoothing windows on rate/p95 panels stay — they are smoothing, not display
  range). Remove all `(1h)`/`(24h)` title suffixes.
- Exclude `18anime` from EN aggregates via `{provider!="18anime"}`.

**Row 0 — At-a-glance (5 stats)**
1. **Kodik Up** — `provider_health_up{provider="kodik"}`
2. **Providers Enabled** — `sum(provider_enabled{provider!="18anime",provider!="kodik"})`
3. **Stage Health** — `sum(provider_health_up{provider!="18anime"}) / clamp_min(count(provider_health_up{provider!="18anime"}),1)`
4. **Canary Age** — `time() - (scheduler_job_last_success_timestamp{exported_job="scraper_playability_canary"} > 0)` (guarded: returns no-data → "never", never 56y)
5. **Fallbacks** — `sum(increase(parser_fallback_total[$__range]))`

**Row 1 — rename → "Parser Performance (catalog parsers)"**
- Parser Success Rate % by provider (keep, `rate(...[5m])`)
- Parser p95 Latency by provider (keep, `rate(...[5m])`)
- **Remove** Player Liveness + Player Health-Check Age.

**Row 2 — rename → "Provider Health"**
- **Provider Management** table (merged): `provider_enabled` ⋈
  `max by(provider)(provider_health_up)` ⋈ `provider_info` →
  Provider / Enabled / Live Up / Reason / Description (joinByField + organize, color
  mappings). Includes Kodik.
- **Provider × Stage Up** state-timeline — overlap fixed (panel `h:12`, `rowHeight`
  ~0.6); incl. `kodik/liveness`.
- **Fallbacked from** — `sum by(from)(increase(parser_fallback_total[$__range]))`
  (horizontal barchart, sorted desc).
- **Stopped fallbacking at** — `sum by(to)(increase(parser_fallback_total[$__range]))`
  (empty `to` → value-mapped "(chain exhausted)").
- **Probe Last Tick** by provider (incl. Kodik).
- **Connect / Disconnect History** state-timeline (`provider_enabled`) — carried
  from the merged management dashboard.

**Row 3 — "Playability Canary"**
- **Top Failing (provider, server, reason, slot)** table — `topk(15, sum by
  (provider, server, reason, anime_slot)(increase(playability_canary_runs_total
  {result="fail"}[$__range])))`, instant table; gradient gauge on the count.
  (Add `anime_slot` column; range-aware window fixes the all-zeros problem given
  the `now-24h` default.)
- **Last Canary Run** stat (guarded).
- **Delete** the Pass/Fail and Failure-Reason barcharts.

**Template vars:** `$provider`, `$stage` unchanged (now also surface
`kodik`/`liveness`).

## Delete

- `infra/grafana/dashboards/scraper-providers.json` (uid
  `scraper-provider-management`) — its table is merged into Row 2. Verify the live
  Grafana evicts uid `scraper-provider-management` after redeploy.

## Verification

1. `go build ./...` in `services/catalog`; `python3 -m json.tool` the dashboard.
2. `make redeploy-catalog` + `make restart-grafana`.
3. Live PromQL: confirm `provider_health_up{provider="kodik"}` appears,
   `player_health_up` disappears, and each panel expr returns ≥1 series.
4. Browser smoke (DS-NF-06): Provider Health shows Kodik; canary table populated
   over 24h; fallback indicators readable; old management dashboard gone.
5. `/animeenigma-after-update` (Russian Trump-mode changelog, commit, push).
