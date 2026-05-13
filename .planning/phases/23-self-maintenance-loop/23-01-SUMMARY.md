---
phase: 23
plan: "01"
subsystem: scheduler
tags: [scheduler, canary, scraper, playability, prometheus, cron, observability]
status: shipped
requirements: [SCRAPER-HEAL-12, SCRAPER-HEAL-13]
provides:
  - "playability_canary_runs_total Prometheus counter {provider, server, result, reason, anime_slot}"
  - "ScraperPlayabilityCanaryJob — daily 03:00 canary with ±5min jitter, 2 anchors + 3 dynamic anime"
  - "POST /api/v1/jobs/scraper_playability_canary manual trigger (skips jitter)"
  - "Per-run JSON log on player_reports Docker volume at /data/reports/canary-runs/"
  - "AnimeSlots() canonical slot literals — single source of truth for the anime_slot label domain"
requires:
  - "libs/streamprobe.Probe (Phase 21)"
  - "/scraper/servers + /scraper/stream HTTP surface (Phase 21)"
  - "animes + watch_history tables (catalog + player services)"
  - "player_reports Docker volume (services/player)"
affects:
  - "Phase 23 Plan 23-02 (Grafana dashboard consumes the new counter)"
  - "Phase 23 Plan 23-03 (alert rules + maintenance-bot dispatch consume the counter + per-run logs)"
tech-stack:
  added:
    - "libs/streamprobe (Phase 21) added as scheduler dependency"
  patterns:
    - "TDD: RED test commit → GREEN implementation commit (Task 1)"
    - "Injectable collaborators (probe func, rng, now, writeFile) for unit-test isolation"
    - "Portable SQL (subquery + GROUP BY instead of Postgres-only DISTINCT ON) works on sqlite + Postgres"
    - "Sentinel server label (`_unreachable`) bounds cardinality on degraded paths (T-23-04)"
key-files:
  created:
    - "services/scheduler/internal/jobs/scraper_playability_canary.go"
    - "services/scheduler/internal/jobs/scraper_playability_canary_test.go"
    - "services/scheduler/internal/config/config_test.go"
  modified:
    - "libs/metrics/parser.go (added PlayabilityCanaryRunsTotal + AnimeSlots)"
    - "libs/metrics/provider_test.go (added 3 test cases for the new counter)"
    - "services/scheduler/internal/config/config.go (3 new JobsConfig fields)"
    - "services/scheduler/internal/service/job.go (JobService struct + Start + Trigger + GetStatus)"
    - "services/scheduler/internal/handler/job.go (TriggerScraperPlayabilityCanary)"
    - "services/scheduler/internal/transport/router.go (manual-trigger route)"
    - "services/scheduler/cmd/scheduler-api/main.go (job construction + boot wiring)"
    - "services/scheduler/go.mod + go.sum (libs/streamprobe dep)"
    - "services/scheduler/Dockerfile (COPY libs/streamprobe + services/scraper go.mod)"
    - "docker/docker-compose.yml (scheduler env vars + player_reports mount + scraper depends_on)"
decisions:
  - "Canary lives in services/scheduler/, not services/scraper/ (CONTEXT D1)"
  - "Anime list refreshed every run (anchors + dynamic from watch_history) (CONTEXT D2)"
  - "Canary calls /scraper/* over HTTP, not in-process, to exercise gateway + middleware (CONTEXT D3)"
  - "anime_slot label uses literal strings (anchor_frieren, recent_1, ...) for Grafana readability (CONTEXT D4)"
  - "Per-run JSON log on player_reports volume for maintenance-bot triage (CONTEXT D5)"
  - "RunNoJitter manual-trigger path so admins don't wait up to 5 minutes (deviation Rule 2)"
  - "Sentinel `server=\"_unreachable\"` when /scraper/servers fails (bounded cardinality + deterministic alert match)"
metrics:
  duration: "16m 8s"
  completed: "2026-05-13"
  tasks_completed: 3
  files_created: 3
  files_modified: 10
  commits: 5
  tests_added: 14
  lines_added: ~1200
---

# Phase 23 Plan 23-01: Canary Cron + Playability Metric Summary

## One-Liner

Daily 03:00 scraper playability canary in scheduler emits `playability_canary_runs_total{provider,server,result,reason,anime_slot}` against 2 anchors + 3 dynamic anime with per-run JSON logs to player_reports.

## Status

