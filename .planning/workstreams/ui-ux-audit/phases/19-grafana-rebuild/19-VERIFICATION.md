---
status: passed
phase: 19
phase_name: "Grafana dashboard rebuild (Kraken)"
verified: 2026-05-13
---

# Phase 19 Verification: Grafana dashboard rebuild (Kraken)

## Must-have truths scorecard (per 19-PLAN.md)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | All 9 dashboards have a top-level `title` matching `Area / Scope` format from the CONTEXT.md naming table | PASS | `grep -hE '"title": "(Services\|Player\|Recs\|Scraper) /' ...` returns 9 matches across all 9 files (see grep below). |
| 2 | `animeenigma-services` no longer has a row titled `"6. Users & Bandwidth"` (numbered prefix stripped) | PASS | `grep '"title":' docker/grafana/dashboards/animeenigma-services.json` no longer matches `^[0-9]+\.`; new title is `"Users & Bandwidth"`. |
| 3 | `content-preferences` and `preference-resolution` row titles no longer contain redundant time-window suffixes (`(Last 7 Days)`, `(Phase 1 Baseline)`) | PASS | `grep -E '\(Last 7 Days\)\|\(Phase 1 Baseline\)' docker/grafana/dashboards/*.json` returns no matches. |
| 4 | Panel-type appropriateness pass completed | PASS | Surveyed 38 timeseries / 27 stat / 1 gauge panels. 0 conversions made — all "ambiguous" candidates resolved as type-appropriate after deeper review (instant-gauge queries are valid timeseries shapes; fixed-window `increase(...[24h])` is a valid stat shape). Per 19-PLAN.md "If a panel's intent is ambiguous, leave it alone — false-negative is safer than wrong type." |
| 5 | All 9 dashboards have purpose-aware time defaults: live-ops `now-1h`/`30s`, service-overview `now-6h`/`1m`, recs `now-24h`/`5m`, aggregates `now-7d`/`5m` | PASS | Per-file dump matches the target matrix (see "Final inventory" table below). |
| 6 | `player-health` has a `refresh` field (previously absent) | PASS | `jq '.refresh' docker/grafana/dashboards/player-health.json` returns `"30s"` (was `null` pre-Phase-19). |
| 7 | All 9 JSON files parse cleanly with `jq .` | PASS | `for f in ...; do jq . "$f" > /dev/null && echo OK; done` returns 9 × OK. |
| 8 | Grafana container restarts cleanly with no dashboard provisioning errors | PASS | `docker compose -f docker/docker-compose.yml restart grafana` → `Container animeenigma-grafana Started`; `docker logs --since=2m grafana \| grep -iE 'error\|fail.*dashboard'` returns 0 dashboard errors. |
| 9 | Renamed dashboards visible via Grafana API with new titles | PASS | `curl -s -u admin:admin http://localhost:3004/api/search?type=dash-db \| jq -r '.[].title'` returns the 8 docker-provisioned dashboards (image-proxy is k8s-only) with the new `Area / Scope` titles. |

**Overall status:** PASSED — 9 / 9 must-have truths met.

## Artifact verification (per 19-PLAN.md "Files touched")

| Artifact | Path | Contains-check | Status |
|---|---|---|---|
| Services overview dashboard | `docker/grafana/dashboards/animeenigma-services.json` | `"title": "Services / Overview"` | FOUND |
| Content preferences dashboard | `docker/grafana/dashboards/content-preferences.json` | `"title": "Recs / Content Preferences"` | FOUND |
| Player health dashboard | `docker/grafana/dashboards/player-health.json` | `"title": "Player / Health"` + `"refresh": "30s"` | FOUND |
| Preference resolution dashboard | `docker/grafana/dashboards/preference-resolution.json` | `"title": "Player / Preference Resolution"` | FOUND |
| Rec engine dashboard | `docker/grafana/dashboards/rec-engine.json` | `"title": "Recs / Engine"` | FOUND |
| Scraper health dashboard | `docker/grafana/dashboards/scraper-health.json` | `"title": "Scraper / Health"` | FOUND |
| Watch activity dashboard | `docker/grafana/dashboards/watch-activity.json` | `"title": "Player / Watch Activity"` | FOUND |
| Scraper provider health canary | `infra/grafana/dashboards/scraper-provider-health.json` | `"title": "Scraper / Provider Health"` | FOUND |
| Image proxy k8s dashboard | `deploy/kustomize/grafana/dashboards/image-proxy.json` | `"title": "Services / Image Proxy"` | FOUND |
| Phase summary | `.planning/workstreams/ui-ux-audit/phases/19-grafana-rebuild/19-SUMMARY.md` | `Phase 19 outcome: PASSED` | FOUND (written this run) |

## Test results

### JSON parse-ability (per plan)

```
$ for f in docker/grafana/dashboards/*.json infra/grafana/dashboards/scraper-provider-health.json deploy/kustomize/grafana/dashboards/image-proxy.json; do
    jq . "$f" > /dev/null && echo "OK: $f" || echo "FAIL: $f"
  done
OK: docker/grafana/dashboards/animeenigma-services.json
OK: docker/grafana/dashboards/content-preferences.json
OK: docker/grafana/dashboards/player-health.json
OK: docker/grafana/dashboards/preference-resolution.json
OK: docker/grafana/dashboards/rec-engine.json
OK: docker/grafana/dashboards/scraper-health.json
OK: docker/grafana/dashboards/watch-activity.json
OK: infra/grafana/dashboards/scraper-provider-health.json
OK: deploy/kustomize/grafana/dashboards/image-proxy.json
```

All 9 dashboards parse-clean.

### No leftover empty / numbered row titles (per plan)

```
$ grep -h '"title":' docker/grafana/dashboards/*.json | grep -E '"---"|"Row [0-9]"'
(no matches)
```

