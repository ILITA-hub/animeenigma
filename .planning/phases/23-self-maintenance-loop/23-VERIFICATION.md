---
phase: 23
phase_name: Self-Maintenance Loop
status: human_needed
verified_date: 2026-05-13
goal_alignment_score: 9/10
must_haves_met: 5/6
---

# Phase 23 Verification — Self-Maintenance Loop

**Phase Goal:** A regression at any upstream site is detected within 24 hours by a daily canary that exercises real production code paths, surfaces a labeled alert into the existing `services/maintenance` bot, and gets dispatched per `.claude/maintenance-prompt.md` Patterns 6/7 — without a human needing to notice.

**Verified:** 2026-05-13
**Status:** human_needed
**Re-verification:** No — initial verification

---

## Status

**human_needed.** Phase 23 ships the full automated foundation — daily canary cron at 03:00 local with ±5 min jitter, the `playability_canary_runs_total{provider, server, result, reason, anime_slot}` metric live on `:8085/metrics`, a 4-panel Grafana dashboard `scraper-provider-health-canary` under the `Self-Healing` folder, three Prometheus alert rules (`ScraperPlayabilityRegression` warning, `ScraperAdDecoySurge` warning, `ScraperUnplayableSpike` critical) loaded and routing through the default policy to the `maintenance-webhook` contact point, 11 new tests passing under `-race`, and `.claude/maintenance-prompt.md` confirmed unmodified (D6) yet still carrying every required Pattern 6/7 + Reason enum string locked in place by symbol-stability tests.

The single outstanding gate is the **real-flow round-trip smoke** explicitly carried over from Plan 23-03 Task 4 (`checkpoint:human-verify`): trigger the canary against the live scheduler → wait for Grafana evaluation → confirm the maintenance bot posts a Pattern 6/7 diagnosis with tier `button_fix` to Telegram. The synthetic webhook tests verify the payload round-trips through `webhookHandler` with labels intact (T-23-09/T-23-10 isolated via `httptest.NewServer`) but they intentionally do NOT invoke the dispatcher — so SC #4's literal "maintenance bot identifies Pattern 6, names fix paths, tiers as button_fix" is verified upstream (via the prompt-symbol-stability tests proving the prompt content the bot consumes is correct) but NOT end-to-end live.

Pre-deploy automated verification PASSED across the board (PromQL parses, alert rules loaded, contact-point linked, tests green, metrics live, dashboard panels render, JSON logs persisted to the player_reports volume).

---

## Goal Alignment

**9/10.** Every artifact called for by the phase goal exists, is wired into the running system, and produces live data on the production stack. The only thing missing is a human pressing the canary trigger and watching the Telegram message arrive — the Task 4 manual gate the user explicitly chose to defer.

---

## Success Criteria Trace