**SHIPPED** — verified end-to-end on production stack 2026-05-13T07:10Z.

## What Got Built

### libs/metrics

- **`PlayabilityCanaryRunsTotal`** — Prometheus counter family.
  Labels: `{provider, server, result, reason, anime_slot}`. 210-series
  cardinality bound documented inline (1 provider × 3 servers × 2 results
  × 7 reasons × 5 slots).
- **`AnimeSlots()`** — single source of truth for the 5 literal slot
  values (`anchor_frieren`, `anchor_one_piece`, `recent_1`, `recent_2`,
  `recent_3`). Per CONTEXT.md D4: literal strings (not numeric indexes)
  for Grafana panel readability.

### services/scheduler

- **`ScraperPlayabilityCanaryJob`** in `internal/jobs/scraper_playability_canary.go`:
  - `composeAnimeList`: 2 anchors (Frieren MAL 52991 + One Piece MAL 21)
    + up to 3 dynamic from watch_history (last 24h, distinct anime, top 3
    by MAX(watched_at) DESC). Fallback to top 3 from `animes` table by
    `updated_at DESC`. Both-empty path returns anchors-only + warns.
  - `probeOne`: HTTP `/scraper/servers` per anime → iterate every returned
    server → HTTP `/scraper/stream` → `libs/streamprobe.Probe` against the
    master URL → emit one TupleResult per (anime_slot, server) pair.
  - Metric emission via
    `metrics.PlayabilityCanaryRunsTotal.WithLabelValues(provider, server,
    result, reason, anime_slot).Inc()` per tuple.
  - Sentinel `server="_unreachable"` when `/scraper/servers` itself fails
    (T-23-04 cardinality bound + deterministic alert-match label value).
  - ±5min uniform jitter via `computeJitter()` — bounded `[-5min, +5min]`.
    Skipped on the manual-trigger path via `RunNoJitter`.
  - Per-run JSON log written to `CanaryReportDir / YYYY-MM-DD-HHMMSS.json`.
    Authorization / Cookie / Set-Cookie / Proxy-Authorization headers
    redacted via `isSensitiveHeader` + `hdrsToHeader` (T-23-01).
  - All collaborators (HTTP client, probe func, clock, RNG, file writer)
    injectable for unit-test isolation.

- **`JobsConfig`** in `internal/config/config.go` gains:
  - `ScraperPlayabilityCanaryCron` (default `0 3 * * *`)
  - `ScraperBaseURL` (default `http://scraper:8088`)
  - `CanaryReportDir` (default `/data/reports/canary-runs`)

- **`JobService.Start`** wires the new cron AddFunc closure mirroring the
  existing `calendar_sync` pattern. Boot log emits
  `"registered job: scraper_playability_canary"` for grepability.
  `TriggerScraperPlayabilityCanary` manual hook uses `RunNoJitter`.

- **Manual-trigger HTTP route**:
  `POST /api/v1/jobs/scraper_playability_canary`. Used by the synthetic
  Pattern 6 smoke in Plan 23-03 and ops manual testing.

### docker-compose

- Scheduler service gains the three new env vars
  (`SCRAPER_PLAYABILITY_CANARY_CRON`, `SCRAPER_BASE_URL`,
  `CANARY_REPORT_DIR`), mounts `player_reports:/data/reports` (shared with
  player), and declares `depends_on: scraper`.

## Tests

14 new tests, all passing under `-race`:

**libs/metrics (3):**
- `TestPlayabilityCanaryRunsTotal_IncrementsCorrectly` — name + labels + .Inc()
- `TestPlayabilityCanaryRunsTotal_AllReasonsAccepted` — every streamprobe Reason value as label
- `TestAnimeSlots_ExactlyFive` — slot literals contract

**services/scheduler/internal/config (2):**
- `TestLoad_CanaryDefaults` — three new fields default correctly
- `TestLoad_CanaryOverride` — env-var overrides honored