Clean.

### Area / Scope title count (per plan, threshold ≥7)

```
$ grep -hE '"title": "(Services|Player|Recs|Scraper) /' \
    docker/grafana/dashboards/*.json \
    infra/grafana/dashboards/scraper-provider-health.json \
    deploy/kustomize/grafana/dashboards/image-proxy.json | wc -l
9
```

9 ≥ 7 — well above the plan threshold.

### Per-dashboard title / time / refresh dump

```
$ for f in docker/grafana/dashboards/*.json infra/grafana/dashboards/scraper-provider-health.json deploy/kustomize/grafana/dashboards/image-proxy.json; do
    echo "=== $(basename "$f") ==="
    jq -r '"\(.title) | \(.time) | refresh=\(.refresh)"' "$f"
  done
=== animeenigma-services.json ===
Services / Overview | {"from":"now-6h","to":"now"} | refresh=1m
=== content-preferences.json ===
Recs / Content Preferences | {"from":"now-7d","to":"now"} | refresh=5m
=== player-health.json ===
Player / Health | {"from":"now-1h","to":"now"} | refresh=30s
=== preference-resolution.json ===
Player / Preference Resolution | {"from":"now-24h","to":"now"} | refresh=5m
=== rec-engine.json ===
Recs / Engine | {"from":"now-24h","to":"now"} | refresh=5m
=== scraper-health.json ===
Scraper / Health | {"from":"now-1h","to":"now"} | refresh=30s
=== watch-activity.json ===
Player / Watch Activity | {"from":"now-7d","to":"now"} | refresh=5m
=== scraper-provider-health.json ===
Scraper / Provider Health | {"from":"now-1h","to":"now"} | refresh=30s
=== image-proxy.json ===
Services / Image Proxy | {"from":"now-6h","to":"now"} | refresh=1m
```

Each row matches the CONTEXT.md target matrix exactly.

### Grafana provisioning verification

```
$ docker compose -f docker/docker-compose.yml restart grafana
 Container animeenigma-grafana Restarting
 Container animeenigma-grafana Started

$ docker compose -f docker/docker-compose.yml logs --since=2m grafana | grep -iE 'error|fail|dashboard'
(no dashboard/error matches — only the pre-existing "Failed to read plugin provisioning files" and
 "Can't read alert notification provisioning files" warnings for plugins/notifiers directories that
 don't exist; both are unrelated to dashboard JSON and are pre-existing on this host.)

$ docker compose -f docker/docker-compose.yml ps grafana
NAME                  IMAGE                    STATUS          PORTS
animeenigma-grafana   grafana/grafana:10.3.3   Up 18 seconds   127.0.0.1:3004->3000/tcp
```

Grafana healthy after restart. Dashboard JSON loaded cleanly.

### Grafana API spot-check

```
$ curl -s -u admin:admin "http://localhost:3004/api/search?type=dash-db" | jq -r '.[].title'
Player / Health
Player / Preference Resolution
Player / Watch Activity
Recs / Content Preferences
Recs / Engine
Scraper / Health
Scraper / Provider Health
Services / Overview
```

8 dashboards visible with new `Area / Scope` titles. `Services / Image Proxy` is k8s-only (lives in `deploy/kustomize/`) and is not provisioned in the docker-compose Grafana — that is expected.

## Commits on `main`

| Commit | Subject |
|---|---|
| `c745568` | `feat(ui-ux-audit/19): Grafana dashboard naming pass (UA-116)` |
| `7f16373` | `chore(ui-ux-audit/19): remove empty rows + rename numbered rows (UA-117/UA-118)` |
| `661ecdd` | `feat(ui-ux-audit/19): time-range defaults per dashboard purpose (UA-120)` |

3 atomic Phase-19 commits; each independently revertable. **Pass 3 (UA-119 panel-type appropriateness) produced 0 conversions — no Pass-3 commit, documented in 19-SUMMARY.md.**

## Audit-finding closure

| Finding | Surface | Mechanism | Status |
|---|---|---|---|
| UA-116 | All 9 dashboards | Top-level `title` standardized to `Area / Scope` format — Services, Player, Recs, Scraper | CLOSED |
| UA-117 | Multi-row dashboards | Empty-row check completed; `Service Overview` in `animeenigma-services` retained because it positionally owns 3 top-level stat panels at `gridPos.y=1` (uncollapsed-row layout, not actually empty); 0-row dashboards (`rec-engine`, `scraper-health`, `scraper-provider-health`, `image-proxy`) are no-op for this pass per execution-scope policy | CLOSED |
| UA-118 | `animeenigma-services` | Numbered prefix stripped: `"6. Users & Bandwidth"` → `"Users & Bandwidth"` | CLOSED |
| UA-119 | All 66 stat / timeseries / gauge panels | Panel-type appropriateness audit completed. 0 conversions: every flagged candidate resolved as correct after deeper review (instant-gauge queries are valid timeseries shapes when Grafana resamples them; `increase(...[fixed-window])` returns a single value per series and fits stat). Per plan rule "If a panel's intent is ambiguous, leave it alone." | CLOSED |
| UA-120 | All 9 dashboards | Purpose-aware time defaults applied: live-ops (`now-1h`/`30s`), service-overview (`now-6h`/`1m`), recs (`now-24h`/`5m`), aggregates (`now-7d`/`5m`). `player-health` gains a `refresh` field for the first time (`30s`). | CLOSED |

**Phase 19 outcome:** PASSED — 5 / 5 audit findings closed across 9 dashboard JSON files. Zero panels added, zero queries changed, zero new dependencies. Grafana provisioning reloaded successfully; 8 dashboards visible via API with the new naming scheme (image-proxy is k8s-only).
