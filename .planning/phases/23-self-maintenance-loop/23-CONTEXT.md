# Phase 23: Self-Maintenance Loop — Context

**Gathered:** 2026-05-13
**Status:** Ready for planning (`/gsd-plan-phase --phase 23`)
**Milestone:** v3.1 Scraper Self-Healing
**Spec:** `docs/plans/2026-05-13-scraper-self-healing-spec.md`
**Depends on:** Phase 21 + Phase 22 complete (streamprobe + per-server fallback + multi-URL extraction shipping)

<domain>
## Phase Boundary

A regression at any upstream site (anitaku.to, vibeplayer.site, streamhg, earnvids, future) is detected within 24 hours by a daily canary that exercises real production code paths, surfaces a labeled alert into the existing maintenance bot, and gets dispatched per `.claude/maintenance-prompt.md` Patterns 6/7 — without a human needing to notice.

**Concretely, this phase delivers:**

1. New scheduler job `services/scheduler/internal/jobs/scraper_playability_canary.go` (flat file alongside existing `anime_loader.go`, `calendar.go`, `cleanup.go`, etc.). Cron: 03:00 daily local time.
2. Canary anime list, refreshed every run:
   - **2 fixed anchors**: Frieren: Beyond Journey's End, One Piece (long-running and popular — drift here is drift in prod).
   - **3 dynamic from recent global watch_history** (catches "what users actually hit"): `SELECT DISTINCT anime_id FROM watch_history WHERE created_at > NOW() - INTERVAL '24h' ORDER BY created_at DESC LIMIT 3`. JOIN against `animes` for MAL id + title. Fallback to top-3 from `anime_list ORDER BY updated_at DESC` when watch_history is empty.
3. Per run: for each anime × every server returned by `gogoanime.ListServers` for episode 1, call `/scraper/stream` and run `libs/streamprobe.Probe` against the returned URL. Record pass/fail + reason per (provider, server, anime_slot).
4. New metric `playability_canary_runs_total{provider, server, result, reason, anime_slot}` where `anime_slot ∈ {anchor_frieren, anchor_one_piece, recent_1, recent_2, recent_3}` and `result ∈ {pass, fail}`. Reason mirrors `streamprobe.ReasonEnum` from Phase 21.
5. Per-run log persisted to the `player_reports` Docker volume (same volume already used by `services/player/internal/handler/report.go`) for post-mortem inspection.
6. New Grafana dashboard `infra/grafana/dashboards/scraper-provider-health.json` with: stacked bar per provider/server (pass/fail counts per 24h), reason breakdown panel, last-canary-run timestamp, top failing (provider, server, reason) tuples.
7. Three Prometheus alert rules in `infra/grafana/alerts/scraper.yaml`, all routing to the existing `services/maintenance` webhook `/api/grafana-webhook`:
   - `ScraperPlayabilityRegression` (warning) — any canary `fail` in last 25 h
   - `ScraperAdDecoySurge` (warning) — `rate(parser_ad_decoy_total[5m]) > 0` sustained 5 min
   - `ScraperUnplayableSpike` (critical) — `rate(parser_unplayable_total[5m]) / rate(scraper_getstream_total[5m]) > 0.05` sustained 5 min
   Alert labels MUST include `provider`, `server`, `reason` — the maintenance bot's reason-enum dispatch table depends on these labels being present.
8. Verification that the maintenance bot already routes the new alerts: a synthetic Pattern 6 alert injection into `/api/grafana-webhook` produces a maintenance response that (a) identifies Pattern 6 by signature, (b) names the correct fix paths (server-priority reorder, WARP toggle, mark-degraded), (c) tiers as `button_fix` for the code-edit path. `.claude/maintenance-prompt.md` was updated 2026-05-13 with the required content — this phase confirms it still parses correctly and the dispatcher returns the expected JSON.

**Out of scope:**
- WARP egress sidecar — separate spec when there is appetite to revive VibePlayer.
- Multi-tenant / per-user canary — aggregate-only, single-tenant.
- Auto-applying fixes (true self-healing without admin click) — the bot proposes `button_fix`; admin approval stays in the loop per existing maintenance design.
- Slack / PagerDuty / email routing — Telegram via maintenance bot is the only channel.

**Requirements covered:**
- SCRAPER-HEAL-12 (canary cron job)
- SCRAPER-HEAL-13 (playability_canary_runs_total metric)
- SCRAPER-HEAL-14 (Grafana dashboard)
- SCRAPER-HEAL-15 (three alert rules → maintenance webhook)
- SCRAPER-HEAL-16 (verify maintenance prompt is in place — pre-shipped 2026-05-13)

