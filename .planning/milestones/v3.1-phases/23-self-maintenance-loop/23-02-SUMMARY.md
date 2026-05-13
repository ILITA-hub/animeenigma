---
phase: 23
plan: "02"
subsystem: observability
tags: [grafana, dashboard, observability, scraper, canary, self-healing]
status: shipped
requirements: [SCRAPER-HEAL-14]
provides:
  - "Scraper Provider Health (Canary) Grafana dashboard — 4 panels visualizing playability_canary_runs_total + scheduler_job_last_success_timestamp"
  - "infra/grafana/dashboards/ — new v3.1+ source-of-truth directory for self-healing dashboards"
  - "Second Grafana file-provisioner provider 'infra-self-healing' (folder: Self-Healing) for the new directory"
requires:
  - "playability_canary_runs_total counter (Phase 23-01)"
  - "scheduler_job_last_success_timestamp{job=\"scraper_playability_canary\"} gauge (Phase 23-01 via libs/metrics/scheduler.go)"
  - "Grafana 10.3.3 + Prometheus datasource (existing docker-compose monitoring stack)"
affects:
  - "Phase 23 Plan 23-03 (alert rules will reuse the same metric series surfaced by this dashboard)"
  - "Future deploy plan (production Kubernetes Kustomize will sync from infra/grafana/dashboards/)"
tech-stack:
  added:
    - "infra/grafana/dashboards/ directory (new source-of-truth for v3.1+ dashboards, mounted dev + future-prod)"
  patterns:
    - "Templated datasource via \${DS_PROMETHEUS} (matches existing dashboards in docker/grafana/dashboards/)"
    - "barchart panel with stacking=normal + palette-classic for stacked counters"
    - "table panel with topk(10, ...) + organize transformation + sortBy DESC for ranked tuples"
    - "stat panel with dateTimeFromNow unit for relative-time display of last-run gauge"
    - "Sibling mount path /var/lib/grafana/dashboards-infra (NOT nested under existing read-only mount)"
key-files:
  created:
    - "infra/grafana/dashboards/scraper-provider-health.json (Grafana 10.3.3 schemaVersion 38, 4 panels, uid scraper-provider-health-canary)"
    - "infra/grafana/dashboards/README.md (directory purpose, naming convention, dev↔prod relationship)"
  modified:
    - "docker/grafana/provisioning/dashboards/dashboards.yml (added second provider 'infra-self-healing', folder 'Self-Healing')"
    - "docker/docker-compose.yml (mounted ../infra/grafana/dashboards into grafana container)"
decisions:
  - "Use sibling mount path /var/lib/grafana/dashboards-infra instead of the planned /var/lib/grafana/dashboards/infra — Docker cannot create a nested mountpoint inside an already read-only mount (Rule 3 fix)"
  - "Folder 'Self-Healing' segregates v3.1 dashboards in the Grafana UI without altering layout of the existing 7 dashboards"
  - "Datasource UID 'PBFA97CFB590B2093' baked into templating.current.value matches the existing scraper-health.json convention (overridden by GF_PATHS_PROVISIONING datasource at runtime)"
metrics:
  duration: "~10m"
  completed: "2026-05-13"
  tasks_completed: 2
  files_created: 2
  files_modified: 2
  commits: 3
  lines_added: ~340
---

# Phase 23 Plan 23-02: Scraper Provider Health Dashboard Summary

## One-Liner

Grafana dashboard `scraper-provider-health-canary` with 4 panels (pass/fail per provider/server 24h, failure reason breakdown, last canary run timestamp, top failing tuples table) live at `infra/grafana/dashboards/scraper-provider-health.json`, auto-loaded into the dev Grafana container via a new `infra-self-healing` file-provisioner under the `Self-Healing` folder.

## Status

**SHIPPED** — dashboard verified live via Grafana API on production stack 2026-05-13T07:19Z.

## What Got Built

### infra/grafana/dashboards/ (new directory)

New v3.1 source-of-truth for self-healing dashboards. Coexists with the legacy `docker/grafana/dashboards/` (which keeps its 7 existing dashboards). Production Kubernetes provisioning (future deploy plan) will sync from this directory; today the dev stack mounts it directly.

