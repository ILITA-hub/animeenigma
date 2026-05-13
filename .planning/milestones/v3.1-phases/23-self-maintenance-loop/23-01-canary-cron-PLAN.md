---
id: 23-01
phase: 23
plan: "01"
type: execute
wave: 1
depends_on: []
files_modified:
  - services/scheduler/go.mod
  - services/scheduler/go.sum
  - services/scheduler/Dockerfile
  - services/scheduler/internal/config/config.go
  - services/scheduler/internal/config/config_test.go
  - services/scheduler/internal/jobs/scraper_playability_canary.go
  - services/scheduler/internal/jobs/scraper_playability_canary_test.go
  - services/scheduler/internal/service/job.go
  - services/scheduler/cmd/scheduler-api/main.go
  - libs/metrics/parser.go
  - libs/metrics/provider_test.go
  - docker/docker-compose.yml
requirements:
  - SCRAPER-HEAL-12
  - SCRAPER-HEAL-13
autonomous: true
tags: [scheduler, canary, scraper, playability, prometheus, cron]

must_haves:
  truths:
    - "Scheduler runs a `scraper_playability_canary` job daily at 03:00 (cron `0 3 * * *`) with ±5 min jitter applied before each invocation"
    - "Canary composes a 5-anime list per run: 2 fixed anchors (anchor_frieren, anchor_one_piece by MAL ID 52991 + 21) + 3 dynamic from watch_history < 24h; falls back to top-3 from anime_list ORDER BY updated_at DESC when watch_history empty; logs a warning when both are empty and runs only the two anchors"
    - "Per anime, canary iterates every server returned by `/scraper/servers?animeId=<malId>&episode=1` via HTTP, calls `/scraper/stream` for each server, and runs libs/streamprobe.Probe against the returned URL"
    - "Metric `playability_canary_runs_total{provider, server, result, reason, anime_slot}` emits exactly once per (anime_slot, server) tuple per run, with `result ∈ {pass, fail}`, `reason` from libs/streamprobe.Reason values, `anime_slot ∈ {anchor_frieren, anchor_one_piece, recent_1, recent_2, recent_3}`"
    - "Each run persists a JSON log to `/data/reports/canary-runs/YYYY-MM-DD-HHMMSS.json` (player_reports Docker volume) containing per-tuple results, sampled hosts, and run timestamp; Authorization / Cookie headers are redacted from logged response headers"
    - "Manual trigger endpoint `POST /api/v1/jobs/scraper_playability_canary` runs the canary on demand and exits with non-zero status when the job's context errors (timeout) — used by the synthetic test in 23-03"
    - "Scheduler boot fails fast if SCRAPER_BASE_URL is unparseable; defaults to `http://scraper:8088` matching the docker-compose network name"
  artifacts:
    - path: services/scheduler/internal/jobs/scraper_playability_canary.go
      provides: "ScraperPlayabilityCanaryJob — anime-list composition, per-server HTTP probe via /scraper/stream, streamprobe.Probe call, metric emission, per-run JSON log to player_reports volume"
      contains: "ScraperPlayabilityCanaryJob"
    - path: services/scheduler/internal/jobs/scraper_playability_canary_test.go
      provides: "TestCanary_AnimeListComposition_Anchors, TestCanary_AnimeListComposition_RecentFromWatchHistory, TestCanary_AnimeListComposition_FallbackToAnimeList, TestCanary_AnimeListComposition_BothEmpty, TestCanary_EmitsMetric_PerTuple, TestCanary_WritesPerRunLog, TestCanary_AllFiveAnimeSlots, TestCanary_JitterIsBounded — tests use httptest scraper + in-memory sqlite + a temp dir for player_reports"
      contains: "TestCanary"
    - path: libs/metrics/parser.go
      provides: "PlayabilityCanaryRunsTotal counter — registered alongside existing parser_* counters; lives in libs/metrics so libs/streamprobe consumers + scheduler both share the same registry without cyclic deps"
      contains: "PlayabilityCanaryRunsTotal"
    - path: services/scheduler/internal/config/config.go
      provides: "JobsConfig.ScraperPlayabilityCanaryCron (default `0 3 * * *`) + JobsConfig.ScraperBaseURL (default `http://scraper:8088`) + JobsConfig.CanaryReportDir (default `/data/reports/canary-runs`)"
      contains: "ScraperPlayabilityCanaryCron"
    - path: services/scheduler/internal/service/job.go
      provides: "JobService.Start wires the canary cron + adds TriggerScraperPlayabilityCanary manual hook; emits SchedulerJobExecutionsTotal{job=\"scraper_playability_canary\",status} like other scheduled jobs"
      contains: "scraper_playability_canary"
    - path: services/scheduler/cmd/scheduler-api/main.go
      provides: "NewScraperPlayabilityCanaryJob constructor wired with db, redisCache, cfg.Jobs, log; passed into NewJobService"
      contains: "NewScraperPlayabilityCanaryJob"
    - path: docker/docker-compose.yml
      provides: "scheduler service gains SCRAPER_BASE_URL + CANARY_REPORT_DIR + SCRAPER_PLAYABILITY_CANARY_CRON env vars; player_reports volume mounted at /data/reports (read-write) — reused, not duplicated"
      contains: "player_reports:/data/reports"
  key_links:
    - from: services/scheduler/internal/jobs/scraper_playability_canary.go
      to: libs/streamprobe.Probe
      via: "direct in-process call against the URL returned by /scraper/stream"
      pattern: "streamprobe.Probe"
    - from: services/scheduler/internal/jobs/scraper_playability_canary.go
      to: libs/metrics.PlayabilityCanaryRunsTotal
      via: "WithLabelValues(provider, server, result, reason, anime_slot).Inc()"
      pattern: "PlayabilityCanaryRunsTotal.WithLabelValues"
    - from: services/scheduler/internal/jobs/scraper_playability_canary.go
      to: "http://scraper:8088/scraper/servers + /scraper/stream"
      via: "HTTP GET to the scraper service inside docker-compose network"
      pattern: "cfg.ScraperBaseURL"
    - from: services/scheduler/internal/jobs/scraper_playability_canary.go
      to: "/data/reports/canary-runs/<ts>.json on player_reports volume"
      via: "os.WriteFile after json.Marshal of the per-run summary"
      pattern: "CanaryReportDir"
    - from: services/scheduler/internal/service/job.go
      to: "cron schedule `0 3 * * *` + ±5min jitter via time.Sleep(jitter) at the head of the closure"
      via: "JobService.Start AddFunc closure"
      pattern: "ScraperPlayabilityCanaryCron"
    - from: services/scheduler/cmd/scheduler-api/main.go
      to: services/scheduler/internal/jobs/ScraperPlayabilityCanaryJob
      via: "constructor + wired into NewJobService alongside the other 4 jobs"
      pattern: "NewScraperPlayabilityCanaryJob"
