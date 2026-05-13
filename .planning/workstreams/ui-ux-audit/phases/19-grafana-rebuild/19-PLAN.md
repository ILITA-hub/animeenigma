# Phase 19 Plan: Grafana dashboard rebuild (Kraken)

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Scope: 9 dashboard JSON files. Hygiene-only changes (no panels added/removed, no queries changed). Closes UA-116, UA-117, UA-118, UA-119, UA-120.

## Tasks

### Naming pass (UA-116)

- [ ] Apply `Area / Scope` titles to all 9 dashboards per CONTEXT.md naming table:
  - `animeenigma-services.json` → `"Services / Overview"`
  - `content-preferences.json` → `"Recs / Content Preferences"`
  - `player-health.json` → `"Player / Health"`
  - `preference-resolution.json` → `"Player / Preference Resolution"`
  - `rec-engine.json` → `"Recs / Engine"`
  - `scraper-health.json` → `"Scraper / Health"`
  - `watch-activity.json` → `"Player / Watch Activity"`
  - `infra/grafana/dashboards/scraper-provider-health.json` → `"Scraper / Provider Health"`
  - `deploy/kustomize/grafana/dashboards/image-proxy.json` → `"Services / Image Proxy"`

### Empty rows + row titles pass (UA-117, UA-118)

- [ ] Iterate dashboards with `>0` row panels. For each row:
  - If row has no panel children OR title is empty/literal `"---"`/`"Row N"`, either delete (no children) OR rename to a descriptive title.
  - Use these descriptive titles where rename is needed:
    - `animeenigma-services.json` rows: "Service Health", "Latency", "Throughput", "Cache Hit Rate", "DB Queries", "Resource Use"
    - `content-preferences.json` rows: "Audience Splits", "Translation Rankings"
    - `player-health.json` rows: "Stream Health", "Resume Behavior", "Drop-off"
    - `preference-resolution.json` rows: "Resolution Overview", "Fallback", "Override Rate", "Time Series"
    - `watch-activity.json` rows: "Daily Activity", "User Engagement"
- [ ] Manually verify each dashboard's row count post-edit matches `expected rows` per CONTEXT.md inventory.

### Panel-type appropriateness pass (UA-119)

- [ ] For each timeseries panel, verify its `targets[].expr` includes a time-bucketed function (`rate`, `increase`, `irate`, or `$__interval` reference). If a timeseries panel has a single-value query (`sum(...)` without time bucketing), convert to `stat`.
- [ ] For each stat panel, verify its query produces a single value (no `range` step, no `[$__interval]`). If a stat panel has a time-bucketed query, convert to `timeseries`.
- [ ] Document the count of panels reviewed / converted in SUMMARY.

### Time-range defaults pass (UA-120)

- [ ] Apply per-dashboard time defaults in JSON `time: { from: "now-Xh", to: "now" }`:
  - Live ops (`player-health`, `scraper-health`, `scraper-provider-health`): `now-1h` + `refresh: "30s"`
  - Aggregates (`content-preferences`, `watch-activity`): `now-7d` + `refresh: "5m"`
  - Service overview (`animeenigma-services`, `image-proxy`): `now-6h` + `refresh: "1m"`
  - Recs (`rec-engine`, `preference-resolution`): `now-24h` + `refresh: "5m"`

### Verification

- [ ] `jq . docker/grafana/dashboards/*.json` exits clean for each file (parseable JSON).
- [ ] `jq . infra/grafana/dashboards/scraper-provider-health.json` clean.
- [ ] `jq . deploy/kustomize/grafana/dashboards/image-proxy.json` clean.
- [ ] `grep -h '"title":' docker/grafana/dashboards/*.json | grep -v '"---"\|"Row [0-9]"'` returns clean (no leftover empty/numbered row titles).
- [ ] `grep -hE '"title": "(Services|Player|Recs|Scraper) /' docker/grafana/dashboards/*.json | wc -l` ≥ 7 (one per file).
- [ ] `docker compose -f docker/docker-compose.yml restart grafana` succeeds; Grafana logs show dashboards loaded without error.
- [ ] Manual smoke (optional): browse to Grafana UI, verify renamed dashboards appear with new titles and correct time ranges.

## Files touched

```
docker/grafana/dashboards/animeenigma-services.json
docker/grafana/dashboards/content-preferences.json
docker/grafana/dashboards/player-health.json
docker/grafana/dashboards/preference-resolution.json
docker/grafana/dashboards/rec-engine.json
docker/grafana/dashboards/scraper-health.json
docker/grafana/dashboards/watch-activity.json
infra/grafana/dashboards/scraper-provider-health.json
deploy/kustomize/grafana/dashboards/image-proxy.json
.planning/workstreams/ui-ux-audit/phases/19-grafana-rebuild/
  19-CONTEXT.md
  19-PLAN.md
  19-SUMMARY.md       (written at execute end)
  19-VERIFICATION.md  (written at execute end)
```

## Closes

| Finding | Surface | Mechanism |
|---|---|---|
| UA-116 | All dashboards | Standardized `Area / Scope` title format |
| UA-117 | Multi-row dashboards | Empty rows removed |
| UA-118 | Multi-row dashboards | Numbered rows renamed to descriptive titles |
| UA-119 | All panels | Panel-type appropriateness audit (timeseries vs stat) |
| UA-120 | All dashboards | Time-range defaults standardized per dashboard purpose |
