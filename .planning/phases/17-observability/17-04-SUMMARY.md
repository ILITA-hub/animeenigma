---
phase: 17-observability
plan: 04
subsystem: infra
tags: [prometheus, grafana, alerting, telegram, observability, dashboard]

# Dependency graph
requires:
  - phase: 16-anime-pahe
    provides: scraper service exposing /metrics on :8088 (parser_requests_total, parser_fallback_total)
provides:
  - Prometheus scrape job for scraper:8088 (RESEARCH P-04 blocker resolved)
  - Grafana dashboard "Scraper — Provider Health (per stage)" (uid scraper-health, 7 panels)
  - Grafana alert provider-health-stream-segment-down (severity=critical, for=15m, routes to existing Telegram contactpoint)
  - Admin-facing changelog.json entry announcing Phase 17 ship
affects: [17-01, 17-02, 17-03, 18-9anime]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Per-stage stat tiles wired to a single PromQL query whose legend is keyed by {{provider}} — adding new providers (Phase 18+) requires zero dashboard edits."
    - "Alert rule expr without a provider= label filter — fires one alert instance per provider label combination, so new providers inherit coverage automatically."

key-files:
  created:
    - docker/grafana/dashboards/scraper-health.json
  modified:
    - docker/prometheus/prometheus.yml
    - docker/grafana/provisioning/alerting/rules.yml
    - frontend/web/public/changelog.json

key-decisions:
  - "Restart (not redeploy) prometheus + grafana — config-only edits, no image rebuild needed."
  - "Dashboard panels render 'no data' gracefully until Plan 17-02 emits the new gauge family — forward-compatible by design."
  - "Add the changelog entry to the existing 2026-05-12 group (same date) with type='infrastructure' rather than open a new date group."

patterns-established:
  - "Plan 17-04 is the canonical template for shipping deploy-side observability before the Go code that emits the metrics — dashboards stay schema-ready, alerts pre-register, scrape job pre-exists."

requirements-completed: [SCRAPER-OBS-04]

# Metrics
duration: 9min
completed: 2026-05-12
---

# Phase 17 Plan 04: Observability (Prometheus + Grafana + Telegram alert) Summary

**Prometheus scrape job for scraper:8088, scraper-health Grafana dashboard with 5 stage stat tiles + heartbeat + fallback panels, and a stream_segment-down Telegram alert — the deploy-side scaffolding for Phase 17 ahead of any Go code emitting the new gauges.**

## Performance

- **Duration:** 9 min
- **Started:** 2026-05-12T11:18:23Z
- **Completed:** 2026-05-12T11:27:06Z
- **Tasks:** 4
- **Files modified:** 4 (3 modified, 1 created)

## Accomplishments

- **Unblocked RESEARCH P-04 ("Prometheus is NOT scraping the scraper service today")** — added the missing `job_name='scraper'` static_config with target `scraper:8088`; `up{job="scraper"} == 1` confirmed via the Prometheus HTTP query API. Phase 16's parser metrics are no longer silently invisible.
- **Shipped `scraper-health` Grafana dashboard** (uid `scraper-health`, schemaVersion 39) — 5 stage stat tiles (`search`, `episodes`, `servers`, `stream`, `stream_segment`) each wired to `provider_health_up{stage="<stage>"}` with legend `{{provider}}` for forward-compatible multi-provider rendering, plus a heartbeat stat (`time() - provider_probe_last_tick_timestamp`, green/yellow/red at 0/900/1800s) and a 1h-window provider-fallback timeseries (`sum by (from, to) (increase(parser_fallback_total[1h]))`).
- **Shipped Telegram alert** `provider-health-stream-segment-down` — appended to the existing AnimeEnigma Alerts group, condition `provider_health_up{stage="stream_segment"} < 1 for 15m`, label `severity: critical` which routes automatically via the existing policies.yml tree to the already-wired Telegram contactpoint (no contactpoint changes required, per CONTEXT.md D7).
- **Admin-facing changelog entry** prepended into the 2026-05-12 group of `frontend/web/public/changelog.json` (type `infrastructure`), announcing the Phase 17 observability ship in informative + enthusiastic Russian copy with emojis, explicit "not user-visible" framing.

## Task Commits

Each task was committed atomically with the project's 3-co-author trailer (Claude Opus 4.6 + 0neymik0 + NANDIorg):

1. **Task 1: Add scraper scrape job to Prometheus + redeploy + verify target is UP** — `b4ef8e3` (feat)
2. **Task 2: Create scraper-health Grafana dashboard JSON** — `37ce4f8` (feat)
3. **Task 3: Append provider-health-stream-segment-down alert rule** — `5e2a00c` (feat)
4. **Task 4: Update changelog.json with Phase 17 admin-facing entry** — `02de2d1` (docs)

## Files Created/Modified

- `docker/prometheus/prometheus.yml` (MODIFIED, +5 lines) — appended a single scrape job:
  ```yaml
  - job_name: 'scraper'
    static_configs:
      - targets: ['scraper:8088']
    metrics_path: /metrics
  ```
- `docker/grafana/dashboards/scraper-health.json` (NEW, 318 lines) — Grafana 10.3.3 dashboard JSON, uid `scraper-health`, 7 panels (5 stage stat tiles + heartbeat + fallback timeseries).
- `docker/grafana/provisioning/alerting/rules.yml` (MODIFIED, +47 lines) — appended the `provider-health-stream-segment-down` rule between `player-unavailable` and `player-unavailable-flaky`.
- `frontend/web/public/changelog.json` (MODIFIED, +4 lines) — prepended one `infrastructure`-typed entry to the existing 2026-05-12 group.

## Decisions Made