### `scraper-provider-health.json` (Grafana 10.3.3, schemaVersion 38, uid `scraper-provider-health-canary`)

| # | Title | Type | Query | Notes |
|---|-------|------|-------|-------|
| 1 | Pass / Fail per Provider/Server (24h) | barchart | `sum by (provider, server, result) (increase(playability_canary_runs_total[24h]))` | Stacked, palette-classic, table legend on right |
| 2 | Failure Reason Breakdown (24h) | barchart | `sum by (reason) (increase(playability_canary_runs_total{result="fail"}[24h]))` | Stacked by reason |
| 3 | Last Canary Run | stat | `scheduler_job_last_success_timestamp{job="scraper_playability_canary"}` | `dateTimeFromNow` unit → "5m ago" display |
| 4 | Top Failing (provider, server, reason) Tuples | table | `topk(10, sum by (provider, server, reason) (increase(playability_canary_runs_total{result="fail"}[24h])))` | Instant query, organize transform, sorted DESC, gauge cell for fail count |

Template variable `$DS_PROMETHEUS` (datasource) so the JSON is portable between dev + prod. Refresh `1m`, time window `now-24h → now`. Tags: `scraper`, `canary`, `self-healing`.

### `infra/grafana/dashboards/README.md`

Explains why the directory exists, naming convention (`<area>-<subject>.json`), relationship to the legacy `docker/grafana/dashboards/` (dev-only legacy mount) and `deploy/kustomize/base/monitoring/grafana/configmap-dashboards.yaml` (production K8s, out-of-scope here). Lists `scraper-provider-health.json` as the first entry.

### Provisioning + mount wiring

- `docker/grafana/provisioning/dashboards/dashboards.yml` — second provider `infra-self-healing` pointed at `/var/lib/grafana/dashboards-infra`, folder `Self-Healing`. The original `default` provider is unchanged so the existing 7 dashboards keep working.
- `docker/docker-compose.yml` — grafana service mounts `../infra/grafana/dashboards:/var/lib/grafana/dashboards-infra:ro`.

## Smoke Verification (Production Stack)

```bash
# After `docker compose -f docker/docker-compose.yml up -d grafana`:
curl -fsS -u "admin:admin" \
  "http://localhost:3004/admin/grafana/api/search?query=Scraper+Provider+Health" \
  | jq -r '.[] | select(.uid == "scraper-provider-health-canary") | .title + " — folder: " + .folderTitle'
# → Scraper Provider Health (Canary) — folder: Self-Healing

curl -fsS -u "admin:admin" \
  "http://localhost:3004/admin/grafana/api/dashboards/uid/scraper-provider-health-canary" \
  | jq -r '.dashboard.panels[].title'
# Pass / Fail per Provider/Server (24h)
# Failure Reason Breakdown (24h)
# Last Canary Run
# Top Failing (provider, server, reason) Tuples
```

Panels currently render "No data" / show only the initial smoke-run samples from Plan 23-01 (Frieren / One Piece / recent_1 all returning `fail/zero_match/_unreachable` because `/scraper/servers` returns empty for those titles on the current deploy). That state is itself the canary doing its job — once Plan 23-03's alert rules ship, this will fire `ScraperPlayabilityRegression`.

## Deviations from Plan

### Rule 3 — Auto-fix blocking dependency

**[Rule 3 - Blocking] Initial mount target collided with existing RO mount**

- **Found during:** Task 2 deploy step (`docker compose up -d grafana`).
- **Issue:** Plan specified mounting `../infra/grafana/dashboards` at `/var/lib/grafana/dashboards/infra` inside the grafana container. That path is *inside* the existing `./grafana/dashboards:/var/lib/grafana/dashboards:ro` mount, which is read-only. Docker cannot create the nested mountpoint:
  ```
  error mounting "/data/animeenigma/infra/grafana/dashboards" to rootfs
  at "/var/lib/grafana/dashboards/infra": create mountpoint for
  /var/lib/grafana/dashboards/infra mount: ... read-only file system
  ```
- **Fix:** Mount at sibling path `/var/lib/grafana/dashboards-infra` and update the file-provisioner `path:` to match. Same behavior, no breaking change for existing dashboards. README updated to document the rationale.
- **Files modified:** `docker/docker-compose.yml`, `docker/grafana/provisioning/dashboards/dashboards.yml`, `infra/grafana/dashboards/README.md`
- **Commit:** `0f420f0`