| # | Success Criterion | Status | Evidence |
|---|---|---|---|
| 1 | Scheduler canary cron at 03:00 local with jitter + manual trigger + 5-label counter + per-run JSON log | VERIFIED | `services/scheduler/internal/jobs/scraper_playability_canary.go` exists with `Run()` (jittered) + `RunNoJitter()` (manual). `SCRAPER_PLAYABILITY_CANARY_CRON: "0 3 * * *"` in `docker/docker-compose.yml` + default in `services/scheduler/internal/config/config.go:91`. `libs/metrics/parser.go:63` registers `PlayabilityCanaryRunsTotal` with 5 labels `{provider, server, result, reason, anime_slot}`. `POST /api/v1/jobs/scraper_playability_canary` wired via `services/scheduler/internal/transport/router.go:50` → `internal/handler/job.go:57` → `internal/service/job.go:222`. Live: `curl -s http://localhost:8085/metrics \| grep playability_canary_runs_total` returns 3 series. `docker exec animeenigma-scheduler ls /data/reports/canary-runs/` shows 2 JSON files persisted on player_reports volume. AnchorFrierenMAL=52991 + AnchorOnePieceMAL=21 hardcoded. composeAnimeList uses watch_history query with anime_list fallback (lines 265+ in canary.go). |
| 2 | Grafana dashboard `scraper-provider-health.json` with 4 panels under "Self-Healing" folder | VERIFIED | `infra/grafana/dashboards/scraper-provider-health.json` exists, schemaVersion 38, uid `scraper-provider-health-canary`, 4 panels: "Pass / Fail per Provider/Server (24h)" (barchart), "Failure Reason Breakdown (24h)" (barchart), "Last Canary Run" (stat), "Top Failing (provider, server, reason) Tuples" (table topk). Live: `curl -s -u admin:admin http://localhost:3004/admin/grafana/api/search?query=Scraper` returns the dashboard under `folder: Self-Healing`. Provisioning wired via `docker/grafana/provisioning/dashboards/dashboards.yml` `infra-self-healing` provider + `docker/docker-compose.yml` `../infra/grafana/dashboards:/var/lib/grafana/dashboards-infra:ro` mount. |
| 3 | Three Prometheus alert rules with provider/server/reason labels, routing to `maintenance-webhook` contact point; PromQL parses live | VERIFIED | `infra/grafana/alerts/scraper.yaml` defines all 3 rules with correct severities (Regression=warning, AdDecoy=warning, Spike=critical) and templated `provider`/`server`/`reason` annotations. Inline-mirrored into `docker/grafana/provisioning/alerting/rules.yml` under group `Scraper Self-Healing` with "KEEP IN SYNC" pointer. Live: `curl -s -u admin:admin http://localhost:3004/api/v1/provisioning/alert-rules` returns ScraperPlayabilityRegression, ScraperAdDecoySurge, ScraperUnplayableSpike. All 3 PromQL expressions parse `success` against live Prometheus at `:9090/prometheus/api/v1/query`. Default policy → `maintenance-webhook` contact point → host-side maintenance daemon on `:8087/api/grafana-webhook` (BasicAuth via `GRAFANA_WEBHOOK_PASS`). |
| 4 | Synthetic Pattern 6 dispatch test: maintenance bot identifies Pattern 6, names fix paths (server-priority reorder, WARP toggle, mark-degraded), tiers as button_fix; wrapped in MAINTENANCE_TEST_MODE=dry_run | PARTIAL — payload round-trip verified, dispatcher invocation deferred to human smoke | `services/maintenance/internal/transport/webhook_synthetic_test.go` ships 4 tests (`TestWebhook_SyntheticPattern6_Accepted`, `..._Pattern7_Accepted`, `..._RequiredLabels_PresentInDispatched`, `..._AuthRejected_NoSubmit`). All 4 pass under `-race`. **However** they verify only that `webhookHandler` round-trips the labels through `submitAlert` — they do NOT invoke the `internal/dispatcher` package and therefore do NOT directly assert "identifies Pattern 6 / names fix paths / tiers as button_fix". `MAINTENANCE_TEST_MODE` exists as a future-hook `bool` field in `internal/config/config.go:25` (+ 3 config tests) but the runtime dispatcher is NOT gated by it yet — synthetic tests isolate via `httptest.NewServer` instead, which is T-23-09/T-23-10 compliant. The dispatcher contract is verified upstream via the prompt-symbol-stability tests (#5 below) which prove Pattern 6/7 sections + the `ad_decoy → button_fix` mapping in the prompt the dispatcher consumes. End-to-end dispatcher round-trip is the Task 4 deferred manual gate. |
| 5 | Maintenance-prompt symbol-stability: every Reason enum value (7) + `computeStreamTTL` + `cacheStream` appear in `.claude/maintenance-prompt.md` | VERIFIED (with one Rule 1 deviation — see below) | `services/maintenance/internal/classifier/maintenance_prompt_symbols_test.go` ships 4 tests: `TestMaintenancePrompt_FilePresentInWorkingDir`, `..._ContainsPatterns6And7` (Pattern 6 + Pattern 7 + "Scraper Playability Regression" section headings), `..._AllReasonsCovered` (iterates `streamprobe.AllReasons()` — all 7 values: playable, ad_decoy, zero_match, status_403, signed_url_expired, cdn_unreachable, empty_response — alias-aware match for status_403/403_upstream), `TestScraperGoSymbols_StillExist` (asserts at least one of `cacheStream` or `computeStreamTTL` exists in non-test .go under `services/scraper/internal/providers/gogoanime/` — slash-alternative semantics matching the prompt's own phrasing). All 4 tests pass. Live grep confirms `computeStreamTTL` exists at `services/scraper/internal/providers/gogoanime/cache.go:46` and is called at `client.go:736`. **Rule 1 deviation:** `cacheStream` symbol does NOT exist in the gogoanime package (only `computeStreamTTL` does). Plan 23-03 interpreted the maintenance-prompt line 173 `cacheStream / computeStreamTTL` slash-alternative as semantic OR; the test passes today via `computeStreamTTL`. Follow-up: either extract a `cacheStream` helper or amend the prompt to drop the dead reference. Either fix passes the existing test. |
| 6 | `.claude/maintenance-prompt.md` NOT modified in this phase (`git diff --quiet` exits 0) | VERIFIED | `git diff --quiet .claude/maintenance-prompt.md` exits 0 against working tree. `git log -1 --format=%H -- .claude/maintenance-prompt.md` resolves to `0d586aa` (commit `docs(13): summary + verification — Phase 13 ... watchlist`) — well before Phase 23 began. `.claude/maintenance-prompt.md` does NOT appear in the current `git status --short` output. The maintenance-state.json (state cursor, not prompt) is modified, but the prompt itself is clean. D6 holds end-to-end. |

---

## Requirements Trace

| Requirement | Plan | Status | Evidence |
|---|---|---|---|
| SCRAPER-HEAL-12 | 23-01 | SATISFIED | `services/scheduler/internal/jobs/scraper_playability_canary.go` — daily cron, 2 anchors + 3 dynamic from watch_history (fallback to anime_list when empty), per-run JSON log to player_reports volume. Confirmed via live `/metrics` + `docker exec ls /data/reports/canary-runs/`. |
| SCRAPER-HEAL-13 | 23-01 | SATISFIED | `libs/metrics/parser.go:63` `PlayabilityCanaryRunsTotal{provider, server, result, reason, anime_slot}`; `AnimeSlots()` returns the exact 5 slot literals. Live counter series confirmed. |
| SCRAPER-HEAL-14 | 23-02 | SATISFIED | `infra/grafana/dashboards/scraper-provider-health.json` with 4 panels matching SC #3 wording (provider/server stacked bar, reason breakdown, last-run timestamp, top failing tuples). Live discoverable in Grafana under `Self-Healing` folder. |
| SCRAPER-HEAL-15 | 23-03 | SATISFIED | `infra/grafana/alerts/scraper.yaml` — 3 rules with severity + provider/server/reason annotations; loaded into Grafana via inline-mirror in `rules.yml`; PromQL parses live; default policy routes to `maintenance-webhook` → host `:8087/api/grafana-webhook`. |
| SCRAPER-HEAL-16 | 23-03 | SATISFIED | `.claude/maintenance-prompt.md` carries Pattern 6 + Pattern 7 + "Scraper Playability Regression" section + all 7 Reason enum values + `computeStreamTTL` reference. Symbol-stability test suite (4 tests) locks this. Prompt file unmodified in this phase per D6. |

No ORPHANED requirements — every ID mapped to v3.1 Phase 23 in `v3.1-REQUIREMENTS.md` is claimed and verified by at least one plan.

---

## Anti-Pattern Scan

Files modified in this phase (per SUMMARY key-files):

| File | Patterns Found | Severity | Notes |
|---|---|---|---|
| `services/scheduler/internal/jobs/scraper_playability_canary.go` | None | — | No TODOs, FIXMEs, empty returns, or placeholder strings in the production code path. Uses injectable collaborators; SQL is parameterized; secrets redacted before disk persistence. |
| `libs/metrics/parser.go` (modified) | None | — | Standard `promauto.NewCounterVec` registration with documented cardinality bound (210 series) and clear label semantics. |
| `infra/grafana/dashboards/scraper-provider-health.json` | None | — | Valid Grafana 10.3.3 schemaVersion 38 JSON, parses via `jq -e .`. |
| `infra/grafana/alerts/scraper.yaml` | None | — | Templated annotations from `$labels.*`; cardinality bound documented in the file header. |
| `services/maintenance/internal/transport/webhook_synthetic_test.go` | None | — | `httptest.NewServer` isolation prevents accidental hits on the live `:8087` daemon. |
| `services/maintenance/internal/classifier/maintenance_prompt_symbols_test.go` | One `t.Logf` breadcrumb for the cacheStream gap | Info | Documented Rule 1 deviation; not a code smell — the breadcrumb is the intended UX for the slash-alternative semantics. |
| `services/maintenance/internal/config/config.go` | None | — | `TestMode` field is wired with a strict `getEnvBool` helper; documented as a future-hook. |
| `docker/docker-compose.yml` | None | — | Standard volume mount additions; `depends_on: scraper` for scheduler ensures startup ordering. |

No blockers, no warnings beyond the already-noted Rule 1 follow-up.

---

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Counter live on scheduler `/metrics` | `curl -s http://localhost:8085/metrics \| grep -c '^playability_canary_runs_total'` | 3 series | PASS |
| Manual canary trigger works | Triggered via plan; per-run JSON log produced | 2026-05-13-{070709,071048}.json on player_reports volume | PASS |
| Alert rules loaded in Grafana | `curl -s -u admin:admin http://localhost:3004/api/v1/provisioning/alert-rules` | All 3 new rules + the existing Stream-Segment-Down listed | PASS |
| Grafana dashboard live | `curl -s -u admin:admin http://localhost:3004/admin/grafana/api/search?query=Scraper` | Returns `Scraper / Provider Health` under `Self-Healing` folder, uid `scraper-provider-health-canary` | PASS |
| All 4 dashboard panels load | `curl -s -u admin:admin http://localhost:3004/api/dashboards/uid/scraper-provider-health-canary` | 4 panel titles returned | PASS |
| PromQL #1 parses live | `curl /prometheus/api/v1/query?query=...regression...` | `"status":"success"` | PASS |
| PromQL #2 parses live | `curl /prometheus/api/v1/query?query=...ad_decoy...` | `"status":"success"` | PASS |
| PromQL #3 parses live | `curl /prometheus/api/v1/query?query=...unplayable.ratio...` | `"status":"success"` | PASS |
| Maintenance tests pass under -race | `cd services/maintenance && go test -race -count=1 ./internal/...` | classifier/config/transport all OK | PASS |
| D6 — maintenance-prompt unmodified | `git diff --quiet .claude/maintenance-prompt.md && echo UNMODIFIED` | UNMODIFIED in working tree | PASS |
| Real-flow alert dispatch round-trip | Manual: trigger canary → wait 2min → check Grafana alert state → confirm Telegram | DEFERRED — Task 4 human-verify checkpoint | SKIP (human) |

---

## Human Verification Required

### 1. Real-Flow Alert → Telegram Round-Trip Smoke (Task 4 deferred gate)

**Test:**
1. `make restart-grafana` (refresh provisioning).
2. `curl -X POST http://localhost:8085/api/v1/jobs/scraper_playability_canary` — manual canary trigger (uses `RunNoJitter`, completes in <2s).
3. Wait ~2 minutes for Grafana to evaluate the rules at its 1-minute interval (so 2 evaluations land).
4. Visit `http://localhost:3004/alerting/list` and confirm `ScraperPlayabilityRegression` is in `Firing` state (the current canary stack returns fail/zero_match/_unreachable so this rule must fire).
5. Within ~30s of the rule firing, check the configured Telegram chat (`TELEGRAM_ADMIN_CHAT_ID`) for a maintenance-bot message diagnosing Pattern 6 or Pattern 7.

**Expected:**
- Telegram message present.
- Message names `Pattern 6` or `Pattern 7` (depending on which reason the canary surfaced — current state is `zero_match` which maps to Pattern 7 in the prompt).
- Message proposes a `button_fix` tier action and names at least one of the fix paths in the prompt (selector / packed-JS adjust / allowlist add / server-priority reorder).
- No errors in `services/maintenance` host-side logs.

**Why human:**
The synthetic webhook tests (transport package) verify the labeled payload round-trips through `webhookHandler` but do not invoke the dispatcher (deliberately — T-23-09/T-23-10 isolation). The full live dispatch round-trip touches Grafana evaluation timers, the maintenance daemon on the host (`:8087`), Telegram API, and the prompt-driven classifier — none of which a unit test can stand up cleanly without dispatching real Telegram messages. This is the Task 4 `checkpoint:human-verify` the user opted to defer to post-deploy smoke.

---

## Deviations

### Rule 1 — `cacheStream` symbol does not exist in gogoanime package (Plan 23-03)

The maintenance-prompt line 173 says "search for `cacheStream` / `computeStreamTTL`" — slash-alternative phrasing. Only `computeStreamTTL` actually exists in `services/scraper/internal/providers/gogoanime/cache.go:46`. Plan 23-03 interpreted the slash as semantic OR and shipped the symbol-stability test as "at least one of the two must exist". Test passes today via `computeStreamTTL`. Follow-up filed (see Plan 23-03 SUMMARY §Follow-up): either rename an inline cache helper to `cacheStream` or amend the prompt to drop the dead reference. Both options preserve the existing test green.

**Verifier disposition:** ACCEPTED. The interpretation is reasonable and the slash phrasing in the prompt supports it. The test would fail loud if both symbols disappeared, which is the actual safety property the must-have was guarding.

### Rule 3 — Dashboards mount sibling path, not nested (Plan 23-02)

Plan 23-02 spec asked for `/var/lib/grafana/dashboards/infra` but that path is inside an already read-only mount. Shipped at `/var/lib/grafana/dashboards-infra` instead. Functional intent (auto-load from new directory) is achieved. Documented in the README.

**Verifier disposition:** ACCEPTED. The plan's intent ("load the new dashboards from `infra/grafana/dashboards`") is met; the literal mount path was a structurally impossible call by the planner. The provisioning file was kept in sync.

### Other note — c745568 concurrent commit (Plan 23-03)

Plan 23-03 Task 1 files (`infra/grafana/alerts/scraper.yaml`, README, `rules.yml`, docker-compose mount) were bundled into commit `c745568` (titled `feat(ui-ux-audit/19): Grafana dashboard naming pass`) due to concurrent index writes between the Phase 23 wave-3 session and a parallel UI-UX audit wave-3 session both committing to `main`. Content is verbatim, co-authors correct, but the commit message does not name Plan 23-03. Documented in Plan 23-03 SUMMARY §Other operational notes. Not a content gap — only a commit-message gap.

**Verifier disposition:** INFORMATIONAL. The artifact content is correct on `main`; an audit reader needs the breadcrumb to find the right commit.

---

## Verdict

Phase 23 ships every artifact the goal demands, every artifact is wired into the running production stack, every artifact emits live data, and every automated test passes under `-race`. The maintenance-prompt is unmodified (D6) yet locked to live Go code by 4 symbol-stability tests. The five MUST-HAVE Success Criteria are all functionally met in the codebase + live stack.

The deferred Task 4 manual gate — trigger the canary live → watch Grafana fire → confirm the maintenance bot posts a Pattern 6/7 diagnosis with tier `button_fix` to Telegram — is the only piece a human still has to press. Per the verification-context instructions, that gates the verdict to `human_needed`, not `passed`.

Recommended next step: run the Human Verification test above. If it passes, this VERIFICATION.md can be re-resolved to `passed` and the milestone v3.1 audit can proceed (`/gsd-audit-milestone`).

---

_Verified: 2026-05-13_
_Verifier: Claude (gsd-verifier, opus-4.7)_