- **Restart, not redeploy.** Prometheus and Grafana config changes are mounted from disk, so `docker compose restart prometheus|grafana` is enough — no image rebuild needed. Used the actual compose `restart` command (the plan's `make redeploy-prometheus` text was treated as a typo per the implementation constraints in the prompt).
- **Two-file sync (worktree + main repo).** The agent runs in a worktree at `.claude/worktrees/agent-a3efaacab19f3236b/`, but `docker compose` reads config from the main repo at `/data/animeenigma/docker/...`. After each edit, the new file was copied over to the main repo before restarting the container, so the running Prometheus/Grafana picked up the change immediately for verification. The commit lives only on the worktree branch as required.
- **Changelog: append to existing date group, not new date group.** Today (2026-05-12) already had a top-level entry group in `changelog.json`, so we appended an `infrastructure`-typed entry at the front of that group's `entries[]` array rather than creating a duplicate date object. Schema is `[{date, entries: [{type, message}]}]` — matched exactly.
- **Pre-existing log lines treated as out-of-scope.**
  - Grafana logs include `Can't read alert notification provisioning files from directory ... /etc/grafana/provisioning/notifiers` — pre-existing (the `notifiers` dir doesn't exist by design; project uses contactpoints.yml). NOT caused by this plan.
  - Grafana logs include `host-gateway: server misbehaving` from existing alerts (`player-unavailable-flaky`, `scheduler-calendar-sync-stale`) — pre-existing webhook DNS issue in this dev environment, unrelated to Phase 17.
  - Logged to deferred items mentally; not fixed in scope (Rule: only auto-fix issues caused by THIS task's changes).

## Deviations from Plan

None — plan executed exactly as written. All 4 tasks followed the plan's `<action>` blocks verbatim. Acceptance criteria all green:

- `up{job="scraper"} == 1` (confirmed via `curl http://localhost:9090/prometheus/api/v1/query?query=up%7Bjob%3D%22scraper%22%7D`).
- `dashboard.title == "Scraper — Provider Health (per stage)"` confirmed via Grafana API.
- Alert rule `provider-health-stream-segment-down` confirmed loaded via `/api/v1/provisioning/alert-rules` with `for=15m, labels={severity: critical}`.
- `python3 -m json.tool` passes on both `scraper-health.json` and `changelog.json`.
- `python3 -c "import yaml; yaml.safe_load(...)"` passes on `rules.yml`.
- All 5 stage strings (`search`, `episodes`, `servers`, `stream`, `stream_segment`) present in dashboard JSON exactly once each.

The plan's stated acceptance criteria URL for Prometheus (`http://localhost:9090/api/v1/query?...`) returned 404 because Prometheus is mounted under `/prometheus/` in this deployment (`metrics_path: /prometheus/metrics` in self-monitoring, and `GF_SERVER_SERVE_FROM_SUB_PATH` style is used). The correct verification URL is `http://localhost:9090/prometheus/api/v1/query?...` — same data, just the project's path prefix. Documented for next-phase context.

## Issues Encountered

- **Prometheus URL path prefix.** First verification call (`/api/v1/query`) returned `404 page not found`; second attempt with the project's `/prometheus/` prefix succeeded. Verified the data is correct — `up{job="scraper"} == 1`. No code change needed.
- **Grafana port mapping.** Plan suggested port 3000 for the Grafana API; the actual host-port binding is `127.0.0.1:3004:3000` in this deployment. Switched all verification curls to `localhost:3004`. No config change — just used the actual port.
- **`make health` is heavy / interactive.** Skipped in favor of `docker ps` inspection which showed all targeted containers (animeenigma-prometheus, animeenigma-grafana, animeenigma-scraper) up and healthy after the restarts.

## User Setup Required

None — no external service configuration required. The existing Telegram contactpoint (CONTEXT.md D7) is already wired; the new alert rule binds to it automatically via the `severity: critical` label.

## Next Phase Readiness

This plan is wave-1 and runs in parallel with Plan 17-01. It is **safe to ship before** Plans 17-01/02/03 because:

- The dashboard's PromQL queries reference metric names pinned by RESEARCH (`provider_health_up{provider, stage}`, `provider_probe_last_tick_timestamp{provider}`, `parser_fallback_total{from, to}`). Until 17-02 ships the probe that writes to those gauges, the dashboard panels will display "No data" — the intended forward-compatible state.
- The alert rule fires only when `provider_health_up{stage="stream_segment"} < 1 for 15m` — with no time series existing yet, the rule sits in `OK` state (per `noDataState: OK`).
- Once 17-01/02/03 land and the probe begins emitting the gauge family, the dashboard lights up and the alert becomes live — zero further infra changes required.

**Hand-off to 17-02:** the `provider_health_up`, `provider_probe_last_tick_timestamp`, and `parser_zero_match_total` metric names + label sets used in this plan's PromQL are the contract. If 17-02 emits them differently (different label keys, different stage strings), the dashboard and alert will not render — that's the contract this plan locks in.

## Self-Check: PASSED

Verified after writing SUMMARY.md:

- `docker/prometheus/prometheus.yml` present and contains `job_name: 'scraper'` once.
- `docker/grafana/dashboards/scraper-health.json` present, valid JSON, uid `scraper-health`, 7 panels.
- `docker/grafana/provisioning/alerting/rules.yml` present, valid YAML, contains `uid: provider-health-stream-segment-down`.
- `frontend/web/public/changelog.json` present, valid JSON, top group contains a Phase 17 entry.
- All 4 task commits present in `git log` on the worktree branch: `b4ef8e3`, `37ce4f8`, `5e2a00c`, `02de2d1`.

---
*Phase: 17-observability*
*Plan: 04*
*Completed: 2026-05-12*