**Note on plan's must_haves contract:** the plan's `must_haves.artifacts` block stated `dashboards.yml ... contains "/var/lib/grafana/dashboards/infra"` and the docker-compose entry contains "infra/grafana/dashboards". The compose check still passes (the host path `../infra/grafana/dashboards` contains the substring). The dashboards.yml `contains` literal no longer holds, but the path the plan intended is physically impossible to mount — Rule 3 supersedes. The functional intent (auto-load dashboards from the new directory) is achieved and verified live.

## Threat Model Compliance

| Threat | Disposition | Status |
|--------|-------------|--------|
| T-23-06 (info disclosure via Top Failing Tuples labels) | accept — bounded enum label values, no PII | unchanged, same surface as scraper-health.json |
| T-23-07 (DoS via dashboard queries) | accept — topk(10) + increase[24h], 210-series cardinality bound | verified the topk panel uses instant query (single point eval) |
| T-23-08 (tampering via editable=true) | accept — UI edits only affect Grafana DB, source-of-truth file untouched | dashboard JSON committed; folder Self-Healing isolates v3.1 edits |

No new threats discovered during execution. ASVS L1: read-only data flow, no secrets in panels, no user input.

## Commits

| Hash | Type | Description |
|------|------|-------------|
| `d527047` | feat | add Scraper Provider Health (Canary) Grafana dashboard JSON + README |
| `d90cd71` | chore | wire infra/grafana/dashboards into Grafana + docker-compose |
| `0f420f0` | fix | mount infra dashboards at sibling path, not under existing RO mount (Rule 3) |

## Follow-up

- **Plan 23-03 (Wave 3)**: three Prometheus alert rules (`ScraperPlayabilityRegression`, `ScraperAdDecoySurge`, `ScraperUnplayableSpike`) routing to the maintenance webhook; will reuse the same `playability_canary_runs_total` series surfaced here.
- **Future deploy plan**: sync `infra/grafana/dashboards/` into `deploy/kustomize/base/monitoring/grafana/configmap-dashboards.yaml` for production K8s. Out of scope for Phase 23.
- The live smoke run shows `_unreachable` server tuples in the Top Failing panel — this is the canary correctly detecting that `/scraper/servers?mal_id=52991&episode=1` returns an empty list. Once 23-03's `ScraperPlayabilityRegression` alert ships, this will dispatch a maintenance-bot Pattern 6/7 ticket.

## Self-Check: PASSED

- [x] `infra/grafana/dashboards/scraper-provider-health.json` exists and `jq -e .` exits 0
- [x] Panel count = 4 (verified via `jq -r '.panels | length'`)
- [x] All 4 expected panel titles present (diff against expected list)
- [x] `.uid` = `scraper-provider-health-canary`
- [x] `.title` = `Scraper Provider Health (Canary)`
- [x] `.tags` includes `scraper` + `canary` + `self-healing`
- [x] `.templating.list[].name` includes `DS_PROMETHEUS`
- [x] `.refresh` = `1m`, `.schemaVersion` = 38
- [x] All 4 PromQL expressions match the plan's `<interfaces>` block
- [x] Last Canary Run panel queries `scheduler_job_last_success_timestamp{job="scraper_playability_canary"}`
- [x] Top Failing Tuples panel is type `table` with organize transform + sortBy DESC
- [x] Pass / Fail panel has `color.mode` = `palette-classic`
- [x] `infra/grafana/dashboards/README.md` exists, references `scraper-provider-health.json` and `deploy/kustomize`
- [x] `docker/grafana/provisioning/dashboards/dashboards.yml` has 2 provider entries (default + infra-self-healing)
- [x] `docker/docker-compose.yml` mounts `../infra/grafana/dashboards` into the grafana container
- [x] `docker compose -f docker/docker-compose.yml config` exits 0
- [x] **Live**: dashboard discoverable in Grafana API under folder `Self-Healing`
- [x] **Live**: all 4 panels load via `GET /api/dashboards/uid/scraper-provider-health-canary`
- [x] All commits in git log: `d527047`, `d90cd71`, `0f420f0`