---

<objective>
Ship the daily 03:00 scraper playability canary as a new scheduler job. The canary exercises real production code paths (HTTP `/scraper/servers` → `/scraper/stream` → `libs/streamprobe.Probe`) against five anime — two stable anchors (Frieren, One Piece) plus three dynamic anime from the last 24h of watch_history — and emits `playability_canary_runs_total{provider, server, result, reason, anime_slot}` so Prometheus + Grafana can detect regressions before users do. Per-run JSON logs are persisted to the existing `player_reports` Docker volume so a downstream maintenance-bot dispatch has artifacts to `cat` instead of having to re-fetch upstream content (which may already have rotated). Covers SCRAPER-HEAL-12 + SCRAPER-HEAL-13.

Purpose: Phase 21 + 22 added the gate and the per-server fallback. Without a canary, regressions only surface when a real user hits the broken provider — which can be hours or days. Five anime × every server × every reason gives the alert rule layer in Plan 23-03 a high-signal stream of pass/fail data. The anchor anime guarantee a continuous baseline; the dynamic recent set catches "what the population is actually watching tonight" regressions.

Output:
- Public types `ScraperPlayabilityCanaryJob` + `Run(ctx)` matching the existing `*SyncJob` / `*CleanupJob` shape in `services/scheduler/internal/jobs/`.
- New counter `PlayabilityCanaryRunsTotal` in `libs/metrics` with documented label cardinality bound (5 anime_slot × ≤3 servers × 2 results × 7 reasons × 1 provider ≈ 210 series — well within Prometheus default).
- Cron registration in `services/scheduler/internal/service/job.go` matching the existing pattern (success/failure metric + last-success gauge + logger), with ±5 min jitter applied via `time.Sleep` at the head of the closure to avoid 03:00:00 fingerprinting.
- Boot wiring + docker-compose env vars (SCRAPER_BASE_URL, CANARY_REPORT_DIR, SCRAPER_PLAYABILITY_CANARY_CRON) without breaking existing services.
- Tests covering anime-list composition (all four branches: anchors-only-on-empty, recent-from-history, fallback-to-anime_list, both-empty), metric emission, per-run log file write, and jitter bounds.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/23-self-maintenance-loop/23-CONTEXT.md
@docs/plans/2026-05-13-scraper-self-healing-spec.md
@.planning/phases/21-playability-foundation/21-01-SUMMARY.md
@.planning/phases/21-playability-foundation/21-02-SUMMARY.md
@.planning/phases/21-playability-foundation/21-03-SUMMARY.md
@CLAUDE.md

<interfaces>
<!-- libs/streamprobe (from Phase 21) — the canary calls Probe directly against the URL returned by /scraper/stream: -->

```go
package streamprobe

type Reason string
const (
    ReasonPlayable         Reason = "playable"
    ReasonAdDecoy          Reason = "ad_decoy"
    ReasonZeroMatch        Reason = "zero_match"
    ReasonStatus403        Reason = "status_403"
    ReasonSignedURLExpired Reason = "signed_url_expired"
    ReasonCDNUnreachable   Reason = "cdn_unreachable"
    ReasonEmptyResponse    Reason = "empty_response"
)
type Result struct {
    Playable bool
    Reason   Reason
    Sampled  []string
}
func Probe(ctx context.Context, masterURL string, headers http.Header) Result
func AllReasons() []Reason  // used in tests to assert exhaustive coverage
```

<!-- Scraper service HTTP surface used by the canary (from Phase 21 handler at services/scraper/internal/handler/scraper.go — same surface the catalog client uses): -->

```
GET  /scraper/servers?animeId=<mal>&episode=1   → 200 { servers: [{ id, name, url, ... }], ... }
GET  /scraper/stream?animeId=<mal>&episode=1&server=<serverID>
                                                → 200 { url, headers?, meta: { gated: bool, tried: [...] }, ... }
```

<!-- libs/metrics — existing register pattern (parser.go currently holds ParserRequestsTotal + ParserRequestDuration + ParserFallbackTotal). The canary metric is added there to keep all scraper-adjacent counters co-located. provider.go already has ParserUnplayableTotal + ParserAdDecoyTotal from Phase 21. -->

<!-- Existing job constructor signatures (anime_loader / calendar / cleanup) — the canary follows the calendar.go shape because the canary calls an HTTP service rather than wrapping a DB worker: -->