**services/scheduler/internal/jobs (10 — canary suite):**
- `TestCanary_AnimeListComposition_BothEmpty` — anchors-only + warn
- `TestCanary_AnimeListComposition_Anchors` — anchors with title resolution
- `TestCanary_AnimeListComposition_RecentFromWatchHistory` — top 3 by MAX(watched_at) DESC
- `TestCanary_AnimeListComposition_FallbackToAnimeList` — top 3 by updated_at DESC
- `TestCanary_EmitsMetric_PerTuple` — 5 anime × 2 servers = 10 unique label sets
- `TestCanary_WritesPerRunLog` — file naming + JSON shape + secret redaction (asserts no Authorization / Set-Cookie value bleeds through)
- `TestCanary_AllFiveAnimeSlots` — every slot literal incremented
- `TestCanary_JitterIsBounded` — 1000 samples ∈ `[-5min, +5min]`
- `TestCanary_RunNoJitter_SkipsJitter` — manual path completes in <2s
- `TestCanary_ScraperUnreachable_DoesNotPanic` — sentinel tuple on /scraper/servers 503

## Smoke Verification (Production Stack)

Triggered manually post-deploy:

```bash
curl -sX POST http://localhost:8085/api/v1/jobs/scraper_playability_canary
# {"success":true,"data":{"status":"job triggered"}}

# After ~1s:
curl -s http://localhost:8085/metrics | grep playability_canary_runs_total
# playability_canary_runs_total{anime_slot="anchor_frieren",provider="gogoanime",reason="zero_match",result="fail",server="_unreachable"} 1
# playability_canary_runs_total{anime_slot="anchor_one_piece",provider="gogoanime",reason="zero_match",result="fail",server="_unreachable"} 1
# playability_canary_runs_total{anime_slot="recent_1",provider="gogoanime",reason="zero_match",result="fail",server="_unreachable"} 1

# Per-run log:
docker exec animeenigma-scheduler ls /data/reports/canary-runs/
# 2026-05-13-070709.json
# 2026-05-13-071048.json
```

The `fail/zero_match/_unreachable` tuples are the canary correctly
detecting that the live `/scraper/servers?mal_id=52991&episode=1` returns
an empty server list — i.e., the system is already detecting upstream
state and would alert on this once Plan 23-03's rules ship. This is the
canary doing its job, not a canary bug.