</domain>

<decisions>
## Implementation Decisions

### D1 — Canary lives in `services/scheduler/`, not `services/scraper/`

Reason: scheduler is the existing home for periodic jobs (`anime_loader.go`, `calendar.go`, `cleanup.go`). Adding it to scraper would mix the "serve user requests" and "exercise yourself for monitoring" concerns. Scheduler already has DB access (needed for watch_history query) and Redis access (needed if winning-server cache prep is desired).

Trade-off accepted: when scheduler crashes the canary stops emitting metrics. Prometheus' `absent_over_time(playability_canary_runs_total[26h])` query covers the dead-canary case via a separate alert rule (added to Phase 23 alerts).

### D2 — Anime list is built fresh each run, not pinned to a static list

Reason: user feedback (2026-05-13) explicitly: "do frieren, one piece and 3 latest watched on animeenigma (get new every time)". This catches regressions on titles users are actually hitting, not just curated anchors. Anchors guarantee 2 stable signals; dynamic adds population-of-the-day coverage.

### D3 — Canary calls `/scraper/stream` over HTTP, not in-process

Reason: exercises the same code path users hit (gateway routing, JSON marshaling, HTTP middleware, response cache). An in-process call would skip middleware bugs that only appear at the HTTP layer. Costs ~10 extra ms per call, fully acceptable at canary cadence.

### D4 — `anime_slot` label uses literal string values, not numeric indexes

Reason: Grafana panel readability. `anime_slot="anchor_frieren"` is self-describing; `anime_slot="0"` requires a legend lookup. The cost is a slightly larger label-cardinality budget (5 values, fixed) — well within Prometheus comfort.

### D5 — Failed canary persists evidence to disk, not just metrics

Reason: when the maintenance bot is dispatched against a Pattern 6/7 alert, it needs to see the actual m3u8 body / failing segment hostnames / packed-JS dictionary to diagnose. A persisted per-run log in `player_reports` volume gives the bot something to `cat` instead of having to re-fetch and risk getting different (e.g., now-rotated) content.

### D6 — Maintenance prompt is treated as already-shipped, not modified again in this phase

Reason: `.claude/maintenance-prompt.md` Patterns 6/7 + Scraper Playability Regression section were added 2026-05-13 as part of spec sign-off. This phase only verifies the prompt still parses + dispatches correctly via the synthetic-alert test. If the dispatcher fails the test, the fix is to amend the prompt — but no proactive edits in this phase.

</decisions>

<open_questions>
None.
</open_questions>

<risks>
## Risks specific to this phase

- **Canary anitaku.to traffic looks suspicious**: 5 anime × N servers nightly is low volume, but consistent. Mitigation: run at 03:00 local (off-peak); reuse the same scraper HTTP client (cookie jar, rate limiter); add ±5 min jitter to the cron tick to avoid 03:00:00 fingerprinting. Confirm in plan.
- **watch_history-driven query is empty on a brand-new install or quiet day**: Fallback path is `anime_list ORDER BY updated_at DESC LIMIT 3`. Edge case: both empty → log a warning, run only the two anchors. Don't fail the whole run.
- **Alert rule cardinality explosion**: `provider × server × reason × anime_slot` could be ~5 providers × 4 servers × 7 reasons × 5 slots = 700 unique label combinations. In practice currently 1 provider × 3 servers × 7 reasons × 5 slots = 105. Prometheus default limits allow this comfortably, but if v3.2 adds more providers it could matter. Mitigation: explicit cardinality-budget note in the alert config.
- **Synthetic Pattern 6 test crashes the production maintenance bot**: inject the synthetic alert against a STAGING /api/grafana-webhook if one exists, otherwise wrap the test in a feature flag (`MAINTENANCE_TEST_MODE=1`) that the bot recognizes and short-circuits to "dry-run only". Confirm in plan-checker.
- **Maintenance bot fix application falls outside its scope**: e.g., a reason="signed_url_expired" alert tells the bot to change a Go constant, but the const it points at doesn't exist (was renamed in a refactor between prompt update and Phase 21 ship). Mitigation: Phase 21 explicitly preserves the constant name `cacheStream` / `computeStreamTTL` referenced in the prompt; Phase 23 acceptance test asserts these symbols still exist via grep.
</risks>