```go
// calendar.go shape (HTTP-driven):
type CalendarSyncJob struct {
    config *config.JobsConfig
    client *http.Client
    log    *logger.Logger
}
func (j *CalendarSyncJob) Run(ctx context.Context) error
```

<!-- DB access — scheduler already opens a *gorm.DB and passes it to CleanupJob. The canary needs raw query against watch_history (player service's table) JOINed with animes (catalog service's table). Since scheduler's domain models don't declare WatchHistory / Anime, use raw SQL via gorm.DB.Raw — there is precedent (no GORM model required for read-only cross-service queries). -->

<!-- Watch history schema (from services/player/internal/domain/watch.go:88-110): -->

```
table watch_history (
    id, user_id, anime_id (uuid → animes.id), episode_number, watched_at, ...
)
table animes (
    id (uuid), mal_id (int), shikimori_id (string), original_name, russian_name, ...
)
```

<!-- Anchor anime MAL IDs (DO NOT GUESS — verify these by querying the live `animes` table during execution or by reading existing test seed data; if either differs, prefer the value present in the DB and adjust the constant + add a comment): -->

- Frieren: Beyond Journey's End — MAL 52991 (Shikimori 52991)
- One Piece — MAL 21 (Shikimori 21)

<!-- Existing service.JobService.Start pattern (services/scheduler/internal/service/job.go:44-131) — the canary AddFunc closure mirrors shikimori/cleanup/topAnime/calendar bodies (metric emit + lastRun assignment). -->
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Register PlayabilityCanaryRunsTotal counter + boot config + docker-compose wiring</name>
  <files>libs/metrics/parser.go, libs/metrics/provider_test.go, services/scheduler/internal/config/config.go, services/scheduler/internal/config/config_test.go, docker/docker-compose.yml</files>
  <read_first>
    - libs/metrics/parser.go (full file — register the new counter alongside existing parser_* counters; same promauto pattern)
    - libs/metrics/provider.go (full file — copy the doc-comment style for the new counter; the `reason` and `server` label conventions are documented there and the canary must reuse them verbatim)
    - libs/streamprobe/reason.go (full file — confirm the 7 Reason string values and re-use them as canary label values)
    - services/scheduler/internal/config/config.go (full file — extend JobsConfig + Load to read SCRAPER_PLAYABILITY_CANARY_CRON, SCRAPER_BASE_URL, CANARY_REPORT_DIR)
    - docker/docker-compose.yml lines 491-516 (scheduler service block — extend `environment:` with the three new env vars; volumes need `player_reports:/data/reports` mount added)
    - docker/docker-compose.yml (search for `player_reports:` named-volume + the player service that already mounts it — copy that mount idiom)
    - .planning/phases/23-self-maintenance-loop/23-CONTEXT.md D4 (anime_slot label uses literal strings, not numeric indexes — required for the counter's label-value contract)
  </read_first>
  <behavior>
    - Test (libs/metrics): `PlayabilityCanaryRunsTotal.WithLabelValues("gogoanime","streamhg","pass","playable","anchor_frieren").Inc()` does not panic; counter value is 1 after one Inc; label names are exactly `[provider, server, result, reason, anime_slot]` in that order (asserted via testutil.CollectAndCount or by reading the registered MetricVec descriptor).
    - Test (libs/metrics): all 7 streamprobe.AllReasons() string values are accepted as the `reason` label value without panicking — guarantees the canary can emit every possible Probe outcome.
    - Test (libs/metrics): the 5 expected anime_slot literal values (`anchor_frieren`, `anchor_one_piece`, `recent_1`, `recent_2`, `recent_3`) all accepted as label values; documented in a Go-level slice `AnimeSlots()` exported for the canary + tests to share so the constants live in exactly one place.
    - Test (config): `config.Load()` with no env returns `JobsConfig.ScraperPlayabilityCanaryCron == "0 3 * * *"`, `JobsConfig.ScraperBaseURL == "http://scraper:8088"`, `JobsConfig.CanaryReportDir == "/data/reports/canary-runs"`.
    - Test (config): `t.Setenv("SCRAPER_PLAYABILITY_CANARY_CRON", "*/5 * * * *")` overrides cleanly.
    - Test (config): empty-string env vars fall through to defaults (`getEnv` semantics already do this — assert behavior, don't change function).
    - docker-compose: `grep "SCRAPER_PLAYABILITY_CANARY_CRON" docker/docker-compose.yml` returns the scheduler-service environment line; `grep "player_reports:/data/reports" docker/docker-compose.yml | wc -l` returns ≥ 2 (player service + scheduler service both mount it).
  </behavior>
  <action>
    1. **Edit libs/metrics/parser.go** — append the canary counter after the existing `ParserFallbackTotal` block:
       ```go
       // PlayabilityCanaryRunsTotal counts scheduler scraper-playability-canary
       // run outcomes per (provider, server, result, reason, anime_slot). Used
       // by the Phase 23 canary job to surface upstream-site regressions within
       // 24h — see services/scheduler/internal/jobs/scraper_playability_canary.go
       // + infra/grafana/alerts/scraper.yaml (Phase 23 Plan 23-03).
       //
       // Label conventions:
       //   provider:   one of registered scraper providers (currently "gogoanime")
       //   server:     normalized embed extractor name (vibeplayer, streamhg, earnvids)
       //   result:     "pass" | "fail"
       //   reason:     one of libs/streamprobe.Reason values (string identity)
       //   anime_slot: one of {anchor_frieren, anchor_one_piece, recent_1,
       //               recent_2, recent_3} — see AnimeSlots() below
       //
       // Cardinality bound (current):
       //   1 provider × 3 servers × 2 results × 7 reasons × 5 slots = 210 series.
       //   Well within Prometheus default limits.
       //
       // SCRAPER-HEAL-13.
       PlayabilityCanaryRunsTotal = promauto.NewCounterVec(
           prometheus.CounterOpts{
               Name: "playability_canary_runs_total",
               Help: "Total count of scraper playability canary results per (provider, server, result, reason, anime_slot)",
           },
           []string{"provider", "server", "result", "reason", "anime_slot"},
       )

       // AnimeSlots returns the canonical literal slot values emitted by the
       // canary. Single source of truth for the counter's anime_slot label
       // domain. Per CONTEXT.md D4: literal strings (not numeric indexes) for
       // Grafana panel readability.
       func AnimeSlots() []string {
           return []string{"anchor_frieren", "anchor_one_piece", "recent_1", "recent_2", "recent_3"}
       }
       ```
       Imports: `prometheus`, `promauto` are already imported. No new deps.
    2. **Add libs/metrics/provider_test.go cases** (the file may already exist with Phase 21 tests — append):
       ```go
       func TestPlayabilityCanaryRunsTotal_AllReasonsAccepted(t *testing.T) {
           for _, r := range streamprobe.AllReasons() {
               PlayabilityCanaryRunsTotal.WithLabelValues("gogoanime", "streamhg", "pass", string(r), "anchor_frieren").Inc()
           }
       }
       func TestAnimeSlots_ExactlyFive(t *testing.T) {
           slots := AnimeSlots()
           require.Len(t, slots, 5)
           require.Equal(t, []string{"anchor_frieren","anchor_one_piece","recent_1","recent_2","recent_3"}, slots)
       }
       ```
       libs/metrics already declares an import of libs/streamprobe? — verify; if NOT, add `replace github.com/ILITA-hub/animeenigma/libs/streamprobe => ../streamprobe` and `require` entry in libs/metrics/go.mod, then `go work sync`. (Project Memory rule "Adding New libs/ Module" applies.)
    3. **Edit services/scheduler/internal/config/config.go** — extend JobsConfig:
       ```go
       type JobsConfig struct {
           // ... existing fields ...
           ScraperPlayabilityCanaryCron string
           ScraperBaseURL               string
           CanaryReportDir              string
       }
       ```
       In `Load()`:
       ```go
       Jobs: JobsConfig{
           // ... existing fields ...
           ScraperPlayabilityCanaryCron: getEnv("SCRAPER_PLAYABILITY_CANARY_CRON", "0 3 * * *"),
           ScraperBaseURL:               getEnv("SCRAPER_BASE_URL", "http://scraper:8088"),
           CanaryReportDir:              getEnv("CANARY_REPORT_DIR", "/data/reports/canary-runs"),
       },
       ```
    4. **Update services/scheduler/internal/config/config_test.go** (create if absent) — add `TestLoad_CanaryDefaults` + `TestLoad_CanaryOverride` using `t.Setenv` (Go 1.17+).
    5. **Edit docker/docker-compose.yml** — find the scheduler service block (around line 491) and:
       - Add to `environment:`:
         ```yaml
         SCRAPER_PLAYABILITY_CANARY_CRON: "0 3 * * *"
         SCRAPER_BASE_URL: http://scraper:8088
         CANARY_REPORT_DIR: /data/reports/canary-runs
         ```
       - Add `volumes:` block (if not present):
         ```yaml
         volumes:
           - player_reports:/data/reports
         ```
       - Add `scraper` to `depends_on:` with `condition: service_started`.
       - DO NOT modify the `player_reports:` named-volume declaration at the bottom of the file — it already exists and is reused.
  </action>
  <verify>
    <automated>cd /data/animeenigma && cd libs/metrics && go test -count=1 -run 'TestPlayabilityCanary|TestAnimeSlots' ./... && cd ../../services/scheduler && go test -count=1 -run 'TestLoad_Canary' ./internal/config/... && grep -c "SCRAPER_PLAYABILITY_CANARY_CRON" /data/animeenigma/docker/docker-compose.yml | grep -v '^0$' && grep -c "player_reports:/data/reports" /data/animeenigma/docker/docker-compose.yml</automated>
  </verify>
  <done>libs/metrics exports PlayabilityCanaryRunsTotal (5 labels) + AnimeSlots() (5 literals); scheduler config Load() returns the three new fields with the correct defaults; docker-compose scheduler service has the three env vars and mounts player_reports; all new unit tests pass.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Implement ScraperPlayabilityCanaryJob — anime-list composition + per-server HTTP probe + metric emit + per-run JSON log</name>
  <files>services/scheduler/internal/jobs/scraper_playability_canary.go, services/scheduler/internal/jobs/scraper_playability_canary_test.go, services/scheduler/go.mod, services/scheduler/go.sum, services/scheduler/Dockerfile</files>
  <read_first>
    - services/scheduler/internal/jobs/calendar.go (full file — copy the HTTP-driven job shape: config + client + log + Run(ctx))
    - services/scheduler/internal/jobs/cleanup.go (full file — copy the DB-driven helper structure when querying watch_history)
    - libs/streamprobe/probe.go (full file — Probe signature, headers handling, 10s total budget, Result fields)
    - libs/streamprobe/reason.go (full file — Reason value strings used as Prometheus label values)
    - libs/metrics/parser.go (from Task 1 — PlayabilityCanaryRunsTotal + AnimeSlots())
    - libs/metrics/provider.go (existing ParserUnplayableTotal doc-comment — match its server-label normalization comment)
    - services/scraper/internal/handler/scraper.go (skim — find the `/scraper/servers` + `/scraper/stream` response JSON shape; the canary unmarshals these structs directly. Note that `meta.tried` and `meta.gated` may be present but the canary does not need them — only `url` is required from /stream.)
    - services/player/internal/domain/watch.go lines 88-110 (WatchHistory struct + TableName — the canary queries `watch_history` via raw SQL since scheduler doesn't import the player domain package)
    - .planning/phases/23-self-maintenance-loop/23-CONTEXT.md (entire file — D2/D3/D4/D5 govern composition, calling pattern, label naming, and per-run log)
    - docs/plans/2026-05-13-scraper-self-healing-spec.md §4.2.a (the canary spec lines 113-140)
  </read_first>
  <behavior>
    - Test `TestCanary_AnimeListComposition_BothEmpty`: in-memory sqlite, no `watch_history` rows, no `anime_list` rows → composed list contains exactly the 2 anchors (Frieren MAL 52991, One Piece MAL 21); logger emitted a warning matching `/canary anime list incomplete/i`; the function does NOT error out (returns len-2 slice, not nil).
    - Test `TestCanary_AnimeListComposition_Anchors`: anchors always present, in slots 0 and 1, regardless of what dynamic rows return. Resolving the anchor MAL ID against the `animes` table returns the title for the per-run log (acceptable if the JOIN returns no row — log still uses MAL ID as fallback title).
    - Test `TestCanary_AnimeListComposition_RecentFromWatchHistory`: seed 5 distinct anime_ids in watch_history with watched_at within the last hour → composed list[2..4] are those most recent 3 distinct anime_ids ordered by watched_at DESC; their slots are labeled recent_1/recent_2/recent_3 in that order.
    - Test `TestCanary_AnimeListComposition_FallbackToAnimeList`: empty watch_history, 5 rows in `anime_list` table → slots[2..4] = top 3 from anime_list ORDER BY updated_at DESC; anchors still occupy slots 0+1.
    - Test `TestCanary_EmitsMetric_PerTuple`: httptest scraper returning 2 servers (streamhg playable, vibeplayer ad_decoy) for each of 5 anime → after Run(), counter samples include `result="pass",reason="playable",anime_slot="anchor_frieren",server="streamhg"` and `result="fail",reason="ad_decoy",anime_slot="anchor_frieren",server="vibeplayer"` (and the same for the other 4 anime_slots). Asserts exactly 10 unique label-set increments (5 anime × 2 servers).
    - Test `TestCanary_WritesPerRunLog`: after Run(), exactly one new `.json` file appears under `t.TempDir()` matching pattern `\d{4}-\d{2}-\d{2}-\d{6}\.json`; file contents JSON-decode into a struct with `RunStartedAt` (RFC3339 string), `Results []TupleResult` of length 10, each TupleResult containing `Provider`, `Server`, `Result`, `Reason`, `AnimeSlot`, `Sampled []string`, `MasterURL`; sensitive headers (Authorization, Cookie, Set-Cookie) NEVER appear in the file body (asserted via `assert.NotContains(t, string(content), "Authorization")`).
    - Test `TestCanary_AllFiveAnimeSlots`: every anime_slot literal from metrics.AnimeSlots() appears at least once in the emitted metric for a single run; no other anime_slot label value present.
    - Test `TestCanary_JitterIsBounded`: the canary's `computeJitter()` helper returns a `time.Duration` in `[-5min, +5min]` over 1000 random calls (table-driven w/ seeded rand.Rand).
    - Test `TestCanary_ScraperUnreachable_DoesNotPanic`: if /scraper/servers returns 503, the canary records a `result="fail",reason="cdn_unreachable"` tuple for each anime_slot under server="_unreachable" (sentinel label value) and continues — does not abort the run mid-list.
  </behavior>
  <action>
    1. **Create services/scheduler/internal/jobs/scraper_playability_canary.go** with this structure:
       ```go
       // Package jobs — scraper_playability_canary.go is the v3.1 Phase 23
       // daily canary that exercises real production scraper code paths
       // (/scraper/servers + /scraper/stream + libs/streamprobe.Probe)
       // against 5 anime — 2 stable anchors + 3 dynamic from watch_history —
       // and emits playability_canary_runs_total{provider, server, result,
       // reason, anime_slot}. Per-run JSON logs persist to the player_reports
       // Docker volume for downstream maintenance-bot dispatch evidence.
       //
       // SCRAPER-HEAL-12 + SCRAPER-HEAL-13.
       package jobs

       const (
           AnchorFrierenMAL   = 52991 // Frieren: Beyond Journey's End — verify against animes table
           AnchorOnePieceMAL  = 21    // One Piece
           ScraperReqTimeout  = 12 * time.Second // gives 10s streamprobe budget + 2s slack
           CanaryRunBudget    = 5 * time.Minute  // hard cap on whole run
       )

       // ScraperPlayabilityCanaryJob runs daily at 03:00 (±5min jitter).
       type ScraperPlayabilityCanaryJob struct {
           db        *gorm.DB
           config    *config.JobsConfig
           client    *http.Client
           log       *logger.Logger
           rng       *rand.Rand // for jitter — seedable for tests
           now       func() time.Time // injectable for tests
           writeFile func(name string, data []byte, perm os.FileMode) error // injectable
       }

       func NewScraperPlayabilityCanaryJob(db *gorm.DB, config *config.JobsConfig, log *logger.Logger) *ScraperPlayabilityCanaryJob

       type TupleResult struct {
           Provider  string   `json:"provider"`
           Server    string   `json:"server"`
           Result    string   `json:"result"`     // "pass" | "fail"
           Reason    string   `json:"reason"`
           AnimeSlot string   `json:"anime_slot"`
           AnimeID   int      `json:"anime_id"`   // MAL id
           Title     string   `json:"title,omitempty"`
           MasterURL string   `json:"master_url"`
           Sampled   []string `json:"sampled,omitempty"`
       }

       type RunSummary struct {
           RunStartedAt time.Time     `json:"run_started_at"`
           RunEndedAt   time.Time     `json:"run_ended_at"`
           Jitter       time.Duration `json:"jitter"`
           Results      []TupleResult `json:"results"`
       }

       func (j *ScraperPlayabilityCanaryJob) Run(ctx context.Context) error
       func (j *ScraperPlayabilityCanaryJob) composeAnimeList(ctx context.Context) ([]canaryAnime, error)
       func (j *ScraperPlayabilityCanaryJob) probeOne(ctx context.Context, slot string, a canaryAnime) []TupleResult
       func (j *ScraperPlayabilityCanaryJob) writeRunLog(s RunSummary) error
       func (j *ScraperPlayabilityCanaryJob) computeJitter() time.Duration
       ```
       Implementation notes (binding):
       - `Run` first sleeps for `computeJitter()` (cap ±5min, sampled from `j.rng`). Then composes anime list. For each anime × each server returned by /scraper/servers, calls /scraper/stream → if URL present, streamprobe.Probe; classifies into TupleResult; appends; emits metric. After all tuples done, calls writeRunLog. Total budget enforced by `ctx` wrapped in `context.WithTimeout(parent, CanaryRunBudget)`.
       - `composeAnimeList`: builds list of `canaryAnime{slot, malID, title}` of length 2..5. Anchors hard-coded. Dynamic part queries via gorm.DB.Raw:
         ```sql
         SELECT DISTINCT ON (wh.anime_id) wh.anime_id, a.mal_id, a.russian
         FROM watch_history wh
         JOIN animes a ON a.id = wh.anime_id
         WHERE wh.watched_at > NOW() - INTERVAL '24 hours'
         ORDER BY wh.anime_id, wh.watched_at DESC
         LIMIT 3
         ```
         (Use Postgres-flavored DISTINCT ON. For test environment running SQLite, fall back to subquery; gate behind `j.db.Dialector.Name() == "postgres"` check.)
       - Fallback when watch_history empty:
         ```sql
         SELECT mal_id, russian FROM animes WHERE deleted_at IS NULL
         ORDER BY updated_at DESC LIMIT 3
         ```
         If anime_list table is a separate concept (verify by reading services/player/internal/domain — the spec uses `anime_list` which is the user's watchlist; map this to the `animes` table for dynamic recent-3 since the spec actually mentions both interchangeably).
       - When both queries return zero rows, log warning `canary anime list incomplete: no recent watch history, no animes — running anchors only`, return the 2-anchor slice.
       - Authorization / Cookie / Set-Cookie headers in any captured response stripped before serialization (write a `redactHeaders(http.Header) http.Header` helper).
       - `writeRunLog` uses `j.config.CanaryReportDir`; ensures parent dir exists via `os.MkdirAll(..., 0755)`; filename `now.Format("2006-01-02-150405") + ".json"`; uses `j.writeFile` to permit a fake in tests.
       - Sentinel server label `"_unreachable"` when /scraper/servers fails — keeps cardinality bounded and gives the alert layer a deterministic label value to match.
       - `computeJitter()`: `time.Duration((j.rng.Int63n(601) - 300) * int64(time.Second))` → -300s to +300s inclusive.
    2. **Create services/scheduler/internal/jobs/scraper_playability_canary_test.go** with all 8 test cases listed in `<behavior>`. Use:
       - `httptest.NewServer` for the fake scraper with handler switch on r.URL.Path; route `/scraper/servers` returns hardcoded servers, `/scraper/stream` returns a URL pointing back at a second httptest server hosting synthetic m3u8 fixtures (reuse the m3u8 fixtures from libs/streamprobe/testdata/ if present, otherwise write minimal `#EXTM3U` + `#EXTINF` + segment-URI inline).
       - `gorm.io/driver/sqlite` already in scheduler's go.mod — use in-memory SQLite for the DB-composition tests. Create tables `animes (mal_id, russian, updated_at, deleted_at)` and `watch_history (anime_id, watched_at)` manually via raw SQL; insert fixture rows; assert composed list.
       - `t.TempDir()` for CanaryReportDir; assert exactly one `.json` file appears matching the pattern.
       - For jitter tests: inject `j.rng = rand.New(rand.NewSource(42))` and assert min/max over 1000 calls fall in [-5min, +5min].
       - For redaction tests: use a fake scraper that includes `Authorization: Bearer secret-token-xyz` in its /scraper/stream response headers, then `assert.NotContains(t, string(logFileContents), "secret-token-xyz")`.
       - Use existing `streamprobe.allowLoopbackForTests` — file-package-private flag inside libs/streamprobe — by reusing httptest's loopback servers; if the flag is not reachable from scheduler tests, point the fake "stream URL" at an external sentinel (e.g., a deliberately-malformed URL that triggers ReasonZeroMatch) and verify reason classification instead. Confirm during execution which path works without touching libs/streamprobe.
    3. **Update services/scheduler/go.mod + go.sum** — add libs/streamprobe + libs/metrics consumer wiring:
       - `require github.com/ILITA-hub/animeenigma/libs/streamprobe v0.0.0`
       - `replace github.com/ILITA-hub/animeenigma/libs/streamprobe => ../../libs/streamprobe`
       - Run `go work sync` after the edit.
    4. **Update services/scheduler/Dockerfile** — add `COPY libs/streamprobe/go.mod libs/streamprobe/go.sum* ./libs/streamprobe/` to the deps stage and `COPY libs/streamprobe/ ./libs/streamprobe/` to the build stage. Match the existing libs/metrics + libs/cache copies exactly.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/scheduler && go test -count=1 -race -run 'TestCanary_' ./internal/jobs/... 2>&1 | tail -30 && grep -c "streamprobe" services/scheduler/go.mod 2>&1 || cd /data/animeenigma && grep -c "streamprobe" /data/animeenigma/services/scheduler/go.mod</automated>
  </verify>
  <done>All 8 `TestCanary_*` tests pass with `-race`; the canary file imports libs/streamprobe + libs/metrics + gorm correctly; Dockerfile builds the scheduler image when run via `make redeploy-scheduler` (deferred to Plan 23-03 final task).</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Wire canary into JobService.Start + main.go + add manual-trigger handler — full scheduler boot path</name>
  <files>services/scheduler/internal/service/job.go, services/scheduler/internal/handler/job.go, services/scheduler/internal/transport/router.go, services/scheduler/cmd/scheduler-api/main.go</files>
  <read_first>
    - services/scheduler/internal/service/job.go (full file — copy the existing AddFunc(closure) pattern for the 4 existing jobs; the canary closure mirrors `calendar_sync`)
    - services/scheduler/internal/handler/job.go (full file — find Trigger* handler shape; add TriggerScraperPlayabilityCanary)
    - services/scheduler/internal/transport/router.go (full file — find route registration for `/api/v1/jobs/{job}` and `/api/v1/jobs/status`; add the new manual-trigger route)
    - services/scheduler/cmd/scheduler-api/main.go (full file — the boot order: db → cache → repos → jobs → service → handler → router; insert the canary job alongside the others)
    - libs/metrics/scheduler.go (full file — re-use SchedulerJobExecutionsTotal/Duration/LastSuccess for the canary, label `job="scraper_playability_canary"`)
  </read_first>
  <behavior>
    - Test (unit, service/job_test.go — create if absent): adding `scraper_playability_canary` cron to JobService.Start succeeds with a valid cron string `0 3 * * *`; an invalid cron string returns an error from JobService.Start so boot aborts (matches existing error-handling pattern).
    - Test (handler-level integration): `POST /api/v1/jobs/scraper_playability_canary` returns 202 Accepted with body `{"status":"triggered"}`; the job's `lastRun` timestamp updates within 5s when invoked against a mocked job that returns nil error.
    - Behavior assertion (manual smoke deferred to Plan 23-03's after-update step): the running scheduler container's `/metrics` endpoint exposes `scheduler_job_executions_total{job="scraper_playability_canary",status="success"}` after a manual trigger.
  </behavior>
  <action>
    1. **Edit services/scheduler/internal/service/job.go** — extend struct:
       ```go
       type JobService struct {
           cron                       *cron.Cron
           shikimoriJob               *jobs.ShikimoriSyncJob
           cleanupJob                 *jobs.CleanupJob
           topAnimeJob                *jobs.TopAnimeSyncJob
           calendarJob                *jobs.CalendarSyncJob
           scraperPlayabilityCanaryJob *jobs.ScraperPlayabilityCanaryJob
           log                        *logger.Logger
           lastShikimoriRun           time.Time
           lastCleanupRun             time.Time
           lastTopAnimeRun            time.Time
           lastCalendarRun            time.Time
           lastCanaryRun              time.Time
       }
       ```
       Update `NewJobService` to take the canary job. Update `Start` to accept a fifth cron string (or change signature to accept `*config.JobsConfig` — verify which is the lower-churn change against the call site in main.go and pick that one):
       ```go
       _, err = s.cron.AddFunc(canaryCron, func() {
           ctx := context.Background()
           s.log.Info("starting scheduled scraper playability canary")
           start := time.Now()
           if err := s.scraperPlayabilityCanaryJob.Run(ctx); err != nil {
               metrics.SchedulerJobExecutionsTotal.WithLabelValues("scraper_playability_canary", "error").Inc()
               metrics.SchedulerJobDuration.WithLabelValues("scraper_playability_canary").Observe(time.Since(start).Seconds())
               s.log.Errorw("scraper playability canary failed", "error", err)
           } else {
               metrics.SchedulerJobExecutionsTotal.WithLabelValues("scraper_playability_canary", "success").Inc()
               metrics.SchedulerJobDuration.WithLabelValues("scraper_playability_canary").Observe(time.Since(start).Seconds())
               metrics.SchedulerJobLastSuccess.WithLabelValues("scraper_playability_canary").SetToCurrentTime()
               s.lastCanaryRun = time.Now()
               s.log.Info("scraper playability canary completed successfully")
           }
       })
       if err != nil {
           return err
       }
       ```
       Add `TriggerScraperPlayabilityCanary(ctx context.Context)` mirroring the existing Trigger* methods. Extend `GetStatus()` to surface `"scraper_playability_canary": {"last_run": s.lastCanaryRun}`.
    2. **Edit services/scheduler/internal/handler/job.go** — add `TriggerScraperPlayabilityCanary(w http.ResponseWriter, r *http.Request)` matching existing handler shape (write `{"status":"triggered"}` 202 + spawn the trigger in a goroutine via `go h.service.TriggerScraperPlayabilityCanary(context.Background())`).
    3. **Edit services/scheduler/internal/transport/router.go** — add route:
       ```go
       r.Post("/api/v1/jobs/scraper_playability_canary", jobHandler.TriggerScraperPlayabilityCanary)
       ```
       Place it near the other `/api/v1/jobs/<name>` routes (search for `TriggerCalendarSync` to find the cluster).
    4. **Edit services/scheduler/cmd/scheduler-api/main.go** — insert:
       ```go
       canaryJob := jobs.NewScraperPlayabilityCanaryJob(db.DB, &cfg.Jobs, log)
       ```
       just after the existing `calendarJob := ...` line. Update the `jobService := service.NewJobService(...)` call to include `canaryJob`. Update the `jobService.Start(...)` call to pass `cfg.Jobs.ScraperPlayabilityCanaryCron` (and the existing 4 cron strings) — or refactor `Start` to take `*config.JobsConfig` directly to keep the signature lean (decide based on existing churn).
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/scheduler && go build ./... && go test -count=1 -race ./internal/service/... ./internal/handler/... ./internal/jobs/... 2>&1 | tail -20</automated>
  </verify>
  <done>`go build ./...` succeeds; existing scheduler tests still pass; new canary tests pass; `grep -n "scraper_playability_canary" services/scheduler/internal/service/job.go services/scheduler/internal/transport/router.go services/scheduler/cmd/scheduler-api/main.go` returns ≥ 3 lines confirming all three wiring points are present.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| scheduler → scraper (in-cluster HTTP) | Same docker-compose network; no auth required currently. Canary inherits the scraper's existing trust posture. |
| scheduler → upstream HLS CDNs (via streamprobe) | streamprobe already enforces SSRF guard (libs/streamprobe/probe.go isPublicHost). The canary inherits this — no additional defense needed at canary layer. |
| canary → player_reports volume | Same volume already used by services/player. Canary writes files; never reads other services' files. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-23-01 | I (Information disclosure) | services/scheduler/internal/jobs/scraper_playability_canary.go writeRunLog | mitigate | Per CONTEXT.md security note: redact Authorization / Cookie / Set-Cookie headers from logged response headers before json.Marshal. Implement `redactHeaders(http.Header) http.Header` helper + assert via `TestCanary_WritesPerRunLog` that the log file body does NOT contain secret-bearing strings. |
| T-23-02 | D (DoS) | upstream anitaku.to / vibeplayer.site / streamhg.site rate-limit retaliation | mitigate | ±5 min jitter on the 03:00 cron tick (`computeJitter() ∈ [-5min, +5min]`); reuse same client+UA the scraper uses (no new fingerprint); 5 anime × 3 servers = 15 cold-path requests per run — well below human-traffic floor. Off-peak (03:00 local) chosen for low correlation with normal traffic. |
| T-23-03 | T (Tampering) | Canary anime-list query SQL injection via watch_history rows | accept | Query is parameterless raw SQL (no string interpolation); SELECT only against trusted internal table; no user-controlled input. Standard parameterized query practice. |
| T-23-04 | I + D | Metric cardinality bomb if `server` label leaks raw URLs | mitigate | `server` label value sourced ONLY from `domain.Server.ID` (which is the embed extractor's normalized name: vibeplayer / streamhg / earnvids — bounded set). Sentinel `_unreachable` used when /scraper/servers fails. Cardinality budget noted in libs/metrics/parser.go counter doc-comment + asserted via the AnimeSlots() constant slice. |
| T-23-05 | E (Elevation of privilege) | canary acquires DB write access via gorm.DB | accept | Canary only executes SELECT queries against watch_history + animes; no INSERT/UPDATE/DELETE. Code review at execution time confirms. |

All ASVS L1 considerations: read-only DB access, no user-supplied input, SSRF guarded by streamprobe, secret redaction enforced by test.
</threat_model>

<verification>
- `cd /data/animeenigma/services/scheduler && go build ./... && go test -count=1 -race ./...` exits 0.
- `cd /data/animeenigma/libs/metrics && go test -count=1 ./...` exits 0.
- `grep -n "playability_canary_runs_total" libs/metrics/parser.go` returns 1+ line.
- `grep -n "AnimeSlots" libs/metrics/parser.go` returns 1+ line; the function body returns exactly the five literal strings.
- `grep -n '0 3 \* \* \*' services/scheduler/internal/config/config.go` returns the line setting the canary cron default.
- `grep -n "ScraperPlayabilityCanary" services/scheduler/cmd/scheduler-api/main.go services/scheduler/internal/service/job.go` returns ≥ 3 lines (constructor call + service-struct field + AddFunc).
- `grep -c "player_reports:/data/reports" docker/docker-compose.yml` returns 2 (player service already mounts it + scheduler now mounts it).
</verification>

<success_criteria>
- All 3 tasks pass their `<verify>` automated commands.
- Phase 23 ROADMAP Success Criteria #1: `services/scheduler/internal/jobs/scraper_playability_canary.go` runs daily at 03:00 with ±5 min jitter, anime list composition matches the four CONTEXT.md branches (anchors-only, anchors+recent, anchors+fallback-anime_list, both-empty warning), per-run logs persist to player_reports volume — VERIFIED via 8 unit tests.
- Phase 23 ROADMAP Success Criteria #2: scheduler `/metrics` exposes `playability_canary_runs_total` with all 5 expected labels and 5 expected anime_slot values — VERIFIED via libs/metrics tests + the canary's per-tuple emission test.
- Threat model: T-23-01 (header redaction), T-23-02 (jitter), T-23-04 (cardinality) all mitigated and asserted by test.
</success_criteria>

<output>
After completion, create `.planning/phases/23-self-maintenance-loop/23-01-SUMMARY.md`.
</output>