Title resolution works for Russian names ("Провожающая в последний путь
Фрирен" for Frieren).

## Deviations from Plan

### Rule 1 — Auto-fix bug

**[Rule 1 - Bug] Production animes table uses `name_ru` + varchar `mal_id`**

- **Found during:** Task 3 smoke deploy
- **Issue:** Initial SQL queries used `russian` column name and assumed
  `mal_id` was integer. Production schema has `name_ru` (varchar(500))
  and `mal_id` (varchar(50)).
- **Fix:** Renamed queries to `name_ru`, scanned `mal_id` as
  `sql.NullString`, added `parseMALID()` helper returning 0 on
  unparseable values (caller skips), updated test schema to match.
- **Files modified:** `services/scheduler/internal/jobs/scraper_playability_canary.go`,
  `services/scheduler/internal/jobs/scraper_playability_canary_test.go`
- **Commit:** 17ab036

### Rule 2 — Auto-add missing critical functionality

**[Rule 2 - Missing functionality] Manual trigger needs jitter skip**

- **Found during:** Task 3 smoke deploy
- **Issue:** Manual `/api/v1/jobs/scraper_playability_canary` blocked up
  to 5 minutes on the jitter sleep — unusable for the synthetic Pattern
  6 smoke test in Plan 23-03 and unacceptable UX for ops.
- **Fix:** Added `RunNoJitter()` entry point used by
  `JobService.TriggerScraperPlayabilityCanary`; scheduled cron still uses
  `Run()` with full jitter. New test
  `TestCanary_RunNoJitter_SkipsJitter` asserts <2s wall time.
- **Files modified:** `services/scheduler/internal/jobs/scraper_playability_canary.go`,
  `services/scheduler/internal/jobs/scraper_playability_canary_test.go`,
  `services/scheduler/internal/service/job.go`
- **Commit:** 17ab036

### Rule 3 — Auto-fix blocking dependency

**[Rule 3 - Blocking] services/scheduler/Dockerfile missing scraper go.mod**

- **Found during:** Task 3 `make redeploy-scheduler`
- **Issue:** `go.work` references `./services/scraper` but the scheduler
  Dockerfile didn't `COPY services/scraper/go.mod` into the builder
  stage. `go mod download` failed with
  `cannot load module ../scraper listed in go.work file`. Pre-existing
  oversight surfaced by my libs/streamprobe dependency addition.
- **Fix:** Added the missing COPY line.
- **Files modified:** `services/scheduler/Dockerfile`
- **Commit:** 17ab036

## Threat Model Compliance

| Threat | Mitigation | Verified |
|--------|------------|----------|
| T-23-01 (header info disclosure) | `redactHeaders` + `isSensitiveHeader` strip Authorization/Cookie/Set-Cookie/Proxy-Authorization before persistence | `TestCanary_WritesPerRunLog` asserts no secret bytes in the on-disk JSON |
| T-23-02 (upstream rate-limit retaliation) | ±5min jitter on the scheduled cron tick; off-peak 03:00 local | `TestCanary_JitterIsBounded` asserts `[-5min, +5min]` over 1000 samples |
| T-23-04 (cardinality bomb) | `server` label sourced only from `domain.Server.ID` (bounded enum); `_unreachable` sentinel for failure path; 210-series budget documented | `TestCanary_AnimeListComposition_*` + cardinality comment in parser.go |
| T-23-03 (SQL injection via watch_history) | Parameterized queries, no user input on the path | Code review during execution |
| T-23-05 (DB privilege escalation) | Canary executes SELECT only, no INSERT/UPDATE/DELETE | Code review during execution |

All T-23-0X mitigations applied. No new threats discovered during execution.

## Commits

| Hash | Type | Description |
|------|------|-------------|
| `faab6c3` | test | RED — failing tests for PlayabilityCanaryRunsTotal + AnimeSlots + canary config |
| `13a3356` | feat | GREEN — implement counter + slots + JobsConfig fields + docker-compose env/volume |
| `b566758` | feat | scraper playability canary job — list composition + probe + per-run JSON log |
| `84758fd` | feat | wire canary into JobService + manual trigger handler + boot |
| `17ab036` | fix | align canary SQL with prod animes schema + add RunNoJitter + Dockerfile scraper dep |

## Follow-up

- Plan 23-02 (Wave 2): Grafana dashboard consumes `playability_canary_runs_total` + `scheduler_job_last_success_timestamp{job="scraper_playability_canary"}` + per-run logs.
- Plan 23-03 (Wave 3): three Prometheus alert rules (`ScraperPlayabilityRegression`, `ScraperAdDecoySurge`, `ScraperUnplayableSpike`) routing to the maintenance webhook; verifies synthetic Pattern 6 dispatch.
- The live smoke run already surfaced that `/scraper/servers` returns empty for Frieren on the current deploy — once Plan 23-03's alert rules land, this state will fire the regression alert. Investigating that gap is Plan 23-03's concern, not 23-01's.

## Self-Check: PASSED

Verified via grep + file existence + tests + smoke:

- [x] `libs/metrics/parser.go` exports `PlayabilityCanaryRunsTotal` + `AnimeSlots()`
- [x] `services/scheduler/internal/jobs/scraper_playability_canary.go` exists with `ScraperPlayabilityCanaryJob.Run` + `RunNoJitter`
- [x] `services/scheduler/internal/jobs/scraper_playability_canary_test.go` exists with 10 `TestCanary_*` tests
- [x] `services/scheduler/internal/config/config.go` defines `ScraperPlayabilityCanaryCron` / `ScraperBaseURL` / `CanaryReportDir` with correct defaults
- [x] `services/scheduler/internal/config/config_test.go` covers defaults + overrides
- [x] `services/scheduler/internal/service/job.go` registers the canary cron + logs `registered job: scraper_playability_canary` + exposes `TriggerScraperPlayabilityCanary` + `GetStatus` surface
- [x] `services/scheduler/internal/handler/job.go` + `internal/transport/router.go` expose `POST /api/v1/jobs/scraper_playability_canary`
- [x] `services/scheduler/cmd/scheduler-api/main.go` constructs the job + wires `cfg.Jobs.ScraperPlayabilityCanaryCron` into `Start`
- [x] `docker/docker-compose.yml` scheduler block has 3 new env vars + `player_reports:/data/reports` mount + `depends_on: scraper`
- [x] All commits present in git log: `faab6c3`, `13a3356`, `b566758`, `84758fd`, `17ab036`
- [x] `go test -count=1 -race ./...` passes for both `libs/metrics` and `services/scheduler`
- [x] `make redeploy-scheduler` succeeds, boot logs show `registered job: scraper_playability_canary`
- [x] `/metrics` exposes `playability_canary_runs_total` with 5 labels after a manual trigger
- [x] Per-run JSON log written to `/data/reports/canary-runs/YYYY-MM-DD-HHMMSS.json` on the player_reports volume
